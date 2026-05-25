package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	inviteRequestTTL         = 7 * 24 * time.Hour
	processingTimeout        = 5 * time.Minute
	appliedWelcomeRetention  = 30 * 24 * time.Hour
	maxInviteRetryAttempts   = 5
	inviteMaintenanceEvery   = 30 * time.Second
	errInviteStateConflict   = "ERR_INVITE_REQUEST_STATE_CONFLICT"
	errInviteForbidden       = "ERR_INVITE_REQUEST_FORBIDDEN"
	errInviteNotFound        = "ERR_INVITE_REQUEST_NOT_FOUND"
	errInviteDuplicateActive = "ERR_INVITE_REQUEST_DUPLICATE_ACTIVE"
)

type GroupInviteRequestInfo struct {
	RequestID           string `json:"request_id"`
	GroupID             string `json:"group_id"`
	RequesterPeerID     string `json:"requester_peer_id"`
	TargetPeerID        string `json:"target_peer_id"`
	Status              string `json:"status"`
	FailureCode         string `json:"failure_code,omitempty"`
	FailureMessage      string `json:"failure_message,omitempty"`
	RejectionReason     string `json:"rejection_reason,omitempty"`
	AttemptCount        int    `json:"attempt_count"`
	MaxAttempts         int    `json:"max_attempts"`
	ProcessingStartedAt *int64 `json:"processing_started_at,omitempty"`
	ExpiresAt           int64  `json:"expires_at"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

type GroupInviteRequestListResult struct {
	Items      []GroupInviteRequestInfo `json:"items"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

func (r *Runtime) startInviteMaintenanceLoop() {
	r.mu.Lock()
	if r.inviteMaintenanceCancel != nil {
		r.inviteMaintenanceCancel()
		r.inviteMaintenanceCancel = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.inviteMaintenanceCancel = cancel
	r.mu.Unlock()

	go func() {
		r.runInviteMaintenanceSweep()
		ticker := time.NewTicker(inviteMaintenanceEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.runInviteMaintenanceSweep()
			}
		}
	}()
}

func (r *Runtime) runInviteMaintenanceSweep() {
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return
	}
	now := time.Now().Unix()
	if _, err := database.FailCorruptProcessingInviteRequests(now); err != nil {
		slog.Debug("invite maintenance: fail corrupt processing", "err", err)
	}
	if _, err := database.FailStaleProcessingInviteRequests(now, int64(processingTimeout.Seconds()), "ERR_INVITE_PROCESSING_TIMEOUT"); err != nil {
		slog.Debug("invite maintenance: fail stale processing", "err", err)
	}
	if _, err := database.ExpirePendingInviteRequests(now); err != nil {
		slog.Debug("invite maintenance: expire pending", "err", err)
	}
	if _, err := database.CleanupAppliedWelcomes(now - int64(appliedWelcomeRetention.Seconds())); err != nil {
		slog.Debug("invite maintenance: cleanup applied welcomes", "err", err)
	}

	// Keep notifications for 30 days
	if err := database.DeleteOldNotifications(30); err != nil {
		slog.Debug("notification maintenance: cleanup failed", "err", err)
	}
}

func (r *Runtime) GetGroupInvitePolicy(groupID string) (string, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return "", fmt.Errorf("group ID is required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return "", err
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return "", fmt.Errorf("database not initialized")
	}
	policy, err := database.GetGroupInvitePolicy(groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("ERR_GROUP_NOT_FOUND: group not found")
		}
		return "", err
	}
	return policy, nil
}

func (r *Runtime) SetGroupInvitePolicy(groupID, policy string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	normalized, err := store.NormalizeGroupInvitePolicy(policy)
	if err != nil {
		return err
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	_, actorPeerID, err := r.requireGroupPermission(groupID, permissionChangeGroupSetting)
	if err != nil {
		return err
	}
	if err := database.SetGroupInvitePolicy(groupID, normalized); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("ERR_GROUP_NOT_FOUND: group not found")
		}
		return err
	}
	if normalized == store.GroupInvitePolicyAnyMember {
		_ = r.processPolicySwitchAnyMember(groupID)
	}
	r.emit("group:invite_policy_changed", map[string]interface{}{
		"group_id": groupID,
		"policy":   normalized,
	})
	go r.replicateGroupInvitePolicyAfterLocalSave(groupID, normalized, actorPeerID)
	return nil
}

