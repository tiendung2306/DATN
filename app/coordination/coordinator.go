package coordination

import (
	"bytes"
	"context"
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
	// OnEnvelopeBroadcast is called when this local node publishes a commit or
	// application envelope to the group topic. Intended for offline replication
	// layers (e.g. blind-store) and not invoked for replayed remote envelopes.
	OnEnvelopeBroadcast func(MessageType, string, []byte)
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

	onMessage     func(*StoredMessage)
	onEpochChange func(uint64)
	onEnvelope    func(MessageType, string, []byte)
	groupInfoFetch GroupInfoFetchFunc

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
		cfg:           opts.Config,
		transport:     opts.Transport,
		clock:         opts.Clock,
		mls:           opts.MLS,
		storage:       opts.Storage,
		localID:       opts.LocalID,
		groupID:       opts.GroupID,
		signingKey:    opts.SigningKey,
		hlc:           NewHLC(opts.Clock, opts.LocalID.String()),
		metrics:       NewMetrics(),
		onMessage:     opts.OnMessage,
		onEpochChange: opts.OnEpochChange,
		onEnvelope:    opts.OnEnvelopeBroadcast,
		groupInfoFetch: opts.GroupInfoFetcher,
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
	c.activeView.RecordHeartbeat(from)
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

	c.singleWriter.BufferProposal(proposal.Data)
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
	c.metrics.RecordEpochFinalization(c.clock.Now().Sub(start))
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

func (c *Coordinator) tryCommitLocked() {
	proposals := c.singleWriter.DrainProposals()
	if len(proposals) == 0 {
		return
	}

	commitBytes, welcomeBytes, newState, newTreeHash, err := c.mls.CreateCommit(c.ctx, c.groupState, proposals)
	if err != nil {
		return
	}

	_ = welcomeBytes // MLS Welcome must be delivered out-of-band, not broadcast in CommitMsg.
	commitMsg := CommitMsg{
		CommitData:  commitBytes,
		NewTreeHash: newTreeHash,
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
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitBytes)))
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

// AddMember adds a new member from their KeyPackage bytes. Only the current
// Token Holder may call this. Returns the MLS Welcome bytes for out-of-band
// delivery to the invitee.
func (c *Coordinator) AddMember(newMemberPeerID peer.ID, keyPackageBytes []byte) ([]byte, error) {
	_ = newMemberPeerID

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil, fmt.Errorf("coordinator not started")
	}
	if c.singleWriter == nil || !c.singleWriter.IsTokenHolder() {
		return nil, ErrNotTokenHolder
	}

	commitBytes, welcomeBytes, newState, newTreeHash, err := c.mls.AddMembers(c.ctx, c.groupState, [][]byte{keyPackageBytes})
	if err != nil {
		return nil, fmt.Errorf("AddMembers: %w", err)
	}

	commitMsg := CommitMsg{
		CommitData:  commitBytes,
		NewTreeHash: newTreeHash,
	}
	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return nil, fmt.Errorf("failed to encode commit envelope")
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
		return nil, fmt.Errorf("persist commit: %w", err)
	}
	if !applied {
		return nil, fmt.Errorf("commit envelope already applied")
	}
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(newState, nextEpoch, newTreeHash, commitBytes)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitBytes)))

	return welcomeBytes, nil
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

	if !c.started {
		return fmt.Errorf("coordinator not started")
	}

	proposalBytes, err := c.mls.CreateProposal(c.ctx, c.groupState, pType, data)
	if err != nil {
		return fmt.Errorf("CreateProposal: %w", err)
	}

	c.broadcastLocked(MsgProposal, ProposalMsg{ProposalType: pType, Data: proposalBytes})

	c.singleWriter.BufferProposal(proposalBytes)
	c.metrics.IncrProposalsReceived()

	if c.singleWriter.IsTokenHolder() {
		c.tryCommitLocked()
	}
	return nil
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
	createdAt := now
	if prevRec != nil {
		if prevRec.MyRole != "" {
			role = prevRec.MyRole
		}
		groupType = prevRec.GroupType
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
	createdAt := now
	if prevRec != nil {
		if prevRec.MyRole != "" {
			role = prevRec.MyRole
		}
		groupType = prevRec.GroupType
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
