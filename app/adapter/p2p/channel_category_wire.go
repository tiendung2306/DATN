package p2p

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/protocol"
)

const ChannelCategorySyncProtocol protocol.ID = "/app/channel-categories/1.0.0"

const channelCategoryMaxFrame = 4 << 20 // 4 MiB

type ChannelCategoryWire struct {
	CategoryID string `json:"category_id"`
	Name       string `json:"name"`
	SortOrder  int    `json:"sort_order"`
	UpdatedAt  int64  `json:"updated_at"`
	CreatedBy  string `json:"created_by"`
}

type ChannelAssignmentWire struct {
	ChannelID  string `json:"channel_id"`
	CategoryID string `json:"category_id"`
}

type ChannelCategorySyncFrameV1 struct {
	V int `json:"v"`

	// request_snapshot | snapshot | upsert_category | delete_category | assign_channel
	Type string `json:"type"`

	EventID string `json:"event_id,omitempty"`

	Category   *ChannelCategoryWire    `json:"category,omitempty"`
	CategoryID string                  `json:"category_id,omitempty"`
	ChannelID  string                  `json:"channel_id,omitempty"`
	Categories []ChannelCategoryWire   `json:"categories,omitempty"`
	Assignments []ChannelAssignmentWire `json:"assignments,omitempty"`
}

func WriteChannelCategoryJSONFrame(w io.Writer, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > channelCategoryMaxFrame {
		return fmt.Errorf("invalid frame size: %d", len(payload))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func ReadChannelCategoryJSONFrame(r io.Reader, out any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > channelCategoryMaxFrame {
		return fmt.Errorf("invalid frame size: %d", n)
	}
	buf := make([]byte, int(n))
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, out)
}
