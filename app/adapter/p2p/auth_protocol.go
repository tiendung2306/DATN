package p2p

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"app/admin"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	AuthProtocolID = "/app/auth/1.0.0"
	authTimeout    = 15 * time.Second
	maxTokenBytes  = 64 * 1024 // 64 KB sanity limit
)

type AuthHandshakeMsg struct {
	Token   *admin.InvitationToken `json:"token"`
	Session SessionClaim           `json:"session"`
}

type verifiedPeerState struct {
	Token          *admin.InvitationToken
	SessionStarted int64
}

// AuthProtocol manages the auth handshake on the /app/auth/1.0.0 stream.
//
// Wire format for each token:
//
//	[4 bytes big-endian uint32: JSON length][JSON bytes of InvitationToken]
//
// Handshake direction (prevents deadlock):
//   - Client (initiator, opens stream): SEND first, then READ
//   - Server (acceptor, SetStreamHandler): READ first, then SEND
//
// Auth state machine (per connection attempt):
//
//	Connected → Handshaking → Verified        (crypto ok → store in verifiedPeers)
//	                        → SecurityFail    (bad sig / expired / PeerID mismatch
//	                                           → blacklist peer + close connection)
//	                        → TransientFail   (IO error / timeout / stream hiccup
//	                                           → reset stream only, no blacklist)
//
// Verified state lifecycle:
//   - Set on successful handshake (inbound or outbound).
//   - Cleared when the last connection to the peer is closed (Disconnected event).
//   - This ensures reconnecting peers always go through a full handshake rather
//     than hitting stale in-memory state from a previous session.
type AuthProtocol struct {
	host             host.Host
	gater            *AuthGater
	localToken       *admin.InvitationToken
	localHandshake   *AuthHandshakeMsg
	rootPublicKey    ed25519.PublicKey
	verifiedPeers    sync.Map // peer.ID → *verifiedPeerState
	handshakingPeers sync.Map // peer.ID → struct{} (outbound handshake in progress)
}

// NewAuthProtocol registers the stream handler and returns the AuthProtocol.
func NewAuthProtocol(
	h host.Host,
	gater *AuthGater,
	localToken *admin.InvitationToken,
	rootPubKey []byte,
	localHandshake *AuthHandshakeMsg,
) *AuthProtocol {
	ap := &AuthProtocol{
		host:           h,
		gater:          gater,
		localToken:     localToken,
		localHandshake: localHandshake,
		rootPublicKey:  ed25519.PublicKey(rootPubKey),
	}
	h.SetStreamHandler(AuthProtocolID, ap.handleIncoming)
	return ap
}

// IsVerified returns true if the peer completed a successful auth handshake
// in the current session.
func (ap *AuthProtocol) IsVerified(id peer.ID) bool {
	_, ok := ap.verifiedPeers.Load(id)
	return ok
}

// GetVerifiedToken returns the stored InvitationToken for a verified peer.
func (ap *AuthProtocol) GetVerifiedToken(id peer.ID) *admin.InvitationToken {
	v, ok := ap.verifiedPeers.Load(id)
	if !ok {
		return nil
	}
	return v.(*verifiedPeerState).Token
}

// handleIncoming is the server-side handler: reads peer token first, then sends own.
//
// Every incoming stream is always fully processed — there is no early return for
// "already verified" peers. This is intentional: a peer that reconnects after a
// restart will open a new stream and must re-authenticate. Skipping the read would
// cause the client to hang waiting for a response, fail with an IO error, and in
// the old design incorrectly blacklist a legitimate peer.
func (ap *AuthProtocol) handleIncoming(s network.Stream) {
	defer s.Close()
	peerID := s.Conn().RemotePeer()

	if err := s.SetDeadline(time.Now().Add(authTimeout)); err != nil {
		slog.Debug("auth: failed to set deadline", "peer", peerID)
	}

	// Server reads first.
	peerHandshake, err := readHandshake(s)
	if err != nil {
		slog.Debug("auth: transient read error (inbound)", "peer", peerID, "err", err)
		ap.rejectTransient(s)
		return
	}
	peerToken := peerHandshake.Token

	if err := ap.verifyPeerToken(peerToken, peerID); err != nil {
		slog.Warn("auth: security verification failed (inbound)", "peer", peerID, "reason", err)
		ap.rejectSecurity(s, peerID, err.Error())
		return
	}

	sessionStarted := peerHandshake.Session.StartedAt
	if sessionStarted > 0 {
		if err := VerifySessionClaim(peerHandshake.Session, peerToken.PeerID, peerToken.PublicKey); err != nil {
			slog.Warn("auth: security verification failed (inbound claim)", "peer", peerID, "reason", err)
			ap.rejectSecurity(s, peerID, err.Error())
			return
		}
	}

	if !ap.acceptSession(peerID, sessionStarted, s.Conn()) {
		slog.Warn("auth: rejecting stale session", "peer", peerID, "started_at", peerHandshake.Session.StartedAt)
		ap.rejectTransient(s)
		return
	}

	// Server sends own token.
	if err := writeHandshake(s, ap.localHandshake); err != nil {
		slog.Debug("auth: transient write error (inbound)", "peer", peerID, "err", err)
		ap.rejectTransient(s)
		return
	}

	ap.verifiedPeers.Store(peerID, &verifiedPeerState{
		Token:          peerToken,
		SessionStarted: sessionStarted,
	})
	slog.Info("auth: peer verified (inbound)", "peer", peerID, "name", peerToken.DisplayName)
}

