package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	GroupInvitePolicyCreatorApproval = "creator_approval"
	GroupInvitePolicyAnyMember       = "any_member"

	InviteRequestStatusPending           = "pending"
	InviteRequestStatusProcessing        = "processing"
	InviteRequestStatusApproved          = "approved"
	InviteRequestStatusRejected          = "rejected"
	InviteRequestStatusFailed            = "failed"
	InviteRequestStatusCancelled         = "cancelled"
	InviteRequestStatusExpired           = "expired"
	InviteRequestStatusPermanentlyFailed = "permanently_failed"
)

type GroupInviteRequestRecord struct {
	RequestID           string
	GroupID             string
	RequesterPeerID     string
	TargetPeerID        string
	Status              string
	FailureCode         string
	FailureMessage      string
	RejectionReason     string
	AttemptCount        int
	MaxAttempts         int
	ProcessingStartedAt sql.NullInt64
	ExpiresAt           int64
	CreatedAt           int64
	UpdatedAt           int64
	IsMirror            bool
}

type InviteRequestTransitionPatch struct {
	FailureCode         *string
	FailureMessage      *string
	RejectionReason     *string
	ProcessingStartedAt *sql.NullInt64
	IncrementAttempt    bool
	ClearFailure        bool
}

func NormalizeGroupInvitePolicy(policy string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(policy)) {
	case "", GroupInvitePolicyCreatorApproval:
		return GroupInvitePolicyCreatorApproval, nil
	case GroupInvitePolicyAnyMember:
		return GroupInvitePolicyAnyMember, nil
	default:
		return "", fmt.Errorf("invalid invite policy %q", policy)
	}
}

func normalizeInviteRequestStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case InviteRequestStatusPending:
		return InviteRequestStatusPending
	case InviteRequestStatusProcessing:
		return InviteRequestStatusProcessing
	case InviteRequestStatusApproved:
		return InviteRequestStatusApproved
	case InviteRequestStatusRejected:
		return InviteRequestStatusRejected
	case InviteRequestStatusFailed:
		return InviteRequestStatusFailed
	case InviteRequestStatusCancelled:
		return InviteRequestStatusCancelled
	case InviteRequestStatusExpired:
		return InviteRequestStatusExpired
	case InviteRequestStatusPermanentlyFailed:
		return InviteRequestStatusPermanentlyFailed
	default:
		return ""
	}
}

func (d *Database) GetGroupInvitePolicy(groupID string) (string, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return "", sql.ErrNoRows
	}
	var policy string
	if err := d.Conn.QueryRow(`SELECT invite_policy FROM mls_groups WHERE group_id = ?`, groupID).Scan(&policy); err != nil {
		return "", err
	}
	return NormalizeGroupInvitePolicy(policy)
}

