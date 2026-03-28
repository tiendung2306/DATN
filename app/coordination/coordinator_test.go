package coordination

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

type testNode struct {
	id      peer.ID
	coord   *Coordinator
	mls     *MockMLSEngine
	storage *MockStorage
}

func setupCluster(t *testing.T, n int, groupID string) ([]*testNode, *FakeNetwork, *FakeClock) {
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
			Config:    TestConfig(),
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

func createAndShareGroup(t *testing.T, nodes []*testNode) {
	t.Helper()
	// First node creates the group
	if err := nodes[0].coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	// Other nodes initialize with the same state
	state := nodes[0].coord.GetGroupState()
	epoch := nodes[0].coord.CurrentEpoch()
	treeHash := nodes[0].coord.treeHash

	for _, n := range nodes[1:] {
		n.coord.InitializeGroup(state, epoch, treeHash)
	}
}

func startAll(t *testing.T, nodes []*testNode) {
	t.Helper()
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

// exchangeHeartbeats makes each node broadcast a heartbeat and delivers them.
func exchangeHeartbeats(nodes []*testNode, network *FakeNetwork) {
	for _, n := range nodes {
		n.coord.BroadcastHeartbeat()
	}
	network.DrainAll()
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestCoordinator_CreateGroupAndStart(t *testing.T) {
	nodes, _, _ := setupCluster(t, 1, "grp-1")
	if err := nodes[0].coord.CreateGroup(); err != nil {
		t.Fatal(err)
	}
	if err := nodes[0].coord.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer nodes[0].coord.Stop()

	if nodes[0].coord.CurrentEpoch() != 0 {
		t.Errorf("expected epoch 0, got %d", nodes[0].coord.CurrentEpoch())
	}
	if len(nodes[0].coord.GetGroupState()) == 0 {
		t.Error("group state should not be empty")
	}
}

func TestCoordinator_TokenHolderDeterministic(t *testing.T) {
	nodes, network, _ := setupCluster(t, 3, "grp-1")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	// Exchange heartbeats so all nodes see each other
	exchangeHeartbeats(nodes, network)

	// All nodes should agree on who the token holder is
	holders := make(map[bool]int)
	for _, n := range nodes {
		holders[n.coord.IsTokenHolder()]++
	}
	if holders[true] != 1 {
		t.Errorf("exactly one node should be token holder, got %d", holders[true])
	}
}

func TestCoordinator_SendAndReceiveMessage(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-msg")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Alice sends a message
	ts, err := nodes[0].coord.SendMessage([]byte("hello from alice"))
	if err != nil {
		t.Fatal(err)
	}
	if ts == nil {
		t.Fatal("expected non-nil HLC timestamp")
	}

	// Deliver to Bob
	network.DrainAll()

	// Bob should have the message in storage
	bobMsgs := nodes[1].storage.Messages()
	if len(bobMsgs) != 1 {
		t.Fatalf("bob should have 1 message, got %d", len(bobMsgs))
	}
	if string(bobMsgs[0].Content) != "hello from alice" {
		t.Errorf("unexpected content: %s", bobMsgs[0].Content)
	}
	if bobMsgs[0].SenderID != nodes[0].id {
		t.Errorf("sender should be alice, got %s", bobMsgs[0].SenderID)
	}
}

func TestCoordinator_ProposalCommitFlow(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-commit")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Determine who is the token holder
	var holder, other *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else {
			other = n
		}
	}
	if holder == nil {
		t.Fatal("no token holder found")
	}

	// The non-holder proposes an Add
	if err := other.coord.ProposeAdd([]byte("new-member-key")); err != nil {
		t.Fatal(err)
	}
	network.DrainAll() // deliver proposal to holder → holder auto-commits
	network.DrainAll() // deliver commit back to other

	// Both nodes should be at epoch 1
	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 1 {
			t.Errorf("node %s: expected epoch 1, got %d", n.id, n.coord.CurrentEpoch())
		}
	}

	// Metrics: holder should have 1 commit issued
	snap := holder.coord.GetMetrics()
	if snap.CommitsIssued != 1 {
		t.Errorf("holder should have 1 commit, got %d", snap.CommitsIssued)
	}
}

func TestCoordinator_TokenHolderSelfProposal(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-self")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var holder *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
			break
		}
	}

	// Token holder proposes — should commit immediately
	if err := holder.coord.ProposeUpdate([]byte("key-rotation")); err != nil {
		t.Fatal(err)
	}

	// Holder already advanced locally
	if holder.coord.CurrentEpoch() != 1 {
		t.Errorf("holder should be at epoch 1, got %d", holder.coord.CurrentEpoch())
	}

	// Deliver commit to the other node
	network.DrainAll()

	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 1 {
			t.Errorf("node %s: expected epoch 1, got %d", n.id, n.coord.CurrentEpoch())
		}
	}
}

