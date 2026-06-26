package coordination

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// CoordinatorOpts configures a Coordinator instance.
type CoordinatorOpts struct {
	Config     *CoordinatorConfig
	Transport  Transport
	Clock      Clock
	MLS        MLSEngine
	Storage    CoordinationStorage
	LocalID    peer.ID
	GroupID    string
	SigningKey []byte
	// GroupInfoFetcher fetches GroupInfo from a winning peer during fork heal.
	// Required for Sprint 2D external-join orchestration; may be nil in tests
	// that do not exercise healing.
	GroupInfoFetcher     GroupInfoFetchFunc
	AuthorizedCommitters AuthorizedCommittersProvider
	// InitialActiveView seeds known-online, authenticated group members at
	// startup/join so token-holder election does not begin from a misleading
	// local-only view while heartbeat discovery catches up.
	InitialActiveView []peer.ID

	OnMessage     func(*StoredMessage) // called when an application message is decrypted
	OnEpochChange func(uint64)         // called when the group advances to a new epoch
	// OnAccessLost is called once when local membership is revoked by a valid
	// MLS commit (e.g. removed from group).
	OnAccessLost func(groupID string, epoch uint64, reason string)
	// OnEnvelopeBroadcast is called when this local node publishes a commit or
	// application envelope to the group topic. Intended for offline replication
	// layers (e.g. blind-store) and not invoked for replayed remote envelopes.
	OnEnvelopeBroadcast func(MessageType, string, []byte)
	// OnAddCommitted fires once per AddCommitDelivery whenever an Add commit
	// has been applied to local MLS state.
	//
	//   - When the local node ran CreateCommit (Token Holder path), welcome
	//     carries the freshly produced MLS Welcome bytes; the runtime MUST
	//     persist pending_welcomes_out, replicate to store peers, and deliver
	//     the Welcome to the invitee. Only the node that ran CreateCommit
	//     owns the ephemeral material required to author the Welcome — no
	//     other node may attempt to reconstruct it.
	//   - When the local node observed the commit on the wire (any non-holder
	//     receiver), welcome is nil; the runtime only transitions its local
	//     group_add_operations row to "commit_observed".
	OnAddCommitted func(delivery AddCommitDelivery, commitEpoch uint64, welcome []byte)
	// OnPeerObserved fires the first time a remote peer enters this group's
	// ActiveView via a heartbeat (transition absent -> present). It does NOT
	// fire on subsequent heartbeats from the same peer.
	//
	// The runtime uses this to self-heal the group_members roster after a
	// join: when MLS leaf enumeration cannot resolve a leaf via the
	// peer_directory (e.g. the peer has not yet handshaked), the first
	// heartbeat from that peer still gives us a verified, online peer_id we
	// can upsert into the local roster. Fires from a fresh goroutine so the
	// service layer may perform DB I/O without blocking the coordinator.
	OnPeerObserved func(groupID string, peerID peer.ID, observedAt time.Time)
	// OnProposalObserved fires whenever a proposal is accepted into the local
	// single-writer buffer.
	OnProposalObserved func(ProposalAuditSummary)
	// OnCommitIssued fires whenever the local node issues a commit as Token Holder.
	OnCommitIssued func(CommitAuditSummary)
	// OnPendingOperationAudit fires when a durable pending operation is
	// rebased, exhausts retries, or fails re-proposal after an epoch change.
	OnPendingOperationAudit func(PendingOperationAuditSummary)
	// OnForkHealEvent fires for high-level fork-heal lifecycle stages.
	OnForkHealEvent func(ForkHealAuditSummary)
	// OnSyncRequired fires when the coordinator detects that it is lagging behind
	// a remote branch and requires a catch-up sync before attempting healing.
	OnSyncRequired func(remote peer.ID, groupID string)
	// HLC provides an optional shared Hybrid Logical Clock singleton.
	// If nil, NewCoordinator instantiates a dedicated clock.
	HLC *HLC
}

// Coordinator orchestrates the Decentralized Coordination Protocol for a
// single MLS group. It ties together ActiveView, SingleWriter, EpochTracker,
// ForkDetector, and HLC into a cohesive message processing pipeline.
//
// Lifecycle: NewCoordinator → CreateGroup / InitializeGroup → Start → Stop.
//
// Thread-safe: all exported methods may be called concurrently.
type Coordinator struct {
	// Immutable after construction
	cfg        *CoordinatorConfig
	transport  Transport
	clock      Clock
	mls        MLSEngine
	storage    CoordinationStorage
	localID    peer.ID
	groupID    string
	signingKey []byte

	hlc     *HLC
	metrics *Metrics

	onMessage            func(*StoredMessage)
	onEpochChange        func(uint64)
	onAccessLost         func(string, uint64, string)
	onEnvelope           func(MessageType, string, []byte)
	onAddCommitted       func(AddCommitDelivery, uint64, []byte)
	onPeerObserved       func(string, peer.ID, time.Time)
	onProposalObserved   func(ProposalAuditSummary)
	onCommitIssued       func(CommitAuditSummary)
	onPendingOperation   func(PendingOperationAuditSummary)
	onForkHealEvent      func(ForkHealAuditSummary)
	onSyncRequired       func(peer.ID, string)
	groupInfoFetch       GroupInfoFetchFunc
	authorizedCommitters AuthorizedCommittersProvider
	localIdentity        []byte

	// Mutable state (protected by mu)
	mu                sync.Mutex
	groupState        []byte
	treeHash          []byte
	lastCommitHash    []byte
	epoch             uint64
	activeView        *ActiveView
	singleWriter      *SingleWriter
	epochTracker      *EpochTracker
	forkDetector      *ForkDetector
	proposalTimerChan <-chan time.Time
	failoverTimerChan <-chan time.Time
	lastTokenHolder   peer.ID
	startedAt         time.Time
	started           bool
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	welcomeReceivedChan chan struct{}

	// healing is set to 1 while a fork-heal goroutine is in flight. Manipulated
	// via atomic CAS so handleAnnounceLocked never blocks on a long heal and
	// duplicate fork events do not trigger overlapping heal pipelines.
	healing atomic.Bool
	// accessRevoked flips when local membership no longer exists in MLS state.
	// All local mutation APIs are blocked once this is true.
	accessRevoked bool

	// pendingAppDeliveries tracks locally-sent MsgApplication envelopes awaiting a
	// direct receipt ACK from each intended recipient. Key format:
	// "<peer-id>|<envelope-hash-hex>".
	pendingAppDeliveries map[string]*pendingAppDelivery

	// syncRetryAttempts tracks the number of consecutive failed sync attempts.
	syncRetryAttempts int
	// deferredForkEvent is a fork event saved during lagging/catch-up sync retry phase.
	deferredForkEvent *ForkEvent
	// operationalMode tracks if the coordinator is in LIVE or CATCHING_UP mode.
	operationalMode GroupOperationalMode
}

type pendingAppDelivery struct {
	peerID       peer.ID
	envelopeHash string
	envelope     []byte
	attempts     int
}

// GroupInfoFetchResult is the winner-branch snapshot fetched over
// /app/group-info/1.0.0 for external-join healing.
type GroupInfoFetchResult struct {
	GroupInfo []byte
	Epoch     uint64
	TreeHash  []byte
}

// GroupInfoFetchFunc fetches GroupInfo from a specific remote peer.
type GroupInfoFetchFunc func(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*GroupInfoFetchResult, error)

// NewCoordinator creates a Coordinator. Call CreateGroup or InitializeGroup,
// then Start to begin processing.
func NewCoordinator(opts CoordinatorOpts) (*Coordinator, error) {
	if err := opts.Config.Validate(); err != nil {
		return nil, err
	}

	c := &Coordinator{
		cfg:                  opts.Config,
		transport:            opts.Transport,
		clock:                opts.Clock,
		mls:                  opts.MLS,
		storage:              opts.Storage,
		localID:              opts.LocalID,
		groupID:              opts.GroupID,
		signingKey:           opts.SigningKey,
		metrics:              NewMetrics(),
		onMessage:            opts.OnMessage,
		onEpochChange:        opts.OnEpochChange,
		onAccessLost:         opts.OnAccessLost,
		onEnvelope:           opts.OnEnvelopeBroadcast,
		onAddCommitted:       opts.OnAddCommitted,
		onPeerObserved:       opts.OnPeerObserved,
		onProposalObserved:   opts.OnProposalObserved,
		onCommitIssued:       opts.OnCommitIssued,
		onPendingOperation:   opts.OnPendingOperationAudit,
		onForkHealEvent:      opts.OnForkHealEvent,
		onSyncRequired:       opts.OnSyncRequired,
		groupInfoFetch:       opts.GroupInfoFetcher,
		authorizedCommitters: opts.AuthorizedCommitters,
		localIdentity:        deriveIdentityFromSigningKey(opts.SigningKey),
		pendingAppDeliveries: make(map[string]*pendingAppDelivery),
		operationalMode:      ModeLive,
		welcomeReceivedChan:  make(chan struct{}, 1),
	}

	if opts.HLC != nil {
		c.hlc = opts.HLC
	} else {
		c.hlc = NewHLC(opts.Clock, opts.LocalID.String())
	}

	c.activeView = NewActiveView(opts.Clock, opts.Config, opts.LocalID, func(members []peer.ID) {
		go c.handleActiveViewChange(members)
	})
	c.activeView.Seed(opts.InitialActiveView)
	c.forkDetector = NewForkDetector()
	return c, nil
}

// handleActiveViewChange is triggered asynchronously whenever the ActiveView
// member list changes (e.g., due to peer eviction or observation).
func (c *Coordinator) handleActiveViewChange(_ []peer.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return
	}

	if c.singleWriter == nil {
		return
	}
	newHolder, err := c.singleWriter.CurrentTokenHolder()
	if err != nil {
		return
	}

	wasTokenHolder := c.lastTokenHolder == c.localID
	isTokenHolder := newHolder == c.localID
	c.lastTokenHolder = newHolder
	if err := c.persistCoordStateLocked(); err != nil {
		slog.Warn("Failed to persist coordination state after active-view change", "group", c.groupID, "error", err)
	}

	// Only flash-commit if the local node has just been PROMOTED (was NOT Token Holder, now IS)
	// and there are pending proposals in the buffer.
	if isTokenHolder && !wasTokenHolder && c.singleWriter != nil && c.singleWriter.ProposalCount() > 0 {
		slog.Info("ActiveView changed: local node promoted to Token Holder with outstanding proposals. Flashing commit.",
			"group", c.groupID,
			"epoch", c.epoch,
			"buffered_proposals", c.singleWriter.ProposalCount(),
		)
		c.tryCommitLocked()
	}
}

// ─── Lifecycle ───────────────────────────────────────────────────────────────

// CreateGroup creates a new MLS group via the crypto engine.
// Must be called before Start.
func (c *Coordinator) CreateGroup() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("coordinator already started")
	}

	opCtx, cancel := c.mlsOperationContext()
	state, treeHash, err := c.mls.CreateGroup(opCtx, c.groupID, c.signingKey, c.cfg.GetMaxPastEpochs())
	cancel()
	if err != nil {
		return fmt.Errorf("CreateGroup: %w", err)
	}

	c.groupState = state
	c.epoch = 0
	c.treeHash = treeHash

	if err := c.storage.SaveGroupRecord(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: state,
		Epoch:      0,
		TreeHash:   treeHash,
		MyRole:     RoleCreator,
		CreatedAt:  c.clock.Now(),
		UpdatedAt:  c.clock.Now(),
	}); err != nil {
		return err
	}
	return c.persistCoordStateLocked()
}

// InitializeGroup sets up the coordinator with a known group state.
// Used in tests and when joining a group via out-of-band state transfer.
func (c *Coordinator) InitializeGroup(groupState []byte, epoch uint64, treeHash []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.groupState = make([]byte, len(groupState))
	copy(c.groupState, groupState)
	c.epoch = epoch
	c.treeHash = make([]byte, len(treeHash))
	copy(c.treeHash, treeHash)
}

// Start subscribes to the group topic and begins processing messages.
func (c *Coordinator) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("coordinator already started")
	}

	if c.groupState == nil {
		rec, err := c.storage.GetGroupRecord(c.groupID)
		if err != nil {
			return fmt.Errorf("no group state — call CreateGroup or InitializeGroup first: %w", err)
		}
		c.groupState = rec.GroupState
		c.epoch = rec.Epoch
		c.treeHash = rec.TreeHash
	}
	if cs, err := c.storage.GetCoordState(c.groupID); err == nil {
		c.lastCommitHash = copyBytes(cs.LastCommitHash)
	} else if !errors.Is(err, ErrGroupNotFound) {
		return fmt.Errorf("load coordination state: %w", err)
	}

	c.epochTracker = NewEpochTracker(c.epoch, c.treeHash)
	c.singleWriter = NewSingleWriter(c.activeView, c.localID, c.epoch, c.cfg)
	c.singleWriter.SetAuthorizedCommitters(c.groupID, c.authorizedCommitters)
	if holder, err := c.singleWriter.CurrentTokenHolder(); err == nil {
		c.lastTokenHolder = holder
	}
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    c.treeHash,
		MemberCount: c.activeView.Size(),
		Epoch:       c.epoch,
		CommitHash:  c.lastCommitHash,
	})

	c.ctx, c.cancel = context.WithCancel(ctx)

	if err := c.transport.Subscribe(GroupTopic(c.groupID), c.handleRawMessage); err != nil {
		c.cancel()
		return fmt.Errorf("subscribe: %w", err)
	}

	c.started = true
	c.startedAt = c.clock.Now()

	c.wg.Add(1)
	go func() { defer c.wg.Done(); c.heartbeatLoop(c.ctx) }()

	if c.cfg.AnnounceInterval > 0 {
		c.wg.Add(1)
		go func() { defer c.wg.Done(); c.announceLoop(c.ctx) }()
	}

	// Resume any incomplete fork healing job from database
	if job, err := c.storage.GetActiveForkHealingJob(c.groupID); err == nil && job != nil {
		if job.Status != "CLEANED" && job.Status != "FAILED_TERMINAL" {
			slog.Info("coordination/startup_resume_job", "group", c.groupID, "trace_id", job.TraceID, "status", job.Status)
			go c.resumeForkHealingJob(job)
		}
	}

	return nil
}

func (c *Coordinator) mlsOperationContext() (context.Context, context.CancelFunc) {
	base := c.ctx
	if base == nil {
		base = context.Background()
	}
	return context.WithTimeout(base, c.cfg.MLSOperationTimeout)
}

// Stop cancels all background work and unsubscribes from the group topic.
func (c *Coordinator) Stop() {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return
	}
	c.cancel()
	c.failoverTimerChan = nil
	_ = c.transport.Unsubscribe(GroupTopic(c.groupID))
	c.started = false
	c.mu.Unlock()

	c.wg.Wait()
}

// ─── Message Handling ────────────────────────────────────────────────────────

// ReceiveDirectMessage applies a coordination envelope received on a libp2p
// direct stream. Wire format matches GossipSub (JSON Envelope); each
// coordinator ignores payloads for other groups.
func (c *Coordinator) ReceiveDirectMessage(from peer.ID, data []byte) {
	c.handleRawMessage(from, data)
}

