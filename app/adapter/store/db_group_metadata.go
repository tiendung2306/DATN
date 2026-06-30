package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	GroupLifecycleActive = "active"
	GroupLifecycleLeft   = "left"
)

// GetGroupChatAvatarMeta returns stored avatar hash/mime for a group chat row (local UI).
func (d *Database) GetGroupChatAvatarMeta(groupID string) (hash string, mime string, updatedAt int64, err error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return "", "", 0, fmt.Errorf("group id required")
	}
	var h, m sql.NullString
	var at sql.NullInt64
	err = d.Conn.QueryRow(
		`SELECT group_avatar_hash, group_avatar_mime, group_avatar_updated_at FROM mls_groups WHERE group_id = ?`,
		groupID,
	).Scan(&h, &m, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, sql.ErrNoRows
	}
	if err != nil {
		return "", "", 0, fmt.Errorf("GetGroupChatAvatarMeta(%q): %w", groupID, err)
	}
	if h.Valid {
		hash = strings.TrimSpace(h.String)
	}
	if m.Valid {
		mime = strings.TrimSpace(m.String)
	}
	if at.Valid {
		updatedAt = at.Int64
	}
	return hash, mime, updatedAt, nil
}

// SetGroupChatAvatar persists avatar blob reference on mls_groups (group-type chats only at call site).
func (d *Database) SetGroupChatAvatar(groupID, hash, mime string, updatedAtUnix int64) error {
	groupID = strings.TrimSpace(groupID)
	hash = strings.TrimSpace(strings.ToLower(hash))
	mime = strings.TrimSpace(mime)
	if groupID == "" {
		return fmt.Errorf("group id required")
	}
	if hash == "" || mime == "" {
		return fmt.Errorf("avatar hash and mime are required")
	}
	_, err := d.Conn.Exec(
		`UPDATE mls_groups SET group_avatar_hash = ?, group_avatar_mime = ?, group_avatar_updated_at = ?, updated_at = CURRENT_TIMESTAMP WHERE group_id = ?`,
		hash, mime, updatedAtUnix, groupID,
	)
	if err != nil {
		return fmt.Errorf("SetGroupChatAvatar(%q): %w", groupID, err)
	}
	return nil
}

// SetGroupCreatorPeerID stores the authoritative creator peer id for a group.
// The value is write-once per row (first non-empty value wins) to avoid local
// cache churn overriding the invariant.
func (d *Database) SetGroupCreatorPeerID(groupID, creatorPeerID string) error {
	groupID = strings.TrimSpace(groupID)
	creatorPeerID = strings.TrimSpace(creatorPeerID)
	if groupID == "" {
		return fmt.Errorf("group id required")
	}
	if creatorPeerID == "" {
		return fmt.Errorf("creator peer id required")
	}
	_, err := d.Conn.Exec(
		`UPDATE mls_groups
		 SET group_creator_peer_id = CASE
		   WHEN trim(group_creator_peer_id) = '' THEN ?
		   ELSE group_creator_peer_id
		 END,
		     updated_at = CURRENT_TIMESTAMP
		 WHERE group_id = ?`,
		creatorPeerID, groupID,
	)
	if err != nil {
		return fmt.Errorf("SetGroupCreatorPeerID(%q): %w", groupID, err)
	}
	return nil
}

// SetDMCounterpartyPeerID stores the product-level intended peer for a DM.
// This is deliberately separate from group_members, which tracks confirmed
// MLS membership and must not be faked before AddMembers commits.
func (d *Database) SetDMCounterpartyPeerID(groupID, peerID string) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" {
		return fmt.Errorf("group id required")
	}
	if peerID == "" {
		return fmt.Errorf("counterparty peer id required")
	}
	res, err := d.Conn.Exec(
		`UPDATE mls_groups
		    SET dm_counterparty_peer_id = ?,
		        updated_at = CURRENT_TIMESTAMP
		  WHERE group_id = ? AND lower(group_type) = 'dm'`,
		peerID, groupID,
	)
	if err != nil {
		return fmt.Errorf("SetDMCounterpartyPeerID(%q): %w", groupID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("SetDMCounterpartyPeerID rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetGroupCreatorPeerID returns the authoritative creator peer id for a group
// if known. Empty string means unknown/unset.
func (d *Database) GetGroupCreatorPeerID(groupID string) (string, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return "", fmt.Errorf("group id required")
	}
	var creator sql.NullString
	err := d.Conn.QueryRow(
		`SELECT group_creator_peer_id FROM mls_groups WHERE group_id = ?`,
		groupID,
	).Scan(&creator)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", sql.ErrNoRows
		}
		return "", fmt.Errorf("GetGroupCreatorPeerID(%q): %w", groupID, err)
	}
	if !creator.Valid {
		return "", nil
	}
	return strings.TrimSpace(creator.String), nil
}

