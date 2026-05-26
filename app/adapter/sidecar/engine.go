package sidecar

import (
	"context"
	"fmt"

	"app/coordination"
	"app/mls_service"
)

// GrpcMLSEngine adapts the gRPC MLSCryptoServiceClient to coordination.MLSEngine.
type GrpcMLSEngine struct {
	client mls_service.MLSCryptoServiceClient
}

// NewMLSEngine wraps an existing gRPC client as coordination.MLSEngine.
func NewMLSEngine(client mls_service.MLSCryptoServiceClient) coordination.MLSEngine {
	return &GrpcMLSEngine{client: client}
}

func truncateSigningKey(key []byte) []byte {
	if len(key) == 64 {
		return key[:32]
	}
	return key
}

func (g *GrpcMLSEngine) CreateGroup(ctx context.Context, groupID string, signingKey []byte) (groupState, treeHash []byte, err error) {
	resp, err := g.client.CreateGroup(ctx, &mls_service.CreateGroupRequest{
		GroupId:    groupID,
		SigningKey: truncateSigningKey(signingKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc CreateGroup: %w", err)
	}
	return resp.GetGroupState(), resp.GetTreeHash(), nil
}

func (g *GrpcMLSEngine) CreateProposal(ctx context.Context, groupState []byte, pType coordination.ProposalType, data []byte) (coordination.CreateProposalResult, error) {
	resp, err := g.client.CreateProposal(ctx, &mls_service.CreateProposalRequest{
		GroupState:   groupState,
		ProposalType: mls_service.MlsProposalType(pType),
		Data:         data,
	})
	if err != nil {
		return coordination.CreateProposalResult{}, fmt.Errorf("grpc CreateProposal: %w", err)
	}
	return coordination.CreateProposalResult{
		ProposalBytes: resp.GetProposalBytes(),
		ProposalRef:   resp.GetProposalRef(),
		NewGroupState: resp.GetNewGroupState(),
	}, nil
}

func (g *GrpcMLSEngine) ProcessProposal(ctx context.Context, groupState []byte, proposalBytes []byte) (coordination.ProcessProposalResult, error) {
	resp, err := g.client.ProcessProposal(ctx, &mls_service.ProcessProposalRequest{
		GroupState:    groupState,
		ProposalBytes: proposalBytes,
	})
	if err != nil {
		return coordination.ProcessProposalResult{}, fmt.Errorf("grpc ProcessProposal: %w", err)
	}
	return coordination.ProcessProposalResult{
		ProposalRef:   resp.GetProposalRef(),
		ProposalType:  resp.GetProposalType(),
		NewGroupState: resp.GetNewGroupState(),
	}, nil
}

func (g *GrpcMLSEngine) CreateCommit(ctx context.Context, groupState []byte, expectedProposalRefs [][]byte) (coordination.CreateCommitResult, error) {
	resp, err := g.client.CreateCommit(ctx, &mls_service.CreateCommitRequest{
		GroupState:           groupState,
		ExpectedProposalRefs: expectedProposalRefs,
	})
	if err != nil {
		return coordination.CreateCommitResult{}, fmt.Errorf("grpc CreateCommit: %w", err)
	}
	return coordination.CreateCommitResult{
		CommitBytes:           resp.GetCommitBytes(),
		WelcomeBytes:          resp.GetWelcomeBytes(),
		GroupInfo:             resp.GetGroupInfo(),
		CommittedProposalRefs: resp.GetCommittedProposalRefs(),
		NewGroupState:         resp.GetNewGroupState(),
		NewTreeHash:           resp.GetNewTreeHash(),
	}, nil
}

func (g *GrpcMLSEngine) StageCommit(ctx context.Context, groupState []byte, commitBytes []byte, includedProposals [][]byte) (coordination.StageCommitResult, error) {
	resp, err := g.client.StageCommit(ctx, &mls_service.StageCommitRequest{
		GroupState:        groupState,
		CommitBytes:       commitBytes,
		IncludedProposals: includedProposals,
	})
	if err != nil {
		return coordination.StageCommitResult{}, fmt.Errorf("grpc StageCommit: %w", err)
	}
	return coordination.StageCommitResult{
		Epoch:         resp.GetEpoch(),
		ProposalRefs:  resp.GetProposalRefs(),
		ProposalTypes: resp.GetProposalTypes(),
	}, nil
}

func (g *GrpcMLSEngine) ProcessCommit(ctx context.Context, groupState []byte, commitBytes []byte, includedProposals [][]byte) (newGroupState, newTreeHash []byte, err error) {
	resp, err := g.client.ProcessCommit(ctx, &mls_service.ProcessCommitRequest{
		GroupState:        groupState,
		CommitBytes:       commitBytes,
		IncludedProposals: includedProposals,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc ProcessCommit: %w", err)
	}
	return resp.GetNewGroupState(), resp.GetNewTreeHash(), nil
}

func (g *GrpcMLSEngine) ProcessWelcome(ctx context.Context, welcomeBytes, signingKey, keyPackageBundlePrivate []byte) (groupState, treeHash []byte, epoch uint64, err error) {
	resp, err := g.client.ProcessWelcome(ctx, &mls_service.ProcessWelcomeRequest{
		WelcomeBytes:            welcomeBytes,
		SigningKey:              truncateSigningKey(signingKey),
		KeyPackageBundlePrivate: keyPackageBundlePrivate,
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("grpc ProcessWelcome: %w", err)
	}
	return resp.GetGroupState(), resp.GetTreeHash(), resp.GetEpoch(), nil
}

func (g *GrpcMLSEngine) GenerateKeyPackage(ctx context.Context, signingKey []byte) (keyPackageBytes, keyPackageBundlePrivate []byte, err error) {
	resp, err := g.client.GenerateKeyPackage(ctx, &mls_service.GenerateKeyPackageRequest{
		SigningKey: truncateSigningKey(signingKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc GenerateKeyPackage: %w", err)
	}
	return resp.GetKeyPackageBytes(), resp.GetKeyPackageBundlePrivate(), nil
}

func (g *GrpcMLSEngine) AddMembers(ctx context.Context, groupState []byte, keyPackages [][]byte) (commitBytes, welcomeBytes, newGroupState, newTreeHash []byte, err error) {
	resp, err := g.client.AddMembers(ctx, &mls_service.AddMembersRequest{
		GroupState:  groupState,
		KeyPackages: keyPackages,
	})
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("grpc AddMembers: %w", err)
	}
	return resp.GetCommitBytes(), resp.GetWelcomeBytes(), resp.GetNewGroupState(), resp.GetNewTreeHash(), nil
}

func (g *GrpcMLSEngine) RemoveMembers(ctx context.Context, groupState []byte, targetIdentities [][]byte) (commitBytes, newGroupState, newTreeHash []byte, err error) {
	resp, err := g.client.RemoveMembers(ctx, &mls_service.RemoveMembersRequest{
		GroupState:       groupState,
		TargetIdentities: targetIdentities,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("grpc RemoveMembers: %w", err)
	}
	return resp.GetCommitBytes(), resp.GetNewGroupState(), resp.GetNewTreeHash(), nil
}

func (g *GrpcMLSEngine) HasMember(ctx context.Context, groupState []byte, identity []byte) (bool, error) {
	resp, err := g.client.HasMember(ctx, &mls_service.HasMemberRequest{
		GroupState: groupState,
		Identity:   identity,
	})
	if err != nil {
		return false, fmt.Errorf("grpc HasMember: %w", err)
	}
	return resp.GetIsMember(), nil
}

func (g *GrpcMLSEngine) ListMemberIdentities(ctx context.Context, groupState []byte) ([][]byte, error) {
	resp, err := g.client.ListMemberIdentities(ctx, &mls_service.ListMemberIdentitiesRequest{
		GroupState: groupState,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ListMemberIdentities: %w", err)
	}
	return resp.GetIdentities(), nil
}

func (g *GrpcMLSEngine) EncryptMessage(ctx context.Context, groupState []byte, plaintext []byte) (ciphertext, newGroupState []byte, err error) {
	resp, err := g.client.EncryptMessage(ctx, &mls_service.EncryptMessageRequest{
		GroupState: groupState,
		Plaintext:  plaintext,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc EncryptMessage: %w", err)
	}
	return resp.GetCiphertext(), resp.GetNewGroupState(), nil
}

func (g *GrpcMLSEngine) DecryptMessage(ctx context.Context, groupState []byte, ciphertext []byte) (plaintext, newGroupState []byte, err error) {
	resp, err := g.client.DecryptMessage(ctx, &mls_service.DecryptMessageRequest{
		GroupState: groupState,
		Ciphertext: ciphertext,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("grpc DecryptMessage: %w", err)
	}
	return resp.GetPlaintext(), resp.GetNewGroupState(), nil
}

func (g *GrpcMLSEngine) ExternalJoin(ctx context.Context, groupInfo, signingKey []byte) (groupState, commitBytes, treeHash []byte, err error) {
	resp, err := g.client.ExternalJoin(ctx, &mls_service.ExternalJoinRequest{
		GroupInfo:  groupInfo,
		SigningKey: truncateSigningKey(signingKey),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("grpc ExternalJoin: %w", err)
	}
	return resp.GetGroupState(), resp.GetCommitBytes(), resp.GetTreeHash(), nil
}

func (g *GrpcMLSEngine) ExportGroupInfo(ctx context.Context, groupState []byte, withRatchetTree bool) (groupInfo []byte, err error) {
	resp, err := g.client.ExportGroupInfo(ctx, &mls_service.ExportGroupInfoRequest{
		GroupState:      groupState,
		WithRatchetTree: withRatchetTree,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportGroupInfo: %w", err)
	}
	return resp.GetGroupInfo(), nil
}

func (g *GrpcMLSEngine) ExportSecret(ctx context.Context, groupState []byte, label string, context []byte, length int) (secret []byte, err error) {
	resp, err := g.client.ExportSecret(ctx, &mls_service.ExportSecretRequest{
		GroupState: groupState,
		Label:      label,
		Length:     uint32(length),
		Context:    append([]byte(nil), context...),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc ExportSecret: %w", err)
	}
	return resp.GetSecret(), nil
}
