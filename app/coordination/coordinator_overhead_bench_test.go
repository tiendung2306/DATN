package coordination

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"

	"app/mls_service"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TestIntegration_CoordinatorOverhead measures the cost breakdown between
// the Go coordination layer and the Rust MLS crypto layer by running the
// same AddMember operation with MockMLS (coordination only) and Real MLS
// (coordination + crypto). The difference isolates the crypto cost.
func TestIntegration_CoordinatorOverhead(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	dataDir := filepath.Join("..", "..", "evaluation", "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("Failed to create data directory: %v", err)
	}

	csvPath := filepath.Join(dataDir, "coordinator_overhead_metrics.csv")
	csvFile, err := os.Create(csvPath)
	if err != nil {
		t.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	fmt.Fprintln(csvFile, "GroupSize,MockMs,RealMs,CryptoMs,CoordinationPct")

	// --- Phase 1: MockMLS sweep (coordination only) ---
	mockSizes := []int{16, 32, 64, 128, 256, 512, 1000}
	mockResults := make(map[int]float64)

	for _, n := range mockSizes {
		t.Logf("--- MockMLS AddMember N=%d ---", n)
		elapsed := benchmarkMockAddMember(t, n)
		mockResults[n] = float64(elapsed.Nanoseconds()) / 1e6
		t.Logf("MockMLS N=%d: %.2f ms", n, mockResults[n])
	}

	// --- Phase 2: Real MLS sweep (coordination + crypto) ---
	realSizes := []int{16, 32, 64}
	realResults := make(map[int]float64)

	pm := newTestProcessManager()
	port, err := pm.StartEngine()
	if err != nil {
		t.Fatalf("Failed to start real MLS engine: %v", err)
	}
	defer pm.StopEngine()

	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024), grpc.MaxCallSendMsgSize(64*1024*1024))) //nolint:staticcheck
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	realEngine := newTestCachedGrpcMLSEngine(mls_service.NewMLSCryptoServiceClient(conn))

	for _, n := range realSizes {
		t.Logf("--- Real MLS AddMember N=%d ---", n)
		elapsed := benchmarkRealAddMember(t, n, realEngine)
		realResults[n] = float64(elapsed.Nanoseconds()) / 1e6
		t.Logf("Real MLS N=%d: %.2f ms", n, realResults[n])
	}

	// --- Write CSV ---
	for _, n := range mockSizes {
		mockMs := mockResults[n]
		realMs, hasReal := realResults[n]
		if !hasReal {
			fmt.Fprintf(csvFile, "%d,%.2f,N/A,N/A,N/A\n", n, mockMs)
			continue
		}
		cryptoMs := realMs - mockMs
		coordPct := (mockMs / realMs) * 100
		fmt.Fprintf(csvFile, "%d,%.2f,%.2f,%.2f,%.2f\n", n, mockMs, realMs, cryptoMs, coordPct)
	}

	t.Logf("Coordinator overhead benchmark completed. CSV saved to: %s", csvPath)
}

// benchmarkMockAddMember measures AddMember latency using MockMLS (coordination only).
// Uses prePopulateMockGroup to inject N members at epoch 10, then measures a single
// AddMember proposal → commit → drain cycle.
func benchmarkMockAddMember(t *testing.T, n int) time.Duration {
	t.Helper()
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := fmt.Sprintf("overhead-mock-%d", n)

	nodes := make([]*testNode, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("node-%d", i)
		nodes[i] = createSweepNodeHelper(t, network, clk, name, groupID)
	}

	prePopulateMockGroup(t, nodes, groupID, 10)
	startAllNodes(t, nodes)
	exchangeHeartbeats(nodes, network)

	targetPeer := peerID("invitee-overhead")

	start := time.Now()

	req := AddMemberRequest{
		TargetPeerID:    targetPeer,
		KeyPackageBytes: []byte("mock-kp"),
		OperationID:     "op-overhead",
	}
	if _, err := nodes[1].coord.AddMember(req); err != nil {
		t.Fatalf("Mock AddMember failed: %v", err)
	}

	network.DrainAll()
	network.DrainAll()

	elapsed := time.Since(start)

	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 11 {
			t.Fatalf("Node did not reach epoch 11, got %d", n.coord.CurrentEpoch())
		}
	}

	return elapsed
}

