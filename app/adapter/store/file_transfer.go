package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrFileTransferNotFound is returned when no row exists for the given file id.
var ErrFileTransferNotFound = errors.New("file transfer not found")

// FileTransferDirection indicates whether this row is outbound ciphertext staging or inbound receive state.
type FileTransferDirection string

const (
	FileTransferDirectionOut FileTransferDirection = "out"
	FileTransferDirectionIn  FileTransferDirection = "in"
)

// FileTransferRecord tracks prepared outbound ciphertext or inbound metadata for MLS-derived file transfer.
type FileTransferRecord struct {
	FileID          string
	GroupID         string
	Direction       FileTransferDirection
	PlaintextSHA256 []byte
	PlaintextSize   int64
	ChunkSize       int
	ChunkCount      int
	ExportEpoch     uint64
	SenderPeerID    string
	CiphertextDir   string
	State           string
	CreatedAt       int64
	UpdatedAt       int64
}

const (
	FileTransferStateReady      = "ready"
	FileTransferStateTransferring = "transferring"
	FileTransferStateCompleted  = "completed"
	FileTransferStateFailed     = "failed"
)

// UpsertFileTransfer inserts or replaces a file transfer row.
func (d *Database) UpsertFileTransfer(rec *FileTransferRecord) error {
	if rec == nil || rec.FileID == "" {
		return errors.New("file transfer: missing file_id")
	}
	now := time.Now().Unix()
	if rec.CreatedAt == 0 {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	_, err := d.Conn.Exec(`
		INSERT INTO file_transfers (
			file_id, group_id, direction, plaintext_sha256, plaintext_size,
			chunk_size, chunk_count, export_epoch, sender_peer_id, ciphertext_dir,
			state, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_id) DO UPDATE SET
			group_id = excluded.group_id,
			direction = excluded.direction,
			plaintext_sha256 = excluded.plaintext_sha256,
			plaintext_size = excluded.plaintext_size,
			chunk_size = excluded.chunk_size,
			chunk_count = excluded.chunk_count,
			export_epoch = excluded.export_epoch,
			sender_peer_id = excluded.sender_peer_id,
			ciphertext_dir = excluded.ciphertext_dir,
			state = excluded.state,
			updated_at = excluded.updated_at
	`,
		rec.FileID,
		rec.GroupID,
		string(rec.Direction),
		rec.PlaintextSHA256,
		rec.PlaintextSize,
		rec.ChunkSize,
		rec.ChunkCount,
		rec.ExportEpoch,
		rec.SenderPeerID,
		rec.CiphertextDir,
		rec.State,
		rec.CreatedAt,
		rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert file_transfers: %w", err)
	}
	return nil
}

// GetFileTransfer returns a row by file_id.
func (d *Database) GetFileTransfer(fileID string) (*FileTransferRecord, error) {
	row := d.Conn.QueryRow(`
		SELECT file_id, group_id, direction, plaintext_sha256, plaintext_size,
		       chunk_size, chunk_count, export_epoch, sender_peer_id, ciphertext_dir,
		       state, created_at, updated_at
		FROM file_transfers WHERE file_id = ?
	`, fileID)
	var rec FileTransferRecord
	var dir string
	err := row.Scan(
		&rec.FileID,
		&rec.GroupID,
		&dir,
		&rec.PlaintextSHA256,
		&rec.PlaintextSize,
		&rec.ChunkSize,
		&rec.ChunkCount,
		&rec.ExportEpoch,
		&rec.SenderPeerID,
		&rec.CiphertextDir,
		&rec.State,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFileTransferNotFound
	}
	rec.Direction = FileTransferDirection(dir)
	return &rec, nil
}
