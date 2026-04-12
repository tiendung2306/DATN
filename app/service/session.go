package service

import (
	"encoding/binary"
	"fmt"
	"time"

	"app/adapter/store"
	"app/adapter/p2p"
)

const sessionStartedAtConfigKey = "session_started_at"

// killSessionPendingConfigKey is set after identity import so other devices lose the session.
const killSessionPendingConfigKey = "kill_session_pending"

func getOrCreateSessionStartedAt(database *store.Database) (int64, error) {
	raw, err := database.GetConfig(sessionStartedAtConfigKey)
	if err == nil && len(raw) == 8 {
		return int64(binary.BigEndian.Uint64(raw)), nil
	}
	if err != nil && !store.IsNotFound(err) {
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

func resetSessionStartedAt(database *store.Database) (int64, error) {
	now := time.Now().UnixMilli()
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(now))
	if err := database.SetConfig(sessionStartedAtConfigKey, buf); err != nil {
		return 0, fmt.Errorf("persist session start: %w", err)
	}
	return now, nil
}

func buildLocalAuthHandshake(database *store.Database, tokenPeerID string) (*p2p.AuthHandshakeMsg, error) {
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

// ApplyIdentityImportSideEffects resets session timing and flags a pending kill-session
// so other devices lose the active session after identity migration.
func ApplyIdentityImportSideEffects(database *store.Database) error {
	if _, err := resetSessionStartedAt(database); err != nil {
		return err
	}
	return database.SetConfig(killSessionPendingConfigKey, []byte("1"))
}

func consumeKillSessionPendingFlag(database *store.Database) error {
	has, err := database.HasConfig(killSessionPendingConfigKey)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	return database.DeleteConfig(killSessionPendingConfigKey)
}
