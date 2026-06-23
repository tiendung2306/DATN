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
	MsgEpochNotify        MessageType = "epoch_notify"
	MsgApplication        MessageType = "application"
	MsgApplicationBatched MessageType = "application_batched"
	MsgDeliveryAck        MessageType = "delivery_ack"
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
//
// For ProposalAdd the envelope additionally carries application-layer routing
// metadata (OperationID, TargetPeerID, RequestID, GroupType, CategoryID,
// KeyPackageHash) so that whichever node ends up being the Token Holder can
// (a) deliver the resulting Welcome out-of-band to the correct invitee and
// (b) correlate the eventual commit with the originating
// group_add_operations / group_invite_requests rows on every node that
// observes the commit. None of these metadata fields participate in the MLS
// cryptographic state — they are routing/audit data only.
type ProposalMsg struct {
	ProposalType ProposalType `json:"proposal_type"`
	Data         []byte       `json:"data"` // MLS Proposal bytes (opaque)
	ProposalRef  []byte       `json:"proposal_ref,omitempty"`

	// Application-layer correlation fields. Populated for ProposalAdd and for
	// ProposalRemove.TargetPeerID so Token Holder election can exclude members
	// removed by the candidate batch. Older wire frames that omit these keys
	// still parse cleanly.
	OperationID    string `json:"operation_id,omitempty"`
	TargetPeerID   string `json:"target_peer_id,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	GroupType      string `json:"group_type,omitempty"`
	CategoryID     string `json:"category_id,omitempty"`
	KeyPackageHash []byte `json:"key_package_hash,omitempty"`
}

// AddCommitDelivery describes a single ProposalAdd that has been bundled into
// a Commit. The Token Holder emits one entry per invitee in the batch so:
//   - on the holder, the runtime knows which (operation, target, group_type,
//     category, request) to attach to the freshly minted Welcome bytes
//     produced by mls.CreateCommit / mls.AddMembers;
//   - on every other receiver, the commit observer can transition the
//     matching group_add_operations row to "commit_observed" even though it
//     never sees the Welcome itself.
//
// WelcomeHash is a stable fingerprint (SHA-256) of the Welcome bytes the
// Token Holder produced. Receivers use it purely for audit / idempotency
// keys; they MUST NOT attempt to reconstruct Welcome material from it.
type AddCommitDelivery struct {
	OperationID    string `json:"operation_id"`
	TargetPeerID   string `json:"target_peer_id"`
	RequestID      string `json:"request_id,omitempty"`
	GroupType      string `json:"group_type,omitempty"`
	CategoryID     string `json:"category_id,omitempty"`
	KeyPackageHash []byte `json:"key_package_hash,omitempty"`
	WelcomeHash    []byte `json:"welcome_hash,omitempty"`
}

// CommitMsg carries an MLS Commit created by the Token Holder.
// Advances the group from epoch E to E+1.
//
// AddDeliveries is populated by the Token Holder when the commit batch
// contains one or more ProposalAdd entries. It is metadata-only; the
// authoritative MLS state still comes from CommitData.
type CommitMsg struct {
	CommitData []byte `json:"commit_data"` // MLS Commit bytes
	// WelcomeData is intentionally not broadcast in normal flow (MLS Welcome is OOB).
	// Kept for backward compatibility with older envelopes.
	WelcomeData   []byte              `json:"welcome_data,omitempty"`
	NewTreeHash   []byte              `json:"new_tree_hash"`
	AddDeliveries []AddCommitDelivery `json:"add_deliveries,omitempty"`
	// IncludedProposals carries the raw standalone MLS proposal messages that
	// the Commit references, plus Go metadata needed for deterministic policy checks.
	IncludedProposals     []ProposalMsg `json:"included_proposals,omitempty"`
	CommittedProposalRefs [][]byte      `json:"committed_proposal_refs,omitempty"`
}

// BufferedProposal is one entry in the SingleWriter buffer. It preserves both
// the raw MLS proposal bytes (consumed verbatim by mls.CreateCommit) and the
// Go-level routing metadata (consumed by the Token Holder to populate
// CommitMsg.AddDeliveries and to dispatch the resulting Welcome). The
// metadata fields are only meaningful for ProposalAdd.
type BufferedProposal struct {
	Type           ProposalType
	Data           []byte
	ProposalRef    []byte
	OperationID    string
	TargetPeerID   string
	RequestID      string
	GroupType      string
	CategoryID     string
	KeyPackageHash []byte
}

// AuthorizedCommittersProvider projects application policy into the
// coordination layer. The coordinator passes the candidate commit batch so
// callers can choose policy by proposal type without the coordinator knowing
// business roles such as creator/admin.
type AuthorizedCommittersProvider func(groupID string, epoch uint64, candidateBatch []BufferedProposal) ([]peer.ID, error)

// RemoveMemberRequest carries both the MLS identity required by Rust and the
// peer-level target used by the coordination election layer.
type RemoveMemberRequest struct {
	TargetPeerID   peer.ID
	TargetIdentity []byte
	OperationID    string
}

// AddMemberResult is the structured outcome of Coordinator.AddMember.
//
//   - When the local node is the current Token Holder, AddMember commits
//     synchronously; Welcome contains the MLS Welcome bytes for the invitee
//     and CommitEpoch / Delivery describe the freshly produced commit.
//   - When the local node is NOT the Token Holder, AddMember broadcasts a
//     ProposalAdd carrying the same routing metadata, returns Deferred=true,
//     and leaves Welcome empty. The actual MLS Commit + Welcome will be
//     produced asynchronously by whichever node is currently the holder; the
//     Welcome MUST be queued and delivered by that holder via the
//     OnAddCommitted callback below.
type AddMemberResult struct {
	OperationID string
	Deferred    bool
	Welcome     []byte
	CommitEpoch uint64
	Delivery    AddCommitDelivery
}

// ProposalAuditSummary is a service-facing semantic summary of one observed proposal.
type ProposalAuditSummary struct {
	GroupID        string
	Epoch          uint64
	ActorPeerID    string
	ProposalType   ProposalType
	OperationID    string
	TargetPeerID   string
	RequestID      string
	GroupType      string
	CategoryID     string
	KeyPackageHash []byte
}

// CommitAuditProposalSummary is one human-auditable proposal included in a commit.
type CommitAuditProposalSummary struct {
	ProposalType ProposalType `json:"proposal_type"`
	OperationID  string       `json:"operation_id,omitempty"`
	TargetPeerID string       `json:"target_peer_id,omitempty"`
	RequestID    string       `json:"request_id,omitempty"`
	GroupType    string       `json:"group_type,omitempty"`
	CategoryID   string       `json:"category_id,omitempty"`
}

// CommitAuditSummary captures one commit issued by the local Token Holder.
type CommitAuditSummary struct {
	GroupID           string
	TokenHolderPeerID string
	PrevEpoch         uint64
	NewEpoch          uint64
	Proposals         []CommitAuditProposalSummary
}

// PendingOperationAuditSummary captures lifecycle transitions for durable
// local operations as they are reconciled across epoch changes.
type PendingOperationAuditSummary struct {
	GroupID           string
	OperationID       string
	OpType            string
	TargetPeerID      string
	Stage             string
	RetryCount        int
	CurrentEpoch      uint64
	PreconditionEpoch uint64
	LastError         string
}

// ForkHealAuditSummary captures a high-level fork-heal lifecycle event.
type ForkHealAuditSummary struct {
	GroupID              string
	TraceID              string
	Stage                string
	WinnerPeerID         string
	WinnerEpoch          uint64
	NewEpoch             uint64
	FailedStep           string
	ReplayedMessageCount int
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
	Epoch       uint64 `json:"epoch"`        // current epoch of the branch
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

// BatchedApplicationMsg carries an MLS-encrypted batched chat message.
// The Ciphertext, when decrypted, will unmarshal into a BatchedPlaintext.
type BatchedApplicationMsg struct {
	Ciphertext []byte `json:"ciphertext"`
}

// BatchedPlaintext represents a batch of application messages that are serialized,
// then encrypted together as a single payload to optimize cryptographic and network overhead.
type BatchedPlaintext struct {
	Events []ApplicationEventPayload `json:"events"`
}

// ApplicationEventPayload is the unencrypted inner content of a batched event.
type ApplicationEventPayload struct {
	EventID   string `json:"event_id"`
	Plaintext []byte `json:"plaintext"`
	HLC       []byte `json:"hlc"` // serialized HLCTimestamp
}

// DeliveryAckMsg acknowledges successful application-layer receipt of one
// application envelope identified by its raw wire hash.
type DeliveryAckMsg struct {
	EnvelopeHash []byte `json:"envelope_hash"`
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
	GroupType  string // channel | dm
	CategoryID string // channel category identifier (empty for dm/group)
	// DMCounterpartyPeerID stores the intended other participant for a DM.
	// It is product metadata, not cryptographic membership truth.
	DMCounterpartyPeerID string
	CreatedAt            time.Time
	UpdatedAt            time.Time
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
	MessageID string
	GroupID   string
	Epoch     uint64
	SenderID  peer.ID
	Content   []byte
	Timestamp HLCTimestamp
	// LocalEchoToken is a process-local correlation token injected by the
	// frontend send path so the local UI can replace its optimistic row with the
	// canonical stored message event without guessing by content/timestamp.
	// It is never persisted to SQLite or sent over the network.
	LocalEchoToken string
	// EnvelopeHash keys exactly-once application for replayed envelopes.
	EnvelopeHash []byte
	CommentCount int
	// ReplayedAt is set (unix ms) when this message has been re-broadcast via
	// Autonomous Replay after fork healing. The frontend uses this to suppress
	// the original from display once the replay copy arrives, preventing duplicates.
	ReplayedAt *int64
	// SupersedesMessageID is the canonical message ID of the original message
	// replaced by this replayed copy. Empty for normal (non-replay) messages.
	SupersedesMessageID string
}

// EnvelopeRecord is one row in the offline envelope_log (wire bytes + ordering).
type EnvelopeRecord struct {
	Seq                int64
	GroupID            string
	MsgType            MessageType
	Epoch              uint64
	Envelope           []byte
	Timestamp          HLCTimestamp
	EnvelopeHash       []byte
	SourcePath         string
	ApplyState         string
	LastApplyError     string
	LastApplyAttemptAt int64
	AppliedAt          int64
	FirstSeenAtMs      int64
	ReceivedAtMs       int64
}

// ReplayEnvelopeState is the durable processing state for a raw envelope.
type ReplayEnvelopeState string

const (
	ReplayStateReceived                     ReplayEnvelopeState = "RECEIVED"
	ReplayStateReady                        ReplayEnvelopeState = "READY"
	ReplayStateApplied                      ReplayEnvelopeState = "APPLIED"
	ReplayStateDuplicateApplied             ReplayEnvelopeState = "DUPLICATE"
	ReplayStateFutureEpoch                  ReplayEnvelopeState = "FUTURE_EPOCH"
	ReplayStateStaleEpoch                   ReplayEnvelopeState = "STALE_EPOCH"
	ReplayStateDecryptFailed                ReplayEnvelopeState = "DECRYPT_FAILED"
	ReplayStatePersistFailed                ReplayEnvelopeState = "PERSIST_FAILED"
	ReplayStateInvalid                      ReplayEnvelopeState = "INVALID"
	ReplayStateBlockedMissingPriorEpoch     ReplayEnvelopeState = "BLOCKED_MISSING_PRIOR_EPOCH"
	ReplayStateBlockedMissingCommit         ReplayEnvelopeState = "BLOCKED_MISSING_COMMIT"
	ReplayStateBlockedStaleRequiresSnapshot ReplayEnvelopeState = "BLOCKED_STALE_REQUIRES_SNAPSHOT"
	ReplayStateBlockedDecryptFailed         ReplayEnvelopeState = "BLOCKED_DECRYPT_FAILED"
	ReplayStateForkConflict                 ReplayEnvelopeState = "FORK_CONFLICT"
	ReplayStateWaitingSync                  ReplayEnvelopeState = "WAITING_SYNC"
	ReplayStateTerminalDroppedWithReason    ReplayEnvelopeState = "TERMINAL_DROPPED_WITH_REASON"
)

// IsPendingApplyState returns true if the envelope state represents a pending or
// non-terminal block that requires coordination or replay.
func IsPendingApplyState(state string) bool {
	switch state {
	case "pending",
		string(ReplayStateReceived),
		string(ReplayStateReady),
		string(ReplayStateFutureEpoch),
		string(ReplayStatePersistFailed),
		string(ReplayStateBlockedMissingPriorEpoch),
		string(ReplayStateBlockedMissingCommit),
		string(ReplayStateForkConflict),
		string(ReplayStateWaitingSync),
		"future_epoch",
		"persist_failed":
		return true
	default:
		return false
	}
}

// ReplayEnvelopeResult describes one replay attempt.
type ReplayEnvelopeResult struct {
	GroupID        string
	MsgType        MessageType
	EnvelopeHash   []byte
	State          ReplayEnvelopeState
	Seq            int64
	MsgEpoch       uint64
	LocalEpoch     uint64
	Error          string
	Terminal       bool
	CursorSafe     bool
	Applied        bool
	AlreadyApplied bool
}

// PendingDeliveryAckRow is a queued delivery ACK to send to target_peer_id.
type PendingDeliveryAckRow struct {
	ID           int64
	TargetPeerID string
	GroupID      string
	AckedSeq     int64
}

// ForkHealEventRecord is one persisted summary row for a fork-heal attempt.
// Used by diagnostics/evaluation APIs (Sprint 2F).
type ForkHealEventRecord struct {
	TraceID              string
	GroupID              string
	WinnerPeerID         string
	WinnerEpoch          uint64
	NewEpoch             uint64
	Outcome              string
	FailedStep           string
	WinnerTreeHash       []byte
	NewTreeHash          []byte
	PartitionStartedAtMs int64
	ScheduledAtMs        int64
	StartedAtMs          int64
	CompletedAtMs        int64
	DurationMs           int64
	TotalMs              int64
	ReplayedMessageCount int
}

// ForkHealAuditRecord is one per-step persisted audit row for a heal trace.
type ForkHealAuditRecord struct {
	TraceID     string
	GroupID     string
	Step        string
	Status      string
	TimestampMs int64
	DurationMs  int64
	Error       string
}

// GroupOperationalMode is the operation mode for coordination message sync.
type GroupOperationalMode string

const (
	ModeLive           GroupOperationalMode = "LIVE"
	ModeCatchingUp     GroupOperationalMode = "CATCHING_UP"
	ModeFrozen         GroupOperationalMode = "FROZEN"
	ModeFrozenForApply GroupOperationalMode = "FROZEN_FOR_APPLY"
)

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
	ActionProcess      EpochAction = iota // epoch matches — process normally
	ActionRejectStale                     // sender is behind — send EpochNotification
	ActionBufferFuture                    // sender is ahead — buffer and request sync
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
	ErrAccessRevoked  = errors.New("coordination: local membership revoked")
)

// PendingOperation represents a durable business-level user intention.
type PendingOperation struct {
	OperationID           string
	GroupID               string
	OpType                string // 'ADD_MEMBER', 'REMOVE_MEMBER', 'ROTATE_KEY', 'METADATA_CHANGE'
	IdempotencyKey        *string
	OperationHash         []byte
	PreconditionEpoch     *uint64
	PreconditionStateHash []byte
	TargetMemberID        *string
	SemanticPayload       []byte
	LatestProposalHash    []byte
	Status                string // 'PENDING', 'PROPOSED', 'COMMITTED', 'SATISFIED_BY_OTHER', 'SUPERSEDED', etc.
	RetryCount            int
	ExpiresAt             *int64
	LastError             *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Milestone 5: Crash-safe fork healing state machine tracking structures.
type ForkHealingJob struct {
	JobID                string
	GroupID              string
	TraceID              string
	Status               string // 'INITIATED', 'SNAPSHOT_CREATED', 'EXTERNAL_JOINED', 'STATE_SWAPPED', 'LOCAL_COMPLETE', 'CLEANED'
	LosingBranchID       string
	WinningBranchID      string
	ForkBaseEpoch        uint64
	LosingEpoch          uint64
	WinningEpoch         uint64
	LosingTreeHash       []byte
	WinningTreeHash      []byte
	WinningCommitHash    []byte
	WinningGroupInfoHash []byte
	WinnerPeerID         string
	WinnerGroupInfo      []byte
	BranchWeightJSON     string
	PendingGroupState    []byte
	PendingEpoch         uint64
	PendingTreeHash      []byte
	ErrorMessage         string
	RetryCount           int
	CreatedAtMs          int64
	UpdatedAtMs          int64
	CompletedAtMs        int64
}

type ApplicationEvent struct {
	EventID              string
	JobID                string
	GroupID              string
	OriginalBranchID     string
	OriginalEpoch        uint64
	AuthorID             string
	EnvelopeHash         []byte
	PayloadSealed        []byte // local sealed ciphertext (AES-GCM)
	PayloadHash          []byte // SHA-256 of raw plaintext
	SealKeyID            string
	SealNonce            []byte
	HlcWallTimeMs        int64
	HlcCounter           uint32
	HlcNodeID            string
	Status               string // 'ORPHANED_OWN', 'ORPHANED_OTHER', 'REPLAY_PENDING', 'REPLAYED', 'REPLAY_FAILED', 'WAITING_AUTHOR_REPLAY', 'UNRECOVERABLE'
	ReplayOperationID    string
	ReplayedEnvelopeHash []byte
	ReplayedAtMs         int64
	ReplayAttemptCount   int
	LastError            string
	CreatedAtMs          int64
	UpdatedAtMs          int64
}

type OutboundReplay struct {
	ReplayOperationID    string
	EventID              string
	JobID                string
	GroupID              string
	ReplayEnvelope       []byte
	ReplayedEnvelopeHash []byte
	Status               string // 'ENQUEUED' | 'BROADCASTED' | 'FAILED'
	AttemptCount         int
	CreatedAtMs          int64
	UpdatedAtMs          int64
}
