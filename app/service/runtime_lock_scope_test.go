package service

import (
	"context"
	"testing"
	"time"

	"app/adapter/store"
	"app/config"
	"app/coordination"
)

func TestGetOfflineSyncStatus_DoesNotBlockEmitWhileDBConnBusy(t *testing.T) {
	db, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	rt := &Runtime{
		cfg:            &config.Config{RuntimeEventReplay: false},
		db:             db,
		coordStorage:   store.NewSQLiteCoordinationStorage(db),
		coordinators:   map[string]*coordination.Coordinator{"g1": nil},
		eventRevisions: map[string]int64{},
	}

	conn, err := db.Conn.Conn(context.Background())
	if err != nil {
		t.Fatalf("checkout conn: %v", err)
	}

	statusDone := make(chan struct{})
	go func() {
		defer close(statusDone)
		_, _ = rt.GetOfflineSyncStatus()
	}()
	time.Sleep(50 * time.Millisecond)

	emitDone := make(chan struct{})
	go func() {
		defer close(emitDone)
		rt.emit("group:message", map[string]interface{}{"group_id": "g1"})
	}()

	select {
	case <-emitDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("emit blocked while GetOfflineSyncStatus waited on the database connection")
	}

	_ = conn.Close()

	select {
	case <-statusDone:
	case <-time.After(2 * time.Second):
		t.Fatal("GetOfflineSyncStatus did not finish after releasing the database connection")
	}
}

func TestGetDiagnosticsSnapshot_DoesNotBlockEmitWhileDBConnBusy(t *testing.T) {
	db, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	rt := &Runtime{
		cfg:            &config.Config{RuntimeEventReplay: false},
		db:             db,
		eventRevisions: map[string]int64{},
	}

	conn, err := db.Conn.Conn(context.Background())
	if err != nil {
		t.Fatalf("checkout conn: %v", err)
	}

	diagDone := make(chan struct{})
	go func() {
		defer close(diagDone)
		_, _ = rt.GetDiagnosticsSnapshot()
	}()
	time.Sleep(50 * time.Millisecond)

	emitDone := make(chan struct{})
	go func() {
		defer close(emitDone)
		rt.emit("node:status", map[string]interface{}{"reason": "test"})
	}()

	select {
	case <-emitDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("emit blocked while GetDiagnosticsSnapshot waited on the database connection")
	}

	_ = conn.Close()

	select {
	case <-diagDone:
	case <-time.After(2 * time.Second):
		t.Fatal("GetDiagnosticsSnapshot did not finish after releasing the database connection")
	}
}
