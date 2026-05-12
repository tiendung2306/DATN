//go:build business_integration

// Sprint join-roster-sync — Phase B (MLS leaf enumeration) + Phase A
// (heartbeat-driven roster sync) regression tests. The user-visible bug
// these guard against was: in group "fififa" with 3 members (Tester1
// creator, Admin token holder, Tester2 joiner), Tester2's GetGroupMembers
// returned only [Admin, self] because Tester1 was the inviter on the
// invite-request but NOT the Welcome source (the Token Holder Admin signed
// the Welcome under the new Proposal/Commit-separation flow). Without MLS
// leaf enumeration the joiner has no way of learning about Tester1 until
// Tester1 sends a message.

package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"app/adapter/store"
	"app/coordination"
)

// makeFakeMLSIdentity returns a fresh Ed25519 keypair stand-in for the
// (peer-id-encoded-as-hex, signing-public-key-bytes) tuple stored in
// peer_directory. The "peer id" is intentionally distinct from real
// libp2p IDs because backfillMLSLeafRoster only persists rows when
// isValidPeerID(peerID) is true — we therefore mint a real libp2p peer
// id below via testPeerID where needed.
func makeFakeMLSPubkey(t *testing.T) []byte {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return pub
}

// TestBusinessP1_JoinRosterSync_MLSLeafEnumerationPopulatesAllMembers is the
// direct regression for the fififa bug. It exercises backfillMLSLeafRoster
// end-to-end: given a joined group with mock MLS state + peer_directory
// rows for Tester1/Admin, GetGroupMembers MUST return all three peers
// without any of them having sent a message.
func TestBusinessP1_JoinRosterSync_MLSLeafEnumerationPopulatesAllMembers(t *testing.T) {
	rt, mock := businessRuntimeAuthorizedWithMockMLS(t)
	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()
	if database == nil {
		t.Fatal("nil db")
	}

	selfInfo, err := rt.GetOnboardingInfo()
	if err != nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	selfID, _ := database.GetMLSIdentity()
	if selfID == nil || len(selfID.PublicKey) == 0 {
		t.Fatal("self MLS identity missing public key")
	}

	// Bootstrap two other peers in the directory: Tester1 (creator) and
	// Admin (Token Holder). Use real libp2p peer IDs so isValidPeerID
	// accepts the upserted group_members rows.
	tester1PeerID := testPeerID(t)
	tester1Pub := makeFakeMLSPubkey(t)
	if err := database.UpsertPeerProfileWithKey(tester1PeerID, "Tester1", hex.EncodeToString(tester1Pub)); err != nil {
		t.Fatalf("seed Tester1: %v", err)
	}
	adminPeerID := testPeerID(t)
	adminPub := makeFakeMLSPubkey(t)
	if err := database.UpsertPeerProfileWithKey(adminPeerID, "Admin", hex.EncodeToString(adminPub)); err != nil {
		t.Fatalf("seed Admin: %v", err)
	}

	// Synthesise a "fififa" group state on Tester2's side. The mock engine
	// does not enforce content beyond JSON-shape, so we persist a minimal
	// bizMockGroupState and tell the mock ListMembersFn to return the
	// three identities a real OpenMLS group would yield.
	gid := "grp-fififa"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    2,
		TreeHash: hex.EncodeToString(bizMockTreeHash(2)),
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	coordStorage := rt.coordStorage
	rt.mu.RUnlock()
	if coordStorage == nil {
		t.Fatal("coord storage unavailable")
	}
	if err := coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    gid,
		GroupState: stateBytes,
		Epoch:      state.Epoch,
		TreeHash:   bizMockTreeHash(state.Epoch),
		MyRole:     coordination.RoleMember,
		GroupType:  "channel",
	}); err != nil {
		t.Fatalf("SaveGroupRecord: %v", err)
	}
	// Persist a placeholder self row so HasGroup(gid) returns true (the
	// runtime treats mls_groups as the join-state oracle).
	if err := database.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     gid,
		PeerID:      selfInfo.PeerID,
		DisplayName: selfID.DisplayName,
		Role:        "member",
		Status:      store.GroupMemberStatusActive,
		Source:      "welcome",
	}); err != nil {
		t.Fatalf("seed self group_member: %v", err)
	}

	selfPub := append([]byte(nil), selfID.PublicKey...)
	mock.SetListMembersFunc(func(_ []byte) ([][]byte, error) {
		return [][]byte{selfPub, tester1Pub, adminPub}, nil
	})

	members, err := rt.GetGroupMembers(gid)
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}

	got := map[string]bool{}
	for _, m := range members {
		got[m.PeerID] = true
	}
	if !got[selfInfo.PeerID] {
		t.Fatalf("self peer %q missing from roster: %+v", selfInfo.PeerID, members)
	}
	if !got[tester1PeerID] {
		t.Fatalf("Tester1 peer %q missing from roster (regression: MLS leaf enumeration not applied)", tester1PeerID)
	}
	if !got[adminPeerID] {
		t.Fatalf("Admin peer %q missing from roster (regression: MLS leaf enumeration not applied)", adminPeerID)
	}
	if len(members) != 3 {
		t.Fatalf("expected exactly 3 members in roster, got %d: %+v", len(members), members)
	}

	// Subsequent GetGroupMembers calls remain stable (idempotent backfill)
	// and the canonical row source for known peers transitions to "mls_leaf"
	// so it does not get overwritten with a weaker history/heartbeat label.
	members2, err := rt.GetGroupMembers(gid)
	if err != nil {
		t.Fatalf("GetGroupMembers (second call): %v", err)
	}
	if len(members2) != 3 {
		t.Fatalf("second call roster size=%d, want 3", len(members2))
	}

	rows, err := database.ListGroupMembers(gid, store.GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	for _, r := range rows {
		if r.PeerID == selfInfo.PeerID {
			continue
		}
		// Either the original mls_leaf upsert or a later profile-refresh
		// is acceptable; what matters is the row exists and is active.
		if r.Status != store.GroupMemberStatusActive {
			t.Fatalf("peer %q status=%q want active", r.PeerID, r.Status)
		}
	}
}

