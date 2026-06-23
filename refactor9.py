import sys

def main():
    path = "app/coordination/coordinator.go"
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    # The block we just inserted is a bit messed up
    # We want to completely replace from pendingEnvs to the end of the method
    
    start_str = """	// M5: Convert unapplied pending application envelopes to ORPHANED_OWN
	storageKey := deriveStorageKey(c.signingKey)
	pendingEnvs, err := c.storage.GetPendingEnvelopes(c.groupID, 1000)"""

    end_str = """					}
				}
			}
		}
	}
}"""
    
    start_idx = content.find(start_str)
    
    # find the end of the function
    func_start = content.find("func (c *Coordinator) reconcileOperationsAfterCommitLocked(commit CommitMsg) {")
    next_func = content.find("func (c *Coordinator) reconcileAndRebaseOperationsLocked() {", func_start)
    
    if start_idx != -1 and next_func != -1:
        new_content = content[:start_idx] + """	// M5: Convert unapplied pending application envelopes to ORPHANED_OWN
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
				_ = c.storage.SaveApplicationEvent(appEv)
			}
		}
	}
}
""" + content[next_func:]
        
        with open(path, "w", encoding="utf-8") as f:
            f.write(new_content)

if __name__ == "__main__":
    main()
