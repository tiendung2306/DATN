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
		best     peer.ID
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
// Each buffered proposal carries both the opaque MLS proposal bytes and the
// Go-level routing metadata required by the Token Holder to:
//   - know which ProposalAdd targets which invitee (so the freshly minted
//     Welcome can be delivered out-of-band to the correct peer), and
//   - emit AddCommitDelivery entries inside CommitMsg so non-holder nodes
//     observing the commit can transition their local group_add_operations
//     rows without seeing the Welcome.
//
// Thread-safe: all methods may be called concurrently.
type SingleWriter struct {
	mu         sync.Mutex
	activeView *ActiveView
	localID    peer.ID
	epoch      uint64
	proposals  []BufferedProposal // buffered proposals for the current epoch
	cfg        *CoordinatorConfig
	groupID    string
	authorized AuthorizedCommittersProvider
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

func (sw *SingleWriter) SetAuthorizedCommitters(groupID string, provider AuthorizedCommittersProvider) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.groupID = groupID
	sw.authorized = provider
}

// IsTokenHolder returns true if the local node is the Token Holder for the
// current epoch. Returns false if the active view is empty.
func (sw *SingleWriter) IsTokenHolder() bool {
	sw.mu.Lock()
	epoch := sw.epoch
	groupID := sw.groupID
	provider := sw.authorized
	batch := sw.peekNextBatchLocked()
	sw.mu.Unlock()

	members := sw.activeView.Members()
	holder, err := sw.computeHolder(members, epoch, groupID, batch, provider)
	if err != nil {
		return false
	}
	return holder == sw.localID
}

// CurrentTokenHolder returns the Token Holder for the current epoch.
func (sw *SingleWriter) CurrentTokenHolder() (peer.ID, error) {
	sw.mu.Lock()
	epoch := sw.epoch
	groupID := sw.groupID
	provider := sw.authorized
	batch := sw.peekNextBatchLocked()
	sw.mu.Unlock()

	return sw.computeHolder(sw.activeView.Members(), epoch, groupID, batch, provider)
}

// BufferProposal adds an MLS Proposal to the internal buffer. The proposal's
// raw MLS bytes are defensively copied; routing metadata fields are stored as
// provided. These are collected by the Token Holder via DrainNextBatch /
// DrainProposals.
func (sw *SingleWriter) BufferProposal(p BufferedProposal) {
	cp := BufferedProposal{
		Type:         p.Type,
		Data:         append([]byte(nil), p.Data...),
		OperationID:  p.OperationID,
		TargetPeerID: p.TargetPeerID,
		RequestID:    p.RequestID,
		GroupType:    p.GroupType,
		CategoryID:   p.CategoryID,
	}
	if len(p.KeyPackageHash) > 0 {
		cp.KeyPackageHash = append([]byte(nil), p.KeyPackageHash...)
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.proposals) < sw.cfg.MaxBatchedProposals {
		sw.proposals = append(sw.proposals, cp)
	}
}

// DrainNextBatch returns the next homogeneous batch of buffered proposals
// (all of the same ProposalType, preserving insertion order) and removes
// them from the buffer. Any remaining proposals of other types stay buffered
// for future epochs.
//
// Homogeneous batching is a deliberate trade-off: RFC 9420 permits mixed
// proposal commits, but crypto-engine/src/mls.rs currently funnels Add /
// Remove / Update through dedicated paths. Splitting the batch in Go avoids
// silent proposal drops while keeping the Rust commit path simple.
//
// Returns nil when no proposals are buffered.
func (sw *SingleWriter) DrainNextBatch() []BufferedProposal {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.proposals) == 0 {
		return nil
	}

	headType := sw.proposals[0].Type
	cutoff := 0
	for cutoff < len(sw.proposals) && sw.proposals[cutoff].Type == headType {
		cutoff++
	}

	batch := sw.proposals[:cutoff]
	sw.proposals = append([]BufferedProposal(nil), sw.proposals[cutoff:]...)
	return batch
}

func (sw *SingleWriter) PeekNextBatch() []BufferedProposal {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return cloneProposalBatch(sw.peekNextBatchLocked())
}

func (sw *SingleWriter) peekNextBatchLocked() []BufferedProposal {
	if len(sw.proposals) == 0 {
		return nil
	}
	headType := sw.proposals[0].Type
	cutoff := 0
	for cutoff < len(sw.proposals) && sw.proposals[cutoff].Type == headType {
		cutoff++
	}
	return sw.proposals[:cutoff]
}

// DrainProposals returns every buffered proposal and clears the buffer.
//
// Prefer DrainNextBatch when calling into mls.CreateCommit so the Token
// Holder commits homogeneous batches per epoch. DrainProposals remains
// available for callers that need to inspect or migrate the full buffer.
func (sw *SingleWriter) DrainProposals() []BufferedProposal {
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

func (sw *SingleWriter) computeHolder(activeView []peer.ID, epoch uint64, groupID string, batch []BufferedProposal, provider AuthorizedCommittersProvider) (peer.ID, error) {
	eligible := activeView
	if provider != nil {
		authorized, err := provider(groupID, epoch, cloneProposalBatch(batch))
		if err != nil {
			return "", err
		}
		eligible = intersectPeerIDs(activeView, authorized)
	}
	eligible = filterRemovedByBatch(eligible, batch)
	if groupID == "" {
		return ComputeTokenHolder(eligible, epoch)
	}
	return computeTokenHolderScoped(groupID, eligible, epoch)
}

func computeTokenHolderScoped(groupID string, activeView []peer.ID, epoch uint64) (peer.ID, error) {
	if len(activeView) == 0 {
		return "", ErrNoActiveView
	}
	var epochBuf [8]byte
	binary.LittleEndian.PutUint64(epochBuf[:], epoch)
	var (
		best     peer.ID
		bestHash [sha256.Size]byte
		first    = true
	)
	for _, pid := range activeView {
		h := sha256.New()
		h.Write([]byte(groupID))
		h.Write(epochBuf[:])
		h.Write([]byte(pid))
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

func intersectPeerIDs(active, authorized []peer.ID) []peer.ID {
	if len(active) == 0 || len(authorized) == 0 {
		return nil
	}
	allowed := make(map[peer.ID]struct{}, len(authorized))
	for _, pid := range authorized {
		if pid != "" {
			allowed[pid] = struct{}{}
		}
	}
	out := make([]peer.ID, 0, len(active))
	for _, pid := range active {
		if _, ok := allowed[pid]; ok {
			out = append(out, pid)
		}
	}
	return out
}

func filterRemovedByBatch(candidates []peer.ID, batch []BufferedProposal) []peer.ID {
	if len(candidates) == 0 || len(batch) == 0 {
		return candidates
	}
	removed := make(map[peer.ID]struct{})
	for _, p := range batch {
		if p.Type != ProposalRemove || p.TargetPeerID == "" {
			continue
		}
		pid, err := peer.Decode(p.TargetPeerID)
		if err == nil && pid != "" {
			removed[pid] = struct{}{}
		}
	}
	if len(removed) == 0 {
		return candidates
	}
	out := make([]peer.ID, 0, len(candidates))
	for _, pid := range candidates {
		if _, ok := removed[pid]; !ok {
			out = append(out, pid)
		}
	}
	return out
}

func cloneProposalBatch(in []BufferedProposal) []BufferedProposal {
	if len(in) == 0 {
		return nil
	}
	out := make([]BufferedProposal, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Data = append([]byte(nil), in[i].Data...)
		out[i].KeyPackageHash = append([]byte(nil), in[i].KeyPackageHash...)
	}
	return out
}
