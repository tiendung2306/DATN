package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func normalizeGroupType(groupType string) string {
	switch strings.TrimSpace(groupType) {
	case "dm":
		return "dm"
	case "group":
		return "group"
	case "channel":
		return "channel"
	default:
		return "channel"
	}
}

// ── pending_welcomes_out ──────────────────────────────────────────────────────

// SavePendingWelcome stores a Welcome that needs to be delivered to targetPeerID.
// Overwrites any previous pending welcome for the same (target, group) pair.
func (d *Database) SavePendingWelcome(targetPeerID, groupID string, welcomeBytes []byte, anchorEpoch uint64, anchorHistoryHash []byte) error {
	_, err := d.Conn.Exec(
		`INSERT OR REPLACE INTO pending_welcomes_out
		 (target_peer_id, group_id, welcome_bytes, delivered, anchor_epoch, anchor_history_hash)
		 VALUES (?, ?, ?, 0, ?, ?)`,
		targetPeerID, groupID, welcomeBytes, anchorEpoch, anchorHistoryHash,
	)
	if err != nil {
		return fmt.Errorf("SavePendingWelcome: %w", err)
	}
	return nil
}

// GetAnyPendingWelcomeForGroup returns the latest stored welcome for a
// (target_peer_id, group_id) pair regardless of delivered flag.
func (d *Database) GetAnyPendingWelcomeForGroup(targetPeerID, groupID string) ([]byte, error) {
	var welcome []byte
	err := d.Conn.QueryRow(
		`SELECT welcome_bytes
		 FROM pending_welcomes_out
		 WHERE target_peer_id = ? AND group_id = ?
		 ORDER BY id DESC
		 LIMIT 1`,
		targetPeerID, groupID,
	).Scan(&welcome)
	if err != nil {
		return nil, err
	}
	if len(welcome) == 0 {
		return nil, sql.ErrNoRows
	}
	return welcome, nil
}

// PendingWelcome is one undelivered Welcome record.
type PendingWelcome struct {
	ID                int64
	TargetPeerID      string
	GroupID           string
	WelcomeBytes      []byte
	AnchorEpoch       uint64
	AnchorHistoryHash []byte
}

// GetPendingWelcomesFor returns all undelivered Welcomes for the given peer.
func (d *Database) GetPendingWelcomesFor(targetPeerID string) ([]PendingWelcome, error) {
	rows, err := d.Conn.Query(
		`SELECT id, target_peer_id, group_id, welcome_bytes, anchor_epoch, anchor_history_hash
		 FROM pending_welcomes_out
		 WHERE target_peer_id = ? AND delivered = 0`,
		targetPeerID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetPendingWelcomesFor: %w", err)
	}
	defer rows.Close()

	var out []PendingWelcome
	for rows.Next() {
		var pw PendingWelcome
		if err := rows.Scan(&pw.ID, &pw.TargetPeerID, &pw.GroupID, &pw.WelcomeBytes, &pw.AnchorEpoch, &pw.AnchorHistoryHash); err != nil {
			return nil, err
		}
		out = append(out, pw)
	}
	return out, rows.Err()
}

func (d *Database) ListPendingWelcomes() ([]PendingWelcome, error) {
	rows, err := d.Conn.Query(
		`SELECT id, target_peer_id, group_id, welcome_bytes, anchor_epoch, anchor_history_hash
		 FROM pending_welcomes_out
		 WHERE delivered = 0`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListPendingWelcomes: %w", err)
	}
	defer rows.Close()

	var out []PendingWelcome
	for rows.Next() {
		var pw PendingWelcome
		if err := rows.Scan(&pw.ID, &pw.TargetPeerID, &pw.GroupID, &pw.WelcomeBytes, &pw.AnchorEpoch, &pw.AnchorHistoryHash); err != nil {
			return nil, err
		}
		out = append(out, pw)
	}
	return out, rows.Err()
}

// MarkWelcomeDelivered marks a pending welcome row as delivered.
func (d *Database) MarkWelcomeDelivered(id int64) error {
	_, err := d.Conn.Exec(
		`UPDATE pending_welcomes_out SET delivered = 1 WHERE id = ?`, id,
	)
	return err
}

