//go:build business_integration

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"app/coordination"
)

// businessIntegrationMLSMock is a coordination.MLSEngine implementation for
// business_integration tests. It mirrors coordination.MockMLSEngine from
// coordination/testutil_test.go (not importable from service) with two fixes:
//   - AddMembers returns Welcome bytes that are valid JSON group state (same as
//     newGroupState) so JoinGroupWithWelcome → ProcessWelcome persists usable state.
//   - ProcessWelcome unmarshals that JSON and returns epoch/tree hash consistently.

type bizMockGroupState struct {
	GroupID  string `json:"group_id"`
	Epoch    uint64 `json:"epoch"`
	TreeHash string `json:"tree_hash"`
}

type bizMockCommitData struct {
	NewEpoch    uint64 `json:"new_epoch"`
	NewTreeHash string `json:"new_tree_hash"`
}

type businessIntegrationMLSMock struct {
	mu            sync.Mutex
	nextErr       error
	hasMemberFn   func(groupState []byte, identity []byte) (bool, error)
	listMembersFn func(groupState []byte) ([][]byte, error)
}

func newBusinessIntegrationMLSMock() *businessIntegrationMLSMock {
	return &businessIntegrationMLSMock{}
}

func (m *businessIntegrationMLSMock) SetNextError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextErr = err
}

func (m *businessIntegrationMLSMock) popError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.nextErr
	m.nextErr = nil
	return err
}

func (m *businessIntegrationMLSMock) SetHasMemberFunc(fn func(groupState []byte, identity []byte) (bool, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hasMemberFn = fn
}

func (m *businessIntegrationMLSMock) SetListMembersFunc(fn func(groupState []byte) ([][]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listMembersFn = fn
}

func bizMockTreeHash(epoch uint64) []byte {
	h := sha256.Sum256([]byte(fmt.Sprintf("tree-epoch-%d", epoch)))
	return h[:]
}

func cloneBytesListForService(in [][]byte) [][]byte {
	if len(in) == 0 {
		return nil
	}
	out := make([][]byte, len(in))
	for i := range in {
		out[i] = append([]byte(nil), in[i]...)
	}
	return out
}

var _ coordination.MLSEngine = (*businessIntegrationMLSMock)(nil)

func (m *businessIntegrationMLSMock) CreateGroup(_ context.Context, groupID string, _ []byte, _ uint32) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	th := bizMockTreeHash(0)
	state := bizMockGroupState{GroupID: groupID, Epoch: 0, TreeHash: hex.EncodeToString(th)}
	stateBytes, _ := json.Marshal(state)
	return stateBytes, th, nil
}

func (m *businessIntegrationMLSMock) CreateProposal(_ context.Context, groupState []byte, _ coordination.ProposalType, data []byte) (coordination.CreateProposalResult, error) {
	if err := m.popError(); err != nil {
		return coordination.CreateProposalResult{}, err
	}
	cp := append([]byte(nil), data...)
	sum := sha256.Sum256(cp)
	return coordination.CreateProposalResult{
		ProposalBytes: cp,
		ProposalRef:   sum[:],
		NewGroupState: append([]byte(nil), groupState...),
	}, nil
}

func (m *businessIntegrationMLSMock) ProcessProposal(_ context.Context, groupState []byte, proposalBytes []byte) (coordination.ProcessProposalResult, error) {
	if err := m.popError(); err != nil {
		return coordination.ProcessProposalResult{}, err
	}
	sum := sha256.Sum256(proposalBytes)
	return coordination.ProcessProposalResult{
		ProposalRef:   sum[:],
		ProposalType:  "Mock",
		NewGroupState: append([]byte(nil), groupState...),
	}, nil
}

func (m *businessIntegrationMLSMock) CreateCommit(_ context.Context, groupState []byte, expectedProposalRefs [][]byte) (coordination.CreateCommitResult, error) {
	if err := m.popError(); err != nil {
		return coordination.CreateCommitResult{}, err
	}
	var state bizMockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return coordination.CreateCommitResult{}, fmt.Errorf("mock: bad state: %w", err)
	}
	state.Epoch++
	newTH := bizMockTreeHash(state.Epoch)
	state.TreeHash = hex.EncodeToString(newTH)

	newStateBytes, _ := json.Marshal(state)
	commitInfo := bizMockCommitData{NewEpoch: state.Epoch, NewTreeHash: state.TreeHash}
	commitBytes, _ := json.Marshal(commitInfo)
	return coordination.CreateCommitResult{
		CommitBytes:           commitBytes,
		CommittedProposalRefs: cloneBytesListForService(expectedProposalRefs),
		NewGroupState:         newStateBytes,
		NewTreeHash:           newTH,
	}, nil
}

func (m *businessIntegrationMLSMock) StageCommit(_ context.Context, _ []byte, commitBytes []byte, _ [][]byte) (coordination.StageCommitResult, error) {
	if err := m.popError(); err != nil {
		return coordination.StageCommitResult{}, err
	}
	var commit bizMockCommitData
	if err := json.Unmarshal(commitBytes, &commit); err != nil {
		return coordination.StageCommitResult{}, fmt.Errorf("mock: bad commit: %w", err)
	}
	return coordination.StageCommitResult{Epoch: commit.NewEpoch}, nil
}

