package service

import (
	"errors"
	"testing"
	"time"

	"app/adapter/store"
	"app/coordination"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func setupMembershipRuntime(t *testing.T) *Runtime {
	t.Helper()
	d, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	return &Runtime{
		db:           d,
		coordStorage: store.NewSQLiteCoordinationStorage(d),
		coordinators: make(map[string]*coordination.Coordinator),
	}
}

func TestLeaveGroupSoftLeaveIsIdempotent(t *testing.T) {
	rt := setupMembershipRuntime(t)
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-1",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}

	if err := rt.LeaveGroup("group-1"); err != nil {
		t.Fatalf("LeaveGroup first: %v", err)
	}
	if err := rt.LeaveGroup("group-1"); err != nil {
		t.Fatalf("LeaveGroup second: %v", err)
	}

	active, err := rt.db.IsGroupActive("group-1")
	if err != nil {
		t.Fatalf("IsGroupActive: %v", err)
	}
	if active {
		t.Fatalf("group still active after leave")
	}
	groups, err := rt.GetGroups()
	if err != nil {
		t.Fatalf("GetGroups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("GetGroups returned left group: %+v", groups)
	}
}

func TestLeaveGroupKeepsMessageHistoryReadable(t *testing.T) {
	rt := setupMembershipRuntime(t)
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-1",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	sender := peer.ID("peer-a")
	if err := rt.coordStorage.SaveMessage(&coordination.StoredMessage{
		GroupID:   "group-1",
		Epoch:     1,
		SenderID:  sender,
		Content:   []byte("hello"),
		Timestamp: coordination.HLCTimestamp{WallTimeMs: 1000, NodeID: sender.String()},
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	if err := rt.LeaveGroup("group-1"); err != nil {
		t.Fatalf("LeaveGroup: %v", err)
	}

	messages, err := rt.GetGroupMessages("group-1")
	if err != nil {
		t.Fatalf("GetGroupMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("message history mismatch: %+v", messages)
	}
}

func TestLeaveGroupMissingGroup(t *testing.T) {
	rt := setupMembershipRuntime(t)
	if err := rt.LeaveGroup("missing"); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("LeaveGroup missing err = %v, want ErrGroupNotFound", err)
	}
}

func TestRemoveMemberFromGroupUnsupported(t *testing.T) {
	rt := setupMembershipRuntime(t)
	priv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}
	if err := rt.RemoveMemberFromGroup("group-1", peerID.String()); !errors.Is(err, ErrRemoveMemberNotSupported) {
		t.Fatalf("RemoveMemberFromGroup err = %v, want ErrRemoveMemberNotSupported", err)
	}
}
