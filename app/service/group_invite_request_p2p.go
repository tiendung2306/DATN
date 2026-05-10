package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	errCreatorUnreachable   = "ERR_CREATOR_UNREACHABLE"
	errGroupCreatorUnknown  = "ERR_GROUP_CREATOR_UNKNOWN"
	groupMemberRoleCreator  = "creator"
)

func (r *Runtime) registerGroupInviteRequestHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(p2p.GroupInviteRequestProtocol, func(s network.Stream) {
		go r.handleGroupInviteRequestStream(s)
	})
	slog.Info("group-invite-request handler registered")
}

func (r *Runtime) removeGroupInviteRequestHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(p2p.GroupInviteRequestProtocol)
}

func (r *Runtime) handleGroupInviteRequestStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	ap := r.node.AuthProtocol
	r.mu.Unlock()
	if ap != nil && !ap.IsVerified(remote) {
		slog.Warn("group-invite-request: unverified peer", "peer", remote)
		return
	}

	raw, err := p2p.ReadGroupInviteWireRaw(s)
	if err != nil {
		slog.Warn("group-invite-request: read frame", "from", remote, "err", err)
		return
	}
	var probe struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		slog.Warn("group-invite-request: probe", "from", remote, "err", err)
		return
	}
	switch strings.ToLower(strings.TrimSpace(probe.Op)) {
	case "push":
		var push p2p.GroupInviteWirePushV1
		if err := json.Unmarshal(raw, &push); err != nil {
			slog.Warn("group-invite-request: push decode", "from", remote, "err", err)
			return
		}
		r.applyInviteRequestPushFromCreator(remote, &push)
	default:
		var req p2p.GroupInviteWireClientReqV1
		if err := json.Unmarshal(raw, &req); err != nil {
			slog.Warn("group-invite-request: req decode", "from", remote, "err", err)
			return
		}
		resp := r.handleGroupInviteWireRPC(remote, &req)
		if err := p2p.WriteGroupInviteWireFrame(s, resp); err != nil {
			slog.Debug("group-invite-request: write resp", "to", remote, "err", err)
		}
	}
}

func (r *Runtime) handleGroupInviteWireRPC(remote peer.ID, req *p2p.GroupInviteWireClientReqV1) *p2p.GroupInviteWireRespV1 {
	out := &p2p.GroupInviteWireRespV1{V: 1}
	if req == nil || req.V != 1 {
		out.Error = "unsupported request version"
		return out
	}
	op := strings.ToLower(strings.TrimSpace(req.Op))
	switch op {
	case "submit":
		rec, err := r.rpcSubmitInviteRequest(remote, req)
		if err != nil {
			out.Error = err.Error()
			return out
		}
		out.OK = true
		out.Record = inviteRecordToWire(rec)
	case "sync":
		rec, err := r.rpcSyncInviteRequest(remote, req)
		if err != nil {
			out.Error = err.Error()
			return out
		}
		out.OK = true
		out.Record = inviteRecordToWire(rec)
	case "cancel":
		rec, err := r.rpcCancelInviteRequest(remote, req)
		if err != nil {
			out.Error = err.Error()
			return out
		}
		out.OK = true
		out.Record = inviteRecordToWire(rec)
	default:
		out.Error = "unknown op"
	}
	return out
}

// rpcSubmitInviteRequest creates an invite row on the creator node (authority).
func (r *Runtime) rpcSubmitInviteRequest(remote peer.ID, req *p2p.GroupInviteWireClientReqV1) (*store.GroupInviteRequestRecord, error) {
	groupID := strings.TrimSpace(req.GroupID)
	targetPeerID := strings.TrimSpace(req.TargetPeerID)
	if groupID == "" || targetPeerID == "" {
		return nil, fmt.Errorf("group_id and target_peer_id are required")
	}
	if _, err := peer.Decode(targetPeerID); err != nil {
		return nil, fmt.Errorf("invalid target_peer_id")
	}
	if remote.String() == targetPeerID {
		return nil, fmt.Errorf("cannot invite yourself")
	}
	isCreator, err := r.isLocalCreator(groupID)
	if err != nil {
		return nil, err
	}
	if !isCreator {
		return nil, fmt.Errorf("%s: only creator accepts invite submissions", errInviteForbidden)
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	policy, err := database.GetGroupInvitePolicy(groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("ERR_GROUP_NOT_FOUND: group not found")
		}
		return nil, err
	}
	if policy != store.GroupInvitePolicyCreatorApproval {
		return nil, fmt.Errorf("%s: group does not use creator approval policy", errInviteForbidden)
	}
	members, err := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
	if err != nil {
		return nil, err
	}
	isMember := false
	for _, m := range members {
		if m.PeerID == remote.String() {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, fmt.Errorf("%s: requester is not an active member", errInviteForbidden)
	}
	already, err := r.isTargetAlreadyMember(groupID, targetPeerID)
	if err != nil {
		return nil, err
	}
	if already {
		return nil, fmt.Errorf("target is already a member")
	}
	id, err := newInviteRequestID()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	rec := store.GroupInviteRequestRecord{
		RequestID:       id,
		GroupID:         groupID,
		RequesterPeerID: remote.String(),
		TargetPeerID:    targetPeerID,
		Status:          store.InviteRequestStatusPending,
		MaxAttempts:     maxInviteRetryAttempts,
		ExpiresAt:       now + int64(inviteRequestTTL.Seconds()),
		CreatedAt:       now,
		UpdatedAt:       now,
		IsMirror:        false,
	}
	if err := database.CreateGroupInviteRequest(rec); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%s: active request already exists for this target", errInviteDuplicateActive)
		}
		return nil, err
	}
	r.emit("group:invite_request_changed", map[string]interface{}{
		"group_id":   groupID,
		"request_id": id,
		"status":     store.InviteRequestStatusPending,
		"reason":     "created_remote",
	})
	out, err := database.GetGroupInviteRequest(id)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Runtime) rpcSyncInviteRequest(remote peer.ID, req *p2p.GroupInviteWireClientReqV1) (*store.GroupInviteRequestRecord, error) {
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rec, err := database.GetGroupInviteRequest(requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s: request not found", errInviteNotFound)
	}
	if err != nil {
		return nil, err
	}
	if rec.IsMirror {
		return nil, fmt.Errorf("%s: cannot sync mirror row", errInviteForbidden)
	}
	isCreator, err := r.isLocalCreator(rec.GroupID)
	if err != nil {
		return nil, err
	}
	if !isCreator {
		return nil, fmt.Errorf("%s: only creator can serve invite sync", errInviteForbidden)
	}
	if remote.String() != rec.RequesterPeerID {
		return nil, fmt.Errorf("%s: only the requester can sync this row", errInviteForbidden)
	}
	return rec, nil
}

