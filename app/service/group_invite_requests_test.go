package service

import (
	"testing"
	"time"

	"app/adapter/store"
	"app/coordination"
)

func TestProcessInviteRequest_AlreadyMemberAutoApproves(t *testing.T) {
	rt := setupMembershipRuntime(t)
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-invite-1",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleCreator,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   "group-invite-1",
		PeerID:    "peer-target",
		Role:      "member",
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember: %v", err)
	}
	nowUnix := time.Now().Unix()
	if err := rt.db.CreateGroupInviteRequest(store.GroupInviteRequestRecord{
		RequestID:       "gir-1",
		GroupID:         "group-invite-1",
		RequesterPeerID: "peer-requester",
		TargetPeerID:    "peer-target",
		Status:          store.InviteRequestStatusPending,
		MaxAttempts:     5,
		ExpiresAt:       nowUnix + 3600,
		CreatedAt:       nowUnix,
		UpdatedAt:       nowUnix,
	}); err != nil {
		t.Fatalf("CreateGroupInviteRequest: %v", err)
	}

	if err := rt.processInviteRequest("gir-1", false); err != nil {
		t.Fatalf("processInviteRequest: %v", err)
	}
	rec, err := rt.db.GetGroupInviteRequest("gir-1")
	if err != nil {
		t.Fatalf("GetGroupInviteRequest: %v", err)
	}
	if rec.Status != store.InviteRequestStatusApproved {
		t.Fatalf("status=%q want approved", rec.Status)
	}
}

// TestCancelGroupInviteRequest_ProcessingReturnsConflict was removed
// (2026-05-10) together with CancelGroupInviteRequest itself. See
// service/group_invite_requests.go for the rationale (P2P cancel would
// require CRDT-style coordination; we keep only Approve/Reject).

