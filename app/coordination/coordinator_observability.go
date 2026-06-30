package coordination

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (c *Coordinator) CurrentEpoch() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch
}

func (c *Coordinator) ActiveMembers() []peer.ID {
	return c.activeView.Members()
}

func (c *Coordinator) IsTokenHolder() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.singleWriter == nil {
		return false
	}
	return c.singleWriter.IsTokenHolder()
}

// CurrentTokenHolder returns the PeerID of the elected Token Holder for the current epoch.
func (c *Coordinator) CurrentTokenHolder() (peer.ID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.singleWriter == nil {
		return "", fmt.Errorf("singleWriter not initialized")
	}
	return c.singleWriter.CurrentTokenHolder()
}

func (c *Coordinator) GetMetrics() MetricsSnapshot {
	return c.metrics.Snapshot()
}

func (c *Coordinator) GetGroupState() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]byte, len(c.groupState))
	copy(cp, c.groupState)
	return cp
}

// GetTreeHash returns a copy of the latest local tree hash snapshot.
func (c *Coordinator) GetTreeHash() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]byte, len(c.treeHash))
	copy(cp, c.treeHash)
	return cp
}

// GetHistoryHash returns a copy of the current HistoryHash R(E).
func (c *Coordinator) GetHistoryHash() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return copyBytes(c.historyHash)
}

// SeedHistoryAnchor seeds the history chain with an anchor received from the
// Token Holder during Welcome join. This must be called before Start so the
// joiner's R(epoch) matches the committer's R(epoch) at the joined epoch.
func (c *Coordinator) SeedHistoryAnchor(epoch uint64, anchorHistoryHash []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.historyChain == nil {
		c.historyChain = make(map[uint64][]byte)
	}
	c.historyChain[epoch] = copyBytes(anchorHistoryHash)
	c.historyHash = copyBytes(anchorHistoryHash)
	c.epoch = epoch
}

// GetOperationalMode returns the current operational mode of the coordinator.
func (c *Coordinator) GetOperationalMode() GroupOperationalMode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.operationalMode
}

// SetOperationalMode updates the operational mode of the coordinator.
func (c *Coordinator) SetOperationalMode(mode GroupOperationalMode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.operationalMode = mode
}

// IsHealing reports whether a fork-heal goroutine is currently in flight.
// Exported for tests and runtime diagnostics.
func (c *Coordinator) IsHealing() bool {
	return c.healing.Load()
}

// heartbeatLoop sends a liveness heartbeat at HeartbeatInterval. Runs on its
// own goroutine so its cadence is independent of the announce loop — tests can
// drive each timer in isolation via the FakeClock without coupling.
func (c *Coordinator) heartbeatLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(c.cfg.HeartbeatInterval):
			c.mu.Lock()
			c.activeView.CheckLiveness()
			c.broadcastLocked(MsgHeartbeat, HeartbeatMsg{})
			c.mu.Unlock()
		}
	}
}

// announceLoop broadcasts a GroupStateAnnouncement at AnnounceInterval. It is
// only spawned when AnnounceInterval > 0; tests using TestConfig (interval=0)
// drive announces manually via BroadcastAnnounce.
func (c *Coordinator) announceLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(c.cfg.AnnounceInterval):
			c.mu.Lock()
			c.broadcastAnnounceLocked()
			c.mu.Unlock()
		}
	}
}

// broadcastAnnounceLocked publishes a GroupStateAnnouncement reflecting the
// node's current TreeHash, member view size, and last commit hash. Caller must
// hold c.mu.
func (c *Coordinator) broadcastAnnounceLocked() {
	if c.groupState == nil || c.healing.Load() {
		return
	}
	ann := GroupStateAnnouncement{
		TreeHash:    c.treeHash,
		MemberCount: c.activeView.Size(),
		Epoch:       c.epoch,
		CommitHash:  copyBytes(c.lastCommitHash),
		HistoryHash: copyBytes(c.historyHash),
	}
	c.forkDetector.UpdateLocal(ann)
	c.broadcastLocked(MsgAnnounce, ann)
}

// BroadcastHeartbeat sends a heartbeat immediately. Used in tests to avoid
// depending on timer goroutines.
func (c *Coordinator) BroadcastHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.broadcastLocked(MsgHeartbeat, HeartbeatMsg{})
}

// BroadcastAnnounce sends a GroupStateAnnouncement immediately.
func (c *Coordinator) BroadcastAnnounce() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.broadcastAnnounceLocked()
}

// TriggerLivenessCheck runs a liveness check immediately. Returns evicted peers.
func (c *Coordinator) TriggerLivenessCheck() []peer.ID {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.activeView.CheckLiveness()
}
