package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// MaxAvatarBytes is the maximum allowed avatar image size (256 KiB).
const MaxAvatarBytes = 256 * 1024

// LocalProfileRow is the single-row local user profile (id = 1).
type LocalProfileRow struct {
	PeerID          string
	DisplayName     string
	Email           sql.NullString
	Phone           sql.NullString
	AvatarHash      sql.NullString
	AvatarMime      sql.NullString
	AvatarUpdatedAt int64
	ProfileRevision int64
	UpdatedAtUnix   int64
}

// PeerDirectoryProfileRow is peer_directory including optional profile fields.
type PeerDirectoryProfileRow struct {
	PeerID           string
	DisplayName      string
	PublicKeyHex     string
	Email            sql.NullString
	Phone            sql.NullString
	AvatarHash       sql.NullString
	AvatarMime       sql.NullString
	AvatarUpdatedAt  int64
	ProfileRevision  int64
	ProfileSignature sql.NullString
	UpdatedAtUnix    int64
}

func (d *Database) UpsertAvatarBlob(hash, mime string, data []byte) error {
	hash = strings.TrimSpace(strings.ToLower(hash))
	mime = strings.TrimSpace(mime)
	if hash == "" || mime == "" || len(data) == 0 {
		return fmt.Errorf("avatar blob: hash, mime and bytes are required")
	}
	if len(data) > MaxAvatarBytes {
		return fmt.Errorf(
			"avatar blob: size %d exceeds max %d (%d KiB); compress client-side before upload",
			len(data), MaxAvatarBytes, MaxAvatarBytes/1024,
		)
	}
	now := time.Now().Unix()
	if _, err := d.Conn.Exec(
		`INSERT OR IGNORE INTO avatar_blobs (hash, mime, size, bytes, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		hash, mime, len(data), data, now, now,
	); err != nil {
		return fmt.Errorf("UpsertAvatarBlob insert: %w", err)
	}
	if _, err := d.Conn.Exec(
		`UPDATE avatar_blobs SET last_used_at = ? WHERE hash = ?`,
		now, hash,
	); err != nil {
		return fmt.Errorf("UpsertAvatarBlob touch: %w", err)
	}
	return nil
}

func (d *Database) GetAvatarBlob(hash string) (mime string, data []byte, err error) {
	hash = strings.TrimSpace(strings.ToLower(hash))
	if hash == "" {
		return "", nil, fmt.Errorf("avatar hash is required")
	}
	var b []byte
	var m string
	err = d.Conn.QueryRow(
		`SELECT mime, bytes FROM avatar_blobs WHERE hash = ?`, hash,
	).Scan(&m, &b)
	if err != nil {
		return "", nil, err
	}
	return m, b, nil
}

func (d *Database) TouchAvatarBlobUsed(hash string) error {
	hash = strings.TrimSpace(strings.ToLower(hash))
	if hash == "" {
		return nil
	}
	_, err := d.Conn.Exec(
		`UPDATE avatar_blobs SET last_used_at = ? WHERE hash = ?`,
		time.Now().Unix(), hash,
	)
	if err != nil {
		return fmt.Errorf("TouchAvatarBlobUsed: %w", err)
	}
	return nil
}

// GetLocalProfile returns the local_profile row (id=1), or sql.ErrNoRows if missing.
func (d *Database) GetLocalProfile() (*LocalProfileRow, error) {
	var r LocalProfileRow
	err := d.Conn.QueryRow(
		`SELECT peer_id, display_name, email, phone, avatar_hash, avatar_mime, avatar_updated_at, profile_revision,
		        COALESCE(updated_at, 0)
		 FROM local_profile WHERE id = 1`,
	).Scan(
		&r.PeerID, &r.DisplayName, &r.Email, &r.Phone, &r.AvatarHash, &r.AvatarMime,
		&r.AvatarUpdatedAt, &r.ProfileRevision, &r.UpdatedAtUnix,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// EnsureLocalProfileRow inserts id=1 row if absent (peer_id and display_name may be empty).
func (d *Database) EnsureLocalProfileRow(peerID, displayName string) error {
	peerID = strings.TrimSpace(peerID)
	displayName = strings.TrimSpace(displayName)
	_, err := d.Conn.Exec(
		`INSERT OR IGNORE INTO local_profile (id, peer_id, display_name, profile_revision, updated_at)
		 VALUES (1, ?, ?, 1, strftime('%s','now'))`,
		peerID, displayName,
	)
	if err != nil {
		return fmt.Errorf("EnsureLocalProfileRow: %w", err)
	}
	return nil
}

// EnsureLocalProfileRevisionFloor heals legacy rows seeded with revision 0 so
// replicated profile records always satisfy the replicated-store monotonicity contract.
func (d *Database) EnsureLocalProfileRevisionFloor(minRevision int64) error {
	if minRevision <= 0 {
		minRevision = 1
	}
	_, err := d.Conn.Exec(
		`UPDATE local_profile
		    SET profile_revision = CASE
		            WHEN profile_revision < ? THEN ?
		            ELSE profile_revision
		        END
		  WHERE id = 1`,
		minRevision, minRevision,
	)
	if err != nil {
		return fmt.Errorf("EnsureLocalProfileRevisionFloor: %w", err)
	}
	return nil
}

// SaveLocalProfileContacts sets email/phone (NULL when not valid) and bumps profile_revision.
func (d *Database) SaveLocalProfileContacts(email, phone sql.NullString, nextRevision int64) error {
	res, err := d.Conn.Exec(
		`UPDATE local_profile SET
			email = ?,
			phone = ?,
			profile_revision = ?,
			updated_at = strftime('%s','now')
		 WHERE id = 1`,
		email, phone, nextRevision,
	)
	if err != nil {
		return fmt.Errorf("SaveLocalProfileContacts: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("SaveLocalProfileContacts: no local_profile row")
	}
	return nil
}

// SaveLocalProfileAvatar sets avatar fields, bumps profile_revision.
func (d *Database) SaveLocalProfileAvatar(hash, mime string, updatedAtUnix, nextRevision int64) error {
	hash = strings.TrimSpace(strings.ToLower(hash))
	mime = strings.TrimSpace(mime)
	if hash == "" || mime == "" {
		return fmt.Errorf("avatar hash and mime are required")
	}
	_, err := d.Conn.Exec(
		`UPDATE local_profile SET
			avatar_hash = ?,
			avatar_mime = ?,
			avatar_updated_at = ?,
			profile_revision = ?,
			updated_at = strftime('%s','now')
		 WHERE id = 1`,
		hash, mime, updatedAtUnix, nextRevision,
	)
	if err != nil {
		return fmt.Errorf("SaveLocalProfileAvatar: %w", err)
	}
	return nil
}

// ClearLocalProfileAvatar clears avatar columns and bumps revision.
func (d *Database) ClearLocalProfileAvatar(nextRevision int64) error {
	_, err := d.Conn.Exec(
		`UPDATE local_profile SET
			avatar_hash = NULL,
			avatar_mime = NULL,
			avatar_updated_at = 0,
			profile_revision = ?,
			updated_at = strftime('%s','now')
		 WHERE id = 1`,
		nextRevision,
	)
	if err != nil {
		return fmt.Errorf("ClearLocalProfileAvatar: %w", err)
	}
	return nil
}

// SyncLocalProfileIdentity copies peer_id and locked display_name from MLS identity into local_profile.
func (d *Database) SyncLocalProfileIdentity(peerID, displayName string) error {
	peerID = strings.TrimSpace(peerID)
	displayName = strings.TrimSpace(displayName)
	_, err := d.Conn.Exec(
		`UPDATE local_profile SET peer_id = ?, display_name = ?, updated_at = strftime('%s','now') WHERE id = 1`,
		peerID, displayName,
	)
	if err != nil {
		return fmt.Errorf("SyncLocalProfileIdentity: %w", err)
	}
	return nil
}

// GetPeerDirectoryProfile returns peer_directory profile columns or sql.ErrNoRows.
func (d *Database) GetPeerDirectoryProfile(peerID string) (*PeerDirectoryProfileRow, error) {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return nil, fmt.Errorf("peer_id is required")
	}
	var r PeerDirectoryProfileRow
	err := d.Conn.QueryRow(
		`SELECT peer_id, display_name, public_key_hex, email, phone, avatar_hash, avatar_mime,
		        COALESCE(avatar_updated_at, 0), COALESCE(profile_revision, 0), profile_signature,
		        CAST(strftime('%s', updated_at) AS INTEGER)
		 FROM peer_directory WHERE peer_id = ?`,
		peerID,
	).Scan(
		&r.PeerID, &r.DisplayName, &r.PublicKeyHex, &r.Email, &r.Phone, &r.AvatarHash, &r.AvatarMime,
		&r.AvatarUpdatedAt, &r.ProfileRevision, &r.ProfileSignature, &r.UpdatedAtUnix,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UpsertPeerDirectorySigned upserts peer_directory with full profile + signature (local writes and verified applies).
func (d *Database) UpsertPeerDirectorySigned(
	peerID, displayName, publicKeyHex string,
	email, phone sql.NullString,
	avatarHash, avatarMime sql.NullString,
	avatarUpdatedAt, profileRevision int64,
	signatureHex string,
) error {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return fmt.Errorf("peer_id is required")
	}
	displayName = strings.TrimSpace(displayName)
	publicKeyHex = strings.ToLower(strings.TrimSpace(publicKeyHex))
	sig := strings.TrimSpace(signatureHex)
	_, err := d.Conn.Exec(
		`INSERT INTO peer_directory (
			peer_id, display_name, public_key_hex, email, phone,
			avatar_hash, avatar_mime, avatar_updated_at, profile_revision, profile_signature, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(peer_id) DO UPDATE SET
			display_name = excluded.display_name,
			public_key_hex = CASE WHEN trim(excluded.public_key_hex) <> '' THEN excluded.public_key_hex ELSE peer_directory.public_key_hex END,
			email = excluded.email,
			phone = excluded.phone,
			avatar_hash = excluded.avatar_hash,
			avatar_mime = excluded.avatar_mime,
			avatar_updated_at = excluded.avatar_updated_at,
			profile_revision = excluded.profile_revision,
			profile_signature = excluded.profile_signature,
			updated_at = CURRENT_TIMESTAMP`,
		peerID, displayName, publicKeyHex, email, phone,
		avatarHash, avatarMime, avatarUpdatedAt, profileRevision, sig,
	)
	if err != nil {
		return fmt.Errorf("UpsertPeerDirectorySigned: %w", err)
	}
	return nil
}

// MergePeerDirectoryProfile applies a verified profile update: revision must be strictly greater,
// or equal for idempotent replay. Field clears are expressed via clearedFields ("email", "phone", "avatar").
// When clearedFields is empty, empty incoming strings preserve existing non-empty values (legacy peers).
func (d *Database) MergePeerDirectoryProfile(
	peerID string,
	newRevision int64,
	email, phone, avatarHash, avatarMime string,
	avatarUpdatedAt int64,
	signature []byte,
	clearedFields []string,
) error {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return fmt.Errorf("peer_id is required")
	}
	if newRevision <= 0 {
		return fmt.Errorf("invalid profile revision")
	}
	existing, err := d.GetPeerDirectoryProfile(peerID)
	if err != nil {
		return err
	}
	if existing.ProfileRevision > newRevision {
		return fmt.Errorf("stale profile revision")
	}
	if existing.ProfileRevision == newRevision {
		// Idempotent replay (e.g. profile push on reconnect with same revision).
		return nil
	}

	cleared := map[string]struct{}{}
	for _, f := range clearedFields {
		switch strings.ToLower(strings.TrimSpace(f)) {
		case "email", "phone", "avatar":
			cleared[strings.ToLower(strings.TrimSpace(f))] = struct{}{}
		}
	}

	mergeStr := func(field string, cur sql.NullString, incoming string) sql.NullString {
		if _, ok := cleared[field]; ok {
			return sql.NullString{}
		}
		in := strings.TrimSpace(incoming)
		if in == "" {
			return cur
		}
		return sql.NullString{String: in, Valid: true}
	}
	mergeAvatarHash := func(cur sql.NullString, incoming string) sql.NullString {
		if _, ok := cleared["avatar"]; ok {
			return sql.NullString{}
		}
		in := strings.TrimSpace(strings.ToLower(incoming))
		if in == "" {
			return cur
		}
		return sql.NullString{String: in, Valid: true}
	}
	mergeAvatarMime := func(cur sql.NullString, incoming string) sql.NullString {
		if _, ok := cleared["avatar"]; ok {
			return sql.NullString{}
		}
		in := strings.TrimSpace(incoming)
		if in == "" {
			return cur
		}
		return sql.NullString{String: in, Valid: true}
	}

	outEmail := mergeStr("email", existing.Email, email)
	outPhone := mergeStr("phone", existing.Phone, phone)
	outHash := mergeAvatarHash(existing.AvatarHash, avatarHash)
	outMime := mergeAvatarMime(existing.AvatarMime, avatarMime)
	outAvatarAt := existing.AvatarUpdatedAt
	if _, ok := cleared["avatar"]; ok {
		outAvatarAt = 0
	} else if strings.TrimSpace(avatarHash) != "" {
		outAvatarAt = avatarUpdatedAt
	}

	sigHex := hex.EncodeToString(signature)
	return d.UpsertPeerDirectorySigned(
		peerID, existing.DisplayName, existing.PublicKeyHex,
		outEmail, outPhone, outHash, outMime, outAvatarAt, newRevision, sigHex,
	)
}

// AvatarContentHash returns lowercase hex SHA-256 of data.
func AvatarContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
