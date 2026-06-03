package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/store"
	"app/coordination"
)

const defaultGroupEventLogLimit = 50

const (
	groupEventTypeCreated               = "group_created"
	groupEventTypeMemberJoined          = "member_joined"
	groupEventTypeMemberLeft            = "member_left"
	groupEventTypeProposalReceived      = "proposal_received"
	groupEventTypeCommitIssued          = "commit_issued"
	groupEventTypeAddCommitObserved     = "add_commit_observed"
	groupEventTypeInviteRequestCreated  = "invite_request_created"
	groupEventTypeInviteRequestApproved = "invite_request_approved"
	groupEventTypeInviteRequestRejected = "invite_request_rejected"
	groupEventTypeReplayBlocked         = "replay_blocked"
	groupEventTypeOperationRebased      = "operation_rebased"
	groupEventTypeOperationRebaseFailed = "operation_rebase_failed"
	groupEventTypeOperationRetryMaxed   = "operation_retry_exhausted"
	groupEventTypeForkHealStarted       = "fork_heal_started"
	groupEventTypeForkHealCompleted     = "fork_heal_completed"
	groupEventTypeForkHealFailed        = "fork_heal_failed"
)

type GroupEventLogEntry struct {
	ID           int64  `json:"id"`
	GroupID      string `json:"group_id"`
	EventType    string `json:"event_type"`
	ActorPeerID  string `json:"actor_peer_id,omitempty"`
	TargetPeerID string `json:"target_peer_id,omitempty"`
	Epoch        uint64 `json:"epoch"`
	PayloadJSON  string `json:"payload_json"`
	CreatedAtMs  int64  `json:"created_at_ms"`
}

func (r *Runtime) GetGroupEventLog(groupID string, limit int) ([]GroupEventLogEntry, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, fmt.Errorf("group ID is required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if limit <= 0 {
		limit = defaultGroupEventLogLimit
	}
	rows, err := database.ListGroupEvents(groupID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]GroupEventLogEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, GroupEventLogEntry{
			ID:           row.ID,
			GroupID:      row.GroupID,
			EventType:    row.EventType,
			ActorPeerID:  row.ActorPeerID,
			TargetPeerID: row.TargetPeerID,
			Epoch:        row.Epoch,
			PayloadJSON:  string(row.PayloadJSON),
			CreatedAtMs:  row.CreatedAtMs,
		})
	}
	return out, nil
}

func (r *Runtime) appendGroupEvent(groupID, eventType, actorPeerID, targetPeerID string, epoch uint64, payload any) {
	groupID = strings.TrimSpace(groupID)
	eventType = strings.TrimSpace(eventType)
	if groupID == "" || eventType == "" {
		return
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("group_event_log marshal failed", "group_id", groupID, "event_type", eventType, "err", err)
		return
	}
	rec := store.GroupEventLogRecord{
		GroupID:      groupID,
		EventType:    eventType,
		ActorPeerID:  strings.TrimSpace(actorPeerID),
		TargetPeerID: strings.TrimSpace(targetPeerID),
		Epoch:        epoch,
		PayloadJSON:  payloadJSON,
		CreatedAtMs:  time.Now().UnixMilli(),
	}
	// Audit log writes are best-effort and must never block user-facing flows
	// such as CreateGroupChat or membership mutations.
	go func() {
		r.mu.RLock()
		database := r.db
		r.mu.RUnlock()
		if database == nil {
			return
		}
		if _, err := database.AppendGroupEvent(rec); err != nil {
			slog.Warn("group_event_log append failed", "group_id", groupID, "event_type", eventType, "err", err)
		}
	}()
}

func proposalTypeLabel(kind coordination.ProposalType) string {
	switch kind {
	case coordination.ProposalAdd:
		return "add"
	case coordination.ProposalRemove:
		return "remove"
	case coordination.ProposalUpdate:
		return "update"
	default:
		return fmt.Sprintf("unknown_%d", kind)
	}
}

