package coordination

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc"

	"app/mls_service"
	"github.com/libp2p/go-libp2p/core/peer"
)

type benchNode struct {
	id      peer.ID
	coord   *Coordinator
	mls     MLSEngine
	storage *MockStorage
	sigKey  []byte // 32-byte signing key cho node này
}

// realClock là real-time clock cho benchmark (không dùng FakeClock).
type realClockImpl struct{}

func (realClockImpl) Now() time.Time                         { return time.Now() }
func (realClockImpl) After(d time.Duration) <-chan time.Time { return time.After(d) }

// benchConfig trả về config cho benchmark: BatchingDelay nhỏ để commit nhanh.
func benchConfig() *CoordinatorConfig {
	cfg := DefaultConfig()
	cfg.BatchingDelay = 0 // Immediate commit — eliminates batching delay noise in healing measurement
	cfg.HeartbeatInterval = 5 * time.Second
	cfg.MLSOperationTimeout = 10 * time.Second // real Rust gRPC cần timeout lớn hơn
	cfg.TokenHolderTimeout = 5 * time.Second   // Production-like timeout
	cfg.PeerDeadAfter = 100                    // Prevent activeView eviction during long divergence advance
	cfg.MaxBatchedProposals = 100              // Allow all 32 nodes × 2 proposals (Remove+Add) per ProposalJoin
	cfg.MaxPastEpochsOverride = 200            // Allow ProposalJoin healing from deep divergence (D up to 100)
	cfg.AnnounceInterval = 0                   // Disable auto-announce; drive manually to control healing triggers
	return cfg
}

// setupRealMLSCluster tạo cluster dùng real clock và real MLS Engine.
// Mỗi node có signing key riêng. Trả về nodes và network.
func setupRealMLSCluster(b *testing.B, n int, groupID string, engine MLSEngine) ([]*benchNode, *FakeNetwork) {
	b.Helper()
	network := NewFakeNetwork()
	clk := realClockImpl{}

	// Build map of identity -> peer.ID to resolve authorized committers from MLS state
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

		// Signing key = 32 bytes; dùng index i để tạo key khác nhau cho mỗi node
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
			b.Fatalf("NewCoordinator[%d]: %v", i, err)
		}

		// Setup authorized committers provider mapping to MLS members.
		// If a node is removed from the MLS group, it will not be elected as Token Holder.
		coord.authorizedCommitters = func(groupID string, epoch uint64, batch []BufferedProposal) ([]peer.ID, error) {
			// Access c.groupState directly to avoid re-entrant deadlock (since c.mu is already held)
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

		// groupInfoFetch: trả về GroupInfo trực tiếp từ remote node (in-process)
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
	return nodes, network
}

// createGroupAndAddAllMembers: node 0 tạo group, sau đó add từng node còn lại vào qua real MLS.
// Dùng AddMembers batch (direct Rust call) để init nhanh, không đi qua proposal pipeline.
func createGroupAndAddAllMembers(b *testing.B, nodes []*benchNode, engine MLSEngine) {
	b.Helper()
	ctx := context.Background()

	// Node 0 tạo group
	if err := nodes[0].coord.CreateGroup(); err != nil {
		b.Fatalf("node 0 create group: %v", err)
	}

	if len(nodes) == 1 {
		return
	}

	// Tạo KeyPackage cho các node 1..n-1
	kps := make([][]byte, len(nodes)-1)
	kpPrivs := make([][]byte, len(nodes)-1)
	for i := 1; i < len(nodes); i++ {
		kp, kpPriv, err := engine.GenerateKeyPackage(ctx, nodes[i].sigKey)
		if err != nil {
			b.Fatalf("GenerateKeyPackage[%d]: %v", i, err)
		}
		kps[i-1] = kp
		kpPrivs[i-1] = kpPriv
	}

	// Node 0 add tất cả bằng một lần AddMembers (batch Rust call)
	gs0 := nodes[0].coord.GetGroupState()
	commitBytes, welcomeBytes, newGS, newTH, err := engine.AddMembers(ctx, gs0, kps)
	if err != nil {
		b.Fatalf("AddMembers: %v", err)
	}
	_ = commitBytes

	// Cập nhật state node 0
	nodes[0].coord.SetStateForTest(nodes[0].coord.CurrentEpoch()+1, newGS, newTH)

	// Mỗi node 1..n-1 xử lý Welcome để có group state
	for i := 1; i < len(nodes); i++ {
		memberGS, memberTH, epoch, err := engine.ProcessWelcome(
			ctx, welcomeBytes, nodes[i].sigKey, kpPrivs[i-1], 0,
		)
		if err != nil {
			b.Fatalf("ProcessWelcome[%d]: %v", i, err)
		}
		nodes[i].coord.SetStateForTest(epoch, memberGS, memberTH)
	}
}

