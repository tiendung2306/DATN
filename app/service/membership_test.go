package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/config"
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

type membershipTestTransport struct {
	local peer.ID
}

func (t *membershipTestTransport) Publish(context.Context, string, []byte) error {
	return nil
}
func (t *membershipTestTransport) Subscribe(string, func(peer.ID, []byte)) error { return nil }
func (t *membershipTestTransport) Unsubscribe(string) error                      { return nil }
func (t *membershipTestTransport) SendDirect(context.Context, peer.ID, []byte) error {
	return nil
}
func (t *membershipTestTransport) LocalPeerID() peer.ID      { return t.local }
func (t *membershipTestTransport) ConnectedPeers() []peer.ID { return []peer.ID{t.local} }

type membershipTestMLSEngine struct{}

func (m *membershipTestMLSEngine) CreateGroup(context.Context, string, []byte) ([]byte, []byte, error) {
	return []byte("state-0"), []byte("tree-0"), nil
}
func (m *membershipTestMLSEngine) CreateProposal(context.Context, []byte, coordination.ProposalType, []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) CreateCommit(context.Context, []byte, [][]byte) ([]byte, []byte, []byte, []byte, error) {
	return nil, nil, nil, nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) ProcessCommit(context.Context, []byte, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) ProcessWelcome(context.Context, []byte, []byte, []byte) ([]byte, []byte, uint64, error) {
	return nil, nil, 0, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) GenerateKeyPackage(context.Context, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) AddMembers(context.Context, []byte, [][]byte) ([]byte, []byte, []byte, []byte, error) {
	return nil, nil, nil, nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) RemoveMembers(context.Context, []byte, [][]byte) ([]byte, []byte, []byte, error) {
	return []byte("commit-1"), []byte("state-1"), []byte("tree-1"), nil
}
func (m *membershipTestMLSEngine) HasMember(context.Context, []byte, []byte) (bool, error) {
	return true, nil
}
func (m *membershipTestMLSEngine) ListMemberIdentities(context.Context, []byte) ([][]byte, error) {
	return nil, nil
}
func (m *membershipTestMLSEngine) EncryptMessage(context.Context, []byte, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) DecryptMessage(context.Context, []byte, []byte) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) ExternalJoin(context.Context, []byte, []byte) ([]byte, []byte, []byte, error) {
	return nil, nil, nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) ExportGroupInfo(context.Context, []byte, bool) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (m *membershipTestMLSEngine) ExportSecret(context.Context, []byte, string, []byte, int) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func attachActiveCreatorCoordinator(t *testing.T, rt *Runtime, groupID string, localID peer.ID) *coordination.Coordinator {
	t.Helper()

	coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
		Config:     coordination.DefaultConfig(),
		Transport:  &membershipTestTransport{local: localID},
		Clock:      coordination.RealClock{},
		MLS:        &membershipTestMLSEngine{},
		Storage:    rt.coordStorage,
		LocalID:    localID,
		GroupID:    groupID,
		SigningKey: []byte("test-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}
	if err := coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := coord.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(coord.Stop)

	rt.mu.Lock()
	rt.coordinators[groupID] = coord
	rt.mu.Unlock()
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   groupID,
		PeerID:    localID.String(),
		Role:      store.GroupMemberRoleCreator,
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember local creator: %v", err)
	}
	return coord
}

func attachMembershipCoordinator(t *testing.T, rt *Runtime, groupID string, localID peer.ID) *coordination.Coordinator {
	t.Helper()

	coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
		Config:     coordination.DefaultConfig(),
		Transport:  &membershipTestTransport{local: localID},
		Clock:      coordination.RealClock{},
		MLS:        &membershipTestMLSEngine{},
		Storage:    rt.coordStorage,
		LocalID:    localID,
		GroupID:    groupID,
		SigningKey: []byte("test-signing-key"),
	})
	if err != nil {
		t.Fatalf("NewCoordinator: %v", err)
	}
	if err := coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := coord.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(coord.Stop)

	rt.mu.Lock()
	rt.coordinators[groupID] = coord
	rt.mu.Unlock()
	return coord
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
		GroupID:      "group-1",
		Epoch:        1,
		SenderID:     sender,
		Content:      []byte("hello"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: 1000, NodeID: sender.String()},
		EnvelopeHash: []byte("history-env-1"),
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	if err := rt.LeaveGroup("group-1"); err != nil {
		t.Fatalf("LeaveGroup: %v", err)
	}

	messages, err := rt.GetGroupMessages("group-1", 50, 0)
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

