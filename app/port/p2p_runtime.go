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

// KeyPackageFetcher retrieves a peer's public KeyPackage bytes (direct/store peers).
type KeyPackageFetcher interface {
	FetchKeyPackage(ctx context.Context, target peer.ID) ([]byte, error)
}

// WelcomePublisher stores Welcome replicas for offline pickup by invitee.
type WelcomePublisher interface {
	StoreWelcomeReplica(ctx context.Context, invitee peer.ID, groupID string, welcome []byte) error
}

// WelcomeLookup pulls a Welcome replica for (localPeer, groupID).
type WelcomeLookup interface {
	FetchWelcome(ctx context.Context, local peer.ID, groupID string) ([]byte, error)
}

// KPAdvertiser replicates local public KeyPackage to store peers.
type KPAdvertiser interface {
	ReplicateKeyPackage(ctx context.Context, local peer.ID, publicKP []byte) error
}
