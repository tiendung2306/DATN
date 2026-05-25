package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
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

// upsertGroupMember is the AUTHORITATIVE write path: caller knows the role
// (e.g. CreateGroupChat passing "creator"). For roster-sync writes where
// the caller only observed a peer's existence but does NOT have role
// information, use upsertGroupMemberFromRosterSync instead — that one
// preserves the existing role column verbatim so the local creator row
// is never silently demoted to "member" by a heartbeat / MLS leaf /
// welcome-source observation.
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

// upsertGroupMemberFromRosterSync is the ROSTER-SYNC write path: caller
// observed a peer (heartbeat, MLS leaf enumeration, message-sender,
// welcome-source, stored-message history, etc.) but has NO authoritative
// information about the peer's role. The DB-level method this delegates
// to leaves the role column untouched on existing rows so the creator's
// own "creator" annotation is never demoted to "member" by a refresh.
//
// For brand-new rows the row lands with role="member" (default — the
// observation alone doesn't promote anyone to "creator"; only an
// explicit CreateGroupChat does).
func (r *Runtime) upsertGroupMemberFromRosterSync(groupID, peerID, source string) error {
	return r.upsertGroupMemberFromRosterEvidence(groupID, peerID, source, false)
}

func (r *Runtime) upsertGroupMemberFromMLSReconcile(groupID, peerID, source string) error {
	return r.upsertGroupMemberFromRosterEvidence(groupID, peerID, source, true)
}

