package coordination

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (c *Coordinator) scheduleHeal(event *ForkEvent) {
	if event == nil {
		return
	}

	// Snapshot fields needed by the goroutine *before* releasing c.mu so the
	// goroutine never reads from c without re-locking.
	traceID := newTraceID()
	localEpoch := c.epoch
	scheduledAt := c.clock.Now()

	// Catch-up sync retry guard: if we are lagging behind, try sequential sync first.
	if event.RemoteEpoch > localEpoch && c.onSyncRequired != nil {
		if c.syncRetryAttempts < 3 {
			c.deferredForkEvent = event
			slog.Info("fork_heal/deferred_for_sync",
				"group", event.GroupID,
				"local_epoch", localEpoch,
				"winner_epoch", event.RemoteEpoch,
				"winner_peer", event.RemotePeer.String(),
				"attempt", c.syncRetryAttempts+1,
			)
			cb := c.onSyncRequired
			peerID := event.RemotePeer
			groupID := event.GroupID
			go cb(peerID, groupID) // Fire asynchronously to avoid holding c.mu
			return
		}
	}

	// Retry limit exhausted or not lagging. Proceed with immediate destructive healing.
	c.deferredForkEvent = nil

	localTreeHash := append([]byte(nil), c.treeHash...)
	partitionWindowMs := int64(0)
	if !event.PartitionStartedAt.IsZero() {
		partitionWindowMs = scheduledAt.Sub(event.PartitionStartedAt).Milliseconds()
	}

	if !c.healing.CompareAndSwap(false, true) {
		slog.Info("fork_heal/skipped_already_running",
			"trace_id", traceID,
			"group", event.GroupID,
			"local_epoch", localEpoch,
			"winner_peer", event.RemotePeer.String(),
			"winner_epoch", event.RemoteEpoch,
		)
		return
	}

	c.metrics.IncrForkHealingsAttempted()

	slog.Info("fork_heal/scheduled",
		"trace_id", traceID,
		"group", event.GroupID,
		"local_epoch", localEpoch,
		"local_tree_hash", hex.EncodeToString(localTreeHash),
		"local_member_count", event.LocalAnnounce.MemberCount,
		"winner_peer", event.RemotePeer.String(),
		"winner_epoch", event.RemoteEpoch,
		"winner_tree_hash", hex.EncodeToString(event.RemoteAnnounce.TreeHash),
		"winner_member_count", event.RemoteAnnounce.MemberCount,
		"partition_started_at_ms", event.PartitionStartedAt.UnixMilli(),
		"partition_window_ms", partitionWindowMs,
		"scheduled_at_ms", scheduledAt.UnixMilli(),
	)
	if c.onForkHealEvent != nil {
		c.onForkHealEvent(ForkHealAuditSummary{
			GroupID:      event.GroupID,
			TraceID:      traceID,
			Stage:        "fork_heal_started",
			WinnerPeerID: event.RemotePeer.String(),
			WinnerEpoch:  event.RemoteEpoch,
			NewEpoch:     localEpoch,
		})
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.runHeal(c.ctx, traceID, event, scheduledAt)
	}()
}

// ResetSyncRetryAttempts resets the catch-up sync retry counter to 0.
func (c *Coordinator) ResetSyncRetryAttempts() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncRetryAttempts = 0
	c.deferredForkEvent = nil
}

// IncrementSyncRetryAttempts increments the retry counter and returns the new value.
func (c *Coordinator) IncrementSyncRetryAttempts() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncRetryAttempts++
	return c.syncRetryAttempts
}

// GetSyncRetryAttempts returns the current retry counter value.
func (c *Coordinator) GetSyncRetryAttempts() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.syncRetryAttempts
}

// TriggerDeferredHeal triggers the deferred fork heal event if present, bypassing retry checks.
func (c *Coordinator) TriggerDeferredHeal() {
	c.mu.Lock()
	if c.deferredForkEvent == nil {
		c.mu.Unlock()
		return
	}
	event := c.deferredForkEvent
	c.deferredForkEvent = nil

	traceID := newTraceID()
	localEpoch := c.epoch
	scheduledAt := c.clock.Now()

	if !c.healing.CompareAndSwap(false, true) {
		c.mu.Unlock()
		slog.Info("fork_heal/skipped_already_running",
			"trace_id", traceID,
			"group", event.GroupID,
			"local_epoch", localEpoch,
			"winner_peer", event.RemotePeer.String(),
			"winner_epoch", event.RemoteEpoch,
		)
		return
	}

	c.metrics.IncrForkHealingsAttempted()

	slog.Info("fork_heal/scheduled_deferred_after_retry",
		"trace_id", traceID,
		"group", event.GroupID,
		"local_epoch", localEpoch,
		"winner_peer", event.RemotePeer.String(),
		"winner_epoch", event.RemoteEpoch,
	)
	if c.onForkHealEvent != nil {
		c.onForkHealEvent(ForkHealAuditSummary{
			GroupID:      event.GroupID,
			TraceID:      traceID,
			Stage:        "fork_heal_started",
			WinnerPeerID: event.RemotePeer.String(),
			WinnerEpoch:  event.RemoteEpoch,
			NewEpoch:     localEpoch,
		})
	}
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.runHeal(c.ctx, traceID, event, scheduledAt)
	}()
}

