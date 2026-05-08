package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"app/coordination"
)

type groupInfoTestMLSEngine struct {
	exportFn func(groupState []byte, withRatchetTree bool) ([]byte, error)
}

func (m *groupInfoTestMLSEngine) CreateGroup(context.Context, string, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) CreateProposal(context.Context, []byte, coordination.ProposalType, []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) CreateCommit(context.Context, []byte, [][]byte) ([]byte, []byte, []byte, []byte, error) {
	return nil, nil, nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) ProcessCommit(context.Context, []byte, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) ProcessWelcome(context.Context, []byte, []byte, []byte) ([]byte, []byte, uint64, error) {
	return nil, nil, 0, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) GenerateKeyPackage(context.Context, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) AddMembers(context.Context, []byte, [][]byte) ([]byte, []byte, []byte, []byte, error) {
	return nil, nil, nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) RemoveMembers(context.Context, []byte, [][]byte) ([]byte, []byte, []byte, error) {
	return nil, nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) HasMember(context.Context, []byte, []byte) (bool, error) {
	return false, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) EncryptMessage(context.Context, []byte, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) DecryptMessage(context.Context, []byte, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) ExternalJoin(context.Context, []byte, []byte) ([]byte, []byte, []byte, error) {
	return nil, nil, nil, errors.New("not implemented")
}
func (m *groupInfoTestMLSEngine) ExportGroupInfo(_ context.Context, groupState []byte, withRatchetTree bool) ([]byte, error) {
	if m.exportFn == nil {
		return nil, errors.New("export not configured")
	}
	return m.exportFn(groupState, withRatchetTree)
}
func (m *groupInfoTestMLSEngine) ExportSecret(context.Context, []byte, string, int) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func TestExportLocalGroupInfo_Success(t *testing.T) {
	rt := setupMembershipRuntime(t)
	coord := &coordination.Coordinator{}
	coord.InitializeGroup([]byte("state-A"), 9, []byte("tree-A"))
	rt.coordinators["g1"] = coord
	rt.mlsEngine = &groupInfoTestMLSEngine{
		exportFn: func(groupState []byte, withRatchetTree bool) ([]byte, error) {
			prefix := "rt=0:"
			if withRatchetTree {
				prefix = "rt=1:"
			}
			return []byte(prefix + string(groupState)), nil
		},
	}

	resp, err := rt.exportLocalGroupInfo("g1", true)
	if err != nil {
		t.Fatalf("exportLocalGroupInfo: %v", err)
	}
	if resp.V != 1 || resp.GroupID != "g1" || resp.Epoch != 9 {
		t.Fatalf("unexpected metadata response: %+v", resp)
	}
	if string(resp.TreeHash) != "tree-A" {
		t.Fatalf("tree hash mismatch: got=%q", string(resp.TreeHash))
	}
	if string(resp.GroupInfo) != "rt=1:state-A" {
		t.Fatalf("group info mismatch: got=%q", string(resp.GroupInfo))
	}
}

func TestExportLocalGroupInfo_GroupNotFound(t *testing.T) {
	rt := setupMembershipRuntime(t)
	rt.mlsEngine = &groupInfoTestMLSEngine{
		exportFn: func([]byte, bool) ([]byte, error) { return []byte("unused"), nil },
	}
	_, err := rt.exportLocalGroupInfo("missing", true)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("error = %v, want ErrGroupNotFound", err)
	}
}

func TestExportLocalGroupInfo_MLSEngineNotReady(t *testing.T) {
	rt := setupMembershipRuntime(t)
	coord := &coordination.Coordinator{}
	coord.InitializeGroup([]byte("state-A"), 1, []byte("tree-A"))
	rt.coordinators["g1"] = coord

	_, err := rt.exportLocalGroupInfo("g1", true)
	if err == nil || !strings.Contains(err.Error(), "mls engine not ready") {
		t.Fatalf("expected mls not ready error, got: %v", err)
	}
}

func TestExportLocalGroupInfo_ExportError(t *testing.T) {
	rt := setupMembershipRuntime(t)
	coord := &coordination.Coordinator{}
	coord.InitializeGroup([]byte("state-A"), 1, []byte("tree-A"))
	rt.coordinators["g1"] = coord
	rt.mlsEngine = &groupInfoTestMLSEngine{
		exportFn: func([]byte, bool) ([]byte, error) { return nil, errors.New("boom") },
	}
	_, err := rt.exportLocalGroupInfo("g1", true)
	if err == nil || !strings.Contains(err.Error(), "ExportGroupInfo") {
		t.Fatalf("expected wrapped export error, got: %v", err)
	}
}
