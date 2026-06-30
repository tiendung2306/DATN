package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	GroupMemberStatusActive = "active"
	GroupMemberStatusLeft   = "left"

	GroupMemberRoleCreator = "creator"
	GroupMemberRoleAdmin   = "admin"
	GroupMemberRoleMember  = "member"
)

type GroupMemberRecord struct {
	GroupID     string
	PeerID      string
	DisplayName string
	Role        string
	Status      string
	Source      string
	JoinedAt    int64
	LeftAt      int64
	UpdatedAt   int64
}

func normalizeGroupMemberStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case GroupMemberStatusLeft:
		return GroupMemberStatusLeft
	default:
		return GroupMemberStatusActive
	}
}

func normalizeGroupMemberRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case GroupMemberRoleCreator:
		return GroupMemberRoleCreator
	case GroupMemberRoleAdmin:
		return GroupMemberRoleAdmin
	default:
		return GroupMemberRoleMember
	}
}

func (d *Database) UpsertGroupMember(rec GroupMemberRecord) error {
	rec.GroupID = strings.TrimSpace(rec.GroupID)
	rec.PeerID = strings.TrimSpace(rec.PeerID)
	if rec.GroupID == "" || rec.PeerID == "" {
		return fmt.Errorf("UpsertGroupMember: group_id and peer_id are required")
	}
	rec.Status = normalizeGroupMemberStatus(rec.Status)
	rec.Role = normalizeGroupMemberRole(rec.Role)
	now := time.Now().Unix()
	if rec.JoinedAt == 0 {
		rec.JoinedAt = now
	}
	_, err := d.Conn.Exec(
		`INSERT INTO group_members (group_id, peer_id, display_name, role, status, source, joined_at, left_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_id, peer_id) DO UPDATE SET
		   display_name = CASE WHEN excluded.display_name <> '' THEN excluded.display_name ELSE group_members.display_name END,
		   role = CASE
		     WHEN group_members.role = 'creator' THEN group_members.role
		     WHEN excluded.role = 'creator' THEN excluded.role
		     ELSE excluded.role
		   END,
		   status = excluded.status,
		   source = CASE WHEN excluded.source <> '' THEN excluded.source ELSE group_members.source END,
		   joined_at = CASE WHEN group_members.joined_at = 0 THEN excluded.joined_at ELSE group_members.joined_at END,
		   left_at = CASE WHEN excluded.status = 'left' THEN excluded.left_at ELSE group_members.left_at END,
		   updated_at = excluded.updated_at`,
		rec.GroupID, rec.PeerID, rec.DisplayName, rec.Role, rec.Status, rec.Source, rec.JoinedAt, rec.LeftAt, now,
	)
	if err != nil {
		return fmt.Errorf("UpsertGroupMember: %w", err)
	}
	return nil
}

// UpsertGroupMemberPreservingRole is like UpsertGroupMember but never changes
// the role of an existing row. Use this when the caller cannot authoritatively
// determine the member's role (e.g. heartbeat-driven roster sync).
//
// Use UpsertGroupMember (not this one) only at sites that authoritatively
// know the role — currently only CreateGroupChat ("creator").
func (d *Database) UpsertGroupMemberPreservingRole(rec GroupMemberRecord) error {
	rec.GroupID = strings.TrimSpace(rec.GroupID)
	rec.PeerID = strings.TrimSpace(rec.PeerID)
	if rec.GroupID == "" || rec.PeerID == "" {
		return fmt.Errorf("UpsertGroupMemberPreservingRole: group_id and peer_id are required")
	}
	rec.Status = normalizeGroupMemberStatus(rec.Status)
	rec.Role = normalizeGroupMemberRole(rec.Role)
	now := time.Now().Unix()
	if rec.JoinedAt == 0 {
		rec.JoinedAt = now
	}
	_, err := d.Conn.Exec(
		`INSERT INTO group_members (group_id, peer_id, display_name, role, status, source, joined_at, left_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_id, peer_id) DO UPDATE SET
		   display_name = CASE WHEN excluded.display_name <> '' THEN excluded.display_name ELSE group_members.display_name END,
		   role = group_members.role,
		   status = excluded.status,
		   source = CASE WHEN excluded.source <> '' THEN excluded.source ELSE group_members.source END,
		   joined_at = CASE WHEN group_members.joined_at = 0 THEN excluded.joined_at ELSE group_members.joined_at END,
		   left_at = CASE WHEN excluded.status = 'left' THEN excluded.left_at ELSE group_members.left_at END,
		   updated_at = excluded.updated_at`,
		rec.GroupID, rec.PeerID, rec.DisplayName, rec.Role, rec.Status, rec.Source, rec.JoinedAt, rec.LeftAt, now,
	)
	if err != nil {
		return fmt.Errorf("UpsertGroupMemberPreservingRole: %w", err)
	}
	return nil
}

