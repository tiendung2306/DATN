package coordination

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

type testNode struct {
	id      peer.ID
	coord   *Coordinator
	mls     *MockMLSEngine
	storage *MockStorage
}

type blockingAddMembersEngine struct {
	*MockMLSEngine
	started chan struct{}
}

type blockingCreateGroupEngine struct {
	*MockMLSEngine
	started chan struct{}
}

func (b *blockingCreateGroupEngine) CreateGroup(ctx context.Context, _ string, _ []byte) ([]byte, []byte, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	return nil, nil, ctx.Err()
}

func (b *blockingAddMembersEngine) AddMembers(ctx context.Context, _ []byte, _ [][]byte) ([]byte, []byte, []byte, []byte, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	return nil, nil, nil, nil, ctx.Err()
}

func (b *blockingAddMembersEngine) CreateCommit(ctx context.Context, _ []byte, _ [][]byte) (CreateCommitResult, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	return CreateCommitResult{}, ctx.Err()
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
		if err := transport.Subscribe("/coordination/direct/1.0.0", coord.ReceiveDirectMessage); err != nil {
			t.Fatalf("Subscribe direct[%d]: %v", i, err)
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

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", timeout, description)
}

func expectPeerMembers(t *testing.T, got, want []peer.ID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("members length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("members[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

// exchangeHeartbeats makes each node broadcast a heartbeat and delivers them.
func exchangeHeartbeats(nodes []*testNode, network *FakeNetwork) {
	for _, n := range nodes {
		n.coord.BroadcastHeartbeat()
	}
	network.DrainAll()
}

func mustRealPeerID(t *testing.T) peer.ID {
	t.Helper()
	priv, _, err := p2pcrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}
	return id
}

func setupClusterWithIDs(t *testing.T, ids []peer.ID, groupID string) ([]*testNode, *FakeNetwork, *FakeClock) {
	t.Helper()
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	nodes := make([]*testNode, 0, len(ids))

	for i, id := range ids {
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
		if err := transport.Subscribe("/coordination/direct/1.0.0", coord.ReceiveDirectMessage); err != nil {
			t.Fatalf("Subscribe direct[%d]: %v", i, err)
		}
		nodes = append(nodes, &testNode{id: id, coord: coord, mls: mls, storage: storage})
	}
	return nodes, network, clk
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

func TestCoordinator_InitialActiveViewSeedsKnownPeers(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	local := peerID("local")
	remote := peerID("remote")
	coord, err := NewCoordinator(CoordinatorOpts{
		Config:            TestConfig(),
		Transport:         network.AddNode(local),
		Clock:             clk,
		MLS:               NewMockMLSEngine(),
		Storage:           NewMockStorage(),
		LocalID:           local,
		GroupID:           "grp-initial-view",
		InitialActiveView: []peer.ID{remote},
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}
	if err := coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := coord.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer coord.Stop()

	expectPeerMembers(t, coord.ActiveMembers(), []peer.ID{local, remote})
}

func TestCoordinator_BootstrapGuardDefersSingletonCommit(t *testing.T) {
	nodes, _, clk := setupCluster(t, 1, "grp-bootstrap-guard")
	coord := nodes[0].coord
	coord.cfg.ViewBootstrapGrace = 200 * time.Millisecond
	remote := peerID("remote-authorized")
	provider := func(string, uint64, []BufferedProposal) ([]peer.ID, error) {
		return []peer.ID{nodes[0].id, remote}, nil
	}
	coord.authorizedCommitters = provider

	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	coord.singleWriter.SetAuthorizedCommitters(coord.groupID, provider)

	res, err := coord.AddMember(AddMemberRequest{
		KeyPackageBytes: []byte("kp-new-member"),
		TargetPeerID:    peerID("new-member"),
		OperationID:     "op-bootstrap-guard",
	})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if !res.Deferred {
		t.Fatalf("AddMember should be deferred during bootstrap singleton view")
	}
	if got := coord.CurrentEpoch(); got != 0 {
		t.Fatalf("epoch after deferred AddMember = %d, want 0", got)
	}
	if got := coord.singleWriter.ProposalCount(); got != 1 {
		t.Fatalf("buffered proposals = %d, want 1", got)
	}

	clk.Advance(200 * time.Millisecond)
	waitForCondition(t, time.Second, func() bool {
		return coord.CurrentEpoch() == 1
	}, "bootstrap grace expiry commits deferred proposal")
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

func TestCoordinator_SendMessage_DirectRetryRepairsDelayedGossip(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-direct-repair")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	if _, err := alice.coord.SendMessage([]byte("repair via direct retry")); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got := len(bob.storage.Messages()); got != 0 {
		t.Fatalf("bob should not have message before gossip drain/direct retry, got %d", got)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		alice.coord.mu.Lock()
		defer alice.coord.mu.Unlock()
		return len(alice.coord.pendingAppDeliveries) == 1
	}, "sender should track pending delivery")

	alice.coord.RetryOutstandingDeliveriesTo(bob.id)

	waitForCondition(t, time.Second, func() bool {
		return len(bob.storage.Messages()) == 1
	}, "direct retry should deliver message")

	network.DrainAll()

	bobMsgs := bob.storage.Messages()
	if len(bobMsgs) != 1 {
		t.Fatalf("bob should have exactly 1 deduplicated message, got %d", len(bobMsgs))
	}
	if string(bobMsgs[0].Content) != "repair via direct retry" {
		t.Fatalf("unexpected content %q", bobMsgs[0].Content)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		alice.coord.mu.Lock()
		defer alice.coord.mu.Unlock()
		return len(alice.coord.pendingAppDeliveries) == 0
	}, "sender should clear pending delivery after ack")
}

func TestCoordinator_SendMessage_RetryOutstandingDeliveriesAfterReconnect(t *testing.T) {
	nodes, network, clk := setupCluster(t, 2, "grp-direct-reconnect")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)
	baseWaiters := clk.WaitersCount()

	alice := nodes[0]
	bob := nodes[1]
	network.Partition([]peer.ID{alice.id}, []peer.ID{bob.id})

	if _, err := alice.coord.SendMessage([]byte("repair after reconnect")); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got := len(bob.storage.Messages()); got != 0 {
		t.Fatalf("bob should not receive message while partitioned, got %d", got)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		alice.coord.mu.Lock()
		defer alice.coord.mu.Unlock()
		return len(alice.coord.pendingAppDeliveries) == 1
	}, "sender should track pending delivery while partitioned")

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return clk.WaitersCount() >= baseWaiters+1
	}, "ack retry timer should register while partitioned")

	for i := 0; i < alice.coord.cfg.ApplicationDirectRetryLimit; i++ {
		clk.Advance(alice.coord.cfg.ApplicationAckTimeout)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		alice.coord.mu.Lock()
		defer alice.coord.mu.Unlock()
		return len(alice.coord.pendingAppDeliveries) == 1
	}, "sender should keep pending delivery for reconnect repair")

	network.Heal()
	alice.coord.RetryOutstandingDeliveriesTo(bob.id)

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return len(bob.storage.Messages()) == 1
	}, "reconnect-triggered retry should deliver message")

	waitForCondition(t, 200*time.Millisecond, func() bool {
		alice.coord.mu.Lock()
		defer alice.coord.mu.Unlock()
		return len(alice.coord.pendingAppDeliveries) == 0
	}, "sender should clear pending delivery after reconnect ack")
}

