package service

import (
	"app/coordination"
	"app/domain"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

type NotificationDTO struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	GroupID     string `json:"group_id"`
	GroupName   string `json:"group_name"`
	ActorID     string `json:"actor_id"`
	ActorName   string `json:"actor_name"`
	TargetID    string `json:"target_id"`
	Content     string `json:"content"`
	IsRead      bool   `json:"is_read"`
	CreatedAt   int64  `json:"created_at"`
}

func (r *Runtime) processNotificationsForMessage(msg *coordination.StoredMessage) {
	var localID peer.ID
	r.mu.RLock()
	if r.node != nil {
		localID = r.node.Host.ID()
	}
	r.mu.RUnlock()

	if localID == "" || msg.SenderID == localID {
		return
	}

	// 1. Mentions & Replies detection
	content := string(msg.Content)
	isMentioned := false
	isReply := false

	// Try to parse as JSON first (Post/Comment/Channel payload)
	var payload struct {
		Type     string `json:"type"`
		Mentions []struct {
			UserID string `json:"user_id"`
		} `json:"mentions"`
		PostID         string `json:"post_id"`
		ParentID       string `json:"parent_id"`
		ReplyToComment string `json:"reply_to_comment_id"`
	}

	if err := json.Unmarshal(msg.Content, &payload); err == nil {
		// Extract cleaner content for notification preview if it's a known JSON type
		if payload.Type == "post" || payload.Type == "comment" || payload.Type == "reply" {
			var raw map[string]interface{}
			_ = json.Unmarshal(msg.Content, &raw)
			if b, ok := raw["body"].(string); ok && b != "" {
				content = b
			} else if c, ok := raw["content"].(string); ok && c != "" {
				content = c
			}
		}

		// Check mentions array
		for _, m := range payload.Mentions {
			if m.UserID == localID.String() {
				isMentioned = true
				break
			}
		}

		// Check reply
		parentID := payload.PostID
		if payload.ParentID != "" {
			parentID = payload.ParentID
		}
		if payload.ReplyToComment != "" {
			parentID = payload.ReplyToComment
		}

		if parentID != "" && r.db != nil {
			// Query if parent message was sent by me
			if sender, err := r.db.GetMessageSender(parentID); err == nil && sender == localID.String() {
				isReply = true
			}
		}
	} else {
		// Legacy / Plaintext mention detection
		// We need our display name to detect @Name mentions
		r.mu.RLock()
		db := r.db
		r.mu.RUnlock()
		if db != nil {
			if identity, err := db.GetMLSIdentity(); err == nil && identity.DisplayName != "" {
				mentionTag := "@" + identity.DisplayName
				if strings.Contains(content, mentionTag) {
					isMentioned = true
				}
			}
		}
	}

	if isMentioned {
		r.insertNotification(domain.NotificationTypeMention, msg.GroupID, msg.SenderID.String(), msg.MessageID, content)
	} else if isReply {
		r.insertNotification(domain.NotificationTypeReply, msg.GroupID, msg.SenderID.String(), msg.MessageID, content)
	}
}

func (r *Runtime) insertNotification(ntype, groupID, actorID, targetID, content string) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return
	}

	// Generate deterministic ID for idempotency
	// If targetID is a 64-char hex (message hash), it's already globally unique for that message.
	// Otherwise (like groupID), we add the day to the seed to allow re-notifications after some time,
	// or just use the targetID if it's meant to be unique (like invite request ID).
	idSeed := fmt.Sprintf("%s-%s-%s-%s", ntype, groupID, actorID, targetID)
	
	// If targetID is empty or short, it's not a message hash. 
	// To avoid collisions between different mentions by the same person in the same group 
	// (if MessageID was somehow missing), we can't just add time because then we lose dedup.
	// But MessageID should NOT be missing now.
	
	hash := sha256.Sum256([]byte(idSeed))
	id := hex.EncodeToString(hash[:16]) // 32 chars hex

	n := &domain.Notification{
		ID:          id,
		Type:        ntype,
		GroupID:     groupID,
		ActorPeerID: actorID,
		TargetID:    targetID,
		Content:     content,
		IsRead:      false,
		CreatedAt:   time.Now(),
	}

	if err := db.InsertNotification(n); err != nil {
		// Likely duplicate, ignore
		return
	}

	slog.Info("Notification generated", "type", ntype, "group", groupID, "id", id)
	
	actorName := actorID
	if db != nil {
		if name, _ := db.GetPeerDisplayName(actorID); name != "" {
			actorName = name
		}
	}

	r.emit("notification:new", map[string]interface{}{
		"id":         id,
		"type":       ntype,
		"group_id":   groupID,
		"actor_id":   actorID,
		"actor_name": actorName,
		"content":    contentPreview(content),
	})
}

// ─── Wails Bindings ──────────────────────────────────────────────────────────

func (r *Runtime) GetNotifications(limit, offset int) ([]NotificationDTO, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.ListNotifications(limit, offset)
	if err != nil {
		return nil, err
	}

	out := make([]NotificationDTO, len(rows))
	for i, n := range rows {
		groupName := ""
		if r.coordStorage != nil {
			if g, err := r.coordStorage.GetGroupRecord(n.GroupID); err == nil && g != nil {
				groupName = g.GroupID
			}
		}
		if groupName == "" {
			groupName = n.GroupID
		}

		actorName := ""
		if name, _ := db.GetPeerDisplayName(n.ActorPeerID); name != "" {
			actorName = name
		}
		if actorName == "" {
			actorName = n.ActorPeerID
		}

		out[i] = NotificationDTO{
			ID:          n.ID,
			Type:        n.Type,
			GroupID:     n.GroupID,
			GroupName:   groupName,
			ActorID:     n.ActorPeerID,
			ActorName:   actorName,
			TargetID:    n.TargetID,
			Content:     contentPreview(n.Content),
			IsRead:      n.IsRead,
			CreatedAt:   n.CreatedAt.UnixMilli(),
		}
	}
	return out, nil
}

func contentPreview(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// If it's still JSON, something went wrong with extraction during insert, try to fix it here
	if strings.HasPrefix(s, "{") {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(s), &data); err == nil {
			if b, ok := data["body"].(string); ok {
				s = b
			} else if c, ok := data["content"].(string); ok {
				s = c
			}
		}
	}
	if len(s) > 100 {
		return s[:97] + "..."
	}
	return s
}

func (r *Runtime) GetUnreadNotificationCount() (int, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	return db.GetUnreadNotificationCount()
}

func (r *Runtime) MarkNotificationRead(id string) error {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	return db.MarkNotificationRead(id)
}

func (r *Runtime) MarkAllNotificationsRead() error {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	return db.MarkAllNotificationsRead()
}