func (r *Runtime) processPolicySwitchAnyMember(groupID string) error {
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	queue, err := database.ListInviteRequestsByStatusesForGroup(groupID, []string{
		store.InviteRequestStatusPending,
		store.InviteRequestStatusFailed,
	}, true, 500)
	if err != nil {
		return err
	}
	total := len(queue)
	processed := 0
	approved := 0
	failed := 0
	for _, item := range queue {
		processed++
		if err := r.processInviteRequest(item.RequestID, true); err != nil {
			failed++
		} else {
			approved++
		}
		r.emit("group:invite_policy_processing", map[string]interface{}{
			"group_id":   groupID,
			"total":      total,
			"processed":  processed,
			"approved":   approved,
			"failed":     failed,
			"remaining":  total - processed,
			"is_running": processed < total,
		})
	}
	return nil
}

func (r *Runtime) RequestGroupInvite(groupID, targetPeerID string) (GroupInviteRequestInfo, error) {
	groupID = strings.TrimSpace(groupID)
	targetPeerID = strings.TrimSpace(targetPeerID)
	if groupID == "" || targetPeerID == "" {
		return GroupInviteRequestInfo{}, fmt.Errorf("group ID and target peer ID are required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if _, err := peer.Decode(targetPeerID); err != nil {
		return GroupInviteRequestInfo{}, fmt.Errorf("invalid target peer ID: %w", err)
	}
	requester, err := r.localPeerID()
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if requester == targetPeerID {
		return GroupInviteRequestInfo{}, fmt.Errorf("cannot invite yourself")
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return GroupInviteRequestInfo{}, fmt.Errorf("database not initialized")
	}
	// Ensure the group exists in local metadata; non-creator flow does not use
	// local policy value for routing (creator is the authority), but we still
	// keep this check so callers get a stable ERR_GROUP_NOT_FOUND contract.
	if _, err := database.GetGroupInvitePolicy(groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GroupInviteRequestInfo{}, fmt.Errorf("ERR_GROUP_NOT_FOUND: group not found")
		}
		return GroupInviteRequestInfo{}, err
	}
	localRec, _, err := r.localGroupMember(groupID)
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if localRec.Status == store.GroupMemberStatusActive && isAdminRole(localRec.Role) {
		if err := r.InvitePeerToGroup(targetPeerID, groupID); err != nil {
			return GroupInviteRequestInfo{}, err
		}
		now := time.Now().Unix()
		return GroupInviteRequestInfo{
			RequestID:       "",
			GroupID:         groupID,
			RequesterPeerID: requester,
			TargetPeerID:    targetPeerID,
			Status:          store.InviteRequestStatusApproved,
			ExpiresAt:       now + int64(inviteRequestTTL.Seconds()),
			CreatedAt:       now,
			UpdatedAt:       now,
			MaxAttempts:     maxInviteRetryAttempts,
		}, nil
	}
	// Non-manager path: forward to one active invite authority.
	//
	// Why we always forward instead of branching on local policy:
	//   - Single-Writer Invariant (PROJECT_PLAN §6.1, coordinator.go:553)
	//     forbids any node that is not the current Token Holder from issuing
	//     an MLS Commit. Members other than the creator are not the Token
	//     Holder by default, so a local AddMember would fail with
	//     ErrNotTokenHolder anyway.
	//   - "any_member" semantically means "an active admin/creator auto-approves
	//     any member submission", not "member commits independently".
	//   - Local policy may be stale (we lack a proactive policy push). The
	//     wire handler on the chosen authority decides the flow there.
	//
	authorities, err := r.resolveGroupInviteAuthorityPeerIDs(groupID)
	if err != nil {
		slog.Warn("invite request: cannot resolve invite authority", "group_id", groupID, "target_peer_id", targetPeerID, "error", err)
		return GroupInviteRequestInfo{}, err
	}
	// Pre-fetch the target's KeyPackage on the requester side and attach it
	// to the wire submit. The requester is the node that just clicked
	// "invite" so it is the most likely peer to have a verified connection
	// with the target right now; the creator may have never met the target
	// (different mDNS island, late join, etc.). Without this attachment,
	// the creator's fetchPeerKeyPackage falls back to discovery / blind
	// store and frequently fails — that is the production bug behind
	// ERR_INVITE_ADD_MEMBER_FAILED on the creator side.
	//
	// Failure here is non-fatal — we still forward the submit. The creator
	// will try its own fetch path; if that also fails the wire response
	// surfaces the canonical error and the row stays in `failed` state for
	// retry rather than silently disappearing.
	var targetKP []byte
	if targetID, decErr := peer.Decode(targetPeerID); decErr == nil {
		if kp, kpErr := r.fetchPeerKeyPackage(targetID); kpErr == nil && len(kp) > 0 {
			targetKP = kp
		} else if kpErr != nil {
			slog.Debug("invite request: requester KP pre-fetch failed (non-fatal)",
				"group_id", groupID, "target_peer_id", targetPeerID, "error", kpErr)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	resp, authorityPID, err := r.groupInviteWireCallAuthorities(ctx, authorities, &p2p.GroupInviteWireClientReqV1{
		V:                1,
		Op:               "submit",
		GroupID:          groupID,
		TargetPeerID:     targetPeerID,
		TargetKeyPackage: targetKP,
	}, false)
	if err != nil {
		slog.Warn("invite request: invite authority unreachable", "group_id", groupID, "target_peer_id", targetPeerID, "error", err)
		return GroupInviteRequestInfo{}, fmt.Errorf("%s: could not reach group invite authority: %w", errCreatorUnreachable, err)
	}
	if !resp.OK || resp.Record == nil {
		msg := "remote invite submit failed"
		if resp != nil && strings.TrimSpace(resp.Error) != "" {
			msg = strings.TrimSpace(resp.Error)
		}
		slog.Warn("invite request: authority rejected or wire failed", "group_id", groupID, "authority_peer_id", authorityPID.String(), "target_peer_id", targetPeerID, "detail", msg)
		return GroupInviteRequestInfo{}, errors.New(msg)
	}
	wireRec := inviteWireToRecord(resp.Record)
	wireRec.IsMirror = true
	if err := database.UpsertGroupInviteRequestMirror(wireRec); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	r.appendGroupEvent(groupID, groupEventTypeInviteRequestCreated, requester, targetPeerID, 0, map[string]any{
		"request_id":        wireRec.RequestID,
		"requester_peer_id": requester,
		"target_peer_id":    targetPeerID,
		"status":            wireRec.Status,
		"source":            "mirror",
	})
	out, err := database.GetGroupInviteRequest(wireRec.RequestID)
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	// Use the creator-side status when emitting so the UI can render the
	// outcome immediately (any_member auto-approves to "approved", while
	// creator_approval stays "pending").
	r.emit("group:invite_request_changed", map[string]interface{}{
		"group_id":   groupID,
		"request_id": wireRec.RequestID,
		"status":     out.Status,
		"reason":     "created_mirror",
	})
	return toInviteRequestInfo(out), nil
}

func (r *Runtime) ApproveGroupInviteRequest(requestID string) (GroupInviteRequestInfo, error) {
	if err := r.ensureSessionActive(); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if _, err := r.requireInviteManagerForRequest(requestID); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if err := r.processInviteRequest(requestID, false); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return GroupInviteRequestInfo{}, fmt.Errorf("database not initialized")
	}
	rec, err := database.GetGroupInviteRequest(requestID)
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	r.appendGroupEvent(rec.GroupID, groupEventTypeInviteRequestApproved, rec.RequesterPeerID, rec.TargetPeerID, 0, map[string]any{
		"request_id":        rec.RequestID,
		"requester_peer_id": rec.RequesterPeerID,
		"target_peer_id":    rec.TargetPeerID,
		"status":            rec.Status,
	})
	return toInviteRequestInfo(rec), nil
}

func (r *Runtime) RejectGroupInviteRequest(requestID, reason string) (GroupInviteRequestInfo, error) {
	rec, err := r.requireInviteManagerForRequest(requestID)
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return GroupInviteRequestInfo{}, fmt.Errorf("database not initialized")
	}
	ok, err := database.TryTransitionInviteRequest(requestID, []string{store.InviteRequestStatusPending}, store.InviteRequestStatusRejected, store.InviteRequestTransitionPatch{
		RejectionReason: ptrString(strings.TrimSpace(reason)),
		ClearFailure:    true,
	})
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if !ok {
		return GroupInviteRequestInfo{}, fmt.Errorf("%s: request is no longer pending", errInviteStateConflict)
	}
	out, err := database.GetGroupInviteRequest(requestID)
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	r.maybePushInviteUpdate(context.Background(), out)
	r.appendGroupEvent(rec.GroupID, groupEventTypeInviteRequestRejected, rec.RequesterPeerID, rec.TargetPeerID, 0, map[string]any{
		"request_id":        requestID,
		"requester_peer_id": rec.RequesterPeerID,
		"target_peer_id":    rec.TargetPeerID,
		"status":            store.InviteRequestStatusRejected,
		"reason":            strings.TrimSpace(reason),
	})
	r.emit("group:invite_request_decided", map[string]interface{}{
		"group_id":       rec.GroupID,
		"request_id":     requestID,
		"status":         store.InviteRequestStatusRejected,
		"requester_id":   rec.RequesterPeerID,
		"target_peer_id": rec.TargetPeerID,
	})
	return toInviteRequestInfo(out), nil
}

