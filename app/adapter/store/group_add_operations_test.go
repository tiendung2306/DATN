package store

import (
	"database/sql"
	"errors"
	"testing"
)

// fixtures keep test bodies focused on the lifecycle invariants we care about
// (idempotent create, monotonic transitions, terminal blocks).
const (
	gaoGroupID    = "grp-add-ops"
	gaoTargetID   = "12D3KooWKtargetAddOps0000000000"
	gaoApproverID = "12D3KooWKapproverAddOps000000000"
	gaoKPHash     = "deadbeefcafebabe"
)

func newAddOpRecord(operationID string, status string) GroupAddOperationRecord {
	return GroupAddOperationRecord{
		OperationID:    operationID,
		GroupID:        gaoGroupID,
		TargetPeerID:   gaoTargetID,
		ApproverPeerID: gaoApproverID,
		KeyPackageHash: gaoKPHash,
		Status:         status,
	}
}

func TestGroupAddOperations_CreateIsIdempotent(t *testing.T) {
	d := setupTestDB(t)

	first, err := d.CreateGroupAddOperation(newAddOpRecord("ga_first", AddOpStatusApproved))
	if err != nil {
		t.Fatalf("CreateGroupAddOperation first: %v", err)
	}
	if first.Status != AddOpStatusApproved {
		t.Fatalf("first.Status=%q want approved", first.Status)
	}
	if first.OperationID != "ga_first" {
		t.Fatalf("first.OperationID=%q want ga_first", first.OperationID)
	}

	// Re-inserting with the same operation_id MUST be a no-op (existing row
	// is returned). This is the path service-layer retries take after a
	// process restart or duplicate approval signal.
	again, err := d.CreateGroupAddOperation(newAddOpRecord("ga_first", AddOpStatusApproved))
	if err != nil {
		t.Fatalf("CreateGroupAddOperation re-insert: %v", err)
	}
	if again.OperationID != first.OperationID {
		t.Fatalf("re-insert created a different row: %q vs %q", again.OperationID, first.OperationID)
	}
	if again.CreatedAt != first.CreatedAt {
		t.Fatalf("re-insert mutated created_at: %d vs %d", again.CreatedAt, first.CreatedAt)
	}
}

func TestGroupAddOperations_ForwardOnlyTransitions(t *testing.T) {
	d := setupTestDB(t)
	_, err := d.CreateGroupAddOperation(newAddOpRecord("ga_seq", AddOpStatusApproved))
	if err != nil {
		t.Fatalf("CreateGroupAddOperation: %v", err)
	}

	if err := d.MarkAddProposalBroadcast("ga_seq", 7); err != nil {
		t.Fatalf("MarkAddProposalBroadcast: %v", err)
	}
	if err := d.MarkAddCommitObserved("ga_seq", 8, []byte("commit-hash")); err != nil {
		t.Fatalf("MarkAddCommitObserved: %v", err)
	}
	if err := d.MarkAddWelcomeQueued("ga_seq", "welcome-hex"); err != nil {
		t.Fatalf("MarkAddWelcomeQueued: %v", err)
	}

	rec, err := d.GetGroupAddOperation("ga_seq")
	if err != nil {
		t.Fatalf("GetGroupAddOperation: %v", err)
	}
	if rec.Status != AddOpStatusWelcomeQueued {
		t.Fatalf("final status=%q want welcome_queued", rec.Status)
	}
	if !rec.ProposalEpoch.Valid || rec.ProposalEpoch.Int64 != 7 {
		t.Fatalf("proposal_epoch=%v want 7", rec.ProposalEpoch)
	}
	if !rec.CommitEpoch.Valid || rec.CommitEpoch.Int64 != 8 {
		t.Fatalf("commit_epoch=%v want 8", rec.CommitEpoch)
	}
	if rec.WelcomeHash != "welcome-hex" {
		t.Fatalf("welcome_hash=%q want welcome-hex", rec.WelcomeHash)
	}

	// Backwards move: should be blocked because the row has already
	// advanced past commit_observed.
	err = d.MarkAddCommitObserved("ga_seq", 9, []byte("any"))
	if !errors.Is(err, ErrAddOperationTerminal) {
		t.Fatalf("expected ErrAddOperationTerminal moving back to commit_observed, got %v", err)
	}

	// Terminal welcome_delivered.
	if err := d.MarkAddWelcomeDelivered("ga_seq"); err != nil {
		t.Fatalf("MarkAddWelcomeDelivered: %v", err)
	}
	// Repeating the terminal transition must report terminal (so callers
	// know the row is already past the desired state).
	err = d.MarkAddWelcomeQueued("ga_seq", "welcome-hex-2")
	if !errors.Is(err, ErrAddOperationTerminal) {
		t.Fatalf("welcome_queued after terminal must be blocked, got %v", err)
	}
}

