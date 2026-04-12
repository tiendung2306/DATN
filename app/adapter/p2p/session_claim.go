package p2p

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
)

// SessionClaim is proof that this runtime session is authoritative for the
// token identity. It is signed by the MLS private key.
type SessionClaim struct {
	StartedAt int64  `json:"started_at"`
	Nonce     []byte `json:"nonce"`
	Signature []byte `json:"signature"`
}

type sessionClaimPayload struct {
	PeerID    string `json:"peer_id"`
	StartedAt int64  `json:"started_at"`
	Nonce     []byte `json:"nonce"`
}

func BuildSessionClaim(signingKey []byte, peerID string, startedAt int64) (SessionClaim, error) {
	priv, err := normalizeEd25519PrivateKey(signingKey)
	if err != nil {
		return SessionClaim{}, err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return SessionClaim{}, fmt.Errorf("generate session nonce: %w", err)
	}
	payload := sessionClaimPayload{
		PeerID:    peerID,
		StartedAt: startedAt,
		Nonce:     nonce,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return SessionClaim{}, fmt.Errorf("marshal session claim payload: %w", err)
	}
	return SessionClaim{
		StartedAt: startedAt,
		Nonce:     nonce,
		Signature: ed25519.Sign(priv, raw),
	}, nil
}

func VerifySessionClaim(claim SessionClaim, tokenPeerID string, tokenPubKey []byte) error {
	if claim.StartedAt <= 0 {
		return fmt.Errorf("invalid session started_at")
	}
	if len(claim.Nonce) < 8 {
		return fmt.Errorf("invalid session nonce")
	}
	if len(claim.Signature) == 0 {
		return fmt.Errorf("missing session signature")
	}
	if len(tokenPubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid token public key length: %d", len(tokenPubKey))
	}
	payload := sessionClaimPayload{
		PeerID:    tokenPeerID,
		StartedAt: claim.StartedAt,
		Nonce:     claim.Nonce,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal session claim payload: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(tokenPubKey), raw, claim.Signature) {
		return fmt.Errorf("invalid session signature")
	}
	return nil
}

func normalizeEd25519PrivateKey(signingKey []byte) (ed25519.PrivateKey, error) {
	switch len(signingKey) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(signingKey), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(signingKey), nil
	default:
		return nil, fmt.Errorf("invalid ed25519 private key size: %d", len(signingKey))
	}
}
