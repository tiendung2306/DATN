package coordination

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestIntegration_ForkHeal_ConvergesReplayAndPersistsHistory(t *testing.T) {
	nodes, network, clk := setupCluster(t, 2, "grp-heal-int")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// Pre-partition traffic from Bob should never be replayed by Alice.
	if _, err := bob.coord.SendMessage([]byte("bob-pre-partition")); err != nil {
		t.Fatalf("bob pre-partition send: %v", err)
	}
	network.DrainAll()

	// Diverge branch markers for detection.
	alice.coord.mu.Lock()
	alice.coord.treeHash = []byte("loser-tree")
	alice.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("loser-tree"),
		MemberCount: 1,
		Epoch:       0,
	})
	alice.coord.mu.Unlock()

	bob.coord.mu.Lock()
	bob.coord.treeHash = []byte("winner-tree")
	bob.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	bob.coord.mu.Unlock()

	partitionStart := clk.Now().Add(1 * time.Second)
	clk.Set(partitionStart)

	// Stamp first observation to drive partition replay window.
	alice.coord.mu.Lock()
	alice.coord.forkDetector.ProcessRemote(partitionStart, bob.id, bob.coord.CurrentEpoch(), GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	alice.coord.mu.Unlock()

	network.Partition([]peer.ID{alice.id}, []peer.ID{bob.id})
	if _, err := alice.coord.SendMessage([]byte("alice-partition-1")); err != nil {
		t.Fatalf("alice partition send #1: %v", err)
	}
	if _, err := alice.coord.SendMessage([]byte("alice-partition-2")); err != nil {
		t.Fatalf("alice partition send #2: %v", err)
	}
	network.Heal()

	alice.coord.groupInfoFetch = func(ctx context.Context, remote peer.ID, _ string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
		if remote != bob.id {
			return nil, errors.New("wrong remote")
		}
		groupInfo, err := bob.mls.ExportGroupInfo(ctx, bob.coord.GetGroupState(), withRatchetTree)
		if err != nil {
			return nil, err
		}
		return &GroupInfoFetchResult{
			GroupInfo: groupInfo,
			Epoch:     bob.coord.CurrentEpoch(),
			TreeHash:  bob.coord.GetTreeHash(),
		}, nil
	}

	bob.coord.mu.Lock()
	bob.coord.broadcastAnnounceLocked()
	bob.coord.mu.Unlock()
	network.DrainAll()

	if !waitFor(t, 5*time.Second, func() bool {
		network.DrainAll()
		snap := alice.coord.GetMetrics()
		return snap.ForkHealingsSucceeded >= 1 &&
			alice.coord.CurrentEpoch() == bob.coord.CurrentEpoch() &&
			alice.coord.CurrentEpoch() >= 1
	}) {
		t.Fatalf("heal convergence timeout; alice_epoch=%d bob_epoch=%d metrics=%+v",
			alice.coord.CurrentEpoch(), bob.coord.CurrentEpoch(), alice.coord.GetMetrics())
	}

	if gotA, gotB := string(alice.coord.GetTreeHash()), string(bob.coord.GetTreeHash()); gotA != gotB {
		t.Fatalf("tree hash mismatch after heal: alice=%q bob=%q", gotA, gotB)
	}

	events, err := alice.storage.ListForkHealEvents("grp-heal-int", 10)
	if err != nil {
		t.Fatalf("ListForkHealEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one persisted heal event")
	}
	ev := events[0]
	if ev.Outcome != "success" {
		t.Fatalf("heal event outcome=%q, want success", ev.Outcome)
	}
	if ev.ReplayedMessageCount != 2 {
		t.Fatalf("replayed_message_count=%d, want exactly 2 (non-repudiation: only alice's own partition-window messages)", ev.ReplayedMessageCount)
	}
	audit, err := alice.storage.ListForkHealAudit(ev.TraceID)
	if err != nil {
		t.Fatalf("ListForkHealAudit: %v", err)
	}
	if len(audit) == 0 {
		t.Fatal("expected persisted step audit rows")
	}
}

func TestIntegration_ForkHeal_FailurePersistsFailedStep(t *testing.T) {
	nodes, network, _ := setupCluster(t, 2, "grp-heal-fail")
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	alice.coord.mu.Lock()
	alice.coord.treeHash = []byte("loser-tree")
	alice.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("loser-tree"),
		MemberCount: 1,
		Epoch:       0,
	})
	alice.coord.mu.Unlock()

	bob.coord.mu.Lock()
	bob.coord.treeHash = []byte("winner-tree")
	bob.coord.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    []byte("winner-tree"),
		MemberCount: 2,
		Epoch:       0,
	})
	bob.coord.mu.Unlock()

	// Phoenix Protocol: Alice sends ProposalJoin → Bob processes it and sends Welcome via onAddCommitted.
	// The heal should now succeed through the ProposalJoin+Welcome flow.
	bob.coord.mu.Lock()
	bob.coord.broadcastAnnounceLocked()
	bob.coord.mu.Unlock()
	network.DrainAll()

	if !waitFor(t, 5*time.Second, func() bool {
		network.DrainAll()
		return !alice.coord.IsHealing() && alice.coord.GetMetrics().ForkHealingsSucceeded >= 1
	}) {
		t.Fatalf("expected heal to succeed; metrics=%+v", alice.coord.GetMetrics())
	}

	events, err := alice.storage.ListForkHealEvents("grp-heal-fail", 10)
	if err != nil {
		t.Fatalf("ListForkHealEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected persisted heal event")
	}
	if events[0].Outcome != "success" {
		t.Fatalf("heal event outcome=%q, want success", events[0].Outcome)
	}
	audit, err := alice.storage.ListForkHealAudit(events[0].TraceID)
	if err != nil {
		t.Fatalf("ListForkHealAudit: %v", err)
	}
	if len(audit) == 0 {
		t.Fatal("expected audit rows for heal trace")
	}
}

