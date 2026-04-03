package main

// group_invite.go — Offline-capable group invite protocol
//
// Design (best-practice, matches RFC 9420 KeyPackage distribution model):
//
//  Phase 1 — Advertisement (done once on P2P start, renewed after each use):
//    Invitee generates a KeyPackage → stores private bundle in SQLite.
//    Public KP bytes are published to DHT ("/app/kp/{peerID}").
//    → Creator can fetch the KP at any time, even while invitee is offline.
//
//  Phase 2 — Invite (creator, works while invitee is offline):
//    Creator fetches public KP from DHT → coord.AddMember → gets Welcome bytes.
//    Welcome stored in SQLite (pending_welcomes_out) for retry.
//    Welcome ALSO pushed to DHT ("/app/welcome/{inviteePeerID}/{groupID}") for
//    the invitee to pull on next startup.
//    If invitee happens to be online: also send directly via stream (fast path).
//
//  Phase 3 — Delivery (invitee, online or reconnecting):
//    On startup:  pull own pending Welcomes from DHT → auto-join.
//    On connect:  creator retries undelivered Welcomes via direct stream.
//    Stream handler "/app/welcome-delivery/1.0.0": receive Welcome → auto-join.
//    After join: regenerate + re-advertise a fresh KP so the next invite works.

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"app/p2p"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	welcomeDeliveryProtocol = protocol.ID("/app/welcome-delivery/1.0.0")
	maxWelcomeFrame         = 4 << 20 // 4 MiB
)

// welcomeDeliveryWire is the JSON payload for /app/welcome-delivery/1.0.0.
type welcomeDeliveryWire struct {
	V          int    `json:"v"`
	GroupID    string `json:"group_id"`
	WelcomeHex string `json:"welcome_hex"`
}

// ── Frame I/O ─────────────────────────────────────────────────────────────────

