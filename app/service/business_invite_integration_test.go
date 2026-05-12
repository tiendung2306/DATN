//go:build business_integration

// Sprint 3 — BI-044–BI-054 (invite legacy & pending invite).

package service

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"app/adapter/store"
)

func TestBusinessP1_Sprint3_BI044_GenerateJoinCode(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	res, err := rt.GenerateJoinCode()
	if err != nil {
		t.Fatalf("GenerateJoinCode: %v", err)
	}
	if res.CodeHex == "" || res.Checksum == "" {
		t.Fatalf("empty fields: %#v", res)
	}
	if len(res.Checksum) < 8 {
		t.Fatalf("checksum too short: %q", res.Checksum)
	}
}

func TestBusinessP1_Sprint3_BI045_ListPendingInvites_EmptyInitially(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	list, err := rt.ListPendingInvites()
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("want no pending invites, got %d", len(list))
	}
}

func TestBusinessP1_Sprint3_BI046_ListPendingInvites_AfterSave(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-inv046"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	welcomeBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	d := rt.db
	rt.mu.RUnlock()
	if err := d.SavePendingInvite(&store.PendingInvite{
		GroupID:      gid,
		GroupType:    "channel",
		WelcomeBytes: welcomeBytes,
		SourcePeerID: "peer-src",
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}
	list, err := rt.ListPendingInvites()
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 invite, got %d", len(list))
	}
	if list[0].GroupID != gid {
		t.Fatalf("group_id=%q", list[0].GroupID)
	}
}

func TestBusinessP1_Sprint3_BI047_AcceptInvite_HappyPath(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	businessPersistMockKPBundle(t, rt)
	gid := "grp-inv047"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	welcomeBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()
	if err := database.SavePendingInvite(&store.PendingInvite{
		GroupID:      gid,
		GroupType:    "group",
		WelcomeBytes: welcomeBytes,
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}
	invID := store.PendingInviteID(gid, welcomeBytes)
	if err := rt.AcceptInvite(invID); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	has, err := database.HasGroup(gid)
	if err != nil || !has {
		t.Fatalf("HasGroup=%v err=%v", has, err)
	}
}

func TestBusinessP1_Sprint3_BI048_RejectInvite_BlocksAccept(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-inv048"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	welcomeBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()
	if err := database.SavePendingInvite(&store.PendingInvite{
		GroupID:      gid,
		GroupType:    "group",
		WelcomeBytes: welcomeBytes,
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}
	invID := store.PendingInviteID(gid, welcomeBytes)
	if err := rt.RejectInvite(invID); err != nil {
		t.Fatalf("RejectInvite: %v", err)
	}
	err = rt.AcceptInvite(invID)
	if !errors.Is(err, ErrInviteAlreadyRejected) {
		t.Fatalf("AcceptInvite err=%v want ErrInviteAlreadyRejected", err)
	}
}

func TestBusinessP1_Sprint3_BI049_ReopenRejectedInvite(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-inv049"
	w1 := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	b1, err := json.Marshal(w1)
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()
	if err := database.SavePendingInvite(&store.PendingInvite{
		GroupID:      gid,
		GroupType:    "channel",
		WelcomeBytes: b1,
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}
	id1 := store.PendingInviteID(gid, b1)
	if err := database.MarkPendingInviteRejected(id1); err != nil {
		t.Fatalf("MarkPendingInviteRejected: %v", err)
	}
	w2 := bizMockGroupState{
		GroupID:  gid,
		Epoch:    2,
		TreeHash: hex.EncodeToString(bizMockTreeHash(2)),
	}
	b2, err := json.Marshal(w2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.ReopenRejectedInvite(&store.PendingInvite{
		ID:            store.PendingInviteID(gid, b2),
		GroupID:       gid,
		GroupType:     "channel",
		WelcomeBytes:  b2,
		SourcePeerID:  "peer-x",
		InviterPeerID: "peer-x",
	})
	if err != nil {
		t.Fatalf("ReopenRejectedInvite: %v", err)
	}
	list, err := rt.ListPendingInvites()
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 pending after reopen, got %d", len(list))
	}
}

func TestBusinessP1_Sprint3_BI050_CheckDHTWelcome_NoReplica(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	err := rt.CheckDHTWelcome("nonexistent-group-050")
	if err == nil {
		t.Fatal("expected error when no welcome from peers")
	}
	if !strings.Contains(err.Error(), "no pending invite") {
		t.Fatalf("err=%v want substring no pending invite", err)
	}
}

// businessAssertOutboundWelcomeQueuedAfterInvite covers BI-051 and the queue prerequisite for BI-054 (resend-on-reconnect is smoke/e2e).
func businessAssertOutboundWelcomeQueuedAfterInvite(t *testing.T) {
	t.Helper()
	aliceRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, aliceRoot)
	alice, _ := businessRuntimeStartMockInWorkDir(t, aliceRoot)
	defer businessShutdownRuntimeInWorkDir(t, alice)

	gid := "grp-inv051"
	cat := businessEnsureCategory(t, alice, "BI-051")
	if err := alice.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}

	bobRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, bobRoot)
	bob, _ := businessRuntimeStartMockInWorkDir(t, bobRoot)
	defer businessShutdownRuntimeInWorkDir(t, bob)

	kp, err := bob.GenerateKeyPackage()
	if err != nil {
		t.Fatal(err)
	}
	bInfo, _ := bob.GetOnboardingInfo()
	businessSeedStoredKeyPackageForPeer(t, alice, bInfo.PeerID, kp.PublicHex)

	if err := alice.InvitePeerToGroup(bInfo.PeerID, gid); err != nil {
		t.Fatalf("InvitePeerToGroup: %v", err)
	}
	alice.mu.RLock()
	ad := alice.db
	alice.mu.RUnlock()
	pending, err := ad.GetPendingWelcomesFor(bInfo.PeerID)
	if err != nil {
		t.Fatalf("GetPendingWelcomesFor: %v", err)
	}
	if len(pending) == 0 {
		t.Fatal("expected outbound pending welcome row for target peer")
	}
	for _, pw := range pending {
		if pw.GroupID == gid && len(pw.WelcomeBytes) > 0 {
			return
		}
	}
	t.Fatalf("no pending welcome for group %q: %#v", gid, pending)
}

