package coordination

import (
	"bytes"
	"context"
	"fmt"
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

func (realClockImpl) Now() time.Time             { return time.Now() }
func (realClockImpl) After(d time.Duration) <-chan time.Time { return time.After(d) }

// benchConfig trả về config cho benchmark: BatchingDelay nhỏ để commit nhanh.
func benchConfig() *CoordinatorConfig {
	cfg := DefaultConfig()
	cfg.BatchingDelay = 100 * time.Millisecond // Tương tự production delay
	cfg.HeartbeatInterval = 5 * time.Second
	cfg.MLSOperationTimeout = 10 * time.Second // real Rust gRPC cần timeout lớn hơn
	cfg.TokenHolderTimeout = 5 * time.Second
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
			SigningKey:  sigKey,
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
		TargetPeerID: targetNode.id,
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

	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithInsecure()) //nolint:staticcheck
	if err != nil {
		b.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	realEngine := newTestGrpcMLSEngine(mls_service.NewMLSCryptoServiceClient(conn))

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

	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithInsecure()) //nolint:staticcheck
	if err != nil {
		b.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	realEngine := newTestGrpcMLSEngine(mls_service.NewMLSCryptoServiceClient(conn))

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