// CancelGroupInviteRequest was intentionally removed (2026-05-10): in a
// serverless P2P mesh, racing a requester-side cancel against a concurrent
// creator-side approve would require CRDT-style coordination across the
// gossip network to avoid split-brain (e.g. cancel succeeds locally while
// the approve already produced a Welcome). For thesis scope we keep only the
// two well-defined transitions:
//   - Creator: ApproveGroupInviteRequest (advances to processing/approved)
//   - Creator: RejectGroupInviteRequest (terminal rejected with reason)
// A requester who changes their mind simply waits — once auto-join has run
// they can opt out via LeaveGroup. Status `cancelled` remains in the schema
// for backward compatibility with rows persisted before this change but is
// no longer produced by any code path.

func (r *Runtime) SyncInviteRequestFromCreator(requestID string) (GroupInviteRequestInfo, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return GroupInviteRequestInfo{}, fmt.Errorf("%s: request ID is required", errInviteNotFound)
	}
	if err := r.ensureSessionActive(); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return GroupInviteRequestInfo{}, fmt.Errorf("database not initialized")
	}
	rec, err := database.GetGroupInviteRequest(requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupInviteRequestInfo{}, fmt.Errorf("%s: request not found", errInviteNotFound)
	}
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if !rec.IsMirror {
		return toInviteRequestInfo(rec), nil
	}
	authorities, err := r.resolveGroupInviteAuthorityPeerIDs(rec.GroupID)
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	local, err := r.localPeerID()
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if local != rec.RequesterPeerID {
		return GroupInviteRequestInfo{}, fmt.Errorf("%s: only the requester can sync this invite request", errInviteForbidden)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	resp, _, err := r.groupInviteWireCallAuthorities(ctx, authorities, &p2p.GroupInviteWireClientReqV1{V: 1, Op: "sync", RequestID: requestID}, true)
	if err != nil {
		return GroupInviteRequestInfo{}, fmt.Errorf("%s: could not reach group invite authority: %w", errCreatorUnreachable, err)
	}
	if !resp.OK || resp.Record == nil {
		msg := "sync failed"
		if resp != nil && strings.TrimSpace(resp.Error) != "" {
			msg = strings.TrimSpace(resp.Error)
		}
		return GroupInviteRequestInfo{}, errors.New(msg)
	}
	wireRec := inviteWireToRecord(resp.Record)
	wireRec.IsMirror = true
	if err := database.UpsertGroupInviteRequestMirror(wireRec); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	out, err := database.GetGroupInviteRequest(requestID)
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	r.emit("group:invite_request_changed", map[string]interface{}{
		"group_id":   rec.GroupID,
		"request_id": requestID,
		"status":     out.Status,
		"reason":     "synced",
	})
	return toInviteRequestInfo(out), nil
}

