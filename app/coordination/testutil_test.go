package coordination

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ═══════════════════════════════════════════════════════════════════════════════
// FakeNetwork + FakeTransport — in-memory network for deterministic tests
// ═══════════════════════════════════════════════════════════════════════════════

// FakeNetwork manages a set of FakeTransport nodes. Messages published by one
// node are queued and delivered to connected peers only when DrainAll is called.
// This avoids re-entrant mutex deadlocks while keeping tests deterministic.
type FakeNetwork struct {
	mu    sync.Mutex
	nodes map[peer.ID]*FakeTransport
	links map[fakePair]bool
	inbox []pendingDelivery
}

type fakePair struct{ a, b string }

type pendingDelivery struct {
	from  peer.ID
	to    *FakeTransport
	topic string
	data  []byte
}

func newFakePair(a, b peer.ID) fakePair {
	sa, sb := string(a), string(b)
	if sa > sb {
		sa, sb = sb, sa
	}
	return fakePair{a: sa, b: sb}
}

func NewFakeNetwork() *FakeNetwork {
	return &FakeNetwork{
		nodes: make(map[peer.ID]*FakeTransport),
		links: make(map[fakePair]bool),
	}
}

func (fn *FakeNetwork) AddNode(id peer.ID) *FakeTransport {
	fn.mu.Lock()
	defer fn.mu.Unlock()

	ft := &FakeTransport{
		localID:  id,
		network:  fn,
		handlers: make(map[string]func(peer.ID, []byte)),
	}
	for existingID := range fn.nodes {
		if existingID != id {
			fn.links[newFakePair(id, existingID)] = true
		}
	}
	fn.nodes[id] = ft
	return ft
}

// Partition disconnects two groups of peers from each other.
func (fn *FakeNetwork) Partition(groupA, groupB []peer.ID) {
	fn.mu.Lock()
	defer fn.mu.Unlock()
	for _, a := range groupA {
		for _, b := range groupB {
			delete(fn.links, newFakePair(a, b))
		}
	}
}

// Heal reconnects all nodes in the network.
func (fn *FakeNetwork) Heal() {
	fn.mu.Lock()
	defer fn.mu.Unlock()
	ids := make([]peer.ID, 0, len(fn.nodes))
	for id := range fn.nodes {
		ids = append(ids, id)
	}
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			fn.links[newFakePair(ids[i], ids[j])] = true
		}
	}
}

func (fn *FakeNetwork) isConnected(a, b peer.ID) bool {
	return fn.links[newFakePair(a, b)]
}

func (fn *FakeNetwork) enqueue(from peer.ID, topic string, data []byte) {
	fn.mu.Lock()
	defer fn.mu.Unlock()
	for id, node := range fn.nodes {
		if id == from || !fn.isConnected(from, id) {
			continue
		}
		cp := make([]byte, len(data))
		copy(cp, data)
		fn.inbox = append(fn.inbox, pendingDelivery{from: from, to: node, topic: topic, data: cp})
	}
}

// DrainAll delivers all queued messages. Messages published during delivery
// are queued and delivered in subsequent rounds until the network is quiet.
func (fn *FakeNetwork) DrainAll() {
	for {
		fn.mu.Lock()
		if len(fn.inbox) == 0 {
			fn.mu.Unlock()
			return
		}
		batch := fn.inbox
		fn.inbox = nil
		fn.mu.Unlock()

		for _, d := range batch {
			d.to.deliver(d.from, d.topic, d.data)
		}
	}
}

// PendingCount returns the number of undelivered messages.
func (fn *FakeNetwork) PendingCount() int {
	fn.mu.Lock()
	defer fn.mu.Unlock()
	return len(fn.inbox)
}

// PendingByType counts how many pending deliveries originate from `from` and
// carry a coordination Envelope of the given message type. Used by Sprint 2B
// tests to assert the heartbeat / announce loops fire on independent cadences.
func (fn *FakeNetwork) PendingByType(from peer.ID, msgType MessageType) int {
	fn.mu.Lock()
	defer fn.mu.Unlock()
	count := 0
	for _, d := range fn.inbox {
		if d.from != from {
			continue
		}
		var env Envelope
		if json.Unmarshal(d.data, &env) != nil {
			continue
		}
		if env.Type == msgType {
			count++
		}
	}
	return count
}

// FakeTransport implements Transport using the in-memory FakeNetwork.
type FakeTransport struct {
	mu       sync.Mutex
	localID  peer.ID
	network  *FakeNetwork
	handlers map[string]func(peer.ID, []byte)
}

var _ Transport = (*FakeTransport)(nil)

func (ft *FakeTransport) Publish(_ context.Context, topic string, data []byte) error {
	ft.network.enqueue(ft.localID, topic, data)
	return nil
}

func (ft *FakeTransport) Subscribe(topic string, handler func(peer.ID, []byte)) error {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.handlers[topic] = handler
	return nil
}

func (ft *FakeTransport) Unsubscribe(topic string) error {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	delete(ft.handlers, topic)
	return nil
}

func (ft *FakeTransport) SendDirect(ctx context.Context, to peer.ID, data []byte) error {
	ft.network.mu.Lock()
	target, ok := ft.network.nodes[to]
	if !ok || !ft.network.isConnected(ft.localID, to) {
		ft.network.mu.Unlock()
		return fmt.Errorf("fake-transport: peer %s unreachable", to)
	}
	ft.network.mu.Unlock()

	// In this test transport, we assume the first protocol registered
	// by the target that looks like a direct protocol is the one to use.
	// Production uses host.NewStream(to, protocolID).
	// Since SendDirect doesn't take a protocol ID, we'll try CoordDirectProtocol first.
	const CoordDirectProtocol = "/coordination/direct/1.0.0"
	target.deliverDirect(ft.localID, CoordDirectProtocol, data)
	return nil
}

