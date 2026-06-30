package coordination

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"app/mls_service"
)

// ProcessManager manages the Rust crypto-engine child process for testing.
type testProcessManager struct {
	port       int
	cmd        *exec.Cmd
	mu         sync.Mutex
	cancelFunc context.CancelFunc
}

func newTestProcessManager() *testProcessManager {
	return &testProcessManager{}
}

func (pm *testProcessManager) getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (pm *testProcessManager) StartEngine() (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	port, err := pm.getFreePort()
	if err != nil {
		return 0, err
	}
	pm.port = port

	executable := "crypto-engine"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}

	cwd, _ := os.Getwd()
	possiblePaths := []string{
		filepath.Join(cwd, "..", "..", "crypto-engine", "target", "release", executable),
		filepath.Join(cwd, "..", "..", "crypto-engine", "target", "debug", executable),
		filepath.Join(cwd, "..", "crypto-engine", "target", "release", executable),
		filepath.Join(cwd, "..", "crypto-engine", "target", "debug", executable),
	}

	var binPath string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			binPath = p
			break
		}
	}

	if binPath == "" {
		return 0, fmt.Errorf("crypto-engine binary not found")
	}

	ctx, cancel := context.WithCancel(context.Background())
	pm.cancelFunc = cancel

	cmd := exec.CommandContext(ctx, binPath, "--port", fmt.Sprintf("%d", port))
	if err := cmd.Start(); err != nil {
		cancel()
		return 0, err
	}

	pm.cmd = cmd
	go cmd.Wait()

	// Wait a moment for server to start
	time.Sleep(500 * time.Millisecond)

	return port, nil
}

func (pm *testProcessManager) StopEngine() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.cancelFunc != nil {
		pm.cancelFunc()
	}
}

// testGrpcMLSEngine adapts the gRPC MLSCryptoServiceClient to MLSEngine for tests.
type testGrpcMLSEngine struct {
	client mls_service.MLSCryptoServiceClient
}

func newTestGrpcMLSEngine(client mls_service.MLSCryptoServiceClient) MLSEngine {
	return &testGrpcMLSEngine{client: client}
}

func (g *testGrpcMLSEngine) CreateGroup(ctx context.Context, groupID string, signingKey []byte, maxPastEpochs uint32) ([]byte, []byte, error) {
	k := signingKey
	if len(k) > 32 {
		k = k[:32]
	}
	resp, err := g.client.CreateGroup(ctx, &mls_service.CreateGroupRequest{GroupId: groupID, SigningKey: k, MaxPastEpochs: maxPastEpochs})
	if err != nil {
		return nil, nil, err
	}
	return resp.GetGroupState(), resp.GetTreeHash(), nil
}

func (g *testGrpcMLSEngine) CreateProposal(ctx context.Context, groupState []byte, pType ProposalType, data []byte) (CreateProposalResult, error) {
	resp, err := g.client.CreateProposal(ctx, &mls_service.CreateProposalRequest{GroupState: groupState, ProposalType: mls_service.MlsProposalType(pType), Data: data})
	if err != nil {
		return CreateProposalResult{}, err
	}
	return CreateProposalResult{ProposalBytes: resp.GetProposalBytes(), ProposalRef: resp.GetProposalRef(), NewGroupState: resp.GetNewGroupState()}, nil
}

func (g *testGrpcMLSEngine) ProcessProposal(ctx context.Context, groupState []byte, proposalBytes []byte) (ProcessProposalResult, error) {
	resp, err := g.client.ProcessProposal(ctx, &mls_service.ProcessProposalRequest{GroupState: groupState, ProposalBytes: proposalBytes})
	if err != nil {
		return ProcessProposalResult{}, err
	}
	return ProcessProposalResult{ProposalRef: resp.GetProposalRef(), ProposalType: resp.GetProposalType(), NewGroupState: resp.GetNewGroupState()}, nil
}

func (g *testGrpcMLSEngine) CreateCommit(ctx context.Context, groupState []byte, expectedProposalRefs [][]byte) (CreateCommitResult, error) {
	resp, err := g.client.CreateCommit(ctx, &mls_service.CreateCommitRequest{GroupState: groupState, ExpectedProposalRefs: expectedProposalRefs})
	if err != nil {
		return CreateCommitResult{}, err
	}
	return CreateCommitResult{
		CommitBytes:           resp.GetCommitBytes(),
		WelcomeBytes:          resp.GetWelcomeBytes(),
		GroupInfo:             resp.GetGroupInfo(),
		CommittedProposalRefs: resp.GetCommittedProposalRefs(),
		NewGroupState:         resp.GetNewGroupState(),
		NewTreeHash:           resp.GetNewTreeHash(),
	}, nil
}

func (g *testGrpcMLSEngine) StageCommit(ctx context.Context, groupState []byte, commitBytes []byte, includedProposals [][]byte) (StageCommitResult, error) {
	resp, err := g.client.StageCommit(ctx, &mls_service.StageCommitRequest{GroupState: groupState, CommitBytes: commitBytes, IncludedProposals: includedProposals})
	if err != nil {
		return StageCommitResult{}, err
	}
	return StageCommitResult{ProposalRefs: resp.GetProposalRefs()}, nil
}

func (g *testGrpcMLSEngine) ProcessCommit(ctx context.Context, groupState []byte, commitBytes []byte, includedProposals [][]byte) ([]byte, []byte, error) {
	resp, err := g.client.ProcessCommit(ctx, &mls_service.ProcessCommitRequest{GroupState: groupState, CommitBytes: commitBytes, IncludedProposals: includedProposals})
	if err != nil {
		return nil, nil, err
	}
	return resp.GetNewGroupState(), resp.GetNewTreeHash(), nil
}

