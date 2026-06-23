package main
import (
	"encoding/json"
	"fmt"
)

type mockCiphertext struct {
	Epoch     uint64 `json:"epoch"`
	Plaintext []byte `json:"plaintext"`
}

func main() {
	batchBytes := []byte(`{"events":[{"event_id":"evt-idem-1","plaintext":"SGVsbG8=","hlc":{}}]}`)
	var cipher mockCiphertext
	json.Unmarshal(batchBytes, &cipher)
	fmt.Printf("plaintext is nil? %v\n", cipher.Plaintext == nil)
}