func (c *Coordinator) handleRawMessage(from peer.ID, data []byte) {
	if from == c.localID {
		return
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	if env.GroupID != c.groupID {
		return
	}

	// P0.2 Clock Skew protection on receive:
	if env.Type == MsgCommit || env.Type == MsgApplication || env.Type == MsgProposal {
		nowMs := c.clock.Now().UnixMilli()
		if err := validateSenderTimestamp(nowMs, env.Timestamp.WallTimeMs); err != nil {
			slog.Warn("Rejected raw message due to invalid sender timestamp", "group", c.groupID, "from", from, "type", env.Type, "err", err)
			return // drop envelope entirely
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Inbox Isolation: Nếu đang trong quá trình Catching Up, cô lập hoàn toàn luồng Gossip live.
	// Chỉ append MsgCommit và MsgApplication vào SQLite (Envelope Log) ở trạng thái pending,
	// không mutate groupState (không handle commit/application ngay).
	if c.operationalMode == ModeCatchingUp {
		if env.Type == MsgCommit || env.Type == MsgApplication {
			if c.cfg != nil && c.cfg.OfflineSyncEnabled {
				_, err := c.storage.AppendEnvelopeWithSource(c.groupID, env.Type, env.Epoch, env.Timestamp, data, "gossip_catchup")
				if err != nil {
					slog.Warn("Gossip catchup: append envelope failed", "group", c.groupID, "err", err)
				} else {
					slog.Debug("Gossip catchup: buffered live envelope to DB inbox", "group", c.groupID, "type", env.Type, "epoch", env.Epoch)
				}
			}
		} else if env.Type == MsgHeartbeat {
			c.handleHeartbeatLocked(from)
		} else if env.Type == MsgAnnounce {
			c.handleAnnounceLocked(from, &env)
		} else if env.Type == MsgDeliveryAck {
			c.handleDeliveryAckLocked(from, &env)
		}
		return
	}

	if c.operationalMode == ModeFrozenForApply {
		if env.Type == MsgCommit || env.Type == MsgApplication {
			_, err := c.storage.AppendEnvelope(c.groupID, env.Type, env.Epoch, env.Timestamp, data)
			if err != nil {
				slog.Warn("Gossip frozen: append envelope failed", "group", c.groupID, "err", err)
			} else {
				slog.Debug("Gossip frozen: buffered live envelope to DB inbox during healing", "group", c.groupID, "type", env.Type, "epoch", env.Epoch)
			}
		} else if env.Type == MsgHeartbeat {
			c.handleHeartbeatLocked(from)
		} else if env.Type == MsgAnnounce {
			c.handleAnnounceLocked(from, &env)
		} else if env.Type == MsgDeliveryAck {
			c.handleDeliveryAckLocked(from, &env)
		}
		return
	}

	switch env.Type {
	case MsgHeartbeat:
		c.handleHeartbeatLocked(from)
	case MsgAnnounce:
		c.handleAnnounceLocked(from, &env)
	case MsgProposal:
		c.handleProposalLocked(from, &env)
	case MsgCommit:
		c.handleCommitLocked(&env, data)
	case MsgApplication:
		c.handleApplicationLocked(from, &env, data)
	case MsgApplicationBatched:
		c.handleApplicationBatchedLocked(from, &env, data)
	case MsgDeliveryAck:
		c.handleDeliveryAckLocked(from, &env)
	}
}

func (c *Coordinator) handleHeartbeatLocked(from peer.ID) {
	c.observePeerAliveLocked(from)
}

func (c *Coordinator) observePeerAliveLocked(from peer.ID) {
	if from == "" || from == c.localID {
		return
	}
	fresh := c.activeView.RecordHeartbeat(from)
	if !fresh || c.onPeerObserved == nil {
		return
	}
	// Fire OnPeerObserved outside c.mu via goroutine so the service handler can
	// perform DB writes without blocking heartbeat processing. We capture the
	// observation timestamp now (under lock) so the handler sees the true
	// first-seen moment even if scheduling is delayed.
	cb := c.onPeerObserved
	groupID := c.groupID
	observedAt := c.clock.Now()
	go cb(groupID, from, observedAt)
}

// ObservePeerAlive lets the runtime feed already-authenticated transport
// observations into this group's ActiveView without waiting for the next
// group heartbeat. The runtime must only call this for peers that are both
// verified by the auth protocol and active members of this group.
func (c *Coordinator) ObservePeerAlive(from peer.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.observePeerAliveLocked(from)
}

func (c *Coordinator) handleAnnounceLocked(from peer.ID, env *Envelope) {
	var ann GroupStateAnnouncement
	if err := json.Unmarshal(env.Payload, &ann); err != nil {
		return
	}

	event := c.forkDetector.ProcessRemote(c.clock.Now(), from, env.Epoch, ann)
	if event == nil || !event.NeedExternalJoin {
		return
	}

	c.metrics.IncrPartitionsDetected()
	event.GroupID = c.groupID
	c.scheduleHeal(event)
}

func (c *Coordinator) handleProposalLocked(from peer.ID, env *Envelope) {
	var proposal ProposalMsg
	if err := json.Unmarshal(env.Payload, &proposal); err != nil {
		return
	}

	// Defensive Guard: If groupState is nil, we cannot process standard proposals.
	// We only allow ProposalJoin to proceed because it bypasses MLS validation.
	if c.groupState == nil && proposal.ProposalType != ProposalJoin {
		slog.Warn("Ignored proposal because local groupState is nil", "type", proposal.ProposalType, "group", c.groupID)
		return
	}

	// PHOENIX PROTOCOL INTERCEPTION:
	// A ProposalJoin requires an atomic Remove(zombie) + Add(fresh) operation to bypass OpenMLS duplicate identity checks.
	// We check this BEFORE epoch validation, but enforce a security epoch guard to prevent ancient replay attacks.
	if proposal.ProposalType == ProposalJoin {
		maxPast := uint64(c.cfg.GetMaxPastEpochs())
		if env.Epoch+maxPast < c.epoch {
			slog.Warn("Rejected ProposalJoin from ancient epoch", "group", c.groupID, "sender", from, "env_epoch", env.Epoch, "current_epoch", c.epoch)
			return
		}

		// If this node is also healing (groupState nil), it cannot transmute ProposalJoin.
		if len(c.groupState) == 0 {
			slog.Info("ProposalJoin: local node also healing, cannot transmute", "node", c.localID, "group", c.groupID)
			return
		}

		// Use the explicit TargetIdentity if provided by the joiner, falling back to TargetPeerID bytes
		targetIdentity := proposal.TargetIdentity
		if len(targetIdentity) == 0 {
			targetIdentity = []byte(proposal.TargetPeerID)
		}

		// 1. Check if member exists in tree before attempting Remove
		//    If the credential was already removed (e.g. by advanceEpochOnWinningBranch),
		//    skip Remove and proceed directly with Add.
		opCtxMember, cancelMember := c.mlsOperationContext()
		hasMember, memberErr := c.mls.HasMember(opCtxMember, c.groupState, targetIdentity)
		cancelMember()
		if memberErr != nil {
			slog.Warn("HasMember check failed during JoinProposal", "err", memberErr, "target", proposal.TargetPeerID)
		}

		if memberErr == nil && hasMember {
			_, bufferedRemove, err := c.createAndStoreLocalProposalLocked(ProposalRemove, targetIdentity, BufferedProposal{TargetPeerID: proposal.TargetPeerID})
			if err == nil {
				c.singleWriter.BufferProposal(bufferedRemove)
			} else {
				slog.Warn("Failed to create RemoveProposal for zombie leaf during JoinProposal", "err", err, "target", proposal.TargetPeerID)
			}
		} else if memberErr == nil {
			slog.Info("Skipped RemoveProposal during JoinProposal — member not found in tree (already removed)", "target", proposal.TargetPeerID)
		}

		// 2. Transmute the JoinProposal into a standard AddProposal and buffer it
		_, bufferedAdd, err := c.createAndStoreLocalProposalLocked(ProposalAdd, proposal.Data, BufferedProposal{
			OperationID:    proposal.OperationID,
			TargetPeerID:   proposal.TargetPeerID,
			RequestID:      proposal.RequestID,
			GroupType:      proposal.GroupType,
			CategoryID:     proposal.CategoryID,
			KeyPackageHash: proposal.KeyPackageHash,
		})
		if err == nil {
			c.singleWriter.BufferProposal(bufferedAdd)
		} else {
			slog.Warn("Failed to create AddProposal for fresh key package during JoinProposal", "err", err, "target", proposal.TargetPeerID)
		}

		// ProposalJoin đi qua cùng Token Holder election path như proposal bình thường.
		// Cơ chế failover (Suspend + re-elect) đảm bảo Token Holder không commit
		// thì node khác takeover, đảm bảo Single-Writer Invariant.
		if len(c.groupState) > 0 {
			if c.singleWriter.IsTokenHolder() {
				c.scheduleBatchCommitLocked()
			} else {
				c.scheduleFailoverTimerLocked()
			}
		} else {
			slog.Info("ProposalJoin: local node also healing, cannot commit", "node", c.localID, "group", c.groupID)
		}
		return
	}

	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		return
	case ActionBufferFuture:
		raw, _ := json.Marshal(env)
		c.epochTracker.BufferFuture(env.Epoch, raw)
		return
	}

	opCtx, cancel := c.mlsOperationContext()
	processed, err := c.mls.ProcessProposal(opCtx, c.groupState, proposal.Data)
	cancel()
	if err != nil {
		slog.Warn("Rejected proposal that failed MLS processing", "group", c.groupID, "epoch", env.Epoch, "from", from, "error", err)
		return
	}
	if len(proposal.ProposalRef) == 0 {
		proposal.ProposalRef = processed.ProposalRef
	}
	if !bytes.Equal(proposal.ProposalRef, processed.ProposalRef) {
		slog.Warn("Rejected proposal with mismatched ProposalRef", "group", c.groupID, "epoch", env.Epoch, "from", from)
		return
	}
	if err := c.persistCurrentEpochStateLocked(processed.NewGroupState); err != nil {
		slog.Error("Failed to persist pending proposal state", "group", c.groupID, "error", err)
		return
	}
	c.groupState = processed.NewGroupState

	c.singleWriter.BufferProposal(BufferedProposal{
		Type:           proposal.ProposalType,
		Data:           proposal.Data,
		ProposalRef:    proposal.ProposalRef,
		OperationID:    proposal.OperationID,
		TargetPeerID:   proposal.TargetPeerID,
		RequestID:      proposal.RequestID,
		GroupType:      proposal.GroupType,
		CategoryID:     proposal.CategoryID,
		KeyPackageHash: proposal.KeyPackageHash,
	})
	c.scheduleFailoverTimerLocked()
	c.metrics.IncrProposalsReceived()
	if c.onProposalObserved != nil {
		c.onProposalObserved(ProposalAuditSummary{
			GroupID:        c.groupID,
			Epoch:          env.Epoch,
			ActorPeerID:    from.String(),
			ProposalType:   proposal.ProposalType,
			OperationID:    proposal.OperationID,
			TargetPeerID:   proposal.TargetPeerID,
			RequestID:      proposal.RequestID,
			GroupType:      proposal.GroupType,
			CategoryID:     proposal.CategoryID,
			KeyPackageHash: append([]byte(nil), proposal.KeyPackageHash...),
		})
	}

	if c.singleWriter.IsTokenHolder() {
		c.scheduleBatchCommitLocked()
	}
}

func (c *Coordinator) handleCommitLocked(env *Envelope, wire []byte) bool {
	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		c.metrics.IncrDuplicateEpochDetected()
		return false
	case ActionBufferFuture:
		c.epochTracker.BufferFuture(env.Epoch, wire)
		return false
	}

	var commit CommitMsg
	if err := json.Unmarshal(env.Payload, &commit); err != nil {
		return false
	}
	commitHash := hashCommitData(commit.CommitData)

	_, alreadyApplied := c.checkAppliedEnvelopeLocked(env, wire)
	if alreadyApplied {
		return false
	}

	start := c.clock.Now()

	opCtx, cancel := c.mlsOperationContext()
	batch := bufferedBatchFromProposalMsgs(commit.IncludedProposals)
	includedProposalBytes := proposalBytesFromMsgs(commit.IncludedProposals)
	staged, err := c.mls.StageCommit(opCtx, c.groupState, commit.CommitData, includedProposalBytes)
	cancel()
	if err != nil {
		c.markInvalidCommitLocked(commitHash)
		return false
	}
	if len(commit.CommittedProposalRefs) > 0 && !proposalRefSetsEqual(staged.ProposalRefs, commit.CommittedProposalRefs) {
		c.markInvalidCommitLocked(commitHash)
		return false
	}
	if len(batch) > 0 {
		sender := decodeEnvelopePeerID(env.From, "")
		if sender == "" {
			slog.Warn("Commit envelope missing sender; skipping token-holder metadata validation for compatibility", "group", c.groupID, "epoch", c.epoch)
		} else {
			holder, err := c.singleWriter.HolderForBatch(batch)
			if err != nil {
				c.markInvalidCommitLocked(commitHash)
				return false
			}
			if sender != holder {
				slog.Warn("Rejected commit from non-token-holder", "group", c.groupID, "epoch", c.epoch, "sender", sender, "holder", holder)
				c.markInvalidCommitLocked(commitHash)
				return false
			}
			if removesPeer(batch, sender) {
				slog.Warn("Rejected commit whose batch removes the committer", "group", c.groupID, "epoch", c.epoch, "sender", sender)
				c.markInvalidCommitLocked(commitHash)
				return false
			}
		}
	}

	opCtx, cancel = c.mlsOperationContext()
	newState, newTreeHash, err := c.mls.ProcessCommit(opCtx, c.groupState, commit.CommitData, includedProposalBytes)
	cancel()
	if err != nil {
		c.markInvalidCommitLocked(commitHash)
		return false
	}
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      nextEpoch,
		TreeHash:   newTreeHash,
		UpdatedAt:  now,
	}, env.Type, wire, env.Timestamp, env.Epoch)
	if err != nil {
		slog.Error("Failed to persist commit apply", "group", c.groupID, "error", err)
		return false
	}
	if !applied {
		return false
	}

	c.advanceEpochLocked(newState, nextEpoch, newTreeHash, commit.CommitData)
	// Primary drain: by ProposalRef (works when this node is the holder or has the same groupState).
	c.singleWriter.DrainBatchByRefs(commit.CommittedProposalRefs)
	// Fallback drain: by raw proposal Data bytes.
	if len(commit.IncludedProposals) > 0 {
		proposalDatas := make([][]byte, 0, len(commit.IncludedProposals))
		for _, p := range commit.IncludedProposals {
			if len(p.Data) > 0 {
				proposalDatas = append(proposalDatas, p.Data)
			}
		}
		c.singleWriter.DrainBatchByData(proposalDatas)
	}
	c.reconcileOperationsAfterCommitLocked(commit)

	// Trigger bidirectional batch replay
	c.triggerBatchReplayAsync(c.groupID)

	c.reconcileAndRebaseOperationsLocked()

	c.updateLocalAccessRevocationLocked(newState, nextEpoch)
	c.metrics.RecordEpochFinalization(c.clock.Now().Sub(start))

	if len(commit.AddDeliveries) > 0 && c.onAddCommitted != nil {
		deliveries := append([]AddCommitDelivery(nil), commit.AddDeliveries...)
		epoch := nextEpoch
		cb := c.onAddCommitted
		go func() {
			for _, d := range deliveries {
				cb(d, epoch, nil)
			}
		}()
	}
	return true
}

func (c *Coordinator) handleApplicationLocked(from peer.ID, env *Envelope, wire []byte) bool {
	return c.handleApplicationDetailedLocked(from, env, wire).Applied
}

