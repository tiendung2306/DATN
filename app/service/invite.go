package service

// invite.go — Offline-capable group invite protocol
//
// Design (best-practice, matches RFC 9420 KeyPackage distribution model):
//
//  Phase 1 — Advertisement (done once on P2P start, renewed after each use):
//    Invitee generates a KeyPackage → stores private bundle in SQLite.
//    Public KP bytes are replicated to currently connected verified peers via
//    custom store stream ("/app/kp-store/1.0.0").
//    → Creator can fetch KP from the invitee directly (fast path) or from
//      store peers when invitee is offline.
//
//  Phase 2 — Invite (creator, works while invitee is offline):
//    Creator (or any_member proposer) fetches public KP (direct or store
//    peer) → records the operation in `group_add_operations` → calls
//    coord.AddMember.
//    If the local node is the current Token Holder, MLS Commit + Welcome are
//    produced synchronously; the runtime persists pending_welcomes_out and
//    replicates the Welcome locally.
//    If the local node is NOT the Token Holder, the coordinator broadcasts a
//    ProposalAdd carrying the operation_id; whichever node holds the token
//    will run CreateCommit and dispatch Welcome via OnAddCommitted on its
//    own machine. Only that node has the ephemeral keys to author Welcome.
//    Welcome ALSO replicated to store peers ("/app/welcome-store/1.0.0") for
//    the invitee to pull on next startup.
//    If invitee happens to be online: also send directly via stream (fast path).
//
//  Phase 3 — Delivery (invitee, online or reconnecting):
//    On startup/manual: pull own pending Welcomes from connected store peers
//    → auto-join.
//    On connect:  creator retries undelivered Welcomes via direct stream.
//    Stream handler "/app/welcome-delivery/1.0.0": receive Welcome → auto-join.
//    After join: regenerate + re-advertise a fresh KP so the next invite works.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/coordination"
	"app/domain"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	welcomeDeliveryProtocol  = protocol.ID("/app/welcome-delivery/1.0.0")
	groupJoinAckProtocol     = protocol.ID("/app/group-join-ack/1.0.0")
	maxWelcomeFrame          = 4 << 20 // 4 MiB
	defaultReplicationFanout = 3
)

// welcomeDeliveryWire is the JSON payload for /app/welcome-delivery/1.0.0.
//
// CategoryID is the channel-category assignment from the inviter's authoritative
// state at the moment the Welcome was sealed. Shipping it inline (instead of
// relying on a follow-up snapshot pull) makes auto-join deterministic: the
// invitee never sees the channel as orphaned even when peer verification has
// not completed yet, and category assignment survives offline replication
// because the same value is persisted in stored_welcomes / pending_invites.
type welcomeDeliveryWire struct {
	V          int    `json:"v"`
	GroupID    string `json:"group_id"`
	GroupType  string `json:"group_type,omitempty"`
	CategoryID string `json:"category_id,omitempty"`
	WelcomeHex string `json:"welcome_hex"`
}

