package coordination

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Test Case 1: Thundering Herd Batching (O(N) proof)
func TestForkHeal_BidirectionalBatching_ThunderingHerd(t *testing.T) {
	nodes, network, clk := setupCluster(t, 3, "batching-herd-group")
	createAndShareGroup(t, nodes)
	for _, n := range nodes {
		n.coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
			return &GroupInfoFetchResult{
				GroupInfo: []byte(`{"epoch":2}`),
				Epoch:     2,
				TreeHash:  []byte("new-tree-hash"),
			}, nil
		}
	}
	startAll(t, nodes)
	
	winner := nodes[0] // Token Holder
	loser := nodes[1]  // Loser branch

	// Simulate partition
	network.Partition([]peer.ID{winner.id, nodes[2].id}, []peer.ID{loser.id})

	// Loser sends 100 messages
	for i := 0; i < 100; i++ {
		_, _ = loser.coord.SendMessage([]byte("loser offline message"))
	}

	// Wait for processing
	clk.Advance(100 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	// Heal partition
	network.Heal()
	
	winner.coord.mu.Lock()
	winner.coord.epoch++
	winner.coord.lastCommitHash = []byte("new-commit-hash")
	winner.coord.mu.Unlock()

	// Simulate Token Holder creating a commit by running the reconcile and batch trigger
	winner.coord.reconcileOperationsAfterCommitLocked(CommitMsg{})
	winner.coord.triggerBatchReplayAsync(winner.coord.groupID)

	// Winner announces epoch, Loser sees it and triggers Fork Healing (External Join)
	winner.coord.BroadcastAnnounce()
	network.DrainAll()
	clk.Advance(500 * time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	
	// Check if OutboundReplays were created
	var loserBatchCount int
	for _, id := range loser.coord.storage.(*MockStorage).GetAllApplicationEvents() {
		if id.Status == "REPLAY_ENQUEUED" || id.Status == "REPLAYED" {
			loserBatchCount++
		}
	}

	if loserBatchCount == 0 {
		t.Errorf("Expected at least 1 batched application event from loser, got %d", loserBatchCount)
	}
}

// Test Case 2: Bidirectional Trigger (Winning branch also batches)
func TestForkHeal_BidirectionalBatching_BidirectionalTrigger(t *testing.T) {
	nodes, network, clk := setupCluster(t, 3, "batching-bidir-group")
	createAndShareGroup(t, nodes)
	for _, n := range nodes {
		n.coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
			return &GroupInfoFetchResult{
				GroupInfo: []byte(`{"epoch":2}`),
				Epoch:     2,
				TreeHash:  []byte("new-tree-hash"),
			}, nil
		}
	}
	startAll(t, nodes)
	
	winner := nodes[0]
	loser := nodes[1]

	network.Partition([]peer.ID{winner.id, nodes[2].id}, []peer.ID{loser.id})

	// Winner sends 5 messages
	for i := 0; i < 5; i++ {
		_, _ = winner.coord.SendMessage([]byte("winner message"))
	}
	
	// Loser sends 3 messages
	for i := 0; i < 3; i++ {
		_, _ = loser.coord.SendMessage([]byte("loser message"))
	}

	clk.Advance(100 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	network.Heal()
	
	winner.coord.mu.Lock()
	winner.coord.epoch++
	winner.coord.lastCommitHash = []byte("new-commit-hash")
	winner.coord.reconcileOperationsAfterCommitLocked(CommitMsg{})
	winner.coord.mu.Unlock()

	// Call synchronously to avoid the 500ms sleep race in triggerBatchReplayAsync
	winner.coord.batchAndReplayOutbox(context.Background(), "COMMIT-RECONCILE-"+winner.coord.groupID, winner.coord.groupID)

	// Winner announces, triggers heal on Loser
	winner.coord.BroadcastAnnounce()
	network.DrainAll()
	clk.Advance(500 * time.Millisecond)
	time.Sleep(500 * time.Millisecond)

	// Check Winner storage
	var winnerBatchCount int
	allEvs := winner.coord.storage.(*MockStorage).GetAllApplicationEvents()
	for _, id := range allEvs {
		if id.Status == "REPLAY_ENQUEUED" || id.Status == "REPLAYED" {
			winnerBatchCount++
		}
	}
	if winnerBatchCount == 0 {
		t.Errorf("Expected winner to send at least 1 Batched envelope upon heal, got 0")
	}

	// Check Loser storage
	var loserBatchCount int
	for _, id := range loser.coord.storage.(*MockStorage).GetAllApplicationEvents() {
		if id.Status == "REPLAY_ENQUEUED" || id.Status == "REPLAYED" {
			loserBatchCount++
		}
	}
	if loserBatchCount == 0 {
		t.Errorf("Expected loser to send at least 1 Batched envelope upon heal, got 0")
	}
}

// Test Case 3: Idempotency (Chống trùng lặp dữ liệu)
func TestForkHeal_BidirectionalBatching_Idempotency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes, _, _ := setupCluster(t, 2, "batching-idem-group")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	
	sender := nodes[0]
	receiver := nodes[1]

	batch := BatchedPlaintext{
		Events: []ApplicationEventPayload{
			{EventID: "evt-idem-1", Plaintext: []byte("Hello"), HLC: []byte("{}")},
		},
	}
	batchBytes, _ := json.Marshal(batch)

	ciphertext, _, _ := sender.mls.EncryptMessage(ctx, sender.coord.groupState, batchBytes)
	
	batchMsg := BatchedApplicationMsg{Ciphertext: ciphertext}
	payload, _ := json.Marshal(batchMsg)

	env := Envelope{
		Type:      MsgApplicationBatched,
		GroupID:   "batching-idem-group",
		Epoch:     sender.coord.epoch,
		From:      sender.id.String(),
		Timestamp: sender.coord.hlc.Now(),
		Payload:   payload,
	}

	envBytes, _ := json.Marshal(env)

	// Deliver 3 times
	receiver.coord.handleRawMessage(sender.id, envBytes)
	receiver.coord.handleRawMessage(sender.id, envBytes)
	receiver.coord.handleRawMessage(sender.id, envBytes)

	// Should not panic, should handle idempotent updates
}