func (c *Coordinator) handleApplicationDetailedLocked(from peer.ID, env *Envelope, wire []byte) ReplayEnvelopeResult {
	result := c.newReplayResultLocked(env, wire)
	envelopeHash, alreadyApplied := c.checkAppliedEnvelopeLocked(env, wire)
	result.EnvelopeHash = envelopeHash
	if alreadyApplied {
		result.State = ReplayStateDuplicateApplied
		result.AlreadyApplied = true
		result.CursorSafe = true
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	if c.epochTracker == nil {
		c.epochTracker = NewEpochTracker(c.epoch, c.treeHash)
	}
	action := c.epochTracker.Validate(env.Epoch)
	maxPastEpochs := uint64(c.cfg.GetMaxPastEpochs())
	if action == ActionRejectStale && env.Type == MsgApplication && env.Epoch+maxPastEpochs >= c.epoch {
		// Enforce time-based retention policy max_past_age_seconds.
		// SECURITY: physical age validation is measured against local first-seen time, NOT sender-provided HLC.
		firstSeenMs := c.clock.Now().UnixMilli()
		if rec, err := c.storage.GetEnvelope(envelopeHash); err == nil && rec != nil && rec.FirstSeenAtMs > 0 {
			firstSeenMs = rec.FirstSeenAtMs
		}

		maxPastAgeSeconds := c.cfg.GetMaxPastAgeSeconds()
		ageSeconds := (c.clock.Now().UnixMilli() - firstSeenMs) / 1000
		if ageSeconds < 0 {
			ageSeconds = 0
		}
		if maxPastAgeSeconds > 0 && ageSeconds > maxPastAgeSeconds {
			slog.Warn("Rejected late-arriving stale application message exceeding age boundary",
				"group", c.groupID, "ageSeconds", ageSeconds, "maxPastAgeSeconds", maxPastAgeSeconds, "firstSeenMs", firstSeenMs)
			// keep action as ActionRejectStale
		} else {
			// Allow slightly stale application messages to be processed, as MLS supports
			// decrypting messages from a window of previous epochs using retained keys.
			action = ActionProcess
		}
	}

	switch action {
	case ActionRejectStale:
		epochDiff := int64(c.epoch) - int64(env.Epoch)
		slog.Warn("Rejected stale message", "group", c.groupID,
			"msgEpoch", env.Epoch, "currentEpoch", c.epoch,
			"epochDiff", epochDiff, "maxPastEpochs", c.cfg.GetMaxPastEpochs())
		result.State = ReplayStateStaleEpoch
		result.Terminal = true
		result.CursorSafe = true
		c.markReplayResultLocked(result)
		return result
	case ActionBufferFuture:
		slog.Info("Buffered future-epoch message", "group", c.groupID, "msgEpoch", env.Epoch)
		c.epochTracker.BufferFuture(env.Epoch, wire)
		result.State = ReplayStateFutureEpoch
		c.markReplayResultLocked(result)
		return result
	}

	var appMsg ApplicationMsg
	if err := json.Unmarshal(env.Payload, &appMsg); err != nil {
		result.State = ReplayStateInvalid
		result.Error = err.Error()
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	localTs, err := c.hlc.Update(env.Timestamp)
	if err != nil {
		slog.Error("HLC update failed (clock drift limit exceeded)", "group", c.groupID, "from", env.From, "error", err)
		result.State = ReplayStateInvalid
		result.Error = "hlc: " + err.Error()
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}
	if localTs.NodeID == "" {
		localTs.NodeID = c.localID.String()
	}

	opCtx, cancel := c.mlsOperationContext()
	plaintext, newState, err := c.mls.DecryptMessage(opCtx, c.groupState, appMsg.Ciphertext)
	cancel()
	if err != nil {
		slog.Error("Failed to decrypt message", "group", c.groupID, "from", env.From, "error", err)
		result.State = ReplayStateDecryptFailed
		result.Error = err.Error()
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	sender := decodeEnvelopePeerID(env.From, from)

	msg := &StoredMessage{
		GroupID:      c.groupID,
		Epoch:        env.Epoch,
		SenderID:     sender,
		Content:      plaintext,
		Timestamp:    localTs,
		EnvelopeHash: envelopeHash,
	}
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyApplication(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		UpdatedAt:  now,
	}, msg, env.Type, wire, env.Timestamp, env.Epoch)
	if err != nil {
		slog.Error("Failed to persist decrypted message", "group", c.groupID, "from", env.From, "error", err)
		result.State = ReplayStatePersistFailed
		result.Error = err.Error()
		c.markReplayResultLocked(result)
		return result
	}
	if !applied {
		result.State = ReplayStateDuplicateApplied
		result.AlreadyApplied = true
		result.CursorSafe = true
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}
	c.groupState = newState
	slog.Info("Message received", "group", c.groupID, "epoch", env.Epoch, "from", env.From, "ts", localTs.WallTimeMs)

	if c.onMessage != nil {
		c.onMessage(msg)
	}
	c.sendDeliveryAckLocked(sender, envelopeHash)
	result.State = ReplayStateApplied
	result.Applied = true
	result.CursorSafe = true
	result.Terminal = true
	c.markReplayResultLocked(result)
	return result
}

func (c *Coordinator) handleDeliveryAckLocked(from peer.ID, env *Envelope) {
	var ack DeliveryAckMsg
	if err := json.Unmarshal(env.Payload, &ack); err != nil {
		return
	}
	if len(ack.EnvelopeHash) == 0 {
		return
	}
	delete(c.pendingAppDeliveries, pendingAppDeliveryKey(from, ack.EnvelopeHash))
}

// ─── Commit Logic ────────────────────────────────────────────────────────────

func (c *Coordinator) bootstrapGraceRemainingLocked() time.Duration {
	grace := c.cfg.ViewBootstrapGrace
	if grace <= 0 || c.startedAt.IsZero() {
		return 0
	}
	elapsed := c.clock.Now().Sub(c.startedAt)
	if elapsed >= grace {
		return 0
	}
	return grace - elapsed
}

func (c *Coordinator) commitViewReadyLocked(batch []BufferedProposal) bool {
	if c.bootstrapGraceRemainingLocked() <= 0 {
		return true
	}
	if c.activeView.Size() > 1 {
		return true
	}
	if c.authorizedCommitters == nil {
		return true
	}
	authorized, err := c.authorizedCommitters(c.groupID, c.epoch, batch)
	if err != nil {
		slog.Warn("Bootstrap view guard skipped: failed to load authorized committers",
			"group", c.groupID,
			"epoch", c.epoch,
			"error", err)
		return true
	}
	removed := make(map[peer.ID]struct{}, len(batch))
	for _, p := range batch {
		if p.Type != ProposalRemove || p.TargetPeerID == "" {
			continue
		}
		if target, err := peer.Decode(p.TargetPeerID); err == nil {
			removed[target] = struct{}{}
		}
	}
	for _, id := range authorized {
		if id != "" && id != c.localID {
			if _, willBeRemoved := removed[id]; willBeRemoved {
				continue
			}
			return false
		}
	}
	return true
}

func (c *Coordinator) deferCommitUntilViewReadyLocked() {
	if c.proposalTimerChan != nil {
		return
	}
	delay := c.bootstrapGraceRemainingLocked()
	if delay <= 0 {
		delay = c.cfg.BatchingDelay
	}
	if delay <= 0 {
		delay = 10 * time.Millisecond
	}
	slog.Info("Deferring token-holder commit while group ActiveView bootstraps",
		"group", c.groupID,
		"epoch", c.epoch,
		"delay", delay,
		"active_view_size", c.activeView.Size())
	ch := c.clock.After(delay)
	c.proposalTimerChan = ch
	go func(timerChan <-chan time.Time) {
		<-timerChan

		c.mu.Lock()
		defer c.mu.Unlock()

		if c.proposalTimerChan == timerChan {
			c.proposalTimerChan = nil
			if c.singleWriter != nil && c.singleWriter.IsTokenHolder() && c.singleWriter.ProposalCount() > 0 {
				c.tryCommitLocked()
			}
		}
	}(ch)
}

// scheduleBatchCommitLocked schedules a commit execution after a short batching delay
// to allow multiple concurrent proposals to be gathered into a single commit.
// If a commit is already scheduled, this is a no-op (letting the current window collect proposals).
func (c *Coordinator) scheduleBatchCommitLocked() {
	if c.proposalTimerChan != nil {
		return // already scheduled, let it accumulate proposals
	}

	delay := c.cfg.BatchingDelay
	if delay <= 0 {
		// Fallback to immediate commit if batching is disabled
		c.tryCommitLocked()
		return
	}

	slog.Info("Scheduling batch commit after delay", "group", c.groupID, "delay", delay)
	ch := c.clock.After(delay)
	c.proposalTimerChan = ch

	go func(timerChan <-chan time.Time) {
		<-timerChan // Wait for the delay to expire (fully testable with FakeClock!)

		c.mu.Lock()
		defer c.mu.Unlock()

		// Ensure that the timer was not canceled or overwritten
		if c.proposalTimerChan == timerChan {
			c.proposalTimerChan = nil
			if c.singleWriter != nil && c.singleWriter.IsTokenHolder() && c.singleWriter.ProposalCount() > 0 {
				slog.Info("Batching delay expired: flashing committed proposals",
					"group", c.groupID,
					"epoch", c.epoch,
					"buffered_proposals", c.singleWriter.ProposalCount(),
				)
				c.tryCommitLocked()
			}
		}
	}(ch)
}

func (c *Coordinator) scheduleFailoverTimerLocked() {
	if c.failoverTimerChan != nil {
		return // already scheduled
	}
	if c.singleWriter == nil || c.singleWriter.ProposalCount() == 0 || c.singleWriter.IsTokenHolder() {
		return // no need for failover
	}

	delay := c.cfg.TokenHolderTimeout
	if delay <= 0 {
		delay = 5 * time.Second
	}

	slog.Info("Scheduling token holder failover timer", "group", c.groupID, "epoch", c.epoch, "delay", delay)
	ch := c.clock.After(delay)
	c.failoverTimerChan = ch

	c.wg.Add(1)
	go func(timerChan <-chan time.Time) {
		defer c.wg.Done()

		select {
		case <-timerChan:
		case <-c.ctx.Done():
			c.mu.Lock()
			if c.failoverTimerChan == timerChan {
				c.failoverTimerChan = nil
			}
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		if c.failoverTimerChan == timerChan {
			c.failoverTimerChan = nil
			if c.singleWriter != nil && c.singleWriter.ProposalCount() > 0 && !c.singleWriter.IsTokenHolder() {
				holder, err := c.singleWriter.CurrentTokenHolder()
				if err == nil && holder != "" {
					slog.Warn("Token Holder failed to commit in time, suspending", "group", c.groupID, "epoch", c.epoch, "holder", holder)
					c.singleWriter.Suspend(holder)
					
					// Re-evaluate: if we became the Token Holder, we should try to commit now.
					if c.singleWriter.IsTokenHolder() {
						c.scheduleBatchCommitLocked()
					} else {
						// Someone else is the new Token Holder, restart the failover timer for them.
						c.scheduleFailoverTimerLocked()
					}
				}
			}
		}
	}(ch)
}

// tryCommitLocked commits the deterministic proposal-ref snapshot for this epoch.
func (c *Coordinator) tryCommitLocked() {
	if !c.singleWriter.IsTokenHolder() {
		return
	}
	if len(c.groupState) == 0 {
		return
	}
	batch := c.singleWriter.SnapshotNextBatch()
	if len(batch) == 0 {
		return
	}
	if !c.commitViewReadyLocked(batch) {
		c.deferCommitUntilViewReadyLocked()
		return
	}
	c.doCommitBatchLocked(batch)
}

// doCommitBatchLocked executes an MLS CreateCommit for the given batch, persists
// the result, and broadcasts the commit envelope. Called by tryCommitLocked
// (normal Token-Holder path) and the batched-commit timer.
func (c *Coordinator) doCommitBatchLocked(batch []BufferedProposal) {
	if len(c.groupState) == 0 {
		slog.Warn("doCommitBatchLocked: groupState is nil, skipping commit", "group", c.groupID)
		return
	}
	prevEpoch := c.epoch

	expectedRefs := make([][]byte, 0, len(batch))
	for i := range batch {
		expectedRefs = append(expectedRefs, append([]byte(nil), batch[i].ProposalRef...))
	}

	opCtx, cancel := c.mlsOperationContext()
	commitResult, err := c.mls.CreateCommit(opCtx, c.groupState, expectedRefs)
	cancel()
	if err != nil {
		slog.Warn("CreateCommit failed for pending proposal batch", "group", c.groupID, "epoch", c.epoch, "error", err)
		return
	}

	committedRefs := commitResult.CommittedProposalRefs
	if len(committedRefs) == 0 {
		committedRefs = expectedRefs
	}
	commitMsg := CommitMsg{
		CommitData:            commitResult.CommitBytes,
		NewTreeHash:           commitResult.NewTreeHash,
		IncludedProposals:     proposalMsgsFromBatch(batch),
		CommittedProposalRefs: cloneBytesList(committedRefs),
	}

	// Surface routing metadata for ProposalAdd commits so observer nodes can
	// correlate the commit with their local group_add_operations rows.
	if batchContainsType(batch, ProposalAdd) {
		commitMsg.AddDeliveries = buildAddDeliveriesFromBatch(batch, commitResult.WelcomeBytes)
	}

	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return
	}
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: commitResult.NewGroupState,
		Epoch:      nextEpoch,
		TreeHash:   commitResult.NewTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts, c.epoch)
	if err != nil || !applied {
		return
	}
	drained := c.singleWriter.DrainBatchByRefs(committedRefs)
	if len(drained) > 0 {
		batch = drained
	}
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(commitResult.NewGroupState, nextEpoch, commitResult.NewTreeHash, commitResult.CommitBytes)
	c.reconcileOperationsAfterCommitLocked(commitMsg)
	c.triggerBatchReplayAsync(c.groupID)
	c.reconcileAndRebaseOperationsLocked()

	c.updateLocalAccessRevocationLocked(commitResult.NewGroupState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitResult.CommitBytes)))
	c.emitCommitIssuedLocked(prevEpoch, nextEpoch, batch)

	// Hand off Welcome dispatch to the runtime. This local node is the Token
	// Holder that just ran CreateCommit, so it is the only node holding the
	// ephemeral key material required to deliver Welcome to each invitee.
	if batchContainsType(batch, ProposalAdd) && c.onAddCommitted != nil && len(commitMsg.AddDeliveries) > 0 {
		deliveries := append([]AddCommitDelivery(nil), commitMsg.AddDeliveries...)
		welcome := append([]byte(nil), commitResult.WelcomeBytes...)
		epoch := nextEpoch
		cb := c.onAddCommitted
		go func() {
			for _, d := range deliveries {
				cb(d, epoch, welcome)
			}
		}()
	}
}

func summarizeBufferedProposal(p BufferedProposal) CommitAuditProposalSummary {
	return CommitAuditProposalSummary{
		ProposalType: p.Type,
		OperationID:  p.OperationID,
		TargetPeerID: p.TargetPeerID,
		RequestID:    p.RequestID,
		GroupType:    p.GroupType,
		CategoryID:   p.CategoryID,
	}
}

func (c *Coordinator) emitCommitIssuedLocked(prevEpoch, newEpoch uint64, batch []BufferedProposal) {
	if c.onCommitIssued == nil {
		return
	}
	proposals := make([]CommitAuditProposalSummary, 0, len(batch))
	for _, item := range batch {
		proposals = append(proposals, summarizeBufferedProposal(item))
	}
	c.onCommitIssued(CommitAuditSummary{
		GroupID:           c.groupID,
		TokenHolderPeerID: c.localID.String(),
		PrevEpoch:         prevEpoch,
		NewEpoch:          newEpoch,
		Proposals:         proposals,
	})
}

func (c *Coordinator) createAndStoreLocalProposalLocked(pType ProposalType, data []byte, meta BufferedProposal) (ProposalMsg, BufferedProposal, error) {
	opCtx, cancel := c.mlsOperationContext()
	result, err := c.mls.CreateProposal(opCtx, c.groupState, pType, data)
	cancel()
	if err != nil {
		return ProposalMsg{}, BufferedProposal{}, err
	}
	if err := c.persistCurrentEpochStateLocked(result.NewGroupState); err != nil {
		return ProposalMsg{}, BufferedProposal{}, err
	}
	c.groupState = result.NewGroupState

	msg := ProposalMsg{
		ProposalType:   pType,
		Data:           append([]byte(nil), result.ProposalBytes...),
		ProposalRef:    append([]byte(nil), result.ProposalRef...),
		OperationID:    meta.OperationID,
		TargetPeerID:   meta.TargetPeerID,
		RequestID:      meta.RequestID,
		GroupType:      meta.GroupType,
		CategoryID:     meta.CategoryID,
		KeyPackageHash: append([]byte(nil), meta.KeyPackageHash...),
	}
	buffered := BufferedProposal{
		Type:           pType,
		Data:           append([]byte(nil), result.ProposalBytes...),
		ProposalRef:    append([]byte(nil), result.ProposalRef...),
		OperationID:    meta.OperationID,
		TargetPeerID:   meta.TargetPeerID,
		RequestID:      meta.RequestID,
		GroupType:      meta.GroupType,
		CategoryID:     meta.CategoryID,
		KeyPackageHash: append([]byte(nil), meta.KeyPackageHash...),
	}
	return msg, buffered, nil
}

func (c *Coordinator) persistCurrentEpochStateLocked(newState []byte) error {
	now := c.clock.Now()
	rec := &GroupRecord{
		GroupID:    c.groupID,
		GroupState: append([]byte(nil), newState...),
		Epoch:      c.epoch,
		TreeHash:   append([]byte(nil), c.treeHash...),
		UpdatedAt:  now,
	}
	if prev, err := c.storage.GetGroupRecord(c.groupID); err == nil && prev != nil {
		rec.MyRole = prev.MyRole
		rec.GroupType = prev.GroupType
		rec.CategoryID = prev.CategoryID
		rec.DMCounterpartyPeerID = prev.DMCounterpartyPeerID
		rec.CreatedAt = prev.CreatedAt
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	return c.storage.SaveGroupRecord(rec)
}

// buildAddDeliveriesFromBatch projects routing metadata for Add proposals in a
// mixed commit batch. The same Welcome bytes are referenced by WelcomeHash
// across deliveries because OpenMLS emits a single combined Welcome per commit.
func buildAddDeliveriesFromBatch(batch []BufferedProposal, welcomeBytes []byte) []AddCommitDelivery {
	if len(batch) == 0 {
		return nil
	}
	var welcomeHash []byte
	if len(welcomeBytes) > 0 {
		sum := sha256.Sum256(welcomeBytes)
		welcomeHash = sum[:]
	}
	out := make([]AddCommitDelivery, 0, len(batch))
	for _, p := range batch {
		if p.Type != ProposalAdd {
			continue
		}
		out = append(out, AddCommitDelivery{
			OperationID:    p.OperationID,
			TargetPeerID:   p.TargetPeerID,
			RequestID:      p.RequestID,
			GroupType:      p.GroupType,
			CategoryID:     p.CategoryID,
			KeyPackageHash: append([]byte(nil), p.KeyPackageHash...),
			WelcomeHash:    welcomeHash,
		})
	}
	return out
}

func proposalMsgsFromBatch(batch []BufferedProposal) []ProposalMsg {
	if len(batch) == 0 {
		return nil
	}
	out := make([]ProposalMsg, 0, len(batch))
	for _, p := range batch {
		out = append(out, ProposalMsg{
			ProposalType:   p.Type,
			Data:           append([]byte(nil), p.Data...),
			ProposalRef:    append([]byte(nil), p.ProposalRef...),
			OperationID:    p.OperationID,
			TargetPeerID:   p.TargetPeerID,
			RequestID:      p.RequestID,
			GroupType:      p.GroupType,
			CategoryID:     p.CategoryID,
			KeyPackageHash: append([]byte(nil), p.KeyPackageHash...),
		})
	}
	return out
}

func bufferedBatchFromProposalMsgs(proposals []ProposalMsg) []BufferedProposal {
	if len(proposals) == 0 {
		return nil
	}
	out := make([]BufferedProposal, 0, len(proposals))
	for _, p := range proposals {
		out = append(out, BufferedProposal{
			Type:           p.ProposalType,
			Data:           append([]byte(nil), p.Data...),
			ProposalRef:    append([]byte(nil), p.ProposalRef...),
			OperationID:    p.OperationID,
			TargetPeerID:   p.TargetPeerID,
			RequestID:      p.RequestID,
			GroupType:      p.GroupType,
			CategoryID:     p.CategoryID,
			KeyPackageHash: append([]byte(nil), p.KeyPackageHash...),
		})
	}
	return out
}

func proposalBytesFromMsgs(proposals []ProposalMsg) [][]byte {
	if len(proposals) == 0 {
		return nil
	}
	out := make([][]byte, 0, len(proposals))
	for _, p := range proposals {
		out = append(out, append([]byte(nil), p.Data...))
	}
	return out
}

func cloneBytesList(in [][]byte) [][]byte {
	if len(in) == 0 {
		return nil
	}
	out := make([][]byte, len(in))
	for i := range in {
		out[i] = append([]byte(nil), in[i]...)
	}
	return out
}

func proposalRefSetsEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	aa := cloneBytesList(a)
	bb := cloneBytesList(b)
	sortBytesList(aa)
	sortBytesList(bb)
	for i := range aa {
		if !bytes.Equal(aa[i], bb[i]) {
			return false
		}
	}
	return true
}

func sortBytesList(items [][]byte) {
	sort.SliceStable(items, func(i, j int) bool {
		return bytes.Compare(items[i], items[j]) < 0
	})
}

func removesPeer(batch []BufferedProposal, pid peer.ID) bool {
	if pid == "" {
		return false
	}
	for _, p := range batch {
		if p.Type == ProposalRemove && p.TargetPeerID == pid.String() {
			return true
		}
	}
	return false
}

func batchContainsType(batch []BufferedProposal, pType ProposalType) bool {
	for _, p := range batch {
		if p.Type == pType {
			return true
		}
	}
	return false
}

func (c *Coordinator) advanceEpochLocked(newState []byte, newEpoch uint64, newTreeHash, commitData []byte) {
	c.groupState = newState
	c.epoch = newEpoch
	c.treeHash = newTreeHash
	slog.Info("Epoch advanced", "group", c.groupID, "newEpoch", newEpoch)

	buffered := c.epochTracker.Advance(newEpoch, newTreeHash)
	c.singleWriter.AdvanceEpoch(newEpoch)
	if holder, err := c.singleWriter.CurrentTokenHolder(); err == nil {
		c.lastTokenHolder = holder
	}
	c.proposalTimerChan = nil // Reset active timer on epoch transition
	c.failoverTimerChan = nil // Reset failover timer on epoch transition

	commitHash := hashCommitData(commitData)
	c.lastCommitHash = copyBytes(commitHash)
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    newTreeHash,
		MemberCount: c.activeView.Size(),
		Epoch:       newEpoch,
		CommitHash:  commitHash,
	})
	if err := c.persistCoordStateLocked(); err != nil {
		slog.Warn("Failed to persist coordination state after epoch advance", "group", c.groupID, "error", err)
	}

	if c.onEpochChange != nil {
		c.onEpochChange(newEpoch)
	}

	for _, raw := range buffered {
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}
		switch env.Type {
		case MsgProposal:
			c.handleProposalLocked(decodeEnvelopePeerID(env.From, ""), &env)
		case MsgCommit:
			c.handleCommitLocked(&env, raw)
		case MsgApplication:
			c.handleApplicationLocked(decodeEnvelopePeerID(env.From, ""), &env, raw)
		case MsgApplicationBatched:
			c.handleApplicationBatchedLocked(decodeEnvelopePeerID(env.From, ""), &env, raw)
		}
	}
}