func (ft *FakeTransport) deliverDirect(from peer.ID, protocol string, data []byte) {
	ft.mu.Lock()
	handler := ft.handlers[protocol]
	ft.mu.Unlock()
	if handler != nil {
		handler(from, data)
	}
}

func (ft *FakeTransport) LocalPeerID() peer.ID { return ft.localID }

func (ft *FakeTransport) ConnectedPeers() []peer.ID {
	ft.network.mu.Lock()
	defer ft.network.mu.Unlock()
	var out []peer.ID
	for id := range ft.network.nodes {
		if id != ft.localID && ft.network.isConnected(ft.localID, id) {
			out = append(out, id)
		}
	}
	return out
}

func (ft *FakeTransport) deliver(from peer.ID, topic string, data []byte) {
	ft.mu.Lock()
	handler := ft.handlers[topic]
	ft.mu.Unlock()
	if handler != nil {
		handler(from, data)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// MockMLSEngine — deterministic mock with JSON-based group state
// ═══════════════════════════════════════════════════════════════════════════════

type mockGroupState struct {
	GroupID     string          `json:"group_id"`
	Epoch       uint64          `json:"epoch"`
	TreeHash    string          `json:"tree_hash"`
	Members     map[string]bool `json:"members"` // peerID -> isMember
	PendingRefs [][]byte        `json:"pending_refs,omitempty"`
}

type mockCommitData struct {
	NewEpoch     uint64          `json:"new_epoch"`
	NewTreeHash  string          `json:"new_tree_hash"`
	NewMembers   map[string]bool `json:"new_members"`
	ProposalRefs [][]byte        `json:"proposal_refs,omitempty"`
}

type mockCiphertext struct {
	Epoch     uint64 `json:"epoch"`
	Plaintext []byte `json:"plaintext"`
}

type MockMLSEngine struct {
	mu            sync.Mutex
	nextErr       error
	hasMemberFn   func(groupState []byte, identity []byte) (bool, error)
	listMembersFn func(groupState []byte) ([][]byte, error)
}

var _ MLSEngine = (*MockMLSEngine)(nil)

func NewMockMLSEngine() *MockMLSEngine { return &MockMLSEngine{} }

// SetNextError makes the next MLS call return this error, then clears it.
func (m *MockMLSEngine) SetNextError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextErr = err
}

func (m *MockMLSEngine) popError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.nextErr
	m.nextErr = nil
	return err
}

// SetHasMemberFunc overrides HasMember behavior in tests.
func (m *MockMLSEngine) SetHasMemberFunc(fn func(groupState []byte, identity []byte) (bool, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hasMemberFn = fn
}

// SetListMembersFunc lets coordination tests inject a deterministic answer
// for MLSEngine.ListMemberIdentities — used to exercise the join-roster
// backfill path without standing up the full Rust engine.
func (m *MockMLSEngine) SetListMembersFunc(fn func(groupState []byte) ([][]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listMembersFn = fn
}

func mockTreeHash(epoch uint64) []byte {
	h := sha256.Sum256([]byte(fmt.Sprintf("tree-epoch-%d", epoch)))
	return h[:]
}

func (m *MockMLSEngine) CreateGroup(_ context.Context, groupID string, creatorKey []byte, _ uint32) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	th := mockTreeHash(0)
	state := mockGroupState{
		GroupID:  groupID,
		Epoch:    0,
		TreeHash: hex.EncodeToString(th),
		Members:  map[string]bool{string(deriveIdentityFromSigningKey(creatorKey)): true},
	}
	stateBytes, _ := json.Marshal(state)
	return stateBytes, th, nil
}

func (m *MockMLSEngine) CreateProposal(_ context.Context, groupState []byte, _ ProposalType, data []byte) (CreateProposalResult, error) {
	if err := m.popError(); err != nil {
		return CreateProposalResult{}, err
	}
	proposalBytes := append([]byte(nil), data...)
	sum := sha256.Sum256(proposalBytes)
	proposalRef := sum[:]
	newState := append([]byte(nil), groupState...)
	var state mockGroupState
	if json.Unmarshal(groupState, &state) == nil {
		state.PendingRefs = append(state.PendingRefs, append([]byte(nil), proposalRef...))
		newState, _ = json.Marshal(state)
	}
	return CreateProposalResult{
		ProposalBytes: proposalBytes,
		ProposalRef:   append([]byte(nil), proposalRef...),
		NewGroupState: newState,
	}, nil
}

func (m *MockMLSEngine) ProcessProposal(_ context.Context, groupState []byte, proposalBytes []byte) (ProcessProposalResult, error) {
	if err := m.popError(); err != nil {
		return ProcessProposalResult{}, err
	}
	sum := sha256.Sum256(proposalBytes)
	proposalRef := sum[:]
	newState := append([]byte(nil), groupState...)
	var state mockGroupState
	if json.Unmarshal(groupState, &state) == nil {
		state.PendingRefs = append(state.PendingRefs, append([]byte(nil), proposalRef...))
		newState, _ = json.Marshal(state)
	}
	return ProcessProposalResult{
		ProposalRef:   append([]byte(nil), proposalRef...),
		ProposalType:  "Mock",
		NewGroupState: newState,
	}, nil
}

func (m *MockMLSEngine) CreateCommit(_ context.Context, groupState []byte, expectedProposalRefs [][]byte) (CreateCommitResult, error) {
	if err := m.popError(); err != nil {
		return CreateCommitResult{}, err
	}
	var state mockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return CreateCommitResult{}, fmt.Errorf("mock: bad state: %w", err)
	}
	state.Epoch++
	newTH := mockTreeHash(state.Epoch)
	state.TreeHash = hex.EncodeToString(newTH)
	state.PendingRefs = nil

	newStateBytes, _ := json.Marshal(state)
	commitInfo := mockCommitData{NewEpoch: state.Epoch, NewTreeHash: state.TreeHash, ProposalRefs: cloneBytesList(expectedProposalRefs)}
	commitBytes, _ := json.Marshal(commitInfo)
	// Return a non-empty Welcome stub so the Token-Holder ProposalAdd path
	// can be exercised end-to-end in tests. The coordinator only routes
	// Welcome bytes when the committed batch contains ProposalAdd entries
	// (see buildAddDeliveriesFromBatch), so returning Welcome here is a
	// harmless no-op for Remove/Update batches.
	welcome := []byte(fmt.Sprintf("mock-welcome:%d", state.Epoch))
	return CreateCommitResult{
		CommitBytes:           commitBytes,
		WelcomeBytes:          welcome,
		CommittedProposalRefs: cloneBytesList(expectedProposalRefs),
		NewGroupState:         newStateBytes,
		NewTreeHash:           newTH,
	}, nil
}

