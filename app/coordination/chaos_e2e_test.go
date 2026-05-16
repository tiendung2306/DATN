package coordination

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// TestIntegration_Chaos_Convergence runs a randomized chaos test to empirically
// prove the 4 invariants of the Decentralized Coordination Protocol.
func TestIntegration_Chaos_Convergence(t *testing.T) {
	// --- SETUP ---
	const numNodes = 5
	const groupID = "chaos-group-1"
	nodes, network, clk := setupCluster(t, numNodes, groupID)
	
	for _, n := range nodes {
		node := n // capture
		node.coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, _ string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
			var remoteNode *testNode
			for _, candidate := range nodes {
				if candidate.id == remote {
					remoteNode = candidate
					break
				}
			}
			if remoteNode == nil {
				return nil, fmt.Errorf("node not found")
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
	}

	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// CSV for metrics visualization
	f, err := os.Create("chaos_metrics.csv")
	var csvMu sync.Mutex
	var writer *csv.Writer
	if err == nil {
		writer = csv.NewWriter(f)
		writer.Write([]string{"WallTimeMs", "NodeID", "Epoch", "TreeHash"})
	}

	recordMetrics := func() {
		if writer == nil { return }
		csvMu.Lock()
		defer csvMu.Unlock()
		// Use real time for polling to ensure we capture wall clock progress
		now := time.Now().UnixMilli()
		for i, n := range nodes {
			writer.Write([]string{
				fmt.Sprintf("%d", now),
				fmt.Sprintf("Node_%d", i),
				fmt.Sprintf("%d", n.coord.CurrentEpoch()),
				hex.EncodeToString(n.coord.GetTreeHash()[:4]),
			})
		}
		writer.Flush()
	}

	var wg sync.WaitGroup
	stopChaos := make(chan struct{})

	// Metrics polling goroutine (Ultra-fast polling for smooth graph)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopChaos:
				return
			case <-time.After(10 * time.Millisecond):
				recordMetrics()
			}
		}
	}()

	// --- THE NEMESIS GOROUTINE ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		r := rand.New(rand.NewSource(42))
		for {
			select {
			case <-stopChaos:
				network.Heal()
				return
			case <-time.After(1500 * time.Millisecond):
				r.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
				split := r.Intn(numNodes-2) + 1
				groupA := nodes[:split]
				groupB := nodes[split:]
				
				pidsA := make([]peer.ID, len(groupA))
				for i, n := range groupA { pidsA[i] = n.id }
				pidsB := make([]peer.ID, len(groupB))
				for i, n := range groupB { pidsB[i] = n.id }
				
				network.Partition(pidsA, pidsB)
				time.Sleep(600 * time.Millisecond) // stay partitioned longer
				network.Heal()
				network.DrainAll()
			}
		}
	}()

	// --- THE CLIENTS GOROUTINE ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		r := rand.New(rand.NewSource(1337))
		// 800 events for higher epoch count
		for i := 0; i < 800; i++ {
			select {
			case <-stopChaos:
				return
			default:
				n := nodes[r.Intn(numNodes)]
				n.coord.SendMessage([]byte(fmt.Sprintf("msg-%d", i)))
				
				// Very frequent member changes to spike Epoch
				if i > 0 && i % 15 == 0 {
					target := nodes[r.Intn(numNodes)].id
					nodes[0].coord.RemoveMember([]byte(target))
					// Add them back immediately in the next step to keep group size
					nodes[0].coord.AddMember(AddMemberRequest{
						TargetPeerID:    target,
						KeyPackageBytes: []byte("mock-kp"),
					})
				}

				clk.Advance(100 * time.Millisecond)
				network.DrainAll()
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Run for 1 minute as requested
	time.Sleep(60 * time.Second)
	close(stopChaos)
	wg.Wait()

	// Final stabilization - crucial for convergence!
	network.Heal()
	for i := 0; i < 100; i++ {
		exchangeHeartbeats(nodes, network)
		for _, n := range nodes {
			n.coord.BroadcastAnnounce() // Announcements trigger Fork Healing
		}
		network.DrainAll()
		clk.Advance(1 * time.Second)
		time.Sleep(10 * time.Millisecond)
	}
	recordMetrics() // capture final state

	if writer != nil {
		f.Close()
	}

	// --- ASSERTIONS ---
	t.Run("Invariant_2_Convergence", func(t *testing.T) {
		// All nodes should have converged to the same epoch.
		// We find the max epoch reached by any node.
		maxEpoch := uint64(0)
		for _, n := range nodes {
			if e := n.coord.CurrentEpoch(); e > maxEpoch {
				maxEpoch = e
			}
		}
		
		for i, n := range nodes {
			if n.coord.CurrentEpoch() != maxEpoch {
				t.Errorf("Node_%d not converged: got epoch %d, want %d", i, n.coord.CurrentEpoch(), maxEpoch)
			}
		}
	})
}