func (m *businessIntegrationMLSMock) ProcessCommit(_ context.Context, groupState, commitBytes []byte, _ [][]byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	var state bizMockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return nil, nil, fmt.Errorf("mock: bad state: %w", err)
	}
	var commit bizMockCommitData
	if err := json.Unmarshal(commitBytes, &commit); err != nil {
		return nil, nil, fmt.Errorf("mock: bad commit: %w", err)
	}
	state.Epoch = commit.NewEpoch
	state.TreeHash = commit.NewTreeHash
	newStateBytes, _ := json.Marshal(state)
	newTH, _ := hex.DecodeString(commit.NewTreeHash)
	return newStateBytes, newTH, nil
}

func (m *businessIntegrationMLSMock) ProcessWelcome(_ context.Context, welcomeBytes, _, _ []byte, _ uint32) ([]byte, []byte, uint64, error) {
	if err := m.popError(); err != nil {
		return nil, nil, 0, err
	}
	var state bizMockGroupState
	if err := json.Unmarshal(welcomeBytes, &state); err != nil {
		return nil, nil, 0, fmt.Errorf("mock: welcome is not valid group state JSON: %w", err)
	}
	th, err := hex.DecodeString(state.TreeHash)
	if err != nil || len(th) != 32 {
		th = bizMockTreeHash(state.Epoch)
	}
	outState := append([]byte(nil), welcomeBytes...)
	return outState, th, state.Epoch, nil
}

func (m *businessIntegrationMLSMock) GenerateKeyPackage(_ context.Context, _ []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	return []byte("mock-key-package"), []byte("mock-kp-bundle-private"), nil
}

func (m *businessIntegrationMLSMock) AddMembers(_ context.Context, groupState []byte, _ [][]byte) ([]byte, []byte, []byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, nil, nil, err
	}
	var state bizMockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("mock: bad state: %w", err)
	}
	state.Epoch++
	newTH := bizMockTreeHash(state.Epoch)
	state.TreeHash = hex.EncodeToString(newTH)
	newStateBytes, _ := json.Marshal(state)
	commitInfo := bizMockCommitData{NewEpoch: state.Epoch, NewTreeHash: state.TreeHash}
	commitBytes, _ := json.Marshal(commitInfo)
	welcomeBytes := append([]byte(nil), newStateBytes...)
	return commitBytes, welcomeBytes, newStateBytes, newTH, nil
}

func (m *businessIntegrationMLSMock) RemoveMembers(_ context.Context, groupState []byte, targetIdentities [][]byte) ([]byte, []byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, nil, err
	}
	if len(targetIdentities) == 0 {
		return nil, nil, nil, fmt.Errorf("mock: no target identities")
	}
	var state bizMockGroupState
	if err := json.Unmarshal(groupState, &state); err != nil {
		return nil, nil, nil, fmt.Errorf("mock: bad state: %w", err)
	}
	state.Epoch++
	newTH := bizMockTreeHash(state.Epoch)
	state.TreeHash = hex.EncodeToString(newTH)
	newStateBytes, _ := json.Marshal(state)
	commitInfo := bizMockCommitData{NewEpoch: state.Epoch, NewTreeHash: state.TreeHash}
	commitBytes, _ := json.Marshal(commitInfo)
	return commitBytes, newStateBytes, newTH, nil
}

func (m *businessIntegrationMLSMock) HasMember(_ context.Context, groupState []byte, identity []byte) (bool, error) {
	if err := m.popError(); err != nil {
		return false, err
	}
	m.mu.Lock()
	fn := m.hasMemberFn
	m.mu.Unlock()
	if fn != nil {
		return fn(groupState, identity)
	}
	return len(identity) > 0, nil
}

func (m *businessIntegrationMLSMock) ListMemberIdentities(_ context.Context, groupState []byte) ([][]byte, error) {
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

func (m *businessIntegrationMLSMock) EncryptMessage(_ context.Context, groupState, plaintext []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	return plaintext, groupState, nil
}

func (m *businessIntegrationMLSMock) DecryptMessage(_ context.Context, groupState, ciphertext []byte) ([]byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, err
	}
	return ciphertext, groupState, nil
}

func (m *businessIntegrationMLSMock) ExternalJoin(_ context.Context, groupInfo, _ []byte, _ uint32) ([]byte, []byte, []byte, error) {
	if err := m.popError(); err != nil {
		return nil, nil, nil, err
	}
	newTH := bizMockTreeHash(1)
	commitInfo := bizMockCommitData{NewEpoch: 1, NewTreeHash: hex.EncodeToString(newTH)}
	commitBytes, _ := json.Marshal(commitInfo)
	return groupInfo, commitBytes, newTH, nil
}

func (m *businessIntegrationMLSMock) ExportGroupInfo(_ context.Context, groupState []byte, withRatchetTree bool) ([]byte, error) {
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

func (m *businessIntegrationMLSMock) ExportSecret(_ context.Context, _ []byte, label string, context []byte, length int) ([]byte, error) {
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