func writeWelcomeFrame(w io.Writer, msg *welcomeDeliveryWire) error {
	data, _ := json.Marshal(msg)
	if len(data) > maxWelcomeFrame {
		return fmt.Errorf("frame too large")
	}
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(data)))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func readWelcomeFrame(r io.Reader) (*welcomeDeliveryWire, error) {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 || n > maxWelcomeFrame {
		return nil, fmt.Errorf("invalid frame length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	var msg welcomeDeliveryWire
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ── Phase 1: KeyPackage advertisement ─────────────────────────────────────────

// advertiseKeyPackage generates (or reuses) the local KeyPackage, stores the
// private bundle in SQLite, and publishes the public bytes to DHT.
// Safe to call multiple times — regenerates only when the existing KP is absent.
func (a *App) advertiseKeyPackage() {
	a.mu.Lock()
	node := a.node
	database := a.db
	a.mu.Unlock()
	if node == nil || database == nil {
		return
	}

	localID := node.Host.ID()

	// Reuse existing KP if already advertised.
	existing, _, err := database.GetKPBundle(localID.String())
	if err == nil && len(existing) > 0 {
		// Re-push to DHT in case the entry expired.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err2 := p2p.AdvertiseKeyPackage(ctx, node.DHT, localID, existing); err2 != nil {
			slog.Warn("Re-advertise KP to DHT failed (will retry later)", "err", err2)
		} else {
			slog.Info("KeyPackage re-advertised to DHT", "peer", localID)
		}
		return
	}

	kpRes, err := a.GenerateKeyPackage()
	if err != nil {
		slog.Error("advertiseKeyPackage: GenerateKeyPackage failed", "err", err)
		return
	}

	publicKP, _ := hex.DecodeString(kpRes.PublicHex)
	privateBundle, _ := hex.DecodeString(kpRes.BundlePrivateHex)

	if err := database.SaveKPBundle(localID.String(), publicKP, privateBundle); err != nil {
		slog.Error("advertiseKeyPackage: SaveKPBundle failed", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p2p.AdvertiseKeyPackage(ctx, node.DHT, localID, publicKP); err != nil {
		slog.Warn("advertiseKeyPackage: DHT put failed (will retry on next start)", "err", err)
	} else {
		slog.Info("KeyPackage advertised to DHT", "peer", localID)
	}
}

// refreshKeyPackage generates a brand-new KeyPackage and re-advertises it after
// the previous one was consumed by an AddMembers call.
func (a *App) refreshKeyPackage() {
	a.mu.Lock()
	node := a.node
	database := a.db
	a.mu.Unlock()
	if node == nil || database == nil {
		return
	}

	// Delete old KP so advertiseKeyPackage generates fresh.
	_, _ = database.Conn.Exec("DELETE FROM kp_bundles WHERE peer_id = ?", node.Host.ID().String())
	a.advertiseKeyPackage()
}

// ── Phase 2: Invite (creator, offline-capable) ────────────────────────────────

// InvitePeerToGroup fetches the target peer's public KeyPackage from the DHT
// (works even if the peer is offline), performs MLS AddMembers, stores the
// resulting Welcome in both SQLite and DHT, and attempts immediate direct
// delivery if the peer is currently connected.
func (a *App) InvitePeerToGroup(peerIDStr, groupID string) error {
	peerIDStr = strings.TrimSpace(peerIDStr)
	groupID = strings.TrimSpace(groupID)
	if peerIDStr == "" || groupID == "" {
		return fmt.Errorf("peer ID and group ID are required")
	}

	targetID, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("invalid peer ID %q: %w", peerIDStr, err)
	}

	a.mu.Lock()
	node := a.node
	coord, hasGroup := a.coordinators[groupID]
	database := a.db
	a.mu.Unlock()

	if node == nil {
		return fmt.Errorf("P2P node not running")
	}
	if !hasGroup || coord == nil {
		return fmt.Errorf("not in group %q — create the group first", groupID)
	}
	if targetID == node.Host.ID() {
		return fmt.Errorf("cannot invite yourself")
	}

	// Prefer DHT (offline invitee), but on small LANs the routing table is often
	// empty so PutValue/GetValue fails — fall back to a direct stream (Noise).
	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(a.ctx, targetID)
	}

	slog.Info("Fetching KeyPackage", "target", targetID, "group", groupID)
	kpBytes, dhtErr := p2p.FetchKeyPackage(context.Background(), node.DHT, targetID)
	if dhtErr != nil {
		slog.Warn("DHT KeyPackage fetch failed, trying direct kp-offer stream",
			"target", targetID, "dht_err", dhtErr)
		var directErr error
		kpBytes, directErr = p2p.FetchKeyPackageDirect(context.Background(), node.Host, targetID)
		if directErr != nil {
			return fmt.Errorf(
				"could not get KeyPackage for %s: DHT: %v; direct stream: %w.\n\n"+
					"Ensure both nodes show as connected in the Dashboard, wait a few seconds after connect, then retry. "+
					"On small LANs the DHT routing table may be empty — direct fetch needs an active libp2p connection.",
				targetID, dhtErr, directErr)
		}
		slog.Info("KeyPackage fetched via direct stream", "target", targetID)
	}

	// AddMembers (Coordinator + MLS engine).
	welcome, err := coord.AddMember(targetID, kpBytes)
	if err != nil {
		return fmt.Errorf("AddMembers: %w", err)
	}

	// Persist Welcome for store-and-forward.
	if err := database.SavePendingWelcome(targetID.String(), groupID, welcome); err != nil {
		slog.Warn("SavePendingWelcome failed", "err", err)
	}

	// Push Welcome to DHT so invitee can pull it on reconnect.
	if dhtErr := p2p.StoreWelcomeInDHT(context.Background(), node.DHT, targetID, groupID, welcome); dhtErr != nil {
		slog.Warn("StoreWelcomeInDHT failed (SQLite retry still active)", "err", dhtErr)
	}

	// Fast path: deliver immediately if peer is currently online.
	go a.deliverWelcome(targetID, groupID, welcome)

	slog.Info("Group invite sent", "group", groupID, "target", targetID)
	return nil
}

// ── Phase 3a: Welcome delivery (creator → invitee, direct stream) ─────────────

// deliverWelcome attempts to send Welcome bytes to targetID via direct stream.
// Called after InvitePeerToGroup and on every peer-connect event.
func (a *App) deliverWelcome(targetID peer.ID, groupID string, welcomeBytes []byte) {
	a.mu.Lock()
	node := a.node
	a.mu.Unlock()
	if node == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s, err := node.Host.NewStream(ctx, targetID, welcomeDeliveryProtocol)
	if err != nil {
		slog.Debug("deliverWelcome: peer not reachable yet (will retry on connect)", "target", targetID, "err", err)
		return
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))

	msg := &welcomeDeliveryWire{V: 1, GroupID: groupID, WelcomeHex: hex.EncodeToString(welcomeBytes)}
	if err := writeWelcomeFrame(s, msg); err != nil {
		slog.Warn("deliverWelcome: write failed", "target", targetID, "err", err)
		return
	}

	slog.Info("Welcome delivered via direct stream", "group", groupID, "target", targetID)
	// Mark as delivered in DB (best-effort).
	a.mu.Lock()
	database := a.db
	a.mu.Unlock()
	if database != nil {
		rows, _ := database.GetPendingWelcomesFor(targetID.String())
		for _, pw := range rows {
			if pw.GroupID == groupID {
				_ = database.MarkWelcomeDelivered(pw.ID)
			}
		}
	}
}

// retryPendingWelcomes is called when a peer connects; sends all undelivered
// Welcomes stored in SQLite for that peer.
func (a *App) retryPendingWelcomes(targetID peer.ID) {
	a.mu.Lock()
	database := a.db
	a.mu.Unlock()
	if database == nil {
		return
	}

	pending, err := database.GetPendingWelcomesFor(targetID.String())
	if err != nil || len(pending) == 0 {
		return
	}
	slog.Info("Retrying pending Welcomes for reconnected peer", "peer", targetID, "count", len(pending))
	for _, pw := range pending {
		a.deliverWelcome(targetID, pw.GroupID, pw.WelcomeBytes)
	}
}

// ── Phase 3b: Welcome receipt (invitee) ───────────────────────────────────────

// ensureLocalPublicKPBytes returns the public KeyPackage bytes, generating and
// persisting a bundle in SQLite if none exists yet.
func (a *App) ensureLocalPublicKPBytes() ([]byte, error) {
	a.mu.Lock()
	node := a.node
	database := a.db
	a.mu.Unlock()
	if node == nil || database == nil {
		return nil, fmt.Errorf("node or database not ready")
	}
	pid := node.Host.ID().String()
	pub, _, err := database.GetKPBundle(pid)
	if err == nil && len(pub) > 0 {
		return pub, nil
	}
	kpRes, err := a.GenerateKeyPackage()
	if err != nil {
		return nil, err
	}
	publicKP, _ := hex.DecodeString(kpRes.PublicHex)
	privateBundle, _ := hex.DecodeString(kpRes.BundlePrivateHex)
	if err := database.SaveKPBundle(pid, publicKP, privateBundle); err != nil {
		return nil, err
	}
	return publicKP, nil
}

func (a *App) handleKPOfferStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	one := make([]byte, 1)
	if _, err := io.ReadFull(s, one); err != nil || one[0] != 0x01 {
		return
	}

	a.mu.Lock()
	var ap *p2p.AuthProtocol
	if a.node != nil {
		ap = a.node.AuthProtocol
	}
	database := a.db
	a.mu.Unlock()

	if ap != nil && !ap.IsVerified(remote) {
		slog.Warn("kp-offer: rejected unverified peer", "peer", remote)
		return
	}
	if database == nil {
		return
	}

	pub, err := a.ensureLocalPublicKPBytes()
	if err != nil {
		slog.Error("kp-offer: could not produce public KP", "err", err)
		return
	}

	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(pub)))
	if _, err := s.Write(lb[:]); err != nil {
		return
	}
	if _, err := s.Write(pub); err != nil {
		return
	}
	slog.Info("kp-offer: served public KeyPackage", "to", remote, "bytes", len(pub))
}

