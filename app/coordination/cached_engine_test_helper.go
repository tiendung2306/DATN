package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"

	"app/mls_service"
)

// testCachedGrpcMLSEngine implements MLSEngine using the stateful cached gRPC
// RPCs. It is a test-only mirror of sidecar.CachedGrpcMLSEngine, kept in the
// coordination package to avoid a circular import.
type testCachedGrpcMLSEngine struct {
	client    mls_service.MLSCryptoServiceClient
	mu        sync.Mutex
	entries   map[string]*testCacheEntry
	opCounter atomic.Uint64
}

type testCacheEntry struct {
	groupID      string
	epoch        uint64
	stateVersion uint64
}

func testStateKey(groupState []byte) string {
	h := sha256.Sum256(groupState)
	return hex.EncodeToString(h[:])
}

func (e *testCachedGrpcMLSEngine) nextOpID() string {
	return fmt.Sprintf("bench-op-%d", e.opCounter.Add(1))
}

func (e *testCachedGrpcMLSEngine) opCtx(ctx context.Context, groupState []byte) (*mls_service.OperationContext, error) {
	key := testStateKey(groupState)

	e.mu.Lock()
	entry, ok := e.entries[key]
	e.mu.Unlock()

	if !ok {
		// Use state hash as cache key for per-node isolation when
		// multiple nodes share the same sidecar in benchmark scenarios.
		resp, err := e.client.LoadGroup(ctx, &mls_service.LoadGroupRequest{
			GroupId:      key,
			GroupState:   groupState,
			StateVersion: 1,
		})
		if err != nil {
			return nil, fmt.Errorf("grpc LoadGroup (auto): %w", err)
		}
		entry = &testCacheEntry{
			groupID:      resp.GetGroupId(),
			epoch:        resp.GetEpoch(),
			stateVersion: resp.GetStateVersion(),
		}
		e.mu.Lock()
		e.entries[key] = entry
		e.mu.Unlock()
	}

	return &mls_service.OperationContext{
		GroupId:              entry.groupID,
		ExpectedEpoch:        entry.epoch,
		ExpectedStateVersion: entry.stateVersion,
		OperationId:          e.nextOpID(),
	}, nil
}

func (e *testCachedGrpcMLSEngine) updateEntry(oldState []byte, newEpoch, newStateVersion uint64, newState []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	oldKey := testStateKey(oldState)
	entry, ok := e.entries[oldKey]
	if !ok {
		return
	}
	entry.epoch = newEpoch
	entry.stateVersion = newStateVersion
	e.entries[testStateKey(newState)] = entry
	delete(e.entries, oldKey)
}

func (e *testCachedGrpcMLSEngine) bumpEntry(state []byte, newEpoch, newStateVersion uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := testStateKey(state)
	entry, ok := e.entries[key]
	if !ok {
		return
	}
	entry.epoch = newEpoch
	entry.stateVersion = newStateVersion
}

func (e *testCachedGrpcMLSEngine) checkpoint(ctx context.Context, groupState []byte) ([]byte, error) {
	e.mu.Lock()
	entry, ok := e.entries[testStateKey(groupState)]
	e.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("cannot checkpoint unknown group state")
	}
	resp, err := e.client.ExportGroupStateCheckpoint(ctx, &mls_service.ExportGroupStateCheckpointRequest{
		GroupId: entry.groupID,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportGroupStateCheckpoint: %w", err)
	}
	return resp.GetGroupState(), nil
}

func newTestCachedGrpcMLSEngine(client mls_service.MLSCryptoServiceClient) MLSEngine {
	return &testCachedGrpcMLSEngine{
		client:  client,
		entries: make(map[string]*testCacheEntry),
	}
}

func truncateKey(k []byte) []byte {
	if len(k) == 64 {
		return k[:32]
	}
	return k
}

