package store

import (
	"fmt"
)

// SaveMLSIdentity persists the local MLS identity (overwrites if exists).
// credential may be empty at key-generation time (Admin assigns it later via
// UpdateMLSDisplayName). We normalise nil → []byte{} to satisfy the NOT NULL
// constraint; proto3 zero-value bytes fields deserialise as nil in Go.
func (d *Database) SaveMLSIdentity(id *MLSIdentity) error {
	credential := id.Credential
	if credential == nil {
		credential = []byte{}
	}
	_, err := d.Conn.Exec(
		`INSERT OR REPLACE INTO mls_identity
		 (id, display_name, public_key, signing_key_private, credential)
		 VALUES (1, ?, ?, ?, ?)`,
		id.DisplayName, id.PublicKey, id.SigningKeyPrivate, credential,
	)
	if err != nil {
		return fmt.Errorf("SaveMLSIdentity: %w", err)
	}
	return nil
}

// GetMLSIdentity retrieves the local MLS identity.
// Returns sql.ErrNoRows if no identity has been generated yet.
func (d *Database) GetMLSIdentity() (*MLSIdentity, error) {
	var id MLSIdentity
	err := d.Conn.QueryRow(
		`SELECT display_name, public_key, signing_key_private, credential
		 FROM mls_identity WHERE id = 1`,
	).Scan(&id.DisplayName, &id.PublicKey, &id.SigningKeyPrivate, &id.Credential)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// UpdateMLSDisplayName updates the display_name and credential of the stored MLS identity.
// Called after an InvitationBundle is imported — the Admin-assigned name replaces the
// placeholder that was stored at key generation time.
func (d *Database) UpdateMLSDisplayName(displayName string) error {
	_, err := d.Conn.Exec(
		`UPDATE mls_identity SET display_name = ?, credential = ? WHERE id = 1`,
		displayName, []byte(displayName),
	)
	if err != nil {
		return fmt.Errorf("UpdateMLSDisplayName: %w", err)
	}
	return nil
}

// HasMLSIdentity returns true if a local MLS identity exists.
func (d *Database) HasMLSIdentity() (bool, error) {
	var count int
	err := d.Conn.QueryRow("SELECT COUNT(*) FROM mls_identity").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("HasMLSIdentity: %w", err)
	}
	return count > 0, nil
}

// ── auth_bundle ───────────────────────────────────────────────────────────────

// SaveAuthBundle persists the InvitationBundle from Admin (overwrites if exists).
func (d *Database) SaveAuthBundle(b *StoredAuthBundle) error {
	_, err := d.Conn.Exec(
		`INSERT OR REPLACE INTO auth_bundle
		 (id, display_name, peer_id, public_key, token_issued_at, token_expires_at,
		  token_signature, bootstrap_addr, root_public_key)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.DisplayName, b.PeerID, b.PublicKey, b.TokenIssuedAt, b.TokenExpiresAt,
		b.TokenSignature, b.BootstrapAddr, b.RootPublicKey,
	)
	if err != nil {
		return fmt.Errorf("SaveAuthBundle: %w", err)
	}
	return nil
}

// GetAuthBundle retrieves the stored InvitationBundle.
// Returns sql.ErrNoRows if no bundle has been imported yet.
func (d *Database) GetAuthBundle() (*StoredAuthBundle, error) {
	var b StoredAuthBundle
	err := d.Conn.QueryRow(
		`SELECT display_name, peer_id, public_key, token_issued_at, token_expires_at,
		        token_signature, bootstrap_addr, root_public_key
		 FROM auth_bundle WHERE id = 1`,
	).Scan(
		&b.DisplayName, &b.PeerID, &b.PublicKey, &b.TokenIssuedAt, &b.TokenExpiresAt,
		&b.TokenSignature, &b.BootstrapAddr, &b.RootPublicKey,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// HasAuthBundle returns true if an InvitationBundle has been imported.
func (d *Database) HasAuthBundle() (bool, error) {
	var count int
	err := d.Conn.QueryRow("SELECT COUNT(*) FROM auth_bundle").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("HasAuthBundle: %w", err)
	}
	return count > 0, nil
}

// ── kp_bundles ────────────────────────────────────────────────────────────────

// SaveKPBundle upserts the local KeyPackage (public + private) keyed by peerID.
// There is at most one active KP per peer.  A new call replaces the old one.
func (d *Database) SaveKPBundle(peerID string, publicKP, privateBundle []byte) error {
	_, err := d.Conn.Exec(
		`INSERT OR REPLACE INTO kp_bundles (peer_id, public_kp, private_bundle)
		 VALUES (?, ?, ?)`,
		peerID, publicKP, privateBundle,
	)
	if err != nil {
		return fmt.Errorf("SaveKPBundle: %w", err)
	}
	return nil
}

// GetKPBundle retrieves the stored public KP + private bundle for peerID.
// Returns sql.ErrNoRows if none has been generated yet.
func (d *Database) GetKPBundle(peerID string) (publicKP, privateBundle []byte, err error) {
	err = d.Conn.QueryRow(
		`SELECT public_kp, private_bundle FROM kp_bundles WHERE peer_id = ?`, peerID,
	).Scan(&publicKP, &privateBundle)
	return
}