type groupJoinAckWire struct {
	V       int    `json:"v"`
	GroupID string `json:"group_id"`
	PeerID  string `json:"peer_id"`
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
// private bundle in SQLite, and replicates the public bytes to verified peers.
// Safe to call multiple times — regenerates only when the existing KP is absent.
func (r *Runtime) advertiseKeyPackage() {
	r.mu.Lock()
	node := r.node
	database := r.db
	r.mu.Unlock()
	if node == nil || database == nil {
		return
	}

	localID := node.Host.ID()

	// Reuse existing KP if already advertised.
	existing, _, err := database.GetKPBundle(localID.String())
	if err == nil && len(existing) > 0 {
		r.replicateKeyPackageToStorePeers(existing)
		return
	}

	kpRes, err := r.GenerateKeyPackage()
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

	r.replicateKeyPackageToStorePeers(publicKP)
}

func (r *Runtime) selectStorePeersLocked(localID peer.ID) []peer.ID {
	if r.node == nil || r.node.AuthProtocol == nil {
		return nil
	}
	peers := r.node.Host.Network().Peers()
	out := make([]peer.ID, 0, len(peers))
	for _, pid := range peers {
		if pid == localID {
			continue
		}
		if !r.node.AuthProtocol.IsVerified(pid) {
			continue
		}
		out = append(out, pid)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	if len(out) > defaultReplicationFanout {
		out = out[:defaultReplicationFanout]
	}
	return out
}

func (r *Runtime) replicateKeyPackageToStorePeers(publicKP []byte) {
	r.mu.Lock()
	node := r.node
	database := r.db
	localID := peer.ID("")
	if node != nil {
		localID = node.Host.ID()
	}
	peers := r.selectStorePeersLocked(localID)
	r.mu.Unlock()
	if node == nil || database == nil || localID == "" || len(publicKP) == 0 {
		return
	}

	_ = database.SaveStoredKeyPackage(localID.String(), publicKP, localID.String())
	go r.publishBlindStoreKeyPackage(localID.String(), publicKP)

	req := p2p.KPStoreRequestV1{
		V:        1,
		PeerID:   localID.String(),
		PublicKP: publicKP,
	}
	for _, pid := range peers {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		s, err := node.Host.NewStream(ctx, pid, p2p.KPStoreProtocol)
		cancel()
		if err != nil {
			continue
		}
		_ = s.SetDeadline(time.Now().Add(15 * time.Second))
		_ = p2p.WriteInviteStoreJSONFrame(s, &req)
		_ = s.Close()
	}
}

// refreshKeyPackage generates a brand-new KeyPackage and re-advertises it after
// the previous one was consumed by an AddMembers call.
func (r *Runtime) refreshKeyPackage() {
	r.mu.Lock()
	node := r.node
	database := r.db
	r.mu.Unlock()
	if node == nil || database == nil {
		return
	}

	// Delete old KP so advertiseKeyPackage generates fresh.
	_, _ = database.Conn.Exec("DELETE FROM kp_bundles WHERE peer_id = ?", node.Host.ID().String())
	r.advertiseKeyPackage()
}

// ── Phase 2: Invite (creator, offline-capable) ────────────────────────────────

// InvitePeerToGroup fetches the target peer's public KeyPackage via direct
// stream or verified store peers (works even if the peer is offline), performs
// MLS AddMembers, stores the resulting Welcome in SQLite + store peers, and
// attempts immediate direct delivery if the peer is currently connected.
func (r *Runtime) InvitePeerToGroup(peerIDStr, groupID string) error {
	peerIDStr = strings.TrimSpace(peerIDStr)
	groupID = strings.TrimSpace(groupID)
	if peerIDStr == "" || groupID == "" {
		return fmt.Errorf("peer ID and group ID are required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}

	targetID, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("invalid peer ID %q: %w", peerIDStr, err)
	}

	r.mu.Lock()
	node := r.node
	coord, hasGroup := r.coordinators[groupID]
	database := r.db
	r.mu.Unlock()

	if node == nil {
		return fmt.Errorf("P2P node not running")
	}
	if !hasGroup || coord == nil {
		return fmt.Errorf("not in group %q — create the group first", groupID)
	}
	if targetID == node.Host.ID() {
		return fmt.Errorf("cannot invite yourself")
	}
	// Resolve groupType + categoryID from the inviter's authoritative state.
	// CategoryID lives in mls_groups (set by CreateGroupChat /
	// AssignCategoryToGroup) and is read here so the Welcome wire frame can
	// carry the assignment to the invitee — making auto-join deterministic
	// without a follow-up snapshot pull on the receiver side.
	groupType := "channel"
	categoryID := ""
	if rec, recErr := r.coordStorage.GetGroupRecord(groupID); recErr == nil {
		if normalizedType, normErr := normalizeGroupTypeRuntime(rec.GroupType); normErr == nil {
			groupType = normalizedType
		}
		categoryID = strings.TrimSpace(rec.CategoryID)
	}
	if groupType == "dm" && database != nil {
		for _, member := range coord.ActiveMembers() {
			if member.String() == targetID.String() {
				return nil
			}
		}
		members, listErr := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
		if listErr == nil {
			hasTarget := false
			activeCount := 0
			for _, rec := range members {
				if strings.TrimSpace(rec.PeerID) == "" {
					continue
				}
				activeCount++
				if rec.PeerID == targetID.String() {
					hasTarget = true
				}
			}
			if !hasTarget && activeCount >= 2 {
				return fmt.Errorf("direct message already has two members")
			}
		}
	}

	// Prefer direct stream from invitee if connected; fall back to store peers.
	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(r.appCtx(), targetID)
	}

	slog.Info("Fetching KeyPackage", "target", targetID, "group", groupID)
	kpBytes, err := r.fetchPeerKeyPackage(targetID)
	if err != nil {
		return fmt.Errorf(
			"could not get KeyPackage for %s: %w.\n\n"+
				"Ensure at least one verified peer is online to act as a store node, or bring the invitee online and retry.",
			targetID, err)
	}

	// Record the Add operation lifecycle row BEFORE dispatching to the
	// coordinator. The operation_id is deterministic over (group, target,
	// KP hash) so concurrent approvals on the same (group, target) reuse
	// the row instead of forking lifecycle. This guarantees that observer
	// nodes processing the resulting commit can correlate to a known row.
	approver := node.Host.ID().String()
	kpHash := computeKeyPackageHash(kpBytes)
	opID := ComputeAddOperationID(groupID, targetID.String(), kpHash)
	if _, opErr := database.CreateGroupAddOperation(store.GroupAddOperationRecord{
		OperationID:    opID,
		GroupID:        groupID,
		TargetPeerID:   targetID.String(),
		ApproverPeerID: approver,
		KeyPackageHash: kpHash,
		Status:         store.AddOpStatusApproved,
	}); opErr != nil {
		slog.Warn("CreateGroupAddOperation failed (continuing)", "group", groupID, "target", targetID, "err", opErr)
	}

	// AddMembers (Coordinator + MLS engine).
	result, err := coord.AddMember(coordination.AddMemberRequest{
		TargetPeerID:    targetID,
		KeyPackageBytes: kpBytes,
		OperationID:     opID,
		GroupType:       groupType,
		CategoryID:      categoryID,
		KeyPackageHash:  hexDecodedOrNil(kpHash),
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "DuplicateSignatureKey") ||
			strings.Contains(msg, "already") ||
			strings.Contains(msg, "duplicate") {
			// Member may already exist in local MLS state from a prior invite.
			// Re-deliver existing pending Welcome for retry flow instead of failing.
			if resent := r.resendPendingWelcome(targetID, groupID, groupType, categoryID); resent {
				return nil
			}
			// Idempotent behavior for UI: inviting an already-added member should
			// be a no-op even when there is nothing new to deliver.
			return nil
		}
		_ = database.MarkAddOperationFailed(opID, "ERR_INVITE_ADD_MEMBER_FAILED", err.Error())
		return fmt.Errorf("AddMembers: %w", err)
	}

	if result.Deferred {
		// The local node is NOT the Token Holder for the current epoch. The
		// ProposalAdd has been broadcast on the group topic; whichever node
		// currently holds the token will author Welcome on its own machine
		// via OnAddCommitted. We MUST NOT fabricate or store a Welcome here.
		if mErr := database.MarkAddProposalBroadcast(opID, coord.CurrentEpoch()); mErr != nil {
			slog.Warn("MarkAddProposalBroadcast failed", "operation", opID, "err", mErr)
		}
		slog.Info("Group invite proposal broadcast",
			"group", groupID, "target", targetID, "operation", opID,
			"reason", "local_not_token_holder",
		)
		return nil
	}

	// Local Token Holder path: we ran CreateCommit so we hold the ephemeral
	// key material; persist + replicate + deliver Welcome.
	r.dispatchTokenHolderWelcome(database, groupID, result.Delivery, result.Welcome)

	// Authoritatively upsert the invited peer as a member so that local database queries (e.g. GetGroups)
	// can immediately resolve DM titles and rosters without waiting for a heartbeat or reconnect.
	if err := r.upsertGroupMember(groupID, targetID.String(), "member", "invite"); err != nil {
		slog.Warn("InvitePeerToGroup: upsertGroupMember failed", "group", groupID, "target", targetID, "err", err)
	}

	r.emit("group:members_changed", map[string]interface{}{
		"group_id": groupID,
		"reason":   "invited",
	})

	slog.Info("Group invite sent", "group", groupID, "target", targetID, "operation", opID)
	return nil
}

// dispatchTokenHolderWelcome persists the Welcome to pending_welcomes_out,
// replicates it to currently verified store peers, fires the fast-path direct
// delivery attempt, and advances the group_add_operations row to
// welcome_queued. It must only be called on the node that ran CreateCommit
// (the Token Holder for the committing epoch) — no other node owns the
// ephemeral material required to author the Welcome.
func (r *Runtime) dispatchTokenHolderWelcome(database *store.Database, groupID string, delivery coordination.AddCommitDelivery, welcome []byte) {
	if len(welcome) == 0 || database == nil {
		return
	}
	targetStr := strings.TrimSpace(delivery.TargetPeerID)
	if targetStr == "" {
		slog.Warn("dispatchTokenHolderWelcome: empty target", "operation", delivery.OperationID)
		return
	}
	targetID, decErr := peer.Decode(targetStr)
	if decErr != nil {
		slog.Warn("dispatchTokenHolderWelcome: invalid target peer", "operation", delivery.OperationID, "err", decErr)
		return
	}

	if err := database.SavePendingWelcome(targetID.String(), groupID, welcome); err != nil {
		slog.Warn("SavePendingWelcome failed", "operation", delivery.OperationID, "group", groupID, "err", err)
	}

	r.replicateWelcomeToStorePeers(targetID, groupID, delivery.GroupType, delivery.CategoryID, welcome)
	go r.deliverWelcome(targetID, groupID, delivery.GroupType, delivery.CategoryID, welcome)

	welcomeHashHex := ""
	if len(delivery.WelcomeHash) > 0 {
		welcomeHashHex = hex.EncodeToString(delivery.WelcomeHash)
	}
	if mErr := database.MarkAddWelcomeQueued(delivery.OperationID, welcomeHashHex); mErr != nil &&
		!errors.Is(mErr, store.ErrAddOperationTerminal) {
		slog.Warn("MarkAddWelcomeQueued failed", "operation", delivery.OperationID, "err", mErr)
	}
}

// computeKeyPackageHash returns the hex-encoded SHA-256 of the public
// KeyPackage bytes. Used as the idempotency dimension of an Add operation:
// two approvals targeting the same peer with the same KeyPackage MUST resolve
// to the same operation_id and reuse the same row.
func computeKeyPackageHash(kp []byte) string {
	if len(kp) == 0 {
		return ""
	}
	sum := sha256.Sum256(kp)
	return hex.EncodeToString(sum[:])
}

// hexDecodedOrNil decodes a hex string, returning nil on error or empty input.
// Used to thread a raw []byte hash into the coordination layer without
// forcing callers to import encoding/hex everywhere.
func hexDecodedOrNil(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}

// ComputeAddOperationID derives the canonical operation_id for an MLS Add
// operation. The id is a stable hash of (group_id, target_peer_id, kp_hash)
// so duplicate approvals on the same target collapse to a single lifecycle
// row, but successive KeyPackage rotations still get fresh ids (the target
// re-advertises KP after each successful join).
//
// kpHash is expected to be the hex-encoded SHA-256 returned by
// computeKeyPackageHash.
func ComputeAddOperationID(groupID, targetPeerID, kpHash string) string {
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(groupID)))
	h.Write([]byte{0})
	h.Write([]byte(strings.TrimSpace(targetPeerID)))
	h.Write([]byte{0})
	h.Write([]byte(strings.TrimSpace(kpHash)))
	return "ga_" + hex.EncodeToString(h.Sum(nil))[:32]
}

