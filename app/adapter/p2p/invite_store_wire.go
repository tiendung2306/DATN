package p2p

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	KPStoreProtocol      = protocol.ID("/app/kp-store/1.0.0")
	KPFetchProtocol      = protocol.ID("/app/kp-fetch/1.0.0")
	WelcomeStoreProtocol = protocol.ID("/app/welcome-store/1.0.0")
	WelcomeFetchProtocol = protocol.ID("/app/welcome-fetch/1.0.0")
)

const inviteStoreMaxFrame = 4 << 20 // 4 MiB

type KPStoreRequestV1 struct {
	V        int    `json:"v"`
	PeerID   string `json:"peer_id"`
	PublicKP []byte `json:"public_kp"`
}

type KPFetchRequestV1 struct {
	V      int    `json:"v"`
	PeerID string `json:"peer_id"`
}

type KPFetchResponseV1 struct {
	V        int    `json:"v"`
	Found    bool   `json:"found"`
	PublicKP []byte `json:"public_kp,omitempty"`
	Error    string `json:"error,omitempty"`
}

type WelcomeStoreRequestV1 struct {
	V             int    `json:"v"`
	InviteePeerID string `json:"invitee_peer_id"`
	GroupID       string `json:"group_id"`
	Welcome       []byte `json:"welcome"`
}

type WelcomeFetchRequestV1 struct {
	V             int    `json:"v"`
	InviteePeerID string `json:"invitee_peer_id"`
	GroupID       string `json:"group_id"`
}

type WelcomeFetchResponseV1 struct {
	V       int    `json:"v"`
	Found   bool   `json:"found"`
	Welcome []byte `json:"welcome,omitempty"`
	Error   string `json:"error,omitempty"`
}

func WriteInviteStoreJSONFrame(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > inviteStoreMaxFrame {
		return fmt.Errorf("invite-store frame size %d invalid", len(data))
	}
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(data)))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func ReadInviteStoreJSONFrame(r io.Reader, out interface{}) error {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 || n > inviteStoreMaxFrame {
		return fmt.Errorf("invite-store frame size %d invalid", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, out)
}