func (r *Runtime) rpcCancelInviteRequest(remote peer.ID, req *p2p.GroupInviteWireClientReqV1) (*store.GroupInviteRequestRecord, error) {
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rec, err := database.GetGroupInviteRequest(requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s: request not found", errInviteNotFound)
	}
	if err != nil {
		return nil, err
	}
	if rec.IsMirror {
		return nil, fmt.Errorf("%s: cannot cancel mirror on creator node", errInviteForbidden)
	}
	isCreator, err := r.isLocalCreator(rec.GroupID)
	if err != nil {
		return nil, err
	}
	if !isCreator {
		return nil, fmt.Errorf("%s: only creator can cancel authority rows here", errInviteForbidden)
	}
	if remote.String() != rec.RequesterPeerID {
		return nil, fmt.Errorf("%s: only the requester can cancel this request remotely", errInviteForbidden)
	}
	if rec.Status == store.InviteRequestStatusProcessing {
		return nil, fmt.Errorf("%s: request is processing", errInviteStateConflict)
	}
	ok, err := database.TryTransitionInviteRequest(requestID,
		[]string{store.InviteRequestStatusPending, store.InviteRequestStatusFailed, store.InviteRequestStatusPermanentlyFailed},
		store.InviteRequestStatusCancelled,
		store.InviteRequestTransitionPatch{ClearFailure: true},
	)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s: request cannot be cancelled in current state", errInviteStateConflict)
	}
	out, err := database.GetGroupInviteRequest(requestID)
	if err != nil {
		return nil, err
	}
	r.emit("group:invite_request_decided", map[string]interface{}{
		"group_id":       rec.GroupID,
		"request_id":     requestID,
		"status":         store.InviteRequestStatusCancelled,
		"requester_id":   rec.RequesterPeerID,
		"target_peer_id": rec.TargetPeerID,
	})
	return out, nil
}

func (r *Runtime) applyInviteRequestPushFromCreator(remote peer.ID, push *p2p.GroupInviteWirePushV1) {
	if push == nil || strings.ToLower(strings.TrimSpace(push.Op)) != "push" {
		return
	}
	rec := inviteWireToRecord(&push.Record)
	groupID := strings.TrimSpace(rec.GroupID)
	if groupID == "" {
		return
	}
	creatorPID, err := r.resolveGroupCreatorPeerID(groupID)
	if err != nil {
		slog.Debug("group-invite push: no creator", "group", groupID, "err", err)
		return
	}
	if creatorPID != remote {
		slog.Warn("group-invite push: remote is not creator", "group", groupID, "remote", remote, "want", creatorPID)
		return
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return
	}
	if err := database.UpsertGroupInviteRequestMirror(rec); err != nil {
		slog.Warn("group-invite push: upsert mirror", "err", err)
		return
	}
	r.emit("group:invite_request_changed", map[string]interface{}{
		"group_id":   rec.GroupID,
		"request_id": rec.RequestID,
		"status":     rec.Status,
		"reason":     "push",
	})
}

