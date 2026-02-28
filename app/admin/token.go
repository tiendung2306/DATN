package admin

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	TokenVersion      = 1
	TokenValidityDays = 365
)

// InvitationToken is the credential signed by the Root Admin that authorizes
// a specific node to join the network. The signature covers all other fields.
// The PeerID binding prevents replay attacks: a stolen token is useless without
// control of the corresponding Libp2p private key, which Noise protocol verifies.
type InvitationToken struct {
	Version     int    `json:"version"`
	DisplayName string `json:"display_name"`
	PeerID      string `json:"peer_id"` // Libp2p PeerID — verified against Noise-authenticated peer
	PublicKey   []byte `json:"public_key"`
	IssuedAt    int64  `json:"issued_at"`
	ExpiresAt   int64  `json:"expires_at"`
	Signature   []byte `json:"signature,omitempty"`
}

// InvitationBundle is the complete package the Admin sends to a new user out-of-band
// (e.g. via Zalo/email). It contains everything needed to bootstrap into the network.
type InvitationBundle struct {
	Token         *InvitationToken `json:"token"`
	BootstrapAddr string           `json:"bootstrap_addr"`
	RootPublicKey []byte           `json:"root_public_key"` // TOFU: trusted on first import
}

// tokenPayload is the subset of InvitationToken that gets signed.
// Omitting the Signature field prevents circular dependency in the signing payload.
type tokenPayload struct {
	Version     int    `json:"version"`
	DisplayName string `json:"display_name"`
	PeerID      string `json:"peer_id"`
	PublicKey   []byte `json:"public_key"`
	IssuedAt    int64  `json:"issued_at"`
	ExpiresAt   int64  `json:"expires_at"`
}

func (t *InvitationToken) signingPayload() ([]byte, error) {
	return json.Marshal(tokenPayload{
		Version:     t.Version,
		DisplayName: t.DisplayName,
		PeerID:      t.PeerID,
		PublicKey:   t.PublicKey,
		IssuedAt:    t.IssuedAt,
		ExpiresAt:   t.ExpiresAt,
	})
}

// SignToken creates and signs an InvitationToken for the given user.
// pubKeyHex is the hex-encoded MLS public key received from the user (CSR step).
// Both peerID and pubKeyHex must be provided by the user via out-of-band channel.
func SignToken(
	privKey ed25519.PrivateKey,
	displayName string,
	peerID string,
	pubKeyHex string,
) (*InvitationToken, error) {
	if displayName == "" {
		return nil, errors.New("display_name is required")
	}
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}
	pubKey, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid pubKeyHex: %w", err)
	}
	if len(pubKey) == 0 {
		return nil, errors.New("public_key is required")
	}

	now := time.Now().Unix()
	token := &InvitationToken{
		Version:     TokenVersion,
		DisplayName: displayName,
		PeerID:      peerID,
		PublicKey:   pubKey,
		IssuedAt:    now,
		ExpiresAt:   now + int64(TokenValidityDays*24*60*60),
	}

	payload, err := token.signingPayload()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signing payload: %w", err)
	}
	token.Signature = ed25519.Sign(privKey, payload)
	return token, nil
}

// VerifyToken verifies the Admin's Ed25519 signature on a token.
// Returns false if the token is nil, has no signature, or the signature is invalid.
func VerifyToken(token *InvitationToken, rootPubKey ed25519.PublicKey) bool {
	if token == nil || len(token.Signature) == 0 || len(rootPubKey) == 0 {
		return false
	}
	payload, err := token.signingPayload()
	if err != nil {
		return false
	}
	return ed25519.Verify(rootPubKey, payload, token.Signature)
}

// SerializeBundle serializes an InvitationBundle to JSON bytes for file export.
func SerializeBundle(b *InvitationBundle) ([]byte, error) {
	return json.MarshalIndent(b, "", "  ")
}

// DeserializeBundle parses and validates an InvitationBundle from JSON bytes.
func DeserializeBundle(data []byte) (*InvitationBundle, error) {
	var b InvitationBundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("invalid bundle format: %w", err)
	}
	if b.Token == nil {
		return nil, errors.New("bundle missing token")
	}
	if b.Token.PeerID == "" {
		return nil, errors.New("bundle token missing peer_id")
	}
	if b.BootstrapAddr == "" {
		return nil, errors.New("bundle missing bootstrap_addr")
	}
	if len(b.RootPublicKey) == 0 {
		return nil, errors.New("bundle missing root_public_key")
	}
	return &b, nil
}
