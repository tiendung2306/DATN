package store

import "testing"

func TestGroupMembers_UpsertListAndLeft(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     "g-1",
		PeerID:      "peer-a",
		DisplayName: "Alice",
		Role:        "creator",
		Status:      GroupMemberStatusActive,
		Source:      "create",
	}); err != nil {
		t.Fatalf("UpsertGroupMember first: %v", err)
	}
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     "g-1",
		PeerID:      "peer-a",
		DisplayName: "Alice Updated",
		Role:        "creator",
		Status:      GroupMemberStatusActive,
		Source:      "profile-refresh",
	}); err != nil {
		t.Fatalf("UpsertGroupMember second: %v", err)
	}
	rows, err := d.ListGroupMembers("g-1", GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers(active): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1", len(rows))
	}
	if rows[0].DisplayName != "Alice Updated" {
		t.Fatalf("display name = %q, want %q", rows[0].DisplayName, "Alice Updated")
	}
	if err := d.MarkGroupMemberLeft("g-1", "peer-a", 0); err != nil {
		t.Fatalf("MarkGroupMemberLeft: %v", err)
	}
	active, err := d.ListGroupMembers("g-1", GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers(active) after left: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active rows after left = %d, want 0", len(active))
	}
	all, err := d.ListGroupMembers("g-1")
	if err != nil {
		t.Fatalf("ListGroupMembers(all): %v", err)
	}
	if len(all) != 1 || all[0].Status != GroupMemberStatusLeft {
		t.Fatalf("all rows mismatch: %+v", all)
	}
}

func TestGroupMembers_UpdateDisplayNameByPeer(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     "g-2",
		PeerID:      "peer-b",
		DisplayName: "",
		Role:        "member",
		Status:      GroupMemberStatusActive,
		Source:      "invite",
	}); err != nil {
		t.Fatalf("UpsertGroupMember: %v", err)
	}
	if err := d.UpdateGroupMemberDisplayNameByPeer("peer-b", "Bob"); err != nil {
		t.Fatalf("UpdateGroupMemberDisplayNameByPeer: %v", err)
	}
	rows, err := d.ListGroupMembers("g-2", GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	if len(rows) != 1 || rows[0].DisplayName != "Bob" {
		t.Fatalf("display name update mismatch: %+v", rows)
	}
}
