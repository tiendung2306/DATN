package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

// SQLiteCoordinationStorage implements coordination.CoordinationStorage
// backed by the existing SQLite database.
type SQLiteCoordinationStorage struct {
	db *Database
}

// NewSQLiteCoordinationStorage wraps the application database.
func NewSQLiteCoordinationStorage(d *Database) *SQLiteCoordinationStorage {
	return &SQLiteCoordinationStorage{db: d}
}

// ── GroupRecord ──────────────────────────────────────────────────────────────

func (s *SQLiteCoordinationStorage) GetGroupRecord(groupID string) (*coordination.GroupRecord, error) {
	var rec coordination.GroupRecord
	var role string
	var createdAt, updatedAt string
	err := s.db.Conn.QueryRow(
		`SELECT group_id, group_state, epoch, tree_hash, my_role, created_at, updated_at
		 FROM mls_groups WHERE group_id = ?`, groupID,
	).Scan(&rec.GroupID, &rec.GroupState, &rec.Epoch, &rec.TreeHash, &role, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, coordination.ErrGroupNotFound
		}
		return nil, fmt.Errorf("GetGroupRecord(%q): %w", groupID, err)
	}
	rec.MyRole = coordination.GroupRole(role)
	rec.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	rec.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
	return &rec, nil
}

func (s *SQLiteCoordinationStorage) SaveGroupRecord(rec *coordination.GroupRecord) error {
	_, err := s.db.Conn.Exec(
		`INSERT INTO mls_groups (group_id, group_state, epoch, tree_hash, my_role, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_id) DO UPDATE SET
		     group_state = excluded.group_state,
		     epoch       = excluded.epoch,
		     tree_hash   = excluded.tree_hash,
		     my_role     = COALESCE(NULLIF(excluded.my_role, ''), mls_groups.my_role),
		     updated_at  = excluded.updated_at`,
		rec.GroupID, rec.GroupState, rec.Epoch, rec.TreeHash,
		string(rec.MyRole), rec.CreatedAt.Format(time.DateTime), rec.UpdatedAt.Format(time.DateTime),
	)
	if err != nil {
		return fmt.Errorf("SaveGroupRecord(%q): %w", rec.GroupID, err)
	}
	return nil
}

func (s *SQLiteCoordinationStorage) ListGroups() ([]*coordination.GroupRecord, error) {
	rows, err := s.db.Conn.Query(
		`SELECT group_id, group_state, epoch, tree_hash, my_role, created_at, updated_at FROM mls_groups`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListGroups: %w", err)
	}
	defer rows.Close()

	var groups []*coordination.GroupRecord
	for rows.Next() {
		var rec coordination.GroupRecord
		var role, createdAt, updatedAt string
		if err := rows.Scan(&rec.GroupID, &rec.GroupState, &rec.Epoch, &rec.TreeHash, &role, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListGroups scan: %w", err)
		}
		rec.MyRole = coordination.GroupRole(role)
		rec.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		rec.UpdatedAt, _ = time.Parse(time.DateTime, updatedAt)
		groups = append(groups, &rec)
	}
	return groups, rows.Err()
}

// ── CoordState ───────────────────────────────────────────────────────────────

func (s *SQLiteCoordinationStorage) GetCoordState(groupID string) (*coordination.CoordState, error) {
	var (
		state        coordination.CoordState
		viewJSON     string
		tokenHolder  string
		commitAtStr  sql.NullString
		proposalJSON string
	)
	err := s.db.Conn.QueryRow(
		`SELECT group_id, active_view, token_holder, last_commit_hash, last_commit_at, pending_proposals
		 FROM coordination_state WHERE group_id = ?`, groupID,
	).Scan(&state.GroupID, &viewJSON, &tokenHolder, &state.LastCommitHash, &commitAtStr, &proposalJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, coordination.ErrGroupNotFound
		}
		return nil, fmt.Errorf("GetCoordState(%q): %w", groupID, err)
	}

	state.TokenHolder = peer.ID(tokenHolder)

	var peerIDs []string
	if err := json.Unmarshal([]byte(viewJSON), &peerIDs); err == nil {
		for _, id := range peerIDs {
			state.ActiveView = append(state.ActiveView, peer.ID(id))
		}
	}

	if commitAtStr.Valid {
		state.LastCommitAt, _ = time.Parse(time.DateTime, commitAtStr.String)
	}

	var proposals [][]byte
	if err := json.Unmarshal([]byte(proposalJSON), &proposals); err == nil {
		state.PendingProposals = proposals
	}

	return &state, nil
}

