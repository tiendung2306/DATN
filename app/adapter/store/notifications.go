package store

import (
	"app/domain"
	"fmt"
	"time"
)

func (d *Database) InsertNotification(n *domain.Notification) error {
	if n.ID == "" {
		return fmt.Errorf("notification ID is required")
	}
	query := `INSERT INTO notifications (id, type, group_id, actor_peer_id, target_id, content, is_read, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	createdAt := n.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := d.Conn.Exec(query, n.ID, n.Type, n.GroupID, n.ActorPeerID, n.TargetID, n.Content, n.IsRead, createdAt)
	return err
}

func (d *Database) ListNotifications(limit, offset int) ([]*domain.Notification, error) {
	query := `SELECT id, type, group_id, actor_peer_id, target_id, content, is_read, created_at 
			  FROM notifications 
			  ORDER BY created_at DESC 
			  LIMIT ? OFFSET ?`
	rows, err := d.Conn.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Notification
	for rows.Next() {
		n := &domain.Notification{}
		if err := rows.Scan(&n.ID, &n.Type, &n.GroupID, &n.ActorPeerID, &n.TargetID, &n.Content, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func (d *Database) MarkNotificationRead(id string) error {
	query := `UPDATE notifications SET is_read = 1 WHERE id = ?`
	_, err := d.Conn.Exec(query, id)
	return err
}

func (d *Database) MarkAllNotificationsRead() error {
	query := `UPDATE notifications SET is_read = 1`
	_, err := d.Conn.Exec(query)
	return err
}

func (d *Database) GetUnreadNotificationCount() (int, error) {
	query := `SELECT COUNT(*) FROM notifications WHERE is_read = 0`
	var count int
	err := d.Conn.QueryRow(query).Scan(&count)
	return count, err
}

func (d *Database) GetMessageSender(messageID string) (string, error) {
	query := `SELECT sender_id FROM messages WHERE id = (SELECT id FROM messages WHERE message_id = ? OR id = ? LIMIT 1)`
	// The messages table uses 'id' (int) and coordination messages might use 'message_id' (text).
	// But stored_messages actually uses 'message_id'. Let's check stored_messages table.
	query = `SELECT sender_id FROM stored_messages WHERE message_id = ? LIMIT 1`
	var senderID string
	err := d.Conn.QueryRow(query, messageID).Scan(&senderID)
	return senderID, err
}

func (d *Database) DeleteOldNotifications(days int) error {
	query := `DELETE FROM notifications WHERE created_at < datetime('now', ?)`
	arg := fmt.Sprintf("-%d days", days)
	_, err := d.Conn.Exec(query, arg)
	return err
}
