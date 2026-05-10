//go:build business_integration

package service

import (
	"context"
	"testing"
)

func TestBusinessP0_Startup_AwaitingBundle_DoesNotRunP2P(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedAwaitingBundleUser(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	if rt.GetAppState() != "AWAITING_BUNDLE" {
		t.Fatalf("app state = %q, want AWAITING_BUNDLE", rt.GetAppState())
	}
	st := rt.GetNodeStatus()
	if st == nil {
		t.Fatal("GetNodeStatus nil")
	}
	if st.IsRunning {
		t.Fatalf("P2P should not start before AUTHORIZED: IsRunning=%v", st.IsRunning)
	}
}

func TestBusinessP0_Startup_Authorized_P2PAttemptOrReady(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedAuthorizedUser(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	if rt.GetAppState() != "AUTHORIZED" {
		t.Fatalf("app state = %q, want AUTHORIZED", rt.GetAppState())
	}
	h := rt.GetRuntimeHealth()
	if h.StartupStage != startupStageReady {
		t.Fatalf("StartupStage = %q, want %q", h.StartupStage, startupStageReady)
	}
}

func TestBusinessP0_Shutdown_Idempotent(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedDBUninitialized(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	rt.Shutdown(context.Background())
	rt.Shutdown(context.Background())
}

func TestBusinessP0_RuntimeHealth_AfterStartupReadyStage(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedDBUninitialized(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)

	h0 := rt.GetRuntimeHealth()
	if h0.StartupStage != startupStageNotStarted {
		t.Fatalf("before Startup StartupStage = %q", h0.StartupStage)
	}

	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	h1 := rt.GetRuntimeHealth()
	if h1.StartupStage != startupStageReady {
		t.Fatalf("after Startup StartupStage = %q, want ready", h1.StartupStage)
	}
}

func TestBusinessP0_GetNodeStatus_HasPeerIDAfterStartup(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	_, peerWant, _ := businessSeedAwaitingBundleUser(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	st := rt.GetNodeStatus()
	if st.PeerID != peerWant {
		t.Fatalf("PeerID = %q, want %q (requires MLS identity for GetOnboardingInfo path)", st.PeerID, peerWant)
	}
}

func TestBusinessP0_GetKnownPeers_NoPanic(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedAwaitingBundleUser(t, dbPath)

	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	defer rt.Shutdown(context.Background())

	_ = rt.GetKnownPeers()
}
