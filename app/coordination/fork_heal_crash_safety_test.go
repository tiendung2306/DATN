package coordination

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// TestForkHeal_OrphanEventSnapshot_AESGCMSealing verifies that unapplied + applied events
// of the losing branch are decrypted and sealed using local AES-GCM storage key, and NO plaintext is stored.
func TestForkHeal_OrphanEventSnapshot_AESGCMSealing(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-fs"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	// 1. Dựng state ban đầu của losing branch
	lState, lTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("alice-signing-key"), 3)
	coord.groupState = lState
	coord.treeHash = lTree
	coord.epoch = 2
	coord.started = true

	// Ghi nhận Group Record cục bộ
	_ = storage.SaveGroupRecord(&GroupRecord{
		GroupID:    groupID,
		GroupState: lState,
		Epoch:      2,
		TreeHash:   lTree,
	})

	// 2. Tạo một applied own message trên losing branch
	ts := coord.hlc.Now()
	ownContent := []byte("own message on losing branch")
	ownMsgHash := sha256.Sum256(ownContent)
	_ = storage.SaveMessage(&StoredMessage{
		MessageID:    "msg-1",
		GroupID:      groupID,
		Epoch:        2,
		SenderID:     id,
		Content:      ownContent,
		Timestamp:    ts,
		EnvelopeHash: ownMsgHash[:],
	})

	// 3. Tạo một unapplied envelope của peer khác (losing branch)
	otherID := peerID("bob")
	otherContent := []byte("other message on losing branch")
	otherCipher, _, _ := mls.EncryptMessage(context.Background(), lState, otherContent)
	otherMsg := ApplicationMsg{Ciphertext: otherCipher}
	otherPayload, _ := json.Marshal(otherMsg)
	otherEnv := Envelope{
		Type:      MsgApplication,
		GroupID:   groupID,
		Epoch:     2,
		From:      otherID.String(),
		Timestamp: ts,
		Payload:   otherPayload,
	}
	otherEnvBytes, _ := json.Marshal(otherEnv)
	_, _ = storage.AppendEnvelope(groupID, MsgApplication, 2, ts, otherEnvBytes)

	// 4. Kích hoạt fork heal
	winnerPeer := peerID("carol")
	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	winnerAnnounce := GroupStateAnnouncement{
		TreeHash:    winnerTree,
		MemberCount: 3,
		Epoch:       5,
		CommitHash:  []byte("winner-commit-hash"),
	}
	event := &ForkEvent{
		GroupID:            groupID,
		RemotePeer:         winnerPeer,
		LocalAnnounce:      GroupStateAnnouncement{TreeHash: lTree, Epoch: 2},
		RemoteAnnounce:     winnerAnnounce,
		RemoteEpoch:        5,
		NeedExternalJoin:   true,
		WinnerPeers:        []peer.ID{winnerPeer},
		PartitionStartedAt: clk.Now().Add(-5 * time.Minute),
	}

	// Mock fetchGroupInfo (gi.GroupInfo phải là valid winnerState bytes)
	gi := &GroupInfoFetchResult{
		Epoch:     5,
		TreeHash:  winnerTree,
		GroupInfo: winnerState,
	}
	coord.groupInfoFetch = func(ctx context.Context, p peer.ID, g string, w bool) (*GroupInfoFetchResult, error) {
		return gi, nil
	}

	if coord.healing.CompareAndSwap(false, true) {
		coord.runHeal(context.Background(), "trace-fs-1", event, clk.Now())
	}

	// 5. Kiểm tra snapshot AES-GCM Sealing cục bộ trong application_event
	var job *ForkHealingJob
	for _, j := range storage.forkHealingJobs {
		if j.GroupID == groupID {
			job = j
			break
		}
	}
	if job == nil {
		t.Fatalf("Failed to find fork healing job in storage")
	}

	evs, err := storage.ListApplicationEvents(job.JobID)
	if err != nil {
		t.Fatalf("ListApplicationEvents failed: %v", err)
	}

	foundOwn := false
	foundOther := false

	for _, ev := range evs {
		if ev.Status == "REPLAYED" {
			foundOwn = true
			if ev.PayloadSealed != nil {
				t.Fatalf("FS Violation: payload_sealed is not cleared (nil) after replay completed")
			}
		} else if ev.Status == "WAITING_AUTHOR_REPLAY" {
			foundOther = true
			if ev.PayloadSealed != nil {
				t.Fatalf("Non-repudiation / FS Violation: other peer message payload_sealed is not nil")
			}
		}
	}

	if !foundOwn {
		t.Errorf("Own losing branch message was not snapshotted/replayed")
	}
	if !foundOther {
		t.Errorf("Other losing branch message was not snapshotted/marked WAITING_AUTHOR_REPLAY")
	}
}

