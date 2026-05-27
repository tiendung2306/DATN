package service

import (
	"encoding/hex"
	"log/slog"
	"sync"

	"app/coordination"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

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
			if result.State == coordination.ReplayStateApplied {
				newEnvelopesApplied++
				progressed = true
				continue
			}
			if result.State == coordination.ReplayStateDuplicateApplied {
				progressed = true
				continue
			}
			slog.Warn("recovery-replay: stopped on unapplied envelope",
				"group", groupID,
				"seq", rec.Seq,
				"source_path", rec.SourcePath,
				"reason", reason,
				"state", result.State,
				"envelope_hash", hex.EncodeToString(result.EnvelopeHash),
				"msg_epoch", result.MsgEpoch,
				"local_epoch", result.LocalEpoch,
				"err", result.Error,
			)
			break
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