// resendPendingWelcome reuses a previously generated undelivered Welcome for
// the same (target, group) pair. This allows "invite again" UX without failing
// when MLS already contains the member from an earlier AddMember commit.
// categoryID is resolved by the caller (InvitePeerToGroup) from the inviter's
// current GroupRecord; the pending_welcomes_out table itself does not retain
// the assignment because it can change between invite attempts.
func (r *Runtime) resendPendingWelcome(targetID peer.ID, groupID, groupType, categoryID string) bool {
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return false
	}
	pending, err := database.GetPendingWelcomesFor(targetID.String())
	if err != nil || len(pending) == 0 {
		return false
	}
	for _, pw := range pending {
		if strings.TrimSpace(pw.GroupID) != groupID || len(pw.WelcomeBytes) == 0 {
			continue
		}
		r.replicateWelcomeToStorePeers(targetID, groupID, groupType, categoryID, pw.WelcomeBytes)
		go r.deliverWelcome(targetID, groupID, groupType, categoryID, pw.WelcomeBytes)
		slog.Info("Re-delivered existing pending welcome", "group", groupID, "target", targetID.String())
		return true
	}
	// Fallback: even if previously marked delivered, keep a copy for explicit
	// re-invite attempts so inviter can resend after invitee rejected locally.
	welcome, err := database.GetAnyPendingWelcomeForGroup(targetID.String(), groupID)
	if err == nil && len(welcome) > 0 {
		r.replicateWelcomeToStorePeers(targetID, groupID, groupType, categoryID, welcome)
		go r.deliverWelcome(targetID, groupID, groupType, categoryID, welcome)
		slog.Info("Re-delivered archived welcome", "group", groupID, "target", targetID.String())
		return true
	}
	return false
}

