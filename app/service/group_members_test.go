package service

import (
	"strings"
	"testing"
	"time"

	"app/adapter/store"
	"app/coordination"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestGetGroupMembers_UsesRosterForOfflineMembers(t *testing.T) {
	rt := setupMembershipRuntime(t)
	_, pubOnline, err := p2pCrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("GenerateEd25519Key online: %v", err)
	}
	onlinePeerID, err := peer.IDFromPublicKey(pubOnline)
	if err != nil {
		t.Fatalf("IDFromPublicKey online: %v", err)
	}
	_, pubOffline, err := p2pCrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("GenerateEd25519Key offline: %v", err)
	}
	offlinePeerID, err := peer.IDFromPublicKey(pubOffline)
	if err != nil {
		t.Fatalf("IDFromPublicKey offline: %v", err)
	}
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-roster",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     "group-roster",
		PeerID:      onlinePeerID.String(),
		DisplayName: "Online User",
		Role:        "member",
		Status:      store.GroupMemberStatusActive,
		Source:      "invite",
	}); err != nil {
		t.Fatalf("UpsertGroupMember peer-online: %v", err)
	}
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID: "group-roster",
		PeerID:  offlinePeerID.String(),
		Role:    "member",
		Status:  store.GroupMemberStatusActive,
		Source:  "invite",
	}); err != nil {
		t.Fatalf("UpsertGroupMember peer-offline: %v", err)
	}

	members, err := rt.GetGroupMembers("group-roster")
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members len=%d, want 2", len(members))
	}
	var foundFallback bool
	for _, m := range members {
		if m.PeerID == offlinePeerID.String() {
			foundFallback = true
			if strings.TrimSpace(m.DisplayName) == "" {
				t.Fatalf("peer-offline display name should have fallback")
			}
		}
	}
	if !foundFallback {
		t.Fatalf("peer-offline missing from members: %+v", members)
	}
}

func TestGetGroupMembers_BackfillsFromKnownSenders(t *testing.T) {
	rt := setupMembershipRuntime(t)
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-backfill",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	_, pub, err := p2pCrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	senderID, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("IDFromPublicKey: %v", err)
	}
	if err := rt.coordStorage.SaveMessage(&coordination.StoredMessage{
		GroupID:      "group-backfill",
		Epoch:        1,
		SenderID:     senderID,
		Content:      []byte("hello"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: 1000, Counter: 0, NodeID: "n1"},
		EnvelopeHash: []byte{1, 2, 3, 4},
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	members, err := rt.GetGroupMembers("group-backfill")
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(members) == 0 {
		t.Fatalf("expected backfilled members, got empty")
	}
	found := false
	for _, m := range members {
		if m.PeerID == senderID.String() {
			found = true
		}
	}
	if !found {
		t.Fatalf("peer-history not found in backfilled roster: %+v", members)
	}
}

func TestGetGroupMembers_PendingInviteDoesNotCreateActiveMember(t *testing.T) {
	rt := setupMembershipRuntime(t)
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-pending",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleCreator,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}

	_, localPub, err := p2pCrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("GenerateEd25519Key local: %v", err)
	}
	localPeerID, err := peer.IDFromPublicKey(localPub)
	if err != nil {
		t.Fatalf("IDFromPublicKey local: %v", err)
	}
	_, inviteePub, err := p2pCrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("GenerateEd25519Key invitee: %v", err)
	}
	inviteePeerID, err := peer.IDFromPublicKey(inviteePub)
	if err != nil {
		t.Fatalf("IDFromPublicKey invitee: %v", err)
	}

	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     "group-pending",
		PeerID:      localPeerID.String(),
		DisplayName: "Creator",
		Role:        "creator",
		Status:      store.GroupMemberStatusActive,
		Source:      "create",
	}); err != nil {
		t.Fatalf("UpsertGroupMember creator: %v", err)
	}

	if err := rt.db.SavePendingWelcome(inviteePeerID.String(), "group-pending", []byte("welcome-payload")); err != nil {
		t.Fatalf("SavePendingWelcome: %v", err)
	}

	members, err := rt.GetGroupMembers("group-pending")
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("members len=%d, want 1 (invitee must remain pending)", len(members))
	}
	if members[0].PeerID != localPeerID.String() {
		t.Fatalf("unexpected member: %+v", members[0])
	}
}
