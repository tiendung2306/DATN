package service

import (
	"bytes"
	"path/filepath"
	"testing"

	"app/adapter/store"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	p2pPeer "github.com/libp2p/go-libp2p/core/peer"
)

func TestIdentityBackupRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := store.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer database.Close()

	priv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	peerID, err := p2pPeer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}

	identity := &store.MLSIdentity{
		DisplayName:       "alice",
		PublicKey:         []byte{0x01, 0x02, 0x03},
		SigningKeyPrivate: []byte{0x04, 0x05, 0x06},
		Credential:        []byte("alice"),
	}
	if err := database.SaveMLSIdentity(identity); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}
	bundle := &store.StoredAuthBundle{
		DisplayName:    "Alice Admin",
		PeerID:         peerID.String(),
		PublicKey:      []byte{0x01, 0x02, 0x03},
		TokenIssuedAt:  111,
		TokenExpiresAt: 222,
		TokenSignature: []byte{0x07, 0x08},
		BootstrapAddr:  "/ip4/127.0.0.1/tcp/4001/p2p/" + peerID.String(),
		RootPublicKey:  []byte{0x09, 0x10},
	}
	if err := database.SaveAuthBundle(bundle); err != nil {
		t.Fatalf("SaveAuthBundle: %v", err)
	}
	if err := database.RestoreGroupsFromBackup([]store.BackupGroupRecord{
		{
			GroupID:    "g1",
			GroupState: []byte("state-1"),
			Epoch:      7,
			TreeHash:   []byte("th-1"),
			MyRole:     "member",
		},
	}); err != nil {
		t.Fatalf("RestoreGroupsFromBackup seed: %v", err)
	}
	if err := database.RestoreStoredMessagesFromBackup([]store.BackupStoredMessage{
		{
			GroupID:       "g1",
			Epoch:         7,
			SenderID:      peerID.String(),
			Content:       []byte("hello"),
			HLCWallTimeMs: 1000,
			HLCCounter:    1,
			HLCNodeID:     peerID.String(),
		},
	}); err != nil {
		t.Fatalf("RestoreStoredMessagesFromBackup seed: %v", err)
	}
	if err := database.RestoreKPBundlesFromBackup([]store.BackupKPBundle{
		{PeerID: peerID.String(), PublicKP: []byte("pk"), PrivateBundle: []byte("kb")},
	}); err != nil {
		t.Fatalf("RestoreKPBundlesFromBackup seed: %v", err)
	}
	if err := database.RestorePendingWelcomesFromBackup([]store.BackupPendingWelcome{
		{TargetPeerID: peerID.String(), GroupID: "g1", WelcomeBytes: []byte("welcome")},
	}); err != nil {
		t.Fatalf("RestorePendingWelcomesFromBackup seed: %v", err)
	}
	if err := database.RestorePendingInvitesFromBackup([]store.BackupPendingInvite{
		{GroupID: "g2", WelcomeBytes: []byte("welcome-2"), SourcePeerID: "store-peer", Status: store.PendingInviteStatusPending},
	}); err != nil {
		t.Fatalf("RestorePendingInvitesFromBackup seed: %v", err)
	}

	backupBytes, err := ExportIdentityBackup(database, priv, "passphrase")
	if err != nil {
		t.Fatalf("ExportIdentityBackup: %v", err)
	}

	imported, err := ImportIdentityBackup(database, backupBytes, "passphrase")
	if err != nil {
		t.Fatalf("ImportIdentityBackup: %v", err)
	}
	if imported.BundlePeerID != peerID.String() {
		t.Fatalf("bundle peer mismatch: got %s", imported.BundlePeerID)
	}

	gotPrivRaw, err := database.GetConfig("libp2p_priv_key")
	if err != nil {
		t.Fatalf("GetConfig(libp2p_priv_key): %v", err)
	}
	expectedPrivRaw, err := p2pCrypto.MarshalPrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPrivateKey: %v", err)
	}
	if !bytes.Equal(gotPrivRaw, expectedPrivRaw) {
		t.Fatalf("libp2p private key mismatch")
	}

	gotIdentity, err := database.GetMLSIdentity()
	if err != nil {
		t.Fatalf("GetMLSIdentity: %v", err)
	}
	if gotIdentity.DisplayName != identity.DisplayName {
		t.Fatalf("identity display mismatch: got %q", gotIdentity.DisplayName)
	}
	if !bytes.Equal(gotIdentity.SigningKeyPrivate, identity.SigningKeyPrivate) {
		t.Fatalf("identity signing key mismatch")
	}

	gotBundle, err := database.GetAuthBundle()
	if err != nil {
		t.Fatalf("GetAuthBundle: %v", err)
	}
	if gotBundle.PeerID != bundle.PeerID {
		t.Fatalf("bundle peer id mismatch: got %q", gotBundle.PeerID)
	}
	if !bytes.Equal(gotBundle.RootPublicKey, bundle.RootPublicKey) {
		t.Fatalf("bundle root public key mismatch")
	}
	gotGroups, err := database.GetAllGroupsForBackup()
	if err != nil {
		t.Fatalf("GetAllGroupsForBackup: %v", err)
	}
	if len(gotGroups) != 1 || gotGroups[0].GroupID != "g1" {
		t.Fatalf("group restore mismatch: %+v", gotGroups)
	}
	gotMsgs, err := database.GetAllStoredMessagesForBackup()
	if err != nil {
		t.Fatalf("GetAllStoredMessagesForBackup: %v", err)
	}
	if len(gotMsgs) != 1 || string(gotMsgs[0].Content) != "hello" {
		t.Fatalf("message restore mismatch: %+v", gotMsgs)
	}
	gotInvites, err := database.ListPendingInvites(false)
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(gotInvites) != 1 || gotInvites[0].GroupID != "g2" {
		t.Fatalf("pending invite restore mismatch: %+v", gotInvites)
	}
}

func TestIdentityBackupWrongPassphrase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := store.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer database.Close()

	priv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	peerID, err := p2pPeer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}

	if err := database.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       "alice",
		PublicKey:         []byte{0x01},
		SigningKeyPrivate: []byte{0x02},
		Credential:        []byte("alice"),
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}
	if err := database.SaveAuthBundle(&store.StoredAuthBundle{
		DisplayName:    "alice",
		PeerID:         peerID.String(),
		PublicKey:      []byte{0x01},
		TokenIssuedAt:  1,
		TokenExpiresAt: 2,
		TokenSignature: []byte{0x03},
		BootstrapAddr:  "/ip4/127.0.0.1/tcp/4001/p2p/" + peerID.String(),
		RootPublicKey:  []byte{0x04},
	}); err != nil {
		t.Fatalf("SaveAuthBundle: %v", err)
	}

	backupBytes, err := ExportIdentityBackup(database, priv, "good-pass")
	if err != nil {
		t.Fatalf("ExportIdentityBackup: %v", err)
	}

	if _, err := ImportIdentityBackup(database, backupBytes, "bad-pass"); err == nil {
		t.Fatalf("expected error for wrong passphrase")
	}
}