func TestGroupAddOperations_FailedBlocksFurtherTransitions(t *testing.T) {
	d := setupTestDB(t)
	_, err := d.CreateGroupAddOperation(newAddOpRecord("ga_fail", AddOpStatusApproved))
	if err != nil {
		t.Fatalf("CreateGroupAddOperation: %v", err)
	}
	if err := d.MarkAddOperationFailed("ga_fail", "ERR_TEST", "synthetic"); err != nil {
		t.Fatalf("MarkAddOperationFailed: %v", err)
	}

	rec, err := d.GetGroupAddOperation("ga_fail")
	if err != nil {
		t.Fatalf("GetGroupAddOperation: %v", err)
	}
	if rec.Status != AddOpStatusFailed {
		t.Fatalf("status=%q want failed", rec.Status)
	}
	if rec.FailureCode != "ERR_TEST" {
		t.Fatalf("failure_code=%q want ERR_TEST", rec.FailureCode)
	}

	// Cannot rescue a failed row by trying to move it forward — caller must
	// create a fresh operation_id instead.
	if err := d.MarkAddProposalBroadcast("ga_fail", 99); !errors.Is(err, ErrAddOperationTerminal) {
		t.Fatalf("forward transition after failure must be blocked, got %v", err)
	}
}

func TestGroupAddOperations_FindActiveDeduplicates(t *testing.T) {
	d := setupTestDB(t)

	rec1, err := d.CreateGroupAddOperation(newAddOpRecord("ga_active", AddOpStatusApproved))
	if err != nil {
		t.Fatalf("CreateGroupAddOperation: %v", err)
	}

	found, err := d.FindActiveGroupAddOperation(gaoGroupID, gaoTargetID, gaoKPHash)
	if err != nil {
		t.Fatalf("FindActiveGroupAddOperation: %v", err)
	}
	if found.OperationID != rec1.OperationID {
		t.Fatalf("FindActiveGroupAddOperation returned %q want %q", found.OperationID, rec1.OperationID)
	}

	// Terminal rows must not be returned (UI/recovery treat them as done).
	if err := d.MarkAddWelcomeDelivered(rec1.OperationID); err != nil {
		t.Fatalf("MarkAddWelcomeDelivered: %v", err)
	}
	_, err = d.FindActiveGroupAddOperation(gaoGroupID, gaoTargetID, gaoKPHash)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("FindActiveGroupAddOperation after terminal: err=%v want sql.ErrNoRows", err)
	}
}

func TestGroupAddOperations_ListRecoverable(t *testing.T) {
	d := setupTestDB(t)
	if _, err := d.CreateGroupAddOperation(newAddOpRecord("ga_a", AddOpStatusApproved)); err != nil {
		t.Fatalf("CreateGroupAddOperation a: %v", err)
	}
	if _, err := d.CreateGroupAddOperation(GroupAddOperationRecord{
		OperationID:    "ga_b",
		GroupID:        gaoGroupID,
		TargetPeerID:   gaoTargetID + "-2",
		KeyPackageHash: gaoKPHash + "-2",
		Status:         AddOpStatusApproved,
	}); err != nil {
		t.Fatalf("CreateGroupAddOperation b: %v", err)
	}
	if err := d.MarkAddProposalBroadcast("ga_a", 1); err != nil {
		t.Fatalf("MarkAddProposalBroadcast a: %v", err)
	}
	if err := d.MarkAddCommitObserved("ga_b", 2, nil); err != nil {
		t.Fatalf("MarkAddCommitObserved b: %v", err)
	}

	// stallSeconds=0 returns everything updated_at <= now (i.e. both rows).
	rows, err := d.ListRecoverableAddOperations(0)
	if err != nil {
		t.Fatalf("ListRecoverableAddOperations: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListRecoverableAddOperations len=%d want 2", len(rows))
	}

	// Terminal rows must be excluded.
	if err := d.MarkAddWelcomeDelivered("ga_a"); err != nil {
		t.Fatalf("MarkAddWelcomeDelivered: %v", err)
	}
	rows, err = d.ListRecoverableAddOperations(0)
	if err != nil {
		t.Fatalf("ListRecoverableAddOperations after terminal: %v", err)
	}
	if len(rows) != 1 || rows[0].OperationID != "ga_b" {
		t.Fatalf("recoverable list mismatch: %+v", rows)
	}
}

func TestGroupAddOperations_ListForTargetIgnoresTerminal(t *testing.T) {
	d := setupTestDB(t)

	if _, err := d.CreateGroupAddOperation(GroupAddOperationRecord{
		OperationID:    "ga_live",
		GroupID:        gaoGroupID,
		TargetPeerID:   gaoTargetID,
		KeyPackageHash: gaoKPHash,
		Status:         AddOpStatusApproved,
	}); err != nil {
		t.Fatalf("CreateGroupAddOperation live: %v", err)
	}
	if _, err := d.CreateGroupAddOperation(GroupAddOperationRecord{
		OperationID:    "ga_done",
		GroupID:        gaoGroupID,
		TargetPeerID:   gaoTargetID,
		KeyPackageHash: gaoKPHash + "-old",
		Status:         AddOpStatusApproved,
	}); err != nil {
		t.Fatalf("CreateGroupAddOperation done: %v", err)
	}
	if err := d.MarkAddWelcomeDelivered("ga_done"); err != nil {
		t.Fatalf("MarkAddWelcomeDelivered done: %v", err)
	}

	rows, err := d.ListGroupAddOperationsForTarget(gaoGroupID, gaoTargetID)
	if err != nil {
		t.Fatalf("ListGroupAddOperationsForTarget: %v", err)
	}
	if len(rows) != 1 || rows[0].OperationID != "ga_live" {
		t.Fatalf("active for target mismatch: %+v", rows)
	}
}
