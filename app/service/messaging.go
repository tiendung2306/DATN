package service

import (
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
	for i, m := range msgs {
		result[i] = MessageInfo{
			GroupID:   m.GroupID,
			Sender:    m.SenderID.String(),
			Content:   string(m.Content),
			Timestamp: m.Timestamp.WallTimeMs,
			IsMine:    m.SenderID == localID,
		}
	}
	return result, nil
}