func (e *testCachedGrpcMLSEngine) CreateGroup(ctx context.Context, groupID string, signingKey []byte, maxPastEpochs uint32) ([]byte, []byte, error) {
	resp, err := e.client.CreateGroupAndLoad(ctx, &mls_service.CreateGroupAndLoadRequest{
		GroupId:       groupID,
		SigningKey:    truncateKey(signingKey),
		MaxPastEpochs: maxPastEpochs,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc CreateGroupAndLoad: %w", err)
	}
	gs := resp.GetGroupState()
	e.mu.Lock()
	e.entries[testStateKey(gs)] = &testCacheEntry{
		groupID:      groupID,
		epoch:        resp.GetEpoch(),
		stateVersion: resp.GetStateVersion(),
	}
	e.mu.Unlock()
	return gs, resp.GetTreeHash(), nil
}

func (e *testCachedGrpcMLSEngine) CreateProposal(ctx context.Context, groupState []byte, pType ProposalType, data []byte) (CreateProposalResult, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return CreateProposalResult{}, err
	}
	resp, err := e.client.CreateProposalCached(ctx, &mls_service.CreateProposalCachedRequest{
		Context:      oc,
		ProposalType: mls_service.MlsProposalType(pType),
		Data:         data,
	})
	if err != nil {
		return CreateProposalResult{}, fmt.Errorf("grpc CreateProposalCached: %w", err)
	}
	newState := resp.GetGroupState()
	if len(newState) > 0 {
		e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	} else {
		e.bumpEntry(groupState, resp.GetEpoch(), resp.GetStateVersion())
	}
	return CreateProposalResult{
		ProposalBytes: resp.GetProposalBytes(),
		ProposalRef:   resp.GetProposalRef(),
		NewGroupState: newState,
	}, nil
}

func (e *testCachedGrpcMLSEngine) ProcessProposal(ctx context.Context, groupState []byte, proposalBytes []byte) (ProcessProposalResult, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return ProcessProposalResult{}, err
	}
	resp, err := e.client.ProcessProposalCached(ctx, &mls_service.ProcessProposalCachedRequest{
		Context:       oc,
		ProposalBytes: proposalBytes,
	})
	if err != nil {
		return ProcessProposalResult{}, fmt.Errorf("grpc ProcessProposalCached: %w", err)
	}
	newState := resp.GetGroupState()
	if len(newState) > 0 {
		e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	} else {
		e.bumpEntry(groupState, resp.GetEpoch(), resp.GetStateVersion())
	}
	return ProcessProposalResult{
		ProposalRef:   resp.GetProposalRef(),
		ProposalType:  resp.GetProposalType(),
		NewGroupState: newState,
	}, nil
}

func (e *testCachedGrpcMLSEngine) CreateCommit(ctx context.Context, groupState []byte, expectedProposalRefs [][]byte) (CreateCommitResult, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return CreateCommitResult{}, err
	}
	resp, err := e.client.CreateCommitCached(ctx, &mls_service.CreateCommitCachedRequest{
		Context:              oc,
		ExpectedProposalRefs: expectedProposalRefs,
	})
	if err != nil {
		return CreateCommitResult{}, fmt.Errorf("grpc CreateCommitCached: %w", err)
	}
	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	return CreateCommitResult{
		CommitBytes:           resp.GetCommitBytes(),
		WelcomeBytes:          resp.GetWelcomeBytes(),
		GroupInfo:             resp.GetGroupInfo(),
		CommittedProposalRefs: resp.GetCommittedProposalRefs(),
		NewGroupState:         newState,
		NewTreeHash:           resp.GetTreeHash(),
	}, nil
}

func (e *testCachedGrpcMLSEngine) StageCommit(ctx context.Context, groupState []byte, commitBytes []byte, includedProposals [][]byte) (StageCommitResult, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return StageCommitResult{}, err
	}
	resp, err := e.client.StageCommitCached(ctx, &mls_service.StageCommitCachedRequest{
		Context:           oc,
		CommitBytes:       commitBytes,
		IncludedProposals: includedProposals,
	})
	if err != nil {
		return StageCommitResult{}, fmt.Errorf("grpc StageCommitCached: %w", err)
	}
	return StageCommitResult{
		Epoch:         resp.GetEpoch(),
		ProposalRefs:  resp.GetProposalRefs(),
		ProposalTypes: resp.GetProposalTypes(),
	}, nil
}

