package sidecar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"

	"app/coordination"
	"app/mls_service"
)

// CachedGrpcMLSEngine implements coordination.MLSEngine using the stateful
// cached gRPC RPCs (Phase B). Instead of sending the full group_state blob on
// every call, it loads the group into the Rust sidecar's RuntimeCache once,
// then subsequent operations reference the cached group by group_id +
// OperationContext. Only mutation operations call ExportGroupStateCheckpoint
// to retrieve the updated state for SQLite persistence (returned as
// NewGroupState to satisfy the stateless MLSEngine interface contract).
type CachedGrpcMLSEngine struct {
	client mls_service.MLSCryptoServiceClient

	mu       sync.Mutex
	entries  map[string]*cacheEntry // keyed by hex(sha256(groupState))
	opCounter atomic.Uint64
}

type cacheEntry struct {
	groupID      string
	epoch        uint64
	stateVersion uint64
}

func stateKey(groupState []byte) string {
	h := sha256.Sum256(groupState)
	return hex.EncodeToString(h[:])
}

func (e *CachedGrpcMLSEngine) nextOpID() string {
	return fmt.Sprintf("op-%d", e.opCounter.Add(1))
}

func (e *CachedGrpcMLSEngine) opCtx(ctx context.Context, groupState []byte) (*mls_service.OperationContext, *cacheEntry, error) {
	key := stateKey(groupState)

	e.mu.Lock()
	entry, ok := e.entries[key]
	e.mu.Unlock()

	if !ok {
		// Auto-load: the coordinator may have restored groupState from SQLite
		// without an explicit LoadGroup call. Use the state hash as the cache
		// key so that multiple nodes sharing the same sidecar (each with their
		// own groupState) get distinct Rust cache entries instead of
		// overwriting each other via the shared MLS group_id.
		resp, err := e.client.LoadGroup(ctx, &mls_service.LoadGroupRequest{
			GroupId:      key,
			GroupState:   groupState,
			StateVersion: 1,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("grpc LoadGroup (auto): %w", err)
		}

		entry = &cacheEntry{
			groupID:      resp.GetGroupId(),
			epoch:        resp.GetEpoch(),
			stateVersion: resp.GetStateVersion(),
		}

		e.mu.Lock()
		e.entries[key] = entry
		e.mu.Unlock()
	}

	return &mls_service.OperationContext{
		GroupId:             entry.groupID,
		ExpectedEpoch:       entry.epoch,
		ExpectedStateVersion: entry.stateVersion,
		OperationId:         e.nextOpID(),
	}, entry, nil
}

func (e *CachedGrpcMLSEngine) updateEntry(oldState []byte, newEpoch, newStateVersion uint64, newState []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()

	oldKey := stateKey(oldState)
	entry, ok := e.entries[oldKey]
	if !ok {
		return
	}

	entry.epoch = newEpoch
	entry.stateVersion = newStateVersion

	newKey := stateKey(newState)
	e.entries[newKey] = entry
	delete(e.entries, oldKey)
}

func (e *CachedGrpcMLSEngine) bumpEntry(state []byte, newEpoch, newStateVersion uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := stateKey(state)
	entry, ok := e.entries[key]
	if !ok {
		return
	}
	entry.epoch = newEpoch
	entry.stateVersion = newStateVersion
}

func (e *CachedGrpcMLSEngine) checkpoint(ctx context.Context, groupState []byte) ([]byte, error) {
	e.mu.Lock()
	entry, ok := e.entries[stateKey(groupState)]
	e.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("cached engine: cannot checkpoint unknown group state")
	}

	resp, err := e.client.ExportGroupStateCheckpoint(ctx, &mls_service.ExportGroupStateCheckpointRequest{
		GroupId: entry.groupID,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportGroupStateCheckpoint: %w", err)
	}
	return resp.GetGroupState(), nil
}

// NewCachedMLSEngine wraps a gRPC client as a cached coordination.MLSEngine.
func NewCachedMLSEngine(client mls_service.MLSCryptoServiceClient) coordination.MLSEngine {
	return &CachedGrpcMLSEngine{
		client:  client,
		entries: make(map[string]*cacheEntry),
	}
}

// LoadGroup loads a group state into the Rust sidecar cache. Must be called
// before any cached operation referencing this group state.
func (e *CachedGrpcMLSEngine) LoadGroup(ctx context.Context, groupID string, groupState []byte) error {
	resp, err := e.client.LoadGroup(ctx, &mls_service.LoadGroupRequest{
		GroupId:      groupID,
		GroupState:   groupState,
		StateVersion: 1,
	})
	if err != nil {
		return fmt.Errorf("grpc LoadGroup: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	key := stateKey(groupState)
	e.entries[key] = &cacheEntry{
		groupID:      resp.GetGroupId(),
		epoch:        resp.GetEpoch(),
		stateVersion: resp.GetStateVersion(),
	}
	return nil
}

func (e *CachedGrpcMLSEngine) CreateGroup(ctx context.Context, groupID string, signingKey []byte, maxPastEpochs uint32) (groupState, treeHash []byte, err error) {
	resp, err := e.client.CreateGroupAndLoad(ctx, &mls_service.CreateGroupAndLoadRequest{
		GroupId:       groupID,
		SigningKey:    truncateSigningKey(signingKey),
		MaxPastEpochs: maxPastEpochs,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc CreateGroupAndLoad: %w", err)
	}

	groupState = resp.GetGroupState()
	if len(groupState) == 0 {
		return nil, nil, fmt.Errorf("grpc CreateGroupAndLoad: empty group_state; %s", rebuildSidecarHint)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	key := stateKey(groupState)
	e.entries[key] = &cacheEntry{
		groupID:      groupID,
		epoch:        resp.GetEpoch(),
		stateVersion: resp.GetStateVersion(),
	}

	return groupState, resp.GetTreeHash(), nil
}

func (e *CachedGrpcMLSEngine) CreateProposal(ctx context.Context, groupState []byte, pType coordination.ProposalType, data []byte) (coordination.CreateProposalResult, error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return coordination.CreateProposalResult{}, err
	}

	resp, err := e.client.CreateProposalCached(ctx, &mls_service.CreateProposalCachedRequest{
		Context:       oc,
		ProposalType:  mls_service.MlsProposalType(pType),
		Data:          data,
	})
	if err != nil {
		return coordination.CreateProposalResult{}, fmt.Errorf("grpc CreateProposalCached: %w", err)
	}

	newState := resp.GetGroupState()
	if len(newState) > 0 {
		e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	} else {
		e.bumpEntry(groupState, resp.GetEpoch(), resp.GetStateVersion())
	}

	return coordination.CreateProposalResult{
		ProposalBytes: resp.GetProposalBytes(),
		ProposalRef:   resp.GetProposalRef(),
		NewGroupState: newState,
	}, nil
}

func (e *CachedGrpcMLSEngine) ProcessProposal(ctx context.Context, groupState []byte, proposalBytes []byte) (coordination.ProcessProposalResult, error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return coordination.ProcessProposalResult{}, err
	}

	resp, err := e.client.ProcessProposalCached(ctx, &mls_service.ProcessProposalCachedRequest{
		Context:        oc,
		ProposalBytes:  proposalBytes,
	})
	if err != nil {
		return coordination.ProcessProposalResult{}, fmt.Errorf("grpc ProcessProposalCached: %w", err)
	}

	newState := resp.GetGroupState()
	if len(newState) > 0 {
		e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)
	} else {
		e.bumpEntry(groupState, resp.GetEpoch(), resp.GetStateVersion())
	}

	return coordination.ProcessProposalResult{
		ProposalRef:   resp.GetProposalRef(),
		ProposalType:  resp.GetProposalType(),
		NewGroupState: newState,
	}, nil
}

func (e *CachedGrpcMLSEngine) CreateCommit(ctx context.Context, groupState []byte, expectedProposalRefs [][]byte) (coordination.CreateCommitResult, error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return coordination.CreateCommitResult{}, err
	}

	resp, err := e.client.CreateCommitCached(ctx, &mls_service.CreateCommitCachedRequest{
		Context:              oc,
		ExpectedProposalRefs: expectedProposalRefs,
	})
	if err != nil {
		return coordination.CreateCommitResult{}, fmt.Errorf("grpc CreateCommitCached: %w", err)
	}

	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)

	return coordination.CreateCommitResult{
		CommitBytes:           resp.GetCommitBytes(),
		WelcomeBytes:          resp.GetWelcomeBytes(),
		GroupInfo:             resp.GetGroupInfo(),
		CommittedProposalRefs: resp.GetCommittedProposalRefs(),
		NewGroupState:         newState,
		NewTreeHash:           resp.GetTreeHash(),
	}, nil
}

