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

// GetGroupMessages returns all stored messages for a group, sorted by HLC.
func (r *Runtime) GetGroupMessages(groupID string) ([]MessageInfo, error) {
	if r.coordStorage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	msgs, err := r.coordStorage.GetMessagesSince(groupID, coordination.HLCTimestamp{})
	if err != nil {
		return nil, err
	}

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
		}
	}
	return result, nil
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
