package p2p

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	// GroupInfoProtocol is used by fork-healing losers to request a winner's
	// verifiable GroupInfo blob before calling ExternalJoin.
	GroupInfoProtocol = protocol.ID("/app/group-info/1.0.0")
)

const groupInfoMaxFrame = 4 << 20 // 4 MiB

// GroupInfoRequestV1 asks a remote peer to export GroupInfo for one group.
type GroupInfoRequestV1 struct {
	V               int    `json:"v"`
	GroupID         string `json:"group_id"`
	WithRatchetTree bool   `json:"with_ratchet_tree"`
}

// GroupInfoResponseV1 returns exported GroupInfo and metadata used by the
// caller's heal orchestrator to verify winner branch alignment.
type GroupInfoResponseV1 struct {
	V         int    `json:"v"`
	GroupID   string `json:"group_id"`
	Epoch     uint64 `json:"epoch"`
	TreeHash  []byte `json:"tree_hash,omitempty"`
	GroupInfo []byte `json:"group_info,omitempty"`
	Error     string `json:"error,omitempty"`
}

func WriteGroupInfoJSONFrame(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > groupInfoMaxFrame {
		return fmt.Errorf("group-info frame size %d invalid", len(data))
	}
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(data)))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func ReadGroupInfoJSONFrame(r io.Reader, out interface{}) error {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 || n > groupInfoMaxFrame {
		return fmt.Errorf("group-info frame size %d invalid", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, out)
}
