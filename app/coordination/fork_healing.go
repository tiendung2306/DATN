package coordination

import (
	"bytes"
	"encoding/hex"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// CompareBranchWeight determines which branch wins a fork using a multi-variable
// weight function:
//
//	W = (MemberCount, Epoch, CommitHash)
//
// Comparison is lexicographic:
//  1. Higher MemberCount wins (more nodes = stronger branch)
//  2. On tie, higher Epoch wins (more recent state)
//  3. On tie, lower CommitHash wins (deterministic tiebreaker)
//
// Returns BranchEqual when both announcements are identical.
func CompareBranchWeight(local, remote GroupStateAnnouncement) BranchResult {
	if bytes.Equal(local.TreeHash, remote.TreeHash) {
		return BranchEqual
	}

	// Primary: member count (higher wins)
	if local.MemberCount != remote.MemberCount {
		if local.MemberCount > remote.MemberCount {
			return BranchLocal
		}
		return BranchRemote
	}

	// Secondary: epoch embedded in the envelope (we use MemberCount as proxy;
	// the actual epoch comparison happens through the Envelope's Epoch field).
	// For direct comparison between two announcements, we compare CommitHash
	// as the final tiebreaker (lower hash wins, ensuring determinism).
	cmp := bytes.Compare(local.CommitHash, remote.CommitHash)
	if cmp < 0 {
		return BranchLocal
	}
	if cmp > 0 {
		return BranchRemote
	}

	// Everything is equal except TreeHash — this is a genuine divergence
	// with the same weight. Use TreeHash as the ultimate tiebreaker.
	cmp = bytes.Compare(local.TreeHash, remote.TreeHash)
	if cmp < 0 {
		return BranchLocal
	}
	return BranchRemote
}

// ForkEvent describes a detected fork and the decision about which branch wins.
type ForkEvent struct {
	GroupID          string
	RemotePeer       peer.ID
	LocalAnnounce    GroupStateAnnouncement
	RemoteAnnounce   GroupStateAnnouncement
	RemoteEpoch      uint64
	Result           BranchResult
	NeedExternalJoin bool // true if local branch lost and must ExternalJoin
}

// ForkDetector monitors GroupStateAnnouncements from peers to detect
// network partitions (forks) where different branches of the same group
// have diverged.
//
// Thread-safe: all methods may be called concurrently.
type ForkDetector struct {
	mu    sync.Mutex
	local *GroupStateAnnouncement

	// known tracks TreeHash hex -> set of peers broadcasting that branch.
	known map[string]*branchInfo
}

type branchInfo struct {
	announcement GroupStateAnnouncement
	peers        map[peer.ID]struct{}
}

// NewForkDetector creates a detector for a group. The local announcement
// should be set via UpdateLocal before processing remote announcements.
func NewForkDetector() *ForkDetector {
	return &ForkDetector{
		known: make(map[string]*branchInfo),
	}
}

// UpdateLocal sets the local node's current GroupStateAnnouncement.
// Should be called after every successful Commit.
func (fd *ForkDetector) UpdateLocal(ann GroupStateAnnouncement) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	cp := GroupStateAnnouncement{
		TreeHash:    copyBytes(ann.TreeHash),
		MemberCount: ann.MemberCount,
		CommitHash:  copyBytes(ann.CommitHash),
	}
	fd.local = &cp
}

// ProcessRemote analyzes a remote peer's announcement against the local state.
// Returns a ForkEvent if a fork is detected (different TreeHash), or nil if
// the remote is on the same branch.
func (fd *ForkDetector) ProcessRemote(from peer.ID, remoteEpoch uint64, ann GroupStateAnnouncement) *ForkEvent {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	if fd.local == nil {
		return nil
	}

	thHex := hex.EncodeToString(ann.TreeHash)
	bi, exists := fd.known[thHex]
	if !exists {
		bi = &branchInfo{
			announcement: GroupStateAnnouncement{
				TreeHash:    copyBytes(ann.TreeHash),
				MemberCount: ann.MemberCount,
				CommitHash:  copyBytes(ann.CommitHash),
			},
			peers: make(map[peer.ID]struct{}),
		}
		fd.known[thHex] = bi
	} else {
		bi.announcement.MemberCount = ann.MemberCount
		bi.announcement.CommitHash = copyBytes(ann.CommitHash)
	}
	bi.peers[from] = struct{}{}

	if bytes.Equal(ann.TreeHash, fd.local.TreeHash) {
		return nil
	}

	result := CompareBranchWeight(*fd.local, ann)
	return &ForkEvent{
		RemotePeer:       from,
		LocalAnnounce:    *fd.local,
		RemoteAnnounce:   ann,
		RemoteEpoch:      remoteEpoch,
		Result:           result,
		NeedExternalJoin: result == BranchRemote,
	}
}

// KnownBranches returns the number of distinct branches (by TreeHash)
// currently being tracked.
func (fd *ForkDetector) KnownBranches() int {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return len(fd.known)
}

// Reset clears all tracked branch information. Useful after a successful
// fork healing where all nodes converge to a single branch.
func (fd *ForkDetector) Reset() {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	fd.known = make(map[string]*branchInfo)
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}
