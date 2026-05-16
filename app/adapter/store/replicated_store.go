package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// NamespaceUserProfileV1 is the replicated namespace for MLS-signed user profiles.
const NamespaceUserProfileV1 = "user.profile.v1"

// NamespaceGroupAvatarV1 is the replicated namespace for MLS-signed group chat avatars.
const NamespaceGroupAvatarV1 = "group.avatar.v1"

var (
	// ErrReplicatedStaleRevision means the incoming record is older than what we store.
	ErrReplicatedStaleRevision = errors.New("replicated: stale revision")
	// ErrReplicatedConflict means same revision but different payload than stored.
	ErrReplicatedConflict = errors.New("replicated: same revision conflicting payload")
)

// ReplicatedRecordRow is a row in replicated_records.
type ReplicatedRecordRow struct {
	Namespace           string
	RecordKey           string
	OwnerPeerID         string
	Revision            int64
	SchemaVersion       int
	BodyJSON            string
	BodyHash            string
	Signature           []byte
	SigningPublicKeyHex string
	DeletedAt           int64
	UpdatedAt           int64
}

// ReplicatedBlobRef links one record to a content-addressed blob hash.
type ReplicatedBlobRef struct {
	Hash     string
	Required bool
}

// ReplicatedBodyHash returns lowercase hex SHA-256 of the UTF-8 body JSON bytes.
func ReplicatedBodyHash(bodyJSON string) string {
	sum := sha256.Sum256([]byte(bodyJSON))
	return hex.EncodeToString(sum[:])
}