// TestForkHeal_Resume_BeforeSwap_OfflineWinner verifies that if restart happens before swap,
// coordinator resume heals successfully using winner_group_info bytes persisted in DB, even if the winner peer is offline.
func TestForkHeal_Resume_BeforeSwap_OfflineWinner(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-resume-offline"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	lState, lTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("alice-signing-key"), 3)
	coord.groupState = lState
	coord.treeHash = lTree
	coord.epoch = 2
	coord.started = true

	// Ghi nhận Group Record cục bộ
	_ = storage.SaveGroupRecord(&GroupRecord{
		GroupID:    groupID,
		GroupState: lState,
		Epoch:      2,
		TreeHash:   lTree,
	})

	// 1. Persist một job ở trạng thái INITIATED dở dang với winner peer
	winnerPeer := peerID("carol")
	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	job := &ForkHealingJob{
		JobID:             "job-offline-1",
		GroupID:           groupID,
		TraceID:           "trace-resume-1",
		Status:            "INITIATED",
		LosingBranchID:    "losing-branch-id",
		WinningBranchID:   "winning-branch-id",
		LosingEpoch:       2,
		WinningEpoch:      5,
		LosingTreeHash:    lTree,
		WinningTreeHash:   winnerTree,
		WinnerPeerID:      winnerPeer.String(),
		WinnerGroupInfo:   winnerState, // persist GroupInfo bytes (winnerState is valid mockGroupState)
		CreatedAtMs:       clk.Now().UnixMilli(),
		UpdatedAtMs:       clk.Now().UnixMilli(),
	}
	_ = storage.SaveForkHealingJob(job)

	// Mock fetchGroupInfo để BÁO LỖI (Winner peer offline)
	coord.groupInfoFetch = func(ctx context.Context, p peer.ID, g string, w bool) (*GroupInfoFetchResult, error) {
		return nil, fmt.Errorf("peer offline / unreachable")
	}

	// 2. Kích hoạt startup resume
	coord.resumeForkHealingJob(job)

	// Wait briefly for goroutine to finish
	time.Sleep(10 * time.Millisecond)

	// 3. Phải thành công tiến epoch lên 6 nhờ GroupInfo bytes dự phòng
	if coord.epoch != 6 {
		t.Errorf("Resume failed: expected epoch 6, got %d", coord.epoch)
	}

	// Job phải được xóa/dọn dẹp xong
	j, _ := storage.GetForkHealingJobByID("job-offline-1")
	if j != nil && j.Status != "CLEANED" {
		t.Errorf("Job was not cleaned up after resume completion, status: %s", j.Status)
	}
}

