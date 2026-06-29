package coordination

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ComputeTokenHolder deterministically elects the Token Holder for a given
// epoch from the active view using:
//
//	argmin SHA-256(groupID || epoch_number_le64 || nodeID)
//
// All nodes with the same groupID, active view, and epoch MUST arrive at the
// same result. Returns ErrNoActiveView if the view is empty.
func ComputeTokenHolder(groupID string, activeView []peer.ID, epoch uint64) (peer.ID, error) {
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
	suspended  map[peer.ID]struct{}
}

// NewSingleWriter creates a SingleWriter for the given group.
func NewSingleWriter(av *ActiveView, localID peer.ID, epoch uint64, cfg *CoordinatorConfig) *SingleWriter {
	return &SingleWriter{
		activeView: av,
		localID:    localID,
		epoch:      epoch,
		cfg:        cfg,
		suspended:  make(map[peer.ID]struct{}),
	}
}

func (sw *SingleWriter) SetAuthorizedCommitters(groupID string, provider AuthorizedCommittersProvider) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.groupID = groupID
	sw.authorized = provider
}

// Suspend temporarily excludes a peer from being elected as Token Holder
// for the remainder of the current epoch. The suspension is cleared automatically
// when the epoch advances.
func (sw *SingleWriter) Suspend(id peer.ID) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.suspended == nil {
		sw.suspended = make(map[peer.ID]struct{})
	}
	sw.suspended[id] = struct{}{}
}

func filterSuspended(candidates []peer.ID, suspended map[peer.ID]struct{}) []peer.ID {
	if len(suspended) == 0 || len(candidates) == 0 {
		return candidates
	}
	out := make([]peer.ID, 0, len(candidates))
	for _, pid := range candidates {
		if _, ok := suspended[pid]; !ok {
			out = append(out, pid)
		}
	}
	return out
}

// IsTokenHolder returns true if the local node is the Token Holder for the
// current epoch. Returns false if the active view is empty.
func (sw *SingleWriter) IsTokenHolder() bool {
	sw.mu.Lock()
	epoch := sw.epoch
	groupID := sw.groupID
	provider := sw.authorized
	batch := sw.peekNextBatchLocked()
	suspended := make(map[peer.ID]struct{}, len(sw.suspended))
	for k, v := range sw.suspended {
		suspended[k] = v
	}
	sw.mu.Unlock()

	members := sw.activeView.Members()
	holder, err := sw.computeHolder(members, epoch, groupID, batch, provider, suspended)
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
	suspended := make(map[peer.ID]struct{}, len(sw.suspended))
	for k, v := range sw.suspended {
		suspended[k] = v
	}
	sw.mu.Unlock()

	return sw.computeHolder(sw.activeView.Members(), epoch, groupID, batch, provider, suspended)
}

// HolderForBatch computes the token holder given a specific batch of proposals,
// applying the current epoch, active view, and suspensions.
func (sw *SingleWriter) HolderForBatch(batch []BufferedProposal) (peer.ID, error) {
	sw.mu.Lock()
	epoch := sw.epoch
	groupID := sw.groupID
	provider := sw.authorized
	suspended := make(map[peer.ID]struct{}, len(sw.suspended))
	for k, v := range sw.suspended {
		suspended[k] = v
	}
	sw.mu.Unlock()

	return sw.computeHolder(sw.activeView.Members(), epoch, groupID, batch, provider, suspended)
}

