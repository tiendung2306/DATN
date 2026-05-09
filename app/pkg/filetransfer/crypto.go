package filetransfer

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
)

// MLS exporter label for Phase 8 file encryption (RFC 9420 exporter).
const Label = "file-transfer"

// ExportSecretLen is the exporter output size: 32-byte AES-256 key + 12-byte GCM nonce base.
const ExportSecretLen = 44

const (
	aesKeyLen   = 32
	gcmNonceLen = 12
)

// DefaultChunkBytes is the default maximum plaintext size per chunk (before AEAD expansion).
const DefaultChunkBytes = 1 << 20

var (
	// ErrExporterMaterial indicates secret length from MLS exporter is not ExportSecretLen.
	ErrExporterMaterial = errors.New("filetransfer: exporter secret must be 44 bytes")
)

// SplitExporterMaterial splits MLS exporter output into an AES-256 key and a 12-byte base nonce.
func SplitExporterMaterial(secret []byte) (aesKey, baseNonce []byte, err error) {
	if len(secret) != ExportSecretLen {
		return nil, nil, fmt.Errorf("%w: got %d", ErrExporterMaterial, len(secret))
	}
	key := make([]byte, aesKeyLen)
	nonce := make([]byte, gcmNonceLen)
	copy(key, secret[:aesKeyLen])
	copy(nonce, secret[aesKeyLen:ExportSecretLen])
	return key, nonce, nil
}

// NonceForChunk derives a unique 12-byte GCM nonce per chunk index under the same base nonce.
// Same scheme as design doc: XOR little-endian chunk index into the first 8 bytes.
func NonceForChunk(baseNonce []byte, chunkIndex uint64) ([]byte, error) {
	if len(baseNonce) != gcmNonceLen {
		return nil, fmt.Errorf("filetransfer: base nonce must be %d bytes", gcmNonceLen)
	}
	out := make([]byte, gcmNonceLen)
	copy(out, baseNonce)
	for i := 0; i < 8; i++ {
		out[i] ^= byte(chunkIndex >> (8 * i))
	}
	return out, nil
}

// EncryptChunk seals plaintext with AES-GCM using a unique nonce per chunk index.
func EncryptChunk(aesKey, baseNonce []byte, chunkIndex uint64, plaintext []byte) ([]byte, error) {
	if len(aesKey) != aesKeyLen {
		return nil, fmt.Errorf("filetransfer: AES key must be %d bytes", aesKeyLen)
	}
	nonce, err := NonceForChunk(baseNonce, chunkIndex)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, nil), nil
}

// DecryptChunk opens ciphertext produced by EncryptChunk.
func DecryptChunk(aesKey, baseNonce []byte, chunkIndex uint64, ciphertext []byte) ([]byte, error) {
	if len(aesKey) != aesKeyLen {
		return nil, fmt.Errorf("filetransfer: AES key must be %d bytes", aesKeyLen)
	}
	nonce, err := NonceForChunk(baseNonce, chunkIndex)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// ChunkCount returns the number of fixed-size chunks needed for a file (last chunk may be smaller).
func ChunkCount(fileSize int64, chunkBytes int) int {
	if chunkBytes <= 0 || fileSize < 0 {
		return 0
	}
	cb := int64(chunkBytes)
	return int((fileSize + cb - 1) / cb)
}
