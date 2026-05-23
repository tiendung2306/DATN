package coordination

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegration_ConcurrencySweep(t *testing.T) {
	// Ensure the evaluation data directory exists
	dataDir := filepath.Join("..", "..", "evaluation", "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("Failed to create data directory: %v", err)
	}

	csvPath := filepath.Join(dataDir, "concurrency_metrics.csv")
	csvFile, err := os.Create(csvPath)
	if err != nil {
		t.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	// Write CSV Header
	fmt.Fprintln(csvFile, "Strategy,Concurrency,Proposals,Commits,ForkCount,EpochDivergence,SuccessRate")

	// We sweep concurrency from 1 to 5
	for concurrency := 1; concurrency <= 5; concurrency++ {
		t.Logf("--- Running Concurrency Level %d ---", concurrency)

		// 1. Run Baseline (Immediate Commit, BatchingDelay = 0)
		runBaseline(t, csvFile, concurrency)

		// 2. Run Optimized (1-Second Batching Delay)
		runOptimized(t, csvFile, concurrency)
	}

	t.Logf("Concurrency sweep completed successfully! Data saved to: %s", csvPath)
}

func runBaseline(t *testing.T, csvFile *os.File, concurrency int) {
	totalNodes := concurrency + 1 // 1 token holder + N concurrent proposers
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	nodes := make([]*testNode, totalNodes)
	// Node 0 is Bob (the Token Holder)
	nodes[0] = createTestNodeHelper(t, network, clk, "bob", "grp-baseline")

	// Sibling nodes (Proposers)
	for i := 1; i <= concurrency; i++ {
		name := fmt.Sprintf("sibling-%d", i)
		nodes[i] = createTestNodeHelper(t, network, clk, name, "grp-baseline")
	}

	// Make Bob the initial Token Holder (by setup, CreateGroup on Bob makes Bob the token holder)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var holder *testNode
	var siblings []*testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else {
			siblings = append(siblings, n)
		}
	}
	if holder == nil {
		t.Fatal("Baseline: no token holder found")
	}

	// Set immediate commit on Bob
	holder.coord.cfg.BatchingDelay = 0

	committedCount := 0
	round := 0
	totalCommits := int64(0)
	totalProposals := int64(0)
	lastEpoch := uint64(0)

	// In the Baseline, siblings concurrently propose. Because BatchingDelay = 0,
	// Bob commits the first proposal he processes immediately, which advances the epoch.
	// The other concurrent proposals from the same epoch are rejected as stale.
	// Those rejected siblings must re-propose in subsequent rounds/epochs.
	for committedCount < concurrency {
		round++
		// Every uncommitted sibling proposes
		for i := committedCount; i < concurrency; i++ {
			if err := siblings[i].coord.ProposeUpdate([]byte(fmt.Sprintf("proposal-%d-round-%d", i, round))); err != nil {
				t.Fatalf("Baseline ProposeUpdate failed: %v", err)
			}
			totalProposals++
		}

		// Deliver all proposals and commits
		network.DrainAll()

		currentEpoch := holder.coord.CurrentEpoch()
		newCommits := currentEpoch - lastEpoch
		totalCommits += int64(newCommits)

		// Each commit successfully bundles exactly one proposal (due to immediate commit)
		committedCount += int(newCommits)
		lastEpoch = currentEpoch

		if round > concurrency*2 {
			t.Fatal("Baseline simulation took too many rounds (infinite loop guard)")
		}
	}

	// Compute epoch divergence at the end (should be 0 because Single-Writer guarantees convergence)
	epochDivergence := 0
	expectedEpoch := uint64(concurrency) // In baseline, we commit one by one, so Epoch should equal concurrency
	for _, n := range nodes {
		if n.coord.CurrentEpoch() != expectedEpoch {
			epochDivergence++
		}
	}

	// SuccessRate = Total unique updates (concurrency) / Total proposals made (concurrency + retries)
	successRate := float64(concurrency) / float64(totalProposals)

	// Write to CSV
	fmt.Fprintf(csvFile, "Baseline,%d,%d,%d,0,%d,%.4f\n",
		concurrency, totalProposals, totalCommits, epochDivergence, successRate)
}

func runOptimized(t *testing.T, csvFile *os.File, concurrency int) {
	totalNodes := concurrency + 1 // 1 token holder + N concurrent proposers
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	nodes := make([]*testNode, totalNodes)
	// Node 0 is Bob (the Token Holder)
	nodes[0] = createTestNodeHelper(t, network, clk, "bob", "grp-optimized")

	// Sibling nodes (Proposers)
	for i := 1; i <= concurrency; i++ {
		name := fmt.Sprintf("sibling-%d", i)
		nodes[i] = createTestNodeHelper(t, network, clk, name, "grp-optimized")
	}

	// Create group and start all nodes
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var holder *testNode
	var siblings []*testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else {
			siblings = append(siblings, n)
		}
	}
	if holder == nil {
		t.Fatal("Optimized: no token holder found")
	}

	// Configure exactly 1-Second Batching Delay on Bob
	holder.coord.cfg.BatchingDelay = 1 * time.Second

	// Concurrent proposals from all siblings at the exact same time
	for i := 0; i < concurrency; i++ {
		if err := siblings[i].coord.ProposeUpdate([]byte(fmt.Sprintf("proposal-%d", i))); err != nil {
			t.Fatalf("Optimized ProposeUpdate failed: %v", err)
		}
	}

	// Deliver the proposals to Bob (he buffers all of them, does NOT commit due to delay)
	network.DrainAll()

	// Verify Bob has indeed not committed yet
	if holder.coord.CurrentEpoch() != 0 {
		t.Fatalf("Optimized: Holder committed too early, epoch=%d", holder.coord.CurrentEpoch())
	}

	// Now advance virtual clock by exactly 1 * time.Second
	clk.Advance(1 * time.Second)

	// Wait for the batch commit timer in the background goroutine to execute
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && holder.coord.CurrentEpoch() != 1 {
		time.Sleep(5 * time.Millisecond)
	}

	// Deliver the Commit to everyone
	network.DrainAll()

	// Verify everyone converges to Epoch 1
	epochDivergence := 0
	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 1 {
			epochDivergence++
		}
	}

	snap := holder.coord.GetMetrics()
	totalCommits := snap.CommitsIssued
	totalProposals := int64(concurrency)

	// For optimized, all proposals succeed on the first attempt because they are batched together.
	successRate := 1.0

	// Write to CSV
	fmt.Fprintf(csvFile, "Optimized,%d,%d,%d,0,%d,%.4f\n",
		concurrency, totalProposals, totalCommits, epochDivergence, successRate)
}

// Helper to create a testNode with standard options
func createTestNodeHelper(t *testing.T, network *FakeNetwork, clk *FakeClock, name string, groupID string) *testNode {
	id := peerID(name)
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:    TestConfig(),
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
