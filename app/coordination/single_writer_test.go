package coordination

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestComputeTokenHolder_Deterministic(t *testing.T) {
	view := []peer.ID{peerID("alice"), peerID("bob"), peerID("carol")}

	h1, err := ComputeTokenHolder(view, 1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ComputeTokenHolder(view, 1)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("same input should produce same result: %s vs %s", h1, h2)
	}
}

func TestComputeTokenHolder_EmptyView(t *testing.T) {
	_, err := ComputeTokenHolder(nil, 1)
	if err != ErrNoActiveView {
		t.Errorf("expected ErrNoActiveView, got %v", err)
	}
}

func TestComputeTokenHolder_DifferentEpochRotates(t *testing.T) {
	view := []peer.ID{peerID("alice"), peerID("bob"), peerID("carol")}

	holders := make(map[peer.ID]int)
	for epoch := uint64(0); epoch < 100; epoch++ {
		h, err := ComputeTokenHolder(view, epoch)
		if err != nil {
			t.Fatal(err)
		}
		holders[h]++
	}

	// Over 100 epochs with 3 peers, each peer should be elected at least once.
	// This is probabilistic but the hash function makes it practically guaranteed.
	for _, pid := range view {
		if holders[pid] == 0 {
			t.Errorf("peer %s was never elected over 100 epochs — hash function likely broken", pid)
		}
	}
}

func TestComputeTokenHolder_OrderIndependent(t *testing.T) {
	view1 := []peer.ID{peerID("alice"), peerID("bob"), peerID("carol")}
	view2 := []peer.ID{peerID("carol"), peerID("alice"), peerID("bob")}

	h1, _ := ComputeTokenHolder(view1, 42)
	h2, _ := ComputeTokenHolder(view2, 42)

	if h1 != h2 {
		t.Errorf("token holder should be independent of input order: %s vs %s", h1, h2)
	}
}

func TestSingleWriter_IsTokenHolder(t *testing.T) {
	clk := NewFakeClock(time.Now())
	cfg := TestConfig()
	av := NewActiveView(clk, cfg, peerID("alice"), nil)
	av.RecordHeartbeat(peerID("bob"))
	av.RecordHeartbeat(peerID("carol"))

	epoch := uint64(1)
	expected, _ := ComputeTokenHolder(av.Members(), epoch)

	sw := NewSingleWriter(av, expected, epoch, cfg)
	if !sw.IsTokenHolder() {
		t.Error("local node should be token holder when it matches the election")
	}

	// Create a writer for a non-holder
	nonHolder := peerID("alice")
	if nonHolder == expected {
		nonHolder = peerID("bob")
	}
	sw2 := NewSingleWriter(av, nonHolder, epoch, cfg)
	if sw2.IsTokenHolder() {
		t.Error("non-holder should not be token holder")
	}
}

func TestSingleWriter_BufferAndDrain(t *testing.T) {
	clk := NewFakeClock(time.Now())
	cfg := TestConfig()
	av := NewActiveView(clk, cfg, peerID("alice"), nil)
	sw := NewSingleWriter(av, peerID("alice"), 1, cfg)

	sw.BufferProposal([]byte("proposal-1"))
	sw.BufferProposal([]byte("proposal-2"))

	if sw.ProposalCount() != 2 {
		t.Errorf("expected 2 proposals, got %d", sw.ProposalCount())
	}

	drained := sw.DrainProposals()
	if len(drained) != 2 {
		t.Errorf("drained should have 2, got %d", len(drained))
	}
	if sw.ProposalCount() != 0 {
		t.Error("buffer should be empty after drain")
	}
	if sw.DrainProposals() != nil {
		t.Error("second drain should return nil")
	}
}

func TestSingleWriter_BufferRespectsCap(t *testing.T) {
	clk := NewFakeClock(time.Now())
	cfg := TestConfig()
	cfg.MaxBatchedProposals = 2
	av := NewActiveView(clk, cfg, peerID("alice"), nil)
	sw := NewSingleWriter(av, peerID("alice"), 1, cfg)

	sw.BufferProposal([]byte("1"))
	sw.BufferProposal([]byte("2"))
	sw.BufferProposal([]byte("3")) // should be dropped

	if sw.ProposalCount() != 2 {
		t.Errorf("buffer should respect MaxBatchedProposals, got %d", sw.ProposalCount())
	}
}

func TestSingleWriter_AdvanceEpoch(t *testing.T) {
	clk := NewFakeClock(time.Now())
	cfg := TestConfig()
	av := NewActiveView(clk, cfg, peerID("alice"), nil)
	sw := NewSingleWriter(av, peerID("alice"), 1, cfg)

	sw.BufferProposal([]byte("proposal-1"))
	sw.AdvanceEpoch(2)

	if sw.Epoch() != 2 {
		t.Errorf("epoch should be 2, got %d", sw.Epoch())
	}
	if sw.ProposalCount() != 0 {
		t.Error("proposals should be cleared on epoch advance")
	}
}

func TestSingleWriter_BufferProposal_DefensiveCopy(t *testing.T) {
	clk := NewFakeClock(time.Now())
	cfg := TestConfig()
	av := NewActiveView(clk, cfg, peerID("alice"), nil)
	sw := NewSingleWriter(av, peerID("alice"), 1, cfg)

	data := []byte("original")
	sw.BufferProposal(data)
	data[0] = 'X' // mutate original

	drained := sw.DrainProposals()
	if drained[0][0] != 'o' {
		t.Error("BufferProposal should make a defensive copy")
	}
}
