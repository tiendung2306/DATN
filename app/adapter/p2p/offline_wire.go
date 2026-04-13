package p2p

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/protocol"
)

// ErrOfflineStreamEnd is returned when a 4-byte zero length marker is read.
var ErrOfflineStreamEnd = errors.New("offline: stream end marker")

// Offline sync / delivery-ack stream protocols (Noise + auth gating on handler side).
const (
	OfflineSyncProtocol      = protocol.ID("/app/offline-sync/1.0.0")
	OfflineDeliveryAckProtocol = protocol.ID("/app/offline-delivery-ack/1.0.0")
)

const (
	offlineMaxFrame = 4 << 20 // 4 MiB
)

// OfflineSyncRequestV1 is sent by the requester (puller) to a responder.
type OfflineSyncRequestV1 struct {
	V      int                        `json:"v"`
	Groups []OfflineSyncRequestGroup  `json:"groups"`
}

// OfflineSyncRequestGroup identifies a group and the last remote seq pulled from this responder.
type OfflineSyncRequestGroup struct {
	GroupID  string `json:"group_id"`
	AfterSeq int64  `json:"after_seq"`
}

// OfflineSyncBatchV1 is one chunk of envelopes for a group (responder → requester).
type OfflineSyncBatchV1 struct {
	GroupID string               `json:"group_id"`
	Entries []OfflineSyncEntryV1 `json:"entries"`
	HasMore bool                 `json:"has_more"`
}

// OfflineSyncEntryV1 carries one logged envelope and its seq in the responder's DB.
type OfflineSyncEntryV1 struct {
	Seq      int64  `json:"seq"`
	Envelope []byte `json:"envelope"`
}

// OfflineSyncAckV1 is sent by the requester after successful replay.
type OfflineSyncAckV1 struct {
	V      int                     `json:"v"`
	Groups []OfflineSyncAckGroupV1 `json:"groups"`
}

// OfflineSyncAckGroupV1 reports the highest responder seq applied per group.
type OfflineSyncAckGroupV1 struct {
	GroupID  string `json:"group_id"`
	AckedSeq int64  `json:"acked_seq"`
}

// OfflineDeliveryAckV1 notifies senders after DHT (or sync) consumption.
type OfflineDeliveryAckV1 struct {
	V         int                         `json:"v"`
	Recipient string                      `json:"recipient"` // consumer peer ID string
	Groups    []OfflineDeliveryAckGroupV1 `json:"groups"`
}

// OfflineDeliveryAckGroupV1 names the group and max seq acknowledged for bundles from the handler's peer.
type OfflineDeliveryAckGroupV1 struct {
	GroupID  string `json:"group_id"`
	AckedSeq int64  `json:"acked_seq"`
}

// WriteOfflineEndMarker writes a 4-byte zero length (end of batch stream).
func WriteOfflineEndMarker(w io.Writer) error {
	var z [4]byte
	_, err := w.Write(z[:])
	return err
}

// WriteOfflineJSONFrame writes a length-prefixed JSON payload.
func WriteOfflineJSONFrame(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > offlineMaxFrame {
		return fmt.Errorf("offline frame size %d invalid", len(data))
	}
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(data)))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ReadOfflineJSONFrame reads a length-prefixed JSON payload or returns ErrOfflineStreamEnd for a zero-length marker.
func ReadOfflineJSONFrame(r io.Reader, out interface{}) error {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 {
		return ErrOfflineStreamEnd
	}
	if n > offlineMaxFrame {
		return fmt.Errorf("offline frame too large: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, out)
}

// ReadOfflineRawFrame reads the next length-prefixed payload (no JSON decode).
func ReadOfflineRawFrame(r io.Reader) ([]byte, error) {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 {
		return nil, io.EOF
	}
	if n > offlineMaxFrame {
		return nil, fmt.Errorf("offline frame too large: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