func (m *MockMLSEngine) StageCommit(_ context.Context, _ []byte, commitBytes []byte, includedProposals [][]byte) (StageCommitResult, error) {
	if err := m.popError(); err != nil {
		return StageCommitResult{}, err
	}
	var commit mockCommitData
	if err := json.Unmarshal(commitBytes, &commit); err != nil {
		return StageCommitResult{}, fmt.Errorf("mock: bad commit: %w", err)
	}
	refs := cloneBytesList(commit.ProposalRefs)
	if len(refs) == 0 {
		for _, proposal := range includedProposals {
			sum := sha256.Sum256(proposal)
			refs = append(refs, sum[:])
		}
	}
	return StageCommitResult{Epoch: commit.NewEpoch, ProposalRefs: refs}, nil
}

func (m *MockMLSEngine) ProcessCommit(_ context.Context, groupState, commitBytes []byte, _ [][]byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	var state mockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return nil, nil, fmt.Errorf("mock: bad state: %w", err)
	}
	var commit mockCommitData
	if err := json.Unmarshal(commitBytes, &commit); err != nil {
		return nil, nil, fmt.Errorf("mock: bad commit: %w", err)
	}
	state.Epoch = commit.NewEpoch
	state.TreeHash = commit.NewTreeHash
	state.PendingRefs = nil
	if commit.NewMembers != nil {
		state.Members = commit.NewMembers
	}
	newStateBytes, _ := json.Marshal(state)
	newTH, _ := hex.DecodeString(commit.NewTreeHash)
	return newStateBytes, newTH, nil
}

func (m *MockMLSEngine) ProcessWelcome(_ context.Context, welcomeBytes, _, _ []byte, _ uint32) ([]byte, []byte, uint64, error) {
	if err := m.popError(); err != nil {
		return nil, nil, 0, err
	}
	// Extract state from welcome (mocked as state bytes directly)
	var state mockGroupState
	if err := json.Unmarshal(welcomeBytes, &state); err != nil {
		// fallback
		return welcomeBytes, mockTreeHash(0), 1, nil
	}
	th, _ := hex.DecodeString(state.TreeHash)
	return welcomeBytes, th, state.Epoch, nil
}

func (m *MockMLSEngine) GenerateKeyPackage(_ context.Context, _ []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	return []byte("mock-key-package"), []byte("mock-kp-bundle-private"), nil
}

func (m *MockMLSEngine) AddMembers(_ context.Context, groupState []byte, targetIdentities [][]byte) ([]byte, []byte, []byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, nil, nil, err
	}
	var state mockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("mock: bad state: %w", err)
	}
	state.Epoch++
	newTH := mockTreeHash(state.Epoch)
	state.TreeHash = hex.EncodeToString(newTH)

	if state.Members == nil {
		state.Members = make(map[string]bool)
	}
	for _, id := range targetIdentities {
		state.Members[string(id)] = true
	}

	newStateBytes, _ := json.Marshal(state)
	commitInfo := mockCommitData{NewEpoch: state.Epoch, NewTreeHash: state.TreeHash, NewMembers: state.Members}
	commitBytes, _ := json.Marshal(commitInfo)
	return commitBytes, []byte("mock-welcome"), newStateBytes, newTH, nil
}

func (m *MockMLSEngine) RemoveMembers(_ context.Context, groupState []byte, targetIdentities [][]byte) ([]byte, []byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, nil, err
	}
	if len(targetIdentities) == 0 {
		return nil, nil, nil, fmt.Errorf("mock: no target identities")
	}
	var state mockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return nil, nil, nil, fmt.Errorf("mock: bad state: %w", err)
	}
	state.Epoch++
	newTH := mockTreeHash(state.Epoch)
	state.TreeHash = hex.EncodeToString(newTH)

	if state.Members == nil {
		state.Members = make(map[string]bool)
	}
	for _, id := range targetIdentities {
		delete(state.Members, string(id))
	}

	newStateBytes, _ := json.Marshal(state)
	commitInfo := mockCommitData{NewEpoch: state.Epoch, NewTreeHash: state.TreeHash, NewMembers: state.Members}
	commitBytes, _ := json.Marshal(commitInfo)
	return commitBytes, newStateBytes, newTH, nil
}

