package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"app/adapter/p2p"
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
)

var (
	ErrGroupNotFound             = errors.New(ErrCodeGroupNotFound + ": group not found")
	ErrRemoveMemberForbidden     = errors.New(ErrCodeRemoveMemberForbidden + ": only group creator can remove members")
	ErrRemoveSelfNotAllowed      = errors.New(ErrCodeRemoveMemberSelf + ": cannot remove yourself; use LeaveGroup")
	ErrRemoveMemberPeerNotKnown  = errors.New(ErrCodeRemoveMemberPeerNotKnown + ": target peer is not verified or missing MLS public key")
	ErrRemoveMemberAccessRevoked = errors.New(ErrCodeRemoveMemberAccessRevoked + ": local membership has been revoked")
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
	coordStorage := r.coordStorage
	coord := r.coordinators[groupID]
	node := r.node
	r.mu.RUnlock()
	if database == nil || coordStorage == nil {
		return fmt.Errorf("%s: runtime not initialized", ErrCodeRuntimeNotInitialized)
	}

	rec, err := coordStorage.GetGroupRecord(groupID)
	if errors.Is(err, coordination.ErrGroupNotFound) {
		return ErrGroupNotFound
	}
	if err != nil {
		return fmt.Errorf("load group record: %w", err)
	}
	if rec == nil {
		return ErrGroupNotFound
	}
	if rec.MyRole != coordination.RoleCreator {
		return ErrRemoveMemberForbidden
	}
	if coord == nil {
		return ErrGroupNotFound
	}

	localPeerID := ""
	if node != nil {
		localPeerID = node.Host.ID().String()
	} else if info, infoErr := r.GetOnboardingInfo(); infoErr == nil && info != nil {
		localPeerID = strings.TrimSpace(info.PeerID)
	}
	if localPeerID != "" && localPeerID == peerID {
		return ErrRemoveSelfNotAllowed
	}

	target, _ := peer.Decode(peerID)
	targetIdentity, err := resolveTargetMLSIdentity(target, node)
	if err != nil {
		return ErrRemoveMemberPeerNotKnown
	}
	if err := coord.RemoveMember(targetIdentity); err != nil {
		if errors.Is(err, coordination.ErrAccessRevoked) {
			return ErrRemoveMemberAccessRevoked
		}
		return fmt.Errorf("%s: %w", ErrCodeRemoveMemberCryptoFailure, err)
	}

	_ = database.MarkGroupMemberLeft(groupID, peerID, 0)
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
