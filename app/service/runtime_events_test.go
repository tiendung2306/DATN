package service

import (
	"encoding/json"
	"testing"

	"app/adapter/store"
	"app/config"
	"app/coordination"
)

func TestRuntimeEmitAddsAggregateRevisionAndDurableLog(t *testing.T) {
	db, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	rt := &Runtime{
		cfg:            &config.Config{RuntimeEventReplay: true},
		db:             db,
		eventRevisions: map[string]int64{},
	}

	rt.emit("group:members_changed", map[string]interface{}{"group_id": "g1"})
	rt.emit("group:members_changed", map[string]interface{}{"group_id": "g1"})

	revs, err := rt.GetAggregateRevisions()
	if err != nil {
		t.Fatalf("GetAggregateRevisions: %v", err)
	}
	if got := revs["group:g1"]; got != 2 {
		t.Fatalf("group:g1 revision=%d, want 2", got)
	}

	events, err := rt.GetRuntimeEventsSince(0, 100)
	if err != nil {
		t.Fatalf("GetRuntimeEventsSince: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len=%d, want 2", len(events))
	}
	last := events[len(events)-1]
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(last.PayloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["aggregate"] != "group:g1" {
		t.Fatalf("aggregate payload=%v, want group:g1", payload["aggregate"])
	}
	if payload["revision"] != float64(2) {
		t.Fatalf("revision payload=%v, want 2", payload["revision"])
	}
}

func TestRuntimeEmitReplayBlockedUsesGroupAggregate(t *testing.T) {
	db, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	rt := &Runtime{
		cfg:            &config.Config{RuntimeEventReplay: true},
		db:             db,
		eventRevisions: map[string]int64{},
	}

	rt.emit("group:replay_blocked", map[string]interface{}{"group_id": "g-replay", "reason": "stale"})

	revs, err := rt.GetAggregateRevisions()
	if err != nil {
		t.Fatalf("GetAggregateRevisions: %v", err)
	}
	if got := revs["group:g-replay"]; got != 1 {
		t.Fatalf("group:g-replay revision=%d, want 1", got)
	}
}

func TestPendingOperationAuditHandler_EmitsRuntimeEvent(t *testing.T) {
	db, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	rt := &Runtime{
		cfg:            &config.Config{RuntimeEventReplay: true},
		db:             db,
		eventRevisions: map[string]int64{},
	}

	handler := rt.makePendingOperationAuditHandler("g-op")
	handler(coordination.PendingOperationAuditSummary{
		GroupID:           "g-op",
		OperationID:       "op-1",
		OpType:            "ADD_MEMBER",
		TargetPeerID:      "peer-2",
		Stage:             "retry_exhausted",
		RetryCount:        5,
		CurrentEpoch:      4,
		PreconditionEpoch: 3,
		LastError:         "max retries exceeded",
	})

	events, err := rt.GetRuntimeEventsSince(0, 10)
	if err != nil {
		t.Fatalf("GetRuntimeEventsSince: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d, want 1", len(events))
	}
	if events[0].Topic != "group:operation_retry_exhausted" {
		t.Fatalf("topic=%q, want group:operation_retry_exhausted", events[0].Topic)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(events[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["aggregate"] != "group:g-op" {
		t.Fatalf("aggregate=%v, want group:g-op", payload["aggregate"])
	}
}
