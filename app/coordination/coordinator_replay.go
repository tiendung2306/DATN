package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// ReplayEnvelopesDetailed applies ciphertext envelopes from offline sync / DHT
// in order and reports whether each envelope was only seen, actually applied,
// or blocked behind a future epoch.
// Caller must hold no Coordinator lock; this method is fully synchronized.
func (c *Coordinator) ReplayEnvelopesDetailed(blobs [][]byte) ([]ReplayEnvelopeResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil, fmt.Errorf("coordinator not started")
	}

	results := make([]ReplayEnvelopeResult, 0, len(blobs))
	for _, raw := range blobs {
		if len(raw) == 0 {
			continue
		}
		var env Envelope
		if jerr := json.Unmarshal(raw, &env); jerr != nil {
			results = append(results, ReplayEnvelopeResult{
				EnvelopeHash: hashEnvelope(raw),
				State:        ReplayStateInvalid,
				Error:        jerr.Error(),
				Terminal:     true,
			})
			continue
		}
		if env.GroupID != c.groupID {
			results = append(results, ReplayEnvelopeResult{
				GroupID:      env.GroupID,
				MsgType:      env.Type,
				EnvelopeHash: hashEnvelope(raw),
				State:        ReplayStateInvalid,
				MsgEpoch:     env.Epoch,
				LocalEpoch:   c.epoch,
				Error:        fmt.Sprintf("envelope group %q does not match coordinator group %q", env.GroupID, c.groupID),
				Terminal:     true,
			})
			continue
		}

		// P0.2 Clock Skew protection:
		nowMs := c.clock.Now().UnixMilli()
		if err := validateSenderTimestamp(nowMs, env.Timestamp.WallTimeMs); err != nil {
			results = append(results, ReplayEnvelopeResult{
				GroupID:      env.GroupID,
				MsgType:      env.Type,
				EnvelopeHash: hashEnvelope(raw),
				State:        ReplayStateInvalid,
				MsgEpoch:     env.Epoch,
				LocalEpoch:   c.epoch,
				Error:        err.Error(),
				Terminal:     true,
				CursorSafe:   false, // future skewed timestamp should not advance cursor!
			})
			continue
		}
		switch env.Type {
		case MsgCommit:
			results = append(results, c.handleCommitDetailedLocked(&env, raw))
		case MsgApplication:
			results = append(results, c.handleApplicationDetailedLocked(decodeEnvelopePeerID(env.From, ""), &env, raw))
		default:
			results = append(results, ReplayEnvelopeResult{
				GroupID:      env.GroupID,
				MsgType:      env.Type,
				EnvelopeHash: hashEnvelope(raw),
				State:        ReplayStateInvalid,
				MsgEpoch:     env.Epoch,
				LocalEpoch:   c.epoch,
				Error:        fmt.Sprintf("unsupported envelope type %q", env.Type),
				Terminal:     true,
			})
			continue
		}
	}
	return results, nil
}

// ReplayEnvelopes applies ciphertext envelopes from offline sync / DHT in order.
// It is kept as a compatibility wrapper for older callers/tests.
func (c *Coordinator) ReplayEnvelopes(blobs [][]byte) (applied int, err error) {
	results, err := c.ReplayEnvelopesDetailed(blobs)
	if err != nil {
		return 0, err
	}
	for _, result := range results {
		if result.State == ReplayStateApplied {
			applied++
		}
	}
	return applied, nil
}

func (c *Coordinator) newReplayResultLocked(env *Envelope, wire []byte) ReplayEnvelopeResult {
	if env == nil {
		return ReplayEnvelopeResult{
			EnvelopeHash: hashEnvelope(wire),
			State:        ReplayStateInvalid,
			LocalEpoch:   c.epoch,
		}
	}
	return ReplayEnvelopeResult{
		GroupID:      env.GroupID,
		MsgType:      env.Type,
		EnvelopeHash: hashEnvelope(wire),
		MsgEpoch:     env.Epoch,
		LocalEpoch:   c.epoch,
	}
}

func (c *Coordinator) markReplayResultLocked(result ReplayEnvelopeResult) {
	if len(result.EnvelopeHash) == 0 || result.State == "" {
		return
	}
	if err := c.storage.MarkEnvelopeReplayState(c.groupID, result.EnvelopeHash, result.State, result.Error, c.clock.Now()); err != nil {
		slog.Warn("Failed to mark envelope replay state", "group", c.groupID, "state", result.State, "err", err)
	}
}

