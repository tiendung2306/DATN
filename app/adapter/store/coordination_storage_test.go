package store

import (
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

func setupTestStorage(t *testing.T) *SQLiteCoordinationStorage {
	t.Helper()
	d, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return NewSQLiteCoordinationStorage(d)
}

func TestSQLiteCoordinationStorage_GroupRecord_SaveAndGet(t *testing.T) {
	s := setupTestStorage(t)

	rec := &coordination.GroupRecord{
		GroupID:    "group-1",
		GroupState: []byte("fake-state"),
		Epoch:      3,
		TreeHash:   []byte("tree-abc"),
		MyRole:     coordination.RoleCreator,
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	if err := s.SaveGroupRecord(rec); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}

	got, err := s.GetGroupRecord("group-1")
	if err != nil {
		t.Fatalf("GetGroupRecord: %v", err)
	}

	if got.GroupID != rec.GroupID {
		t.Errorf("GroupID = %q, want %q", got.GroupID, rec.GroupID)
	}
	if string(got.GroupState) != string(rec.GroupState) {
		t.Errorf("GroupState mismatch")
	}
	if got.Epoch != rec.Epoch {
		t.Errorf("Epoch = %d, want %d", got.Epoch, rec.Epoch)
	}
	if string(got.TreeHash) != string(rec.TreeHash) {
		t.Errorf("TreeHash mismatch")
	}
	if got.MyRole != coordination.RoleCreator {
		t.Errorf("MyRole = %q, want %q", got.MyRole, coordination.RoleCreator)
	}
}

func TestSQLiteCoordinationStorage_GroupRecord_NotFound(t *testing.T) {
	s := setupTestStorage(t)

	_, err := s.GetGroupRecord("nonexistent")
	if err != coordination.ErrGroupNotFound {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestSQLiteCoordinationStorage_GroupRecord_Upsert(t *testing.T) {
	s := setupTestStorage(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	rec := &coordination.GroupRecord{
		GroupID:    "group-u",
		GroupState: []byte("v1"),
		Epoch:      0,
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.SaveGroupRecord(rec); err != nil {
		t.Fatal(err)
	}

	rec.GroupState = []byte("v2")
	rec.Epoch = 5
	rec.UpdatedAt = now.Add(time.Hour)
	if err := s.SaveGroupRecord(rec); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetGroupRecord("group-u")
	if got.Epoch != 5 {
		t.Errorf("Epoch = %d after upsert, want 5", got.Epoch)
	}
	if string(got.GroupState) != "v2" {
		t.Errorf("GroupState = %q after upsert, want %q", got.GroupState, "v2")
	}
}

func TestSQLiteCoordinationStorage_GroupRecord_RejectsEmptyState(t *testing.T) {
	s := setupTestStorage(t)
	err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:   "group-empty-state",
		Epoch:     1,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected empty group_state to be rejected")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "empty group_state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSQLiteCoordinationStorage_GroupRecord_Upsert_PreservesMyRoleWhenOmitted(t *testing.T) {
	s := setupTestStorage(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-role",
		GroupState: []byte("v0"),
		Epoch:      0,
		TreeHash:   []byte("t0"),
		MyRole:     coordination.RoleCreator,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate epoch advancement saving state without MyRole (zero value).
	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-role",
		GroupState: []byte("v1"),
		Epoch:      1,
		TreeHash:   []byte("t1"),
		MyRole:     "",
		CreatedAt:  time.Time{},
		UpdatedAt:  now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetGroupRecord("group-role")
	if err != nil {
		t.Fatal(err)
	}
	if got.MyRole != coordination.RoleCreator {
		t.Errorf("MyRole = %q after upsert with empty MyRole, want %q", got.MyRole, coordination.RoleCreator)
	}
}

func TestSQLiteCoordinationStorage_GroupRecord_DMCounterpartySurvivesEpochSave(t *testing.T) {
	s := setupTestStorage(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:              "dm-meta",
		GroupState:           []byte("v0"),
		Epoch:                0,
		MyRole:               coordination.RoleCreator,
		GroupType:            "dm",
		DMCounterpartyPeerID: "peer-b",
		CreatedAt:            now,
		UpdatedAt:            now,
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "dm-meta",
		GroupState: []byte("v1"),
		Epoch:      1,
		MyRole:     "",
		GroupType:  "",
		CreatedAt:  time.Time{},
		UpdatedAt:  now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetGroupRecord("dm-meta")
	if err != nil {
		t.Fatal(err)
	}
	if got.DMCounterpartyPeerID != "peer-b" {
		t.Fatalf("DMCounterpartyPeerID=%q want peer-b", got.DMCounterpartyPeerID)
	}
}

func TestSQLiteCoordinationStorage_ListGroups(t *testing.T) {
	s := setupTestStorage(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, id := range []string{"g-a", "g-b", "g-c"} {
		if err := s.SaveGroupRecord(&coordination.GroupRecord{
			GroupID: id, GroupState: []byte("s"), MyRole: coordination.RoleMember,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	groups, err := s.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("ListGroups returned %d, want 3", len(groups))
	}
}

func TestSQLiteCoordinationStorage_CoordState_SaveAndGet(t *testing.T) {
	s := setupTestStorage(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID: "cg-1", GroupState: []byte("s"), MyRole: coordination.RoleMember,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	state := &coordination.CoordState{
		GroupID:          "cg-1",
		ActiveView:       []peer.ID{"12D3KooWPbEBrDhZhfnAbZ1iwSQiQTsoz9NJKyxdkBL3Jiyu2wor", "12D3KooWChZkqKQ3oEL7Va9cYFc5aRgXhR4CWPP14uJtarqpL7iM"},
		TokenHolder:      "12D3KooWPbEBrDhZhfnAbZ1iwSQiQTsoz9NJKyxdkBL3Jiyu2wor",
		LastCommitHash:   []byte("hash123"),
		LastCommitAt:     now,
		PendingProposals: [][]byte{[]byte("p1"), []byte("p2")},
	}
	if err := s.SaveCoordState(state); err != nil {
		t.Fatalf("SaveCoordState: %v", err)
	}

	got, err := s.GetCoordState("cg-1")
	if err != nil {
		t.Fatalf("GetCoordState: %v", err)
	}
	if got.GroupID != "cg-1" {
		t.Errorf("GroupID = %q", got.GroupID)
	}
	if len(got.ActiveView) != 2 {
		t.Errorf("ActiveView len = %d, want 2", len(got.ActiveView))
	}
	if got.TokenHolder == "" {
		t.Errorf("TokenHolder should not be empty")
	}
	if len(got.PendingProposals) != 2 {
		t.Errorf("PendingProposals len = %d, want 2", len(got.PendingProposals))
	}
}

func TestSQLiteCoordinationStorage_CoordState_NotFound(t *testing.T) {
	s := setupTestStorage(t)

	_, err := s.GetCoordState("nonexistent")
	if err != coordination.ErrGroupNotFound {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestSQLiteCoordinationStorage_Message_SaveAndQuery(t *testing.T) {
	s := setupTestStorage(t)

	msgs := []*coordination.StoredMessage{
		{GroupID: "mg-1", Epoch: 1, SenderID: "alice", Content: []byte("hello"),
			Timestamp: coordination.HLCTimestamp{WallTimeMs: 1000, Counter: 0, NodeID: "alice"}, EnvelopeHash: []byte("env-1")},
		{GroupID: "mg-1", Epoch: 1, SenderID: "bob", Content: []byte("world"),
			Timestamp: coordination.HLCTimestamp{WallTimeMs: 1000, Counter: 1, NodeID: "bob"}, EnvelopeHash: []byte("env-2")},
		{GroupID: "mg-1", Epoch: 2, SenderID: "alice", Content: []byte("epoch2"),
			Timestamp: coordination.HLCTimestamp{WallTimeMs: 2000, Counter: 0, NodeID: "alice"}, EnvelopeHash: []byte("env-3")},
	}

	for _, m := range msgs {
		if err := s.SaveMessage(m); err != nil {
			t.Fatalf("SaveMessage: %v", err)
		}
	}

	after := coordination.HLCTimestamp{WallTimeMs: 1000, Counter: 0, NodeID: "alice"}
	got, err := s.GetMessagesSince("mg-1", after)
	if err != nil {
		t.Fatalf("GetMessagesSince: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("GetMessagesSince returned %d messages, want 2", len(got))
	}
	if string(got[0].Content) != "world" {
		t.Errorf("first message = %q, want %q", got[0].Content, "world")
	}
	if string(got[1].Content) != "epoch2" {
		t.Errorf("second message = %q, want %q", got[1].Content, "epoch2")
	}
}

func TestSQLiteCoordinationStorage_Message_EmptyResult(t *testing.T) {
	s := setupTestStorage(t)

	got, err := s.GetMessagesSince("mg-empty", coordination.HLCTimestamp{})
	if err != nil {
		t.Fatalf("GetMessagesSince: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 messages, got %d", len(got))
	}
}

func TestSQLiteCoordinationStorage_Message_DeduplicatesByEnvelopeHash(t *testing.T) {
	s := setupTestStorage(t)
	hash := []byte("same-envelope")
	msg := &coordination.StoredMessage{
		GroupID:      "mg-dedup",
		Epoch:        1,
		SenderID:     "alice",
		Content:      []byte("hello"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: 1000, Counter: 0, NodeID: "alice"},
		EnvelopeHash: hash,
	}
	if err := s.SaveMessage(msg); err != nil {
		t.Fatalf("SaveMessage first: %v", err)
	}
	if err := s.SaveMessage(msg); err != nil {
		t.Fatalf("SaveMessage duplicate: %v", err)
	}

	got, err := s.GetMessagesSince("mg-dedup", coordination.HLCTimestamp{})
	if err != nil {
		t.Fatalf("GetMessagesSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("stored_messages rows=%d, want 1", len(got))
	}
}

func TestSQLiteCoordinationStorage_AppliedEnvelopeMarkers(t *testing.T) {
	s := setupTestStorage(t)
	hash := []byte("env-hash")

	applied, err := s.HasAppliedEnvelope("g1", hash)
	if err != nil {
		t.Fatalf("HasAppliedEnvelope before mark: %v", err)
	}
	if applied {
		t.Fatal("expected envelope to be absent before mark")
	}

	if err := s.MarkEnvelopeApplied("g1", coordination.MsgApplication, 1, hash); err != nil {
		t.Fatalf("MarkEnvelopeApplied: %v", err)
	}
	if err := s.MarkEnvelopeApplied("g1", coordination.MsgApplication, 1, hash); err != nil {
		t.Fatalf("MarkEnvelopeApplied duplicate: %v", err)
	}

	applied, err = s.HasAppliedEnvelope("g1", hash)
	if err != nil {
		t.Fatalf("HasAppliedEnvelope after mark: %v", err)
	}
	if !applied {
		t.Fatal("expected envelope to be marked applied")
	}
}

func TestSQLiteCoordinationStorage_EnvelopeLog_AppendAndSince(t *testing.T) {
	s := setupTestStorage(t)
	ts := coordination.HLCTimestamp{WallTimeMs: 1, Counter: 0, NodeID: "p1"}
	wire := []byte(`{"type":"application","group_id":"g1","epoch":0,"from":"p1","ts":{"l":1,"c":0,"id":"p1"},"payload":{}}`)

	seq1, err := s.AppendEnvelope("g1", coordination.MsgApplication, 0, ts, wire)
	if err != nil || seq1 != 1 {
		t.Fatalf("AppendEnvelope first: seq=%d err=%v", seq1, err)
	}
	seq2, err := s.AppendEnvelope("g1", coordination.MsgApplication, 0, ts, wire)
	if err != nil || seq2 != 0 {
		t.Fatalf("AppendEnvelope duplicate: seq=%d err=%v", seq2, err)
	}
	latest, _ := s.GetLatestSeq("g1")
	if latest != 1 {
		t.Fatalf("GetLatestSeq = %d, want 1", latest)
	}
	recs, err := s.GetEnvelopesSince("g1", 0, 10)
	if err != nil || len(recs) != 1 {
		t.Fatalf("GetEnvelopesSince after 0: %d recs err=%v", len(recs), err)
	}
	sum := sha256.Sum256(wire)
	if string(recs[0].EnvelopeHash) != string(sum[:]) {
		t.Fatalf("EnvelopeHash not persisted")
	}
	if recs[0].SourcePath != "local" || recs[0].ApplyState != "pending" {
		t.Fatalf("source/state = %q/%q, want local/pending", recs[0].SourcePath, recs[0].ApplyState)
	}
	if err := s.MarkEnvelopeReplayState("g1", recs[0].EnvelopeHash, coordination.ReplayStateFutureEpoch, "waiting for commit", time.Now()); err != nil {
		t.Fatalf("MarkEnvelopeReplayState: %v", err)
	}
	pending, err := s.GetPendingEnvelopes("g1", 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("GetPendingEnvelopes: len=%d err=%v", len(pending), err)
	}
	if pending[0].Seq != 1 || pending[0].ApplyState != string(coordination.ReplayStateFutureEpoch) {
		t.Fatalf("pending[0] = %+v, want seq 1 future_epoch", pending[0])
	}
	if err := s.MarkEnvelopeReplayState("g1", recs[0].EnvelopeHash, coordination.ReplayStateApplied, "", time.Now()); err != nil {
		t.Fatalf("MarkEnvelopeReplayState applied: %v", err)
	}
	pending, err = s.GetPendingEnvelopes("g1", 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("GetPendingEnvelopes after applied: len=%d err=%v", len(pending), err)
	}
	recs, err = s.GetEnvelopesSince("g1", 1, 10)
	if err != nil || len(recs) != 0 {
		t.Fatalf("GetEnvelopesSince after 1: %+v err=%v", recs, err)
	}
}

func TestSQLiteCoordinationStorage_SyncAcksAndPullCursor(t *testing.T) {
	s := setupTestStorage(t)
	if err := s.RecordSyncAck("peerB", "g1", 5); err != nil {
		t.Fatal(err)
	}
	ack, _ := s.GetSyncAck("peerB", "g1")
	if ack != 5 {
		t.Fatalf("GetSyncAck = %d", ack)
	}
	min, _ := s.GetMinAckedSeq("g1", []string{"peerB", "peerC"})
	if min != 0 {
		t.Fatalf("GetMinAckedSeq = %d, want 0 (peerC missing)", min)
	}
	_ = s.RecordSyncAck("peerC", "g1", 3)
	min, _ = s.GetMinAckedSeq("g1", []string{"peerB", "peerC"})
	if min != 3 {
		t.Fatalf("GetMinAckedSeq = %d, want 3", min)
	}
	_ = s.SetOfflinePullCursor("g1", "peerA", 7)
	cur, _ := s.GetOfflinePullCursor("g1", "peerA")
	if cur != 7 {
		t.Fatalf("pull cursor = %d", cur)
	}
}

func TestSQLiteCoordinationStorage_ApplyApplication_IsAtomicAndIdempotent(t *testing.T) {
	s := setupTestStorage(t)
	now := time.Now()
	rec := &coordination.GroupRecord{
		GroupID:    "g-atomic-app",
		GroupState: []byte("state-1"),
		Epoch:      1,
		TreeHash:   []byte("tree-1"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	msg := &coordination.StoredMessage{
		GroupID:      "g-atomic-app",
		Epoch:        1,
		SenderID:     "12D3KooWPbEBrDhZhfnAbZ1iwSQiQTsoz9NJKyxdkBL3Jiyu2wor",
		Content:      []byte("hello"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: now.UnixMilli(), Counter: 0, NodeID: "n1"},
		EnvelopeHash: []byte("h1"),
	}
	wire := []byte(`{"type":"application","group_id":"g-atomic-app","epoch":1}`)
	applied, seq, err := s.ApplyApplication(rec, msg, coordination.MsgApplication, wire, msg.Timestamp, 1)
	if err != nil || !applied || seq != 1 {
		t.Fatalf("ApplyApplication first: applied=%v seq=%d err=%v", applied, seq, err)
	}
	applied, seq, err = s.ApplyApplication(rec, msg, coordination.MsgApplication, wire, msg.Timestamp, 1)
	if err != nil || applied || seq != 0 {
		t.Fatalf("ApplyApplication duplicate: applied=%v seq=%d err=%v", applied, seq, err)
	}
	gotMsgs, err := s.GetMessagesSince("g-atomic-app", coordination.HLCTimestamp{})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotMsgs) != 1 {
		t.Fatalf("messages rows=%d, want 1", len(gotMsgs))
	}
}

func TestSQLiteCoordinationStorage_ApplyCommit_PersistsGroupState(t *testing.T) {
	s := setupTestStorage(t)
	now := time.Now()
	rec := &coordination.GroupRecord{
		GroupID:    "g-atomic-commit",
		GroupState: []byte("state-2"),
		Epoch:      2,
		TreeHash:   []byte("tree-2"),
		MyRole:     coordination.RoleCreator,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	wire := []byte(`{"type":"commit","group_id":"g-atomic-commit","epoch":1}`)
	ts := coordination.HLCTimestamp{WallTimeMs: now.UnixMilli(), Counter: 0, NodeID: "n1"}
	applied, seq, err := s.ApplyCommit(rec, coordination.MsgCommit, wire, ts, 1)
	if err != nil || !applied || seq != 1 {
		t.Fatalf("ApplyCommit: applied=%v seq=%d err=%v", applied, seq, err)
	}
	got, err := s.GetGroupRecord("g-atomic-commit")
	if err != nil {
		t.Fatal(err)
	}
	if got.Epoch != 2 || string(got.GroupState) != "state-2" {
		t.Fatalf("persisted group mismatch: epoch=%d state=%q", got.Epoch, got.GroupState)
	}
}

func TestSQLiteCoordinationStorage_ForkHealHistory_RecordAndList(t *testing.T) {
	s := setupTestStorage(t)
	ev := &coordination.ForkHealEventRecord{
		TraceID:              "trace-1",
		GroupID:              "g-heal",
		WinnerPeerID:         "peer-winner",
		WinnerEpoch:          7,
		NewEpoch:             8,
		Outcome:              "success",
		WinnerTreeHash:       []byte("winner-tree"),
		NewTreeHash:          []byte("new-tree"),
		PartitionStartedAtMs: 1000,
		ScheduledAtMs:        1100,
		StartedAtMs:          1200,
		CompletedAtMs:        1500,
		DurationMs:           300,
		TotalMs:              400,
		ReplayedMessageCount: 2,
	}
	if err := s.RecordForkHealEvent(ev); err != nil {
		t.Fatalf("RecordForkHealEvent: %v", err)
	}
	if err := s.RecordForkHealAudit(&coordination.ForkHealAuditRecord{
		TraceID: "trace-1", GroupID: "g-heal", Step: "external_join", Status: "completed", TimestampMs: 1300, DurationMs: 25,
	}); err != nil {
		t.Fatalf("RecordForkHealAudit: %v", err)
	}

	events, err := s.ListForkHealEvents("g-heal", 10)
	if err != nil {
		t.Fatalf("ListForkHealEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].TraceID != "trace-1" || events[0].Outcome != "success" {
		t.Fatalf("unexpected event row: %+v", events[0])
	}
	audit, err := s.ListForkHealAudit("trace-1")
	if err != nil {
		t.Fatalf("ListForkHealAudit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("audit len = %d, want 1", len(audit))
	}
	if audit[0].Step != "external_join" || audit[0].Status != "completed" {
		t.Fatalf("unexpected audit row: %+v", audit[0])
	}
}

func TestSQLiteCoordinationStorage_ForkHealHistory_PruneCap(t *testing.T) {
	s := setupTestStorage(t)
	for i := 0; i < 3; i++ {
		traceID := "trace-cap-" + string(rune('a'+i))
		if err := s.RecordForkHealEvent(&coordination.ForkHealEventRecord{
			TraceID: traceID, GroupID: "g-cap", Outcome: "success",
		}); err != nil {
			t.Fatalf("RecordForkHealEvent[%d]: %v", i, err)
		}
		if err := s.RecordForkHealAudit(&coordination.ForkHealAuditRecord{
			TraceID: traceID, GroupID: "g-cap", Step: "state_swap", Status: "completed", TimestampMs: int64(i + 1),
		}); err != nil {
			t.Fatalf("RecordForkHealAudit[%d]: %v", i, err)
		}
	}
	if _, err := s.PruneForkHealHistory(0, 2); err != nil {
		t.Fatalf("PruneForkHealHistory: %v", err)
	}
	events, err := s.ListForkHealEvents("g-cap", 10)
	if err != nil {
		t.Fatalf("ListForkHealEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events after cap prune = %d, want 2", len(events))
	}
}

func TestSQLiteCoordinationStorage_ForkHealingJob_LifecycleAndUniqueConstraint(t *testing.T) {
	s := setupTestStorage(t)

	// 1. Verify Save and GetActiveForkHealingJob
	job1 := &coordination.ForkHealingJob{
		JobID:             "job-active-1",
		GroupID:           "g-test-unique",
		TraceID:           "trace-1",
		Status:            "INITIATED",
		LosingBranchID:    "lose-1",
		WinningBranchID:   "win-1",
		ForkBaseEpoch:       2,
		LosingEpoch:         2,
		WinningEpoch:        5,
		LosingTreeHash:      []byte("lose-tree"),
		WinningTreeHash:     []byte("win-tree"),
		WinningCommitHash:   []byte("win-commit"),
		WinnerPeerID:        "carol",
		CreatedAtMs:         1000,
		UpdatedAtMs:         1000,
	}

	if err := s.SaveForkHealingJob(job1); err != nil {
		t.Fatalf("SaveForkHealingJob: %v", err)
	}

	active, err := s.GetActiveForkHealingJob("g-test-unique")
	if err != nil || active == nil {
		t.Fatalf("GetActiveForkHealingJob failed: %v", err)
	}
	if active.JobID != "job-active-1" || active.Status != "INITIATED" {
		t.Errorf("Unexpected active job field values: %+v", active)
	}

	byID, err := s.GetForkHealingJobByID("job-active-1")
	if err != nil || byID == nil {
		t.Fatalf("GetForkHealingJobByID failed: %v", err)
	}
	if byID.LosingEpoch != 2 || string(byID.LosingTreeHash) != "lose-tree" {
		t.Errorf("GetForkHealingJobByID fields mismatch: %+v", byID)
	}

	// 2. Verify UNIQUE active job constraint: cannot insert second active job for same group
	job2 := &coordination.ForkHealingJob{
		JobID:             "job-active-2",
		GroupID:           "g-test-unique",
		TraceID:           "trace-2",
		Status:            "SNAPSHOT_CREATED", // Also active
		WinningTreeHash:   []byte("win-tree-2"),
		WinningCommitHash: []byte("win-commit-2"),
		CreatedAtMs:         2000,
		UpdatedAtMs:         2000,
	}
	err = s.SaveForkHealingJob(job2)
	if err == nil {
		t.Fatal("expected UNIQUE constraint error for multiple active jobs on same group, got nil")
	}

	// 3. Mark first job as CLEANED (inactive) and verify we can now insert/activate another job
	job1.Status = "CLEANED"
	job1.UpdatedAtMs = 3000
	if err := s.SaveForkHealingJob(job1); err != nil {
		t.Fatalf("SaveForkHealingJob transition to CLEANED failed: %v", err)
	}

	// Check active job is now nil
	active, err = s.GetActiveForkHealingJob("g-test-unique")
	if err != nil {
		t.Fatalf("GetActiveForkHealingJob: %v", err)
	}
	if active != nil {
		t.Errorf("expected no active job, got %+v", active)
	}

	// Now we can successfully save the new active job
	if err := s.SaveForkHealingJob(job2); err != nil {
		t.Fatalf("Failed to save second active job after first was cleaned: %v", err)
	}

	active, _ = s.GetActiveForkHealingJob("g-test-unique")
	if active == nil || active.JobID != "job-active-2" {
		t.Errorf("expected active job to be job-active-2, got %+v", active)
	}

	// Clean up second job by deleting it
	if err := s.DeleteForkHealingJob("job-active-2"); err != nil {
		t.Fatalf("DeleteForkHealingJob: %v", err)
	}
	byID, _ = s.GetForkHealingJobByID("job-active-2")
	if byID != nil {
		t.Errorf("expected job to be deleted, found: %+v", byID)
	}
}

func TestSQLiteCoordinationStorage_ApplicationEventsAndPayloadShredding(t *testing.T) {
	s := setupTestStorage(t)

	// Persist job first
	job := &coordination.ForkHealingJob{
		JobID:       "job-ev-1",
		GroupID:     "g-ev",
		Status:      "STATE_SWAPPED",
		CreatedAtMs: 1000,
		UpdatedAtMs: 1000,
	}
	_ = s.SaveForkHealingJob(job)

	// Save application events
	ev1 := &coordination.ApplicationEvent{
		EventID:          "ev-1",
		JobID:            "job-ev-1",
		GroupID:          "g-ev",
		OriginalBranchID: "lose-branch",
		OriginalEpoch:    2,
		AuthorID:         "alice",
		EnvelopeHash:     []byte{1, 2, 3},
		PayloadSealed:    []byte("super-secret-encrypted-payload"),
		PayloadHash:      []byte{9, 9},
		SealKeyID:        "key-1",
		SealNonce:        []byte{5, 5, 5},
		HlcWallTimeMs:    2000,
		HlcCounter:       1,
		HlcNodeID:        "alice",
		Status:           "ORPHANED_OWN",
		CreatedAtMs:      1000,
		UpdatedAtMs:      1000,
	}

	ev2 := &coordination.ApplicationEvent{
		EventID:          "ev-2",
		JobID:            "job-ev-1",
		GroupID:          "g-ev",
		OriginalBranchID: "lose-branch",
		OriginalEpoch:    2,
		AuthorID:         "bob",
		EnvelopeHash:     []byte{4, 5, 6},
		PayloadSealed:    []byte("bob-sealed-stuff"),
		PayloadHash:      []byte{8, 8},
		Status:           "WAITING_AUTHOR_REPLAY",
		CreatedAtMs:      1000,
		UpdatedAtMs:      1000,
	}

	if err := s.SaveApplicationEvent(ev1); err != nil {
		t.Fatalf("SaveApplicationEvent 1: %v", err)
	}
	if err := s.SaveApplicationEvent(ev2); err != nil {
		t.Fatalf("SaveApplicationEvent 2: %v", err)
	}

	// List events
	list, err := s.ListApplicationEvents("job-ev-1")
	if err != nil {
		t.Fatalf("ListApplicationEvents: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 events, got %d", len(list))
	}

	// Verify order is HLC ascending (ev1 is 2000 HlcWallTimeMs, ev2 has 0)
	if list[0].EventID != "ev-2" || list[1].EventID != "ev-1" {
		t.Errorf("ordering is incorrect, got: %s then %s", list[0].EventID, list[1].EventID)
	}

	// Update status
	if err := s.UpdateApplicationEventStatus("ev-1", "REPLAYED"); err != nil {
		t.Fatalf("UpdateApplicationEventStatus: %v", err)
	}

	// ClearSealedPayloads (Shredding)
	if err := s.ClearSealedPayloads("job-ev-1"); err != nil {
		t.Fatalf("ClearSealedPayloads: %v", err)
	}

	// Fetch again to verify shredded fields are NULL while other metadata is intact
	list, _ = s.ListApplicationEvents("job-ev-1")
	for _, ev := range list {
		if ev.EventID == "ev-1" {
			if ev.PayloadSealed != nil || ev.SealNonce != nil || ev.SealKeyID != "" {
				t.Errorf("payload fields not shredded for REPLAYED event: %+v", ev)
			}
			if ev.AuthorID != "alice" || ev.Status != "REPLAYED" {
				t.Errorf("metadata corrupted during shredding: %+v", ev)
			}
		} else if ev.EventID == "ev-2" {
			if ev.PayloadSealed != nil {
				t.Errorf("payload fields not shredded for WAITING_AUTHOR_REPLAY event: %+v", ev)
			}
		}
	}
}

func TestSQLiteCoordinationStorage_OutboundReplayQueue(t *testing.T) {
	s := setupTestStorage(t)

	req1 := &coordination.OutboundReplay{
		ReplayOperationID:    "op-1",
		EventID:              "ev-1",
		JobID:                "job-out-1",
		GroupID:              "g-out",
		ReplayEnvelope:       []byte("envelope-data-1"),
		ReplayedEnvelopeHash: []byte{1, 1},
		Status:               "ENQUEUED",
		AttemptCount:         1,
		CreatedAtMs:          1000,
		UpdatedAtMs:          1000,
	}

	req2 := &coordination.OutboundReplay{
		ReplayOperationID:    "op-2",
		EventID:              "ev-2",
		JobID:                "job-out-1",
		GroupID:              "g-out",
		ReplayEnvelope:       []byte("envelope-data-2"),
		ReplayedEnvelopeHash: []byte{2, 2},
		Status:               "BROADCASTED",
		AttemptCount:         2,
		CreatedAtMs:          2000,
		UpdatedAtMs:          2000,
	}

	if err := s.SaveOutboundReplay(req1); err != nil {
		t.Fatalf("SaveOutboundReplay 1: %v", err)
	}
	if err := s.SaveOutboundReplay(req2); err != nil {
		t.Fatalf("SaveOutboundReplay 2: %v", err)
	}

	list, err := s.ListOutboundReplays("job-out-1")
	if err != nil || len(list) != 2 {
		t.Fatalf("ListOutboundReplays failed: %v (len=%d)", err, len(list))
	}

	var found1, found2 bool
	for _, r := range list {
		if r.ReplayOperationID == "op-1" {
			found1 = true
			if r.Status != "ENQUEUED" || string(r.ReplayEnvelope) != "envelope-data-1" {
				t.Errorf("field mismatch for op-1: %+v", r)
			}
		} else if r.ReplayOperationID == "op-2" {
			found2 = true
			if r.Status != "BROADCASTED" || r.AttemptCount != 2 {
				t.Errorf("field mismatch for op-2: %+v", r)
			}
		}
	}
	if !found1 || !found2 {
		t.Errorf("not all outbound records retrieved")
	}

	// Delete
	if err := s.DeleteOutboundReplaysForJob("job-out-1"); err != nil {
		t.Fatalf("DeleteOutboundReplaysForJob: %v", err)
	}

	list, _ = s.ListOutboundReplays("job-out-1")
	if len(list) != 0 {
		t.Errorf("expected outbox queue to be empty, got: %d records", len(list))
	}
}

func TestSQLiteCoordinationStorage_GetActiveForkHealingJob_ExcludesNewerCleaned(t *testing.T) {
	s := setupTestStorage(t)
	groupID := "g-cleaned-filter"

	// 1. Save an older active job
	jobActive := &coordination.ForkHealingJob{
		JobID:             "job-active-old",
		GroupID:           groupID,
		Status:            "INITIATED",
		WinningTreeHash:   []byte("win-tree-1"),
		WinningCommitHash: []byte("win-commit-1"),
		CreatedAtMs:       1000,
		UpdatedAtMs:       1000,
	}
	if err := s.SaveForkHealingJob(jobActive); err != nil {
		t.Fatalf("SaveForkHealingJob: %v", err)
	}

	// 2. Save a newer CLEANED (inactive) job
	jobCleaned := &coordination.ForkHealingJob{
		JobID:             "job-cleaned-new",
		GroupID:           groupID,
		Status:            "CLEANED",
		WinningTreeHash:   []byte("win-tree-2"),
		WinningCommitHash: []byte("win-commit-2"),
		CreatedAtMs:       5000, // Newer
		UpdatedAtMs:       5000,
	}
	if err := s.SaveForkHealingJob(jobCleaned); err != nil {
		t.Fatalf("SaveForkHealingJob: %v", err)
	}

	// 3. GetActiveForkHealingJob must return jobActive ("job-active-old"), NOT jobCleaned ("job-cleaned-new")
	active, err := s.GetActiveForkHealingJob(groupID)
	if err != nil {
		t.Fatalf("GetActiveForkHealingJob: %v", err)
	}
	if active == nil {
		t.Fatalf("expected to retrieve active job, got nil")
	}
	if active.JobID != "job-active-old" {
		t.Errorf("GetActiveForkHealingJob returned incorrect job: got %s, want job-active-old", active.JobID)
	}
}
