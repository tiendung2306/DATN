package db

import (
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
		ActiveView:       []peer.ID{"peer-a", "peer-b"},
		TokenHolder:      "peer-a",
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
	if got.TokenHolder != "peer-a" {
		t.Errorf("TokenHolder = %q", got.TokenHolder)
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
			Timestamp: coordination.HLCTimestamp{WallTimeMs: 1000, Counter: 0, NodeID: "alice"}},
		{GroupID: "mg-1", Epoch: 1, SenderID: "bob", Content: []byte("world"),
			Timestamp: coordination.HLCTimestamp{WallTimeMs: 1000, Counter: 1, NodeID: "bob"}},
		{GroupID: "mg-1", Epoch: 2, SenderID: "alice", Content: []byte("epoch2"),
			Timestamp: coordination.HLCTimestamp{WallTimeMs: 2000, Counter: 0, NodeID: "alice"}},
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
