package store

import (
	"testing"
	"time"

	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestGroupLifecycle_PurgeGroupMetadata(t *testing.T) {
	d := setupTestDB(t)
	s := NewSQLiteCoordinationStorage(d)

	groupID := "purge-test-group"

	// 1. Seed some group records
	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    groupID,
		GroupState: []byte("state-data"),
		MyRole:     coordination.RoleCreator,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}

	// Seed group members
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     groupID,
		PeerID:      "peer-1",
		DisplayName: "Peer One",
		Role:        "member",
		Status:      GroupMemberStatusActive,
		JoinedAt:    time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember: %v", err)
	}

	// Seed coordination state
	if err := s.SaveCoordState(&coordination.CoordState{
		GroupID:     groupID,
		TokenHolder: "peer-1",
		ActiveView:  []peer.ID{"peer-1"},
	}); err != nil {
		t.Fatalf("SaveCoordState: %v", err)
	}

	// Seed stored messages (THIS MUST NOT BE PURGED)
	if err := s.SaveMessage(&coordination.StoredMessage{
		GroupID:      groupID,
		Epoch:        1,
		SenderID:     "peer-1",
		Content:      []byte("historic chat message"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: 1000, Counter: 0, NodeID: "peer-1"},
		EnvelopeHash: []byte("env-hash-1"),
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	// Verify they all exist
	hasGroup, err := d.HasGroup(groupID)
	if err != nil || !hasGroup {
		t.Fatalf("HasGroup initially = %v, err = %v", hasGroup, err)
	}

	members, err := d.ListGroupMembers(groupID, GroupMemberStatusActive)
	if err != nil || len(members) != 1 {
		t.Fatalf("members len = %v, err = %v", len(members), err)
	}

	cState, err := s.GetCoordState(groupID)
	if err != nil || cState == nil {
		t.Fatalf("GetCoordState: %v", err)
	}

	msgs, err := s.GetMessagesSince(groupID, coordination.HLCTimestamp{})
	if err != nil || len(msgs) != 1 {
		t.Fatalf("messages count = %v, err = %v", len(msgs), err)
	}

	// 2. Perform the purge
	if err := d.PurgeGroupMetadata(groupID); err != nil {
		t.Fatalf("PurgeGroupMetadata: %v", err)
	}

	// 3. Verify cryptographic and coordination metadata are completely gone
	hasGroup, err = d.HasGroup(groupID)
	if err != nil || hasGroup {
		t.Fatalf("expected HasGroup to be false after purge, got %v", hasGroup)
	}

	members, err = d.ListGroupMembers(groupID, GroupMemberStatusActive)
	if err != nil || len(members) != 0 {
		t.Fatalf("expected members to be empty, got %d", len(members))
	}

	_, err = s.GetCoordState(groupID)
	if err != coordination.ErrGroupNotFound {
		t.Fatalf("expected GetCoordState to return ErrGroupNotFound, got %v", err)
	}

	// 4. Verify message history remains perfectly preserved
	msgs, err = s.GetMessagesSince(groupID, coordination.HLCTimestamp{})
	if err != nil || len(msgs) != 1 || string(msgs[0].Content) != "historic chat message" {
		t.Fatalf("expected messages to remain preserved, got %d messages: %v", len(msgs), err)
	}
}

func TestGroupLifecycle_MarkLeftAndListActive(t *testing.T) {
	d := setupTestDB(t)
	s := NewSQLiteCoordinationStorage(d)

	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "active-group",
		GroupState: []byte("state-a"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("SaveGroupRecord active: %v", err)
	}
	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "left-group",
		GroupState: []byte("state-l"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("SaveGroupRecord left: %v", err)
	}

	if err := d.MarkGroupLeft("left-group"); err != nil {
		t.Fatalf("MarkGroupLeft: %v", err)
	}
	active, err := d.IsGroupActive("left-group")
	if err != nil {
		t.Fatalf("IsGroupActive: %v", err)
	}
	if active {
		t.Fatalf("left-group reported active")
	}

	groups, err := s.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].GroupID != "active-group" {
		t.Fatalf("active group listing mismatch: %+v", groups)
	}
}

func TestGroupLifecycle_BackupRestorePreservesLeftState(t *testing.T) {
	d := setupTestDB(t)
	s := NewSQLiteCoordinationStorage(d)

	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "left-group",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := d.MarkGroupLeft("left-group"); err != nil {
		t.Fatalf("MarkGroupLeft: %v", err)
	}

	backup, err := d.GetAllGroupsForBackup()
	if err != nil {
		t.Fatalf("GetAllGroupsForBackup: %v", err)
	}
	if len(backup) != 1 || backup[0].LifecycleStatus != GroupLifecycleLeft || backup[0].LeftAt == 0 {
		t.Fatalf("backup lifecycle mismatch: %+v", backup)
	}

	restored := setupTestDB(t)
	if err := restored.RestoreGroupsFromBackup(backup); err != nil {
		t.Fatalf("RestoreGroupsFromBackup: %v", err)
	}
	active, err := restored.IsGroupActive("left-group")
	if err != nil {
		t.Fatalf("restored IsGroupActive: %v", err)
	}
	if active {
		t.Fatalf("restored left group reported active")
	}
}

func TestGroupLifecycle_BackupRestorePreservesChannelCategory(t *testing.T) {
	d := setupTestDB(t)
	s := NewSQLiteCoordinationStorage(d)
	now := time.Now()
	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "chan-1",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleMember,
		GroupType:  "channel",
		CategoryID: "cat-test-restore",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	backup, err := d.GetAllGroupsForBackup()
	if err != nil {
		t.Fatalf("GetAllGroupsForBackup: %v", err)
	}
	if len(backup) != 1 || backup[0].CategoryID != "cat-test-restore" || backup[0].GroupType != "channel" {
		t.Fatalf("backup metadata: %+v", backup[0])
	}
	restored := setupTestDB(t)
	if err := restored.RestoreGroupsFromBackup(backup); err != nil {
		t.Fatalf("RestoreGroupsFromBackup: %v", err)
	}
	rs := NewSQLiteCoordinationStorage(restored)
	rec, err := rs.GetGroupRecord("chan-1")
	if err != nil {
		t.Fatalf("GetGroupRecord: %v", err)
	}
	if rec.GroupType != "channel" || rec.CategoryID != "cat-test-restore" {
		t.Fatalf("restored group: type=%q category=%q", rec.GroupType, rec.CategoryID)
	}
}

func TestListJoinedGroupChatIDsForReplication_GroupTypeOnly(t *testing.T) {
	d := setupTestDB(t)
	s := NewSQLiteCoordinationStorage(d)
	now := time.Now()
	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "g-repl-1",
		GroupState: []byte{1},
		MyRole:     coordination.RoleMember,
		GroupType:  "group",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord group: %v", err)
	}
	if err := s.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "chan-repl-x",
		GroupState: []byte{2},
		MyRole:     coordination.RoleMember,
		GroupType:  "channel",
		CategoryID: "c1",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord channel: %v", err)
	}
	ids, err := d.ListJoinedGroupChatIDsForReplication(256)
	if err != nil {
		t.Fatalf("ListJoinedGroupChatIDsForReplication: %v", err)
	}
	if len(ids) != 1 || ids[0] != "g-repl-1" {
		t.Fatalf("ids=%v want [g-repl-1]", ids)
	}
}
