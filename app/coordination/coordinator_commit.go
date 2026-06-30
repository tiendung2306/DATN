package coordination

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (c *Coordinator) handleCommitLocked(env *Envelope, wire []byte) bool {
	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		c.metrics.IncrDuplicateEpochDetected()
		return false
	case ActionBufferFuture:
		c.epochTracker.BufferFuture(env.Epoch, wire)
		return false
	}

	var commit CommitMsg
	if err := json.Unmarshal(env.Payload, &commit); err != nil {
		return false
	}
	commitHash := hashCommitData(commit.CommitData)

	_, alreadyApplied := c.checkAppliedEnvelopeLocked(env, wire)
	if alreadyApplied {
		return false
	}

	start := c.clock.Now()

	opCtx, cancel := c.mlsOperationContext()
	batch := bufferedBatchFromProposalMsgs(commit.IncludedProposals)
	includedProposalBytes := proposalBytesFromMsgs(commit.IncludedProposals)
	staged, err := c.mls.StageCommit(opCtx, c.groupState, commit.CommitData, includedProposalBytes)
	cancel()
	if err != nil {
		c.markInvalidCommitLocked(commitHash)
		return false
	}
	if len(commit.CommittedProposalRefs) > 0 && !proposalRefSetsEqual(staged.ProposalRefs, commit.CommittedProposalRefs) {
		c.markInvalidCommitLocked(commitHash)
		return false
	}
	if len(batch) > 0 {
		sender := decodeEnvelopePeerID(env.From, "")
		if sender == "" {
			slog.Warn("Commit envelope missing sender; skipping token-holder metadata validation for compatibility", "group", c.groupID, "epoch", c.epoch)
		} else {
			holder, err := c.singleWriter.HolderForBatch(batch)
			if err != nil {
				c.markInvalidCommitLocked(commitHash)
				return false
			}
			if sender != holder {
				slog.Warn("Rejected commit from non-token-holder", "group", c.groupID, "epoch", c.epoch, "sender", sender, "holder", holder)
				c.markInvalidCommitLocked(commitHash)
				return false
			}
			if removesPeer(batch, sender) {
				slog.Warn("Rejected commit whose batch removes the committer", "group", c.groupID, "epoch", c.epoch, "sender", sender)
				c.markInvalidCommitLocked(commitHash)
				return false
			}
		}
	}

	opCtx, cancel = c.mlsOperationContext()
	newState, newTreeHash, err := c.mls.ProcessCommit(opCtx, c.groupState, commit.CommitData, includedProposalBytes)
	cancel()
	if err != nil {
		c.markInvalidCommitLocked(commitHash)
		return false
	}
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      nextEpoch,
		TreeHash:   newTreeHash,
		UpdatedAt:  now,
	}, env.Type, wire, env.Timestamp, env.Epoch)
	if err != nil {
		slog.Error("Failed to persist commit apply", "group", c.groupID, "error", err)
		return false
	}
	if !applied {
		return false
	}

	c.advanceEpochLocked(newState, nextEpoch, newTreeHash, commit.CommitData)
	// Primary drain: by ProposalRef (works when this node is the holder or has the same groupState).
	c.singleWriter.DrainBatchByRefs(commit.CommittedProposalRefs)
	// Fallback drain: by raw proposal Data bytes.
	if len(commit.IncludedProposals) > 0 {
		proposalDatas := make([][]byte, 0, len(commit.IncludedProposals))
		for _, p := range commit.IncludedProposals {
			if len(p.Data) > 0 {
				proposalDatas = append(proposalDatas, p.Data)
			}
		}
		c.singleWriter.DrainBatchByData(proposalDatas)
	}
	c.reconcileOperationsAfterCommitLocked(commit)

	// Trigger bidirectional batch replay
	c.triggerBatchReplayAsync(c.groupID)

	c.reconcileAndRebaseOperationsLocked()

	c.updateLocalAccessRevocationLocked(newState, nextEpoch)
	c.metrics.RecordEpochFinalization(c.clock.Now().Sub(start))

	if len(commit.AddDeliveries) > 0 && c.onAddCommitted != nil {
		deliveries := append([]AddCommitDelivery(nil), commit.AddDeliveries...)
		epoch := nextEpoch
		cb := c.onAddCommitted
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for _, d := range deliveries {
				cb(d, epoch, nil)
			}
		}()
	}
	return true
}

