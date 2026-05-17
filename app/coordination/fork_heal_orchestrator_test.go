package coordination

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ─── Sprint 2B: independent timers for heartbeat and announce ────────────────

func setupClusterWithCfg(t *testing.T, n int, groupID string, cfg *CoordinatorConfig) ([]*testNode, *FakeNetwork, *FakeClock) {
	t.Helper()
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	names := []string{"alice", "bob", "carol", "dave", "eve"}

	nodes := make([]*testNode, n)
	for i := 0; i < n; i++ {
		id := peerID(names[i%len(names)])
		transport := network.AddNode(id)
		mls := NewMockMLSEngine()
		storage := NewMockStorage()

		coord, err := NewCoordinator(CoordinatorOpts{
			Config:    cfg,
			Transport: transport,
			Clock:     clk,
			MLS:       mls,
			Storage:   storage,
			LocalID:   id,
			GroupID:   groupID,
		})
		if err != nil {
			t.Fatalf("NewCoordinator[%d]: %v", i, err)
		}
		nodes[i] = &testNode{id: id, coord: coord, mls: mls, storage: storage}
	}
	return nodes, network, clk
}

// waitFor polls fn until it returns true or timeout elapses. Used to bridge
// FakeClock-driven goroutines that signal completion asynchronously.
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return fn()
}

// waitForWaiters blocks until the FakeClock has at least n After-waiters
// registered, or timeout elapses. Used before clk.Advance to guarantee that
// background loops have already registered their timers — without this the
// scheduler may not have run the goroutine body yet and Advance would no-op.
func waitForWaiters(t *testing.T, clk *FakeClock, n int, timeout time.Duration) {
	t.Helper()
	if !waitFor(t, timeout, func() bool { return clk.WaitersCount() >= n }) {
		t.Fatalf("FakeClock never reached %d waiters (have %d)", n, clk.WaitersCount())
	}
}

func TestCoordinator_AnnounceLoop_FiresOnInterval(t *testing.T) {
	cfg := TestConfig()
	cfg.HeartbeatInterval = 10 * time.Second
	cfg.AnnounceInterval = 200 * time.Millisecond

	nodes, network, clk := setupClusterWithCfg(t, 2, "grp-ann", cfg)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	// 2 nodes × 2 loops (heartbeat + announce) = 4 waiters before Advance.
	waitForWaiters(t, clk, 4, time.Second)
	clk.Advance(cfg.AnnounceInterval)

	if !waitFor(t, time.Second, func() bool {
		return network.PendingByType(nodes[0].id, MsgAnnounce) >= 1
	}) {
		t.Fatalf("expected at least 1 announce from alice within timeout, got %d",
			network.PendingByType(nodes[0].id, MsgAnnounce))
	}
}

func TestCoordinator_AnnounceLoop_DisabledWhenZero(t *testing.T) {
	cfg := TestConfig()
	cfg.HeartbeatInterval = 50 * time.Millisecond
	cfg.AnnounceInterval = 0

	nodes, network, clk := setupClusterWithCfg(t, 2, "grp-ann-disabled", cfg)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	// Only heartbeat goroutines run when AnnounceInterval=0: 2 nodes × 1 loop.
	waitForWaiters(t, clk, 2, time.Second)
	for i := 0; i < 3; i++ {
		clk.Advance(cfg.HeartbeatInterval)
		if !waitFor(t, time.Second, func() bool {
			return network.PendingByType(nodes[0].id, MsgHeartbeat) >= i+1
		}) {
			t.Fatalf("heartbeat tick %d did not propagate (saw %d)",
				i+1, network.PendingByType(nodes[0].id, MsgHeartbeat))
		}
		waitForWaiters(t, clk, 2, time.Second)
	}

	if got := network.PendingByType(nodes[0].id, MsgAnnounce); got != 0 {
		t.Fatalf("AnnounceInterval=0 should suppress announces, got %d", got)
	}
}

func TestCoordinator_HeartbeatAndAnnounceLoops_Independent(t *testing.T) {
	cfg := TestConfig()
	cfg.HeartbeatInterval = 50 * time.Millisecond
	cfg.AnnounceInterval = 200 * time.Millisecond

	nodes, network, clk := setupClusterWithCfg(t, 2, "grp-indep", cfg)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	// 2 nodes × 2 loops each = 4 waiters before first Advance.
	waitForWaiters(t, clk, 4, time.Second)

	for i := 0; i < 4; i++ {
		clk.Advance(cfg.HeartbeatInterval)
		if !waitFor(t, time.Second, func() bool {
			return network.PendingByType(nodes[0].id, MsgHeartbeat) >= i+1
		}) {
			t.Fatalf("heartbeat tick %d did not propagate (saw %d)",
				i+1, network.PendingByType(nodes[0].id, MsgHeartbeat))
		}
		// Each heartbeat fire re-arms the next After(); wait for the loop to
		// re-register before the next Advance so we never race past it.
		waitForWaiters(t, clk, 4, time.Second)
	}

	if !waitFor(t, time.Second, func() bool {
		return network.PendingByType(nodes[0].id, MsgAnnounce) >= 1
	}) {
		t.Fatalf("announce should fire at 200ms, got %d",
			network.PendingByType(nodes[0].id, MsgAnnounce))
	}

	hbCount := network.PendingByType(nodes[0].id, MsgHeartbeat)
	if hbCount < 4 {
		t.Errorf("expected >= 4 heartbeats after 200ms (4 ticks), got %d", hbCount)
	}
}