// ─── Public API ──────────────────────────────────────────────────────────────

// AddMemberRequest carries everything the runtime needs to (a) build a
// ProposalAdd envelope on a non-holder node, or (b) emit an AddCommitDelivery
// on the Token Holder node. The runtime is the source of truth for the
// operation_id, group_type and category_id; the coordinator only forwards
// these fields verbatim.
type AddMemberRequest struct {
	TargetPeerID    peer.ID
	KeyPackageBytes []byte
	OperationID     string
	RequestID       string
	GroupType       string
	CategoryID      string
	KeyPackageHash  []byte
}

// AddMember performs an MLS Add for a new member following the Single-Writer
// Protocol:
//
//   - If the local node is the current Token Holder and the startup ActiveView
//     is sufficiently initialized, the MLS commit is created synchronously and
//     the result includes the Welcome bytes for out-of-band delivery to the
//     invitee.
//   - Otherwise the local node creates a ProposalAdd carrying req's routing
//     metadata, broadcasts it to the group topic, buffers it locally (in
//     case the local node becomes the holder for the next epoch), and
//     returns Deferred=true. The returned Welcome is empty in the deferred
//     case — only the node that ultimately runs CreateCommit may author the
//     Welcome.
//
// AddMember NEVER returns ErrNotTokenHolder: failure of the local node to
// hold the token is part of the protocol, not an error condition.
func (c *Coordinator) AddMember(req AddMemberRequest) (AddMemberResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return AddMemberResult{}, fmt.Errorf("coordinator not started")
	}
	if c.accessRevoked {
		slog.Warn("Mutation rejected: access revoked",
			"group", c.groupID,
			"epoch", c.epoch,
			"op", "AddMember",
			"reason", "access_revoked",
			"violation_source", "local_membership_guard")
		return AddMemberResult{}, ErrAccessRevoked
	}
	if len(req.KeyPackageBytes) == 0 {
		return AddMemberResult{}, fmt.Errorf("AddMember: key package is required")
	}

	// Idempotency check
	idKey := "add_member_" + req.TargetPeerID.String()
	if req.OperationID != "" {
		if existing, err := c.storage.GetPendingOperation(req.OperationID); err == nil && existing != nil {
			if existing.Status == "COMMITTED" || existing.Status == "SATISFIED_BY_OTHER" {
				var commitEpoch uint64
				if existing.PreconditionEpoch != nil {
					commitEpoch = *existing.PreconditionEpoch
				}
				return AddMemberResult{
					OperationID: existing.OperationID,
					Deferred:    false,
					CommitEpoch: commitEpoch,
				}, nil
			}
			if existing.Status == "PROPOSED" {
				return AddMemberResult{
					OperationID: existing.OperationID,
					Deferred:    true,
				}, nil
			}
		}
	}
	if existing, err := c.storage.GetPendingOperationByIdempotencyKey(c.groupID, idKey); err == nil && existing != nil {
		if existing.Status == "COMMITTED" || existing.Status == "SATISFIED_BY_OTHER" {
			var commitEpoch uint64
			if existing.PreconditionEpoch != nil {
				commitEpoch = *existing.PreconditionEpoch
			}
			return AddMemberResult{
				OperationID: existing.OperationID,
				Deferred:    false,
				CommitEpoch: commitEpoch,
			}, nil
		}
		if existing.Status == "PROPOSED" {
			return AddMemberResult{
				OperationID: existing.OperationID,
				Deferred:    true,
			}, nil
		}
	}

	opID := req.OperationID
	if opID == "" {
		opID = "op_" + newTraceID()
	}

	epochCopy := c.epoch
	targetMemberCopy := req.TargetPeerID.String()
	idKeyCopy := idKey

	op := &PendingOperation{
		OperationID:       opID,
		GroupID:           c.groupID,
		OpType:            "ADD_MEMBER",
		IdempotencyKey:    &idKeyCopy,
		OperationHash:     append([]byte(nil), req.KeyPackageHash...),
		PreconditionEpoch: &epochCopy,
		TargetMemberID:    &targetMemberCopy,
		SemanticPayload:   req.KeyPackageBytes,
		Status:            "PENDING",
		CreatedAt:         c.clock.Now(),
		UpdatedAt:         c.clock.Now(),
	}
	_ = c.storage.SavePendingOperation(op)

	delivery := AddCommitDelivery{
		OperationID:    opID,
		TargetPeerID:   req.TargetPeerID.String(),
		RequestID:      req.RequestID,
		GroupType:      req.GroupType,
		CategoryID:     req.CategoryID,
		KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
	}

	msg, buffered, err := c.createAndStoreLocalProposalLocked(ProposalAdd, req.KeyPackageBytes, BufferedProposal{
		OperationID:    opID,
		TargetPeerID:   req.TargetPeerID.String(),
		RequestID:      req.RequestID,
		GroupType:      req.GroupType,
		CategoryID:     req.CategoryID,
		KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
	})
	if err != nil {
		errStr := err.Error()
		op.Status = "FAILED_PRECONDITION"
		op.LastError = &errStr
		op.UpdatedAt = c.clock.Now()
		_ = c.storage.SavePendingOperation(op)
		return AddMemberResult{}, fmt.Errorf("CreateProposal: %w", err)
	}

	c.singleWriter.BufferProposal(buffered)
	c.scheduleFailoverTimerLocked()
	c.metrics.IncrProposalsReceived()

	// Update op status to proposed
	h := sha256.Sum256(msg.Data)
	op.LatestProposalHash = h[:]
	op.Status = "PROPOSED"
	op.UpdatedAt = c.clock.Now()
	_ = c.storage.SavePendingOperation(op)

	if c.onProposalObserved != nil {
		c.onProposalObserved(ProposalAuditSummary{
			GroupID:        c.groupID,
			Epoch:          c.epoch,
			ActorPeerID:    c.localID.String(),
			ProposalType:   ProposalAdd,
			OperationID:    opID,
			TargetPeerID:   req.TargetPeerID.String(),
			RequestID:      req.RequestID,
			GroupType:      req.GroupType,
			CategoryID:     req.CategoryID,
			KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
		})
	}

	// Non-holder path: broadcast ProposalAdd and let whichever node holds
	// the token at the committing epoch author the Welcome.
	if c.singleWriter == nil || !c.singleWriter.IsTokenHolder() {
		c.broadcastLocked(MsgProposal, msg)
		return AddMemberResult{
			OperationID: opID,
			Deferred:    true,
			Delivery:    delivery,
		}, nil
	}
	if !c.commitViewReadyLocked([]BufferedProposal{buffered}) {
		c.broadcastLocked(MsgProposal, msg)
		c.deferCommitUntilViewReadyLocked()
		return AddMemberResult{
			OperationID: opID,
			Deferred:    true,
			Delivery:    delivery,
		}, nil
	}

	// Token Holder path: commit synchronously.
	slog.Info("Coordinator AddMember: token-holder path started",
		"group", c.groupID,
		"epoch", c.epoch,
		"target", req.TargetPeerID.String(),
		"operation_id", opID,
		"timeout", c.cfg.MLSOperationTimeout,
	)
	expectedRefs := [][]byte{buffered.ProposalRef}
	opCtx, cancel := c.mlsOperationContext()
	commitResult, err := c.mls.CreateCommit(opCtx, c.groupState, expectedRefs)
	cancel()
	if err != nil {
		return AddMemberResult{}, fmt.Errorf("CreateCommit: %w", err)
	}
	slog.Info("Coordinator AddMember: sidecar CreateCommit completed",
		"group", c.groupID,
		"next_epoch", c.epoch+1,
		"target", req.TargetPeerID.String(),
		"operation_id", opID,
		"commit_bytes", len(commitResult.CommitBytes),
		"welcome_bytes", len(commitResult.WelcomeBytes),
	)

	if len(commitResult.WelcomeBytes) > 0 {
		sum := sha256.Sum256(commitResult.WelcomeBytes)
		delivery.WelcomeHash = sum[:]
	}

	commitMsg := CommitMsg{
		CommitData:            commitResult.CommitBytes,
		NewTreeHash:           commitResult.NewTreeHash,
		AddDeliveries:         []AddCommitDelivery{delivery},
		IncludedProposals:     []ProposalMsg{msg},
		CommittedProposalRefs: cloneBytesList(commitResult.CommittedProposalRefs),
	}
	if len(commitMsg.CommittedProposalRefs) == 0 {
		commitMsg.CommittedProposalRefs = cloneBytesList(expectedRefs)
	}
	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return AddMemberResult{}, fmt.Errorf("failed to encode commit envelope")
	}
	prevEpoch := c.epoch
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: commitResult.NewGroupState,
		Epoch:      nextEpoch,
		TreeHash:   commitResult.NewTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts, c.epoch)
	if err != nil {
		return AddMemberResult{}, fmt.Errorf("persist commit: %w", err)
	}
	if !applied {
		return AddMemberResult{}, fmt.Errorf("commit envelope already applied")
	}
	c.singleWriter.DrainBatchByRefs(commitMsg.CommittedProposalRefs)
	slog.Info("Coordinator AddMember: commit persisted",
		"group", c.groupID,
		"next_epoch", nextEpoch,
		"target", req.TargetPeerID.String(),
		"operation_id", opID,
	)
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(commitResult.NewGroupState, nextEpoch, commitResult.NewTreeHash, commitResult.CommitBytes)
	c.reconcileOperationsAfterCommitLocked(commitMsg)
	c.reconcileAndRebaseOperationsLocked()

	c.updateLocalAccessRevocationLocked(commitResult.NewGroupState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitResult.CommitBytes)))
	c.emitCommitIssuedLocked(prevEpoch, nextEpoch, []BufferedProposal{{
		Type:           ProposalAdd,
		OperationID:    opID,
		TargetPeerID:   req.TargetPeerID.String(),
		RequestID:      req.RequestID,
		GroupType:      req.GroupType,
		CategoryID:     req.CategoryID,
		KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
	}})

	// Hand off Welcome delivery on the holder side. We dispatch via the
	// callback so the same outbox-style code path (pending_welcomes_out +
	// store replication + direct delivery) handles both this synchronous
	// commit and the asynchronous proposal-commit case in tryCommitLocked.
	if c.onAddCommitted != nil && len(commitResult.WelcomeBytes) > 0 {
		welcome := append([]byte(nil), commitResult.WelcomeBytes...)
		cb := c.onAddCommitted
		epoch := nextEpoch
		go cb(delivery, epoch, welcome)
	}

	return AddMemberResult{
		OperationID: opID,
		Welcome:     commitResult.WelcomeBytes,
		CommitEpoch: nextEpoch,
		Delivery:    delivery,
	}, nil
}

// RemoveMember removes an existing member from the group.
//
// Behavior follows Single-Writer:
//   - If local node is Token Holder, it commits removal immediately.
//   - Otherwise it broadcasts a ProposalRemove and waits for the holder commit.
//
// targetIdentity must be the MLS BasicCredential identity bytes (signing public
// key bytes) for the member to remove.
func (c *Coordinator) RemoveMember(targetIdentity []byte) error {
	return c.RemoveMemberWithPeer(RemoveMemberRequest{TargetIdentity: targetIdentity})
}

func (c *Coordinator) RemoveMemberWithPeer(req RemoveMemberRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return fmt.Errorf("coordinator not started")
	}
	if c.accessRevoked {
		slog.Warn("Mutation rejected: access revoked",
			"group", c.groupID,
			"epoch", c.epoch,
			"op", "RemoveMember",
			"reason", "access_revoked",
			"violation_source", "local_membership_guard")
		return ErrAccessRevoked
	}
	if len(req.TargetIdentity) == 0 {
		return fmt.Errorf("target identity is required")
	}

	// Idempotency check
	idKey := "remove_member_" + req.TargetPeerID.String()
	if req.OperationID != "" {
		if existing, err := c.storage.GetPendingOperation(req.OperationID); err == nil && existing != nil {
			if existing.Status == "COMMITTED" || existing.Status == "SATISFIED_BY_OTHER" {
				return nil
			}
			if existing.Status == "PROPOSED" {
				return nil
			}
		}
	}
	if existing, err := c.storage.GetPendingOperationByIdempotencyKey(c.groupID, idKey); err == nil && existing != nil {
		if existing.Status == "COMMITTED" || existing.Status == "SATISFIED_BY_OTHER" {
			return nil
		}
		if existing.Status == "PROPOSED" {
			return nil
		}
	}

	opID := req.OperationID
	if opID == "" {
		opID = "op_" + newTraceID()
	}

	epochCopy := c.epoch
	targetMemberCopy := req.TargetPeerID.String()
	idKeyCopy := idKey

	op := &PendingOperation{
		OperationID:       opID,
		GroupID:           c.groupID,
		OpType:            "REMOVE_MEMBER",
		IdempotencyKey:    &idKeyCopy,
		OperationHash:     nil,
		PreconditionEpoch: &epochCopy,
		TargetMemberID:    &targetMemberCopy,
		SemanticPayload:   req.TargetIdentity,
		Status:            "PENDING",
		CreatedAt:         c.clock.Now(),
		UpdatedAt:         c.clock.Now(),
	}
	_ = c.storage.SavePendingOperation(op)

	if c.singleWriter == nil || !c.singleWriter.IsTokenHolder() {
		// Update op status to proposed before we exit
		op.Status = "PROPOSED"
		op.UpdatedAt = c.clock.Now()
		_ = c.storage.SavePendingOperation(op)

		return c.proposeLockedWithMetadata(ProposalRemove, req.TargetIdentity, BufferedProposal{TargetPeerID: req.TargetPeerID.String(), OperationID: opID})
	}

	msg, buffered, err := c.createAndStoreLocalProposalLocked(ProposalRemove, req.TargetIdentity, BufferedProposal{TargetPeerID: req.TargetPeerID.String(), OperationID: opID})
	if err != nil {
		errStr := err.Error()
		op.Status = "FAILED_PRECONDITION"
		op.LastError = &errStr
		op.UpdatedAt = c.clock.Now()
		_ = c.storage.SavePendingOperation(op)
		return fmt.Errorf("CreateProposal: %w", err)
	}

	c.singleWriter.BufferProposal(buffered)
	c.scheduleFailoverTimerLocked()

	// Update op status to proposed
	h := sha256.Sum256(msg.Data)
	op.LatestProposalHash = h[:]
	op.Status = "PROPOSED"
	op.UpdatedAt = c.clock.Now()
	_ = c.storage.SavePendingOperation(op)

	if !c.commitViewReadyLocked([]BufferedProposal{buffered}) {
		c.broadcastLocked(MsgProposal, msg)
		c.deferCommitUntilViewReadyLocked()
		return nil
	}
	expectedRefs := [][]byte{buffered.ProposalRef}
	opCtx, cancel := c.mlsOperationContext()
	commitResult, err := c.mls.CreateCommit(opCtx, c.groupState, expectedRefs)
	cancel()
	if err != nil {
		return fmt.Errorf("CreateCommit: %w", err)
	}

	commitMsg := CommitMsg{
		CommitData:            commitResult.CommitBytes,
		NewTreeHash:           commitResult.NewTreeHash,
		IncludedProposals:     []ProposalMsg{msg},
		CommittedProposalRefs: cloneBytesList(commitResult.CommittedProposalRefs),
	}
	if len(commitMsg.CommittedProposalRefs) == 0 {
		commitMsg.CommittedProposalRefs = cloneBytesList(expectedRefs)
	}
	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return fmt.Errorf("failed to encode commit envelope")
	}
	prevEpoch := c.epoch
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: commitResult.NewGroupState,
		Epoch:      nextEpoch,
		TreeHash:   commitResult.NewTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts, c.epoch)
	if err != nil {
		return fmt.Errorf("persist commit: %w", err)
	}
	if !applied {
		return fmt.Errorf("commit envelope already applied")
	}
	c.singleWriter.DrainBatchByRefs(commitMsg.CommittedProposalRefs)
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(commitResult.NewGroupState, nextEpoch, commitResult.NewTreeHash, commitResult.CommitBytes)
	c.reconcileOperationsAfterCommitLocked(commitMsg)
	c.reconcileAndRebaseOperationsLocked()

	c.updateLocalAccessRevocationLocked(commitResult.NewGroupState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitResult.CommitBytes)))
	c.emitCommitIssuedLocked(prevEpoch, nextEpoch, []BufferedProposal{{
		Type:         ProposalRemove,
		TargetPeerID: req.TargetPeerID.String(),
		OperationID:  opID,
	}})

	return nil
}

// SendMessage encrypts plaintext and broadcasts it as an application message.
// Returns the HLC timestamp assigned to the message.
func (c *Coordinator) SendMessage(plaintext []byte) (*HLCTimestamp, error) {
	return c.sendMessage(plaintext, "")
}

// SendMessageWithLocalEchoToken mirrors SendMessage but tags the locally emitted
// StoredMessage with a process-local correlation token so the frontend can
// replace optimistic echoes deterministically.
func (c *Coordinator) SendMessageWithLocalEchoToken(plaintext []byte, localEchoToken string) (*HLCTimestamp, error) {
	return c.sendMessage(plaintext, localEchoToken)
}

