package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/coordination"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func waitReplicatedRecord(t *testing.T, d *store.Database, namespace, recordKey string) *store.ReplicatedRecordRow {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		row, err := d.GetReplicatedRecord(namespace, recordKey)
		if err == nil && row != nil && row.Revision > 0 {
			return row
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for replicated record ns=%s key=%s", namespace, recordKey)
	return nil
}

func seedAdminTestIdentity(t *testing.T, rt *Runtime) peer.ID {
	t.Helper()
	priv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair local: %v", err)
	}
	localID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey local: %v", err)
	}
	mlsPub, mlsPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey MLS: %v", err)
	}
	if err := rt.db.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "Local",
		PublicKey:         mlsPub,
		SigningKeyPrivate: append([]byte(nil), mlsPriv...),
		Credential:        []byte{7, 8, 9},
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}
	rt.privKey = priv
	return localID
}

func seedAdminTestGroup(t *testing.T, rt *Runtime, groupID string, localID peer.ID, role string) {
	t.Helper()
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    groupID,
		GroupState: []byte("state"),
		MyRole:     coordination.RoleCreator,
		GroupType:  "group",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   groupID,
		PeerID:    localID.String(),
		Role:      role,
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember local: %v", err)
	}
}

func seedAdminTestMember(t *testing.T, rt *Runtime, groupID, role string) peer.ID {
	t.Helper()
	priv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair target: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey target: %v", err)
	}
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   groupID,
		PeerID:    pid.String(),
		Role:      role,
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember target: %v", err)
	}
	return pid
}

func TestSetGroupMemberAdmin_CreatorGrantAndRevoke(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localID := seedAdminTestIdentity(t, rt)
	seedAdminTestGroup(t, rt, "group-admin", localID, store.GroupMemberRoleCreator)
	targetID := seedAdminTestMember(t, rt, "group-admin", store.GroupMemberRoleMember)

	if err := rt.SetGroupMemberAdmin("group-admin", targetID.String(), true); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	target, err := rt.db.GetGroupMember("group-admin", targetID.String())
	if err != nil {
		t.Fatalf("GetGroupMember grant: %v", err)
	}
	if target.Role != store.GroupMemberRoleAdmin {
		t.Fatalf("role after grant=%q, want admin", target.Role)
	}

	if err := rt.SetGroupMemberAdmin("group-admin", targetID.String(), false); err != nil {
		t.Fatalf("revoke admin: %v", err)
	}
	target, err = rt.db.GetGroupMember("group-admin", targetID.String())
	if err != nil {
		t.Fatalf("GetGroupMember revoke: %v", err)
	}
	if target.Role != store.GroupMemberRoleMember {
		t.Fatalf("role after revoke=%q, want member", target.Role)
	}
}

func TestSetGroupMemberAdmin_RejectsCreatorRevoke(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localID := seedAdminTestIdentity(t, rt)
	seedAdminTestGroup(t, rt, "group-admin", localID, store.GroupMemberRoleCreator)

	if err := rt.SetGroupMemberAdmin("group-admin", localID.String(), false); err == nil {
		t.Fatal("expected error when revoking creator admin role")
	}
}

func TestRemoveMemberFromGroup_CreatorMustDemoteAdminFirst(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localID := seedAdminTestIdentity(t, rt)
	attachActiveCreatorCoordinator(t, rt, "group-admin", localID)
	targetID := seedAdminTestMember(t, rt, "group-admin", store.GroupMemberRoleAdmin)

	if err := rt.RemoveMemberFromGroup("group-admin", targetID.String()); !errors.Is(err, ErrRemoveAdminRequiresDemote) {
		t.Fatalf("remove admin err=%v, want ErrRemoveAdminRequiresDemote", err)
	}

	if err := rt.db.SetGroupMemberRole("group-admin", targetID.String(), store.GroupMemberRoleMember); err != nil {
		t.Fatalf("demote target: %v", err)
	}
	origGetVerifiedTokenPublicKey := getVerifiedTokenPublicKey
	t.Cleanup(func() { getVerifiedTokenPublicKey = origGetVerifiedTokenPublicKey })
	getVerifiedTokenPublicKey = func(_ *p2p.P2PNode, target peer.ID) []byte {
		if target == targetID {
			return []byte("target-mls-pubkey")
		}
		return nil
	}
	if err := rt.RemoveMemberFromGroup("group-admin", targetID.String()); err != nil {
		t.Fatalf("remove after demote: %v", err)
	}
}

