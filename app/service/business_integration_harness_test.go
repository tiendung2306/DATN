//go:build business_integration

package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/admin"
	"app/config"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Run from app/: go test -tags=business_integration ./service -count=1 -run 'TestBusinessP0|TestBusinessP1_Sprint2'
//
// Sprint 2 injects r.mlsEngine after Startup() so CreateGroupChat/SendGroupMessage work without the Rust sidecar.
// Same-package assignment is intentional for tests only.

func businessIntegrationChdirToTemp(t *testing.T) (tempRoot string) {
	t.Helper()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldwd)
	})
	return dir
}

func businessNewRuntime(cfg *config.Config) *Runtime {
	rt := NewRuntime(cfg)
	rt.SetContext(context.Background())
	return rt
}

// businessSeedDBUninitialized creates an on-disk DB with schema but no MLS identity (UNINITIALIZED path).
func businessSeedDBUninitialized(t *testing.T, dbPath string) {
	t.Helper()
	d, err := store.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	d.Close()
}

// businessSeedAwaitingBundleUser writes libp2p key + MLS identity; app state is AWAITING_BUNDLE.
// Returns values needed to build a matching InvitationBundle.
func businessSeedAwaitingBundleUser(t *testing.T, dbPath string) (libp2pPriv p2pCrypto.PrivKey, peerIDStr string, mlsPubHex string) {
	t.Helper()
	libp2pPriv, _, err := p2pCrypto.GenerateKeyPair(p2pCrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(libp2pPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}
	pubMLS, privMLS, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	rawLib, err := p2pCrypto.MarshalPrivateKey(libp2pPriv)
	if err != nil {
		t.Fatalf("MarshalPrivateKey: %v", err)
	}

	d, err := store.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer d.Close()

	if err := d.SetConfig(p2p.Libp2pPrivKeyConfigKey, rawLib); err != nil {
		t.Fatalf("SetConfig libp2p: %v", err)
	}
	if err := d.SaveMLSIdentity(&store.MLSIdentity{
		PublicKey:         pubMLS,
		SigningKeyPrivate: append([]byte(nil), privMLS...),
		Credential:        nil,
	}); err != nil {
		t.Fatalf("SaveMLSIdentity: %v", err)
	}

	return libp2pPriv, pid.String(), hex.EncodeToString(pubMLS)
}

// businessCreateSignedInvitationBundle signs a bundle using a fresh admin key (isolated in-memory DB).
func businessCreateSignedInvitationBundle(t *testing.T, displayName, userPeerID, mlsPubHex, bootstrapAddr string) []byte {
	t.Helper()
	adminDB, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB admin: %v", err)
	}
	defer adminDB.Close()

	if _, err := admin.SetupAdminKey(adminDB, "business-integration-admin-pass"); err != nil {
		t.Fatalf("SetupAdminKey: %v", err)
	}
	adminPriv, err := admin.UnlockAdminKey(adminDB, "business-integration-admin-pass")
	if err != nil {
		t.Fatalf("UnlockAdminKey: %v", err)
	}
	bundleJSON, err := admin.CreateInvitationBundle(adminPriv, displayName, userPeerID, mlsPubHex, bootstrapAddr)
	if err != nil {
		t.Fatalf("CreateInvitationBundle: %v", err)
	}
	return bundleJSON
}

// businessImportBundleIntoDB opens dbPath and imports the bundle bytes (same checks as GUI import).
func businessImportBundleIntoDB(t *testing.T, dbPath string, libp2pPriv p2pCrypto.PrivKey, bundleJSON []byte) {
	t.Helper()
	d, err := store.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer d.Close()
	if err := p2p.ImportInvitationBundle(d, libp2pPriv, bundleJSON); err != nil {
		t.Fatalf("ImportInvitationBundle: %v", err)
	}
}

// businessSeedAuthorizedUser writes awaiting-bundle state then imports a valid signed bundle so state becomes AUTHORIZED.
func businessSeedAuthorizedUser(t *testing.T, dbPath string) {
	t.Helper()
	libp2pPriv, peerIDStr, mlsPubHex := businessSeedAwaitingBundleUser(t, dbPath)
	bootstrapPeer := testPeerID(t)
	bootstrapAddr := "/ip4/127.0.0.1/tcp/4001/p2p/" + bootstrapPeer
	bundle := businessCreateSignedInvitationBundle(t, "BizUser", peerIDStr, mlsPubHex, bootstrapAddr)
	businessImportBundleIntoDB(t, dbPath, libp2pPriv, bundle)
}

func businessDefaultConfig(dbPath string) *config.Config {
	return &config.Config{
		DBPath:             dbPath,
		P2PPort:            0,
		BootstrapAddr:      "",
		RuntimeEventReplay: false,
	}
}

func businessDBPath(tempRoot string) string {
	return filepath.Join(tempRoot, "app.db")
}

// businessRuntimeAuthorizedWithMockMLS seeds an AUTHORIZED user, starts P2P,
// and assigns a deterministic MLS mock (always, even if a sidecar exists).
func businessRuntimeAuthorizedWithMockMLS(t *testing.T) (*Runtime, *businessIntegrationMLSMock) {
	t.Helper()
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedAuthorizedUser(t, dbPath)
	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	mock := newBusinessIntegrationMLSMock()
	rt.mlsEngine = mock
	t.Cleanup(func() { rt.Shutdown(context.Background()) })
	return rt, mock
}

func businessEnsureCategory(t *testing.T, rt *Runtime, name string) string {
	t.Helper()
	info, err := rt.CreateChannelCategory(name)
	if err != nil {
		t.Fatalf("CreateChannelCategory: %v", err)
	}
	if info.CategoryID == "" {
		t.Fatal("empty category id")
	}
	return info.CategoryID
}

func businessSeedAuthorizedWorkDir(t *testing.T, root string) {
	t.Helper()
	businessSeedAuthorizedUser(t, businessDBPath(root))
}

func businessRuntimeStartMockInWorkDir(t *testing.T, root string) (*Runtime, *businessIntegrationMLSMock) {
	t.Helper()
	dbPath := businessDBPath(root)
	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	mock := newBusinessIntegrationMLSMock()
	rt.mlsEngine = mock
	return rt, mock
}

func businessShutdownRuntimeInWorkDir(t *testing.T, rt *Runtime) {
	t.Helper()
	if rt != nil {
		rt.Shutdown(context.Background())
	}
}

func bizMLSIdentityPubFromRuntimeDB(t *testing.T, rt *Runtime) []byte {
	t.Helper()
	rt.mu.RLock()
	d := rt.db
	rt.mu.RUnlock()
	if d == nil {
		t.Fatal("nil db")
	}
	id, err := d.GetMLSIdentity()
	if err != nil {
		t.Fatalf("GetMLSIdentity: %v", err)
	}
	return append([]byte(nil), id.PublicKey...)
}