func TestCoordinator_ReplayEnvelopes_DeduplicatesDuplicateApplication(t *testing.T) {
	nodes, _, _ := setupCluster(t, 2, "grp-replay")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	if _, err := nodes[0].coord.SendMessage([]byte("offline hello")); err != nil {
		t.Fatal(err)
	}

	recs, err := nodes[0].storage.GetEnvelopesSince("grp-replay", 0, 10)
	if err != nil {
		t.Fatalf("GetEnvelopesSince: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 local envelope, got %d", len(recs))
	}

	applied, err := nodes[1].coord.ReplayEnvelopes([][]byte{recs[0].Envelope, recs[0].Envelope})
	if err != nil {
		t.Fatalf("ReplayEnvelopes: %v", err)
	}
	if applied != 1 {
		t.Fatalf("ReplayEnvelopes applied=%d, want 1", applied)
	}

	msgs := nodes[1].storage.Messages()
	if len(msgs) != 1 {
		t.Fatalf("recipient stored %d messages, want 1", len(msgs))
	}
	if string(msgs[0].Content) != "offline hello" {
		t.Fatalf("recipient content=%q, want %q", msgs[0].Content, "offline hello")
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
		Epoch:       0,
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

func TestCoordinator_DoesNotAdvanceEpochWhenCommitPersistFails(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-persist-fail")
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
	if holder == nil {
		t.Fatal("no token holder found")
	}

	holder.storage.FailNextApplyCommit()
	if err := holder.coord.ProposeUpdate([]byte("advance")); err != nil {
		t.Fatal(err)
	}
	network.DrainAll()

	if holder.coord.CurrentEpoch() != 0 {
		t.Fatalf("holder epoch advanced despite persist failure: got %d want 0", holder.coord.CurrentEpoch())
	}
}

func TestCoordinator_RemoveMember_TokenHolderCommitsImmediately(t *testing.T) {
	aliceID := mustRealPeerID(t)
	bobID := mustRealPeerID(t)
	nodes, network, _ := setupClusterWithIDs(t, []peer.ID{aliceID, bobID}, "grp-remove-holder")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var holder, other *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else {
			other = n
		}
	}
	if holder == nil || other == nil {
		t.Fatal("failed to elect holder/other")
	}

	if err := holder.coord.RemoveMember([]byte("target-identity-bob")); err != nil {
		t.Fatalf("RemoveMember(holder): %v", err)
	}

	network.DrainAll() // deliver holder's commit to other

	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 1 {
			t.Fatalf("node %s: expected epoch 1 after remove commit, got %d", n.id, n.coord.CurrentEpoch())
		}
	}
	snap := holder.coord.GetMetrics()
	if snap.CommitsIssued != 1 {
		t.Fatalf("holder commits_issued=%d, want 1", snap.CommitsIssued)
	}
}

func TestCoordinator_RemoveMember_NonHolderProposesAndHolderCommits(t *testing.T) {
	aliceID := mustRealPeerID(t)
	bobID := mustRealPeerID(t)
	nodes, network, _ := setupClusterWithIDs(t, []peer.ID{aliceID, bobID}, "grp-remove-non-holder")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var holder, other *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else {
			other = n
		}
	}
	if holder == nil || other == nil {
		t.Fatal("failed to elect holder/other")
	}

	if err := other.coord.RemoveMember([]byte("target-identity-alice")); err != nil {
		t.Fatalf("RemoveMember(non-holder): %v", err)
	}

	network.DrainAll() // proposal reaches holder -> holder auto-commits
	network.DrainAll() // commit reaches proposer

	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 1 {
			t.Fatalf("node %s: expected epoch 1 after remove proposal flow, got %d", n.id, n.coord.CurrentEpoch())
		}
	}
	snap := holder.coord.GetMetrics()
	if snap.CommitsIssued != 1 {
		t.Fatalf("holder commits_issued=%d, want 1", snap.CommitsIssued)
	}
}

