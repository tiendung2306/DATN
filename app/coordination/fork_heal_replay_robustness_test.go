package coordination

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// failingStorage embeds CoordinationStorage and allows mocking SaveGroupRecord failures.
type failingStorage struct {
	CoordinationStorage
	failSave bool
}

func (fs *failingStorage) SaveGroupRecord(rec *GroupRecord) error {
	if fs.failSave {
		return errors.New("mock db save failure")
	}
	return fs.CoordinationStorage.SaveGroupRecord(rec)
}

// TestIntegration_Replay_NonRepudiationIsolation ensures that when multiple nodes are partitioned
// in the losing branch and both generate traffic, each node only replays its own messages.
func TestIntegration_Replay_NonRepudiationIsolation(t *testing.T) {
	groupID := "grp-isolation"
	nodes, network, clk := setupCluster(t, 3, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0] // loser node 1
	bob := nodes[1]   // winner node
	carol := nodes[2] // loser node 2

	// Diverge branch markers for detection.
	alice.coord.mu.Lock()
	alice.coord.treeHash = []byte("loser-tree")
	alice.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("loser-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	alice.coord.mu.Unlock()

	carol.coord.mu.Lock()
	carol.coord.treeHash = []byte("loser-tree")
	carol.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("loser-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	carol.coord.mu.Unlock()

	bob.coord.mu.Lock()
	bob.coord.treeHash = []byte("winner-tree")
	bob.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 3,
		Epoch:       0,
	})
	bob.coord.mu.Unlock()

	partitionStart := clk.Now().Add(1 * time.Second)
	clk.Set(partitionStart)

	// Update Alice's fork detector with Bob's winning branch announcement
	alice.coord.mu.Lock()
	alice.coord.forkDetector.ProcessRemote(partitionStart, bob.id, bob.coord.CurrentEpoch(), GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 3,
		Epoch:       0,
	})
	alice.coord.mu.Unlock()

	// Partition losing segment (Alice & Carol) away from winning segment (Bob)
	network.Partition([]peer.ID{alice.id, carol.id}, []peer.ID{bob.id})

	// Alice and Carol send messages while partitioned.
	// These reach each other (within losing segment) but NOT Bob.
	if _, err := alice.coord.SendMessage([]byte("msg-from-alice")); err != nil {
		t.Fatalf("Alice message send: %v", err)
	}
	if _, err := carol.coord.SendMessage([]byte("msg-from-carol")); err != nil {
		t.Fatalf("Carol message send: %v", err)
	}
	network.DrainAll()

	// Heal network partition
	network.Heal()

	// Configure Alice and Carol to fetch Bob's group info during heal.
	// Both losers need groupInfoFetch so they can send ProposalJoin to Bob.
	for _, loser := range []*testNode{alice, carol} {
		l := loser
		l.coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, _ string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
			if remote != bob.id {
				return nil, errors.New("wrong remote")
			}
			groupInfo, err := bob.mls.ExportGroupInfo(ctx, bob.coord.GetGroupState(), withRatchetTree)
			if err != nil {
				return nil, err
			}
			return &GroupInfoFetchResult{
				GroupInfo: groupInfo,
				Epoch:     bob.coord.CurrentEpoch(),
				TreeHash:  bob.coord.GetTreeHash(),
			}, nil
		}
	}

	// Trigger heal by having Bob announce his winning branch.
	// Both Alice and Carol will detect the fork and initiate healing concurrently.
	// The test waits for BOTH losers to successfully heal.
	bob.coord.mu.Lock()
	bob.coord.broadcastAnnounceLocked()
	bob.coord.mu.Unlock()

	// Wait for both Alice AND Carol to successfully converge
	network.DrainAll()

	if !waitFor(t, 10*time.Second, func() bool {
		network.DrainAll()
		aliceOK := alice.coord.GetMetrics().ForkHealingsSucceeded >= 1 &&
			alice.coord.CurrentEpoch() >= 1
		carolOK := carol.coord.GetMetrics().ForkHealingsSucceeded >= 1 &&
			carol.coord.CurrentEpoch() >= 1
		return aliceOK && carolOK
	}) {
		t.Fatalf("Heal convergence timeout; alice_epoch=%d carol_epoch=%d bob_epoch=%d alice_metrics=%+v carol_metrics=%+v",
			alice.coord.CurrentEpoch(), carol.coord.CurrentEpoch(), bob.coord.CurrentEpoch(),
			alice.coord.GetMetrics(), carol.coord.GetMetrics())
	}

	// Verify Bob's storage to check replayed messages.
	// Since Carol has not healed or replayed her messages yet, Bob should only have
	// received Alice's replayed messages. Alice must NOT have replayed Carol's messages.
	bobMsgs := bob.storage.Messages()
	aliceReplayCount := 0
	carolReplayCount := 0

	for _, msg := range bobMsgs {
		contentStr := string(msg.Content)
		if contentStr == "msg-from-alice" {
			aliceReplayCount++
		}
		if contentStr == "msg-from-carol" {
			carolReplayCount++
		}
	}

	if aliceReplayCount != 1 {
		t.Errorf("expected Bob to receive exactly 1 replayed message from Alice, got %d", aliceReplayCount)
	}
	if carolReplayCount != 1 {
		t.Errorf("expected Bob to receive exactly 1 replayed message from Carol (Carol replays her own), got %d", carolReplayCount)
	}
}