func (e *testCachedGrpcMLSEngine) ProcessCommit(ctx context.Context, groupState []byte, commitBytes []byte, includedProposals [][]byte) ([]byte, []byte, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, nil, err
	}
	resp, err := e.client.ProcessCommitCached(ctx, &mls_service.ProcessCommitCachedRequest{
		Context:           oc,
		CommitBytes:       commitBytes,
		IncludedProposals: includedProposals,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc ProcessCommitCached: %w", err)
	}
	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	return newState, resp.GetTreeHash(), nil
}

func (e *testCachedGrpcMLSEngine) ProcessWelcome(ctx context.Context, welcomeBytes, signingKey, keyPackageBundlePrivate []byte, maxPastEpochs uint32) ([]byte, []byte, uint64, error) {
	resp, err := e.client.ProcessWelcomeAndLoad(ctx, &mls_service.ProcessWelcomeAndLoadRequest{
		GroupId:                 "",
		WelcomeBytes:            welcomeBytes,
		SigningKey:              truncateKey(signingKey),
		KeyPackageBundlePrivate: keyPackageBundlePrivate,
		MaxPastEpochs:           maxPastEpochs,
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("grpc ProcessWelcomeAndLoad: %w", err)
	}
	gs := resp.GetGroupState()
	// Load the new group state into Rust cache under the state hash key.
	stateHashKey := testStateKey(gs)
	if _, err := e.client.LoadGroup(ctx, &mls_service.LoadGroupRequest{
		GroupId:      stateHashKey,
		GroupState:   gs,
		StateVersion: resp.GetStateVersion(),
	}); err != nil {
		return nil, nil, 0, fmt.Errorf("grpc LoadGroup (welcome): %w", err)
	}

	e.mu.Lock()
	e.entries[stateHashKey] = &testCacheEntry{
		groupID:      stateHashKey,
		epoch:        resp.GetEpoch(),
		stateVersion: resp.GetStateVersion(),
	}
	e.mu.Unlock()
	return gs, resp.GetTreeHash(), resp.GetEpoch(), nil
}

func (e *testCachedGrpcMLSEngine) GenerateKeyPackage(ctx context.Context, signingKey []byte) ([]byte, []byte, error) {
	resp, err := e.client.GenerateKeyPackage(ctx, &mls_service.GenerateKeyPackageRequest{
		SigningKey: truncateKey(signingKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc GenerateKeyPackage: %w", err)
	}
	return resp.GetKeyPackageBytes(), resp.GetKeyPackageBundlePrivate(), nil
}

func (e *testCachedGrpcMLSEngine) AddMembers(ctx context.Context, groupState []byte, keyPackages [][]byte) ([]byte, []byte, []byte, []byte, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	resp, err := e.client.AddMembersCached(ctx, &mls_service.AddMembersCachedRequest{
		Context:     oc,
		KeyPackages: keyPackages,
	})
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("grpc AddMembersCached: %w", err)
	}
	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	return resp.GetCommitBytes(), resp.GetWelcomeBytes(), newState, resp.GetTreeHash(), nil
}

func (e *testCachedGrpcMLSEngine) RemoveMembers(ctx context.Context, groupState []byte, targetIdentities [][]byte) ([]byte, []byte, []byte, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, nil, nil, err
	}
	resp, err := e.client.RemoveMembersCached(ctx, &mls_service.RemoveMembersCachedRequest{
		Context:          oc,
		TargetIdentities: targetIdentities,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("grpc RemoveMembersCached: %w", err)
	}
	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	return resp.GetCommitBytes(), newState, resp.GetTreeHash(), nil
}

func (e *testCachedGrpcMLSEngine) HasMember(ctx context.Context, groupState []byte, identity []byte) (bool, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return false, err
	}
	resp, err := e.client.HasMemberCached(ctx, &mls_service.HasMemberCachedRequest{
		Context:  oc,
		Identity: identity,
	})
	if err != nil {
		return false, fmt.Errorf("grpc HasMemberCached: %w", err)
	}
	return resp.GetIsMember(), nil
}

func (e *testCachedGrpcMLSEngine) ListMemberIdentities(ctx context.Context, groupState []byte) ([][]byte, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.ListMemberIdentitiesCached(ctx, &mls_service.ListMemberIdentitiesCachedRequest{
		Context: oc,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ListMemberIdentitiesCached: %w", err)
	}
	return resp.GetIdentities(), nil
}

func (e *testCachedGrpcMLSEngine) EncryptMessage(ctx context.Context, groupState []byte, plaintext []byte) ([]byte, []byte, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, nil, err
	}
	resp, err := e.client.EncryptMessageCached(ctx, &mls_service.EncryptMessageCachedRequest{
		Context:   oc,
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc EncryptMessageCached: %w", err)
	}
	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	return resp.GetCiphertext(), newState, nil
}

func (e *testCachedGrpcMLSEngine) DecryptMessage(ctx context.Context, groupState []byte, ciphertext []byte) ([]byte, []byte, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, nil, err
	}
	resp, err := e.client.DecryptMessageCached(ctx, &mls_service.DecryptMessageCachedRequest{
		Context:    oc,
		Ciphertext: ciphertext,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc DecryptMessageCached: %w", err)
	}
	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	return resp.GetPlaintext(), newState, nil
}

func (e *testCachedGrpcMLSEngine) ExternalJoin(ctx context.Context, groupInfo, signingKey []byte, maxPastEpochs uint32) ([]byte, []byte, []byte, error) {
	resp, err := e.client.ExternalJoinAndLoad(ctx, &mls_service.ExternalJoinAndLoadRequest{
		GroupId:       "",
		GroupInfo:     groupInfo,
		SigningKey:    truncateKey(signingKey),
		MaxPastEpochs: maxPastEpochs,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("grpc ExternalJoinAndLoad: %w", err)
	}
	gs := resp.GetGroupState()
	// Load the new group state into Rust cache under the state hash key.
	stateHashKey := testStateKey(gs)
	if _, err := e.client.LoadGroup(ctx, &mls_service.LoadGroupRequest{
		GroupId:      stateHashKey,
		GroupState:   gs,
		StateVersion: resp.GetStateVersion(),
	}); err != nil {
		return nil, nil, nil, fmt.Errorf("grpc LoadGroup (externaljoin): %w", err)
	}

	e.mu.Lock()
	e.entries[stateHashKey] = &testCacheEntry{
		groupID:      stateHashKey,
		epoch:        resp.GetEpoch(),
		stateVersion: resp.GetStateVersion(),
	}
	e.mu.Unlock()
	return gs, resp.GetCommitBytes(), resp.GetTreeHash(), nil
}

func (e *testCachedGrpcMLSEngine) ExportGroupInfo(ctx context.Context, groupState []byte, withRatchetTree bool) ([]byte, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.ExportGroupInfoCached(ctx, &mls_service.ExportGroupInfoCachedRequest{
		Context:         oc,
		WithRatchetTree: withRatchetTree,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportGroupInfoCached: %w", err)
	}
	return resp.GetGroupInfo(), nil
}

func (e *testCachedGrpcMLSEngine) ExportSecret(ctx context.Context, groupState []byte, label string, exporterContext []byte, length int) ([]byte, error) {
	oc, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.ExportSecretCached(ctx, &mls_service.ExportSecretCachedRequest{
		Context:         oc,
		Label:           label,
		Length:          uint32(length),
		ExporterContext: append([]byte(nil), exporterContext...),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportSecretCached: %w", err)
	}
	return resp.GetSecret(), nil
}
