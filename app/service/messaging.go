package service

import (
	"database/sql"
	"fmt"
	"log/slog"

	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

// SendGroupMessage encrypts and broadcasts a text message to the group.
func (r *Runtime) SendGroupMessage(groupID string, text string) error {
	if text == "" {
		return nil
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}

	slog.Info("Sending group message", "group", groupID, "len", len(text))

	r.mu.Lock()
	coord, ok := r.coordinators[groupID]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("not in group %q", groupID)
	}
	if r.coordStorage != nil {
		if rec, recErr := r.coordStorage.GetGroupRecord(groupID); recErr == nil {
			if rec.GroupType == "channel" {
				if err := validateChannelOutboundMessage(text); err != nil {
					return err
				}
			}
		}
	}

	_, err := coord.SendMessage([]byte(text))
	return err
}

func (r *Runtime) mapStoredMessagesToMessageInfo(msgs []*coordination.StoredMessage) []MessageInfo {
	var localID peer.ID
	r.mu.Lock()
	if r.node != nil {
		localID = r.node.Host.ID()
	}
	r.mu.Unlock()

	result := make([]MessageInfo, len(msgs))
	var localName string
	if r.db != nil {
		if identity, err := r.db.GetMLSIdentity(); err == nil {
			localName = identity.DisplayName
		}
	}

	for i, m := range msgs {
		senderName := ""
		if m.SenderID == localID {
			senderName = localName
		}
		if senderName == "" && r.node != nil && r.node.AuthProtocol != nil {
			if tok := r.node.AuthProtocol.GetVerifiedToken(m.SenderID); tok != nil {
				senderName = tok.DisplayName
			}
		}
		if senderName == "" && r.db != nil {
			if name, _ := r.db.GetPeerDisplayName(m.SenderID.String()); name != "" {
				senderName = name
			}
		}

		result[i] = MessageInfo{
			MessageID:         m.MessageID,
			GroupID:           m.GroupID,
			Sender:            m.SenderID.String(),
			SenderDisplayName: senderName,
			Content:           string(m.Content),
			Timestamp:         m.Timestamp.WallTimeMs,
			IsMine:            m.SenderID == localID,
			Status:            "published",
			CommentCount:      m.CommentCount,
		}
	}
	return result
}

// GetGroupMessages returns stored messages for a group. If limit > 0, it uses pagination.
func (r *Runtime) GetGroupMessages(groupID string, limit, offset int) ([]MessageInfo, error) {
	if r.coordStorage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
 
	var msgs []*coordination.StoredMessage
	var err error
	if limit > 0 {
		msgs, err = r.coordStorage.GetMessagesPaginated(groupID, limit, offset)
	} else {
		msgs, err = r.coordStorage.GetMessagesSince(groupID, coordination.HLCTimestamp{})
	}
	if err != nil {
		return nil, err
	}
	return r.mapStoredMessagesToMessageInfo(msgs), nil
}

// GetGroupPosts returns paginated 'post' messages for a group.
func (r *Runtime) GetGroupPosts(groupID string, limit, offset int) ([]MessageInfo, error) {
	if r.coordStorage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	msgs, err := r.coordStorage.GetPostsPaginated(groupID, limit, offset)
	if err != nil {
		return nil, err
	}
	return r.mapStoredMessagesToMessageInfo(msgs), nil
}

// GetPostComments returns paginated 'comment'/'reply' messages for a specific post.
func (r *Runtime) GetPostComments(groupID, postID string, limit, offset int) ([]MessageInfo, error) {
	if r.coordStorage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	msgs, err := r.coordStorage.GetCommentsPaginated(groupID, postID, limit, offset)
	if err != nil {
		return nil, err
	}
	return r.mapStoredMessagesToMessageInfo(msgs), nil
}

// RetryMessage re-sends an existing persisted message by ID.
func (r *Runtime) RetryMessage(groupID string, messageID string) error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	if groupID == "" || messageID == "" {
		return fmt.Errorf("group ID and message ID are required")
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	row, err := db.GetStoredMessageByID(groupID, messageID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("message not found")
		}
		return fmt.Errorf("invalid retry target: %w", err)
	}
	return r.SendGroupMessage(groupID, row.Content)
}

// DeleteLocalMessage removes one locally persisted message row.
func (r *Runtime) DeleteLocalMessage(groupID string, messageID string) error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	if groupID == "" || messageID == "" {
		return fmt.Errorf("group ID and message ID are required")
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	if err := db.DeleteStoredMessageByID(groupID, messageID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("message not found")
		}
		return fmt.Errorf("invalid delete target: %w", err)
	}
	return nil
}