func TestLeaveGroupCreatorBlocked(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair local: %v", err)
	}
	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey local: %v", err)
	}
	if err := rt.db.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "Local",
		PublicKey:         []byte{1, 2, 3},
		SigningKeyPrivate: []byte{4, 5, 6},
		Credential:        []byte{7, 8, 9},
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}
	rt.privKey = localPriv
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-creator-leave",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleCreator,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   "group-creator-leave",
		PeerID:    localPeerID.String(),
		Role:      store.GroupMemberRoleCreator,
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember local: %v", err)
	}

	if err := rt.LeaveGroup("group-creator-leave"); !errors.Is(err, ErrCreatorCannotLeave) {
		t.Fatalf("LeaveGroup err=%v want ErrCreatorCannotLeave", err)
	}
}

func TestRemoveMemberFromGroupCreatorOnly(t *testing.T) {
	rt := setupMembershipRuntime(t)
	now := time.Now()
	localPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair local: %v", err)
	}
	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey local: %v", err)
	}
	if err := rt.db.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "Local",
		PublicKey:         []byte{1, 2, 3},
		SigningKeyPrivate: []byte{4, 5, 6},
		Credential:        []byte{7, 8, 9},
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}
	rt.privKey = localPriv
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-1",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}

	priv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   "group-1",
		PeerID:    localPeerID.String(),
		Role:      store.GroupMemberRoleMember,
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember local: %v", err)
	}
	if err := rt.RemoveMemberFromGroup("group-1", peerID.String()); !errors.Is(err, ErrRemoveMemberForbidden) {
		t.Fatalf("RemoveMemberFromGroup err = %v, want ErrRemoveMemberForbidden", err)
	}
}

func TestRemoveMemberFromGroupRejectsSelf(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair local: %v", err)
	}
	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey local: %v", err)
	}
	if err := rt.db.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "Local",
		PublicKey:         []byte{1, 2, 3},
		SigningKeyPrivate: []byte{4, 5, 6},
		Credential:        []byte{7, 8, 9},
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}
	rt.privKey = localPriv

	attachActiveCreatorCoordinator(t, rt, "group-1", localPeerID)
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   "group-1",
		PeerID:    localPeerID.String(),
		Role:      "creator",
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember local: %v", err)
	}

	if err := rt.RemoveMemberFromGroup("group-1", localPeerID.String()); !errors.Is(err, ErrRemoveSelfNotAllowed) {
		t.Fatalf("RemoveMemberFromGroup self err = %v, want ErrRemoveSelfNotAllowed", err)
	}
}

func TestRemoveMemberFromGroupSuccessMarksTargetLeft(t *testing.T) {
	rt := setupMembershipRuntime(t)
	origGetVerifiedTokenPublicKey := getVerifiedTokenPublicKey
	t.Cleanup(func() { getVerifiedTokenPublicKey = origGetVerifiedTokenPublicKey })

	localPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair local: %v", err)
	}
	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey local: %v", err)
	}
	targetPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair target: %v", err)
	}
	targetPeerID, err := peer.IDFromPrivateKey(targetPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey target: %v", err)
	}
	if err := rt.db.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "Local",
		PublicKey:         []byte{1, 2, 3},
		SigningKeyPrivate: []byte{4, 5, 6},
		Credential:        []byte{7, 8, 9},
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}
	rt.privKey = localPriv

	attachActiveCreatorCoordinator(t, rt, "group-1", localPeerID)
	if err := rt.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:   "group-1",
		PeerID:    targetPeerID.String(),
		Role:      "member",
		Status:    store.GroupMemberStatusActive,
		Source:    "test",
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("UpsertGroupMember target: %v", err)
	}
	getVerifiedTokenPublicKey = func(_ *p2p.P2PNode, target peer.ID) []byte {
		if target == targetPeerID {
			return []byte("target-mls-pubkey")
		}
		return nil
	}

	if err := rt.RemoveMemberFromGroup("group-1", targetPeerID.String()); err != nil {
		t.Fatalf("RemoveMemberFromGroup: %v", err)
	}

	rows, err := rt.db.ListGroupMembers("group-1")
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	foundTarget := false
	for _, row := range rows {
		if row.PeerID == targetPeerID.String() {
			foundTarget = true
			if row.Status != store.GroupMemberStatusLeft {
				t.Fatalf("target status = %s, want %s", row.Status, store.GroupMemberStatusLeft)
			}
		}
	}
	if !foundTarget {
		t.Fatalf("target member row not found")
	}
}

