//go:build business_integration

// Sprint 2 — BI-032–BI-043 (members: KP, add/join, errors, remove matrix, access revoked).

package service

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestBusinessP1_Sprint2_BI032_CreatorRoleInRoster(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "bi032"
	if err := rt.CreateGroupChat(gid, "dm", ""); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	rt.mu.RLock()
	db := rt.db
	rt.mu.RUnlock()
	if db == nil {
		t.Fatal("nil db")
	}
	recs, err := db.ListGroupMembers(gid, store.GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	var creator bool
	for _, r := range recs {
		if r.Role == "creator" {
			creator = true
			break
		}
	}
	if !creator {
		t.Fatal("expected creator role row in DB")
	}
}

func TestBusinessP1_Sprint2_BI033_GenerateKeyPackage(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	kp, err := rt.GenerateKeyPackage()
	if err != nil {
		t.Fatalf("GenerateKeyPackage: %v", err)
	}
	if kp.PublicHex == "" || kp.BundlePrivateHex == "" {
		t.Fatalf("empty kp fields: %#v", kp)
	}
}

func TestBusinessP1_Sprint2_BI034_AddMember_ReturnsWelcome(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-034"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	kp, err := rt.GenerateKeyPackage()
	if err != nil {
		t.Fatal(err)
	}
	remotePeer := testPeerID(t)
	welcomeHex, err := rt.AddMemberToGroup(gid, remotePeer, kp.PublicHex)
	if err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}
	if welcomeHex == "" {
		t.Fatal("empty welcome hex")
	}
}

func TestBusinessP1_Sprint2_BI035_JoinWithWelcome(t *testing.T) {
	aliceRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, aliceRoot)
	alice, _ := businessRuntimeStartMockInWorkDir(t, aliceRoot)
	defer businessShutdownRuntimeInWorkDir(t, alice)

	gid := "grp-035"
	cat := businessEnsureCategory(t, alice, "BI-035")
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
	aInfo, _ := alice.GetOnboardingInfo()
	bInfo, _ := bob.GetOnboardingInfo()
	if aInfo.PeerID == bInfo.PeerID {
		t.Fatal("alice and bob share peer id")
	}
	welcomeHex, err := alice.AddMemberToGroup(gid, bInfo.PeerID, kp.PublicHex)
	if err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}
	if err := bob.JoinGroupWithWelcome(gid, welcomeHex, kp.BundlePrivateHex); err != nil {
		t.Fatalf("JoinGroupWithWelcome: %v", err)
	}
}

func TestBusinessP1_Sprint2_BI036_AddMember_InvalidKP(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if err := rt.CreateGroupChat("g-badkp", "group", ""); err != nil {
		t.Fatal(err)
	}
	peerStr := testPeerID(t)
	_, err := rt.AddMemberToGroup("g-badkp", peerStr, "not-hex!!!")
	if err == nil {
		t.Fatal("expected error for bad KP hex")
	}
}

func TestBusinessP1_Sprint2_BI037_RosterAfterJoin_LocalEvidence(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	cat := businessEnsureCategory(t, rt, "BI-037")
	gid := "grp-037"
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	aInfo, _ := rt.GetOnboardingInfo()
	am, err := rt.GetGroupMembers(gid)
	if err != nil {
		t.Fatal(err)
	}
	var sawAlice bool
	for _, m := range am {
		if m.PeerID == aInfo.PeerID {
			sawAlice = true
			break
		}
	}
	if !sawAlice {
		t.Fatal("creator missing from roster after create")
	}
	peerRemote := testPeerID(t)
	kp, err := rt.GenerateKeyPackage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.AddMemberToGroup(gid, peerRemote, kp.PublicHex); err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}
	am2, err := rt.GetGroupMembers(gid)
	if err != nil {
		t.Fatal(err)
	}
	var sawRemote bool
	for _, m := range am2 {
		if m.PeerID == peerRemote {
			sawRemote = true
			break
		}
	}
	if !sawRemote {
		t.Fatal("added peer missing from roster after AddMember")
	}
}

