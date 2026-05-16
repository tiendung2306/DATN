package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/coordination"
)

func TestSaveGroupChatAvatar_CreatorSetsAndClears(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	rt.coordStorage = store.NewSQLiteCoordinationStorage(d)
	now := time.Now()
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	gid := "g-local-avatar"
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    gid,
		GroupState: []byte{1},
		MyRole:     coordination.RoleCreator,
		GroupType:  "group",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := rt.SaveGroupChatAvatar(gid, png, 1); err != nil {
		t.Fatalf("SaveGroupChatAvatar: %v", err)
	}
	groups, err := rt.GetGroups()
	if err != nil {
		t.Fatalf("GetGroups: %v", err)
	}
	var found string
	for _, g := range groups {
		if g.GroupID == gid {
			found = g.GroupAvatarDataURL
			break
		}
	}
	if found == "" {
		t.Fatalf("expected group_avatar_data_url set")
	}
	if err := rt.SaveGroupChatAvatar(gid, nil, 2); err != nil {
		t.Fatalf("clear SaveGroupChatAvatar: %v", err)
	}
	groups2, err := rt.GetGroups()
	if err != nil {
		t.Fatalf("GetGroups2: %v", err)
	}
	for _, g := range groups2 {
		if g.GroupID == gid && g.GroupAvatarDataURL != "" {
			t.Fatalf("expected avatar cleared, got %q", g.GroupAvatarDataURL)
		}
	}
}

func TestSaveGroupChatAvatar_MemberDenied(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	rt.coordStorage = store.NewSQLiteCoordinationStorage(d)
	now := time.Now()
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	gid := "g-member"
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
	if err := rt.SaveGroupChatAvatar(gid, png, 1); err == nil {
		t.Fatalf("expected error for non-creator")
	}
}

func TestSaveGroupChatAvatar_ChannelRejected(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	rt.coordStorage = store.NewSQLiteCoordinationStorage(d)
	now := time.Now()
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	gid := "chan-1"
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    gid,
		GroupState: []byte{1},
		MyRole:     coordination.RoleCreator,
		GroupType:  "channel",
		CategoryID: "cat",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := rt.SaveGroupChatAvatar(gid, png, 1); err == nil {
		t.Fatalf("expected error for channel")
	}
}

