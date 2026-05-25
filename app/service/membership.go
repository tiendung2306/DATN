package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

var getVerifiedTokenPublicKey = func(node *p2p.P2PNode, target peer.ID) []byte {
	if node == nil || node.AuthProtocol == nil {
		return nil
	}
	tok := node.AuthProtocol.GetVerifiedToken(target)
	if tok == nil || len(tok.PublicKey) == 0 {
		return nil
	}
	out := make([]byte, len(tok.PublicKey))
	copy(out, tok.PublicKey)
	return out
}

// Stable error codes consumed by the frontend (see
// app/frontend/src/lib/formatRemoveMemberError.ts). Wire format is
// "<CODE>: <human-readable English detail>" so the UI can pattern-match the
// prefix while logs still keep the technical detail intact.
const (
	ErrCodeGroupNotFound             = "ERR_GROUP_NOT_FOUND"
	ErrCodeRemoveMemberForbidden     = "ERR_REMOVE_MEMBER_FORBIDDEN"
	ErrCodeRemoveMemberSelf          = "ERR_REMOVE_MEMBER_SELF"
	ErrCodeRemoveMemberPeerNotKnown  = "ERR_REMOVE_MEMBER_PEER_NOT_VERIFIED"
	ErrCodeRemoveMemberAccessRevoked = "ERR_REMOVE_MEMBER_ACCESS_REVOKED"
	ErrCodeRemoveMemberCryptoFailure = "ERR_REMOVE_MEMBER_CRYPTO_FAILURE"
	ErrCodeRemoveMemberInvalidPeerID = "ERR_REMOVE_MEMBER_INVALID_PEER_ID"
	ErrCodeRuntimeNotInitialized     = "ERR_RUNTIME_NOT_INITIALIZED"
	ErrCodeCreatorCannotLeave        = "ERR_CREATOR_CANNOT_LEAVE"
	ErrCodeRemoveCreatorForbidden    = "ERR_REMOVE_CREATOR_FORBIDDEN"
	ErrCodeRemoveAdminForbidden      = "ERR_REMOVE_ADMIN_FORBIDDEN"
	ErrCodeRemoveAdminRequiresDemote = "ERR_REMOVE_ADMIN_REQUIRES_DEMOTE"
)

var (
	ErrGroupNotFound             = errors.New(ErrCodeGroupNotFound + ": group not found")
	ErrRemoveMemberForbidden     = errors.New(ErrCodeRemoveMemberForbidden + ": admin role required to remove members")
	ErrRemoveSelfNotAllowed      = errors.New(ErrCodeRemoveMemberSelf + ": cannot remove yourself; use LeaveGroup")
	ErrRemoveMemberPeerNotKnown  = errors.New(ErrCodeRemoveMemberPeerNotKnown + ": target peer is not verified or missing MLS public key")
	ErrRemoveMemberAccessRevoked = errors.New(ErrCodeRemoveMemberAccessRevoked + ": local membership has been revoked")
	ErrCreatorCannotLeave        = errors.New(ErrCodeCreatorCannotLeave + ": group creator cannot leave the group")
	ErrRemoveCreatorForbidden    = errors.New(ErrCodeRemoveCreatorForbidden + ": group creator cannot be removed")
	ErrRemoveAdminForbidden      = errors.New(ErrCodeRemoveAdminForbidden + ": admins cannot remove other admins")
	ErrRemoveAdminRequiresDemote = errors.New(ErrCodeRemoveAdminRequiresDemote + ": revoke admin role before removing this member")
)

