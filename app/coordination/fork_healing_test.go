package coordination

import (
	"testing"
	"time"
)

// fixedT is a deterministic timestamp used by ForkDetector unit tests where
// the actual partition timestamp is irrelevant to the assertion.
var fixedT = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestCompareBranchWeight_SameTreeHash(t *testing.T) {
	a := GroupStateAnnouncement{TreeHash: []byte("same"), MemberCount: 3, Epoch: 0, CommitHash: []byte("abc")}
	b := GroupStateAnnouncement{TreeHash: []byte("same"), MemberCount: 5, Epoch: 0, CommitHash: []byte("abc")}

	if CompareBranchWeight(a, b) != BranchEqual {
		t.Error("same TreeHash and CommitHash should return BranchEqual")
	}

	b.CommitHash = []byte("xyz")
	if CompareBranchWeight(a, b) == BranchEqual {
		t.Error("same TreeHash with different CommitHash is a fork and must not return BranchEqual")
	}
}

func TestCompareBranchWeight_MoreMembers_Wins(t *testing.T) {
	local := GroupStateAnnouncement{TreeHash: []byte("aa"), MemberCount: 5, Epoch: 0, CommitHash: []byte("commit-local")}
	remote := GroupStateAnnouncement{TreeHash: []byte("bb"), MemberCount: 3, Epoch: 0, CommitHash: []byte("commit-remote")}

	if CompareBranchWeight(local, remote) != BranchLocal {
		t.Error("local with more members should win")
	}
	if CompareBranchWeight(remote, local) != BranchRemote {
		t.Error("remote with more members should win")
	}
}

func TestCompareBranchWeight_SameMembers_LowerCommitHash_Wins(t *testing.T) {
	local := GroupStateAnnouncement{TreeHash: []byte("aa"), MemberCount: 3, Epoch: 0, CommitHash: []byte{0x01}}
	remote := GroupStateAnnouncement{TreeHash: []byte("bb"), MemberCount: 3, Epoch: 0, CommitHash: []byte{0x02}}

	if CompareBranchWeight(local, remote) != BranchLocal {
		t.Error("local with lower commit hash should win when member count is equal")
	}
}

func TestCompareBranchWeight_SameMembers_HigherEpoch_Wins(t *testing.T) {
	local := GroupStateAnnouncement{TreeHash: []byte("aa"), MemberCount: 3, Epoch: 10, CommitHash: []byte("commit-local")}
	remote := GroupStateAnnouncement{TreeHash: []byte("bb"), MemberCount: 3, Epoch: 20, CommitHash: []byte("commit-remote")}

	if CompareBranchWeight(local, remote) != BranchRemote {
		t.Error("remote with higher epoch should win when member count is equal")
	}
	if CompareBranchWeight(remote, local) != BranchLocal {
		t.Error("local with higher epoch should win when member count is equal")
	}
}

func TestCompareBranchWeight_SameMembersAndEpoch_LowerCommitHash_Wins(t *testing.T) {
	local := GroupStateAnnouncement{TreeHash: []byte("aa"), MemberCount: 3, Epoch: 10, CommitHash: []byte{0x01}}
	remote := GroupStateAnnouncement{TreeHash: []byte("bb"), MemberCount: 3, Epoch: 10, CommitHash: []byte{0x02}}

	if CompareBranchWeight(local, remote) != BranchLocal {
		t.Error("local with lower commit hash should win when member count and epoch are equal")
	}
}

func TestCompareBranchWeight_FinalTiebreaker_TreeHash(t *testing.T) {
	local := GroupStateAnnouncement{TreeHash: []byte{0x01}, MemberCount: 3, Epoch: 0}
	remote := GroupStateAnnouncement{TreeHash: []byte{0x02}, MemberCount: 3, Epoch: 0}

	if CompareBranchWeight(local, remote) != BranchLocal {
		t.Error("when everything else is equal, lower TreeHash should win")
	}
}

