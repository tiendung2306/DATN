package coordination

import (
	"bytes"
	"encoding/json"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ReceiveDirectMessage applies a coordination envelope received on a libp2p
// direct stream. Wire format matches GossipSub (JSON Envelope); each
// coordinator ignores payloads for other groups.
func (c *Coordinator) ReceiveDirectMessage(from peer.ID, data []byte) {
	c.handleRawMessage(from, data)
}

func (c *Coordinator) handleRawMessage(from peer.ID, data []byte) {
	if from == c.localID {
		return
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	if env.GroupID != c.groupID {
		return
	}

	// P0.2 Clock Skew protection on receive:
	if env.Type == MsgCommit || env.Type == MsgApplication || env.Type == MsgProposal {
		nowMs := c.clock.Now().UnixMilli()
		if err := validateSenderTimestamp(nowMs, env.Timestamp.WallTimeMs); err != nil {
			slog.Warn("Rejected raw message due to invalid sender timestamp", "group", c.groupID, "from", from, "type", env.Type, "err", err)
			return // drop envelope entirely
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Inbox Isolation: Nếu đang trong quá trình Catching Up, cô lập hoàn toàn luồng Gossip live.
	// Chỉ append MsgCommit và MsgApplication vào SQLite (Envelope Log) ở trạng thái pending,
	// không mutate groupState (không handle commit/application ngay).
	if c.operationalMode == ModeCatchingUp {
		if env.Type == MsgCommit || env.Type == MsgApplication {
			if c.cfg != nil && c.cfg.OfflineSyncEnabled {
				_, err := c.storage.AppendEnvelopeWithSource(c.groupID, env.Type, env.Epoch, env.Timestamp, data, "gossip_catchup")
				if err != nil {
					slog.Warn("Gossip catchup: append envelope failed", "group", c.groupID, "err", err)
				} else {
					slog.Debug("Gossip catchup: buffered live envelope to DB inbox", "group", c.groupID, "type", env.Type, "epoch", env.Epoch)
				}
			}
		} else {
			c.handleControlMessageLocked(from, &env)
		}
		return
	}

	if c.operationalMode == ModeFrozenForApply {
		if env.Type == MsgCommit || env.Type == MsgApplication {
			_, err := c.storage.AppendEnvelope(c.groupID, env.Type, env.Epoch, env.Timestamp, data)
			if err != nil {
				slog.Warn("Gossip frozen: append envelope failed", "group", c.groupID, "err", err)
			} else {
				slog.Debug("Gossip frozen: buffered live envelope to DB inbox during healing", "group", c.groupID, "type", env.Type, "epoch", env.Epoch)
			}
		} else {
			c.handleControlMessageLocked(from, &env)
		}
		return
	}

	switch env.Type {
	case MsgHeartbeat:
		c.handleHeartbeatLocked(from)
	case MsgAnnounce:
		c.handleAnnounceLocked(from, &env)
	case MsgProposal:
		c.handleProposalLocked(from, &env)
	case MsgCommit:
		c.handleCommitLocked(&env, data)
	case MsgApplication:
		c.handleApplicationLocked(from, &env, data)
	case MsgApplicationBatched:
		c.handleApplicationBatchedLocked(from, &env, data)
	case MsgDeliveryAck:
		c.handleDeliveryAckLocked(from, &env)
	case MsgHistoryQuery:
		c.handleHistoryQueryLocked(from, &env)
	case MsgHistoryReply:
		c.handleHistoryReplyLocked(from, &env)
	}
}

// handleControlMessageLocked dispatches non-commit, non-application control
// messages shared between ModeCatchingUp and ModeFrozenForApply branches.
func (c *Coordinator) handleControlMessageLocked(from peer.ID, env *Envelope) {
	switch env.Type {
	case MsgHeartbeat:
		c.handleHeartbeatLocked(from)
	case MsgAnnounce:
		c.handleAnnounceLocked(from, env)
	case MsgDeliveryAck:
		c.handleDeliveryAckLocked(from, env)
	case MsgHistoryQuery:
		c.handleHistoryQueryLocked(from, env)
	case MsgHistoryReply:
		c.handleHistoryReplyLocked(from, env)
	}
}

func (c *Coordinator) handleHeartbeatLocked(from peer.ID) {
	c.observePeerAliveLocked(from)
}

func (c *Coordinator) observePeerAliveLocked(from peer.ID) {
	if from == "" || from == c.localID {
		return
	}
	fresh := c.activeView.RecordHeartbeat(from)
	if !fresh || c.onPeerObserved == nil {
		return
	}
	// Fire OnPeerObserved outside c.mu via goroutine so the service handler can
	// perform DB writes without blocking heartbeat processing. We capture the
	// observation timestamp now (under lock) so the handler sees the true
	// first-seen moment even if scheduling is delayed.
	cb := c.onPeerObserved
	groupID := c.groupID
	observedAt := c.clock.Now()
	go cb(groupID, from, observedAt)
}

// ObservePeerAlive lets the runtime feed already-authenticated transport
// observations into this group's ActiveView without waiting for the next
// group heartbeat. The runtime must only call this for peers that are both
// verified by the auth protocol and active members of this group.
func (c *Coordinator) ObservePeerAlive(from peer.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.observePeerAliveLocked(from)
}

func (c *Coordinator) handleAnnounceLocked(from peer.ID, env *Envelope) {
	var ann GroupStateAnnouncement
	if err := json.Unmarshal(env.Payload, &ann); err != nil {
		return
	}

	// Same epoch: compare HistoryHash directly.
	if ann.Epoch == c.epoch {
		if sameBranch(ann, GroupStateAnnouncement{HistoryHash: c.historyHash, Epoch: c.epoch}) {
			// Same branch — record support, no fork.
			c.forkDetector.ProcessRemote(c.clock.Now(), from, env.Epoch, ann)
			return
		}
		// Different HistoryHash at same epoch → fork.
		event := c.forkDetector.ProcessRemote(c.clock.Now(), from, env.Epoch, ann)
		if event == nil || !event.NeedExternalJoin {
			return
		}
		c.metrics.IncrPartitionsDetected()
		event.GroupID = c.groupID
		c.scheduleHeal(event)
		return
	}

	// Remote is ahead (higher epoch): query our epoch's HistoryHash to
	// determine if the remote diverged at or after our epoch.
	if ann.Epoch > c.epoch {
		c.sendHistoryQuery(from, c.epoch)
		// Still record the remote branch for tracking.
		c.forkDetector.ProcessRemote(c.clock.Now(), from, env.Epoch, ann)
		return
	}

	// Remote is behind (lower epoch): record support; the remote will
	// query us if it detects a potential fork.
	c.forkDetector.ProcessRemote(c.clock.Now(), from, env.Epoch, ann)
}

// sendHistoryQuery sends a MsgHistoryQuery to a peer asking for its R(epoch).
func (c *Coordinator) sendHistoryQuery(to peer.ID, epoch uint64) {
	msg := HistoryQueryMsg{Epoch: epoch}
	wire := c.buildEnvelopeWithTimestampLocked(MsgHistoryQuery, msg, c.hlc.Now())
	if len(wire) == 0 {
		slog.Warn("sendHistoryQuery: build envelope failed", "group", c.groupID)
		return
	}
	c.wg.Add(1)
	go func(pid peer.ID, w []byte) {
		defer c.wg.Done()
		if err := c.sendDirectEnvelope(pid, w); err != nil {
			slog.Debug("sendHistoryQuery: direct send failed", "group", c.groupID, "peer", pid, "err", err)
		}
	}(to, wire)
}

// handleHistoryQueryLocked responds to a history query from a peer.
func (c *Coordinator) handleHistoryQueryLocked(from peer.ID, env *Envelope) {
	var q HistoryQueryMsg
	if err := json.Unmarshal(env.Payload, &q); err != nil {
		return
	}
	var reply HistoryReplyMsg
	reply.Epoch = q.Epoch
	if h, ok := c.historyChain[q.Epoch]; ok {
		reply.HistoryHash = copyBytes(h)
		reply.Known = true
	} else {
		reply.Known = false
	}
	wire := c.buildEnvelopeWithTimestampLocked(MsgHistoryReply, reply, c.hlc.Now())
	if len(wire) == 0 {
		slog.Warn("handleHistoryQueryLocked: build envelope failed", "group", c.groupID)
		return
	}
	c.wg.Add(1)
	go func(pid peer.ID, w []byte) {
		defer c.wg.Done()
		if err := c.sendDirectEnvelope(pid, w); err != nil {
			slog.Debug("handleHistoryQueryLocked: direct send failed", "group", c.groupID, "peer", pid, "err", err)
		}
	}(from, wire)
}

// handleHistoryReplyLocked processes a history reply from a peer.
func (c *Coordinator) handleHistoryReplyLocked(from peer.ID, env *Envelope) {
	var r HistoryReplyMsg
	if err := json.Unmarshal(env.Payload, &r); err != nil {
		return
	}
	if !r.Known {
		// Peer doesn't have our epoch in its chain — can't determine
		// fork status from this peer. Let existing heuristics handle it.
		slog.Debug("handleHistoryReplyLocked: peer does not know epoch", "group", c.groupID, "peer", from, "epoch", r.Epoch)
		return
	}
	localHash, ok := c.historyChain[r.Epoch]
	if !ok {
		// We don't have this epoch either — nothing to compare.
		return
	}
	if bytes.Equal(localHash, r.HistoryHash) {
		// Same branch — remote is ahead on the same chain. Trigger
		// catch-up sync, NOT fork healing.
		slog.Info("handleHistoryReplyLocked: same branch confirmed via cross-epoch query", "group", c.groupID, "peer", from, "epoch", r.Epoch)
		if c.onSyncRequired != nil {
			go c.onSyncRequired(from, c.groupID)
			return
		}
		// No sync callback registered — fall through to ExternalJoin
		// so the node can catch up by joining the remote's branch.
		slog.Info("handleHistoryReplyLocked: no sync callback, falling through to ExternalJoin", "group", c.groupID, "peer", from, "epoch", r.Epoch)
	}
	// Different HistoryHash at the queried epoch → real fork, OR same branch
	// but no sync callback — proceed to fork detection and heal.
	slog.Warn("handleHistoryReplyLocked: fork confirmed via cross-epoch query", "group", c.groupID, "peer", from, "epoch", r.Epoch)
	localAnn := GroupStateAnnouncement{
		TreeHash:    c.treeHash,
		MemberCount: c.activeView.Size(),
		Epoch:       c.epoch,
		CommitHash:  copyBytes(c.lastCommitHash),
		HistoryHash: copyBytes(c.historyHash),
	}
	result, remoteAnn, _ := c.forkDetector.CompareWithPeer(from)
	remoteEpoch := remoteAnn.Epoch
	if remoteEpoch == 0 {
		remoteEpoch = r.Epoch
	}
	event := &ForkEvent{
		GroupID:          c.groupID,
		RemotePeer:       from,
		LocalAnnounce:    localAnn,
		RemoteAnnounce:   remoteAnn,
		RemoteEpoch:      remoteEpoch,
		Result:           result,
		NeedExternalJoin: result == BranchRemote,
	}
	if !event.NeedExternalJoin {
		slog.Info("handleHistoryReplyLocked: fork confirmed but local branch wins, skipping heal",
			"group", c.groupID, "peer", from, "epoch", r.Epoch)
		return
	}
	c.metrics.IncrPartitionsDetected()
	c.scheduleHeal(event)
}