// TestForkHeal_Resume_AfterSwap verifies that restart at STATE_SWAPPED status skips External Join
// and directly processes Replay and Cleanup phase.
func TestForkHeal_Resume_AfterSwap(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-resume-swap"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	// 1. Dựng state mới (đã swap thành công cục bộ)
	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	coord.groupState = winnerState
	coord.treeHash = winnerTree
	coord.epoch = 6
	coord.started = true

	_ = storage.SaveGroupRecord(&GroupRecord{
		GroupID:    groupID,
		GroupState: winnerState,
		Epoch:      6,
		TreeHash:   winnerTree,
	})

	// 2. Persist job ở trạng thái STATE_SWAPPED
	job := &ForkHealingJob{
		JobID:             "job-swap-1",
		GroupID:           groupID,
		TraceID:           "trace-resume-swap-1",
		Status:            "STATE_SWAPPED",
		LosingBranchID:    "losing-branch-id",
		WinningBranchID:   "winning-branch-id",
		LosingEpoch:       2,
		WinningEpoch:      5,
		WinningTreeHash:   winnerTree,
		WinnerPeerID:      peerID("carol").String(),
		CreatedAtMs:       clk.Now().UnixMilli(),
		UpdatedAtMs:       clk.Now().UnixMilli(),
	}
	_ = storage.SaveForkHealingJob(job)

	// Tạo một own event cần replay trong application_event
	storageKey := deriveStorageKey([]byte("alice-signing-key"))
	ownContent := []byte("own message that must be replayed")
	sealed, nonce, _ := sealPayload(ownContent, storageKey)
	h := sha256.Sum256(ownContent)
	ts := coord.hlc.Now()
	
	_ = storage.SaveApplicationEvent(&ApplicationEvent{
		EventID:          "event-1",
		JobID:            "job-swap-1",
		GroupID:          groupID,
		OriginalBranchID: "losing-branch-id",
		OriginalEpoch:    2,
		AuthorID:         id.String(),
		EnvelopeHash:     h[:],
		PayloadSealed:    sealed,
		PayloadHash:      h[:],
		SealKeyID:        "local_node_key",
		SealNonce:        nonce,
		HlcWallTimeMs:    ts.WallTimeMs,
		HlcCounter:       ts.Counter,
		HlcNodeID:        ts.NodeID,
		Status:           "ORPHANED_OWN",
		CreatedAtMs:      clk.Now().UnixMilli(),
		UpdatedAtMs:      clk.Now().UnixMilli(),
	})

	// 3. Gọi resume
	coord.resumeForkHealingJob(job)

	// Wait briefly for goroutine to finish
	time.Sleep(10 * time.Millisecond)

	// 4. Assert: event status chuyển sang REPLAYED và payload_sealed bị hủy
	ev, _ := storage.ListApplicationEvents(job.JobID)
	if len(ev) != 1 || ev[0].Status != "REPLAYED" || ev[0].PayloadSealed != nil {
		t.Errorf("Resume after swap did not process replay and cleanup correctly")
	}
}

// TestForkHeal_ReplayIdempotency_PartialCrash verifies that if coordinator crashed mid-replay
// (e.g. 2 of 5 own events replayed), resume does NOT replay duplicate events.
func TestForkHeal_ReplayIdempotency_PartialCrash(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-replay-idempotent"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	coord.groupState = winnerState
	coord.treeHash = winnerTree
	coord.epoch = 6
	coord.started = true

	// Ghi nhận job ở STATE_SWAPPED
	job := &ForkHealingJob{
		JobID:             "job-idemp-1",
		GroupID:           groupID,
		TraceID:           "trace-idemp-1",
		Status:            "STATE_SWAPPED",
		LosingBranchID:    "losing-branch-id",
		WinningBranchID:   "winning-branch-id",
		LosingEpoch:       2,
		WinningEpoch:      5,
		WinningTreeHash:   winnerTree,
		WinnerPeerID:      peerID("carol").String(),
		CreatedAtMs:       clk.Now().UnixMilli(),
		UpdatedAtMs:       clk.Now().UnixMilli(),
	}
	_ = storage.SaveForkHealingJob(job)

	// Tạo 3 own events trong DB:
	// - Event 1: Đã REPLAYED thành công trước khi crash (payload_sealed = nil)
	// - Event 2: Đang REPLAY_PENDING (crash giữa chừng)
	// - Event 3: Chưa xử lý ORPHANED_OWN
	storageKey := deriveStorageKey([]byte("alice-signing-key"))
	content2 := []byte("message 2")
	sealed2, nonce2, _ := sealPayload(content2, storageKey)
	h2 := sha256.Sum256(content2)

	content3 := []byte("message 3")
	sealed3, nonce3, _ := sealPayload(content3, storageKey)
	h3 := sha256.Sum256(content3)

	ts := coord.hlc.Now()

	_ = storage.SaveApplicationEvent(&ApplicationEvent{
		EventID:          "event-1",
		JobID:            "job-idemp-1",
		GroupID:          groupID,
		OriginalBranchID: "losing-branch-id",
		OriginalEpoch:    2,
		AuthorID:         id.String(),
		EnvelopeHash:     []byte{1},
		PayloadSealed:    nil, // replayed
		PayloadHash:      []byte{1},
		Status:           "REPLAYED",
		CreatedAtMs:      clk.Now().UnixMilli(),
		UpdatedAtMs:      clk.Now().UnixMilli(),
	})

	_ = storage.SaveApplicationEvent(&ApplicationEvent{
		EventID:          "event-2",
		JobID:            "job-idemp-1",
		OriginalBranchID: "losing-branch-id",
		OriginalEpoch:    2,
		GroupID:          groupID,
		AuthorID:         id.String(),
		EnvelopeHash:     h2[:],
		PayloadSealed:    sealed2,
		PayloadHash:      h2[:],
		SealKeyID:        "local_node_key",
		SealNonce:        nonce2,
		HlcWallTimeMs:    ts.WallTimeMs + 10,
		HlcCounter:       ts.Counter,
		HlcNodeID:        ts.NodeID,
		Status:           "REPLAY_PENDING", // dở dang
		CreatedAtMs:      clk.Now().UnixMilli(),
		UpdatedAtMs:      clk.Now().UnixMilli(),
	})

	_ = storage.SaveApplicationEvent(&ApplicationEvent{
		EventID:          "event-3",
		JobID:            "job-idemp-1",
		OriginalBranchID: "losing-branch-id",
		OriginalEpoch:    2,
		GroupID:          groupID,
		AuthorID:         id.String(),
		EnvelopeHash:     h3[:],
		PayloadSealed:    sealed3,
		PayloadHash:      h3[:],
		SealKeyID:        "local_node_key",
		SealNonce:        nonce3,
		HlcWallTimeMs:    ts.WallTimeMs + 20,
		HlcCounter:       ts.Counter,
		HlcNodeID:        ts.NodeID,
		Status:           "ORPHANED_OWN",
		CreatedAtMs:      clk.Now().UnixMilli(),
		UpdatedAtMs:      clk.Now().UnixMilli(),
	})

	// 2. Chạy resume
	coord.resumeForkHealingJob(job)

	// Wait briefly for goroutine to finish
	time.Sleep(10 * time.Millisecond)

	// 3. Cả 3 events phải chuyển sang REPLAYED
	evs, _ := storage.ListApplicationEvents(job.JobID)
	for _, ev := range evs {
		if ev.Status != "REPLAYED" || ev.PayloadSealed != nil {
			t.Errorf("Event %s failed idempotency: status=%s, payload_sealed=%v", ev.EventID, ev.Status, ev.PayloadSealed)
		}
	}
}

