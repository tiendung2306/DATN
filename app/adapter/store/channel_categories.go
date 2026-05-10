package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ChannelCategoryRecord struct {
	CategoryID string
	Name       string
	SortOrder  int
	CreatedBy  string
	CreatedAt  int64
	UpdatedAt  int64
}

func (d *Database) UpsertChannelCategory(rec ChannelCategoryRecord) error {
	rec.CategoryID = strings.TrimSpace(rec.CategoryID)
	rec.Name = strings.TrimSpace(rec.Name)
	if rec.CategoryID == "" {
		return fmt.Errorf("category ID is required")
	}
	if rec.Name == "" {
		return fmt.Errorf("category name is required")
	}
	now := time.Now().Unix()
	if rec.CreatedAt <= 0 {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt <= 0 {
		rec.UpdatedAt = now
	}
	_, err := d.Conn.Exec(
		`INSERT INTO channel_categories (category_id, name, sort_order, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(category_id) DO UPDATE SET
		    name = excluded.name,
		    sort_order = excluded.sort_order,
		    created_by = CASE WHEN channel_categories.created_by = '' THEN excluded.created_by ELSE channel_categories.created_by END,
		    updated_at = excluded.updated_at`,
		rec.CategoryID, rec.Name, rec.SortOrder, rec.CreatedBy, rec.CreatedAt, rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("UpsertChannelCategory(%q): %w", rec.CategoryID, err)
	}
	return nil
}

func (d *Database) GetChannelCategory(categoryID string) (*ChannelCategoryRecord, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, sql.ErrNoRows
	}
	var out ChannelCategoryRecord
	err := d.Conn.QueryRow(
		`SELECT category_id, name, sort_order, created_by, created_at, updated_at
		 FROM channel_categories
		 WHERE category_id = ?`,
		categoryID,
	).Scan(&out.CategoryID, &out.Name, &out.SortOrder, &out.CreatedBy, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (d *Database) ListChannelCategories() ([]ChannelCategoryRecord, error) {
	rows, err := d.Conn.Query(
		`SELECT category_id, name, sort_order, created_by, created_at, updated_at
		 FROM channel_categories
		 ORDER BY sort_order ASC, created_at ASC, category_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListChannelCategories: %w", err)
	}
	defer rows.Close()
	out := make([]ChannelCategoryRecord, 0)
	for rows.Next() {
		var rec ChannelCategoryRecord
		if err := rows.Scan(&rec.CategoryID, &rec.Name, &rec.SortOrder, &rec.CreatedBy, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, fmt.Errorf("ListChannelCategories scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListChannelCategories rows: %w", err)
	}
	return out, nil
}

func (d *Database) DeleteChannelCategory(categoryID string) error {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return fmt.Errorf("category ID is required")
	}
	_, err := d.Conn.Exec(`DELETE FROM channel_categories WHERE category_id = ?`, categoryID)
	if err != nil {
		return fmt.Errorf("DeleteChannelCategory(%q): %w", categoryID, err)
	}
	return nil
}

func (d *Database) CountActiveChannelsInCategory(categoryID string) (int, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return 0, nil
	}
	var count int
	err := d.Conn.QueryRow(
		`SELECT COUNT(*) FROM mls_groups
		 WHERE lifecycle_status = 'active'
		   AND lower(group_type) = 'channel'
		   AND category_id = ?`,
		categoryID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountActiveChannelsInCategory(%q): %w", categoryID, err)
	}
	return count, nil
}

func (d *Database) AssignCategoryToGroup(groupID, categoryID string) error {
	groupID = strings.TrimSpace(groupID)
	categoryID = strings.TrimSpace(categoryID)
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}
	res, err := d.Conn.Exec(
		`UPDATE mls_groups
		 SET category_id = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE group_id = ?`,
		categoryID, groupID,
	)
	if err != nil {
		return fmt.Errorf("AssignCategoryToGroup(%q,%q): %w", groupID, categoryID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("AssignCategoryToGroup(%q,%q) rows: %w", groupID, categoryID, err)
	}
	if n == 0 {
		return fmt.Errorf("AssignCategoryToGroup(%q,%q): no mls_groups row (create/join group first)", groupID, categoryID)
	}
	return nil
}

func (d *Database) ListActiveChannelsWithoutCategory() ([]string, error) {
	rows, err := d.Conn.Query(
		`SELECT group_id FROM mls_groups
		 WHERE lifecycle_status = 'active'
		   AND lower(group_type) = 'channel'
		   AND trim(category_id) = ''`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListActiveChannelsWithoutCategory: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, fmt.Errorf("ListActiveChannelsWithoutCategory scan: %w", err)
		}
		out = append(out, groupID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListActiveChannelsWithoutCategory rows: %w", err)
	}
	return out, nil
}

func (d *Database) ListChannelAssignments() ([]ChannelAssignmentRecord, error) {
	rows, err := d.Conn.Query(
		`SELECT group_id, category_id FROM mls_groups
		 WHERE lifecycle_status = 'active'
		   AND lower(group_type) = 'channel'
		   AND trim(category_id) <> ''`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListChannelAssignments: %w", err)
	}
	defer rows.Close()
	out := make([]ChannelAssignmentRecord, 0)
	for rows.Next() {
		var rec ChannelAssignmentRecord
		if err := rows.Scan(&rec.ChannelID, &rec.CategoryID); err != nil {
			return nil, fmt.Errorf("ListChannelAssignments scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListChannelAssignments rows: %w", err)
	}
	return out, nil
}

type ChannelAssignmentRecord struct {
	ChannelID  string
	CategoryID string
}

func (d *Database) MarkCategorySyncEventApplied(eventID string) (alreadyApplied bool, err error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return false, fmt.Errorf("event ID is required")
	}
	res, err := d.Conn.Exec(
		`INSERT OR IGNORE INTO channel_category_sync_events (event_id, applied_at) VALUES (?, ?)`,
		eventID, time.Now().Unix(),
	)
	if err != nil {
		return false, fmt.Errorf("MarkCategorySyncEventApplied(%q): %w", eventID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("MarkCategorySyncEventApplied rows affected: %w", err)
	}
	return affected == 0, nil
}
