package coordination

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	GroupInfoFetcher GroupInfoFetchFunc

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

	onMessage       func(*StoredMessage)
	onEpochChange   func(uint64)
	onAccessLost    func(string, uint64, string)
	onEnvelope      func(MessageType, string, []byte)
	onAddCommitted  func(AddCommitDelivery, uint64, []byte)
	onPeerObserved  func(string, peer.ID, time.Time)
	groupInfoFetch  GroupInfoFetchFunc
	localIdentity   []byte

	// Mutable state (protected by mu)
	mu           sync.Mutex
	groupState   []byte
	treeHash     []byte
	epoch        uint64
	activeView   *ActiveView
	singleWriter *SingleWriter
	epochTracker *EpochTracker
	forkDetector *ForkDetector
	started      bool
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup

	// healing is set to 1 while a fork-heal goroutine is in flight. Manipulated
	// via atomic CAS so handleAnnounceLocked never blocks on a long heal and
	// duplicate fork events do not trigger overlapping heal pipelines.
	healing atomic.Bool
	// accessRevoked flips when local membership no longer exists in MLS state.
	// All local mutation APIs are blocked once this is true.
	accessRevoked bool
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
		cfg:            opts.Config,
		transport:      opts.Transport,
		clock:          opts.Clock,
		mls:            opts.MLS,
		storage:        opts.Storage,
		localID:        opts.LocalID,
		groupID:        opts.GroupID,
		signingKey:     opts.SigningKey,
		hlc:            NewHLC(opts.Clock, opts.LocalID.String()),
		metrics:        NewMetrics(),
		onMessage:      opts.OnMessage,
		onEpochChange:  opts.OnEpochChange,
		onAccessLost:   opts.OnAccessLost,
		onEnvelope:     opts.OnEnvelopeBroadcast,
		onAddCommitted: opts.OnAddCommitted,
		onPeerObserved: opts.OnPeerObserved,
		groupInfoFetch: opts.GroupInfoFetcher,
		localIdentity:  deriveIdentityFromSigningKey(opts.SigningKey),
	}

	c.activeView = NewActiveView(opts.Clock, opts.Config, opts.LocalID, nil)
	c.forkDetector = NewForkDetector()
	return c, nil
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

	state, treeHash, err := c.mls.CreateGroup(context.Background(), c.groupID, c.signingKey)
	if err != nil {
		return fmt.Errorf("CreateGroup: %w", err)
	}

	c.groupState = state
	c.epoch = 0
	c.treeHash = treeHash

	return c.storage.SaveGroupRecord(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: state,
		Epoch:      0,
		TreeHash:   treeHash,
		MyRole:     RoleCreator,
		CreatedAt:  c.clock.Now(),
		UpdatedAt:  c.clock.Now(),
	})
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

	c.epochTracker = NewEpochTracker(c.epoch, c.treeHash)
	c.singleWriter = NewSingleWriter(c.activeView, c.localID, c.epoch, c.cfg)
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    c.treeHash,
		MemberCount: c.activeView.Size(),
	})

	c.ctx, c.cancel = context.WithCancel(ctx)

	if err := c.transport.Subscribe(GroupTopic(c.groupID), c.handleRawMessage); err != nil {
		c.cancel()
		return fmt.Errorf("subscribe: %w", err)
	}

	c.started = true

	c.wg.Add(1)
	go func() { defer c.wg.Done(); c.heartbeatLoop(c.ctx) }()

	if c.cfg.AnnounceInterval > 0 {
		c.wg.Add(1)
		go func() { defer c.wg.Done(); c.announceLoop(c.ctx) }()
	}

	return nil
}

// Stop cancels all background work and unsubscribes from the group topic.
func (c *Coordinator) Stop() {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return
	}
	c.cancel()
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

	c.mu.Lock()
	defer c.mu.Unlock()

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
	}
}