func (r *Runtime) makeProposalAuditHandler(_ string) func(coordination.ProposalAuditSummary) {
	return func(summary coordination.ProposalAuditSummary) {
		r.appendGroupEvent(summary.GroupID, groupEventTypeProposalReceived, summary.ActorPeerID, summary.TargetPeerID, summary.Epoch, map[string]any{
			"proposal_type":  proposalTypeLabel(summary.ProposalType),
			"operation_id":   summary.OperationID,
			"target_peer_id": summary.TargetPeerID,
			"request_id":     summary.RequestID,
			"group_type":     summary.GroupType,
			"category_id":    summary.CategoryID,
		})
	}
}

func (r *Runtime) makeCommitAuditHandler(_ string) func(coordination.CommitAuditSummary) {
	return func(summary coordination.CommitAuditSummary) {
		proposals := make([]map[string]any, 0, len(summary.Proposals))
		for _, proposal := range summary.Proposals {
			proposals = append(proposals, map[string]any{
				"proposal_type":  proposalTypeLabel(proposal.ProposalType),
				"operation_id":   proposal.OperationID,
				"target_peer_id": proposal.TargetPeerID,
				"request_id":     proposal.RequestID,
				"group_type":     proposal.GroupType,
				"category_id":    proposal.CategoryID,
			})
		}
		r.appendGroupEvent(summary.GroupID, groupEventTypeCommitIssued, summary.TokenHolderPeerID, "", summary.NewEpoch, map[string]any{
			"token_holder_peer_id": summary.TokenHolderPeerID,
			"previous_epoch":       summary.PrevEpoch,
			"new_epoch":            summary.NewEpoch,
			"proposals":            proposals,
		})
	}
}

func (r *Runtime) makePendingOperationAuditHandler(_ string) func(coordination.PendingOperationAuditSummary) {
	return func(summary coordination.PendingOperationAuditSummary) {
		eventType := groupEventTypeOperationRebased
		topic := "group:operation_rebased"
		switch summary.Stage {
		case "retry_exhausted":
			eventType = groupEventTypeOperationRetryMaxed
			topic = "group:operation_retry_exhausted"
		case "rebase_failed":
			eventType = groupEventTypeOperationRebaseFailed
			topic = "group:operation_rebase_failed"
		}

		payload := map[string]any{
			"operation_id":       summary.OperationID,
			"op_type":            summary.OpType,
			"target_peer_id":     summary.TargetPeerID,
			"stage":              summary.Stage,
			"retry_count":        summary.RetryCount,
			"current_epoch":      summary.CurrentEpoch,
			"precondition_epoch": summary.PreconditionEpoch,
			"last_error":         summary.LastError,
		}
		r.appendGroupEvent(summary.GroupID, eventType, "", summary.TargetPeerID, summary.CurrentEpoch, payload)
		r.emit(topic, map[string]interface{}{
			"group_id":           summary.GroupID,
			"operation_id":       summary.OperationID,
			"op_type":            summary.OpType,
			"target_peer_id":     summary.TargetPeerID,
			"stage":              summary.Stage,
			"retry_count":        summary.RetryCount,
			"current_epoch":      summary.CurrentEpoch,
			"precondition_epoch": summary.PreconditionEpoch,
			"last_error":         summary.LastError,
		})
	}
}

func (r *Runtime) makeForkHealAuditHandler(_ string) func(coordination.ForkHealAuditSummary) {
	return func(summary coordination.ForkHealAuditSummary) {
		r.appendGroupEvent(summary.GroupID, summary.Stage, summary.WinnerPeerID, "", summary.NewEpoch, map[string]any{
			"trace_id":               summary.TraceID,
			"winner_peer_id":         summary.WinnerPeerID,
			"winner_epoch":           summary.WinnerEpoch,
			"new_epoch":              summary.NewEpoch,
			"failed_step":            summary.FailedStep,
			"replayed_message_count": summary.ReplayedMessageCount,
		})
	}
}
