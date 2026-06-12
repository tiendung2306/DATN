package coordination

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestCoordinator_CensorshipFailover(t *testing.T) {
	nodes, network, clk := setupCluster(t, 3, "grp-censorship")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Determine who the initial token holder is
	var holder, proposer *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else if proposer == nil {
			proposer = n
		}
	}
	if holder == nil || proposer == nil {
		t.Fatal("failed to identify roles")
	}

	// Isolate the token holder so it never receives the proposal (simulating Byzantine censorship or network drop).
	// The other 2 nodes will still have it in their ActiveView until heartbeat timeout (which is much longer).
	others := []peer.ID{}
	for _, n := range nodes {
		if n.id != holder.id {
			others = append(others, n.id)
		}
	}
	network.Partition([]peer.ID{holder.id}, others)

	// Proposer creates a proposal
	if err := proposer.coord.ProposeUpdate([]byte("rotation-against-censorship")); err != nil {
		t.Fatal(err)
	}
	network.DrainAll()

	if proposer.coord.singleWriter.ProposalCount() == 0 {
		t.Fatal("proposer should have buffered its own proposal")
	}

	// Fast forward the clock past the TokenHolderTimeout
	clk.Advance(nodes[0].coord.cfg.TokenHolderTimeout + 1*time.Second)
	network.DrainAll()

	// Wait for the failover timer goroutines to process
	time.Sleep(100 * time.Millisecond) // yield to allow goroutines to run and lock
	
	// The new token holder will schedule a batch commit. Advance clock past BatchingDelay.
	clk.Advance(nodes[0].coord.cfg.BatchingDelay + 100*time.Millisecond)
	network.DrainAll()

	// Both non-isolated nodes should now be at epoch 1
	waitForCondition(t, 2*time.Second, func() bool {
		advancedCount := 0
		for _, n := range nodes {
			if n.id != holder.id && n.coord.CurrentEpoch() == 1 {
				advancedCount++
			}
		}
		return advancedCount == 2
	}, "failover token holder should commit and advance epoch")

	if holder.coord.CurrentEpoch() != 0 {
		t.Errorf("isolated holder should still be at epoch 0, got %d", holder.coord.CurrentEpoch())
	}
}
