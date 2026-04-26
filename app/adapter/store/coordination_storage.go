package store

import (
	"crypto/sha256"
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
		`SELECT group_id, group_state, epoch, tree_hash, my_role, created_at, updated_at
		 FROM mls_groups
		 WHERE lifecycle_status = 'active'`,
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

	if tokenHolder != "" {
		if pid, err := peer.Decode(tokenHolder); err == nil {
			state.TokenHolder = pid
		} else {
			state.TokenHolder = peer.ID(tokenHolder)
		}
	}

	var peerIDs []string
	if err := json.Unmarshal([]byte(viewJSON), &peerIDs); err == nil {
		for _, id := range peerIDs {
			if pid, derr := peer.Decode(id); derr == nil {
				state.ActiveView = append(state.ActiveView, pid)
			} else {
				state.ActiveView = append(state.ActiveView, peer.ID(id))
			}
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
		peerIDs[i] = pid.String()
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
		state.GroupID, string(viewJSON), state.TokenHolder.String(),
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
		`INSERT OR IGNORE INTO stored_messages (
			group_id, epoch, sender_id, content, hlc_wall_time_ms, hlc_counter, hlc_node_id, envelope_hash
		 )
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.GroupID, msg.Epoch, msg.SenderID.String(), msg.Content,
		msg.Timestamp.WallTimeMs, msg.Timestamp.Counter, msg.Timestamp.NodeID, msg.EnvelopeHash,
	)
	if err != nil {
		return fmt.Errorf("SaveMessage: %w", err)
	}
	return nil
}

func (s *SQLiteCoordinationStorage) ApplyCommit(rec *coordination.GroupRecord, msgType coordination.MessageType, envelope []byte, ts coordination.HLCTimestamp) (bool, int64, error) {
	if rec == nil || rec.GroupID == "" || len(envelope) == 0 {
		return false, 0, fmt.Errorf("ApplyCommit: invalid input")
	}
	tx, err := s.db.Conn.Begin()
	if err != nil {
		return false, 0, fmt.Errorf("ApplyCommit begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	hash := sha256.Sum256(envelope)
	applied, err := hasAppliedEnvelopeTx(tx, rec.GroupID, hash[:])
	if err != nil {
		return false, 0, fmt.Errorf("ApplyCommit has-applied: %w", err)
	}
	if applied {
		if err := tx.Commit(); err != nil {
			return false, 0, fmt.Errorf("ApplyCommit commit-noop: %w", err)
		}
		return false, 0, nil
	}

	if err := saveGroupRecordTx(tx, rec); err != nil {
		return false, 0, fmt.Errorf("ApplyCommit save-group: %w", err)
	}
	if err := markEnvelopeAppliedTx(tx, rec.GroupID, msgType, rec.Epoch, hash[:]); err != nil {
		return false, 0, fmt.Errorf("ApplyCommit mark-applied: %w", err)
	}
	seq, err := appendEnvelopeTx(tx, rec.GroupID, msgType, rec.Epoch, ts, envelope)
	if err != nil {
		return false, 0, fmt.Errorf("ApplyCommit append-envelope: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, 0, fmt.Errorf("ApplyCommit commit: %w", err)
	}
	return true, seq, nil
}

func (s *SQLiteCoordinationStorage) ApplyApplication(rec *coordination.GroupRecord, msg *coordination.StoredMessage, msgType coordination.MessageType, envelope []byte, ts coordination.HLCTimestamp) (bool, int64, error) {
	if rec == nil || msg == nil || rec.GroupID == "" || msg.GroupID == "" || len(envelope) == 0 {
		return false, 0, fmt.Errorf("ApplyApplication: invalid input")
	}
	tx, err := s.db.Conn.Begin()
	if err != nil {
		return false, 0, fmt.Errorf("ApplyApplication begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	hash := sha256.Sum256(envelope)
	applied, err := hasAppliedEnvelopeTx(tx, rec.GroupID, hash[:])
	if err != nil {
		return false, 0, fmt.Errorf("ApplyApplication has-applied: %w", err)
	}
	if applied {
		if err := tx.Commit(); err != nil {
			return false, 0, fmt.Errorf("ApplyApplication commit-noop: %w", err)
		}
		return false, 0, nil
	}

	if err := saveGroupRecordTx(tx, rec); err != nil {
		return false, 0, fmt.Errorf("ApplyApplication save-group: %w", err)
	}
	if err := saveMessageTx(tx, msg); err != nil {
		return false, 0, fmt.Errorf("ApplyApplication save-message: %w", err)
	}
	if err := markEnvelopeAppliedTx(tx, rec.GroupID, msgType, rec.Epoch, hash[:]); err != nil {
		return false, 0, fmt.Errorf("ApplyApplication mark-applied: %w", err)
	}
	seq, err := appendEnvelopeTx(tx, rec.GroupID, msgType, rec.Epoch, ts, envelope)
	if err != nil {
		return false, 0, fmt.Errorf("ApplyApplication append-envelope: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, 0, fmt.Errorf("ApplyApplication commit: %w", err)
	}
	return true, seq, nil
}

func (s *SQLiteCoordinationStorage) HasAppliedEnvelope(groupID string, envelopeHash []byte) (bool, error) {
	if len(envelopeHash) == 0 {
		return false, nil
	}
	var exists int
	err := s.db.Conn.QueryRow(
		`SELECT 1 FROM applied_envelopes WHERE group_id = ? AND envelope_hash = ? LIMIT 1`,
		groupID, envelopeHash,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("HasAppliedEnvelope: %w", err)
	}
	return true, nil
}

func (s *SQLiteCoordinationStorage) MarkEnvelopeApplied(groupID string, msgType coordination.MessageType, epoch uint64, envelopeHash []byte) error {
	if len(envelopeHash) == 0 {
		return nil
	}
	_, err := s.db.Conn.Exec(
		`INSERT OR IGNORE INTO applied_envelopes (group_id, envelope_hash, msg_type, epoch, applied_at)
		 VALUES (?, ?, ?, ?, strftime('%s','now'))`,
		groupID, envelopeHash, string(msgType), epoch,
	)
	if err != nil {
		return fmt.Errorf("MarkEnvelopeApplied: %w", err)
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
		if pid, err := peer.Decode(senderID); err == nil {
			m.SenderID = pid
		} else {
			m.SenderID = peer.ID(senderID)
		}
		msgs = append(msgs, &m)
	}
	return msgs, rows.Err()
}

// ── Offline envelope log + ACKs ─────────────────────────────────────────────

func (s *SQLiteCoordinationStorage) AppendEnvelope(groupID string, msgType coordination.MessageType, epoch uint64, ts coordination.HLCTimestamp, envelope []byte) (int64, error) {
	tx, err := s.db.Conn.Begin()
	if err != nil {
		return 0, fmt.Errorf("AppendEnvelope begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	hash := sha256.Sum256(envelope)
	res, err := tx.Exec(
		`INSERT OR IGNORE INTO envelope_dedup (group_id, envelope_hash, created_at)
		 VALUES (?, ?, strftime('%s','now'))`,
		groupID, hash[:],
	)
	if err != nil {
		return 0, fmt.Errorf("AppendEnvelope dedup: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Duplicate envelope (same bytes) already stored for this group.
		return 0, nil
	}

	var next int64
	err = tx.QueryRow(
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM envelope_log WHERE group_id = ?`, groupID,
	).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("AppendEnvelope seq: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO envelope_log (group_id, seq, msg_type, epoch, envelope, hlc_wall_ms, hlc_counter, hlc_node_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		groupID, next, string(msgType), epoch, envelope,
		ts.WallTimeMs, ts.Counter, ts.NodeID,
	)
	if err != nil {
		return 0, fmt.Errorf("AppendEnvelope insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("AppendEnvelope commit: %w", err)
	}
	return next, nil
}

func hasAppliedEnvelopeTx(tx *sql.Tx, groupID string, envelopeHash []byte) (bool, error) {
	var exists int
	err := tx.QueryRow(
		`SELECT 1 FROM applied_envelopes WHERE group_id = ? AND envelope_hash = ? LIMIT 1`,
		groupID, envelopeHash,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func saveGroupRecordTx(tx *sql.Tx, rec *coordination.GroupRecord) error {
	_, err := tx.Exec(
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
	return err
}

func saveMessageTx(tx *sql.Tx, msg *coordination.StoredMessage) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO stored_messages (
			group_id, epoch, sender_id, content, hlc_wall_time_ms, hlc_counter, hlc_node_id, envelope_hash
		 )
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.GroupID, msg.Epoch, msg.SenderID.String(), msg.Content,
		msg.Timestamp.WallTimeMs, msg.Timestamp.Counter, msg.Timestamp.NodeID, msg.EnvelopeHash,
	)
	return err
}

func markEnvelopeAppliedTx(tx *sql.Tx, groupID string, msgType coordination.MessageType, epoch uint64, envelopeHash []byte) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO applied_envelopes (group_id, envelope_hash, msg_type, epoch, applied_at)
		 VALUES (?, ?, ?, ?, strftime('%s','now'))`,
		groupID, envelopeHash, string(msgType), epoch,
	)
	return err
}

func appendEnvelopeTx(tx *sql.Tx, groupID string, msgType coordination.MessageType, epoch uint64, ts coordination.HLCTimestamp, envelope []byte) (int64, error) {
	hash := sha256.Sum256(envelope)
	res, err := tx.Exec(
		`INSERT OR IGNORE INTO envelope_dedup (group_id, envelope_hash, created_at)
		 VALUES (?, ?, strftime('%s','now'))`,
		groupID, hash[:],
	)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, nil
	}

	var next int64
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM envelope_log WHERE group_id = ?`, groupID,
	).Scan(&next); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(
		`INSERT INTO envelope_log (group_id, seq, msg_type, epoch, envelope, hlc_wall_ms, hlc_counter, hlc_node_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		groupID, next, string(msgType), epoch, envelope, ts.WallTimeMs, ts.Counter, ts.NodeID,
	); err != nil {
		return 0, err
	}
	return next, nil
}

func (s *SQLiteCoordinationStorage) GetEnvelopesSince(groupID string, afterSeq int64, maxCount int) ([]*coordination.EnvelopeRecord, error) {
	if maxCount < 1 {
		maxCount = 50
	}
	rows, err := s.db.Conn.Query(
		`SELECT seq, group_id, msg_type, epoch, envelope, hlc_wall_ms, hlc_counter, hlc_node_id
		 FROM envelope_log
		 WHERE group_id = ? AND seq > ?
		 ORDER BY seq ASC
		 LIMIT ?`,
		groupID, afterSeq, maxCount,
	)
	if err != nil {
		return nil, fmt.Errorf("GetEnvelopesSince: %w", err)
	}
	defer rows.Close()

	var out []*coordination.EnvelopeRecord
	for rows.Next() {
		var r coordination.EnvelopeRecord
		var mt string
		if err := rows.Scan(&r.Seq, &r.GroupID, &mt, &r.Epoch, &r.Envelope,
			&r.Timestamp.WallTimeMs, &r.Timestamp.Counter, &r.Timestamp.NodeID); err != nil {
			return nil, fmt.Errorf("GetEnvelopesSince scan: %w", err)
		}
		r.MsgType = coordination.MessageType(mt)
		out = append(out, &r)
	}
	return out, rows.Err()
}

func (s *SQLiteCoordinationStorage) GetLatestSeq(groupID string) (int64, error) {
	var max sql.NullInt64
	err := s.db.Conn.QueryRow(
		`SELECT MAX(seq) FROM envelope_log WHERE group_id = ?`, groupID,
	).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("GetLatestSeq: %w", err)
	}
	if !max.Valid {
		return 0, nil
	}
	return max.Int64, nil
}

func (s *SQLiteCoordinationStorage) PruneEnvelopes(cutoffUnix int64, maxPerGroup int) (removed int, err error) {
	res, err := s.db.Conn.Exec(
		`DELETE FROM envelope_log WHERE created_at < ?`, cutoffUnix,
	)
	if err != nil {
		return 0, fmt.Errorf("PruneEnvelopes age: %w", err)
	}
	n64, _ := res.RowsAffected()
	removed += int(n64)
	_, _ = s.db.Conn.Exec(`DELETE FROM envelope_dedup WHERE created_at < ?`, cutoffUnix)

	if maxPerGroup < 1 {
		return removed, nil
	}

	// Per-group FIFO cap: delete oldest rows beyond maxPerGroup.
	rows, err := s.db.Conn.Query(`SELECT DISTINCT group_id FROM envelope_log`)
	if err != nil {
		return removed, fmt.Errorf("PruneEnvelopes list groups: %w", err)
	}
	var groups []string
	for rows.Next() {
		var gid string
		if err := rows.Scan(&gid); err != nil {
			_ = rows.Close()
			return removed, err
		}
		groups = append(groups, gid)
	}
	_ = rows.Close()

	for _, gid := range groups {
		var cnt int
		if err := s.db.Conn.QueryRow(
			`SELECT COUNT(*) FROM envelope_log WHERE group_id = ?`, gid,
		).Scan(&cnt); err != nil {
			return removed, err
		}
		for cnt > maxPerGroup {
			_, err := s.db.Conn.Exec(
				`DELETE FROM envelope_log WHERE id = (
					SELECT id FROM envelope_log WHERE group_id = ? ORDER BY seq ASC LIMIT 1
				)`, gid,
			)
			if err != nil {
				return removed, fmt.Errorf("PruneEnvelopes cap: %w", err)
			}
			removed++
			cnt--
		}
	}
	return removed, nil
}

func (s *SQLiteCoordinationStorage) RecordSyncAck(peerID, groupID string, ackedSeq int64) error {
	now := time.Now().Unix()
	_, err := s.db.Conn.Exec(
		`INSERT INTO sync_acks (peer_id, group_id, acked_seq, acked_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(peer_id, group_id) DO UPDATE SET
		   acked_seq = CASE WHEN excluded.acked_seq > sync_acks.acked_seq THEN excluded.acked_seq ELSE sync_acks.acked_seq END,
		   acked_at = excluded.acked_at`,
		peerID, groupID, ackedSeq, now,
	)
	if err != nil {
		return fmt.Errorf("RecordSyncAck: %w", err)
	}
	return nil
}

func (s *SQLiteCoordinationStorage) GetSyncAck(peerID, groupID string) (int64, error) {
	var ack int64
	err := s.db.Conn.QueryRow(
		`SELECT acked_seq FROM sync_acks WHERE peer_id = ? AND group_id = ?`,
		peerID, groupID,
	).Scan(&ack)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("GetSyncAck: %w", err)
	}
	return ack, nil
}

func (s *SQLiteCoordinationStorage) GetMinAckedSeq(groupID string, peerIDs []string) (int64, error) {
	if len(peerIDs) == 0 {
		return 0, nil
	}
	var minAck int64 = -1
	for _, pid := range peerIDs {
		ack, err := s.GetSyncAck(pid, groupID)
		if err != nil {
			return 0, err
		}
		if minAck < 0 || ack < minAck {
			minAck = ack
		}
	}
	if minAck < 0 {
		return 0, nil
	}
	return minAck, nil
}

func (s *SQLiteCoordinationStorage) EnqueuePendingDeliveryAck(targetPeerID, groupID string, ackedSeq int64) error {
	now := time.Now().Unix()
	_, err := s.db.Conn.Exec(
		`INSERT INTO pending_delivery_acks (target_peer_id, group_id, acked_seq, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(target_peer_id, group_id) DO UPDATE SET
		   acked_seq = CASE WHEN excluded.acked_seq > pending_delivery_acks.acked_seq THEN excluded.acked_seq ELSE pending_delivery_acks.acked_seq END,
		   created_at = excluded.created_at`,
		targetPeerID, groupID, ackedSeq, now,
	)
	if err != nil {
		return fmt.Errorf("EnqueuePendingDeliveryAck: %w", err)
	}
	return nil
}

func (s *SQLiteCoordinationStorage) ListPendingDeliveryAcksForTarget(targetPeerID string) ([]coordination.PendingDeliveryAckRow, error) {
	rows, err := s.db.Conn.Query(
		`SELECT id, target_peer_id, group_id, acked_seq FROM pending_delivery_acks WHERE target_peer_id = ?`,
		targetPeerID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListPendingDeliveryAcksForTarget: %w", err)
	}
	defer rows.Close()

	var out []coordination.PendingDeliveryAckRow
	for rows.Next() {
		var r coordination.PendingDeliveryAckRow
		if err := rows.Scan(&r.ID, &r.TargetPeerID, &r.GroupID, &r.AckedSeq); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLiteCoordinationStorage) DeletePendingDeliveryAck(id int64) error {
	_, err := s.db.Conn.Exec(`DELETE FROM pending_delivery_acks WHERE id = ?`, id)
	return err
}

func (s *SQLiteCoordinationStorage) GetOfflinePullCursor(groupID, remotePeerID string) (int64, error) {
	var seq int64
	err := s.db.Conn.QueryRow(
		`SELECT last_remote_seq FROM offline_sync_pull_state WHERE group_id = ? AND remote_peer_id = ?`,
		groupID, remotePeerID,
	).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("GetOfflinePullCursor: %w", err)
	}
	return seq, nil
}

func (s *SQLiteCoordinationStorage) GetKnownGroupMembers(groupID string) ([]string, error) {
	rows, err := s.db.Conn.Query(
		`SELECT DISTINCT sender_id FROM stored_messages WHERE group_id = ?`, groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetKnownGroupMembers: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *SQLiteCoordinationStorage) SetOfflinePullCursor(groupID, remotePeerID string, lastRemoteSeq int64) error {
	now := time.Now().Unix()
	_, err := s.db.Conn.Exec(
		`INSERT INTO offline_sync_pull_state (group_id, remote_peer_id, last_remote_seq, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(group_id, remote_peer_id) DO UPDATE SET
		   last_remote_seq = excluded.last_remote_seq,
		   updated_at = excluded.updated_at`,
		groupID, remotePeerID, lastRemoteSeq, now,
	)
	if err != nil {
		return fmt.Errorf("SetOfflinePullCursor: %w", err)
	}
	return nil
}