func (r *Runtime) fetchPeerKeyPackage(targetID peer.ID) ([]byte, error) {
	r.mu.Lock()
	node := r.node
	database := r.db
	localID := peer.ID("")
	if node != nil {
		localID = node.Host.ID()
	}
	storePeers := r.selectStorePeersLocked(localID)
	r.mu.Unlock()
	if node == nil || database == nil {
		return nil, fmt.Errorf("runtime not ready")
	}
	blindPeers := r.blindStoreFetchCandidates(localID, "kp:"+targetID.String())

	if kp, err := p2p.FetchKeyPackageDirect(context.Background(), node.Host, targetID); err == nil && len(kp) > 0 {
		_ = database.SaveStoredKeyPackage(targetID.String(), kp, targetID.String())
		return kp, nil
	}

	localCopy, err := database.GetStoredKeyPackage(targetID.String())
	if err == nil && len(localCopy) > 0 {
		return localCopy, nil
	}

	peerSet := make(map[peer.ID]struct{})
	ordered := make([]peer.ID, 0, len(blindPeers)+len(storePeers))
	for _, pid := range blindPeers {
		if pid == targetID {
			continue
		}
		if _, ok := peerSet[pid]; ok {
			continue
		}
		peerSet[pid] = struct{}{}
		ordered = append(ordered, pid)
	}
	for _, pid := range storePeers {
		if pid == targetID {
			continue
		}
		if _, ok := peerSet[pid]; ok {
			continue
		}
		peerSet[pid] = struct{}{}
		ordered = append(ordered, pid)
	}

	req := p2p.KPFetchRequestV1{V: 1, PeerID: targetID.String()}
	for _, pid := range ordered {
		if pid == targetID {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		s, err := node.Host.NewStream(ctx, pid, p2p.KPFetchProtocol)
		cancel()
		if err != nil {
			continue
		}
		_ = s.SetDeadline(time.Now().Add(15 * time.Second))
		if err := p2p.WriteInviteStoreJSONFrame(s, &req); err != nil {
			_ = s.Close()
			continue
		}
		var resp p2p.KPFetchResponseV1
		if err := p2p.ReadInviteStoreJSONFrame(s, &resp); err == nil && resp.V == 1 && resp.Found && len(resp.PublicKP) > 0 {
			_ = s.Close()
			_ = database.SaveStoredKeyPackage(targetID.String(), resp.PublicKP, pid.String())
			return resp.PublicKP, nil
		}
		_ = s.Close()
	}

	return nil, fmt.Errorf("key package not found from direct stream or store peers")
}

func (r *Runtime) replicateWelcomeToStorePeers(inviteeID peer.ID, groupID, groupType, categoryID string, welcome []byte) {
	r.mu.Lock()
	node := r.node
	database := r.db
	localID := peer.ID("")
	if node != nil {
		localID = node.Host.ID()
	}
	peers := r.selectStorePeersLocked(localID)
	r.mu.Unlock()
	if node == nil || database == nil || localID == "" || len(welcome) == 0 {
		return
	}
	_ = database.SaveStoredWelcome(inviteeID.String(), groupID, groupType, categoryID, welcome, localID.String())
	go r.publishBlindStoreWelcome(inviteeID.String(), groupID, groupType, categoryID, welcome)

	req := p2p.WelcomeStoreRequestV1{
		V:             1,
		InviteePeerID: inviteeID.String(),
		GroupID:       groupID,
		GroupType:     groupType,
		CategoryID:    categoryID,
		Welcome:       welcome,
	}
	for _, pid := range peers {
		if pid == inviteeID {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		s, err := node.Host.NewStream(ctx, pid, p2p.WelcomeStoreProtocol)
		cancel()
		if err != nil {
			continue
		}
		_ = s.SetDeadline(time.Now().Add(15 * time.Second))
		_ = p2p.WriteInviteStoreJSONFrame(s, &req)
		_ = s.Close()
	}
}

// fetchWelcomeFromStorePeers returns (welcomeBytes, groupType, categoryID,
// sourcePeerID, error). sourcePeerID is the original inviter (creator) when
// available — store peers are required to forward this value; only legacy
// data may have it empty. Callers MUST propagate this value (not their own
// peer ID) to savePendingInviteFromWelcome / SaveStoredWelcome so creator
// hint resolution survives every hop.
func (r *Runtime) fetchWelcomeFromStorePeers(groupID string) ([]byte, string, string, string, error) {
	r.mu.Lock()
	node := r.node
	database := r.db
	localID := peer.ID("")
	if node != nil {
		localID = node.Host.ID()
	}
	storePeers := r.selectStorePeersLocked(localID)
	r.mu.Unlock()
	if node == nil || database == nil || localID == "" {
		return nil, "", "", "", fmt.Errorf("runtime not ready")
	}
	blindPeers := r.blindStoreFetchCandidates(localID, "welcome:"+localID.String()+":"+groupID)

	if wb, storedGroupType, storedCategoryID, storedSrc, err := database.GetStoredWelcome(localID.String(), groupID); err == nil && len(wb) > 0 {
		// Use source_peer_id from the row (inviter / store peer), never
		// localID — passing self breaks GetGroupInviteCreatorHint and
		// resolveGroupCreatorPeerID for non-creators who need to forward
		// invite requests to the real creator.
		_ = r.savePendingInviteFromWelcome(groupID, storedGroupType, storedCategoryID, wb, storedSrc, false)
		return wb, storedGroupType, storedCategoryID, storedSrc, nil
	}

	peerSet := make(map[peer.ID]struct{})
	ordered := make([]peer.ID, 0, len(blindPeers)+len(storePeers))
	for _, pid := range blindPeers {
		if _, ok := peerSet[pid]; ok {
			continue
		}
		peerSet[pid] = struct{}{}
		ordered = append(ordered, pid)
	}
	for _, pid := range storePeers {
		if _, ok := peerSet[pid]; ok {
			continue
		}
		peerSet[pid] = struct{}{}
		ordered = append(ordered, pid)
	}

	req := p2p.WelcomeFetchRequestV1{
		V:             1,
		InviteePeerID: localID.String(),
		GroupID:       groupID,
	}
	for _, pid := range ordered {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		s, err := node.Host.NewStream(ctx, pid, p2p.WelcomeFetchProtocol)
		cancel()
		if err != nil {
			continue
		}
		_ = s.SetDeadline(time.Now().Add(15 * time.Second))
		if err := p2p.WriteInviteStoreJSONFrame(s, &req); err != nil {
			_ = s.Close()
			continue
		}
		var resp p2p.WelcomeFetchResponseV1
		if err := p2p.ReadInviteStoreJSONFrame(s, &resp); err == nil && resp.V == 1 && resp.Found && len(resp.Welcome) > 0 {
			_ = s.Close()
			// Prefer the explicit SourcePeerID forwarded by the store peer
			// (= original inviter / creator). Fall back to the responder's
			// own peer ID only when the field is missing — this preserves
			// creator hint resolution on legacy responders that have not
			// upgraded yet, while never losing a known good source.
			source := strings.TrimSpace(resp.SourcePeerID)
			if source == "" {
				source = pid.String()
			}
			_ = database.SaveStoredWelcome(localID.String(), groupID, resp.GroupType, resp.CategoryID, resp.Welcome, source)
			_ = r.savePendingInviteFromWelcome(groupID, resp.GroupType, resp.CategoryID, resp.Welcome, source, false)
			return resp.Welcome, resp.GroupType, resp.CategoryID, source, nil
		}
		_ = s.Close()
	}
	return nil, "", "", "", fmt.Errorf("welcome not found from store peers")
}

// savePendingInviteFromWelcome is the single chokepoint for every welcome a
// node receives. categoryID is the channel-category assignment carried inline
// by the inviter on the wire frame (welcomeDeliveryWire / WelcomeStoreRequestV1
// / WelcomeFetchResponseV1 / blindStoreEnvelopeV1). Pass "" only when the
// caller genuinely has no value (e.g. legacy stored_welcomes rows persisted
// before this field existed); the post-apply path falls back to a snapshot
// pull in that case.
func (r *Runtime) savePendingInviteFromWelcome(groupID, groupType, categoryID string, welcome []byte, sourcePeerID string, reopenRejected bool) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || len(welcome) == 0 {
		return fmt.Errorf("group ID and welcome are required")
	}
	normalizedGroupType, err := normalizeGroupTypeRuntime(groupType)
	if err != nil {
		return err
	}
	categoryID = strings.TrimSpace(categoryID)

	r.mu.RLock()
	node := r.node
	database := r.db
	localID := ""
	if node != nil {
		localID = node.Host.ID().String()
	}
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}

	// Defensive guard: never persist the local node as the welcome source.
	// stored_welcomes.source_peer_id and pending_invites.source_peer_id /
	// inviter_peer_id are read by GetGroupInviteCreatorHint to forward
	// invite requests to the creator. If this row records "self" as source,
	// member nodes will mis-route invite requests back to themselves and
	// hit ERR_GROUP_CREATOR_UNKNOWN. We drop the bad value to "" instead
	// (empty rows are still valid; the creator hint may then come from a
	// different row or fail loudly with the canonical error).
	sourcePeerID = strings.TrimSpace(sourcePeerID)
	if sourcePeerID != "" && localID != "" && sourcePeerID == localID {
		slog.Warn("savePendingInviteFromWelcome: dropping self as source (programmer error)",
			"group_id", groupID, "self", localID)
		sourcePeerID = ""
	}

	active, err := database.IsGroupActive(groupID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if active {
		return nil
	}

	// Auto-join semantics (Discord/Slack-style): once a Welcome reaches this
	// node we attempt to apply it immediately for every group type. The pending
	// row is only persisted as a fallback when applyWelcome cannot run yet
	// (sidecar not ready, KP rotated, MLS rejection); processPendingWelcomes
	// retries those rows on the next startup or refresh tick.
	if applyErr := r.applyWelcome(groupID, normalizedGroupType, hex.EncodeToString(welcome), categoryID); applyErr == nil {
		if strings.TrimSpace(sourcePeerID) != "" {
			_ = database.SetGroupCreatorPeerID(groupID, strings.TrimSpace(sourcePeerID))
		}
		if strings.TrimSpace(sourcePeerID) != "" {
			// welcome-source is observation-only: we know this peer
			// delivered the Welcome (could be Token Holder, creator, or
			// any forwarder) but we don't authoritatively know their
			// role. Preserve any existing role on this row.
			_ = r.upsertGroupMemberFromRosterSync(groupID, sourcePeerID, "welcome-source")
			r.emit("group:members_changed", map[string]interface{}{
				"group_id": groupID,
				"reason":   "welcome_source",
			})
		}
		if localID != "" {
			_ = database.SaveStoredWelcome(localID, groupID, normalizedGroupType, categoryID, welcome, sourcePeerID)
		}
		// Channel category restoration. Inline categoryID from the wire is the
		// authoritative path (deterministic, no network race). Snapshot pull
		// only runs when the wire did not carry the value — keeps legacy /
		// replication paths working without requiring a schema migration on
		// older deployments.
		r.applyChannelCategoryAfterAutoJoin(groupID, normalizedGroupType, categoryID, sourcePeerID)
		inviteID := store.PendingInviteID(groupID, welcome)
		_ = database.SavePendingInvite(&store.PendingInvite{
			ID:            inviteID,
			GroupID:       groupID,
			GroupType:     normalizedGroupType,
			CategoryID:    categoryID,
			WelcomeBytes:  append([]byte(nil), welcome...),
			SourcePeerID:  sourcePeerID,
			InviterPeerID: strings.TrimSpace(sourcePeerID),
			Status:        store.PendingInviteStatusAccepted,
		})
		_ = database.MarkPendingInviteAccepted(inviteID)
		r.emit("invite:auto_joined", map[string]interface{}{
			"id":           inviteID,
			"group_id":     groupID,
			"group_type":   normalizedGroupType,
			"inviter_peer": strings.TrimSpace(sourcePeerID),
		})

		// Generate notification
		r.insertNotification(domain.NotificationTypeGroupAdd, groupID, sourcePeerID, groupID, "")

		return nil
	} else {
		slog.Debug("auto-join via Welcome deferred",
			"group", groupID, "type", normalizedGroupType, "err", applyErr)
	}
	if localID != "" {
		_ = database.SaveStoredWelcome(localID, groupID, normalizedGroupType, categoryID, welcome, sourcePeerID)
	}
	inviteID := store.PendingInviteID(groupID, welcome)
	if latest, latestErr := database.GetLatestPendingInviteByGroup(groupID); latestErr == nil {
		if latest.Status == store.PendingInviteStatusAccepted {
			return nil
		}
		if latest.Status == store.PendingInviteStatusRejected {
			if !reopenRejected {
				// Keep local rejection sticky for passive/background refresh paths.
				return nil
			}
			reopenedID, reopenErr := database.ReopenRejectedInvite(&store.PendingInvite{
				ID:            inviteID,
				GroupID:       groupID,
				GroupType:     normalizedGroupType,
				CategoryID:    categoryID,
				WelcomeBytes:  append([]byte(nil), welcome...),
				SourcePeerID:  sourcePeerID,
				InviterPeerID: strings.TrimSpace(sourcePeerID),
			})
			if reopenErr == nil {
				r.emit("invite:received", map[string]interface{}{
					"id":         reopenedID,
					"group_id":   groupID,
					"group_type": normalizedGroupType,
					"status":     store.PendingInviteStatusPending,
					"reason":     "reinvited",
				})
				return nil
			}
		}
	}

	inv := &store.PendingInvite{
		GroupID:      groupID,
		GroupType:    normalizedGroupType,
		CategoryID:   categoryID,
		WelcomeBytes: append([]byte(nil), welcome...),
		SourcePeerID: sourcePeerID,
		Status:       store.PendingInviteStatusPending,
	}
	if err := database.SavePendingInvite(inv); err != nil {
		return err
	}
	r.emit("invite:received", map[string]interface{}{
		"id":         inviteID,
		"group_id":   groupID,
		"group_type": normalizedGroupType,
		"status":     store.PendingInviteStatusPending,
		"reason":     "new",
	})
	return nil
}

// processPendingWelcomesOnStartup attempts to auto-join every Welcome left in
// pending state from a previous session — typical reasons:
//   - The runtime was offline when the Welcome arrived and only received the
//     replicated copy on next startup.
//   - The Rust sidecar was not ready yet when savePendingInviteFromWelcome ran.
//   - The process crashed between persisting the pending row and applying it.
//
// Best-effort: failures stay pending so the next startup or
// refreshPendingInvites tick retries them. Caller passes the P2P node's
// context so cancellation propagates if the node is being torn down.
func (r *Runtime) processPendingWelcomesOnStartup(ctx context.Context) {
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return
	}

	// Refresh from local stored_welcomes / connected store peers first so
	// rows that only exist as replicated objects materialise into pending
	// rows that the loop below can pick up.
	if err := r.refreshPendingInvites(ctx); err != nil {
		slog.Debug("processPendingWelcomesOnStartup: refresh failed", "err", err)
	}

	rows, err := database.ListPendingInvites(false)
	if err != nil {
		slog.Warn("processPendingWelcomesOnStartup: list pending failed", "err", err)
		return
	}
	for _, inv := range rows {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if inv.Status != store.PendingInviteStatusPending {
			continue
		}
		if applyErr := r.applyWelcome(inv.GroupID, inv.GroupType, hex.EncodeToString(inv.WelcomeBytes), inv.CategoryID); applyErr != nil {
			slog.Debug("auto-join pending welcome deferred to next retry",
				"group", inv.GroupID, "type", inv.GroupType, "err", applyErr)
			continue
		}
		if strings.TrimSpace(inv.SourcePeerID) != "" {
			_ = database.SetGroupCreatorPeerID(inv.GroupID, strings.TrimSpace(inv.SourcePeerID))
		}
		if strings.TrimSpace(inv.SourcePeerID) != "" {
			// Observation-only — same reasoning as the wire-path
			// welcome-source upsert above.
			_ = r.upsertGroupMemberFromRosterSync(inv.GroupID, inv.SourcePeerID, "welcome-source")
			r.emit("group:members_changed", map[string]interface{}{
				"group_id": inv.GroupID,
				"reason":   "welcome_source",
			})
		}
		_ = database.MarkPendingInviteAccepted(inv.ID)
		r.applyChannelCategoryAfterAutoJoin(inv.GroupID, inv.GroupType, inv.CategoryID, inv.SourcePeerID)
		r.emit("invite:auto_joined", map[string]interface{}{
			"id":           inv.ID,
			"group_id":     inv.GroupID,
			"group_type":   inv.GroupType,
			"inviter_peer": strings.TrimSpace(inv.SourcePeerID),
		})
	}
}