func (m *MockMLSEngine) HasMember(_ context.Context, groupState []byte, identity []byte) (bool, error) {
	if err := m.popError(); err != nil {
		return false, err
	}
	m.mu.Lock()
	fn := m.hasMemberFn
	m.mu.Unlock()
	if fn != nil {
		return fn(groupState, identity)
	}
	var state mockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return false, fmt.Errorf("mock: bad state in HasMember: %w", err)
	}
	if state.Members == nil {
		return len(identity) > 0, nil
	}
	return state.Members[string(identity)], nil
}

func (m *MockMLSEngine) ListMemberIdentities(_ context.Context, groupState []byte) ([][]byte, error) {
	if err := m.popError(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	fn := m.listMembersFn
	m.mu.Unlock()
	if fn != nil {
		return fn(groupState)
	}
	return nil, nil
}

func (m *MockMLSEngine) EncryptMessage(_ context.Context, groupState, plaintext []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	var state mockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		// Fallback for non-JSON groupState (legacy integration tests)
		return plaintext, groupState, nil
	}

	cipher := mockCiphertext{
		Epoch:     state.Epoch,
		Plaintext: plaintext,
	}
	cipherBytes, _ := json.Marshal(cipher)
	return cipherBytes, groupState, nil
}

func (m *MockMLSEngine) DecryptMessage(_ context.Context, groupState, ciphertext []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	var state mockGroupState
	isNewState := json.Unmarshal(groupState, &state) == nil

	var cipher mockCiphertext
	if err := json.Unmarshal(ciphertext, &cipher); err != nil {
		// Fallback for non-JSON ciphertext (legacy messages in tests)
		return ciphertext, groupState, nil
	}

	if isNewState && cipher.Epoch > state.Epoch {
		return nil, nil, fmt.Errorf("mock forward secrecy: message epoch %d > local epoch %d", cipher.Epoch, state.Epoch)
	}

	return cipher.Plaintext, groupState, nil
}

func (m *MockMLSEngine) ExternalJoin(_ context.Context, groupInfo, creatorKey []byte, _ uint32) ([]byte, []byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, nil, err
	}
	// In our mock, ExportGroupInfo returns "group-info:rt=X:<JSON_STATE>"
	data := groupInfo
	if bytes.HasPrefix(data, []byte("group-info:")) {
		parts := bytes.SplitN(data, []byte(":"), 3)
		if len(parts) == 3 {
			data = parts[2]
		}
	}

	var state mockGroupState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, nil, nil, fmt.Errorf("mock external join: bad group info: %w", err)
	}

	// External Join increases epoch by 1
	state.Epoch++
	newTH := mockTreeHash(state.Epoch)
	state.TreeHash = hex.EncodeToString(newTH)

	// Add the joiner (self) to members
	if state.Members == nil {
		state.Members = make(map[string]bool)
	}
	state.Members[string(deriveIdentityFromSigningKey(creatorKey))] = true

	newStateBytes, _ := json.Marshal(state)
	commitInfo := mockCommitData{
		NewEpoch:    state.Epoch,
		NewTreeHash: state.TreeHash,
		NewMembers:  state.Members,
	}
	commitBytes, _ := json.Marshal(commitInfo)

	return newStateBytes, commitBytes, newTH, nil
}

func (m *MockMLSEngine) ExportGroupInfo(_ context.Context, groupState []byte, withRatchetTree bool) ([]byte, error) {
	if err := m.popError(); err != nil {
		return nil, err
	}
	prefix := []byte("group-info:rt=0:")
	if withRatchetTree {
		prefix = []byte("group-info:rt=1:")
	}
	out := make([]byte, 0, len(prefix)+len(groupState))
	out = append(out, prefix...)
	out = append(out, groupState...)
	return out, nil
}

func (m *MockMLSEngine) ExportSecret(_ context.Context, _ []byte, label string, context []byte, length int) ([]byte, error) {
	if err := m.popError(); err != nil {
		return nil, err
	}
	h := sha256.Sum256(append(append([]byte(label+":"), context...), byte(length)))
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = h[i%32] ^ byte(i)
	}
	return out, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// MockStorage — in-memory CoordinationStorage
// ═══════════════════════════════════════════════════════════════════════════════

type MockStorage struct {
	mu       sync.Mutex
	groups   map[string]*GroupRecord
	coords   map[string]*CoordState
	messages []*StoredMessage

	envByGroup map[string][]*EnvelopeRecord
	appliedEnv map[string]map[string]struct{}
	syncAcks   map[string]map[string]int64 // groupID -> peerID -> ackedSeq
	pendingAck []PendingDeliveryAckRow
	pullCursor map[string]int64 // "groupID|remotePeerID"
	nextEnvID  map[string]int64 // groupID -> next seq
	healEvents []*ForkHealEventRecord
	healAudit  []*ForkHealAuditRecord
	pendingOps map[string]*PendingOperation // opID -> PendingOperation

	forkHealingJobs   map[string]*ForkHealingJob
	applicationEvents map[string]*ApplicationEvent // eventID -> Event
	outboundReplays   map[string]*OutboundReplay

	failApplyCommitOnce      bool
	failApplyApplicationOnce bool
}

