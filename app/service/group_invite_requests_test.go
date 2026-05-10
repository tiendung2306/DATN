package service

import (
	"strings"
	"testing"
	"time"

	"app/adapter/store"
	"app/coordination"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
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

func TestCancelGroupInviteRequest_ProcessingReturnsConflict(t *testing.T) {
	rt := setupMembershipRuntime(t)
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-invite-2",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleCreator,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}

	priv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	rt.privKey = priv
	if err := rt.db.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "Local",
		PublicKey:         []byte{1, 2, 3},
		SigningKeyPrivate: []byte{4, 5, 6},
		Credential:        []byte{7, 8, 9},
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}

	nowUnix := time.Now().Unix()
	if err := rt.db.CreateGroupInviteRequest(store.GroupInviteRequestRecord{
		RequestID:       "gir-2",
		GroupID:         "group-invite-2",
		RequesterPeerID: "peer-requester",
		TargetPeerID:    "peer-target",
		Status:          store.InviteRequestStatusProcessing,
		MaxAttempts:     5,
		ExpiresAt:       nowUnix + 3600,
		CreatedAt:       nowUnix,
		UpdatedAt:       nowUnix,
	}); err != nil {
		t.Fatalf("CreateGroupInviteRequest: %v", err)
	}

	_, err = rt.CancelGroupInviteRequest("gir-2")
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if !strings.Contains(err.Error(), "ERR_INVITE_REQUEST_STATE_CONFLICT") {
		t.Fatalf("err=%v want state conflict", err)
	}
}

