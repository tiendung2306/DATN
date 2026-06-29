package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/store"
)

var (
	ErrInviteNotFound        = errors.New("invite not found")
	ErrInviteAlreadyRejected = errors.New("invite already rejected")
)

// JoinCodeResult is the frontend-safe public KeyPackage wrapper.
type JoinCodeResult struct {
	CodeHex    string `json:"code_hex"`
	Checksum   string `json:"checksum"`
	CreatedAt  int64  `json:"created_at"`
	OneTimeUse bool   `json:"one_time_use"`
}

// PendingInviteInfo is an invite row shown by the product UI.
type PendingInviteInfo struct {
	ID          string `json:"id"`
	GroupID     string `json:"group_id"`
	GroupType   string `json:"group_type"`
	GroupName   string `json:"group_name,omitempty"`
	InviterPeer string `json:"inviter_peer,omitempty"`
	ReceivedAt  int64  `json:"received_at"`
	Status      string `json:"status"`
}

// GenerateJoinCode returns the local public KeyPackage without exposing the
// private KeyPackageBundle material required to consume a later Welcome.
func (r *Runtime) GenerateJoinCode() (JoinCodeResult, error) {
	if err := r.ensureSessionActive(); err != nil {
		return JoinCodeResult{}, err
	}
	publicKP, err := r.ensureLocalPublicKPBytes()
	if err != nil {
		return JoinCodeResult{}, err
	}
	sum := sha256.Sum256(publicKP)
	return JoinCodeResult{
		CodeHex:    hex.EncodeToString(publicKP),
		Checksum:   hex.EncodeToString(sum[:4]),
		CreatedAt:  time.Now().Unix(),
		OneTimeUse: true,
	}, nil
}

// ListPendingInvites refreshes local pending invites from store peers, then
// returns only actionable pending rows for the UI.
func (r *Runtime) ListPendingInvites() ([]PendingInviteInfo, error) {
	if err := r.refreshPendingInvites(context.Background()); err != nil {
		// Listing local rows should remain useful even when the network refresh fails.
		// The warning is intentionally not returned to avoid making the UI brittle.
		slog.Debug("refresh pending invites failed", "err", err)
	}

	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := database.ListPendingInvites(false)
	if err != nil {
		return nil, err
	}
	out := make([]PendingInviteInfo, 0, len(rows))
	for _, inv := range rows {
		if strings.EqualFold(strings.TrimSpace(inv.GroupType), "dm") {
			continue
		}
		out = append(out, pendingInviteInfoFromStore(inv))
	}
	return out, nil
}

// AcceptInvite joins the group represented by a pending Welcome. It is
// idempotent once the group has already been joined.
func (r *Runtime) AcceptInvite(inviteID string) error {
	inviteID = strings.TrimSpace(inviteID)
	if inviteID == "" {
		return ErrInviteNotFound
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

	inv, err := database.GetPendingInvite(inviteID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInviteNotFound
	}
	if err != nil {
		return fmt.Errorf("load pending invite: %w", err)
	}
	if inv.Status == store.PendingInviteStatusRejected {
		return ErrInviteAlreadyRejected
	}

	joined, err := database.HasGroup(inv.GroupID)
	if err != nil {
		return err
	}
	if joined {
		active, err := database.IsGroupActive(inv.GroupID)
		if err == nil && !active {
			slog.Info("AcceptInvite: group was previously left. Purging stale metadata.", "group_id", inv.GroupID)
			_ = database.PurgeGroupMetadata(inv.GroupID)
			joined = false
		}
	}
	if joined {
		if err := database.MarkPendingInviteAccepted(inv.ID); err != nil {
			return err
		}
		r.emit("invite:accepted", map[string]interface{}{
			"id":       inv.ID,
			"group_id": inv.GroupID,
			"status":   store.PendingInviteStatusAccepted,
			"reason":   "already_joined",
		})
		return nil
	}

	if err := r.applyWelcome(inv.GroupID, inv.GroupType, hex.EncodeToString(inv.WelcomeBytes), inv.CategoryID, inv.SourcePeerID, 0, nil); err != nil {
		return fmt.Errorf("accept invite: %w", err)
	}
	if strings.TrimSpace(inv.SourcePeerID) != "" {
		// welcome-source observation. The role of the delivering peer is
		// unknown to us; preserve any existing annotation.
		_ = r.upsertGroupMemberFromRosterSync(inv.GroupID, inv.SourcePeerID, "welcome-source")
		r.emit("group:members_changed", map[string]interface{}{
			"group_id": inv.GroupID,
			"reason":   "welcome_source",
		})
	}
	if err := database.MarkPendingInviteAccepted(inv.ID); err != nil {
		return err
	}
	r.emit("invite:accepted", map[string]interface{}{
		"id":       inv.ID,
		"group_id": inv.GroupID,
		"status":   store.PendingInviteStatusAccepted,
		"reason":   "accepted",
	})
	return nil
}

// RejectInvite is intentionally local-only; no protocol-level rejection exists.
func (r *Runtime) RejectInvite(inviteID string) error {
	inviteID = strings.TrimSpace(inviteID)
	if inviteID == "" {
		return ErrInviteNotFound
	}

	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	inv, err := database.GetPendingInvite(inviteID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInviteNotFound
	}
	if err != nil {
		return err
	}
	if err := database.MarkPendingInviteRejected(inviteID); errors.Is(err, sql.ErrNoRows) {
		return ErrInviteNotFound
	} else if err != nil {
		return err
	}
	// Keep rejected row as tombstone so passive/background refresh does not
	// resurrect the same welcome immediately.
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node != nil {
		_ = database.DeleteStoredWelcome(node.Host.ID().String(), inv.GroupID)
	}
	r.emit("invite:rejected", map[string]interface{}{
		"id":       inviteID,
		"group_id": inv.GroupID,
		"status":   store.PendingInviteStatusRejected,
		"reason":   "rejected",
	})
	return nil
}

func pendingInviteInfoFromStore(inv store.PendingInvite) PendingInviteInfo {
	return PendingInviteInfo{
		ID:          inv.ID,
		GroupID:     inv.GroupID,
		GroupType:   inv.GroupType,
		GroupName:   inv.GroupName,
		InviterPeer: inv.InviterPeerID,
		ReceivedAt:  inv.ReceivedAt,
		Status:      inv.Status,
	}
}