// LeaveGroup performs a local soft leave: active participation stops, while
// local group state and message history remain available for archive UX.
func (r *Runtime) LeaveGroup(groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return ErrGroupNotFound
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}

	r.mu.RLock()
	database := r.db
	node := r.node
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}

	exists, err := database.HasGroup(groupID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrGroupNotFound
	}
	if rec, _, roleErr := r.localGroupMember(groupID); roleErr == nil && isCreatorRole(rec.Role) {
		return ErrCreatorCannotLeave
	}

	active, err := database.IsGroupActive(groupID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	r.mu.Lock()
	var coordToStop interface{ Stop() }
	if r.coordinators != nil {
		if coord := r.coordinators[groupID]; coord != nil {
			coordToStop = coord
			delete(r.coordinators, groupID)
		}
	}
	r.mu.Unlock()

	if coordToStop != nil {
		coordToStop.Stop()
	}

	if !active && coordToStop == nil {
		return nil
	}
	if err := database.MarkGroupLeft(groupID); errors.Is(err, sql.ErrNoRows) {
		return ErrGroupNotFound
	} else if err != nil {
		return err
	}
	localPeerID := ""
	if node != nil {
		localPeerID = node.Host.ID().String()
	} else if info, infoErr := r.GetOnboardingInfo(); infoErr == nil && info != nil {
		localPeerID = info.PeerID
	}
	if localPeerID != "" {
		_ = database.MarkGroupMemberLeft(groupID, localPeerID, 0)
		r.appendGroupEvent(groupID, groupEventTypeMemberLeft, localPeerID, localPeerID, 0, map[string]any{
			"peer_id": localPeerID,
			"reason":  "left",
		})
	}

	r.emit("group:left", map[string]interface{}{
		"group_id": groupID,
		"reason":   "left",
	})
	r.emit("group:members_changed", map[string]interface{}{
		"group_id": groupID,
		"reason":   "left",
	})
	return nil
}

func (r *Runtime) RemoveMemberFromGroup(groupID string, peerID string) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" {
		return ErrGroupNotFound
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	if peerID == "" {
		return fmt.Errorf("%s: peer ID is required", ErrCodeRemoveMemberInvalidPeerID)
	}
	if _, err := peer.Decode(peerID); err != nil {
		return fmt.Errorf("%s: invalid peer ID %q: %v", ErrCodeRemoveMemberInvalidPeerID, peerID, err)
	}

	r.mu.RLock()
	database := r.db
	coord := r.coordinators[groupID]
	node := r.node
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("%s: runtime not initialized", ErrCodeRuntimeNotInitialized)
	}

	actor, localPeerID, err := r.requireGroupPermission(groupID, permissionRemoveMembers)
	if err != nil {
		return ErrRemoveMemberForbidden
	}
	if coord == nil {
		return ErrGroupNotFound
	}

	if localPeerID != "" && localPeerID == peerID {
		return ErrRemoveSelfNotAllowed
	}
	targetRec, err := database.GetGroupMember(groupID, peerID)
	if err != nil {
		return err
	}
	if targetRec == nil || targetRec.Status != store.GroupMemberStatusActive {
		return ErrRemoveMemberPeerNotKnown
	}
	if isCreatorRole(targetRec.Role) {
		return ErrRemoveCreatorForbidden
	}
	if strings.EqualFold(strings.TrimSpace(targetRec.Role), store.GroupMemberRoleAdmin) {
		if isCreatorRole(actor.Role) {
			return ErrRemoveAdminRequiresDemote
		}
		return ErrRemoveAdminForbidden
	}

	target, _ := peer.Decode(peerID)
	targetIdentity, err := resolveTargetMLSIdentity(target, node)
	if err != nil {
		return ErrRemoveMemberPeerNotKnown
	}
	if err := coord.RemoveMemberWithPeer(coordination.RemoveMemberRequest{TargetPeerID: target, TargetIdentity: targetIdentity}); err != nil {
		if errors.Is(err, coordination.ErrAccessRevoked) {
			return ErrRemoveMemberAccessRevoked
		}
		return fmt.Errorf("%s: %w", ErrCodeRemoveMemberCryptoFailure, err)
	}

	_ = database.MarkGroupMemberLeft(groupID, peerID, 0)
	r.appendGroupEvent(groupID, groupEventTypeMemberLeft, localPeerID, peerID, 0, map[string]any{
		"peer_id": peerID,
		"reason":  "removed",
	})
	r.emit("group:members_changed", map[string]interface{}{
		"group_id":       groupID,
		"reason":         "removed",
		"target_peer_id": peerID,
	})
	return nil
}

func resolveTargetMLSIdentity(target peer.ID, node *p2p.P2PNode) ([]byte, error) {
	if target == "" {
		return nil, ErrRemoveMemberPeerNotKnown
	}
	if pub := getVerifiedTokenPublicKey(node, target); len(pub) > 0 {
		return pub, nil
	}
	return nil, ErrRemoveMemberPeerNotKnown
}
