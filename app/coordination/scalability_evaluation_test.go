package coordination

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// SweepConfig returns optimal parameters for synchronous high-scale (1000-node) sweeps.
// Tickers are practically disabled via 1-hour durations to prevent goroutine thrashing.
func SweepConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		TokenHolderTimeout:     1 * time.Hour,
		HeartbeatInterval:      1 * time.Hour,
		AnnounceInterval:       0,
		PeerDeadAfter:          3,
		MaxBatchedProposals:    10000,
		KeyRotationInterval:    0,
		ReplayThrottleMs:       0,
		MetricsEnabled:         true,
		OfflineSyncEnabled:     false,
		EnvelopeLogTTL:         7 * 24 * time.Hour,
		EnvelopeLogMaxPerGroup: 10000,
		BatchingDelay:          0, // Immediate commit on token holder
	}
}

func createSweepNodeHelper(t *testing.T, network *FakeNetwork, clk *FakeClock, name string, groupID string) *testNode {
	id := peerID(name)
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:    SweepConfig(),
		Transport: transport,
		Clock:     clk,
		MLS:       mls,
		Storage:   storage,
		LocalID:   id,
		GroupID:   groupID,
	})
	if err != nil {
		t.Fatalf("NewCoordinator helper failed for %s: %v", name, err)
	}
	return &testNode{id: id, coord: coord, mls: mls, storage: storage}
}

