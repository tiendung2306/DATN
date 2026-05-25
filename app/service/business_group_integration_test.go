//go:build business_integration

// Sprint 2 — BI-026–BI-031 (group create, category, duplicate, GetGroups, GetGroupStatus).

package service

import (
	"app/adapter/store"
	"strings"
	"testing"
)

func TestBusinessP1_Sprint2_BI026_CreateDMGroup_CreatorInRoster(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "dm-bi026"

	if err := rt.CreateGroupChat(gid, "dm", ""); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	info, err := rt.GetOnboardingInfo()
	if err != nil || info == nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	members, err := rt.GetGroupMembers(gid)
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(members) < 1 {
		t.Fatalf("expected at least creator in roster, got %d", len(members))
	}
	var sawCreator bool
	for _, m := range members {
		if m.PeerID == info.PeerID {
			sawCreator = true
			break
		}
	}
	if !sawCreator {
		t.Fatal("creator peer not found in roster")
	}
}

func TestBusinessP1_Sprint2_BI026_CreateGroup_PersistsCreatorMetadata(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "group-bi026-metadata"

	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	info, err := rt.GetOnboardingInfo()
	if err != nil || info == nil {
		t.Fatalf("GetOnboardingInfo: %v", err)
	}
	creatorPeerID, err := rt.db.GetGroupCreatorPeerID(gid)
	if err != nil {
		t.Fatalf("GetGroupCreatorPeerID: %v", err)
	}
	if creatorPeerID != info.PeerID {
		t.Fatalf("creator peer id = %q, want %q", creatorPeerID, info.PeerID)
	}
	members, err := rt.db.ListGroupMembers(gid, store.GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	var creatorRow *store.GroupMemberRecord
	for i := range members {
		if members[i].PeerID == info.PeerID {
			creatorRow = &members[i]
			break
		}
	}
	if creatorRow == nil {
		t.Fatal("creator row missing from group_members")
	}
	if creatorRow.Role != "creator" {
		t.Fatalf("creator role = %q, want creator", creatorRow.Role)
	}
	if creatorRow.Source != "create" {
		t.Fatalf("creator source = %q, want create", creatorRow.Source)
	}
}

func TestBusinessP1_Sprint2_BI027_CreateChannelGroup_TypeAndCategory(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	catID := businessEnsureCategory(t, rt, "BI-027 Cat")
	gid := "chan-bi027"

	if err := rt.CreateGroupChat(gid, "channel", catID); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	groups, err := rt.GetGroups()
	if err != nil {
		t.Fatalf("GetGroups: %v", err)
	}
	var found *GroupInfo
	for i := range groups {
		if groups[i].GroupID == gid {
			found = &groups[i]
			break
		}
	}
	if found == nil {
		t.Fatal("channel group not in GetGroups")
	}
	if found.GroupType != "channel" {
		t.Fatalf("GroupType=%q want channel", found.GroupType)
	}
	if found.CategoryID != catID {
		t.Fatalf("CategoryID=%q want %q", found.CategoryID, catID)
	}
}

func TestBusinessP1_Sprint2_BI028_DuplicateGroupID_Rejected(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "dup-bi028"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatalf("first CreateGroupChat: %v", err)
	}
	err := rt.CreateGroupChat(gid, "group", "")
	if err == nil {
		t.Fatal("expected error on duplicate group id")
	}
	if !strings.Contains(err.Error(), "already in group") {
		t.Fatalf("error=%v want already in group", err)
	}
}

func TestBusinessP1_Sprint2_BI029_GetGroups_DMChannelAfterLeave(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)

	if err := rt.CreateGroupChat("g-dm", "dm", ""); err != nil {
		t.Fatalf("CreateGroupChat dm: %v", err)
	}
	cat := businessEnsureCategory(t, rt, "BI-029")
	if err := rt.CreateGroupChat("g-chan", "channel", cat); err != nil {
		t.Fatalf("CreateGroupChat channel: %v", err)
	}
	if err := rt.CreateGroupChat("g-left", "group", ""); err != nil {
		t.Fatalf("CreateGroupChat group: %v", err)
	}
	if err := rt.LeaveGroup("g-left"); err != nil {
		t.Fatalf("LeaveGroup: %v", err)
	}

	groups, err := rt.GetGroups()
	if err != nil {
		t.Fatalf("GetGroups: %v", err)
	}
	ids := make(map[string]struct{})
	for _, g := range groups {
		ids[g.GroupID] = struct{}{}
	}
	if _, ok := ids["g-dm"]; !ok {
		t.Fatal("expected dm group in list")
	}
	if _, ok := ids["g-chan"]; !ok {
		t.Fatal("expected channel group in list")
	}
	if _, ok := ids["g-left"]; ok {
		t.Fatal("left group should not appear in GetGroups (lifecycle inactive)")
	}
}

func TestBusinessP1_Sprint2_BI030_GetGroupStatus_Existing(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "status-bi030"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	st := rt.GetGroupStatus(gid)
	if errVal, ok := st["error"].(string); ok && errVal == "not in group" {
		t.Fatalf("unexpected error status: %#v", st)
	}
	if _, ok := st["epoch"]; !ok {
		t.Fatalf("expected epoch in status: %#v", st)
	}
}

func TestBusinessP1_Sprint2_BI031_GetGroupStatus_Missing(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	st := rt.GetGroupStatus("no-such-group-bi031")
	errStr, _ := st["error"].(string)
	if errStr != "not in group" {
		t.Fatalf("GetGroupStatus missing = %#v", st)
	}
}

func TestBusinessP1_Sprint2_BI026_ChannelRequiresCategory(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	err := rt.CreateGroupChat("ch-no-cat", "channel", "")
	if err == nil {
		t.Fatal("expected ERR_CATEGORY_REQUIRED")
	}
	if !strings.Contains(err.Error(), "ERR_CATEGORY_REQUIRED") {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint2_BI027_ChannelUnknownCategory(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	err := rt.CreateGroupChat("ch-bad-cat", "channel", "cat-does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected category error")
	}
	if !strings.Contains(err.Error(), "ERR_CATEGORY_NOT_FOUND") {
		t.Fatalf("err=%v", err)
	}
}