var _ CoordinationStorage = (*MockStorage)(nil)

func NewMockStorage() *MockStorage {
	return &MockStorage{
		groups:            make(map[string]*GroupRecord),
		coords:            make(map[string]*CoordState),
		envByGroup:        make(map[string][]*EnvelopeRecord),
		appliedEnv:        make(map[string]map[string]struct{}),
		syncAcks:          make(map[string]map[string]int64),
		pullCursor:        make(map[string]int64),
		nextEnvID:         make(map[string]int64),
		pendingOps:        make(map[string]*PendingOperation),
		forkHealingJobs:   make(map[string]*ForkHealingJob),
		applicationEvents: make(map[string]*ApplicationEvent),
		outboundReplays:   make(map[string]*OutboundReplay),
	}
}

func (s *MockStorage) FailNextApplyCommit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failApplyCommitOnce = true
}

func (s *MockStorage) FailNextApplyApplication() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failApplyApplicationOnce = true
}

func (s *MockStorage) GetGroupRecord(groupID string) (*GroupRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.groups[groupID]
	if !ok {
		return nil, ErrGroupNotFound
	}
	return rec, nil
}

func (s *MockStorage) SaveGroupRecord(rec *GroupRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[rec.GroupID] = rec
	return nil
}

func (s *MockStorage) ListGroups() ([]*GroupRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*GroupRecord, 0, len(s.groups))
	for _, rec := range s.groups {
		out = append(out, rec)
	}
	return out, nil
}

func (s *MockStorage) GetCoordState(groupID string) (*CoordState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, ok := s.coords[groupID]
	if !ok {
		return nil, ErrGroupNotFound
	}
	return cs, nil
}

func (s *MockStorage) SaveCoordState(state *CoordState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coords[state.GroupID] = state
	return nil
}

func (s *MockStorage) SaveMessage(msg *StoredMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(msg.EnvelopeHash) > 0 {
		for _, existing := range s.messages {
			if msg.GroupID == existing.GroupID && bytes.Equal(msg.EnvelopeHash, existing.EnvelopeHash) {
				return nil
			}
		}
	}
	s.messages = append(s.messages, msg)
	return nil
}

func (s *MockStorage) MarkMessageReplayed(groupID string, envelopeHash []byte, now time.Time) error {
	if len(envelopeHash) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nowMs := now.UnixMilli()
	for _, msg := range s.messages {
		if msg.GroupID == groupID && bytes.Equal(msg.EnvelopeHash, envelopeHash) {
			msg.ReplayedAt = &nowMs
			return nil
		}
	}
	return nil
}

func (s *MockStorage) GetMessagesByOwnerInRange(groupID, senderID string, startMs, endMs int64) ([]*StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*StoredMessage
	for _, msg := range s.messages {
		if msg.GroupID != groupID {
			continue
		}
		if msg.SenderID.String() != senderID {
			continue
		}
		if msg.Timestamp.WallTimeMs < startMs || msg.Timestamp.WallTimeMs > endMs {
			continue
		}
		out = append(out, msg)
	}
	return out, nil
}

func (s *MockStorage) HasAppliedEnvelope(groupID string, envelopeHash []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(envelopeHash) == 0 || s.appliedEnv[groupID] == nil {
		return false, nil
	}
	_, ok := s.appliedEnv[groupID][string(envelopeHash)]
	return ok, nil
}

func (s *MockStorage) ClearAppliedEnvelope(groupID string, envelopeHash []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(envelopeHash) == 0 || s.appliedEnv[groupID] == nil {
		return
	}
	delete(s.appliedEnv[groupID], string(envelopeHash))
}

func (s *MockStorage) MarkEnvelopeApplied(groupID string, msgType MessageType, epoch uint64, envelopeHash []byte) error {
	_ = msgType
	_ = epoch
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(envelopeHash) == 0 {
		return nil
	}
	if s.appliedEnv[groupID] == nil {
		s.appliedEnv[groupID] = make(map[string]struct{})
	}
	s.appliedEnv[groupID][string(envelopeHash)] = struct{}{}
	for _, rec := range s.envByGroup[groupID] {
		if bytes.Equal(rec.EnvelopeHash, envelopeHash) {
			rec.ApplyState = string(ReplayStateApplied)
			rec.AppliedAt = time.Now().Unix()
		}
	}
	return nil
}

func (s *MockStorage) MarkEnvelopeReplayState(groupID string, envelopeHash []byte, state ReplayEnvelopeState, lastErr string, now time.Time) error {
	if len(envelopeHash) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range s.envByGroup[groupID] {
		if bytes.Equal(rec.EnvelopeHash, envelopeHash) {
			rec.ApplyState = string(state)
			rec.LastApplyError = lastErr
			rec.LastApplyAttemptAt = now.Unix()
			if state == ReplayStateApplied || state == ReplayStateDuplicateApplied {
				rec.AppliedAt = now.Unix()
			}
		}
	}
	return nil
}

