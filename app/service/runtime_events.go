package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"app/adapter/store"
)

type RuntimeEvent struct {
	Seq         int64  `json:"seq"`
	Topic       string `json:"topic"`
	Aggregate   string `json:"aggregate"`
	AggregateID string `json:"aggregate_id"`
	Revision    int64  `json:"revision"`
	PayloadJSON string `json:"payload_json"`
	CreatedAt   int64  `json:"created_at"`
}

func (r *Runtime) emitRuntimeEvent(event string, data map[string]interface{}) {
	payload := clonePayload(data)
	agg, aggID := classifyRuntimeEvent(event, payload)

	r.mu.Lock()
	if r.eventRevisions == nil {
		r.eventRevisions = make(map[string]int64)
	}
	rev := r.eventRevisions[agg] + 1
	r.eventRevisions[agg] = rev
	sink := r.uiEvents
	ctx := r.ctx
	db := r.db
	replayEnabled := r.cfg != nil && r.cfg.RuntimeEventReplay
	r.mu.Unlock()

	payload["aggregate"] = agg
	payload["aggregate_id"] = aggID
	payload["revision"] = rev

	var seq int64
	if replayEnabled && db != nil && event != "runtime:event_available" {
		raw, _ := json.Marshal(payload)
		if s, err := db.AppendRuntimeEvent(store.RuntimeEventRecord{
			Topic:       event,
			Aggregate:   agg,
			AggregateID: aggID,
			Revision:    rev,
			PayloadJSON: raw,
			CreatedAt:   time.Now().Unix(),
		}); err == nil {
			seq = s
			if seq%500 == 0 {
				_ = db.PruneRuntimeEvents(0, 10000)
			}
		}
	}

	if sink != nil && ctx != nil {
		sink.Emit(ctx, event, payload)
		if replayEnabled && seq > 0 && event != "runtime:event_available" {
			sink.Emit(ctx, "runtime:event_available", map[string]interface{}{
				"seq":   seq,
				"topic": event,
			})
		}
	}
}

func clonePayload(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in)+3)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func classifyRuntimeEvent(topic string, payload map[string]interface{}) (string, string) {
	switch topic {
	case "group:message", "group:epoch", "group:joined", "group:left", "group:members_changed":
		if groupID, _ := payload["group_id"].(string); strings.TrimSpace(groupID) != "" {
			return "group:" + groupID, groupID
		}
		return "group", ""
	case "node:status", "p2p:status":
		return "node_status", ""
	case "admin:status":
		return "admin_status", ""
	case "runtime:health", "startup:progress", "startup:error", "app:state_changed":
		return "runtime_health", ""
	case "invite:received", "invite:accepted", "invite:rejected":
		if groupID, _ := payload["group_id"].(string); strings.TrimSpace(groupID) != "" {
			return "invite:" + groupID, groupID
		}
		return "invite", ""
	case "channel_categories:changed":
		return "workspace_categories", ""
	default:
		return topic, ""
	}
}

func (r *Runtime) GetAggregateRevisions() (map[string]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]int64, len(r.eventRevisions))
	for k, v := range r.eventRevisions {
		out[k] = v
	}
	return out, nil
}

func (r *Runtime) GetRuntimeEventCursor() (int64, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	return db.GetLatestRuntimeSeq()
}

func (r *Runtime) GetRuntimeEventsSince(lastSeq int64, limit int) ([]RuntimeEvent, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := db.ListRuntimeEventsSince(lastSeq, limit)
	if err != nil {
		return nil, err
	}
	out := make([]RuntimeEvent, 0, len(rows))
	for _, rec := range rows {
		out = append(out, RuntimeEvent{
			Seq:         rec.Seq,
			Topic:       rec.Topic,
			Aggregate:   rec.Aggregate,
			AggregateID: rec.AggregateID,
			Revision:    rec.Revision,
			PayloadJSON: string(rec.PayloadJSON),
			CreatedAt:   rec.CreatedAt,
		})
	}
	return out, nil
}