// benchmarkRealAddMember measures AddMember latency using Real MLS engine
// (coordination + crypto). Creates a group with N members via AddMembers batch,
// then measures a single AddMember proposal → commit → drain cycle.
func benchmarkRealAddMember(t *testing.T, n int, engine MLSEngine) time.Duration {
	t.Helper()
	network := NewFakeNetwork()
	clk := realClockImpl{}
	groupID := fmt.Sprintf("overhead-real-%d", n)

	identityToPeer := make(map[string]peer.ID)
	for i := 0; i < n; i++ {
		sigKey := make([]byte, 32)
		sigKey[0] = byte(i + 1)
		ident := deriveIdentityFromSigningKey(sigKey)
		identityToPeer[string(ident)] = peerID(fmt.Sprintf("node-%d", i))
	}

	nodes := make([]*benchNode, n)
	for i := 0; i < n; i++ {
		id := peerID(fmt.Sprintf("node-%d", i))
		transport := network.AddNode(id)
		storage := NewMockStorage()

		sigKey := make([]byte, 32)
		sigKey[0] = byte(i + 1)

		coord, err := NewCoordinator(CoordinatorOpts{
			Config:     benchConfig(),
			Transport:  transport,
			Clock:      clk,
			MLS:        engine,
			Storage:    storage,
			LocalID:    id,
			GroupID:    groupID,
			SigningKey: sigKey,
		})
		if err != nil {
			t.Fatalf("NewCoordinator[%d]: %v", i, err)
		}

		coord.authorizedCommitters = func(groupID string, epoch uint64, batch []BufferedProposal) ([]peer.ID, error) {
			gs := coord.groupState
			if len(gs) == 0 {
				return nil, nil
			}
			gsCopy := make([]byte, len(gs))
			copy(gsCopy, gs)
			identities, err := engine.ListMemberIdentities(context.Background(), gsCopy)
			if err != nil {
				return nil, err
			}
			var pids []peer.ID
			for _, ident := range identities {
				if pid, ok := identityToPeer[string(ident)]; ok {
					pids = append(pids, pid)
				}
			}
			return pids, nil
		}

		coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, _ string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
			var remoteNode *benchNode
			for _, candidate := range nodes {
				if candidate != nil && candidate.id == remote {
					remoteNode = candidate
					break
				}
			}
			if remoteNode == nil {
				return nil, fmt.Errorf("node not found: %s", remote)
			}
			groupInfo, err := remoteNode.mls.ExportGroupInfo(ctx, remoteNode.coord.GetGroupState(), withRatchetTree)
			if err != nil {
				return nil, err
			}
			return &GroupInfoFetchResult{
				GroupInfo: groupInfo,
				Epoch:     remoteNode.coord.CurrentEpoch(),
				TreeHash:  remoteNode.coord.GetTreeHash(),
			}, nil
		}

		nodes[i] = &benchNode{id: id, coord: coord, mls: engine, storage: storage, sigKey: sigKey}
	}

	// Setup: create group + add all members
	if err := nodes[0].coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	if n > 1 {
		ctx := context.Background()
		kps := make([][]byte, n-1)
		kpPrivs := make([][]byte, n-1)
		for i := 1; i < n; i++ {
			kp, kpPriv, err := engine.GenerateKeyPackage(ctx, nodes[i].sigKey)
			if err != nil {
				t.Fatalf("GenerateKeyPackage[%d]: %v", i, err)
			}
			kps[i-1] = kp
			kpPrivs[i-1] = kpPriv
		}

		gs0 := nodes[0].coord.GetGroupState()
		_, welcomeBytes, newGS, newTH, err := engine.AddMembers(ctx, gs0, kps)
		if err != nil {
			t.Fatalf("AddMembers: %v", err)
		}

		nodes[0].coord.SetStateForTest(nodes[0].coord.CurrentEpoch()+1, newGS, newTH)

		for i := 1; i < n; i++ {
			memberGS, memberTH, epoch, err := engine.ProcessWelcome(ctx, welcomeBytes, nodes[i].sigKey, kpPrivs[i-1], 0)
			if err != nil {
				t.Fatalf("ProcessWelcome[%d]: %v", i, err)
			}
			nodes[i].coord.SetStateForTest(epoch, memberGS, memberTH)
		}
	}

	// Start all nodes
	for _, n := range nodes {
		if err := n.coord.Start(context.Background()); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	defer func() {
		for _, n := range nodes {
			n.coord.Stop()
		}
	}()
	exchangeHeartbeatsForBench(nodes, network)

	// Generate a real KeyPackage for the invitee
	inviteeSigKey := make([]byte, 32)
	inviteeSigKey[0] = byte(255)
	ctx := context.Background()
	inviteeKP, _, err := engine.GenerateKeyPackage(ctx, inviteeSigKey)
	if err != nil {
		t.Fatalf("GenerateKeyPackage for invitee: %v", err)
	}

	// Set onAddCommitted for Welcome forwarding (not needed for AddMember but for safety)
	for _, bn := range nodes {
		bn := bn
		bn.coord.onAddCommitted = func(delivery AddCommitDelivery, epoch uint64, welcome []byte) {
			for _, candidate := range nodes {
				if candidate.id.String() == delivery.TargetPeerID {
					go candidate.coord.ProcessWelcomeIfWaiting(context.Background(), welcome)
					break
				}
			}
		}
	}

	// --- Measure AddMember ---
	start := time.Now()

	inviteePeer := peerID("invitee-real")
	req := AddMemberRequest{
		TargetPeerID:    inviteePeer,
		KeyPackageBytes: inviteeKP,
		OperationID:     "op-overhead-real",
	}
	if _, err := nodes[1].coord.AddMember(req); err != nil {
		t.Fatalf("Real AddMember failed: %v", err)
	}

	// Drain proposal → Token Holder, then commit → all nodes
	network.DrainAll()
	network.DrainAll()

	elapsed := time.Since(start)

	return elapsed
}
