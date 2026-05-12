//go:build business_integration

// Sprint 5 — BI-099–BI-101 runtime event replay APIs.

package service

import (
	"encoding/json"
	"testing"
)

func TestBusinessP1_Sprint5_BI099_GetRuntimeEventCursor_MonotonicAfterEmit(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLSAndEventReplay(t)
	cur0, err := rt.GetRuntimeEventCursor()
	if err != nil {
		t.Fatalf("GetRuntimeEventCursor: %v", err)
	}
	rt.emit("group:message", map[string]interface{}{"group_id": "evt-g1", "text": "hi"})
	cur1, err := rt.GetRuntimeEventCursor()
	if err != nil {
		t.Fatalf("GetRuntimeEventCursor: %v", err)
	}
	if cur1 <= cur0 {
		t.Fatalf("cursor %d not > %d after emit", cur1, cur0)
	}
}

func TestBusinessP1_Sprint5_BI100_GetRuntimeEventsSince_RespectsCursor(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLSAndEventReplay(t)
	rt.emit("group:joined", map[string]interface{}{"group_id": "evt-g2"})
	rt.emit("group:left", map[string]interface{}{"group_id": "evt-g2"})
	all, err := rt.GetRuntimeEventsSince(0, 50)
	if err != nil {
		t.Fatalf("GetRuntimeEventsSince: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("want >=2 events, got %d", len(all))
	}
	lastSeq := all[len(all)-1].Seq
	since, err := rt.GetRuntimeEventsSince(lastSeq, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range since {
		if ev.Seq <= lastSeq {
			t.Fatalf("expected seq > %d, got %d", lastSeq, ev.Seq)
		}
	}
}

func TestBusinessP1_Sprint5_BI101_GetAggregateRevisions_Updates(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLSAndEventReplay(t)
	rt.emit("channel_categories:changed", map[string]interface{}{"reason": "test"})
	before, err := rt.GetAggregateRevisions()
	if err != nil {
		t.Fatalf("GetAggregateRevisions: %v", err)
	}
	if before["workspace_categories"] == 0 {
		t.Fatalf("expected workspace_categories revision > 0, got %v", before)
	}
	rt.emit("channel_categories:changed", map[string]interface{}{"reason": "test2"})
	after, err := rt.GetAggregateRevisions()
	if err != nil {
		t.Fatal(err)
	}
	if after["workspace_categories"] <= before["workspace_categories"] {
		t.Fatalf("revision did not increase: before=%v after=%v", before, after)
	}
	// Payload JSON is durable when replay enabled
	events, err := rt.GetRuntimeEventsSince(0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ev := range events {
		if ev.Topic != "channel_categories:changed" {
			continue
		}
		var payload map[string]interface{}
		if json.Unmarshal([]byte(ev.PayloadJSON), &payload) != nil {
			continue
		}
		if payload["aggregate"] == "workspace_categories" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no durable channel_categories event with workspace_categories aggregate")
	}
}
