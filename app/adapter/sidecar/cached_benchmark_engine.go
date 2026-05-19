package sidecar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"app/mls_service"
)

// CachedBenchmarkEngine exposes the Phase A hot-cache sidecar RPCs for
// controlled benchmarks. It is intentionally separate from coordination.MLSEngine
// so production continues to use the stateless full-blob path unless a later
// production-grade adapter is explicitly introduced.
type CachedBenchmarkEngine struct {
	client mls_service.MLSCryptoServiceClient
}

type CachedGroupMetadata struct {
	GroupID        string
	Epoch          uint64
	StateVersion   uint64
	TreeHash       []byte
	Dirty          bool
	StateSizeBytes uint64
}

type CachedEncryptResult struct {
	Ciphertext   []byte
	Epoch        uint64
	StateVersion uint64
}

type CachedDecryptResult struct {
	Plaintext    []byte
	Epoch        uint64
	StateVersion uint64
}

type CachedUpdateCommitResult struct {
	CommitBytes  []byte
	TreeHash     []byte
	Epoch        uint64
	StateVersion uint64
}

type CachedProcessCommitResult struct {
	TreeHash     []byte
	Epoch        uint64
	StateVersion uint64
}

type CachedExportSecretResult struct {
	Secret       []byte
	Epoch        uint64
	StateVersion uint64
}

type CachedCheckpointResult struct {
	GroupState     []byte
	TreeHash       []byte
	Epoch          uint64
	StateVersion   uint64
	StateSizeBytes uint64
}

func NewCachedBenchmarkEngine(client mls_service.MLSCryptoServiceClient) *CachedBenchmarkEngine {
	return &CachedBenchmarkEngine{client: client}
}