func (c *Coordinator) handleHeartbeatLocked(from peer.ID) {
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
	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		return
	case ActionBufferFuture:
		raw, _ := json.Marshal(env)
		c.epochTracker.BufferFuture(env.Epoch, raw)
		return
	}

	var proposal ProposalMsg
	if err := json.Unmarshal(env.Payload, &proposal); err != nil {
		return
	}

	c.singleWriter.BufferProposal(BufferedProposal{
		Type:           proposal.ProposalType,
		Data:           proposal.Data,
		OperationID:    proposal.OperationID,
		TargetPeerID:   proposal.TargetPeerID,
		RequestID:      proposal.RequestID,
		GroupType:      proposal.GroupType,
		CategoryID:     proposal.CategoryID,
		KeyPackageHash: proposal.KeyPackageHash,
	})
	c.metrics.IncrProposalsReceived()

	if c.singleWriter.IsTokenHolder() {
		c.tryCommitLocked()
	}
}

func (c *Coordinator) handleCommitLocked(env *Envelope, wire []byte) bool {
	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		c.metrics.IncrDuplicateEpochDetected()
		return false
	case ActionBufferFuture:
		raw, _ := json.Marshal(env)
		c.epochTracker.BufferFuture(env.Epoch, raw)
		return false
	}

	var commit CommitMsg
	if err := json.Unmarshal(env.Payload, &commit); err != nil {
		return false
	}

	_, alreadyApplied := c.checkAppliedEnvelopeLocked(env, wire)
	if alreadyApplied {
		return false
	}

	start := c.clock.Now()

	newState, newTreeHash, err := c.mls.ProcessCommit(c.ctx, c.groupState, commit.CommitData)
	if err != nil {
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
	}, env.Type, wire, env.Timestamp)
	if err != nil {
		slog.Error("Failed to persist commit apply", "group", c.groupID, "error", err)
		return false
	}
	if !applied {
		return false
	}

	c.advanceEpochLocked(newState, nextEpoch, newTreeHash, commit.CommitData)
	c.updateLocalAccessRevocationLocked(newState, nextEpoch)
	c.metrics.RecordEpochFinalization(c.clock.Now().Sub(start))

	// Surface AddCommitDeliveries to the runtime so non-holder receivers can
	// transition their local group_add_operations row to "commit_observed".
	// Welcome bytes are intentionally NOT propagated here — only the node
	// that ran CreateCommit owns the ephemeral material to author them.
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
	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		slog.Warn("Rejected stale message", "group", c.groupID, "msgEpoch", env.Epoch, "currentEpoch", c.epoch)
		return false
	case ActionBufferFuture:
		slog.Info("Buffered future-epoch message", "group", c.groupID, "msgEpoch", env.Epoch)
		raw, _ := json.Marshal(env)
		c.epochTracker.BufferFuture(env.Epoch, raw)
		return false
	}

	var appMsg ApplicationMsg
	if err := json.Unmarshal(env.Payload, &appMsg); err != nil {
		return false
	}

	envelopeHash, alreadyApplied := c.checkAppliedEnvelopeLocked(env, wire)
	if alreadyApplied {
		return false
	}

	localTs := c.hlc.Update(env.Timestamp)
	if localTs.NodeID == "" {
		localTs.NodeID = c.localID.String()
	}

	plaintext, newState, err := c.mls.DecryptMessage(c.ctx, c.groupState, appMsg.Ciphertext)
	if err != nil {
		slog.Error("Failed to decrypt message", "group", c.groupID, "from", env.From, "error", err)
		return false
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
	}, msg, env.Type, wire, env.Timestamp)
	if err != nil {
		slog.Error("Failed to persist decrypted message", "group", c.groupID, "from", env.From, "error", err)
		return false
	}
	if !applied {
		return false
	}
	c.groupState = newState
	slog.Info("Message received", "group", c.groupID, "epoch", env.Epoch, "from", env.From, "ts", localTs.WallTimeMs)

	if c.onMessage != nil {
		c.onMessage(msg)
	}
	return true
}

