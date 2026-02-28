package admin

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"app/db"

	"golang.org/x/crypto/argon2"
)

const AdminKeyConfigKey = "admin_root_private_key"

// SetupAdminKey generates a new Root Admin Ed25519 key pair.
// The private key is encrypted with Argon2id+AES-256-GCM and stored in the DB.
// Returns the public key bytes. Call this only once; use UnlockAdminKey afterward.
func SetupAdminKey(database *db.Database, passphrase string) (ed25519.PublicKey, error) {
	hasKey, err := database.HasConfig(AdminKeyConfigKey)
	if err != nil {
		return nil, err
	}
	if hasKey {
		return nil, fmt.Errorf("admin key already exists; use passphrase to unlock it")
	}

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate root key pair: %w", err)
	}

	encrypted, err := encryptKey([]byte(privKey), passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt admin key: %w", err)
	}

	if err := database.SetConfig(AdminKeyConfigKey, encrypted); err != nil {
		return nil, fmt.Errorf("failed to store admin key: %w", err)
	}

	return pubKey, nil
}

// UnlockAdminKey decrypts and returns the root admin private key from DB.
func UnlockAdminKey(database *db.Database, passphrase string) (ed25519.PrivateKey, error) {
	encrypted, err := database.GetConfig(AdminKeyConfigKey)
	if err != nil {
		if db.IsNotFound(err) {
			return nil, fmt.Errorf("no admin key found; run --admin-setup first")
		}
		return nil, err
	}

	privKeyBytes, err := decryptKey(encrypted, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt admin key (wrong passphrase?): %w", err)
	}

	return ed25519.PrivateKey(privKeyBytes), nil
}

// CreateInvitationBundle builds and serializes an InvitationBundle for a new user.
// peerID and pubKeyHex are provided by the user via out-of-band channel (CSR step).
// bootstrapAddr is the admin node's own multiaddr (used as the network entry point).
func CreateInvitationBundle(
	privKey ed25519.PrivateKey,
	displayName string,
	peerID string,
	pubKeyHex string,
	bootstrapAddr string,
) ([]byte, error) {
	if bootstrapAddr == "" {
		return nil, fmt.Errorf("bootstrap_addr is required")
	}
	token, err := SignToken(privKey, displayName, peerID, pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to sign token: %w", err)
	}

	rootPubKey := privKey.Public().(ed25519.PublicKey)
	bundle := &InvitationBundle{
		Token:         token,
		BootstrapAddr: bootstrapAddr,
		RootPublicKey: []byte(rootPubKey),
	}

	return SerializeBundle(bundle)
}

// PublicKeyHex returns the hex-encoded public key derived from a private key.
// Useful for displaying the root public key to distribute with the app binary.
func PublicKeyHex(privKey ed25519.PrivateKey) string {
	return hex.EncodeToString(privKey.Public().(ed25519.PublicKey))
}

// ─── Encryption helpers ───────────────────────────────────────────────────────
// Wire format: [16 bytes Argon2 salt][12 bytes AES-GCM nonce][ciphertext+tag]

func encryptKey(plaintext []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	result := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

func decryptKey(data []byte, passphrase string) ([]byte, error) {
	const minLen = 16 + 12 + 1
	if len(data) < minLen {
		return nil, fmt.Errorf("encrypted data too short (%d bytes)", len(data))
	}

	salt := data[:16]
	nonce := data[16:28]
	ciphertext := data[28:]

	key := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong passphrase?): %w", err)
	}
	return plaintext, nil
}

// deriveKey uses Argon2id with OWASP-recommended interactive parameters.
func deriveKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, 1, 64*1024, 4, 32)
}