// applyChannelCategoryAfterAutoJoin restores the channel-category assignment
// once a Welcome has been applied. The wire-carried categoryID is the primary,
// deterministic path: when present we write it straight into the assignment
// table (no network call, no race with peer verification). When absent (legacy
// rows / older replication frames) we fall back to a best-effort snapshot
// pull from the welcome source — same as before, but only as a fallback.
func (r *Runtime) applyChannelCategoryAfterAutoJoin(groupID, groupType, categoryID, sourcePeerID string) {
	if !strings.EqualFold(strings.TrimSpace(groupType), "channel") {
		return
	}
	groupID = strings.TrimSpace(groupID)
	categoryID = strings.TrimSpace(categoryID)
	if groupID == "" {
		return
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database != nil && categoryID != "" {
		if err := database.AssignCategoryToGroupWhenReady(groupID, categoryID); err != nil {
			slog.Warn("apply channel category after auto-join: assign failed",
				"group_id", groupID, "category_id", categoryID, "error", err)
		} else {
			r.emit("channel_categories:changed", map[string]interface{}{
				"reason":      "welcome_inline",
				"channel_id":  groupID,
				"category_id": categoryID,
			})
			return
		}
	}
	// Fallback path: ask the welcome source for a snapshot — this covers
	// older deployments that have not yet learned to ship category_id inline.
	pid, err := peer.Decode(strings.TrimSpace(sourcePeerID))
	if err != nil {
		return
	}
	go r.scheduleChannelCategorySync(pid)
}

func (r *Runtime) refreshPendingInvites(ctx context.Context) error {
	r.mu.RLock()
	node := r.node
	database := r.db
	localID := peer.ID("")
	if node != nil {
		localID = node.Host.ID()
	}
	storePeers := r.selectStorePeersLocked(localID)
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	if localID == "" {
		return nil
	}

	localWelcomes, err := database.ListStoredWelcomesFor(localID.String())
	if err != nil {
		return err
	}
	for _, item := range localWelcomes {
		_ = r.savePendingInviteFromWelcome(item.GroupID, item.GroupType, item.CategoryID, item.WelcomeBytes, item.SourcePeerID, false)
	}
	if node == nil {
		return nil
	}

	req := p2p.WelcomeListRequestV1{V: 1, InviteePeerID: localID.String()}
	for _, pid := range storePeers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		streamCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		s, err := node.Host.NewStream(streamCtx, pid, p2p.WelcomeListProtocol)
		cancel()
		if err != nil {
			continue
		}
		_ = s.SetDeadline(time.Now().Add(15 * time.Second))
		if err := p2p.WriteInviteStoreJSONFrame(s, &req); err != nil {
			_ = s.Close()
			continue
		}
		var resp p2p.WelcomeListResponseV1
		if err := p2p.ReadInviteStoreJSONFrame(s, &resp); err != nil || resp.V != 1 {
			_ = s.Close()
			continue
		}
		_ = s.Close()
		for _, item := range resp.Invites {
			if item.GroupID == "" || len(item.Welcome) == 0 {
				continue
			}
			source := item.SourcePeerID
			if source == "" {
				source = pid.String()
			}
			_ = database.SaveStoredWelcome(localID.String(), item.GroupID, item.GroupType, item.CategoryID, item.Welcome, source)
			_ = r.savePendingInviteFromWelcome(item.GroupID, item.GroupType, item.CategoryID, item.Welcome, source, false)
		}
	}
	return nil
}