func (r *Runtime) upsertGroupMemberFromRosterEvidence(groupID, peerID, source string, reactivateLeft bool) error {
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
	if !reactivateLeft {
		rec, err := database.GetGroupMember(groupID, peerID)
		if err != nil {
			return err
		}
		if rec != nil && rec.Status == store.GroupMemberStatusLeft {
			return nil
		}
	}

	displayName := r.resolveDisplayNameForPeer(peerID)
	return database.UpsertGroupMemberPreservingRole(store.GroupMemberRecord{
		GroupID:     groupID,
		PeerID:      peerID,
		DisplayName: displayName,
		Role:        "member",
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
		// Self observation — caller has no opinion on role. Preserve so the
		// creator's own "creator" annotation set by CreateGroupChat survives
		// every UI refresh.
		_ = r.upsertGroupMemberFromRosterSync(groupID, node.Host.ID().String(), "self")
	}

	known, err := r.coordStorage.GetKnownGroupMembers(groupID)
	if err == nil {
		for _, peerID := range known {
			if !isValidPeerID(peerID) {
				continue
			}
			_ = r.upsertGroupMemberFromRosterSync(groupID, peerID, "history")
		}
	}
	// MLS leaf enumeration: every refresh asks the crypto sidecar for the
	// authoritative roster directly so we recover from any local drift
	// (stale stored_messages cache, fork heal, missed welcome-source).
	r.reconcileGroupRosterWithMLS(groupID)
}

// backfillMLSLeafRoster reconstructs the local group_members roster from
// cryptographic ground truth by enumerating every leaf in the MLS group's
// ratchet tree and translating each leaf's BasicCredential identity into a
// peer.ID via the peer_directory pubkey index.
//
// This is the Phase B "MLS leaf enumeration" path: independent of welcome-
// source, message history, and heartbeats, it guarantees that any joiner
// who has completed JoinGroupWithWelcome sees the full membership of the
// group in its UI immediately. The complementary Phase A heartbeat path
// (Coordinator.OnPeerObserved) catches any leaf whose pubkey is not yet
// in the directory because the inviting handshake has not completed.
//
// Safe to call repeatedly: UpsertGroupMember is idempotent, the call
// requires no extra MLS state mutation, and missing leaves (directory
// miss) are demoted to a debug-level log instead of failing the refresh.
func (r *Runtime) backfillMLSLeafRoster(groupID string) {
	_, _ = r.reconcileGroupRosterWithMLS(groupID)
}

func (r *Runtime) reconcileGroupRosterWithMLS(groupID string) (bool, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return false, nil
	}
	r.mu.RLock()
	engine := r.mlsEngine
	storage := r.coordStorage
	database := r.db
	ctx := r.ctx
	node := r.node
	r.mu.RUnlock()
	if engine == nil || storage == nil || database == nil {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	rec, err := storage.GetGroupRecord(groupID)
	if err != nil || rec == nil || len(rec.GroupState) == 0 {
		return false, err
	}
	identities, err := engine.ListMemberIdentities(ctx, rec.GroupState)
	if err != nil {
		slog.Debug("backfillMLSLeafRoster ListMemberIdentities failed",
			"group_id", groupID, "err", err)
		return false, err
	}
	if len(identities) == 0 {
		return false, nil
	}

	localPubHex := ""
	localPeerID := ""
	if id, err := database.GetMLSIdentity(); err == nil && id != nil {
		if len(id.PublicKey) > 0 {
			localPubHex = strings.ToLower(hex.EncodeToString(id.PublicKey))
		}
	}
	if node != nil {
		localPeerID = node.Host.ID().String()
	}

	resolved := 0
	missed := 0
	changed := false
	currentPeers := make(map[string]struct{}, len(identities))
	for _, id := range identities {
		if len(id) == 0 {
			continue
		}
		pubHex := strings.ToLower(hex.EncodeToString(id))
		if pubHex == localPubHex {
			if localPeerID != "" {
				currentPeers[localPeerID] = struct{}{}
				if existing, gerr := database.GetGroupMember(groupID, localPeerID); gerr == nil && (existing == nil || existing.Status != store.GroupMemberStatusActive) {
					changed = true
				}
				_ = r.upsertGroupMemberFromMLSReconcile(groupID, localPeerID, "self")
			}
			continue
		}
		peerID, lookupErr := database.GetPeerIDByPublicKeyHex(pubHex)
		if lookupErr != nil {
			slog.Debug("backfillMLSLeafRoster lookup failed",
				"group_id", groupID, "pubkey", pubHex, "err", lookupErr)
			missed++
			continue
		}
		if peerID == "" {
			missed++
			continue
		}
		currentPeers[peerID] = struct{}{}
		if existing, gerr := database.GetGroupMember(groupID, peerID); gerr == nil && (existing == nil || existing.Status != store.GroupMemberStatusActive) {
			changed = true
		}
		if err := r.upsertGroupMemberFromMLSReconcile(groupID, peerID, "mls_leaf"); err != nil {
			slog.Debug("backfillMLSLeafRoster upsert failed",
				"group_id", groupID, "peer", peerID, "err", err)
			continue
		}
		resolved++
	}
	if missed > 0 {
		slog.Debug("backfillMLSLeafRoster directory misses",
			"group_id", groupID, "resolved", resolved, "missed", missed,
			"hint", "Phase A heartbeat path will recover once peer handshakes")
	}
	if missed == 0 {
		rows, listErr := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
		if listErr == nil {
			for _, row := range rows {
				if _, ok := currentPeers[row.PeerID]; ok {
					continue
				}
				if err := database.MarkGroupMemberLeft(groupID, row.PeerID, 0); err == nil {
					changed = true
				}
			}
		}
	}
	// NOTE: do NOT emit group:members_changed from here. backfillMLSLeafRoster
	// is intentionally called from inside GetGroupMembers (via
	// ensureGroupRosterBackfilled), and the frontend listens to that event by
	// re-calling GetGroupMembers — emitting unconditionally on every UI fetch
	// creates a self-perpetuating refresh storm:
	//
	//   GetGroupMembers -> backfill (resolved>0) -> emit -> UI handleMembersChanged
	//     -> refreshGroupMembers -> GetGroupMembers -> backfill -> emit -> ...
	//
	// The caller that genuinely changes state (joinGroupWithWelcome,
	// makePeerObservedHandler) is responsible for emitting members_changed
	// exactly once per state transition; backfill remains pure read-through
	// reconciliation.
	return changed, nil
}
