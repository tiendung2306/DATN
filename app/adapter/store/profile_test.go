package store

import (
	"database/sql"
	"testing"
)

func TestUpsertAvatarBlob_DedupeSingleRow(t *testing.T) {
	d := setupTestDB(t)
	data := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	hash := AvatarContentHash(data)
	if err := d.UpsertAvatarBlob(hash, "image/png", data); err != nil {
		t.Fatalf("UpsertAvatarBlob first: %v", err)
	}
	if err := d.UpsertAvatarBlob(hash, "image/png", data); err != nil {
		t.Fatalf("UpsertAvatarBlob second: %v", err)
	}
	var n int
	if err := d.Conn.QueryRow(`SELECT COUNT(*) FROM avatar_blobs WHERE hash = ?`, hash).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("avatar_blobs rows=%d, want 1 (dedupe by hash)", n)
	}
}

func TestUpsertAvatarBlob_ExceedsMaxSize(t *testing.T) {
	d := setupTestDB(t)
	data := make([]byte, MaxAvatarBytes+1)
	hash := AvatarContentHash(data)
	if err := d.UpsertAvatarBlob(hash, "image/png", data); err == nil {
		t.Fatal("expected error for oversized blob")
	}
}

func TestMergePeerDirectoryProfile_StaleRevision(t *testing.T) {
	d := setupTestDB(t)
	const peer = "peer-merge-stale"
	pub := "aabb0011223344556677889900ccdd"
	if err := d.UpsertPeerProfileWithKey(peer, "Bob", pub); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	sig := make([]byte, 64)
	if err := d.MergePeerDirectoryProfile(peer, 1, "", "", "", "", 0, sig, nil); err != nil {
		t.Fatalf("MergePeerDirectoryProfile rev 1: %v", err)
	}
	if err := d.MergePeerDirectoryProfile(peer, 1, "x@y.z", "", "", "", 0, sig, nil); err != nil {
		t.Fatalf("idempotent same revision: %v", err)
	}
	row, _ := d.GetPeerDirectoryProfile(peer)
	if row.ProfileRevision != 1 {
		t.Fatalf("revision want 1 got %d", row.ProfileRevision)
	}
	if err := d.MergePeerDirectoryProfile(peer, 0, "", "", "", "", 0, sig, nil); err == nil {
		t.Fatal("expected error for revision 0")
	}
	if err := d.MergePeerDirectoryProfile(peer, 2, "ok@x.y", "", "", "", 0, sig, nil); err != nil {
		t.Fatalf("rev 2: %v", err)
	}
	if err := d.MergePeerDirectoryProfile(peer, 1, "stale@x.y", "", "", "", 0, sig, nil); err == nil {
		t.Fatal("expected error when newRevision < stored revision")
	}
}

func TestMergePeerDirectoryProfile_EmptyIncomingPreservesEmail(t *testing.T) {
	d := setupTestDB(t)
	const peer = "peer-merge-preserve"
	pub := "deadbeef00112233445566778899aa"
	if err := d.UpsertPeerProfileWithKey(peer, "Carol", pub); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	if err := d.UpsertPeerDirectorySigned(peer, "Carol", pub,
		sql.NullString{String: "keep@example.com", Valid: true},
		sql.NullString{},
		sql.NullString{}, sql.NullString{},
		0, 1, "00",
	); err != nil {
		t.Fatalf("UpsertPeerDirectorySigned: %v", err)
	}
	sig := make([]byte, 64)
	if err := d.MergePeerDirectoryProfile(peer, 2, "", "", "", "", 0, sig, nil); err != nil {
		t.Fatalf("MergePeerDirectoryProfile: %v", err)
	}
	row, err := d.GetPeerDirectoryProfile(peer)
	if err != nil {
		t.Fatalf("GetPeerDirectoryProfile: %v", err)
	}
	if !row.Email.Valid || row.Email.String != "keep@example.com" {
		t.Fatalf("email=%+v, want keep@example.com preserved", row.Email)
	}
	if row.ProfileRevision != 2 {
		t.Fatalf("profile_revision=%d, want 2", row.ProfileRevision)
	}
}

func TestMergePeerDirectoryProfile_UpdatesEmailWhenNonEmpty(t *testing.T) {
	d := setupTestDB(t)
	const peer = "peer-merge-update"
	pub := "cafebabe00112233445566778899aa"
	if err := d.UpsertPeerProfileWithKey(peer, "Dan", pub); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	if err := d.UpsertPeerDirectorySigned(peer, "Dan", pub,
		sql.NullString{String: "old@example.com", Valid: true},
		sql.NullString{},
		sql.NullString{}, sql.NullString{},
		0, 1, "00",
	); err != nil {
		t.Fatalf("UpsertPeerDirectorySigned: %v", err)
	}
	sig := make([]byte, 64)
	if err := d.MergePeerDirectoryProfile(peer, 2, "new@example.com", "", "", "", 0, sig, nil); err != nil {
		t.Fatalf("MergePeerDirectoryProfile: %v", err)
	}
	row, err := d.GetPeerDirectoryProfile(peer)
	if err != nil {
		t.Fatalf("GetPeerDirectoryProfile: %v", err)
	}
	if !row.Email.Valid || row.Email.String != "new@example.com" {
		t.Fatalf("email=%+v, want new@example.com", row.Email)
	}
}

func TestMergePeerDirectoryProfile_ClearedFieldsClearsEmail(t *testing.T) {
	d := setupTestDB(t)
	const peer = "peer-merge-clear-email"
	pub := "babe0000112233445566778899aa"
	if err := d.UpsertPeerProfileWithKey(peer, "Eve", pub); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	if err := d.UpsertPeerDirectorySigned(peer, "Eve", pub,
		sql.NullString{String: "gone@example.com", Valid: true},
		sql.NullString{},
		sql.NullString{}, sql.NullString{},
		0, 1, "00",
	); err != nil {
		t.Fatalf("UpsertPeerDirectorySigned: %v", err)
	}
	sig := make([]byte, 64)
	if err := d.MergePeerDirectoryProfile(peer, 2, "", "", "", "", 0, sig, []string{"email"}); err != nil {
		t.Fatalf("MergePeerDirectoryProfile: %v", err)
	}
	row, err := d.GetPeerDirectoryProfile(peer)
	if err != nil {
		t.Fatalf("GetPeerDirectoryProfile: %v", err)
	}
	if row.Email.Valid {
		t.Fatalf("email should be cleared, got %+v", row.Email)
	}
}