func TestRPCSubmitInviteRequest_AdminAuthorityAccepted(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localID := seedAdminTestIdentity(t, rt)
	seedAdminTestGroup(t, rt, "group-admin", localID, store.GroupMemberRoleAdmin)
	requesterID := seedAdminTestMember(t, rt, "group-admin", store.GroupMemberRoleMember)
	targetID := seedAdminTestMember(t, rt, "other-group", store.GroupMemberRoleMember)
	if err := rt.db.SetGroupInvitePolicy("group-admin", store.GroupInvitePolicyCreatorApproval); err != nil {
		t.Fatalf("SetGroupInvitePolicy: %v", err)
	}
	rec, err := rt.rpcSubmitInviteRequest(requesterID, &p2p.GroupInviteWireClientReqV1{
		V:            1,
		Op:           "submit",
		GroupID:      "group-admin",
		TargetPeerID: targetID.String(),
	})
	if err != nil {
		t.Fatalf("rpcSubmitInviteRequest: %v", err)
	}
	if rec == nil || rec.RequesterPeerID != requesterID.String() || rec.TargetPeerID != targetID.String() {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if rec.Status != store.InviteRequestStatusPending {
		t.Fatalf("status=%q want pending", rec.Status)
	}
}

func TestSetGroupInvitePolicy_AdminAllowedAndReplicated(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localID := seedAdminTestIdentity(t, rt)
	seedAdminTestGroup(t, rt, "group-admin", localID, store.GroupMemberRoleAdmin)

	if err := rt.SetGroupInvitePolicy("group-admin", store.GroupInvitePolicyAnyMember); err != nil {
		t.Fatalf("SetGroupInvitePolicy: %v", err)
	}
	got, err := rt.db.GetGroupInvitePolicy("group-admin")
	if err != nil {
		t.Fatalf("GetGroupInvitePolicy: %v", err)
	}
	if got != store.GroupInvitePolicyAnyMember {
		t.Fatalf("policy=%q want %q", got, store.GroupInvitePolicyAnyMember)
	}

	row := waitReplicatedRecord(t, rt.db, store.NamespaceGroupInvitePolicyV1, "group-admin")
	var wire groupInvitePolicyWireV1
	if err := json.Unmarshal([]byte(row.BodyJSON), &wire); err != nil {
		t.Fatalf("unmarshal replicated policy wire: %v", err)
	}
	if wire.ActorPeerID != localID.String() {
		t.Fatalf("actor_peer_id=%q want %q", wire.ActorPeerID, localID.String())
	}
	if wire.Policy != store.GroupInvitePolicyAnyMember {
		t.Fatalf("wire policy=%q want %q", wire.Policy, store.GroupInvitePolicyAnyMember)
	}
}

func TestRPCSyncInviteRequest_AdminAuthorityAccepted(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localID := seedAdminTestIdentity(t, rt)
	seedAdminTestGroup(t, rt, "group-admin", localID, store.GroupMemberRoleAdmin)
	requesterID := seedAdminTestMember(t, rt, "group-admin", store.GroupMemberRoleMember)
	now := time.Now().Unix()
	if err := rt.db.CreateGroupInviteRequest(store.GroupInviteRequestRecord{
		RequestID:       "req-sync-admin",
		GroupID:         "group-admin",
		RequesterPeerID: requesterID.String(),
		TargetPeerID:    seedAdminTestMember(t, rt, "other-group", store.GroupMemberRoleMember).String(),
		Status:          store.InviteRequestStatusPending,
		MaxAttempts:     maxInviteRetryAttempts,
		ExpiresAt:       now + 3600,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("CreateGroupInviteRequest: %v", err)
	}

	rec, err := rt.rpcSyncInviteRequest(requesterID, &p2p.GroupInviteWireClientReqV1{
		V: 1, Op: "sync", RequestID: "req-sync-admin",
	})
	if err != nil {
		t.Fatalf("rpcSyncInviteRequest: %v", err)
	}
	if rec == nil || rec.RequestID != "req-sync-admin" {
		t.Fatalf("unexpected record: %+v", rec)
	}
}

func TestApplyInviteRequestPushFromCreator_AdminAcceptedMemberRejected(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localID := seedAdminTestIdentity(t, rt)
	seedAdminTestGroup(t, rt, "group-admin", localID, store.GroupMemberRoleMember)
	adminID := seedAdminTestMember(t, rt, "group-admin", store.GroupMemberRoleAdmin)
	memberID := seedAdminTestMember(t, rt, "group-admin", store.GroupMemberRoleMember)

	adminPush := &p2p.GroupInviteWirePushV1{
		V: 1,
		Op: "push",
		Record: p2p.InviteRequestRecordWire{
			RequestID:       "req-admin-push",
			GroupID:         "group-admin",
			RequesterPeerID: localID.String(),
			TargetPeerID:    testPeerID(t),
			Status:          store.InviteRequestStatusApproved,
			MaxAttempts:     maxInviteRetryAttempts,
			ExpiresAt:       time.Now().Unix() + 3600,
			IsMirror:        false,
			CreatedAt:       time.Now().Unix(),
			UpdatedAt:       time.Now().Unix(),
		},
	}
	rt.applyInviteRequestPushFromCreator(adminID, adminPush)
	got, err := rt.db.GetGroupInviteRequest("req-admin-push")
	if err != nil {
		t.Fatalf("GetGroupInviteRequest admin push: %v", err)
	}
	if !got.IsMirror || got.Status != store.InviteRequestStatusApproved {
		t.Fatalf("unexpected mirrored row: %+v", got)
	}

	memberPush := &p2p.GroupInviteWirePushV1{
		V: 1,
		Op: "push",
		Record: p2p.InviteRequestRecordWire{
			RequestID:       "req-member-push",
			GroupID:         "group-admin",
			RequesterPeerID: localID.String(),
			TargetPeerID:    testPeerID(t),
			Status:          store.InviteRequestStatusRejected,
			MaxAttempts:     maxInviteRetryAttempts,
			ExpiresAt:       time.Now().Unix() + 3600,
			IsMirror:        false,
			CreatedAt:       time.Now().Unix(),
			UpdatedAt:       time.Now().Unix(),
		},
	}
	rt.applyInviteRequestPushFromCreator(memberID, memberPush)
	if _, err := rt.db.GetGroupInviteRequest("req-member-push"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("member push should be ignored, err=%v", err)
	}
}