// ── Phase 3a: Welcome delivery (creator → invitee, direct stream) ─────────────

// deliverWelcome attempts to send Welcome bytes to targetID via direct stream.
// Called after InvitePeerToGroup and on every peer-connect event.
func (r *Runtime) deliverWelcome(targetID peer.ID, groupID, groupType, categoryID string, welcomeBytes []byte) {
	r.mu.Lock()
	node := r.node
	r.mu.Unlock()
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

	msg := &welcomeDeliveryWire{
		V:          1,
		GroupID:    groupID,
		GroupType:  groupType,
		CategoryID: categoryID,
		WelcomeHex: hex.EncodeToString(welcomeBytes),
	}
	if err := writeWelcomeFrame(s, msg); err != nil {
		slog.Warn("deliverWelcome: write failed", "target", targetID, "err", err)
		return
	}

	slog.Info("Welcome delivered via direct stream", "group", groupID, "target", targetID)
	// Mark as delivered in DB (best-effort).
	r.mu.Lock()
	database := r.db
	r.mu.Unlock()
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
func (r *Runtime) retryPendingWelcomes(targetID peer.ID) {
	r.mu.Lock()
	database := r.db
	coordStorage := r.coordStorage
	r.mu.Unlock()
	if database == nil {
		return
	}

	pending, err := database.GetPendingWelcomesFor(targetID.String())
	if err != nil || len(pending) == 0 {
		return
	}
	slog.Info("Retrying pending Welcomes for reconnected peer", "peer", targetID, "count", len(pending))
	for _, pw := range pending {
		groupType := "channel"
		categoryID := ""
		if coordStorage != nil {
			if rec, err := coordStorage.GetGroupRecord(pw.GroupID); err == nil {
				if normalizedType, normErr := normalizeGroupTypeRuntime(rec.GroupType); normErr == nil {
					groupType = normalizedType
				}
				categoryID = strings.TrimSpace(rec.CategoryID)
			}
		}
		r.deliverWelcome(targetID, pw.GroupID, groupType, categoryID, pw.WelcomeBytes)
	}
}

// ── Phase 3b: Welcome receipt (invitee) ───────────────────────────────────────

// ensureLocalPublicKPBytes returns the public KeyPackage bytes, generating and
// persisting a bundle in SQLite if none exists yet.
func (r *Runtime) ensureLocalPublicKPBytes() ([]byte, error) {
	r.mu.Lock()
	node := r.node
	database := r.db
	r.mu.Unlock()
	if node == nil || database == nil {
		return nil, fmt.Errorf("node or database not ready")
	}
	pid := node.Host.ID().String()
	pub, _, err := database.GetKPBundle(pid)
	if err == nil && len(pub) > 0 {
		return pub, nil
	}
	kpRes, err := r.GenerateKeyPackage()
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

func (r *Runtime) handleKPOfferStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	one := make([]byte, 1)
	if _, err := io.ReadFull(s, one); err != nil || one[0] != 0x01 {
		return
	}

	r.mu.Lock()
	var ap *p2p.AuthProtocol
	if r.node != nil {
		ap = r.node.AuthProtocol
	}
	database := r.db
	r.mu.Unlock()

	if ap != nil && !ap.IsVerified(remote) {
		slog.Warn("kp-offer: rejected unverified peer", "peer", remote)
		return
	}
	if database == nil {
		return
	}

	pub, err := r.ensureLocalPublicKPBytes()
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

func (r *Runtime) registerKPOfferHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(p2p.KPOfferProtocol, func(s network.Stream) {
		go r.handleKPOfferStream(s)
	})
	slog.Info("KP offer handler registered", "protocol", string(p2p.KPOfferProtocol))
}

func (r *Runtime) removeKPOfferHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(p2p.KPOfferProtocol)
}

func (r *Runtime) handleKPStoreStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	var ap *p2p.AuthProtocol
	if r.node != nil {
		ap = r.node.AuthProtocol
	}
	database := r.db
	r.mu.Unlock()
	if database == nil {
		return
	}
	if ap != nil && !ap.IsVerified(remote) {
		return
	}

	var req p2p.KPStoreRequestV1
	if err := p2p.ReadInviteStoreJSONFrame(s, &req); err != nil || req.V != 1 || req.PeerID == "" || len(req.PublicKP) == 0 {
		return
	}
	_ = database.SaveStoredKeyPackage(req.PeerID, req.PublicKP, remote.String())
}

func (r *Runtime) handleKPFetchStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	var ap *p2p.AuthProtocol
	if r.node != nil {
		ap = r.node.AuthProtocol
	}
	database := r.db
	r.mu.Unlock()
	if database == nil {
		return
	}
	if ap != nil && !ap.IsVerified(remote) {
		return
	}

	var req p2p.KPFetchRequestV1
	if err := p2p.ReadInviteStoreJSONFrame(s, &req); err != nil || req.V != 1 || req.PeerID == "" {
		return
	}

	resp := p2p.KPFetchResponseV1{V: 1, Found: false}
	if kp, err := database.GetStoredKeyPackage(req.PeerID); err == nil && len(kp) > 0 {
		resp.Found = true
		resp.PublicKP = kp
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		resp.Error = err.Error()
	}
	_ = p2p.WriteInviteStoreJSONFrame(s, &resp)
}

