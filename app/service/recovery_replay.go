package service

import (
	"encoding/hex"
	"log/slog"
	"sync"

	"app/coordination"
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

	for {
		r.mu.RLock()
		cs := r.coordStorage
		coord := r.coordinators[groupID]
		r.mu.RUnlock()
		if cs == nil || coord == nil {
			return
		}

		records, err := cs.GetPendingEnvelopes(groupID, 100)
		if err != nil {
			slog.Warn("recovery-replay: load pending envelopes failed", "group", groupID, "reason", reason, "err", err)
			return
		}
		if len(records) == 0 {
			return
		}

		progressed := false
		for _, rec := range records {
			results, err := coord.ReplayEnvelopesDetailed([][]byte{rec.Envelope})
			if err != nil {
				slog.Warn("recovery-replay: replay failed", "group", groupID, "seq", rec.Seq, "reason", reason, "err", err)
				return
			}
			if len(results) == 0 {
				return
			}
			result := results[0]
			if result.State == coordination.ReplayStateApplied || result.State == coordination.ReplayStateDuplicateApplied {
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
			return
		}
		if !progressed {
			return
		}
	}
}
