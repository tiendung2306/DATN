package store

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

type StoredMessageRow struct {
	ID      string
	GroupID string
	Content string
}

var canonicalMessageIDRegex = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

func isCanonicalMessageID(messageID string) bool {
	return canonicalMessageIDRegex.MatchString(strings.TrimSpace(messageID))
}

func (d *Database) GetStoredMessageByID(groupID string, messageID string) (*StoredMessageRow, error) {
	if !isCanonicalMessageID(messageID) {
		return nil, fmt.Errorf("invalid message_id: expected 64-char hex envelope hash")
	}
	var row StoredMessageRow
	var envelopeHash []byte
	err := d.Conn.QueryRow(
		`SELECT group_id, content, envelope_hash
		 FROM stored_messages
		 WHERE group_id = ?
		   AND LOWER(HEX(envelope_hash)) = LOWER(?)
		 LIMIT 1`,
		groupID, messageID,
	).Scan(&row.GroupID, &row.Content, &envelopeHash)
	if err != nil {
		return nil, err
	}
	if len(envelopeHash) == 0 {
		return nil, fmt.Errorf("invalid stored message: missing envelope_hash")
	}
	row.ID = hex.EncodeToString(envelopeHash)
	return &row, nil
}

func (d *Database) DeleteStoredMessageByID(groupID string, messageID string) error {
	if !isCanonicalMessageID(messageID) {
		return fmt.Errorf("invalid message_id: expected 64-char hex envelope hash")
	}
	res, err := d.Conn.Exec(
		`DELETE FROM stored_messages
		 WHERE group_id = ?
		   AND LOWER(HEX(envelope_hash)) = LOWER(?)`,
		groupID, messageID,
	)
	if err != nil {
		return fmt.Errorf("DeleteStoredMessageByID: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("DeleteStoredMessageByID rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
