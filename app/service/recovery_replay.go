package service

import (
	"encoding/hex"
	"log/slog"
	"strings"
	"sync"
	"time"

	"app/coordination"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (r *Runtime) emitReplayBlocked(groupID, reason string, rec *coordination.EnvelopeRecord, result coordination.ReplayEnvelopeResult) {
	groupID = strings.TrimSpace(groupID)
	reason = strings.TrimSpace(reason)
	if groupID == "" || reason == "" {
		return
	}
	payload := map[string]interface{}{
		"group_id":      groupID,
		"reason":        reason,
		"state":         string(result.State),
		"seq":           rec.Seq,
		"msg_epoch":     result.MsgEpoch,
		"local_epoch":   result.LocalEpoch,
		"error":         result.Error,
		"envelope_hash": hex.EncodeToString(result.EnvelopeHash),
	}
	r.appendGroupEvent(groupID, groupEventTypeReplayBlocked, "", "", result.LocalEpoch, payload)
	r.emit("group:replay_blocked", payload)
}

func (r *Runtime) replayLockForGroup(groupID string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replayLocks == nil {
		r.replayLocks = make(map[string]*sync.Mutex)
	}
	if r.replayLocks[groupID] == nil {
		r.replayLocks[groupID] = &sync.Mutex{}
	}
	return r.replayLocks[groupID]
}

func (r *Runtime) replayPendingEnvelopesForGroup(groupID, reason string) {
	if groupID == "" {
		return
	}
	lock := r.replayLockForGroup(groupID)
	lock.Lock()
	defer lock.Unlock()

	r.mu.RLock()
	coord := r.coordinators[groupID]
	cs := r.coordStorage
	r.mu.RUnlock()
	if coord == nil || cs == nil {
		return
	}

	startEpoch := coord.CurrentEpoch()
	newEnvelopesApplied := 0
	hasPendingEnvelopes := false

	// 1. Chuyển sang CATCHING_UP mode để cô lập live gossip
	coord.SetOperationalMode(coordination.ModeCatchingUp)
	slog.Info("recovery-replay: group operational mode changed to CATCHING_UP", "group", groupID, "reason", reason)

	defer func() {
		// 4. Chuyển về LIVE khi replay xong hoặc dừng lại
		coord.SetOperationalMode(coordination.ModeLive)
		slog.Info("recovery-replay: group operational mode restored to LIVE", "group", groupID, "reason", reason)
	}()

	for {
		records, err := cs.GetPendingEnvelopes(groupID, 100)
		if err != nil {
			slog.Warn("recovery-replay: load pending envelopes failed", "group", groupID, "reason", reason, "err", err)
			break
		}
		if len(records) == 0 {
			break
		}
		hasPendingEnvelopes = true

		progressed := false
		for _, rec := range records {
			results, err := coord.ReplayEnvelopesDetailed([][]byte{rec.Envelope})
			if err != nil {
				slog.Warn("recovery-replay: replay failed", "group", groupID, "seq", rec.Seq, "reason", reason, "err", err)
				break
			}
			if len(results) == 0 {
				break
			}
			result := results[0]

			switch result.State {
			case coordination.ReplayStateApplied:
				newEnvelopesApplied++
				progressed = true

			case coordination.ReplayStateDuplicateApplied:
				progressed = true

			case coordination.ReplayStateStaleEpoch:
				// Milestone 1.2: Không bao giờ coi stale là success
				err := cs.MarkEnvelopeReplayState(groupID, rec.EnvelopeHash, coordination.ReplayStateBlockedStaleRequiresSnapshot, "stale_epoch_requires_recovery_snapshot", time.Now())
				if err != nil {
					slog.Warn("recovery-replay: failed to mark stale envelope blocked", "group", groupID, "seq", rec.Seq, "err", err)
				}
				r.emitReplayBlocked(groupID, "stale_epoch_requires_recovery_snapshot", rec, result)
				progressed = true

			case coordination.ReplayStateDecryptFailed:
				err := cs.MarkEnvelopeReplayState(groupID, rec.EnvelopeHash, coordination.ReplayStateBlockedDecryptFailed, "decrypt_failed_or_missing_past_key", time.Now())
				if err != nil {
					slog.Warn("recovery-replay: failed to mark decrypt failed envelope blocked", "group", groupID, "seq", rec.Seq, "err", err)
				}
				r.emitReplayBlocked(groupID, "decrypt_failed_or_missing_past_key", rec, result)
				progressed = true

			case coordination.ReplayStateFutureEpoch:
				// Gặp tin nhắn của Epoch tương lai -> bị chặn tạm thời cho đến khi có commit
				err := cs.MarkEnvelopeReplayState(groupID, rec.EnvelopeHash, coordination.ReplayStateBlockedMissingPriorEpoch, "future_epoch_missing_prior_commit", time.Now())
				if err != nil {
					slog.Warn("recovery-replay: failed to mark future envelope blocked", "group", groupID, "seq", rec.Seq, "err", err)
				}
				r.emitReplayBlocked(groupID, "future_epoch_missing_prior_commit", rec, result)
				// Không tiến thêm được nữa vì missing prior commit
				progressed = false

			default:
				err := cs.MarkEnvelopeReplayState(groupID, rec.EnvelopeHash, coordination.ReplayStateBlockedDecryptFailed, string(result.State), time.Now())
				if err != nil {
					slog.Warn("recovery-replay: failed to mark envelope blocked", "group", groupID, "seq", rec.Seq, "err", err)
				}
				r.emitReplayBlocked(groupID, string(result.State), rec, result)
				progressed = false
			}

			if !progressed {
				break
			}
		}
		if !progressed {
			break
		}
	}

	endEpoch := coord.CurrentEpoch()

	if newEnvelopesApplied > 0 || endEpoch > startEpoch {
		// Progress made! Reset the retry attempts
		coord.ResetSyncRetryAttempts()
		slog.Info("recovery-replay: progress made, reset sync retry counter",
			"group", groupID,
			"new_envelopes_applied", newEnvelopesApplied,
			"epoch_advanced", endEpoch > startEpoch,
			"current_epoch", endEpoch,
		)
	} else if hasPendingEnvelopes {
		// No progress made, and we still have pending envelopes in the database (we are stuck).
		attempts := coord.IncrementSyncRetryAttempts()
		slog.Warn("recovery-replay: stuck on pending envelopes, incremented retry counter",
			"group", groupID,
			"attempt", attempts,
			"current_epoch", endEpoch,
		)

		if attempts < 3 {
			// Trigger another sync pull from a connected active member
			activeMembers := coord.ActiveMembers()
			r.mu.RLock()
			node := r.node
			r.mu.RUnlock()

			if node != nil {
				var targetPeer peer.ID
				for _, m := range activeMembers {
					if m != node.Host.ID() && node.Host.Network().Connectedness(m) == network.Connected {
						targetPeer = m
						break
					}
				}
				if targetPeer != "" {
					slog.Info("recovery-replay: triggering sync retry pull",
						"group", groupID,
						"from_peer", targetPeer.String(),
						"attempt", attempts,
					)
					go r.scheduleOfflineSyncPull(targetPeer)
				} else {
					slog.Warn("recovery-replay: no connected active members found to retry sync", "group", groupID)
				}
			}
		} else {
			// Stuck after 3 attempts. Fall back to destructive Fork Healing (External Join)
			slog.Warn("recovery-replay: retry exhausted after 3 attempts, triggering fork heal fallback",
				"group", groupID,
				"current_epoch", endEpoch,
			)
			go coord.TriggerDeferredHeal()
		}
	}
}