func (d *Database) SetGroupInvitePolicy(groupID, policy string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return sql.ErrNoRows
	}
	normalized, err := NormalizeGroupInvitePolicy(policy)
	if err != nil {
		return err
	}
	res, err := d.Conn.Exec(`UPDATE mls_groups SET invite_policy = ?, updated_at = CURRENT_TIMESTAMP WHERE group_id = ?`, normalized, groupID)
	if err != nil {
		return fmt.Errorf("SetGroupInvitePolicy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("SetGroupInvitePolicy rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *Database) CreateGroupInviteRequest(rec GroupInviteRequestRecord) error {
	if strings.TrimSpace(rec.RequestID) == "" || strings.TrimSpace(rec.GroupID) == "" || strings.TrimSpace(rec.RequesterPeerID) == "" || strings.TrimSpace(rec.TargetPeerID) == "" {
		return fmt.Errorf("CreateGroupInviteRequest: required fields missing")
	}
	now := time.Now().Unix()
	status := normalizeInviteRequestStatus(rec.Status)
	if status == "" {
		status = InviteRequestStatusPending
	}
	if rec.CreatedAt <= 0 {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt <= 0 {
		rec.UpdatedAt = now
	}
	if rec.ExpiresAt <= 0 {
		return fmt.Errorf("CreateGroupInviteRequest: expires_at is required")
	}
	if rec.MaxAttempts <= 0 {
		rec.MaxAttempts = 5
	}
	mirror := 0
	if rec.IsMirror {
		mirror = 1
	}
	_, err := d.Conn.Exec(
		`INSERT INTO group_invite_requests (
			request_id, group_id, requester_peer_id, target_peer_id, status,
			failure_code, failure_message, rejection_reason, attempt_count, max_attempts,
			processing_started_at, expires_at, created_at, updated_at, is_mirror
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.RequestID, rec.GroupID, rec.RequesterPeerID, rec.TargetPeerID, status,
		strings.TrimSpace(rec.FailureCode), strings.TrimSpace(rec.FailureMessage), strings.TrimSpace(rec.RejectionReason),
		rec.AttemptCount, rec.MaxAttempts, rec.ProcessingStartedAt, rec.ExpiresAt, rec.CreatedAt, rec.UpdatedAt, mirror,
	)
	if err != nil {
		return fmt.Errorf("CreateGroupInviteRequest: %w", err)
	}
	return nil
}

func (d *Database) GetGroupInviteRequest(requestID string) (*GroupInviteRequestRecord, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, sql.ErrNoRows
	}
	var rec GroupInviteRequestRecord
	var mirrorInt int
	if err := d.Conn.QueryRow(
		`SELECT request_id, group_id, requester_peer_id, target_peer_id, status, failure_code, failure_message, rejection_reason,
		        attempt_count, max_attempts, processing_started_at, expires_at, created_at, updated_at, is_mirror
		 FROM group_invite_requests
		 WHERE request_id = ?`,
		requestID,
	).Scan(
		&rec.RequestID, &rec.GroupID, &rec.RequesterPeerID, &rec.TargetPeerID, &rec.Status, &rec.FailureCode, &rec.FailureMessage, &rec.RejectionReason,
		&rec.AttemptCount, &rec.MaxAttempts, &rec.ProcessingStartedAt, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt, &mirrorInt,
	); err != nil {
		return nil, err
	}
	rec.IsMirror = mirrorInt != 0
	return &rec, nil
}

func (d *Database) TryTransitionInviteRequest(requestID string, fromStatuses []string, toStatus string, patch InviteRequestTransitionPatch) (bool, error) {
	requestID = strings.TrimSpace(requestID)
	toStatus = normalizeInviteRequestStatus(toStatus)
	if requestID == "" || toStatus == "" || len(fromStatuses) == 0 {
		return false, fmt.Errorf("TryTransitionInviteRequest: invalid arguments")
	}
	holders := make([]string, 0, len(fromStatuses))
	args := make([]interface{}, 0, 12+len(fromStatuses))
	now := time.Now().Unix()
	args = append(args, toStatus)
	query := `UPDATE group_invite_requests SET status = ?`
	if patch.ClearFailure {
		query += `, failure_code = '', failure_message = '', rejection_reason = ''`
	}
	if patch.FailureCode != nil {
		query += `, failure_code = ?`
		args = append(args, strings.TrimSpace(*patch.FailureCode))
	}
	if patch.FailureMessage != nil {
		query += `, failure_message = ?`
		args = append(args, strings.TrimSpace(*patch.FailureMessage))
	}
	if patch.RejectionReason != nil {
		query += `, rejection_reason = ?`
		args = append(args, strings.TrimSpace(*patch.RejectionReason))
	}
	if patch.ProcessingStartedAt != nil {
		query += `, processing_started_at = ?`
		args = append(args, patch.ProcessingStartedAt)
	}
	if patch.IncrementAttempt {
		query += `, attempt_count = attempt_count + 1`
	}
	query += `, updated_at = ? WHERE request_id = ? AND is_mirror = 0 AND (`
	args = append(args, now, requestID)
	for i, status := range fromStatuses {
		norm := normalizeInviteRequestStatus(status)
		if norm == "" {
			continue
		}
		if i > 0 {
			query += ` OR `
		}
		query += `status = ?`
		holders = append(holders, norm)
	}
	query += `)`
	for _, h := range holders {
		args = append(args, h)
	}
	if len(holders) == 0 {
		return false, fmt.Errorf("TryTransitionInviteRequest: no valid from statuses")
	}
	res, err := d.Conn.Exec(query, args...)
	if err != nil {
		return false, fmt.Errorf("TryTransitionInviteRequest: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("TryTransitionInviteRequest rows: %w", err)
	}
	return n > 0, nil
}

func (d *Database) ListGroupInviteRequests(groupID string, statuses []string, cursor string, limit int) ([]GroupInviteRequestRecord, string, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, "", nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	args := []interface{}{groupID}
	query := `SELECT request_id, group_id, requester_peer_id, target_peer_id, status, failure_code, failure_message, rejection_reason,
	                 attempt_count, max_attempts, processing_started_at, expires_at, created_at, updated_at, is_mirror
	          FROM group_invite_requests
			  WHERE group_id = ?`
	filtered := make([]string, 0, len(statuses))
	for _, s := range statuses {
		if n := normalizeInviteRequestStatus(s); n != "" {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) > 0 {
		query += ` AND status IN (` + strings.TrimRight(strings.Repeat("?,", len(filtered)), ",") + `)`
		for _, s := range filtered {
			args = append(args, s)
		}
	}
	if cursor != "" {
		parts := strings.Split(cursor, ":")
		if len(parts) == 2 {
			if ts, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
				query += ` AND (updated_at < ? OR (updated_at = ? AND request_id < ?))`
				args = append(args, ts, ts, parts[1])
			}
		}
	}
	query += ` ORDER BY updated_at DESC, request_id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := d.Conn.Query(query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("ListGroupInviteRequests: %w", err)
	}
	defer rows.Close()
	out := make([]GroupInviteRequestRecord, 0, limit+1)
	for rows.Next() {
		var rec GroupInviteRequestRecord
		var mirrorInt int
		if err := rows.Scan(
			&rec.RequestID, &rec.GroupID, &rec.RequesterPeerID, &rec.TargetPeerID, &rec.Status, &rec.FailureCode, &rec.FailureMessage, &rec.RejectionReason,
			&rec.AttemptCount, &rec.MaxAttempts, &rec.ProcessingStartedAt, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt, &mirrorInt,
		); err != nil {
			return nil, "", fmt.Errorf("ListGroupInviteRequests scan: %w", err)
		}
		rec.IsMirror = mirrorInt != 0
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		last := out[limit-1]
		next = fmt.Sprintf("%d:%s", last.UpdatedAt, last.RequestID)
		out = out[:limit]
	}
	return out, next, nil
}

func (d *Database) ListInviteRequestsByStatusesForGroup(groupID string, statuses []string, asc bool, limit int) ([]GroupInviteRequestRecord, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, nil
	}
	filtered := make([]string, 0, len(statuses))
	for _, s := range statuses {
		if n := normalizeInviteRequestStatus(s); n != "" {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 500
	}
	order := "DESC"
	if asc {
		order = "ASC"
	}
	args := []interface{}{groupID}
	query := `SELECT request_id, group_id, requester_peer_id, target_peer_id, status, failure_code, failure_message, rejection_reason,
	                 attempt_count, max_attempts, processing_started_at, expires_at, created_at, updated_at, is_mirror
	          FROM group_invite_requests
			  WHERE group_id = ? AND is_mirror = 0 AND status IN (` + strings.TrimRight(strings.Repeat("?,", len(filtered)), ",") + `)
			  ORDER BY created_at ` + order + `, request_id ` + order + ` LIMIT ?`
	for _, s := range filtered {
		args = append(args, s)
	}
	args = append(args, limit)
	rows, err := d.Conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListInviteRequestsByStatusesForGroup: %w", err)
	}
	defer rows.Close()
	out := make([]GroupInviteRequestRecord, 0, limit)
	for rows.Next() {
		var rec GroupInviteRequestRecord
		var mirrorInt int
		if err := rows.Scan(
			&rec.RequestID, &rec.GroupID, &rec.RequesterPeerID, &rec.TargetPeerID, &rec.Status, &rec.FailureCode, &rec.FailureMessage, &rec.RejectionReason,
			&rec.AttemptCount, &rec.MaxAttempts, &rec.ProcessingStartedAt, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt, &mirrorInt,
		); err != nil {
			return nil, err
		}
		rec.IsMirror = mirrorInt != 0
		out = append(out, rec)
	}
	return out, rows.Err()
}

// UpsertGroupInviteRequestMirror inserts or replaces a member-side mirror row (is_mirror=1).
func (d *Database) UpsertGroupInviteRequestMirror(rec GroupInviteRequestRecord) error {
	if strings.TrimSpace(rec.RequestID) == "" || strings.TrimSpace(rec.GroupID) == "" || strings.TrimSpace(rec.RequesterPeerID) == "" || strings.TrimSpace(rec.TargetPeerID) == "" {
		return fmt.Errorf("UpsertGroupInviteRequestMirror: required fields missing")
	}
	now := time.Now().Unix()
	status := normalizeInviteRequestStatus(rec.Status)
	if status == "" {
		status = InviteRequestStatusPending
	}
	if rec.CreatedAt <= 0 {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt <= 0 {
		rec.UpdatedAt = now
	}
	if rec.ExpiresAt <= 0 {
		return fmt.Errorf("UpsertGroupInviteRequestMirror: expires_at is required")
	}
	if rec.MaxAttempts <= 0 {
		rec.MaxAttempts = 5
	}
	_, err := d.Conn.Exec(
		`INSERT OR REPLACE INTO group_invite_requests (
			request_id, group_id, requester_peer_id, target_peer_id, status,
			failure_code, failure_message, rejection_reason, attempt_count, max_attempts,
			processing_started_at, expires_at, created_at, updated_at, is_mirror
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		rec.RequestID, rec.GroupID, rec.RequesterPeerID, rec.TargetPeerID, status,
		strings.TrimSpace(rec.FailureCode), strings.TrimSpace(rec.FailureMessage), strings.TrimSpace(rec.RejectionReason),
		rec.AttemptCount, rec.MaxAttempts, rec.ProcessingStartedAt, rec.ExpiresAt, rec.CreatedAt, rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("UpsertGroupInviteRequestMirror: %w", err)
	}
	return nil
}

func (d *Database) FailStaleProcessingInviteRequests(now, processingTimeoutSec int64, failureCode string) (int64, error) {
	if now <= 0 || processingTimeoutSec <= 0 {
		return 0, nil
	}
	if strings.TrimSpace(failureCode) == "" {
		failureCode = "ERR_INVITE_PROCESSING_TIMEOUT"
	}
	res, err := d.Conn.Exec(
		`UPDATE group_invite_requests
		 SET status = ?, failure_code = ?, failure_message = ?, updated_at = ?
		 WHERE is_mirror = 0 AND status = ? AND processing_started_at IS NOT NULL AND (? - processing_started_at) > ?`,
		InviteRequestStatusFailed, failureCode, "invite processing timed out", now,
		InviteRequestStatusProcessing, now, processingTimeoutSec,
	)
	if err != nil {
		return 0, fmt.Errorf("FailStaleProcessingInviteRequests: %w", err)
	}
	return res.RowsAffected()
}

func (d *Database) FailCorruptProcessingInviteRequests(now int64) (int64, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	res, err := d.Conn.Exec(
		`UPDATE group_invite_requests
		 SET status = ?, failure_code = ?, failure_message = ?, updated_at = ?
		 WHERE is_mirror = 0 AND status = ? AND processing_started_at IS NULL`,
		InviteRequestStatusFailed, "ERR_INVITE_CORRUPT_PROCESSING_STATE", "processing request missing processing_started_at", now,
		InviteRequestStatusProcessing,
	)
	if err != nil {
		return 0, fmt.Errorf("FailCorruptProcessingInviteRequests: %w", err)
	}
	return res.RowsAffected()
}

func (d *Database) ExpirePendingInviteRequests(now int64) (int64, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	res, err := d.Conn.Exec(
		`UPDATE group_invite_requests
		 SET status = ?, updated_at = ?
		 WHERE is_mirror = 0 AND status = ? AND expires_at <= ?`,
		InviteRequestStatusExpired, now, InviteRequestStatusPending, now,
	)
	if err != nil {
		return 0, fmt.Errorf("ExpirePendingInviteRequests: %w", err)
	}
	return res.RowsAffected()
}

func (d *Database) MarkWelcomeApplied(fingerprint, groupID string, appliedAt int64) (bool, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	groupID = strings.TrimSpace(groupID)
	if fingerprint == "" || groupID == "" {
		return false, fmt.Errorf("MarkWelcomeApplied: fingerprint and group ID are required")
	}
	if appliedAt <= 0 {
		appliedAt = time.Now().Unix()
	}
	res, err := d.Conn.Exec(
		`INSERT OR IGNORE INTO applied_welcomes (welcome_fingerprint, group_id, applied_at)
		 VALUES (?, ?, ?)`,
		fingerprint, groupID, appliedAt,
	)
	if err != nil {
		return false, fmt.Errorf("MarkWelcomeApplied: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("MarkWelcomeApplied rows: %w", err)
	}
	return n > 0, nil
}

func (d *Database) CleanupAppliedWelcomes(before int64) (int64, error) {
	if before <= 0 {
		return 0, nil
	}
	res, err := d.Conn.Exec(`DELETE FROM applied_welcomes WHERE applied_at < ?`, before)
	if err != nil {
		return 0, fmt.Errorf("CleanupAppliedWelcomes: %w", err)
	}
	return res.RowsAffected()
}

func IsInviteRequestConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique") || errors.Is(err, sql.ErrNoRows)
}
