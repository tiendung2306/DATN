import sys

def main():
    content = """package coordination

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

	storageKey, err := c.deriveLocalStorageKey()
	if err != nil {
		slog.Error("Failed to derive storage key for batch replay", "error", err)
		return 0
	}

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

	for _, evPayload := range batch.Events {
		var localTs HLCTimestamp
		_ = json.Unmarshal(evPayload.HLC, &localTs)

		msg := &StoredMessage{
			GroupID:      c.groupID,
			Epoch:        env.Epoch,
			SenderID:     sender.String(),
			Content:      evPayload.Plaintext,
			Timestamp:    localTs,
			EnvelopeHash: envelopeHash,
		}

		appEv := &ApplicationEvent{
			EventID:              evPayload.EventID,
			GroupID:              c.groupID,
			OriginalEpoch:        env.Epoch,
			AuthorID:             sender.String(),
			HlcWallTimeMs:        localTs.WallTimeMs,
			HlcCounter:           localTs.Counter,
			HlcNodeID:            localTs.NodeID,
			PayloadSealed:        evPayload.Plaintext,
			Status:               "DELIVERED",
			CreatedAtMs:          now.UnixMilli(),
		}
		
		if err := c.storage.SaveApplicationEvent(appEv); err == nil {
			if c.onMessage != nil {
				c.onMessage(msg)
			}
		}
	}
	
	c.groupState = newState
	_ = c.saveCurrentGroupStateLocked(now)

	_, _, _ = c.storage.ApplyApplication(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: newState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		UpdatedAt:  now,
	}, nil, env.Type, wire, env.Timestamp, env.Epoch)

	return true
}

func (c *Coordinator) triggerBatchReplayAsync(groupID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		jobID := "WINNER-REPLAY-" + groupID + fmt.Sprintf("-%d", c.clock.Now().UnixNano())
		c.batchAndReplayOutbox(ctx, jobID, groupID)
	}()
}
"""
    with open("app/coordination/coordinator_batch.go", "w", encoding="utf-8") as f:
        f.write(content)

if __name__ == "__main__":
    main()