func TestBusinessP1_Sprint3_BI051_InvitePeerToGroup_WithStoredKP(t *testing.T) {
	businessAssertOutboundWelcomeQueuedAfterInvite(t)
}

func TestBusinessP1_Sprint3_BI052_InvitePeerToGroup_NoKeyPackage(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-inv052"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	target := testPeerID(t)
	err := rt.InvitePeerToGroup(target, gid)
	if err == nil {
		t.Fatal("expected error when KP unavailable")
	}
	if !strings.Contains(err.Error(), "KeyPackage") {
		t.Fatalf("err=%v want KeyPackage mention", err)
	}
}

func TestBusinessP1_Sprint3_BI053_GetKPStatus_Advertised(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	businessPersistMockKPBundle(t, rt)
	st := rt.GetKPStatus()
	adv, _ := st["advertised"].(bool)
	if !adv {
		t.Fatalf("expected advertised true, got %#v", st)
	}
}

func TestBusinessP1_Sprint3_BI054_OutboundWelcomeQueuedForInvite(t *testing.T) {
	businessAssertOutboundWelcomeQueuedAfterInvite(t)
}

// Auto-join semantics: receiving a Welcome via savePendingInviteFromWelcome
// (the single chokepoint used by direct stream / replication / blind-store
// paths) must apply the Welcome immediately so the invitee joins without a
// manual Accept click. Pending row is preserved as audit trail with status=accepted.
func TestBusinessP1_AutoJoin_OnIncomingWelcome(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	businessPersistMockKPBundle(t, rt)
	gid := "grp-autojoin-incoming"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	welcomeBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}

	if err := rt.savePendingInviteFromWelcome(gid, "channel", "", welcomeBytes, "peer-inviter", false); err != nil {
		t.Fatalf("savePendingInviteFromWelcome: %v", err)
	}

	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()

	has, err := database.HasGroup(gid)
	if err != nil || !has {
		t.Fatalf("expected auto-join HasGroup=true, has=%v err=%v", has, err)
	}
	invID := store.PendingInviteID(gid, welcomeBytes)
	inv, err := database.GetPendingInvite(invID)
	if err != nil {
		t.Fatalf("GetPendingInvite: %v", err)
	}
	if inv.Status != store.PendingInviteStatusAccepted {
		t.Fatalf("invite status=%q want accepted", inv.Status)
	}
}

// Auto-join recovery: rows left in pending state from a previous session
// (e.g., the runtime was offline when the Welcome arrived) must be picked up
// and applied by processPendingWelcomesOnStartup.
func TestBusinessP1_AutoJoin_ProcessesPendingOnStartup(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	businessPersistMockKPBundle(t, rt)
	gid := "grp-autojoin-startup"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	welcomeBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()
	if err := database.SavePendingInvite(&store.PendingInvite{
		GroupID:      gid,
		GroupType:    "channel",
		WelcomeBytes: welcomeBytes,
		SourcePeerID: "peer-inviter",
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}

	rt.processPendingWelcomesOnStartup(rt.appCtx())

	has, err := database.HasGroup(gid)
	if err != nil || !has {
		t.Fatalf("expected processPendingWelcomesOnStartup to join group, has=%v err=%v", has, err)
	}
	invID := store.PendingInviteID(gid, welcomeBytes)
	inv, err := database.GetPendingInvite(invID)
	if err != nil {
		t.Fatalf("GetPendingInvite: %v", err)
	}
	if inv.Status != store.PendingInviteStatusAccepted {
		t.Fatalf("invite status=%q want accepted", inv.Status)
	}
}

// Auto-join graceful fallback: when applyWelcome cannot run (no local KP
// bundle persisted yet → ProcessWelcome would fail) the pending row must
// stay pending so processPendingWelcomesOnStartup can retry next launch.
func TestBusinessP1_AutoJoin_DeferredWhenKPMissing(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	// Intentionally skip businessPersistMockKPBundle — applyWelcome should
	// fail at GetKPBundle and leave the row in pending state.
	gid := "grp-autojoin-kp-missing"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	welcomeBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}

	if err := rt.savePendingInviteFromWelcome(gid, "channel", "", welcomeBytes, "peer-inviter", false); err != nil {
		t.Fatalf("savePendingInviteFromWelcome: %v", err)
	}

	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()
	has, err := database.HasGroup(gid)
	if err != nil {
		t.Fatalf("HasGroup: %v", err)
	}
	if has {
		t.Fatal("expected group NOT joined (KP bundle missing)")
	}
	invID := store.PendingInviteID(gid, welcomeBytes)
	inv, err := database.GetPendingInvite(invID)
	if err != nil {
		t.Fatalf("GetPendingInvite: %v", err)
	}
	if inv.Status != store.PendingInviteStatusPending {
		t.Fatalf("invite status=%q want pending (deferred)", inv.Status)
	}

	// Now persist the KP bundle and verify the deferred startup sweep
	// completes the join.
	businessPersistMockKPBundle(t, rt)
	rt.processPendingWelcomesOnStartup(rt.appCtx())

	has, err = database.HasGroup(gid)
	if err != nil || !has {
		t.Fatalf("expected retry to succeed after KP available, has=%v err=%v", has, err)
	}
}

