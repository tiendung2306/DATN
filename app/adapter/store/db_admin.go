package store

import (
	"fmt"
)

type AdminIssuanceRecord struct {
	ID           string
	DisplayName  string
	PeerID       string
	PublicKeyHex string
	IssuedAt     int64
	ExpiresAt    int64
	Note         string
	BundlePath   string
}

func (d *Database) SaveAdminIssuanceRecord(rec AdminIssuanceRecord) error {
	_, err := d.Conn.Exec(
		`INSERT OR REPLACE INTO admin_issuance_history
		 (id, display_name, peer_id, public_key_hex, issued_at, expires_at, note, bundle_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.DisplayName, rec.PeerID, rec.PublicKeyHex, rec.IssuedAt, rec.ExpiresAt, rec.Note, rec.BundlePath,
	)
	if err != nil {
		return fmt.Errorf("SaveAdminIssuanceRecord: %w", err)
	}
	return nil
}

func (d *Database) ListAdminIssuanceHistory() ([]AdminIssuanceRecord, error) {
	rows, err := d.Conn.Query(
		`SELECT id, display_name, peer_id, public_key_hex, issued_at, expires_at, note, bundle_path
		 FROM admin_issuance_history
		 ORDER BY issued_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListAdminIssuanceHistory: %w", err)
	}
	defer rows.Close()
	out := make([]AdminIssuanceRecord, 0)
	for rows.Next() {
		var rec AdminIssuanceRecord
		if err := rows.Scan(&rec.ID, &rec.DisplayName, &rec.PeerID, &rec.PublicKeyHex, &rec.IssuedAt, &rec.ExpiresAt, &rec.Note, &rec.BundlePath); err != nil {
			return nil, fmt.Errorf("ListAdminIssuanceHistory scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