func (e *CachedGrpcMLSEngine) StageCommit(ctx context.Context, groupState []byte, commitBytes []byte, includedProposals [][]byte) (coordination.StageCommitResult, error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return coordination.StageCommitResult{}, err
	}

	resp, err := e.client.StageCommitCached(ctx, &mls_service.StageCommitCachedRequest{
		Context:            oc,
		CommitBytes:        commitBytes,
		IncludedProposals:  includedProposals,
	})
	if err != nil {
		return coordination.StageCommitResult{}, fmt.Errorf("grpc StageCommitCached: %w", err)
	}

	return coordination.StageCommitResult{
		Epoch:         resp.GetEpoch(),
		ProposalRefs:  resp.GetProposalRefs(),
		ProposalTypes: resp.GetProposalTypes(),
	}, nil
}

func (e *CachedGrpcMLSEngine) ProcessCommit(ctx context.Context, groupState []byte, commitBytes []byte, includedProposals [][]byte) (newGroupState, newTreeHash []byte, err error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, nil, err
	}

	resp, err := e.client.ProcessCommitCached(ctx, &mls_service.ProcessCommitCachedRequest{
		Context:            oc,
		CommitBytes:        commitBytes,
		IncludedProposals:  includedProposals,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc ProcessCommitCached: %w", err)
	}

	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)

	return newState, resp.GetTreeHash(), nil
}

