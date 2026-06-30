package coordination

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (c *Coordinator) handleApplicationLocked(from peer.ID, env *Envelope, wire []byte) bool {
	return c.handleApplicationDetailedLocked(from, env, wire).Applied
}

func (c *Coordinator) handleApplicationDetailedLocked(from peer.ID, env *Envelope, wire []byte) ReplayEnvelopeResult {
	result := c.newReplayResultLocked(env, wire)
	envelopeHash, alreadyApplied := c.checkAppliedEnvelopeLocked(env, wire)
	result.EnvelopeHash = envelopeHash
	if alreadyApplied {
		result.State = ReplayStateDuplicateApplied
		result.AlreadyApplied = true
		result.CursorSafe = true
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	if c.epochTracker == nil {
		c.epochTracker = NewEpochTracker(c.epoch, c.treeHash)
	}
	action := c.epochTracker.Validate(env.Epoch)
	maxPastEpochs := uint64(c.cfg.GetMaxPastEpochs())
	if action == ActionRejectStale && env.Type == MsgApplication && env.Epoch+maxPastEpochs >= c.epoch {
		// Enforce time-based retention policy max_past_age_seconds.
		// SECURITY: physical age validation is measured against local first-seen time, NOT sender-provided HLC.
		firstSeenMs := c.clock.Now().UnixMilli()
		if rec, err := c.storage.GetEnvelope(envelopeHash); err == nil && rec != nil && rec.FirstSeenAtMs > 0 {
			firstSeenMs = rec.FirstSeenAtMs
		}

		maxPastAgeSeconds := c.cfg.GetMaxPastAgeSeconds()
		ageSeconds := (c.clock.Now().UnixMilli() - firstSeenMs) / 1000
		if ageSeconds < 0 {
			ageSeconds = 0
		}
		if maxPastAgeSeconds > 0 && ageSeconds > maxPastAgeSeconds {
			slog.Warn("Rejected late-arriving stale application message exceeding age boundary",
				"group", c.groupID, "ageSeconds", ageSeconds, "maxPastAgeSeconds", maxPastAgeSeconds, "firstSeenMs", firstSeenMs)
			// keep action as ActionRejectStale
		} else {
			// Allow slightly stale application messages to be processed, as MLS supports
			// decrypting messages from a window of previous epochs using retained keys.
			action = ActionProcess
		}
	}

	switch action {
	case ActionRejectStale:
		epochDiff := int64(c.epoch) - int64(env.Epoch)
		slog.Warn("Rejected stale message", "group", c.groupID,
			"msgEpoch", env.Epoch, "currentEpoch", c.epoch,
			"epochDiff", epochDiff, "maxPastEpochs", c.cfg.GetMaxPastEpochs())
		result.State = ReplayStateStaleEpoch
		result.Terminal = true
		result.CursorSafe = true
		c.markReplayResultLocked(result)
		return result
	case ActionBufferFuture:
		slog.Info("Buffered future-epoch message", "group", c.groupID, "msgEpoch", env.Epoch)
		c.epochTracker.BufferFuture(env.Epoch, wire)
		result.State = ReplayStateFutureEpoch
		c.markReplayResultLocked(result)
		return result
	}

	var appMsg ApplicationMsg
	if err := json.Unmarshal(env.Payload, &appMsg); err != nil {
		result.State = ReplayStateInvalid
		result.Error = err.Error()
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	localTs, err := c.hlc.Update(env.Timestamp)
	if err != nil {
		slog.Error("HLC update failed (clock drift limit exceeded)", "group", c.groupID, "from", env.From, "error", err)
		result.State = ReplayStateInvalid
		result.Error = "hlc: " + err.Error()
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}
	if localTs.NodeID == "" {
		localTs.NodeID = c.localID.String()
	}

	if c.mls == nil {
		slog.Error("Cannot decrypt message: crypto engine not available", "group", c.groupID, "from", env.From)
		result.State = ReplayStateDecryptFailed
		result.Error = "crypto engine not available"
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}
	opCtx, cancel := c.mlsOperationContext()
	plaintext, newState, err := c.mls.DecryptMessage(opCtx, c.groupState, appMsg.Ciphertext)
	cancel()
	if err != nil {
		slog.Error("Failed to decrypt message", "group", c.groupID, "from", env.From, "error", err)
		result.State = ReplayStateDecryptFailed
		result.Error = err.Error()
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	sender := decodeEnvelopePeerID(env.From, from)

	msg := &StoredMessage{
		GroupID:      c.groupID,
		Epoch:        env.Epoch,
		SenderID:     sender,
		Content:      plaintext,
		Timestamp:    localTs,
		EnvelopeHash: envelopeHash,
	}
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyApplication(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		UpdatedAt:  now,
	}, msg, env.Type, wire, env.Timestamp, env.Epoch)
	if err != nil {
		slog.Error("Failed to persist decrypted message", "group", c.groupID, "from", env.From, "error", err)
		result.State = ReplayStatePersistFailed
		result.Error = err.Error()
		c.markReplayResultLocked(result)
		return result
	}
	if !applied {
		result.State = ReplayStateDuplicateApplied
		result.AlreadyApplied = true
		result.CursorSafe = true
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}
	c.groupState = newState
	slog.Info("Message received", "group", c.groupID, "epoch", env.Epoch, "from", env.From, "ts", localTs.WallTimeMs)

	if c.onMessage != nil {
		c.onMessage(msg)
	}
	c.sendDeliveryAckLocked(sender, envelopeHash)
	result.State = ReplayStateApplied
	result.Applied = true
	result.CursorSafe = true
	result.Terminal = true
	c.markReplayResultLocked(result)
	return result
}

func (c *Coordinator) handleDeliveryAckLocked(from peer.ID, env *Envelope) {
	var ack DeliveryAckMsg
	if err := json.Unmarshal(env.Payload, &ack); err != nil {
		return
	}
	if len(ack.EnvelopeHash) == 0 {
		return
	}
	delete(c.pendingAppDeliveries, pendingAppDeliveryKey(from, ack.EnvelopeHash))
}

// SendMessage encrypts plaintext and broadcasts it as an application message.
// Returns the HLC timestamp assigned to the message.
func (c *Coordinator) SendMessage(plaintext []byte) (*HLCTimestamp, error) {
	return c.sendMessage(plaintext, "")
}

// SendMessageWithLocalEchoToken mirrors SendMessage but tags the locally emitted
// StoredMessage with a process-local correlation token so the frontend can
// replace optimistic echoes deterministically.
func (c *Coordinator) SendMessageWithLocalEchoToken(plaintext []byte, localEchoToken string) (*HLCTimestamp, error) {
	return c.sendMessage(plaintext, localEchoToken)
}

func (c *Coordinator) sendMessage(plaintext []byte, localEchoToken string) (*HLCTimestamp, error) {
	if c.healing.Load() {
		return nil, fmt.Errorf("fork healing in progress: message rejected to avoid cross-epoch state corruption")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil, fmt.Errorf("coordinator not started")
	}
	c.updateLocalAccessRevocationLocked(c.groupState, c.epoch)
	if c.accessRevoked {
		slog.Warn("Mutation rejected: access revoked",
			"group", c.groupID,
			"epoch", c.epoch,
			"op", "SendMessage",
			"reason", "access_revoked",
			"violation_source", "local_membership_guard")
		return nil, ErrAccessRevoked
	}

	ts := c.hlc.Now()
	if ts.NodeID == "" {
		ts.NodeID = c.localID.String()
	}

	if c.mls == nil {
		return nil, fmt.Errorf("crypto engine not available — build the Rust project first")
	}
	opCtx, cancel := c.mlsOperationContext()
	ciphertext, newState, err := c.mls.EncryptMessage(opCtx, c.groupState, plaintext)
	cancel()
	if err != nil {
		slog.Error("Failed to encrypt message", "group", c.groupID, "error", err)
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgApplication, ApplicationMsg{Ciphertext: ciphertext}, ts)
	if len(envBytes) == 0 {
		return nil, fmt.Errorf("encode envelope")
	}
	envelopeHash := hashEnvelope(envBytes)

	msg := &StoredMessage{
		GroupID:        c.groupID,
		Epoch:          c.epoch,
		SenderID:       c.localID,
		Content:        plaintext,
		Timestamp:      ts,
		LocalEchoToken: localEchoToken,
		EnvelopeHash:   envelopeHash,
	}
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyApplication(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		UpdatedAt:  now,
	}, msg, MsgApplication, envBytes, ts, c.epoch)
	if err != nil {
		return nil, fmt.Errorf("persist application: %w", err)
	}
	if !applied {
		return nil, fmt.Errorf("application envelope already applied")
	}
	c.groupState = newState
	c.publishPreparedEnvelopeLocked(MsgApplication, envBytes)
	c.trackPendingApplicationDeliveriesLocked(envBytes, envelopeHash)
	slog.Info("Message sent", "group", c.groupID, "epoch", c.epoch, "ts", ts.WallTimeMs)
	if c.onMessage != nil {
		c.onMessage(msg)
	}

	return &ts, nil
}

func pendingAppDeliveryKey(pid peer.ID, envelopeHash []byte) string {
	return pid.String() + "|" + hex.EncodeToString(envelopeHash)
}

func (c *Coordinator) trackPendingApplicationDeliveriesLocked(envBytes, envelopeHash []byte) {
	if len(envBytes) == 0 || len(envelopeHash) == 0 || c.cfg.ApplicationDirectRetryLimit == 0 {
		return
	}
	recipients := c.applicationRecipientsLocked()
	for _, pid := range recipients {
		key := pendingAppDeliveryKey(pid, envelopeHash)
		if _, exists := c.pendingAppDeliveries[key]; exists {
			continue
		}
		c.pendingAppDeliveries[key] = &pendingAppDelivery{
			peerID:       pid,
			envelopeHash: hex.EncodeToString(envelopeHash),
			envelope:     append([]byte(nil), envBytes...),
		}
		go c.applicationAckWatchLoop(pid, append([]byte(nil), envelopeHash...))
	}
}

func (c *Coordinator) applicationRecipientsLocked() []peer.ID {
	seen := make(map[string]struct{})
	out := make([]peer.ID, 0)
	addPeer := func(pid peer.ID) {
		if pid == "" || pid == c.localID {
			return
		}
		key := pid.String()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, pid)
	}
	for _, pid := range c.activeView.Members() {
		addPeer(pid)
	}
	if len(out) == 0 {
		for _, pid := range c.transport.ConnectedPeers() {
			addPeer(pid)
		}
	}
	return out
}

