package service

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

// SendGroupMessage encrypts and broadcasts a text message to the group.
func (r *Runtime) SendGroupMessage(groupID string, text string) error {
	return r.SendGroupMessageWithLocalEchoToken(groupID, text, "")
}

// SendGroupMessageWithLocalEchoToken encrypts and broadcasts a text message to
// the group while tagging the locally emitted UI event with a correlation token
// used to replace optimistic frontend placeholders deterministically.
func (r *Runtime) SendGroupMessageWithLocalEchoToken(groupID string, text string, localEchoToken string) error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}

	slog.Info("Sending group message", "group", groupID, "len", len(text))

	if strings.TrimSpace(groupID) == "" {
		return fmt.Errorf("group ID is required")
	}
	if r.coordStorage == nil {
		return fmt.Errorf("group metadata storage not initialized")
	}
	rec, recErr := r.coordStorage.GetGroupRecord(groupID)
	if recErr != nil {
		return fmt.Errorf("group metadata unavailable: %w", recErr)
	}
	if err := validateOutboundByGroupType(rec.GroupType, text); err != nil {
		return err
	}

	r.mu.Lock()
	coord, ok := r.coordinators[groupID]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("not in group %q", groupID)
	}

	var err error
	if strings.TrimSpace(localEchoToken) != "" {
		_, err = coord.SendMessageWithLocalEchoToken([]byte(text), strings.TrimSpace(localEchoToken))
	} else {
		_, err = coord.SendMessage([]byte(text))
	}
	return err
}

func validateOutboundByGroupType(groupType string, text string) error {
	switch strings.TrimSpace(strings.ToLower(groupType)) {
	case "channel":
		return validateChannelOutboundMessage(text)
	case "group":
		return validateDMOutboundMessage(text)
	case "dm":
		return validateDMOutboundMessage(text)
	default:
		return fmt.Errorf("ERR_GROUP_TYPE_INVALID: unsupported group type %q", groupType)
	}
}

func (r *Runtime) mapStoredMessagesToMessageInfo(msgs []*coordination.StoredMessage) []MessageInfo {
	var localID peer.ID
	r.mu.RLock()
	if r.node != nil {
		localID = r.node.Host.ID()
	}
	r.mu.RUnlock()

	result := make([]MessageInfo, len(msgs))
	replayLinks := r.resolveReplaySupersedesMessageIDs(msgs)
	var localName string
	if r.db != nil {
		if identity, err := r.db.GetMLSIdentity(); err == nil {
			localName = identity.DisplayName
		}
	}

	for i, m := range msgs {
		result[i] = r.messageInfoFromStoredMessage(m, localID, localName, replayLinks)
	}
	return result
}

func (r *Runtime) resolveReplaySupersedesMessageIDs(msgs []*coordination.StoredMessage) map[string]string {
	if len(msgs) == 0 {
		return nil
	}
	groupID := strings.TrimSpace(msgs[0].GroupID)
	if groupID == "" {
		return nil
	}
	ids := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		if msg == nil || strings.TrimSpace(msg.MessageID) == "" {
			continue
		}
		ids = append(ids, msg.MessageID)
	}
	if len(ids) == 0 {
		return nil
	}

	r.mu.RLock()
	cs := r.coordStorage
	r.mu.RUnlock()
	if cs == nil {
		return nil
	}

	links, err := cs.ResolveReplayCanonicalOriginalMessageIDs(groupID, ids)
	if err != nil {
		slog.Warn("resolve replay canonical originals failed", "group_id", groupID, "messages", len(ids), "err", err)
		return nil
	}
	return links
}

func (r *Runtime) messageInfoFromStoredMessage(m *coordination.StoredMessage, localID peer.ID, localName string, replayLinks map[string]string) MessageInfo {
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

	info := MessageInfo{
		MessageID:         m.MessageID,
		GroupID:           m.GroupID,
		Sender:            m.SenderID.String(),
		SenderDisplayName: senderName,
		Content:           string(m.Content),
		Timestamp:         m.Timestamp.WallTimeMs,
		IsMine:            m.SenderID == localID,
		Status:            "published",
		CommentCount:      m.CommentCount,
		LocalEchoToken:    strings.TrimSpace(m.LocalEchoToken),
		ReplayedAt:        m.ReplayedAt,
	}
	if replayLinks != nil {
		info.SupersedesMessageID = strings.TrimSpace(replayLinks[m.MessageID])
	}
	return info
}

func (r *Runtime) messageInfoPayload(info MessageInfo) map[string]interface{} {
	payload := map[string]interface{}{
		"message_id":          info.MessageID,
		"group_id":            info.GroupID,
		"sender":              info.Sender,
		"sender_display_name": info.SenderDisplayName,
		"content":             info.Content,
		"timestamp":           info.Timestamp,
		"is_mine":             info.IsMine,
		"status":              info.Status,
		"comment_count":       info.CommentCount,
	}
	if info.LocalEchoToken != "" {
		payload["local_echo_token"] = info.LocalEchoToken
	}
	if info.ReplayedAt != nil {
		payload["replayed_at"] = *info.ReplayedAt
	}
	if info.SupersedesMessageID != "" {
		payload["supersedes_message_id"] = info.SupersedesMessageID
	}
	return payload
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