func (g *testGrpcMLSEngine) ProcessWelcome(ctx context.Context, welcomeBytes, signingKey, keyPackageBundlePrivate []byte, maxPastEpochs uint32) ([]byte, []byte, uint64, error) {
	k := signingKey
	if len(k) > 32 {
		k = k[:32]
	}
	resp, err := g.client.ProcessWelcome(ctx, &mls_service.ProcessWelcomeRequest{WelcomeBytes: welcomeBytes, SigningKey: k, KeyPackageBundlePrivate: keyPackageBundlePrivate, MaxPastEpochs: maxPastEpochs})
	if err != nil {
		return nil, nil, 0, err
	}
	return resp.GetGroupState(), resp.GetTreeHash(), resp.GetEpoch(), nil
}

func (g *testGrpcMLSEngine) GenerateKeyPackage(ctx context.Context, signingKey []byte) ([]byte, []byte, error) {
	k := signingKey
	if len(k) > 32 {
		k = k[:32]
	}
	resp, err := g.client.GenerateKeyPackage(ctx, &mls_service.GenerateKeyPackageRequest{SigningKey: k})
	if err != nil {
		return nil, nil, err
	}
	return resp.GetKeyPackageBytes(), resp.GetKeyPackageBundlePrivate(), nil
}

func (g *testGrpcMLSEngine) AddMembers(ctx context.Context, groupState []byte, keyPackages [][]byte) ([]byte, []byte, []byte, []byte, error) {
	resp, err := g.client.AddMembers(ctx, &mls_service.AddMembersRequest{GroupState: groupState, KeyPackages: keyPackages})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return resp.GetCommitBytes(), resp.GetWelcomeBytes(), resp.GetNewGroupState(), resp.GetNewTreeHash(), nil
}

func (g *testGrpcMLSEngine) RemoveMembers(ctx context.Context, groupState []byte, targetIdentities [][]byte) ([]byte, []byte, []byte, error) {
	resp, err := g.client.RemoveMembers(ctx, &mls_service.RemoveMembersRequest{GroupState: groupState, TargetIdentities: targetIdentities})
	if err != nil {
		return nil, nil, nil, err
	}
	return resp.GetCommitBytes(), resp.GetNewGroupState(), resp.GetNewTreeHash(), nil
}

func (g *testGrpcMLSEngine) HasMember(ctx context.Context, groupState []byte, identity []byte) (bool, error) {
	resp, err := g.client.HasMember(ctx, &mls_service.HasMemberRequest{GroupState: groupState, Identity: identity})
	if err != nil {
		return false, err
	}
	return resp.GetIsMember(), nil
}

func (g *testGrpcMLSEngine) ListMemberIdentities(ctx context.Context, groupState []byte) ([][]byte, error) {
	resp, err := g.client.ListMemberIdentities(ctx, &mls_service.ListMemberIdentitiesRequest{GroupState: groupState})
	if err != nil {
		return nil, err
	}
	return resp.GetIdentities(), nil
}

func (g *testGrpcMLSEngine) EncryptMessage(ctx context.Context, groupState []byte, plaintext []byte) ([]byte, []byte, error) {
	resp, err := g.client.EncryptMessage(ctx, &mls_service.EncryptMessageRequest{GroupState: groupState, Plaintext: plaintext})
	if err != nil {
		return nil, nil, err
	}
	return resp.GetCiphertext(), resp.GetNewGroupState(), nil
}

func (g *testGrpcMLSEngine) DecryptMessage(ctx context.Context, groupState []byte, ciphertext []byte) ([]byte, []byte, error) {
	resp, err := g.client.DecryptMessage(ctx, &mls_service.DecryptMessageRequest{GroupState: groupState, Ciphertext: ciphertext})
	if err != nil {
		return nil, nil, err
	}
	return resp.GetPlaintext(), resp.GetNewGroupState(), nil
}

func (g *testGrpcMLSEngine) ExternalJoin(ctx context.Context, groupInfo, signingKey []byte, maxPastEpochs uint32) ([]byte, []byte, []byte, error) {
	k := signingKey
	if len(k) > 32 {
		k = k[:32]
	}
	resp, err := g.client.ExternalJoin(ctx, &mls_service.ExternalJoinRequest{GroupInfo: groupInfo, SigningKey: k, MaxPastEpochs: maxPastEpochs})
	if err != nil {
		return nil, nil, nil, err
	}
	return resp.GetGroupState(), resp.GetCommitBytes(), resp.GetTreeHash(), nil
}

func (g *testGrpcMLSEngine) ExportGroupInfo(ctx context.Context, groupState []byte, withRatchetTree bool) ([]byte, error) {
	resp, err := g.client.ExportGroupInfo(ctx, &mls_service.ExportGroupInfoRequest{GroupState: groupState, WithRatchetTree: withRatchetTree})
	if err != nil {
		return nil, err
	}
	return resp.GetGroupInfo(), nil
}

func (g *testGrpcMLSEngine) ExportSecret(ctx context.Context, groupState []byte, label string, exporterContext []byte, length int) ([]byte, error) {
	resp, err := g.client.ExportSecret(ctx, &mls_service.ExportSecretRequest{GroupState: groupState, Label: label, Context: exporterContext, Length: uint32(length)})
	if err != nil {
		return nil, err
	}
	return resp.GetSecret(), nil
}
