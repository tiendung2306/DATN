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

	"github.com/libp2p/go-libp2p/core/peer"
)

var errReplicationStaleGroupRole = errors.New("replication: stale group role revision")

const groupRoleWireVersion = 1

type groupRoleWireV1 struct {
	V            int    `json:"v"`
	EventID      string `json:"event_id"`
	GroupID      string `json:"group_id"`
	TargetPeerID string `json:"target_peer_id"`
	NewRole      string `json:"new_role"`
	ActorPeerID  string `json:"actor_peer_id"`
	CreatedAt    int64  `json:"created_at"`
	Revision     int64  `json:"revision"`
	Epoch        uint64 `json:"epoch"`
}

func groupRoleRecordKey(groupID, targetPeerID string) string {
	return strings.TrimSpace(groupID) + "|" + strings.TrimSpace(targetPeerID)
}

func (r *Runtime) GetGroupAdmins(groupID string) ([]MemberInfo, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, ErrGroupNotFound
	}
	members, err := r.GetGroupMembers(groupID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberInfo, 0)
	for _, m := range members {
		if m.IsAdmin {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *Runtime) SetGroupMemberAdmin(groupID, targetPeerID string, isAdmin bool) error {
	groupID = strings.TrimSpace(groupID)
	targetPeerID = strings.TrimSpace(targetPeerID)
	if groupID == "" || targetPeerID == "" {
		return fmt.Errorf("group_id and target_peer_id are required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	if _, err := peer.Decode(targetPeerID); err != nil {
		return fmt.Errorf("invalid target peer ID: %w", err)
	}
	_, actorPeerID, err := r.requireGroupPermission(groupID, permissionManageAdmins)
	if err != nil {
		return err
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	target, err := db.GetGroupMember(groupID, targetPeerID)
	if err != nil {
		return err
	}
	if target == nil || target.Status != store.GroupMemberStatusActive {
		return ErrRemoveMemberPeerNotKnown
	}
	if isCreatorRole(target.Role) {
		if isAdmin {
			return nil
		}
		return fmt.Errorf("%s: creator admin role cannot be revoked", errInviteForbidden)
	}
	newRole := store.GroupMemberRoleMember
	if isAdmin {
		newRole = store.GroupMemberRoleAdmin
	}
	if target.Role == newRole {
		return nil
	}
	if err := db.SetGroupMemberRole(groupID, targetPeerID, newRole); err != nil {
		return err
	}
	r.appendGroupEvent(groupID, "admin_role_changed", actorPeerID, targetPeerID, 0, map[string]any{
		"target_peer_id": targetPeerID,
		"new_role":       newRole,
	})
	r.emit("group:members_changed", map[string]interface{}{"group_id": groupID, "reason": "admin_role_changed"})
	r.emit("group:admins_changed", map[string]interface{}{"group_id": groupID, "target_peer_id": targetPeerID, "role": newRole})
	go r.replicateGroupRoleAfterLocalSave(groupID, targetPeerID, newRole, actorPeerID)
	return nil
}

func (r *Runtime) nextGroupRoleRevision(groupID, targetPeerID string) (int64, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	row, err := db.GetReplicatedRecord(store.NamespaceGroupRoleV1, groupRoleRecordKey(groupID, targetPeerID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 1, nil
		}
		return 0, err
	}
	return row.Revision + 1, nil
}

func (r *Runtime) packSignedGroupRolePayload(groupID, targetPeerID, newRole, actorPeerID string) (wireJSON, sig []byte, err error) {
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
	rev, err := r.nextGroupRoleRevision(groupID, targetPeerID)
	if err != nil {
		return nil, nil, err
	}
	epoch := uint64(0)
	if r.coordStorage != nil {
		if rec, recErr := r.coordStorage.GetGroupRecord(groupID); recErr == nil && rec != nil {
			epoch = rec.Epoch
		}
	}
	wire := groupRoleWireV1{
		V: groupRoleWireVersion, EventID: fmt.Sprintf("role:%s:%s:%d", groupID, targetPeerID, rev),
		GroupID: groupID, TargetPeerID: targetPeerID, NewRole: newRole, ActorPeerID: actorPeerID,
		CreatedAt: time.Now().Unix(), Revision: rev, Epoch: epoch,
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

func (r *Runtime) persistOwnReplicatedGroupRole(wireJSON, signature []byte) error {
	var w groupRoleWireV1
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
		store.NamespaceGroupRoleV1, groupRoleRecordKey(w.GroupID, w.TargetPeerID), w.ActorPeerID,
		w.Revision, 1, body, store.ReplicatedBodyHash(body), signature,
		strings.ToLower(hex.EncodeToString(identity.PublicKey)), 0, nil,
	)
}

func (r *Runtime) replicateGroupRoleAfterLocalSave(groupID, targetPeerID, newRole, actorPeerID string) {
	wire, sig, err := r.packSignedGroupRolePayload(groupID, targetPeerID, newRole, actorPeerID)
	if err != nil || len(wire) == 0 || len(sig) == 0 {
		slog.Debug("group role replication pack failed", "group", groupID, "target", targetPeerID, "err", err)
		return
	}
	if err := r.persistOwnReplicatedGroupRole(wire, sig); err != nil {
		slog.Debug("group role replication persist failed", "group", groupID, "target", targetPeerID, "err", err)
		return
	}
	r.publishBlindStoreReplicatedGroupRole(wire, sig)
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil || node.AuthProtocol == nil {
		return
	}
	meta := p2p.ReplicaPushMetaV1{Namespace: store.NamespaceGroupRoleV1, RecordKey: groupRoleRecordKey(groupID, targetPeerID)}
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
				slog.Debug("group role replica push failed", "peer", to, "group", groupID, "err", err)
			}
		}()
	}
}

func (r *Runtime) applySignedRemoteGroupRolePush(actorPeerID string, wireJSON, signature []byte) error {
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
	var w groupRoleWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return err
	}
	if w.V != groupRoleWireVersion {
		return fmt.Errorf("unsupported group role wire version %d", w.V)
	}
	if strings.TrimSpace(w.ActorPeerID) != actorPeerID {
		return fmt.Errorf("actor peer mismatch")
	}
	ok, err := r.isActiveGroupCreatorPeer(w.GroupID, actorPeerID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("group role push: actor is not creator")
	}
	target, err := db.GetGroupMember(w.GroupID, w.TargetPeerID)
	// We still merge the replicated record authoritatively even if the target member
	// is not yet backfilled/created in group_members. Roster backfill and dynamic
	// GetGroupMembers resolution will pick it up correctly when they are processed.
	memberExists := err == nil && target != nil && target.Status == store.GroupMemberStatusActive

	if memberExists && isCreatorRole(target.Role) {
		return fmt.Errorf("group role push: creator role immutable")
	}
	newRole := store.GroupMemberRoleMember
	if strings.EqualFold(strings.TrimSpace(w.NewRole), store.GroupMemberRoleAdmin) {
		newRole = store.GroupMemberRoleAdmin
	}
	body := string(wireJSON)
	if err := db.TryMergeReplicatedRecord(
		store.NamespaceGroupRoleV1, groupRoleRecordKey(w.GroupID, w.TargetPeerID), actorPeerID,
		w.Revision, 1, body, store.ReplicatedBodyHash(body), signature,
		strings.ToLower(hex.EncodeToString(pubBytes)), 0, nil,
	); err != nil {
		if errors.Is(err, store.ErrReplicatedStaleRevision) {
			return errReplicationStaleGroupRole
		}
		return err
	}
	if memberExists {
		if err := db.SetGroupMemberRole(w.GroupID, w.TargetPeerID, newRole); err != nil {
			return err
		}
	}
	r.emit("group:members_changed", map[string]interface{}{"group_id": w.GroupID, "reason": "admin_role_changed"})
	r.emit("group:admins_changed", map[string]interface{}{"group_id": w.GroupID, "target_peer_id": w.TargetPeerID, "role": newRole})
	return nil
}
