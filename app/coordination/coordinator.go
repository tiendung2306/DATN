package coordination

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	mu                  sync.Mutex
	groupState          []byte
	treeHash            []byte
	lastCommitHash      []byte
	historyHash         []byte            // R(E) = H(R(E-1) ∥ CommitHash(E))
	historyChain        map[uint64][]byte // epoch → R(epoch)
	epoch               uint64
	activeView          *ActiveView
	singleWriter        *SingleWriter
	epochTracker        *EpochTracker
	forkDetector        *ForkDetector
	proposalTimerChan   <-chan time.Time
	failoverTimerChan   <-chan time.Time
	lastTokenHolder     peer.ID
	startedAt           time.Time
	started             bool
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
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
		if len(cs.HistoryChain) > 0 {
			c.historyChain = cs.HistoryChain
			if h, ok := c.historyChain[c.epoch]; ok {
				c.historyHash = copyBytes(h)
			}
		}
	} else if !errors.Is(err, ErrGroupNotFound) {
		return fmt.Errorf("load coordination state: %w", err)
	}

	// Seed history chain if not already initialized (fresh group or joiner
	// that didn't receive an anchor). For epoch 0 creators, R(0) is
	// deterministic. For joiners, the anchor should have been seeded before
	// Start; if not, we fall back to initialHistoryHash so the node still
	// has a valid chain root.
	if c.historyChain == nil {
		c.historyChain = make(map[uint64][]byte)
	}
	if c.historyHash == nil {
		r0 := initialHistoryHash(c.groupID)
		c.historyHash = r0
		c.historyChain[c.epoch] = copyBytes(r0)
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
		HistoryHash: copyBytes(c.historyHash),
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
			c.wg.Add(1)
			go func() {
				defer c.wg.Done()
				c.resumeForkHealingJob(job)
			}()
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