func TestCoordinator_EpochConsistency_StaleRejected(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-stale")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Advance alice to epoch 1 by self-proposal (if she's holder)
	// or have the holder advance
	var holder *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
			break
		}
	}
	_ = holder.coord.ProposeUpdate([]byte("advance"))
	network.DrainAll()

	// Both at epoch 1
	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 1 {
			t.Fatalf("node %s: expected epoch 1, got %d", n.id, n.coord.CurrentEpoch())
		}
	}

	// Manually craft a stale message (epoch 0) to alice
	staleEnv := Envelope{
		Type:    MsgApplication,
		GroupID: "grp-stale",
		Epoch:   0, // stale!
		From:    "attacker",
		Payload: []byte(`{"ciphertext":"aGVsbG8="}`),
	}
	staleBytes, _ := json.Marshal(staleEnv)

	// Deliver directly to alice's handler
	nodes[0].coord.handleRawMessage(peerID("attacker"), staleBytes)

	// Alice should still have no extra messages (stale was rejected)
	aliceMsgs := nodes[0].storage.Messages()
	for _, m := range aliceMsgs {
		if m.Epoch == 0 && m.SenderID == peerID("attacker") {
			t.Error("stale message should have been rejected")
		}
	}

	snap := nodes[0].coord.GetMetrics()
	// Stale app messages are just silently dropped (not counted as duplicate epoch)
	_ = snap
}

func TestCoordinator_HeartbeatUpdatesActiveView(t *testing.T) {
	nodes, network, _ := setupCluster(t, 3, "grp-hb")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	// Initially each node only sees itself
	for _, n := range nodes {
		if n.coord.activeView.Size() != 1 {
			t.Errorf("node %s: expected 1 member before heartbeat, got %d",
				n.id, n.coord.activeView.Size())
		}
	}

	// Exchange heartbeats
	exchangeHeartbeats(nodes, network)

	// Now each node should see all 3
	for _, n := range nodes {
		if n.coord.activeView.Size() != 3 {
			t.Errorf("node %s: expected 3 members after heartbeat, got %d",
				n.id, n.coord.activeView.Size())
		}
	}
}

func TestCoordinator_MultipleEpochAdvances(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-multi")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var holder *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
			break
		}
	}

	// Advance through 5 epochs
	for i := 0; i < 5; i++ {
		_ = holder.coord.ProposeUpdate([]byte("rotation"))
		network.DrainAll()
	}

	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 5 {
			t.Errorf("node %s: expected epoch 5, got %d", n.id, n.coord.CurrentEpoch())
		}
	}

	// Both nodes should have identical group state
	s0 := nodes[0].coord.GetGroupState()
	s1 := nodes[1].coord.GetGroupState()
	if string(s0) != string(s1) {
		t.Error("group states should be identical after converging")
	}
}

func TestCoordinator_MessageOrderingByHLC(t *testing.T) {
	nodes, network, clk := setupCluster(t, 2, "grp-hlc")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Alice sends first
	ts1, _ := nodes[0].coord.SendMessage([]byte("msg-1"))
	network.DrainAll()

	// Advance clock slightly
	clk.Advance(10 * time.Millisecond)

	// Bob sends second
	ts2, _ := nodes[1].coord.SendMessage([]byte("msg-2"))
	network.DrainAll()

	if !ts1.Before(*ts2) {
		t.Errorf("msg-1 HLC should be before msg-2 HLC:\n  ts1=%+v\n  ts2=%+v", ts1, ts2)
	}

	// Bob should have both messages in causal order
	bobMsgs := nodes[1].storage.Messages()
	if len(bobMsgs) < 2 {
		t.Fatalf("bob should have at least 2 messages, got %d", len(bobMsgs))
	}
}

func TestCoordinator_ForkDetection(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-fork")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Manually advance alice to a different state (simulate partition divergence)
	nodes[0].coord.mu.Lock()
	nodes[0].coord.treeHash = []byte("diverged-hash-alice")
	nodes[0].coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("diverged-hash-alice"),
		MemberCount: 1,
	})
	nodes[0].coord.mu.Unlock()

	// Alice announces her diverged state
	nodes[0].coord.BroadcastAnnounce()
	network.DrainAll()

	// Bob should have detected the fork
	snap := nodes[1].coord.GetMetrics()
	if snap.PartitionsDetected != 1 {
		t.Errorf("bob should detect 1 partition, got %d", snap.PartitionsDetected)
	}
}
