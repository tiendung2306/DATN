package store

import "fmt"

type RuntimeEventRecord struct {
	Seq         int64
	Topic       string
	Aggregate   string
	AggregateID string
	Revision    int64
	PayloadJSON []byte
	CreatedAt   int64
}

func (d *Database) AppendRuntimeEvent(rec RuntimeEventRecord) (int64, error) {
	res, err := d.Conn.Exec(
		`INSERT INTO runtime_event_log
		 (topic, aggregate, aggregate_id, revision, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rec.Topic, rec.Aggregate, rec.AggregateID, rec.Revision, rec.PayloadJSON, rec.CreatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("AppendRuntimeEvent: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("AppendRuntimeEvent last insert id: %w", err)
	}
	return id, nil
}

func (d *Database) ListRuntimeEventsSince(lastSeq int64, limit int) ([]RuntimeEventRecord, error) {
	rows, err := d.Conn.Query(
		`SELECT seq, topic, aggregate, aggregate_id, revision, payload_json, created_at
		 FROM runtime_event_log
		 WHERE seq > ?
		 ORDER BY seq ASC
		 LIMIT ?`,
		lastSeq, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ListRuntimeEventsSince: %w", err)
	}
	defer rows.Close()
	out := make([]RuntimeEventRecord, 0)
	for rows.Next() {
		var rec RuntimeEventRecord
		if err := rows.Scan(&rec.Seq, &rec.Topic, &rec.Aggregate, &rec.AggregateID, &rec.Revision, &rec.PayloadJSON, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("ListRuntimeEventsSince scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (d *Database) GetLatestRuntimeSeq() (int64, error) {
	var seq int64
	if err := d.Conn.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM runtime_event_log`).Scan(&seq); err != nil {
		return 0, fmt.Errorf("GetLatestRuntimeSeq: %w", err)
	}
	return seq, nil
}

func (d *Database) PruneRuntimeEvents(minSeqToKeep int64, maxRowsToKeep int64) error {
	if minSeqToKeep > 0 {
		if _, err := d.Conn.Exec(`DELETE FROM runtime_event_log WHERE seq < ?`, minSeqToKeep); err != nil {
			return fmt.Errorf("PruneRuntimeEvents minSeq: %w", err)
		}
	}
	if maxRowsToKeep > 0 {
		if _, err := d.Conn.Exec(
			`DELETE FROM runtime_event_log
			 WHERE seq <= (
			   SELECT COALESCE(MAX(seq), 0) - ? FROM runtime_event_log
			 )`,
			maxRowsToKeep,
		); err != nil {
			return fmt.Errorf("PruneRuntimeEvents maxRows: %w", err)
		}
	}
	return nil
}
