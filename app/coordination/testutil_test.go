package coordination

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

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

func (ft *FakeTransport) SendDirect(_ context.Context, _ peer.ID, _ []byte) error {
	return nil // not used in coordinator tests yet
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
	GroupID  string `json:"group_id"`
	Epoch    uint64 `json:"epoch"`
	TreeHash string `json:"tree_hash"`
}

type mockCommitData struct {
	NewEpoch    uint64 `json:"new_epoch"`
	NewTreeHash string `json:"new_tree_hash"`
}

type MockMLSEngine struct {
	mu      sync.Mutex
	nextErr error
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

func mockTreeHash(epoch uint64) []byte {
	h := sha256.Sum256([]byte(fmt.Sprintf("tree-epoch-%d", epoch)))
	return h[:]
}

func (m *MockMLSEngine) CreateGroup(_ context.Context, groupID string, _ []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	th := mockTreeHash(0)
	state := mockGroupState{GroupID: groupID, Epoch: 0, TreeHash: hex.EncodeToString(th)}
	stateBytes, _ := json.Marshal(state)
	return stateBytes, th, nil
}

func (m *MockMLSEngine) CreateProposal(_ context.Context, _ []byte, _ ProposalType, data []byte) ([]byte, error) {
	if err := m.popError(); err != nil {
		return nil, err
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func (m *MockMLSEngine) CreateCommit(_ context.Context, groupState []byte, _ [][]byte) ([]byte, []byte, []byte, []byte, error) {
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

	newStateBytes, _ := json.Marshal(state)
	commitInfo := mockCommitData{NewEpoch: state.Epoch, NewTreeHash: state.TreeHash}
	commitBytes, _ := json.Marshal(commitInfo)
	return commitBytes, nil, newStateBytes, newTH, nil
}

func (m *MockMLSEngine) ProcessCommit(_ context.Context, groupState, commitBytes []byte) ([]byte, []byte, error) {
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
	newStateBytes, _ := json.Marshal(state)
	newTH, _ := hex.DecodeString(commit.NewTreeHash)
	return newStateBytes, newTH, nil
}

func (m *MockMLSEngine) ProcessWelcome(_ context.Context, welcomeBytes, _, _ []byte) ([]byte, []byte, uint64, error) {
	if err := m.popError(); err != nil {
		return nil, nil, 0, err
	}
	return welcomeBytes, mockTreeHash(0), 1, nil
}

func (m *MockMLSEngine) GenerateKeyPackage(_ context.Context, _ []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	return []byte("mock-key-package"), []byte("mock-kp-bundle-private"), nil
}

func (m *MockMLSEngine) AddMembers(_ context.Context, groupState []byte, _ [][]byte) ([]byte, []byte, []byte, []byte, error) {
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
	newStateBytes, _ := json.Marshal(state)
	commitInfo := mockCommitData{NewEpoch: state.Epoch, NewTreeHash: state.TreeHash}
	commitBytes, _ := json.Marshal(commitInfo)
	return commitBytes, []byte("mock-welcome"), newStateBytes, newTH, nil
}

func (m *MockMLSEngine) EncryptMessage(_ context.Context, groupState, plaintext []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	return plaintext, groupState, nil // identity: ciphertext == plaintext
}

func (m *MockMLSEngine) DecryptMessage(_ context.Context, groupState, ciphertext []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	return ciphertext, groupState, nil // identity: plaintext == ciphertext
}

func (m *MockMLSEngine) ExternalJoin(_ context.Context, groupInfo, _ []byte) ([]byte, []byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, nil, err
	}
	return groupInfo, []byte("ext-commit"), mockTreeHash(0), nil
}

func (m *MockMLSEngine) ExportSecret(_ context.Context, _ []byte, label string, length int) ([]byte, error) {
	if err := m.popError(); err != nil {
		return nil, err
	}
	h := sha256.Sum256([]byte(label))
	if length > len(h) {
		length = len(h)
	}
	return h[:length], nil
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

	failApplyCommitOnce      bool
	failApplyApplicationOnce bool
}

var _ CoordinationStorage = (*MockStorage)(nil)

func NewMockStorage() *MockStorage {
	return &MockStorage{
		groups:     make(map[string]*GroupRecord),
		coords:     make(map[string]*CoordState),
		envByGroup: make(map[string][]*EnvelopeRecord),
		appliedEnv: make(map[string]map[string]struct{}),
		syncAcks:   make(map[string]map[string]int64),
		pullCursor: make(map[string]int64),
		nextEnvID:  make(map[string]int64),
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

func (s *MockStorage) HasAppliedEnvelope(groupID string, envelopeHash []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(envelopeHash) == 0 || s.appliedEnv[groupID] == nil {
		return false, nil
	}
	_, ok := s.appliedEnv[groupID][string(envelopeHash)]
	return ok, nil
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
	return nil
}

func (s *MockStorage) ApplyCommit(rec *GroupRecord, msgType MessageType, envelope []byte, ts HLCTimestamp) (bool, int64, error) {
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
		Seq: seq, GroupID: rec.GroupID, MsgType: msgType, Epoch: rec.Epoch, Envelope: envelope, Timestamp: ts,
	})
	return true, seq, nil
}

func (s *MockStorage) ApplyApplication(rec *GroupRecord, msg *StoredMessage, msgType MessageType, envelope []byte, ts HLCTimestamp) (bool, int64, error) {
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
		Seq: seq, GroupID: rec.GroupID, MsgType: msgType, Epoch: rec.Epoch, Envelope: envelope, Timestamp: ts,
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
	return out, nil
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
	s.nextEnvID[groupID]++
	seq := s.nextEnvID[groupID]
	rec := &EnvelopeRecord{
		Seq: seq, GroupID: groupID, MsgType: msgType, Epoch: epoch,
		Envelope: envelope, Timestamp: ts,
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