// TestBusinessP1_JoinRosterSync_MissingDirectoryEntryFallsThroughToPhaseA
// documents the contract between Phase B and Phase A. When MLS enumeration
// returns identities the peer_directory has NOT seen yet (e.g. the peer
// has not completed AuthProtocol handshake), backfillMLSLeafRoster MUST
// silently skip them — never upsert a fabricated row — so the operator
// is not misled into thinking the roster is complete. The complementary
// Phase A path (Coordinator.OnPeerObserved on first heartbeat) will catch
// the peer the moment it comes online.
func TestBusinessP1_JoinRosterSync_MissingDirectoryEntryFallsThroughToPhaseA(t *testing.T) {
	rt, mock := businessRuntimeAuthorizedWithMockMLS(t)
	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()

	selfInfo, _ := rt.GetOnboardingInfo()
	selfID, _ := database.GetMLSIdentity()
	selfPub := append([]byte(nil), selfID.PublicKey...)

	gid := "grp-fififa-no-directory"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	stateBytes, _ := json.Marshal(state)
	rt.mu.RLock()
	coordStorage := rt.coordStorage
	rt.mu.RUnlock()
	if err := coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    gid,
		GroupState: stateBytes,
		Epoch:      state.Epoch,
		TreeHash:   bizMockTreeHash(state.Epoch),
		MyRole:     coordination.RoleMember,
		GroupType:  "channel",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     gid,
		PeerID:      selfInfo.PeerID,
		DisplayName: selfID.DisplayName,
		Role:        "member",
		Status:      store.GroupMemberStatusActive,
		Source:      "welcome",
	}); err != nil {
		t.Fatal(err)
	}

	// Return a leaf whose pubkey is unknown to peer_directory.
	unknownPub := makeFakeMLSPubkey(t)
	mock.SetListMembersFunc(func(_ []byte) ([][]byte, error) {
		return [][]byte{selfPub, unknownPub}, nil
	})

	members, err := rt.GetGroupMembers(gid)
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("only self should be in roster (unknown pubkey must NOT fabricate a row): got %d members: %+v", len(members), members)
	}
	if members[0].PeerID != selfInfo.PeerID {
		t.Fatalf("expected self, got %q", members[0].PeerID)
	}
	// Sanity: the unknown pubkey lookup is indeed a miss.
	got, _ := database.GetPeerIDByPublicKeyHex(strings.ToLower(hex.EncodeToString(unknownPub)))
	if got != "" {
		t.Fatalf("directory unexpectedly resolved unknown pubkey: %q", got)
	}
}