// Wire-path regression guard: Alice invites Bob through the real sender API
// (`InvitePeerToGroup`), the queued Welcome bytes flow into Bob's wire
// chokepoint (`savePendingInviteFromWelcome`), and the post-condition the
// user actually observes — "Bob is inside the group without clicking Accept" —
// is asserted at the end of the pipe. This pins down the cross-cutting
// invariant that earlier API-isolation tests miss: every prior layer can be
// green individually while the journey breaks (welcome arrives but stays
// pending). If this test ever fails, auto-join semantics have regressed.
func TestBusinessP1_WirePath_AliceInvitesBob_BobAutoJoinsEndToEnd(t *testing.T) {
	gid := "grp-wirepath-e2e"

	aliceRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, aliceRoot)
	alice, _ := businessRuntimeStartMockInWorkDir(t, aliceRoot)
	defer businessShutdownRuntimeInWorkDir(t, alice)
	cat := businessEnsureCategory(t, alice, "WirePath")
	if err := alice.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}

	bobRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, bobRoot)
	bob, _ := businessRuntimeStartMockInWorkDir(t, bobRoot)
	defer businessShutdownRuntimeInWorkDir(t, bob)

	// Production startup runs advertiseKeyPackage which persists the KP
	// bundle into Bob's DB; the test reproduces that side-effect explicitly
	// because the mock harness stops short of advertising.
	businessPersistMockKPBundle(t, bob)
	bInfo, _ := bob.GetOnboardingInfo()
	bob.mu.RLock()
	bobDB := bob.db
	bob.mu.RUnlock()
	bobPubKP, _, kpErr := bobDB.GetKPBundle(bInfo.PeerID)
	if kpErr != nil {
		t.Fatalf("Bob persisted KP missing: %v", kpErr)
	}
	businessSeedStoredKeyPackageForPeer(t, alice, bInfo.PeerID, hex.EncodeToString(bobPubKP))

	if err := alice.InvitePeerToGroup(bInfo.PeerID, gid); err != nil {
		t.Fatalf("InvitePeerToGroup: %v", err)
	}

	alice.mu.RLock()
	aliceDB := alice.db
	alice.mu.RUnlock()
	pending, err := aliceDB.GetPendingWelcomesFor(bInfo.PeerID)
	if err != nil || len(pending) == 0 {
		t.Fatalf("Alice did not queue Welcome for Bob: err=%v len=%d", err, len(pending))
	}
	var welcomeBytes []byte
	for _, pw := range pending {
		if pw.GroupID == gid && len(pw.WelcomeBytes) > 0 {
			welcomeBytes = pw.WelcomeBytes
			break
		}
	}
	if len(welcomeBytes) == 0 {
		t.Fatalf("no welcome bytes for group %q in alice's outbound queue", gid)
	}

	aInfo, _ := alice.GetOnboardingInfo()
	// Wire-delivery proxy: this is the chokepoint that direct stream,
	// replication, and blind-store fetchers all funnel into. Bypassing it
	// is exactly the bug class earlier tests missed.
	if err := bob.savePendingInviteFromWelcome(gid, "channel", "", welcomeBytes, aInfo.PeerID, false); err != nil {
		t.Fatalf("savePendingInviteFromWelcome (wire path): %v", err)
	}

	if has, herr := bobDB.HasGroup(gid); herr != nil || !has {
		t.Fatalf("regression: Bob did NOT auto-join after wire delivery; HasGroup=%v err=%v", has, herr)
	}
	invID := store.PendingInviteID(gid, welcomeBytes)
	inv, err := bobDB.GetPendingInvite(invID)
	if err != nil {
		t.Fatalf("GetPendingInvite: %v", err)
	}
	if inv.Status != store.PendingInviteStatusAccepted {
		t.Fatalf("regression: pending invite status=%q want accepted", inv.Status)
	}
}
