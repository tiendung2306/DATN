package store

import (
	"strings"
	"testing"
	"time"
)

func TestGroupInviteRequest_ActiveUniqueConstraint(t *testing.T) {
	d := setupTestDB(t)
	now := time.Now().Unix()
	base := GroupInviteRequestRecord{
		GroupID:         "g-1",
		RequesterPeerID: "peer-a",
		TargetPeerID:    "peer-b",
		Status:          InviteRequestStatusPending,
		MaxAttempts:     5,
		ExpiresAt:       now + 3600,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	first := base
	first.RequestID = "req-1"
	if err := d.CreateGroupInviteRequest(first); err != nil {
		t.Fatalf("CreateGroupInviteRequest first: %v", err)
	}
	second := base
	second.RequestID = "req-2"
	if err := d.CreateGroupInviteRequest(second); err == nil {
		t.Fatalf("expected unique constraint error for duplicate active request")
	} else if !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("expected unique constraint error, got: %v", err)
	}
}

func TestGroupInviteRequest_ExpirePending(t *testing.T) {
	d := setupTestDB(t)
	now := time.Now().Unix()
	if err := d.CreateGroupInviteRequest(GroupInviteRequestRecord{
		RequestID:       "req-expire",
		GroupID:         "g-1",
		RequesterPeerID: "peer-a",
		TargetPeerID:    "peer-b",
		Status:          InviteRequestStatusPending,
		MaxAttempts:     5,
		ExpiresAt:       now - 5,
		CreatedAt:       now - 10,
		UpdatedAt:       now - 10,
	}); err != nil {
		t.Fatalf("CreateGroupInviteRequest: %v", err)
	}
	changed, err := d.ExpirePendingInviteRequests(now)
	if err != nil {
		t.Fatalf("ExpirePendingInviteRequests: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed=%d want 1", changed)
	}
	rec, err := d.GetGroupInviteRequest("req-expire")
	if err != nil {
		t.Fatalf("GetGroupInviteRequest: %v", err)
	}
	if rec.Status != InviteRequestStatusExpired {
		t.Fatalf("status=%q want expired", rec.Status)
	}
}

func TestGroupInviteRequest_ExpirePendingDoesNotTouchMirrors(t *testing.T) {
	d := setupTestDB(t)
	now := time.Now().Unix()
	mirror := GroupInviteRequestRecord{
		RequestID:       "req-mirror-exp",
		GroupID:         "g-1",
		RequesterPeerID: "peer-a",
		TargetPeerID:    "peer-c",
		Status:          InviteRequestStatusPending,
		MaxAttempts:     5,
		ExpiresAt:       now - 5,
		CreatedAt:       now - 10,
		UpdatedAt:       now - 10,
		IsMirror:        true,
	}
	if err := d.UpsertGroupInviteRequestMirror(mirror); err != nil {
		t.Fatalf("UpsertGroupInviteRequestMirror: %v", err)
	}
	changed, err := d.ExpirePendingInviteRequests(now)
	if err != nil {
		t.Fatalf("ExpirePendingInviteRequests: %v", err)
	}
	if changed != 0 {
		t.Fatalf("changed=%d want 0 (mirror skipped)", changed)
	}
	rec, err := d.GetGroupInviteRequest("req-mirror-exp")
	if err != nil {
		t.Fatalf("GetGroupInviteRequest: %v", err)
	}
	if rec.Status != InviteRequestStatusPending {
		t.Fatalf("status=%q want pending", rec.Status)
	}
}