func TestCoordinator_LocalRemovedAfterCommit_BlocksMutations(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-removed-local")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Simulate that node[1] is no longer in membership after processing epoch 1.
	nodes[1].mls.SetHasMemberFunc(func(groupState []byte, _ []byte) (bool, error) {
		var st mockGroupState
		if err := json.Unmarshal(groupState, &st); err != nil {
			return false, err
		}
		return st.Epoch == 0, nil
	})

	var holder, removed *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else {
			removed = n
		}
	}
	if holder == nil || removed == nil {
		t.Fatal("failed to elect holder/removed")
	}
	removed.coord.localIdentity = []byte("local-identity")

	if err := holder.coord.ProposeUpdate([]byte("rotate")); err != nil {
		t.Fatalf("holder propose update: %v", err)
	}
	network.DrainAll()

	if removed.coord.CurrentEpoch() != 1 {
		t.Fatalf("removed node epoch=%d, want 1", removed.coord.CurrentEpoch())
	}
	if _, err := removed.coord.SendMessage([]byte("blocked")); !errors.Is(err, ErrAccessRevoked) {
		t.Fatalf("SendMessage err=%v, want ErrAccessRevoked", err)
	}
	if err := removed.coord.ProposeUpdate([]byte("blocked")); !errors.Is(err, ErrAccessRevoked) {
		t.Fatalf("ProposeUpdate err=%v, want ErrAccessRevoked", err)
	}
}

// addMemberObservation records the per-delivery arguments fed into the
// OnAddCommitted callback so unit tests can verify token-holder vs observer
// semantics (welcome present iff this node ran CreateCommit).
type addMemberObservation struct {
	delivery    AddCommitDelivery
	commitEpoch uint64
	welcomeLen  int
}

// TestCoordinator_AddMember_NonHolderProposesAndHolderCommits is the direct
// regression for the user's bug report: when the creator/approver is NOT the
// current Token Holder, AddMember MUST broadcast a ProposalAdd carrying
// routing metadata instead of returning ErrNotTokenHolder. The actual Token
// Holder receives the proposal, commits it, and emits AddCommitDelivery so
// every node — including the original approver — can update its lifecycle
// state. Only the Token Holder receives the Welcome bytes in its callback;
// the approver receives a nil-welcome observer callback for audit purposes.
func TestCoordinator_AddMember_NonHolderProposesAndHolderCommits(t *testing.T) {
	aliceID := mustRealPeerID(t)
	bobID := mustRealPeerID(t)
	nodes, network, _ := setupClusterWithIDs(t, []peer.ID{aliceID, bobID}, "grp-add-non-holder")
	createAndShareGroup(t, nodes)

	// Capture observations keyed by the node's stable peer id, NOT by the
	// runtime IsTokenHolder() check at callback fire time — the epoch
	// advance from the commit can rotate the holder, which would otherwise
	// route the callback into the wrong bucket. Callbacks fire from
	// independent goroutines (holder's tryCommitLocked vs. observer's
	// handleCommitLocked drain), so the map must be mutex-guarded.
	var obsMu sync.Mutex
	obsByNode := make(map[peer.ID][]addMemberObservation)
	for _, n := range nodes {
		n := n
		n.coord.onAddCommitted = func(d AddCommitDelivery, epoch uint64, welcome []byte) {
			obsMu.Lock()
			obsByNode[n.id] = append(obsByNode[n.id], addMemberObservation{
				delivery: d, commitEpoch: epoch, welcomeLen: len(welcome),
			})
			obsMu.Unlock()
		}
	}
	snapshot := func(id peer.ID) []addMemberObservation {
		obsMu.Lock()
		defer obsMu.Unlock()
		out := make([]addMemberObservation, len(obsByNode[id]))
		copy(out, obsByNode[id])
		return out
	}

	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var holder, other *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else {
			other = n
		}
	}
	if holder == nil || other == nil {
		t.Fatal("failed to elect holder/other")
	}

	const operationID = "ga_test_operation_1"
	const kpHash = "deadbeef"
	res, err := other.coord.AddMember(AddMemberRequest{
		TargetPeerID:    peerID("invitee"),
		KeyPackageBytes: []byte("mock-key-package-bytes"),
		OperationID:     operationID,
		KeyPackageHash:  []byte(kpHash),
		GroupType:       "channel",
		CategoryID:      "cat-1",
	})
	if err != nil {
		t.Fatalf("AddMember(non-holder): %v", err)
	}
	if !res.Deferred {
		t.Fatalf("non-holder AddMember must return Deferred=true, got %#v", res)
	}
	if len(res.Welcome) != 0 {
		t.Fatalf("non-holder must NOT receive Welcome bytes (only token holder owns ephemeral material), got %d bytes", len(res.Welcome))
	}
	if res.OperationID != operationID {
		t.Fatalf("OperationID round-trip mismatch: got %q want %q", res.OperationID, operationID)
	}

	network.DrainAll() // proposal -> holder; holder commits -> back to other
	network.DrainAll()

	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 1 {
			t.Fatalf("node %s: epoch=%d want 1 after proposal-commit", n.id, n.coord.CurrentEpoch())
		}
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && (len(snapshot(holder.id)) == 0 || len(snapshot(other.id)) == 0) {
		time.Sleep(5 * time.Millisecond)
	}

	holderObs := snapshot(holder.id)
	if len(holderObs) != 1 {
		t.Fatalf("token holder OnAddCommitted callback count=%d, want 1", len(holderObs))
	}
	if holderObs[0].welcomeLen <= 0 {
		t.Fatalf("token holder must receive non-empty Welcome bytes, got %d", holderObs[0].welcomeLen)
	}
	if holderObs[0].delivery.OperationID != operationID {
		t.Fatalf("holder delivery operation_id=%q want %q", holderObs[0].delivery.OperationID, operationID)
	}

	nonHolderObs := snapshot(other.id)
	if len(nonHolderObs) != 1 {
		t.Fatalf("non-holder OnAddCommitted callback count=%d, want 1 (observer marks commit_observed)", len(nonHolderObs))
	}
	if nonHolderObs[0].welcomeLen != 0 {
		t.Fatalf("non-holder MUST receive nil Welcome (no ephemeral keys), got %d bytes", nonHolderObs[0].welcomeLen)
	}
	if nonHolderObs[0].delivery.OperationID != operationID {
		t.Fatalf("observer delivery operation_id=%q want %q", nonHolderObs[0].delivery.OperationID, operationID)
	}
	if nonHolderObs[0].commitEpoch != 1 {
		t.Fatalf("observer commit_epoch=%d want 1", nonHolderObs[0].commitEpoch)
	}
}

