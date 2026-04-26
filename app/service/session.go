package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
)

const sessionStartedAtConfigKey = "session_started_at"

// killSessionPendingConfigKey is set after identity import so other devices lose the session.
const killSessionPendingConfigKey = "kill_session_pending"

const sessionReplacedAtConfigKey = "session_replaced_at"

const (
	SessionStateUnknown  = "unknown"
	SessionStateActive   = "active"
	SessionStateReplaced = "replaced"
)

var ErrSessionReplaced = errors.New("session has been replaced by a newer device")

type SessionStatus struct {
	State              string `json:"state"`
	SessionStartedAt   int64  `json:"session_started_at"`
	ReplacedDetectedAt int64  `json:"replaced_detected_at,omitempty"`
}

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

func encodeInt64Config(v int64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(v))
	return buf
}

func readInt64Config(database *store.Database, key string) (int64, bool, error) {
	raw, err := database.GetConfig(key)
	if err != nil {
		if store.IsNotFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if len(raw) != 8 {
		return 0, false, nil
	}
	return int64(binary.BigEndian.Uint64(raw)), true, nil
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
	if err := database.DeleteConfig(sessionReplacedAtConfigKey); err != nil && !store.IsNotFound(err) {
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

func (r *Runtime) GetSessionStatus() (SessionStatus, error) {
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return SessionStatus{State: SessionStateUnknown}, nil
	}

	startedAt, _, err := readInt64Config(database, sessionStartedAtConfigKey)
	if err != nil {
		return SessionStatus{}, fmt.Errorf("read session start: %w", err)
	}
	replacedAt, replaced, err := readInt64Config(database, sessionReplacedAtConfigKey)
	if err != nil {
		return SessionStatus{}, fmt.Errorf("read session replaced: %w", err)
	}
	status := SessionStatus{
		State:            SessionStateActive,
		SessionStartedAt: startedAt,
	}
	if replaced {
		status.State = SessionStateReplaced
		status.ReplacedDetectedAt = replacedAt
	}
	if startedAt == 0 && !replaced {
		status.State = SessionStateUnknown
	}
	return status, nil
}

// AcknowledgeSessionReplaced is a UI hook for recording that the user saw the
// lockout screen. It intentionally does not clear the replaced state.
func (r *Runtime) AcknowledgeSessionReplaced() error {
	status, err := r.GetSessionStatus()
	if err != nil {
		return err
	}
	if status.State != SessionStateReplaced {
		return nil
	}
	r.emit("session:replaced_acknowledged", map[string]interface{}{
		"replaced_detected_at": status.ReplacedDetectedAt,
	})
	return nil
}

func (r *Runtime) ensureSessionActive() error {
	status, err := r.GetSessionStatus()
	if err != nil {
		return err
	}
	if status.State == SessionStateReplaced {
		return ErrSessionReplaced
	}
	return nil
}

func isSessionReplaced(database *store.Database) (bool, error) {
	replacedAt, replaced, err := readInt64Config(database, sessionReplacedAtConfigKey)
	if err != nil {
		return false, err
	}
	return replaced && replacedAt > 0, nil
}

func (r *Runtime) markSessionReplaced(reason string, superseding p2p.SessionClaim) {
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return
	}

	now := time.Now().UnixMilli()
	if existing, ok, err := readInt64Config(database, sessionReplacedAtConfigKey); err == nil && ok && existing > 0 {
		now = existing
	} else if err := database.SetConfig(sessionReplacedAtConfigKey, encodeInt64Config(now)); err != nil {
		slog.Warn("failed to persist session replacement", "error", err)
		return
	}

	r.mu.Lock()
	r.stopCoordinatorsLocked()
	r.stopNetworkLocked()
	r.mu.Unlock()

	r.setP2PStatus(false, "session replaced by newer device")
	r.emit("session:replaced", map[string]interface{}{
		"state":                SessionStateReplaced,
		"reason":               reason,
		"session_started_at":   superseding.StartedAt,
		"replaced_detected_at": now,
	})
}