// ─── Commit Logic ────────────────────────────────────────────────────────────

// tryCommitLocked drains a single homogeneous proposal batch and commits it.
// Mixed-type batching is rejected by the Rust engine; here we let
// SingleWriter return the next homogeneous run (Add / Remove / Update) so
// proposals never get silently dropped — remaining proposal types are wiped
// alongside the epoch advance because their underlying MLS proposal bytes
// were authored against the pre-commit epoch and can no longer be applied.
func (c *Coordinator) tryCommitLocked() {
	batch := c.singleWriter.DrainNextBatch()
	if len(batch) == 0 {
		return
	}

	rawProposals := make([][]byte, 0, len(batch))
	for i := range batch {
		rawProposals = append(rawProposals, batch[i].Data)
	}

	commitBytes, welcomeBytes, newState, newTreeHash, err := c.mls.CreateCommit(c.ctx, c.groupState, rawProposals)
	if err != nil {
		return
	}

	commitMsg := CommitMsg{
		CommitData:  commitBytes,
		NewTreeHash: newTreeHash,
	}

	// Surface routing metadata for ProposalAdd commits so observer nodes can
	// correlate the commit with their local group_add_operations rows.
	if batch[0].Type == ProposalAdd {
		commitMsg.AddDeliveries = buildAddDeliveriesFromBatch(batch, welcomeBytes)
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
		GroupState: newState,
		Epoch:      nextEpoch,
		TreeHash:   newTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts)
	if err != nil || !applied {
		return
	}
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(newState, nextEpoch, newTreeHash, commitBytes)
	c.updateLocalAccessRevocationLocked(newState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitBytes)))

	// Hand off Welcome dispatch to the runtime. This local node is the Token
	// Holder that just ran CreateCommit, so it is the only node holding the
	// ephemeral key material required to deliver Welcome to each invitee.
	if batch[0].Type == ProposalAdd && c.onAddCommitted != nil && len(commitMsg.AddDeliveries) > 0 {
		deliveries := append([]AddCommitDelivery(nil), commitMsg.AddDeliveries...)
		welcome := append([]byte(nil), welcomeBytes...)
		epoch := nextEpoch
		cb := c.onAddCommitted
		go func() {
			for _, d := range deliveries {
				cb(d, epoch, welcome)
			}
		}()
	}
}

// buildAddDeliveriesFromBatch projects routing metadata from a homogeneous
// ProposalAdd batch into AddCommitDelivery entries. The same Welcome bytes
// are referenced by WelcomeHash across deliveries because OpenMLS emits a
// single combined Welcome per commit batch.
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

