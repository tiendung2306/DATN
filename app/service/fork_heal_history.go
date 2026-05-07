package service

import (
	"encoding/hex"
	"errors"
	"strings"
)

type ForkHealAuditEntry struct {
	Step        string `json:"step"`
	Status      string `json:"status"`
	TimestampMs int64  `json:"timestamp_ms"`
	DurationMs  int64  `json:"duration_ms"`
	Error       string `json:"error,omitempty"`
}

type ForkHealHistoryEntry struct {
	TraceID              string              `json:"trace_id"`
	GroupID              string              `json:"group_id"`
	WinnerPeerID         string              `json:"winner_peer_id,omitempty"`
	WinnerEpoch          uint64              `json:"winner_epoch"`
	NewEpoch             uint64              `json:"new_epoch"`
	Outcome              string              `json:"outcome"`
	FailedStep           string              `json:"failed_step,omitempty"`
	WinnerTreeHashHex    string              `json:"winner_tree_hash_hex,omitempty"`
	NewTreeHashHex       string              `json:"new_tree_hash_hex,omitempty"`
	PartitionStartedAtMs int64               `json:"partition_started_at_ms"`
	ScheduledAtMs        int64               `json:"scheduled_at_ms"`
	StartedAtMs          int64               `json:"started_at_ms"`
	CompletedAtMs        int64               `json:"completed_at_ms"`
	DurationMs           int64               `json:"duration_ms"`
	TotalMs              int64               `json:"total_ms"`
	ReplayedMessageCount int                 `json:"replayed_message_count"`
	Audit                []ForkHealAuditEntry `json:"audit,omitempty"`
}

// GetForkHealHistory returns persisted fork-heal summaries and step-level audit
// rows for diagnostics/developer mode.
func (r *Runtime) GetForkHealHistory(groupID string, limit int) ([]ForkHealHistoryEntry, error) {
	r.mu.RLock()
	cs := r.coordStorage
	r.mu.RUnlock()
	if cs == nil {
		return nil, errors.New("storage not ready")
	}
	groupID = strings.TrimSpace(groupID)
	if limit <= 0 {
		limit = 20
	}
	events, err := cs.ListForkHealEvents(groupID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ForkHealHistoryEntry, 0, len(events))
	for _, ev := range events {
		if ev == nil {
			continue
		}
		auditRows, err := cs.ListForkHealAudit(ev.TraceID)
		if err != nil {
			return nil, err
		}
		audit := make([]ForkHealAuditEntry, 0, len(auditRows))
		for _, row := range auditRows {
			if row == nil {
				continue
			}
			audit = append(audit, ForkHealAuditEntry{
				Step:        row.Step,
				Status:      row.Status,
				TimestampMs: row.TimestampMs,
				DurationMs:  row.DurationMs,
				Error:       row.Error,
			})
		}
		out = append(out, ForkHealHistoryEntry{
			TraceID:              ev.TraceID,
			GroupID:              ev.GroupID,
			WinnerPeerID:         ev.WinnerPeerID,
			WinnerEpoch:          ev.WinnerEpoch,
			NewEpoch:             ev.NewEpoch,
			Outcome:              ev.Outcome,
			FailedStep:           ev.FailedStep,
			WinnerTreeHashHex:    hex.EncodeToString(ev.WinnerTreeHash),
			NewTreeHashHex:       hex.EncodeToString(ev.NewTreeHash),
			PartitionStartedAtMs: ev.PartitionStartedAtMs,
			ScheduledAtMs:        ev.ScheduledAtMs,
			StartedAtMs:          ev.StartedAtMs,
			CompletedAtMs:        ev.CompletedAtMs,
			DurationMs:           ev.DurationMs,
			TotalMs:              ev.TotalMs,
			ReplayedMessageCount: ev.ReplayedMessageCount,
			Audit:                audit,
		})
	}
	return out, nil
}
