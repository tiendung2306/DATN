package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
)

var (
	ErrGroupNotFound            = errors.New("group not found")
	ErrRemoveMemberNotSupported = errors.New("remove member is not supported yet")
)

// LeaveGroup performs a local soft leave: active participation stops, while
// local group state and message history remain available for archive UX.
func (r *Runtime) LeaveGroup(groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return ErrGroupNotFound
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}

	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}

	exists, err := database.HasGroup(groupID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrGroupNotFound
	}

	active, err := database.IsGroupActive(groupID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	r.mu.Lock()
	var coordToStop interface{ Stop() }
	if r.coordinators != nil {
		if coord := r.coordinators[groupID]; coord != nil {
			coordToStop = coord
			delete(r.coordinators, groupID)
		}
	}
	r.mu.Unlock()

	if coordToStop != nil {
		coordToStop.Stop()
	}

	if !active && coordToStop == nil {
		return nil
	}
	if err := database.MarkGroupLeft(groupID); errors.Is(err, sql.ErrNoRows) {
		return ErrGroupNotFound
	} else if err != nil {
		return err
	}

	r.emit("group:left", map[string]interface{}{"group_id": groupID})
	r.emit("group:members_changed", map[string]interface{}{
		"group_id": groupID,
		"reason":   "left",
	})
	return nil
}

// RemoveMemberFromGroup is deliberately not implemented until group roles and
// MLS Remove semantics are productized end-to-end.
func (r *Runtime) RemoveMemberFromGroup(groupID string, peerID string) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" {
		return ErrGroupNotFound
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	if peerID == "" {
		return fmt.Errorf("peer ID is required")
	}
	if _, err := peer.Decode(peerID); err != nil {
		return fmt.Errorf("invalid peer ID %q: %w", peerID, err)
	}
	return ErrRemoveMemberNotSupported
}
