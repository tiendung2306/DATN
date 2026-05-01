package service

import (
	"encoding/json"
	"testing"

	"app/adapter/store"
	"app/config"
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
