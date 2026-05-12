package store

// group_add_operations — MLS Add operation lifecycle tracker.
//
// This table is the persistent source of truth for the coordination layer's
// view of an Add operation. Approval (group_invite_requests) is a
// business-level concept; commit authority moves with the Single-Writer
// token. Separating the two states lets the runtime:
//
//  1. Mark an invite as "approved" the moment the creator (or any_member
//     policy) authorises the Add, regardless of which node will end up
//     running CreateCommit.
//  2. Idempotently retry Welcome delivery if the Token Holder restarts after
//     CreateCommit succeeded but before the invitee acknowledged the join —
//     the row carries enough metadata to rebuild the outbox state.
//  3. Surface coordination progress in the UI ("waiting for commit", "MLS
//     committed, delivering Welcome", "joined") without overloading the
//     invite request status enum.
//
// Status transitions are append-only forward; terminal rows
// (welcome_delivered, failed) are not reactivated.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// AddOpStatusApproved means the runtime has authoritatively decided to
	// add the target peer. The local node has NOT yet acted on the MLS layer.
	AddOpStatusApproved = "approved"

	// AddOpStatusProposalBroadcast means the local node was not the Token
	// Holder and has broadcast a ProposalAdd to the group topic. The Welcome
	// is expected to arrive from whichever node ends up running CreateCommit
	// for the committing epoch.
	AddOpStatusProposalBroadcast = "proposal_broadcast"

	// AddOpStatusCommitObserved means a Commit referencing this operation has
	// been processed locally. On observer nodes this is terminal coordination
	// success (the invitee will receive the Welcome from the Token Holder).
	// On the Token Holder itself the status moves forward to welcome_queued
	// once the runtime has persisted the Welcome to pending_welcomes_out.
	AddOpStatusCommitObserved = "commit_observed"

	// AddOpStatusWelcomeQueued means the Token Holder has persisted the
	// Welcome bytes to pending_welcomes_out and replicated to store peers.
	// The runtime keeps retrying direct delivery in the background.
	AddOpStatusWelcomeQueued = "welcome_queued"

	// AddOpStatusWelcomeDelivered means the invitee has acknowledged the
	// join handshake (groupJoinAckProtocol). Terminal success.
	AddOpStatusWelcomeDelivered = "welcome_delivered"

	// AddOpStatusFailed is terminal failure. failure_code/failure_message
	// describe the reason and gate manual retry via a fresh operation_id.
	AddOpStatusFailed = "failed"
)

// addOpStatusOrder ranks each lifecycle status so transitions can be enforced
// as monotonically non-decreasing (except for terminal failed). A status with
// higher rank may not be overwritten by a lower rank.
var addOpStatusOrder = map[string]int{
	AddOpStatusApproved:          1,
	AddOpStatusProposalBroadcast: 2,
	AddOpStatusCommitObserved:    3,
	AddOpStatusWelcomeQueued:     4,
	AddOpStatusWelcomeDelivered:  5,
	AddOpStatusFailed:            99,
}

// ErrAddOperationTerminal is returned when a forward transition would move
// a terminal-status operation backwards (e.g. retrying a row already marked
// welcome_delivered or failed).
var ErrAddOperationTerminal = errors.New("group_add_operations: terminal status")

// GroupAddOperationRecord mirrors one row of the group_add_operations table.
type GroupAddOperationRecord struct {
	OperationID      string
	RequestID        string
	GroupID          string
	TargetPeerID     string
	RequesterPeerID  string
	ApproverPeerID   string
	KeyPackageHash   string
	ProposalEpoch    sql.NullInt64
	CommitEpoch      sql.NullInt64
	CommitHash       []byte
	WelcomeHash      string
	Status           string
	FailureCode      string
	FailureMessage   string
	CreatedAt        int64
	UpdatedAt        int64
	LastAttemptAt    int64
}