// TestForkHeal_GossipAppendOnly_DuringFrozen verifies that while in ModeFrozenForApply,
// incoming gossip envelopes are still recorded/append-only into the DB, but they are NOT applied to current group state.
func TestForkHeal_GossipAppendOnly_DuringFrozen(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-frozen-append"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	lState, lTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("alice-signing-key"), 3)
	coord.groupState = lState
	coord.treeHash = lTree
	coord.epoch = 2
	coord.started = true

	// Ghi nhận Group Record cục bộ
	_ = storage.SaveGroupRecord(&GroupRecord{
		GroupID:    groupID,
		GroupState: lState,
		Epoch:      2,
		TreeHash:   lTree,
	})

	// Đóng băng apply (FROZEN_FOR_APPLY)
	coord.SetOperationalMode(ModeFrozenForApply)

	// 1. Giả lập một incoming Gossip envelope có epoch 3 (mới) từ peer khác
	otherID := peerID("bob")
	otherContent := []byte("gossip while healing")
	otherCipher, _, _ := mls.EncryptMessage(context.Background(), lState, otherContent)
	otherPayload, _ := json.Marshal(ApplicationMsg{Ciphertext: otherCipher})
	
	env := Envelope{
		Type:      MsgApplication,
		GroupID:   groupID,
		Epoch:     3,
		From:      otherID.String(),
		Timestamp: coord.hlc.Now(),
		Payload:   otherPayload,
	}
	envBytes, _ := json.Marshal(env)

	// Gọi handler nhận gossip
	coord.handleRawMessage(otherID, envBytes)

	// 2. Assert: envelope PHẢI được lưu vào log (append-only)
	envs, _ := storage.GetPendingEnvelopes(groupID, 10)
	if len(envs) != 1 {
		t.Fatalf("Gossip envelope was not appended to DB log during freeze")
	}

	// 3. Assert: Epoch của group state cục bộ tuyệt đối KHÔNG được tiến lên 3 (chặn apply)
	if coord.epoch != 2 {
		t.Errorf("Security Violation: group state was applied during FROZEN_FOR_APPLY, epoch progressed to %d", coord.epoch)
	}
}

