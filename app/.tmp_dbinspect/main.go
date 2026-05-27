package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		panic("usage: dbinspect <db>...")
	}
	for _, path := range os.Args[1:] {
		inspect(path)
	}
}

func inspect(path string) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		fmt.Printf("open %s: %v\n", path, err)
		return
	}
	defer db.Close()
	fmt.Printf("\n=== %s ===\n", path)
	queryRows(db, `SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	for _, t := range queryStrings(db, `SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`) {
		if strings.Contains(t, "welcome") || strings.Contains(t, "invite") || strings.Contains(t, "blind") || strings.Contains(t, "offline") || strings.Contains(t, "message") || strings.Contains(t, "envelope") || strings.Contains(t, "mls") || strings.Contains(t, "group") {
			fmt.Printf("\nSCHEMA %s\n", t)
			queryRows(db, `PRAGMA table_info(`+t+`)`)
		}
	}
	groupIDs := queryStrings(db, `SELECT group_id FROM mls_groups WHERE group_id LIKE '%Nhóm chat 1%' ORDER BY updated_at DESC, created_at DESC LIMIT 20`)
	if len(groupIDs) == 0 {
		groupIDs = []string{"Nhóm chat 1"}
	}
	for _, gid := range groupIDs {
		fmt.Printf("\n-- group %q --\n", gid)
		queryRows(db, `SELECT group_id, group_type, epoch, my_role, group_creator_peer_id, lifecycle_status, left_at, length(group_state), updated_at FROM mls_groups WHERE group_id = ?`, gid)
		queryRows(db, `SELECT peer_id, display_name, role, status, source, joined_at, left_at, updated_at FROM group_members WHERE group_id = ? ORDER BY peer_id`, gid)
		queryRows(db, `SELECT id, target_peer_id, group_id, length(welcome_bytes), delivered, created_at FROM pending_welcomes_out WHERE group_id = ? ORDER BY id`, gid)
		queryRows(db, `SELECT id, group_id, group_type, inviter_peer_id, source_peer_id, status, length(welcome_bytes), received_at, updated_at FROM pending_invites WHERE group_id = ? ORDER BY received_at`, gid)
		queryRows(db, `SELECT invitee_peer_id, group_id, group_type, source_peer_id, length(welcome_bytes), created_at FROM stored_welcomes WHERE group_id = ? ORDER BY created_at`, gid)
		queryRows(db, `SELECT operation_id, group_id, target_peer_id, status, commit_epoch, welcome_hash, updated_at FROM group_add_operations WHERE group_id = ? ORDER BY updated_at`, gid)
		queryRows(db, `SELECT id, group_id, sender_id, hex(substr(content, 1, 10)) as content_hex, hlc_wall_time_ms, length(envelope_hash) FROM stored_messages WHERE group_id = ? ORDER BY hlc_wall_time_ms`, gid)
		queryRows(db, `SELECT seq, group_id, msg_type, epoch, length(envelope), hlc_wall_ms, created_at FROM envelope_log WHERE group_id = ? ORDER BY seq`, gid)
		queryRows(db, `SELECT object_id, record_type, target_peer_id, group_id, length(payload), created_at FROM replicated_records WHERE group_id = ? OR target_peer_id IN (SELECT peer_id FROM group_members WHERE group_id = ?) ORDER BY created_at`, gid, gid)
		queryRows(db, `SELECT group_id, remote_peer_id, last_remote_seq, updated_at FROM offline_sync_pull_state WHERE group_id = ? ORDER BY remote_peer_id`, gid)
	}
}

func queryStrings(db *sql.DB, q string, args ...any) []string {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			out = append(out, s)
		}
	}
	return out
}

func queryRows(db *sql.DB, q string, args ...any) {
	rows, err := db.Query(q, args...)
	if err != nil {
		fmt.Printf("query err: %v\n", err)
		return
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	fmt.Println(strings.Join(cols, " | "))
	count := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Printf("scan err: %v\n", err)
			continue
		}
		parts := make([]string, len(cols))
		for i, v := range vals {
			switch x := v.(type) {
			case nil:
				parts[i] = "NULL"
			case []byte:
				parts[i] = string(x)
			default:
				parts[i] = fmt.Sprint(x)
			}
		}
		fmt.Println(strings.Join(parts, " | "))
		count++
	}
	if count == 0 {
		fmt.Println("(none)")
	}
}
