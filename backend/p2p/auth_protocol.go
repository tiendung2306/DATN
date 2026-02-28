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

	"backend/admin"

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

// AuthProtocol manages the auth handshake on the /app/auth/1.0.0 stream.
//
// Wire format for each token:
//   [4 bytes big-endian uint32: JSON length][JSON bytes of InvitationToken]
//
// Handshake direction (prevents deadlock):
//   - Client (initiator, opens stream): SEND first, then READ
//   - Server (acceptor, SetStreamHandler): READ first, then SEND
type AuthProtocol struct {
	host          host.Host
	gater         *AuthGater
	localToken    *admin.InvitationToken
	rootPublicKey ed25519.PublicKey
	verifiedPeers sync.Map // peer.ID → *admin.InvitationToken
}

// NewAuthProtocol registers the stream handler and returns the AuthProtocol.
func NewAuthProtocol(
	h host.Host,
	gater *AuthGater,
	localToken *admin.InvitationToken,
	rootPubKey []byte,
) *AuthProtocol {
	ap := &AuthProtocol{
		host:          h,
		gater:         gater,
		localToken:    localToken,
		rootPublicKey: ed25519.PublicKey(rootPubKey),
	}
	h.SetStreamHandler(AuthProtocolID, ap.handleIncoming)
	return ap
}

// IsVerified returns true if the peer completed a successful auth handshake.
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
	return v.(*admin.InvitationToken)
}

// handleIncoming is the server-side handler: reads peer token first, then sends own.
// Registered via SetStreamHandler; called automatically by libp2p on new streams.
func (ap *AuthProtocol) handleIncoming(s network.Stream) {
	defer s.Close()
	peerID := s.Conn().RemotePeer()

	if ap.IsVerified(peerID) {
		return // duplicate handshake, ignore
	}

	if err := s.SetDeadline(time.Now().Add(authTimeout)); err != nil {
		slog.Debug("auth: failed to set deadline", "peer", peerID)
	}

	// Server reads first
	peerToken, err := readToken(s)
	if err != nil {
		slog.Warn("auth: failed to read peer token", "peer", peerID, "err", err)
		ap.reject(s, peerID)
		return
	}

	if err := ap.verifyPeerToken(peerToken, peerID); err != nil {
		slog.Warn("auth: peer token invalid", "peer", peerID, "err", err)
		ap.reject(s, peerID)
		return
	}

	// Server sends own token
	if err := writeToken(s, ap.localToken); err != nil {
		slog.Warn("auth: failed to send own token", "peer", peerID, "err", err)
		return
	}

	ap.verifiedPeers.Store(peerID, peerToken)
	slog.Info("auth: peer verified (incoming)", "peer", peerID, "name", peerToken.DisplayName)
}

// InitiateHandshake is the client-side handler: sends own token first, then reads peer's.
// Call this after establishing any new outbound connection.
func (ap *AuthProtocol) InitiateHandshake(ctx context.Context, peerID peer.ID) {
	if ap.IsVerified(peerID) {
		return // already verified
	}

	s, err := ap.host.NewStream(ctx, peerID, AuthProtocolID)
	if err != nil {
		// Do NOT blacklist here — stream failure is typically transient
		// (peer reconnecting, brief network hiccup). Blacklisting would
		// permanently block a legitimate peer for the rest of the session.
		slog.Warn("auth: failed to open stream to peer", "peer", peerID, "err", err)
		return
	}
	defer s.Close()

	if err := s.SetDeadline(time.Now().Add(authTimeout)); err != nil {
		slog.Debug("auth: failed to set deadline", "peer", peerID)
	}

	// Client sends first
	if err := writeToken(s, ap.localToken); err != nil {
		slog.Warn("auth: failed to send own token", "peer", peerID, "err", err)
		ap.reject(s, peerID)
		return
	}

	// Client reads peer's token
	peerToken, err := readToken(s)
	if err != nil {
		slog.Warn("auth: failed to read peer token", "peer", peerID, "err", err)
		ap.reject(s, peerID)
		return
	}

	if err := ap.verifyPeerToken(peerToken, peerID); err != nil {
		slog.Warn("auth: peer token invalid", "peer", peerID, "err", err)
		ap.reject(s, peerID)
		return
	}

	ap.verifiedPeers.Store(peerID, peerToken)
	slog.Info("auth: peer verified (outgoing)", "peer", peerID, "name", peerToken.DisplayName)
}

// verifyPeerToken performs all three checks for a received token.
func (ap *AuthProtocol) verifyPeerToken(token *admin.InvitationToken, authenticatedPeerID peer.ID) error {
	// 1. Admin signature
	if !admin.VerifyToken(token, ap.rootPublicKey) {
		return fmt.Errorf("invalid Admin signature")
	}
	// 2. Not expired
	if time.Now().Unix() > token.ExpiresAt {
		return fmt.Errorf("token expired at %d", token.ExpiresAt)
	}
	// 3. PeerID binding — token.PeerID must match Noise-authenticated peer identity.
	// This prevents Eve from replaying Alice's token: Eve cannot forge Alice's Noise key.
	if token.PeerID != authenticatedPeerID.String() {
		return fmt.Errorf("token PeerID mismatch: token=%s, noise_peer=%s",
			token.PeerID, authenticatedPeerID)
	}
	return nil
}

func (ap *AuthProtocol) reject(s network.Stream, peerID peer.ID) {
	s.Reset() //nolint:errcheck
	ap.gater.Blacklist(peerID)
	ap.host.Network().ClosePeer(peerID) //nolint:errcheck
}

// ─── Network Notifee ──────────────────────────────────────────────────────────

// authNetworkNotifee triggers InitiateHandshake on every new outbound-originated connection.
// It covers mDNS, DHT, and manually bootstrapped peers — no manual wiring needed.
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

func (n *authNetworkNotifee) Disconnected(_ network.Network, _ network.Conn) {}
func (n *authNetworkNotifee) Listen(_ network.Network, _ ma.Multiaddr)        {}
func (n *authNetworkNotifee) ListenClose(_ network.Network, _ ma.Multiaddr)   {}

// ─── Wire format helpers ──────────────────────────────────────────────────────

func writeToken(w io.Writer, token *admin.InvitationToken) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := w.Write(lenBuf); err != nil {
		return fmt.Errorf("write length prefix: %w", err)
	}
	_, err = w.Write(data)
	return err
}

func readToken(r io.Reader) (*admin.InvitationToken, error) {
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
	var token admin.InvitationToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}
	return &token, nil
}