func waitReplicatedGroupAvatarRow(t *testing.T, d *store.Database, gid string) *store.ReplicatedRecordRow {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		row, err := d.GetReplicatedRecord(store.NamespaceGroupAvatarV1, gid)
		if err == nil && row != nil && row.Revision > 0 {
			return row
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout waiting for replicated group avatar row")
	return nil
}

func TestSaveGroupChatAvatar_PersistsReplicatedRecord(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	rt.SetContext(context.Background())
	rt.coordStorage = store.NewSQLiteCoordinationStorage(d)
	now := time.Now()
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	gid := "g-repl-avatar"
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    gid,
		GroupState: []byte{1},
		MyRole:     coordination.RoleCreator,
		GroupType:  "group",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := rt.SaveGroupChatAvatar(gid, png, 1); err != nil {
		t.Fatalf("SaveGroupChatAvatar: %v", err)
	}
	row := waitReplicatedGroupAvatarRow(t, d, gid)
	if row.Namespace != store.NamespaceGroupAvatarV1 || row.RecordKey != gid {
		t.Fatalf("unexpected row ns/key: %+v", row)
	}
	var w groupAvatarWireV1
	if err := json.Unmarshal([]byte(row.BodyJSON), &w); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	if w.Revision < 1 || w.GroupID != gid {
		t.Fatalf("bad wire: %+v", w)
	}
	if store.AvatarContentHash(png) != strings.TrimSpace(strings.ToLower(w.AvatarHash)) {
		t.Fatalf("wire hash mismatch: %q vs computed", w.AvatarHash)
	}
}

func TestApplySignedRemoteGroupAvatarPush_MemberMerges(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	rt.SetContext(context.Background())
	rt.coordStorage = store.NewSQLiteCoordinationStorage(d)
	now := time.Now()
	gid := "g-recv-avatar"
	creatorPeer := "12D3KooWKcreatorPeerIDhere000000000"
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
		GroupID: gid, PeerID: creatorPeer, DisplayName: "Cr",
		Role: "creator", Status: store.GroupMemberStatusActive, Source: "test",
	}); err != nil {
		t.Fatalf("UpsertGroupMember creator: %v", err)
	}
	if err := d.UpsertGroupMember(store.GroupMemberRecord{
		GroupID: gid, PeerID: info.PeerID, DisplayName: "Me",
		Role: "member", Status: store.GroupMemberStatusActive, Source: "test",
	}); err != nil {
		t.Fatalf("UpsertGroupMember self: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey(creatorPeer, "Creator", hex.EncodeToString(pub)); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	hash := store.AvatarContentHash(png)
	if err := d.UpsertAvatarBlob(hash, "image/png", png); err != nil {
		t.Fatalf("UpsertAvatarBlob: %v", err)
	}
	wire := groupAvatarWireV1{
		V:               groupAvatarWireVersion,
		GroupID:         gid,
		CreatorPeerID:   creatorPeer,
		AvatarHash:      hash,
		AvatarMime:      "image/png",
		AvatarUpdatedAt: 4242,
		Revision:        1,
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sig := ed25519.Sign(priv, raw)
	if err := rt.applySignedRemoteGroupAvatarPush(creatorPeer, raw, sig, png); err != nil {
		t.Fatalf("applySignedRemoteGroupAvatarPush: %v", err)
	}
	h, mime, at, err := d.GetGroupChatAvatarMeta(gid)
	if err != nil {
		t.Fatalf("GetGroupChatAvatarMeta: %v", err)
	}
	if h != hash || mime != "image/png" || at != 4242 {
		t.Fatalf("meta mismatch h=%q mime=%q at=%d", h, mime, at)
	}
}

func TestApplySignedRemoteGroupAvatarPush_RejectsNonCreatorSigner(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	rt.SetContext(context.Background())
	rt.coordStorage = store.NewSQLiteCoordinationStorage(d)
	now := time.Now()
	gid := "g-reject-nc"
	creatorPeer := "12D3KooWKcreatorPeerIDhere000000000"
	pubC, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pubM, privM, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey member: %v", err)
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
		GroupID: gid, PeerID: creatorPeer, DisplayName: "Cr",
		Role: "creator", Status: store.GroupMemberStatusActive, Source: "test",
	}); err != nil {
		t.Fatalf("UpsertGroupMember creator: %v", err)
	}
	if err := d.UpsertGroupMember(store.GroupMemberRecord{
		GroupID: gid, PeerID: info.PeerID, DisplayName: "Me",
		Role: "member", Status: store.GroupMemberStatusActive, Source: "test",
	}); err != nil {
		t.Fatalf("UpsertGroupMember self: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey(creatorPeer, "Creator", hex.EncodeToString(pubC)); err != nil {
		t.Fatalf("UpsertPeerProfile creator: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey(info.PeerID, "Me", hex.EncodeToString(pubM)); err != nil {
		t.Fatalf("UpsertPeerProfile member: %v", err)
	}
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	hash := store.AvatarContentHash(png)
	wire := groupAvatarWireV1{
		V:               groupAvatarWireVersion,
		GroupID:         gid,
		CreatorPeerID:   info.PeerID,
		AvatarHash:      hash,
		AvatarMime:      "image/png",
		AvatarUpdatedAt: 1,
		Revision:        1,
	}
	raw, _ := json.Marshal(wire)
	sig := ed25519.Sign(privM, raw)
	if err := rt.applySignedRemoteGroupAvatarPush(info.PeerID, raw, sig, png); err == nil {
		t.Fatal("expected error when member is not roster creator")
	}
}

func TestApplySignedRemoteGroupAvatarPush_StaleRevision(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	rt.SetContext(context.Background())
	rt.coordStorage = store.NewSQLiteCoordinationStorage(d)
	now := time.Now()
	gid := "g-stale-av"
	creatorPeer := "12D3KooWKcreatorPeerIDhere000000000"
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := rt.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    gid,
		GroupState: []byte{1},
		MyRole:     coordination.RoleCreator,
		GroupType:  "group",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	if err := d.UpsertGroupMember(store.GroupMemberRecord{
		GroupID: gid, PeerID: creatorPeer, DisplayName: "Cr",
		Role: "creator", Status: store.GroupMemberStatusActive, Source: "test",
	}); err != nil {
		t.Fatalf("UpsertGroupMember: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey(creatorPeer, "Creator", hex.EncodeToString(pub)); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	hash := store.AvatarContentHash(png)
	if err := d.UpsertAvatarBlob(hash, "image/png", png); err != nil {
		t.Fatalf("UpsertAvatarBlob: %v", err)
	}
	mk := func(rev int64) ([]byte, []byte) {
		wire := groupAvatarWireV1{
			V:               groupAvatarWireVersion,
			GroupID:         gid,
			CreatorPeerID:   creatorPeer,
			AvatarHash:      hash,
			AvatarMime:      "image/png",
			AvatarUpdatedAt: rev,
			Revision:        rev,
		}
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return raw, ed25519.Sign(priv, raw)
	}
	r1, s1 := mk(1)
	if err := rt.applySignedRemoteGroupAvatarPush(creatorPeer, r1, s1, png); err != nil {
		t.Fatalf("apply rev1: %v", err)
	}
	r2, s2 := mk(2)
	if err := rt.applySignedRemoteGroupAvatarPush(creatorPeer, r2, s2, png); err != nil {
		t.Fatalf("apply rev2: %v", err)
	}
	rStale, sStale := mk(1)
	err = rt.applySignedRemoteGroupAvatarPush(creatorPeer, rStale, sStale, png)
	if err == nil {
		t.Fatal("expected stale error")
	}
	if !errors.Is(err, errReplicationStaleGroupAvatar) {
		t.Fatalf("want stale group avatar, got %v", err)
	}
}