func TestIntegration_EpochConvergenceSweep(t *testing.T) {
	// Silence logging to prevent massive CPU thrashing and disk I/O overhead
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Ensure the evaluation data directory exists
	dataDir := filepath.Join("..", "..", "evaluation", "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("Failed to create data directory: %v", err)
	}

	csvPath := filepath.Join(dataDir, "epoch_convergence_metrics.csv")
	csvFile, err := os.Create(csvPath)
	if err != nil {
		t.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	// Write CSV Header
	fmt.Fprintln(csvFile, "GroupSize,AddMemberMs,RemoveMemberMs,UpdateMemberMs")

	// We sweep exponentially to N = 1000 nodes
	sweepPoints := []int{5, 10, 50, 100, 250, 500, 750, 1000}

	for _, N := range sweepPoints {
		t.Logf("--- Benchmarking Group Size N = %d ---", N)

		// 1. Measure Add Member
		addDur := benchmarkAddMember(t, N)

		// 2. Measure Remove Member
		removeDur := benchmarkRemoveMember(t, N)

		// 3. Measure Update Proposal/Commit
		updateDur := benchmarkUpdateMember(t, N)

		// Write results to CSV
		fmt.Fprintf(csvFile, "%d,%.4f,%.4f,%.4f\n",
			N,
			float64(addDur.Nanoseconds())/1e6,
			float64(removeDur.Nanoseconds())/1e6,
			float64(updateDur.Nanoseconds())/1e6,
		)
	}

	t.Logf("Epoch convergence sweep completed successfully! Data saved to: %s", csvPath)
}

func benchmarkAddMember(t *testing.T, N int) time.Duration {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := fmt.Sprintf("sweep-add-%d", N)

	// Create N nodes
	nodes := make([]*testNode, N)
	for i := 0; i < N; i++ {
		name := fmt.Sprintf("node-%d", i)
		nodes[i] = createSweepNodeHelper(t, network, clk, name, groupID)
	}

	// Pre-populate group with N members at Epoch 10
	prePopulateMockGroup(t, nodes, groupID, 10)
	startAllNodes(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Sibling node (Node 1) proposes adding a new member
	targetPeer := peerID("invitee-target")

	start := time.Now()

	// Trigger Add Member Proposal
	req := AddMemberRequest{
		TargetPeerID:    targetPeer,
		KeyPackageBytes: []byte("mock-kp"),
		OperationID:     "op-add-id",
	}
	_, err := nodes[1].coord.AddMember(req)
	if err != nil {
		t.Fatalf("AddMember proposal failed: %v", err)
	}

	// Deliver Proposal to the Token Holder
	network.DrainAll()

	// Deliver Commit back to all nodes in the cluster
	network.DrainAll()

	elapsed := time.Since(start)

	// Verify all nodes converged to Epoch 11
	for i, n := range nodes {
		if n.coord.CurrentEpoch() != 11 {
			t.Fatalf("Node %d did not reach expected epoch 11, got %d", i, n.coord.CurrentEpoch())
		}
	}

	return elapsed
}

func benchmarkRemoveMember(t *testing.T, N int) time.Duration {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := fmt.Sprintf("sweep-remove-%d", N)

	// Create N nodes
	nodes := make([]*testNode, N)
	for i := 0; i < N; i++ {
		name := fmt.Sprintf("node-%d", i)
		nodes[i] = createSweepNodeHelper(t, network, clk, name, groupID)
	}

	// Pre-populate group with N members at Epoch 10
	prePopulateMockGroup(t, nodes, groupID, 10)
	startAllNodes(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Sibling node (Node 1) proposes removing Node 2
	targetIdentity := []byte(nodes[2].id)

	start := time.Now()

	// Trigger Remove Member Proposal
	err := nodes[1].coord.RemoveMember(targetIdentity)
	if err != nil {
		t.Fatalf("RemoveMember proposal failed: %v", err)
	}

	// Deliver Proposal to the Token Holder
	network.DrainAll()

	// Deliver Commit back to all nodes in the cluster
	network.DrainAll()

	elapsed := time.Since(start)

	// Verify remaining active nodes converged to Epoch 11
	// Note: Node 2 itself won't advance since it was removed
	for i, n := range nodes {
		if i == 2 {
			continue
		}
		if n.coord.CurrentEpoch() != 11 {
			t.Fatalf("Node %d did not reach expected epoch 11, got %d", i, n.coord.CurrentEpoch())
		}
	}

	return elapsed
}

func benchmarkUpdateMember(t *testing.T, N int) time.Duration {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := fmt.Sprintf("sweep-update-%d", N)

	// Create N nodes
	nodes := make([]*testNode, N)
	for i := 0; i < N; i++ {
		name := fmt.Sprintf("node-%d", i)
		nodes[i] = createSweepNodeHelper(t, network, clk, name, groupID)
	}

	// Pre-populate group with N members at Epoch 10
	prePopulateMockGroup(t, nodes, groupID, 10)
	startAllNodes(t, nodes)
	exchangeHeartbeats(nodes, network)

	start := time.Now()

	// Sibling node (Node 1) proposes key rotation update
	err := nodes[1].coord.ProposeUpdate([]byte("key-rotation-bytes"))
	if err != nil {
		t.Fatalf("ProposeUpdate failed: %v", err)
	}

	// Deliver Proposal to the Token Holder
	network.DrainAll()

	// Deliver Commit back to all nodes in the cluster
	network.DrainAll()

	elapsed := time.Since(start)

	// Verify all nodes converged to Epoch 11
	for i, n := range nodes {
		if n.coord.CurrentEpoch() != 11 {
			t.Fatalf("Node %d did not reach expected epoch 11, got %d", i, n.coord.CurrentEpoch())
		}
	}

	return elapsed
}

// prePopulateMockGroup creates a group record of size N and initializes it directly
// in Go storage & memory for all coordinators, avoiding slow successive joins.
func prePopulateMockGroup(t *testing.T, nodes []*testNode, groupID string, epoch uint64) []byte {
	th := mockTreeHash(epoch)
	members := make(map[peer.ID]bool)
	mockMembers := make(map[string]bool)

	for _, n := range nodes {
		members[n.id] = true
		identity := []byte(n.id)
		mockMembers[string(identity)] = true
	}

	// Construct mock OpenMLS group state JSON representation
	state := mockGroupState{
		GroupID:    groupID,
		Epoch:      epoch,
		TreeHash:   hex.EncodeToString(th),
		Members:    mockMembers,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Failed to marshal mock group state: %v", err)
	}

	// Save to Storage & Coordinator memory for all nodes
	pids := make([]peer.ID, 0, len(nodes))
	for _, n := range nodes {
		pids = append(pids, n.id)
	}

	for _, n := range nodes {
		// Populate Group Record in SQLite (MockStorage)
		err := n.storage.SaveGroupRecord(&GroupRecord{
			GroupID:    groupID,
			GroupState: stateBytes,
			Epoch:      epoch,
			TreeHash:   th,
			MyRole:     RoleMember,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		})
		if err != nil {
			t.Fatalf("Failed to pre-populate mock group record: %v", err)
		}

		// Populate Coord State in SQLite (MockStorage)
		err = n.storage.SaveCoordState(&CoordState{
			GroupID:    groupID,
			ActiveView: pids,
		})
		if err != nil {
			t.Fatalf("Failed to pre-populate mock coord state: %v", err)
		}

		// Initialize Group directly in Coordinator Memory
		n.coord.InitializeGroup(stateBytes, epoch, th)
	}

	return stateBytes
}

func startAllNodes(t *testing.T, nodes []*testNode) {
	ctx := context.Background()
	for i, n := range nodes {
		if err := n.coord.Start(ctx); err != nil {
			t.Fatalf("Start[%d]: %v", i, err)
		}
	}
	t.Cleanup(func() {
		for _, n := range nodes {
			n.coord.Stop()
		}
	})
}
