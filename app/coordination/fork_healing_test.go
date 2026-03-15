package coordination

import (
	"testing"
)

func TestCompareBranchWeight_SameTreeHash(t *testing.T) {
	a := GroupStateAnnouncement{TreeHash: []byte("same"), MemberCount: 3, CommitHash: []byte("abc")}
	b := GroupStateAnnouncement{TreeHash: []byte("same"), MemberCount: 5, CommitHash: []byte("xyz")}

	if CompareBranchWeight(a, b) != BranchEqual {
		t.Error("same TreeHash should return BranchEqual regardless of other fields")
	}
}

func TestCompareBranchWeight_MoreMembers_Wins(t *testing.T) {
	local := GroupStateAnnouncement{TreeHash: []byte("aa"), MemberCount: 5, CommitHash: []byte("aaa")}
	remote := GroupStateAnnouncement{TreeHash: []byte("bb"), MemberCount: 3, CommitHash: []byte("aaa")}

	if CompareBranchWeight(local, remote) != BranchLocal {
		t.Error("local with more members should win")
	}
	if CompareBranchWeight(remote, local) != BranchRemote {
		t.Error("remote with more members should win")
	}
}

func TestCompareBranchWeight_SameMembers_LowerCommitHash_Wins(t *testing.T) {
	local := GroupStateAnnouncement{TreeHash: []byte("aa"), MemberCount: 3, CommitHash: []byte{0x01}}
	remote := GroupStateAnnouncement{TreeHash: []byte("bb"), MemberCount: 3, CommitHash: []byte{0x02}}

	if CompareBranchWeight(local, remote) != BranchLocal {
		t.Error("local with lower commit hash should win when member count is equal")
	}
}

func TestCompareBranchWeight_FinalTiebreaker_TreeHash(t *testing.T) {
	local := GroupStateAnnouncement{TreeHash: []byte{0x01}, MemberCount: 3, CommitHash: []byte{0x01}}
	remote := GroupStateAnnouncement{TreeHash: []byte{0x02}, MemberCount: 3, CommitHash: []byte{0x01}}

	if CompareBranchWeight(local, remote) != BranchLocal {
		t.Error("when everything else is equal, lower TreeHash should win")
	}
}

func TestForkDetector_NoFork(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 3,
		CommitHash:  []byte("commit-1"),
	})

	event := fd.ProcessRemote(peerID("peer-1"), 5, GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 3,
		CommitHash:  []byte("commit-1"),
	})

	if event != nil {
		t.Error("same TreeHash should not produce a fork event")
	}
}

func TestForkDetector_DetectsFork(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 3,
		CommitHash:  []byte("commit-1"),
	})

	event := fd.ProcessRemote(peerID("peer-1"), 5, GroupStateAnnouncement{
		TreeHash:    []byte("hash-B"),
		MemberCount: 5,
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
}

func TestForkDetector_LocalWins(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("hash-A"),
		MemberCount: 5,
		CommitHash:  []byte("commit-1"),
	})

	event := fd.ProcessRemote(peerID("peer-1"), 3, GroupStateAnnouncement{
		TreeHash:    []byte("hash-B"),
		MemberCount: 2,
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

	event := fd.ProcessRemote(peerID("peer-1"), 5, GroupStateAnnouncement{
		TreeHash:    []byte("hash-B"),
		MemberCount: 3,
		CommitHash:  []byte("commit-1"),
	})

	if event != nil {
		t.Error("no local set -> no fork event")
	}
}

func TestForkDetector_KnownBranches(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash: []byte("hash-A"), MemberCount: 3, CommitHash: []byte("c1"),
	})

	fd.ProcessRemote(peerID("p1"), 5, GroupStateAnnouncement{
		TreeHash: []byte("hash-B"), MemberCount: 2, CommitHash: []byte("c2"),
	})
	fd.ProcessRemote(peerID("p2"), 5, GroupStateAnnouncement{
		TreeHash: []byte("hash-C"), MemberCount: 1, CommitHash: []byte("c3"),
	})
	fd.ProcessRemote(peerID("p3"), 5, GroupStateAnnouncement{
		TreeHash: []byte("hash-B"), MemberCount: 2, CommitHash: []byte("c2"),
	})

	if fd.KnownBranches() != 2 {
		t.Errorf("expected 2 unique branches (B and C), got %d", fd.KnownBranches())
	}
}

func TestForkDetector_Reset(t *testing.T) {
	fd := NewForkDetector()
	fd.UpdateLocal(GroupStateAnnouncement{
		TreeHash: []byte("h-A"), MemberCount: 3, CommitHash: []byte("c1"),
	})
	fd.ProcessRemote(peerID("p1"), 5, GroupStateAnnouncement{
		TreeHash: []byte("h-B"), MemberCount: 2, CommitHash: []byte("c2"),
	})

	fd.Reset()

	if fd.KnownBranches() != 0 {
		t.Error("Reset should clear all known branches")
	}
}
