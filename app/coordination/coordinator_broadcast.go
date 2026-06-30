package coordination

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (c *Coordinator) broadcastLocked(msgType MessageType, payload interface{}) {
	_ = c.broadcastWithTimestampLocked(msgType, payload, c.hlc.Now())
}

func (c *Coordinator) broadcastWithTimestampLocked(msgType MessageType, payload interface{}, ts HLCTimestamp) []byte {
	envBytes := c.buildEnvelopeWithTimestampLocked(msgType, payload, ts)
	if len(envBytes) == 0 {
		return nil
	}
	c.publishPreparedEnvelopeLocked(msgType, envBytes)
	return envBytes
}

func (c *Coordinator) buildEnvelopeWithTimestampLocked(msgType MessageType, payload interface{}, ts HLCTimestamp) []byte {
	return c.buildEnvelopeWithEpochAndTimestampLocked(msgType, payload, c.epoch, ts)
}

func (c *Coordinator) buildEnvelopeWithEpochAndTimestampLocked(msgType MessageType, payload interface{}, epoch uint64, ts HLCTimestamp) []byte {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	env := Envelope{
		Type:      msgType,
		GroupID:   c.groupID,
		Epoch:     epoch,
		From:      c.localID.String(),
		Timestamp: ts,
		Payload:   payloadBytes,
	}
	envBytes, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	return envBytes
}

func (c *Coordinator) publishPreparedEnvelopeLocked(msgType MessageType, envBytes []byte) {
	if err := c.transport.Publish(c.ctx, GroupTopic(c.groupID), envBytes); err != nil {
		slog.Warn("Failed to publish coordination envelope",
			"group", c.groupID,
			"type", msgType,
			"epoch", c.epoch,
			"err", err,
		)
	}
	if c.onEnvelope != nil && (msgType == MsgCommit || msgType == MsgApplication) {
		c.onEnvelope(msgType, c.groupID, envBytes)
	}
}

func (c *Coordinator) appendOfflineEnvelopeLocked(wire []byte) {
	if c.cfg == nil || !c.cfg.OfflineSyncEnabled || len(wire) == 0 {
		return
	}
	var env Envelope
	if err := json.Unmarshal(wire, &env); err != nil {
		return
	}
	if env.Type != MsgCommit && env.Type != MsgApplication {
		return
	}
	if _, err := c.storage.AppendEnvelope(c.groupID, env.Type, env.Epoch, env.Timestamp, wire); err != nil {
		slog.Warn("Failed to append offline envelope", "group", c.groupID, "type", env.Type, "error", err)
	}
}

func (c *Coordinator) persistCoordStateLocked() error {
	if c.storage == nil {
		return nil
	}
	var holder peer.ID
	if c.singleWriter != nil {
		if h, err := c.singleWriter.CurrentTokenHolder(); err == nil {
			holder = h
		}
	}
	return c.storage.SaveCoordState(&CoordState{
		GroupID:        c.groupID,
		ActiveView:     c.activeView.Members(),
		TokenHolder:    holder,
		LastCommitHash: copyBytes(c.lastCommitHash),
		HistoryChain:   c.historyChain,
	})
}

func (c *Coordinator) persistCurrentEpochStateLocked(newState []byte) error {
	now := c.clock.Now()
	rec := &GroupRecord{
		GroupID:    c.groupID,
		GroupState: append([]byte(nil), newState...),
		Epoch:      c.epoch,
		TreeHash:   append([]byte(nil), c.treeHash...),
		UpdatedAt:  now,
	}
	if prev, err := c.storage.GetGroupRecord(c.groupID); err == nil && prev != nil {
		rec.MyRole = prev.MyRole
		rec.GroupType = prev.GroupType
		rec.CategoryID = prev.CategoryID
		rec.DMCounterpartyPeerID = prev.DMCounterpartyPeerID
		rec.CreatedAt = prev.CreatedAt
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	return c.storage.SaveGroupRecord(rec)
}

func (c *Coordinator) saveCurrentGroupStateLocked(now time.Time) error {
	prevRec, err := c.storage.GetGroupRecord(c.groupID)
	if err != nil && !errors.Is(err, ErrGroupNotFound) {
		return err
	}
	role := RoleMember
	groupType := ""
	categoryID := ""
	createdAt := now
	if prevRec != nil {
		if prevRec.MyRole != "" {
			role = prevRec.MyRole
		}
		groupType = prevRec.GroupType
		categoryID = prevRec.CategoryID
		if !prevRec.CreatedAt.IsZero() {
			createdAt = prevRec.CreatedAt
		}
	}
	return c.storage.SaveGroupRecord(&GroupRecord{
		GroupID:    c.groupID,
		GroupState: c.groupState,
		Epoch:      c.epoch,
		TreeHash:   c.treeHash,
		MyRole:     role,
		GroupType:  groupType,
		CategoryID: categoryID,
		CreatedAt:  createdAt,
		UpdatedAt:  now,
	})
}

func decodeEnvelopePeerID(raw string, fallback peer.ID) peer.ID {
	if raw == "" {
		return fallback
	}
	id, err := peer.Decode(raw)
	if err != nil {
		return fallback
	}
	return id
}

// updateLocalAccessRevocationLocked checks local membership against the latest
// MLS state and flips accessRevoked once if local identity is no longer present.
// Caller must hold c.mu.
func (c *Coordinator) updateLocalAccessRevocationLocked(groupState []byte, epoch uint64) {
	if c.accessRevoked {
		return
	}
	if len(c.localIdentity) == 0 {
		return
	}
	opCtx, cancel := c.mlsOperationContext()
	ok, err := c.mls.HasMember(opCtx, groupState, c.localIdentity)
	cancel()
	if err != nil {
		slog.Warn("Failed membership query", "group", c.groupID, "epoch", epoch, "err", err)
		return
	}
	if ok {
		return
	}
	c.accessRevoked = true
	slog.Warn("Local membership revoked", "group", c.groupID, "epoch", epoch)
	if c.onAccessLost != nil {
		cb := c.onAccessLost
		groupID := c.groupID
		go cb(groupID, epoch, "removed")
	}
}

func deriveIdentityFromSigningKey(signingKey []byte) []byte {
	var pub ed25519.PublicKey
	switch len(signingKey) {
	case ed25519.SeedSize:
		pub = ed25519.NewKeyFromSeed(signingKey).Public().(ed25519.PublicKey)
	case ed25519.PrivateKeySize:
		pub = ed25519.PrivateKey(signingKey).Public().(ed25519.PublicKey)
	default:
		return nil
	}
	out := make([]byte, len(pub))
	copy(out, pub)
	return out
}
