package main

import (
	"encoding/binary"
	"fmt"
	"time"

	"app/db"
	"app/p2p"
)

const sessionStartedAtConfigKey = "session_started_at"

func getOrCreateSessionStartedAt(database *db.Database) (int64, error) {
	raw, err := database.GetConfig(sessionStartedAtConfigKey)
	if err == nil && len(raw) == 8 {
		return int64(binary.BigEndian.Uint64(raw)), nil
	}
	if err != nil && !db.IsNotFound(err) {
		return 0, fmt.Errorf("read session start: %w", err)
	}
	now := time.Now().UnixMilli()
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(now))
	if err := database.SetConfig(sessionStartedAtConfigKey, buf); err != nil {
		return 0, fmt.Errorf("persist session start: %w", err)
	}
	return now, nil
}

func resetSessionStartedAt(database *db.Database) (int64, error) {
	now := time.Now().UnixMilli()
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(now))
	if err := database.SetConfig(sessionStartedAtConfigKey, buf); err != nil {
		return 0, fmt.Errorf("persist session start: %w", err)
	}
	return now, nil
}

func buildLocalAuthHandshake(database *db.Database, tokenPeerID string) (*p2p.AuthHandshakeMsg, error) {
	identity, err := database.GetMLSIdentity()
	if err != nil {
		return nil, fmt.Errorf("load MLS identity for session claim: %w", err)
	}
	startedAt, err := getOrCreateSessionStartedAt(database)
	if err != nil {
		return nil, err
	}
	claim, err := p2p.BuildSessionClaim(identity.SigningKeyPrivate, tokenPeerID, startedAt)
	if err != nil {
		return nil, fmt.Errorf("build session claim: %w", err)
	}
	return &p2p.AuthHandshakeMsg{Session: claim}, nil
}

func consumeKillSessionPendingFlag(database *db.Database) error {
	has, err := database.HasConfig(killSessionPendingConfigKey)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	return database.DeleteConfig(killSessionPendingConfigKey)
}
