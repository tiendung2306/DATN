package coordination

import (
	"crypto/sha256"
	"fmt"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/peer"
)

// AddMemberRequest carries everything the runtime needs to (a) build a
// ProposalAdd envelope on a non-holder node, or (b) emit an AddCommitDelivery
// on the Token Holder node. The runtime is the source of truth for the
// operation_id, group_type and category_id; the coordinator only forwards
// these fields verbatim.
type AddMemberRequest struct {
	TargetPeerID    peer.ID
	KeyPackageBytes []byte
	OperationID     string
	RequestID       string
	GroupType       string
	CategoryID      string
	KeyPackageHash  []byte
}

// AddMember performs an MLS Add for a new member following the Single-Writer
// Protocol:
//
//   - If the local node is the current Token Holder and the startup ActiveView
//     is sufficiently initialized, the MLS commit is created synchronously and
//     the result includes the Welcome bytes for out-of-band delivery to the
//     invitee.
//   - Otherwise the local node creates a ProposalAdd carrying req's routing
//     metadata, broadcasts it to the group topic, buffers it locally (in
//     case the local node becomes the holder for the next epoch), and
//     returns Deferred=true. The returned Welcome is empty in the deferred
//     case — only the node that ultimately runs CreateCommit may author the
//     Welcome.
//
// AddMember NEVER returns ErrNotTokenHolder: failure of the local node to
// hold the token is part of the protocol, not an error condition.
func (c *Coordinator) AddMember(req AddMemberRequest) (AddMemberResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return AddMemberResult{}, fmt.Errorf("coordinator not started")
	}
	if c.accessRevoked {
		slog.Warn("Mutation rejected: access revoked",
			"group", c.groupID,
			"epoch", c.epoch,
			"op", "AddMember",
			"reason", "access_revoked",
			"violation_source", "local_membership_guard")
		return AddMemberResult{}, ErrAccessRevoked
	}
	if len(req.KeyPackageBytes) == 0 {
		return AddMemberResult{}, fmt.Errorf("AddMember: key package is required")
	}

	// Idempotency check
	idKey := "add_member_" + req.TargetPeerID.String()
	if req.OperationID != "" {
		if existing, err := c.storage.GetPendingOperation(req.OperationID); err == nil && existing != nil {
			if existing.Status == "COMMITTED" || existing.Status == "SATISFIED_BY_OTHER" {
				var commitEpoch uint64
				if existing.PreconditionEpoch != nil {
					commitEpoch = *existing.PreconditionEpoch
				}
				return AddMemberResult{
					OperationID: existing.OperationID,
					Deferred:    false,
					CommitEpoch: commitEpoch,
				}, nil
			}
			if existing.Status == "PROPOSED" {
				return AddMemberResult{
					OperationID: existing.OperationID,
					Deferred:    true,
				}, nil
			}
		}
	}
	if existing, err := c.storage.GetPendingOperationByIdempotencyKey(c.groupID, idKey); err == nil && existing != nil {
		if existing.Status == "COMMITTED" || existing.Status == "SATISFIED_BY_OTHER" {
			var commitEpoch uint64
			if existing.PreconditionEpoch != nil {
				commitEpoch = *existing.PreconditionEpoch
			}
			return AddMemberResult{
				OperationID: existing.OperationID,
				Deferred:    false,
				CommitEpoch: commitEpoch,
			}, nil
		}
		if existing.Status == "PROPOSED" {
			return AddMemberResult{
				OperationID: existing.OperationID,
				Deferred:    true,
			}, nil
		}
	}

	opID := req.OperationID
	if opID == "" {
		opID = "op_" + newTraceID()
	}

	epochCopy := c.epoch
	targetMemberCopy := req.TargetPeerID.String()
	idKeyCopy := idKey

	op := &PendingOperation{
		OperationID:       opID,
		GroupID:           c.groupID,
		OpType:            "ADD_MEMBER",
		IdempotencyKey:    &idKeyCopy,
		OperationHash:     append([]byte(nil), req.KeyPackageHash...),
		PreconditionEpoch: &epochCopy,
		TargetMemberID:    &targetMemberCopy,
		SemanticPayload:   req.KeyPackageBytes,
		Status:            "PENDING",
		CreatedAt:         c.clock.Now(),
		UpdatedAt:         c.clock.Now(),
	}
	if err := c.storage.SavePendingOperation(op); err != nil {
		slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
	}

	delivery := AddCommitDelivery{
		OperationID:    opID,
		TargetPeerID:   req.TargetPeerID.String(),
		RequestID:      req.RequestID,
		GroupType:      req.GroupType,
		CategoryID:     req.CategoryID,
		KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
	}

	msg, buffered, err := c.createAndStoreLocalProposalLocked(ProposalAdd, req.KeyPackageBytes, BufferedProposal{
		OperationID:    opID,
		TargetPeerID:   req.TargetPeerID.String(),
		RequestID:      req.RequestID,
		GroupType:      req.GroupType,
		CategoryID:     req.CategoryID,
		KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
	})
	if err != nil {
		errStr := err.Error()
		op.Status = "FAILED_PRECONDITION"
		op.LastError = &errStr
		op.UpdatedAt = c.clock.Now()
		if err := c.storage.SavePendingOperation(op); err != nil {
			slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
		}
		return AddMemberResult{}, fmt.Errorf("CreateProposal: %w", err)
	}

	c.singleWriter.BufferProposal(buffered)
	c.scheduleFailoverTimerLocked()
	c.metrics.IncrProposalsReceived()

	// Update op status to proposed
	h := sha256.Sum256(msg.Data)
	op.LatestProposalHash = h[:]
	op.Status = "PROPOSED"
	op.UpdatedAt = c.clock.Now()
	if err := c.storage.SavePendingOperation(op); err != nil {
		slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
	}

	if c.onProposalObserved != nil {
		c.onProposalObserved(ProposalAuditSummary{
			GroupID:        c.groupID,
			Epoch:          c.epoch,
			ActorPeerID:    c.localID.String(),
			ProposalType:   ProposalAdd,
			OperationID:    opID,
			TargetPeerID:   req.TargetPeerID.String(),
			RequestID:      req.RequestID,
			GroupType:      req.GroupType,
			CategoryID:     req.CategoryID,
			KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
		})
	}

	// Non-holder path: broadcast ProposalAdd and let whichever node holds
	// the token at the committing epoch author the Welcome.
	if c.singleWriter == nil || !c.singleWriter.IsTokenHolder() {
		c.broadcastLocked(MsgProposal, msg)
		return AddMemberResult{
			OperationID: opID,
			Deferred:    true,
			Delivery:    delivery,
		}, nil
	}
	if !c.commitViewReadyLocked([]BufferedProposal{buffered}) {
		c.broadcastLocked(MsgProposal, msg)
		c.deferCommitUntilViewReadyLocked()
		return AddMemberResult{
			OperationID: opID,
			Deferred:    true,
			Delivery:    delivery,
		}, nil
	}

	// Token Holder path: commit synchronously.
	slog.Info("Coordinator AddMember: token-holder path started",
		"group", c.groupID,
		"epoch", c.epoch,
		"target", req.TargetPeerID.String(),
		"operation_id", opID,
		"timeout", c.cfg.MLSOperationTimeout,
	)
	expectedRefs := [][]byte{buffered.ProposalRef}
	opCtx, cancel := c.mlsOperationContext()
	commitResult, err := c.mls.CreateCommit(opCtx, c.groupState, expectedRefs)
	cancel()
	if err != nil {
		return AddMemberResult{}, fmt.Errorf("CreateCommit: %w", err)
	}
	slog.Info("Coordinator AddMember: sidecar CreateCommit completed",
		"group", c.groupID,
		"next_epoch", c.epoch+1,
		"target", req.TargetPeerID.String(),
		"operation_id", opID,
		"commit_bytes", len(commitResult.CommitBytes),
		"welcome_bytes", len(commitResult.WelcomeBytes),
	)

	if len(commitResult.WelcomeBytes) > 0 {
		sum := sha256.Sum256(commitResult.WelcomeBytes)
		delivery.WelcomeHash = sum[:]
	}

	commitMsg := CommitMsg{
		CommitData:            commitResult.CommitBytes,
		NewTreeHash:           commitResult.NewTreeHash,
		AddDeliveries:         []AddCommitDelivery{delivery},
		IncludedProposals:     []ProposalMsg{msg},
		CommittedProposalRefs: cloneBytesList(commitResult.CommittedProposalRefs),
	}
	if len(commitMsg.CommittedProposalRefs) == 0 {
		commitMsg.CommittedProposalRefs = cloneBytesList(expectedRefs)
	}
	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return AddMemberResult{}, fmt.Errorf("failed to encode commit envelope")
	}
	prevEpoch := c.epoch
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: commitResult.NewGroupState,
		Epoch:      nextEpoch,
		TreeHash:   commitResult.NewTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts, c.epoch)
	if err != nil {
		return AddMemberResult{}, fmt.Errorf("persist commit: %w", err)
	}
	if !applied {
		return AddMemberResult{}, fmt.Errorf("commit envelope already applied")
	}
	c.singleWriter.DrainBatchByRefs(commitMsg.CommittedProposalRefs)
	slog.Info("Coordinator AddMember: commit persisted",
		"group", c.groupID,
		"next_epoch", nextEpoch,
		"target", req.TargetPeerID.String(),
		"operation_id", opID,
	)
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(commitResult.NewGroupState, nextEpoch, commitResult.NewTreeHash, commitResult.CommitBytes)
	c.reconcileOperationsAfterCommitLocked(commitMsg)
	c.reconcileAndRebaseOperationsLocked()

	c.updateLocalAccessRevocationLocked(commitResult.NewGroupState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitResult.CommitBytes)))
	c.emitCommitIssuedLocked(prevEpoch, nextEpoch, []BufferedProposal{{
		Type:           ProposalAdd,
		OperationID:    opID,
		TargetPeerID:   req.TargetPeerID.String(),
		RequestID:      req.RequestID,
		GroupType:      req.GroupType,
		CategoryID:     req.CategoryID,
		KeyPackageHash: append([]byte(nil), req.KeyPackageHash...),
	}})

	// Hand off Welcome delivery on the holder side. We dispatch via the
	// callback so the same outbox-style code path (pending_welcomes_out +
	// store replication + direct delivery) handles both this synchronous
	// commit and the asynchronous proposal-commit case in tryCommitLocked.
	if c.onAddCommitted != nil && len(commitResult.WelcomeBytes) > 0 {
		welcome := append([]byte(nil), commitResult.WelcomeBytes...)
		cb := c.onAddCommitted
		epoch := nextEpoch
		go cb(delivery, epoch, welcome)
	}

	return AddMemberResult{
		OperationID: opID,
		Welcome:     commitResult.WelcomeBytes,
		CommitEpoch: nextEpoch,
		Delivery:    delivery,
	}, nil
}

