//go:build business_integration

// Sprint 5 — BI-081–BI-086 channel categories (BI-087 P2P sync: smoke/deferred).

package service

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestBusinessP1_Sprint5_BI081_ListChannelCategories_Baseline(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	cats, err := rt.ListChannelCategories()
	if err != nil {
		t.Fatalf("ListChannelCategories: %v", err)
	}
	if len(cats) == 0 {
		t.Fatal("expected baseline categories after ensure")
	}
	var sawGeneral bool
	for _, c := range cats {
		if strings.Contains(strings.ToLower(c.Name), "general") || c.CategoryID == defaultChannelCategoryID {
			sawGeneral = true
			break
		}
	}
	if !sawGeneral {
		t.Fatalf("expected default/general category in list: %+v", cats)
	}
}

func TestBusinessP1_Sprint5_BI082_CreateChannelCategory(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	info, err := rt.CreateChannelCategory("Sprint5-NewCat")
	if err != nil {
		t.Fatalf("CreateChannelCategory: %v", err)
	}
	if info.CategoryID == "" || info.Name != "Sprint5-NewCat" {
		t.Fatalf("unexpected info: %+v", info)
	}
	cats, err := rt.ListChannelCategories()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cats {
		if c.CategoryID == info.CategoryID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created category not listed")
	}
}

func TestBusinessP1_Sprint5_BI083_CreateCategory_BlankRejected(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	_, err := rt.CreateChannelCategory("   ")
	if err == nil {
		t.Fatal("expected error for blank category name")
	}
}

// BI-083 matrix also expects rejecting duplicate category names. Current implementation generates a new ID per
// CreateChannelCategory and does not enforce unique names — see product note in sprint summary.
func TestBusinessP1_Sprint5_BI083_DuplicateName_AllowsMultipleIDs(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	name := "DupCatName"
	a, err := rt.CreateChannelCategory(name)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rt.CreateChannelCategory(name)
	if err != nil {
		t.Fatal(err)
	}
	if a.CategoryID == b.CategoryID {
		t.Fatalf("expected distinct category IDs for duplicate names, got %q", a.CategoryID)
	}
}

func TestBusinessP1_Sprint5_BI084_AssignChannelCategory(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	catID := businessEnsureCategory(t, rt, "S5-Assign")
	gid := "grp-s5-084"
	if err := rt.CreateGroupChat(gid, "channel", catID); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	cat2, err := rt.CreateChannelCategory("S5-Second")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.AssignChannelCategory(gid, cat2.CategoryID); err != nil {
		t.Fatalf("AssignChannelCategory: %v", err)
	}
	groups, err := rt.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if g.GroupID == gid && g.CategoryID == cat2.CategoryID {
			return
		}
	}
	t.Fatalf("channel %q not found with category %q in GetGroups: %+v", gid, cat2.CategoryID, groups)
}

func TestBusinessP1_Sprint5_BI085_DeleteUnusedCategory(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	info, err := rt.CreateChannelCategory("S5-DeleteMe")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.DeleteChannelCategory(info.CategoryID); err != nil {
		t.Fatalf("DeleteChannelCategory: %v", err)
	}
	cats, err := rt.ListChannelCategories()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cats {
		if c.CategoryID == info.CategoryID {
			t.Fatal("category still listed after delete")
		}
	}
}

func TestBusinessP1_Sprint5_BI086_DeleteCategory_NotEmptyBlocked(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	catID := businessEnsureCategory(t, rt, "S5-NonEmpty")
	gid := "grp-s5-086"
	if err := rt.CreateGroupChat(gid, "channel", catID); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	err := rt.DeleteChannelCategory(catID)
	if err == nil {
		t.Fatal("expected error when category still has active channel")
	}
	if !strings.Contains(err.Error(), "ERR_CATEGORY_NOT_EMPTY") {
		t.Fatalf("err=%v", err)
	}
}