func (c *Coordinator) handleCommitDetailedLocked(env *Envelope, wire []byte) ReplayEnvelopeResult {
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

	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		result.State = ReplayStateStaleEpoch
		result.Terminal = true
		result.CursorSafe = true
		c.markReplayResultLocked(result)
		return result
	case ActionBufferFuture:
		c.epochTracker.BufferFuture(env.Epoch, wire)
		result.State = ReplayStateFutureEpoch
		c.markReplayResultLocked(result)
		return result
	}

	if c.handleCommitLocked(env, wire) {
		result.State = ReplayStateApplied
		result.Applied = true
		result.CursorSafe = true
		result.Terminal = true
		c.markReplayResultLocked(result)
		return result
	}

	if applied, err := c.storage.HasAppliedEnvelope(c.groupID, envelopeHash); err != nil {
		result.State = ReplayStatePersistFailed
		result.Error = err.Error()
	} else if applied {
		result.State = ReplayStateDuplicateApplied
		result.AlreadyApplied = true
		result.CursorSafe = true
		result.Terminal = true
	} else {
		result.State = ReplayStateInvalid
		result.Error = "commit replay did not apply"
		result.Terminal = true
	}
	c.markReplayResultLocked(result)
	return result
}

func (c *Coordinator) bootstrapGraceRemainingLocked() time.Duration {
	grace := c.cfg.ViewBootstrapGrace
	if grace <= 0 || c.startedAt.IsZero() {
		return 0
	}
	elapsed := c.clock.Now().Sub(c.startedAt)
	if elapsed >= grace {
		return 0
	}
	return grace - elapsed
}

func (c *Coordinator) commitViewReadyLocked(batch []BufferedProposal) bool {
	if c.bootstrapGraceRemainingLocked() <= 0 {
		return true
	}
	if c.activeView.Size() > 1 {
		return true
	}
	if c.authorizedCommitters == nil {
		return true
	}
	authorized, err := c.authorizedCommitters(c.groupID, c.epoch, batch)
	if err != nil {
		slog.Warn("Bootstrap view guard skipped: failed to load authorized committers",
			"group", c.groupID,
			"epoch", c.epoch,
			"error", err)
		return true
	}
	removed := make(map[peer.ID]struct{}, len(batch))
	for _, p := range batch {
		if p.Type != ProposalRemove || p.TargetPeerID == "" {
			continue
		}
		if target, err := peer.Decode(p.TargetPeerID); err == nil {
			removed[target] = struct{}{}
		}
	}
	for _, id := range authorized {
		if id != "" && id != c.localID {
			if _, willBeRemoved := removed[id]; willBeRemoved {
				continue
			}
			return false
		}
	}
	return true
}

func (c *Coordinator) deferCommitUntilViewReadyLocked() {
	if c.proposalTimerChan != nil {
		return
	}
	delay := c.bootstrapGraceRemainingLocked()
	if delay <= 0 {
		delay = c.cfg.BatchingDelay
	}
	if delay <= 0 {
		delay = 10 * time.Millisecond
	}
	slog.Info("Deferring token-holder commit while group ActiveView bootstraps",
		"group", c.groupID,
		"epoch", c.epoch,
		"delay", delay,
		"active_view_size", c.activeView.Size())
	ch := c.clock.After(delay)
	c.proposalTimerChan = ch
	go func(timerChan <-chan time.Time) {
		<-timerChan

		c.mu.Lock()
		defer c.mu.Unlock()

		if c.proposalTimerChan == timerChan {
			c.proposalTimerChan = nil
			if c.singleWriter != nil && c.singleWriter.IsTokenHolder() && c.singleWriter.ProposalCount() > 0 {
				c.tryCommitLocked()
			}
		}
	}(ch)
}

