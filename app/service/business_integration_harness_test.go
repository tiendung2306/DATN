//go:build business_integration

package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// businessRuntimeAuthorizedWithMockMLSAndEventReplay enables durable runtime event replay (DB-backed seq).
func businessRuntimeAuthorizedWithMockMLSAndEventReplay(t *testing.T) (*Runtime, *businessIntegrationMLSMock) {
	t.Helper()
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedAuthorizedUser(t, dbPath)
	cfg := businessDefaultConfig(dbPath)
	cfg.RuntimeEventReplay = true
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

// businessPersistMockKPBundle persists the mock MLS KeyPackage bundle so ApplyInvite/join paths and
// GetKPStatus see a stored KP (GenerateKeyPackage alone only returns hex).
func businessPersistMockKPBundle(t *testing.T, rt *Runtime) {
	t.Helper()
	kp, err := rt.GenerateKeyPackage()
	if err != nil {
		t.Fatalf("GenerateKeyPackage: %v", err)
	}
	pubKP, err := hex.DecodeString(kp.PublicHex)
	if err != nil {
		t.Fatal(err)
	}
	privKP, err := hex.DecodeString(kp.BundlePrivateHex)
	if err != nil {
		t.Fatal(err)
	}
	info, err := rt.GetOnboardingInfo()
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()
	if database == nil {
		t.Fatal("nil db")
	}
	if err := database.SaveKPBundle(info.PeerID, pubKP, privKP); err != nil {
		t.Fatalf("SaveKPBundle: %v", err)
	}
}

// businessSeedStoredKeyPackageForPeer puts the invitee's public KeyPackage into the inviter's DB so
// InvitePeerToGroup can load KP via GetStoredKeyPackage without a live P2P fetch.
func businessSeedStoredKeyPackageForPeer(t *testing.T, inviter *Runtime, targetPeerID string, publicKPHex string) {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimSpace(publicKPHex))
	if err != nil {
		t.Fatalf("decode KP hex: %v", err)
	}
	targetPeerID = strings.TrimSpace(targetPeerID)
	if targetPeerID == "" {
		t.Fatal("empty target peer id")
	}
	inviter.mu.RLock()
	d := inviter.db
	inviter.mu.RUnlock()
	if d == nil {
		t.Fatal("nil db")
	}
	if err := d.SaveStoredKeyPackage(targetPeerID, raw, "business_integration_seed"); err != nil {
		t.Fatalf("SaveStoredKeyPackage: %v", err)
	}
}

// businessSeedInviteRequest inserts a group_invite_requests row on rt's DB (same schema as runtime flows).
func businessSeedInviteRequest(t *testing.T, rt *Runtime, rec store.GroupInviteRequestRecord) {
	t.Helper()
	if strings.TrimSpace(rec.RequestID) == "" {
		t.Fatal("request id required")
	}
	if rec.ExpiresAt <= 0 {
		rec.ExpiresAt = time.Now().Unix() + 3600
	}
	rt.mu.RLock()
	d := rt.db
	rt.mu.RUnlock()
	if d == nil {
		t.Fatal("nil db")
	}
	if err := d.CreateGroupInviteRequest(rec); err != nil {
		t.Fatalf("CreateGroupInviteRequest: %v", err)
	}
}

// businessSeedGroupInvitePolicy / sprint4MirrorInvitePolicyAfterCreator /
// sprint4EnsureAliceOnBobMemberTable were removed (2026-05-10). Members no
// longer drive invite-request flow from local state — they always forward
// to the creator over the wire (rpcSubmitInviteRequest), which is the only
// node holding the MLS Token under the Single-Writer Invariant. The new
// integration tests exercise the creator-side handler directly, so the old
// "seed local member-side state" helpers are no longer needed.

// sprint4AliceBobJoinedChannel creates a channel group on Alice and joins Bob
// through the real wire-path chokepoint (`savePendingInviteFromWelcome`) so
// every test built on top of this helper exercises auto-join semantics
// end-to-end. If a regression silently turns auto-join back into manual
// Accept-only, every dependent test fails immediately at this helper.
func sprint4AliceBobJoinedChannel(t *testing.T, gid string) (*Runtime, *Runtime) {
	t.Helper()
	aliceRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, aliceRoot)
	alice, _ := businessRuntimeStartMockInWorkDir(t, aliceRoot)
	cat := businessEnsureCategory(t, alice, "S4")
	if err := alice.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}

	bobRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, bobRoot)
	bob, _ := businessRuntimeStartMockInWorkDir(t, bobRoot)

	// Persist Bob's KP bundle so applyWelcome can resolve the private bundle
	// when the wire-path delivery runs (this is what
	// app/service/invite.go:advertiseKeyPackage does in production).
	businessPersistMockKPBundle(t, bob)
	bInfo, _ := bob.GetOnboardingInfo()
	bob.mu.RLock()
	bobDB := bob.db
	bob.mu.RUnlock()
	bobPublicKP, _, kpErr := bobDB.GetKPBundle(bInfo.PeerID)
	if kpErr != nil {
		t.Fatalf("Bob persisted KP missing: %v", kpErr)
	}

	welcomeHex, err := alice.AddMemberToGroup(gid, bInfo.PeerID, hex.EncodeToString(bobPublicKP))
	if err != nil {
		t.Fatalf("AddMemberToGroup: %v", err)
	}
	welcomeBytes, err := hex.DecodeString(welcomeHex)
	if err != nil {
		t.Fatalf("decode welcome hex: %v", err)
	}
	aInfo, _ := alice.GetOnboardingInfo()

	// Wire-path delivery: this is the SAME entry point used by
	// handleWelcomeDelivery (direct stream), refreshPendingInvites
	// (replication), and checkStoredWelcomes (blind store). If auto-join
	// regresses, this call leaves Bob in pending state and the assertion
	// below fires loud.
	if err := bob.savePendingInviteFromWelcome(gid, "channel", "", welcomeBytes, aInfo.PeerID, false); err != nil {
		t.Fatalf("savePendingInviteFromWelcome (wire path): %v", err)
	}
	if has, herr := bobDB.HasGroup(gid); herr != nil || !has {
		t.Fatalf("regression: Bob did not auto-join through wire path; HasGroup=%v err=%v", has, herr)
	}

	t.Cleanup(func() {
		businessShutdownRuntimeInWorkDir(t, bob)
		businessShutdownRuntimeInWorkDir(t, alice)
	})
	return alice, bob
}