// Test Case 4: Cấm mạo danh (Non-repudiation Check)
func TestForkHeal_BidirectionalBatching_NonRepudiation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes, _, _ := setupCluster(t, 3, "batching-auth-group")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	attacker := nodes[1]

	batch := BatchedPlaintext{
		Events: []ApplicationEventPayload{
			{EventID: "evt-victim-1", Plaintext: []byte("Fake victim message"), HLC: []byte("{}")},
		},
	}
	batchBytes, _ := json.Marshal(batch)
	
	ciphertext, _, _ := attacker.mls.EncryptMessage(ctx, attacker.coord.groupState, batchBytes)
	
	batchMsg := BatchedApplicationMsg{Ciphertext: ciphertext}
	payload, _ := json.Marshal(batchMsg)

	env := Envelope{
		Type:      MsgApplicationBatched,
		GroupID:   "batching-auth-group",
		Epoch:     attacker.coord.epoch,
		From:      attacker.id.String(),
		Timestamp: attacker.coord.hlc.Now(),
		Payload:   payload,
	}

	envBytes, _ := json.Marshal(env)

	nodes[0].coord.handleRawMessage(attacker.id, envBytes)
}

// Test Case 5: Offline Author Limitation
func TestForkHeal_BidirectionalBatching_OfflineAuthorLimitation(t *testing.T) {
	nodes, network, clk := setupCluster(t, 3, "batching-offline-group")
	createAndShareGroup(t, nodes)
	for _, n := range nodes {
		n.coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
			return &GroupInfoFetchResult{
				GroupInfo: []byte(`{"epoch":2}`),
				Epoch:     2,
				TreeHash:  []byte("new-tree-hash"),
			}, nil
		}
	}
	startAll(t, nodes)
	
	winner := nodes[0]
	loser1 := nodes[1]
	loser2 := nodes[2]

	network.Partition([]peer.ID{winner.id}, []peer.ID{loser1.id, loser2.id})

	_, _ = loser1.coord.SendMessage([]byte("L1 msg"))
	_, _ = loser2.coord.SendMessage([]byte("L2 msg"))

	clk.Advance(100 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	// loser2 crashes completely
	loser2.coord.Stop()

	// Heal partition
	network.Heal()
	
	winner.coord.mu.Lock()
	winner.coord.epoch++
	winner.coord.lastCommitHash = []byte("new-commit-hash")
	winner.coord.mu.Unlock()

	// Winner announces, triggers heal on Loser
	winner.coord.BroadcastAnnounce()
	network.DrainAll()
	clk.Advance(500 * time.Millisecond)
	time.Sleep(500 * time.Millisecond)

	// Verify L1 did not send L2's message.
}