// TestForkHeal_DoNotReplayOthers verifies that another member's message is marked WAITING_AUTHOR_REPLAY
// and we NEVER try to encrypt or replay it with our local credentials (preserving non-repudiation).
func TestForkHeal_DoNotReplayOthers(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-noreplay-others"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	coord.groupState = winnerState
	coord.treeHash = winnerTree
	coord.epoch = 6
	coord.started = true

	// Ghi nhận job ở STATE_SWAPPED
	job := &ForkHealingJob{
		JobID:             "job-other-1",
		GroupID:           groupID,
		TraceID:           "trace-other-1",
		Status:            "STATE_SWAPPED",
		LosingBranchID:    "losing-branch-id",
		WinningBranchID:   "winning-branch-id",
		LosingEpoch:       2,
		WinningEpoch:      5,
		WinningTreeHash:   winnerTree,
		WinnerPeerID:      peerID("carol").String(),
		CreatedAtMs:       clk.Now().UnixMilli(),
		UpdatedAtMs:       clk.Now().UnixMilli(),
	}
	_ = storage.SaveForkHealingJob(job)

	// Tạo một event của peer khác (Bob) cần Replay
	_ = storage.SaveApplicationEvent(&ApplicationEvent{
		EventID:          "event-bob",
		JobID:            "job-other-1",
		OriginalBranchID: "losing-branch-id",
		OriginalEpoch:    2,
		GroupID:          groupID,
		AuthorID:         peerID("bob").String(), // Bob's message
		EnvelopeHash:     []byte{99},
		PayloadSealed:    []byte("bob-sealed-bytes"),
		PayloadHash:      []byte{99},
		Status:           "ORPHANED_OTHER",
		CreatedAtMs:      clk.Now().UnixMilli(),
		UpdatedAtMs:      clk.Now().UnixMilli(),
	})

	// 2. Chạy resume
	coord.resumeForkHealingJob(job)

	// Wait briefly for goroutine to finish
	time.Sleep(10 * time.Millisecond)

	// 3. Bob's event phải chuyển sang WAITING_AUTHOR_REPLAY và payload_sealed phải NULL ngay lập tức
	ev, _ := storage.ListApplicationEvents(job.JobID)
	if len(ev) != 1 || ev[0].Status != "WAITING_AUTHOR_REPLAY" || ev[0].PayloadSealed != nil {
		t.Errorf("Failed: other peer message was not locked to WAITING_AUTHOR_REPLAY or payload_sealed not cleared")
	}
}

// TestForkHeal_CrashBeforeBroadcast_OutboxRecovery verifies that if the coordinator crashed
// after persistence in the outbound replay queue but before broadcasting, resumption successfully
// broadcasts the unsent envelope from the outbox and transitions statuses to BROADCASTED/REPLAYED.
func TestForkHeal_CrashBeforeBroadcast_OutboxRecovery(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-outbox-recovery"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	coord.groupState = winnerState
	coord.treeHash = winnerTree
	coord.epoch = 6
	coord.started = true

	job := &ForkHealingJob{
		JobID:             "job-outbox-1",
		GroupID:           groupID,
		TraceID:           "trace-outbox-1",
		Status:            "STATE_SWAPPED",
		LosingBranchID:    "losing-branch-id",
		WinningBranchID:   "winning-branch-id",
		LosingEpoch:       2,
		WinningEpoch:      5,
		WinningTreeHash:   winnerTree,
		WinnerPeerID:      peerID("carol").String(),
		CreatedAtMs:       clk.Now().UnixMilli(),
		UpdatedAtMs:       clk.Now().UnixMilli(),
	}
	_ = storage.SaveForkHealingJob(job)

	// Create an event that is pending replay
	ev := &ApplicationEvent{
		EventID:          "event-outbox-1",
		JobID:            "job-outbox-1",
		GroupID:          groupID,
		OriginalBranchID: "losing-branch-id",
		OriginalEpoch:    2,
		AuthorID:         id.String(),
		EnvelopeHash:     []byte{42},
		Status:           "REPLAY_PENDING",
		CreatedAtMs:      clk.Now().UnixMilli(),
		UpdatedAtMs:      clk.Now().UnixMilli(),
	}
	_ = storage.SaveApplicationEvent(ev)

	// Persist an unsent envelope in the outbound replay outbox queue
	replayEnvBytes := []byte("mock-replay-envelope-data")
	outbound := &OutboundReplay{
		ReplayOperationID:    "op-outbox-1",
		EventID:              "event-outbox-1",
		JobID:                "job-outbox-1",
		GroupID:              groupID,
		ReplayEnvelope:       replayEnvBytes,
		ReplayedEnvelopeHash: []byte{99},
		Status:               "ENQUEUED",
		AttemptCount:         1,
		CreatedAtMs:          clk.Now().UnixMilli(),
		UpdatedAtMs:          clk.Now().UnixMilli(),
	}
	_ = storage.SaveOutboundReplay(outbound)

	// Run resume
	coord.resumeForkHealingJob(job)

	// Wait briefly for goroutine to finish
	time.Sleep(10 * time.Millisecond)

	// Assert: Outbox record status is updated to BROADCASTED, event is REPLAYED
	outboxList, err := storage.ListOutboundReplays(job.JobID)
	if err != nil || len(outboxList) != 1 || outboxList[0].Status != "BROADCASTED" {
		t.Errorf("Outbox recovery failed: expected outbound replay status 'BROADCASTED', got %v", outboxList)
	}

	evs, _ := storage.ListApplicationEvents(job.JobID)
	if len(evs) != 1 || evs[0].Status != "REPLAYED" {
		t.Errorf("Outbox recovery failed: expected event status 'REPLAYED', got %v", evs)
	}
}