func (c *Coordinator) advanceEpochLocked(newState []byte, newEpoch uint64, newTreeHash, commitData []byte) {
	c.groupState = newState
	c.epoch = newEpoch
	c.treeHash = newTreeHash
	slog.Info("Epoch advanced", "group", c.groupID, "newEpoch", newEpoch)

	buffered := c.epochTracker.Advance(newEpoch, newTreeHash)
	c.singleWriter.AdvanceEpoch(newEpoch)

	var commitHash []byte
	if len(commitData) > 0 {
		h := sha256.Sum256(commitData)
		commitHash = h[:]
	}
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    newTreeHash,
		MemberCount: c.activeView.Size(),
		CommitHash:  commitHash,
	})

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
//   - If the local node is the current Token Holder, the MLS commit is
//     created synchronously via mls.AddMembers and the result includes the
//     Welcome bytes for out-of-band delivery to the invitee.
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

	delivery := AddCommitDelivery{
		OperationID:    req.OperationID,
		TargetPeerID:   req.TargetPeerID.String(),
		RequestID:      req.RequestID,
		GroupType:      req.GroupType,
		CategoryID:     req.CategoryID,
		KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
	}

	// Non-holder path: broadcast ProposalAdd and let whichever node holds
	// the token at the committing epoch author the Welcome.
	if c.singleWriter == nil || !c.singleWriter.IsTokenHolder() {
		proposalBytes, err := c.mls.CreateProposal(c.ctx, c.groupState, ProposalAdd, req.KeyPackageBytes)
		if err != nil {
			return AddMemberResult{}, fmt.Errorf("CreateProposal: %w", err)
		}

		msg := ProposalMsg{
			ProposalType:   ProposalAdd,
			Data:           proposalBytes,
			OperationID:    req.OperationID,
			TargetPeerID:   req.TargetPeerID.String(),
			RequestID:      req.RequestID,
			GroupType:      req.GroupType,
			CategoryID:     req.CategoryID,
			KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
		}
		c.broadcastLocked(MsgProposal, msg)

		c.singleWriter.BufferProposal(BufferedProposal{
			Type:           ProposalAdd,
			Data:           proposalBytes,
			OperationID:    req.OperationID,
			TargetPeerID:   req.TargetPeerID.String(),
			RequestID:      req.RequestID,
			GroupType:      req.GroupType,
			CategoryID:     req.CategoryID,
			KeyPackageHash: msg.KeyPackageHash,
		})
		c.metrics.IncrProposalsReceived()

		return AddMemberResult{
			OperationID: req.OperationID,
			Deferred:    true,
			Delivery:    delivery,
		}, nil
	}

	// Token Holder path: commit synchronously.
	commitBytes, welcomeBytes, newState, newTreeHash, err := c.mls.AddMembers(c.ctx, c.groupState, [][]byte{req.KeyPackageBytes})
	if err != nil {
		return AddMemberResult{}, fmt.Errorf("AddMembers: %w", err)
	}

	if len(welcomeBytes) > 0 {
		sum := sha256.Sum256(welcomeBytes)
		delivery.WelcomeHash = sum[:]
	}

	commitMsg := CommitMsg{
		CommitData:    commitBytes,
		NewTreeHash:   newTreeHash,
		AddDeliveries: []AddCommitDelivery{delivery},
	}
	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return AddMemberResult{}, fmt.Errorf("failed to encode commit envelope")
	}
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      nextEpoch,
		TreeHash:   newTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts)
	if err != nil {
		return AddMemberResult{}, fmt.Errorf("persist commit: %w", err)
	}
	if !applied {
		return AddMemberResult{}, fmt.Errorf("commit envelope already applied")
	}
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(newState, nextEpoch, newTreeHash, commitBytes)
	c.updateLocalAccessRevocationLocked(newState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitBytes)))

	// Hand off Welcome delivery on the holder side. We dispatch via the
	// callback so the same outbox-style code path (pending_welcomes_out +
	// store replication + direct delivery) handles both this synchronous
	// commit and the asynchronous proposal-commit case in tryCommitLocked.
	if c.onAddCommitted != nil && len(welcomeBytes) > 0 {
		welcome := append([]byte(nil), welcomeBytes...)
		cb := c.onAddCommitted
		epoch := nextEpoch
		go cb(delivery, epoch, welcome)
	}

	return AddMemberResult{
		OperationID: req.OperationID,
		Welcome:     welcomeBytes,
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
	if len(targetIdentity) == 0 {
		return fmt.Errorf("target identity is required")
	}

	if c.singleWriter == nil || !c.singleWriter.IsTokenHolder() {
		return c.proposeLocked(ProposalRemove, targetIdentity)
	}

	commitBytes, newState, newTreeHash, err := c.mls.RemoveMembers(c.ctx, c.groupState, [][]byte{targetIdentity})
	if err != nil {
		return fmt.Errorf("RemoveMembers: %w", err)
	}

	commitMsg := CommitMsg{
		CommitData:  commitBytes,
		NewTreeHash: newTreeHash,
	}
	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return fmt.Errorf("failed to encode commit envelope")
	}
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      nextEpoch,
		TreeHash:   newTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts)
	if err != nil {
		return fmt.Errorf("persist commit: %w", err)
	}
	if !applied {
		return fmt.Errorf("commit envelope already applied")
	}
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(newState, nextEpoch, newTreeHash, commitBytes)
	c.updateLocalAccessRevocationLocked(newState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitBytes)))

	return nil
}

