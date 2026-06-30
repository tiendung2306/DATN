package coordination

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

func deriveStorageKey(signingKey []byte) []byte {
	h := sha256.Sum256(signingKey)
	return h[:]
}

func sealPayload(plaintext, key []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = aesgcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func openPayload(ciphertext, nonce, key []byte) (plaintext []byte, err error) {
	if len(ciphertext) == 0 || len(nonce) == 0 {
		return nil, fmt.Errorf("openPayload: empty ciphertext or nonce")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err = aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// newTraceID returns a short hex identifier suitable for tagging log lines
// belonging to a single fork-heal pipeline. 8 hex chars (32 bits) is enough
// for human readability and disambiguating concurrent heals across nodes.
func newTraceID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}
