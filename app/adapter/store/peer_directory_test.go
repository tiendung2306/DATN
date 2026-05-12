package store

import "testing"

// TestPeerDirectory_PublicKeyColumnExists is a smoke test for the migration
// performed by ensureColumnExists("peer_directory", "public_key_hex", ...):
// freshly initialized DBs must expose the new column so production callers
// (UpsertPeerProfileWithKey, GetPeerIDByPublicKeyHex) can read/write it
// without a runtime "no such column" error.
func TestPeerDirectory_PublicKeyColumnExists(t *testing.T) {
	d := setupTestDB(t)
	rows, err := d.Conn.Query(`PRAGMA table_info(peer_directory)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt *string
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan column row: %v", err)
		}
		if name == "public_key_hex" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("peer_directory.public_key_hex column missing after InitDB — migration regressed")
	}
}

// TestPeerDirectory_UpsertWithKey_RoundTrip validates the canonical write
// path used by node_status.go: a verified InvitationToken arrives, we stash
// (peer_id, display_name, pubkey_hex). GetPeerIDByPublicKeyHex must then
// resolve the pubkey back to the same peer_id so the joiner's MLS leaf
// enumeration can attribute every leaf to a libp2p identity.
func TestPeerDirectory_UpsertWithKey_RoundTrip(t *testing.T) {
	d := setupTestDB(t)
	const peerID = "peer-alice-1234567890abcdef"
	const name = "Alice"
	const pub = "deadbeefcafebabe1234567890abcdef"

	if err := d.UpsertPeerProfileWithKey(peerID, name, pub); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}

	gotName, err := d.GetPeerDisplayName(peerID)
	if err != nil {
		t.Fatalf("GetPeerDisplayName: %v", err)
	}
	if gotName != name {
		t.Fatalf("display_name=%q, want %q", gotName, name)
	}

	gotPeer, err := d.GetPeerIDByPublicKeyHex(pub)
	if err != nil {
		t.Fatalf("GetPeerIDByPublicKeyHex: %v", err)
	}
	if gotPeer != peerID {
		t.Fatalf("peer_id by pubkey=%q, want %q", gotPeer, peerID)
	}

	// Case-insensitive: hex stored lowercased; uppercase input should match.
	gotPeerUpper, err := d.GetPeerIDByPublicKeyHex("DEADBEEFCAFEBABE1234567890ABCDEF")
	if err != nil {
		t.Fatalf("GetPeerIDByPublicKeyHex(upper): %v", err)
	}
	if gotPeerUpper != peerID {
		t.Fatalf("case-insensitive lookup failed: got %q want %q", gotPeerUpper, peerID)
	}
}

// TestPeerDirectory_UpsertWithKey_PreservesPubkeyOnEmptyUpdate documents
// the partial-update contract used by legacy callers (SavePeerProfile +
// UpsertPeerProfile): passing publicKeyHex="" MUST NOT clobber a previously
// stored pubkey. Otherwise, restarting the app before the AuthProtocol
// handshake completes would wipe directory rows that joinGroupWithWelcome
// depends on.
func TestPeerDirectory_UpsertWithKey_PreservesPubkeyOnEmptyUpdate(t *testing.T) {
	d := setupTestDB(t)
	const peerID = "peer-bob-1234567890abcdef"
	const pub = "abcdef0011223344556677889900aabb"

	if err := d.UpsertPeerProfileWithKey(peerID, "Bob", pub); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey (initial): %v", err)
	}
	// Legacy path: display-name-only upsert with empty pubkey.
	if err := d.UpsertPeerProfileWithKey(peerID, "Bob Updated", ""); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey (legacy): %v", err)
	}

	gotPeer, err := d.GetPeerIDByPublicKeyHex(pub)
	if err != nil {
		t.Fatalf("GetPeerIDByPublicKeyHex after empty update: %v", err)
	}
	if gotPeer != peerID {
		t.Fatalf("pubkey wiped by empty update: got %q want %q", gotPeer, peerID)
	}
	name, _ := d.GetPeerDisplayName(peerID)
	if name != "Bob Updated" {
		t.Fatalf("display_name not refreshed: got %q want %q", name, "Bob Updated")
	}
}

// TestPeerDirectory_GetPeerIDByPublicKeyHex_Miss validates the directory
// miss path: an unknown pubkey must return ("", nil) — never an error and
// never a stale match — so callers can fall through to Phase A heartbeat
// sync gracefully.
func TestPeerDirectory_GetPeerIDByPublicKeyHex_Miss(t *testing.T) {
	d := setupTestDB(t)
	got, err := d.GetPeerIDByPublicKeyHex("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("expected nil error on miss, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty peer_id on miss, got %q", got)
	}
	// Empty input also treated as miss (no SQL hit) — defensive contract
	// for callers that pass straight from hex.EncodeToString output.
	got2, err := d.GetPeerIDByPublicKeyHex("")
	if err != nil {
		t.Fatalf("empty pubkey lookup error: %v", err)
	}
	if got2 != "" {
		t.Fatalf("empty pubkey returned non-empty: %q", got2)
	}
}

// TestPeerDirectory_UpsertWithKey_OverwritePubkey covers the rare but
// supported case where a peer's signing key rotates (Admin re-issues an
// InvitationToken with a fresh pubkey). The new pubkey replaces the old
// one and lookups by the old pubkey become misses.
func TestPeerDirectory_UpsertWithKey_OverwritePubkey(t *testing.T) {
	d := setupTestDB(t)
	const peerID = "peer-carol-1234567890abcdef"
	const oldPub = "0000000000000000000000000000aaaa"
	const newPub = "1111111111111111111111111111bbbb"

	if err := d.UpsertPeerProfileWithKey(peerID, "Carol", oldPub); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertPeerProfileWithKey(peerID, "Carol", newPub); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.GetPeerIDByPublicKeyHex(newPub); got != peerID {
		t.Fatalf("new pubkey lookup: got %q want %q", got, peerID)
	}
	if got, _ := d.GetPeerIDByPublicKeyHex(oldPub); got != "" {
		t.Fatalf("old pubkey must no longer resolve: got %q", got)
	}
}