// TestIntegration_Replay_IdempotencyDeduplication ensures that duplicate replayed envelopes
// are processed idempotently and applied exactly once.
func TestIntegration_Replay_IdempotencyDeduplication(t *testing.T) {
	groupID := "grp-idempotency"
	nodes, network, _ := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// Alice sends a message, but we partition Bob so he doesn't receive it normally
	network.Partition([]peer.ID{alice.id}, []peer.ID{bob.id})
	if _, err := alice.coord.SendMessage([]byte("idempotency-test-msg")); err != nil {
		t.Fatalf("Alice SendMessage: %v", err)
	}
	network.DrainAll()

	// Get the raw envelope from Alice's offline envelope log
	recs, err := alice.storage.GetEnvelopesSince(groupID, 0, 10)
	if err != nil {
		t.Fatalf("GetEnvelopesSince: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("expected at least one envelope logged on Alice")
	}

	// Retrieve the Application Envelope
	var appEnvBytes []byte
	for _, rec := range recs {
		if rec.MsgType == MsgApplication {
			appEnvBytes = rec.Envelope
			break
		}
	}
	if len(appEnvBytes) == 0 {
		t.Fatal("could not find Application envelope in Alice's log")
	}

	// Heal the network and call ReplayEnvelopes on Bob with the same envelope TWICE in a single batch
	network.Heal()
	applied, err := bob.coord.ReplayEnvelopes([][]byte{appEnvBytes, appEnvBytes})
	if err != nil {
		t.Fatalf("ReplayEnvelopes failed: %v", err)
	}

	// Bob should apply the first envelope but deduplicate the second (applied must equal 1)
	if applied != 1 {
		t.Errorf("expected applied count = 1 (idempotent), got %d", applied)
	}

	// Sequentially call ReplayEnvelopes again with the same envelope
	appliedSecond, err := bob.coord.ReplayEnvelopes([][]byte{appEnvBytes})
	if err != nil {
		t.Fatalf("Second ReplayEnvelopes failed: %v", err)
	}
	if appliedSecond != 0 {
		t.Errorf("expected sequential duplicate to apply 0 envelopes, got %d", appliedSecond)
	}

	// Verify Bob's storage has exactly one stored message for this payload
	bobMsgs := bob.storage.Messages()
	matchCount := 0
	for _, m := range bobMsgs {
		if string(m.Content) == "idempotency-test-msg" {
			matchCount++
		}
	}
	if matchCount != 1 {
		t.Errorf("expected Bob to store exactly 1 message, got %d", matchCount)
	}
}

// TestIntegration_Replay_OrderPreservation verifies that when a node replays multiple messages,
// they are re-encrypted and broadcast in the exact relative chronological HLC order.
func TestIntegration_Replay_OrderPreservation(t *testing.T) {
	groupID := "grp-order"
	nodes, network, clk := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// Diverge branch markers for detection.
	alice.coord.mu.Lock()
	alice.coord.treeHash = []byte("loser-tree")
	alice.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("loser-tree"),
		MemberCount: 1,
		Epoch:       0,
	})
	alice.coord.mu.Unlock()

	bob.coord.mu.Lock()
	bob.coord.treeHash = []byte("winner-tree")
	bob.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	bob.coord.mu.Unlock()

	partitionStart := clk.Now().Add(1 * time.Second)
	clk.Set(partitionStart)

	alice.coord.mu.Lock()
	alice.coord.forkDetector.ProcessRemote(partitionStart, bob.id, bob.coord.CurrentEpoch(), GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	alice.coord.mu.Unlock()

	// Partition network
	network.Partition([]peer.ID{alice.id}, []peer.ID{bob.id})

	// Alice sends first-msg, then second-msg.
	// HLC naturally increments logical counter or physical time.
	if _, err := alice.coord.SendMessage([]byte("order-msg-1")); err != nil {
		t.Fatalf("SendMessage #1: %v", err)
	}
	if _, err := alice.coord.SendMessage([]byte("order-msg-2")); err != nil {
		t.Fatalf("SendMessage #2: %v", err)
	}
	network.DrainAll()

	// Heal partition
	network.Heal()

	alice.coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, _ string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
		if remote != bob.id {
			return nil, errors.New("wrong remote")
		}
		groupInfo, err := bob.mls.ExportGroupInfo(ctx, bob.coord.GetGroupState(), withRatchetTree)
		if err != nil {
			return nil, err
		}
		return &GroupInfoFetchResult{
			GroupInfo: groupInfo,
			Epoch:     bob.coord.CurrentEpoch(),
			TreeHash:  bob.coord.GetTreeHash(),
		}, nil
	}

	// Trigger heal
	bob.coord.mu.Lock()
	bob.coord.broadcastAnnounceLocked()
	bob.coord.mu.Unlock()
	network.DrainAll()

	if !waitFor(t, 5*time.Second, func() bool {
		network.DrainAll()
		snap := alice.coord.GetMetrics()
		return snap.ForkHealingsSucceeded >= 1
	}) {
		t.Fatalf("Heal failed to succeed")
	}

	// Verify Bob received replayed messages in chronological order.
	bobMsgs := bob.storage.Messages()
	var orderedPayloads []string
	var orderedTimestamps []HLCTimestamp

	for _, msg := range bobMsgs {
		contentStr := string(msg.Content)
		if contentStr == "order-msg-1" || contentStr == "order-msg-2" {
			orderedPayloads = append(orderedPayloads, contentStr)
			orderedTimestamps = append(orderedTimestamps, msg.Timestamp)
		}
	}

	if len(orderedPayloads) != 2 {
		t.Fatalf("expected Bob to receive exactly 2 replayed messages, got %d", len(orderedPayloads))
	}

	// Check content order
	if orderedPayloads[0] != "order-msg-1" || orderedPayloads[1] != "order-msg-2" {
		t.Errorf("Causal order violated! Expected [order-msg-1, order-msg-2], got %+v", orderedPayloads)
	}

	// Check logical clock (HLC) order
	if !orderedTimestamps[0].Before(orderedTimestamps[1]) {
		t.Errorf("HLC logical ordering violated! Timestamp 1 (%+v) should be before Timestamp 2 (%+v)",
			orderedTimestamps[0], orderedTimestamps[1])
	}

	// Check Alice marked both original messages as replayed
	aliceMsgs, err := alice.storage.GetMessagesSince(groupID, HLCTimestamp{})
	if err != nil {
		t.Fatalf("Alice GetMessagesSince: %v", err)
	}
	replayedCount := 0
	for _, m := range aliceMsgs {
		if m.ReplayedAt != nil {
			replayedCount++
		}
	}
	if replayedCount != 2 {
		t.Errorf("expected Alice to mark 2 messages as replayed, got %d", replayedCount)
	}
}