// ClearGroupChatAvatar removes the group avatar reference from mls_groups.
func (d *Database) ClearGroupChatAvatar(groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group id required")
	}
	_, err := d.Conn.Exec(
		`UPDATE mls_groups SET group_avatar_hash = '', group_avatar_mime = '', group_avatar_updated_at = 0, updated_at = CURRENT_TIMESTAMP WHERE group_id = ?`,
		groupID,
	)
	if err != nil {
		return fmt.Errorf("ClearGroupChatAvatar(%q): %w", groupID, err)
	}
	return nil
}

func (d *Database) HasGroup(groupID string) (bool, error) {
	var one int
	err := d.Conn.QueryRow(`SELECT 1 FROM mls_groups WHERE group_id = ?`, groupID).Scan(&one)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("HasGroup(%q): %w", groupID, err)
}

// ListJoinedGroupChatIDsForReplication returns group_id values for active group-type chats (replica pull keys).
func (d *Database) ListJoinedGroupChatIDsForReplication(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 256
	}
	if limit > 512 {
		limit = 512
	}
	rows, err := d.Conn.Query(
		`SELECT group_id FROM mls_groups
		 WHERE group_type = 'group' AND lifecycle_status = ?
		 ORDER BY updated_at DESC LIMIT ?`,
		GroupLifecycleActive, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ListJoinedGroupChatIDsForReplication: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var gid string
		if err := rows.Scan(&gid); err != nil {
			return nil, fmt.Errorf("ListJoinedGroupChatIDsForReplication scan: %w", err)
		}
		gid = strings.TrimSpace(gid)
		if gid != "" {
			out = append(out, gid)
		}
	}
	return out, rows.Err()
}

// MarkGroupLeft marks a group as locally left while preserving state/history.
func (d *Database) MarkGroupLeft(groupID string) error {
	exists, err := d.HasGroup(groupID)
	if err != nil {
		return err
	}
	if !exists {
		return sql.ErrNoRows
	}
	_, err = d.Conn.Exec(
		`UPDATE mls_groups
		 SET lifecycle_status = ?, left_at = CASE WHEN left_at = 0 THEN ? ELSE left_at END, updated_at = CURRENT_TIMESTAMP
		 WHERE group_id = ?`,
		GroupLifecycleLeft,
		time.Now().Unix(),
		groupID,
	)
	if err != nil {
		return fmt.Errorf("MarkGroupLeft(%q): %w", groupID, err)
	}
	return nil
}

func (d *Database) IsGroupActive(groupID string) (bool, error) {
	var status string
	err := d.Conn.QueryRow(`SELECT lifecycle_status FROM mls_groups WHERE group_id = ?`, groupID).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, sql.ErrNoRows
		}
		return false, fmt.Errorf("IsGroupActive(%q): %w", groupID, err)
	}
	return status == "" || status == GroupLifecycleActive, nil
}

// PurgeGroupMetadata completely purges cryptographic/coordination metadata of a group to prepare for recreation/restart.
// It MUST NOT delete any entries from stored_messages to preserve chat history.
func (d *Database) PurgeGroupMetadata(groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("PurgeGroupMetadata: group ID is required")
	}

	tx, err := d.Conn.Begin()
	if err != nil {
		return fmt.Errorf("PurgeGroupMetadata begin tx: %w", err)
	}
	defer tx.Rollback()

	tables := []string{
		"mls_groups",
		"group_members",
		"coordination_state",
		"group_add_operations",
		"pending_welcomes_out",
		"stored_welcomes",
		"pending_invites",
		"applied_welcomes",
		"applied_envelopes",
		"envelope_log",
		"envelope_dedup",
		"group_event_log",
		"fork_heal_events",
		"fork_audit",
		"group_invite_requests",
	}

	for _, table := range tables {
		_, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE group_id = ?", table), groupID)
		if err != nil {
			return fmt.Errorf("PurgeGroupMetadata delete from %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("PurgeGroupMetadata commit: %w", err)
	}
	return nil
}
