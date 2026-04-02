package coordination

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ─── HLC Timestamp ───────────────────────────────────────────────────────────

// HLCTimestamp is a Hybrid Logical Clock timestamp that provides causal
// consistency and total ordering for application messages across P2P nodes
// without requiring synchronized wall clocks (NTP-independent).
//
// Comparison is lexicographic: (WallTimeMs, Counter, NodeID).
type HLCTimestamp struct {
	WallTimeMs int64  `json:"l"`  // physical component (unix milliseconds)
	Counter    uint32 `json:"c"`  // logical counter for events at same WallTimeMs
	NodeID     string `json:"id"` // deterministic tiebreaker (peer.ID string)
}

// Before returns true if t is ordered before other.
func (t HLCTimestamp) Before(other HLCTimestamp) bool {
	if t.WallTimeMs != other.WallTimeMs {
		return t.WallTimeMs < other.WallTimeMs
	}
	if t.Counter != other.Counter {
		return t.Counter < other.Counter
	}
	return t.NodeID < other.NodeID
}

// Equal returns true if t and other represent the same logical instant.
func (t HLCTimestamp) Equal(other HLCTimestamp) bool {
	return t.WallTimeMs == other.WallTimeMs &&
		t.Counter == other.Counter &&
		t.NodeID == other.NodeID
}

// IsZero returns true if the timestamp has not been initialized.
func (t HLCTimestamp) IsZero() bool {
	return t.WallTimeMs == 0 && t.Counter == 0 && t.NodeID == ""
}

// ─── Wire Message Types ──────────────────────────────────────────────────────

// MessageType identifies the kind of message inside an Envelope.
type MessageType string

const (
	MsgProposal    MessageType = "proposal"
	MsgCommit      MessageType = "commit"
	MsgHeartbeat   MessageType = "heartbeat"
	MsgAnnounce    MessageType = "announce"
	MsgEpochNotify MessageType = "epoch_notify"
	MsgApplication MessageType = "application"
)

// Envelope is the top-level wire format for all coordination messages.
// All message types within a group share a single GossipSub topic and are
// demuxed by the Type field. Direct peer-to-peer messages (e.g. EpochNotify)
// use Transport.SendDirect and still wrap content in an Envelope.
type Envelope struct {
	Type      MessageType     `json:"type"`
	GroupID   string          `json:"group_id"`
	Epoch     uint64          `json:"epoch"`
	From      string          `json:"from"` // peer.ID.String()
	Timestamp HLCTimestamp    `json:"ts"`
	Payload   json.RawMessage `json:"payload"`
}

// ProposalMsg carries an MLS Proposal created by any group member.
// Only the current Token Holder may bundle these into a Commit.
type ProposalMsg struct {
	ProposalType ProposalType `json:"proposal_type"`
	Data         []byte       `json:"data"` // MLS Proposal bytes (opaque)
}

// CommitMsg carries an MLS Commit created by the Token Holder.
// Advances the group from epoch E to E+1.
type CommitMsg struct {
	CommitData []byte `json:"commit_data"` // MLS Commit bytes
	// WelcomeData is intentionally not broadcast in normal flow (MLS Welcome is OOB).
	// Kept for backward compatibility with older envelopes.
	WelcomeData []byte `json:"welcome_data,omitempty"`
	NewTreeHash []byte `json:"new_tree_hash"`
}

// HeartbeatMsg is a lightweight liveness signal broadcast periodically.
type HeartbeatMsg struct {
	// No extra fields — From + Epoch in Envelope are sufficient.
}

// GroupStateAnnouncement is broadcast periodically to detect network partitions.
// Nodes compare TreeHash values; divergence indicates a fork.
type GroupStateAnnouncement struct {
	TreeHash    []byte `json:"tree_hash"`
	MemberCount int    `json:"member_count"` // online members in this branch
	CommitHash  []byte `json:"commit_hash"`  // hash of last Commit (tiebreaker)
}

// EpochNotification is sent directly to a peer whose message carried a stale
// epoch, informing them of the current state so they can sync.
type EpochNotification struct {
	CurrentEpoch uint64 `json:"current_epoch"`
	TreeHash     []byte `json:"tree_hash"`
}

// ApplicationMsg carries an MLS-encrypted chat message.
type ApplicationMsg struct {
	Ciphertext []byte `json:"ciphertext"` // MLS ciphertext bytes
}

// ─── Persistence Types ───────────────────────────────────────────────────────

// GroupRole indicates the local node's relationship to a group.
type GroupRole string

const (
	RoleCreator GroupRole = "creator"
	RoleMember  GroupRole = "member"
)

// GroupRecord is the persistent state of an MLS group stored in SQLite.
// GroupState is opaque bytes from Go's perspective; only Rust deserializes it.
type GroupRecord struct {
	GroupID    string
	GroupState []byte // serialized OpenMLS group state
	Epoch      uint64
	TreeHash   []byte
	MyRole     GroupRole
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CoordState is the persistent coordination metadata for a group.
type CoordState struct {
	GroupID          string
	ActiveView       []peer.ID
	TokenHolder      peer.ID
	LastCommitHash   []byte
	LastCommitAt     time.Time
	PendingProposals [][]byte // buffered MLS Proposal bytes
}

// StoredMessage is a decrypted message stored in SQLite for UI display.
type StoredMessage struct {
	GroupID   string
	Epoch     uint64
	SenderID  peer.ID
	Content   []byte
	Timestamp HLCTimestamp
}

// ─── Enum Types ──────────────────────────────────────────────────────────────

// ProposalType identifies the kind of MLS Proposal.
type ProposalType int

const (
	ProposalAdd ProposalType = iota
	ProposalRemove
	ProposalUpdate
)

// EpochAction is the result of validating an incoming message's epoch
// against the local epoch.
type EpochAction int

const (
	ActionProcess     EpochAction = iota // epoch matches — process normally
	ActionRejectStale                    // sender is behind — send EpochNotification
	ActionBufferFuture                   // sender is ahead — buffer and request sync
)

// BranchResult is the outcome of comparing two branches during fork healing.
type BranchResult int

const (
	BranchLocal  BranchResult = iota // local branch wins
	BranchRemote                     // remote branch wins
	BranchEqual                      // branches are identical
)

// ─── GossipSub Topic Naming ──────────────────────────────────────────────────

// GroupTopic returns the GossipSub topic name for a given group.
// All coordination and application messages for the group share this topic.
func GroupTopic(groupID string) string {
	return "/org/group/" + groupID
}

// ─── Sentinel Errors ─────────────────────────────────────────────────────────

var (
	ErrNotTokenHolder = errors.New("coordination: not the current epoch token holder")
	ErrStaleEpoch     = errors.New("coordination: message epoch is behind local epoch")
	ErrFutureEpoch    = errors.New("coordination: message epoch is ahead of local epoch")
	ErrGroupNotFound  = errors.New("coordination: group not found")
	ErrNoActiveView   = errors.New("coordination: active view is empty")
	ErrInvalidConfig  = errors.New("coordination: invalid configuration")
)