func (c *Coordinator) applicationAckWatchLoop(pid peer.ID, envelopeHash []byte) {
	key := pendingAppDeliveryKey(pid, envelopeHash)
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(c.cfg.ApplicationAckTimeout):
		}

		c.mu.Lock()
		pending, ok := c.pendingAppDeliveries[key]
		if !ok {
			c.mu.Unlock()
			return
		}
		if pending.attempts >= c.cfg.ApplicationDirectRetryLimit {
			c.mu.Unlock()
			return
		}
		pending.attempts++
		wire := append([]byte(nil), pending.envelope...)
		groupID := c.groupID
		attempt := pending.attempts
		c.mu.Unlock()

		if err := c.sendDirectEnvelope(pid, wire); err != nil {
			slog.Debug("application direct retry failed",
				"group", groupID,
				"peer", pid,
				"attempt", attempt,
				"err", err,
			)
			continue
		}
		slog.Info("application direct retry sent",
			"group", groupID,
			"peer", pid,
			"attempt", attempt,
		)
	}
}

func (c *Coordinator) sendDeliveryAckLocked(to peer.ID, envelopeHash []byte) {
	if to == "" || to == c.localID || len(envelopeHash) == 0 {
		return
	}
	envBytes := c.buildEnvelopeWithEpochAndTimestampLocked(
		MsgDeliveryAck,
		DeliveryAckMsg{EnvelopeHash: append([]byte(nil), envelopeHash...)},
		c.epoch,
		c.hlc.Now(),
	)
	if len(envBytes) == 0 {
		return
	}
	go func(pid peer.ID, wire []byte) {
		if err := c.sendDirectEnvelope(pid, wire); err != nil {
			slog.Debug("delivery ack send failed", "group", c.groupID, "peer", pid, "err", err)
		}
	}(to, envBytes)
}

func (c *Coordinator) sendDirectEnvelope(to peer.ID, wire []byte) error {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	dctx, cancel := context.WithTimeout(ctx, c.cfg.ApplicationAckTimeout)
	defer cancel()
	return c.transport.SendDirect(dctx, to, wire)
}

// RetryOutstandingDeliveriesTo immediately re-sends every still-unacked local
// application envelope to one peer. Intended for reconnect / re-verify hooks.
func (c *Coordinator) RetryOutstandingDeliveriesTo(pid peer.ID) {
	if pid == "" || pid == c.localID {
		return
	}
	c.mu.Lock()
	var wires [][]byte
	for _, pending := range c.pendingAppDeliveries {
		if pending.peerID != pid {
			continue
		}
		wires = append(wires, append([]byte(nil), pending.envelope...))
	}
	c.mu.Unlock()
	for _, wire := range wires {
		if err := c.sendDirectEnvelope(pid, wire); err != nil {
			slog.Debug("retry outstanding delivery failed", "group", c.groupID, "peer", pid, "err", err)
		}
	}
}