func startAllForBench(b *testing.B, nodes []*benchNode) {
	b.Helper()
	for _, n := range nodes {
		if err := n.coord.Start(context.Background()); err != nil {
			b.Fatalf("start node %s: %v", n.id, err)
		}
	}
}

func exchangeHeartbeatsForBench(nodes []*benchNode, network *FakeNetwork) {
	for _, n := range nodes {
		n.coord.BroadcastHeartbeat()
	}
	network.DrainAll()
}

// waitForBench polls condition với real time.Sleep.
func waitForBench(b *testing.B, timeout time.Duration, cond func() (*ForkHealingJob, bool)) {
	b.Helper()
	deadline := time.Now().Add(timeout)
	var lastJob *ForkHealingJob
	for time.Now().Before(deadline) {
		job, ok := cond()
		if job != nil {
			lastJob = job
		}
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	b.Fatalf("timeout waiting for bench condition; last job: %+v", lastJob)
}

// waitForBenchWithNetwork polls condition while continuously draining the network
func waitForBenchWithNetwork(b *testing.B, timeout time.Duration, network *FakeNetwork, cond func() (*ForkHealingJob, bool)) {
	b.Helper()
	deadline := time.Now().Add(timeout)
	var lastJob *ForkHealingJob
	for time.Now().Before(deadline) {
		job, ok := cond()
		if job != nil {
			lastJob = job
		}
		if ok {
			return
		}
		network.DrainAll()
		time.Sleep(10 * time.Millisecond)
	}
	b.Fatalf("timeout waiting for bench condition; last job: %+v", lastJob)
}

// advanceEpochOnWinningBranch: node 0 remove target node để tiến epoch
func advanceEpochOnWinningBranch(b *testing.B, nodes []*benchNode, network *FakeNetwork, targetNode *benchNode) {
	b.Helper()

	targetIdentity := deriveIdentityFromSigningKey(targetNode.sigKey)

	// Node 0 (Token Holder) tự remove target node để tiến epoch
	err := nodes[0].coord.RemoveMemberWithPeer(RemoveMemberRequest{
		TargetPeerID:   targetNode.id,
		TargetIdentity: targetIdentity,
	})
	if err != nil {
		b.Fatalf("RemoveMemberWithPeer failed: %v", err)
	}

	// Drain để các node khác trong winning branch tự apply commit
	network.DrainAll()
}

// ─────────────────────────────────────────────────────────────────────────────
// BenchmarkForkHeal_EndToEndLatency
// Đo latency end-to-end của fork-healing: từ khi heal triggered đến khi
// nodeX hoàn thành ExternalJoin và đạt STATE_SWAPPED / CLEANED.
// ─────────────────────────────────────────────────────────────────────────────
func BenchmarkForkHeal_EndToEndLatency(b *testing.B) {
	sizes := []int{16, 32, 64}

	pm := newTestProcessManager()
	port, err := pm.StartEngine()
	if err != nil {
		b.Skipf("Skipping benchmark: Failed to start real MLS engine: %v", err)
	}
	defer pm.StopEngine()

	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024), grpc.MaxCallSendMsgSize(64*1024*1024))) //nolint:staticcheck
	if err != nil {
		b.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	realEngine := newTestCachedGrpcMLSEngine(mls_service.NewMLSCryptoServiceClient(conn))

	for _, n := range sizes {
		n := n
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				nodes, network := setupRealMLSCluster(b, n, fmt.Sprintf("bench-latency-%d", i), realEngine)
				createGroupAndAddAllMembers(b, nodes, realEngine)
				startAllForBench(b, nodes)
				exchangeHeartbeatsForBench(nodes, network)

				// Set onAddCommitted trên TẤT CẢ nodes để node nào là Token Holder
				// cũng forward Welcome cho Loser (nodeX) khi commit thành công.
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

				// nodeX là node cuối bị partition
				nodeX := nodes[n-1]

				// --- Partition ---
				var groupA []peer.ID
				for j := 0; j < n-1; j++ {
					groupA = append(groupA, nodes[j].id)
				}
				groupB := []peer.ID{nodeX.id}
				network.Partition(groupA, groupB)

				// --- Winning branch tạo epoch mới bằng cách remove nodeX ---
				advanceEpochOnWinningBranch(b, nodes, network, nodeX)

				// --- Heal ---
				network.Heal()

				b.StartTimer()

				// Trigger fork detection: node 0 broadcast announce
				nodes[0].coord.BroadcastAnnounce()
				network.DrainAll()

				// Chờ nodeX hoàn thành heal (CLEANED)
				groupIDStr := fmt.Sprintf("bench-latency-%d", i)
				waitForBenchWithNetwork(b, 30*time.Second, network, func() (*ForkHealingJob, bool) {
					job, _ := nodeX.storage.GetActiveForkHealingJob(groupIDStr)
					// Dưới Phoenix Protocol, khi hoàn thành sáp nhập, status chuyển thành CLEANED
					// và GetActiveForkHealingJob trả về nil. Epoch của nodeX sẽ khớp Winner.
					if nodeX.coord.CurrentEpoch() == nodes[0].coord.CurrentEpoch() && bytes.Equal(nodeX.coord.GetTreeHash(), nodes[0].coord.GetTreeHash()) {
						return job, true
					}
					return job, false
				})

				b.StopTimer()

				for _, node := range nodes {
					node.coord.Stop()
				}
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BenchmarkForkHeal_ThunderingHerd
// K nodes bị partition đồng thời. Đo thời gian để tất cả K nodes converge.
// ─────────────────────────────────────────────────────────────────────────────
func BenchmarkForkHeal_ThunderingHerd(b *testing.B) {
	kSizes := []int{4, 8, 16, 32, 64}
	n := 128 // Tổng số node để có đủ winning nodes khi K=64

	pm := newTestProcessManager()
	port, err := pm.StartEngine()
	if err != nil {
		b.Skipf("Skipping benchmark: Failed to start real MLS engine: %v", err)
	}
	defer pm.StopEngine()

	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024), grpc.MaxCallSendMsgSize(64*1024*1024))) //nolint:staticcheck
	if err != nil {
		b.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	realEngine := newTestCachedGrpcMLSEngine(mls_service.NewMLSCryptoServiceClient(conn))

	for _, k := range kSizes {
		k := k
		b.Run(fmt.Sprintf("K=%d", k), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				nodes, network := setupRealMLSCluster(b, n, fmt.Sprintf("bench-herd-%d", i), realEngine)
				createGroupAndAddAllMembers(b, nodes, realEngine)
				startAllForBench(b, nodes)
				exchangeHeartbeatsForBench(nodes, network)

				// Set onAddCommitted trên TẤT CẢ nodes để Token Holder forward Welcome
				for _, bn := range nodes {
					bn := bn
					bn.coord.onAddCommitted = func(delivery AddCommitDelivery, _ uint64, welcome []byte) {
						for _, candidate := range nodes {
							if candidate.id.String() == delivery.TargetPeerID {
								go candidate.coord.ProcessWelcomeIfWaiting(context.Background(), welcome)
								break
							}
						}
					}
				}

				// Partition: winning = nodes[0..n-k-1], losing = nodes[n-k..n-1]
				var groupA, groupB []peer.ID
				for j := 0; j < n-k; j++ {
					groupA = append(groupA, nodes[j].id)
				}
				for j := n - k; j < n; j++ {
					groupB = append(groupB, nodes[j].id)
				}
				network.Partition(groupA, groupB)

				// Advance winning branch by removing the last node (which is one of the losing nodes)
				advanceEpochOnWinningBranch(b, nodes, network, nodes[n-1])

				// Heal
				network.Heal()

				b.StartTimer()

				// Trigger fork detection
				nodes[0].coord.BroadcastAnnounce()
				network.DrainAll()

				winnerEpoch := nodes[0].coord.CurrentEpoch()
				winnerTreeHash := nodes[0].coord.GetTreeHash()
				waitForBenchWithNetwork(b, 60*time.Second, network, func() (*ForkHealingJob, bool) {
					converged := 0
					for j := n - k; j < n; j++ {
						if nodes[j].coord.CurrentEpoch() >= winnerEpoch && bytes.Equal(nodes[j].coord.GetTreeHash(), winnerTreeHash) {
							converged++
						}
					}
					return nil, converged == k
				})

				b.StopTimer()

				for _, node := range nodes {
					node.coord.Stop()
				}
			}
		})
	}
}