// runHeal executes the fork-heal pipeline. It runs on its own goroutine and is
// guaranteed to release c.healing on exit.
func (c *Coordinator) runHeal(ctx context.Context, traceID string, event *ForkEvent, scheduledAt time.Time) {
	defer c.healing.Store(false)
	defer func() {
		c.mu.Lock()
		if c.groupState == nil {
			if rec, err := c.storage.GetGroupRecord(c.groupID); err == nil && len(rec.GroupState) > 0 {
				c.groupState = rec.GroupState
				c.epoch = rec.Epoch
				c.treeHash = rec.TreeHash
				slog.Warn("fork_heal/restored_group_state_from_db",
					"group", c.groupID, "epoch", c.epoch, "trace_id", traceID)
			}
		}
		c.mu.Unlock()
	}()
	if ctx == nil {
		ctx = context.Background()
	}

	startedAt := c.clock.Now()
	slog.Info("fork_heal/started",
		"trace_id", traceID,
		"group", event.GroupID,
		"queued_ms", startedAt.Sub(scheduledAt).Milliseconds(),
	)
	c.recordForkHealAudit(traceID, event.GroupID, "started", "completed", startedAt, startedAt.Sub(scheduledAt).Milliseconds(), "")

	if ctx.Err() != nil {
		slog.Info("fork_heal/aborted",
			"trace_id", traceID,
			"group", event.GroupID,
			"reason", ctx.Err().Error(),
			"duration_ms", c.clock.Now().Sub(startedAt).Milliseconds(),
		)
		return
	}

	// M5: 1. Khởi tạo / Tìm kiếm Job Fork Healing bền vững
	job, err := c.storage.GetActiveForkHealingJob(event.GroupID)
	if err != nil {
		slog.Error("fork_heal/db_get_job_failed", "group", event.GroupID, "err", err)
	}
	if job == nil {
		losingBranchID := hex.EncodeToString(c.lastCommitHash)
		winningBranchID := hex.EncodeToString(event.RemoteAnnounce.CommitHash)

		job = &ForkHealingJob{
			JobID:             fmt.Sprintf("job-%s-%d", event.GroupID, startedAt.UnixMilli()),
			GroupID:           event.GroupID,
			TraceID:           traceID,
			Status:            "INITIATED",
			LosingBranchID:    losingBranchID,
			WinningBranchID:   winningBranchID,
			ForkBaseEpoch:     c.epoch,
			LosingEpoch:       c.epoch,
			WinningEpoch:      event.RemoteEpoch,
			LosingTreeHash:    c.treeHash,
			WinningTreeHash:   event.RemoteAnnounce.TreeHash,
			WinningCommitHash: event.RemoteAnnounce.CommitHash,
			WinnerPeerID:      event.RemotePeer.String(),
			CreatedAtMs:       startedAt.UnixMilli(),
			UpdatedAtMs:       startedAt.UnixMilli(),
		}
		if err := c.storage.SaveForkHealingJob(job); err != nil {
			slog.Error("fork_heal/db_save_job_failed", "group", event.GroupID, "err", err)
		}
	}

	storageKey := deriveStorageKey(c.signingKey)

	// M5: 2. Freeze apply & user sends, gossip append-only
	c.SetOperationalMode(ModeFrozenForApply)
	slog.Info("fork_heal/frozen_for_apply", "group", event.GroupID)
	defer func() {
		c.SetOperationalMode(ModeLive)
		slog.Info("fork_heal/live_unfrozen", "group", event.GroupID)
	}()

	// M5: 3. Snapshot các events thuộc partition window & AES-GCM Sealing cục bộ
	if job.Status == "INITIATED" {
		stepStart := c.clock.Now()
		c.recordForkHealAudit(traceID, event.GroupID, "snapshot_orphan", "started", stepStart, 0, "")

		losingBranchID := job.LosingBranchID

		// A. Snapshot các own messages đã apply thành công trong losing branch partition window
		// Using 0 as startMs to capture all orphaned messages that were locally applied but not committed globally
		ownMsgs, err := c.storage.GetMessagesByOwnerInRange(c.groupID, c.localID.String(), 0, startedAt.UnixMilli())
		if err == nil {
			for _, msg := range ownMsgs {
				sealedPayload, nonce, sealErr := sealPayload(msg.Content, storageKey)
				if sealErr == nil {
					h := sha256.Sum256(msg.Content)
					appEv := &ApplicationEvent{
						EventID:          hex.EncodeToString(msg.EnvelopeHash),
						JobID:            job.JobID,
						GroupID:          event.GroupID,
						OriginalBranchID: losingBranchID,
						OriginalEpoch:    msg.Epoch,
						AuthorID:         c.localID.String(),
						EnvelopeHash:     msg.EnvelopeHash,
						PayloadSealed:    sealedPayload,
						PayloadHash:      h[:],
						SealKeyID:        "local_node_key",
						SealNonce:        nonce,
						HlcWallTimeMs:    msg.Timestamp.WallTimeMs,
						HlcCounter:       msg.Timestamp.Counter,
						HlcNodeID:        msg.Timestamp.NodeID,
						Status:           "ORPHANED_OWN",
						CreatedAtMs:      c.clock.Now().UnixMilli(),
						UpdatedAtMs:      c.clock.Now().UnixMilli(),
					}
					if err := c.storage.SaveApplicationEvent(appEv); err != nil {
						slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
					}
				}
			}
		}

		// B. Snapshot các unapplied envelopes trong log
		pendingEnvs, err := c.storage.GetPendingEnvelopes(event.GroupID, 1000)
		if err == nil {
			for _, record := range pendingEnvs {
				// M5: Removed PartitionStartedAt filter because ANY unapplied envelope is an orphan when state-swapping

				if record.MsgType == MsgApplication {
					var env Envelope
					if err := json.Unmarshal(record.Envelope, &env); err == nil {
						var appMsg ApplicationMsg
						if err := json.Unmarshal(env.Payload, &appMsg); err == nil {
							c.mu.Lock()
							plaintext, _, decErr := c.mls.DecryptMessage(ctx, c.groupState, appMsg.Ciphertext)
							c.mu.Unlock()

							var status string
							var sealedPayload, nonce []byte
							var pHash []byte

							if decErr != nil {
								status = "UNRECOVERABLE"
								pHash = record.EnvelopeHash
							} else {
								h := sha256.Sum256(plaintext)
								pHash = h[:]

								if env.From == c.localID.String() {
									status = "ORPHANED_OWN"
									sealedPayload, nonce, _ = sealPayload(plaintext, storageKey)
								} else {
									status = "WAITING_AUTHOR_REPLAY"
								}
							}

							appEv := &ApplicationEvent{
								EventID:          hex.EncodeToString(record.EnvelopeHash),
								JobID:            job.JobID,
								GroupID:          event.GroupID,
								OriginalBranchID: losingBranchID,
								OriginalEpoch:    record.Epoch,
								AuthorID:         env.From,
								EnvelopeHash:     record.EnvelopeHash,
								PayloadSealed:    sealedPayload,
								PayloadHash:      pHash,
								SealKeyID:        "local_node_key",
								SealNonce:        nonce,
								HlcWallTimeMs:    record.Timestamp.WallTimeMs,
								HlcCounter:       record.Timestamp.Counter,
								HlcNodeID:        record.Timestamp.NodeID,
								Status:           status,
								CreatedAtMs:      c.clock.Now().UnixMilli(),
								UpdatedAtMs:      c.clock.Now().UnixMilli(),
							}
							if err := c.storage.SaveApplicationEvent(appEv); err != nil {
								slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
							}
						}
					}
				}
			}
		}

		job.Status = "SNAPSHOT_CREATED"
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		if err := c.storage.SaveForkHealingJob(job); err != nil {
			slog.Warn("Failed to save fork healing job", "group", c.groupID, "trace_id", job.TraceID, "error", err)
		}
		c.recordForkHealAudit(traceID, event.GroupID, "snapshot_orphan", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
	}

	// M5: 4. Fetch GroupInfo & External Join
	var newEpoch uint64
	var newState, newTreeHash []byte

	if job.Status == "SNAPSHOT_CREATED" {
		stepStart := c.clock.Now()
		c.recordForkHealAudit(traceID, event.GroupID, "proposal_join_generation", "started", stepStart, 0, "")

		// 1. Generate a fresh KeyPackage via Rust (using our unique signing key)
		kp, kpPriv, err := c.mls.GenerateKeyPackage(ctx, c.signingKey)
		if err != nil {
			c.recordForkHealAudit(traceID, event.GroupID, "proposal_join_generation", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
			c.logHealFailed(traceID, event, startedAt, scheduledAt, "proposal_join_generation", err)
			return
		}

		// 2. Wrap the KeyPackage in a ProposalJoin message
		h := sha256.Sum256(kp)
		kpHash := h[:]

		msg := ProposalMsg{
			ProposalType:   ProposalJoin,
			Data:           kp,
			TargetPeerID:   c.localID.String(),
			TargetIdentity: c.localIdentity,
			KeyPackageHash: kpHash,
			OperationID:    job.JobID,
		}

		// 3. Drop current MlsGroup state from Go memory (but keep SQLite history intact)
		c.mu.Lock()
		c.groupState = nil
		c.treeHash = nil
		// Build envelope under lock to safely access c.epoch, groupID, localID, hlc
		envBytes := c.buildEnvelopeWithTimestampLocked(MsgProposal, msg, c.hlc.Now())
		c.mu.Unlock()

		// Publish outside lock to prevent re-entrant deadlock from synchronous transport callbacks
		if len(envBytes) > 0 {
			c.publishPreparedEnvelopeLocked(MsgProposal, envBytes)
		}

		// 4. Update the persistent healing job state to PROPOSAL_SENT, cache the private key bundle
		job.Status = "PROPOSAL_SENT"
		job.PendingBundlePrivate = kpPriv
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		if saveErr := c.storage.SaveForkHealingJob(job); saveErr != nil {
			slog.Error("fork_heal/db_save_job_failed", "group", event.GroupID, "err", saveErr)
		}

		c.recordForkHealAudit(traceID, event.GroupID, "proposal_join_broadcast", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
		slog.Info("fork_heal/proposal_sent", "group", event.GroupID, "job", job.JobID, "node", c.localID)

		// 5. Suspend goroutine and await Welcome message signal (prevents early return mode leaks)
		// Retry up to 3 times: if the Token Holder was healing (groupState nil) when the
		// initial ProposalJoin arrived, a failover will elect a new Token Holder. Re-broadcasting
		// gives the new Token Holder a chance to receive and transmute the ProposalJoin.
		const maxProposalJoinRetries = 3
		for attempt := 1; attempt <= maxProposalJoinRetries; attempt++ {
			if attempt > 1 {
				slog.Info("fork_heal/proposal_join_retry", "group", event.GroupID, "attempt", attempt)
				c.mu.Lock()
				retryEnvBytes := c.buildEnvelopeWithTimestampLocked(MsgProposal, msg, c.hlc.Now())
				c.mu.Unlock()
				if len(retryEnvBytes) > 0 {
					c.publishPreparedEnvelopeLocked(MsgProposal, retryEnvBytes)
				}
			}
			select {
			case <-c.welcomeReceivedChan:
				slog.Info("fork_heal/welcome_received_signal", "group", event.GroupID)
				// Reload the job from DB to get the decrypted state and updated status (EXTERNAL_JOINED)
				if updatedJob, reloadErr := c.storage.GetActiveForkHealingJob(event.GroupID); reloadErr == nil && updatedJob != nil {
					job = updatedJob
				} else {
					slog.Error("fork_heal/reload_job_failed", "group", event.GroupID)
					c.logHealFailed(traceID, event, startedAt, scheduledAt, "reload_job", fmt.Errorf("failed to reload job from DB after welcome signal"))
					return
				}
			case <-ctx.Done():
				slog.Warn("fork_heal/awaiting_welcome_cancelled", "group", event.GroupID)
				return
			case <-c.clock.After(c.cfg.MLSOperationTimeout):
				// Grace check: ProcessWelcomeIfWaiting may have sent the
				// signal simultaneously with the timer firing. Do a
				// non-blocking check to prefer the Welcome over retry.
				select {
				case <-c.welcomeReceivedChan:
					slog.Info("fork_heal/welcome_received_signal_grace", "group", event.GroupID)
					if updatedJob, reloadErr := c.storage.GetActiveForkHealingJob(event.GroupID); reloadErr == nil && updatedJob != nil {
						job = updatedJob
					} else {
						slog.Error("fork_heal/reload_job_failed", "group", event.GroupID)
						c.logHealFailed(traceID, event, startedAt, scheduledAt, "reload_job", fmt.Errorf("failed to reload job from DB after welcome signal (grace)"))
						return
					}
				default:
					if attempt < maxProposalJoinRetries {
						slog.Warn("fork_heal/awaiting_welcome_timeout_retry", "group", event.GroupID, "attempt", attempt)
						continue
					}
					slog.Warn("fork_heal/awaiting_welcome_timeout", "group", event.GroupID)
					c.logHealFailed(traceID, event, startedAt, scheduledAt, "awaiting_welcome", fmt.Errorf("timeout waiting for Welcome message"))
					return
				}
			}
			break
		}
	}

	// M5: 5. Swap Group State & Crypto-shredding keys (DB Transaction Boundary)
	if job.Status == "EXTERNAL_JOINED" {
		stepStart := c.clock.Now()
		c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "started", stepStart, 0, "")

		if len(newState) == 0 {
			if len(job.PendingGroupState) == 0 {
				err := fmt.Errorf("pending_group_state is missing at EXTERNAL_JOINED phase")
				c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
				c.logHealFailed(traceID, event, startedAt, scheduledAt, "state_swap", err)
				return
			}
			newState = job.PendingGroupState
			newEpoch = job.PendingEpoch
			newTreeHash = job.PendingTreeHash
		}

		if err := c.applyHealedState(newState, newTreeHash, newEpoch); err != nil {
			c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "failed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), err.Error())
			c.logHealFailed(traceID, event, startedAt, scheduledAt, "state_swap", err)
			return
		}

		job.Status = "STATE_SWAPPED"
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		if err := c.storage.SaveForkHealingJob(job); err != nil {
			slog.Warn("Failed to save fork healing job", "group", c.groupID, "trace_id", job.TraceID, "error", err)
		}
		c.recordForkHealAudit(traceID, event.GroupID, "state_swap", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
	}

	// M5: 6. Autonomous Replay (Bidirectional Batched Replay)
	var replayedCount int
	if job.Status == "STATE_SWAPPED" {
		stepStart := c.clock.Now()
		c.recordForkHealAudit(traceID, event.GroupID, "replay_started", "started", stepStart, 0, "")

		// 1. Phục hồi và phát lại bất kỳ batched envelope nào còn kẹt trong outbound queue
		outboundList, err := c.storage.ListOutboundReplays(job.JobID)
		if err == nil {
			for _, outbound := range outboundList {
				if outbound.Status == "ENQUEUED" || outbound.Status == "FAILED" {
					evs, _ := c.storage.ListApplicationEvents(job.JobID)
					var matchEvs []*ApplicationEvent
					for _, ev := range evs {
						if ev.ReplayOperationID == outbound.ReplayOperationID {
							matchEvs = append(matchEvs, ev)
						}
					}
					if len(matchEvs) > 0 {
						if err := c.broadcastBatchedOutboundReplay(outbound, matchEvs); err == nil {
							replayedCount += len(matchEvs)
						}
					}
				}
			}
		}

		// 2. Gom cụm và phát lại các orphan events của mình
		replayedCount += c.batchAndReplayOutbox(ctx, job.JobID, job.GroupID)

		job.Status = "LOCAL_COMPLETE"
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		if err := c.storage.SaveForkHealingJob(job); err != nil {
			slog.Warn("Failed to save fork healing job", "group", c.groupID, "trace_id", job.TraceID, "error", err)
		}
		c.recordForkHealAudit(traceID, event.GroupID, "replay_completed", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
	}

	// M5: 7. Shredding sealed payloads & transition CLEANED
	if job.Status == "LOCAL_COMPLETE" {
		if err := c.storage.ClearSealedPayloads(job.JobID); err != nil {
			slog.Warn("Failed to clear sealed payloads", "group", c.groupID, "job_id", job.JobID, "error", err)
		}

		job.Status = "CLEANED"
		job.CompletedAtMs = c.clock.Now().UnixMilli()
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		if err := c.storage.SaveForkHealingJob(job); err != nil {
			slog.Warn("Failed to save fork healing job", "group", c.groupID, "trace_id", job.TraceID, "error", err)
		}

		completedAt := c.clock.Now()
		c.metrics.IncrForkHealingsSucceeded()
		c.metrics.RecordExternalJoin(completedAt.Sub(startedAt))
		slog.Info("fork_heal/completed",
			"trace_id", traceID,
			"group", event.GroupID,
			"outcome", "success",
			"new_epoch", job.PendingEpoch,
			"duration_ms", completedAt.Sub(startedAt).Milliseconds(),
		)

		c.recordForkHealEvent(&ForkHealEventRecord{
			TraceID:              traceID,
			GroupID:              event.GroupID,
			WinnerPeerID:         event.RemotePeer.String(),
			WinnerEpoch:          event.RemoteEpoch,
			NewEpoch:             job.PendingEpoch,
			Outcome:              "success",
			PartitionStartedAtMs: event.PartitionStartedAt.UnixMilli(),
			ScheduledAtMs:        scheduledAt.UnixMilli(),
			StartedAtMs:          startedAt.UnixMilli(),
			CompletedAtMs:        completedAt.UnixMilli(),
			DurationMs:           completedAt.Sub(startedAt).Milliseconds(),
			TotalMs:              completedAt.Sub(scheduledAt).Milliseconds(),
			ReplayedMessageCount: replayedCount,
		})

		if c.onForkHealEvent != nil {
			c.onForkHealEvent(ForkHealAuditSummary{
				GroupID:              event.GroupID,
				TraceID:              traceID,
				Stage:                "fork_heal_completed",
				WinnerPeerID:         event.RemotePeer.String(),
				WinnerEpoch:          event.RemoteAnnounce.Epoch,
				NewEpoch:             job.PendingEpoch,
				ReplayedMessageCount: replayedCount,
				DurationMs:           completedAt.Sub(startedAt).Milliseconds(),
			})
		}
	}
}

func (c *Coordinator) broadcastOutboundReplay(outbound *OutboundReplay, ev *ApplicationEvent) error {
	c.mu.Lock()
	c.appendOfflineEnvelopeLocked(outbound.ReplayEnvelope)
	c.publishPreparedEnvelopeLocked(MsgApplication, outbound.ReplayEnvelope)
	c.mu.Unlock()

	outbound.Status = "BROADCASTED"
	outbound.UpdatedAtMs = c.clock.Now().UnixMilli()
	if err := c.storage.SaveOutboundReplay(outbound); err != nil {
		return fmt.Errorf("save outbound replay broadcasted: %w", err)
	}

	ev.Status = "REPLAYED"
	ev.ReplayedAtMs = c.clock.Now().UnixMilli()
	if err := c.storage.SaveApplicationEvent(ev); err != nil {
		return fmt.Errorf("save application event replayed: %w", err)
	}

	// Mark the original stored message as replayed so the frontend can
	// suppress it once the re-broadcast copy is received and stored.
	if len(ev.EnvelopeHash) > 0 {
		now := c.clock.Now()
		if mErr := c.storage.MarkMessageReplayed(c.groupID, ev.EnvelopeHash, now); mErr != nil {
			slog.Warn("fork_heal/mark_replayed_failed", "group", c.groupID, "err", mErr)
		}
	}

	return nil
}

func phaseBeforeStateSwapped(status string) bool {
	switch status {
	case "INITIATED", "FROZEN_FOR_APPLY", "SNAPSHOT_CREATED", "EXTERNAL_JOINED":
		return true
	default:
		return false
	}
}

func (c *Coordinator) isAlreadyOnWinningBranch(job *ForkHealingJob) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If the job has not even reached EXTERNAL_JOINED state, we cannot be on the winning branch yet
	if job.Status == "INITIATED" || job.Status == "SNAPSHOT_CREATED" {
		return false
	}

	// 1. If local epoch is equal to the expected post-swap epoch, tree hash must match
	if job.PendingEpoch > 0 && c.epoch == job.PendingEpoch {
		return bytes.Equal(c.treeHash, job.PendingTreeHash)
	}

	// 2. If local epoch is equal to the winning branch's epoch, tree hash must match
	if c.epoch == job.WinningEpoch {
		return bytes.Equal(c.treeHash, job.WinningTreeHash)
	}

	return false
}

func (c *Coordinator) resumeForkHealingJob(job *ForkHealingJob) {
	slog.Info("fork_heal/resume_triggered", "group", job.GroupID, "status", job.Status)

	winnerPeer, err := peer.Decode(job.WinnerPeerID)
	if err != nil {
		winnerPeer = peer.ID(job.WinnerPeerID)
	}

	// Self-healing detection: Nếu group state cục bộ thực chất đã swap thành công trước khi crash
	if c.isAlreadyOnWinningBranch(job) && phaseBeforeStateSwapped(job.Status) {
		slog.Info("fork_heal/resume_detect_already_swapped", "group", job.GroupID, "epoch", c.epoch)
		job.Status = "STATE_SWAPPED"
		job.UpdatedAtMs = c.clock.Now().UnixMilli()
		if err := c.storage.SaveForkHealingJob(job); err != nil {
			slog.Warn("Failed to save fork healing job", "group", c.groupID, "trace_id", job.TraceID, "error", err)
		}
	}

	if !c.healing.CompareAndSwap(false, true) {
		slog.Warn("fork_heal/resume_skipped_already_healing", "group", job.GroupID)
		return
	}

	event := &ForkEvent{
		GroupID:            job.GroupID,
		RemotePeer:         winnerPeer,
		RemoteEpoch:        job.WinningEpoch,
		NeedExternalJoin:   true,
		WinnerPeers:        []peer.ID{winnerPeer},
		PartitionStartedAt: time.UnixMilli(job.CreatedAtMs),
		RemoteAnnounce: GroupStateAnnouncement{
			TreeHash: job.WinningTreeHash,
			Epoch:    job.WinningEpoch,
		},
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.runHeal(c.ctx, job.TraceID, event, time.UnixMilli(job.CreatedAtMs))
	}()
}

// ProcessWelcomeIfWaiting is called by the application layer when a Welcome message
// is received. If the coordinator is waiting for a Welcome to heal a fork, it
// processes it and resumes the healing job.
func (c *Coordinator) ProcessWelcomeIfWaiting(ctx context.Context, welcomeBytes []byte) bool {
	c.mu.Lock()
	job, err := c.storage.GetActiveForkHealingJob(c.groupID)
	if err != nil || job == nil || job.Status != "PROPOSAL_SENT" {
		c.mu.Unlock()
		return false
	}

	if len(job.PendingBundlePrivate) == 0 {
		c.mu.Unlock()
		return false
	}

	// Copy immutable properties needed for MLS call to local variables and release c.mu
	// to prevent blocking other goroutines during the heavy cryptographic MLS/gRPC call.
	signingKey := c.signingKey
	maxPastEpochs := c.cfg.GetMaxPastEpochs()
	pendingPrivate := job.PendingBundlePrivate
	c.mu.Unlock()

	groupState, treeHash, epoch, err := c.mls.ProcessWelcome(ctx, welcomeBytes, signingKey, pendingPrivate, maxPastEpochs)
	if err != nil {
		slog.Warn("fork_heal/process_welcome_failed", "group", c.groupID, "err", err)
		return false
	}

	c.mu.Lock()
	// Re-verify the active job status to protect against concurrent welcome processing races
	job, err = c.storage.GetActiveForkHealingJob(c.groupID)
	if err != nil || job == nil || job.Status != "PROPOSAL_SENT" {
		c.mu.Unlock()
		return false
	}

	job.Status = "EXTERNAL_JOINED"
	job.WinningEpoch = epoch
	job.WinningTreeHash = treeHash
	job.PendingGroupState = groupState
	job.PendingEpoch = epoch
	job.PendingTreeHash = treeHash
	job.UpdatedAtMs = c.clock.Now().UnixMilli()
	if err := c.storage.SaveForkHealingJob(job); err != nil {
		slog.Warn("Failed to save fork healing job", "group", c.groupID, "trace_id", job.TraceID, "error", err)
	}

	c.mu.Unlock()

	// Signal the waiting runHeal goroutine to proceed with state swap.
	// runHeal reloads the job from DB and finds status=EXTERNAL_JOINED.
	select {
	case c.welcomeReceivedChan <- struct{}{}:
	default:
	}

	return true
}

func (c *Coordinator) fetchGroupInfoForHeal(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*GroupInfoFetchResult, error) {
	if c.groupInfoFetch == nil {
		return nil, fmt.Errorf("group-info fetcher not configured")
	}
	gi, err := c.groupInfoFetch(ctx, remote, groupID, withRatchetTree)
	if err != nil {
		return nil, err
	}
	if gi == nil {
		return nil, fmt.Errorf("empty group-info response")
	}
	if len(gi.GroupInfo) == 0 {
		return nil, fmt.Errorf("group-info response missing payload")
	}
	return gi, nil
}

func (c *Coordinator) applyHealedState(newState, newTreeHash []byte, newEpoch uint64) error {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	prevRec, err := c.storage.GetGroupRecord(c.groupID)
	if err != nil && !errors.Is(err, ErrGroupNotFound) {
		return fmt.Errorf("load group record: %w", err)
	}

	role := RoleMember
	groupType := ""
	categoryID := ""
	createdAt := now
	if prevRec != nil {
		if prevRec.MyRole != "" {
			role = prevRec.MyRole
		}
		groupType = prevRec.GroupType
		categoryID = prevRec.CategoryID
		if !prevRec.CreatedAt.IsZero() {
			createdAt = prevRec.CreatedAt
		}
	}
	if err := c.storage.SaveGroupRecord(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      newEpoch,
		TreeHash:   newTreeHash,
		MyRole:     role,
		GroupType:  groupType,
		CategoryID: categoryID,
		CreatedAt:  createdAt,
		UpdatedAt:  now,
	}); err != nil {
		return fmt.Errorf("persist healed group state: %w", err)
	}

	c.groupState = append([]byte(nil), newState...)
	c.treeHash = append([]byte(nil), newTreeHash...)
	c.epoch = newEpoch
	c.epochTracker = NewEpochTracker(newEpoch, newTreeHash)
	c.singleWriter = NewSingleWriter(c.activeView, c.localID, newEpoch, c.cfg)
	c.singleWriter.SetAuthorizedCommitters(c.groupID, c.authorizedCommitters)

	commitHash := hashCommitData(nil)
	c.lastCommitHash = copyBytes(commitHash)

	// Reset history chain for the winning branch. After external join, the
	// node starts a fresh chain rooted at the new epoch. R(newEpoch) is
	// seeded from initialHistoryHash so all healed nodes converge.
	c.historyChain = make(map[uint64][]byte)
	c.historyHash = initialHistoryHash(c.groupID)
	c.historyChain[newEpoch] = copyBytes(c.historyHash)

	c.forkDetector.Reset()
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    newTreeHash,
		MemberCount: c.activeView.Size(),
		Epoch:       newEpoch,
		CommitHash:  commitHash,
		HistoryHash: copyBytes(c.historyHash),
	})
	if err := c.persistCoordStateLocked(); err != nil {
		return fmt.Errorf("persist healed coordination state: %w", err)
	}
	if c.onEpochChange != nil {
		c.onEpochChange(newEpoch)
	}
	return nil
}

