//go:build business_integration

package service

import (
	"context"
	"strings"
	"testing"

	"app/adapter/p2p"
	"app/adapter/store"
)

func TestBusinessP0_AppState_UninitializedAfterStartup(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedDBUninitialized(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	if got := rt.GetAppState(); got != "UNINITIALIZED" {
		t.Fatalf("GetAppState() = %q, want UNINITIALIZED", got)
	}
}

func TestBusinessP0_GenerateKeys_RequiresCryptoEngineWhenUninitialized(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedDBUninitialized(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	_, err := rt.GenerateKeys()
	if err == nil {
		t.Fatal("GenerateKeys: expected error when crypto sidecar is unavailable")
	}
	if !strings.Contains(err.Error(), "crypto engine") {
		t.Fatalf("GenerateKeys error = %v, want message about crypto engine", err)
	}
}

func TestBusinessP0_GenerateKeys_RejectsWhenMLSAlreadyExists(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedAwaitingBundleUser(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	_, err := rt.GenerateKeys()
	if err == nil {
		t.Fatal("expected error when MLS identity already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("GenerateKeys error = %v, want already exists", err)
	}
}

func TestBusinessP0_GetOnboardingInfo_AwaitingBundle(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	_, peerIDWant, pubHexWant := businessSeedAwaitingBundleUser(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	info, err := rt.GetOnboardingInfo()
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	if info.PeerID != peerIDWant || info.PublicKeyHex != pubHexWant {
		t.Fatalf("GetOnboardingInfo = %+v, want peer %s pub %s", info, peerIDWant, pubHexWant)
	}
}

func TestBusinessP0_ImportBundle_BecomesAuthorized(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	_, peerIDStr, mlsPubHex := businessSeedAwaitingBundleUser(t, dbPath)
	bootstrapPeer := testPeerID(t)
	bootstrapAddr := "/ip4/127.0.0.1/tcp/4001/p2p/" + bootstrapPeer
	bundle := businessCreateSignedInvitationBundle(t, "Alice", peerIDStr, mlsPubHex, bootstrapAddr)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	if rt.GetAppState() != "AWAITING_BUNDLE" {
		t.Fatalf("pre-import state = %s, want AWAITING_BUNDLE", rt.GetAppState())
	}

	if err := p2p.ImportInvitationBundle(rt.db, rt.privKey, bundle); err != nil {
		t.Fatalf("ImportInvitationBundle: %v", err)
	}
	if got := rt.GetAppState(); got != "AUTHORIZED" {
		t.Fatalf("GetAppState after import = %q, want AUTHORIZED", got)
	}
}

func TestBusinessP0_ImportBundle_InvalidAdminSignatureRejected(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	libp2pPriv, peerIDStr, mlsPubHex := businessSeedAwaitingBundleUser(t, dbPath)
	bootstrapPeer := testPeerID(t)
	bootstrapAddr := "/ip4/127.0.0.1/tcp/4001/p2p/" + bootstrapPeer
	bundle := businessCreateSignedInvitationBundle(t, "Alice", peerIDStr, mlsPubHex, bootstrapAddr)
	if len(bundle) < 10 {
		t.Fatal("bundle too short")
	}
	bundle[len(bundle)-5] ^= 0xff

	d, err := store.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer d.Close()
	err = p2p.ImportInvitationBundle(d, libp2pPriv, bundle)
	if err == nil {
		t.Fatal("expected invalid bundle error")
	}
	if !strings.Contains(err.Error(), "signature") && !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBusinessP0_ImportBundle_PeerIDMismatchRejected(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	libp2pPriv, _, mlsPubHex := businessSeedAwaitingBundleUser(t, dbPath)
	wrongPeer := testPeerID(t)
	bootstrapPeer := testPeerID(t)
	bootstrapAddr := "/ip4/127.0.0.1/tcp/4001/p2p/" + bootstrapPeer
	bundle := businessCreateSignedInvitationBundle(t, "Alice", wrongPeer, mlsPubHex, bootstrapAddr)

	d, err := store.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer d.Close()

	err = p2p.ImportInvitationBundle(d, libp2pPriv, bundle)
	if err == nil {
		t.Fatal("expected PeerID mismatch error")
	}
	if !strings.Contains(err.Error(), "PeerID") && !strings.Contains(err.Error(), "peer") {
		t.Fatalf("unexpected error: %v", err)
	}
}
