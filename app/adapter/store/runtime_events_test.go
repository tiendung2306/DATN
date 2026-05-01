package store

import (
	"testing"
	"time"
)

func TestRuntimeEventLogAppendAndList(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	seq1, err := db.AppendRuntimeEvent(RuntimeEventRecord{
		Topic:       "group:members_changed",
		Aggregate:   "group:g1",
		AggregateID: "g1",
		Revision:    1,
		PayloadJSON: []byte(`{"group_id":"g1","revision":1}`),
		CreatedAt:   time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("AppendRuntimeEvent #1: %v", err)
	}
	seq2, err := db.AppendRuntimeEvent(RuntimeEventRecord{
		Topic:       "group:members_changed",
		Aggregate:   "group:g1",
		AggregateID: "g1",
		Revision:    2,
		PayloadJSON: []byte(`{"group_id":"g1","revision":2}`),
		CreatedAt:   time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("AppendRuntimeEvent #2: %v", err)
	}
	if seq2 <= seq1 {
		t.Fatalf("seq monotonic violation: seq1=%d seq2=%d", seq1, seq2)
	}

	rows, err := db.ListRuntimeEventsSince(0, 100)
	if err != nil {
		t.Fatalf("ListRuntimeEventsSince: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(rows))
	}
	if rows[0].Revision != 1 || rows[1].Revision != 2 {
		t.Fatalf("unexpected revisions: %+v", rows)
	}
}