// TestCoordinator_AddMember_TokenHolderCommitsDirectly mirrors the
// RemoveMember happy-path: when the local node IS the Token Holder, AddMember
// commits synchronously and yields Welcome bytes in the result struct.
func TestCoordinator_AddMember_TokenHolderCommitsDirectly(t *testing.T) {
	aliceID := mustRealPeerID(t)
	bobID := mustRealPeerID(t)
	nodes, network, _ := setupClusterWithIDs(t, []peer.ID{aliceID, bobID}, "grp-add-holder")
	createAndShareGroup(t, nodes)

	// Callback fires from a goroutine after tryCommitLocked succeeds AND
	// (separately) when the network drain delivers the commit to the
	// observer node. The map must be mutex-guarded to avoid the
	// concurrent-map-writes fatal that surfaced when this test ran
	// inside the full coordination suite.
	var obsMu sync.Mutex
	obsByNode := make(map[peer.ID][]addMemberObservation)
	for _, n := range nodes {
		n := n
		n.coord.onAddCommitted = func(d AddCommitDelivery, epoch uint64, welcome []byte) {
			obsMu.Lock()
			obsByNode[n.id] = append(obsByNode[n.id], addMemberObservation{
				delivery: d, commitEpoch: epoch, welcomeLen: len(welcome),
			})
			obsMu.Unlock()
		}
	}
	snapshot := func(id peer.ID) []addMemberObservation {
		obsMu.Lock()
		defer obsMu.Unlock()
		out := make([]addMemberObservation, len(obsByNode[id]))
		copy(out, obsByNode[id])
		return out
	}

	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var holder *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
			break
		}
	}
	if holder == nil {
		t.Fatal("failed to elect token holder")
	}

	res, err := holder.coord.AddMember(AddMemberRequest{
		TargetPeerID:    peerID("invitee"),
		KeyPackageBytes: []byte("mock-key-package-bytes"),
		OperationID:     "ga_direct_1",
		KeyPackageHash:  []byte("kphash"),
	})
	if err != nil {
		t.Fatalf("AddMember(holder): %v", err)
	}
	if res.Deferred {
		t.Fatal("holder AddMember must NOT defer")
	}
	if len(res.Welcome) == 0 {
		t.Fatal("holder AddMember must produce Welcome bytes")
	}
	if res.CommitEpoch != 1 {
		t.Fatalf("CommitEpoch=%d want 1", res.CommitEpoch)
	}

	network.DrainAll()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && len(snapshot(holder.id)) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	holderObs := snapshot(holder.id)
	if len(holderObs) != 1 {
		t.Fatalf("token holder OnAddCommitted count=%d, want 1", len(holderObs))
	}
	if holderObs[0].welcomeLen == 0 {
		t.Fatal("token holder direct AddMember must surface Welcome in callback")
	}
}