// SendMessage encrypts plaintext and broadcasts it as an application message.
// Returns the HLC timestamp assigned to the message.
func (c *Coordinator) SendMessage(plaintext []byte) (*HLCTimestamp, error) {
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

	ciphertext, newState, err := c.mls.EncryptMessage(c.ctx, c.groupState, plaintext)
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
		GroupID:      c.groupID,
		Epoch:        c.epoch,
		SenderID:     c.localID,
		Content:      plaintext,
		Timestamp:    ts,
		EnvelopeHash: envelopeHash,
	}
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyApplication(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		UpdatedAt:  now,
	}, msg, MsgApplication, envBytes, ts)
	if err != nil {
		return nil, fmt.Errorf("persist application: %w", err)
	}
	if !applied {
		return nil, fmt.Errorf("application envelope already applied")
	}
	c.groupState = newState
	c.publishPreparedEnvelopeLocked(MsgApplication, envBytes)
	slog.Info("Message sent", "group", c.groupID, "epoch", c.epoch, "ts", ts.WallTimeMs)
	if c.onMessage != nil {
		c.onMessage(msg)
	}

	return &ts, nil
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

	proposalBytes, err := c.mls.CreateProposal(c.ctx, c.groupState, pType, data)
	if err != nil {
		return fmt.Errorf("CreateProposal: %w", err)
	}

	c.broadcastLocked(MsgProposal, ProposalMsg{ProposalType: pType, Data: proposalBytes})

	c.singleWriter.BufferProposal(BufferedProposal{Type: pType, Data: proposalBytes})
	c.metrics.IncrProposalsReceived()

	if c.singleWriter.IsTokenHolder() {
		c.tryCommitLocked()
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
	ok, err := c.mls.HasMember(c.ctx, groupState, c.localIdentity)
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
	_ = c.transport.Publish(c.ctx, GroupTopic(c.groupID), envBytes)
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

// ReplayEnvelopes applies ciphertext envelopes from offline sync / DHT in order.
// Caller must hold no Coordinator lock; this method is fully synchronized.
func (c *Coordinator) ReplayEnvelopes(blobs [][]byte) (applied int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return 0, fmt.Errorf("coordinator not started")
	}

	for _, raw := range blobs {
		if len(raw) == 0 {
			continue
		}
		var env Envelope
		if jerr := json.Unmarshal(raw, &env); jerr != nil {
			continue
		}
		if env.GroupID != c.groupID {
			continue
		}
		switch env.Type {
		case MsgCommit:
			if c.handleCommitLocked(&env, raw) {
				applied++
			}
		case MsgApplication:
			if c.handleApplicationLocked(decodeEnvelopePeerID(env.From, ""), &env, raw) {
				applied++
			}
		default:
			continue
		}
	}
	return applied, nil
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
	ann := GroupStateAnnouncement{
		TreeHash:    c.treeHash,
		MemberCount: c.activeView.Size(),
	}
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
	localTreeHash := append([]byte(nil), c.treeHash...)
	scheduledAt := c.clock.Now()
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

	go c.runHeal(c.ctx, traceID, event, scheduledAt)
}

