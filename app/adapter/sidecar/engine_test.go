package sidecar

import (
	"strings"
	"testing"
)

func TestRequireNonEmptyState_AcceptsNonEmptyState(t *testing.T) {
	state, err := requireNonEmptyState("CreateProposal", "new_group_state", []byte("state"))
	if err != nil {
		t.Fatalf("requireNonEmptyState returned error: %v", err)
	}
	if string(state) != "state" {
		t.Fatalf("state=%q want %q", state, "state")
	}
}

func TestRequireNonEmptyState_RejectsEmptyStateWithRebuildHint(t *testing.T) {
	_, err := requireNonEmptyState("CreateProposal", "new_group_state", nil)
	if err == nil {
		t.Fatal("expected error for empty state")
	}
	msg := err.Error()
	if !strings.Contains(msg, "CreateProposal") || !strings.Contains(msg, "new_group_state") {
		t.Fatalf("error %q missing rpc/field context", msg)
	}
	if !strings.Contains(msg, "cargo build") {
		t.Fatalf("error %q missing rebuild hint", msg)
	}
}
