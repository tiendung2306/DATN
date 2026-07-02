package service

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"app/adapter/store"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

type groupPermission string

const (
	permissionManageAdmins       groupPermission = "manage_admins"
	permissionManageInvites      groupPermission = "manage_invites"
	permissionRemoveMembers      groupPermission = "remove_members"
	permissionChangeGroupSetting groupPermission = "change_group_settings"
)

func isCreatorRole(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), store.GroupMemberRoleCreator)
}

func isAdminRole(role string) bool {
	role = strings.TrimSpace(strings.ToLower(role))
	return role == store.GroupMemberRoleCreator || role == store.GroupMemberRoleAdmin
}

func (r *Runtime) localGroupMember(groupID string) (*store.GroupMemberRecord, string, error) {
	local, err := r.localPeerID()
	if err != nil {
		return nil, "", err
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return nil, local, fmt.Errorf("database not initialized")
	}
	rec, err := db.GetGroupMember(groupID, local)
	if err != nil {
		return nil, local, err
	}
	if rec == nil {
		return nil, local, ErrGroupNotFound
	}
	return rec, local, nil
}

func (r *Runtime) requireGroupPermission(groupID string, perm groupPermission) (*store.GroupMemberRecord, string, error) {
	rec, local, err := r.localGroupMember(groupID)
	if err != nil {
		return nil, local, err
	}
	if rec.Status != store.GroupMemberStatusActive {
		return nil, local, ErrGroupNotFound
	}
	switch perm {
	case permissionManageAdmins:
		if !isCreatorRole(rec.Role) {
			return nil, local, fmt.Errorf("%s: only creator can manage admins", errInviteForbidden)
		}
	case permissionManageInvites, permissionRemoveMembers, permissionChangeGroupSetting:
		if !isAdminRole(rec.Role) {
			return nil, local, fmt.Errorf("%s: admin role required", errInviteForbidden)
		}
	default:
		return nil, local, fmt.Errorf("unknown permission %q", perm)
	}
	return rec, local, nil
}

func (r *Runtime) isLocalCreator(groupID string) (bool, error) {
	rec, _, err := r.localGroupMember(groupID)
	if errors.Is(err, ErrGroupNotFound) || errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return rec.Status == store.GroupMemberStatusActive && isCreatorRole(rec.Role), nil
}

func (r *Runtime) authorizedCommittersForGroup(groupID string, _ uint64, _ []coordination.BufferedProposal) ([]peer.ID, error) {
	rows, err := r.listAuthorizedGroupMembers(groupID)
	if err != nil {
		return nil, err
	}
	return authorizedPeerIDsFromRows(rows), nil
}

func (r *Runtime) authorizedCommittersProvider(database *store.Database) coordination.AuthorizedCommittersProvider {
	return func(groupID string, _ uint64, _ []coordination.BufferedProposal) ([]peer.ID, error) {
		rows, err := listAuthorizedGroupMembersFromDB(database, groupID)
		if err != nil {
			return nil, err
		}
		return authorizedPeerIDsFromRows(rows), nil
	}
}

func authorizedPeerIDsFromRows(rows []store.GroupMemberRecord) []peer.ID {
	out := make([]peer.ID, 0, len(rows))
	for _, row := range rows {
		pid, err := peer.Decode(strings.TrimSpace(row.PeerID))
		if err == nil && pid != "" {
			out = append(out, pid)
		}
	}
	return out
}

func (r *Runtime) listAuthorizedGroupMembers(groupID string) ([]store.GroupMemberRecord, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	return listAuthorizedGroupMembersFromDB(db, groupID)
}

func listAuthorizedGroupMembersFromDB(db *store.Database, groupID string) ([]store.GroupMemberRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.ListAuthorizedCommitters(groupID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		iCreator := isCreatorRole(rows[i].Role)
		jCreator := isCreatorRole(rows[j].Role)
		if iCreator != jCreator {
			return iCreator
		}
		return rows[i].PeerID < rows[j].PeerID
	})
	return rows, nil
}

func (r *Runtime) isActiveGroupAuthorizedPeer(groupID, peerID string) (bool, error) {
	peerID = strings.TrimSpace(peerID)
	if groupID == "" || peerID == "" {
		return false, nil
	}
	rec, err := func() (*store.GroupMemberRecord, error) {
		r.mu.RLock()
		db := r.db
		r.mu.RUnlock()
		if db == nil {
			return nil, fmt.Errorf("database not initialized")
		}
		return db.GetGroupMember(groupID, peerID)
	}()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if rec != nil && rec.Status == store.GroupMemberStatusActive && isAdminRole(rec.Role) {
		return true, nil
	}
	return r.isActiveGroupCreatorPeer(groupID, peerID)
}

func (r *Runtime) authorizedGroupPeerIDs(groupID string) ([]peer.ID, error) {
	rows, err := r.listAuthorizedGroupMembers(groupID)
	if err != nil {
		return nil, err
	}
	out := make([]peer.ID, 0, len(rows))
	for _, row := range rows {
		pid, err := peer.Decode(strings.TrimSpace(row.PeerID))
		if err == nil && pid != "" {
			out = append(out, pid)
		}
	}
	return out, nil
}

func canRemoveMemberByRole(actorRole, targetRole string, samePeer bool) bool {
	if samePeer || !isAdminRole(actorRole) {
		return false
	}
	if isCreatorRole(targetRole) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(targetRole), store.GroupMemberRoleAdmin) {
		return false
	}
	return true
}