func (e *CachedGrpcMLSEngine) ProcessWelcome(ctx context.Context, welcomeBytes, signingKey, keyPackageBundlePrivate []byte, maxPastEpochs uint32) (groupState, treeHash []byte, epoch uint64, err error) {
	resp, err := e.client.ProcessWelcomeAndLoad(ctx, &mls_service.ProcessWelcomeAndLoadRequest{
		WelcomeBytes:            welcomeBytes,
		SigningKey:              truncateSigningKey(signingKey),
		KeyPackageBundlePrivate: keyPackageBundlePrivate,
		MaxPastEpochs:           maxPastEpochs,
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("grpc ProcessWelcomeAndLoad: %w", err)
	}

	groupState = resp.GetGroupState()
	if len(groupState) == 0 {
		return nil, nil, 0, fmt.Errorf("grpc ProcessWelcomeAndLoad: empty group_state; %s", rebuildSidecarHint)
	}

	// Load the new group state into Rust cache under the state hash key
	// for per-node isolation.
	stateHashKey := stateKey(groupState)
	if _, err := e.client.LoadGroup(ctx, &mls_service.LoadGroupRequest{
		GroupId:      stateHashKey,
		GroupState:   groupState,
		StateVersion: resp.GetStateVersion(),
	}); err != nil {
		return nil, nil, 0, fmt.Errorf("grpc LoadGroup (welcome): %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.entries[stateHashKey] = &cacheEntry{
		groupID:      stateHashKey,
		epoch:        resp.GetEpoch(),
		stateVersion: resp.GetStateVersion(),
	}

	return groupState, resp.GetTreeHash(), resp.GetEpoch(), nil
}

func (e *CachedGrpcMLSEngine) GenerateKeyPackage(ctx context.Context, signingKey []byte) (keyPackageBytes, keyPackageBundlePrivate []byte, err error) {
	resp, err := e.client.GenerateKeyPackage(ctx, &mls_service.GenerateKeyPackageRequest{
		SigningKey: truncateSigningKey(signingKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc GenerateKeyPackage: %w", err)
	}
	return resp.GetKeyPackageBytes(), resp.GetKeyPackageBundlePrivate(), nil
}

func (e *CachedGrpcMLSEngine) AddMembers(ctx context.Context, groupState []byte, keyPackages [][]byte) (commitBytes, welcomeBytes, newGroupState, newTreeHash []byte, err error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	resp, err := e.client.AddMembersCached(ctx, &mls_service.AddMembersCachedRequest{
		Context:      oc,
		KeyPackages:  keyPackages,
	})
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("grpc AddMembersCached: %w", err)
	}

	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)

	return resp.GetCommitBytes(), resp.GetWelcomeBytes(), newState, resp.GetTreeHash(), nil
}

func (e *CachedGrpcMLSEngine) RemoveMembers(ctx context.Context, groupState []byte, targetIdentities [][]byte) (commitBytes, newGroupState, newTreeHash []byte, err error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, nil, nil, err
	}

	resp, err := e.client.RemoveMembersCached(ctx, &mls_service.RemoveMembersCachedRequest{
		Context:           oc,
		TargetIdentities:  targetIdentities,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("grpc RemoveMembersCached: %w", err)
	}

	newState := resp.GetGroupState()
	e.updateEntry(groupState, resp.GetEpoch(), resp.GetStateVersion(), newState)

	return resp.GetCommitBytes(), newState, resp.GetTreeHash(), nil
}

func (e *CachedGrpcMLSEngine) HasMember(ctx context.Context, groupState []byte, identity []byte) (bool, error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return false, err
	}

	resp, err := e.client.HasMemberCached(ctx, &mls_service.HasMemberCachedRequest{
		Context:   oc,
		Identity:  identity,
	})
	if err != nil {
		return false, fmt.Errorf("grpc HasMemberCached: %w", err)
	}
	return resp.GetIsMember(), nil
}

func (e *CachedGrpcMLSEngine) ListMemberIdentities(ctx context.Context, groupState []byte) ([][]byte, error) {
	oc, _, err := e.opCtx(ctx, groupState)
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

func (e *CachedGrpcMLSEngine) EncryptMessage(ctx context.Context, groupState []byte, plaintext []byte) (ciphertext, newGroupState []byte, err error) {
	oc, _, err := e.opCtx(ctx, groupState)
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

func (e *CachedGrpcMLSEngine) DecryptMessage(ctx context.Context, groupState []byte, ciphertext []byte) (plaintext, newGroupState []byte, err error) {
	oc, _, err := e.opCtx(ctx, groupState)
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

func (e *CachedGrpcMLSEngine) ExternalJoin(ctx context.Context, groupInfo, signingKey []byte, maxPastEpochs uint32) (groupState, commitBytes, treeHash []byte, err error) {
	resp, err := e.client.ExternalJoinAndLoad(ctx, &mls_service.ExternalJoinAndLoadRequest{
		GroupInfo:     groupInfo,
		SigningKey:    truncateSigningKey(signingKey),
		MaxPastEpochs: maxPastEpochs,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("grpc ExternalJoinAndLoad: %w", err)
	}

	groupState = resp.GetGroupState()
	if len(groupState) == 0 {
		return nil, nil, nil, fmt.Errorf("grpc ExternalJoinAndLoad: empty group_state; %s", rebuildSidecarHint)
	}

	// Load the new group state into Rust cache under the state hash key
	// for per-node isolation.
	stateHashKey := stateKey(groupState)
	if _, err := e.client.LoadGroup(ctx, &mls_service.LoadGroupRequest{
		GroupId:      stateHashKey,
		GroupState:   groupState,
		StateVersion: resp.GetStateVersion(),
	}); err != nil {
		return nil, nil, nil, fmt.Errorf("grpc LoadGroup (externaljoin): %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.entries[stateHashKey] = &cacheEntry{
		groupID:      stateHashKey,
		epoch:        resp.GetEpoch(),
		stateVersion: resp.GetStateVersion(),
	}

	return groupState, resp.GetCommitBytes(), resp.GetTreeHash(), nil
}

func (e *CachedGrpcMLSEngine) ExportGroupInfo(ctx context.Context, groupState []byte, withRatchetTree bool) (groupInfo []byte, err error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.ExportGroupInfoCached(ctx, &mls_service.ExportGroupInfoCachedRequest{
		Context:          oc,
		WithRatchetTree:  withRatchetTree,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportGroupInfoCached: %w", err)
	}
	return resp.GetGroupInfo(), nil
}

func (e *CachedGrpcMLSEngine) ExportSecret(ctx context.Context, groupState []byte, label string, context []byte, length int) (secret []byte, err error) {
	oc, _, err := e.opCtx(ctx, groupState)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.ExportSecretCached(ctx, &mls_service.ExportSecretCachedRequest{
		Context: oc,
		Label:   label,
		Length:  uint32(length),
		ExporterContext: append([]byte(nil), context...),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportSecretCached: %w", err)
	}
	return resp.GetSecret(), nil
}
