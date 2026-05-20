//go:build business_integration

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/admin"
)

func TestBusiness_RejoinGroupAfterRemoval(t *testing.T) {
	gid := "grp-rejoin-test"

	// 1. Create a shared Admin key
	adminDB, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB admin: %v", err)
	}
	defer adminDB.Close()
	adminPass := "shared-test-pass"
	if _, err := admin.SetupAdminKey(adminDB, adminPass); err != nil {
		t.Fatalf("SetupAdminKey: %v", err)
	}
	adminPriv, err := admin.UnlockAdminKey(adminDB, adminPass)
	if err != nil {
		t.Fatalf("UnlockAdminKey: %v", err)
	}

	seedNode := func(name string, root string) (*Runtime, *businessIntegrationMLSMock) {
		dbPath := businessDBPath(root)
		priv, peerID, mlsPub := businessSeedAwaitingBundleUser(t, dbPath)
		bundleJSON, err := admin.CreateInvitationBundle(adminPriv, name, peerID, mlsPub, "/ip4/127.0.0.1/tcp/4001/p2p/test-bootstrap")
		if err != nil {
			t.Fatalf("CreateInvitationBundle for %s: %v", name, err)
		}
		
		d, err := store.InitDB(dbPath)
		if err != nil {
			t.Fatalf("InitDB for %s: %v", name, err)
		}
		if err := p2p.ImportInvitationBundle(d, priv, bundleJSON); err != nil {
			d.Close()
			t.Fatalf("ImportInvitationBundle for %s: %v", name, err)
		}
		d.Close()

		return businessRuntimeStartMockInWorkDir(t, root)
	}

	aliceRoot := t.TempDir()
	alice, _ := seedNode("Alice", aliceRoot)
	defer businessShutdownRuntimeInWorkDir(t, alice)

	bobRoot := t.TempDir()
	bob, bobMock := seedNode("Bob", bobRoot)
	defer businessShutdownRuntimeInWorkDir(t, bob)

	// Persist KP bundle for Bob so Alice can find it
	businessPersistMockKPBundle(t, bob)

	// Explicitly connect Alice and Bob
	if err := alice.node.Host.Connect(context.Background(), bob.node.Host.Peerstore().PeerInfo(bob.node.Host.ID())); err != nil {
		t.Fatalf("Failed to connect Alice to Bob: %v", err)
	}

	// 1. Alice creates group and invites Bob
	cat := businessEnsureCategory(t, alice, "RejoinTest")
	if err := alice.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatalf("Alice CreateGroupChat: %v", err)
	}
	if err := alice.InvitePeerToGroup(bob.node.Host.ID().String(), gid); err != nil {
		t.Fatalf("Alice InvitePeerToGroup: %v", err)
	}

	// Wait for Bob to auto-join
	businessWaitUntil(t, func() bool {
		active, _ := bob.db.IsGroupActive(gid)
		return active
	}, 10*time.Second, "Bob should auto-join group initially")

	// 2. Mock Bob's removal detection: after Epoch 2 (removal), HasMember returns false
	bobMock.mu.Lock()
	bobMock.hasMemberFn = func(groupState []byte, identity []byte) (bool, error) {
		var state bizMockGroupState
		if err := json.Unmarshal(groupState, &state); err != nil {
			return true, nil // default to true if state unreadable
		}
		if state.Epoch >= 2 { // removal epoch
			return false, nil
		}
		return true, nil
	}
	bobMock.mu.Unlock()

	// Alice removes Bob from the group (advances to Epoch 2)
	if err := alice.RemoveMemberFromGroup(gid, bob.node.Host.ID().String()); err != nil {
		t.Fatalf("Alice RemoveMemberFromGroup: %v", err)
	}

	// Wait for Bob to detect removal
	businessWaitUntil(t, func() bool {
		active, _ := bob.db.IsGroupActive(gid)
		return !active
	}, 10*time.Second, "Bob should detect removal and mark group as left")

	// Verify Bob's DB state: group status='left' and member status='left'
	businessWaitUntil(t, func() bool {
		var gStatus string
		var gLeftAt int64
		err = bob.db.Conn.QueryRow("SELECT lifecycle_status, left_at FROM mls_groups WHERE group_id = ?", gid).Scan(&gStatus, &gLeftAt)
		if err != nil || gStatus != store.GroupLifecycleLeft || gLeftAt == 0 {
			return false
		}
		var mStatus string
		var mLeftAt int64
		err = bob.db.Conn.QueryRow("SELECT status, left_at FROM group_members WHERE group_id = ? AND peer_id = ?", gid, bob.node.Host.ID().String()).Scan(&mStatus, &mLeftAt)
		return err == nil && mStatus == store.GroupMemberStatusLeft
	}, 5*time.Second, "Bob group and member should be marked as left in DB")

	// Reset Bob's mock to return true again for re-join
	bobMock.mu.Lock()
	bobMock.hasMemberFn = nil // reset to default (true)
	bobMock.mu.Unlock()

	// Persist a NEW KP bundle for Bob for the re-invite
	businessPersistMockKPBundle(t, bob)

	// 3. Alice invites Bob BACK (advances to Epoch 3)
	if err := alice.InvitePeerToGroup(bob.node.Host.ID().String(), gid); err != nil {
		t.Fatalf("Alice InvitePeerToGroup (re-invite): %v", err)
	}

	// 4. Bob should auto-join AGAIN
	businessWaitUntil(t, func() bool {
		active, _ := bob.db.IsGroupActive(gid)
		return active
	}, 10*time.Second, "Bob should auto-join group after re-invite")

	// Verify Bob's DB state after re-join
	businessWaitUntil(t, func() bool {
		var gStatus string
		var gLeftAt int64
		err = bob.db.Conn.QueryRow("SELECT lifecycle_status, left_at FROM mls_groups WHERE group_id = ?", gid).Scan(&gStatus, &gLeftAt)
		if err != nil || gStatus != store.GroupLifecycleActive || gLeftAt != 0 {
			return false
		}
		var mStatus string
		var mLeftAt int64
		err = bob.db.Conn.QueryRow("SELECT status, left_at FROM group_members WHERE group_id = ? AND peer_id = ?", gid, bob.node.Host.ID().String()).Scan(&mStatus, &mLeftAt)
		return err == nil && mStatus == store.GroupMemberStatusActive && mLeftAt == 0
	}, 10*time.Second, "Bob group and member should be active/0 after re-join")
}

func businessWaitUntil(t *testing.T, check func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal(msg)
}
