package service

import (
	"strings"
	"testing"
	"time"

	"app/adapter/store"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestSavePendingInviteFromWelcome_RejectTombstoneStickyOnPassiveRefresh(t *testing.T) {
	rt := setupMembershipRuntime(t)
	groupID := "group-reject-sticky"
	welcome := []byte("welcome-1")
	if err := rt.savePendingInviteFromWelcome(groupID, "group", "", welcome, "peer-a", true, 0, nil); err != nil {
		t.Fatalf("savePendingInviteFromWelcome initial: %v", err)
	}
	inviteID := store.PendingInviteID(groupID, welcome)
	if err := rt.RejectInvite(inviteID); err != nil {
		t.Fatalf("RejectInvite: %v", err)
	}
	if err := rt.savePendingInviteFromWelcome(groupID, "group", "", welcome, "peer-a", false, 0, nil); err != nil {
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
	if err := rt.savePendingInviteFromWelcome(groupID, "group", "", first, "peer-a", true, 0, nil); err != nil {
		t.Fatalf("savePendingInviteFromWelcome first: %v", err)
	}
	firstID := store.PendingInviteID(groupID, first)
	if err := rt.RejectInvite(firstID); err != nil {
		t.Fatalf("RejectInvite: %v", err)
	}
	if err := rt.savePendingInviteFromWelcome(groupID, "group", "", second, "peer-a", true, 0, nil); err != nil {
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

func TestFinalizeJoinedWelcome_PersistsAuditForLegacyJoinPath(t *testing.T) {
	rt := setupMembershipRuntime(t)
	groupID := "group-legacy-join"
	groupType := "group"
	welcome := []byte("welcome-legacy")

	rt.finalizeJoinedWelcome(groupID, groupType, "", welcome, "", true)

	fp := fallbackWelcomeFingerprint(groupID, welcome)
	var appliedAt int64
	if err := rt.db.Conn.QueryRow(
		`SELECT applied_at FROM applied_welcomes WHERE welcome_fingerprint = ? AND group_id = ?`,
		fp, groupID,
	).Scan(&appliedAt); err != nil {
		t.Fatalf("applied_welcomes missing row: %v", err)
	}
	if appliedAt == 0 {
		t.Fatal("applied_welcomes.applied_at should be non-zero")
	}

	inviteID := store.PendingInviteID(groupID, welcome)
	inv, err := rt.db.GetPendingInvite(inviteID)
	if err != nil {
		t.Fatalf("GetPendingInvite: %v", err)
	}
	if inv.Status != store.PendingInviteStatusAccepted {
		t.Fatalf("pending invite status=%q want accepted", inv.Status)
	}
	if inv.GroupType != groupType {
		t.Fatalf("pending invite group_type=%q want %q", inv.GroupType, groupType)
	}
	if got := string(inv.WelcomeBytes); got != string(welcome) {
		t.Fatalf("pending invite welcome=%q want %q", got, welcome)
	}
}

// TestAcceptInvite_RejoinLeftGroup verifies that when a group exists in the DB
// but was previously left (lifecycle_status = 'left', active = false), calling
// AcceptInvite on a new Welcome does NOT short-circuit with "already_joined" but
// instead purges stale metadata and attempts to re-join. Stored messages MUST be
// preserved after the purge.
//
// Because this test runs without a live P2P node or MLS engine, applyWelcome
// will fail with "P2P node not running" — that is the expected error. The key
// invariant being tested is that the code path reaches applyWelcome at all
// (i.e. does not silently return nil claiming "already joined"), AND that
// stored_messages rows survive the purge.
func TestAcceptInvite_RejoinLeftGroup(t *testing.T) {
	rt := setupMembershipRuntime(t)

	groupID := "group-rejoin-lifecycle"
	senderID := peer.ID("peer-sender")

	// 1. Seed an active group record (so LeaveGroup can mark it left).
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    groupID,
		GroupState: []byte("state-epoch-0"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}

	// 2. Persist a message so we can verify it survives the purge.
	if err := rt.coordStorage.SaveMessage(&coordination.StoredMessage{
		GroupID:      groupID,
		Epoch:        1,
		SenderID:     senderID,
		Content:      []byte("hello from before leave"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: 1000, NodeID: senderID.String()},
		EnvelopeHash: []byte("env-hash-1"),
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	// 3. Leave the group — marks lifecycle_status = 'left', active = false.
	if err := rt.LeaveGroup(groupID); err != nil {
		t.Fatalf("LeaveGroup: %v", err)
	}

	// Confirm the group is now inactive.
	active, err := rt.db.IsGroupActive(groupID)
	if err != nil {
		t.Fatalf("IsGroupActive: %v", err)
	}
	if active {
		t.Fatalf("expected group to be inactive after LeaveGroup")
	}

	// 4. Save a new pending invite (new Welcome bytes) for the same group.
	newWelcomeBytes := []byte("welcome-bytes-new-epoch-for-rejoin")
	if err := rt.db.SavePendingInvite(&store.PendingInvite{
		GroupID:      groupID,
		GroupType:    "group",
		WelcomeBytes: newWelcomeBytes,
		SourcePeerID: "peer-inviter",
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}

	newInviteID := store.PendingInviteID(groupID, newWelcomeBytes)

	// 5. Call AcceptInvite.
	//
	// Expected behaviour with our fix applied:
	//   a) AcceptInvite sees joined=true, active=false → calls PurgeGroupMetadata
	//      and resets joined=false.
	//   b) Falls through to applyWelcome, which fails with "P2P node not running"
	//      because rt.node == nil in this unit test.
	//
	// WITHOUT the fix, the old code would have returned nil immediately after
	// marking the invite "accepted" with reason "already_joined", never calling
	// applyWelcome at all.
	acceptErr := rt.AcceptInvite(newInviteID)

	if acceptErr == nil {
		t.Fatal("expected AcceptInvite to fail (no P2P node), but it returned nil — " +
			"possible regression: 'already_joined' short-circuit is incorrectly reached")
	}
	if !strings.Contains(acceptErr.Error(), "P2P node not running") &&
		!strings.Contains(acceptErr.Error(), "node or database not ready") &&
		!strings.Contains(acceptErr.Error(), "crypto engine") {
		// applyWelcome itself checks node/engine. Any of these errors confirms we
		// correctly reached the re-join path instead of the "already_joined" early return.
		t.Fatalf("unexpected error kind (should be P2P/engine error from applyWelcome): %v", acceptErr)
	}

	// 6. Verify stored_messages were NOT deleted by PurgeGroupMetadata.
	msgs, err := rt.GetGroupMessages(groupID, 50, 0)
	if err != nil {
		t.Fatalf("GetGroupMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("stored_messages count = %d, want 1 (history must survive purge)", len(msgs))
	}
	if msgs[0].Content != "hello from before leave" {
		t.Fatalf("unexpected message content: %q", msgs[0].Content)
	}
}