// advanceMultipleEpochsOnWinningBranch advances the winning branch by D real MLS epochs
// using CreateProposal(Update) + CreateCommit per epoch. This creates legitimate
// divergence: group state evolves through D real epoch transitions, producing
// a group state that grows with D (realistic e2e scenario).
func advanceMultipleEpochsOnWinningBranch(b *testing.B, nodes []*benchNode, network *FakeNetwork, depth int, winningCount int) {
	b.Helper()
	advanceBranchEpochs(b, nodes, 0, winningCount, depth)
}

// advanceEpochsOnLosingBranch advances the losing branch by depth epochs using
// direct MLS engine calls. losingStart is the index of the first losing-branch node.
func advanceEpochsOnLosingBranch(b *testing.B, nodes []*benchNode, losingStart int, depth int) {
	b.Helper()
	advanceBranchEpochs(b, nodes, losingStart, len(nodes)-losingStart, depth)
}

// advanceBranchEpochs advances a branch starting at branchStart with branchCount nodes
// by depth epochs using CreateProposal(Update) + CreateCommit.
func advanceBranchEpochs(b *testing.B, nodes []*benchNode, branchStart, branchCount, depth int) {
	b.Helper()
	ctx := context.Background()
	engine := nodes[branchStart].mls
	startEpoch := nodes[branchStart].coord.CurrentEpoch()

	for i := 0; i < depth; i++ {
		gs := nodes[branchStart].coord.GetGroupState()

		propResult, err := engine.CreateProposal(ctx, gs, ProposalUpdate, nil)
		if err != nil {
			b.Fatalf("CreateProposal[Update][%d] failed: %v", i, err)
		}

		commitResult, err := engine.CreateCommit(ctx, propResult.NewGroupState, [][]byte{propResult.ProposalRef})
		if err != nil {
			b.Fatalf("CreateCommit[%d] failed: %v", i, err)
		}

		newEpoch := startEpoch + uint64(i+1)

		for j := branchStart; j < branchStart+branchCount; j++ {
			nodes[j].coord.SetStateForTest(newEpoch, commitResult.NewGroupState, commitResult.NewTreeHash)
		}
	}
}

