package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SavePeerProfile inserts or updates a peer's display name mapping.
func (d *Database) SavePeerProfile(peerID string, displayName string) error {
	if peerID == "" || displayName == "" {
		return nil
	}
	_, err := d.Conn.Exec(
		`INSERT OR REPLACE INTO peer_directory (peer_id, display_name) VALUES (?, ?)`,
		peerID, displayName,
	)
	if err != nil {
		return fmt.Errorf("SavePeerProfile: %w", err)
	}
	return nil
}

func (d *Database) UpsertPeerProfile(peerID, displayName string) error {
	return d.SavePeerProfile(peerID, displayName)
}

// UpsertPeerProfileWithKey inserts or updates a peer's display name and public
// key hex. The public key is stored in a separate column so that future
// lookups by credential (GetPeerIDByPublicKeyHex) work even before the peer
// directory is fully populated by heartbeats. This is critical for the
// group_members sync path: when MLS leaf enumeration yields a credential
// that has no peer_directory row yet, we can still resolve it via the public
// key column. The display_name is always upserted so that later heartbeats
// with updated display names propagate cleanly.
//
// Returns nil if peerID is empty (defensive no-op matching SavePeerProfile).
func (d *Database) UpsertPeerProfileWithKey(peerID, displayName, publicKeyHex string) error {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return nil
	}
	displayName = strings.TrimSpace(displayName)
	publicKeyHex = strings.ToLower(strings.TrimSpace(publicKeyHex))
	_, err := d.Conn.Exec(
		`INSERT INTO peer_directory (peer_id, display_name, public_key_hex)
		 VALUES (?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   display_name = CASE WHEN excluded.display_name <> '' THEN excluded.display_name ELSE peer_directory.display_name END,
		   public_key_hex = CASE WHEN excluded.public_key_hex <> '' THEN excluded.public_key_hex ELSE peer_directory.public_key_hex END`,
		peerID, displayName, publicKeyHex,
	)
	if err != nil {
		return fmt.Errorf("UpsertPeerProfileWithKey: %w", err)
	}
	return nil
}

// GetPeerIDByPublicKeyHex looks up a peer_id by its Ed25519 public key hex.
// Returns "" + nil when the key is not found (not an error) so callers can fall
// back to heartbeat-driven roster sync (Phase A) for that leaf. Empty input
// is treated as a miss rather than an error so callers can pass straight from
// hex.EncodeToString output without extra null checks.
func (d *Database) GetPeerIDByPublicKeyHex(publicKeyHex string) (string, error) {
	publicKeyHex = strings.ToLower(strings.TrimSpace(publicKeyHex))
	if publicKeyHex == "" {
		return "", nil
	}
	var peerID string
	err := d.Conn.QueryRow(
		`SELECT peer_id FROM peer_directory WHERE public_key_hex = ?`, publicKeyHex,
	).Scan(&peerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("GetPeerIDByPublicKeyHex: %w", err)
	}
	return strings.TrimSpace(peerID), nil
}

// GetPeerDisplayName retrieves the display name of a peer, or empty if unknown.
func (d *Database) GetPeerDisplayName(peerID string) (string, error) {
	var name string
	err := d.Conn.QueryRow("SELECT display_name FROM peer_directory WHERE peer_id = ?", peerID).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return name, nil
}

func (d *Database) GetPeerProfile(peerID string) (string, error) {
	return d.GetPeerDisplayName(peerID)
}

// GetAllPeerProfiles returns all cached PeerID -> Display Name mappings.
func (d *Database) GetAllPeerProfiles() (map[string]string, error) {
	rows, err := d.Conn.Query("SELECT peer_id, display_name FROM peer_directory")
	if err != nil {
		return nil, fmt.Errorf("GetAllPeerProfiles: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var pid, name string
		if err := rows.Scan(&pid, &name); err != nil {
			return nil, err
		}
		out[pid] = name
	}
	return out, rows.Err()
}
