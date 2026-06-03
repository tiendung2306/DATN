//go:build business_integration

package service

import (
	"testing"
	"time"

	"app/coordination"
)

func TestBusinessP1_Diagnostics_ExposeReplayAndOperationPressure(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "diag-replay-pressure"
	if err := rt.CreateGroupChat(gid, "dm", ""); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}

	rt.mu.RLock()
	cs := rt.coordStorage
	coord := rt.coordinators[gid]
	rt.mu.RUnlock()
	if cs == nil || coord == nil {
		t.Fatal("coordination stack not ready")
	}
	for i := 0; i < 50; i++ {
		if coord.GetOperationalMode() == coordination.ModeLive {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)

	ts := coordination.HLCTimestamp{WallTimeMs: time.Now().UnixMilli(), Counter: 1, NodeID: "diag"}
	env1 := []byte(`{"type":"application","group_id":"diag-replay-pressure","epoch":0,"from":"peer-a","ts":{"l":1,"c":0,"id":"a"},"payload":{"ciphertext":"AQ=="}}`)
	env2 := []byte(`{"type":"application","group_id":"diag-replay-pressure","epoch":0,"from":"peer-b","ts":{"l":2,"c":0,"id":"b"},"payload":{"ciphertext":"Ag=="}}`)

	if _, err := cs.AppendEnvelopeWithSource(gid, coordination.MsgApplication, 0, ts, env1, "test"); err != nil {
		t.Fatalf("AppendEnvelopeWithSource env1: %v", err)
	}
	if _, err := cs.AppendEnvelopeWithSource(gid, coordination.MsgApplication, 0, ts, env2, "test"); err != nil {
		t.Fatalf("AppendEnvelopeWithSource env2: %v", err)
	}
	recs, err := cs.GetEnvelopesSince(gid, 0, 10)
	if err != nil {
		t.Fatalf("GetEnvelopesSince: %v", err)
	}
	if len(recs) < 2 {
		t.Fatalf("expected 2 envelope records, got %d", len(recs))
	}
	if err := cs.MarkEnvelopeReplayState(gid, recs[0].EnvelopeHash, coordination.ReplayStateBlockedStaleRequiresSnapshot, "stale", time.Now()); err != nil {
		t.Fatalf("MarkEnvelopeReplayState stale: %v", err)
	}
	if err := cs.MarkEnvelopeReplayState(gid, recs[1].EnvelopeHash, coordination.ReplayStateBlockedDecryptFailed, "decrypt", time.Now()); err != nil {
		t.Fatalf("MarkEnvelopeReplayState decrypt: %v", err)
	}

	coord.SetOperationalMode(coordination.ModeCatchingUp)
	coord.IncrementSyncRetryAttempts()
	coord.IncrementSyncRetryAttempts()

	now := time.Now()
	if err := cs.SavePendingOperation(&coordination.PendingOperation{
		OperationID:     "op-proposed",
		GroupID:         gid,
		OpType:          "ADD_MEMBER",
		SemanticPayload: []byte("{}"),
		Status:          "PROPOSED",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("SavePendingOperation proposed: %v", err)
	}
	if err := cs.SavePendingOperation(&coordination.PendingOperation{
		OperationID:     "op-exhausted",
		GroupID:         gid,
		OpType:          "REMOVE_MEMBER",
		SemanticPayload: []byte("{}"),
		Status:          "FAILED_RETRY_EXHAUSTED",
		RetryCount:      5,
		CreatedAt:       now.Add(time.Millisecond),
		UpdatedAt:       now.Add(time.Millisecond),
	}); err != nil {
		t.Fatalf("SavePendingOperation exhausted: %v", err)
	}

	snapshot, err := rt.GetDiagnosticsSnapshot()
	if err != nil {
		t.Fatalf("GetDiagnosticsSnapshot: %v", err)
	}
	var group DiagnosticsGroupSnapshot
	found := false
	for _, item := range snapshot.Groups {
		if item.GroupID == gid {
			group = item
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("group %q missing from diagnostics", gid)
	}
	if group.OperationalMode != string(coordination.ModeCatchingUp) {
		t.Fatalf("OperationalMode=%q want %q", group.OperationalMode, coordination.ModeCatchingUp)
	}
	if group.SyncRetryAttempts != 2 {
		t.Fatalf("SyncRetryAttempts=%d want 2", group.SyncRetryAttempts)
	}
	if group.PendingEnvelopes != 0 {
		t.Fatalf("PendingEnvelopes=%d want 0 because only terminal blocked states remain", group.PendingEnvelopes)
	}
	if group.ReplayStateCounts[string(coordination.ReplayStateBlockedStaleRequiresSnapshot)] != 1 {
		t.Fatalf("stale blocked count=%d want 1", group.ReplayStateCounts[string(coordination.ReplayStateBlockedStaleRequiresSnapshot)])
	}
	if group.ReplayStateCounts[string(coordination.ReplayStateBlockedDecryptFailed)] != 1 {
		t.Fatalf("decrypt blocked count=%d want 1", group.ReplayStateCounts[string(coordination.ReplayStateBlockedDecryptFailed)])
	}
	if group.PendingOperations != 2 {
		t.Fatalf("PendingOperations=%d want 2", group.PendingOperations)
	}
	if group.OperationStatuses["PROPOSED"] != 1 {
		t.Fatalf("PROPOSED count=%d want 1", group.OperationStatuses["PROPOSED"])
	}
	if group.OperationStatuses["FAILED_RETRY_EXHAUSTED"] != 1 {
		t.Fatalf("FAILED_RETRY_EXHAUSTED count=%d want 1", group.OperationStatuses["FAILED_RETRY_EXHAUSTED"])
	}
}
