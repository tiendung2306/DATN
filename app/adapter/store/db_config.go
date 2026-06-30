package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// SetConfig upserts a key-value pair in system_config.
func (d *Database) SetConfig(key string, value []byte) error {
	_, err := d.Conn.Exec(
		"INSERT OR REPLACE INTO system_config (key, value) VALUES (?, ?)",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("SetConfig(%q): %w", key, err)
	}
	return nil
}

// GetConfig retrieves a value from system_config.
// Returns sql.ErrNoRows if the key does not exist.
func (d *Database) GetConfig(key string) ([]byte, error) {
	var value []byte
	err := d.Conn.QueryRow(
		"SELECT value FROM system_config WHERE key = ?", key,
	).Scan(&value)
	if err != nil {
		return nil, err // includes sql.ErrNoRows unwrapped for easy errors.Is checks
	}
	return value, nil
}

// HasConfig returns true if the given key exists in system_config.
func (d *Database) HasConfig(key string) (bool, error) {
	var count int
	err := d.Conn.QueryRow(
		"SELECT COUNT(*) FROM system_config WHERE key = ?", key,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("HasConfig(%q): %w", key, err)
	}
	return count > 0, nil
}

// DeleteConfig removes a key from system_config. It is not an error if the key
// does not exist.
func (d *Database) DeleteConfig(key string) error {
	_, err := d.Conn.Exec("DELETE FROM system_config WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("DeleteConfig(%q): %w", key, err)
	}
	return nil
}

// ErrNotFound is a sentinel that callers can check when a record is missing.
// Use errors.Is(err, db.ErrNotFound) after GetMLSIdentity / GetAuthBundle.
var ErrNotFound = sql.ErrNoRows

// IsNotFound is a convenience wrapper around errors.Is(err, sql.ErrNoRows).
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
