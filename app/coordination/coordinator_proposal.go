package coordination

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (c *Coordinator) handleProposalLocked(from peer.ID, env *Envelope) {
	var proposal ProposalMsg
	if err := json.Unmarshal(env.Payload, &proposal); err != nil {
		return
	}

	// Defensive Guard: If groupState is nil, we cannot process standard proposals.
	// We only allow ProposalJoin to proceed because it bypasses MLS validation.
	if c.groupState == nil && proposal.ProposalType != ProposalJoin {
		slog.Warn("Ignored proposal because local groupState is nil", "type", proposal.ProposalType, "group", c.groupID)
		return
	}

	// PHOENIX PROTOCOL INTERCEPTION:
	// A ProposalJoin requires an atomic Remove(zombie) + Add(fresh) operation to bypass OpenMLS duplicate identity checks.
	// We check this BEFORE epoch validation, but enforce a security epoch guard to prevent ancient replay attacks.
	if proposal.ProposalType == ProposalJoin {
		maxPast := uint64(c.cfg.GetMaxPastEpochs())
		if env.Epoch+maxPast < c.epoch {
			slog.Warn("Rejected ProposalJoin from ancient epoch", "group", c.groupID, "sender", from, "env_epoch", env.Epoch, "current_epoch", c.epoch)
			return
		}

		// If this node is also healing (groupState nil), it cannot transmute ProposalJoin.
		if len(c.groupState) == 0 {
			slog.Info("ProposalJoin: local node also healing, cannot transmute", "node", c.localID, "group", c.groupID)
			return
		}

		// Use the explicit TargetIdentity if provided by the joiner, falling back to TargetPeerID bytes
		targetIdentity := proposal.TargetIdentity
		if len(targetIdentity) == 0 {
			targetIdentity = []byte(proposal.TargetPeerID)
		}

		// 1. Check if member exists in tree before attempting Remove
		//    If the credential was already removed (e.g. by advanceEpochOnWinningBranch),
		//    skip Remove and proceed directly with Add.
		opCtxMember, cancelMember := c.mlsOperationContext()
		hasMember, memberErr := c.mls.HasMember(opCtxMember, c.groupState, targetIdentity)
		cancelMember()
		if memberErr != nil {
			slog.Warn("HasMember check failed during JoinProposal", "err", memberErr, "target", proposal.TargetPeerID)
		}

		if memberErr == nil && hasMember {
			_, bufferedRemove, err := c.createAndStoreLocalProposalLocked(ProposalRemove, targetIdentity, BufferedProposal{TargetPeerID: proposal.TargetPeerID})
			if err == nil {
				c.singleWriter.BufferProposal(bufferedRemove)
			} else {
				slog.Warn("Failed to create RemoveProposal for zombie leaf during JoinProposal", "err", err, "target", proposal.TargetPeerID)
			}
		} else if memberErr == nil {
			slog.Info("Skipped RemoveProposal during JoinProposal — member not found in tree (already removed)", "target", proposal.TargetPeerID)
		}

		// 2. Transmute the JoinProposal into a standard AddProposal and buffer it
		_, bufferedAdd, err := c.createAndStoreLocalProposalLocked(ProposalAdd, proposal.Data, BufferedProposal{
			OperationID:    proposal.OperationID,
			TargetPeerID:   proposal.TargetPeerID,
			RequestID:      proposal.RequestID,
			GroupType:      proposal.GroupType,
			CategoryID:     proposal.CategoryID,
			KeyPackageHash: proposal.KeyPackageHash,
		})
		if err == nil {
			c.singleWriter.BufferProposal(bufferedAdd)
		} else {
			slog.Warn("Failed to create AddProposal for fresh key package during JoinProposal", "err", err, "target", proposal.TargetPeerID)
		}

		// ProposalJoin đi qua cùng Token Holder election path như proposal bình thường.
		// Cơ chế failover (Suspend + re-elect) đảm bảo Token Holder không commit
		// thì node khác takeover, đảm bảo Single-Writer Invariant.
		if len(c.groupState) > 0 {
			if c.singleWriter.IsTokenHolder() {
				c.scheduleBatchCommitLocked()
			} else {
				c.scheduleFailoverTimerLocked()
			}
		} else {
			slog.Info("ProposalJoin: local node also healing, cannot commit", "node", c.localID, "group", c.groupID)
		}
		return
	}

	action := c.epochTracker.Validate(env.Epoch)
	switch action {
	case ActionRejectStale:
		return
	case ActionBufferFuture:
		raw, _ := json.Marshal(env)
		c.epochTracker.BufferFuture(env.Epoch, raw)
		return
	}

	opCtx, cancel := c.mlsOperationContext()
	processed, err := c.mls.ProcessProposal(opCtx, c.groupState, proposal.Data)
	cancel()
	if err != nil {
		slog.Warn("Rejected proposal that failed MLS processing", "group", c.groupID, "epoch", env.Epoch, "from", from, "error", err)
		return
	}
	if len(proposal.ProposalRef) == 0 {
		proposal.ProposalRef = processed.ProposalRef
	}
	if !bytes.Equal(proposal.ProposalRef, processed.ProposalRef) {
		slog.Warn("Rejected proposal with mismatched ProposalRef", "group", c.groupID, "epoch", env.Epoch, "from", from)
		return
	}
	if err := c.persistCurrentEpochStateLocked(processed.NewGroupState); err != nil {
		slog.Error("Failed to persist pending proposal state", "group", c.groupID, "error", err)
		return
	}
	c.groupState = processed.NewGroupState

	c.singleWriter.BufferProposal(BufferedProposal{
		Type:           proposal.ProposalType,
		Data:           proposal.Data,
		ProposalRef:    proposal.ProposalRef,
		OperationID:    proposal.OperationID,
		TargetPeerID:   proposal.TargetPeerID,
		RequestID:      proposal.RequestID,
		GroupType:      proposal.GroupType,
		CategoryID:     proposal.CategoryID,
		KeyPackageHash: proposal.KeyPackageHash,
	})
	c.scheduleFailoverTimerLocked()
	c.metrics.IncrProposalsReceived()
	if c.onProposalObserved != nil {
		c.onProposalObserved(ProposalAuditSummary{
			GroupID:        c.groupID,
			Epoch:          env.Epoch,
			ActorPeerID:    from.String(),
			ProposalType:   proposal.ProposalType,
			OperationID:    proposal.OperationID,
			TargetPeerID:   proposal.TargetPeerID,
			RequestID:      proposal.RequestID,
			GroupType:      proposal.GroupType,
			CategoryID:     proposal.CategoryID,
			KeyPackageHash: append([]byte(nil), proposal.KeyPackageHash...),
		})
	}

	if c.singleWriter.IsTokenHolder() {
		c.scheduleBatchCommitLocked()
	}
}