// InitiateHandshake is the client-side handler: sends own token first, then reads peer's.
// Call this after establishing any new outbound connection.
//
// The IsVerified guard prevents redundant re-handshakes when mDNS re-discovers a
// peer that is already live (no new connection, same session). It does NOT block
// reconnects: Disconnected clears the verified state, so a restarted peer will
// always trigger a fresh handshake.
func (ap *AuthProtocol) InitiateHandshake(ctx context.Context, peerID peer.ID) {
	if ap.IsVerified(peerID) {
		return // already verified on this active session
	}
	if _, loaded := ap.handshakingPeers.LoadOrStore(peerID, struct{}{}); loaded {
		return // another outbound handshake is already in progress
	}
	defer ap.handshakingPeers.Delete(peerID)

	s, err := ap.host.NewStream(ctx, peerID, AuthProtocolID)
	if err != nil {
		// Transient: stream open can fail if the remote is still initializing.
		// Never blacklist here — it would permanently block a legitimate peer.
		slog.Debug("auth: transient stream open failure (outbound)", "peer", peerID, "err", err)
		return
	}
	defer s.Close()

	if err := s.SetDeadline(time.Now().Add(authTimeout)); err != nil {
		slog.Debug("auth: failed to set deadline", "peer", peerID)
	}

	// Client sends first.
	if err := writeHandshake(s, ap.localHandshake); err != nil {
		if ap.IsVerified(peerID) {
			slog.Debug("auth: transient write ignored after verify (outbound)", "peer", peerID, "err", err)
		} else {
			slog.Debug("auth: transient write error (outbound)", "peer", peerID, "err", err)
		}
		ap.rejectTransient(s)
		return
	}

	// Client reads peer's token.
	peerHandshake, err := readHandshake(s)
	if err != nil {
		if ap.IsVerified(peerID) {
			slog.Debug("auth: transient read ignored after verify (outbound)", "peer", peerID, "err", err)
		} else {
			slog.Debug("auth: transient read error (outbound)", "peer", peerID, "err", err)
		}
		ap.rejectTransient(s)
		return
	}
	peerToken := peerHandshake.Token

	if err := ap.verifyPeerToken(peerToken, peerID); err != nil {
		slog.Warn("auth: security verification failed (outbound)", "peer", peerID, "reason", err)
		ap.rejectSecurity(s, peerID, err.Error())
		return
	}

	sessionStarted := peerHandshake.Session.StartedAt
	if sessionStarted > 0 {
		if err := VerifySessionClaim(peerHandshake.Session, peerToken.PeerID, peerToken.PublicKey); err != nil {
			slog.Warn("auth: security verification failed (outbound claim)", "peer", peerID, "reason", err)
			ap.rejectSecurity(s, peerID, err.Error())
			return
		}
	}
	if !ap.acceptSession(peerID, sessionStarted, s.Conn()) {
		slog.Warn("auth: rejecting stale session (outbound)", "peer", peerID, "started_at", peerHandshake.Session.StartedAt)
		ap.rejectTransient(s)
		return
	}

	ap.verifiedPeers.Store(peerID, &verifiedPeerState{
		Token:          peerToken,
		SessionStarted: sessionStarted,
	})
	slog.Info("auth: peer verified (outbound)", "peer", peerID, "name", peerToken.DisplayName)
}

func (ap *AuthProtocol) acceptSession(peerID peer.ID, startedAt int64, acceptedConn network.Conn) bool {
	v, ok := ap.verifiedPeers.Load(peerID)
	if ok {
		existing := v.(*verifiedPeerState)
		if startedAt < existing.SessionStarted {
			return false
		}
	}

	// Evict other concurrent connections for this PeerID if this session is newer.
	for _, conn := range ap.host.Network().ConnsToPeer(peerID) {
		if acceptedConn != nil && conn.ID() == acceptedConn.ID() {
			continue
		}
		if err := conn.Close(); err != nil {
			slog.Debug("auth: close old conn failed", "peer", peerID, "err", err)
		}
	}
	return true
}

