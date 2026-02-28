package p2p

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"app/admin"
	"app/db"
	"app/mls_service"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// OnboardNewUser calls the Rust engine to generate an MLS Ed25519 key pair and persists it.
// No display_name is set here — the name is assigned by Admin in the InvitationBundle.
// Must be called once when the app is in StateUninitialized.
func OnboardNewUser(
	ctx context.Context,
	database *db.Database,
	mlsClient mls_service.MLSCryptoServiceClient,
) error {
	resp, err := mlsClient.GenerateIdentity(ctx, &mls_service.GenerateIdentityRequest{})
	if err != nil {
		return fmt.Errorf("Rust GenerateIdentity failed: %w", err)
	}
	if len(resp.PublicKey) == 0 || len(resp.SigningKeyPrivate) == 0 {
		return errors.New("Rust engine returned empty key material")
	}

	return database.SaveMLSIdentity(&db.MLSIdentity{
		DisplayName:       "", // set by Admin via InvitationToken; updated on bundle import
		PublicKey:         resp.PublicKey,
		SigningKeyPrivate:  resp.SigningKeyPrivate,
		Credential:        resp.Credential,
	})
}

// OnboardingInfo holds the two values a user sends to Admin (the CSR step).
// No display_name here — the Admin decides the name.
type OnboardingInfo struct {
	PeerID       string // Libp2p PeerID — bound in token to prevent replay attacks
	PublicKeyHex string // hex-encoded MLS public key — Admin puts this in the token
}

// GetOnboardingInfo returns the PeerID and MLS public key for this node.
// The user sends these to Admin so Admin can create the InvitationBundle (CSR step).
func GetOnboardingInfo(database *db.Database, libp2pPrivKey crypto.PrivKey) (*OnboardingInfo, error) {
	pid, err := peer.IDFromPrivateKey(libp2pPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive PeerID: %w", err)
	}

	identity, err := database.GetMLSIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to load MLS identity: %w", err)
	}

	return &OnboardingInfo{
		PeerID:       pid.String(),
		PublicKeyHex: hex.EncodeToString(identity.PublicKey),
	}, nil
}

// ImportInvitationBundle validates and stores a bundle received from the Admin.
//
// Performs four checks before storing:
//  1. Admin signature is valid (prevents forgery)
//  2. Token has not expired
//  3. token.PeerID matches this node's PeerID (prevents token replay by other peers)
//  4. token.PublicKey matches this node's MLS public key (prevents token theft)
func ImportInvitationBundle(
	database *db.Database,
	libp2pPrivKey crypto.PrivKey,
	bundleJSON []byte,
) error {
	bundle, err := admin.DeserializeBundle(bundleJSON)
	if err != nil {
		return fmt.Errorf("failed to parse bundle: %w", err)
	}

	// 1. Verify Admin signature
	if !admin.VerifyToken(bundle.Token, bundle.RootPublicKey) {
		return errors.New("invalid Admin signature on token")
	}

	// 2. Verify token not expired
	if time.Now().Unix() > bundle.Token.ExpiresAt {
		return errors.New("token has expired")
	}

	// 3. Verify PeerID matches this node (anti-replay binding)
	myPeerID, err := peer.IDFromPrivateKey(libp2pPrivKey)
	if err != nil {
		return fmt.Errorf("failed to derive local PeerID: %w", err)
	}
	if bundle.Token.PeerID != myPeerID.String() {
		return fmt.Errorf("token PeerID mismatch: token=%s, local=%s",
			bundle.Token.PeerID, myPeerID.String())
	}

	// 4. Verify MLS public key matches local identity
	identity, err := database.GetMLSIdentity()
	if err != nil {
		return fmt.Errorf("failed to load local MLS identity: %w", err)
	}
	if !bytes.Equal(bundle.Token.PublicKey, identity.PublicKey) {
		return errors.New("token PublicKey does not match local MLS identity")
	}

	if err := database.SaveAuthBundle(&db.StoredAuthBundle{
		DisplayName:    bundle.Token.DisplayName,
		PeerID:         bundle.Token.PeerID,
		PublicKey:      bundle.Token.PublicKey,
		TokenIssuedAt:  bundle.Token.IssuedAt,
		TokenExpiresAt: bundle.Token.ExpiresAt,
		TokenSignature: bundle.Token.Signature,
		BootstrapAddr:  bundle.BootstrapAddr,
		RootPublicKey:  bundle.RootPublicKey,
	}); err != nil {
		return err
	}

	// Update local MLS identity with the Admin-assigned display name.
	// This is the moment the user gets their official name in the system.
	return database.UpdateMLSDisplayName(bundle.Token.DisplayName)
}

// BuildLocalToken reconstructs the full InvitationToken from the stored bundle.
// The PeerID is taken from the stored bundle (verified during import).
func BuildLocalToken(b *db.StoredAuthBundle) *admin.InvitationToken {
	return &admin.InvitationToken{
		Version:     admin.TokenVersion,
		DisplayName: b.DisplayName,
		PeerID:      b.PeerID,
		PublicKey:   b.PublicKey,
		IssuedAt:    b.TokenIssuedAt,
		ExpiresAt:   b.TokenExpiresAt,
		Signature:   b.TokenSignature,
	}
}