func (r *Runtime) RetryGroupInviteRequest(requestID string) (GroupInviteRequestInfo, error) {
	if _, err := r.requireInviteManagerForRequest(requestID); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	if err := r.processInviteRequest(requestID, false); err != nil {
		return GroupInviteRequestInfo{}, err
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return GroupInviteRequestInfo{}, fmt.Errorf("database not initialized")
	}
	rec, err := database.GetGroupInviteRequest(requestID)
	if err != nil {
		return GroupInviteRequestInfo{}, err
	}
	return toInviteRequestInfo(rec), nil
}

func (r *Runtime) ListGroupInviteRequests(groupID string, statuses []string, cursor string, limit int) (GroupInviteRequestListResult, error) {
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return GroupInviteRequestListResult{}, fmt.Errorf("database not initialized")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	rows, next, err := database.ListGroupInviteRequests(groupID, statuses, cursor, limit)
	if err != nil {
		return GroupInviteRequestListResult{}, err
	}
	items := make([]GroupInviteRequestInfo, 0, len(rows))
	for i := range rows {
		items = append(items, toInviteRequestInfo(&rows[i]))
	}
	return GroupInviteRequestListResult{
		Items:      items,
		NextCursor: next,
	}, nil
}

func (r *Runtime) processInviteRequest(requestID string, allowAnyMember bool) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return fmt.Errorf("%s: request ID is required", errInviteNotFound)
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	defer func() {
		rec, err := database.GetGroupInviteRequest(requestID)
		if err != nil || rec == nil || rec.IsMirror {
			return
		}
		switch rec.Status {
		case store.InviteRequestStatusApproved, store.InviteRequestStatusFailed, store.InviteRequestStatusPermanentlyFailed:
			r.maybePushInviteUpdate(context.Background(), rec)
		}
	}()
	rec, err := database.GetGroupInviteRequest(requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: request not found", errInviteNotFound)
	}
	if err != nil {
		return err
	}
	if rec.IsMirror {
		return fmt.Errorf("%s: mirror invite rows cannot be processed locally", errInviteForbidden)
	}
	if !allowAnyMember {
		if _, _, err := r.requireGroupPermission(rec.GroupID, permissionManageInvites); err != nil {
			return err
		}
	}

	started := sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	ok, err := database.TryTransitionInviteRequest(requestID,
		[]string{store.InviteRequestStatusPending, store.InviteRequestStatusFailed},
		store.InviteRequestStatusProcessing,
		store.InviteRequestTransitionPatch{
			ProcessingStartedAt: &started,
			IncrementAttempt:    true,
			ClearFailure:        true,
		},
	)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s: request already processed", errInviteStateConflict)
	}
	rec, err = database.GetGroupInviteRequest(requestID)
	if err != nil {
		return err
	}
	if rec.AttemptCount > rec.MaxAttempts {
		_, _ = database.TryTransitionInviteRequest(requestID, []string{store.InviteRequestStatusProcessing}, store.InviteRequestStatusPermanentlyFailed, store.InviteRequestTransitionPatch{
			FailureCode:    ptrString("ERR_INVITE_MAX_ATTEMPTS_EXCEEDED"),
			FailureMessage: ptrString("invite request reached maximum retry attempts"),
		})
		return fmt.Errorf("ERR_INVITE_MAX_ATTEMPTS_EXCEEDED: request reached maximum retry attempts")
	}
	alreadyMember, err := r.isTargetAlreadyMember(rec.GroupID, rec.TargetPeerID)
	if err != nil {
		_, _ = database.TryTransitionInviteRequest(requestID, []string{store.InviteRequestStatusProcessing}, store.InviteRequestStatusFailed, store.InviteRequestTransitionPatch{
			FailureCode:    ptrString("ERR_INVITE_MEMBER_CHECK_FAILED"),
			FailureMessage: ptrString(err.Error()),
		})
		return err
	}
	if alreadyMember {
		_, _ = database.TryTransitionInviteRequest(requestID, []string{store.InviteRequestStatusProcessing}, store.InviteRequestStatusApproved, store.InviteRequestTransitionPatch{
			ProcessingStartedAt: &sql.NullInt64{},
			ClearFailure:        true,
		})
		return nil
	}
	if err := r.InvitePeerToGroup(rec.TargetPeerID, rec.GroupID); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "already") || strings.Contains(msg, "duplicate") {
			_, _ = database.TryTransitionInviteRequest(requestID, []string{store.InviteRequestStatusProcessing}, store.InviteRequestStatusApproved, store.InviteRequestTransitionPatch{
				ProcessingStartedAt: &sql.NullInt64{},
				ClearFailure:        true,
			})
			return nil
		}
		_, _ = database.TryTransitionInviteRequest(requestID, []string{store.InviteRequestStatusProcessing}, store.InviteRequestStatusFailed, store.InviteRequestTransitionPatch{
			FailureCode:         ptrString("ERR_INVITE_ADD_MEMBER_FAILED"),
			FailureMessage:      ptrString(err.Error()),
			ProcessingStartedAt: &sql.NullInt64{},
		})
		return err
	}
	_, err = database.TryTransitionInviteRequest(requestID, []string{store.InviteRequestStatusProcessing}, store.InviteRequestStatusApproved, store.InviteRequestTransitionPatch{
		ProcessingStartedAt: &sql.NullInt64{},
		ClearFailure:        true,
	})
	if err != nil {
		return err
	}

	// Link the invite request row to its MLS operation so UI / API can show
	// the joint lifecycle (approved + waiting for commit + waiting for
	// Welcome). We resolve the operation by (group, target) since
	// InvitePeerToGroup just upserted a row for that pair.
	if ops, listErr := database.ListGroupAddOperationsForTarget(rec.GroupID, rec.TargetPeerID); listErr == nil && len(ops) > 0 {
		if linkErr := database.LinkInviteRequestToAddOperation(requestID, ops[0].OperationID); linkErr != nil {
			slog.Debug("LinkInviteRequestToAddOperation skipped",
				"request_id", requestID, "operation_id", ops[0].OperationID, "err", linkErr)
		}
	}
	return nil
}

