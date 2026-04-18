package coordination

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ─── Transport ───────────────────────────────────────────────────────────────

// Transport abstracts the network layer so the coordination logic can be
// tested with in-memory channels instead of real libp2p connections.
//
// Production: wraps libp2p GossipSub (Publish/Subscribe) + direct streams (SendDirect).
// Test:       in-memory message routing between TestNode instances.
type Transport interface {
	// Publish broadcasts data to all subscribers of the given topic.
	Publish(ctx context.Context, topic string, data []byte) error

	// Subscribe registers a handler for messages arriving on the given topic.
	// Only one handler per topic is supported; calling Subscribe again on the
	// same topic replaces the previous handler.
	Subscribe(topic string, handler func(from peer.ID, data []byte)) error

	// Unsubscribe removes the handler for the given topic and leaves it.
	Unsubscribe(topic string) error

	// SendDirect sends data to a specific peer over a point-to-point stream.
	// Used for EpochNotification and state sync — not broadcast.
	SendDirect(ctx context.Context, to peer.ID, data []byte) error

	// LocalPeerID returns this node's peer identity.
	LocalPeerID() peer.ID

	// ConnectedPeers returns the set of currently connected peer IDs.
	ConnectedPeers() []peer.ID
}

// ─── Clock ───────────────────────────────────────────────────────────────────

// Clock abstracts time so that timeout-dependent logic (Token Holder failover,
// heartbeat intervals) can be tested deterministically with a fake clock.
//
// Production: delegates to time.Now() and time.After().
// Test:       FakeClock where Advance(d) triggers pending timers instantly.
type Clock interface {
	// Now returns the current time.
	Now() time.Time

	// After returns a channel that receives the current time after duration d.
	After(d time.Duration) <-chan time.Time
}

// ─── MLSEngine ───────────────────────────────────────────────────────────────

// MLSEngine abstracts the Rust crypto sidecar so the coordination layer can
// be unit-tested with a mock that returns deterministic fake states.
//
// All methods are stateless from the caller's perspective: GroupState bytes go
// in, new GroupState bytes come out. Go never inspects the contents — only
// Rust deserializes them into an OpenMLS Ratchet Tree.
//
// Production: adapter that converts []byte ↔ protobuf and calls gRPC.
// Test:       MockMLSEngine returning canned responses without Rust.
type MLSEngine interface {
	// CreateGroup initializes a new MLS group. The caller becomes the sole member.
	// Returns the initial group state and tree hash.
	CreateGroup(ctx context.Context, groupID string, signingKey []byte) (groupState, treeHash []byte, err error)

	// CreateProposal generates an MLS Proposal (Add, Remove, or Update).
	CreateProposal(ctx context.Context, groupState []byte, pType ProposalType, data []byte) (proposalBytes []byte, err error)

	// CreateCommit bundles one or more Proposals into a Commit, advancing the
	// group to the next epoch. Returns the Commit bytes, an optional Welcome
	// message for newly added members, the updated GroupState, and the new tree hash.
	CreateCommit(ctx context.Context, groupState []byte, proposals [][]byte) (commitBytes, welcomeBytes, newGroupState, newTreeHash []byte, err error)

	// ProcessCommit applies a Commit received from the Token Holder.
	// Returns the new group state and tree hash after applying the commit.
	ProcessCommit(ctx context.Context, groupState []byte, commitBytes []byte) (newGroupState, newTreeHash []byte, err error)

	// ProcessWelcome processes a Welcome message to join an existing group.
	// keyPackageBundlePrivate must be the opaque blob from GenerateKeyPackage (never shared OOB).
	// Returns the group state, tree hash, and MLS epoch at the joined state.
	ProcessWelcome(ctx context.Context, welcomeBytes, signingKey, keyPackageBundlePrivate []byte) (groupState, treeHash []byte, epoch uint64, err error)

	// GenerateKeyPackage builds a public KeyPackage and a private bundle blob for the invitee.
	GenerateKeyPackage(ctx context.Context, signingKey []byte) (keyPackageBytes, keyPackageBundlePrivate []byte, err error)

	// AddMembers performs an MLS add (proposal+commit+welcome) for the given KeyPackages.
	AddMembers(ctx context.Context, groupState []byte, keyPackages [][]byte) (commitBytes, welcomeBytes, newGroupState, newTreeHash []byte, err error)

	// EncryptMessage encrypts plaintext using the current epoch's application secret.
	EncryptMessage(ctx context.Context, groupState []byte, plaintext []byte) (ciphertext, newGroupState []byte, err error)

	// DecryptMessage decrypts an MLS ciphertext received from a group member.
	DecryptMessage(ctx context.Context, groupState []byte, ciphertext []byte) (plaintext, newGroupState []byte, err error)

	// ExternalJoin performs an external join into a group using its GroupInfo.
	// Used during fork healing when a node on the losing branch joins the winner.
	ExternalJoin(ctx context.Context, groupInfo, signingKey []byte) (groupState, commitBytes, treeHash []byte, err error)

	// ExportSecret derives a secret from the current MLS epoch using the
	// Exporter mechanism (RFC 9420 §8). Used for file transfer key derivation.
	ExportSecret(ctx context.Context, groupState []byte, label string, length int) (secret []byte, err error)
}

