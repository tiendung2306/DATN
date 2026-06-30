package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (c *Coordinator) broadcastBatchedOutboundReplay(outbound *OutboundReplay, evs []*ApplicationEvent) error {
	c.mu.Lock()
	c.appendOfflineEnvelopeLocked(outbound.ReplayEnvelope)
	c.publishPreparedEnvelopeLocked(MsgApplicationBatched, outbound.ReplayEnvelope)
	c.mu.Unlock()

	outbound.Status = "BROADCASTED"
	outbound.UpdatedAtMs = c.clock.Now().UnixMilli()
	if err := c.storage.SaveOutboundReplay(outbound); err != nil {
		return err
	}

	for _, ev := range evs {
		ev.Status = "REPLAYED"
		ev.ReplayedAtMs = c.clock.Now().UnixMilli()
		if err := c.storage.SaveApplicationEvent(ev); err != nil {
			slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
		}

		if len(ev.EnvelopeHash) > 0 {
			now := c.clock.Now()
			if mErr := c.storage.MarkMessageReplayed(c.groupID, ev.EnvelopeHash, now); mErr != nil {
				slog.Warn("fork_heal/mark_replayed_failed (batched)", "group", c.groupID, "err", mErr)
			}
		}
	}
	return nil
}

func (c *Coordinator) batchAndReplayOutbox(ctx context.Context, jobID, groupID string) int {
	var replayedCount int
	events, err := c.storage.ListApplicationEvents(jobID)
	if err != nil {
		return 0
	}

	var batch BatchedPlaintext
	var pendingEvs []*ApplicationEvent

	storageKey := deriveStorageKey(c.signingKey)

	for _, ev := range events {
		if ev.AuthorID != c.localID.String() {
			ev.Status = "WAITING_AUTHOR_REPLAY"
			ev.PayloadSealed = nil
			ev.SealNonce = nil
			ev.SealKeyID = ""
			if err := c.storage.SaveApplicationEvent(ev); err != nil {
				slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
			}
			continue
		}

		if ev.Status == "ORPHANED_OWN" || ev.Status == "REPLAY_PENDING" {
			if len(ev.PayloadSealed) == 0 || len(ev.SealNonce) == 0 {
				ev.Status = "WAITING_AUTHOR_REPLAY"
				if err := c.storage.SaveApplicationEvent(ev); err != nil {
					slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
				}
				continue
			}
			ev.Status = "REPLAY_PENDING"
			ev.ReplayAttemptCount++
			if err := c.storage.SaveApplicationEvent(ev); err != nil {
				slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
			}

			plaintext, decErr := openPayload(ev.PayloadSealed, ev.SealNonce, storageKey)
			if decErr == nil {
				localTs := HLCTimestamp{
					WallTimeMs: ev.HlcWallTimeMs,
					Counter:    ev.HlcCounter,
					NodeID:     ev.HlcNodeID,
				}
				hlcBytes, _ := json.Marshal(localTs)
				batch.Events = append(batch.Events, ApplicationEventPayload{
					EventID:   ev.EventID,
					Plaintext: plaintext,
					HLC:       hlcBytes,
				})
				pendingEvs = append(pendingEvs, ev)
			} else {
				ev.Status = "REPLAY_FAILED"
				ev.LastError = decErr.Error()
				if err := c.storage.SaveApplicationEvent(ev); err != nil {
					slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
				}
			}
		}
	}

	if len(batch.Events) == 0 {
		return 0
	}

	const MaxBatchSize = 50
	var batches []BatchedPlaintext
	var pendingChunks [][]*ApplicationEvent

	for i := 0; i < len(batch.Events); i += MaxBatchSize {
		end := i + MaxBatchSize
		if end > len(batch.Events) {
			end = len(batch.Events)
		}
		batches = append(batches, BatchedPlaintext{Events: batch.Events[i:end]})
		pendingChunks = append(pendingChunks, pendingEvs[i:end])
	}

	for i, b := range batches {
		chunkPending := pendingChunks[i]
		batchBytes, _ := json.Marshal(b)

		c.mu.Lock()
		ciphertext, nextGroupState, encErr := c.mls.EncryptMessage(ctx, c.groupState, batchBytes)
		c.mu.Unlock()

		if encErr != nil {
			for _, ev := range chunkPending {
				ev.Status = "REPLAY_FAILED"
				ev.LastError = encErr.Error()
				if err := c.storage.SaveApplicationEvent(ev); err != nil {
					slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
				}
			}
			continue
		}

		ts := c.hlc.Now()

		c.mu.Lock()
		envBytes := c.buildEnvelopeWithTimestampLocked(MsgApplicationBatched, BatchedApplicationMsg{Ciphertext: ciphertext}, ts)
		c.groupState = nextGroupState
		if err := c.saveCurrentGroupStateLocked(c.clock.Now()); err != nil {
			slog.Warn("Failed to save group state during batch replay", "group", c.groupID, "error", err)
		}
		c.mu.Unlock()

		replayedHash := sha256.Sum256(envBytes)
		replayOpID := hex.EncodeToString(replayedHash[:])

		outbound := &OutboundReplay{
			ReplayOperationID:    replayOpID,
			EventID:              fmt.Sprintf("BATCH-%d", i),
			JobID:                jobID,
			GroupID:              groupID,
			ReplayEnvelope:       envBytes,
			ReplayedEnvelopeHash: replayedHash[:],
			Status:               "ENQUEUED",
			AttemptCount:         1,
			CreatedAtMs:          c.clock.Now().UnixMilli(),
			UpdatedAtMs:          c.clock.Now().UnixMilli(),
		}
		if err := c.storage.SaveOutboundReplay(outbound); err != nil {
			slog.Warn("Failed to save outbound replay", "group", c.groupID, "replay_op_id", replayOpID, "error", err)
		}

		for _, ev := range chunkPending {
			ev.ReplayOperationID = replayOpID
			ev.ReplayedEnvelopeHash = replayedHash[:]
			ev.Status = "REPLAY_ENQUEUED"
			if err := c.storage.SaveApplicationEvent(ev); err != nil {
				slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
			}
		}

		if err := c.broadcastBatchedOutboundReplay(outbound, chunkPending); err == nil {
			replayedCount += len(chunkPending)
		}
	}

	return replayedCount
}