func (r *Runtime) handleWelcomeStoreStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	var ap *p2p.AuthProtocol
	localID := peer.ID("")
	if r.node != nil {
		ap = r.node.AuthProtocol
		localID = r.node.Host.ID()
	}
	database := r.db
	r.mu.Unlock()
	if database == nil {
		return
	}
	if ap != nil && !ap.IsVerified(remote) {
		return
	}

	var req p2p.WelcomeStoreRequestV1
	if err := p2p.ReadInviteStoreJSONFrame(s, &req); err != nil ||
		req.V != 1 || req.InviteePeerID == "" || req.GroupID == "" || len(req.Welcome) == 0 {
		return
	}
	_ = database.SaveStoredWelcome(req.InviteePeerID, req.GroupID, req.GroupType, req.CategoryID, req.Welcome, remote.String())
	if localID != "" && req.InviteePeerID == localID.String() {
		_ = r.savePendingInviteFromWelcome(req.GroupID, req.GroupType, req.CategoryID, req.Welcome, remote.String(), false)
	}
}

func (r *Runtime) handleWelcomeFetchStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	var ap *p2p.AuthProtocol
	if r.node != nil {
		ap = r.node.AuthProtocol
	}
	database := r.db
	r.mu.Unlock()
	if database == nil {
		return
	}
	if ap != nil && !ap.IsVerified(remote) {
		return
	}

	var req p2p.WelcomeFetchRequestV1
	if err := p2p.ReadInviteStoreJSONFrame(s, &req); err != nil ||
		req.V != 1 || req.InviteePeerID == "" || req.GroupID == "" {
		return
	}

	resp := p2p.WelcomeFetchResponseV1{V: 1, Found: false}
	if wb, groupType, categoryID, srcPeerID, err := database.GetStoredWelcome(req.InviteePeerID, req.GroupID); err == nil && len(wb) > 0 {
		resp.Found = true
		resp.Welcome = wb
		resp.GroupType = groupType
		resp.CategoryID = categoryID
		resp.SourcePeerID = srcPeerID
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		resp.Error = err.Error()
	}
	_ = p2p.WriteInviteStoreJSONFrame(s, &resp)
}

func (r *Runtime) handleWelcomeListStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	var ap *p2p.AuthProtocol
	if r.node != nil {
		ap = r.node.AuthProtocol
	}
	database := r.db
	r.mu.Unlock()
	if database == nil {
		return
	}
	if ap != nil && !ap.IsVerified(remote) {
		return
	}

	var req p2p.WelcomeListRequestV1
	if err := p2p.ReadInviteStoreJSONFrame(s, &req); err != nil ||
		req.V != 1 || req.InviteePeerID == "" {
		return
	}

	resp := p2p.WelcomeListResponseV1{V: 1}
	rows, err := database.ListStoredWelcomesFor(req.InviteePeerID)
	if err != nil {
		resp.Error = err.Error()
		_ = p2p.WriteInviteStoreJSONFrame(s, &resp)
		return
	}
	for _, row := range rows {
		resp.Invites = append(resp.Invites, p2p.WelcomeListItemV1{
			GroupID:      row.GroupID,
			GroupType:    row.GroupType,
			CategoryID:   row.CategoryID,
			Welcome:      row.WelcomeBytes,
			SourcePeerID: row.SourcePeerID,
			CreatedAt:    row.CreatedAt,
		})
	}
	_ = p2p.WriteInviteStoreJSONFrame(s, &resp)
}

func (r *Runtime) registerInviteStoreHandlers() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(p2p.KPStoreProtocol, func(s network.Stream) {
		go r.handleKPStoreStream(s)
	})
	r.node.Host.SetStreamHandler(p2p.KPFetchProtocol, func(s network.Stream) {
		go r.handleKPFetchStream(s)
	})
	r.node.Host.SetStreamHandler(p2p.WelcomeStoreProtocol, func(s network.Stream) {
		go r.handleWelcomeStoreStream(s)
	})
	r.node.Host.SetStreamHandler(p2p.WelcomeFetchProtocol, func(s network.Stream) {
		go r.handleWelcomeFetchStream(s)
	})
	r.node.Host.SetStreamHandler(p2p.WelcomeListProtocol, func(s network.Stream) {
		go r.handleWelcomeListStream(s)
	})
}

func (r *Runtime) removeInviteStoreHandlers() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(p2p.KPStoreProtocol)
	r.node.Host.RemoveStreamHandler(p2p.KPFetchProtocol)
	r.node.Host.RemoveStreamHandler(p2p.WelcomeStoreProtocol)
	r.node.Host.RemoveStreamHandler(p2p.WelcomeFetchProtocol)
	r.node.Host.RemoveStreamHandler(p2p.WelcomeListProtocol)
}

// registerWelcomeDeliveryHandler registers the stream handler so the invitee
// records pending invites when a Welcome is pushed by the creator.
func (r *Runtime) registerWelcomeDeliveryHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(welcomeDeliveryProtocol, func(s network.Stream) {
		go r.handleWelcomeDelivery(s)
	})
	slog.Info("Welcome delivery handler registered")
}

func (r *Runtime) removeWelcomeDeliveryHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(welcomeDeliveryProtocol)
}

func (r *Runtime) handleWelcomeDelivery(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))

	msg, err := readWelcomeFrame(s)
	if err != nil || msg.V != 1 || msg.GroupID == "" || msg.WelcomeHex == "" {
		slog.Warn("handleWelcomeDelivery: bad frame", "err", err)
		return
	}
	welcome, err := hex.DecodeString(strings.TrimSpace(msg.WelcomeHex))
	if err != nil {
		slog.Warn("handleWelcomeDelivery: invalid welcome hex", "group", msg.GroupID, "err", err)
		return
	}
	sourcePeerID := s.Conn().RemotePeer().String()
	if err := r.savePendingInviteFromWelcome(msg.GroupID, msg.GroupType, msg.CategoryID, welcome, sourcePeerID, true); err != nil {
		slog.Error("handleWelcomeDelivery: save pending invite failed", "group", msg.GroupID, "err", err)
		return
	}
}

// checkStoredWelcomes tries to fetch pending Welcome objects from connected store
// peers for group IDs not joined yet.
func (r *Runtime) checkStoredWelcomes(groupIDs []string) {
	r.mu.Lock()
	node := r.node
	r.mu.Unlock()
	if node == nil || len(groupIDs) == 0 {
		return
	}

	for _, gid := range groupIDs {
		r.mu.Lock()
		_, already := r.coordinators[gid]
		r.mu.Unlock()
		if already {
			continue
		}

		wb, groupType, categoryID, source, err := r.fetchWelcomeFromStorePeers(gid)
		if err != nil {
			continue // not found yet
		}
		// Always carry source through; an empty source breaks
		// GetGroupInviteCreatorHint for member nodes that later need to
		// resolve the group creator (RequestGroupInvite forward path).
		if err := r.savePendingInviteFromWelcome(gid, groupType, categoryID, wb, source, false); err != nil {
			slog.Warn("checkStoredWelcomes: save pending invite failed", "group", gid, "err", err)
		}
	}
}