func (d *Database) ListGroupMembers(groupID string, statuses ...string) ([]GroupMemberRecord, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, fmt.Errorf("ListGroupMembers: group_id is required")
	}
	query := `SELECT group_id, peer_id, display_name, role, status, source, joined_at, left_at, updated_at
		   FROM group_members
		  WHERE group_id = ?`
	args := []interface{}{groupID}
	if len(statuses) > 0 {
		holders := make([]string, 0, len(statuses))
		for _, s := range statuses {
			n := normalizeGroupMemberStatus(s)
			if n != "" {
				holders = append(holders, "?")
				args = append(args, n)
			}
		}
		if len(holders) > 0 {
			query += ` AND status IN (` + strings.Join(holders, ",") + `)`
		}
	}
	query += ` ORDER BY joined_at ASC, peer_id ASC`
	rows, err := d.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListGroupMembers: %w", err)
	}
	defer rows.Close()
	out := make([]GroupMemberRecord, 0)
	for rows.Next() {
		var rec GroupMemberRecord
		if err := rows.Scan(&rec.GroupID, &rec.PeerID, &rec.DisplayName, &rec.Role, &rec.Status, &rec.Source, &rec.JoinedAt, &rec.LeftAt, &rec.UpdatedAt); err != nil {
			return nil, fmt.Errorf("ListGroupMembers scan: %w", err)
		}
		rec.Status = normalizeGroupMemberStatus(rec.Status)
		rec.Role = normalizeGroupMemberRole(rec.Role)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (d *Database) GetGroupMember(groupID, peerID string) (*GroupMemberRecord, error) {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" || peerID == "" {
		return nil, fmt.Errorf("GetGroupMember: group_id and peer_id are required")
	}
	var rec GroupMemberRecord
	err := d.Conn.QueryRow(
		`SELECT group_id, peer_id, display_name, role, status, source, joined_at, left_at, updated_at
		 FROM group_members
		 WHERE group_id = ? AND peer_id = ?`,
		groupID, peerID,
	).Scan(&rec.GroupID, &rec.PeerID, &rec.DisplayName, &rec.Role, &rec.Status, &rec.Source, &rec.JoinedAt, &rec.LeftAt, &rec.UpdatedAt)
	if err != nil {
		return nil, err
	}
	rec.Status = normalizeGroupMemberStatus(rec.Status)
	rec.Role = normalizeGroupMemberRole(rec.Role)
	return &rec, nil
}

