package store

import (
	"database/sql"
	"errors"
	"testing"
)

func TestTryMergeReplicatedRecord_InsertAndUpgrade(t *testing.T) {
	d := setupTestDB(t)
	const ns = NamespaceUserProfileV1
	const key = "peer-repl-1"
	sig := make([]byte, 64)
	body1 := `{"v":1,"peer_id":"` + key + `","profile_revision":1}`
	h1 := ReplicatedBodyHash(body1)
	if err := d.TryMergeReplicatedRecord(ns, key, key, 1, 1, body1, h1, sig, "aa", 0, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	row, err := d.GetReplicatedRecord(ns, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Revision != 1 || row.BodyJSON != body1 {
		t.Fatalf("unexpected row %+v", row)
	}
	body2 := `{"v":1,"peer_id":"` + key + `","profile_revision":2}`
	h2 := ReplicatedBodyHash(body2)
	if err := d.TryMergeReplicatedRecord(ns, key, key, 2, 1, body2, h2, sig, "aa", 0, nil); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	row2, err := d.GetReplicatedRecord(ns, key)
	if err != nil {
		t.Fatalf("get2: %v", err)
	}
	if row2.Revision != 2 || row2.BodyJSON != body2 {
		t.Fatalf("want rev 2, got %+v", row2)
	}
}

func TestTryMergeReplicatedRecord_StaleAndIdempotent(t *testing.T) {
	d := setupTestDB(t)
	const ns = NamespaceUserProfileV1
	const key = "peer-repl-2"
	sig := make([]byte, 64)
	body := `{"v":1,"peer_id":"` + key + `","profile_revision":5}`
	h := ReplicatedBodyHash(body)
	if err := d.TryMergeReplicatedRecord(ns, key, key, 5, 1, body, h, sig, "bb", 0, nil); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := d.TryMergeReplicatedRecord(ns, key, key, 4, 1, body, h, sig, "bb", 0, nil); err == nil {
		t.Fatal("expected stale revision error")
	} else if !errors.Is(err, ErrReplicatedStaleRevision) {
		t.Fatalf("want ErrReplicatedStaleRevision, got %v", err)
	}
	if err := d.TryMergeReplicatedRecord(ns, key, key, 5, 1, body, h, sig, "bb", 0, nil); err != nil {
		t.Fatalf("idempotent same rev+hash: %v", err)
	}
	bodyOther := `{"v":1,"peer_id":"` + key + `","profile_revision":5,"x":1}`
	hOther := ReplicatedBodyHash(bodyOther)
	if err := d.TryMergeReplicatedRecord(ns, key, key, 5, 1, bodyOther, hOther, sig, "bb", 0, nil); err == nil {
		t.Fatal("expected conflict on same revision different hash")
	} else if !errors.Is(err, ErrReplicatedConflict) {
		t.Fatalf("want ErrReplicatedConflict, got %v", err)
	}
}

func TestReplicatedBodyHashMismatch(t *testing.T) {
	d := setupTestDB(t)
	const ns = NamespaceUserProfileV1
	const key = "peer-repl-3"
	sig := make([]byte, 64)
	body := `{"v":1}`
	if err := d.TryMergeReplicatedRecord(ns, key, key, 1, 1, body, "deadbeef", sig, "cc", 0, nil); err == nil {
		t.Fatal("expected hash mismatch")
	}
}

func TestReplicatedRecordBlobRefsReplaceOnMerge(t *testing.T) {
	d := setupTestDB(t)
	const ns = NamespaceUserProfileV1
	const key = "peer-repl-refs"
	sig := make([]byte, 64)
	body1 := `{"v":1,"avatar_hash":"aaa"}`
	if err := d.TryMergeReplicatedRecord(
		ns, key, key, 1, 1, body1, ReplicatedBodyHash(body1), sig, "aa", 0,
		[]ReplicatedBlobRef{{Hash: "aaa", Required: true}},
	); err != nil {
		t.Fatalf("merge rev 1: %v", err)
	}
	refs, err := d.GetReplicatedRecordBlobRefs(ns, key)
	if err != nil {
		t.Fatalf("refs rev 1: %v", err)
	}
	if len(refs) != 1 || refs[0].Hash != "aaa" || !refs[0].Required {
		t.Fatalf("unexpected refs rev1: %+v", refs)
	}
	body2 := `{"v":1}`
	if err := d.TryMergeReplicatedRecord(
		ns, key, key, 2, 1, body2, ReplicatedBodyHash(body2), sig, "aa", 0, nil,
	); err != nil {
		t.Fatalf("merge rev 2: %v", err)
	}
	refs, err = d.GetReplicatedRecordBlobRefs(ns, key)
	if err != nil {
		t.Fatalf("refs rev 2: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("refs should be cleared on replacement, got %+v", refs)
	}
}

func TestListKnownProfilePeerIDsExcludesLocal(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertPeerProfileWithKey("local-peer", "Local", ""); err != nil {
		t.Fatalf("local: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey("remote-a", "A", ""); err != nil {
		t.Fatalf("remote-a: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey("remote-b", "B", ""); err != nil {
		t.Fatalf("remote-b: %v", err)
	}
	got, err := d.ListKnownProfilePeerIDs("local-peer", 10)
	if err != nil {
		t.Fatalf("ListKnownProfilePeerIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d got=%v", len(got), got)
	}
	for _, id := range got {
		if id == "local-peer" {
			t.Fatalf("local peer should be excluded: %v", got)
		}
	}
}

func TestGCUnreferencedAvatarBlobsPreservesReferencedRows(t *testing.T) {
	d := setupTestDB(t)
	old := int64(1)
	if _, err := d.Conn.Exec(
		`INSERT INTO avatar_blobs (hash, mime, size, bytes, created_at, last_used_at)
		 VALUES ('keep', 'image/png', 1, x'01', ?, ?),
		        ('drop', 'image/png', 1, x'02', ?, ?)`,
		old, old, old, old,
	); err != nil {
		t.Fatalf("insert avatar blobs: %v", err)
	}
	if err := d.UpsertPeerProfileWithKey("peer-with-avatar", "Peer", ""); err != nil {
		t.Fatalf("UpsertPeerProfileWithKey: %v", err)
	}
	if err := d.UpsertPeerDirectorySigned("peer-with-avatar", "Peer", "",
		sql.NullString{}, sql.NullString{},
		sql.NullString{String: "keep", Valid: true}, sql.NullString{String: "image/png", Valid: true},
		1, 1, "00",
	); err != nil {
		t.Fatalf("UpsertPeerDirectorySigned: %v", err)
	}
	deleted, err := d.GCUnreferencedAvatarBlobs(2, 10)
	if err != nil {
		t.Fatalf("GCUnreferencedAvatarBlobs: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d want 1", deleted)
	}
	var count int
	if err := d.Conn.QueryRow(`SELECT COUNT(*) FROM avatar_blobs WHERE hash = 'keep'`).Scan(&count); err != nil {
		t.Fatalf("count keep: %v", err)
	}
	if count != 1 {
		t.Fatalf("referenced avatar blob should be preserved")
	}
	if err := d.Conn.QueryRow(`SELECT COUNT(*) FROM avatar_blobs WHERE hash = 'drop'`).Scan(&count); err != nil {
		t.Fatalf("count drop: %v", err)
	}
	if count != 0 {
		t.Fatalf("unreferenced avatar blob should be deleted")
	}
}