// scheduleBatchCommitLocked schedules a commit execution after a short batching delay
// to allow multiple concurrent proposals to be gathered into a single commit.
// If a commit is already scheduled, this is a no-op (letting the current window collect proposals).
func (c *Coordinator) scheduleBatchCommitLocked() {
	if c.proposalTimerChan != nil {
		return // already scheduled, let it accumulate proposals
	}

	delay := c.cfg.BatchingDelay
	if delay <= 0 {
		// Fallback to immediate commit if batching is disabled
		c.tryCommitLocked()
		return
	}

	slog.Info("Scheduling batch commit after delay", "group", c.groupID, "delay", delay)
	ch := c.clock.After(delay)
	c.proposalTimerChan = ch

	c.wg.Add(1)
	go func(timerChan <-chan time.Time) {
		defer c.wg.Done()

		select {
		case <-timerChan:
		case <-c.ctx.Done():
			c.mu.Lock()
			if c.proposalTimerChan == timerChan {
				c.proposalTimerChan = nil
			}
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		// Ensure that the timer was not canceled or overwritten
		if c.proposalTimerChan == timerChan {
			c.proposalTimerChan = nil
			if c.singleWriter != nil && c.singleWriter.IsTokenHolder() && c.singleWriter.ProposalCount() > 0 {
				slog.Info("Batching delay expired: flashing committed proposals",
					"group", c.groupID,
					"epoch", c.epoch,
					"buffered_proposals", c.singleWriter.ProposalCount(),
				)
				c.tryCommitLocked()
			}
		}
	}(ch)
}

func (c *Coordinator) scheduleFailoverTimerLocked() {
	if c.failoverTimerChan != nil {
		return // already scheduled
	}
	if c.singleWriter == nil || c.singleWriter.ProposalCount() == 0 || c.singleWriter.IsTokenHolder() {
		return // no need for failover
	}

	delay := c.cfg.TokenHolderTimeout
	if delay <= 0 {
		delay = 5 * time.Second
	}

	slog.Info("Scheduling token holder failover timer", "group", c.groupID, "epoch", c.epoch, "delay", delay)
	ch := c.clock.After(delay)
	c.failoverTimerChan = ch

	c.wg.Add(1)
	go func(timerChan <-chan time.Time) {
		defer c.wg.Done()

		select {
		case <-timerChan:
		case <-c.ctx.Done():
			c.mu.Lock()
			if c.failoverTimerChan == timerChan {
				c.failoverTimerChan = nil
			}
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		if c.failoverTimerChan == timerChan {
			c.failoverTimerChan = nil
			if c.singleWriter != nil && c.singleWriter.ProposalCount() > 0 && !c.singleWriter.IsTokenHolder() {
				holder, err := c.singleWriter.CurrentTokenHolder()
				if err == nil && holder != "" {
					slog.Warn("Token Holder failed to commit in time, suspending", "group", c.groupID, "epoch", c.epoch, "holder", holder)
					c.singleWriter.Suspend(holder)

					// Re-evaluate: if we became the Token Holder, we should try to commit now.
					if c.singleWriter.IsTokenHolder() {
						c.scheduleBatchCommitLocked()
					} else {
						// Someone else is the new Token Holder, restart the failover timer for them.
						c.scheduleFailoverTimerLocked()
					}
				}
			}
		}
	}(ch)
}

// tryCommitLocked commits the deterministic proposal-ref snapshot for this epoch.
func (c *Coordinator) tryCommitLocked() {
	if !c.singleWriter.IsTokenHolder() {
		return
	}
	if len(c.groupState) == 0 {
		return
	}
	batch := c.singleWriter.SnapshotNextBatch()
	if len(batch) == 0 {
		return
	}
	if !c.commitViewReadyLocked(batch) {
		c.deferCommitUntilViewReadyLocked()
		return
	}
	c.doCommitBatchLocked(batch)
}

// doCommitBatchLocked executes an MLS CreateCommit for the given batch, persists
// the result, and broadcasts the commit envelope. Called by tryCommitLocked
// (normal Token-Holder path) and the batched-commit timer.
func (c *Coordinator) doCommitBatchLocked(batch []BufferedProposal) {
	if len(c.groupState) == 0 {
		slog.Warn("doCommitBatchLocked: groupState is nil, skipping commit", "group", c.groupID)
		return
	}
	prevEpoch := c.epoch

	expectedRefs := make([][]byte, 0, len(batch))
	for i := range batch {
		expectedRefs = append(expectedRefs, append([]byte(nil), batch[i].ProposalRef...))
	}

	opCtx, cancel := c.mlsOperationContext()
	commitResult, err := c.mls.CreateCommit(opCtx, c.groupState, expectedRefs)
	cancel()
	if err != nil {
		slog.Warn("CreateCommit failed for pending proposal batch", "group", c.groupID, "epoch", c.epoch, "error", err)
		return
	}

	committedRefs := commitResult.CommittedProposalRefs
	if len(committedRefs) == 0 {
		committedRefs = expectedRefs
	}
	commitMsg := CommitMsg{
		CommitData:            commitResult.CommitBytes,
		NewTreeHash:           commitResult.NewTreeHash,
		IncludedProposals:     proposalMsgsFromBatch(batch),
		CommittedProposalRefs: cloneBytesList(committedRefs),
	}

	// Surface routing metadata for ProposalAdd commits so observer nodes can
	// correlate the commit with their local group_add_operations rows.
	if batchContainsType(batch, ProposalAdd) {
		commitMsg.AddDeliveries = buildAddDeliveriesFromBatch(batch, commitResult.WelcomeBytes)
	}

	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return
	}
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: commitResult.NewGroupState,
		Epoch:      nextEpoch,
		TreeHash:   commitResult.NewTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts, c.epoch)
	if err != nil || !applied {
		return
	}
	drained := c.singleWriter.DrainBatchByRefs(committedRefs)
	if len(drained) > 0 {
		batch = drained
	}
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(commitResult.NewGroupState, nextEpoch, commitResult.NewTreeHash, commitResult.CommitBytes)
	c.reconcileOperationsAfterCommitLocked(commitMsg)
	c.triggerBatchReplayAsync(c.groupID)
	c.reconcileAndRebaseOperationsLocked()

	c.updateLocalAccessRevocationLocked(commitResult.NewGroupState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitResult.CommitBytes)))
	c.emitCommitIssuedLocked(prevEpoch, nextEpoch, batch)

	// Hand off Welcome dispatch to the runtime. This local node is the Token
	// Holder that just ran CreateCommit, so it is the only node holding the
	// ephemeral key material required to deliver Welcome to each invitee.
	if batchContainsType(batch, ProposalAdd) && c.onAddCommitted != nil && len(commitMsg.AddDeliveries) > 0 {
		deliveries := append([]AddCommitDelivery(nil), commitMsg.AddDeliveries...)
		welcome := append([]byte(nil), commitResult.WelcomeBytes...)
		epoch := nextEpoch
		cb := c.onAddCommitted
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for _, d := range deliveries {
				cb(d, epoch, welcome)
			}
		}()
	}
}