// CreateGroupAddOperation inserts a new operation row. The caller MUST pre-compute
// a stable operation_id (see service.ComputeAddOperationID) so retries on the
// same (group, target, KP hash) reuse the row instead of forking lifecycle.
// Returns the persisted row (re-fetched after insert/upsert).
func (d *Database) CreateGroupAddOperation(rec GroupAddOperationRecord) (*GroupAddOperationRecord, error) {
	rec.OperationID = strings.TrimSpace(rec.OperationID)
	rec.GroupID = strings.TrimSpace(rec.GroupID)
	rec.TargetPeerID = strings.TrimSpace(rec.TargetPeerID)
	rec.KeyPackageHash = strings.TrimSpace(rec.KeyPackageHash)
	if rec.OperationID == "" || rec.GroupID == "" || rec.TargetPeerID == "" || rec.KeyPackageHash == "" {
		return nil, fmt.Errorf("CreateGroupAddOperation: required fields missing")
	}
	status := strings.TrimSpace(rec.Status)
	if status == "" {
		status = AddOpStatusApproved
	}
	if _, ok := addOpStatusOrder[status]; !ok {
		return nil, fmt.Errorf("CreateGroupAddOperation: invalid status %q", status)
	}
	now := time.Now().Unix()
	if rec.CreatedAt <= 0 {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now

	// INSERT OR IGNORE so concurrent approvals on the same (group, target,
	// kp hash) don't error out — the first writer wins; subsequent callers
	// see the existing row via GetGroupAddOperation below.
	_, err := d.Conn.Exec(
		`INSERT OR IGNORE INTO group_add_operations (
			operation_id, request_id, group_id, target_peer_id,
			requester_peer_id, approver_peer_id, key_package_hash,
			status, created_at, updated_at, last_attempt_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.OperationID, rec.RequestID, rec.GroupID, rec.TargetPeerID,
		rec.RequesterPeerID, rec.ApproverPeerID, rec.KeyPackageHash,
		status, rec.CreatedAt, rec.UpdatedAt, rec.LastAttemptAt,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateGroupAddOperation: %w", err)
	}

	return d.GetGroupAddOperation(rec.OperationID)
}

// GetGroupAddOperation fetches one row by operation_id.
func (d *Database) GetGroupAddOperation(operationID string) (*GroupAddOperationRecord, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return nil, sql.ErrNoRows
	}
	var rec GroupAddOperationRecord
	err := d.Conn.QueryRow(
		`SELECT operation_id, request_id, group_id, target_peer_id,
		        requester_peer_id, approver_peer_id, key_package_hash,
		        proposal_epoch, commit_epoch, commit_hash, welcome_hash,
		        status, failure_code, failure_message,
		        created_at, updated_at, last_attempt_at
		 FROM group_add_operations
		 WHERE operation_id = ?`,
		operationID,
	).Scan(
		&rec.OperationID, &rec.RequestID, &rec.GroupID, &rec.TargetPeerID,
		&rec.RequesterPeerID, &rec.ApproverPeerID, &rec.KeyPackageHash,
		&rec.ProposalEpoch, &rec.CommitEpoch, &rec.CommitHash, &rec.WelcomeHash,
		&rec.Status, &rec.FailureCode, &rec.FailureMessage,
		&rec.CreatedAt, &rec.UpdatedAt, &rec.LastAttemptAt,
	)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// FindActiveGroupAddOperation returns the live row (any status except failed
// / welcome_delivered) keyed by (group_id, target_peer_id, key_package_hash).
// Returns sql.ErrNoRows if none exists.
func (d *Database) FindActiveGroupAddOperation(groupID, targetPeerID, keyPackageHash string) (*GroupAddOperationRecord, error) {
	var rec GroupAddOperationRecord
	err := d.Conn.QueryRow(
		`SELECT operation_id, request_id, group_id, target_peer_id,
		        requester_peer_id, approver_peer_id, key_package_hash,
		        proposal_epoch, commit_epoch, commit_hash, welcome_hash,
		        status, failure_code, failure_message,
		        created_at, updated_at, last_attempt_at
		 FROM group_add_operations
		 WHERE group_id = ? AND target_peer_id = ? AND key_package_hash = ?
		   AND status NOT IN (?, ?)
		 ORDER BY created_at DESC, operation_id DESC
		 LIMIT 1`,
		strings.TrimSpace(groupID), strings.TrimSpace(targetPeerID), strings.TrimSpace(keyPackageHash),
		AddOpStatusFailed, AddOpStatusWelcomeDelivered,
	).Scan(
		&rec.OperationID, &rec.RequestID, &rec.GroupID, &rec.TargetPeerID,
		&rec.RequesterPeerID, &rec.ApproverPeerID, &rec.KeyPackageHash,
		&rec.ProposalEpoch, &rec.CommitEpoch, &rec.CommitHash, &rec.WelcomeHash,
		&rec.Status, &rec.FailureCode, &rec.FailureMessage,
		&rec.CreatedAt, &rec.UpdatedAt, &rec.LastAttemptAt,
	)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// transitionAddOperation moves an operation forward to newStatus iff the
// existing rank is strictly lower (terminal statuses block backward moves).
// Optional fields are applied atomically with the status change.
func (d *Database) transitionAddOperation(operationID, newStatus string, mut func(setters *addOpUpdate)) error {
	operationID = strings.TrimSpace(operationID)
	newStatus = strings.TrimSpace(newStatus)
	newRank, ok := addOpStatusOrder[newStatus]
	if operationID == "" || !ok {
		return fmt.Errorf("transitionAddOperation: invalid args (op=%q status=%q)", operationID, newStatus)
	}

	upd := addOpUpdate{}
	if mut != nil {
		mut(&upd)
	}

	parts := []string{"status = ?", "updated_at = ?"}
	args := []any{newStatus, time.Now().Unix()}
	if upd.proposalEpochSet {
		parts = append(parts, "proposal_epoch = ?")
		args = append(args, upd.proposalEpoch)
	}
	if upd.commitEpochSet {
		parts = append(parts, "commit_epoch = ?")
		args = append(args, upd.commitEpoch)
	}
	if upd.commitHashSet {
		parts = append(parts, "commit_hash = ?")
		args = append(args, upd.commitHash)
	}
	if upd.welcomeHashSet {
		parts = append(parts, "welcome_hash = ?")
		args = append(args, upd.welcomeHash)
	}
	if upd.failureCodeSet {
		parts = append(parts, "failure_code = ?")
		args = append(args, upd.failureCode)
	}
	if upd.failureMessageSet {
		parts = append(parts, "failure_message = ?")
		args = append(args, upd.failureMessage)
	}
	if upd.bumpLastAttempt {
		parts = append(parts, "last_attempt_at = ?")
		args = append(args, time.Now().Unix())
	}

	// Build dynamic WHERE for the forward-only invariant. We compare the
	// stored status against the rank table to keep the SQL portable.
	// Allowed prior statuses: anything ranked < newRank (Failed is rank 99
	// so cannot be overridden by anything except Failed itself).
	allowedPrior := make([]string, 0, len(addOpStatusOrder))
	for s, r := range addOpStatusOrder {
		if r < newRank {
			allowedPrior = append(allowedPrior, s)
		}
	}
	if newStatus == AddOpStatusFailed {
		// Failed may be set from any non-terminal status (i.e. anything
		// except welcome_delivered or another failed row).
		allowedPrior = allowedPrior[:0]
		for s, r := range addOpStatusOrder {
			if r < addOpStatusOrder[AddOpStatusWelcomeDelivered] {
				allowedPrior = append(allowedPrior, s)
			}
		}
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(allowedPrior)), ",")
	query := fmt.Sprintf(
		`UPDATE group_add_operations SET %s WHERE operation_id = ? AND status IN (%s)`,
		strings.Join(parts, ", "), placeholders,
	)
	args = append(args, operationID)
	for _, s := range allowedPrior {
		args = append(args, s)
	}

	res, err := d.Conn.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("transitionAddOperation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("transitionAddOperation rows: %w", err)
	}
	if n == 0 {
		// Either the row no longer exists or it has already reached a higher rank.
		existing, gErr := d.GetGroupAddOperation(operationID)
		if gErr != nil {
			return ErrAddOperationTerminal
		}
		if addOpStatusOrder[existing.Status] >= newRank {
			return ErrAddOperationTerminal
		}
		return fmt.Errorf("transitionAddOperation: no row updated for %q (current=%q)", operationID, existing.Status)
	}
	return nil
}

type addOpUpdate struct {
	proposalEpoch     sql.NullInt64
	proposalEpochSet  bool
	commitEpoch       sql.NullInt64
	commitEpochSet    bool
	commitHash        []byte
	commitHashSet     bool
	welcomeHash       string
	welcomeHashSet    bool
	failureCode       string
	failureCodeSet    bool
	failureMessage    string
	failureMessageSet bool
	bumpLastAttempt   bool
}

// MarkAddProposalBroadcast records that the local node broadcast a ProposalAdd
// for this operation at the given epoch.
func (d *Database) MarkAddProposalBroadcast(operationID string, proposalEpoch uint64) error {
	return d.transitionAddOperation(operationID, AddOpStatusProposalBroadcast, func(u *addOpUpdate) {
		u.proposalEpochSet = true
		u.proposalEpoch = sql.NullInt64{Int64: int64(proposalEpoch), Valid: true}
		u.bumpLastAttempt = true
	})
}

// MarkAddCommitObserved records that an MLS Commit referencing this operation
// has been processed locally at commitEpoch. commitHash may be empty when the
// caller does not have the commit bytes (rare but allowed).
func (d *Database) MarkAddCommitObserved(operationID string, commitEpoch uint64, commitHash []byte) error {
	return d.transitionAddOperation(operationID, AddOpStatusCommitObserved, func(u *addOpUpdate) {
		u.commitEpochSet = true
		u.commitEpoch = sql.NullInt64{Int64: int64(commitEpoch), Valid: true}
		if len(commitHash) > 0 {
			u.commitHashSet = true
			u.commitHash = append([]byte(nil), commitHash...)
		}
	})
}

// MarkAddWelcomeQueued records that the Token Holder has persisted the
// Welcome to pending_welcomes_out and replicated to store peers.
func (d *Database) MarkAddWelcomeQueued(operationID string, welcomeHash string) error {
	return d.transitionAddOperation(operationID, AddOpStatusWelcomeQueued, func(u *addOpUpdate) {
		u.welcomeHashSet = true
		u.welcomeHash = welcomeHash
		u.bumpLastAttempt = true
	})
}

// MarkAddWelcomeDelivered records terminal success: the invitee acknowledged
// the join handshake.
func (d *Database) MarkAddWelcomeDelivered(operationID string) error {
	return d.transitionAddOperation(operationID, AddOpStatusWelcomeDelivered, nil)
}

// MarkAddOperationFailed records terminal failure with a structured reason.
func (d *Database) MarkAddOperationFailed(operationID, failureCode, failureMessage string) error {
	return d.transitionAddOperation(operationID, AddOpStatusFailed, func(u *addOpUpdate) {
		u.failureCodeSet = true
		u.failureCode = failureCode
		u.failureMessageSet = true
		u.failureMessage = failureMessage
		u.bumpLastAttempt = true
	})
}

// ListGroupAddOperationsForTarget returns all live operations (non-terminal)
// that target the given peer in the given group. Used by the join-ack handler
// to mark every matching operation as welcome_delivered, regardless of which
// KeyPackage hash was used (a peer may re-advertise its KP between attempts).
func (d *Database) ListGroupAddOperationsForTarget(groupID, targetPeerID string) ([]GroupAddOperationRecord, error) {
	rows, err := d.Conn.Query(
		`SELECT operation_id, request_id, group_id, target_peer_id,
		        requester_peer_id, approver_peer_id, key_package_hash,
		        proposal_epoch, commit_epoch, commit_hash, welcome_hash,
		        status, failure_code, failure_message,
		        created_at, updated_at, last_attempt_at
		 FROM group_add_operations
		 WHERE group_id = ? AND target_peer_id = ?
		   AND status NOT IN (?, ?)
		 ORDER BY created_at DESC`,
		strings.TrimSpace(groupID), strings.TrimSpace(targetPeerID),
		AddOpStatusFailed, AddOpStatusWelcomeDelivered,
	)
	if err != nil {
		return nil, fmt.Errorf("ListGroupAddOperationsForTarget: %w", err)
	}
	defer rows.Close()
	var out []GroupAddOperationRecord
	for rows.Next() {
		var rec GroupAddOperationRecord
		if err := rows.Scan(
			&rec.OperationID, &rec.RequestID, &rec.GroupID, &rec.TargetPeerID,
			&rec.RequesterPeerID, &rec.ApproverPeerID, &rec.KeyPackageHash,
			&rec.ProposalEpoch, &rec.CommitEpoch, &rec.CommitHash, &rec.WelcomeHash,
			&rec.Status, &rec.FailureCode, &rec.FailureMessage,
			&rec.CreatedAt, &rec.UpdatedAt, &rec.LastAttemptAt,
		); err != nil {
			return nil, fmt.Errorf("ListGroupAddOperationsForTarget scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ListRecoverableAddOperations returns operations whose lifecycle is mid-flight
// (proposal_broadcast / commit_observed / welcome_queued) and have not been
// touched within stallSeconds. Used by the runtime startup pass to retry
// Welcome delivery / re-broadcast stale proposals.
func (d *Database) ListRecoverableAddOperations(stallSeconds int64) ([]GroupAddOperationRecord, error) {
	if stallSeconds < 0 {
		stallSeconds = 0
	}
	cutoff := time.Now().Unix() - stallSeconds
	rows, err := d.Conn.Query(
		`SELECT operation_id, request_id, group_id, target_peer_id,
		        requester_peer_id, approver_peer_id, key_package_hash,
		        proposal_epoch, commit_epoch, commit_hash, welcome_hash,
		        status, failure_code, failure_message,
		        created_at, updated_at, last_attempt_at
		 FROM group_add_operations
		 WHERE status IN (?, ?, ?)
		   AND updated_at <= ?
		 ORDER BY updated_at ASC`,
		AddOpStatusProposalBroadcast,
		AddOpStatusCommitObserved,
		AddOpStatusWelcomeQueued,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("ListRecoverableAddOperations: %w", err)
	}
	defer rows.Close()
	var out []GroupAddOperationRecord
	for rows.Next() {
		var rec GroupAddOperationRecord
		if err := rows.Scan(
			&rec.OperationID, &rec.RequestID, &rec.GroupID, &rec.TargetPeerID,
			&rec.RequesterPeerID, &rec.ApproverPeerID, &rec.KeyPackageHash,
			&rec.ProposalEpoch, &rec.CommitEpoch, &rec.CommitHash, &rec.WelcomeHash,
			&rec.Status, &rec.FailureCode, &rec.FailureMessage,
			&rec.CreatedAt, &rec.UpdatedAt, &rec.LastAttemptAt,
		); err != nil {
			return nil, fmt.Errorf("ListRecoverableAddOperations scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// LinkInviteRequestToAddOperation records the operation_id on the invite
// request row so UI / API surfaces can correlate the two without an extra
// lookup. Idempotent: silently no-ops when the request row already points
// at the same operation, and refuses to overwrite a non-empty link with a
// different value.
func (d *Database) LinkInviteRequestToAddOperation(requestID, operationID string) error {
	requestID = strings.TrimSpace(requestID)
	operationID = strings.TrimSpace(operationID)
	if requestID == "" || operationID == "" {
		return fmt.Errorf("LinkInviteRequestToAddOperation: required fields missing")
	}
	res, err := d.Conn.Exec(
		`UPDATE group_invite_requests
		 SET add_operation_id = ?, updated_at = ?
		 WHERE request_id = ? AND (add_operation_id = '' OR add_operation_id = ?)`,
		operationID, time.Now().Unix(), requestID, operationID,
	)
	if err != nil {
		return fmt.Errorf("LinkInviteRequestToAddOperation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("LinkInviteRequestToAddOperation rows: %w", err)
	}
	if n == 0 {
		// Row may not exist (mirror not yet upserted) or already linked to
		// a different operation. Surface as a soft error so callers can decide.
		return fmt.Errorf("LinkInviteRequestToAddOperation: request_id %q either missing or already linked to a different operation", requestID)
	}
	return nil
}
