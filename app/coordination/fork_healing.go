package coordination

import (
	"bytes"
	"encoding/hex"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// CompareBranchWeight determines which branch wins a fork using deterministic
// tie-breakers. Runtime fork selection should prefer compareBranchWeightWithSupport
// so branch quorum/support is considered before cryptographic tiebreakers.
func CompareBranchWeight(local, remote GroupStateAnnouncement) BranchResult {
	return compareBranchWeightWithSupport(local, remote, 1, 1, false, false)
}

func compareBranchWeightWithSupport(local, remote GroupStateAnnouncement, localSupport, remoteSupport int, localInvalid, remoteInvalid bool) BranchResult {
	if sameBranch(local, remote) {
		return BranchEqual
	}

	if localInvalid != remoteInvalid {
		if localInvalid {
			return BranchRemote
		}
		return BranchLocal
	}

	if localSupport != remoteSupport {
		if localSupport > remoteSupport {
			return BranchLocal
		}
		return BranchRemote
	}

	if local.MemberCount != remote.MemberCount {
		if local.MemberCount > remote.MemberCount {
			return BranchLocal
		}
		return BranchRemote
	}

	if local.Epoch != remote.Epoch {
		if local.Epoch > remote.Epoch {
			return BranchLocal
		}
		return BranchRemote
	}

	cmp := bytes.Compare(local.CommitHash, remote.CommitHash)
	if cmp < 0 {
		return BranchLocal
	}
	if cmp > 0 {
		return BranchRemote
	}

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

	// WinnerPeers is the set of all peers observed to be on the winning branch
	// at the time the fork was detected. The triggering peer (RemotePeer) is
	// always included. Used by runHeal to retry GroupInfo fetch across multiple
	// peers if the first one is unreachable at heal time.
	WinnerPeers []peer.ID

	// PartitionStartedAt is the wall-clock time at which the local node first
	// observed the divergent TreeHash from the *winning* remote branch. Used
	// by Autonomous Replay (Sprint 2E) to determine the partition window of
	// own messages that must be re-encrypted under the healed group state.
	// Zero value if no winning remote branch was tracked when the event fired.
	PartitionStartedAt time.Time
}

// ForkDetector monitors GroupStateAnnouncements from peers to detect
// network partitions (forks) where different branches of the same group
// have diverged.
//
// Thread-safe: all methods may be called concurrently.
type ForkDetector struct {
	mu    sync.Mutex
	local *GroupStateAnnouncement

	// known tracks BranchID -> set of peers broadcasting that branch.
	known map[string]*branchInfo

	// invalidCommits tracks commit hashes rejected by MLS or Single-Writer
	// validation so fork heal never selects a locally-known-invalid branch.
	invalidCommits map[string]struct{}
}

type branchInfo struct {
	announcement GroupStateAnnouncement
	peers        map[peer.ID]struct{}
	// firstSeenAt is the wall-clock time of the first ProcessRemote call that
	// surfaced this TreeHash. It is preserved across subsequent observations
	// so we know exactly when the branch became visible to the local node —
	// crucial input for Autonomous Replay's partition window computation.
	firstSeenAt time.Time
}

// NewForkDetector creates a detector for a group. The local announcement
// should be set via UpdateLocal before processing remote announcements.
func NewForkDetector() *ForkDetector {
	return &ForkDetector{
		known:          make(map[string]*branchInfo),
		invalidCommits: make(map[string]struct{}),
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
		Epoch:       ann.Epoch,
		CommitHash:  copyBytes(ann.CommitHash),
	}
	fd.local = &cp
}

// ProcessRemote analyzes a remote peer's announcement against the local state.
// Returns a ForkEvent if a fork is detected (different BranchID), or nil if
// the remote is on the same branch.
//
// observedAt should be the local clock reading at the moment the announcement
// was received. It is recorded as firstSeenAt for new branches only; subsequent
// announcements for the same BranchID do not bump it.
func (fd *ForkDetector) ProcessRemote(observedAt time.Time, from peer.ID, remoteEpoch uint64, ann GroupStateAnnouncement) *ForkEvent {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	if fd.local == nil {
		return nil
	}

	key := branchKey(ann)
	bi, exists := fd.known[key]
	if !exists {
		bi = &branchInfo{
			announcement: GroupStateAnnouncement{
				TreeHash:    copyBytes(ann.TreeHash),
				MemberCount: ann.MemberCount,
				Epoch:       ann.Epoch,
				CommitHash:  copyBytes(ann.CommitHash),
			},
			peers:       make(map[peer.ID]struct{}),
			firstSeenAt: observedAt,
		}
		fd.known[key] = bi
	} else {
		bi.announcement.MemberCount = ann.MemberCount
		bi.announcement.Epoch = ann.Epoch
		bi.announcement.CommitHash = copyBytes(ann.CommitHash)
		bi.announcement.TreeHash = copyBytes(ann.TreeHash)
	}
	bi.peers[from] = struct{}{}

	if sameBranch(ann, *fd.local) {
		return nil
	}

	localSupport := fd.branchSupportLocked(*fd.local)
	remoteSupport := len(bi.peers)
	result := compareBranchWeightWithSupport(
		*fd.local,
		ann,
		localSupport,
		remoteSupport,
		fd.isInvalidCommitLocked(fd.local.CommitHash),
		fd.isInvalidCommitLocked(ann.CommitHash),
	)

	// Collect all known peers on the winning branch so runHeal can retry across
	// them if the triggering peer is unreachable at heal time.
	winnerPeers := make([]peer.ID, 0, len(bi.peers))
	// Put the triggering peer first — most likely to still be reachable.
	winnerPeers = append(winnerPeers, from)
	for p := range bi.peers {
		if p != from {
			winnerPeers = append(winnerPeers, p)
		}
	}

	return &ForkEvent{
		RemotePeer:         from,
		LocalAnnounce:      *fd.local,
		RemoteAnnounce:     ann,
		RemoteEpoch:        remoteEpoch,
		Result:             result,
		NeedExternalJoin:   result == BranchRemote,
		WinnerPeers:        winnerPeers,
		PartitionStartedAt: bi.firstSeenAt,
	}
}

// KnownBranches returns the number of distinct remote branches (by BranchID)
// currently being tracked.
func (fd *ForkDetector) KnownBranches() int {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return len(fd.known)
}

// MarkInvalidCommit records a commit hash that failed local MLS or
// Single-Writer validation. Invalid branches may still be observed for audit,
// but they must never win fork-heal selection.
func (fd *ForkDetector) MarkInvalidCommit(commitHash []byte) {
	if len(commitHash) == 0 {
		return
	}
	fd.mu.Lock()
	defer fd.mu.Unlock()
	fd.invalidCommits[hex.EncodeToString(commitHash)] = struct{}{}
}

// Reset clears all tracked branch information. Useful after a successful
// fork healing where all nodes converge to a single branch.
func (fd *ForkDetector) Reset() {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	fd.known = make(map[string]*branchInfo)
}

func (fd *ForkDetector) branchSupportLocked(ann GroupStateAnnouncement) int {
	support := 1
	if bi, ok := fd.known[branchKey(ann)]; ok {
		support += len(bi.peers)
	}
	return support
}

func (fd *ForkDetector) isInvalidCommitLocked(commitHash []byte) bool {
	if len(commitHash) == 0 {
		return false
	}
	_, ok := fd.invalidCommits[hex.EncodeToString(commitHash)]
	return ok
}

func branchKey(ann GroupStateAnnouncement) string {
	if len(ann.CommitHash) > 0 {
		return "c:" + hex.EncodeToString(ann.CommitHash)
	}
	return "t:" + hex.EncodeToString(ann.TreeHash)
}

func sameBranch(a, b GroupStateAnnouncement) bool {
	return branchKey(a) == branchKey(b)
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}