func (s *MockStorage) ApplyCommit(rec *GroupRecord, msgType MessageType, envelope []byte, ts HLCTimestamp, envEpoch uint64) (bool, int64, error) {
	hash := sha256.Sum256(envelope)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failApplyCommitOnce {
		s.failApplyCommitOnce = false
		return false, 0, fmt.Errorf("mock apply commit failure")
	}
	if s.appliedEnv[rec.GroupID] == nil {
		s.appliedEnv[rec.GroupID] = make(map[string]struct{})
	}
	key := string(hash[:])
	if _, ok := s.appliedEnv[rec.GroupID][key]; ok {
		return false, 0, nil
	}
	s.groups[rec.GroupID] = rec
	s.appliedEnv[rec.GroupID][key] = struct{}{}
	s.nextEnvID[rec.GroupID]++
	seq := s.nextEnvID[rec.GroupID]
	s.envByGroup[rec.GroupID] = append(s.envByGroup[rec.GroupID], &EnvelopeRecord{
		Seq: seq, GroupID: rec.GroupID, MsgType: msgType, Epoch: envEpoch, Envelope: envelope, Timestamp: ts,
		EnvelopeHash: hash[:], SourcePath: "local", ApplyState: string(ReplayStateApplied), AppliedAt: time.Now().Unix(),
		FirstSeenAtMs: time.Now().UnixMilli(), ReceivedAtMs: time.Now().UnixMilli(),
	})
	return true, seq, nil
}

func (s *MockStorage) ApplyApplication(rec *GroupRecord, msg *StoredMessage, msgType MessageType, envelope []byte, ts HLCTimestamp, envEpoch uint64) (bool, int64, error) {
	hash := sha256.Sum256(envelope)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failApplyApplicationOnce {
		s.failApplyApplicationOnce = false
		return false, 0, fmt.Errorf("mock apply application failure")
	}
	if s.appliedEnv[rec.GroupID] == nil {
		s.appliedEnv[rec.GroupID] = make(map[string]struct{})
	}
	key := string(hash[:])
	if _, ok := s.appliedEnv[rec.GroupID][key]; ok {
		return false, 0, nil
	}
	s.groups[rec.GroupID] = rec
	s.messages = append(s.messages, msg)
	s.appliedEnv[rec.GroupID][key] = struct{}{}
	s.nextEnvID[rec.GroupID]++
	seq := s.nextEnvID[rec.GroupID]
	s.envByGroup[rec.GroupID] = append(s.envByGroup[rec.GroupID], &EnvelopeRecord{
		Seq: seq, GroupID: rec.GroupID, MsgType: msgType, Epoch: envEpoch, Envelope: envelope, Timestamp: ts,
		EnvelopeHash: hash[:], SourcePath: "local", ApplyState: string(ReplayStateApplied), AppliedAt: time.Now().Unix(),
		FirstSeenAtMs: time.Now().UnixMilli(), ReceivedAtMs: time.Now().UnixMilli(),
	})
	return true, seq, nil
}

func (s *MockStorage) GetMessagesSince(groupID string, after HLCTimestamp) ([]*StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*StoredMessage
	for _, msg := range s.messages {
		if msg.GroupID == groupID && after.Before(msg.Timestamp) {
			out = append(out, msg)
		}
	}
	// Sort by HLC ASC to match causal order
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out, nil
}

