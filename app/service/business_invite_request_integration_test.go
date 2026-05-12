//go:build business_integration

// Sprint 4 — BI-055–BI-069 (group invite requests / policy).

package service

import (
	"strings"
	"testing"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestBusinessP1_Sprint4_BI055_GetDefaultInvitePolicy(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-s4-055"
	cat := businessEnsureCategory(t, rt, "S4-055")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	pol, err := rt.GetGroupInvitePolicy(gid)
	if err != nil {
		t.Fatalf("GetGroupInvitePolicy: %v", err)
	}
	if pol != store.GroupInvitePolicyCreatorApproval {
		t.Fatalf("policy=%q want creator_approval", pol)
	}
}

func TestBusinessP1_Sprint4_BI056_SetInvitePolicy_Persists(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-s4-056"
	cat := businessEnsureCategory(t, rt, "S4-056")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	if err := rt.SetGroupInvitePolicy(gid, store.GroupInvitePolicyAnyMember); err != nil {
		t.Fatalf("SetGroupInvitePolicy: %v", err)
	}
	pol, err := rt.GetGroupInvitePolicy(gid)
	if err != nil {
		t.Fatalf("GetGroupInvitePolicy: %v", err)
	}
	if pol != store.GroupInvitePolicyAnyMember {
		t.Fatalf("policy=%q want any_member", pol)
	}
}

func TestBusinessP1_Sprint4_BI057_NonCreatorCannotSetPolicy(t *testing.T) {
	gid := "grp-s4-057"
	alice, bob := sprint4AliceBobJoinedChannel(t, gid)
	err := bob.SetGroupInvitePolicy(gid, store.GroupInvitePolicyAnyMember)
	if err == nil {
		t.Fatal("expected forbidden for non-creator")
	}
	if !strings.Contains(err.Error(), errInviteForbidden) {
		t.Fatalf("err=%v want %s", err, errInviteForbidden)
	}
	_ = alice
}

// sprint4CharlieRuntime starts a third authorized node and returns peer ID + KP hex + shutdown via t.Cleanup.
func sprint4CharlieRuntime(t *testing.T) (peerID string, publicKPHex string) {
	t.Helper()
	root := t.TempDir()
	businessSeedAuthorizedWorkDir(t, root)
	rt, _ := businessRuntimeStartMockInWorkDir(t, root)
	t.Cleanup(func() { businessShutdownRuntimeInWorkDir(t, rt) })
	kp, err := rt.GenerateKeyPackage()
	if err != nil {
		t.Fatal(err)
	}
	info, _ := rt.GetOnboardingInfo()
	return info.PeerID, kp.PublicHex
}

// BI-058 — any_member policy: a member-side submit reaching the creator's
// wire handler auto-approves via creator's own MLS execution path. We test
// the creator side directly because the in-process libp2p connection gater
// blocks dials between two test hosts (bob.RequestGroupInvite would fail at
// the wire layer in this harness, not for any product reason). The creator
// being the Token Holder is what makes auto-approve safe under the
// Single-Writer Invariant; member-side self-process was removed (2026-05-10)
// because non-Token-Holder members hit ErrNotTokenHolder.
func TestBusinessP1_Sprint4_BI058_RequestGroupInvite_AnyMember(t *testing.T) {
	gid := "grp-s4-058"
	alice, bob := sprint4AliceBobJoinedChannel(t, gid)
	if err := alice.SetGroupInvitePolicy(gid, store.GroupInvitePolicyAnyMember); err != nil {
		t.Fatalf("SetGroupInvitePolicy: %v", err)
	}
	charliePeer, charliePubHex := sprint4CharlieRuntime(t)
	businessSeedStoredKeyPackageForPeer(t, alice, charliePeer, charliePubHex)
	bInfo, _ := bob.GetOnboardingInfo()
	bobPID, err := peer.Decode(bInfo.PeerID)
	if err != nil {
		t.Fatalf("decode bob: %v", err)
	}
	resp := alice.handleGroupInviteWireRPC(bobPID, &p2p.GroupInviteWireClientReqV1{
		V: 1, Op: "submit", GroupID: gid, TargetPeerID: charliePeer,
	})
	if !resp.OK || resp.Record == nil {
		t.Fatalf("submit failed: ok=%v err=%q", resp.OK, resp.Error)
	}
	// Creator auto-approves any_member synchronously, so the row returned to
	// the requester (after mirror upsert) should be terminal-approved.
	if resp.Record.Status != store.InviteRequestStatusApproved {
		t.Fatalf("any_member auto-approve expected status=approved, got %q", resp.Record.Status)
	}
}

// BI-059 — second submit for the same (group, target) pair while the first
// is still active must hit the unique-active guard on the creator. Seed the
// duplicate row directly on the creator's DB (the only authoritative copy
// in the new architecture).
func TestBusinessP1_Sprint4_BI059_RequestDuplicateActive(t *testing.T) {
	gid := "grp-s4-059"
	alice, bob := sprint4AliceBobJoinedChannel(t, gid)
	if err := alice.SetGroupInvitePolicy(gid, store.GroupInvitePolicyCreatorApproval); err != nil {
		t.Fatalf("SetGroupInvitePolicy: %v", err)
	}
	bInfo, _ := bob.GetOnboardingInfo()
	bobPID, err := peer.Decode(bInfo.PeerID)
	if err != nil {
		t.Fatalf("decode bob: %v", err)
	}
	charliePeer, charliePubHex := sprint4CharlieRuntime(t)
	businessSeedStoredKeyPackageForPeer(t, alice, charliePeer, charliePubHex)
	now := time.Now().Unix()
	businessSeedInviteRequest(t, alice, store.GroupInviteRequestRecord{
		RequestID:       "req-s4-059-seed",
		GroupID:         gid,
		RequesterPeerID: bInfo.PeerID,
		TargetPeerID:    charliePeer,
		Status:          store.InviteRequestStatusPending,
		ExpiresAt:       now + 7200,
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	resp := alice.handleGroupInviteWireRPC(bobPID, &p2p.GroupInviteWireClientReqV1{
		V: 1, Op: "submit", GroupID: gid, TargetPeerID: charliePeer,
	})
	if resp.OK {
		t.Fatal("expected duplicate active error")
	}
	if !strings.Contains(resp.Error, errInviteDuplicateActive) {
		t.Fatalf("err=%q want substring %s", resp.Error, errInviteDuplicateActive)
	}
}

// BI-060 — submitting an invite for a peer who is already a member is
// rejected by the creator's wire handler with a stable "target is already
// a member" message. We test the wire handler so the contract exercised
// end-to-end by RequestGroupInvite is anchored regardless of harness
// limitations on inter-runtime dial.
func TestBusinessP1_Sprint4_BI060_TargetAlreadyMember_RequestCompletes(t *testing.T) {
	gid := "grp-s4-060"
	alice, bob := sprint4AliceBobJoinedChannel(t, gid)
	if err := alice.SetGroupInvitePolicy(gid, store.GroupInvitePolicyAnyMember); err != nil {
		t.Fatalf("SetGroupInvitePolicy: %v", err)
	}
	bInfo, _ := bob.GetOnboardingInfo()
	bobPID, err := peer.Decode(bInfo.PeerID)
	if err != nil {
		t.Fatalf("decode bob: %v", err)
	}
	aInfo, _ := alice.GetOnboardingInfo() // already-member target = alice herself
	resp := alice.handleGroupInviteWireRPC(bobPID, &p2p.GroupInviteWireClientReqV1{
		V: 1, Op: "submit", GroupID: gid, TargetPeerID: aInfo.PeerID,
	})
	if resp.OK {
		t.Fatalf("expected reject for already-member target, got record=%+v", resp.Record)
	}
	if !strings.Contains(strings.ToLower(resp.Error), "already a member") {
		t.Fatalf("err=%q want substring 'already a member'", resp.Error)
	}
}

func TestBusinessP1_Sprint4_BI061_ApproveGroupInviteRequest(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-s4-061"
	cat := businessEnsureCategory(t, rt, "S4-061")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	businessPersistMockKPBundle(t, rt)

	charliePeer, charliePubHex := sprint4CharlieRuntime(t)
	businessSeedStoredKeyPackageForPeer(t, rt, charliePeer, charliePubHex)

	bobPeer := testPeerID(t)
	now := time.Now().Unix()
	reqID := "req-s4-061"
	businessSeedInviteRequest(t, rt, store.GroupInviteRequestRecord{
		RequestID:       reqID,
		GroupID:         gid,
		RequesterPeerID: bobPeer,
		TargetPeerID:    charliePeer,
		Status:          store.InviteRequestStatusPending,
		ExpiresAt:       now + 7200,
		CreatedAt:       now,
		UpdatedAt:       now,
		MaxAttempts:     5,
	})

	out, err := rt.ApproveGroupInviteRequest(reqID)
	if err != nil {
		t.Fatalf("ApproveGroupInviteRequest: %v", err)
	}
	if out.Status != store.InviteRequestStatusApproved {
		t.Fatalf("status=%q want approved", out.Status)
	}
}

func TestBusinessP1_Sprint4_BI062_RejectGroupInviteRequest(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-s4-062"
	cat := businessEnsureCategory(t, rt, "S4-062")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	bobPeer := testPeerID(t)
	target := testPeerID(t)
	now := time.Now().Unix()
	reqID := "req-s4-062"
	businessSeedInviteRequest(t, rt, store.GroupInviteRequestRecord{
		RequestID:       reqID,
		GroupID:         gid,
		RequesterPeerID: bobPeer,
		TargetPeerID:    target,
		Status:          store.InviteRequestStatusPending,
		ExpiresAt:       now + 7200,
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	out, err := rt.RejectGroupInviteRequest(reqID, "no capacity")
	if err != nil {
		t.Fatalf("RejectGroupInviteRequest: %v", err)
	}
	if out.Status != store.InviteRequestStatusRejected {
		t.Fatalf("status=%q", out.Status)
	}
	if !strings.Contains(out.RejectionReason, "no capacity") {
		t.Fatalf("reason=%q", out.RejectionReason)
	}
}

// BI-063 (CancelGroupInviteRequest by requester) was removed (2026-05-10)
// alongside the API itself: in a serverless P2P mesh, racing a requester
// cancel against a concurrent creator approve would require CRDT-style
// coordination across the gossip network. We keep only Approve/Reject.

// TestBusinessP1_AnyMember_WireSubmitAutoApproves pins the new
// any_member contract: the creator-side wire handler (the only node holding
// the MLS Token under the Single-Writer Invariant) auto-approves a member
// submission synchronously and returns a record with status=approved.
//
// The previous "policy drift" lazy-sync (member's local cache was assumed
// to drive flow selection) was removed (2026-05-10) — members always
// forward to the creator now, so there is nothing to synchronize. Without
// this guard, the auto-approve regression would manifest end-to-end as
// rows stuck in `pending` forever from any non-creator's invite attempt.
func TestBusinessP1_AnyMember_WireSubmitAutoApproves(t *testing.T) {
	gid := "grp-any-member-auto"
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	cat := businessEnsureCategory(t, rt, "S4-AnyMember")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	if err := rt.SetGroupInvitePolicy(gid, store.GroupInvitePolicyAnyMember); err != nil {
		t.Fatalf("SetGroupInvitePolicy: %v", err)
	}
	businessPersistMockKPBundle(t, rt)
	charliePeer, charliePubHex := sprint4CharlieRuntime(t)
	businessSeedStoredKeyPackageForPeer(t, rt, charliePeer, charliePubHex)

	fakeMember := testPeerID(t)
	rt.mu.RLock()
	db := rt.db
	rt.mu.RUnlock()
	if err := db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   gid,
		PeerID:    fakeMember,
		Role:      "member",
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember: %v", err)
	}
	memberPID, err := peer.Decode(fakeMember)
	if err != nil {
		t.Fatalf("decode member: %v", err)
	}
	resp := rt.handleGroupInviteWireRPC(memberPID, &p2p.GroupInviteWireClientReqV1{
		V: 1, Op: "submit", GroupID: gid, TargetPeerID: charliePeer,
	})
	if !resp.OK || resp.Record == nil {
		t.Fatalf("expected ok with record, got ok=%v err=%q", resp.OK, resp.Error)
	}
	if resp.Record.Status != store.InviteRequestStatusApproved {
		t.Fatalf("any_member auto-approve expected status=approved, got %q", resp.Record.Status)
	}
}

// TestBusinessP1_CreatorApproval_WireSubmitStaysPending mirrors the
// any_member test for the other branch: creator_approval policy must keep
// the row in pending state on submit so the creator can decide via UI.
func TestBusinessP1_CreatorApproval_WireSubmitStaysPending(t *testing.T) {
	gid := "grp-creator-approval-wire"
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	cat := businessEnsureCategory(t, rt, "S4-CreatorApproval")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	if err := rt.SetGroupInvitePolicy(gid, store.GroupInvitePolicyCreatorApproval); err != nil {
		t.Fatalf("SetGroupInvitePolicy: %v", err)
	}
	fakeMember := testPeerID(t)
	rt.mu.RLock()
	db := rt.db
	rt.mu.RUnlock()
	if err := db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   gid,
		PeerID:    fakeMember,
		Role:      "member",
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember: %v", err)
	}
	memberPID, err := peer.Decode(fakeMember)
	if err != nil {
		t.Fatalf("decode member: %v", err)
	}
	target := testPeerID(t)
	resp := rt.handleGroupInviteWireRPC(memberPID, &p2p.GroupInviteWireClientReqV1{
		V: 1, Op: "submit", GroupID: gid, TargetPeerID: target,
	})
	if !resp.OK || resp.Record == nil {
		t.Fatalf("expected ok pending row, got ok=%v err=%q", resp.OK, resp.Error)
	}
	if resp.Record.Status != store.InviteRequestStatusPending {
		t.Fatalf("creator_approval expected status=pending, got %q", resp.Record.Status)
	}
}

func TestBusinessP1_Sprint4_BI064_ListGroupInviteRequests_FilterStatus(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-s4-064"
	cat := businessEnsureCategory(t, rt, "S4-064")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	base := store.GroupInviteRequestRecord{
		GroupID:     gid,
		ExpiresAt:   now + 7200,
		CreatedAt:   now,
		UpdatedAt:   now,
		MaxAttempts: 5,
	}
	tA := testPeerID(t)
	tB := testPeerID(t)
	tC := testPeerID(t)

	r1 := base
	r1.RequestID = "req-s4-064-p"
	r1.RequesterPeerID = testPeerID(t)
	r1.TargetPeerID = tA
	r1.Status = store.InviteRequestStatusPending
	businessSeedInviteRequest(t, rt, r1)

	r2 := base
	r2.RequestID = "req-s4-064-r"
	r2.RequesterPeerID = testPeerID(t)
	r2.TargetPeerID = tB
	r2.Status = store.InviteRequestStatusRejected
	r2.UpdatedAt = now + 1
	businessSeedInviteRequest(t, rt, r2)

	r3 := base
	r3.RequestID = "req-s4-064-a"
	r3.RequesterPeerID = testPeerID(t)
	r3.TargetPeerID = tC
	r3.Status = store.InviteRequestStatusApproved
	r3.UpdatedAt = now + 2
	businessSeedInviteRequest(t, rt, r3)

	res, err := rt.ListGroupInviteRequests(gid, []string{store.InviteRequestStatusPending}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].RequestID != r1.RequestID {
		t.Fatalf("pending filter: got %+v", res.Items)
	}
}

func TestBusinessP1_Sprint4_BI065_ListGroupInviteRequests_Pagination(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-s4-065"
	cat := businessEnsureCategory(t, rt, "S4-065")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for i := range 5 {
		req := store.GroupInviteRequestRecord{
			RequestID:       "req-s4-065-" + string(rune('a'+i)),
			GroupID:         gid,
			RequesterPeerID: testPeerID(t),
			TargetPeerID:    testPeerID(t),
			Status:          store.InviteRequestStatusRejected,
			ExpiresAt:       now + 7200,
			CreatedAt:       now,
			UpdatedAt:       now + int64(i),
			MaxAttempts:     5,
		}
		businessSeedInviteRequest(t, rt, req)
	}

	page1, err := rt.ListGroupInviteRequests(gid, []string{store.InviteRequestStatusRejected}, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page1 len=%d", len(page1.Items))
	}
	if page1.NextCursor == "" {
		t.Fatal("expected next cursor")
	}
	page2, err := rt.ListGroupInviteRequests(gid, []string{store.InviteRequestStatusRejected}, page1.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("page2 len=%d", len(page2.Items))
	}
	seen := map[string]bool{}
	for _, it := range page1.Items {
		seen[it.RequestID] = true
	}
	for _, it := range page2.Items {
		if seen[it.RequestID] {
			t.Fatalf("duplicate request_id across pages: %s", it.RequestID)
		}
	}
}

func TestBusinessP1_Sprint4_BI066_SyncInviteRequestFromCreator_NonMirrorShortCircuit(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-s4-066"
	cat := businessEnsureCategory(t, rt, "S4-066")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	reqID := "req-s4-066"
	businessSeedInviteRequest(t, rt, store.GroupInviteRequestRecord{
		RequestID:       reqID,
		GroupID:         gid,
		RequesterPeerID: testPeerID(t),
		TargetPeerID:    testPeerID(t),
		Status:          store.InviteRequestStatusPending,
		ExpiresAt:       now + 7200,
		CreatedAt:       now,
		UpdatedAt:       now,
		IsMirror:        false,
	})

	out, err := rt.SyncInviteRequestFromCreator(reqID)
	if err != nil {
		t.Fatalf("SyncInviteRequestFromCreator: %v", err)
	}
	if out.RequestID != reqID {
		t.Fatalf("request id mismatch")
	}
}

func TestBusinessP1_Sprint4_BI067_RetryGroupInviteRequest_FromFailed(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-s4-067"
	cat := businessEnsureCategory(t, rt, "S4-067")
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	businessPersistMockKPBundle(t, rt)

	charliePeer, charliePubHex := sprint4CharlieRuntime(t)
	businessSeedStoredKeyPackageForPeer(t, rt, charliePeer, charliePubHex)

	now := time.Now().Unix()
	reqID := "req-s4-067"
	businessSeedInviteRequest(t, rt, store.GroupInviteRequestRecord{
		RequestID:       reqID,
		GroupID:         gid,
		RequesterPeerID: testPeerID(t),
		TargetPeerID:    charliePeer,
		Status:          store.InviteRequestStatusFailed,
		FailureCode:     "ERR_TEST",
		ExpiresAt:       now + 7200,
		CreatedAt:       now,
		UpdatedAt:       now,
		AttemptCount:    1,
		MaxAttempts:     5,
	})

	out, err := rt.RetryGroupInviteRequest(reqID)
	if err != nil {
		t.Fatalf("RetryGroupInviteRequest: %v", err)
	}
	if out.Status != store.InviteRequestStatusApproved && out.Status != store.InviteRequestStatusFailed && out.Status != store.InviteRequestStatusProcessing {
		t.Fatalf("unexpected terminal status %q", out.Status)
	}
}