func (c *Coordinator) handleApplicationBatchedLocked(from peer.ID, env *Envelope, wire []byte) bool {
	result := c.newReplayResultLocked(env, wire)
	envelopeHash, alreadyApplied := c.checkAppliedEnvelopeLocked(env, wire)
	result.EnvelopeHash = envelopeHash
	if alreadyApplied {
		result.State = ReplayStateDuplicateApplied
		result.AlreadyApplied = true
		result.CursorSafe = true
		result.Terminal = true
		c.markReplayResultLocked(result)
		return true
	}

	if c.epochTracker == nil {
		c.epochTracker = NewEpochTracker(c.epoch, c.treeHash)
	}
	action := c.epochTracker.Validate(env.Epoch)
	maxPastEpochs := uint64(c.cfg.GetMaxPastEpochs())
	if action == ActionRejectStale && env.Epoch+maxPastEpochs >= c.epoch {
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
			slog.Warn("Rejected late-arriving stale batched application message", "ageSeconds", ageSeconds)
		} else {
			action = ActionProcess
		}
	}

	if action != ActionProcess {
		return false
	}

	var batchMsg BatchedApplicationMsg
	if err := json.Unmarshal(env.Payload, &batchMsg); err != nil {
		return false
	}

	opCtx, cancel := c.mlsOperationContext()
	plaintext, newState, err := c.mls.DecryptMessage(opCtx, c.groupState, batchMsg.Ciphertext)
	cancel()
	if err != nil {
		slog.Error("Failed to decrypt batched message", "error", err)
		return false
	}

	var batch BatchedPlaintext
	if err := json.Unmarshal(plaintext, &batch); err != nil {
		slog.Error("Failed to unmarshal batched plaintext", "error", err)
		return false
	}

	sender := decodeEnvelopePeerID(env.From, from)
	now := c.clock.Now()

	var msgs []*StoredMessage
	storageKey := deriveStorageKey(c.signingKey)

	for _, evPayload := range batch.Events {
		var localTs HLCTimestamp
		_ = json.Unmarshal(evPayload.HLC, &localTs)

		msg := &StoredMessage{
			GroupID:      c.groupID,
			Epoch:        env.Epoch,
			SenderID:     sender,
			Content:      evPayload.Plaintext,
			Timestamp:    localTs,
			EnvelopeHash: envelopeHash,
		}
		msgs = append(msgs, msg)

		sealedPayload, nonce, sealErr := sealPayload(evPayload.Plaintext, storageKey)
		var h []byte
		if sealErr == nil {
			hTmp := sha256.Sum256(evPayload.Plaintext)
			h = hTmp[:]
		}

		appEv := &ApplicationEvent{
			EventID:       evPayload.EventID,
			GroupID:       c.groupID,
			OriginalEpoch: env.Epoch,
			AuthorID:      sender.String(),
			HlcWallTimeMs: localTs.WallTimeMs,
			HlcCounter:    localTs.Counter,
			HlcNodeID:     localTs.NodeID,
			PayloadSealed: sealedPayload,
			PayloadHash:   h,
			SealKeyID:     "local_node_key",
			SealNonce:     nonce,
			Status:        "DELIVERED",
			CreatedAtMs:   now.UnixMilli(),
		}

		if err := c.storage.SaveApplicationEvent(appEv); err == nil {
			if c.onMessage != nil {
				c.onMessage(msg)
			}
		}
	}

	c.groupState = newState
	if err := c.saveCurrentGroupStateLocked(now); err != nil {
		slog.Warn("Failed to save group state after batched application", "group", c.groupID, "error", err)
	}

	if _, _, err := c.storage.ApplyBatchedApplication(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		UpdatedAt:  now,
	}, msgs, env.Type, wire, env.Timestamp, env.Epoch); err != nil {
		slog.Warn("Failed to apply batched application", "group", c.groupID, "error", err)
	}

	return true
}

func (c *Coordinator) triggerBatchReplayAsync(groupID string) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		baseCtx := c.ctx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		ctx, cancel := context.WithTimeout(baseCtx, 10*time.Second)
		defer cancel()
		jobID := "WINNER-REPLAY-" + groupID + fmt.Sprintf("-%d", c.clock.Now().UnixNano())
		c.batchAndReplayOutbox(ctx, jobID, groupID)
	}()
}
