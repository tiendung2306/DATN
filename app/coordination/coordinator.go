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
		hlc:           NewHLC(opts.Clock, string(opts.LocalID)),
		metrics:       NewMetrics(),
		onMessage:     opts.OnMessage,
		onEpochChange: opts.OnEpochChange,
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
		c.handleCommitLocked(&env)
	case MsgApplication:
		c.handleApplicationLocked(&env)
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

func (c *Coordinator) handleCommitLocked(env *Envelope) {
	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		c.metrics.IncrDuplicateEpochDetected()
		return
	case ActionBufferFuture:
		raw, _ := json.Marshal(env)
		c.epochTracker.BufferFuture(env.Epoch, raw)
		return
	}

	var commit CommitMsg
	if err := json.Unmarshal(env.Payload, &commit); err != nil {
		return
	}

	start := c.clock.Now()

	newState, newTreeHash, err := c.mls.ProcessCommit(c.ctx, c.groupState, commit.CommitData)
	if err != nil {
		return
	}

	c.advanceEpochLocked(newState, c.epoch+1, newTreeHash, commit.CommitData)
	c.metrics.RecordEpochFinalization(c.clock.Now().Sub(start))
}

func (c *Coordinator) handleApplicationLocked(env *Envelope) {
	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		slog.Warn("Rejected stale message", "group", c.groupID, "msgEpoch", env.Epoch, "currentEpoch", c.epoch)
		return
	case ActionBufferFuture:
		slog.Info("Buffered future-epoch message", "group", c.groupID, "msgEpoch", env.Epoch)
		raw, _ := json.Marshal(env)
		c.epochTracker.BufferFuture(env.Epoch, raw)
		return
	}

	var appMsg ApplicationMsg
	if err := json.Unmarshal(env.Payload, &appMsg); err != nil {
		return
	}

	localTs := c.hlc.Update(env.Timestamp)

	plaintext, newState, err := c.mls.DecryptMessage(c.ctx, c.groupState, appMsg.Ciphertext)
	if err != nil {
		slog.Error("Failed to decrypt message", "group", c.groupID, "from", env.From, "error", err)
		return
	}
	c.groupState = newState
	slog.Info("Message received", "group", c.groupID, "epoch", env.Epoch, "from", env.From, "ts", localTs.WallTimeMs)

	msg := &StoredMessage{
		GroupID:   c.groupID,
		Epoch:     env.Epoch,
		SenderID:  peer.ID(env.From),
		Content:   plaintext,
		Timestamp: localTs,
	}
	_ = c.storage.SaveMessage(msg)

	if c.onMessage != nil {
		c.onMessage(msg)
	}
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
	c.broadcastLocked(MsgCommit, commitMsg)

	c.advanceEpochLocked(newState, c.epoch+1, newTreeHash, commitBytes)
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

	_ = c.storage.SaveGroupRecord(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      newEpoch,
		TreeHash:   newTreeHash,
		UpdatedAt:  c.clock.Now(),
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
			c.handleProposalLocked(peer.ID(env.From), &env)
		case MsgCommit:
			c.handleCommitLocked(&env)
		case MsgApplication:
			c.handleApplicationLocked(&env)
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
	c.broadcastLocked(MsgCommit, commitMsg)

	c.advanceEpochLocked(newState, c.epoch+1, newTreeHash, commitBytes)
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

	ciphertext, newState, err := c.mls.EncryptMessage(c.ctx, c.groupState, plaintext)
	if err != nil {
		slog.Error("Failed to encrypt message", "group", c.groupID, "error", err)
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	c.groupState = newState

	c.broadcastWithTimestampLocked(MsgApplication, ApplicationMsg{Ciphertext: ciphertext}, ts)
	slog.Info("Message sent", "group", c.groupID, "epoch", c.epoch, "ts", ts.WallTimeMs)

	msg := &StoredMessage{
		GroupID:   c.groupID,
		Epoch:     c.epoch,
		SenderID:  c.localID,
		Content:   plaintext,
		Timestamp: ts,
	}
	_ = c.storage.SaveMessage(msg)
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
	c.broadcastWithTimestampLocked(msgType, payload, c.hlc.Now())
}

func (c *Coordinator) broadcastWithTimestampLocked(msgType MessageType, payload interface{}, ts HLCTimestamp) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	env := Envelope{
		Type:      msgType,
		GroupID:   c.groupID,
		Epoch:     c.epoch,
		From:      string(c.localID),
		Timestamp: ts,
		Payload:   payloadBytes,
	}
	envBytes, err := json.Marshal(env)
	if err != nil {
		return
	}
	_ = c.transport.Publish(c.ctx, GroupTopic(c.groupID), envBytes)
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