func TestForkDetector_NoFork(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 3,
		Epoch:       0,
		CommitHash:  []byte("commit-1"),
	})

	event := fd.ProcessRemote(fixedT, peerID("peer-1"), 5, GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 3,
		Epoch:       0,
		CommitHash:  []byte("commit-1"),
	})

	if event != nil {
		t.Error("same branch id should not produce a fork event")
	}
}

func TestForkDetector_BranchIdentityUsesCommitHash(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("synthetic-same-tree"),
		MemberCount: 3,
		Epoch:       2,
		CommitHash:  []byte("commit-local"),
	})

	event := fd.ProcessRemote(fixedT, peerID("peer-1"), 2, GroupStateAnnouncement{
		TreeHash:    []byte("synthetic-same-tree"),
		MemberCount: 3,
		Epoch:       2,
		CommitHash:  []byte("commit-remote"),
	})
	if event == nil {
		t.Fatal("different CommitHash must produce a fork even when TreeHash matches")
	}
	if fd.KnownBranches() != 1 {
		t.Fatalf("known remote branches=%d, want 1", fd.KnownBranches())
	}

	fd.ProcessRemote(fixedT, peerID("peer-2"), 2, GroupStateAnnouncement{
		TreeHash:    []byte("synthetic-same-tree"),
		MemberCount: 3,
		Epoch:       2,
		CommitHash:  []byte("commit-third"),
	})
	if fd.KnownBranches() != 2 {
		t.Fatalf("same TreeHash with different CommitHash must be tracked as separate branches, got %d",
			fd.KnownBranches())
	}
}

func TestForkDetector_RemoteBranchSupportBeatsCommitHashTiebreaker(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("tree-local"),
		MemberCount: 3,
		Epoch:       2,
		CommitHash:  []byte{0x01},
	})

	remote := GroupStateAnnouncement{
		TreeHash:    []byte("tree-remote"),
		MemberCount: 3,
		Epoch:       2,
		CommitHash:  []byte{0xff},
	}
	if ev := fd.ProcessRemote(fixedT, peerID("peer-1"), 2, remote); ev == nil {
		t.Fatal("first remote observation should surface a fork")
	} else if ev.Result != BranchLocal {
		t.Fatalf("local lower commit hash should win at equal support, got %v", ev.Result)
	}

	ev := fd.ProcessRemote(fixedT.Add(time.Second), peerID("peer-2"), 2, remote)
	if ev == nil {
		t.Fatal("second remote observation should still surface the fork")
	}
	if ev.Result != BranchRemote {
		t.Fatalf("remote branch support=2 should beat local support=1 despite larger commit hash, got %v", ev.Result)
	}
	if !ev.NeedExternalJoin {
		t.Fatal("local node should heal toward the better-supported remote branch")
	}
}

func TestForkDetector_InvalidRemoteCommitCannotWin(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("tree-local"),
		MemberCount: 1,
		Epoch:       2,
		CommitHash:  []byte{0xf0},
	})

	invalidCommit := []byte{0x01}
	fd.MarkInvalidCommit(invalidCommit)

	event := fd.ProcessRemote(fixedT, peerID("peer-1"), 2, GroupStateAnnouncement{
		TreeHash:    []byte("tree-invalid"),
		MemberCount: 3,
		Epoch:       2,
		CommitHash:  invalidCommit,
	})
	if event == nil {
		t.Fatal("invalid branch is still a fork and should be reported")
	}
	if event.Result != BranchLocal {
		t.Fatalf("invalid remote commit must not win branch selection, got %v", event.Result)
	}
	if event.NeedExternalJoin {
		t.Fatal("local node must not heal into a branch whose commit was rejected as invalid")
	}
}

func TestForkDetector_DetectsFork(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 3,
		Epoch:       0,
		CommitHash:  []byte("commit-1"),
	})

	observed := fixedT.Add(7 * time.Second)
	event := fd.ProcessRemote(observed, peerID("peer-1"), 5, GroupStateAnnouncement{
		TreeHash:    []byte("hash-B"),
		MemberCount: 5,
		Epoch:       0,
		CommitHash:  []byte("commit-2"),
	})

	if event == nil {
		t.Fatal("different TreeHash should produce a fork event")
	}
	if event.Result != BranchRemote {
		t.Error("remote with 5 members should beat local with 3")
	}
	if !event.NeedExternalJoin {
		t.Error("losing branch should set NeedExternalJoin=true")
	}
	if !event.PartitionStartedAt.Equal(observed) {
		t.Errorf("PartitionStartedAt should equal first-observation time: got %v want %v",
			event.PartitionStartedAt, observed)
	}
}

