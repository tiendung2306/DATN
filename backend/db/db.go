package db

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
)

type Database struct {
	Conn *sql.DB
}

func InitDB(path string) (*Database, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &Database{Conn: db}
	if err := d.createTables(); err != nil {
		return nil, err
	}

	slog.Info("Database initialized", "path", path)
	return d, nil
}

func (d *Database) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			public_key BLOB,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id TEXT NOT NULL,
			sender_id TEXT NOT NULL,
			content TEXT,
			mls_message BLOB,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS system_config (
			key TEXT PRIMARY KEY,
			value BLOB
		);`,
	}

	for _, q := range queries {
		if _, err := d.Conn.Exec(q); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}
	return nil
}

func (d *Database) Close() {
	if d.Conn != nil {
		d.Conn.Close()
	}
}