// SetGroupMemberRole updates an active member's role. The creator role is
// immutable: callers may not promote someone to creator or demote an existing
// creator through this path.
func (d *Database) SetGroupMemberRole(groupID, peerID, role string) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	role = normalizeGroupMemberRole(role)
	if groupID == "" || peerID == "" {
		return fmt.Errorf("SetGroupMemberRole: group_id and peer_id are required")
	}
	if role == GroupMemberRoleCreator {
		return fmt.Errorf("SetGroupMemberRole: creator role is immutable")
	}
	rec, err := d.GetGroupMember(groupID, peerID)
	if err != nil {
		return err
	}
	if rec == nil || rec.Status != GroupMemberStatusActive {
		return sql.ErrNoRows
	}
	if rec.Role == GroupMemberRoleCreator {
		return fmt.Errorf("SetGroupMemberRole: creator role is immutable")
	}
	res, err := d.Conn.Exec(
		`UPDATE group_members
		 SET role = ?, updated_at = ?
		 WHERE group_id = ? AND peer_id = ? AND status = ? AND role <> ?`,
		role, time.Now().Unix(), groupID, peerID, GroupMemberStatusActive, GroupMemberRoleCreator,
	)
	if err != nil {
		return fmt.Errorf("SetGroupMemberRole: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("SetGroupMemberRole rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *Database) ListGroupAdmins(groupID string) ([]GroupMemberRecord, error) {
	return d.listGroupMembersByRoles(groupID, []string{GroupMemberRoleCreator, GroupMemberRoleAdmin})
}

func (d *Database) ListAuthorizedCommitters(groupID string) ([]GroupMemberRecord, error) {
	return d.ListGroupAdmins(groupID)
}

func (d *Database) listGroupMembersByRoles(groupID string, roles []string) ([]GroupMemberRecord, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, fmt.Errorf("listGroupMembersByRoles: group_id is required")
	}
	filtered := make([]string, 0, len(roles))
	for _, role := range roles {
		n := normalizeGroupMemberRole(role)
		if n != "" {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	args := []interface{}{groupID, GroupMemberStatusActive}
	holders := make([]string, 0, len(filtered))
	for _, role := range filtered {
		holders = append(holders, "?")
		args = append(args, role)
	}
	rows, err := d.Conn.Query(
		`SELECT group_id, peer_id, display_name, role, status, source, joined_at, left_at, updated_at
		   FROM group_members
		  WHERE group_id = ? AND status = ? AND role IN (`+strings.Join(holders, ",")+`)
		  ORDER BY role ASC, joined_at ASC, peer_id ASC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("listGroupMembersByRoles: %w", err)
	}
	defer rows.Close()
	out := make([]GroupMemberRecord, 0)
	for rows.Next() {
		var rec GroupMemberRecord
		if err := rows.Scan(&rec.GroupID, &rec.PeerID, &rec.DisplayName, &rec.Role, &rec.Status, &rec.Source, &rec.JoinedAt, &rec.LeftAt, &rec.UpdatedAt); err != nil {
			return nil, fmt.Errorf("listGroupMembersByRoles scan: %w", err)
		}
		rec.Status = normalizeGroupMemberStatus(rec.Status)
		rec.Role = normalizeGroupMemberRole(rec.Role)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (d *Database) MarkGroupMemberLeft(groupID, peerID string, leftAt int64) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" || peerID == "" {
		return fmt.Errorf("MarkGroupMemberLeft: group_id and peer_id are required")
	}
	if leftAt <= 0 {
		leftAt = time.Now().Unix()
	}
	res, err := d.Conn.Exec(
		`UPDATE group_members
		 SET status = ?, left_at = ?, updated_at = ?
		 WHERE group_id = ? AND peer_id = ?`,
		GroupMemberStatusLeft, leftAt, time.Now().Unix(), groupID, peerID,
	)
	if err != nil {
		return fmt.Errorf("MarkGroupMemberLeft: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("MarkGroupMemberLeft rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *Database) UpdateGroupMemberDisplayNameByPeer(peerID, displayName string) error {
	peerID = strings.TrimSpace(peerID)
	displayName = strings.TrimSpace(displayName)
	if peerID == "" || displayName == "" {
		return nil
	}
	_, err := d.Conn.Exec(
		`UPDATE group_members
		 SET display_name = ?, updated_at = ?
		 WHERE peer_id = ?`,
		displayName, time.Now().Unix(), peerID,
	)
	if err != nil {
		return fmt.Errorf("UpdateGroupMemberDisplayNameByPeer: %w", err)
	}
	return nil
}