// runHeal executes the fork-heal pipeline. It runs on its own goroutine and is
// guaranteed to release c.healing on exit.
func (c *Coordinator) runHeal(ctx context.Context, traceID string, event *ForkEvent, scheduledAt time.Time) {
	defer c.healing.Store(false)

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

	stepStart := c.clock.Now()
	slog.Info("fork_heal/groupinfo_request_started",
		"trace_id", traceID,
		"group", event.GroupID,
		"winner_peers", len(event.WinnerPeers),
	)
	c.recordForkHealAudit(traceID, event.GroupID, "groupinfo_request", "started", stepStart, 0, "")
	// Try each known winner peer in order; the triggering peer is first.
	peers := event.WinnerPeers
	if len(peers) == 0 {
		peers = []peer.ID{event.RemotePeer}
	}
	var gi *GroupInfoFetchResult
	var lastFetchErr error
	for _, remotePeer := range peers {
		gi, lastFetchErr = c.fetchGroupInfoForHeal(ctx, remotePeer, event.GroupID, true)
		if lastFetchErr == nil {
			break
		}
		slog.Warn("fork_heal/groupinfo_request_peer_failed",
			"trace_id", traceID,
			"group", event.GroupID,
			"peer", remotePeer.String(),
			"err", lastFetchErr,
		)
	}
	if lastFetchErr != nil {
		c.recordForkHealAudit(traceID, event.GroupID, "groupinfo_request", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), lastFetchErr.Error())
		c.logHealFailed(traceID, event, startedAt, scheduledAt, "groupinfo_request", lastFetchErr)
		return
	}
	groupInfoReqDur := c.clock.Now().Sub(stepStart).Milliseconds()
	slog.Info("fork_heal/groupinfo_request_completed",
		"trace_id", traceID,
		"group", event.GroupID,
		"winner_epoch", gi.Epoch,
		"group_info_bytes", len(gi.GroupInfo),
		"duration_ms", groupInfoReqDur,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "groupinfo_request", "completed", c.clock.Now(), groupInfoReqDur, "")

	stepStart = c.clock.Now()
	slog.Info("fork_heal/groupinfo_received_started",
		"trace_id", traceID,
		"group", event.GroupID,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "groupinfo_received", "started", stepStart, 0, "")
	// Accept the GroupInfo if the winner's epoch has only advanced (never regressed).
	// A hard TreeHash equality check would create false positives when the winning
	// branch commits one or two more epochs between the MsgAnnounce we received and
	// the GroupInfo we just fetched — a normal condition under any network traffic.
	if gi.Epoch < event.RemoteEpoch {
		err := fmt.Errorf("winner epoch regressed: got %d, expected >= %d", gi.Epoch, event.RemoteEpoch)
		c.recordForkHealAudit(traceID, event.GroupID, "groupinfo_received", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
		c.logHealFailed(traceID, event, startedAt, scheduledAt, "groupinfo_received", err)
		return
	}
	if len(gi.TreeHash) > 0 && len(event.RemoteAnnounce.TreeHash) > 0 && !bytes.Equal(gi.TreeHash, event.RemoteAnnounce.TreeHash) {
		slog.Warn("fork_heal/groupinfo_received_tree_hash_advanced",
			"trace_id", traceID,
			"group", event.GroupID,
			"announce_tree_hash", hex.EncodeToString(event.RemoteAnnounce.TreeHash),
			"fetched_tree_hash", hex.EncodeToString(gi.TreeHash),
			"winner_epoch", gi.Epoch,
		)
	}
	groupInfoRecvDur := c.clock.Now().Sub(stepStart).Milliseconds()
	slog.Info("fork_heal/groupinfo_received_completed",
		"trace_id", traceID,
		"group", event.GroupID,
		"winner_tree_hash", hex.EncodeToString(gi.TreeHash),
		"duration_ms", groupInfoRecvDur,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "groupinfo_received", "completed", c.clock.Now(), groupInfoRecvDur, "")

	stepStart = c.clock.Now()
	slog.Info("fork_heal/external_join_started",
		"trace_id", traceID,
		"group", event.GroupID,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "external_join", "started", stepStart, 0, "")
	newState, externalCommit, newTreeHash, err := c.mls.ExternalJoin(ctx, gi.GroupInfo, c.signingKey)
	if err != nil {
		c.recordForkHealAudit(traceID, event.GroupID, "external_join", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
		c.logHealFailed(traceID, event, startedAt, scheduledAt, "external_join", err)
		return
	}
	newEpoch := gi.Epoch + 1
	externalJoinDur := c.clock.Now().Sub(stepStart).Milliseconds()
	slog.Info("fork_heal/external_join_completed",
		"trace_id", traceID,
		"group", event.GroupID,
		"new_epoch", newEpoch,
		"new_tree_hash", hex.EncodeToString(newTreeHash),
		"external_commit_bytes", len(externalCommit),
		"duration_ms", externalJoinDur,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "external_join", "completed", c.clock.Now(), externalJoinDur, "")

	stepStart = c.clock.Now()
	slog.Info("fork_heal/state_swap_started",
		"trace_id", traceID,
		"group", event.GroupID,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "started", stepStart, 0, "")
	if err := c.applyHealedState(newState, newTreeHash, newEpoch, externalCommit); err != nil {
		c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
		c.logHealFailed(traceID, event, startedAt, scheduledAt, "state_swap", err)
		return
	}
	stateSwapDur := c.clock.Now().Sub(stepStart).Milliseconds()
	slog.Info("fork_heal/state_swap_completed",
		"trace_id", traceID,
		"group", event.GroupID,
		"new_epoch", newEpoch,
		"duration_ms", stateSwapDur,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "completed", c.clock.Now(), stateSwapDur, "")

	stepStart = c.clock.Now()
	slog.Info("fork_heal/external_commit_started",
		"trace_id", traceID,
		"group", event.GroupID,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "external_commit", "started", stepStart, 0, "")
	if err := c.broadcastExternalCommit(gi.Epoch, externalCommit, newTreeHash); err != nil {
		c.recordForkHealAudit(traceID, event.GroupID, "external_commit", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
		c.logHealFailed(traceID, event, startedAt, scheduledAt, "external_commit", err)
		return
	}
	externalCommitDur := c.clock.Now().Sub(stepStart).Milliseconds()
	slog.Info("fork_heal/external_commit_completed",
		"trace_id", traceID,
		"group", event.GroupID,
		"duration_ms", externalCommitDur,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "external_commit", "completed", c.clock.Now(), externalCommitDur, "")

	// Replay body lands in Sprint 2E; keep step markers stable now so Phase 9.2
	// parsers do not need to change when replay logic is added.
	stepStart = c.clock.Now()
	slog.Info("fork_heal/replay_started_started",
		"trace_id", traceID,
		"group", event.GroupID,
		"partition_started_at_ms", event.PartitionStartedAt.UnixMilli(),
	)
	c.recordForkHealAudit(traceID, event.GroupID, "replay_started", "started", stepStart, 0, "")
	replayWindow, err := c.collectReplayWindowMessages(event.PartitionStartedAt, startedAt)
	if err != nil {
		c.recordForkHealAudit(traceID, event.GroupID, "replay_started", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
		c.logHealFailed(traceID, event, startedAt, scheduledAt, "replay_started", err)
		return
	}
	replayStartDur := c.clock.Now().Sub(stepStart).Milliseconds()
	slog.Info("fork_heal/replay_started_completed",
		"trace_id", traceID,
		"group", event.GroupID,
		"window_message_count", len(replayWindow),
		"duration_ms", replayStartDur,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "replay_started", "completed", c.clock.Now(), replayStartDur, "")

	stepStart = c.clock.Now()
	slog.Info("fork_heal/replay_completed_started",
		"trace_id", traceID,
		"group", event.GroupID,
		"window_message_count", len(replayWindow),
		"replay_throttle_ms", c.cfg.ReplayThrottleMs,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "replay_completed", "started", stepStart, 0, "")
	replayedCount, err := c.replayWindowMessages(ctx, replayWindow)
	if err != nil {
		c.recordForkHealAudit(traceID, event.GroupID, "replay_completed", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
		c.logHealFailed(traceID, event, startedAt, scheduledAt, "replay_completed", err)
		return
	}
	replayCompletedDur := c.clock.Now().Sub(stepStart).Milliseconds()
	slog.Info("fork_heal/replay_completed_completed",
		"trace_id", traceID,
		"group", event.GroupID,
		"replayed_count", replayedCount,
		"duration_ms", replayCompletedDur,
	)
	c.recordForkHealAudit(traceID, event.GroupID, "replay_completed", "completed", c.clock.Now(), replayCompletedDur, "")

	completedAt := c.clock.Now()
	c.metrics.IncrForkHealingsSucceeded()
	c.metrics.RecordExternalJoin(completedAt.Sub(startedAt))
	slog.Info("fork_heal/completed",
		"trace_id", traceID,
		"group", event.GroupID,
		"outcome", "success",
		"new_epoch", newEpoch,
		"new_tree_hash", hex.EncodeToString(newTreeHash),
		"duration_ms", completedAt.Sub(startedAt).Milliseconds(),
		"total_ms", completedAt.Sub(scheduledAt).Milliseconds(),
	)
	slog.Info("fork_heal/aggregate",
		"trace_id", traceID,
		"group", event.GroupID,
		"winner_peer", event.RemotePeer.String(),
		"winner_epoch", gi.Epoch,
		"winner_member_count", event.RemoteAnnounce.MemberCount,
		"local_member_count_before", event.LocalAnnounce.MemberCount,
		"new_epoch", newEpoch,
		"new_tree_hash", hex.EncodeToString(newTreeHash),
		"replayed_message_count", replayedCount,
		"partition_window_ms", completedAt.Sub(event.PartitionStartedAt).Milliseconds(),
		"total_ms", completedAt.Sub(scheduledAt).Milliseconds(),
	)
	c.recordForkHealEvent(&ForkHealEventRecord{
		TraceID:              traceID,
		GroupID:              event.GroupID,
		WinnerPeerID:         event.RemotePeer.String(),
		WinnerEpoch:          gi.Epoch,
		NewEpoch:             newEpoch,
		Outcome:              "success",
		FailedStep:           "",
		WinnerTreeHash:       append([]byte(nil), gi.TreeHash...),
		NewTreeHash:          append([]byte(nil), newTreeHash...),
		PartitionStartedAtMs: event.PartitionStartedAt.UnixMilli(),
		ScheduledAtMs:        scheduledAt.UnixMilli(),
		StartedAtMs:          startedAt.UnixMilli(),
		CompletedAtMs:        completedAt.UnixMilli(),
		DurationMs:           completedAt.Sub(startedAt).Milliseconds(),
		TotalMs:              completedAt.Sub(scheduledAt).Milliseconds(),
		ReplayedMessageCount: replayedCount,
	})
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

func (c *Coordinator) applyHealedState(newState, newTreeHash []byte, newEpoch uint64, externalCommit []byte) error {
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

	var commitHash []byte
	if len(externalCommit) > 0 {
		sum := sha256.Sum256(externalCommit)
		commitHash = sum[:]
	}
	c.forkDetector.Reset()
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    newTreeHash,
		MemberCount: c.activeView.Size(),
		CommitHash:  commitHash,
	})
	if c.onEpochChange != nil {
		c.onEpochChange(newEpoch)
	}
	return nil
}

func (c *Coordinator) broadcastExternalCommit(envelopeEpoch uint64, externalCommit, newTreeHash []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ts := c.hlc.Now()
	wire := c.buildEnvelopeWithEpochAndTimestampLocked(MsgCommit, CommitMsg{
		CommitData:  externalCommit,
		NewTreeHash: newTreeHash,
	}, envelopeEpoch, ts)
	if len(wire) == 0 {
		return fmt.Errorf("encode external commit envelope")
	}
	c.publishPreparedEnvelopeLocked(MsgCommit, wire)
	c.appendOfflineEnvelopeLocked(wire)
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
