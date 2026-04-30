package service

import (
	"fmt"
	"strings"
	"time"

	"app/adapter/store"

	"github.com/libp2p/go-libp2p/core/peer"
)

func isValidPeerID(peerID string) bool {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return false
	}
	_, err := peer.Decode(peerID)
	return err == nil
}

func shortPeerID(peerID string) string {
	if len(peerID) <= 14 {
		return peerID
	}
	return peerID[:6] + "..." + peerID[len(peerID)-6:]
}

func (r *Runtime) resolveDisplayNameForPeer(peerID string) string {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return ""
	}

	r.mu.RLock()
	node := r.node
	database := r.db
	r.mu.RUnlock()

	localID := ""
	localName := ""
	if database != nil {
		if onboarding, err := r.GetOnboardingInfo(); err == nil && onboarding != nil {
			localID = onboarding.PeerID
		}
		if identity, err := database.GetMLSIdentity(); err == nil {
			localName = strings.TrimSpace(identity.DisplayName)
		}
	}
	if peerID == localID && localName != "" {
		return localName
	}
	if node != nil && node.AuthProtocol != nil {
		if pid, err := peer.Decode(peerID); err == nil {
			if tok := node.AuthProtocol.GetVerifiedToken(pid); tok != nil && strings.TrimSpace(tok.DisplayName) != "" {
				if database != nil {
					_ = database.UpsertPeerProfile(peerID, strings.TrimSpace(tok.DisplayName))
				}
				return strings.TrimSpace(tok.DisplayName)
			}
		}
	}
	if database != nil {
		if name, _ := database.GetPeerProfile(peerID); strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return shortPeerID(peerID)
}

func (r *Runtime) upsertGroupMember(groupID, peerID, role, source string) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" || peerID == "" {
		return fmt.Errorf("group_id and peer_id are required")
	}
	if !isValidPeerID(peerID) {
		return fmt.Errorf("invalid peer_id format")
	}
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}

	displayName := r.resolveDisplayNameForPeer(peerID)
	return database.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     groupID,
		PeerID:      peerID,
		DisplayName: displayName,
		Role:        role,
		Status:      store.GroupMemberStatusActive,
		Source:      source,
		UpdatedAt:   time.Now().Unix(),
	})
}

func (r *Runtime) ensureGroupRosterBackfilled(groupID string) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || r.coordStorage == nil {
		return
	}
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node != nil {
		_ = r.upsertGroupMember(groupID, node.Host.ID().String(), "member", "self")
	}

	known, err := r.coordStorage.GetKnownGroupMembers(groupID)
	if err != nil {
		return
	}
	for _, peerID := range known {
		if !isValidPeerID(peerID) {
			continue
		}
		_ = r.upsertGroupMember(groupID, peerID, "member", "history")
	}
}