func (a *App) registerKPOfferHandler() {
	if a.node == nil {
		return
	}
	a.node.Host.SetStreamHandler(p2p.KPOfferProtocol, func(s network.Stream) {
		go a.handleKPOfferStream(s)
	})
	slog.Info("KP offer handler registered", "protocol", string(p2p.KPOfferProtocol))
}

func (a *App) removeKPOfferHandler() {
	if a.node == nil {
		return
	}
	a.node.Host.RemoveStreamHandler(p2p.KPOfferProtocol)
}

// registerWelcomeDeliveryHandler registers the stream handler so the invitee
// auto-joins groups when a Welcome is pushed by the creator.
func (a *App) registerWelcomeDeliveryHandler() {
	if a.node == nil {
		return
	}
	a.node.Host.SetStreamHandler(welcomeDeliveryProtocol, func(s network.Stream) {
		go a.handleWelcomeDelivery(s)
	})
	slog.Info("Welcome delivery handler registered")
}

func (a *App) removeWelcomeDeliveryHandler() {
	if a.node == nil {
		return
	}
	a.node.Host.RemoveStreamHandler(welcomeDeliveryProtocol)
}

func (a *App) handleWelcomeDelivery(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))

	msg, err := readWelcomeFrame(s)
	if err != nil || msg.V != 1 || msg.GroupID == "" || msg.WelcomeHex == "" {
		slog.Warn("handleWelcomeDelivery: bad frame", "err", err)
		return
	}

	if err := a.applyWelcome(msg.GroupID, msg.WelcomeHex); err != nil {
		slog.Error("handleWelcomeDelivery: applyWelcome failed", "group", msg.GroupID, "err", err)
		return
	}
}

