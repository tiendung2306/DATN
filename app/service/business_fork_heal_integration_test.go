//go:build business_integration

// BI-107 fork-heal history API (storage seeded via coordination records).

package service

import (
	"testing"

	"app/coordination"
)

func TestBusinessP1_Sprint6_BI107_GetForkHealHistory(t *testing.T) {
	rt := setupMembershipRuntime(t)
	if err := rt.coordStorage.RecordForkHealEvent(&coordination.ForkHealEventRecord{
		TraceID:              "trace-bi107",
		GroupID:              "g-bi107",
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
		TraceID: "trace-bi107", GroupID: "g-bi107", Step: "external_join", Status: "completed", TimestampMs: 1250, DurationMs: 25,
	}); err != nil {
		t.Fatalf("RecordForkHealAudit: %v", err)
	}

	rows, err := rt.GetForkHealHistory("g-bi107", 10)
	if err != nil {
		t.Fatalf("GetForkHealHistory: %v", err)
	}
	if len(rows) != 1 || rows[0].TraceID != "trace-bi107" {
		t.Fatalf("unexpected %+v", rows)
	}
}