// Two channel groups can live under different categories. Runtime sets ConversationTitle from group_id for channels,
// so we use distinct group_ids that share the same human-readable slug ("announcements") to model “same name” per category.
func TestBusinessP1_Sprint5_TwoChannels_SameSlugDifferentCategories(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	catA := businessEnsureCategory(t, rt, "S5-DupCat-A")
	catB := businessEnsureCategory(t, rt, "S5-DupCat-B")
	slug := "announcements"
	gidA := "chan-" + slug + "-workspace-a"
	gidB := "chan-" + slug + "-workspace-b"
	if err := rt.CreateGroupChat(gidA, "channel", catA); err != nil {
		t.Fatalf("CreateGroupChat A: %v", err)
	}
	if err := rt.CreateGroupChat(gidB, "channel", catB); err != nil {
		t.Fatalf("CreateGroupChat B: %v", err)
	}
	groups, err := rt.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	var gotA, gotB *GroupInfo
	for i := range groups {
		g := &groups[i]
		switch g.GroupID {
		case gidA:
			gotA = g
		case gidB:
			gotB = g
		}
	}
	if gotA == nil || gotB == nil {
		t.Fatalf("missing groups: gotA=%v gotB=%v all=%+v", gotA != nil, gotB != nil, groups)
	}
	if gotA.CategoryID != catA || gotB.CategoryID != catB {
		t.Fatalf("category mismatch: A=%q want %q B=%q want %q", gotA.CategoryID, catA, gotB.CategoryID, catB)
	}
	if gotA.GroupType != "channel" || gotB.GroupType != "channel" {
		t.Fatalf("want channel type: %+v %+v", gotA, gotB)
	}
	if !strings.Contains(gotA.ConversationTitle, slug) || !strings.Contains(gotB.ConversationTitle, slug) {
		t.Fatalf("titles should include slug %q: %q %q", slug, gotA.ConversationTitle, gotB.ConversationTitle)
	}
}

// Regression: after Welcome join, Bob's channel keeps category_id from
// replicated invite metadata (stored_welcomes), matching creator assignment.
func TestBusiness_ChannelCategoryPreservedAfterJoinFromStoredWelcome(t *testing.T) {
	aliceRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, aliceRoot)
	alice, _ := businessRuntimeStartMockInWorkDir(t, aliceRoot)
	defer businessShutdownRuntimeInWorkDir(t, alice)

	catID := businessEnsureCategory(t, alice, "BUG-Category-C")
	gid := "grp-bug-category-lost"
	if err := alice.CreateGroupChat(gid, "channel", catID); err != nil {
		t.Fatalf("alice CreateGroupChat: %v", err)
	}

	bobRoot := t.TempDir()
	businessSeedAuthorizedWorkDir(t, bobRoot)
	bob, _ := businessRuntimeStartMockInWorkDir(t, bobRoot)
	defer businessShutdownRuntimeInWorkDir(t, bob)

	kp, err := bob.GenerateKeyPackage()
	if err != nil {
		t.Fatalf("bob GenerateKeyPackage: %v", err)
	}
	bInfo, _ := bob.GetOnboardingInfo()
	welcomeHex, err := alice.AddMemberToGroup(gid, bInfo.PeerID, kp.PublicHex)
	if err != nil {
		t.Fatalf("alice AddMemberToGroup: %v", err)
	}
	welcomeBytes, err := hex.DecodeString(welcomeHex)
	if err != nil {
		t.Fatalf("decode welcome hex: %v", err)
	}
	aInfo, _ := alice.GetOnboardingInfo()
	bob.mu.RLock()
	bobDB := bob.db
	bob.mu.RUnlock()
	if bobDB == nil {
		t.Fatal("bob db nil")
	}
	// Simulates blind-store / inviter replication: invitee row carries category_id
	// before JoinGroupWithWelcome (manual accept path).
	if err := bobDB.SaveStoredWelcome(bInfo.PeerID, gid, "channel", catID, welcomeBytes, aInfo.PeerID, 0, nil); err != nil {
		t.Fatalf("SaveStoredWelcome: %v", err)
	}
	if err := bob.JoinGroupWithWelcome(gid, welcomeHex, kp.BundlePrivateHex); err != nil {
		t.Fatalf("bob JoinGroupWithWelcome: %v", err)
	}

	groups, err := bob.GetGroups()
	if err != nil {
		t.Fatalf("bob GetGroups: %v", err)
	}
	var joined *GroupInfo
	for i := range groups {
		if groups[i].GroupID == gid {
			joined = &groups[i]
			break
		}
	}
	if joined == nil {
		t.Fatalf("bob missing joined group %q, groups=%+v", gid, groups)
	}
	if joined.CategoryID != catID {
		t.Fatalf("bob channel category: got %q want %q", joined.CategoryID, catID)
	}
}
