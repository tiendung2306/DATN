package store

import "testing"

func setupTestDB(t *testing.T) *Database {
	t.Helper()
	d, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestPendingInvite_SaveListAndDeduplicate(t *testing.T) {
	d := setupTestDB(t)

	inv := &PendingInvite{
		GroupID:       "group-1",
		WelcomeBytes:  []byte("welcome-1"),
		SourcePeerID:  "peer-a",
		InviterPeerID: "peer-a",
	}
	if err := d.SavePendingInvite(inv); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}
	if err := d.SavePendingInvite(&PendingInvite{
		GroupID:      "group-1",
		WelcomeBytes: []byte("welcome-1"),
		SourcePeerID: "peer-b",
	}); err != nil {
		t.Fatalf("SavePendingInvite duplicate: %v", err)
	}

	rows, err := d.ListPendingInvites(false)
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("pending invite count = %d, want 1", len(rows))
	}
	if rows[0].ID != PendingInviteID("group-1", []byte("welcome-1")) {
		t.Fatalf("invite id mismatch: got %q", rows[0].ID)
	}
	if rows[0].Status != PendingInviteStatusPending {
		t.Fatalf("status = %q, want pending", rows[0].Status)
	}
}

func TestPendingInvite_StatusTransitions(t *testing.T) {
	d := setupTestDB(t)
	id := PendingInviteID("group-1", []byte("welcome-1"))
	if err := d.SavePendingInvite(&PendingInvite{
		GroupID:      "group-1",
		WelcomeBytes: []byte("welcome-1"),
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}

	if err := d.MarkPendingInviteRejected(id); err != nil {
		t.Fatalf("MarkPendingInviteRejected: %v", err)
	}
	pending, err := d.ListPendingInvites(false)
	if err != nil {
		t.Fatalf("ListPendingInvites(false): %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending count after reject = %d, want 0", len(pending))
	}

	all, err := d.ListPendingInvites(true)
	if err != nil {
		t.Fatalf("ListPendingInvites(true): %v", err)
	}
	if len(all) != 1 || all[0].Status != PendingInviteStatusRejected {
		t.Fatalf("terminal invite mismatch: %+v", all)
	}

	if err := d.SavePendingInvite(&PendingInvite{
		GroupID:      "group-1",
		WelcomeBytes: []byte("welcome-1"),
	}); err != nil {
		t.Fatalf("SavePendingInvite after reject: %v", err)
	}
	got, err := d.GetPendingInvite(id)
	if err != nil {
		t.Fatalf("GetPendingInvite: %v", err)
	}
	if got.Status != PendingInviteStatusRejected {
		t.Fatalf("status after duplicate save = %q, want rejected", got.Status)
	}
}

func TestStoredWelcome_ListForInvitee(t *testing.T) {
	d := setupTestDB(t)
	if err := d.SaveStoredWelcome("peer-a", "group-1", []byte("welcome-1"), "peer-store"); err != nil {
		t.Fatalf("SaveStoredWelcome: %v", err)
	}
	if err := d.SaveStoredWelcome("peer-b", "group-2", []byte("welcome-2"), "peer-store"); err != nil {
		t.Fatalf("SaveStoredWelcome other: %v", err)
	}

	rows, err := d.ListStoredWelcomesFor("peer-a")
	if err != nil {
		t.Fatalf("ListStoredWelcomesFor: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("stored welcome count = %d, want 1", len(rows))
	}
	if rows[0].GroupID != "group-1" || string(rows[0].WelcomeBytes) != "welcome-1" {
		t.Fatalf("stored welcome mismatch: %+v", rows[0])
	}
}