func (c *Coordinator) sendMessage(plaintext []byte, localEchoToken string) (*HLCTimestamp, error) {
	if c.healing.Load() {
		return nil, fmt.Errorf("fork healing in progress: message rejected to avoid cross-epoch state corruption")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil, fmt.Errorf("coordinator not started")
	}
	c.updateLocalAccessRevocationLocked(c.groupState, c.epoch)
	if c.accessRevoked {
		slog.Warn("Mutation rejected: access revoked",
			"group", c.groupID,
			"epoch", c.epoch,
			"op", "SendMessage",
			"reason", "access_revoked",
			"violation_source", "local_membership_guard")
		return nil, ErrAccessRevoked
	}

	ts := c.hlc.Now()
	if ts.NodeID == "" {
		ts.NodeID = c.localID.String()
	}

	opCtx, cancel := c.mlsOperationContext()
	ciphertext, newState, err := c.mls.EncryptMessage(opCtx, c.groupState, plaintext)
	cancel()
	if err != nil {
		slog.Error("Failed to encrypt message", "group", c.groupID, "error", err)
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgApplication, ApplicationMsg{Ciphertext: ciphertext}, ts)
	if len(envBytes) == 0 {
		return nil, fmt.Errorf("encode envelope")
	}
	envelopeHash := hashEnvelope(envBytes)

	msg := &StoredMessage{
		GroupID:        c.groupID,
		Epoch:          c.epoch,
		SenderID:       c.localID,
		Content:        plaintext,
		Timestamp:      ts,
		LocalEchoToken: localEchoToken,
		EnvelopeHash:   envelopeHash,
	}
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyApplication(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		UpdatedAt:  now,
	}, msg, MsgApplication, envBytes, ts, c.epoch)
	if err != nil {
		return nil, fmt.Errorf("persist application: %w", err)
	}
	if !applied {
		return nil, fmt.Errorf("application envelope already applied")
	}
	c.groupState = newState
	c.publishPreparedEnvelopeLocked(MsgApplication, envBytes)
	c.trackPendingApplicationDeliveriesLocked(envBytes, envelopeHash)
	slog.Info("Message sent", "group", c.groupID, "epoch", c.epoch, "ts", ts.WallTimeMs)
	if c.onMessage != nil {
		c.onMessage(msg)
	}

	return &ts, nil
}

func pendingAppDeliveryKey(pid peer.ID, envelopeHash []byte) string {
	return pid.String() + "|" + hex.EncodeToString(envelopeHash)
}

func (c *Coordinator) trackPendingApplicationDeliveriesLocked(envBytes, envelopeHash []byte) {
	if len(envBytes) == 0 || len(envelopeHash) == 0 || c.cfg.ApplicationDirectRetryLimit == 0 {
		return
	}
	recipients := c.applicationRecipientsLocked()
	for _, pid := range recipients {
		key := pendingAppDeliveryKey(pid, envelopeHash)
		if _, exists := c.pendingAppDeliveries[key]; exists {
			continue
		}
		c.pendingAppDeliveries[key] = &pendingAppDelivery{
			peerID:       pid,
			envelopeHash: hex.EncodeToString(envelopeHash),
			envelope:     append([]byte(nil), envBytes...),
		}
		go c.applicationAckWatchLoop(pid, append([]byte(nil), envelopeHash...))
	}
}

func (c *Coordinator) applicationRecipientsLocked() []peer.ID {
	seen := make(map[string]struct{})
	out := make([]peer.ID, 0)
	addPeer := func(pid peer.ID) {
		if pid == "" || pid == c.localID {
			return
		}
		key := pid.String()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, pid)
	}
	for _, pid := range c.activeView.Members() {
		addPeer(pid)
	}
	if len(out) == 0 {
		for _, pid := range c.transport.ConnectedPeers() {
			addPeer(pid)
		}
	}
	return out
}

func (c *Coordinator) applicationAckWatchLoop(pid peer.ID, envelopeHash []byte) {
	key := pendingAppDeliveryKey(pid, envelopeHash)
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(c.cfg.ApplicationAckTimeout):
		}

		c.mu.Lock()
		pending, ok := c.pendingAppDeliveries[key]
		if !ok {
			c.mu.Unlock()
			return
		}
		if pending.attempts >= c.cfg.ApplicationDirectRetryLimit {
			c.mu.Unlock()
			return
		}
		pending.attempts++
		wire := append([]byte(nil), pending.envelope...)
		groupID := c.groupID
		attempt := pending.attempts
		c.mu.Unlock()

		if err := c.sendDirectEnvelope(pid, wire); err != nil {
			slog.Debug("application direct retry failed",
				"group", groupID,
				"peer", pid,
				"attempt", attempt,
				"err", err,
			)
			continue
		}
		slog.Info("application direct retry sent",
			"group", groupID,
			"peer", pid,
			"attempt", attempt,
		)
	}
}

func (c *Coordinator) sendDeliveryAckLocked(to peer.ID, envelopeHash []byte) {
	if to == "" || to == c.localID || len(envelopeHash) == 0 {
		return
	}
	envBytes := c.buildEnvelopeWithEpochAndTimestampLocked(
		MsgDeliveryAck,
		DeliveryAckMsg{EnvelopeHash: append([]byte(nil), envelopeHash...)},
		c.epoch,
		c.hlc.Now(),
	)
	if len(envBytes) == 0 {
		return
	}
	go func(pid peer.ID, wire []byte) {
		if err := c.sendDirectEnvelope(pid, wire); err != nil {
			slog.Debug("delivery ack send failed", "group", c.groupID, "peer", pid, "err", err)
		}
	}(to, envBytes)
}

func (c *Coordinator) sendDirectEnvelope(to peer.ID, wire []byte) error {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	dctx, cancel := context.WithTimeout(ctx, c.cfg.ApplicationAckTimeout)
	defer cancel()
	return c.transport.SendDirect(dctx, to, wire)
}

// RetryOutstandingDeliveriesTo immediately re-sends every still-unacked local
// application envelope to one peer. Intended for reconnect / re-verify hooks.
func (c *Coordinator) RetryOutstandingDeliveriesTo(pid peer.ID) {
	if pid == "" || pid == c.localID {
		return
	}
	c.mu.Lock()
	var wires [][]byte
	for _, pending := range c.pendingAppDeliveries {
		if pending.peerID != pid {
			continue
		}
		wires = append(wires, append([]byte(nil), pending.envelope...))
	}
	c.mu.Unlock()
	for _, wire := range wires {
		if err := c.sendDirectEnvelope(pid, wire); err != nil {
			slog.Debug("retry outstanding delivery failed", "group", c.groupID, "peer", pid, "err", err)
		}
	}
}

// ProposeAdd broadcasts an Add proposal.
func (c *Coordinator) ProposeAdd(memberData []byte) error {
	return c.propose(ProposalAdd, memberData)
}

// ProposeRemove broadcasts a Remove proposal.
func (c *Coordinator) ProposeRemove(memberData []byte) error {
	return c.propose(ProposalRemove, memberData)
}

// ProposeUpdate broadcasts an Update proposal (key rotation).
func (c *Coordinator) ProposeUpdate(data []byte) error {
	return c.propose(ProposalUpdate, data)
}

func (c *Coordinator) propose(pType ProposalType, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.proposeLocked(pType, data)
}

func (c *Coordinator) proposeLocked(pType ProposalType, data []byte) error {
	return c.proposeLockedWithMetadata(pType, data, BufferedProposal{})
}

func (c *Coordinator) proposeLockedWithMetadata(pType ProposalType, data []byte, meta BufferedProposal) error {
	if !c.started {
		return fmt.Errorf("coordinator not started")
	}
	if c.accessRevoked {
		slog.Warn("Mutation rejected: access revoked",
			"group", c.groupID,
			"epoch", c.epoch,
			"op", "Propose",
			"reason", "access_revoked",
			"violation_source", "local_membership_guard")
		return ErrAccessRevoked
	}

	msg, buffered, err := c.createAndStoreLocalProposalLocked(pType, data, meta)
	if err != nil {
		return fmt.Errorf("CreateProposal: %w", err)
	}

	c.broadcastLocked(MsgProposal, msg)

	c.singleWriter.BufferProposal(buffered)
	c.scheduleFailoverTimerLocked()
	c.metrics.IncrProposalsReceived()
	if c.onProposalObserved != nil {
		c.onProposalObserved(ProposalAuditSummary{
			GroupID:      c.groupID,
			Epoch:        c.epoch,
			ActorPeerID:  c.localID.String(),
			ProposalType: pType,
			TargetPeerID: meta.TargetPeerID,
		})
	}

	if c.singleWriter.IsTokenHolder() {
		c.scheduleBatchCommitLocked()
	}
	return nil
}

func deriveIdentityFromSigningKey(signingKey []byte) []byte {
	var pub ed25519.PublicKey
	switch len(signingKey) {
	case ed25519.SeedSize:
		pub = ed25519.NewKeyFromSeed(signingKey).Public().(ed25519.PublicKey)
	case ed25519.PrivateKeySize:
		pub = ed25519.PrivateKey(signingKey).Public().(ed25519.PublicKey)
	default:
		return nil
	}
	out := make([]byte, len(pub))
	copy(out, pub)
	return out
}

// updateLocalAccessRevocationLocked checks local membership against the latest
// MLS state and flips accessRevoked once if local identity is no longer present.
// Caller must hold c.mu.
func (c *Coordinator) updateLocalAccessRevocationLocked(groupState []byte, epoch uint64) {
	if c.accessRevoked {
		return
	}
	if len(c.localIdentity) == 0 {
		return
	}
	opCtx, cancel := c.mlsOperationContext()
	ok, err := c.mls.HasMember(opCtx, groupState, c.localIdentity)
	cancel()
	if err != nil {
		slog.Warn("Failed membership query", "group", c.groupID, "epoch", epoch, "err", err)
		return
	}
	if ok {
		return
	}
	c.accessRevoked = true
	slog.Warn("Local membership revoked", "group", c.groupID, "epoch", epoch)
	if c.onAccessLost != nil {
		cb := c.onAccessLost
		groupID := c.groupID
		go cb(groupID, epoch, "removed")
	}
}

// ─── Broadcasting ────────────────────────────────────────────────────────────

func (c *Coordinator) broadcastLocked(msgType MessageType, payload interface{}) {
	_ = c.broadcastWithTimestampLocked(msgType, payload, c.hlc.Now())
}

func (c *Coordinator) broadcastWithTimestampLocked(msgType MessageType, payload interface{}, ts HLCTimestamp) []byte {
	envBytes := c.buildEnvelopeWithTimestampLocked(msgType, payload, ts)
	if len(envBytes) == 0 {
		return nil
	}
	c.publishPreparedEnvelopeLocked(msgType, envBytes)
	return envBytes
}

func (c *Coordinator) buildEnvelopeWithTimestampLocked(msgType MessageType, payload interface{}, ts HLCTimestamp) []byte {
	return c.buildEnvelopeWithEpochAndTimestampLocked(msgType, payload, c.epoch, ts)
}

func (c *Coordinator) buildEnvelopeWithEpochAndTimestampLocked(msgType MessageType, payload interface{}, epoch uint64, ts HLCTimestamp) []byte {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	env := Envelope{
		Type:      msgType,
		GroupID:   c.groupID,
		Epoch:     epoch,
		From:      c.localID.String(),
		Timestamp: ts,
		Payload:   payloadBytes,
	}
	envBytes, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	return envBytes
}

func (c *Coordinator) publishPreparedEnvelopeLocked(msgType MessageType, envBytes []byte) {
	if err := c.transport.Publish(c.ctx, GroupTopic(c.groupID), envBytes); err != nil {
		slog.Warn("Failed to publish coordination envelope",
			"group", c.groupID,
			"type", msgType,
			"epoch", c.epoch,
			"err", err,
		)
	}
	if c.onEnvelope != nil && (msgType == MsgCommit || msgType == MsgApplication) {
		c.onEnvelope(msgType, c.groupID, envBytes)
	}
}

func (c *Coordinator) appendOfflineEnvelopeLocked(wire []byte) {
	if c.cfg == nil || !c.cfg.OfflineSyncEnabled || len(wire) == 0 {
		return
	}
	var env Envelope
	if err := json.Unmarshal(wire, &env); err != nil {
		return
	}
	if env.Type != MsgCommit && env.Type != MsgApplication {
		return
	}
	_, _ = c.storage.AppendEnvelope(c.groupID, env.Type, env.Epoch, env.Timestamp, wire)
}

// ReplayEnvelopesDetailed applies ciphertext envelopes from offline sync / DHT
// in order and reports whether each envelope was only seen, actually applied,
// or blocked behind a future epoch.
// Caller must hold no Coordinator lock; this method is fully synchronized.
func (c *Coordinator) ReplayEnvelopesDetailed(blobs [][]byte) ([]ReplayEnvelopeResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil, fmt.Errorf("coordinator not started")
	}

	results := make([]ReplayEnvelopeResult, 0, len(blobs))
	for _, raw := range blobs {
		if len(raw) == 0 {
			continue
		}
		var env Envelope
		if jerr := json.Unmarshal(raw, &env); jerr != nil {
			results = append(results, ReplayEnvelopeResult{
				EnvelopeHash: hashEnvelope(raw),
				State:        ReplayStateInvalid,
				Error:        jerr.Error(),
				Terminal:     true,
			})
			continue
		}
		if env.GroupID != c.groupID {
			results = append(results, ReplayEnvelopeResult{
				GroupID:      env.GroupID,
				MsgType:      env.Type,
				EnvelopeHash: hashEnvelope(raw),
				State:        ReplayStateInvalid,
				MsgEpoch:     env.Epoch,
				LocalEpoch:   c.epoch,
				Error:        fmt.Sprintf("envelope group %q does not match coordinator group %q", env.GroupID, c.groupID),
				Terminal:     true,
			})
			continue
		}

		// P0.2 Clock Skew protection:
		nowMs := c.clock.Now().UnixMilli()
		if err := validateSenderTimestamp(nowMs, env.Timestamp.WallTimeMs); err != nil {
			results = append(results, ReplayEnvelopeResult{
				GroupID:      env.GroupID,
				MsgType:      env.Type,
				EnvelopeHash: hashEnvelope(raw),
				State:        ReplayStateInvalid,
				MsgEpoch:     env.Epoch,
				LocalEpoch:   c.epoch,
				Error:        err.Error(),
				Terminal:     true,
				CursorSafe:   false, // future skewed timestamp should not advance cursor!
			})
			continue
		}
		switch env.Type {
		case MsgCommit:
			results = append(results, c.handleCommitDetailedLocked(&env, raw))
		case MsgApplication:
			results = append(results, c.handleApplicationDetailedLocked(decodeEnvelopePeerID(env.From, ""), &env, raw))
		default:
			results = append(results, ReplayEnvelopeResult{
				GroupID:      env.GroupID,
				MsgType:      env.Type,
				EnvelopeHash: hashEnvelope(raw),
				State:        ReplayStateInvalid,
				MsgEpoch:     env.Epoch,
				LocalEpoch:   c.epoch,
				Error:        fmt.Sprintf("unsupported envelope type %q", env.Type),
				Terminal:     true,
			})
			continue
		}
	}
	return results, nil
}

// ReplayEnvelopes applies ciphertext envelopes from offline sync / DHT in order.
// It is kept as a compatibility wrapper for older callers/tests.
func (c *Coordinator) ReplayEnvelopes(blobs [][]byte) (applied int, err error) {
	results, err := c.ReplayEnvelopesDetailed(blobs)
	if err != nil {
		return 0, err
	}
	for _, result := range results {
		if result.State == ReplayStateApplied {
			applied++
		}
	}
	return applied, nil
}

func (c *Coordinator) handleCommitDetailedLocked(env *Envelope, wire []byte) ReplayEnvelopeResult {
	result := c.newReplayResultLocked(env, wire)
	envelopeHash, alreadyApplied := c.checkAppliedEnvelopeLocked(env, wire)
	result.EnvelopeHash = envelopeHash
	if alreadyApplied {
		result.State = ReplayStateDuplicateApplied
		result.AlreadyApplied = true
		result.CursorSafe = true
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		result.State = ReplayStateStaleEpoch
		result.Terminal = true
		result.CursorSafe = true
		c.markReplayResultLocked(result)
		return result
	case ActionBufferFuture:
		c.epochTracker.BufferFuture(env.Epoch, wire)
		result.State = ReplayStateFutureEpoch
		c.markReplayResultLocked(result)
		return result
	}

	if c.handleCommitLocked(env, wire) {
		result.State = ReplayStateApplied
		result.Applied = true
		result.CursorSafe = true
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	if applied, err := c.storage.HasAppliedEnvelope(c.groupID, envelopeHash); err != nil {
		result.State = ReplayStatePersistFailed
		result.Error = err.Error()
	} else if applied {
		result.State = ReplayStateDuplicateApplied
		result.AlreadyApplied = true
		result.CursorSafe = true
		result.Terminal = true
	} else {
		result.State = ReplayStateInvalid
		result.Error = "commit replay did not apply"
		result.Terminal = true
	}
	c.markReplayResultLocked(result)
	return result
}

func validateSenderTimestamp(nowMs int64, senderMs int64) error {
	const maxFutureSkewMs = int64(5 * 60 * 1000) // 5 minutes
	if senderMs > nowMs+maxFutureSkewMs {
		return fmt.Errorf("sender timestamp too far in future: received %d is more than %dms ahead of physical %d", senderMs, maxFutureSkewMs, nowMs)
	}
	return nil
}

func (c *Coordinator) newReplayResultLocked(env *Envelope, wire []byte) ReplayEnvelopeResult {
	if env == nil {
		return ReplayEnvelopeResult{
			EnvelopeHash: hashEnvelope(wire),
			State:        ReplayStateInvalid,
			LocalEpoch:   c.epoch,
		}
	}
	return ReplayEnvelopeResult{
		GroupID:      env.GroupID,
		MsgType:      env.Type,
		EnvelopeHash: hashEnvelope(wire),
		MsgEpoch:     env.Epoch,
		LocalEpoch:   c.epoch,
	}
}

func (c *Coordinator) markReplayResultLocked(result ReplayEnvelopeResult) {
	if len(result.EnvelopeHash) == 0 || result.State == "" {
		return
	}
	if err := c.storage.MarkEnvelopeReplayState(c.groupID, result.EnvelopeHash, result.State, result.Error, c.clock.Now()); err != nil {
		slog.Warn("Failed to mark envelope replay state", "group", c.groupID, "state", result.State, "err", err)
	}
}

func (c *Coordinator) checkAppliedEnvelopeLocked(env *Envelope, wire []byte) ([]byte, bool) {
	if len(wire) == 0 || (env.Type != MsgCommit && env.Type != MsgApplication) {
		return nil, false
	}
	envelopeHash := hashEnvelope(wire)
	applied, err := c.storage.HasAppliedEnvelope(c.groupID, envelopeHash)
	if err != nil {
		slog.Warn("Failed to query applied envelope", "group", c.groupID, "type", env.Type, "err", err)
		return envelopeHash, false
	}
	return envelopeHash, applied
}

func hashEnvelope(wire []byte) []byte {
	if len(wire) == 0 {
		return nil
	}
	sum := sha256.Sum256(wire)
	return sum[:]
}

func hashCommitData(commitData []byte) []byte {
	if len(commitData) == 0 {
		return nil
	}
	sum := sha256.Sum256(commitData)
	return sum[:]
}

func (c *Coordinator) markInvalidCommitLocked(commitHash []byte) {
	if len(commitHash) == 0 || c.forkDetector == nil {
		return
	}
	c.forkDetector.MarkInvalidCommit(commitHash)
}

func (c *Coordinator) persistCoordStateLocked() error {
	if c.storage == nil {
		return nil
	}
	var holder peer.ID
	if c.singleWriter != nil {
		if h, err := c.singleWriter.CurrentTokenHolder(); err == nil {
			holder = h
		}
	}
	return c.storage.SaveCoordState(&CoordState{
		GroupID:        c.groupID,
		ActiveView:     c.activeView.Members(),
		TokenHolder:    holder,
		LastCommitHash: copyBytes(c.lastCommitHash),
	})
}

func decodeEnvelopePeerID(raw string, fallback peer.ID) peer.ID {
	if raw == "" {
		return fallback
	}
	id, err := peer.Decode(raw)
	if err != nil {
		return fallback
	}
	return id
}

// ─── Periodic Tasks ──────────────────────────────────────────────────────────

// heartbeatLoop sends a liveness heartbeat at HeartbeatInterval. Runs on its
// own goroutine so its cadence is independent of the announce loop — tests can
// drive each timer in isolation via the FakeClock without coupling.
func (c *Coordinator) heartbeatLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(c.cfg.HeartbeatInterval):
			c.mu.Lock()
			c.activeView.CheckLiveness()
			c.broadcastLocked(MsgHeartbeat, HeartbeatMsg{})
			c.mu.Unlock()
		}
	}
}