// TestIntegration_Heal_AuditStateSwapFailure verifies that when an error occurs during the latter
// steps of healing (e.g. SaveGroupRecord fails inside applyHealedState), the transaction fails gracefully,
// IsHealing flag is cleared, and the exact step failure is persisted to audit database.
func TestIntegration_Heal_AuditStateSwapFailure(t *testing.T) {
	groupID := "grp-swap-fail"
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	aliceID := peerID("alice")
	bobID := peerID("bob")

	// Alice uses custom failingStorage wrapping MockStorage
	aliceMockStorage := NewMockStorage()
	aliceStorage := &failingStorage{CoordinationStorage: aliceMockStorage}
	aliceMLS := NewMockMLSEngine()

	aliceTransport := network.AddNode(aliceID)
	aliceCoord, err := NewCoordinator(CoordinatorOpts{
		Config:    TestConfig(),
		Transport: aliceTransport,
		Clock:     clk,
		MLS:       aliceMLS,
		Storage:   aliceStorage,
		LocalID:   aliceID,
		GroupID:   groupID,
	})
	if err != nil {
		t.Fatalf("NewCoordinator Alice: %v", err)
	}

	bobStorage := NewMockStorage()
	bobMLS := NewMockMLSEngine()
	bobTransport := network.AddNode(bobID)
	bobCoord, err := NewCoordinator(CoordinatorOpts{
		Config:    TestConfig(),
		Transport: bobTransport,
		Clock:     clk,
		MLS:       bobMLS,
		Storage:   bobStorage,
		LocalID:   bobID,
		GroupID:   groupID,
	})
	if err != nil {
		t.Fatalf("NewCoordinator Bob: %v", err)
	}

	nodes := []*testNode{
		{id: aliceID, coord: aliceCoord, mls: aliceMLS, storage: aliceMockStorage},
		{id: bobID, coord: bobCoord, mls: bobMLS, storage: bobStorage},
	}

	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	// Diverge branch markers for detection.
	aliceCoord.mu.Lock()
	aliceCoord.treeHash = []byte("loser-tree")
	aliceMockStorage.groups[groupID].TreeHash = []byte("loser-tree") // reflect in Alice storage
	aliceCoord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("loser-tree"),
		MemberCount: 1,
		Epoch:       0,
	})
	aliceCoord.mu.Unlock()

	bobCoord.mu.Lock()
	bobCoord.treeHash = []byte("winner-tree")
	bobCoord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	bobCoord.mu.Unlock()

	// Connect Bob GroupInfo fetcher
	aliceCoord.groupInfoFetch = func(ctx context.Context, remote peer.ID, _ string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
		if remote != bobID {
			return nil, errors.New("wrong remote")
		}
		groupInfo, err := bobMLS.ExportGroupInfo(ctx, bobCoord.GetGroupState(), withRatchetTree)
		if err != nil {
			return nil, err
		}
		return &GroupInfoFetchResult{
			GroupInfo: groupInfo,
			Epoch:     bobCoord.CurrentEpoch(),
			TreeHash:  bobCoord.GetTreeHash(),
		}, nil
	}

	// Trigger SaveGroupRecord failure on Alice storage to crash applyHealedState (State Swap)
	aliceStorage.failSave = true

	// Hook Bob's onAddCommitted to forward Welcome to Alice so Alice's runHeal can
	// reach the state_swap step (where failSave will cause the expected failure).
	nodesMap := map[string]*Coordinator{aliceID.String(): aliceCoord}
	bobCoord.onAddCommitted = func(delivery AddCommitDelivery, commitEpoch uint64, welcome []byte) {
		targetCoord, ok := nodesMap[delivery.TargetPeerID]
		if !ok {
			return
		}
		welcomePayload := welcome
		var dummy mockGroupState
		if len(welcomePayload) == 0 || json.Unmarshal(welcomePayload, &dummy) != nil {
			winnerState := mockGroupState{
				Epoch:    commitEpoch,
				TreeHash: hex.EncodeToString(mockTreeHash(commitEpoch)),
				Members:  make(map[string]bool),
			}
			welcomePayload, _ = json.Marshal(winnerState)
		}
		go targetCoord.ProcessWelcomeIfWaiting(context.Background(), welcomePayload)
	}

	// Alice observes Bob announcement, triggers heal
	partitionStart := clk.Now().Add(1 * time.Second)
	clk.Set(partitionStart)
	aliceCoord.mu.Lock()
	aliceCoord.forkDetector.ProcessRemote(partitionStart, bobID, bobCoord.CurrentEpoch(), GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	aliceCoord.mu.Unlock()

	// Bob announces his branch to trigger the ProcessRemote callback
	bobCoord.mu.Lock()
	bobCoord.broadcastAnnounceLocked()
	bobCoord.mu.Unlock()
	network.DrainAll()

	// Wait for healing attempt to execute and terminate
	if !waitFor(t, time.Second, func() bool {
		network.DrainAll()
		return !aliceCoord.IsHealing() && aliceCoord.GetMetrics().ForkHealingsAttempted >= 1
	}) {
		t.Fatalf("expected failed heal attempt to finish; metrics=%+v", aliceCoord.GetMetrics())
	}

	// Check Alice's metrics: should record attempt but 0 successes
	snap := aliceCoord.GetMetrics()
	if snap.ForkHealingsAttempted != 1 {
		t.Errorf("expected 1 heal attempt, got %d", snap.ForkHealingsAttempted)
	}
	if snap.ForkHealingsSucceeded != 0 {
		t.Errorf("expected 0 heal successes, got %d", snap.ForkHealingsSucceeded)
	}

	// Verify persisted failed heal event is captured in SQLite
	events, err := aliceMockStorage.ListForkHealEvents(groupID, 10)
	if err != nil {
		t.Fatalf("ListForkHealEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one persisted failed heal event record")
	}

	ev := events[0]
	if ev.Outcome != "failed" {
		t.Errorf("expected outcome = failed, got %q", ev.Outcome)
	}
	if ev.FailedStep != "state_swap" {
		t.Errorf("expected FailedStep = 'state_swap', got %q", ev.FailedStep)
	}

	// Check audit logs for the step traces
	audit, err := aliceMockStorage.ListForkHealAudit(ev.TraceID)
	if err != nil {
		t.Fatalf("ListForkHealAudit: %v", err)
	}
	if len(audit) == 0 {
		t.Fatal("expected at least one audit record persisted")
	}

	// Check that we recorded a failed state_swap step
	foundFailStep := false
	for _, a := range audit {
		if a.Step == "state_swap" && a.Status == "failed" {
			foundFailStep = true
			if !strings.Contains(a.Error, "mock db save failure") {
				t.Errorf("expected audit error to contain database failure, got %q", a.Error)
			}
		}
	}
	if !foundFailStep {
		t.Error("expected state_swap step to be marked as failed in audit trail")
	}
}
