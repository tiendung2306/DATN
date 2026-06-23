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

func main() {
	plaintext := []byte(`{"events":[{"event_id":"evt-idem-1","plaintext":"SGVsbG8=","hlc":{}}]}`)
	groupState := []byte(`{"epoch":1}`)
	
	var state mockGroupState
	json.Unmarshal(groupState, &state)

	cipher := mockCiphertext{
		Epoch:     state.Epoch,
		Plaintext: plaintext,
	}
	cipherBytes, _ := json.Marshal(cipher)
	
	// simulate BatchedApplicationMsg
	type BatchedApplicationMsg struct {
		Ciphertext []byte `json:"ciphertext"`
	}
	batchMsg := BatchedApplicationMsg{Ciphertext: cipherBytes}
	payload, _ := json.Marshal(batchMsg)

	// Receiver
	var recvBatchMsg BatchedApplicationMsg
	json.Unmarshal(payload, &recvBatchMsg)

	var recvCipher mockCiphertext
	if err := json.Unmarshal(recvBatchMsg.Ciphertext, &recvCipher); err != nil {
		fmt.Println("fallback")
	}

	fmt.Println("plaintext:", string(recvCipher.Plaintext))
}
