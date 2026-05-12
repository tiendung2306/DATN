package p2p

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/protocol"
)

// GroupInviteRequestProtocol multiplexes submit/sync/cancel (member→creator) and
// push updates (creator→member) for group invite request rows.
const GroupInviteRequestProtocol = protocol.ID("/app/group-invite-request/1.0.0")

const groupInviteRequestMaxFrame = 512 << 10 // 512 KiB

// InviteRequestRecordWire maps to store.GroupInviteRequestRecord for JSON transport.
type InviteRequestRecordWire struct {
	RequestID           string  `json:"request_id"`
	GroupID             string  `json:"group_id"`
	RequesterPeerID     string  `json:"requester_peer_id"`
	TargetPeerID        string  `json:"target_peer_id"`
	Status              string  `json:"status"`
	FailureCode         string  `json:"failure_code,omitempty"`
	FailureMessage      string  `json:"failure_message,omitempty"`
	RejectionReason     string  `json:"rejection_reason,omitempty"`
	AttemptCount        int     `json:"attempt_count"`
	MaxAttempts         int     `json:"max_attempts"`
	ProcessingStartedAt *int64  `json:"processing_started_at,omitempty"`
	ExpiresAt           int64   `json:"expires_at"`
	CreatedAt           int64   `json:"created_at"`
	UpdatedAt           int64   `json:"updated_at"`
	IsMirror            bool    `json:"is_mirror,omitempty"`
}

// GroupInviteWireClientReqV1 is the member→creator frame (submit | sync | cancel).
//
// TargetKeyPackage carries the requester's freshly-fetched KeyPackage for the
// target peer on a "submit" op. The requester is — by topology — the node
// most likely to have a verified connection with the target (it is the one
// triggering the invite flow), while the creator may have never met the
// target. Attaching the KP avoids the creator having to re-run discovery,
// which is the bug class behind ERR_INVITE_ADD_MEMBER_FAILED when creator
// has no live link to the target. Optional for backward compatibility:
// when empty, creator falls back to its own fetchPeerKeyPackage path.
type GroupInviteWireClientReqV1 struct {
	V                int    `json:"v"`
	Op               string `json:"op"`
	GroupID          string `json:"group_id,omitempty"`
	TargetPeerID     string `json:"target_peer_id,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
	TargetKeyPackage []byte `json:"target_key_package,omitempty"`
}

// GroupInviteWireRespV1 is creator→member response or ack.
type GroupInviteWireRespV1 struct {
	V      int                    `json:"v"`
	OK     bool                   `json:"ok"`
	Error  string                 `json:"error,omitempty"`
	Record *InviteRequestRecordWire `json:"record,omitempty"`
}

// GroupInviteWirePushV1 is creator→member push (same protocol, inbound on requester).
type GroupInviteWirePushV1 struct {
	V      int                   `json:"v"`
	Op     string                `json:"op"`
	Record InviteRequestRecordWire `json:"record"`
}

func WriteGroupInviteWireFrame(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > groupInviteRequestMaxFrame {
		return fmt.Errorf("group-invite-request frame size %d invalid", len(data))
	}
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(data)))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func ReadGroupInviteWireFrame(r io.Reader, out any) error {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 || n > groupInviteRequestMaxFrame {
		return fmt.Errorf("group-invite-request frame size %d invalid", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, out)
}

// ReadGroupInviteWireRaw reads one frame and returns the JSON payload bytes.
func ReadGroupInviteWireRaw(r io.Reader) ([]byte, error) {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 || n > groupInviteRequestMaxFrame {
		return nil, fmt.Errorf("group-invite-request frame size %d invalid", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