// verifyPeerToken performs three cryptographic checks on a received token.
func (ap *AuthProtocol) verifyPeerToken(token *admin.InvitationToken, authenticatedPeerID peer.ID) error {
	// 1. Admin signature — proves the token was issued by the trusted root key.
	if !admin.VerifyToken(token, ap.rootPublicKey) {
		return fmt.Errorf("invalid Admin signature")
	}
	// 2. Expiry — tokens are valid for 365 days from issuance.
	if time.Now().Unix() > token.ExpiresAt {
		return fmt.Errorf("token expired at %d", token.ExpiresAt)
	}
	// 3. PeerID binding — token.PeerID must match the Noise-authenticated identity.
	// This prevents Eve from replaying Alice's token: she cannot forge Alice's Noise key.
	if token.PeerID != authenticatedPeerID.String() {
		return fmt.Errorf("PeerID mismatch: token=%s noise=%s",
			token.PeerID, authenticatedPeerID)
	}
	return nil
}

// rejectSecurity handles cryptographic auth failures.
// Blacklists the peer (with TTL) and closes the connection.
// Only call when verifyPeerToken returns an error.
func (ap *AuthProtocol) rejectSecurity(s network.Stream, peerID peer.ID, reason string) {
	s.Reset()                           //nolint:errcheck
	ap.gater.Blacklist(peerID, reason)  // TTL-based; peer can retry after expiry
	ap.host.Network().ClosePeer(peerID) //nolint:errcheck
}

// rejectTransient handles IO/timing failures (EOF, timeout, stream reset).
// Resets the stream only — does NOT blacklist the peer.
// The peer may be reconnecting, restarting, or experiencing a brief network hiccup.
func (ap *AuthProtocol) rejectTransient(s network.Stream) {
	s.Reset() //nolint:errcheck
}

// ─── Network Notifee ──────────────────────────────────────────────────────────

// authNetworkNotifee triggers auth handshakes and maintains verified-peer state.
type authNetworkNotifee struct {
	ap *AuthProtocol
}

func (n *authNetworkNotifee) Connected(_ network.Network, c network.Conn) {
	// Only initiate from our side for outbound connections to avoid both sides
	// racing as "client". For inbound connections the peer opens the stream first.
	if c.Stat().Direction == network.DirOutbound {
		go n.ap.InitiateHandshake(context.Background(), c.RemotePeer())
	}
}

// Disconnected clears the verified state when the peer has no more live connections.
//
// This is the key to correct reconnect behaviour: without this, a restarted peer
// would arrive with empty verifiedPeers, open a new auth stream, but the server
// side would skip the handshake (old "IsVerified" guard) and close without
// responding — causing the client to fail with an IO error.
func (n *authNetworkNotifee) Disconnected(_ network.Network, c network.Conn) {
	peerID := c.RemotePeer()
	// Only evict if this was the last connection to the peer.
	// A peer may have simultaneous TCP + QUIC connections; keep the verified state
	// until all of them are gone.
	if len(n.ap.host.Network().ConnsToPeer(peerID)) == 0 {
		n.ap.verifiedPeers.Delete(peerID)
		slog.Debug("auth: verified state cleared (last connection closed)", "peer", peerID)
	}
}

func (n *authNetworkNotifee) Listen(_ network.Network, _ ma.Multiaddr)      {}
func (n *authNetworkNotifee) ListenClose(_ network.Network, _ ma.Multiaddr) {}

// ─── Wire format helpers ──────────────────────────────────────────────────────

func writeHandshake(w io.Writer, hs *AuthHandshakeMsg) error {
	if hs == nil || hs.Token == nil {
		return fmt.Errorf("empty auth handshake")
	}
	data, err := json.Marshal(hs)
	if err != nil {
		return fmt.Errorf("marshal handshake: %w", err)
	}
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := w.Write(lenBuf); err != nil {
		return fmt.Errorf("write length prefix: %w", err)
	}
	_, err = w.Write(data)
	return err
}

func readHandshake(r io.Reader) (*AuthHandshakeMsg, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, fmt.Errorf("read length prefix: %w", err)
	}
	length := binary.BigEndian.Uint32(lenBuf)
	if length == 0 || length > maxTokenBytes {
		return nil, fmt.Errorf("invalid token length: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read token data: %w", err)
	}
	var hs AuthHandshakeMsg
	if err := json.Unmarshal(data, &hs); err == nil && hs.Token != nil {
		return &hs, nil
	}
	// Backward compatibility: older nodes send raw InvitationToken.
	var token admin.InvitationToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("unmarshal handshake/token: %w", err)
	}
	return &AuthHandshakeMsg{
		Token: &token,
		Session: SessionClaim{
			StartedAt: 0, // legacy node without session claims
		},
	}, nil
}
