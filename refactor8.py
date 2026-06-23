import sys

def main():
    path = "app/coordination/coordinator_batch.go"
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    # We need to implement chunking in batchAndReplayOutbox
    old_batch_logic = """	var batch BatchedPlaintext
	var pendingEvs []*ApplicationEvent

	storageKey := deriveStorageKey(c.signingKey)


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
	
	c.mu.Lock()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgApplicationBatched, BatchedApplicationMsg{Ciphertext: ciphertext}, ts)
	c.groupState = nextGroupState
	_ = c.saveCurrentGroupStateLocked(c.clock.Now())
	c.mu.Unlock()

	replayedHash := sha256.Sum256(envBytes)
	replayOpID := hex.EncodeToString(replayedHash[:])

	outbound := &OutboundReplay{
		ReplayOperationID:    replayOpID,
		EventID:              "BATCH",
		JobID:                jobID,
		GroupID:              groupID,
		ReplayEnvelope:       envBytes,
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
	}"""
    
    new_batch_logic = """	storageKey := deriveStorageKey(c.signingKey)
	var allPendingEvs []*ApplicationEvent

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
			allPendingEvs = append(allPendingEvs, ev)
		}
	}

	if len(allPendingEvs) == 0 {
		return 0
	}

	const maxEventsPerBatch = 50
	for i := 0; i < len(allPendingEvs); i += maxEventsPerBatch {
		end := i + maxEventsPerBatch
		if end > len(allPendingEvs) {
			end = len(allPendingEvs)
		}
		chunk := allPendingEvs[i:end]

		var batch BatchedPlaintext
		var pendingEvs []*ApplicationEvent

		for _, ev := range chunk {
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
				_ = c.storage.SaveApplicationEvent(ev)
			}
		}

		if len(batch.Events) == 0 {
			continue
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
			continue
		}

		ts := c.hlc.Now()

		c.mu.Lock()
		envBytes := c.buildEnvelopeWithTimestampLocked(MsgApplicationBatched, BatchedApplicationMsg{Ciphertext: ciphertext}, ts)
		c.groupState = nextGroupState
		_ = c.saveCurrentGroupStateLocked(c.clock.Now())
		c.mu.Unlock()

		replayedHash := sha256.Sum256(envBytes)
		replayOpID := hex.EncodeToString(replayedHash[:])

		outbound := &OutboundReplay{
			ReplayOperationID:    replayOpID,
			EventID:              "BATCH",
			JobID:                jobID,
			GroupID:              groupID,
			ReplayEnvelope:       envBytes,
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
	}"""
    
    content = content.replace(old_batch_logic, new_batch_logic)
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

if __name__ == "__main__":
    main()
