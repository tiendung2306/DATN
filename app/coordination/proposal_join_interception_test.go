package coordination

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestCoordinator_ProposalJoinInterception(t *testing.T) {
	nodes, _, _ := setupCluster(t, 2, "test-group-join")
	winner := nodes[0]

	// Configure a long BatchingDelay to prevent the Token Holder from automatically
	// executing the commit during our assertions.
	winner.coord.cfg.BatchingDelay = 10 * time.Second

	// Initialize and start coordinator correctly
	if err := winner.coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if err := winner.coord.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Simulate the scenario where Carol's credential exists in the MLS tree (zombie leaf cleanup needed)
	winner.mls.SetHasMemberFunc(func(_ []byte, _ []byte) (bool, error) {
		return true, nil
	})

	// Simulate receiving a MsgProposal of type ProposalJoin from Carol
	joinProp := ProposalMsg{
		ProposalType:   ProposalJoin,
		Data:           []byte("join-data"),
		ProposalRef:    []byte("join-ref"),
		TargetPeerID:   "carol",
		TargetIdentity: []byte("carol-identity"),
	}
	propBytes, _ := json.Marshal(joinProp)

	env := &Envelope{
		Type:    MsgProposal,
		Epoch:   winner.coord.CurrentEpoch(),
		From:    "carol",
		Payload: propBytes,
	}

	// BatchingDelay = 10s prevents the batch commit timer from firing during assertions.
	winner.coord.mu.Lock()
	winner.coord.handleProposalLocked(peer.ID("carol"), env)
	winner.coord.mu.Unlock()

	// Verify that the SingleWriter buffer now contains BOTH a ProposalRemove and a ProposalAdd
	winner.coord.mu.Lock()
	batch := winner.coord.singleWriter.SnapshotNextBatch()
	winner.coord.mu.Unlock()

	for i, p := range batch {
		t.Logf("Proposal[%d]: Type=%v, TargetPeerID=%s, OperationID=%s", i, p.Type, p.TargetPeerID, p.OperationID)
	}
	if len(batch) != 2 {
		t.Fatalf("Expected 2 proposals in buffer, got %d", len(batch))
	}

	hasRemove := false
	hasAdd := false
	for _, p := range batch {
		if p.Type == ProposalRemove && p.TargetPeerID == "carol" {
			hasRemove = true
		}
		if p.Type == ProposalAdd && p.TargetPeerID == "carol" {
			hasAdd = true
		}
	}

	if !hasRemove {
		t.Errorf("Expected buffer to contain ProposalRemove for zombie leaf")
	}
	if !hasAdd {
		t.Errorf("Expected buffer to contain transmuted ProposalAdd")
	}
}

// TestCoordinator_ProposalJoinInterception_SkipRemove verifies that when HasMember returns false
// (credential not found in tree), only the AddProposal is buffered — no Remove.
func TestCoordinator_ProposalJoinInterception_SkipRemove(t *testing.T) {
	nodes, _, _ := setupCluster(t, 2, "test-group-skip-remove")
	winner := nodes[0]

	winner.coord.cfg.BatchingDelay = 10 * time.Second

	if err := winner.coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if err := winner.coord.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// HasMember returns false — credential does NOT exist in tree
	winner.mls.SetHasMemberFunc(func(_ []byte, _ []byte) (bool, error) {
		return false, nil
	})

	joinProp := ProposalMsg{
		ProposalType:   ProposalJoin,
		Data:           []byte("join-data"),
		ProposalRef:    []byte("join-ref"),
		TargetPeerID:   "dave",
		TargetIdentity: []byte("dave-identity"),
	}
	propBytes, _ := json.Marshal(joinProp)

	env := &Envelope{
		Type:    MsgProposal,
		Epoch:   winner.coord.CurrentEpoch(),
		From:    "dave",
		Payload: propBytes,
	}

	// BatchingDelay = 10s prevents the batch commit timer from firing during assertions.
	winner.coord.mu.Lock()
	winner.coord.handleProposalLocked(peer.ID("dave"), env)
	winner.coord.mu.Unlock()

	winner.coord.mu.Lock()
	batch := winner.coord.singleWriter.SnapshotNextBatch()
	winner.coord.mu.Unlock()

	// Only AddProposal should be buffered (Remove skipped because member not in tree)
	if len(batch) != 1 {
		t.Fatalf("Expected 1 proposal (Add only) in buffer, got %d", len(batch))
	}
	if batch[0].Type != ProposalAdd {
		t.Errorf("Expected buffer to contain ProposalAdd, got %v", batch[0].Type)
	}
	if batch[0].TargetPeerID != "dave" {
		t.Errorf("Expected Add proposal TargetPeerID dave, got %s", batch[0].TargetPeerID)
	}
}
