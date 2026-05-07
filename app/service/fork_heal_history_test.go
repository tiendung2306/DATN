package service

import (
	"testing"

	"app/coordination"
)

func TestGetForkHealHistory_ReturnsEventsWithAudit(t *testing.T) {
	rt := setupMembershipRuntime(t)
	if err := rt.coordStorage.RecordForkHealEvent(&coordination.ForkHealEventRecord{
		TraceID:              "trace-h1",
		GroupID:              "g-h1",
		WinnerPeerID:         "peer-w",
		WinnerEpoch:          3,
		NewEpoch:             4,
		Outcome:              "success",
		PartitionStartedAtMs: 1000,
		ScheduledAtMs:        1100,
		StartedAtMs:          1200,
		CompletedAtMs:        1300,
		DurationMs:           100,
		TotalMs:              200,
		ReplayedMessageCount: 1,
		WinnerTreeHash:       []byte("winner"),
		NewTreeHash:          []byte("new"),
	}); err != nil {
		t.Fatalf("RecordForkHealEvent: %v", err)
	}
	if err := rt.coordStorage.RecordForkHealAudit(&coordination.ForkHealAuditRecord{
		TraceID: "trace-h1", GroupID: "g-h1", Step: "external_join", Status: "completed", TimestampMs: 1250, DurationMs: 25,
	}); err != nil {
		t.Fatalf("RecordForkHealAudit: %v", err)
	}

	rows, err := rt.GetForkHealHistory("g-h1", 10)
	if err != nil {
		t.Fatalf("GetForkHealHistory: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("history len = %d, want 1", len(rows))
	}
	if rows[0].TraceID != "trace-h1" || rows[0].Outcome != "success" {
		t.Fatalf("unexpected history row: %+v", rows[0])
	}
	if len(rows[0].Audit) != 1 {
		t.Fatalf("audit len = %d, want 1", len(rows[0].Audit))
	}
	if rows[0].Audit[0].Step != "external_join" || rows[0].Audit[0].Status != "completed" {
		t.Fatalf("unexpected audit row: %+v", rows[0].Audit[0])
	}
}