// RemoveMember removes an existing member from the group.
//
// Behavior follows Single-Writer:
//   - If local node is Token Holder, it commits removal immediately.
//   - Otherwise it broadcasts a ProposalRemove and waits for the holder commit.
//
// targetIdentity must be the MLS BasicCredential identity bytes (signing public
// key bytes) for the member to remove.
func (c *Coordinator) RemoveMember(targetIdentity []byte) error {
	return c.RemoveMemberWithPeer(RemoveMemberRequest{TargetIdentity: targetIdentity})
}

func (c *Coordinator) RemoveMemberWithPeer(req RemoveMemberRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return fmt.Errorf("coordinator not started")
	}
	if c.accessRevoked {
		slog.Warn("Mutation rejected: access revoked",
			"group", c.groupID,
			"epoch", c.epoch,
			"op", "RemoveMember",
			"reason", "access_revoked",
			"violation_source", "local_membership_guard")
		return ErrAccessRevoked
	}
	if len(req.TargetIdentity) == 0 {
		return fmt.Errorf("target identity is required")
	}

	// Idempotency check
	idKey := "remove_member_" + req.TargetPeerID.String()
	if req.OperationID != "" {
		if existing, err := c.storage.GetPendingOperation(req.OperationID); err == nil && existing != nil {
			if existing.Status == "COMMITTED" || existing.Status == "SATISFIED_BY_OTHER" {
				return nil
			}
			if existing.Status == "PROPOSED" {
				return nil
			}
		}
	}
	if existing, err := c.storage.GetPendingOperationByIdempotencyKey(c.groupID, idKey); err == nil && existing != nil {
		if existing.Status == "COMMITTED" || existing.Status == "SATISFIED_BY_OTHER" {
			return nil
		}
		if existing.Status == "PROPOSED" {
			return nil
		}
	}

	opID := req.OperationID
	if opID == "" {
		opID = "op_" + newTraceID()
	}

	epochCopy := c.epoch
	targetMemberCopy := req.TargetPeerID.String()
	idKeyCopy := idKey

	op := &PendingOperation{
		OperationID:       opID,
		GroupID:           c.groupID,
		OpType:            "REMOVE_MEMBER",
		IdempotencyKey:    &idKeyCopy,
		OperationHash:     nil,
		PreconditionEpoch: &epochCopy,
		TargetMemberID:    &targetMemberCopy,
		SemanticPayload:   req.TargetIdentity,
		Status:            "PENDING",
		CreatedAt:         c.clock.Now(),
		UpdatedAt:         c.clock.Now(),
	}
	if err := c.storage.SavePendingOperation(op); err != nil {
		slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
	}

	if c.singleWriter == nil || !c.singleWriter.IsTokenHolder() {
		// Update op status to proposed before we exit
		op.Status = "PROPOSED"
		op.UpdatedAt = c.clock.Now()
		if err := c.storage.SavePendingOperation(op); err != nil {
			slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
		}

		return c.proposeLockedWithMetadata(ProposalRemove, req.TargetIdentity, BufferedProposal{TargetPeerID: req.TargetPeerID.String(), OperationID: opID})
	}

	msg, buffered, err := c.createAndStoreLocalProposalLocked(ProposalRemove, req.TargetIdentity, BufferedProposal{TargetPeerID: req.TargetPeerID.String(), OperationID: opID})
	if err != nil {
		errStr := err.Error()
		op.Status = "FAILED_PRECONDITION"
		op.LastError = &errStr
		op.UpdatedAt = c.clock.Now()
		if err := c.storage.SavePendingOperation(op); err != nil {
			slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
		}
		return fmt.Errorf("CreateProposal: %w", err)
	}

	c.singleWriter.BufferProposal(buffered)
	c.scheduleFailoverTimerLocked()

	// Update op status to proposed
	h := sha256.Sum256(msg.Data)
	op.LatestProposalHash = h[:]
	op.Status = "PROPOSED"
	op.UpdatedAt = c.clock.Now()
	if err := c.storage.SavePendingOperation(op); err != nil {
		slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
	}

	if !c.commitViewReadyLocked([]BufferedProposal{buffered}) {
		c.broadcastLocked(MsgProposal, msg)
		c.deferCommitUntilViewReadyLocked()
		return nil
	}
	expectedRefs := [][]byte{buffered.ProposalRef}
	opCtx, cancel := c.mlsOperationContext()
	commitResult, err := c.mls.CreateCommit(opCtx, c.groupState, expectedRefs)
	cancel()
	if err != nil {
		return fmt.Errorf("CreateCommit: %w", err)
	}

	commitMsg := CommitMsg{
		CommitData:            commitResult.CommitBytes,
		NewTreeHash:           commitResult.NewTreeHash,
		IncludedProposals:     []ProposalMsg{msg},
		CommittedProposalRefs: cloneBytesList(commitResult.CommittedProposalRefs),
	}
	if len(commitMsg.CommittedProposalRefs) == 0 {
		commitMsg.CommittedProposalRefs = cloneBytesList(expectedRefs)
	}
	ts := c.hlc.Now()
	envBytes := c.buildEnvelopeWithTimestampLocked(MsgCommit, commitMsg, ts)
	if len(envBytes) == 0 {
		return fmt.Errorf("failed to encode commit envelope")
	}
	prevEpoch := c.epoch
	nextEpoch := c.epoch + 1
	now := c.clock.Now()
	applied, _, err := c.storage.ApplyCommit(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: commitResult.NewGroupState,
		Epoch:      nextEpoch,
		TreeHash:   commitResult.NewTreeHash,
		UpdatedAt:  now,
	}, MsgCommit, envBytes, ts, c.epoch)
	if err != nil {
		return fmt.Errorf("persist commit: %w", err)
	}
	if !applied {
		return fmt.Errorf("commit envelope already applied")
	}
	c.singleWriter.DrainBatchByRefs(commitMsg.CommittedProposalRefs)
	c.publishPreparedEnvelopeLocked(MsgCommit, envBytes)

	c.advanceEpochLocked(commitResult.NewGroupState, nextEpoch, commitResult.NewTreeHash, commitResult.CommitBytes)
	c.reconcileOperationsAfterCommitLocked(commitMsg)
	c.reconcileAndRebaseOperationsLocked()

	c.updateLocalAccessRevocationLocked(commitResult.NewGroupState, nextEpoch)
	c.metrics.IncrCommitsIssued()
	c.metrics.AddCommitBytes(int64(len(commitResult.CommitBytes)))
	c.emitCommitIssuedLocked(prevEpoch, nextEpoch, []BufferedProposal{{
		Type:         ProposalRemove,
		TargetPeerID: req.TargetPeerID.String(),
		OperationID:  opID,
	}})

	return nil
}