// announceLoop broadcasts a GroupStateAnnouncement at AnnounceInterval. It is
// only spawned when AnnounceInterval > 0; tests using TestConfig (interval=0)
// drive announces manually via BroadcastAnnounce.
func (c *Coordinator) announceLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(c.cfg.AnnounceInterval):
			c.mu.Lock()
			c.broadcastAnnounceLocked()
			c.mu.Unlock()
		}
	}
}

// broadcastAnnounceLocked publishes a GroupStateAnnouncement reflecting the
// node's current TreeHash, member view size, and last commit hash. Caller must
// hold c.mu.
func (c *Coordinator) broadcastAnnounceLocked() {
	if c.groupState == nil || c.healing.Load() {
		return
	}
	ann := GroupStateAnnouncement{
		TreeHash:    c.treeHash,
		MemberCount: c.activeView.Size(),
		Epoch:       c.epoch,
		CommitHash:  copyBytes(c.lastCommitHash),
	}
	c.forkDetector.UpdateLocal(ann)
	c.broadcastLocked(MsgAnnounce, ann)
}

// ─── Manual Triggers (for tests) ─────────────────────────────────────────────

// BroadcastHeartbeat sends a heartbeat immediately. Used in tests to avoid
// depending on timer goroutines.
func (c *Coordinator) BroadcastHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.broadcastLocked(MsgHeartbeat, HeartbeatMsg{})
}

// BroadcastAnnounce sends a GroupStateAnnouncement immediately.
func (c *Coordinator) BroadcastAnnounce() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.broadcastAnnounceLocked()
}

// TriggerLivenessCheck runs a liveness check immediately. Returns evicted peers.
func (c *Coordinator) TriggerLivenessCheck() []peer.ID {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.activeView.CheckLiveness()
}

// ─── Observability ───────────────────────────────────────────────────────────

func (c *Coordinator) CurrentEpoch() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch
}

func (c *Coordinator) ActiveMembers() []peer.ID {
	return c.activeView.Members()
}

func (c *Coordinator) IsTokenHolder() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.singleWriter == nil {
		return false
	}
	return c.singleWriter.IsTokenHolder()
}

// CurrentTokenHolder returns the PeerID of the elected Token Holder for the current epoch.
func (c *Coordinator) CurrentTokenHolder() (peer.ID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.singleWriter == nil {
		return "", fmt.Errorf("singleWriter not initialized")
	}
	return c.singleWriter.CurrentTokenHolder()
}

func (c *Coordinator) GetMetrics() MetricsSnapshot {
	return c.metrics.Snapshot()
}

func (c *Coordinator) GetGroupState() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]byte, len(c.groupState))
	copy(cp, c.groupState)
	return cp
}

// GetTreeHash returns a copy of the latest local tree hash snapshot.
func (c *Coordinator) GetTreeHash() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]byte, len(c.treeHash))
	copy(cp, c.treeHash)
	return cp
}

// GetOperationalMode returns the current operational mode of the coordinator.
func (c *Coordinator) GetOperationalMode() GroupOperationalMode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.operationalMode
}

// SetOperationalMode updates the operational mode of the coordinator.
func (c *Coordinator) SetOperationalMode(mode GroupOperationalMode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.operationalMode = mode
}

// IsHealing reports whether a fork-heal goroutine is currently in flight.
// Exported for tests and runtime diagnostics.
func (c *Coordinator) IsHealing() bool {
	return c.healing.Load()
}

// ─── Fork Healing Orchestration ──────────────────────────────────────────────
//
// scheduleHeal kicks off the fork-heal pipeline for an event flagged by
// ForkDetector as NeedExternalJoin. At most one heal goroutine may run per
// Coordinator instance — the atomic CAS on c.healing acts as a non-blocking
// mutex so handleAnnounceLocked never stalls on a long-running heal even when
// several peers race to surface the same partition.
//
// Sprint 2B implements only the scaffold (flag management + structured logging
// contract). Sprint 2C–2E will fill in the actual GroupInfo exchange, atomic
// state transition, and Autonomous Replay between the "started" and
// "completed" log lines.
//
// Caller must hold c.mu.
func (c *Coordinator) scheduleHeal(event *ForkEvent) {
	if event == nil {
		return
	}

	// Snapshot fields needed by the goroutine *before* releasing c.mu so the
	// goroutine never reads from c without re-locking.
	traceID := newTraceID()
	localEpoch := c.epoch
	scheduledAt := c.clock.Now()

	// Catch-up sync retry guard: if we are lagging behind, try sequential sync first.
	if event.RemoteEpoch > localEpoch && c.onSyncRequired != nil {
		if c.syncRetryAttempts < 3 {
			c.deferredForkEvent = event
			slog.Info("fork_heal/deferred_for_sync",
				"group", event.GroupID,
				"local_epoch", localEpoch,
				"winner_epoch", event.RemoteEpoch,
				"winner_peer", event.RemotePeer.String(),
				"attempt", c.syncRetryAttempts+1,
			)
			cb := c.onSyncRequired
			peerID := event.RemotePeer
			groupID := event.GroupID
			go cb(peerID, groupID) // Fire asynchronously to avoid holding c.mu
			return
		}
	}

	// Retry limit exhausted or not lagging. Proceed with immediate destructive healing.
	c.deferredForkEvent = nil

	localTreeHash := append([]byte(nil), c.treeHash...)
	partitionWindowMs := int64(0)
	if !event.PartitionStartedAt.IsZero() {
		partitionWindowMs = scheduledAt.Sub(event.PartitionStartedAt).Milliseconds()
	}

	if !c.healing.CompareAndSwap(false, true) {
		slog.Info("fork_heal/skipped_already_running",
			"trace_id", traceID,
			"group", event.GroupID,
			"local_epoch", localEpoch,
			"winner_peer", event.RemotePeer.String(),
			"winner_epoch", event.RemoteEpoch,
		)
		return
	}

	c.metrics.IncrForkHealingsAttempted()

	slog.Info("fork_heal/scheduled",
		"trace_id", traceID,
		"group", event.GroupID,
		"local_epoch", localEpoch,
		"local_tree_hash", hex.EncodeToString(localTreeHash),
		"local_member_count", event.LocalAnnounce.MemberCount,
		"winner_peer", event.RemotePeer.String(),
		"winner_epoch", event.RemoteEpoch,
		"winner_tree_hash", hex.EncodeToString(event.RemoteAnnounce.TreeHash),
		"winner_member_count", event.RemoteAnnounce.MemberCount,
		"partition_started_at_ms", event.PartitionStartedAt.UnixMilli(),
		"partition_window_ms", partitionWindowMs,
		"scheduled_at_ms", scheduledAt.UnixMilli(),
	)
	if c.onForkHealEvent != nil {
		c.onForkHealEvent(ForkHealAuditSummary{
			GroupID:      event.GroupID,
			TraceID:      traceID,
			Stage:        "fork_heal_started",
			WinnerPeerID: event.RemotePeer.String(),
			WinnerEpoch:  event.RemoteEpoch,
			NewEpoch:     localEpoch,
		})
	}

	go c.runHeal(c.ctx, traceID, event, scheduledAt)
}

// ResetSyncRetryAttempts resets the catch-up sync retry counter to 0.
func (c *Coordinator) ResetSyncRetryAttempts() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncRetryAttempts = 0
	c.deferredForkEvent = nil
}

// IncrementSyncRetryAttempts increments the retry counter and returns the new value.
func (c *Coordinator) IncrementSyncRetryAttempts() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncRetryAttempts++
	return c.syncRetryAttempts
}

// GetSyncRetryAttempts returns the current retry counter value.
func (c *Coordinator) GetSyncRetryAttempts() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.syncRetryAttempts
}

// TriggerDeferredHeal triggers the deferred fork heal event if present, bypassing retry checks.
func (c *Coordinator) TriggerDeferredHeal() {
	c.mu.Lock()
	if c.deferredForkEvent == nil {
		c.mu.Unlock()
		return
	}
	event := c.deferredForkEvent
	c.deferredForkEvent = nil

	traceID := newTraceID()
	localEpoch := c.epoch
	scheduledAt := c.clock.Now()

	if !c.healing.CompareAndSwap(false, true) {
		c.mu.Unlock()
		slog.Info("fork_heal/skipped_already_running",
			"trace_id", traceID,
			"group", event.GroupID,
			"local_epoch", localEpoch,
			"winner_peer", event.RemotePeer.String(),
			"winner_epoch", event.RemoteEpoch,
		)
		return
	}

	c.metrics.IncrForkHealingsAttempted()

	slog.Info("fork_heal/scheduled_deferred_after_retry",
		"trace_id", traceID,
		"group", event.GroupID,
		"local_epoch", localEpoch,
		"winner_peer", event.RemotePeer.String(),
		"winner_epoch", event.RemoteEpoch,
	)
	if c.onForkHealEvent != nil {
		c.onForkHealEvent(ForkHealAuditSummary{
			GroupID:      event.GroupID,
			TraceID:      traceID,
			Stage:        "fork_heal_started",
			WinnerPeerID: event.RemotePeer.String(),
			WinnerEpoch:  event.RemoteEpoch,
			NewEpoch:     localEpoch,
		})
	}
	c.mu.Unlock()

	go c.runHeal(c.ctx, traceID, event, scheduledAt)
}

