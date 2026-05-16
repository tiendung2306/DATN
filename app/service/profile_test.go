package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"testing"

	"app/adapter/p2p"
	"app/adapter/store"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func testProfileRuntime(t *testing.T, d *store.Database) *Runtime {
	t.Helper()
	libp2pPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	rawLib, err := p2pCrypto.MarshalPrivateKey(libp2pPriv)
	if err != nil {
		t.Fatalf("MarshalPrivateKey: %v", err)
	}
	if err := d.SetConfig(p2p.Libp2pPrivKeyConfigKey, rawLib); err != nil {
		t.Fatalf("SetConfig libp2p: %v", err)
	}

	pubMLS, privMLS, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	if err := d.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "ProfileTester",
		PublicKey:         pubMLS,
		SigningKeyPrivate: append([]byte(nil), privMLS...),
		Credential:        []byte{},
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}

	rt := NewRuntime(nil)
	rt.SetContext(context.Background())
	rt.mu.Lock()
	rt.db = d
	rt.privKey = libp2pPriv
	rt.mu.Unlock()
	return rt
}

func TestApplySignedPeerProfile_AcceptsGreaterRevision(t *testing.T) {
	d := openServiceTestDB(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	rt := testProfileRuntime(t, d)
	info, err := p2p.GetOnboardingInfo(d, rt.privKey)
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	peerID := info.PeerID
	pubHex := hex.EncodeToString(pub)
	if err := d.UpsertPeerProfileWithKey(peerID, "ProfileTester", pubHex); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}

	wire := profileWireV1{
		V:               profileWireVersion,
		PeerID:          peerID,
		DisplayName:     "ProfileTester",
		Email:           "peer@example.com",
		Phone:           "",
		AvatarHash:      "",
		AvatarMime:      "",
		AvatarUpdatedAt: 0,
		ProfileRevision: 1,
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	sig := ed25519.Sign(priv, raw)
	if err := rt.ApplySignedPeerProfile(peerID, raw, sig); err != nil {
		t.Fatalf("ApplySignedPeerProfile: %v", err)
	}
	row, err := d.GetPeerDirectoryProfile(peerID)
	if err != nil {
		t.Fatalf("GetPeerDirectoryProfile: %v", err)
	}
	if !row.Email.Valid || row.Email.String != "peer@example.com" {
		t.Fatalf("email=%+v", row.Email)
	}
	if row.ProfileRevision != 1 {
		t.Fatalf("revision=%d want 1", row.ProfileRevision)
	}
}

func TestApplySignedPeerProfile_RejectsStaleRevision(t *testing.T) {
	d := openServiceTestDB(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	rt := testProfileRuntime(t, d)
	info, err := p2p.GetOnboardingInfo(d, rt.privKey)
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	peerID := info.PeerID
	pubHex := hex.EncodeToString(pub)
	if err := d.UpsertPeerProfileWithKey(peerID, "ProfileTester", pubHex); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}

	mkWire := func(rev int64, email string) ([]byte, []byte) {
		wire := profileWireV1{
			V:               profileWireVersion,
			PeerID:          peerID,
			DisplayName:     "ProfileTester",
			Email:           email,
			Phone:           "",
			AvatarHash:      "",
			AvatarMime:      "",
			AvatarUpdatedAt: 0,
			ProfileRevision: rev,
		}
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		return raw, ed25519.Sign(priv, raw)
	}
	raw1, sig1 := mkWire(1, "a@b.c")
	if err := rt.ApplySignedPeerProfile(peerID, raw1, sig1); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	rawDup, sigDup := mkWire(1, "a@b.c")
	if err := rt.ApplySignedPeerProfile(peerID, rawDup, sigDup); err != nil {
		t.Fatalf("duplicate same revision+payload should be idempotent: %v", err)
	}
	raw2, sig2 := mkWire(2, "newer@x.y")
	if err := rt.ApplySignedPeerProfile(peerID, raw2, sig2); err != nil {
		t.Fatalf("apply rev 2: %v", err)
	}
	rawStale, sigStale := mkWire(1, "old@back")
	if err := rt.ApplySignedPeerProfile(peerID, rawStale, sigStale); err == nil {
		t.Fatal("expected error when new revision is lower than stored")
	}
}

func TestApplySignedPeerProfile_RejectsBadSignature(t *testing.T) {
	d := openServiceTestDB(t)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	rt := testProfileRuntime(t, d)
	info, err := p2p.GetOnboardingInfo(d, rt.privKey)
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	peerID := info.PeerID
	pubHex := hex.EncodeToString(pub)
	if err := d.UpsertPeerProfileWithKey(peerID, "ProfileTester", pubHex); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	wire := profileWireV1{
		V:               profileWireVersion,
		PeerID:          peerID,
		DisplayName:     "ProfileTester",
		ProfileRevision: 1,
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	_, wrongPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	badSig := ed25519.Sign(wrongPriv, raw)
	if err := rt.ApplySignedPeerProfile(peerID, raw, badSig); err == nil {
		t.Fatal("expected verify error")
	}
}

func TestApplySignedPeerProfile_ClearedFieldsProjectTombstone(t *testing.T) {
	d := openServiceTestDB(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	rt := testProfileRuntime(t, d)
	info, err := p2p.GetOnboardingInfo(d, rt.privKey)
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	peerID := info.PeerID
	if err := d.UpsertPeerProfileWithKey(peerID, "ProfileTester", hex.EncodeToString(pub)); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	sign := func(w profileWireV1) ([]byte, []byte) {
		raw, err := json.Marshal(w)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		return raw, ed25519.Sign(priv, raw)
	}
	raw1, sig1 := sign(profileWireV1{
		V:               profileWireVersion,
		PeerID:          peerID,
		DisplayName:     "ProfileTester",
		Email:           "clear-me@example.com",
		ProfileRevision: 1,
	})
	if err := rt.ApplySignedPeerProfile(peerID, raw1, sig1); err != nil {
		t.Fatalf("apply rev1: %v", err)
	}
	raw2, sig2 := sign(profileWireV1{
		V:               profileWireVersion,
		PeerID:          peerID,
		DisplayName:     "ProfileTester",
		ProfileRevision: 2,
		ClearedFields:   []string{"email"},
	})
	if err := rt.ApplySignedPeerProfile(peerID, raw2, sig2); err != nil {
		t.Fatalf("apply tombstone: %v", err)
	}
	row, err := d.GetPeerDirectoryProfile(peerID)
	if err != nil {
		t.Fatalf("GetPeerDirectoryProfile: %v", err)
	}
	if row.Email.Valid {
		t.Fatalf("email should be tombstoned, got %+v", row.Email)
	}
}

func TestUpdateMyProfile_RoundTrip(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	if err := rt.UpdateMyProfile(UpdateUserProfileRequest{Email: "me@example.com", Phone: "+100"}); err != nil {
		t.Fatalf("UpdateMyProfile: %v", err)
	}
	info, err := rt.GetMyProfile()
	if err != nil {
		t.Fatalf("GetMyProfile: %v", err)
	}
	if info.Email != "me@example.com" || info.Phone != "+100" {
		t.Fatalf("profile email=%q phone=%q", info.Email, info.Phone)
	}
	if info.DisplayName != "ProfileTester" {
		t.Fatalf("display_name=%q", info.DisplayName)
	}
}

func TestSaveMyProfile_MinimalPNGAvatar(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	got, err := rt.SaveMyProfile(UpdateUserProfileRequest{}, png, 1)
	if err != nil {
		t.Fatalf("SaveMyProfile: %v", err)
	}
	if got.AvatarHash == "" || got.AvatarDataURL == "" {
		t.Fatalf("expected avatar populated, got %#v", got)
	}
}

func TestSaveMyProfile_StoresReplicatedRecordBlobRefAndClearsIt(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	got, err := rt.SaveMyProfile(UpdateUserProfileRequest{}, png, 1)
	if err != nil {
		t.Fatalf("SaveMyProfile: %v", err)
	}
	ob, err := p2p.GetOnboardingInfo(d, rt.privKey)
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	row, err := d.GetReplicatedRecord(store.NamespaceUserProfileV1, ob.PeerID)
	if err != nil {
		t.Fatalf("GetReplicatedRecord: %v", err)
	}
	if row.Revision != 1 {
		t.Fatalf("replicated revision=%d want 1", row.Revision)
	}
	refs, err := d.GetReplicatedRecordBlobRefs(store.NamespaceUserProfileV1, ob.PeerID)
	if err != nil {
		t.Fatalf("GetReplicatedRecordBlobRefs: %v", err)
	}
	if len(refs) != 1 || refs[0].Hash != got.AvatarHash || !refs[0].Required {
		t.Fatalf("unexpected blob refs: %+v, avatar hash %q", refs, got.AvatarHash)
	}
	if err := rt.ClearMyAvatar(); err != nil {
		t.Fatalf("ClearMyAvatar: %v", err)
	}
	refs, err = d.GetReplicatedRecordBlobRefs(store.NamespaceUserProfileV1, ob.PeerID)
	if err != nil {
		t.Fatalf("GetReplicatedRecordBlobRefs after clear: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("blob refs should be cleared with avatar tombstone, got %+v", refs)
	}
}

func TestSaveMyProfile_RejectsNonImageAvatar(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	if _, err := rt.SaveMyProfile(UpdateUserProfileRequest{}, []byte("not an image"), 1); err == nil {
		t.Fatal("expected format error")
	}
}

func TestValidateAvatarImageBytes_PNGUnixLineEnding(t *testing.T) {
	data := append([]byte{0x89, 0x50, 0x4e, 0x47, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 64)...)
	mime, err := validateAvatarImageBytes(data)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if mime != "image/png" {
		t.Fatalf("mime=%q", mime)
	}
}

func TestGetMyProfile_ResolvesAvatarWhenPeerDirectoryMimeNull(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	if _, err := rt.SaveMyProfile(UpdateUserProfileRequest{}, png, 1); err != nil {
		t.Fatalf("SaveMyProfile: %v", err)
	}
	ob, err := p2p.GetOnboardingInfo(d, rt.privKey)
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	if _, err := d.Conn.Exec(`UPDATE peer_directory SET avatar_mime = NULL WHERE peer_id = ?`, ob.PeerID); err != nil {
		t.Fatalf("clear peer avatar_mime: %v", err)
	}
	prof, err := rt.GetMyProfile()
	if err != nil {
		t.Fatalf("GetMyProfile: %v", err)
	}
	if prof.AvatarDataURL == "" {
		t.Fatal("expected non-empty avatar_data_url when blob exists but peer_directory.avatar_mime is NULL")
	}
}

func TestBuildReplicatedProfilePullRequestIncludesReplicaKeys(t *testing.T) {
	d := openServiceTestDB(t)
	rt := testProfileRuntime(t, d)
	self := peer.ID("self-peer")
	remote := peer.ID("replica-peer")
	selfID := self.String()
	remoteID := remote.String()
	if err := d.UpsertPeerProfileWithKey(selfID, "Self", ""); err != nil {
		t.Fatalf("self profile: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey("owner-offline", "Offline Owner", ""); err != nil {
		t.Fatalf("offline owner profile: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey(remoteID, "Replica", ""); err != nil {
		t.Fatalf("replica profile: %v", err)
	}
	if err := d.UpsertReplicatedPullCursor(remoteID, store.NamespaceUserProfileV1, "owner-offline", 7); err != nil {
		t.Fatalf("cursor: %v", err)
	}
	req, err := rt.buildReplicatedProfilePullRequest(remote, self, d)
	if err != nil {
		t.Fatalf("buildReplicatedProfilePullRequest: %v", err)
	}
	seen := map[string]bool{}
	for _, k := range req.Keys {
		seen[k] = true
	}
	if !seen[remoteID] {
		t.Fatalf("request should include direct remote profile key: %+v", req.Keys)
	}
	if !seen["owner-offline"] {
		t.Fatalf("request should include known owner key for replica recovery: %+v", req.Keys)
	}
	if seen[selfID] {
		t.Fatalf("request should not include local self key: %+v", req.Keys)
	}
	if req.Cursors["owner-offline"] != 7 {
		t.Fatalf("cursor owner-offline=%d want 7", req.Cursors["owner-offline"])
	}
}

func openServiceTestDB(t *testing.T) *store.Database {
	t.Helper()
	d, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
