package store

import (
	"testing"
	"time"
)

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

func TestPendingWelcome_GetAnyPendingWelcomeForGroupIncludesDelivered(t *testing.T) {
	d := setupTestDB(t)
	if err := d.SavePendingWelcome("peer-a", "group-1", []byte("welcome-1")); err != nil {
		t.Fatalf("SavePendingWelcome: %v", err)
	}
	rows, err := d.GetPendingWelcomesFor("peer-a")
	if err != nil || len(rows) != 1 {
		t.Fatalf("GetPendingWelcomesFor: rows=%d err=%v", len(rows), err)
	}
	if err := d.MarkWelcomeDelivered(rows[0].ID); err != nil {
		t.Fatalf("MarkWelcomeDelivered: %v", err)
	}
	got, err := d.GetAnyPendingWelcomeForGroup("peer-a", "group-1")
	if err != nil {
		t.Fatalf("GetAnyPendingWelcomeForGroup: %v", err)
	}
	if string(got) != "welcome-1" {
		t.Fatalf("welcome mismatch: got %q", string(got))
	}
}

func TestPendingInvite_ReopenRejectedInvite(t *testing.T) {
	d := setupTestDB(t)
	firstWelcome := []byte("welcome-1")
	secondWelcome := []byte("welcome-2")
	if err := d.SavePendingInvite(&PendingInvite{
		GroupID:       "group-1",
		GroupType:     "group",
		WelcomeBytes:  firstWelcome,
		SourcePeerID:  "peer-a",
		InviterPeerID: "peer-a",
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}
	firstID := PendingInviteID("group-1", firstWelcome)
	if err := d.MarkPendingInviteRejected(firstID); err != nil {
		t.Fatalf("MarkPendingInviteRejected: %v", err)
	}
	reopenedID, err := d.ReopenRejectedInvite(&PendingInvite{
		ID:            PendingInviteID("group-1", secondWelcome),
		GroupID:       "group-1",
		GroupType:     "group",
		WelcomeBytes:  secondWelcome,
		SourcePeerID:  "peer-a",
		InviterPeerID: "peer-a",
	})
	if err != nil {
		t.Fatalf("ReopenRejectedInvite: %v", err)
	}
	if reopenedID == firstID {
		t.Fatalf("reopened id should be replaced with new welcome hash")
	}
	latest, err := d.GetLatestPendingInviteByGroup("group-1")
	if err != nil {
		t.Fatalf("GetLatestPendingInviteByGroup: %v", err)
	}
	if latest.Status != PendingInviteStatusPending {
		t.Fatalf("latest status = %q, want pending", latest.Status)
	}
	if latest.ID != reopenedID {
		t.Fatalf("latest id = %q, want %q", latest.ID, reopenedID)
	}
}

func TestStoredWelcome_ListForInvitee(t *testing.T) {
	d := setupTestDB(t)
	if err := d.SaveStoredWelcome("peer-a", "group-1", "dm", "", []byte("welcome-1"), "peer-store"); err != nil {
		t.Fatalf("SaveStoredWelcome: %v", err)
	}
	if err := d.SaveStoredWelcome("peer-b", "group-2", "channel", "", []byte("welcome-2"), "peer-store"); err != nil {
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
	if rows[0].GroupType != "dm" {
		t.Fatalf("stored welcome group type = %q, want dm", rows[0].GroupType)
	}
}

func TestStoredWelcome_GetReturnsSourcePeerID(t *testing.T) {
	d := setupTestDB(t)
	const inviter = "12D3KooWKexampleInviterPeerIDhere"
	if err := d.SaveStoredWelcome("peer-a", "group-x", "channel", "cat-1", []byte("wb"), inviter); err != nil {
		t.Fatal(err)
	}
	wb, gt, cat, src, err := d.GetStoredWelcome("peer-a", "group-x")
	if err != nil {
		t.Fatalf("GetStoredWelcome: %v", err)
	}
	if string(wb) != "wb" || gt != "channel" || cat != "cat-1" || src != inviter {
		t.Fatalf("got wb=%q gt=%q cat=%q src=%q", string(wb), gt, cat, src)
	}
}

func TestGetGroupInviteCreatorHint_FromInviterPeerID(t *testing.T) {
	d := setupTestDB(t)
	const creator = "12D3KooWKcreatorPeerIDhere000000000"
	now := time.Now().Unix()
	inv := &PendingInvite{
		ID:            PendingInviteID("g-hint", []byte("w1")),
		GroupID:       "g-hint",
		GroupType:     "channel",
		SourcePeerID:  "",
		InviterPeerID: creator,
		WelcomeBytes:  []byte("w1"),
		Status:        PendingInviteStatusAccepted,
		ReceivedAt:    now,
		UpdatedAt:     now,
	}
	if err := d.SavePendingInvite(inv); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetGroupInviteCreatorHint("g-hint")
	if err != nil {
		t.Fatalf("GetGroupInviteCreatorHint: %v", err)
	}
	if got != creator {
		t.Fatalf("hint=%q want %q", got, creator)
	}
}

// Regression: GetGroupInviteCreatorHint must skip rows where source_peer_id
// is empty / whitespace and pick the next non-empty hint. Earlier the
// query returned the most recent row unconditionally, so a row written by
// a buggy caller with source = "" could shadow a good earlier row.
func TestGetGroupInviteCreatorHint_SkipsEmptySourceRows(t *testing.T) {
	d := setupTestDB(t)
	const creator = "12D3KooWKcreatorPeerIDhere000000000"

	// Older row: known good source.
	if err := d.SaveStoredWelcome("invitee", "g-skip", "channel", "", []byte("w-old"), creator); err != nil {
		t.Fatalf("SaveStoredWelcome older: %v", err)
	}
	// Newer row: blank source (the bug class — must NOT shadow the good one).
	if err := d.SaveStoredWelcome("invitee", "g-skip", "channel", "", []byte("w-new"), "   "); err != nil {
		t.Fatalf("SaveStoredWelcome newer-blank: %v", err)
	}

	got, err := d.GetGroupInviteCreatorHint("g-skip")
	if err != nil {
		t.Fatalf("GetGroupInviteCreatorHint: %v", err)
	}
	if got != creator {
		t.Fatalf("hint=%q want %q (blank-source row must not shadow good row)", got, creator)
	}
}

// Regression: when a non-creator member receives a welcome via wire and
// also stores it (replication path), source_peer_id must be the inviter,
// not the local invitee. This invariant is enforced at the caller layer
// (savePendingInviteFromWelcome guards self) — this test verifies the DB
// contract: SaveStoredWelcome / GetStoredWelcome carry the value through
// faithfully without trimming inviter context.
func TestStoredWelcome_RoundTrip_PreservesInviterIdentity(t *testing.T) {
	d := setupTestDB(t)
	const inviter = "12D3KooWKinviter111111111111111"
	const invitee = "12D3KooWKinvitee222222222222222"
	if err := d.SaveStoredWelcome(invitee, "g-rt", "channel", "cat-rt", []byte("wb"), inviter); err != nil {
		t.Fatalf("SaveStoredWelcome: %v", err)
	}
	wb, gt, cat, src, err := d.GetStoredWelcome(invitee, "g-rt")
	if err != nil {
		t.Fatalf("GetStoredWelcome: %v", err)
	}
	if string(wb) != "wb" || gt != "channel" || cat != "cat-rt" {
		t.Fatalf("payload mismatch: wb=%q gt=%q cat=%q", wb, gt, cat)
	}
	if src != inviter {
		t.Fatalf("source_peer_id=%q want inviter %q (round-trip lost identity)", src, inviter)
	}
	if src == invitee {
		t.Fatalf("source_peer_id=%q must not equal invitee %q", src, invitee)
	}
}

// Regression: SaveStoredWelcome must NOT overwrite a known good
// source_peer_id with an empty incoming value. Same heal semantics as
// category_id. This protects creator-hint resolution against any caller
// that forgot to pass source (e.g. legacy checkStoredWelcomes "" path).
func TestStoredWelcome_BlankSource_DoesNotClobberGoodHint(t *testing.T) {
	d := setupTestDB(t)
	const inviter = "12D3KooWKinviterXheal000000000000"
	const invitee = "12D3KooWKinviteeXheal000000000000"
	if err := d.SaveStoredWelcome(invitee, "g-heal", "channel", "", []byte("w1"), inviter); err != nil {
		t.Fatalf("first SaveStoredWelcome: %v", err)
	}
	// Caller forgot the source: must not wipe the good hint.
	if err := d.SaveStoredWelcome(invitee, "g-heal", "channel", "", []byte("w2"), "   "); err != nil {
		t.Fatalf("second SaveStoredWelcome: %v", err)
	}
	_, _, _, src, err := d.GetStoredWelcome(invitee, "g-heal")
	if err != nil {
		t.Fatalf("GetStoredWelcome: %v", err)
	}
	if src != inviter {
		t.Fatalf("regression: source_peer_id=%q got clobbered, want %q", src, inviter)
	}
}

// Same heal semantics for SaveStoredWelcomeIfNewer (blind-store
// replication path). A newer payload with empty source must not erase a
// pre-existing inviter hint.
func TestStoredWelcomeIfNewer_BlankSource_DoesNotClobberGoodHint(t *testing.T) {
	d := setupTestDB(t)
	const inviter = "12D3KooWKinviterXhealnewer0000000"
	const invitee = "12D3KooWKinviteeXhealnewer0000000"
	now := time.Now().Unix()
	if err := d.SaveStoredWelcomeIfNewer(invitee, "g-heal-newer", "channel", "", []byte("w1"), inviter, now-10); err != nil {
		t.Fatalf("first SaveStoredWelcomeIfNewer: %v", err)
	}
	// A newer publish with blank source — must keep inviter on file.
	if err := d.SaveStoredWelcomeIfNewer(invitee, "g-heal-newer", "channel", "", []byte("w2"), "", now); err != nil {
		t.Fatalf("second SaveStoredWelcomeIfNewer: %v", err)
	}
	_, _, _, src, err := d.GetStoredWelcome(invitee, "g-heal-newer")
	if err != nil {
		t.Fatalf("GetStoredWelcome: %v", err)
	}
	if src != inviter {
		t.Fatalf("regression: source_peer_id=%q lost (heal failed), want %q", src, inviter)
	}
}
