package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"

	"app/adapter/store"
	"app/adapter/p2p"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	p2pPeer "github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/crypto/argon2"
)

const (
	backupFormatVersionV1 = 1
	backupFormatVersionV2 = 2

	backupSaltLen  = 16
	backupNonceLen = 12

	// Same Argon2id parameters as admin key encryption.
	backupArgonTime    = 1
	backupArgonMemory  = 64 * 1024
	backupArgonThreads = 4
	backupArgonKeyLen  = 32
)

// BackupPayload is the plaintext structure encrypted in .backup files.
// It contains all identity material needed to restore a device.
type BackupPayload struct {
	Version int `json:"version"`

	Libp2pPrivateKey []byte `json:"libp2p_private_key"`

	MLSDisplayName string `json:"mls_display_name"`
	MLSPublicKey   []byte `json:"mls_public_key"`
	MLSSigningKey  []byte `json:"mls_signing_key"`
	MLSCredential  []byte `json:"mls_credential"`

	BundleDisplayName string `json:"bundle_display_name"`
	BundlePeerID      string `json:"bundle_peer_id"`
	BundlePublicKey   []byte `json:"bundle_public_key"`
	TokenIssuedAt     int64  `json:"token_issued_at"`
	TokenExpiresAt    int64  `json:"token_expires_at"`
	TokenSignature    []byte `json:"token_signature"`
	BootstrapAddr     string `json:"bootstrap_addr"`
	RootPublicKey     []byte `json:"root_public_key"`

	Groups          []store.BackupGroupRecord    `json:"groups,omitempty"`
	StoredMessages  []store.BackupStoredMessage  `json:"stored_messages,omitempty"`
	KPBundles       []store.BackupKPBundle       `json:"kp_bundles,omitempty"`
	PendingWelcomes []store.BackupPendingWelcome `json:"pending_welcomes,omitempty"`
}

// ExportIdentityBackup reads local identity material from DB and encrypts it
// using Argon2id + AES-256-GCM.
//
// Wire format: [16B salt][12B nonce][ciphertext+tag].
func ExportIdentityBackup(database *store.Database, privKey p2pCrypto.PrivKey, passphrase string) ([]byte, error) {
	if database == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if privKey == nil {
		return nil, fmt.Errorf("libp2p private key is required")
	}
	if passphrase == "" {
		return nil, fmt.Errorf("passphrase is required")
	}

	identity, err := database.GetMLSIdentity()
	if err != nil {
		return nil, fmt.Errorf("load MLS identity: %w", err)
	}
	bundle, err := database.GetAuthBundle()
	if err != nil {
		return nil, fmt.Errorf("load auth bundle: %w", err)
	}

	rawPriv, err := p2pCrypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal libp2p private key: %w", err)
	}

	groups, err := database.GetAllGroupsForBackup()
	if err != nil {
		return nil, err
	}
	messages, err := database.GetAllStoredMessagesForBackup()
	if err != nil {
		return nil, err
	}
	kpBundles, err := database.GetAllKPBundlesForBackup()
	if err != nil {
		return nil, err
	}
	pendingWelcomes, err := database.GetAllPendingWelcomesForBackup()
	if err != nil {
		return nil, err
	}

	payload := BackupPayload{
		Version:           backupFormatVersionV2,
		Libp2pPrivateKey:  rawPriv,
		MLSDisplayName:    identity.DisplayName,
		MLSPublicKey:      append([]byte(nil), identity.PublicKey...),
		MLSSigningKey:     append([]byte(nil), identity.SigningKeyPrivate...),
		MLSCredential:     append([]byte(nil), identity.Credential...),
		BundleDisplayName: bundle.DisplayName,
		BundlePeerID:      bundle.PeerID,
		BundlePublicKey:   append([]byte(nil), bundle.PublicKey...),
		TokenIssuedAt:     bundle.TokenIssuedAt,
		TokenExpiresAt:    bundle.TokenExpiresAt,
		TokenSignature:    append([]byte(nil), bundle.TokenSignature...),
		BootstrapAddr:     bundle.BootstrapAddr,
		RootPublicKey:     append([]byte(nil), bundle.RootPublicKey...),
		Groups:            groups,
		StoredMessages:    messages,
		KPBundles:         kpBundles,
		PendingWelcomes:   pendingWelcomes,
	}

	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal backup payload: %w", err)
	}

	return encryptBackupPayload(plaintext, passphrase)
}

