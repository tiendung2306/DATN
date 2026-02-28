package p2p

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// AuthGater implements network.ConnectionGater.
//
// Strategy: allow all connections initially so the auth handshake can proceed.
// After a peer fails the handshake, blacklist it to block further connections.
// This is a "fail-closed on second attempt" design: new peers get one chance.
type AuthGater struct {
	blacklist sync.Map // peer.ID → struct{}
}

func NewAuthGater() *AuthGater {
	return &AuthGater{}
}

// Blacklist permanently blocks a peer that failed the auth handshake.
func (g *AuthGater) Blacklist(id peer.ID) {
	g.blacklist.Store(id, struct{}{})
}

func (g *AuthGater) isBlacklisted(id peer.ID) bool {
	_, ok := g.blacklist.Load(id)
	return ok
}

// InterceptPeerDial — block dialing to already-blacklisted peers.
func (g *AuthGater) InterceptPeerDial(p peer.ID) bool {
	return !g.isBlacklisted(p)
}

// InterceptAddrDial — always allow (address-level filtering not needed here).
func (g *AuthGater) InterceptAddrDial(_ peer.ID, _ ma.Multiaddr) bool {
	return true
}

// InterceptAccept — allow all inbound connections; auth happens at app layer.
func (g *AuthGater) InterceptAccept(_ network.ConnMultiaddrs) bool {
	return true
}

// InterceptSecured — block already-blacklisted peers after Noise handshake.
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
