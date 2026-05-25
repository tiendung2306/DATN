package store

import "testing"

func TestGroupMembers_UpsertListAndLeft(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     "g-1",
		PeerID:      "peer-a",
		DisplayName: "Alice",
		Role:        "creator",
		Status:      GroupMemberStatusActive,
		Source:      "create",
	}); err != nil {
		t.Fatalf("UpsertGroupMember first: %v", err)
	}
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     "g-1",
		PeerID:      "peer-a",
		DisplayName: "Alice Updated",
		Role:        "creator",
		Status:      GroupMemberStatusActive,
		Source:      "profile-refresh",
	}); err != nil {
		t.Fatalf("UpsertGroupMember second: %v", err)
	}
	rows, err := d.ListGroupMembers("g-1", GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers(active): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("active rows = %d, want 1", len(rows))
	}
	if rows[0].DisplayName != "Alice Updated" {
		t.Fatalf("display name = %q, want %q", rows[0].DisplayName, "Alice Updated")
	}
	if err := d.MarkGroupMemberLeft("g-1", "peer-a", 0); err != nil {
		t.Fatalf("MarkGroupMemberLeft: %v", err)
	}
	active, err := d.ListGroupMembers("g-1", GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers(active) after left: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active rows after left = %d, want 0", len(active))
	}
	all, err := d.ListGroupMembers("g-1")
	if err != nil {
		t.Fatalf("ListGroupMembers(all): %v", err)
	}
	if len(all) != 1 || all[0].Status != GroupMemberStatusLeft {
		t.Fatalf("all rows mismatch: %+v", all)
	}
}

// TestGroupMembers_UpsertPreservingRole_DoesNotDowngradeCreator pins the
// contract of the roster-sync write path. UpsertGroupMemberPreservingRole
// MUST NOT mutate the role column on an existing row, even if the caller
// passes a different role value in the input record. This guard is what
// keeps the creator's local row from being silently demoted to "member"
// every time the UI refreshes — pre-fix, ensureGroupRosterBackfilled and
// backfillMLSLeafRoster (which run on every GetGroupMembers tick) routed
// through plain UpsertGroupMember with role="member" and clobbered the
// row CreateGroupChat had just written with role="creator".
func TestGroupMembers_UpsertPreservingRole_DoesNotDowngradeCreator(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     "g-creator",
		PeerID:      "peer-creator",
		DisplayName: "Creator",
		Role:        "creator",
		Status:      GroupMemberStatusActive,
		Source:      "create",
	}); err != nil {
		t.Fatalf("UpsertGroupMember (initial creator): %v", err)
	}

	// Simulate the production refresh-storm: every UI-triggered roster
	// sync site routes through UpsertGroupMemberPreservingRole without
	// claiming to know the role.
	syntheticSources := []string{"self", "mls_leaf", "heartbeat", "welcome-source", "profile-refresh", "message", "history"}
	for _, src := range syntheticSources {
		if err := d.UpsertGroupMemberPreservingRole(GroupMemberRecord{
			GroupID:     "g-creator",
			PeerID:      "peer-creator",
			DisplayName: "Creator Refreshed",
			Role:        "member", // ignored by preserving variant on existing rows
			Status:      GroupMemberStatusActive,
			Source:      src,
		}); err != nil {
			t.Fatalf("UpsertGroupMemberPreservingRole (refresh source %q): %v", src, err)
		}
	}

	rows, err := d.ListGroupMembers("g-creator", GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1: %+v", len(rows), rows)
	}
	if rows[0].Role != "creator" {
		t.Fatalf("creator role demoted by preserving upsert: got %q, want %q (display=%q, source=%q)",
			rows[0].Role, "creator", rows[0].DisplayName, rows[0].Source)
	}
	// Sanity: other columns DID update so we know upsert ran (not a no-op).
	if rows[0].DisplayName != "Creator Refreshed" {
		t.Fatalf("display name not refreshed: got %q", rows[0].DisplayName)
	}
	if rows[0].Source != "history" {
		t.Fatalf("source not refreshed to last upsert: got %q want %q", rows[0].Source, "history")
	}
}