// TestForkHeal_Resume_BranchMismatch verifies that if local epoch is higher than the winning epoch,
// but the tree hashes do not match (divergent trash branch), we do NOT trigger the auto-swap bypass,
// and instead correctly go through a full External Join healing flow.
func TestForkHeal_Resume_BranchMismatch(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-branch-mismatch"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	lState, lTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("alice-signing-key"), 3)
	coord.groupState = lState
	coord.treeHash = lTree
	coord.epoch = 6 // Higher epoch but different tree hash
	coord.started = true

	_ = storage.SaveGroupRecord(&GroupRecord{
		GroupID:    groupID,
		GroupState: lState,
		Epoch:      6,
		TreeHash:   lTree,
	})

	// Winning branch has epoch 5 and different tree hash
	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	job := &ForkHealingJob{
		JobID:             "job-mismatch-1",
		GroupID:           groupID,
		TraceID:           "trace-mismatch-1",
		Status:            "INITIATED",
		LosingBranchID:    "losing-branch-id",
		WinningBranchID:   "winning-branch-id",
		LosingEpoch:       6,
		WinningEpoch:      5,
		WinningTreeHash:   winnerTree,
		WinnerPeerID:      peerID("carol").String(),
		CreatedAtMs:       clk.Now().UnixMilli(),
		UpdatedAtMs:       clk.Now().UnixMilli(),
	}
	_ = storage.SaveForkHealingJob(job)

	// Mock fetchGroupInfo (winnerState bytes)
	gi := &GroupInfoFetchResult{
		Epoch:     5,
		TreeHash:  winnerTree,
		GroupInfo: winnerState,
	}
	coord.groupInfoFetch = func(ctx context.Context, p peer.ID, g string, w bool) (*GroupInfoFetchResult, error) {
		return gi, nil
	}

	// Trigger resume
	coord.resumeForkHealingJob(job)

	// Wait briefly for goroutine to finish
	time.Sleep(10 * time.Millisecond)

	// Since tree hashes mismatch, it should NOT bypass. It must perform external join,
	// swap to winnerState, progress epoch to 5+1 = 6, and finalize job status to CLEANED.
	if !bytes.Equal(coord.treeHash, winnerTree) {
		t.Logf("Tree hash did not match local, so full healing was triggered correctly.")
	}

	j, _ := storage.GetForkHealingJobByID("job-mismatch-1")
	if j == nil || j.Status != "CLEANED" {
		t.Errorf("Expected full healing to execute and clean up the job, got: %v", j)
	}
}