// BenchmarkForkHeal_PartitionDivergence
// N=32 nodes, partitioned 30 vs 2. Both branches advance independently during
// partition, creating a true MLS fork (divergent TreeHash). Winning branch advances
// D epochs, losing branch advances 2 epochs. Measures healing time vs divergence depth.
// Expected: healing time ≈ O(1) regardless of D, because External Proposal + Token Holder
// Commit + ProcessWelcome does not replay intermediate epochs.
func BenchmarkForkHeal_PartitionDivergence(b *testing.B) {
	n := 32
	winningCount := 30
	losingAdvance := 2
	depths := []int{5, 10, 20, 50}

	pm := newTestProcessManager()
	port, err := pm.StartEngine()
	if err != nil {
		b.Skipf("Skipping benchmark: Failed to start real MLS engine: %v", err)
	}
	defer pm.StopEngine()

	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024), grpc.MaxCallSendMsgSize(64*1024*1024))) //nolint:staticcheck
	if err != nil {
		b.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	realEngine := newTestCachedGrpcMLSEngine(mls_service.NewMLSCryptoServiceClient(conn))

	csvFile, err := os.Create(filepath.Join("..", "..", "evaluation", "data", "partition_divergence_metrics.csv"))
	if err != nil {
		b.Fatalf("Failed to create CSV: %v", err)
	}
	defer csvFile.Close()
	fmt.Fprintln(csvFile, "DivergenceDepth,HealingTimeMs,GroupStateBytes")

	for _, d := range depths {
		d := d
		b.Run(fmt.Sprintf("D=%d", d), func(b *testing.B) {
			const rounds = 10
			var times []int64
			var winnerGSBytes int

			for r := 0; r < rounds; r++ {
				b.StopTimer()

				groupIDStr := fmt.Sprintf("bench-divergence-%d-%d", d, r)
				nodes, network := setupRealMLSCluster(b, n, groupIDStr, realEngine)
				createGroupAndAddAllMembers(b, nodes, realEngine)
				startAllForBench(b, nodes)
				exchangeHeartbeatsForBench(nodes, network)

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

				// Partition: Group A (winning, 30 nodes) vs Group B (losing, 2 nodes)
				var groupA, groupB []peer.ID
				for j := 0; j < winningCount; j++ {
					groupA = append(groupA, nodes[j].id)
				}
				for j := winningCount; j < n; j++ {
					groupB = append(groupB, nodes[j].id)
				}
				network.Partition(groupA, groupB)

				// Both branches advance independently during partition, creating true fork
				advanceMultipleEpochsOnWinningBranch(b, nodes, network, d, winningCount)
				advanceEpochsOnLosingBranch(b, nodes, winningCount, losingAdvance)

				winnerGSBytes = len(nodes[0].coord.GetGroupState())

				network.Heal()

				b.StartTimer()

				// Re-exchange heartbeats so forkDetector.local.MemberCount reflects
				// the full activeView (32 members) before announce triggers healing.
				exchangeHeartbeatsForBench(nodes, network)

				// Reset fork detector for ALL nodes so stale known branches from
				// before partition don't cause false fork detection or prevent healing.
				for _, bn := range nodes {
					bn.coord.ResetForkDetectorForTest()
				}

				// Track healing completion of the FIRST losing node via channel.
				// The callback captures the internal duration_ms reported by runHeal,
				// which measures from heal start to completion without DrainAll noise.
				healedChan := make(chan int64, 1)
				for j := winningCount; j < n; j++ {
					bn := nodes[j]
					bn.coord.onForkHealEvent = func(summary ForkHealAuditSummary) {
						if summary.Stage == "fork_heal_completed" {
							select {
							case healedChan <- summary.DurationMs:
							default:
							}
						}
					}
				}

				nodes[0].coord.BroadcastAnnounce()

				// Drain the announce cascade so fork detection triggers promptly.
				network.DrainAll()

				// Background drainer: continuously delivers messages so healing
				// pipeline (ProposalJoin → Commit → Welcome) progresses.
				drainDone := make(chan struct{})
				go func() {
					for {
						select {
						case <-drainDone:
							return
						default:
							network.DrainAll()
							time.Sleep(1 * time.Millisecond)
						}
					}
				}()

				var elapsedMs int64
				select {
				case elapsedMs = <-healedChan:
				case <-time.After(300 * time.Second):
					close(drainDone)
					b.Fatalf("timeout waiting for first heal; depth=%d round=%d", d, r)
				}
				close(drainDone)

				slog.Info("bench/converged", "depth", d, "round", r, "healing_ms", elapsedMs)
				times = append(times, elapsedMs)

				b.StopTimer()

				for _, node := range nodes {
					node.coord.Stop()
				}
			}

			// Compute median
			sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
			median := times[len(times)/2]
			slog.Info("bench/median", "depth", d, "median_ms", median, "all_ms", times)

			fmt.Fprintf(csvFile, "%d,%.2f,%d\n", d, float64(median), winnerGSBytes)
			csvFile.Sync()

			b.ReportMetric(float64(median), "healing_ms")
		})
	}
}