func TestCoordinator_AddMember_TimesOutInsteadOfHangingForever(t *testing.T) {
	aliceID := mustRealPeerID(t)
	bobID := mustRealPeerID(t)
	nodes, network, _ := setupClusterWithIDs(t, []peer.ID{aliceID, bobID}, "grp-add-timeout")
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
	if holder == nil {
		t.Fatal("failed to elect token holder")
	}

	holder.coord.cfg.MLSOperationTimeout = 50 * time.Millisecond
	blocker := &blockingAddMembersEngine{
		MockMLSEngine: NewMockMLSEngine(),
		started:       make(chan struct{}),
	}
	holder.coord.mls = blocker

	startedAt := time.Now()
	_, err := holder.coord.AddMember(AddMemberRequest{
		TargetPeerID:    peerID("invitee-timeout"),
		KeyPackageBytes: []byte("mock-key-package-bytes"),
		OperationID:     "ga_timeout_1",
		KeyPackageHash:  []byte("kphash-timeout"),
	})
	elapsed := time.Since(startedAt)

	if err == nil {
		t.Fatal("expected AddMember timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("AddMember took too long after timeout: %v", elapsed)
	}
	if holder.coord.CurrentEpoch() != 0 {
		t.Fatalf("epoch advanced unexpectedly to %d", holder.coord.CurrentEpoch())
	}
}

func TestCoordinator_CreateGroup_TimesOutInsteadOfHangingForever(t *testing.T) {
	id := mustRealPeerID(t)
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := TestConfig()
	cfg.MLSOperationTimeout = 50 * time.Millisecond
	blocker := &blockingCreateGroupEngine{
		MockMLSEngine: NewMockMLSEngine(),
		started:       make(chan struct{}),
	}

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:    cfg,
		Transport: network.AddNode(id),
		Clock:     clk,
		MLS:       blocker,
		Storage:   NewMockStorage(),
		LocalID:   id,
		GroupID:   "grp-create-timeout",
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	startedAt := time.Now()
	err = coord.CreateGroup()
	elapsed := time.Since(startedAt)

	if err == nil {
		t.Fatal("expected CreateGroup timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("CreateGroup took too long after timeout: %v", elapsed)
	}
}

// peerObservation records a single OnPeerObserved firing so the test can
// assert exactly-once-per-transition semantics for Phase A roster sync.
type peerObservation struct {
	groupID string
	peer    peer.ID
	at      time.Time
}

// TestCoordinator_OnPeerObserved_FiresOnceOnFirstHeartbeat verifies the Phase
// A invariant: OnPeerObserved fires exactly once on the absent→present
// transition for a remote peer, and does NOT fire on subsequent heartbeats
// from the same peer (which would spam DB writes every ~5s). After an
// eviction via CheckLiveness, a fresh heartbeat is treated as a new
// observation and fires the callback again — that re-fire is acceptable
// (and useful) because the roster may need re-confirmation post-restart.
func TestCoordinator_OnPeerObserved_FiresOnceOnFirstHeartbeat(t *testing.T) {
	groupID := "grp-peer-observed"
	nodes, network, _ := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)

	var (
		obsMu sync.Mutex
		obs   []peerObservation
	)
	nodes[0].coord.onPeerObserved = func(g string, p peer.ID, at time.Time) {
		obsMu.Lock()
		defer obsMu.Unlock()
		obs = append(obs, peerObservation{groupID: g, peer: p, at: at})
	}

	startAll(t, nodes)

	waitForObs := func(want int) []peerObservation {
		t.Helper()
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			obsMu.Lock()
			got := len(obs)
			snap := append([]peerObservation(nil), obs...)
			obsMu.Unlock()
			if got >= want {
				return snap
			}
			time.Sleep(5 * time.Millisecond)
		}
		obsMu.Lock()
		defer obsMu.Unlock()
		return append([]peerObservation(nil), obs...)
	}

	// First heartbeat from node[1] → callback fires once with peer=node[1].
	nodes[1].coord.BroadcastHeartbeat()
	network.DrainAll()
	got := waitForObs(1)
	if len(got) != 1 {
		t.Fatalf("first heartbeat: observations=%d, want 1; got=%+v", len(got), got)
	}
	if got[0].groupID != groupID {
		t.Fatalf("first heartbeat: groupID=%q want %q", got[0].groupID, groupID)
	}
	if got[0].peer != nodes[1].id {
		t.Fatalf("first heartbeat: peer=%s want %s", got[0].peer, nodes[1].id)
	}

	// Second heartbeat from the same peer → MUST NOT fire callback again.
	nodes[1].coord.BroadcastHeartbeat()
	network.DrainAll()
	time.Sleep(50 * time.Millisecond)
	obsMu.Lock()
	if len(obs) != 1 {
		obsMu.Unlock()
		t.Fatalf("second heartbeat must not refire OnPeerObserved; observations=%d", len(obs))
	}
	obsMu.Unlock()

	// Evict node[1] via repeated liveness checks, then a fresh heartbeat
	// should treat it as a new observation (acceptable behavior on rejoin).
	for i := 0; i < nodes[0].coord.cfg.PeerDeadAfter; i++ {
		nodes[0].coord.TriggerLivenessCheck()
	}
	if nodes[0].coord.activeView.Contains(nodes[1].id) {
		t.Fatal("expected node[1] to be evicted from active view")
	}
	nodes[1].coord.BroadcastHeartbeat()
	network.DrainAll()
	got = waitForObs(2)
	if len(got) != 2 {
		t.Fatalf("post-eviction rejoin: observations=%d, want 2; got=%+v", len(got), got)
	}
	if got[1].peer != nodes[1].id {
		t.Fatalf("post-eviction rejoin: peer=%s want %s", got[1].peer, nodes[1].id)
	}
}

func TestSingleWriter_SnapshotNextBatch_MixedDeterministicRefs(t *testing.T) {
	clk := NewFakeClock(time.Now())
	cfg := TestConfig()
	av := NewActiveView(clk, cfg, peerID("alice"), nil)
	sw := NewSingleWriter(av, peerID("alice"), 1, cfg)

	sw.BufferProposal(BufferedProposal{Type: ProposalAdd, Data: []byte("kp-1"), ProposalRef: []byte{0x20}, OperationID: "op-1"})
	sw.BufferProposal(BufferedProposal{Type: ProposalRemove, Data: []byte("identity-3"), ProposalRef: []byte{0x10}})
	sw.BufferProposal(BufferedProposal{Type: ProposalAdd, Data: []byte("kp-2"), ProposalRef: []byte{0x30}, OperationID: "op-2"})

	batch := sw.SnapshotNextBatch()
	if len(batch) != 3 {
		t.Fatalf("batch len=%d, want 3 mixed proposals", len(batch))
	}
	if batch[0].Type != ProposalRemove || batch[1].OperationID != "op-1" || batch[2].OperationID != "op-2" {
		t.Fatalf("batch not sorted by ProposalRef: %+v", batch)
	}

	drained := sw.DrainBatchByRefs([][]byte{{0x10}, {0x20}, {0x30}})
	if len(drained) != 3 {
		t.Fatalf("drained len=%d, want 3", len(drained))
	}
	if sw.ProposalCount() != 0 {
		t.Fatalf("proposal count=%d, want 0", sw.ProposalCount())
	}
}