// applyWelcome joins a group using a Welcome hex string and the private bundle
// stored in SQLite (generated during advertisement). categoryID is the
// inviter-side channel category when known (wire / pending / stored_welcomes);
// pass "" when unknown.
func (r *Runtime) applyWelcome(groupID, groupType, welcomeHex string, categoryID string) error {
	r.mu.Lock()
	node := r.node
	database := r.db
	_, already := r.coordinators[groupID]
	r.mu.Unlock()

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
	welcomeRaw, err := hex.DecodeString(strings.TrimSpace(welcomeHex))
	if err != nil {
		return fmt.Errorf("decode welcome hex: %w", err)
	}

	if err := r.joinGroupWithWelcome(groupID, welcomeHex, hex.EncodeToString(privateBundle), groupType, strings.TrimSpace(categoryID)); err != nil {
		return err
	}
	fp := fallbackWelcomeFingerprint(groupID, welcomeRaw)
	if inserted, markErr := database.MarkWelcomeApplied(fp, groupID, time.Now().Unix()); markErr == nil && !inserted {
		// Replay-safe no-op: this welcome has already been applied earlier.
		slog.Debug("welcome fingerprint already applied", "group", groupID)
	}

	// KP is consumed after a successful join; generate a fresh one for next invite.
	go r.refreshKeyPackage()

	r.emit("group:joined", map[string]interface{}{
		"group_id": groupID,
	})
	go r.broadcastGroupJoinAck(groupID)
	slog.Info("Joined group via Welcome", "group", groupID)
	return nil
}

func fallbackWelcomeFingerprint(groupID string, welcomeBytes []byte) string {
	hasher := sha256.New()
	group := strings.TrimSpace(groupID)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(group)))
	hasher.Write(lenBuf[:])
	hasher.Write([]byte(group))
	hasher.Write(welcomeBytes)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (r *Runtime) broadcastGroupJoinAck(groupID string) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return
	}
	r.mu.RLock()
	node := r.node
	database := r.db
	r.mu.RUnlock()
	if node == nil || database == nil || node.AuthProtocol == nil {
		return
	}

	localPeerID := node.Host.ID().String()
	msg := &groupJoinAckWire{
		V:       1,
		GroupID: groupID,
		PeerID:  localPeerID,
	}
	for _, pid := range node.Host.Network().Peers() {
		if pid.String() == localPeerID {
			continue
		}
		if !node.AuthProtocol.IsVerified(pid) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		s, err := node.Host.NewStream(ctx, pid, groupJoinAckProtocol)
		cancel()
		if err != nil {
			continue
		}
		_ = s.SetDeadline(time.Now().Add(10 * time.Second))
		_ = p2p.WriteInviteStoreJSONFrame(s, msg)
		_ = s.Close()
	}
}

func (r *Runtime) handleGroupJoinAckStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(15 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.RLock()
	node := r.node
	database := r.db
	auth := (*p2p.AuthProtocol)(nil)
	if node != nil {
		auth = node.AuthProtocol
	}
	r.mu.RUnlock()
	if node == nil || database == nil {
		return
	}
	if auth != nil && !auth.IsVerified(remote) {
		return
	}

	var msg groupJoinAckWire
	if err := p2p.ReadInviteStoreJSONFrame(s, &msg); err != nil || msg.V != 1 || strings.TrimSpace(msg.GroupID) == "" || strings.TrimSpace(msg.PeerID) == "" {
		return
	}
	if remote.String() != msg.PeerID {
		return
	}
	has, err := database.HasGroup(msg.GroupID)
	if err != nil || !has {
		return
	}
	// join-ack is a presence signal from a peer who just finished joining;
	// observation-only — no role information.
	if err := r.upsertGroupMemberFromRosterSync(msg.GroupID, msg.PeerID, "join-ack"); err != nil {
		return
	}
	r.emit("group:members_changed", map[string]interface{}{
		"group_id": msg.GroupID,
		"reason":   "joined_ack",
	})

	// Best-effort: terminate the matching group_add_operations lifecycle.
	// We look up the operation by (group, target) regardless of which
	// specific KeyPackage hash was used — the join ack is the
	// cryptographically authoritative signal that the invitee successfully
	// applied a Welcome we (or any Token Holder) authored.
	if rows, listErr := database.ListGroupAddOperationsForTarget(msg.GroupID, msg.PeerID); listErr == nil {
		for _, op := range rows {
			if op.Status == store.AddOpStatusWelcomeDelivered || op.Status == store.AddOpStatusFailed {
				continue
			}
			if mErr := database.MarkAddWelcomeDelivered(op.OperationID); mErr != nil &&
				!errors.Is(mErr, store.ErrAddOperationTerminal) {
				slog.Warn("MarkAddWelcomeDelivered failed",
					"operation", op.OperationID, "group", msg.GroupID, "err", mErr)
			}
		}
	}
}

func (r *Runtime) registerGroupJoinAckHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(groupJoinAckProtocol, func(s network.Stream) {
		go r.handleGroupJoinAckStream(s)
	})
}

func (r *Runtime) removeGroupJoinAckHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(groupJoinAckProtocol)
}

// ── Wails bindings ────────────────────────────────────────────────────────────

// CheckDHTWelcome is kept for backward compatibility with existing UI bindings.
// It now checks pending Welcome replicas from connected peers (not DHT).
func (r *Runtime) CheckDHTWelcome(groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	r.mu.Lock()
	node := r.node
	r.mu.Unlock()
	if node == nil {
		return fmt.Errorf("P2P node not running")
	}

	wb, groupType, categoryID, _, err := r.fetchWelcomeFromStorePeers(groupID)
	if err != nil {
		return fmt.Errorf("no pending invite found for group %q from connected peers", groupID)
	}
	if err := r.applyWelcome(groupID, groupType, hex.EncodeToString(wb), categoryID); err != nil {
		return err
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database != nil {
		_ = database.MarkPendingInviteAccepted(store.PendingInviteID(groupID, wb))
	}
	// Restore channel-category assignment from the inviter-side metadata so
	// users who go through the legacy CheckDHTWelcome path (rather than the
	// auto-join chokepoint) still see the correct category in the UI.
	r.applyChannelCategoryAfterAutoJoin(groupID, groupType, categoryID, "")
	return nil
}

// GetKPStatus returns whether the local node has a KeyPackage advertised.
func (r *Runtime) GetKPStatus() map[string]interface{} {
	r.mu.Lock()
	node := r.node
	database := r.db
	r.mu.Unlock()

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
	rt *Runtime
}

func (h *peerConnectedHook) Listen(network.Network, ma.Multiaddr)      {}
func (h *peerConnectedHook) ListenClose(network.Network, ma.Multiaddr) {}
func (h *peerConnectedHook) Connected(_ network.Network, c network.Conn) {
	p := c.RemotePeer()
	go h.rt.retryPendingWelcomes(p)
	// Keep key package replicas fresh whenever a verified peer connects.
	go h.rt.advertiseKeyPackage()
	go h.rt.scheduleOfflineSyncPull(p)
	go h.rt.scheduleReplicatedProfilePull(p)
	go h.rt.scheduleChannelCategorySync(p)
	go h.rt.flushPendingDeliveryAcksTo(p)
	go h.rt.emitNodeStatusChanged("peer_connected")
	go h.rt.emitAllGroupsMembersChanged("presence")
}
func (h *peerConnectedHook) Disconnected(network.Network, network.Conn) {
	go h.rt.emitNodeStatusChanged("peer_disconnected")
	go h.rt.emitAllGroupsMembersChanged("presence")
}
