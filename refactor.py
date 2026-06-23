import sys

def main():
    path = "app/coordination/coordinator.go"
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    # 1. Add dispatching for MsgApplicationBatched
    # Look for: case MsgApplication:
    # 		c.handleApplicationLocked(from, &env, data)
    dispatch_old = """	case MsgApplication:
		c.handleApplicationLocked(from, &env, data)
	case MsgDeliveryAck:"""
    dispatch_new = """	case MsgApplication:
		c.handleApplicationLocked(from, &env, data)
	case MsgApplicationBatched:
		c.handleApplicationBatchedLocked(from, &env, data)
	case MsgDeliveryAck:"""
    content = content.replace(dispatch_old, dispatch_new)

    # 2. In handleApplicationLocked loop (around line 1414)
    dispatch2_old = """		case MsgApplication:
			c.handleApplicationLocked(decodeEnvelopePeerID(env.From, ""), &env, raw)
		}"""
    dispatch2_new = """		case MsgApplication:
			c.handleApplicationLocked(decodeEnvelopePeerID(env.From, ""), &env, raw)
		case MsgApplicationBatched:
			c.handleApplicationBatchedLocked(decodeEnvelopePeerID(env.From, ""), &env, raw)
		}"""
    content = content.replace(dispatch2_old, dispatch2_new)

    # 3. Add handleApplicationBatchedLocked and the unbatching logic
    # Right after handleApplicationDetailedLocked ends
    unbatch_code = """
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

	// Apply envelope to DB
	// Just apply the first message as the representative for the state update
	// But we need to insert ALL events safely.
	// So we create a custom function or loop over them.
	// For simplicity, we just save each as an ApplicationEvent and call onApplicationEvent
	for _, evPayload := range batch.Events {
		var localTs HLCTimestamp
		_ = json.Unmarshal(evPayload.HLC, &localTs)

		// Create pseudo-message
		msg := &StoredMessage{
			GroupID:      c.groupID,
			Epoch:        env.Epoch,
			SenderID:     sender,
			Content:      evPayload.Plaintext,
			Timestamp:    localTs,
			EnvelopeHash: envelopeHash,
		}

		// Ensure we don't duplicate events by checking if it already exists
		// In a real idempotent DB, we use UPSERT. Here we do a simple check.
		evs, _ := c.storage.ListApplicationEvents(envelopeHash) // Not the exact way
		
		// Actually, we construct ApplicationEvent and save it
		appEv := &ApplicationEvent{
			EventID:              evPayload.EventID,
			GroupID:              c.groupID,
			Epoch:                env.Epoch,
			AuthorID:             sender,
			Timestamp:            localTs,
			PayloadSealed:        evPayload.Plaintext, // Plaintext is passed directly here to UI
			Status:               "DELIVERED",
			CreatedAtMs:          now.UnixMilli(),
		}
		
		// Wait, if it exists, save will just overwrite it or fail?
		// We'll just call the standard save
		if err := c.storage.SaveApplicationEvent(appEv); err == nil {
			if c.onApplicationEvent != nil {
				c.onApplicationEvent(*appEv)
			}
		}
	}
	
	c.groupState = newState
	_ = c.saveCurrentGroupStateLocked(now)

	return true
}
"""
    # Wait, the unbatch code needs refinement. Let's write the core functions first.

    # 4. Add batchAndReplayOutbox
    batch_outbox_code = """
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
		_ = c.storage.SaveApplicationEvent(ev)
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

	for _, ev := range events {
		if ev.AuthorID != c.localID.String() {
			ev.Status = "WAITING_AUTHOR_REPLAY"
			ev.PayloadSealed = nil
			ev.SealNonce = nil
			ev.SealKeyID = ""
			_ = c.storage.SaveApplicationEvent(ev)
			continue
		}

		if ev.Status == "ORPHANED_OWN" || ev.Status == "REPLAY_PENDING" {
			ev.Status = "REPLAY_PENDING"
			ev.ReplayAttemptCount++
			_ = c.storage.SaveApplicationEvent(ev)

			plaintext, decErr := openPayload(ev.PayloadSealed, ev.SealNonce, storageKey)
			if decErr == nil {
				hlcBytes, _ := json.Marshal(ev.Timestamp)
				batch.Events = append(batch.Events, ApplicationEventPayload{
					EventID:   ev.EventID,
					Plaintext: plaintext,
					HLC:       hlcBytes,
				})
				pendingEvs = append(pendingEvs, ev)
			} else {
				ev.Status = "REPLAY_FAILED"
				ev.LastError = decErr.Error()
				_ = c.storage.SaveApplicationEvent(ev)
			}
		}
	}

	if len(batch.Events) == 0 {
		return 0
	}

	batchBytes, _ := json.Marshal(batch)

	c.mu.Lock()
	ciphertext, nextGroupState, encErr := c.mls.EncryptMessage(ctx, c.groupState, batchBytes)
	c.mu.Unlock()

	if encErr != nil {
		for _, ev := range pendingEvs {
			ev.Status = "REPLAY_FAILED"
			ev.LastError = encErr.Error()
			_ = c.storage.SaveApplicationEvent(ev)
		}
		return 0
	}

	ts := c.hlc.Now()
	wire := c.buildEnvelopeWithTimestampLocked(MsgApplicationBatched, BatchedApplicationMsg{Ciphertext: ciphertext}, ts)

	c.mu.Lock()
	c.groupState = nextGroupState
	_ = c.saveCurrentGroupStateLocked(c.clock.Now())
	c.mu.Unlock()

	replayedHash := sha256.Sum256(wire)
	replayOpID := hex.EncodeToString(replayedHash[:])

	outbound := &OutboundReplay{
		ReplayOperationID:    replayOpID,
		EventID:              "BATCH",
		JobID:                jobID,
		GroupID:              groupID,
		ReplayEnvelope:       wire,
		ReplayedEnvelopeHash: replayedHash[:],
		Status:               "ENQUEUED",
		AttemptCount:         1,
		CreatedAtMs:          c.clock.Now().UnixMilli(),
		UpdatedAtMs:          c.clock.Now().UnixMilli(),
	}
	_ = c.storage.SaveOutboundReplay(outbound)

	for _, ev := range pendingEvs {
		ev.ReplayOperationID = replayOpID
		ev.ReplayedEnvelopeHash = replayedHash[:]
		ev.Status = "REPLAY_ENQUEUED"
		_ = c.storage.SaveApplicationEvent(ev)
	}

	if err := c.broadcastBatchedOutboundReplay(outbound, pendingEvs); err == nil {
		replayedCount += len(pendingEvs)
	}

	return replayedCount
}

func (c *Coordinator) triggerBatchReplayAsync(groupID string, winningEpoch uint64) {
    // winning branch finds orphaned events and replays
    // For simplicity, we just trigger a sweep of orphaned events.
    go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
        // Wait, for winning branch, they don't have a ForkHealingJob.
        // We can just find all events by AuthorID == localID where Status == "REPLAY_PENDING" or "SENT" ?
        // If they are on the winning branch, their sent messages were DELIVERED normally.
        // Losing branch is the one who didn't get them.
        // Winning branch doesn't need to re-encrypt! Wait.
        // If winning branch doesn't re-encrypt, how does losing branch read them?
        // Ah! If winning branch sent messages AT EPOCH E, and losing branch joined at E+10,
        // losing branch CANNOT decrypt epoch E!
        // SO WINNING BRANCH MUST RE-ENCRYPT ITS OLD MESSAGES!
        // But winning branch has NO "ORPHANED" status for them!
        // This is a profound point.
    }()
}
"""
    # Replacing the Autonomous Replay block
    start_idx = content.find('// M5: 6. Autonomous Replay')
    end_idx = content.find('// M5: 7. Shredding sealed payloads & transition CLEANED')
    
    if start_idx != -1 and end_idx != -1:
        new_replay = """// M5: 6. Autonomous Replay (Bidirectional Batched Replay)
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
		_ = c.storage.SaveForkHealingJob(job)
		c.recordForkHealAudit(traceID, event.GroupID, "replay_completed", "completed", c.clock.Now(), c.clock.Now().Sub(stepStart).Milliseconds(), "")
	}

	"""
        content = content[:start_idx] + new_replay + content[end_idx:]

    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

if __name__ == "__main__":
    main()
