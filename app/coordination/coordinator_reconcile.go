package coordination

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
)

func (c *Coordinator) reconcileOperationsAfterCommitLocked(commit CommitMsg) {
	ops, err := c.storage.ListPendingOperations(c.groupID)
	if err != nil {
		slog.Error("Failed to list pending operations for reconcile", "group", c.groupID, "error", err)
		return
	}

	for _, op := range ops {
		if op.Status != "PENDING" && op.Status != "PROPOSED" {
			continue
		}

		if op.OpType == "ADD_MEMBER" {
			for _, d := range commit.AddDeliveries {
				if d.OperationID == op.OperationID || d.TargetPeerID == *op.TargetMemberID {
					if d.OperationID == op.OperationID {
						op.Status = "COMMITTED"
					} else {
						op.Status = "SATISFIED_BY_OTHER"
					}
					op.UpdatedAt = c.clock.Now()
					if err := c.storage.SavePendingOperation(op); err != nil {
						slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
					}
					break
				}
			}
		} else if op.OpType == "REMOVE_MEMBER" {
			for _, p := range commit.IncludedProposals {
				if p.ProposalType == ProposalRemove && p.TargetPeerID == *op.TargetMemberID {
					if p.OperationID == op.OperationID {
						op.Status = "COMMITTED"
					} else {
						op.Status = "SATISFIED_BY_OTHER"
					}
					op.UpdatedAt = c.clock.Now()
					if err := c.storage.SavePendingOperation(op); err != nil {
						slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
					}
					break
				}
			}
		}
	}

	// Supersede stale membership operations: when an ADD/REMOVE membership
	// operation commits for a target, all prior opposite-type membership
	// operations for that same target become logically invalid. Mark them
	// SUPERSEDED and release their IdempotencyKey so future re-add or
	// re-remove attempts are not blocked by stale history.
	c.supersedeStaleMembershipOpsLocked(commit)

	// M5: Convert unapplied pending application envelopes to ORPHANED_OWN
	storageKey := deriveStorageKey(c.signingKey)
	pendingEnvs, err := c.storage.GetPendingEnvelopes(c.groupID, 1000)
	if err == nil {
		for _, record := range pendingEnvs {
			if record.MsgType == MsgApplication {
				var env Envelope
				if err := json.Unmarshal(record.Envelope, &env); err == nil {
					if env.From == c.localID.String() && env.Epoch < c.epoch {
						var appMsg ApplicationMsg
						if err := json.Unmarshal(env.Payload, &appMsg); err == nil {
							// For unapplied envelopes, we could try to decrypt them here
							// but normally they are already applied if we are the winner.
						}
					}
				}
			}
		}
	}

	// Winner specific logic: Replay messages sent in the previous epoch
	ownMsgs, _ := c.storage.GetMessagesByOwnerInRange(c.groupID, c.localID.String(), 0, c.clock.Now().UnixMilli())
	for _, m := range ownMsgs {
		if m.Epoch == c.epoch-1 {
			// Seal payload and add to ApplicationEvents
			sealedPayload, nonce, sealErr := sealPayload(m.Content, storageKey)
			if sealErr == nil {
				h := sha256.Sum256(m.Content)
				appEv := &ApplicationEvent{
					EventID:          hex.EncodeToString(m.EnvelopeHash),
					JobID:            "COMMIT-RECONCILE-" + c.groupID,
					GroupID:          c.groupID,
					OriginalBranchID: "",
					OriginalEpoch:    m.Epoch,
					AuthorID:         c.localID.String(),
					EnvelopeHash:     m.EnvelopeHash,
					PayloadSealed:    sealedPayload,
					PayloadHash:      h[:],
					SealKeyID:        "local_node_key",
					SealNonce:        nonce,
					HlcWallTimeMs:    m.Timestamp.WallTimeMs,
					HlcCounter:       m.Timestamp.Counter,
					HlcNodeID:        m.Timestamp.NodeID,
					Status:           "ORPHANED_OWN",
					CreatedAtMs:      c.clock.Now().UnixMilli(),
					UpdatedAtMs:      c.clock.Now().UnixMilli(),
				}
				if err := c.storage.SaveApplicationEvent(appEv); err != nil {
					slog.Warn("Failed to save application event", "group", c.groupID, "error", err)
				}
			}
		}
	}
}