func (r *Runtime) isTargetAlreadyMember(groupID, targetPeerID string) (bool, error) {
	r.mu.RLock()
	coord := r.coordinators[groupID]
	database := r.db
	r.mu.RUnlock()
	if coord != nil {
		for _, m := range coord.ActiveMembers() {
			if m.String() == targetPeerID {
				return true, nil
			}
		}
	}
	if database != nil {
		members, err := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
		if err == nil {
			for _, rec := range members {
				if rec.PeerID == targetPeerID {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (r *Runtime) requireInviteManagerForRequest(requestID string) (*store.GroupInviteRequestRecord, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("%s: request ID is required", errInviteNotFound)
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rec, err := database.GetGroupInviteRequest(requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s: request not found", errInviteNotFound)
	}
	if err != nil {
		return nil, err
	}
	if rec.IsMirror {
		return nil, fmt.Errorf("%s: invalid invite row for this action", errInviteForbidden)
	}
	if _, _, err := r.requireGroupPermission(rec.GroupID, permissionManageInvites); err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *Runtime) localPeerID() (string, error) {
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node != nil {
		return node.Host.ID().String(), nil
	}
	info, err := r.GetOnboardingInfo()
	if err != nil || info == nil || strings.TrimSpace(info.PeerID) == "" {
		return "", fmt.Errorf("local peer ID unavailable")
	}
	return strings.TrimSpace(info.PeerID), nil
}

func newInviteRequestID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate invite request ID: %w", err)
	}
	return "gir-" + hex.EncodeToString(b[:]), nil
}

func toInviteRequestInfo(rec *store.GroupInviteRequestRecord) GroupInviteRequestInfo {
	info := GroupInviteRequestInfo{
		RequestID:       rec.RequestID,
		GroupID:         rec.GroupID,
		RequesterPeerID: rec.RequesterPeerID,
		TargetPeerID:    rec.TargetPeerID,
		Status:          rec.Status,
		FailureCode:     rec.FailureCode,
		FailureMessage:  rec.FailureMessage,
		RejectionReason: rec.RejectionReason,
		AttemptCount:    rec.AttemptCount,
		MaxAttempts:     rec.MaxAttempts,
		ExpiresAt:       rec.ExpiresAt,
		CreatedAt:       rec.CreatedAt,
		UpdatedAt:       rec.UpdatedAt,
	}
	if rec.ProcessingStartedAt.Valid {
		ts := rec.ProcessingStartedAt.Int64
		info.ProcessingStartedAt = &ts
	}
	return info
}

func ptrString(v string) *string {
	return &v
}