func TestCoordinator_TokenHolderCommitEnvelopeIncludesProposalsAndRefs(t *testing.T) {
	nodes, _, _ := setupCluster(t, 1, "grp-commit-envelope-proposals")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	result, err := nodes[0].coord.AddMember(AddMemberRequest{
		TargetPeerID:    peerID("invitee-envelope"),
		KeyPackageBytes: []byte("mock-key-package-envelope"),
		OperationID:     "ga_envelope_1",
		RequestID:       "req_envelope_1",
		KeyPackageHash:  []byte("kphash-envelope"),
	})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if result.Deferred {
		t.Fatal("single-node holder path must commit synchronously")
	}

	recs, err := nodes[0].storage.GetEnvelopesSince("grp-commit-envelope-proposals", 0, 10)
	if err != nil {
		t.Fatalf("GetEnvelopesSince: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("expected local commit envelope")
	}
	var env Envelope
	if err := json.Unmarshal(recs[0].Envelope, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var commit CommitMsg
	if err := json.Unmarshal(env.Payload, &commit); err != nil {
		t.Fatalf("decode commit: %v", err)
	}
	if len(commit.IncludedProposals) != 1 {
		t.Fatalf("IncludedProposals len=%d, want 1", len(commit.IncludedProposals))
	}
	if len(commit.CommittedProposalRefs) != 1 {
		t.Fatalf("CommittedProposalRefs len=%d, want 1", len(commit.CommittedProposalRefs))
	}
	if !bytes.Equal(commit.IncludedProposals[0].ProposalRef, commit.CommittedProposalRefs[0]) {
		t.Fatalf("included proposal ref does not match committed ref")
	}
	if commit.IncludedProposals[0].OperationID != "ga_envelope_1" {
		t.Fatalf("operation metadata not preserved: %+v", commit.IncludedProposals[0])
	}
}

func TestCoordinator_AnnounceIncludesLastCommitHash(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-announce-commit-hash")
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
	if holder == nil {
		t.Fatal("failed to elect token holder")
	}

	if _, err := holder.coord.AddMember(AddMemberRequest{
		TargetPeerID:    peerID("invitee-announce"),
		KeyPackageBytes: []byte("mock-key-package-announce"),
		OperationID:     "ga_announce_commit_hash",
		KeyPackageHash:  []byte("kphash-announce"),
	}); err != nil {
		t.Fatalf("AddMember(holder): %v", err)
	}
	network.DrainAll()

	recs, err := holder.storage.GetEnvelopesSince("grp-announce-commit-hash", 0, 10)
	if err != nil {
		t.Fatalf("GetEnvelopesSince: %v", err)
	}
	var expectedCommitHash []byte
	for _, rec := range recs {
		if rec.MsgType != MsgCommit {
			continue
		}
		var env Envelope
		if err := json.Unmarshal(rec.Envelope, &env); err != nil {
			t.Fatalf("decode commit envelope: %v", err)
		}
		var commit CommitMsg
		if err := json.Unmarshal(env.Payload, &commit); err != nil {
			t.Fatalf("decode commit payload: %v", err)
		}
		sum := sha256.Sum256(commit.CommitData)
		expectedCommitHash = sum[:]
		break
	}
	if len(expectedCommitHash) == 0 {
		t.Fatal("expected at least one persisted commit envelope")
	}

	holder.coord.BroadcastAnnounce()

	network.mu.Lock()
	defer network.mu.Unlock()
	for _, pending := range network.inbox {
		if pending.from != holder.id {
			continue
		}
		var env Envelope
		if err := json.Unmarshal(pending.data, &env); err != nil || env.Type != MsgAnnounce {
			continue
		}
		var ann GroupStateAnnouncement
		if err := json.Unmarshal(env.Payload, &ann); err != nil {
			t.Fatalf("decode announce payload: %v", err)
		}
		if !bytes.Equal(ann.CommitHash, expectedCommitHash) {
			t.Fatalf("announce CommitHash=%x want %x", ann.CommitHash, expectedCommitHash)
		}
		return
	}
	t.Fatal("expected a pending announce envelope from holder")
}

func TestCoordinator_StartLoadsPersistedLastCommitHashForAnnounce(t *testing.T) {
	id := peerID("alice")
	groupID := "grp-load-last-commit-hash"
	storage := NewMockStorage()
	treeHash := []byte("tree-after-restart")
	lastCommitHash := []byte("last-commit-hash")
	if err := storage.SaveGroupRecord(&GroupRecord{
		GroupID:    groupID,
		GroupState: []byte("persisted-state"),
		Epoch:      7,
		TreeHash:   treeHash,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := storage.SaveCoordState(&CoordState{
		GroupID:        groupID,
		ActiveView:     []peer.ID{id},
		TokenHolder:    id,
		LastCommitHash: lastCommitHash,
	}); err != nil {
		t.Fatalf("SaveCoordState: %v", err)
	}

	network := NewFakeNetwork()
	coord, err := NewCoordinator(CoordinatorOpts{
		Config:    TestConfig(),
		Transport: network.AddNode(id),
		Clock:     NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		MLS:       NewMockMLSEngine(),
		Storage:   storage,
		LocalID:   id,
		GroupID:   groupID,
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}
	if err := coord.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer coord.Stop()

	if !bytes.Equal(coord.lastCommitHash, lastCommitHash) {
		t.Fatalf("loaded lastCommitHash=%x want %x", coord.lastCommitHash, lastCommitHash)
	}
}

func TestBuildAddDeliveriesFromBatch_FiltersMixedBatch(t *testing.T) {
	deliveries := buildAddDeliveriesFromBatch([]BufferedProposal{
		{
			Type:           ProposalRemove,
			TargetPeerID:   "peer-remove",
			OperationID:    "remove-op",
			KeyPackageHash: []byte("remove-hash"),
		},
		{
			Type:           ProposalAdd,
			TargetPeerID:   "peer-add",
			OperationID:    "add-op",
			RequestID:      "request-add",
			GroupType:      "dm",
			CategoryID:     "cat",
			KeyPackageHash: []byte("add-hash"),
		},
	}, []byte("welcome-for-adds"))

	if len(deliveries) != 1 {
		t.Fatalf("deliveries len=%d, want only the ProposalAdd delivery", len(deliveries))
	}
	if deliveries[0].TargetPeerID != "peer-add" || deliveries[0].OperationID != "add-op" {
		t.Fatalf("unexpected add delivery metadata: %+v", deliveries[0])
	}
	if len(deliveries[0].WelcomeHash) == 0 {
		t.Fatalf("welcome hash should be populated for add delivery")
	}
}

func TestCoordinator_LocalRemoved_CallbackFiresOnceOnDuplicateReplay(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-removed-callback")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	var callbackCount int
	nodes[1].coord.onAccessLost = func(_ string, _ uint64, _ string) {
		callbackCount++
	}
	nodes[1].coord.localIdentity = []byte("local-identity")
	nodes[1].mls.SetHasMemberFunc(func(groupState []byte, _ []byte) (bool, error) {
		var st mockGroupState
		if err := json.Unmarshal(groupState, &st); err != nil {
			return false, err
		}
		return st.Epoch == 0, nil
	})

	var holder, removed *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
		} else {
			removed = n
		}
	}
	if holder == nil || removed == nil {
		t.Fatal("failed to elect holder/removed")
	}

	if err := holder.coord.ProposeUpdate([]byte("rotate")); err != nil {
		t.Fatalf("holder propose update: %v", err)
	}
	network.DrainAll()

	// Re-apply the same commit envelope via replay path; callback must not fire again.
	recs, err := holder.storage.GetEnvelopesSince("grp-removed-callback", 0, 10)
	if err != nil {
		t.Fatalf("GetEnvelopesSince: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("expected at least one envelope")
	}
	_, err = removed.coord.ReplayEnvelopes([][]byte{recs[0].Envelope, recs[0].Envelope})
	if err != nil {
		t.Fatalf("ReplayEnvelopes: %v", err)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for callbackCount < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if callbackCount != 1 {
		t.Fatalf("access-lost callback count=%d, want 1", callbackCount)
	}
}

func TestCoordinator_TokenHolderFailover_AutoCommitsOutstanding(t *testing.T) {
	// Setup a 3-node cluster
	nodes, network, clk := setupCluster(t, 3, "grp-failover-autocommit")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Determine the initial Token Holder (Alice)
	var holderA, nodeB, nodeC *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holderA = n
			break
		}
	}
	if holderA == nil {
		t.Fatal("no token holder found")
	}

	// Identify B and C (Bob and Charlie)
	var siblings []*testNode
	for _, n := range nodes {
		if n.id != holderA.id {
			siblings = append(siblings, n)
		}
	}
	nodeB = siblings[0]
	nodeC = siblings[1]

	// Alice (Token Holder) goes offline. We partition Alice from Bob and Charlie.
	network.Partition([]peer.ID{holderA.id}, []peer.ID{nodeB.id, nodeC.id})

	// Bob proposes an update. This proposal is broadcast to Charlie but NOT to Alice.
	if err := nodeB.coord.ProposeUpdate([]byte("failover-rotate")); err != nil {
		t.Fatal(err)
	}

	// Drain network to deliver proposal between Bob and Charlie.
	network.DrainAll()

	// Assert that Bob is NOT yet the Token Holder, so he hasn't committed yet
	if nodeB.coord.CurrentEpoch() != 0 {
		t.Fatalf("Bob should still be at epoch 0, got %d", nodeB.coord.CurrentEpoch())
	}

	// Alice is now partitioned. We trigger liveness checks on Bob and Charlie.
	// PeerDeadAfter is 3. We advance the clock and trigger liveness 3 times.
	for i := 0; i < nodeB.coord.cfg.PeerDeadAfter; i++ {
		clk.Advance(nodeB.coord.cfg.HeartbeatInterval)
		nodeB.coord.TriggerLivenessCheck()
		nodeC.coord.TriggerLivenessCheck()
	}

	// Bob and Charlie should have evicted Alice
	if nodeB.coord.activeView.Contains(holderA.id) {
		t.Fatal("Bob should have evicted Alice from active view")
	}

	// The eviction callbackhandleActiveViewChange has fired asynchronously on Bob and Charlie.
	// Bob should have automatically detected that he is the new Token Holder, drained the buffer,
	// and committed the outstanding "failover-rotate" proposal.
	// Wait up to a short deadline for epoch transition.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && nodeB.coord.CurrentEpoch() != 1 {
		time.Sleep(5 * time.Millisecond)
	}

	// Drain network to deliver Bob's commit to Charlie
	network.DrainAll()

	// Assert that Bob and Charlie advanced to Epoch 1 automatically!
	if nodeB.coord.CurrentEpoch() != 1 {
		t.Fatalf("Bob epoch=%d want 1 (failover auto-commit failed)", nodeB.coord.CurrentEpoch())
	}
	if nodeC.coord.CurrentEpoch() != 1 {
		t.Fatalf("Charlie epoch=%d want 1 (failover auto-commit failed)", nodeC.coord.CurrentEpoch())
	}

	// Verify that Bob's commit metrics reflect that he issued the commit
	snap := nodeB.coord.GetMetrics()
	if snap.CommitsIssued != 1 {
		t.Fatalf("Bob should have issued 1 commit, got %d", snap.CommitsIssued)
	}
}

func TestCoordinator_BatchCommit_SuccessfulAccumulation(t *testing.T) {
	// Setup a 3-node cluster
	nodes, network, clk := setupCluster(t, 3, "grp-batch-accumulate")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Determine the initial Token Holder (Bob)
	var holderB, nodeA, nodeC *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holderB = n
			break
		}
	}
	if holderB == nil {
		t.Fatal("no token holder found")
	}

	// Identify A and C
	var siblings []*testNode
	for _, n := range nodes {
		if n.id != holderB.id {
			siblings = append(siblings, n)
		}
	}
	nodeA = siblings[0]
	nodeC = siblings[1]

	// Configure the production-fidelity delay of 1 * time.Second on Bob (Token Holder)
	holderB.coord.cfg.BatchingDelay = 1 * time.Second

	// Alice and Charlie concurrently broadcast ProposeUpdate
	if err := nodeA.coord.ProposeUpdate([]byte("proposal-alice")); err != nil {
		t.Fatal(err)
	}
	if err := nodeC.coord.ProposeUpdate([]byte("proposal-charlie")); err != nil {
		t.Fatal(err)
	}

	// Drain network so both proposals reach Bob's node before the timer fires
	network.DrainAll()

	// Verify that Bob has buffered both proposals, but has NOT committed yet
	// because the 1-second delay is still active.
	if holderB.coord.CurrentEpoch() != 0 {
		t.Fatalf("Holder should still be at epoch 0, got %d", holderB.coord.CurrentEpoch())
	}
	if holderB.coord.singleWriter.ProposalCount() != 2 {
		t.Fatalf("Holder should have 2 buffered proposals, got %d", holderB.coord.singleWriter.ProposalCount())
	}

	// Now advance virtual clock by exactly 1 * time.Second
	clk.Advance(1 * time.Second)

	// Since handleActiveViewChange / timer executes in a goroutine, sleep briefly to let it commit
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && holderB.coord.CurrentEpoch() != 1 {
		time.Sleep(5 * time.Millisecond)
	}

	// Drain network to deliver Commit to everyone
	network.DrainAll()

	// Verify that Bob successfully advanced to Epoch 1
	if holderB.coord.CurrentEpoch() != 1 {
		t.Fatalf("Holder should have advanced to epoch 1, got %d", holderB.coord.CurrentEpoch())
	}

	// Verify that Bob's commit metrics reflect that exactly 1 commit was issued (proving batching!)
	snap := holderB.coord.GetMetrics()
	if snap.CommitsIssued != 1 {
		t.Fatalf("Expected exactly 1 commit issued, got %d (batching failed!)", snap.CommitsIssued)
	}

	// Verify that all 3 nodes have converged to Epoch 1
	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 1 {
			t.Errorf("Node %s: expected epoch 1, got %d", n.id, n.coord.CurrentEpoch())
		}
	}
}