func (s *SQLiteCoordinationStorage) SaveCoordState(state *coordination.CoordState) error {
	peerIDs := make([]string, len(state.ActiveView))
	for i, pid := range state.ActiveView {
		peerIDs[i] = string(pid)
	}
	viewJSON, _ := json.Marshal(peerIDs)
	proposalJSON, _ := json.Marshal(state.PendingProposals)

	var commitAtStr *string
	if !state.LastCommitAt.IsZero() {
		s := state.LastCommitAt.Format(time.DateTime)
		commitAtStr = &s
	}

	_, err := s.db.Conn.Exec(
		`INSERT INTO coordination_state (group_id, active_view, token_holder, last_commit_hash, last_commit_at, pending_proposals)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_id) DO UPDATE SET
		     active_view       = excluded.active_view,
		     token_holder      = excluded.token_holder,
		     last_commit_hash  = excluded.last_commit_hash,
		     last_commit_at    = excluded.last_commit_at,
		     pending_proposals = excluded.pending_proposals`,
		state.GroupID, string(viewJSON), string(state.TokenHolder),
		state.LastCommitHash, commitAtStr, string(proposalJSON),
	)
	if err != nil {
		return fmt.Errorf("SaveCoordState(%q): %w", state.GroupID, err)
	}
	return nil
}

// ── StoredMessage ────────────────────────────────────────────────────────────

func (s *SQLiteCoordinationStorage) SaveMessage(msg *coordination.StoredMessage) error {
	_, err := s.db.Conn.Exec(
		`INSERT INTO stored_messages (group_id, epoch, sender_id, content, hlc_wall_time_ms, hlc_counter, hlc_node_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.GroupID, msg.Epoch, string(msg.SenderID), msg.Content,
		msg.Timestamp.WallTimeMs, msg.Timestamp.Counter, msg.Timestamp.NodeID,
	)
	if err != nil {
		return fmt.Errorf("SaveMessage: %w", err)
	}
	return nil
}

func (s *SQLiteCoordinationStorage) GetMessagesSince(groupID string, after coordination.HLCTimestamp) ([]*coordination.StoredMessage, error) {
	rows, err := s.db.Conn.Query(
		`SELECT group_id, epoch, sender_id, content, hlc_wall_time_ms, hlc_counter, hlc_node_id
		 FROM stored_messages
		 WHERE group_id = ?
		   AND (hlc_wall_time_ms > ?
		        OR (hlc_wall_time_ms = ? AND hlc_counter > ?)
		        OR (hlc_wall_time_ms = ? AND hlc_counter = ? AND hlc_node_id > ?))
		 ORDER BY hlc_wall_time_ms ASC, hlc_counter ASC, hlc_node_id ASC`,
		groupID,
		after.WallTimeMs,
		after.WallTimeMs, after.Counter,
		after.WallTimeMs, after.Counter, after.NodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetMessagesSince: %w", err)
	}
	defer rows.Close()

	var msgs []*coordination.StoredMessage
	for rows.Next() {
		var m coordination.StoredMessage
		var senderID string
		if err := rows.Scan(&m.GroupID, &m.Epoch, &senderID, &m.Content,
			&m.Timestamp.WallTimeMs, &m.Timestamp.Counter, &m.Timestamp.NodeID); err != nil {
			return nil, fmt.Errorf("GetMessagesSince scan: %w", err)
		}
		m.SenderID = peer.ID(senderID)
		msgs = append(msgs, &m)
	}
	return msgs, rows.Err()
}