func (s *MockStorage) GetMessagesPaginated(groupID string, limit, offset int) ([]*StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*StoredMessage
	for _, msg := range s.messages {
		if msg.GroupID == groupID {
			out = append(out, msg)
		}
	}
	// Sort by timestamp DESC to match production SQLite behavior
	// (Actually MockStorage stores in insertion order, which is roughly ASC)
	// For tests, we'll just reverse the order and apply offset/limit.
	// But wait, scanMessages in production handles the sorting via SQL ORDER BY.

	// A simple mock pagination:
	if offset >= len(out) {
		return nil, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (s *MockStorage) GetPostsPaginated(groupID string, limit, offset int) ([]*StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*StoredMessage
	for _, msg := range s.messages {
		if msg.GroupID != groupID {
			continue
		}
		var content map[string]interface{}
		if err := json.Unmarshal(msg.Content, &content); err == nil {
			if t, ok := content["type"].(string); ok && t == "post" {
				out = append(out, msg)
			}
		}
	}
	if offset >= len(out) {
		return nil, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (s *MockStorage) GetCommentsPaginated(groupID, postID string, limit, offset int) ([]*StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*StoredMessage
	for _, msg := range s.messages {
		if msg.GroupID != groupID {
			continue
		}
		var content map[string]interface{}
		if err := json.Unmarshal(msg.Content, &content); err == nil {
			msgType, _ := content["type"].(string)
			if msgType == "comment" || msgType == "reply" {
				pID, _ := content["post_id"].(string)
				parID, _ := content["parent_id"].(string)
				if pID == postID || parID == postID {
					out = append(out, msg)
				}
			}
		}
	}
	if offset >= len(out) {
		return nil, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

// Messages returns all stored messages (test helper).
func (s *MockStorage) Messages() []*StoredMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*StoredMessage, len(s.messages))
	copy(cp, s.messages)
	return cp
}

func (s *MockStorage) AppendEnvelope(groupID string, msgType MessageType, epoch uint64, ts HLCTimestamp, envelope []byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := sha256.Sum256(envelope)
	for _, r := range s.envByGroup[groupID] {
		if bytes.Equal(r.EnvelopeHash, hash[:]) {
			r.ReceivedAtMs = time.Now().UnixMilli()
			return 0, nil
		}
	}
	s.nextEnvID[groupID]++
	seq := s.nextEnvID[groupID]
	rec := &EnvelopeRecord{
		Seq: seq, GroupID: groupID, MsgType: msgType, Epoch: epoch,
		Envelope: envelope, Timestamp: ts, EnvelopeHash: hash[:],
		SourcePath: "local", ApplyState: "pending",
		FirstSeenAtMs: time.Now().UnixMilli(), ReceivedAtMs: time.Now().UnixMilli(),
	}
	s.envByGroup[groupID] = append(s.envByGroup[groupID], rec)
	return seq, nil
}

func (s *MockStorage) AppendEnvelopeWithSource(groupID string, msgType MessageType, epoch uint64, ts HLCTimestamp, envelope []byte, sourcePath string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := sha256.Sum256(envelope)
	for _, r := range s.envByGroup[groupID] {
		if bytes.Equal(r.EnvelopeHash, hash[:]) {
			r.ReceivedAtMs = time.Now().UnixMilli()
			return 0, nil
		}
	}
	s.nextEnvID[groupID]++
	seq := s.nextEnvID[groupID]
	rec := &EnvelopeRecord{
		Seq: seq, GroupID: groupID, MsgType: msgType, Epoch: epoch,
		Envelope: envelope, Timestamp: ts, EnvelopeHash: hash[:],
		SourcePath: sourcePath, ApplyState: "pending",
		FirstSeenAtMs: time.Now().UnixMilli(), ReceivedAtMs: time.Now().UnixMilli(),
	}
	s.envByGroup[groupID] = append(s.envByGroup[groupID], rec)
	return seq, nil
}

func (s *MockStorage) GetEnvelopesSince(groupID string, afterSeq int64, maxCount int) ([]*EnvelopeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxCount < 1 {
		maxCount = 50
	}
	var out []*EnvelopeRecord
	for _, r := range s.envByGroup[groupID] {
		if r.Seq <= afterSeq {
			continue
		}
		out = append(out, r)
		if len(out) >= maxCount {
			break
		}
	}
	return out, nil
}

func (s *MockStorage) GetEnvelope(envelopeHash []byte) (*EnvelopeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, envs := range s.envByGroup {
		for _, r := range envs {
			if bytes.Equal(r.EnvelopeHash, envelopeHash) {
				return r, nil
			}
		}
	}
	return nil, nil
}

func (s *MockStorage) GetPendingEnvelopes(groupID string, maxCount int) ([]*EnvelopeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxCount < 1 {
		maxCount = 100
	}
	var out []*EnvelopeRecord
	for _, r := range s.envByGroup[groupID] {
		if IsPendingApplyState(string(r.ApplyState)) {
			out = append(out, r)
			if len(out) >= maxCount {
				break
			}
		}
	}
	return out, nil
}

func (s *MockStorage) GetLatestSeq(groupID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextEnvID[groupID], nil
}

func (s *MockStorage) PruneEnvelopes(cutoffUnix int64, maxPerGroup int) (int, error) {
	_ = cutoffUnix
	_ = maxPerGroup
	return 0, nil
}

func (s *MockStorage) RecordSyncAck(peerID, groupID string, ackedSeq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncAcks[groupID] == nil {
		s.syncAcks[groupID] = make(map[string]int64)
	}
	if ackedSeq > s.syncAcks[groupID][peerID] {
		s.syncAcks[groupID][peerID] = ackedSeq
	}
	return nil
}

func (s *MockStorage) GetSyncAck(peerID, groupID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncAcks[groupID] == nil {
		return 0, nil
	}
	return s.syncAcks[groupID][peerID], nil
}

func (s *MockStorage) GetMinAckedSeq(groupID string, peerIDs []string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(peerIDs) == 0 {
		return 0, nil
	}
	var min int64 = -1
	for _, pid := range peerIDs {
		ack := int64(0)
		if s.syncAcks[groupID] != nil {
			ack = s.syncAcks[groupID][pid]
		}
		if min < 0 || ack < min {
			min = ack
		}
	}
	if min < 0 {
		return 0, nil
	}
	return min, nil
}

func (s *MockStorage) EnqueuePendingDeliveryAck(targetPeerID, groupID string, ackedSeq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingAck = append(s.pendingAck, PendingDeliveryAckRow{
		ID: int64(len(s.pendingAck) + 1), TargetPeerID: targetPeerID, GroupID: groupID, AckedSeq: ackedSeq,
	})
	return nil
}

func (s *MockStorage) ListPendingDeliveryAcksForTarget(targetPeerID string) ([]PendingDeliveryAckRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []PendingDeliveryAckRow
	for _, r := range s.pendingAck {
		if r.TargetPeerID == targetPeerID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *MockStorage) DeletePendingDeliveryAck(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []PendingDeliveryAckRow
	for _, r := range s.pendingAck {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	s.pendingAck = kept
	return nil
}

func (s *MockStorage) GetOfflinePullCursor(groupID, remotePeerID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pullCursor[groupID+"|"+remotePeerID], nil
}

func (s *MockStorage) SetOfflinePullCursor(groupID, remotePeerID string, lastRemoteSeq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pullCursor[groupID+"|"+remotePeerID] = lastRemoteSeq
	return nil
}

func (s *MockStorage) GetKnownGroupMembers(groupID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{})
	for _, msg := range s.messages {
		if msg.GroupID == groupID {
			seen[msg.SenderID.String()] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out, nil
}

func (s *MockStorage) RecordForkHealEvent(event *ForkHealEventRecord) error {
	if event == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *event
	cp.WinnerTreeHash = append([]byte(nil), event.WinnerTreeHash...)
	cp.NewTreeHash = append([]byte(nil), event.NewTreeHash...)
	s.healEvents = append(s.healEvents, &cp)
	return nil
}

func (s *MockStorage) RecordForkHealAudit(audit *ForkHealAuditRecord) error {
	if audit == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *audit
	s.healAudit = append(s.healAudit, &cp)
	return nil
}

func (s *MockStorage) ListForkHealEvents(groupID string, limit int) ([]*ForkHealEventRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*ForkHealEventRecord
	for i := len(s.healEvents) - 1; i >= 0; i-- {
		ev := s.healEvents[i]
		if groupID != "" && ev.GroupID != groupID {
			continue
		}
		cp := *ev
		cp.WinnerTreeHash = append([]byte(nil), ev.WinnerTreeHash...)
		cp.NewTreeHash = append([]byte(nil), ev.NewTreeHash...)
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *MockStorage) ListForkHealAudit(traceID string) ([]*ForkHealAuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*ForkHealAuditRecord
	for _, row := range s.healAudit {
		if traceID != "" && row.TraceID != traceID {
			continue
		}
		cp := *row
		out = append(out, &cp)
	}
	return out, nil
}

func (s *MockStorage) PruneForkHealHistory(cutoffUnix int64, maxPerGroup int) (int, error) {
	_ = cutoffUnix
	_ = maxPerGroup
	return 0, nil
}

func (s *MockStorage) SavePendingOperation(op *PendingOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *op
	s.pendingOps[op.OperationID] = &cp
	return nil
}

func (s *MockStorage) GetPendingOperation(opID string) (*PendingOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.pendingOps[opID]
	if !ok {
		return nil, fmt.Errorf("sql: no rows in result set")
	}
	cp := *op
	return &cp, nil
}

func (s *MockStorage) ListPendingOperations(groupID string) ([]*PendingOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*PendingOperation
	for _, op := range s.pendingOps {
		if op.GroupID == groupID {
			cp := *op
			list = append(list, &cp)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list, nil
}

func (s *MockStorage) GetPendingOperationByIdempotencyKey(groupID string, idempotencyKey string) (*PendingOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, op := range s.pendingOps {
		if op.GroupID == groupID && op.IdempotencyKey != nil && *op.IdempotencyKey == idempotencyKey {
			cp := *op
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("sql: no rows in result set")
}

func (s *MockStorage) DeletePendingOperation(opID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pendingOps, opID)
	return nil
}

func (s *MockStorage) SaveForkHealingJob(job *ForkHealingJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *job
	s.forkHealingJobs[job.JobID] = &cp
	return nil
}

func (s *MockStorage) GetActiveForkHealingJob(groupID string) (*ForkHealingJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *ForkHealingJob
	for _, job := range s.forkHealingJobs {
		if job.GroupID == groupID && job.Status != "CLEANED" && job.Status != "FAILED_TERMINAL" {
			if latest == nil || job.CreatedAtMs > latest.CreatedAtMs {
				latest = job
			}
		}
	}
	if latest == nil {
		return nil, nil
	}
	cp := *latest
	return &cp, nil
}

func (s *MockStorage) GetForkHealingJobByID(jobID string) (*ForkHealingJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.forkHealingJobs[jobID]
	if !ok {
		return nil, nil
	}
	cp := *job
	return &cp, nil
}

func (s *MockStorage) DeleteForkHealingJob(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.forkHealingJobs, jobID)
	return nil
}

func (s *MockStorage) SaveApplicationEvent(ev *ApplicationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *ev
	s.applicationEvents[ev.EventID] = &cp
	return nil
}

func (s *MockStorage) ListApplicationEvents(jobID string) ([]*ApplicationEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*ApplicationEvent
	for _, ev := range s.applicationEvents {
		if ev.JobID == jobID {
			cp := *ev
			list = append(list, &cp)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].HlcWallTimeMs != list[j].HlcWallTimeMs {
			return list[i].HlcWallTimeMs < list[j].HlcWallTimeMs
		}
		return list[i].HlcCounter < list[j].HlcCounter
	})
	return list, nil
}

func (s *MockStorage) UpdateApplicationEventStatus(eventID string, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev, ok := s.applicationEvents[eventID]
	if !ok {
		return fmt.Errorf("sql: no rows in result set")
	}
	ev.Status = status
	return nil
}

func (s *MockStorage) ClearSealedPayloads(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.applicationEvents {
		if ev.JobID == jobID && (ev.Status == "REPLAYED" || ev.Status == "WAITING_AUTHOR_REPLAY" || ev.Status == "UNRECOVERABLE") {
			ev.PayloadSealed = nil
			ev.SealNonce = nil
			ev.SealKeyID = ""
		}
	}
	return nil
}

func (s *MockStorage) SaveOutboundReplay(req *OutboundReplay) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *req
	s.outboundReplays[req.ReplayOperationID] = &cp
	return nil
}

func (s *MockStorage) ListOutboundReplays(jobID string) ([]*OutboundReplay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*OutboundReplay
	for _, req := range s.outboundReplays {
		if req.JobID == jobID {
			cp := *req
			list = append(list, &cp)
		}
	}
	return list, nil
}

func (s *MockStorage) DeleteOutboundReplaysForJob(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, req := range s.outboundReplays {
		if req.JobID == jobID {
			delete(s.outboundReplays, id)
		}
	}
	return nil
}