func (d *Database) MarkWelcomeDeliveredFor(targetPeerID, groupID string) error {
	_, err := d.Conn.Exec(
		`UPDATE pending_welcomes_out
		 SET delivered = 1
		 WHERE target_peer_id = ? AND group_id = ?`,
		targetPeerID, groupID,
	)
	return err
}

func (d *Database) ReopenWelcomeDelivery(targetPeerID, groupID string) error {
	_, err := d.Conn.Exec(
		`UPDATE pending_welcomes_out
		 SET delivered = 0
		 WHERE target_peer_id = ? AND group_id = ?`,
		targetPeerID, groupID,
	)
	return err
}

func (d *Database) ReopenUnacknowledgedPendingWelcomes() (int64, error) {
	res, err := d.Conn.Exec(
		`UPDATE pending_welcomes_out
		 SET delivered = 0
		 WHERE delivered = 1
		   AND EXISTS (
		     SELECT 1 FROM group_add_operations gao
		     WHERE gao.group_id = pending_welcomes_out.group_id
		       AND gao.target_peer_id = pending_welcomes_out.target_peer_id
		       AND gao.status NOT IN ('welcome_delivered', 'failed')
		   )`,
	)
	if err != nil {
		return 0, fmt.Errorf("ReopenUnacknowledgedPendingWelcomes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("ReopenUnacknowledgedPendingWelcomes rows: %w", err)
	}
	return n, nil
}

// ── stored_keypackages ────────────────────────────────────────────────────────

// SaveStoredKeyPackage upserts a public KeyPackage replica for peerID.
func (d *Database) SaveStoredKeyPackage(peerID string, publicKP []byte, sourcePeerID string) error {
	_, err := d.Conn.Exec(
		`INSERT INTO stored_keypackages (peer_id, public_kp, source_peer_id, updated_at)
		 VALUES (?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(peer_id) DO UPDATE SET
		   public_kp = excluded.public_kp,
		   source_peer_id = excluded.source_peer_id,
		   updated_at = excluded.updated_at`,
		peerID, publicKP, sourcePeerID,
	)
	if err != nil {
		return fmt.Errorf("SaveStoredKeyPackage: %w", err)
	}
	return nil
}

// SaveStoredKeyPackageIfNewer upserts only when publishedAt is newer.
func (d *Database) SaveStoredKeyPackageIfNewer(peerID string, publicKP []byte, sourcePeerID string, publishedAt int64) error {
	_, err := d.Conn.Exec(
		`INSERT INTO stored_keypackages (peer_id, public_kp, source_peer_id, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   public_kp = CASE WHEN excluded.updated_at >= stored_keypackages.updated_at THEN excluded.public_kp ELSE stored_keypackages.public_kp END,
		   source_peer_id = CASE WHEN excluded.updated_at >= stored_keypackages.updated_at THEN excluded.source_peer_id ELSE stored_keypackages.source_peer_id END,
		   updated_at = CASE WHEN excluded.updated_at >= stored_keypackages.updated_at THEN excluded.updated_at ELSE stored_keypackages.updated_at END`,
		peerID, publicKP, sourcePeerID, publishedAt,
	)
	if err != nil {
		return fmt.Errorf("SaveStoredKeyPackageIfNewer: %w", err)
	}
	return nil
}

// GetStoredKeyPackage fetches a replicated public KeyPackage for peerID.
func (d *Database) GetStoredKeyPackage(peerID string) ([]byte, error) {
	var kp []byte
	if err := d.Conn.QueryRow(
		`SELECT public_kp FROM stored_keypackages WHERE peer_id = ?`, peerID,
	).Scan(&kp); err != nil {
		return nil, err
	}
	return kp, nil
}

// ── stored_welcomes ───────────────────────────────────────────────────────────

// SaveStoredWelcome upserts a replicated Welcome for (invitee, group).
// categoryID is the channel-category id at the time of welcome creation; pass
// "" for non-channel groups or when the value is unknown (older wire frames).
// On conflict, a non-empty incoming category_id always wins so a later wire
// carrying the metadata can heal earlier rows that were saved before this
// field existed.
func (d *Database) SaveStoredWelcome(inviteePeerID, groupID, groupType, categoryID string, welcome []byte, sourcePeerID string, anchorEpoch uint64, anchorHistoryHash []byte) error {
	groupType = normalizeGroupType(groupType)
	categoryID = strings.TrimSpace(categoryID)
	sourcePeerID = strings.TrimSpace(sourcePeerID)
	_, err := d.Conn.Exec(
		`INSERT INTO stored_welcomes (invitee_peer_id, group_id, group_type, category_id, welcome_bytes, source_peer_id, anchor_epoch, anchor_history_hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(invitee_peer_id, group_id) DO UPDATE SET
		   group_type = excluded.group_type,
		   category_id = CASE WHEN excluded.category_id <> '' THEN excluded.category_id ELSE stored_welcomes.category_id END,
		   welcome_bytes = excluded.welcome_bytes,
		   source_peer_id = CASE WHEN trim(excluded.source_peer_id) <> '' THEN excluded.source_peer_id ELSE stored_welcomes.source_peer_id END,
		   anchor_epoch = CASE WHEN excluded.anchor_epoch > stored_welcomes.anchor_epoch THEN excluded.anchor_epoch ELSE stored_welcomes.anchor_epoch END,
		   anchor_history_hash = CASE WHEN excluded.anchor_epoch > stored_welcomes.anchor_epoch THEN excluded.anchor_history_hash ELSE stored_welcomes.anchor_history_hash END,
		   created_at = excluded.created_at`,
		inviteePeerID, groupID, groupType, categoryID, welcome, sourcePeerID, anchorEpoch, anchorHistoryHash,
	)
	if err != nil {
		return fmt.Errorf("SaveStoredWelcome: %w", err)
	}
	return nil
}

// SaveStoredWelcomeIfNewer upserts only when publishedAt is newer. Same
// category_id heal semantics as SaveStoredWelcome: a non-empty incoming
// category_id always wins regardless of timestamp comparison so missing
// metadata in the older row gets filled in.
func (d *Database) SaveStoredWelcomeIfNewer(inviteePeerID, groupID, groupType, categoryID string, welcome []byte, sourcePeerID string, publishedAt int64, anchorEpoch uint64, anchorHistoryHash []byte) error {
	groupType = normalizeGroupType(groupType)
	categoryID = strings.TrimSpace(categoryID)
	sourcePeerID = strings.TrimSpace(sourcePeerID)
	_, err := d.Conn.Exec(
		`INSERT INTO stored_welcomes (invitee_peer_id, group_id, group_type, category_id, welcome_bytes, source_peer_id, anchor_epoch, anchor_history_hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(invitee_peer_id, group_id) DO UPDATE SET
		   group_type = CASE WHEN excluded.created_at >= stored_welcomes.created_at THEN excluded.group_type ELSE stored_welcomes.group_type END,
		   category_id = CASE WHEN excluded.category_id <> '' THEN excluded.category_id ELSE stored_welcomes.category_id END,
		   welcome_bytes = CASE WHEN excluded.created_at >= stored_welcomes.created_at THEN excluded.welcome_bytes ELSE stored_welcomes.welcome_bytes END,
		   source_peer_id = CASE WHEN trim(excluded.source_peer_id) <> '' AND excluded.created_at >= stored_welcomes.created_at THEN excluded.source_peer_id ELSE stored_welcomes.source_peer_id END,
		   anchor_epoch = CASE WHEN excluded.anchor_epoch > stored_welcomes.anchor_epoch THEN excluded.anchor_epoch ELSE stored_welcomes.anchor_epoch END,
		   anchor_history_hash = CASE WHEN excluded.anchor_epoch > stored_welcomes.anchor_epoch THEN excluded.anchor_history_hash ELSE stored_welcomes.anchor_history_hash END,
		   created_at = CASE WHEN excluded.created_at >= stored_welcomes.created_at THEN excluded.created_at ELSE stored_welcomes.created_at END`,
		inviteePeerID, groupID, groupType, categoryID, welcome, sourcePeerID, anchorEpoch, anchorHistoryHash, publishedAt,
	)
	if err != nil {
		return fmt.Errorf("SaveStoredWelcomeIfNewer: %w", err)
	}
	return nil
}

// DeleteStoredWelcome removes a stored welcome replica for an invitee+group pair.
func (d *Database) DeleteStoredWelcome(inviteePeerID, groupID string) error {
	res, err := d.Conn.Exec(
		`DELETE FROM stored_welcomes WHERE invitee_peer_id = ? AND group_id = ?`,
		inviteePeerID, groupID,
	)
	if err != nil {
		return fmt.Errorf("DeleteStoredWelcome: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("DeleteStoredWelcome rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetStoredWelcome fetches a replicated Welcome for (invitee, group).
// The returned sourcePeerID is who replicated/stored the welcome (typically
// the inviter / group creator) — callers must pass it to invite join paths,
// not the invitee's own peer ID, or creator resolution for RequestGroupInvite
// breaks on member nodes.
func (d *Database) GetStoredWelcome(inviteePeerID, groupID string) ([]byte, string, string, string, uint64, []byte, error) {
	var wb []byte
	var groupType, categoryID, sourcePeerID string
	var anchorEpoch uint64
	var anchorHistoryHash []byte
	if err := d.Conn.QueryRow(
		`SELECT welcome_bytes, group_type, category_id, source_peer_id, anchor_epoch, anchor_history_hash FROM stored_welcomes WHERE invitee_peer_id = ? AND group_id = ?`,
		inviteePeerID, groupID,
	).Scan(&wb, &groupType, &categoryID, &sourcePeerID, &anchorEpoch, &anchorHistoryHash); err != nil {
		return nil, "", "", "", 0, nil, err
	}
	return wb, normalizeGroupType(groupType), strings.TrimSpace(categoryID), strings.TrimSpace(sourcePeerID), anchorEpoch, anchorHistoryHash, nil
}

// StoredWelcome is one replicated Welcome object retained locally.
type StoredWelcome struct {
	InviteePeerID     string
	GroupID           string
	GroupType         string
	CategoryID        string
	WelcomeBytes      []byte
	SourcePeerID      string
	AnchorEpoch       uint64
	AnchorHistoryHash []byte
	CreatedAt         int64
}

// ListStoredWelcomesFor returns all stored Welcome replicas for an invitee.
func (d *Database) ListStoredWelcomesFor(inviteePeerID string) ([]StoredWelcome, error) {
	rows, err := d.Conn.Query(
		`SELECT invitee_peer_id, group_id, group_type, category_id, welcome_bytes, source_peer_id, anchor_epoch, anchor_history_hash, created_at
		 FROM stored_welcomes
		 WHERE invitee_peer_id = ?
		 ORDER BY created_at DESC`,
		inviteePeerID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListStoredWelcomesFor: %w", err)
	}
	defer rows.Close()

	var out []StoredWelcome
	for rows.Next() {
		var rec StoredWelcome
		if err := rows.Scan(&rec.InviteePeerID, &rec.GroupID, &rec.GroupType, &rec.CategoryID, &rec.WelcomeBytes, &rec.SourcePeerID, &rec.AnchorEpoch, &rec.AnchorHistoryHash, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("ListStoredWelcomesFor scan: %w", err)
		}
		rec.GroupType = normalizeGroupType(rec.GroupType)
		rec.CategoryID = strings.TrimSpace(rec.CategoryID)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// GetGroupInviteCreatorHint returns a peer ID likely to be the group creator
// for invite-request forwarding. Order:
//  1. stored_welcomes.source_peer_id (welcome replicator / inviter)
//  2. pending_invites.source_peer_id
//  3. pending_invites.inviter_peer_id (same as source on newer rows; helps
//     when source_peer_id column was empty in legacy data)
//
// Joined members often lack role=creator in group_members; these hints are the
// reliable path for RequestGroupInvite on a member node.
func (d *Database) GetGroupInviteCreatorHint(groupID string) (string, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return "", sql.ErrNoRows
	}
	var src string
	err := d.Conn.QueryRow(
		`SELECT source_peer_id FROM stored_welcomes
		 WHERE group_id = ? AND trim(source_peer_id) != ''
		 ORDER BY created_at DESC LIMIT 1`,
		groupID,
	).Scan(&src)
	if err == nil && strings.TrimSpace(src) != "" {
		return strings.TrimSpace(src), nil
	}
	err = d.Conn.QueryRow(
		`SELECT source_peer_id FROM pending_invites
		 WHERE group_id = ? AND trim(source_peer_id) != ''
		 ORDER BY received_at DESC LIMIT 1`,
		groupID,
	).Scan(&src)
	if err == nil && strings.TrimSpace(src) != "" {
		return strings.TrimSpace(src), nil
	}
	err = d.Conn.QueryRow(
		`SELECT inviter_peer_id FROM pending_invites
		 WHERE group_id = ? AND trim(inviter_peer_id) != ''
		 ORDER BY updated_at DESC, received_at DESC LIMIT 1`,
		groupID,
	).Scan(&src)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(src) == "" {
		return "", sql.ErrNoRows
	}
	return strings.TrimSpace(src), nil
}

// ── pending_invites ───────────────────────────────────────────────────────────

const (
	PendingInviteStatusPending  = "pending"
	PendingInviteStatusAccepted = "accepted"
	PendingInviteStatusRejected = "rejected"
	PendingInviteStatusInvalid  = "invalid"
	PendingInviteStatusExpired  = "expired"
)

// PendingInvite is an invitee-side pending Welcome lifecycle row.
type PendingInvite struct {
	ID            string
	GroupID       string
	GroupType     string
	CategoryID    string
	GroupName     string
	InviterPeerID string
	WelcomeHash   []byte
	WelcomeBytes  []byte
	SourcePeerID  string
	Status        string
	ReceivedAt    int64
	UpdatedAt     int64
}

// PendingInviteID deterministically identifies one Welcome for one group.
func PendingInviteID(groupID string, welcomeBytes []byte) string {
	h := sha256.New()
	h.Write([]byte(groupID))
	h.Write([]byte{0})
	h.Write(welcomeBytes)
	return hex.EncodeToString(h.Sum(nil))
}

func pendingInviteHash(welcomeBytes []byte) []byte {
	sum := sha256.Sum256(welcomeBytes)
	return sum[:]
}

// SavePendingInvite upserts a local pending invite without overwriting terminal states.
func (d *Database) SavePendingInvite(inv *PendingInvite) error {
	if inv == nil {
		return fmt.Errorf("SavePendingInvite: invite is nil")
	}
	if strings.TrimSpace(inv.GroupID) == "" {
		return fmt.Errorf("SavePendingInvite: group_id is required")
	}
	if len(inv.WelcomeBytes) == 0 {
		return fmt.Errorf("SavePendingInvite: welcome_bytes is required")
	}
	now := time.Now().Unix()
	id := strings.TrimSpace(inv.ID)
	if id == "" {
		id = PendingInviteID(inv.GroupID, inv.WelcomeBytes)
	}
	status := strings.TrimSpace(inv.Status)
	if status == "" {
		status = PendingInviteStatusPending
	}
	receivedAt := inv.ReceivedAt
	if receivedAt == 0 {
		receivedAt = now
	}
	welcomeHash := append([]byte(nil), inv.WelcomeHash...)
	if len(welcomeHash) == 0 {
		welcomeHash = pendingInviteHash(inv.WelcomeBytes)
	}

	_, err := d.Conn.Exec(
		`INSERT INTO pending_invites
		 (id, group_id, group_type, category_id, group_name, inviter_peer_id, welcome_hash, welcome_bytes, source_peer_id, status, received_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_id, welcome_hash) DO UPDATE SET
		   group_type = CASE
		     WHEN excluded.group_type IN ('channel', 'group', 'dm') THEN excluded.group_type
		     ELSE pending_invites.group_type
		   END,
		   category_id = CASE
		     WHEN excluded.category_id <> '' THEN excluded.category_id
		     ELSE pending_invites.category_id
		   END,
		   group_name = CASE
		     WHEN pending_invites.group_name = '' THEN excluded.group_name
		     ELSE pending_invites.group_name
		   END,
		   inviter_peer_id = CASE
		     WHEN pending_invites.inviter_peer_id = '' THEN excluded.inviter_peer_id
		     ELSE pending_invites.inviter_peer_id
		   END,
		   welcome_bytes = excluded.welcome_bytes,
		   source_peer_id = CASE
		     WHEN excluded.source_peer_id != '' THEN excluded.source_peer_id
		     ELSE pending_invites.source_peer_id
		   END,
		   status = CASE
		     WHEN pending_invites.status IN ('accepted', 'rejected') THEN pending_invites.status
		     ELSE excluded.status
		   END,
		   updated_at = excluded.updated_at`,
		id,
		inv.GroupID,
		normalizeGroupType(inv.GroupType),
		strings.TrimSpace(inv.CategoryID),
		inv.GroupName,
		inv.InviterPeerID,
		welcomeHash,
		inv.WelcomeBytes,
		inv.SourcePeerID,
		status,
		receivedAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("SavePendingInvite: %w", err)
	}
	return nil
}

// ListPendingInvites returns local invite rows ordered newest first.
func (d *Database) ListPendingInvites(includeTerminal bool) ([]PendingInvite, error) {
	query := `SELECT id, group_id, group_type, category_id, group_name, inviter_peer_id, welcome_hash, welcome_bytes, source_peer_id, status, received_at, updated_at
	          FROM pending_invites`
	if !includeTerminal {
		query += ` WHERE status = 'pending'`
	}
	query += ` ORDER BY received_at DESC, updated_at DESC`

	rows, err := d.Conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("ListPendingInvites: %w", err)
	}
	defer rows.Close()

	var out []PendingInvite
	for rows.Next() {
		var inv PendingInvite
		if err := rows.Scan(
			&inv.ID,
			&inv.GroupID,
			&inv.GroupType,
			&inv.CategoryID,
			&inv.GroupName,
			&inv.InviterPeerID,
			&inv.WelcomeHash,
			&inv.WelcomeBytes,
			&inv.SourcePeerID,
			&inv.Status,
			&inv.ReceivedAt,
			&inv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("ListPendingInvites scan: %w", err)
		}
		inv.GroupType = normalizeGroupType(inv.GroupType)
		inv.CategoryID = strings.TrimSpace(inv.CategoryID)
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (d *Database) GetPendingInvite(id string) (*PendingInvite, error) {
	var inv PendingInvite
	err := d.Conn.QueryRow(
		`SELECT id, group_id, group_type, category_id, group_name, inviter_peer_id, welcome_hash, welcome_bytes, source_peer_id, status, received_at, updated_at
		 FROM pending_invites
		 WHERE id = ?`,
		id,
	).Scan(
		&inv.ID,
		&inv.GroupID,
		&inv.GroupType,
		&inv.CategoryID,
		&inv.GroupName,
		&inv.InviterPeerID,
		&inv.WelcomeHash,
		&inv.WelcomeBytes,
		&inv.SourcePeerID,
		&inv.Status,
		&inv.ReceivedAt,
		&inv.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	inv.GroupType = normalizeGroupType(inv.GroupType)
	inv.CategoryID = strings.TrimSpace(inv.CategoryID)
	return &inv, nil
}

func (d *Database) GetLatestPendingInviteByGroup(groupID string) (*PendingInvite, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, sql.ErrNoRows
	}
	var inv PendingInvite
	err := d.Conn.QueryRow(
		`SELECT id, group_id, group_type, category_id, group_name, inviter_peer_id, welcome_hash, welcome_bytes, source_peer_id, status, received_at, updated_at
		 FROM pending_invites
		 WHERE group_id = ?
		 ORDER BY updated_at DESC, received_at DESC
		 LIMIT 1`,
		groupID,
	).Scan(
		&inv.ID,
		&inv.GroupID,
		&inv.GroupType,
		&inv.CategoryID,
		&inv.GroupName,
		&inv.InviterPeerID,
		&inv.WelcomeHash,
		&inv.WelcomeBytes,
		&inv.SourcePeerID,
		&inv.Status,
		&inv.ReceivedAt,
		&inv.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	inv.GroupType = normalizeGroupType(inv.GroupType)
	inv.CategoryID = strings.TrimSpace(inv.CategoryID)
	return &inv, nil
}

func (d *Database) MarkPendingInviteAccepted(id string) error {
	return d.markPendingInviteStatus(id, PendingInviteStatusAccepted)
}

func (d *Database) MarkPendingInviteRejected(id string) error {
	return d.markPendingInviteStatus(id, PendingInviteStatusRejected)
}

func (d *Database) markPendingInviteStatus(id, status string) error {
	res, err := d.Conn.Exec(
		`UPDATE pending_invites
		 SET status = ?, updated_at = ?
		 WHERE id = ?`,
		status,
		time.Now().Unix(),
		id,
	)
	if err != nil {
		return fmt.Errorf("markPendingInviteStatus: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("markPendingInviteStatus rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *Database) DeletePendingInvite(id string) error {
	res, err := d.Conn.Exec(`DELETE FROM pending_invites WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("DeletePendingInvite: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("DeletePendingInvite rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *Database) ReopenRejectedInvite(inv *PendingInvite) (string, error) {
	if inv == nil {
		return "", fmt.Errorf("ReopenRejectedInvite: invite is nil")
	}
	if strings.TrimSpace(inv.GroupID) == "" || len(inv.WelcomeBytes) == 0 {
		return "", fmt.Errorf("ReopenRejectedInvite: group_id and welcome_bytes are required")
	}
	now := time.Now().Unix()
	welcomeHash := append([]byte(nil), inv.WelcomeHash...)
	if len(welcomeHash) == 0 {
		welcomeHash = pendingInviteHash(inv.WelcomeBytes)
	}
	newID := strings.TrimSpace(inv.ID)
	if newID == "" {
		newID = PendingInviteID(inv.GroupID, inv.WelcomeBytes)
	}
	res, err := d.Conn.Exec(
		`UPDATE pending_invites
		 SET id = ?,
		     group_type = ?,
		     category_id = CASE WHEN ? != '' THEN ? ELSE category_id END,
		     group_name = CASE WHEN ? != '' THEN ? ELSE group_name END,
		     inviter_peer_id = CASE WHEN ? != '' THEN ? ELSE inviter_peer_id END,
		     welcome_hash = ?,
		     welcome_bytes = ?,
		     source_peer_id = CASE WHEN ? != '' THEN ? ELSE source_peer_id END,
		     status = ?,
		     received_at = ?,
		     updated_at = ?
		 WHERE group_id = ? AND status = ?
		   AND updated_at = (
		     SELECT MAX(updated_at) FROM pending_invites WHERE group_id = ? AND status = ?
		   )`,
		newID,
		normalizeGroupType(inv.GroupType),
		strings.TrimSpace(inv.CategoryID), strings.TrimSpace(inv.CategoryID),
		strings.TrimSpace(inv.GroupName), strings.TrimSpace(inv.GroupName),
		strings.TrimSpace(inv.InviterPeerID), strings.TrimSpace(inv.InviterPeerID),
		welcomeHash,
		inv.WelcomeBytes,
		strings.TrimSpace(inv.SourcePeerID), strings.TrimSpace(inv.SourcePeerID),
		PendingInviteStatusPending,
		now,
		now,
		strings.TrimSpace(inv.GroupID),
		PendingInviteStatusRejected,
		strings.TrimSpace(inv.GroupID),
		PendingInviteStatusRejected,
	)
	if err != nil {
		return "", fmt.Errorf("ReopenRejectedInvite: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("ReopenRejectedInvite rows: %w", err)
	}
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return newID, nil
}