func TestGroupMembers_UpsertPreservingRole_DoesNotDowngradeAdmin(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID: "g-admin", PeerID: "peer-admin", DisplayName: "Admin",
		Role: GroupMemberRoleAdmin, Status: GroupMemberStatusActive, Source: "grant-admin",
	}); err != nil {
		t.Fatalf("UpsertGroupMember admin: %v", err)
	}
	if err := d.UpsertGroupMemberPreservingRole(GroupMemberRecord{
		GroupID: "g-admin", PeerID: "peer-admin", DisplayName: "Admin Refreshed",
		Role: GroupMemberRoleMember, Status: GroupMemberStatusActive, Source: "heartbeat",
	}); err != nil {
		t.Fatalf("UpsertGroupMemberPreservingRole: %v", err)
	}
	rows, err := d.ListGroupMembers("g-admin", GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	if len(rows) != 1 || rows[0].Role != GroupMemberRoleAdmin {
		t.Fatalf("admin role not preserved: %+v", rows)
	}
}

// TestGroupMembers_UpsertPreservingRole_InsertsNewRowWithMember verifies
// that UpsertGroupMemberPreservingRole still INSERTs a freshly-discovered
// peer with the supplied (normalised) role. This is the normal path on a
// joiner's node when MLS leaf enumeration or heartbeat reveals a peer for
// the first time — the row must land active with role="member" so the UI
// sees them immediately.
func TestGroupMembers_UpsertPreservingRole_InsertsNewRowWithMember(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMemberPreservingRole(GroupMemberRecord{
		GroupID:     "g-fresh",
		PeerID:      "peer-new",
		DisplayName: "New Peer",
		Role:        "member",
		Status:      GroupMemberStatusActive,
		Source:      "heartbeat",
	}); err != nil {
		t.Fatalf("UpsertGroupMemberPreservingRole (fresh insert): %v", err)
	}
	rows, _ := d.ListGroupMembers("g-fresh", GroupMemberStatusActive)
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1: %+v", len(rows), rows)
	}
	if rows[0].Role != "member" {
		t.Fatalf("fresh insert role=%q want %q", rows[0].Role, "member")
	}
	if rows[0].DisplayName != "New Peer" {
		t.Fatalf("display name=%q want %q", rows[0].DisplayName, "New Peer")
	}
}

// TestGroupMembers_UpsertOverwritesRole confirms the authoritative path
// (plain UpsertGroupMember) DOES update role when the caller knows it.
// This is the path CreateGroupChat uses; it must work even if a prior
// roster-sync placeholder row exists (e.g. the creator briefly appeared
// in a heartbeat upstream of their own CreateGroupChat call due to clock
// skew or recovery).
func TestGroupMembers_UpsertOverwritesRole(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMemberPreservingRole(GroupMemberRecord{
		GroupID:     "g-roleflip",
		PeerID:      "peer-roleflip",
		DisplayName: "X",
		Role:        "member",
		Status:      GroupMemberStatusActive,
		Source:      "heartbeat",
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     "g-roleflip",
		PeerID:      "peer-roleflip",
		DisplayName: "X",
		Role:        "creator",
		Status:      GroupMemberStatusActive,
		Source:      "create",
	}); err != nil {
		t.Fatal(err)
	}
	rows, _ := d.ListGroupMembers("g-roleflip", GroupMemberStatusActive)
	if len(rows) != 1 || rows[0].Role != "creator" {
		t.Fatalf("authoritative upsert failed to set role: %+v", rows)
	}
}