func (c *CachedBenchmarkEngine) LoadGroup(ctx context.Context, groupID string, groupState []byte, stateVersion uint64) (*CachedGroupMetadata, error) {
	resp, err := c.client.LoadGroup(ctx, &mls_service.LoadGroupRequest{
		GroupId:      groupID,
		GroupState:   groupState,
		StateVersion: stateVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc LoadGroup: %w", err)
	}
	return metadataFromLoad(resp), nil
}

func (c *CachedBenchmarkEngine) GetGroupMetadata(ctx context.Context, groupID string) (*CachedGroupMetadata, error) {
	resp, err := c.client.GetGroupMetadata(ctx, &mls_service.GetGroupMetadataRequest{GroupId: groupID})
	if err != nil {
		return nil, fmt.Errorf("grpc GetGroupMetadata: %w", err)
	}
	return &CachedGroupMetadata{
		GroupID:        resp.GetGroupId(),
		Epoch:          resp.GetEpoch(),
		StateVersion:   resp.GetStateVersion(),
		TreeHash:       append([]byte(nil), resp.GetTreeHash()...),
		Dirty:          resp.GetDirty(),
		StateSizeBytes: resp.GetStateSizeBytes(),
	}, nil
}

func (c *CachedBenchmarkEngine) EncryptMessage(ctx context.Context, groupID string, epoch, stateVersion uint64, plaintext []byte) (*CachedEncryptResult, error) {
	resp, err := c.client.EncryptMessageCached(ctx, &mls_service.EncryptMessageCachedRequest{
		Context:   operationContext(groupID, epoch, stateVersion),
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc EncryptMessageCached: %w", err)
	}
	return &CachedEncryptResult{
		Ciphertext:   append([]byte(nil), resp.GetCiphertext()...),
		Epoch:        resp.GetEpoch(),
		StateVersion: resp.GetStateVersion(),
	}, nil
}

func (c *CachedBenchmarkEngine) DecryptMessage(ctx context.Context, groupID string, epoch, stateVersion uint64, ciphertext []byte) (*CachedDecryptResult, error) {
	resp, err := c.client.DecryptMessageCached(ctx, &mls_service.DecryptMessageCachedRequest{
		Context:    operationContext(groupID, epoch, stateVersion),
		Ciphertext: ciphertext,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc DecryptMessageCached: %w", err)
	}
	return &CachedDecryptResult{
		Plaintext:    append([]byte(nil), resp.GetPlaintext()...),
		Epoch:        resp.GetEpoch(),
		StateVersion: resp.GetStateVersion(),
	}, nil
}

func (c *CachedBenchmarkEngine) CreateUpdateCommit(ctx context.Context, groupID string, epoch, stateVersion uint64) (*CachedUpdateCommitResult, error) {
	resp, err := c.client.CreateUpdateCommitCached(ctx, &mls_service.CreateUpdateCommitCachedRequest{
		Context: operationContext(groupID, epoch, stateVersion),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc CreateUpdateCommitCached: %w", err)
	}
	return &CachedUpdateCommitResult{
		CommitBytes:  append([]byte(nil), resp.GetCommitBytes()...),
		TreeHash:     append([]byte(nil), resp.GetTreeHash()...),
		Epoch:        resp.GetEpoch(),
		StateVersion: resp.GetStateVersion(),
	}, nil
}

func (c *CachedBenchmarkEngine) ProcessCommit(ctx context.Context, groupID string, epoch, stateVersion uint64, commitBytes []byte) (*CachedProcessCommitResult, error) {
	resp, err := c.client.ProcessCommitCached(ctx, &mls_service.ProcessCommitCachedRequest{
		Context:     operationContext(groupID, epoch, stateVersion),
		CommitBytes: commitBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ProcessCommitCached: %w", err)
	}
	return &CachedProcessCommitResult{
		TreeHash:     append([]byte(nil), resp.GetTreeHash()...),
		Epoch:        resp.GetEpoch(),
		StateVersion: resp.GetStateVersion(),
	}, nil
}

func (c *CachedBenchmarkEngine) ExportSecret(ctx context.Context, groupID string, epoch, stateVersion uint64, label string, exporterContext []byte, length int) (*CachedExportSecretResult, error) {
	resp, err := c.client.ExportSecretCached(ctx, &mls_service.ExportSecretCachedRequest{
		Context:         operationContext(groupID, epoch, stateVersion),
		Label:           label,
		Length:          uint32(length),
		ExporterContext: append([]byte(nil), exporterContext...),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportSecretCached: %w", err)
	}
	return &CachedExportSecretResult{
		Secret:       append([]byte(nil), resp.GetSecret()...),
		Epoch:        resp.GetEpoch(),
		StateVersion: resp.GetStateVersion(),
	}, nil
}

func (c *CachedBenchmarkEngine) ExportCheckpoint(ctx context.Context, groupID string) (*CachedCheckpointResult, error) {
	resp, err := c.client.ExportGroupStateCheckpoint(ctx, &mls_service.ExportGroupStateCheckpointRequest{GroupId: groupID})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportGroupStateCheckpoint: %w", err)
	}
	return &CachedCheckpointResult{
		GroupState:     append([]byte(nil), resp.GetGroupState()...),
		TreeHash:       append([]byte(nil), resp.GetTreeHash()...),
		Epoch:          resp.GetEpoch(),
		StateVersion:   resp.GetStateVersion(),
		StateSizeBytes: resp.GetStateSizeBytes(),
	}, nil
}

func (c *CachedBenchmarkEngine) UnloadGroup(ctx context.Context, groupID string) (bool, error) {
	resp, err := c.client.UnloadGroup(ctx, &mls_service.UnloadGroupRequest{GroupId: groupID})
	if err != nil {
		return false, fmt.Errorf("grpc UnloadGroup: %w", err)
	}
	return resp.GetUnloaded(), nil
}

func metadataFromLoad(resp *mls_service.LoadGroupResponse) *CachedGroupMetadata {
	return &CachedGroupMetadata{
		GroupID:        resp.GetGroupId(),
		Epoch:          resp.GetEpoch(),
		StateVersion:   resp.GetStateVersion(),
		TreeHash:       append([]byte(nil), resp.GetTreeHash()...),
		Dirty:          false,
		StateSizeBytes: resp.GetStateSizeBytes(),
	}
}

func operationContext(groupID string, epoch, stateVersion uint64) *mls_service.OperationContext {
	return &mls_service.OperationContext{
		GroupId:              groupID,
		ExpectedEpoch:        epoch,
		ExpectedStateVersion: stateVersion,
		OperationId:          newOperationID(),
	}
}

func newOperationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "op-rand-failed"
	}
	return hex.EncodeToString(b[:])
}
