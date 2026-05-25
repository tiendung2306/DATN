package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/coordination"
)

func TestApplySignedRemoteGroupInvitePolicyPush_AdminActorApplies(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	rt.SetContext(context.Background())
	rt.coordStorage = store.NewSQLiteCoordinationStorage(d)
	gid := "g-policy-admin"
	now := time.Now()
	adminPeer := "12D3KooWPolicyAdminPeer000000000000"
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    gid,
		GroupState: []byte{1},
		MyRole:     coordination.RoleMember,
		GroupType:  "group",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	info, err := p2p.GetOnboardingInfo(d, rt.privKey)
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	if err := d.UpsertGroupMember(store.GroupMemberRecord{
		GroupID: gid, PeerID: adminPeer, DisplayName: "Admin",
		Role: store.GroupMemberRoleAdmin, Status: store.GroupMemberStatusActive, Source: "test",
	}); err != nil {
		t.Fatalf("UpsertGroupMember admin: %v", err)
	}
	if err := d.UpsertGroupMember(store.GroupMemberRecord{
		GroupID: gid, PeerID: info.PeerID, DisplayName: "Me",
		Role: store.GroupMemberRoleMember, Status: store.GroupMemberStatusActive, Source: "test",
	}); err != nil {
		t.Fatalf("UpsertGroupMember self: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey(adminPeer, "Admin", hex.EncodeToString(pub)); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	wire := groupInvitePolicyWireV1{
		V:           groupInvitePolicyWireVersion,
		GroupID:     gid,
		Policy:      store.GroupInvitePolicyAnyMember,
		ActorPeerID: adminPeer,
		CreatedAt:   now.Unix(),
		Revision:    1,
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sig := ed25519.Sign(priv, raw)
	if err := rt.applySignedRemoteGroupInvitePolicyPush(adminPeer, raw, sig); err != nil {
		t.Fatalf("applySignedRemoteGroupInvitePolicyPush: %v", err)
	}
	got, err := d.GetGroupInvitePolicy(gid)
	if err != nil {
		t.Fatalf("GetGroupInvitePolicy: %v", err)
	}
	if got != store.GroupInvitePolicyAnyMember {
		t.Fatalf("policy=%q want %q", got, store.GroupInvitePolicyAnyMember)
	}
}