// BufferProposal adds an MLS Proposal to the internal buffer. The proposal's
// raw MLS bytes are defensively copied; routing metadata fields are stored as
// provided. These are collected by the Token Holder via SnapshotNextBatch /
// DrainProposals.
func (sw *SingleWriter) BufferProposal(p BufferedProposal) {
	cp := BufferedProposal{
		Type:         p.Type,
		Data:         append([]byte(nil), p.Data...),
		ProposalRef:  append([]byte(nil), p.ProposalRef...),
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

	if len(cp.ProposalRef) > 0 {
		for _, existing := range sw.proposals {
			if bytes.Equal(existing.ProposalRef, cp.ProposalRef) {
				return
			}
		}
	}
	if len(sw.proposals) >= sw.cfg.MaxBatchedProposals {
		return
	}
	sw.proposals = append(sw.proposals, cp)
}

// SnapshotNextBatch returns the deterministic candidate batch for this epoch.
// The batch is proposal-ref ordered so all nodes with the same pending set
// compute the same Token Holder without relying on local arrival order.
func (sw *SingleWriter) SnapshotNextBatch() []BufferedProposal {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return cloneProposalBatch(sw.snapshotNextBatchLocked())
}

func (sw *SingleWriter) snapshotNextBatchLocked() []BufferedProposal {
	if len(sw.proposals) == 0 {
		return nil
	}
	batch := cloneProposalBatch(sw.proposals)
	sort.SliceStable(batch, func(i, j int) bool {
		return proposalRefLess(batch[i], batch[j])
	})
	if len(batch) > sw.cfg.MaxBatchedProposals {
		batch = batch[:sw.cfg.MaxBatchedProposals]
	}
	return batch
}

func (sw *SingleWriter) PeekNextBatch() []BufferedProposal {
	return sw.SnapshotNextBatch()
}

func (sw *SingleWriter) peekNextBatchLocked() []BufferedProposal {
	return sw.snapshotNextBatchLocked()
}

// DrainBatchByRefs removes exactly the proposals committed by a successful MLS
// Commit. Proposals are only drained after Rust returns a valid commit.
func (sw *SingleWriter) DrainBatchByRefs(refs [][]byte) []BufferedProposal {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if len(refs) == 0 || len(sw.proposals) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		wanted[string(ref)] = struct{}{}
	}
	drained := make([]BufferedProposal, 0, len(refs))
	remaining := make([]BufferedProposal, 0, len(sw.proposals))
	for _, proposal := range sw.proposals {
		if _, ok := wanted[string(proposal.ProposalRef)]; ok {
			drained = append(drained, proposal)
			continue
		}
		remaining = append(remaining, proposal)
	}
	sw.proposals = remaining
	sort.SliceStable(drained, func(i, j int) bool {
		return proposalRefLess(drained[i], drained[j])
	})
	return cloneProposalBatch(drained)
}

// DrainBatchByData removes buffered proposals whose raw MLS proposal bytes
// match any entry in datas. This is a fallback for receiver nodes whose local
// ProposalRef (computed via mls.ProcessProposal against their own groupState)
// may differ from the ref computed by the Token Holder, while the raw proposal
// bytes are always identical because they came from the same broadcast message.
func (sw *SingleWriter) DrainBatchByData(datas [][]byte) []BufferedProposal {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if len(datas) == 0 || len(sw.proposals) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(datas))
	for _, d := range datas {
		wanted[string(d)] = struct{}{}
	}
	drained := make([]BufferedProposal, 0, len(datas))
	remaining := make([]BufferedProposal, 0, len(sw.proposals))
	for _, proposal := range sw.proposals {
		if _, ok := wanted[string(proposal.Data)]; ok {
			drained = append(drained, proposal)
			continue
		}
		remaining = append(remaining, proposal)
	}
	sw.proposals = remaining
	return cloneProposalBatch(drained)
}

// DrainProposals returns every buffered proposal and clears the buffer.
//
// Prefer SnapshotNextBatch + DrainBatchByRefs when calling into mls.CreateCommit.
// DrainProposals remains available for callers that need to inspect or migrate the full buffer.
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
	sw.suspended = make(map[peer.ID]struct{})
}

// Epoch returns the current epoch.
func (sw *SingleWriter) Epoch() uint64 {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.epoch
}

func (sw *SingleWriter) computeHolder(activeView []peer.ID, epoch uint64, groupID string, batch []BufferedProposal, provider AuthorizedCommittersProvider, suspended map[peer.ID]struct{}) (peer.ID, error) {
	eligible := activeView
	if provider != nil {
		authorized, err := provider(groupID, epoch, cloneProposalBatch(batch))
		if err != nil {
			return "", err
		}
		eligible = intersectPeerIDs(activeView, authorized)
	}
	eligible = filterRemovedByBatch(eligible, batch)
	eligible = filterSuspended(eligible, suspended)
	return ComputeTokenHolder(groupID, eligible, epoch)
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
	removed := make(map[string]struct{})
	for _, p := range batch {
		if p.Type != ProposalRemove || p.TargetPeerID == "" {
			continue
		}
		removed[p.TargetPeerID] = struct{}{}
	}
	if len(removed) == 0 {
		return candidates
	}
	out := make([]peer.ID, 0, len(candidates))
	for _, pid := range candidates {
		if _, ok := removed[pid.String()]; !ok {
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
		out[i].ProposalRef = append([]byte(nil), in[i].ProposalRef...)
		out[i].KeyPackageHash = append([]byte(nil), in[i].KeyPackageHash...)
	}
	return out
}

func proposalRefLess(a, b BufferedProposal) bool {
	if len(a.ProposalRef) == 0 || len(b.ProposalRef) == 0 {
		if len(a.ProposalRef) != len(b.ProposalRef) {
			return len(a.ProposalRef) > len(b.ProposalRef)
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return bytes.Compare(a.Data, b.Data) < 0
	}
	return bytes.Compare(a.ProposalRef, b.ProposalRef) < 0
}