// ImportIdentityBackup decrypts and validates .backup bytes, then restores
// system_config + mls_identity + auth_bundle + persisted group/chat data.
func ImportIdentityBackup(database *store.Database, encrypted []byte, passphrase string) (*BackupPayload, error) {
	if database == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if passphrase == "" {
		return nil, fmt.Errorf("passphrase is required")
	}

	plaintext, err := decryptBackupPayload(encrypted, passphrase)
	if err != nil {
		return nil, err
	}

	var payload BackupPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal backup payload: %w", err)
	}

	if err := validateBackupPayload(&payload); err != nil {
		return nil, err
	}

	if err := database.SetConfig(p2p.Libp2pPrivKeyConfigKey, payload.Libp2pPrivateKey); err != nil {
		return nil, fmt.Errorf("restore libp2p private key: %w", err)
	}
	if err := database.SaveMLSIdentity(&store.MLSIdentity{
		DisplayName:       payload.MLSDisplayName,
		PublicKey:         payload.MLSPublicKey,
		SigningKeyPrivate: payload.MLSSigningKey,
		Credential:        payload.MLSCredential,
	}); err != nil {
		return nil, fmt.Errorf("restore mls identity: %w", err)
	}
	if err := database.SaveAuthBundle(&store.StoredAuthBundle{
		DisplayName:    payload.BundleDisplayName,
		PeerID:         payload.BundlePeerID,
		PublicKey:      payload.BundlePublicKey,
		TokenIssuedAt:  payload.TokenIssuedAt,
		TokenExpiresAt: payload.TokenExpiresAt,
		TokenSignature: payload.TokenSignature,
		BootstrapAddr:  payload.BootstrapAddr,
		RootPublicKey:  payload.RootPublicKey,
	}); err != nil {
		return nil, fmt.Errorf("restore auth bundle: %w", err)
	}
	// Full content migration is supported from backup format v2+.
	if payload.Version >= backupFormatVersionV2 {
		if err := database.ClearApplicationDataForIdentityImport(); err != nil {
			return nil, fmt.Errorf("clear application data before restore: %w", err)
		}
		if err := database.RestoreGroupsFromBackup(payload.Groups); err != nil {
			return nil, err
		}
		if err := database.RestoreStoredMessagesFromBackup(payload.StoredMessages); err != nil {
			return nil, err
		}
		if err := database.RestoreKPBundlesFromBackup(payload.KPBundles); err != nil {
			return nil, err
		}
		if err := database.RestorePendingWelcomesFromBackup(payload.PendingWelcomes); err != nil {
			return nil, err
		}
	}

	return &payload, nil
}

func validateBackupPayload(p *BackupPayload) error {
	if p.Version != backupFormatVersionV1 && p.Version != backupFormatVersionV2 {
		return fmt.Errorf("unsupported backup version %d", p.Version)
	}
	if len(p.Libp2pPrivateKey) == 0 {
		return fmt.Errorf("backup payload missing libp2p_private_key")
	}
	if len(p.MLSPublicKey) == 0 || len(p.MLSSigningKey) == 0 {
		return fmt.Errorf("backup payload missing MLS key material")
	}
	if p.BundlePeerID == "" {
		return fmt.Errorf("backup payload missing bundle_peer_id")
	}
	if len(p.BundlePublicKey) == 0 {
		return fmt.Errorf("backup payload missing bundle_public_key")
	}
	if p.BootstrapAddr == "" || len(p.RootPublicKey) == 0 {
		return fmt.Errorf("backup payload missing bootstrap/root trust data")
	}

	priv, err := p2pCrypto.UnmarshalPrivateKey(p.Libp2pPrivateKey)
	if err != nil {
		return fmt.Errorf("invalid libp2p private key in backup: %w", err)
	}
	peerID, err := p2pPeer.IDFromPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("derive peer id from backup key: %w", err)
	}
	if peerID.String() != p.BundlePeerID {
		return fmt.Errorf("backup peer mismatch: key-derived=%s bundle=%s", peerID, p.BundlePeerID)
	}

	return nil
}

func encryptBackupPayload(plaintext []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, backupSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	key := deriveBackupKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(salt)+len(nonce)+len(ct))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func decryptBackupPayload(encrypted []byte, passphrase string) ([]byte, error) {
	if len(encrypted) < backupSaltLen+backupNonceLen+1 {
		return nil, fmt.Errorf("invalid backup data: too short")
	}
	salt := encrypted[:backupSaltLen]
	nonce := encrypted[backupSaltLen : backupSaltLen+backupNonceLen]
	ct := encrypted[backupSaltLen+backupNonceLen:]
	key := deriveBackupKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt backup data: %w", err)
	}
	return pt, nil
}

func deriveBackupKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey(
		[]byte(passphrase),
		salt,
		backupArgonTime,
		backupArgonMemory,
		backupArgonThreads,
		backupArgonKeyLen,
	)
}
