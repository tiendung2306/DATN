package store

import (
	"testing"
	"time"

	"app/coordination"
)

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