func TestForkDetector_PartitionStartedAt_StableAcrossObservations(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 3,
		Epoch:       0,
		CommitHash:  []byte("commit-1"),
	})

	first := fixedT.Add(1 * time.Second)
	second := fixedT.Add(30 * time.Second)
	remoteAnn := GroupStateAnnouncement{
		TreeHash:    []byte("hash-B"),
		MemberCount: 5,
		Epoch:       0,
		CommitHash:  []byte("commit-2"),
	}

	ev1 := fd.ProcessRemote(first, peerID("peer-1"), 5, remoteAnn)
	ev2 := fd.ProcessRemote(second, peerID("peer-2"), 5, remoteAnn)

	if ev1 == nil || ev2 == nil {
		t.Fatal("expected both observations to surface fork events")
	}
	if !ev2.PartitionStartedAt.Equal(first) {
		t.Errorf("PartitionStartedAt should be sticky to first observation: got %v want %v",
			ev2.PartitionStartedAt, first)
	}
}

func TestForkDetector_LocalWins(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 5,
		Epoch:       0,
		CommitHash:  []byte("commit-1"),
	})

	event := fd.ProcessRemote(fixedT, peerID("peer-1"), 3, GroupStateAnnouncement{
		TreeHash:    []byte("hash-B"),
		MemberCount: 2,
		Epoch:       0,
		CommitHash:  []byte("commit-2"),
	})

	if event == nil {
		t.Fatal("expected fork event")
	}
	if event.Result != BranchLocal {
		t.Error("local with 5 members should beat remote with 2")
	}
	if event.NeedExternalJoin {
		t.Error("winning branch should not need ExternalJoin")
	}
}

func TestForkDetector_NoLocalSet(t *testing.T) {
	fd := NewForkDetector()

	event := fd.ProcessRemote(fixedT, peerID("peer-1"), 5, GroupStateAnnouncement{
		TreeHash:    []byte("hash-B"),
		MemberCount: 3,
		Epoch:       0,
		CommitHash:  []byte("commit-1"),
	})

	if event != nil {
		t.Error("no local set -> no fork event")
	}
}

func TestForkDetector_KnownBranches(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash: []byte("hash-A"), MemberCount: 3, Epoch: 0, CommitHash: []byte("c1"),
	})

	fd.ProcessRemote(fixedT, peerID("p1"), 5, GroupStateAnnouncement{
		TreeHash: []byte("hash-B"), MemberCount: 2, Epoch: 0, CommitHash: []byte("c2"),
	})
	fd.ProcessRemote(fixedT, peerID("p2"), 5, GroupStateAnnouncement{
		TreeHash: []byte("hash-C"), MemberCount: 1, Epoch: 0, CommitHash: []byte("c3"),
	})
	fd.ProcessRemote(fixedT, peerID("p3"), 5, GroupStateAnnouncement{
		TreeHash: []byte("hash-B"), MemberCount: 2, Epoch: 0, CommitHash: []byte("c2"),
	})

	if fd.KnownBranches() != 2 {
		t.Errorf("expected 2 unique branches (B and C), got %d", fd.KnownBranches())
	}
}

func TestForkDetector_Reset(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash: []byte("h-A"), MemberCount: 3, Epoch: 0, CommitHash: []byte("c1"),
	})
	fd.ProcessRemote(fixedT, peerID("p1"), 5, GroupStateAnnouncement{
		TreeHash: []byte("h-B"), MemberCount: 2, Epoch: 0, CommitHash: []byte("c2"),
	})

	fd.Reset()

	if fd.KnownBranches() != 0 {
		t.Error("Reset should clear all known branches")
	}
}
