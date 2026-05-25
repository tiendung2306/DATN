package store

import "testing"

func TestGroupEventLogAppendAndListNewestFirst(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	if _, err := db.AppendGroupEvent(GroupEventLogRecord{
		GroupID:      "g1",
		EventType:    "proposal_received",
		ActorPeerID:  "peer-a",
		TargetPeerID: "peer-b",
		Epoch:        1,
		PayloadJSON:  []byte(`{"proposal_type":"add"}`),
		CreatedAtMs:  1000,
	}); err != nil {
		t.Fatalf("AppendGroupEvent #1: %v", err)
	}
	if _, err := db.AppendGroupEvent(GroupEventLogRecord{
		GroupID:      "g1",
		EventType:    "commit_issued",
		ActorPeerID:  "peer-a",
		TargetPeerID: "",
		Epoch:        2,
		PayloadJSON:  []byte(`{"new_epoch":2}`),
		CreatedAtMs:  2000,
	}); err != nil {
		t.Fatalf("AppendGroupEvent #2: %v", err)
	}
	if _, err := db.AppendGroupEvent(GroupEventLogRecord{
		GroupID:      "g2",
		EventType:    "group_created",
		ActorPeerID:  "peer-z",
		TargetPeerID: "",
		Epoch:        0,
		PayloadJSON:  []byte(`{"initial_epoch":0}`),
		CreatedAtMs:  3000,
	}); err != nil {
		t.Fatalf("AppendGroupEvent #3: %v", err)
	}

	rows, err := db.ListGroupEvents("g1", 10)
	if err != nil {
		t.Fatalf("ListGroupEvents: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(rows))
	}
	if rows[0].EventType != "commit_issued" || rows[1].EventType != "proposal_received" {
		t.Fatalf("unexpected order: %+v", rows)
	}

	limited, err := db.ListGroupEvents("g1", 1)
	if err != nil {
		t.Fatalf("ListGroupEvents limit=1: %v", err)
	}
	if len(limited) != 1 || limited[0].EventType != "commit_issued" {
		t.Fatalf("unexpected limited rows: %+v", limited)
	}
}