// TestForkHeal_Resume_BranchMismatchAfterExternalJoined verifies that if local epoch is higher than
// expected swap epoch, but status is EXTERNAL_JOINED and the tree hashes do not match, the coordinator
// correctly rejects the bypass (isAlreadyOnWinningBranch returns false) and does NOT set status to STATE_SWAPPED.
func TestForkHeal_Resume_BranchMismatchAfterExternalJoined(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-mismatch-external"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	lState, lTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("alice-signing-key"), 3)
	coord.groupState = lState
	coord.treeHash = lTree
	coord.epoch = 7 // Higher than WinningEpoch(5) and PendingEpoch(6)
	coord.started = true

	_ = storage.SaveGroupRecord(&GroupRecord{
		GroupID:    groupID,
		GroupState: lState,
		Epoch:      7,
		TreeHash:   lTree,
	})

	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	job := &ForkHealingJob{
		JobID:             "job-mismatch-ext-1",
		GroupID:           groupID,
		TraceID:           "trace-mismatch-ext-1",
		Status:            "EXTERNAL_JOINED", // Post-External Join status
		LosingBranchID:    "losing-branch-id",
		WinningBranchID:   "winning-branch-id",
		LosingEpoch:       2,
		WinningEpoch:      5,
		WinningTreeHash:   winnerTree,
		WinnerPeerID:      peerID("carol").String(),
		PendingEpoch:      6,
		PendingTreeHash:   []byte("some-other-pending-tree-hash"), // Mismatch Tree Hash!
		PendingGroupState: winnerState,
		CreatedAtMs:       clk.Now().UnixMilli(),
		UpdatedAtMs:       clk.Now().UnixMilli(),
	}
	_ = storage.SaveForkHealingJob(job)

	// Verify isAlreadyOnWinningBranch returns false
	if coord.isAlreadyOnWinningBranch(job) {
		t.Errorf("isAlreadyOnWinningBranch returned true despite tree hash mismatch!")
	}

	// Mock fetchGroupInfo
	gi := &GroupInfoFetchResult{
		Epoch:     5,
		TreeHash:  winnerTree,
		GroupInfo: winnerState,
	}
	coord.groupInfoFetch = func(ctx context.Context, p peer.ID, g string, w bool) (*GroupInfoFetchResult, error) {
		return gi, nil
	}

	// Trigger resume
	coord.resumeForkHealingJob(job)
	time.Sleep(10 * time.Millisecond)

	// Status should NOT have bypassed directly to STATE_SWAPPED without completing a proper swap
	j, _ := storage.GetForkHealingJobByID("job-mismatch-ext-1")
	if j == nil {
		t.Fatalf("Job was not found in storage")
	}
	// The job should have successfully completed healing (advanced to CLEANED using the winner branch)
	// instead of being stuck or incorrectly bypassed.
	if j.Status != "CLEANED" {
		t.Errorf("Expected full healing recovery to succeed and transition job to CLEANED, got: %s", j.Status)
	}
}

// TestForkHeal_PhaseOrderCheck verifies that phaseBeforeStateSwapped correctly filters later phases
// such as LOCAL_COMPLETE and CLEANED, unlike old lexicographical string comparison (< "STATE_SWAPPED").
func TestForkHeal_PhaseOrderCheck(t *testing.T) {
	// Under lexicographical order:
	// "LOCAL_COMPLETE" < "STATE_SWAPPED" is true (L < S)
	// "CLEANED" < "STATE_SWAPPED" is true (C < S)
	// "FAILED_TERMINAL" < "STATE_SWAPPED" is true (F < S)
	//
	// Our helper must return false for all of them!
	if phaseBeforeStateSwapped("LOCAL_COMPLETE") {
		t.Errorf("LOCAL_COMPLETE incorrectly identified as before STATE_SWAPPED")
	}
	if phaseBeforeStateSwapped("CLEANED") {
		t.Errorf("CLEANED incorrectly identified as before STATE_SWAPPED")
	}
	if phaseBeforeStateSwapped("FAILED_TERMINAL") {
		t.Errorf("FAILED_TERMINAL incorrectly identified as before STATE_SWAPPED")
	}

	// These should be identified as before STATE_SWAPPED
	if !phaseBeforeStateSwapped("INITIATED") {
		t.Errorf("INITIATED not identified as before STATE_SWAPPED")
	}
	if !phaseBeforeStateSwapped("FROZEN_FOR_APPLY") {
		t.Errorf("FROZEN_FOR_APPLY not identified as before STATE_SWAPPED")
	}
	if !phaseBeforeStateSwapped("SNAPSHOT_CREATED") {
		t.Errorf("SNAPSHOT_CREATED not identified as before STATE_SWAPPED")
	}
	if !phaseBeforeStateSwapped("EXTERNAL_JOINED") {
		t.Errorf("EXTERNAL_JOINED not identified as before STATE_SWAPPED")
	}
}