func TestRemoveMemberFromGroupFailsWhenTargetNotVerified(t *testing.T) {
	rt := setupMembershipRuntime(t)
	origGetVerifiedTokenPublicKey := getVerifiedTokenPublicKey
	t.Cleanup(func() { getVerifiedTokenPublicKey = origGetVerifiedTokenPublicKey })
	getVerifiedTokenPublicKey = func(_ *p2p.P2PNode, _ peer.ID) []byte { return nil }

	localPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair local: %v", err)
	}
	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey local: %v", err)
	}
	targetPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair target: %v", err)
	}
	targetPeerID, err := peer.IDFromPrivateKey(targetPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey target: %v", err)
	}
	if err := rt.db.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "Local",
		PublicKey:         []byte{1, 2, 3},
		SigningKeyPrivate: []byte{4, 5, 6},
		Credential:        []byte{7, 8, 9},
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}
	rt.privKey = localPriv
	attachActiveCreatorCoordinator(t, rt, "group-1", localPeerID)

	err = rt.RemoveMemberFromGroup("group-1", targetPeerID.String())
	if err == nil {
		t.Fatal("expected remove to fail when target is not verified")
	}
}

func TestRemoveMemberFromGroup_AdminCannotRemoveAdmin(t *testing.T) {
	rt := setupMembershipRuntime(t)
	localID := seedAdminTestIdentity(t, rt)
	attachMembershipCoordinator(t, rt, "group-admin-remove", localID)
	seedAdminTestGroup(t, rt, "group-admin-remove", localID, store.GroupMemberRoleAdmin)
	targetID := seedAdminTestMember(t, rt, "group-admin-remove", store.GroupMemberRoleAdmin)

	if err := rt.RemoveMemberFromGroup("group-admin-remove", targetID.String()); !errors.Is(err, ErrRemoveAdminForbidden) {
		t.Fatalf("RemoveMemberFromGroup err=%v want ErrRemoveAdminForbidden", err)
	}
}

func TestRemoveMemberFromGroup_AdminCanRemoveMember(t *testing.T) {
	rt := setupMembershipRuntime(t)
	origGetVerifiedTokenPublicKey := getVerifiedTokenPublicKey
	t.Cleanup(func() { getVerifiedTokenPublicKey = origGetVerifiedTokenPublicKey })

	localID := seedAdminTestIdentity(t, rt)
	attachMembershipCoordinator(t, rt, "group-admin-remove-ok", localID)
	seedAdminTestGroup(t, rt, "group-admin-remove-ok", localID, store.GroupMemberRoleAdmin)
	targetID := seedAdminTestMember(t, rt, "group-admin-remove-ok", store.GroupMemberRoleMember)
	getVerifiedTokenPublicKey = func(_ *p2p.P2PNode, target peer.ID) []byte {
		if target == targetID {
			return []byte("target-mls-pubkey")
		}
		return nil
	}

	if err := rt.RemoveMemberFromGroup("group-admin-remove-ok", targetID.String()); err != nil {
		t.Fatalf("RemoveMemberFromGroup: %v", err)
	}
	got, err := rt.db.GetGroupMember("group-admin-remove-ok", targetID.String())
	if err != nil {
		t.Fatalf("GetGroupMember target: %v", err)
	}
	if got.Status != store.GroupMemberStatusLeft {
		t.Fatalf("target status=%q want %q", got.Status, store.GroupMemberStatusLeft)
	}
}

func TestAccessLostHandler_EmitsGroupLeftRemovedAndMarksGroupLeft(t *testing.T) {
	rt := setupMembershipRuntime(t)
	rt.cfg = &config.Config{RuntimeEventReplay: true}
	now := time.Now()
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    "group-access-lost",
		GroupState: []byte("state"),
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}

	handler := rt.makeAccessLostHandler("group-access-lost")
	handler("group-access-lost", 9, "removed")

	active, err := rt.db.IsGroupActive("group-access-lost")
	if err != nil {
		t.Fatalf("IsGroupActive: %v", err)
	}
	if active {
		t.Fatalf("group should be marked left after access lost")
	}

	events, err := rt.GetRuntimeEventsSince(0, 50)
	if err != nil {
		t.Fatalf("GetRuntimeEventsSince: %v", err)
	}
	found := false
	for _, ev := range events {
		if ev.Topic != "group:left" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(ev.PayloadJSON), &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload["group_id"] == "group-access-lost" && payload["reason"] == "removed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing group:left(reason=removed) event for access lost")
	}
}
