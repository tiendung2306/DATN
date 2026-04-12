package port

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// P2PRuntime abstracts what invite / status code needs from libp2p.
type P2PRuntime interface {
	LocalPeerID() peer.ID
	ConnectedPeerIDs() []peer.ID
	AuthDisplayName(pid peer.ID) string
	IsVerified(pid peer.ID) bool
	NewWelcomeStream(ctx context.Context, to peer.ID, protocol string) (StreamWriter, error)
}

// StreamWriter is the minimal stream API for welcome delivery.
type StreamWriter interface {
	Close() error
	SetDeadline(t time.Time) error
	Write(p []byte) (n int, err error)
}

// KeyPackageFetcher retrieves a peer's public KeyPackage bytes (DHT or direct).
type KeyPackageFetcher interface {
	FetchKeyPackage(ctx context.Context, target peer.ID) ([]byte, error)
}

// WelcomePublisher stores Welcome for offline pickup (e.g. DHT).
type WelcomePublisher interface {
	StoreWelcomeInDHT(ctx context.Context, invitee peer.ID, groupID string, welcome []byte) error
}

// WelcomeLookup pulls a Welcome from DHT for (localPeer, groupID).
type WelcomeLookup interface {
	FetchWelcomeFromDHT(ctx context.Context, local peer.ID, groupID string) ([]byte, error)
}

// KPAdvertiser publishes local public KeyPackage to DHT.
type KPAdvertiser interface {
	AdvertiseKeyPackage(ctx context.Context, local peer.ID, publicKP []byte) error
}