// ─── Sprint 2B: scheduleHeal scaffold ────────────────────────────────────────

// fakeForkPeer is a minimal peer.ID stand-in used for fork-event payloads in
// scheduleHeal unit tests. We pick a constant value so log assertions remain
// stable across test runs.
var fakeForkPeer = peerID("winner-peer")

func attachStaticGroupInfoFetcher(c *Coordinator, epoch uint64, treeHash []byte, payload []byte) {
	c.groupInfoFetch = func(_ context.Context, _ peer.ID, _ string, _ bool) (*GroupInfoFetchResult, error) {
		return &GroupInfoFetchResult{
			GroupInfo: append([]byte(nil), payload...),
			Epoch:     epoch,
			TreeHash:  append([]byte(nil), treeHash...),
		}, nil
	}
}

func newRemoteForkEvent(groupID string, partitionStartedAt time.Time) *ForkEvent {
	return &ForkEvent{
		GroupID:    groupID,
		RemotePeer: fakeForkPeer,
		LocalAnnounce: GroupStateAnnouncement{
			TreeHash:    []byte("loser-hash"),
			MemberCount: 2,
			Epoch:       0,
			CommitHash:  []byte{0x02},
		},
		RemoteAnnounce: GroupStateAnnouncement{
			TreeHash:    []byte("winner-hash"),
			MemberCount: 5,
			Epoch:       7,
			CommitHash:  []byte{0x01},
		},
		RemoteEpoch:        7,
		Result:             BranchRemote,
		NeedExternalJoin:   true,
		PartitionStartedAt: partitionStartedAt,
	}
}

func TestCoordinator_ScheduleHeal_RecordsMetrics(t *testing.T) {
	nodes, _, clk := setupCluster(t, 1, "grp-heal-metrics")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	c := nodes[0].coord
	mockState, _ := json.Marshal(mockGroupState{GroupID: "grp-heal-metrics", Epoch: 7, TreeHash: "winner-hash"})
	attachStaticGroupInfoFetcher(c, 7, []byte("winner-hash"), mockState)
	event := newRemoteForkEvent("grp-heal-metrics", clk.Now().Add(-2*time.Second))

	c.mu.Lock()
	c.scheduleHeal(event)
	c.mu.Unlock()

	if !waitFor(t, time.Second, func() bool {
		snap := c.GetMetrics()
		return snap.ForkHealingsAttempted == 1 && snap.ForkHealingsSucceeded == 1
	}) {
		snap := c.GetMetrics()
		t.Fatalf("scaffold heal should record attempt+success metrics: attempted=%d succeeded=%d",
			snap.ForkHealingsAttempted, snap.ForkHealingsSucceeded)
	}
	if c.IsHealing() {
		t.Fatal("healing flag should be cleared after goroutine exits")
	}
}

func TestCoordinator_ScheduleHeal_ConcurrencyGuard(t *testing.T) {
	nodes, _, clk := setupCluster(t, 1, "grp-heal-guard")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	c := nodes[0].coord

	if !c.healing.CompareAndSwap(false, true) {
		t.Fatal("test setup: healing flag should start false")
	}
	t.Cleanup(func() { c.healing.Store(false) })

	event := newRemoteForkEvent("grp-heal-guard", clk.Now())
	c.mu.Lock()
	c.scheduleHeal(event)
	c.mu.Unlock()

	time.Sleep(20 * time.Millisecond)

	snap := c.GetMetrics()
	if snap.ForkHealingsAttempted != 0 {
		t.Errorf("scheduleHeal must not record an attempt when CAS fails, got %d",
			snap.ForkHealingsAttempted)
	}
	if snap.ForkHealingsSucceeded != 0 {
		t.Errorf("scheduleHeal must not record success when CAS fails, got %d",
			snap.ForkHealingsSucceeded)
	}
}