// GetReplicatedRecord returns the latest stored row for (namespace, record_key).
func (d *Database) GetReplicatedRecord(namespace, recordKey string) (*ReplicatedRecordRow, error) {
	namespace = strings.TrimSpace(namespace)
	recordKey = strings.TrimSpace(recordKey)
	if namespace == "" || recordKey == "" {
		return nil, fmt.Errorf("replicated: namespace and record_key required")
	}
	var r ReplicatedRecordRow
	err := d.Conn.QueryRow(
		`SELECT namespace, record_key, owner_peer_id, revision, schema_version,
		        body_json, body_hash, signature, signing_public_key_hex, deleted_at,
		        COALESCE(updated_at,0)
		 FROM replicated_records
		 WHERE namespace = ? AND record_key = ?`,
		namespace, recordKey,
	).Scan(
		&r.Namespace, &r.RecordKey, &r.OwnerPeerID, &r.Revision, &r.SchemaVersion,
		&r.BodyJSON, &r.BodyHash, &r.Signature, &r.SigningPublicKeyHex, &r.DeletedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// TryMergeReplicatedRecord applies revision monotonicity + idempotent replay.
// body_hash must match ReplicatedBodyHash(bodyJSON).
func (d *Database) TryMergeReplicatedRecord(
	namespace, recordKey, ownerPeerID string,
	revision int64,
	schemaVersion int,
	bodyJSON string,
	bodyHash string,
	signature []byte,
	signingPublicKeyHex string,
	deletedAt int64,
	blobRefs []ReplicatedBlobRef,
) error {
	namespace = strings.TrimSpace(namespace)
	recordKey = strings.TrimSpace(recordKey)
	ownerPeerID = strings.TrimSpace(ownerPeerID)
	if namespace == "" || recordKey == "" || ownerPeerID == "" {
		return fmt.Errorf("replicated: namespace, record_key, owner_peer_id required")
	}
	if revision <= 0 {
		return fmt.Errorf("replicated: revision must be > 0")
	}
	wantHash := strings.ToLower(strings.TrimSpace(bodyHash))
	if wantHash == "" {
		return fmt.Errorf("replicated: body_hash required")
	}
	if ReplicatedBodyHash(bodyJSON) != wantHash {
		return fmt.Errorf("replicated: body_hash mismatch")
	}
	pubHex := strings.ToLower(strings.TrimSpace(signingPublicKeyHex))
	refs := normalizeReplicatedBlobRefs(blobRefs)

	var curRev int64
	var curHash string
	err := d.Conn.QueryRow(
		`SELECT revision, body_hash FROM replicated_records WHERE namespace = ? AND record_key = ?`,
		namespace, recordKey,
	).Scan(&curRev, &curHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = d.Conn.Exec(
			`INSERT INTO replicated_records (
				namespace, record_key, owner_peer_id, revision, schema_version,
				body_json, body_hash, signature, signing_public_key_hex, deleted_at, updated_at
			) VALUES (?,?,?,?,?,?,?,?,?,?, strftime('%s','now'))`,
			namespace, recordKey, ownerPeerID, revision, schemaVersion,
			bodyJSON, wantHash, signature, pubHex, deletedAt,
		)
		if err != nil {
			return fmt.Errorf("replicated insert: %w", err)
		}
		if err := d.replaceReplicatedRecordBlobRefs(namespace, recordKey, refs); err != nil {
			return err
		}
		return nil
	case err != nil:
		return err
	default:
		if curRev > revision {
			return fmt.Errorf("%w: have %d got %d", ErrReplicatedStaleRevision, curRev, revision)
		}
		if curRev == revision {
			if strings.EqualFold(strings.TrimSpace(curHash), wantHash) {
				if err := d.replaceReplicatedRecordBlobRefs(namespace, recordKey, refs); err != nil {
					return err
				}
				return nil
			}
			return fmt.Errorf("%w", ErrReplicatedConflict)
		}
		_, err = d.Conn.Exec(
			`UPDATE replicated_records SET
				owner_peer_id = ?,
				revision = ?,
				schema_version = ?,
				body_json = ?,
				body_hash = ?,
				signature = ?,
				signing_public_key_hex = ?,
				deleted_at = ?,
				updated_at = strftime('%s','now')
			 WHERE namespace = ? AND record_key = ?`,
			ownerPeerID, revision, schemaVersion,
			bodyJSON, wantHash, signature, pubHex, deletedAt,
			namespace, recordKey,
		)
		if err != nil {
			return fmt.Errorf("replicated update: %w", err)
		}
		if err := d.replaceReplicatedRecordBlobRefs(namespace, recordKey, refs); err != nil {
			return err
		}
		return nil
	}
}

// PutReplicatedRecordForce replaces the row for (namespace, record_key) (local authoritative write).
func (d *Database) PutReplicatedRecordForce(
	namespace, recordKey, ownerPeerID string,
	revision int64,
	schemaVersion int,
	bodyJSON string,
	bodyHash string,
	signature []byte,
	signingPublicKeyHex string,
	deletedAt int64,
	blobRefs []ReplicatedBlobRef,
) error {
	namespace = strings.TrimSpace(namespace)
	recordKey = strings.TrimSpace(recordKey)
	ownerPeerID = strings.TrimSpace(ownerPeerID)
	if namespace == "" || recordKey == "" || ownerPeerID == "" {
		return fmt.Errorf("replicated: namespace, record_key, owner_peer_id required")
	}
	if revision <= 0 {
		return fmt.Errorf("replicated: revision must be > 0")
	}
	wantHash := strings.ToLower(strings.TrimSpace(bodyHash))
	if wantHash == "" || ReplicatedBodyHash(bodyJSON) != wantHash {
		return fmt.Errorf("replicated: body_hash mismatch")
	}
	_, err := d.Conn.Exec(
		`INSERT INTO replicated_records (
			namespace, record_key, owner_peer_id, revision, schema_version,
			body_json, body_hash, signature, signing_public_key_hex, deleted_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?, strftime('%s','now'))
		ON CONFLICT(namespace, record_key) DO UPDATE SET
			owner_peer_id = excluded.owner_peer_id,
			revision = excluded.revision,
			schema_version = excluded.schema_version,
			body_json = excluded.body_json,
			body_hash = excluded.body_hash,
			signature = excluded.signature,
			signing_public_key_hex = excluded.signing_public_key_hex,
			deleted_at = excluded.deleted_at,
			updated_at = strftime('%s','now')`,
		namespace, recordKey, ownerPeerID,
		revision, schemaVersion, bodyJSON, wantHash, signature,
		strings.ToLower(strings.TrimSpace(signingPublicKeyHex)), deletedAt,
	)
	if err != nil {
		return fmt.Errorf("PutReplicatedRecordForce: %w", err)
	}
	if err := d.replaceReplicatedRecordBlobRefs(namespace, recordKey, normalizeReplicatedBlobRefs(blobRefs)); err != nil {
		return err
	}
	return nil
}

func normalizeReplicatedBlobRefs(in []ReplicatedBlobRef) []ReplicatedBlobRef {
	seen := make(map[string]bool, len(in))
	out := make([]ReplicatedBlobRef, 0, len(in))
	for _, ref := range in {
		h := strings.TrimSpace(strings.ToLower(ref.Hash))
		if h == "" {
			continue
		}
		required, ok := seen[h]
		if ok {
			seen[h] = required || ref.Required
			continue
		}
		seen[h] = ref.Required
	}
	for h, required := range seen {
		out = append(out, ReplicatedBlobRef{Hash: h, Required: required})
	}
	return out
}

func (d *Database) replaceReplicatedRecordBlobRefs(namespace, recordKey string, refs []ReplicatedBlobRef) error {
	tx, err := d.Conn.Begin()
	if err != nil {
		return fmt.Errorf("replicated blob refs begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM replicated_record_blobs WHERE namespace = ? AND record_key = ?`,
		namespace, recordKey,
	); err != nil {
		return fmt.Errorf("replicated blob refs delete: %w", err)
	}
	for _, ref := range refs {
		required := 0
		if ref.Required {
			required = 1
		}
		if _, err := tx.Exec(
			`INSERT INTO replicated_record_blobs (namespace, record_key, blob_hash, required)
			 VALUES (?, ?, ?, ?)`,
			namespace, recordKey, ref.Hash, required,
		); err != nil {
			return fmt.Errorf("replicated blob refs insert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replicated blob refs commit: %w", err)
	}
	return nil
}

// GetReplicatedRecordBlobRefs returns blob refs attached to a replicated record.
func (d *Database) GetReplicatedRecordBlobRefs(namespace, recordKey string) ([]ReplicatedBlobRef, error) {
	rows, err := d.Conn.Query(
		`SELECT blob_hash, required FROM replicated_record_blobs
		 WHERE namespace = ? AND record_key = ?
		 ORDER BY blob_hash ASC`,
		strings.TrimSpace(namespace), strings.TrimSpace(recordKey),
	)
	if err != nil {
		return nil, fmt.Errorf("GetReplicatedRecordBlobRefs: %w", err)
	}
	defer rows.Close()
	var out []ReplicatedBlobRef
	for rows.Next() {
		var ref ReplicatedBlobRef
		var required int
		if err := rows.Scan(&ref.Hash, &required); err != nil {
			return nil, err
		}
		ref.Required = required != 0
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListKnownProfilePeerIDs returns peers known through peer_directory.
func (d *Database) ListKnownProfilePeerIDs(excludePeerID string, limit int) ([]string, error) {
	excludePeerID = strings.TrimSpace(excludePeerID)
	if limit <= 0 {
		limit = 256
	}
	rows, err := d.Conn.Query(
		`SELECT peer_id FROM peer_directory
		 WHERE trim(peer_id) <> '' AND peer_id <> ?
		 ORDER BY updated_at DESC, peer_id ASC
		 LIMIT ?`,
		excludePeerID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ListKnownProfilePeerIDs: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetReplicatedPullCursor returns last_revision seen for a remote peer + key (0 if none).
func (d *Database) GetReplicatedPullCursor(remotePeerID, namespace, recordKey string) (int64, error) {
	var rev int64
	err := d.Conn.QueryRow(
		`SELECT last_revision FROM replicated_pull_state
		 WHERE remote_peer_id = ? AND namespace = ? AND record_key = ?`,
		strings.TrimSpace(remotePeerID), strings.TrimSpace(namespace), strings.TrimSpace(recordKey),
	).Scan(&rev)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return rev, nil
}

// UpsertReplicatedPullCursor bumps the cursor to at least lastRevision.
func (d *Database) UpsertReplicatedPullCursor(remotePeerID, namespace, recordKey string, lastRevision int64) error {
	_, err := d.Conn.Exec(
		`INSERT INTO replicated_pull_state (remote_peer_id, namespace, record_key, last_revision, updated_at)
		 VALUES (?,?,?,?, strftime('%s','now'))
		 ON CONFLICT(remote_peer_id, namespace, record_key) DO UPDATE SET
			last_revision = MAX(replicated_pull_state.last_revision, excluded.last_revision),
			updated_at = strftime('%s','now')`,
		strings.TrimSpace(remotePeerID), strings.TrimSpace(namespace), strings.TrimSpace(recordKey), lastRevision,
	)
	if err != nil {
		return fmt.Errorf("UpsertReplicatedPullCursor: %w", err)
	}
	return nil
}

// UpsertReplicatedBlob stores bytes keyed by content hash (same size cap as avatars).
func (d *Database) UpsertReplicatedBlob(hash, mime string, data []byte) error {
	hash = strings.TrimSpace(strings.ToLower(hash))
	mime = strings.TrimSpace(mime)
	if hash == "" || mime == "" || len(data) == 0 {
		return fmt.Errorf("replicated blob: hash, mime and bytes are required")
	}
	if len(data) > MaxAvatarBytes {
		return fmt.Errorf("replicated blob: size %d exceeds max %d", len(data), MaxAvatarBytes)
	}
	now := time.Now().Unix()
	if _, err := d.Conn.Exec(
		`INSERT OR IGNORE INTO replicated_blobs (hash, mime, size, bytes, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		hash, mime, len(data), data, now, now,
	); err != nil {
		return fmt.Errorf("UpsertReplicatedBlob insert: %w", err)
	}
	if _, err := d.Conn.Exec(
		`UPDATE replicated_blobs SET last_used_at = ? WHERE hash = ?`,
		now, hash,
	); err != nil {
		return fmt.Errorf("UpsertReplicatedBlob touch: %w", err)
	}
	return nil
}

// GCUnreferencedReplicatedBlobs removes old replicated_blobs rows that no record references.
func (d *Database) GCUnreferencedReplicatedBlobs(cutoffUnix int64, limit int) (int64, error) {
	return d.gcBlobTable("replicated_blobs", cutoffUnix, limit, `
		SELECT b.hash FROM replicated_blobs b
		WHERE b.last_used_at < ?
		  AND NOT EXISTS (
			SELECT 1 FROM replicated_record_blobs rb WHERE rb.blob_hash = b.hash
		  )
		ORDER BY b.last_used_at ASC
		LIMIT ?`)
}

// GCUnreferencedAvatarBlobs removes old avatar_blobs rows not referenced by projections or replicated records.
func (d *Database) GCUnreferencedAvatarBlobs(cutoffUnix int64, limit int) (int64, error) {
	return d.gcBlobTable("avatar_blobs", cutoffUnix, limit, `
		SELECT b.hash FROM avatar_blobs b
		WHERE b.last_used_at < ?
		  AND NOT EXISTS (
			SELECT 1 FROM local_profile lp WHERE lower(COALESCE(lp.avatar_hash, '')) = b.hash
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM peer_directory pd WHERE lower(COALESCE(pd.avatar_hash, '')) = b.hash
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM replicated_record_blobs rb WHERE rb.blob_hash = b.hash
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM mls_groups g WHERE lower(trim(COALESCE(g.group_avatar_hash, ''))) = b.hash
		  )
		ORDER BY b.last_used_at ASC
		LIMIT ?`)
}

func (d *Database) gcBlobTable(table string, cutoffUnix int64, limit int, selectSQL string) (int64, error) {
	table = strings.TrimSpace(table)
	if table != "replicated_blobs" && table != "avatar_blobs" {
		return 0, fmt.Errorf("unsupported blob table %q", table)
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.Conn.Query(selectSQL, cutoffUnix, limit)
	if err != nil {
		return 0, fmt.Errorf("gc select %s: %w", table, err)
	}
	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			_ = rows.Close()
			return 0, err
		}
		hashes = append(hashes, h)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var deleted int64
	for _, h := range hashes {
		res, err := d.Conn.Exec(`DELETE FROM `+table+` WHERE hash = ?`, h)
		if err != nil {
			return deleted, fmt.Errorf("gc delete %s: %w", table, err)
		}
		n, _ := res.RowsAffected()
		deleted += n
	}
	return deleted, nil
}
