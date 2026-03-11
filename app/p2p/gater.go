package p2p

import (
	"log/slog"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// blacklistTTL is how long a security-failed peer stays blocked.
// After TTL expires the peer gets one fresh chance on next connection attempt.
const blacklistTTL = 30 * time.Minute

type blacklistEntry struct {
	until  time.Time
	reason string
}

// AuthGater implements network.ConnectionGater.
//
// Strategy: allow all connections initially so the auth handshake can proceed.
// Only peers that fail *cryptographic* verification are blacklisted (invalid Admin
// signature, expired token, PeerID mismatch). IO errors and transient failures are
// never blacklisted — they are handled at stream level with a simple reset.
//
// Blacklist entries expire after blacklistTTL so a misconfigured peer can recover
// without requiring a full app restart.
type AuthGater struct {
	mu      sync.Mutex
	entries map[peer.ID]blacklistEntry
}

func NewAuthGater() *AuthGater {
	return &AuthGater{entries: make(map[peer.ID]blacklistEntry)}
}

// Blacklist blocks a peer that failed cryptographic auth for blacklistTTL.
// reason is logged and stored for diagnostics.
func (g *AuthGater) Blacklist(id peer.ID, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.entries[id] = blacklistEntry{
		until:  time.Now().Add(blacklistTTL),
		reason: reason,
	}
	slog.Warn("gater: peer blacklisted",
		"peer", id,
		"reason", reason,
		"expires_in", blacklistTTL,
	)
}

// isBlacklisted checks whether the peer is currently blocked.
// Expired entries are evicted lazily on access.
func (g *AuthGater) isBlacklisted(id peer.ID) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.entries[id]
	if !ok {
		return false
	}
	if time.Now().After(entry.until) {
		delete(g.entries, id)
		slog.Info("gater: blacklist entry expired, peer re-allowed", "peer", id)
		return false
	}
	return true
}

// InterceptPeerDial — block dialing to blacklisted peers.
func (g *AuthGater) InterceptPeerDial(p peer.ID) bool {
	return !g.isBlacklisted(p)
}

// InterceptAddrDial — always allow; address-level filtering not needed here.
func (g *AuthGater) InterceptAddrDial(_ peer.ID, _ ma.Multiaddr) bool {
	return true
}

// InterceptAccept — allow all inbound connections; auth happens at app layer.
func (g *AuthGater) InterceptAccept(_ network.ConnMultiaddrs) bool {
	return true
}

// InterceptSecured — block blacklisted peers after Noise handshake.
func (g *AuthGater) InterceptSecured(_ network.Direction, id peer.ID, _ network.ConnMultiaddrs) bool {
	return !g.isBlacklisted(id)
}

// InterceptUpgraded — final gate; rejects blacklisted peers after full upgrade.
func (g *AuthGater) InterceptUpgraded(c network.Conn) (bool, control.DisconnectReason) {
	if g.isBlacklisted(c.RemotePeer()) {
		return false, control.DisconnectReason(0)
	}
	return true, control.DisconnectReason(0)
}