func TestBusinessP1_Sprint2_BI038_Leave_BlocksSend(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "leave-038"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	if err := rt.SendGroupMessage(gid, "ping"); err != nil {
		t.Fatal(err)
	}
	if err := rt.LeaveGroup(gid); err != nil {
		t.Fatal(err)
	}
	err := rt.SendGroupMessage(gid, "after leave")
	if err == nil {
		t.Fatal("expected send blocked after leave")
	}
	if !strings.Contains(err.Error(), "not in group") {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint2_BI039_RemoveMember_HappyPath(t *testing.T) {
	orig := getVerifiedTokenPublicKey
	t.Cleanup(func() { getVerifiedTokenPublicKey = orig })

	aliceRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, aliceRoot)
	alice, _ := businessRuntimeStartMockInWorkDir(t, aliceRoot)
	defer businessShutdownRuntimeInWorkDir(t, alice)

	gid := "grp-039"
	cat := businessEnsureCategory(t, alice, "BI-039")
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
	bobPID, err := peer.Decode(bInfo.PeerID)
	if err != nil {
		t.Fatal(err)
	}
	getVerifiedTokenPublicKey = func(_ *p2p.P2PNode, target peer.ID) []byte {
		if target == bobPID {
			return bizMLSIdentityPubFromRuntimeDB(t, bob)
		}
		return nil
	}

	welcomeHex, err := alice.AddMemberToGroup(gid, bInfo.PeerID, kp.PublicHex)
	if err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}
	if err := bob.JoinGroupWithWelcome(gid, welcomeHex, kp.BundlePrivateHex); err != nil {
		t.Fatalf("JoinGroupWithWelcome: %v", err)
	}
	if err := alice.RemoveMemberFromGroup(gid, bInfo.PeerID); err != nil {
		t.Fatalf("RemoveMemberFromGroup: %v", err)
	}
}

func TestBusinessP1_Sprint2_BI040_RemoveMember_ForbiddenNonCreator(t *testing.T) {
	aliceRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, aliceRoot)
	alice, _ := businessRuntimeStartMockInWorkDir(t, aliceRoot)
	defer businessShutdownRuntimeInWorkDir(t, alice)

	gid := "grp-040"
	cat := businessEnsureCategory(t, alice, "BI-040")
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
	aInfo, _ := alice.GetOnboardingInfo()
	welcomeHex, err := alice.AddMemberToGroup(gid, bInfo.PeerID, kp.PublicHex)
	if err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}
	if err := bob.JoinGroupWithWelcome(gid, welcomeHex, kp.BundlePrivateHex); err != nil {
		t.Fatalf("JoinGroupWithWelcome: %v", err)
	}
	err = bob.RemoveMemberFromGroup(gid, aInfo.PeerID)
	if !errors.Is(err, ErrRemoveMemberForbidden) {
		t.Fatalf("RemoveMemberFromGroup err=%v want ErrRemoveMemberForbidden", err)
	}
}

func TestBusinessP1_Sprint2_BI041_RemoveMember_SelfBlocked(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-041"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	info, _ := rt.GetOnboardingInfo()
	err := rt.RemoveMemberFromGroup(gid, info.PeerID)
	if !errors.Is(err, ErrRemoveSelfNotAllowed) {
		t.Fatalf("err=%v want self", err)
	}
}

func TestBusinessP1_Sprint2_BI042_RemoveMember_PeerNotVerified(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-042"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	target := testPeerID(t)
	err := rt.RemoveMemberFromGroup(gid, target)
	if !errors.Is(err, ErrRemoveMemberPeerNotKnown) {
		t.Fatalf("err=%v want peer not verified", err)
	}
}

func TestBusinessP1_Sprint2_BI043_AccessRevoked_BlocksSend(t *testing.T) {
	rt, mock := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "grp-043"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	localPub := bizMLSIdentityPubFromRuntimeDB(t, rt)
	mock.SetHasMemberFunc(func(_ []byte, identity []byte) (bool, error) {
		return bytes.Equal(identity, localPub), nil
	})
	kp, err := rt.GenerateKeyPackage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.AddMemberToGroup(gid, testPeerID(t), kp.PublicHex); err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}
	mock.SetHasMemberFunc(func([]byte, []byte) (bool, error) {
		return false, nil
	})
	err = rt.SendGroupMessage(gid, "after revoke")
	if !errors.Is(err, coordination.ErrAccessRevoked) {
		t.Fatalf("SendGroupMessage err=%v want ErrAccessRevoked", err)
	}
}

func TestBusinessP1_Sprint2_GroupMetadata_CreatorRoleRecord(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "meta-check"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	cs := rt.coordStorage
	rt.mu.RUnlock()
	rec, err := cs.GetGroupRecord(gid)
	if err != nil {
		t.Fatalf("GetGroupRecord: %v", err)
	}
	if rec.MyRole != coordination.RoleCreator {
		t.Fatalf("MyRole=%v want creator", rec.MyRole)
	}
}