// checkDHTWelcomes is called on startup; fetches any Welcome stored in DHT for
// groups the local node has been invited to but hasn't joined yet.
// Since DHT keys include the group ID, we build the key list from pending_welcomes_out
// rows where we are the invitee — but that table lives on the creator side.
// Instead, we probe a small set of well-known group IDs passed by the coordinator
// (e.g., the group IDs we see in GossipSub announces from peers).
//
// Practical approach: the Wails binding also exposes CheckDHTWelcome(groupID)
// so the UI (or user) can manually poll once after receiving a verbal invite.
func (a *App) checkDHTWelcomes(groupIDs []string) {
	a.mu.Lock()
	node := a.node
	a.mu.Unlock()
	if node == nil || len(groupIDs) == 0 {
		return
	}

	for _, gid := range groupIDs {
		a.mu.Lock()
		_, already := a.coordinators[gid]
		a.mu.Unlock()
		if already {
			continue
		}

		wb, err := p2p.FetchWelcomeFromDHT(context.Background(), node.DHT, node.Host.ID(), gid)
		if err != nil {
			continue // not found yet
		}
		if err := a.applyWelcome(gid, hex.EncodeToString(wb)); err != nil {
			slog.Warn("checkDHTWelcomes: apply failed", "group", gid, "err", err)
		}
	}
}

// applyWelcome joins a group using a Welcome hex string and the private bundle
// stored in SQLite (generated during advertisement).
func (a *App) applyWelcome(groupID, welcomeHex string) error {
	a.mu.Lock()
	node := a.node
	database := a.db
	_, already := a.coordinators[groupID]
	a.mu.Unlock()

	if already {
		return nil // already joined
	}
	if node == nil || database == nil {
		return fmt.Errorf("node or database not ready")
	}

	_, privateBundle, err := database.GetKPBundle(node.Host.ID().String())
	if err != nil {
		return fmt.Errorf("no local KeyPackage bundle found — was advertiseKeyPackage called? %w", err)
	}

	if err := a.JoinGroupWithWelcome(groupID, welcomeHex, hex.EncodeToString(privateBundle)); err != nil {
		return err
	}

	// KP is consumed after a successful join; generate a fresh one for next invite.
	go a.refreshKeyPackage()

	wailsRuntime.EventsEmit(a.ctx, "group:joined", map[string]interface{}{
		"group_id": groupID,
	})
	slog.Info("Joined group via Welcome", "group", groupID)
	return nil
}

// ── Wails bindings ────────────────────────────────────────────────────────────

// CheckDHTWelcome checks the DHT for a pending Welcome for the given groupID
// and auto-joins if found. Useful when the user knows they were invited to a
// specific group but the direct delivery stream was missed.
func (a *App) CheckDHTWelcome(groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}
	a.mu.Lock()
	node := a.node
	a.mu.Unlock()
	if node == nil {
		return fmt.Errorf("P2P node not running")
	}

	wb, err := p2p.FetchWelcomeFromDHT(context.Background(), node.DHT, node.Host.ID(), groupID)
	if err != nil {
		return fmt.Errorf("no pending invite found for group %q in DHT", groupID)
	}
	return a.applyWelcome(groupID, hex.EncodeToString(wb))
}

// GetKPStatus returns whether the local node has a KeyPackage advertised.
func (a *App) GetKPStatus() map[string]interface{} {
	a.mu.Lock()
	node := a.node
	database := a.db
	a.mu.Unlock()

	if node == nil || database == nil {
		return map[string]interface{}{"advertised": false}
	}
	kp, _, err := database.GetKPBundle(node.Host.ID().String())
	return map[string]interface{}{
		"advertised": err == nil && len(kp) > 0,
		"peer_id":    node.Host.ID().String(),
	}
}

// ── Peer connect notification hook ───────────────────────────────────────────

// peerConnectedHook is registered as a libp2p Network Notifee so the creator
// retries pending Welcome deliveries whenever the invitee reconnects.
type peerConnectedHook struct {
	app *App
}

func (h *peerConnectedHook) Listen(network.Network, ma.Multiaddr)      {}
func (h *peerConnectedHook) ListenClose(network.Network, ma.Multiaddr) {}
func (h *peerConnectedHook) Connected(_ network.Network, c network.Conn) {
	p := c.RemotePeer()
	go h.app.retryPendingWelcomes(p)
	// DHT PutValue needs peers in the routing table — retry KP advertisement
	// whenever anyone connects (common on small LANs).
	go h.app.advertiseKeyPackage()
}
func (h *peerConnectedHook) Disconnected(network.Network, network.Conn) {}