func TestCoordinator_HandleAnnounce_TriggersHealOnLosingBranch(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-heal-trigger")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Force alice into a "diverged" state with a TreeHash that — combined with
	// a remote announce of higher MemberCount — guarantees alice is the loser.
	nodes[0].coord.mu.Lock()
	nodes[0].coord.treeHash = []byte("loser-tree-hash")
	nodes[0].coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("loser-tree-hash"),
		MemberCount: 1,
		Epoch:       0,
	})
	nodes[0].coord.mu.Unlock()
	nodes[0].coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, _ string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
		if remote != nodes[1].id {
			return nil, context.DeadlineExceeded
		}
		groupInfo, err := nodes[1].mls.ExportGroupInfo(ctx, nodes[1].coord.GetGroupState(), withRatchetTree)
		if err != nil {
			return nil, err
		}
		return &GroupInfoFetchResult{
			GroupInfo: groupInfo,
			Epoch:     nodes[1].coord.CurrentEpoch(),
			TreeHash:  nodes[1].coord.GetTreeHash(),
		}, nil
	}

	// Bob announces a winning branch (more members).
	bob := nodes[1].coord
	bob.mu.Lock()
	bob.treeHash = []byte("winner-tree-hash")
	bob.broadcastAnnounceLocked()
	bob.mu.Unlock()
	network.DrainAll()

	if !waitFor(t, time.Second, func() bool {
		snap := nodes[0].coord.GetMetrics()
		return snap.PartitionsDetected >= 1 && snap.ForkHealingsAttempted >= 1 && snap.ForkHealingsSucceeded >= 1
	}) {
		snap := nodes[0].coord.GetMetrics()
		t.Fatalf("alice should detect partition and complete heal: detected=%d attempted=%d succeeded=%d",
			snap.PartitionsDetected, snap.ForkHealingsAttempted, snap.ForkHealingsSucceeded)
	}
	if !waitFor(t, time.Second, func() bool { return !nodes[0].coord.IsHealing() }) {
		t.Fatal("healing flag should clear after scaffold goroutine completes")
	}
}

func TestCoordinator_Heal_ReplaysOwnPartitionWindowMessages(t *testing.T) {
	nodes, network, clk := setupCluster(t, 2, "grp-heal-replay")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Force divergent branches.
	nodes[0].coord.mu.Lock()
	nodes[0].coord.treeHash = []byte("loser-tree-hash")
	nodes[0].coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("loser-tree-hash"),
		MemberCount: 1,
		Epoch:       0,
	})
	nodes[0].coord.mu.Unlock()

	nodes[1].coord.mu.Lock()
	nodes[1].coord.treeHash = []byte("winner-tree-hash")
	nodes[1].coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree-hash"),
		MemberCount: 2,
		Epoch:       0,
	})
	nodes[1].coord.mu.Unlock()

	// Simulate that loser observed winner branch earlier; this timestamp becomes
	// sticky PartitionStartedAt and drives replay window selection.
	partitionStart := clk.Now().Add(1 * time.Second)
	clk.Set(partitionStart)
	nodes[0].coord.mu.Lock()
	nodes[0].coord.forkDetector.ProcessRemote(partitionStart, nodes[1].id, 0, GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree-hash"),
		MemberCount: 2,
		Epoch:       0,
	})
	nodes[0].coord.mu.Unlock()

	// Produce local messages while partitioned (these should be replayed).
	network.Partition([]peer.ID{nodes[0].id}, []peer.ID{nodes[1].id})
	if _, err := nodes[0].coord.SendMessage([]byte("partition-msg-1")); err != nil {
		t.Fatalf("SendMessage #1: %v", err)
	}
	if _, err := nodes[0].coord.SendMessage([]byte("partition-msg-2")); err != nil {
		t.Fatalf("SendMessage #2: %v", err)
	}
	network.Heal()

	// Hook GroupInfo fetcher from winner state.
	nodes[0].coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, _ string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
		if remote != nodes[1].id {
			return nil, context.DeadlineExceeded
		}
		groupInfo, err := nodes[1].mls.ExportGroupInfo(ctx, nodes[1].coord.GetGroupState(), withRatchetTree)
		if err != nil {
			return nil, err
		}
		return &GroupInfoFetchResult{
			GroupInfo: groupInfo,
			Epoch:     nodes[1].coord.CurrentEpoch(),
			TreeHash:  nodes[1].coord.GetTreeHash(),
		}, nil
	}

	// Trigger heal from winner announce.
	nodes[1].coord.mu.Lock()
	nodes[1].coord.broadcastAnnounceLocked()
	nodes[1].coord.mu.Unlock()
	network.DrainAll()

	if !waitFor(t, time.Second, func() bool {
		snap := nodes[0].coord.GetMetrics()
		return snap.ForkHealingsSucceeded >= 1
	}) {
		t.Fatalf("expected successful heal; metrics=%+v", nodes[0].coord.GetMetrics())
	}
	if got := network.PendingByType(nodes[0].id, MsgApplication); got < 2 {
		t.Fatalf("expected replayed application envelopes >=2, got %d", got)
	}
}