func TestGroupMembers_SetGroupMemberRole_GrantAndRevokeAdmin(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID: "g-role", PeerID: "peer-a", Role: GroupMemberRoleMember,
		Status: GroupMemberStatusActive, Source: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetGroupMemberRole("g-role", "peer-a", GroupMemberRoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	admins, err := d.ListGroupAdmins("g-role")
	if err != nil {
		t.Fatalf("ListGroupAdmins: %v", err)
	}
	if len(admins) != 1 || admins[0].PeerID != "peer-a" || admins[0].Role != GroupMemberRoleAdmin {
		t.Fatalf("admins mismatch after grant: %+v", admins)
	}
	if err := d.SetGroupMemberRole("g-role", "peer-a", GroupMemberRoleMember); err != nil {
		t.Fatalf("revoke admin: %v", err)
	}
	admins, err = d.ListGroupAdmins("g-role")
	if err != nil {
		t.Fatalf("ListGroupAdmins after revoke: %v", err)
	}
	if len(admins) != 0 {
		t.Fatalf("admins after revoke = %+v, want none", admins)
	}
}

func TestGroupMembers_SetGroupMemberRole_CreatorImmutable(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID: "g-creator-immutable", PeerID: "peer-creator", Role: GroupMemberRoleCreator,
		Status: GroupMemberStatusActive, Source: "create",
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetGroupMemberRole("g-creator-immutable", "peer-creator", GroupMemberRoleMember); err == nil {
		t.Fatal("expected creator demotion to fail")
	}
	if err := d.SetGroupMemberRole("g-creator-immutable", "peer-creator", GroupMemberRoleAdmin); err == nil {
		t.Fatal("expected creator admin overwrite to fail")
	}
	rows, err := d.ListGroupMembers("g-creator-immutable", GroupMemberStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Role != GroupMemberRoleCreator {
		t.Fatalf("creator role changed: %+v", rows)
	}
}

func TestGroupMembers_ListAuthorizedCommitters_ActiveCreatorAndAdminsOnly(t *testing.T) {
	d := setupTestDB(t)
	seed := []GroupMemberRecord{
		{GroupID: "g-authz", PeerID: "creator", Role: GroupMemberRoleCreator, Status: GroupMemberStatusActive},
		{GroupID: "g-authz", PeerID: "admin", Role: GroupMemberRoleAdmin, Status: GroupMemberStatusActive},
		{GroupID: "g-authz", PeerID: "left-admin", Role: GroupMemberRoleAdmin, Status: GroupMemberStatusLeft, LeftAt: 1},
		{GroupID: "g-authz", PeerID: "member", Role: GroupMemberRoleMember, Status: GroupMemberStatusActive},
	}
	for _, rec := range seed {
		if err := d.UpsertGroupMember(rec); err != nil {
			t.Fatalf("seed %s: %v", rec.PeerID, err)
		}
	}
	rows, err := d.ListAuthorizedCommitters("g-authz")
	if err != nil {
		t.Fatalf("ListAuthorizedCommitters: %v", err)
	}
	got := map[string]string{}
	for _, row := range rows {
		got[row.PeerID] = row.Role
	}
	if len(got) != 2 || got["creator"] != GroupMemberRoleCreator || got["admin"] != GroupMemberRoleAdmin {
		t.Fatalf("authorized committers mismatch: %+v", rows)
	}
}

func TestGroupMembers_UpdateDisplayNameByPeer(t *testing.T) {
	d := setupTestDB(t)
	if err := d.UpsertGroupMember(GroupMemberRecord{
		GroupID:     "g-2",
		PeerID:      "peer-b",
		DisplayName: "",
		Role:        "member",
		Status:      GroupMemberStatusActive,
		Source:      "invite",
	}); err != nil {
		t.Fatalf("UpsertGroupMember: %v", err)
	}
	if err := d.UpdateGroupMemberDisplayNameByPeer("peer-b", "Bob"); err != nil {
		t.Fatalf("UpdateGroupMemberDisplayNameByPeer: %v", err)
	}
	rows, err := d.ListGroupMembers("g-2", GroupMemberStatusActive)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	if len(rows) != 1 || rows[0].DisplayName != "Bob" {
		t.Fatalf("display name update mismatch: %+v", rows)
	}
}