// runHeal executes the fork-heal pipeline. It runs on its own goroutine and is
// guaranteed to release c.healing on exit.
func (c *Coordinator) runHeal(ctx context.Context, traceID string, event *ForkEvent, scheduledAt time.Time) {
	defer c.healing.Store(false)
	if ctx == nil {
		ctx = context.Background()
	}

	startedAt := c.clock.Now()
	slog.Info("fork_heal/started",
		"trace_id", traceID,
		"group", event.GroupID,
		"queued_ms", startedAt.Sub(scheduledAt).Milliseconds(),
	)
	c.recordForkHealAudit(traceID, event.GroupID, "started", "completed", startedAt, startedAt.Sub(scheduledAt).Milliseconds(), "")

	if ctx.Err() != nil {
		slog.Info("fork_heal/aborted",
			"trace_id", traceID,
			"group", event.GroupID,
			"reason", ctx.Err().Error(),
			"duration_ms", c.clock.Now().Sub(startedAt).Milliseconds(),
		)
		return
	}

	// M5: 1. Khởi tạo / Tìm kiếm Job Fork Healing bền vững
	job, err := c.storage.GetActiveForkHealingJob(event.GroupID)
	if err != nil {
		slog.Error("fork_heal/db_get_job_failed", "group", event.GroupID, "err", err)
	}
	if job == nil {
		losingBranchID := hex.EncodeToString(c.lastCommitHash)
		winningBranchID := hex.EncodeToString(event.RemoteAnnounce.CommitHash)

		job = &ForkHealingJob{
			JobID:             fmt.Sprintf("job-%s-%d", event.GroupID, startedAt.UnixMilli()),
			GroupID:           event.GroupID,
			TraceID:           traceID,
			Status:            "INITIATED",
			LosingBranchID:    losingBranchID,
			WinningBranchID:   winningBranchID,
			ForkBaseEpoch:     c.epoch,
			LosingEpoch:       c.epoch,
			WinningEpoch:      event.RemoteEpoch,
			LosingTreeHash:    c.treeHash,
			WinningTreeHash:   event.RemoteAnnounce.TreeHash,
			WinningCommitHash: event.RemoteAnnounce.CommitHash,
			WinnerPeerID:      event.RemotePeer.String(),
			CreatedAtMs:       startedAt.UnixMilli(),
			UpdatedAtMs:       startedAt.UnixMilli(),
		}
		if err := c.storage.SaveForkHealingJob(job); err != nil {
			slog.Error("fork_heal/db_save_job_failed", "group", event.GroupID, "err", err)
		}
	}

	storageKey := deriveStorageKey(c.signingKey)

	// M5: 2. Freeze apply & user sends, gossip append-only
	c.SetOperationalMode(ModeFrozenForApply)
	slog.Info("fork_heal/frozen_for_apply", "group", event.GroupID)
	defer func() {
		c.SetOperationalMode(ModeLive)
		slog.Info("fork_heal/live_unfrozen", "group", event.GroupID)
	}()

	// M5: 3. Snapshot các events thuộc partition window & AES-GCM Sealing cục bộ
	if job.Status == "INITIATED" {
		stepStart := c.clock.Now()
		c.recordForkHealAudit(traceID, event.GroupID, "snapshot_orphan", "started", stepStart, 0, "")

		losingBranchID := job.LosingBranchID

		// A. Snapshot các own messages đã apply thành công trong losing branch partition window
		// Using 0 as startMs to capture all orphaned messages that were locally applied but not committed globally
		ownMsgs, err := c.storage.GetMessagesByOwnerInRange(c.groupID, c.localID.String(), 0, startedAt.UnixMilli())
		if err == nil {
			for _, msg := range ownMsgs {
				sealedPayload, nonce, sealErr := sealPayload(msg.Content, storageKey)
				if sealErr == nil {
					h := sha256.Sum256(msg.Content)
					appEv := &ApplicationEvent{
						EventID:          hex.EncodeToString(msg.EnvelopeHash),
						JobID:            job.JobID,
						GroupID:          event.GroupID,
						OriginalBranchID: losingBranchID,
						OriginalEpoch:    msg.Epoch,
						AuthorID:         c.localID.String(),
						EnvelopeHash:     msg.EnvelopeHash,
						PayloadSealed:    sealedPayload,
						PayloadHash:      h[:],
						SealKeyID:        "local_node_key",
						SealNonce:        nonce,
						HlcWallTimeMs:    msg.Timestamp.WallTimeMs,
						HlcCounter:       msg.Timestamp.Counter,
						HlcNodeID:        msg.Timestamp.NodeID,
						Status:           "ORPHANED_OWN",
						CreatedAtMs:      c.clock.Now().UnixMilli(),
						UpdatedAtMs:      c.clock.Now().UnixMilli(),
					}
					_ = c.storage.SaveApplicationEvent(appEv)
				}
			}
		}

		// B. Snapshot các unapplied envelopes trong log
		pendingEnvs, err := c.storage.GetPendingEnvelopes(event.GroupID, 1000)
		if err == nil {
			for _, record := range pendingEnvs {
				// M5: Removed PartitionStartedAt filter because ANY unapplied envelope is an orphan when state-swapping

				if record.MsgType == MsgApplication {
					var env Envelope
					if err := json.Unmarshal(record.Envelope, &env); err == nil {
						var appMsg ApplicationMsg
						if err := json.Unmarshal(env.Payload, &appMsg); err == nil {
							c.mu.Lock()
							plaintext, _, decErr := c.mls.DecryptMessage(ctx, c.groupState, appMsg.Ciphertext)
							c.mu.Unlock()

							var status string
							var sealedPayload, nonce []byte
							var pHash []byte

							if decErr != nil {
								status = "UNRECOVERABLE"
								pHash = record.EnvelopeHash
							} else {
								h := sha256.Sum256(plaintext)
								pHash = h[:]

								if env.From == c.localID.String() {
									status = "ORPHANED_OWN"
									sealedPayload, nonce, _ = sealPayload(plaintext, storageKey)
								} else {
									status = "WAITING_AUTHOR_REPLAY"
								}
							}

							appEv := &ApplicationEvent{
								EventID:          hex.EncodeToString(record.EnvelopeHash),
								JobID:            job.JobID,
								GroupID:          event.GroupID,
								OriginalBranchID: losingBranchID,
								OriginalEpoch:    record.Epoch,
								AuthorID:         env.From,
								EnvelopeHash:     record.EnvelopeHash,
								PayloadSealed:    sealedPayload,
								PayloadHash:      pHash,
								SealKeyID:        "local_node_key",
								SealNonce:        nonce,
								HlcWallTimeMs:    record.Timestamp.WallTimeMs,
								HlcCounter:       record.Timestamp.Counter,
								HlcNodeID:        record.Timestamp.NodeID,
								Status:           status,
								CreatedAtMs:      c.clock.Now().UnixMilli(),
								UpdatedAtMs:      c.clock.Now().UnixMilli(),
							}
							_ = c.storage.SaveApplicationEvent(appEv)
						}
					}
				}
			}
		}

		job.Status = "SNAPSHOT_CREATED"
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		_ = c.storage.SaveForkHealingJob(job)
		c.recordForkHealAudit(traceID, event.GroupID, "snapshot_orphan", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
	}

	// M5: 4. Fetch GroupInfo & External Join
	var newEpoch uint64
	var newState, newTreeHash []byte

	if job.Status == "SNAPSHOT_CREATED" {
		stepStart := c.clock.Now()
		c.recordForkHealAudit(traceID, event.GroupID, "proposal_join_generation", "started", stepStart, 0, "")

		// 1. Generate a fresh KeyPackage via Rust (using our unique signing key)
		kp, kpPriv, err := c.mls.GenerateKeyPackage(ctx, c.signingKey)
		if err != nil {
			c.recordForkHealAudit(traceID, event.GroupID, "proposal_join_generation", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
			c.logHealFailed(traceID, event, startedAt, scheduledAt, "proposal_join_generation", err)
			return
		}

		// 2. Wrap the KeyPackage in a ProposalJoin message
		h := sha256.Sum256(kp)
		kpHash := h[:]

		msg := ProposalMsg{
			ProposalType:   ProposalJoin,
			Data:           kp,
			TargetPeerID:   c.localID.String(),
			TargetIdentity: c.localIdentity,
			KeyPackageHash: kpHash,
			OperationID:    job.JobID,
		}

		// 3. Drop current MlsGroup state from Go memory (but keep SQLite history intact)
		c.mu.Lock()
		c.groupState = nil
		c.treeHash = nil
		// Build envelope under lock to safely access c.epoch, groupID, localID, hlc
		envBytes := c.buildEnvelopeWithTimestampLocked(MsgProposal, msg, c.hlc.Now())
		c.mu.Unlock()

		// Publish outside lock to prevent re-entrant deadlock from synchronous transport callbacks
		if len(envBytes) > 0 {
			c.publishPreparedEnvelopeLocked(MsgProposal, envBytes)
		}

		// 4. Update the persistent healing job state to PROPOSAL_SENT, cache the private key bundle
		job.Status = "PROPOSAL_SENT"
		job.PendingBundlePrivate = kpPriv
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		if saveErr := c.storage.SaveForkHealingJob(job); saveErr != nil {
			slog.Error("fork_heal/db_save_job_failed", "group", event.GroupID, "err", saveErr)
		}

		c.recordForkHealAudit(traceID, event.GroupID, "proposal_join_broadcast", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
		slog.Info("fork_heal/proposal_sent", "group", event.GroupID, "job", job.JobID, "node", c.localID)

		// 5. Suspend goroutine and await Welcome message signal (prevents early return mode leaks)
		// Retry up to 3 times: if the Token Holder was healing (groupState nil) when the
		// initial ProposalJoin arrived, a failover will elect a new Token Holder. Re-broadcasting
		// gives the new Token Holder a chance to receive and transmute the ProposalJoin.
		const maxProposalJoinRetries = 3
		for attempt := 1; attempt <= maxProposalJoinRetries; attempt++ {
			if attempt > 1 {
				slog.Info("fork_heal/proposal_join_retry", "group", event.GroupID, "attempt", attempt)
				c.mu.Lock()
				retryEnvBytes := c.buildEnvelopeWithTimestampLocked(MsgProposal, msg, c.hlc.Now())
				c.mu.Unlock()
				if len(retryEnvBytes) > 0 {
					c.publishPreparedEnvelopeLocked(MsgProposal, retryEnvBytes)
				}
			}
			select {
			case <-c.welcomeReceivedChan:
				slog.Info("fork_heal/welcome_received_signal", "group", event.GroupID)
				// Reload the job from DB to get the decrypted state and updated status (EXTERNAL_JOINED)
				if updatedJob, reloadErr := c.storage.GetActiveForkHealingJob(event.GroupID); reloadErr == nil && updatedJob != nil {
					job = updatedJob
				} else {
					slog.Error("fork_heal/reload_job_failed", "group", event.GroupID)
					c.logHealFailed(traceID, event, startedAt, scheduledAt, "reload_job", fmt.Errorf("failed to reload job from DB after welcome signal"))
					return
				}
			case <-ctx.Done():
				slog.Warn("fork_heal/awaiting_welcome_cancelled", "group", event.GroupID)
				return
			case <-c.clock.After(c.cfg.MLSOperationTimeout):
				if attempt < maxProposalJoinRetries {
					slog.Warn("fork_heal/awaiting_welcome_timeout_retry", "group", event.GroupID, "attempt", attempt)
					continue
				}
				slog.Warn("fork_heal/awaiting_welcome_timeout", "group", event.GroupID)
				c.logHealFailed(traceID, event, startedAt, scheduledAt, "awaiting_welcome", fmt.Errorf("timeout waiting for Welcome message"))
				return
			}
			break
		}
	}

	// M5: 5. Swap Group State & Crypto-shredding keys (DB Transaction Boundary)
	if job.Status == "EXTERNAL_JOINED" {
		stepStart := c.clock.Now()
		c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "started", stepStart, 0, "")

		if len(newState) == 0 {
			if len(job.PendingGroupState) == 0 {
				err := fmt.Errorf("pending_group_state is missing at EXTERNAL_JOINED phase")
				c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
				c.logHealFailed(traceID, event, startedAt, scheduledAt, "state_swap", err)
				return
			}
			newState = job.PendingGroupState
			newEpoch = job.PendingEpoch
			newTreeHash = job.PendingTreeHash
		}

		if err := c.applyHealedState(newState, newTreeHash, newEpoch); err != nil {
			c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
			c.logHealFailed(traceID, event, startedAt, scheduledAt, "state_swap", err)
			return
		}

		job.Status = "STATE_SWAPPED"
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		_ = c.storage.SaveForkHealingJob(job)
		c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
	}

	// M5: 6. Autonomous Replay (Bidirectional Batched Replay)
	var replayedCount int
	if job.Status == "STATE_SWAPPED" {
		stepStart := c.clock.Now()
		c.recordForkHealAudit(traceID, event.GroupID, "replay_started", "started", stepStart, 0, "")

		// 1. Phục hồi và phát lại bất kỳ batched envelope nào còn kẹt trong outbound queue
		outboundList, err := c.storage.ListOutboundReplays(job.JobID)
		if err == nil {
			for _, outbound := range outboundList {
				if outbound.Status == "ENQUEUED" || outbound.Status == "FAILED" {
					evs, _ := c.storage.ListApplicationEvents(job.JobID)
					var matchEvs []*ApplicationEvent
					for _, ev := range evs {
						if ev.ReplayOperationID == outbound.ReplayOperationID {
							matchEvs = append(matchEvs, ev)
						}
					}
					if len(matchEvs) > 0 {
						if err := c.broadcastBatchedOutboundReplay(outbound, matchEvs); err == nil {
							replayedCount += len(matchEvs)
						}
					}
				}
			}
		}

		// 2. Gom cụm và phát lại các orphan events của mình
		replayedCount += c.batchAndReplayOutbox(ctx, job.JobID, job.GroupID)

		job.Status = "LOCAL_COMPLETE"
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		_ = c.storage.SaveForkHealingJob(job)
		c.recordForkHealAudit(traceID, event.GroupID, "replay_completed", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
	}

	// M5: 7. Shredding sealed payloads & transition CLEANED
	if job.Status == "LOCAL_COMPLETE" {
		_ = c.storage.ClearSealedPayloads(job.JobID)

		job.Status = "CLEANED"
		job.CompletedAtMs = c.clock.Now().UnixMilli()
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		_ = c.storage.SaveForkHealingJob(job)

		completedAt := c.clock.Now()
		c.metrics.IncrForkHealingsSucceeded()
		c.metrics.RecordExternalJoin(completedAt.Sub(startedAt))
		slog.Info("fork_heal/completed",
			"trace_id", traceID,
			"group", event.GroupID,
			"outcome", "success",
			"new_epoch", job.PendingEpoch,
			"duration_ms", completedAt.Sub(startedAt).Milliseconds(),
		)

		c.recordForkHealEvent(&ForkHealEventRecord{
			TraceID:              traceID,
			GroupID:              event.GroupID,
			WinnerPeerID:         event.RemotePeer.String(),
			WinnerEpoch:          event.RemoteEpoch,
			NewEpoch:             job.PendingEpoch,
			Outcome:              "success",
			PartitionStartedAtMs: event.PartitionStartedAt.UnixMilli(),
			ScheduledAtMs:        scheduledAt.UnixMilli(),
			StartedAtMs:          startedAt.UnixMilli(),
			CompletedAtMs:        completedAt.UnixMilli(),
			DurationMs:           completedAt.Sub(startedAt).Milliseconds(),
			TotalMs:              completedAt.Sub(scheduledAt).Milliseconds(),
			ReplayedMessageCount: replayedCount,
		})

		if c.onForkHealEvent != nil {
			c.onForkHealEvent(ForkHealAuditSummary{
				GroupID:              event.GroupID,
				TraceID:              traceID,
				Stage:                "fork_heal_completed",
				WinnerPeerID:         event.RemotePeer.String(),
				WinnerEpoch:          event.RemoteAnnounce.Epoch,
				NewEpoch:             job.PendingEpoch,
				ReplayedMessageCount: replayedCount,
			})
		}
	}
}

func (c *Coordinator) broadcastOutboundReplay(outbound *OutboundReplay, ev *ApplicationEvent) error {
	c.mu.Lock()
	c.appendOfflineEnvelopeLocked(outbound.ReplayEnvelope)
	c.publishPreparedEnvelopeLocked(MsgApplication, outbound.ReplayEnvelope)
	c.mu.Unlock()

	outbound.Status = "BROADCASTED"
	outbound.UpdatedAtMs = c.clock.Now().UnixMilli()
	if err := c.storage.SaveOutboundReplay(outbound); err != nil {
		return fmt.Errorf("save outbound replay broadcasted: %w", err)
	}

	ev.Status = "REPLAYED"
	ev.ReplayedAtMs = c.clock.Now().UnixMilli()
	if err := c.storage.SaveApplicationEvent(ev); err != nil {
		return fmt.Errorf("save application event replayed: %w", err)
	}

	// Mark the original stored message as replayed so the frontend can
	// suppress it once the re-broadcast copy is received and stored.
	if len(ev.EnvelopeHash) > 0 {
		now := c.clock.Now()
		if mErr := c.storage.MarkMessageReplayed(c.groupID, ev.EnvelopeHash, now); mErr != nil {
			slog.Warn("fork_heal/mark_replayed_failed", "group", c.groupID, "err", mErr)
		}
	}

	return nil
}

func phaseBeforeStateSwapped(status string) bool {
	switch status {
	case "INITIATED", "FROZEN_FOR_APPLY", "SNAPSHOT_CREATED", "EXTERNAL_JOINED":
		return true
	default:
		return false
	}
}

func (c *Coordinator) isAlreadyOnWinningBranch(job *ForkHealingJob) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If the job has not even reached EXTERNAL_JOINED state, we cannot be on the winning branch yet
	if job.Status == "INITIATED" || job.Status == "SNAPSHOT_CREATED" {
		return false
	}

	// 1. If local epoch is equal to the expected post-swap epoch, tree hash must match
	if job.PendingEpoch > 0 && c.epoch == job.PendingEpoch {
		return bytes.Equal(c.treeHash, job.PendingTreeHash)
	}

	// 2. If local epoch is equal to the winning branch's epoch, tree hash must match
	if c.epoch == job.WinningEpoch {
		return bytes.Equal(c.treeHash, job.WinningTreeHash)
	}

	return false
}

func (c *Coordinator) resumeForkHealingJob(job *ForkHealingJob) {
	slog.Info("fork_heal/resume_triggered", "group", job.GroupID, "status", job.Status)

	winnerPeer, err := peer.Decode(job.WinnerPeerID)
	if err != nil {
		winnerPeer = peer.ID(job.WinnerPeerID)
	}

	// Self-healing detection: Nếu group state cục bộ thực chất đã swap thành công trước khi crash
	if c.isAlreadyOnWinningBranch(job) && phaseBeforeStateSwapped(job.Status) {
		slog.Info("fork_heal/resume_detect_already_swapped", "group", job.GroupID, "epoch", c.epoch)
		job.Status = "STATE_SWAPPED"
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		_ = c.storage.SaveForkHealingJob(job)
	}

	if !c.healing.CompareAndSwap(false, true) {
		slog.Warn("fork_heal/resume_skipped_already_healing", "group", job.GroupID)
		return
	}

	event := &ForkEvent{
		GroupID:            job.GroupID,
		RemotePeer:         winnerPeer,
		RemoteEpoch:        job.WinningEpoch,
		NeedExternalJoin:   true,
		WinnerPeers:        []peer.ID{winnerPeer},
		PartitionStartedAt: time.UnixMilli(job.CreatedAtMs),
		RemoteAnnounce: GroupStateAnnouncement{
			TreeHash: job.WinningTreeHash,
			Epoch:    job.WinningEpoch,
		},
	}

	go c.runHeal(c.ctx, job.TraceID, event, time.UnixMilli(job.CreatedAtMs))
}

// ProcessWelcomeIfWaiting is called by the application layer when a Welcome message
// is received. If the coordinator is waiting for a Welcome to heal a fork, it
// processes it and resumes the healing job.
func (c *Coordinator) ProcessWelcomeIfWaiting(ctx context.Context, welcomeBytes []byte) bool {
	c.mu.Lock()
	job, err := c.storage.GetActiveForkHealingJob(c.groupID)
	if err != nil || job == nil || job.Status != "PROPOSAL_SENT" {
		c.mu.Unlock()
		return false
	}

	if len(job.PendingBundlePrivate) == 0 {
		c.mu.Unlock()
		return false
	}

	// Copy immutable properties needed for MLS call to local variables and release c.mu
	// to prevent blocking other goroutines during the heavy cryptographic MLS/gRPC call.
	signingKey := c.signingKey
	maxPastEpochs := c.cfg.GetMaxPastEpochs()
	pendingPrivate := job.PendingBundlePrivate
	c.mu.Unlock()

	groupState, treeHash, epoch, err := c.mls.ProcessWelcome(ctx, welcomeBytes, signingKey, pendingPrivate, maxPastEpochs)
	if err != nil {
		slog.Warn("fork_heal/process_welcome_failed", "group", c.groupID, "err", err)
		return false
	}

	c.mu.Lock()
	// Re-verify the active job status to protect against concurrent welcome processing races
	job, err = c.storage.GetActiveForkHealingJob(c.groupID)
	if err != nil || job == nil || job.Status != "PROPOSAL_SENT" {
		c.mu.Unlock()
		return false
	}

	job.Status = "EXTERNAL_JOINED"
	job.WinningEpoch = epoch
	job.WinningTreeHash = treeHash
	job.PendingGroupState = groupState
	job.PendingEpoch = epoch
	job.PendingTreeHash = treeHash
	job.UpdatedAtMs = c.clock.Now().UnixMilli()
	_ = c.storage.SaveForkHealingJob(job)

	c.mu.Unlock()

	// Signal the waiting runHeal goroutine to proceed with state swap.
	// runHeal reloads the job from DB and finds status=EXTERNAL_JOINED.
	select {
	case c.welcomeReceivedChan <- struct{}{}:
	default:
	}

	return true
}

func deriveStorageKey(signingKey []byte) []byte {
	h := sha256.Sum256(signingKey)
	return h[:]
}

func sealPayload(plaintext, key []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = aesgcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func openPayload(ciphertext, nonce, key []byte) (plaintext []byte, err error) {
	if len(ciphertext) == 0 || len(nonce) == 0 {
		return nil, fmt.Errorf("openPayload: empty ciphertext or nonce")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err = aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func (c *Coordinator) fetchGroupInfoForHeal(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
	if c.groupInfoFetch == nil {
		return nil, fmt.Errorf("group-info fetcher not configured")
	}
	gi, err := c.groupInfoFetch(ctx, remote, groupID, withRatchetTree)
	if err != nil {
		return nil, err
	}
	if gi == nil {
		return nil, fmt.Errorf("empty group-info response")
	}
	if len(gi.GroupInfo) == 0 {
		return nil, fmt.Errorf("group-info response missing payload")
	}
	return gi, nil
}

func (c *Coordinator) applyHealedState(newState, newTreeHash []byte, newEpoch uint64) error {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	prevRec, err := c.storage.GetGroupRecord(c.groupID)
	if err != nil && !errors.Is(err, ErrGroupNotFound) {
		return fmt.Errorf("load group record: %w", err)
	}

	role := RoleMember
	groupType := ""
	categoryID := ""
	createdAt := now
	if prevRec != nil {
		if prevRec.MyRole != "" {
			role = prevRec.MyRole
		}
		groupType = prevRec.GroupType
		categoryID = prevRec.CategoryID
		if !prevRec.CreatedAt.IsZero() {
			createdAt = prevRec.CreatedAt
		}
	}
	if err := c.storage.SaveGroupRecord(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      newEpoch,
		TreeHash:   newTreeHash,
		MyRole:     role,
		GroupType:  groupType,
		CategoryID: categoryID,
		CreatedAt:  createdAt,
		UpdatedAt:  now,
	}); err != nil {
		return fmt.Errorf("persist healed group state: %w", err)
	}

	c.groupState = append([]byte(nil), newState...)
	c.treeHash = append([]byte(nil), newTreeHash...)
	c.epoch = newEpoch
	c.epochTracker = NewEpochTracker(newEpoch, newTreeHash)
	c.singleWriter = NewSingleWriter(c.activeView, c.localID, newEpoch, c.cfg)
	c.singleWriter.SetAuthorizedCommitters(c.groupID, c.authorizedCommitters)

	commitHash := hashCommitData(nil)
	c.lastCommitHash = copyBytes(commitHash)
	c.forkDetector.Reset()
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    newTreeHash,
		MemberCount: c.activeView.Size(),
		Epoch:       newEpoch,
		CommitHash:  commitHash,
	})
	if err := c.persistCoordStateLocked(); err != nil {
		return fmt.Errorf("persist healed coordination state: %w", err)
	}
	if c.onEpochChange != nil {
		c.onEpochChange(newEpoch)
	}
	return nil
}

func (c *Coordinator) logHealFailed(traceID string, event *ForkEvent, startedAt, scheduledAt time.Time, step string, err error) {
	completedAt := c.clock.Now()
	slog.Error("fork_heal/failed",
		"trace_id", traceID,
		"group", event.GroupID,
		"step", step,
		"error", err,
		"duration_ms", completedAt.Sub(startedAt).Milliseconds(),
		"total_ms", completedAt.Sub(scheduledAt).Milliseconds(),
	)
	slog.Info("fork_heal/aggregate",
		"trace_id", traceID,
		"group", event.GroupID,
		"winner_peer", event.RemotePeer.String(),
		"winner_epoch", event.RemoteEpoch,
		"outcome", "failed",
		"failed_step", step,
		"partition_window_ms", completedAt.Sub(event.PartitionStartedAt).Milliseconds(),
		"total_ms", completedAt.Sub(scheduledAt).Milliseconds(),
	)
	c.recordForkHealEvent(&ForkHealEventRecord{
		TraceID:              traceID,
		GroupID:              event.GroupID,
		WinnerPeerID:         event.RemotePeer.String(),
		WinnerEpoch:          event.RemoteEpoch,
		NewEpoch:             c.CurrentEpoch(),
		Outcome:              "failed",
		FailedStep:           step,
		WinnerTreeHash:       append([]byte(nil), event.RemoteAnnounce.TreeHash...),
		NewTreeHash:          append([]byte(nil), c.GetTreeHash()...),
		PartitionStartedAtMs: event.PartitionStartedAt.UnixMilli(),
		ScheduledAtMs:        scheduledAt.UnixMilli(),
		StartedAtMs:          startedAt.UnixMilli(),
		CompletedAtMs:        completedAt.UnixMilli(),
		DurationMs:           completedAt.Sub(startedAt).Milliseconds(),
		TotalMs:              completedAt.Sub(scheduledAt).Milliseconds(),
		ReplayedMessageCount: 0,
	})
	if c.onForkHealEvent != nil {
		c.onForkHealEvent(ForkHealAuditSummary{
			GroupID:      event.GroupID,
			TraceID:      traceID,
			Stage:        "fork_heal_failed",
			WinnerPeerID: event.RemotePeer.String(),
			WinnerEpoch:  event.RemoteEpoch,
			NewEpoch:     c.CurrentEpoch(),
			FailedStep:   step,
		})
	}
}

func (c *Coordinator) recordForkHealAudit(traceID, groupID, step, status string, ts time.Time, durationMs int64, errMsg string) {
	if traceID == "" || groupID == "" || step == "" || status == "" {
		return
	}
	if err := c.storage.RecordForkHealAudit(&ForkHealAuditRecord{
		TraceID:     traceID,
		GroupID:     groupID,
		Step:        step,
		Status:      status,
		TimestampMs: ts.UnixMilli(),
		DurationMs:  durationMs,
		Error:       errMsg,
	}); err != nil {
		slog.Warn("fork_heal/audit_persist_failed", "trace_id", traceID, "group", groupID, "step", step, "status", status, "err", err)
	}
}

func (c *Coordinator) recordForkHealEvent(event *ForkHealEventRecord) {
	if event == nil {
		return
	}
	if err := c.storage.RecordForkHealEvent(event); err != nil {
		slog.Warn("fork_heal/event_persist_failed", "trace_id", event.TraceID, "group", event.GroupID, "err", err)
	}
}

func (c *Coordinator) emitPendingOperationAuditLocked(summary PendingOperationAuditSummary) {
	if c.onPendingOperation == nil {
		return
	}
	cb := c.onPendingOperation
	go cb(summary)
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (c *Coordinator) collectReplayWindowMessages(partitionStart, healStartedAt time.Time) ([]*StoredMessage, error) {
	if partitionStart.IsZero() {
		return nil, nil
	}
	startMs := partitionStart.UnixMilli()
	endMs := healStartedAt.UnixMilli()
	msgs, err := c.storage.GetMessagesByOwnerInRange(c.groupID, c.localID.String(), startMs, endMs)
	if err != nil {
		return nil, fmt.Errorf("GetMessagesByOwnerInRange: %w", err)
	}
	return msgs, nil
}

func (c *Coordinator) replayWindowMessages(ctx context.Context, window []*StoredMessage) (int, error) {
	if len(window) == 0 {
		return 0, nil
	}
	throttle := time.Duration(c.cfg.ReplayThrottleMs) * time.Millisecond
	replayed := 0
	for i, msg := range window {
		if ctx.Err() != nil {
			return replayed, ctx.Err()
		}
		c.mu.Lock()
		ciphertext, newState, err := c.mls.EncryptMessage(ctx, c.groupState, msg.Content)
		if err != nil {
			c.mu.Unlock()
			return replayed, fmt.Errorf("EncryptMessage replay idx=%d: %w", i, err)
		}
		ts := c.hlc.Now()
		wire := c.buildEnvelopeWithTimestampLocked(MsgApplication, ApplicationMsg{Ciphertext: ciphertext}, ts)
		if len(wire) == 0 {
			c.mu.Unlock()
			return replayed, fmt.Errorf("encode replay application envelope idx=%d", i)
		}
		c.groupState = newState
		c.publishPreparedEnvelopeLocked(MsgApplication, wire)
		c.appendOfflineEnvelopeLocked(wire)
		now := c.clock.Now()
		c.mu.Unlock()

		// Mark the original stored message as replayed so the frontend can
		// suppress it once the re-broadcast copy is received and stored.
		if len(msg.EnvelopeHash) > 0 {
			if mErr := c.storage.MarkMessageReplayed(c.groupID, msg.EnvelopeHash, now); mErr != nil {
				slog.Warn("fork_heal/mark_replayed_failed", "group", c.groupID, "err", mErr)
			}
		}
		replayed++

		if throttle > 0 && i < len(window)-1 {
			select {
			case <-ctx.Done():
				return replayed, ctx.Err()
			case <-c.clock.After(throttle):
			}
		}
	}
	c.mu.Lock()
	err := c.saveCurrentGroupStateLocked(c.clock.Now())
	c.mu.Unlock()
	if err != nil {
		return replayed, err
	}
	return replayed, nil
}

func (c *Coordinator) saveCurrentGroupStateLocked(now time.Time) error {
	prevRec, err := c.storage.GetGroupRecord(c.groupID)
	if err != nil && !errors.Is(err, ErrGroupNotFound) {
		return fmt.Errorf("load group record: %w", err)
	}
	role := RoleMember
	groupType := ""
	categoryID := ""
	createdAt := now
	if prevRec != nil {
		if prevRec.MyRole != "" {
			role = prevRec.MyRole
		}
		groupType = prevRec.GroupType
		categoryID = prevRec.CategoryID
		if !prevRec.CreatedAt.IsZero() {
			createdAt = prevRec.CreatedAt
		}
	}
	return c.storage.SaveGroupRecord(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: c.groupState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		MyRole:     role,
		GroupType:  groupType,
		CategoryID: categoryID,
		CreatedAt:  createdAt,
		UpdatedAt:  now,
	})
}

// newTraceID returns a short hex identifier suitable for tagging log lines
// belonging to a single fork-heal pipeline. 8 hex chars (32 bits) is enough
// for human readability and disambiguating concurrent heals across nodes.
func newTraceID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}

func (c *Coordinator) reconcileOperationsAfterCommitLocked(commit CommitMsg) {
	ops, err := c.storage.ListPendingOperations(c.groupID)
	if err != nil {
		slog.Error("Failed to list pending operations for reconcile", "group", c.groupID, "error", err)
		return
	}

	for _, op := range ops {
		if op.Status != "PENDING" && op.Status != "PROPOSED" {
			continue
		}

		if op.OpType == "ADD_MEMBER" {
			for _, d := range commit.AddDeliveries {
				if d.OperationID == op.OperationID || d.TargetPeerID == *op.TargetMemberID {
					if d.OperationID == op.OperationID {
						op.Status = "COMMITTED"
					} else {
						op.Status = "SATISFIED_BY_OTHER"
					}
					op.UpdatedAt = c.clock.Now()
					_ = c.storage.SavePendingOperation(op)
					break
				}
			}
		} else if op.OpType == "REMOVE_MEMBER" {
			for _, p := range commit.IncludedProposals {
				if p.ProposalType == ProposalRemove && p.TargetPeerID == *op.TargetMemberID {
					if p.OperationID == op.OperationID {
						op.Status = "COMMITTED"
					} else {
						op.Status = "SATISFIED_BY_OTHER"
					}
					op.UpdatedAt = c.clock.Now()
					_ = c.storage.SavePendingOperation(op)
					break
				}
			}
		}
	}

	// M5: Convert unapplied pending application envelopes to ORPHANED_OWN
	storageKey := deriveStorageKey(c.signingKey)
	pendingEnvs, err := c.storage.GetPendingEnvelopes(c.groupID, 1000)
	if err == nil {
		for _, record := range pendingEnvs {
			if record.MsgType == MsgApplication {
				var env Envelope
				if err := json.Unmarshal(record.Envelope, &env); err == nil {
					if env.From == c.localID.String() && env.Epoch < c.epoch {
						var appMsg ApplicationMsg
						if err := json.Unmarshal(env.Payload, &appMsg); err == nil {
							// For unapplied envelopes, we could try to decrypt them here
							// but normally they are already applied if we are the winner.
						}
					}
				}
			}
		}
	}

	// Winner specific logic: Replay messages sent in the previous epoch
	ownMsgs, _ := c.storage.GetMessagesByOwnerInRange(c.groupID, c.localID.String(), 0, c.clock.Now().UnixMilli())
	for _, m := range ownMsgs {
		if m.Epoch == c.epoch-1 {
			// Seal payload and add to ApplicationEvents
			sealedPayload, nonce, sealErr := sealPayload(m.Content, storageKey)
			if sealErr == nil {
				h := sha256.Sum256(m.Content)
				appEv := &ApplicationEvent{
					EventID:          hex.EncodeToString(m.EnvelopeHash),
					JobID:            "COMMIT-RECONCILE-" + c.groupID,
					GroupID:          c.groupID,
					OriginalBranchID: "",
					OriginalEpoch:    m.Epoch,
					AuthorID:         c.localID.String(),
					EnvelopeHash:     m.EnvelopeHash,
					PayloadSealed:    sealedPayload,
					PayloadHash:      h[:],
					SealKeyID:        "local_node_key",
					SealNonce:        nonce,
					HlcWallTimeMs:    m.Timestamp.WallTimeMs,
					HlcCounter:       m.Timestamp.Counter,
					HlcNodeID:        m.Timestamp.NodeID,
					Status:           "ORPHANED_OWN",
					CreatedAtMs:      c.clock.Now().UnixMilli(),
					UpdatedAtMs:      c.clock.Now().UnixMilli(),
				}
				_ = c.storage.SaveApplicationEvent(appEv)
			}
		}
	}
}
func (c *Coordinator) reconcileAndRebaseOperationsLocked() {
	ops, err := c.storage.ListPendingOperations(c.groupID)
	if err != nil {
		slog.Error("Failed to list pending operations for reconcile", "group", c.groupID, "error", err)
		return
	}

	for _, op := range ops {
		if op.Status != "PENDING" && op.Status != "PROPOSED" {
			continue
		}

		if op.ExpiresAt != nil && *op.ExpiresAt > 0 && c.clock.Now().Unix() > *op.ExpiresAt {
			op.Status = "FAILED_EXPIRED"
			errStr := "operation TTL expired"
			op.LastError = &errStr
			op.UpdatedAt = c.clock.Now()
			_ = c.storage.SavePendingOperation(op)
			continue
		}

		// P0.1 Fix: use MLS HasMember for membership-based satisfied check.
		// ActiveView tracks liveness (online/offline), NOT MLS group membership.
		// An offline peer is still an MLS member; using ActiveView here would
		// incorrectly mark Remove(offlinePeer) as SATISFIED_BY_OTHER.
		satisfied := false
		if op.TargetMemberID != nil && *op.TargetMemberID != "" {
			opCtx, cancel := c.mlsOperationContext()
			isMember, err := c.mls.HasMember(opCtx, c.groupState, []byte(*op.TargetMemberID))
			cancel()
			if err == nil {
				if op.OpType == "ADD_MEMBER" && isMember {
					satisfied = true
				} else if op.OpType == "REMOVE_MEMBER" && !isMember {
					satisfied = true
				}
			} else {
				slog.Warn("HasMember check failed during reconcile; skipping satisfied check",
					"group", c.groupID, "opID", op.OperationID, "error", err)
			}
		}

		if satisfied {
			op.Status = "SATISFIED_BY_OTHER"
			op.UpdatedAt = c.clock.Now()
			_ = c.storage.SavePendingOperation(op)
			continue
		}

		if op.PreconditionEpoch != nil && *op.PreconditionEpoch < c.epoch {
			maxRetries := c.cfg.ApplicationDirectRetryLimit
			if maxRetries <= 0 {
				maxRetries = 5
			}
			if op.RetryCount >= maxRetries {
				op.Status = "FAILED_RETRY_EXHAUSTED"
				errStr := "max retries exceeded"
				op.LastError = &errStr
				op.UpdatedAt = c.clock.Now()
				_ = c.storage.SavePendingOperation(op)
				c.emitPendingOperationAuditLocked(PendingOperationAuditSummary{
					GroupID:      c.groupID,
					OperationID:  op.OperationID,
					OpType:       op.OpType,
					TargetPeerID: derefString(op.TargetMemberID),
					Stage:        "retry_exhausted",
					RetryCount:   op.RetryCount,
					CurrentEpoch: c.epoch,
					LastError:    errStr,
				})
				continue
			}

			op.RetryCount++
			prevPreconditionEpoch := uint64(0)
			if op.PreconditionEpoch != nil {
				prevPreconditionEpoch = *op.PreconditionEpoch
			}
			epochCopy := c.epoch
			op.PreconditionEpoch = &epochCopy
			op.UpdatedAt = c.clock.Now()

			slog.Info("Rebasing pending operation to new epoch",
				"group", c.groupID, "opID", op.OperationID, "type", op.OpType, "newEpoch", c.epoch, "retry", op.RetryCount)

			var reProposed bool
			var reProposeErr error

			if op.OpType == "ADD_MEMBER" {
				msg, buffered, err := c.createAndStoreLocalProposalLocked(ProposalAdd, op.SemanticPayload, BufferedProposal{
					OperationID:    op.OperationID,
					TargetPeerID:   *op.TargetMemberID,
					KeyPackageHash: append([]byte(nil), op.OperationHash...),
				})
				if err == nil {
					c.broadcastLocked(MsgProposal, msg)
					c.singleWriter.BufferProposal(buffered)
					if c.singleWriter.IsTokenHolder() {
						c.scheduleBatchCommitLocked()
					} else {
						c.scheduleFailoverTimerLocked()
					}
					h := sha256.Sum256(msg.Data)
					op.LatestProposalHash = h[:]
					op.Status = "PROPOSED"
					reProposed = true
				} else {
					reProposeErr = err
				}
			} else if op.OpType == "REMOVE_MEMBER" {
				msg, buffered, err := c.createAndStoreLocalProposalLocked(ProposalRemove, op.SemanticPayload, BufferedProposal{
					TargetPeerID: *op.TargetMemberID,
					OperationID:  op.OperationID,
				})
				if err == nil {
					c.broadcastLocked(MsgProposal, msg)
					c.singleWriter.BufferProposal(buffered)
					if c.singleWriter.IsTokenHolder() {
						c.scheduleBatchCommitLocked()
					} else {
						c.scheduleFailoverTimerLocked()
					}
					h := sha256.Sum256(msg.Data)
					op.LatestProposalHash = h[:]
					op.Status = "PROPOSED"
					reProposed = true
				} else {
					reProposeErr = err
				}
			}

			if reProposed {
				if dbOp, dbErr := c.storage.GetPendingOperation(op.OperationID); dbErr == nil && dbOp != nil {
					if dbOp.Status == "COMMITTED" || dbOp.Status == "SATISFIED_BY_OTHER" {
						op.Status = dbOp.Status
						op.UpdatedAt = dbOp.UpdatedAt
						op.LastError = dbOp.LastError
						_ = c.storage.SavePendingOperation(op)
						continue
					}
				}
				op.LastError = nil
				_ = c.storage.SavePendingOperation(op)
				c.emitPendingOperationAuditLocked(PendingOperationAuditSummary{
					GroupID:           c.groupID,
					OperationID:       op.OperationID,
					OpType:            op.OpType,
					TargetPeerID:      derefString(op.TargetMemberID),
					Stage:             "rebased",
					RetryCount:        op.RetryCount,
					CurrentEpoch:      c.epoch,
					PreconditionEpoch: prevPreconditionEpoch,
				})
			} else if reProposeErr != nil {
				errStr := reProposeErr.Error()
				op.LastError = &errStr
				slog.Warn("Failed to re-propose rebased operation", "opID", op.OperationID, "error", reProposeErr)
				_ = c.storage.SavePendingOperation(op)
				c.emitPendingOperationAuditLocked(PendingOperationAuditSummary{
					GroupID:           c.groupID,
					OperationID:       op.OperationID,
					OpType:            op.OpType,
					TargetPeerID:      derefString(op.TargetMemberID),
					Stage:             "rebase_failed",
					RetryCount:        op.RetryCount,
					CurrentEpoch:      c.epoch,
					PreconditionEpoch: prevPreconditionEpoch,
					LastError:         errStr,
				})
			}
		}
	}

	if c.singleWriter.IsTokenHolder() {
		c.scheduleBatchCommitLocked()
	}
}
