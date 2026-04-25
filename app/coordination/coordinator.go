package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

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
}

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
	go func() { defer c.wg.Done(); c.periodicLoop(c.ctx) }()

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

	event := c.forkDetector.ProcessRemote(from, env.Epoch, ann)
	if event != nil && event.NeedExternalJoin {
		c.metrics.IncrPartitionsDetected()
	}
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
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	env := Envelope{
		Type:      msgType,
		GroupID:   c.groupID,
		Epoch:     c.epoch,
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

func (c *Coordinator) periodicLoop(ctx context.Context) {
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
	ann := GroupStateAnnouncement{
		TreeHash:    c.treeHash,
		MemberCount: c.activeView.Size(),
	}
	c.broadcastLocked(MsgAnnounce, ann)
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
