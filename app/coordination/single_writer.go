package coordination

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ComputeTokenHolder deterministically elects the Token Holder for a given
// epoch from the active view using:
//
//	argmin SHA-256(nodeID || epoch_number_le64)
//
// All nodes with the same active view and epoch MUST arrive at the same result.
// Returns ErrNoActiveView if the view is empty.
func ComputeTokenHolder(activeView []peer.ID, epoch uint64) (peer.ID, error) {
	if len(activeView) == 0 {
		return "", ErrNoActiveView
	}

	var epochBuf [8]byte
	binary.LittleEndian.PutUint64(epochBuf[:], epoch)

	var (
		best    peer.ID
		bestHash [sha256.Size]byte
		first    = true
	)
	for _, pid := range activeView {
		h := sha256.New()
		h.Write([]byte(pid))
		h.Write(epochBuf[:])
		var candidate [sha256.Size]byte
		h.Sum(candidate[:0])

		if first || hashLess(candidate, bestHash) {
			best = pid
			bestHash = candidate
			first = false
		}
	}
	return best, nil
}

// hashLess returns true if a < b in byte-lexicographic order.
func hashLess(a, b [sha256.Size]byte) bool {
	for i := 0; i < sha256.Size; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// SingleWriter manages the Single-Writer Protocol for one group.
//
// It tracks the current epoch, buffers incoming MLS Proposals, and determines
// whether the local node is the Token Holder who should create Commits.
//
// Thread-safe: all methods may be called concurrently.
type SingleWriter struct {
	mu         sync.Mutex
	activeView *ActiveView
	localID    peer.ID
	epoch      uint64
	proposals  [][]byte // buffered proposals for the current epoch
	cfg        *CoordinatorConfig
}

// NewSingleWriter creates a SingleWriter for the given group.
func NewSingleWriter(av *ActiveView, localID peer.ID, epoch uint64, cfg *CoordinatorConfig) *SingleWriter {
	return &SingleWriter{
		activeView: av,
		localID:    localID,
		epoch:      epoch,
		cfg:        cfg,
	}
}

// IsTokenHolder returns true if the local node is the Token Holder for the
// current epoch. Returns false if the active view is empty.
func (sw *SingleWriter) IsTokenHolder() bool {
	sw.mu.Lock()
	epoch := sw.epoch
	sw.mu.Unlock()

	members := sw.activeView.Members()
	holder, err := ComputeTokenHolder(members, epoch)
	if err != nil {
		return false
	}
	return holder == sw.localID
}

// CurrentTokenHolder returns the Token Holder for the current epoch.
func (sw *SingleWriter) CurrentTokenHolder() (peer.ID, error) {
	sw.mu.Lock()
	epoch := sw.epoch
	sw.mu.Unlock()

	return ComputeTokenHolder(sw.activeView.Members(), epoch)
}

// BufferProposal adds an MLS Proposal to the internal buffer.
// These are collected by the Token Holder via DrainProposals.
func (sw *SingleWriter) BufferProposal(proposal []byte) {
	cp := make([]byte, len(proposal))
	copy(cp, proposal)

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.proposals) < sw.cfg.MaxBatchedProposals {
		sw.proposals = append(sw.proposals, cp)
	}
}

// DrainProposals returns all buffered proposals and clears the buffer.
// Intended to be called only by the Token Holder before creating a Commit.
func (sw *SingleWriter) DrainProposals() [][]byte {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.proposals) == 0 {
		return nil
	}
	result := sw.proposals
	sw.proposals = nil
	return result
}

// ProposalCount returns the number of buffered proposals.
func (sw *SingleWriter) ProposalCount() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return len(sw.proposals)
}

// AdvanceEpoch updates the epoch and clears the proposal buffer.
// Called after a Commit is successfully processed.
func (sw *SingleWriter) AdvanceEpoch(newEpoch uint64) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.epoch = newEpoch
	sw.proposals = nil
}

// Epoch returns the current epoch.
func (sw *SingleWriter) Epoch() uint64 {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.epoch
}