func (r *Runtime) resolveGroupCreatorPeerID(groupID string) (peer.ID, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return "", fmt.Errorf("group ID is required")
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return "", fmt.Errorf("database not initialized")
	}
	members, err := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
	if err != nil {
		return "", err
	}
	for _, m := range members {
		if strings.EqualFold(strings.TrimSpace(m.Role), groupMemberRoleCreator) {
			pid, err := peer.Decode(strings.TrimSpace(m.PeerID))
			if err != nil {
				continue
			}
			return pid, nil
		}
	}
	if hint, err := database.GetGroupInviteCreatorHint(groupID); err == nil {
		pid, derr := peer.Decode(strings.TrimSpace(hint))
		if derr == nil {
			slog.Debug("invite: resolved creator from welcome metadata", "group_id", groupID, "creator_peer", pid.String())
			return pid, nil
		}
		slog.Warn("invite: creator hint invalid peer id", "group_id", groupID, "hint", hint, "error", derr)
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Debug("invite: creator hint lookup failed", "group_id", groupID, "error", err)
	}
	return "", fmt.Errorf("%s: Chưa đồng bộ đủ thông tin nhóm.", errGroupCreatorUnknown)
}

func (r *Runtime) groupInviteWireCall(ctx context.Context, remote peer.ID, req *p2p.GroupInviteWireClientReqV1) (*p2p.GroupInviteWireRespV1, error) {
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil {
		return nil, fmt.Errorf("p2p node not ready")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(ctx, remote)
	}
	s, err := node.Host.NewStream(ctx, remote, p2p.GroupInviteRequestProtocol)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	if err := p2p.WriteGroupInviteWireFrame(s, req); err != nil {
		return nil, err
	}
	var resp p2p.GroupInviteWireRespV1
	if err := p2p.ReadGroupInviteWireFrame(s, &resp); err != nil {
		return nil, err
	}
	if resp.V != 1 {
		return nil, fmt.Errorf("unsupported response version")
	}
	return &resp, nil
}

func (r *Runtime) maybePushInviteUpdate(ctx context.Context, rec *store.GroupInviteRequestRecord) {
	if rec == nil || rec.IsMirror {
		return
	}
	isCreator, err := r.isLocalCreator(rec.GroupID)
	if err != nil || !isCreator {
		return
	}
	local, err := r.localPeerID()
	if err != nil || local == rec.RequesterPeerID {
		return
	}
	if err := r.pushInviteRequestToRequester(ctx, rec); err != nil {
		slog.Debug("group-invite push failed", "request_id", rec.RequestID, "err", err)
	}
}

func (r *Runtime) pushInviteRequestToRequester(ctx context.Context, rec *store.GroupInviteRequestRecord) error {
	if rec == nil {
		return nil
	}
	target, err := peer.Decode(rec.RequesterPeerID)
	if err != nil {
		return err
	}
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil {
		return fmt.Errorf("p2p node not ready")
	}
	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(ctx, target)
	}
	s, err := node.Host.NewStream(ctx, target, p2p.GroupInviteRequestProtocol)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	wire := inviteRecordToWire(rec)
	push := p2p.GroupInviteWirePushV1{V: 1, Op: "push", Record: *wire}
	return p2p.WriteGroupInviteWireFrame(s, &push)
}

func inviteRecordToWire(rec *store.GroupInviteRequestRecord) *p2p.InviteRequestRecordWire {
	if rec == nil {
		return nil
	}
	w := &p2p.InviteRequestRecordWire{
		RequestID:       rec.RequestID,
		GroupID:         rec.GroupID,
		RequesterPeerID: rec.RequesterPeerID,
		TargetPeerID:    rec.TargetPeerID,
		Status:          rec.Status,
		FailureCode:     rec.FailureCode,
		FailureMessage:  rec.FailureMessage,
		RejectionReason: rec.RejectionReason,
		AttemptCount:    rec.AttemptCount,
		MaxAttempts:     rec.MaxAttempts,
		ExpiresAt:       rec.ExpiresAt,
		CreatedAt:       rec.CreatedAt,
		UpdatedAt:       rec.UpdatedAt,
		IsMirror:        rec.IsMirror,
	}
	if rec.ProcessingStartedAt.Valid {
		ts := rec.ProcessingStartedAt.Int64
		w.ProcessingStartedAt = &ts
	}
	return w
}

func inviteWireToRecord(w *p2p.InviteRequestRecordWire) store.GroupInviteRequestRecord {
	if w == nil {
		return store.GroupInviteRequestRecord{}
	}
	rec := store.GroupInviteRequestRecord{
		RequestID:       strings.TrimSpace(w.RequestID),
		GroupID:         strings.TrimSpace(w.GroupID),
		RequesterPeerID: strings.TrimSpace(w.RequesterPeerID),
		TargetPeerID:    strings.TrimSpace(w.TargetPeerID),
		Status:          strings.TrimSpace(w.Status),
		FailureCode:     strings.TrimSpace(w.FailureCode),
		FailureMessage:  strings.TrimSpace(w.FailureMessage),
		RejectionReason: strings.TrimSpace(w.RejectionReason),
		AttemptCount:    w.AttemptCount,
		MaxAttempts:     w.MaxAttempts,
		ExpiresAt:       w.ExpiresAt,
		CreatedAt:       w.CreatedAt,
		UpdatedAt:       w.UpdatedAt,
		IsMirror:        w.IsMirror,
	}
	if w.ProcessingStartedAt != nil {
		rec.ProcessingStartedAt = sql.NullInt64{Int64: *w.ProcessingStartedAt, Valid: true}
	}
	return rec
}