func (c *Coordinator) checkAppliedEnvelopeLocked(env *Envelope, wire []byte) ([]byte, bool) {
	if len(wire) == 0 || (env.Type != MsgCommit && env.Type != MsgApplication) {
		return nil, false
	}
	envelopeHash := hashEnvelope(wire)
	applied, err := c.storage.HasAppliedEnvelope(c.groupID, envelopeHash)
	if err != nil {
		slog.Warn("Failed to query applied envelope", "group", c.groupID, "type", env.Type, "err", err)
		return envelopeHash, false
	}
	return envelopeHash, applied
}

func hashEnvelope(wire []byte) []byte {
	if len(wire) == 0 {
		return nil
	}
	sum := sha256.Sum256(wire)
	return sum[:]
}

func hashCommitData(commitData []byte) []byte {
	if len(commitData) == 0 {
		return nil
	}
	sum := sha256.Sum256(commitData)
	return sum[:]
}

// initialHistoryHash returns the deterministic genesis R(0) for a group.
// R(0) = SHA-256("phoenix/history/v1" ∥ groupID). This is the same on every
// node so that all nodes starting at epoch 0 share the same chain anchor
// without any coordination.
func initialHistoryHash(groupID string) []byte {
	h := sha256.New()
	h.Write([]byte("phoenix/history/v1"))
	h.Write([]byte(groupID))
	return h.Sum(nil)
}

// computeHistoryHash calculates R(E) = H(R(E-1) ∥ CommitHash(E)).
func computeHistoryHash(prevHistoryHash, commitHash []byte) []byte {
	h := sha256.New()
	h.Write(prevHistoryHash)
	h.Write(commitHash)
	return h.Sum(nil)
}

func (c *Coordinator) markInvalidCommitLocked(commitHash []byte) {
	if len(commitHash) == 0 || c.forkDetector == nil {
		return
	}
	c.forkDetector.MarkInvalidCommit(commitHash)
}

func (c *Coordinator) collectReplayWindowMessages(partitionStart, healStartedAt time.Time) ([]*StoredMessage, error) {
	if partitionStart.IsZero() {
		return nil, nil
	}
	startMs := partitionStart.UnixMilli()
	endMs := healStartedAt.UnixMilli()
	msgs, err := c.storage.GetMessagesByOwnerInRange(c.groupID, c.localID.String(), startMs, endMs)
	if err != nil {
		return nil, fmt.Errorf("GetMessagesByOwnerInRange: %w", err)
	}
	return msgs, nil
}

func (c *Coordinator) replayWindowMessages(ctx context.Context, window []*StoredMessage) (int, error) {
	if len(window) == 0 {
		return 0, nil
	}
	throttle := time.Duration(c.cfg.ReplayThrottleMs) * time.Millisecond
	replayed := 0
	for i, msg := range window {
		if ctx.Err() != nil {
			return replayed, ctx.Err()
		}
		c.mu.Lock()
		ciphertext, newState, err := c.mls.EncryptMessage(ctx, c.groupState, msg.Content)
		if err != nil {
			c.mu.Unlock()
			return replayed, fmt.Errorf("EncryptMessage replay idx=%d: %w", i, err)
		}
		ts := c.hlc.Now()
		wire := c.buildEnvelopeWithTimestampLocked(MsgApplication, ApplicationMsg{Ciphertext: ciphertext}, ts)
		if len(wire) == 0 {
			c.mu.Unlock()
			return replayed, fmt.Errorf("encode replay application envelope idx=%d", i)
		}
		c.groupState = newState
		c.publishPreparedEnvelopeLocked(MsgApplication, wire)
		c.appendOfflineEnvelopeLocked(wire)
		now := c.clock.Now()
		c.mu.Unlock()

		// Mark the original stored message as replayed so the frontend can
		// suppress it once the re-broadcast copy is received and stored.
		if len(msg.EnvelopeHash) > 0 {
			if mErr := c.storage.MarkMessageReplayed(c.groupID, msg.EnvelopeHash, now); mErr != nil {
				slog.Warn("fork_heal/mark_replayed_failed", "group", c.groupID, "err", mErr)
			}
		}
		replayed++

		if throttle > 0 && i < len(window)-1 {
			select {
			case <-ctx.Done():
				return replayed, ctx.Err()
			case <-c.clock.After(throttle):
			}
		}
	}
	c.mu.Lock()
	err := c.saveCurrentGroupStateLocked(c.clock.Now())
	c.mu.Unlock()
	if err != nil {
		return replayed, err
	}
	return replayed, nil
}
