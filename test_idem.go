package main
import (
	"encoding/json"
	"fmt"
)

type mockGroupState struct {
	Epoch uint64 `json:"epoch"`
}

type mockCiphertext struct {
	Epoch     uint64 `json:"epoch"`
	Plaintext []byte `json:"plaintext"`
}

type ApplicationEventPayload struct {
	EventID   string `json:"event_id"`
	Plaintext []byte `json:"plaintext"`
	HLC       json.RawMessage `json:"hlc,omitempty"`
}

type BatchedPlaintext struct {
	Events []ApplicationEventPayload `json:"events"`
}

type BatchedApplicationMsg struct {
	Ciphertext []byte `json:"ciphertext"`
}

func main() {
	batch := BatchedPlaintext{
		Events: []ApplicationEventPayload{
			{EventID: "evt-idem-1", Plaintext: []byte("Hello"), HLC: []byte("{}")},
		},
	}
	batchBytes, _ := json.Marshal(batch)
	
	state := mockGroupState{Epoch: 1}
	
	cipher := mockCiphertext{
		Epoch:     state.Epoch,
		Plaintext: batchBytes,
	}
	ciphertext, _ := json.Marshal(cipher)
	
	batchMsg := BatchedApplicationMsg{Ciphertext: ciphertext}
	payload, _ := json.Marshal(batchMsg)

	// Receiver side
	var recvBatchMsg BatchedApplicationMsg
	json.Unmarshal(payload, &recvBatchMsg)

	var recvCipher mockCiphertext
	json.Unmarshal(recvBatchMsg.Ciphertext, &recvCipher)

	plaintext := recvCipher.Plaintext

	var recvBatch BatchedPlaintext
	err := json.Unmarshal(plaintext, &recvBatch)
	fmt.Printf("err: %v\n", err)
	fmt.Printf("plaintext: %s\n", string(plaintext))
}