func (c *Coordinator) logHealFailed(traceID string, event *ForkEvent, startedAt, scheduledAt time.Time, step string, err error) {
	completedAt := c.clock.Now()
	slog.Error("fork_heal/failed",
		"trace_id", traceID,
		"group", event.GroupID,
		"step", step,
		"error", err,
		"duration_ms", completedAt.Sub(startedAt).Milliseconds(),
		"total_ms", completedAt.Sub(scheduledAt).Milliseconds(),
	)
	slog.Info("fork_heal/aggregate",
		"trace_id", traceID,
		"group", event.GroupID,
		"winner_peer", event.RemotePeer.String(),
		"winner_epoch", event.RemoteEpoch,
		"outcome", "failed",
		"failed_step", step,
		"partition_window_ms", completedAt.Sub(event.PartitionStartedAt).Milliseconds(),
		"total_ms", completedAt.Sub(scheduledAt).Milliseconds(),
	)
	c.recordForkHealEvent(&ForkHealEventRecord{
		TraceID:              traceID,
		GroupID:              event.GroupID,
		WinnerPeerID:         event.RemotePeer.String(),
		WinnerEpoch:          event.RemoteEpoch,
		NewEpoch:             c.CurrentEpoch(),
		Outcome:              "failed",
		FailedStep:           step,
		WinnerTreeHash:       append([]byte(nil), event.RemoteAnnounce.TreeHash...),
		NewTreeHash:          append([]byte(nil), c.GetTreeHash()...),
		PartitionStartedAtMs: event.PartitionStartedAt.UnixMilli(),
		ScheduledAtMs:        scheduledAt.UnixMilli(),
		StartedAtMs:          startedAt.UnixMilli(),
		CompletedAtMs:        completedAt.UnixMilli(),
		DurationMs:           completedAt.Sub(startedAt).Milliseconds(),
		TotalMs:              completedAt.Sub(scheduledAt).Milliseconds(),
		ReplayedMessageCount: 0,
	})
	if c.onForkHealEvent != nil {
		c.onForkHealEvent(ForkHealAuditSummary{
			GroupID:      event.GroupID,
			TraceID:      traceID,
			Stage:        "fork_heal_failed",
			WinnerPeerID: event.RemotePeer.String(),
			WinnerEpoch:  event.RemoteEpoch,
			NewEpoch:     c.CurrentEpoch(),
			FailedStep:   step,
		})
	}
}

func (c *Coordinator) recordForkHealAudit(traceID, groupID, step, status string, ts time.Time, durationMs int64, errMsg string) {
	if traceID == "" || groupID == "" || step == "" || status == "" {
		return
	}
	if err := c.storage.RecordForkHealAudit(&ForkHealAuditRecord{
		TraceID:     traceID,
		GroupID:     groupID,
		Step:        step,
		Status:      status,
		TimestampMs: ts.UnixMilli(),
		DurationMs:  durationMs,
		Error:       errMsg,
	}); err != nil {
		slog.Warn("fork_heal/audit_persist_failed", "trace_id", traceID, "group", groupID, "step", step, "status", status, "err", err)
	}
}

func (c *Coordinator) recordForkHealEvent(event *ForkHealEventRecord) {
	if event == nil {
		return
	}
	if err := c.storage.RecordForkHealEvent(event); err != nil {
		slog.Warn("fork_heal/event_persist_failed", "trace_id", event.TraceID, "group", event.GroupID, "err", err)
	}
}