func TestCoordinator_StaleProposal_ResendAndCommit(t *testing.T) {
	// Setup a 3-node cluster
	nodes, network, _ := setupCluster(t, 3, "grp-stale-resend")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Determine the initial Token Holder (Bob)
	var holderB, nodeA, nodeC *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holderB = n
			break
		}
	}
	if holderB == nil {
		t.Fatal("no token holder found")
	}

	// Identify A and C
	var siblings []*testNode
	for _, n := range nodes {
		if n.id != holderB.id {
			siblings = append(siblings, n)
		}
	}
	nodeA = siblings[0]
	nodeC = siblings[1]

	// Configure immediate commit on Bob (BatchingDelay: 0) to trigger race condition
	holderB.coord.cfg.BatchingDelay = 0

	// Alice and Charlie concurrently broadcast proposals at Epoch 0 BEFORE network draining.
	// Since both call ProposeUpdate at Epoch 0, both proposals will carry epoch = 0.
	if err := nodeA.coord.ProposeUpdate([]byte("proposal-alice")); err != nil {
		t.Fatal(err)
	}

	if err := nodeC.coord.ProposeUpdate([]byte("proposal-charlie")); err != nil {
		t.Fatal(err)
	}

	// Bob processes Alice's proposal (epoch 0) first (or Charlie's, either way one wins).
	// Let's drain the network. The winning proposal is committed immediately, advancing everyone to Epoch 1.
	// The losing proposal is then received by Bob at Epoch 1. Since Bob is at Epoch 1 and the losing proposal
	// carries epoch 0, Bob rejects it as stale. Charlie receives the winner's commit, advances to Epoch 1,
	// and clears his own proposal buffer.
	network.DrainAll()

	// Verify that the entire group has successfully converged to Epoch 1
	if holderB.coord.CurrentEpoch() != 1 {
		t.Fatalf("Bob epoch=%d, want 1", holderB.coord.CurrentEpoch())
	}
	if nodeA.coord.CurrentEpoch() != 1 {
		t.Fatalf("Alice epoch=%d, want 1", nodeA.coord.CurrentEpoch())
	}
	if nodeC.coord.CurrentEpoch() != 1 {
		t.Fatalf("Charlie epoch=%d, want 1", nodeC.coord.CurrentEpoch())
	}

	// Since Charlie's proposal was rejected and his buffer is empty,
	// Charlie realizes his update was left behind and re-proposes it at Epoch 1.
	if err := nodeC.coord.ProposeUpdate([]byte("proposal-charlie-retry")); err != nil {
		t.Fatal(err)
	}

	// Bob receives Charlie's retried proposal (epoch 1), buffers it, and commits it.
	network.DrainAll()

	// Verify that the entire group has successfully converged to Epoch 2,
	// proving that the stale proposal retry was committed successfully!
	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 2 {
			t.Errorf("Node %s: expected final epoch 2, got %d", n.id, n.coord.CurrentEpoch())
		}
	}
}