func (c *Coordinator) supersedeStaleMembershipOpsLocked(commit CommitMsg) {
	type targetAction struct {
		addCommitted    bool
		removeCommitted bool
	}
	affected := make(map[string]*targetAction)
	committedOpIDs := make(map[string]bool)

	for _, d := range commit.AddDeliveries {
		if d.TargetPeerID == "" {
			continue
		}
		if affected[d.TargetPeerID] == nil {
			affected[d.TargetPeerID] = &targetAction{}
		}
		affected[d.TargetPeerID].addCommitted = true
		committedOpIDs[d.OperationID] = true
	}

	for _, p := range commit.IncludedProposals {
		if p.ProposalType == ProposalRemove && p.TargetPeerID != "" {
			if affected[p.TargetPeerID] == nil {
				affected[p.TargetPeerID] = &targetAction{}
			}
			affected[p.TargetPeerID].removeCommitted = true
			committedOpIDs[p.OperationID] = true
		}
	}

	if len(affected) == 0 {
		return
	}

	ops, err := c.storage.ListPendingOperations(c.groupID)
	if err != nil {
		slog.Error("Failed to list pending operations for supersession", "group", c.groupID, "error", err)
		return
	}

	for _, op := range ops {
		if op.Status != "COMMITTED" && op.Status != "SATISFIED_BY_OTHER" {
			continue
		}
		if op.TargetMemberID == nil || *op.TargetMemberID == "" {
			continue
		}
		// Never supersede operations that are part of the current commit itself.
		if committedOpIDs[op.OperationID] {
			continue
		}

		info, ok := affected[*op.TargetMemberID]
		if !ok {
			continue
		}

		shouldSupersede := false
		if info.removeCommitted && op.OpType == "ADD_MEMBER" {
			shouldSupersede = true
		}
		if info.addCommitted && op.OpType == "REMOVE_MEMBER" {
			shouldSupersede = true
		}

		if !shouldSupersede {
			continue
		}

		op.Status = "SUPERSEDED"
		op.IdempotencyKey = nil
		op.UpdatedAt = c.clock.Now()
		if err := c.storage.SavePendingOperation(op); err != nil {
			slog.Warn("Failed to save superseded pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
		}
		c.emitPendingOperationAuditLocked(PendingOperationAuditSummary{
			GroupID:      c.groupID,
			OperationID:  op.OperationID,
			OpType:       op.OpType,
			TargetPeerID: derefString(op.TargetMemberID),
			Stage:        "superseded",
			CurrentEpoch: c.epoch,
		})
	}
}

func (c *Coordinator) reconcileAndRebaseOperationsLocked() {
	ops, err := c.storage.ListPendingOperations(c.groupID)
	if err != nil {
		slog.Error("Failed to list pending operations for reconcile", "group", c.groupID, "error", err)
		return
	}

	for _, op := range ops {
		if op.Status != "PENDING" && op.Status != "PROPOSED" {
			continue
		}

		if op.ExpiresAt != nil && *op.ExpiresAt > 0 && c.clock.Now().Unix() > *op.ExpiresAt {
			op.Status = "FAILED_EXPIRED"
			errStr := "operation TTL expired"
			op.LastError = &errStr
			op.UpdatedAt = c.clock.Now()
			if err := c.storage.SavePendingOperation(op); err != nil {
				slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
			}
			continue
		}

		// P0.1 Fix: use MLS HasMember for membership-based satisfied check.
		// ActiveView tracks liveness (online/offline), NOT MLS group membership.
		// An offline peer is still an MLS member; using ActiveView here would
		// incorrectly mark Remove(offlinePeer) as SATISFIED_BY_OTHER.
		//
		// HasMember expects MLS identity bytes (raw signing public key), not a
		// peer ID string. For REMOVE_MEMBER, op.SemanticPayload holds the
		// target's MLS identity. For ADD_MEMBER, we don't have the target's
		// MLS identity (only their peer ID and key package), so we skip this
		// check — satisfaction is already detected by
		// reconcileOperationsAfterCommitLocked via commit.AddDeliveries.
		satisfied := false
		if op.TargetMemberID != nil && *op.TargetMemberID != "" && op.OpType == "REMOVE_MEMBER" && len(op.SemanticPayload) > 0 {
			opCtx, cancel := c.mlsOperationContext()
			isMember, err := c.mls.HasMember(opCtx, c.groupState, op.SemanticPayload)
			cancel()
			if err == nil {
				if !isMember {
					satisfied = true
				}
			} else {
				slog.Warn("HasMember check failed during reconcile; skipping satisfied check",
					"group", c.groupID, "opID", op.OperationID, "error", err)
			}
		}

		if satisfied {
			op.Status = "SATISFIED_BY_OTHER"
			op.UpdatedAt = c.clock.Now()
			if err := c.storage.SavePendingOperation(op); err != nil {
				slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
			}
			continue
		}

		if op.PreconditionEpoch != nil && *op.PreconditionEpoch < c.epoch {
			maxRetries := c.cfg.ApplicationDirectRetryLimit
			if maxRetries <= 0 {
				maxRetries = 5
			}
			if op.RetryCount >= maxRetries {
				op.Status = "FAILED_RETRY_EXHAUSTED"
				errStr := "max retries exceeded"
				op.LastError = &errStr
				op.UpdatedAt = c.clock.Now()
				if err := c.storage.SavePendingOperation(op); err != nil {
					slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
				}
				c.emitPendingOperationAuditLocked(PendingOperationAuditSummary{
					GroupID:      c.groupID,
					OperationID:  op.OperationID,
					OpType:       op.OpType,
					TargetPeerID: derefString(op.TargetMemberID),
					Stage:        "retry_exhausted",
					RetryCount:   op.RetryCount,
					CurrentEpoch: c.epoch,
					LastError:    errStr,
				})
				continue
			}

			op.RetryCount++
			prevPreconditionEpoch := uint64(0)
			if op.PreconditionEpoch != nil {
				prevPreconditionEpoch = *op.PreconditionEpoch
			}
			epochCopy := c.epoch
			op.PreconditionEpoch = &epochCopy
			op.UpdatedAt = c.clock.Now()

			slog.Info("Rebasing pending operation to new epoch",
				"group", c.groupID, "opID", op.OperationID, "type", op.OpType, "newEpoch", c.epoch, "retry", op.RetryCount)

			var reProposed bool
			var reProposeErr error

			if op.OpType == "ADD_MEMBER" {
				msg, buffered, err := c.createAndStoreLocalProposalLocked(ProposalAdd, op.SemanticPayload, BufferedProposal{
					OperationID:    op.OperationID,
					TargetPeerID:   *op.TargetMemberID,
					KeyPackageHash: append([]byte(nil), op.OperationHash...),
				})
				if err == nil {
					c.broadcastLocked(MsgProposal, msg)
					c.singleWriter.BufferProposal(buffered)
					if c.singleWriter.IsTokenHolder() {
						c.scheduleBatchCommitLocked()
					} else {
						c.scheduleFailoverTimerLocked()
					}
					h := sha256.Sum256(msg.Data)
					op.LatestProposalHash = h[:]
					op.Status = "PROPOSED"
					reProposed = true
				} else {
					reProposeErr = err
				}
			} else if op.OpType == "REMOVE_MEMBER" {
				msg, buffered, err := c.createAndStoreLocalProposalLocked(ProposalRemove, op.SemanticPayload, BufferedProposal{
					TargetPeerID: *op.TargetMemberID,
					OperationID:  op.OperationID,
				})
				if err == nil {
					c.broadcastLocked(MsgProposal, msg)
					c.singleWriter.BufferProposal(buffered)
					if c.singleWriter.IsTokenHolder() {
						c.scheduleBatchCommitLocked()
					} else {
						c.scheduleFailoverTimerLocked()
					}
					h := sha256.Sum256(msg.Data)
					op.LatestProposalHash = h[:]
					op.Status = "PROPOSED"
					reProposed = true
				} else {
					reProposeErr = err
				}
			}

			if reProposed {
				if dbOp, dbErr := c.storage.GetPendingOperation(op.OperationID); dbErr == nil && dbOp != nil {
					if dbOp.Status == "COMMITTED" || dbOp.Status == "SATISFIED_BY_OTHER" {
						op.Status = dbOp.Status
						op.UpdatedAt = dbOp.UpdatedAt
						op.LastError = dbOp.LastError
						if err := c.storage.SavePendingOperation(op); err != nil {
							slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
						}
						continue
					}
				}
				op.LastError = nil
				if err := c.storage.SavePendingOperation(op); err != nil {
					slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
				}
				c.emitPendingOperationAuditLocked(PendingOperationAuditSummary{
					GroupID:           c.groupID,
					OperationID:       op.OperationID,
					OpType:            op.OpType,
					TargetPeerID:      derefString(op.TargetMemberID),
					Stage:             "rebased",
					RetryCount:        op.RetryCount,
					CurrentEpoch:      c.epoch,
					PreconditionEpoch: prevPreconditionEpoch,
				})
			} else if reProposeErr != nil {
				errStr := reProposeErr.Error()
				op.LastError = &errStr
				slog.Warn("Failed to re-propose rebased operation", "opID", op.OperationID, "error", reProposeErr)
				if err := c.storage.SavePendingOperation(op); err != nil {
					slog.Warn("Failed to save pending operation", "group", c.groupID, "opID", op.OperationID, "error", err)
				}
				c.emitPendingOperationAuditLocked(PendingOperationAuditSummary{
					GroupID:           c.groupID,
					OperationID:       op.OperationID,
					OpType:            op.OpType,
					TargetPeerID:      derefString(op.TargetMemberID),
					Stage:             "rebase_failed",
					RetryCount:        op.RetryCount,
					CurrentEpoch:      c.epoch,
					PreconditionEpoch: prevPreconditionEpoch,
					LastError:         errStr,
				})
			}
		}
	}

	if c.singleWriter.IsTokenHolder() {
		c.scheduleBatchCommitLocked()
	}
}

func (c *Coordinator) emitPendingOperationAuditLocked(summary PendingOperationAuditSummary) {
	if c.onPendingOperation == nil {
		return
	}
	cb := c.onPendingOperation
	go cb(summary)
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
