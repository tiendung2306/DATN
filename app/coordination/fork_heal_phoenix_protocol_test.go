package coordination

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// TestForkHeal_PhoenixProtocol_HappyPath validates the complete flow:
// 1. Partition occurs.
// 2. Winner branch advances, Loser branch queues application messages.
// 3. Partition heals. Loser detects fork, runs runHeal, drops group state,
//    broadcasts JoinProposal and suspends at PROPOSAL_SENT caching private keys.
// 4. Token Holder (Winner) intercepts JoinProposal, automatically inserts
//    zombie leaf RemoveProposal and transmutes to AddProposal.
// 5. Token Holder commits, generating WelcomeBytes.
// 6. Loser receives WelcomeBytes via ProcessWelcomeIfWaiting, restores group state,
//    swaps state, resumes job, and performs Autonomous Replay of own messages.
func TestForkHeal_PhoenixProtocol_HappyPath(t *testing.T) {
	nodes, network, clk := setupCluster(t, 2, "phoenix-happy-path")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)

	winner := nodes[0] // Token Holder / Winner node
	loser := nodes[1]  // Loser node

	// Setup longer timeout to prevent premature FakeClock timeout triggers
	winner.coord.cfg.MLSOperationTimeout = 5 * time.Second
	loser.coord.cfg.MLSOperationTimeout = 5 * time.Second

	// Setup loser's mock groupInfoFetch to prevent hanging, although under Phoenix
	// losing branch doesn't strictly need GroupInfo to generate its KeyPackage.
	loser.coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
		return &GroupInfoFetchResult{
			GroupInfo: []byte(`{"epoch":2,"TreeHash":"` + hex.EncodeToString(mockTreeHash(2)) + `"}`),
			Epoch:     2,
			TreeHash:  mockTreeHash(2),
		}, nil
	}

	// Setup onAddCommitted on winner to automatically forward the generated Welcome
	// to the loser node, mimicking the application layer direct stream delivery.
	// We mock a structurally valid Welcome payload for MockMLSEngine containing the correct commitEpoch.
	winner.coord.onAddCommitted = func(delivery AddCommitDelivery, commitEpoch uint64, welcome []byte) {
		winnerState := mockGroupState{
			Epoch:    commitEpoch,
			TreeHash: hex.EncodeToString(mockTreeHash(commitEpoch)),
			Members:  map[string]bool{winner.id.String(): true, loser.id.String(): true},
		}
		validWelcome, _ := json.Marshal(winnerState)
		go func() {
			loser.coord.ProcessWelcomeIfWaiting(context.Background(), validWelcome)
		}()
	}

	// 1. Partition winner and loser
	network.Partition([]peer.ID{winner.id}, []peer.ID{loser.id})

	// 2. Winner branch advances (simulate commit)
	winner.coord.mu.Lock()
	winner.coord.epoch++
	winner.coord.lastCommitHash = []byte("new-winning-commit-hash")
	newWinnerTH := mockTreeHash(winner.coord.epoch)
	winner.coord.treeHash = newWinnerTH
	// Diverge history hash so same-epoch fork detection fires.
	winner.coord.historyHash = []byte("winner-advanced-hist")
	if winner.coord.historyChain == nil {
		winner.coord.historyChain = make(map[uint64][]byte)
	}
	winner.coord.historyChain[winner.coord.epoch] = []byte("winner-advanced-hist")
	winner.coord.epochTracker = NewEpochTracker(winner.coord.epoch, newWinnerTH)
	winner.coord.mu.Unlock()

	// Loser stays at same epoch but with different HistoryHash to simulate
	// a fork. Set a high CommitHash so the winner wins the tiebreaker.
	loser.coord.mu.Lock()
	loser.coord.epoch = winner.coord.epoch
	loser.coord.historyHash = []byte("loser-stale-hist")
	loser.coord.lastCommitHash = []byte("zzz-loser-stale-commit")
	if loser.coord.historyChain == nil {
		loser.coord.historyChain = make(map[uint64][]byte)
	}
	loser.coord.historyChain[loser.coord.epoch] = []byte("loser-stale-hist")
	loser.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    loser.coord.treeHash,
		MemberCount: 1,
		Epoch:       loser.coord.epoch,
		CommitHash:  loser.coord.lastCommitHash,
		HistoryHash: []byte("loser-stale-hist"),
	})
	loser.coord.mu.Unlock()

	// 3. Loser branch sends message to local outbox during partition
	_, err := loser.coord.SendMessage([]byte("loser message during partition"))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	clk.Advance(100 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	// 4. Heal network partition
	network.Heal()

	// 5. Trigger fork detection via announcement
	winner.coord.BroadcastAnnounce()
	network.DrainAll()

	// Give the runHeal routine time to execute, generate KeyPackage, and broadcast JoinProposal
	clk.Advance(100 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	network.DrainAll() // Delivers JoinProposal to winner

	// Verify loser is suspended at PROPOSAL_SENT and cached the bundle before commit finishes
	job, err := loser.coord.storage.GetActiveForkHealingJob(loser.coord.groupID)
	if err != nil {
		t.Fatalf("GetActiveForkHealingJob failed: %v", err)
	}
	if job == nil {
		t.Fatalf("Active fork healing job not found on loser")
	}
	if job.Status != "PROPOSAL_SENT" {
		t.Errorf("Expected job status 'PROPOSAL_SENT', got '%s'", job.Status)
	}
	if len(job.PendingBundlePrivate) == 0 {
		t.Errorf("Expected PendingBundlePrivate to be cached, got empty")
	}
	jobID := job.JobID

	// Check that loser's local memory GroupState has been dropped
	loser.coord.mu.Lock()
	if loser.coord.groupState != nil {
		t.Errorf("Expected loser groupState to be nil/dropped, but it is still in memory")
	}
	loser.coord.mu.Unlock()

	// Give winner time to schedule and execute the batch commit, generating the Welcome,
	// which automatically invokes ProcessWelcomeIfWaiting on loser.
	clk.Advance(200 * time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	network.DrainAll() // Deliver commit and flush events

	// Verify loser completed the healing job successfully
	finalJob, err := loser.coord.storage.(*MockStorage).GetForkHealingJobByID(jobID)
	if err != nil {
		t.Fatalf("GetForkHealingJobByID failed: %v", err)
	}
	if finalJob == nil {
		t.Fatalf("Fork healing job %s not found on loser after completion", jobID)
	}
	if finalJob.Status != "CLEANED" {
		t.Errorf("Expected job status 'CLEANED', got '%s'", finalJob.Status)
	}

	// Check that loser's local memory GroupState has been restored and swapped to winner's new epoch (Epoch 2)
	loser.coord.mu.Lock()
	if loser.coord.groupState == nil {
		t.Errorf("Expected loser groupState to be restored in memory, got nil")
	}
	loser.coord.mu.Unlock()

	if loser.coord.CurrentEpoch() != 2 {
		t.Errorf("Expected loser epoch 2, got %d", loser.coord.CurrentEpoch())
	}

	// Verify Autonomous Replay: loser replayed its own messages
	var replayedCount int
	for _, ev := range loser.coord.storage.(*MockStorage).GetAllApplicationEvents() {
		if ev.Status == "REPLAYED" && ev.AuthorID == loser.id.String() {
			replayedCount++
		}
	}
	if replayedCount != 1 {
		t.Errorf("Expected 1 replayed message from loser, got %d", replayedCount)
	}
}

// TestForkHeal_PhoenixProtocol_ProcessWelcomeErrors validates error scenarios during ProcessWelcome.
func TestForkHeal_PhoenixProtocol_ProcessWelcomeErrors(t *testing.T) {
	nodes, _, _ := setupCluster(t, 2, "phoenix-errors")
	loser := nodes[1]

	// 1. Receive Welcome when NO active job exists
	processed := loser.coord.ProcessWelcomeIfWaiting(context.Background(), []byte("bad-welcome"))
	if processed {
		t.Errorf("Expected false when no active job is running")
	}

	// Create an active job in INITIATED state (not PROPOSAL_SENT)
	job := &ForkHealingJob{
		JobID:        "test-job-errors",
		GroupID:      loser.coord.groupID,
		Status:       "INITIATED",
		WinnerPeerID: nodes[0].id.String(),
	}
	_ = loser.coord.storage.SaveForkHealingJob(job)

	// 2. Receive Welcome when job status is NOT PROPOSAL_SENT
	processed = loser.coord.ProcessWelcomeIfWaiting(context.Background(), []byte("bad-welcome"))
	if processed {
		t.Errorf("Expected false when job status is INITIATED")
	}

	// Advance status to PROPOSAL_SENT, but keep PendingBundlePrivate empty
	job.Status = "PROPOSAL_SENT"
	_ = loser.coord.storage.SaveForkHealingJob(job)

	// 3. Receive Welcome when PendingBundlePrivate is empty
	processed = loser.coord.ProcessWelcomeIfWaiting(context.Background(), []byte("bad-welcome"))
	if processed {
		t.Errorf("Expected false when PendingBundlePrivate is empty")
	}

	// Set private bundle key package
	job.PendingBundlePrivate = []byte("mock-private-key")
	_ = loser.coord.storage.SaveForkHealingJob(job)

	// 4. Corrupt Welcome bytes causing ProcessWelcome decoding failure
	// We trigger failure by having the Mock MLS engine return error on next call
	loser.coord.mls.(*MockMLSEngine).SetNextError(fmt.Errorf("crypto decryption error"))
	processed = loser.coord.ProcessWelcomeIfWaiting(context.Background(), []byte("invalid-hex-or-json"))
	if processed {
		t.Errorf("Expected false when ProcessWelcome returns error")
	}

	// 5. Idempotency: Process welcome on a job that is already CLEANED
	job.Status = "CLEANED"
	_ = loser.coord.storage.SaveForkHealingJob(job)
	processed = loser.coord.ProcessWelcomeIfWaiting(context.Background(), []byte("welcome"))
	if processed {
		t.Errorf("Expected false for already completed CLEANED jobs")
	}
}
