package service

import (
	"testing"

	"app/adapter/store"
)

func TestSavePendingInviteFromWelcome_RejectTombstoneStickyOnPassiveRefresh(t *testing.T) {
	rt := setupMembershipRuntime(t)
	groupID := "group-reject-sticky"
	welcome := []byte("welcome-1")
	if err := rt.savePendingInviteFromWelcome(groupID, "group", welcome, "peer-a", true); err != nil {
		t.Fatalf("savePendingInviteFromWelcome initial: %v", err)
	}
	inviteID := store.PendingInviteID(groupID, welcome)
	if err := rt.RejectInvite(inviteID); err != nil {
		t.Fatalf("RejectInvite: %v", err)
	}
	if err := rt.savePendingInviteFromWelcome(groupID, "group", welcome, "peer-a", false); err != nil {
		t.Fatalf("savePendingInviteFromWelcome passive: %v", err)
	}
	rows, err := rt.db.ListPendingInvites(false)
	if err != nil {
		t.Fatalf("ListPendingInvites(false): %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("pending invites resurrected on passive refresh: %d", len(rows))
	}
}

func TestSavePendingInviteFromWelcome_ReinviteReopensRejected(t *testing.T) {
	rt := setupMembershipRuntime(t)
	groupID := "group-reinvite"
	first := []byte("welcome-1")
	second := []byte("welcome-2")
	if err := rt.savePendingInviteFromWelcome(groupID, "group", first, "peer-a", true); err != nil {
		t.Fatalf("savePendingInviteFromWelcome first: %v", err)
	}
	firstID := store.PendingInviteID(groupID, first)
	if err := rt.RejectInvite(firstID); err != nil {
		t.Fatalf("RejectInvite: %v", err)
	}
	if err := rt.savePendingInviteFromWelcome(groupID, "group", second, "peer-a", true); err != nil {
		t.Fatalf("savePendingInviteFromWelcome reinvite: %v", err)
	}
	rows, err := rt.db.ListPendingInvites(false)
	if err != nil {
		t.Fatalf("ListPendingInvites(false): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("pending invite count = %d, want 1", len(rows))
	}
	if rows[0].ID != store.PendingInviteID(groupID, second) {
		t.Fatalf("pending invite id = %q, want reinvite id", rows[0].ID)
	}
	if rows[0].Status != store.PendingInviteStatusPending {
		t.Fatalf("status = %q, want pending", rows[0].Status)
	}
}