func (c *Coordinator) emitCommitIssuedLocked(prevEpoch, newEpoch uint64, batch []BufferedProposal) {
	if c.onCommitIssued == nil {
		return
	}
	proposals := make([]CommitAuditProposalSummary, 0, len(batch))
	for _, item := range batch {
		proposals = append(proposals, summarizeBufferedProposal(item))
	}
	c.onCommitIssued(CommitAuditSummary{
		GroupID:           c.groupID,
		TokenHolderPeerID: c.localID.String(),
		PrevEpoch:         prevEpoch,
		NewEpoch:          newEpoch,
		Proposals:         proposals,
	})
}

func (c *Coordinator) advanceEpochLocked(newState []byte, newEpoch uint64, newTreeHash, commitData []byte) {
	c.groupState = newState
	c.epoch = newEpoch
	c.treeHash = newTreeHash
	slog.Info("Epoch advanced", "group", c.groupID, "newEpoch", newEpoch)

	buffered := c.epochTracker.Advance(newEpoch, newTreeHash)
	c.singleWriter.AdvanceEpoch(newEpoch)
	if holder, err := c.singleWriter.CurrentTokenHolder(); err == nil {
		c.lastTokenHolder = holder
	}
	c.proposalTimerChan = nil // Reset active timer on epoch transition
	c.failoverTimerChan = nil // Reset failover timer on epoch transition

	commitHash := hashCommitData(commitData)
	c.lastCommitHash = copyBytes(commitHash)

	// Advance HistoryHash chain: R(E) = H(R(E-1) ∥ CommitHash(E))
	if c.historyChain == nil {
		c.historyChain = make(map[uint64][]byte)
	}
	if c.historyHash == nil {
		c.historyHash = initialHistoryHash(c.groupID)
		c.historyChain[newEpoch-1] = copyBytes(c.historyHash)
	}
	c.historyHash = computeHistoryHash(c.historyHash, commitHash)
	c.historyChain[newEpoch] = copyBytes(c.historyHash)

	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    newTreeHash,
		MemberCount: c.activeView.Size(),
		Epoch:       newEpoch,
		CommitHash:  commitHash,
		HistoryHash: copyBytes(c.historyHash),
	})
	if err := c.persistCoordStateLocked(); err != nil {
		slog.Warn("Failed to persist coordination state after epoch advance", "group", c.groupID, "error", err)
	}

	if c.onEpochChange != nil {
		c.onEpochChange(newEpoch)
	}

	for _, raw := range buffered {
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}
		switch env.Type {
		case MsgProposal:
			c.handleProposalLocked(decodeEnvelopePeerID(env.From, ""), &env)
		case MsgCommit:
			c.handleCommitLocked(&env, raw)
		case MsgApplication:
			c.handleApplicationLocked(decodeEnvelopePeerID(env.From, ""), &env, raw)
		case MsgApplicationBatched:
			c.handleApplicationBatchedLocked(decodeEnvelopePeerID(env.From, ""), &env, raw)
		}
	}
}

func validateSenderTimestamp(nowMs int64, senderMs int64) error {
	const maxFutureSkewMs = int64(5 * 60 * 1000) // 5 minutes
	if senderMs > nowMs+maxFutureSkewMs {
		return fmt.Errorf("sender timestamp too far in future: received %d is more than %dms ahead of physical %d", senderMs, maxFutureSkewMs, nowMs)
	}
	return nil
}