// ─── CoordinationStorage ─────────────────────────────────────────────────────

// CoordinationStorage abstracts database access so the coordination layer can
// be tested with an in-memory store instead of SQLite.
//
// Production: extends the existing db.Database with new tables (mls_groups,
//
//	coordination_state, and columns for HLC timestamps).
//
// Test:       simple map-based implementation.
type CoordinationStorage interface {
	// GetGroupRecord retrieves the MLS group state for the given group ID.
	// Returns ErrGroupNotFound if the group does not exist.
	GetGroupRecord(groupID string) (*GroupRecord, error)

	// SaveGroupRecord persists or updates the MLS group state.
	SaveGroupRecord(rec *GroupRecord) error

	// ListGroups returns all known groups.
	ListGroups() ([]*GroupRecord, error)

	// GetCoordState retrieves the coordination metadata for a group.
	// Returns ErrGroupNotFound if no coordination state exists for the group.
	GetCoordState(groupID string) (*CoordState, error)

	// SaveCoordState persists or updates the coordination metadata.
	SaveCoordState(state *CoordState) error

	// SaveMessage stores a decrypted message for UI display and history.
	SaveMessage(msg *StoredMessage) error

	// GetMessagesSince retrieves messages for a group with HLC timestamps
	// after the given lower bound, sorted by HLC ascending.
	GetMessagesSince(groupID string, after HLCTimestamp) ([]*StoredMessage, error)

	// AppendEnvelope stores a raw JSON Envelope for offline replay (MsgCommit / MsgApplication only).
	AppendEnvelope(groupID string, msgType MessageType, epoch uint64, ts HLCTimestamp, envelope []byte) (seq int64, err error)

	// GetEnvelopesSince returns envelopes with seq > afterSeq from this node's log, ordered by seq ASC.
	GetEnvelopesSince(groupID string, afterSeq int64, maxCount int) ([]*EnvelopeRecord, error)

	GetLatestSeq(groupID string) (int64, error)

	// PruneEnvelopes deletes rows older than cutoff (created_at unix) and caps rows per group.
	PruneEnvelopes(cutoffUnix int64, maxPerGroup int) (removed int, err error)

	RecordSyncAck(peerID string, groupID string, ackedSeq int64) error
	GetSyncAck(peerID string, groupID string) (int64, error)
	// GetMinAckedSeq returns the minimum ack across peerIDs; missing peers count as 0.
	GetMinAckedSeq(groupID string, peerIDs []string) (int64, error)

	EnqueuePendingDeliveryAck(targetPeerID, groupID string, ackedSeq int64) error
	ListPendingDeliveryAcksForTarget(targetPeerID string) ([]PendingDeliveryAckRow, error)
	DeletePendingDeliveryAck(id int64) error

	GetOfflinePullCursor(groupID, remotePeerID string) (lastRemoteSeq int64, err error)
	SetOfflinePullCursor(groupID, remotePeerID string, lastRemoteSeq int64) error

	// GetKnownGroupMembers returns the distinct sender IDs that have ever sent
	// a message in the group (from stored_messages). Used to identify recipients
	// for DHT offline mailbox push and senders to pull from.
	GetKnownGroupMembers(groupID string) ([]string, error)
}
