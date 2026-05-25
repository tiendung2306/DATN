//go:build business_integration

package service

import "testing"

func TestBusinessP1_E2E_RealSidecar_StartDirectMessage_WithStoredKP(t *testing.T) {
	aliceRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, aliceRoot)
	alice := businessRuntimeStartRealInWorkDir(t, aliceRoot)
	t.Cleanup(func() { businessShutdownRuntimeInWorkDir(t, alice) })

	bobRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, bobRoot)
	bob := businessRuntimeStartRealInWorkDir(t, bobRoot)
	t.Cleanup(func() { businessShutdownRuntimeInWorkDir(t, bob) })

	bobInfo, err := bob.GetOnboardingInfo()
	if err != nil {
		t.Fatalf("bob GetOnboardingInfo: %v", err)
	}
	bobKP, err := bob.GenerateKeyPackage()
	if err != nil {
		t.Fatalf("bob GenerateKeyPackage: %v", err)
	}

	// Isolate the DM create path from live P2P discovery so the test exercises
	// the same stored-KP invite flow used by StartDirectMessage after cache hits.
	businessSeedStoredKeyPackageForPeer(t, alice, bobInfo.PeerID, bobKP.PublicHex)

	groupID, err := alice.StartDirectMessage(bobInfo.PeerID)
	if err != nil {
		t.Fatalf("alice StartDirectMessage: %v", err)
	}
	if groupID == "" {
		t.Fatal("empty DM group id")
	}

	alice.mu.RLock()
	database := alice.db
	storage := alice.coordStorage
	alice.mu.RUnlock()
	if database == nil || storage == nil {
		t.Fatal("alice runtime not initialized")
	}

	rec, err := storage.GetGroupRecord(groupID)
	if err != nil {
		t.Fatalf("GetGroupRecord: %v", err)
	}
	if rec.GroupType != "dm" {
		t.Fatalf("group_type=%q want dm", rec.GroupType)
	}
	if rec.Epoch != 1 {
		t.Fatalf("epoch=%d want 1 after DM AddMembers commit", rec.Epoch)
	}

	pending, err := database.GetPendingWelcomesFor(bobInfo.PeerID)
	if err != nil {
		t.Fatalf("GetPendingWelcomesFor: %v", err)
	}
	found := false
	for _, row := range pending {
		if row.GroupID == groupID && len(row.WelcomeBytes) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pending welcome for DM %q and peer %q", groupID, bobInfo.PeerID)
	}
}
