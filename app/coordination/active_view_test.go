package coordination

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func peerID(s string) peer.ID { return peer.ID(s) }

func TestActiveView_InitWithLocalOnly(t *testing.T) {
	clk := NewFakeClock(time.Now())
	av := NewActiveView(clk, TestConfig(), peerID("local"), nil)

	if av.Size() != 1 {
		t.Errorf("new view should have 1 member (local), got %d", av.Size())
	}
	if !av.Contains(peerID("local")) {
		t.Error("new view should contain local peer")
	}
}

func TestActiveView_HeartbeatAddsNewPeer(t *testing.T) {
	var notified bool
	clk := NewFakeClock(time.Now())
	av := NewActiveView(clk, TestConfig(), peerID("local"), func([]peer.ID) {
		notified = true
	})

	av.RecordHeartbeat(peerID("remote-1"))

	if !av.Contains(peerID("remote-1")) {
		t.Error("RecordHeartbeat should add peer")
	}
	if av.Size() != 2 {
		t.Errorf("size should be 2, got %d", av.Size())
	}
	if !notified {
		t.Error("onChange should fire when new peer is added")
	}
}

func TestActiveView_HeartbeatExistingPeer_NoCallback(t *testing.T) {
	callCount := 0
	clk := NewFakeClock(time.Now())
	av := NewActiveView(clk, TestConfig(), peerID("local"), func([]peer.ID) {
		callCount++
	})

	av.RecordHeartbeat(peerID("remote-1")) // first time -> callback
	av.RecordHeartbeat(peerID("remote-1")) // already exists -> no callback

	if callCount != 1 {
		t.Errorf("onChange should fire once for new peer, got %d", callCount)
	}
}

func TestActiveView_CheckLiveness_Eviction(t *testing.T) {
	cfg := TestConfig()
	cfg.PeerDeadAfter = 3
	clk := NewFakeClock(time.Now())

	var lastMembers []peer.ID
	av := NewActiveView(clk, cfg, peerID("local"), func(m []peer.ID) {
		lastMembers = m
	})

	av.RecordHeartbeat(peerID("remote-1"))

	av.CheckLiveness() // missed = 1
	av.CheckLiveness() // missed = 2
	evicted := av.CheckLiveness() // missed = 3 -> evicted

	if len(evicted) != 1 || evicted[0] != peerID("remote-1") {
		t.Errorf("should evict remote-1, got %v", evicted)
	}
	if av.Contains(peerID("remote-1")) {
		t.Error("evicted peer should not be in view")
	}
	if len(lastMembers) != 1 || lastMembers[0] != peerID("local") {
		t.Errorf("onChange should report only local after eviction, got %v", lastMembers)
	}
}

func TestActiveView_HeartbeatResetsMissedCounter(t *testing.T) {
	cfg := TestConfig()
	cfg.PeerDeadAfter = 3
	clk := NewFakeClock(time.Now())
	av := NewActiveView(clk, cfg, peerID("local"), nil)

	av.RecordHeartbeat(peerID("remote-1"))
	av.CheckLiveness() // missed = 1
	av.CheckLiveness() // missed = 2
	av.RecordHeartbeat(peerID("remote-1")) // reset
	evicted := av.CheckLiveness()          // missed = 1

	if len(evicted) != 0 {
		t.Errorf("peer should not be evicted after heartbeat reset, got %v", evicted)
	}
	if !av.Contains(peerID("remote-1")) {
		t.Error("peer should still be in view")
	}
}

func TestActiveView_LocalNeverEvicted(t *testing.T) {
	cfg := TestConfig()
	cfg.PeerDeadAfter = 1
	clk := NewFakeClock(time.Now())
	av := NewActiveView(clk, cfg, peerID("local"), nil)

	for i := 0; i < 10; i++ {
		av.CheckLiveness()
	}

	if !av.Contains(peerID("local")) {
		t.Error("local peer must never be evicted")
	}
}

func TestActiveView_Evict_Forced(t *testing.T) {
	clk := NewFakeClock(time.Now())
	av := NewActiveView(clk, TestConfig(), peerID("local"), nil)

	av.RecordHeartbeat(peerID("remote-1"))
	av.Evict(peerID("remote-1"))

	if av.Contains(peerID("remote-1")) {
		t.Error("Evict should remove peer")
	}
}

func TestActiveView_Evict_LocalIgnored(t *testing.T) {
	clk := NewFakeClock(time.Now())
	av := NewActiveView(clk, TestConfig(), peerID("local"), nil)

	av.Evict(peerID("local"))

	if !av.Contains(peerID("local")) {
		t.Error("cannot evict local peer")
	}
}

func TestActiveView_Members_Sorted(t *testing.T) {
	clk := NewFakeClock(time.Now())
	av := NewActiveView(clk, TestConfig(), peerID("z-local"), nil)

	av.RecordHeartbeat(peerID("c-peer"))
	av.RecordHeartbeat(peerID("a-peer"))
	av.RecordHeartbeat(peerID("b-peer"))

	members := av.Members()
	for i := 1; i < len(members); i++ {
		if members[i-1] >= members[i] {
			t.Errorf("members should be sorted: %v", members)
			break
		}
	}
}