// ProposeAdd broadcasts an Add proposal.
func (c *Coordinator) ProposeAdd(memberData []byte) error {
	return c.propose(ProposalAdd, memberData)
}

// ProposeRemove broadcasts a Remove proposal.
func (c *Coordinator) ProposeRemove(memberData []byte) error {
	return c.propose(ProposalRemove, memberData)
}

// ProposeUpdate broadcasts an Update proposal (key rotation).
func (c *Coordinator) ProposeUpdate(data []byte) error {
	return c.propose(ProposalUpdate, data)
}

func (c *Coordinator) propose(pType ProposalType, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.proposeLocked(pType, data)
}

func (c *Coordinator) proposeLocked(pType ProposalType, data []byte) error {
	return c.proposeLockedWithMetadata(pType, data, BufferedProposal{})
}

func (c *Coordinator) proposeLockedWithMetadata(pType ProposalType, data []byte, meta BufferedProposal) error {
	if !c.started {
		return fmt.Errorf("coordinator not started")
	}
	if c.accessRevoked {
		slog.Warn("Mutation rejected: access revoked",
			"group", c.groupID,
			"epoch", c.epoch,
			"op", "Propose",
			"reason", "access_revoked",
			"violation_source", "local_membership_guard")
		return ErrAccessRevoked
	}

	msg, buffered, err := c.createAndStoreLocalProposalLocked(pType, data, meta)
	if err != nil {
		return fmt.Errorf("CreateProposal: %w", err)
	}

	c.broadcastLocked(MsgProposal, msg)

	c.singleWriter.BufferProposal(buffered)
	c.scheduleFailoverTimerLocked()
	c.metrics.IncrProposalsReceived()
	if c.onProposalObserved != nil {
		c.onProposalObserved(ProposalAuditSummary{
			GroupID:      c.groupID,
			Epoch:        c.epoch,
			ActorPeerID:  c.localID.String(),
			ProposalType: pType,
			TargetPeerID: meta.TargetPeerID,
		})
	}

	if c.singleWriter.IsTokenHolder() {
		c.scheduleBatchCommitLocked()
	}
	return nil
}

func (c *Coordinator) createAndStoreLocalProposalLocked(pType ProposalType, data []byte, meta BufferedProposal) (ProposalMsg, BufferedProposal, error) {
	opCtx, cancel := c.mlsOperationContext()
	result, err := c.mls.CreateProposal(opCtx, c.groupState, pType, data)
	cancel()
	if err != nil {
		return ProposalMsg{}, BufferedProposal{}, err
	}
	if err := c.persistCurrentEpochStateLocked(result.NewGroupState); err != nil {
		return ProposalMsg{}, BufferedProposal{}, err
	}
	c.groupState = result.NewGroupState

	msg := ProposalMsg{
		ProposalType:   pType,
		Data:           append([]byte(nil), result.ProposalBytes...),
		ProposalRef:    append([]byte(nil), result.ProposalRef...),
		OperationID:    meta.OperationID,
		TargetPeerID:   meta.TargetPeerID,
		RequestID:      meta.RequestID,
		GroupType:      meta.GroupType,
		CategoryID:     meta.CategoryID,
		KeyPackageHash: append([]byte(nil), meta.KeyPackageHash...),
	}
	buffered := BufferedProposal{
		Type:           pType,
		Data:           append([]byte(nil), result.ProposalBytes...),
		ProposalRef:    append([]byte(nil), result.ProposalRef...),
		OperationID:    meta.OperationID,
		TargetPeerID:   meta.TargetPeerID,
		RequestID:      meta.RequestID,
		GroupType:      meta.GroupType,
		CategoryID:     meta.CategoryID,
		KeyPackageHash: append([]byte(nil), meta.KeyPackageHash...),
	}
	return msg, buffered, nil
}