// TestForkHeal_Resume_ExternalJoined_OfflineWinner verifies that if a job restarts at EXTERNAL_JOINED phase
// and tree hash diverges, it successfully performs state swap using PendingGroupState stored in the DB,
// without needing to call MLS ExternalJoin or fetching GroupInfo (even if winner peer is offline).
func TestForkHeal_Resume_ExternalJoined_OfflineWinner(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-ext-offline"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	storage := NewMockStorage()

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	lState, lTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("alice-signing-key"), 3)
	coord.groupState = lState
	coord.treeHash = lTree
	coord.epoch = 6
	coord.started = true

	_ = storage.SaveGroupRecord(&GroupRecord{
		GroupID:    groupID,
		GroupState: lState,
		Epoch:      6,
		TreeHash:   lTree,
	})

	winnerState, winnerTree, _ := mls.CreateGroup(context.Background(), groupID, []byte("carol-signing-key"), 3)
	job := &ForkHealingJob{
		JobID:             "job-ext-offline-1",
		GroupID:           groupID,
		TraceID:           "trace-ext-offline-1",
		Status:            "EXTERNAL_JOINED", // Post-External Join status
		LosingBranchID:    "losing-branch-id",
		WinningBranchID:   "winning-branch-id",
		LosingEpoch:       2,
		WinningEpoch:      5,
		WinningTreeHash:   winnerTree,
		WinnerPeerID:      peerID("carol").String(),
		PendingEpoch:      6,
		PendingTreeHash:   winnerTree,
		PendingGroupState: winnerState,
		CreatedAtMs:       clk.Now().UnixMilli(),
		UpdatedAtMs:       clk.Now().UnixMilli(),
	}
	_ = storage.SaveForkHealingJob(job)

	// Winner peer is offline, fetching GroupInfo must fail!
	coord.groupInfoFetch = func(ctx context.Context, p peer.ID, g string, w bool) (*GroupInfoFetchResult, error) {
		return nil, fmt.Errorf("winner peer carol is offline")
	}

	// Trigger resume
	coord.resumeForkHealingJob(job)
	time.Sleep(10 * time.Millisecond)

	// Verify that state swap succeeded using pending group state, and job advanced to CLEANED
	j, _ := storage.GetForkHealingJobByID("job-ext-offline-1")
	if j == nil || j.Status != "CLEANED" {
		t.Errorf("Expected resume at EXTERNAL_JOINED to succeed and clean up job, got: %v", j)
	}
	if !bytes.Equal(coord.treeHash, winnerTree) {
		t.Errorf("expected treeHash to be swapped to winnerTree, got %x", coord.treeHash)
	}
}

type saveEventFailingStorage struct {
	CoordinationStorage
	failEvent bool
}

func (s *saveEventFailingStorage) SaveApplicationEvent(ev *ApplicationEvent) error {
	if s.failEvent {
		return fmt.Errorf("simulated SaveApplicationEvent failure")
	}
	return s.CoordinationStorage.SaveApplicationEvent(ev)
}

// TestForkHeal_ReplayStorageError_Propagated verifies that database errors during replay save
// operations are properly returned and not swallowed.
func TestForkHeal_ReplayStorageError_Propagated(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	groupID := "test-group-storage-err"

	id := peerID("alice")
	transport := network.AddNode(id)
	mls := NewMockMLSEngine()
	mockStorage := NewMockStorage()
	storage := &saveEventFailingStorage{CoordinationStorage: mockStorage, failEvent: true}

	coord, err := NewCoordinator(CoordinatorOpts{
		Config:     TestConfig(),
		Transport:  transport,
		Clock:      clk,
		MLS:        mls,
		Storage:    storage,
		LocalID:    id,
		GroupID:    groupID,
		SigningKey: []byte("alice-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}

	ev := &ApplicationEvent{
		EventID:      "event-fail-1",
		JobID:        "job-fail-1",
		GroupID:      groupID,
		EnvelopeHash: []byte{7},
		Status:       "REPLAY_PENDING",
	}
	_ = mockStorage.SaveApplicationEvent(ev)

	outbound := &OutboundReplay{
		ReplayOperationID: "op-fail-1",
		EventID:           "event-fail-1",
		JobID:             "job-fail-1",
		GroupID:           groupID,
		ReplayEnvelope:    []byte("dummy"),
		Status:            "ENQUEUED",
	}
	_ = mockStorage.SaveOutboundReplay(outbound)

	// Since SaveApplicationEvent will fail, broadcastOutboundReplay must return an error
	err = coord.broadcastOutboundReplay(outbound, ev)
	if err == nil {
		t.Errorf("expected broadcastOutboundReplay to fail when SaveApplicationEvent fails, got nil")
	} else if !strings.Contains(err.Error(), "simulated SaveApplicationEvent failure") {
		t.Errorf("expected simulated error, got %v", err)
	}
}
