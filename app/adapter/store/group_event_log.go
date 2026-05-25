package store

import "fmt"

type GroupEventLogRecord struct {
	ID           int64
	GroupID      string
	EventType    string
	ActorPeerID  string
	TargetPeerID string
	Epoch        uint64
	PayloadJSON  []byte
	CreatedAtMs  int64
}

func (d *Database) AppendGroupEvent(rec GroupEventLogRecord) (int64, error) {
	res, err := d.Conn.Exec(
		`INSERT INTO group_event_log
		 (group_id, event_type, actor_peer_id, target_peer_id, epoch, payload_json, created_at_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.GroupID, rec.EventType, rec.ActorPeerID, rec.TargetPeerID, rec.Epoch, rec.PayloadJSON, rec.CreatedAtMs,
	)
	if err != nil {
		return 0, fmt.Errorf("AppendGroupEvent: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("AppendGroupEvent last insert id: %w", err)
	}
	return id, nil
}

func (d *Database) ListGroupEvents(groupID string, limit int) ([]GroupEventLogRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Conn.Query(
		`SELECT id, group_id, event_type, actor_peer_id, target_peer_id, epoch, payload_json, created_at_ms
		 FROM group_event_log
		 WHERE group_id = ?
		 ORDER BY created_at_ms DESC, id DESC
		 LIMIT ?`,
		groupID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ListGroupEvents: %w", err)
	}
	defer rows.Close()
	out := make([]GroupEventLogRecord, 0)
	for rows.Next() {
		var rec GroupEventLogRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.GroupID,
			&rec.EventType,
			&rec.ActorPeerID,
			&rec.TargetPeerID,
			&rec.Epoch,
			&rec.PayloadJSON,
			&rec.CreatedAtMs,
		); err != nil {
			return nil, fmt.Errorf("ListGroupEvents scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
