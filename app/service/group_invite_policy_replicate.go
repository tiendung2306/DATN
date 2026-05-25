package service

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
)

var errReplicationStaleGroupInvitePolicy = errors.New("replication: stale group invite policy revision")

const groupInvitePolicyWireVersion = 1

type groupInvitePolicyWireV1 struct {
	V           int    `json:"v"`
	GroupID     string `json:"group_id"`
	Policy      string `json:"policy"`
	ActorPeerID string `json:"actor_peer_id"`
	CreatedAt   int64  `json:"created_at"`
	Revision    int64  `json:"revision"`
}

func (r *Runtime) nextGroupInvitePolicyRevision(groupID string) (int64, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	row, err := db.GetReplicatedRecord(store.NamespaceGroupInvitePolicyV1, strings.TrimSpace(groupID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 1, nil
		}
		return 0, err
	}
	return row.Revision + 1, nil
}

func (r *Runtime) packSignedGroupInvitePolicyPayload(groupID, policy, actorPeerID string) (wireJSON, sig []byte, err error) {
	groupID = strings.TrimSpace(groupID)
	policy, err = store.NormalizeGroupInvitePolicy(policy)
	if err != nil {
		return nil, nil, err
	}
	actorPeerID = strings.TrimSpace(actorPeerID)
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return nil, nil, fmt.Errorf("database not initialized")
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return nil, nil, err
	}
	rev, err := r.nextGroupInvitePolicyRevision(groupID)
	if err != nil {
		return nil, nil, err
	}
	wire := groupInvitePolicyWireV1{
		V:           groupInvitePolicyWireVersion,
		GroupID:     groupID,
		Policy:      policy,
		ActorPeerID: actorPeerID,
		CreatedAt:   time.Now().Unix(),
		Revision:    rev,
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, nil, err
	}
	priv, err := normalizeMLSEd25519PrivateKey(identity.SigningKeyPrivate)
	if err != nil {
		return nil, nil, err
	}
	return raw, signProfileWire(priv, raw), nil
}

func (r *Runtime) persistOwnReplicatedGroupInvitePolicy(wireJSON, signature []byte) error {
	var w groupInvitePolicyWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return err
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return err
	}
	body := string(wireJSON)
	return db.PutReplicatedRecordForce(
		store.NamespaceGroupInvitePolicyV1, w.GroupID, w.ActorPeerID,
		w.Revision, 1, body, store.ReplicatedBodyHash(body), signature,
		strings.ToLower(hex.EncodeToString(identity.PublicKey)), 0, nil,
	)
}

func (r *Runtime) replicateGroupInvitePolicyAfterLocalSave(groupID, policy, actorPeerID string) {
	wire, sig, err := r.packSignedGroupInvitePolicyPayload(groupID, policy, actorPeerID)
	if err != nil || len(wire) == 0 || len(sig) == 0 {
		slog.Debug("group invite policy replication pack failed", "group", groupID, "err", err)
		return
	}
	if err := r.persistOwnReplicatedGroupInvitePolicy(wire, sig); err != nil {
		slog.Debug("group invite policy replication persist failed", "group", groupID, "err", err)
		return
	}
	r.publishBlindStoreReplicatedGroupInvitePolicy(wire, sig)
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil || node.AuthProtocol == nil {
		return
	}
	meta := p2p.ReplicaPushMetaV1{Namespace: store.NamespaceGroupInvitePolicyV1, RecordKey: strings.TrimSpace(groupID)}
	self := node.Host.ID()
	for _, pid := range node.AuthProtocol.VerifiedPeerIDs() {
		if pid == self {
			continue
		}
		to := pid
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			if err := p2p.PushReplicaStoreRecord(ctx, node.Host, to, meta, wire, sig, nil); err != nil {
				slog.Debug("group invite policy replica push failed", "peer", to, "group", groupID, "err", err)
			}
		}()
	}
}

func (r *Runtime) applySignedRemoteGroupInvitePolicyPush(actorPeerID string, wireJSON, signature []byte) error {
	actorPeerID = strings.TrimSpace(actorPeerID)
	if actorPeerID == "" {
		return fmt.Errorf("actor peer id required")
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	profile, err := db.GetPeerDirectoryProfile(actorPeerID)
	if err != nil {
		return err
	}
	pubBytes, err := hex.DecodeString(strings.TrimSpace(profile.PublicKeyHex))
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid stored public key for actor")
	}
	if err := verifyProfileWire(ed25519.PublicKey(pubBytes), wireJSON, signature); err != nil {
		return err
	}
	var w groupInvitePolicyWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return err
	}
	if w.V != groupInvitePolicyWireVersion {
		return fmt.Errorf("unsupported group invite policy wire version %d", w.V)
	}
	if strings.TrimSpace(w.ActorPeerID) != actorPeerID {
		return fmt.Errorf("actor peer mismatch")
	}
	if _, err := store.NormalizeGroupInvitePolicy(w.Policy); err != nil {
		return err
	}
	ok, err := r.isActiveGroupAuthorizedPeer(w.GroupID, actorPeerID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("group invite policy push: actor is not an active admin")
	}
	body := string(wireJSON)
	if err := db.TryMergeReplicatedRecord(
		store.NamespaceGroupInvitePolicyV1, strings.TrimSpace(w.GroupID), actorPeerID,
		w.Revision, 1, body, store.ReplicatedBodyHash(body), signature,
		strings.ToLower(hex.EncodeToString(pubBytes)), 0, nil,
	); err != nil {
		if errors.Is(err, store.ErrReplicatedStaleRevision) {
			return errReplicationStaleGroupInvitePolicy
		}
		return err
	}
	if err := db.SetGroupInvitePolicy(w.GroupID, w.Policy); err != nil {
		return err
	}
	if w.Policy == store.GroupInvitePolicyAnyMember {
		_ = r.processPolicySwitchAnyMember(w.GroupID)
	}
	r.emit("group:invite_policy_changed", map[string]interface{}{
		"group_id": w.GroupID,
		"policy":   w.Policy,
	})
	return nil
}
