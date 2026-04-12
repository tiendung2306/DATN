package service

import (
	"time"

	"app/admin"
	"app/adapter/store"
)

// AppState represents the onboarding stage of this node.
type AppState int

const (
	// StateUninitialized: No MLS identity exists yet.
	// Action required: run with --setup to generate keys.
	StateUninitialized AppState = iota

	// StateAwaitingBundle: MLS identity exists but no valid InvitationBundle.
	// Action required: send PeerID + PublicKey to Admin, then import the bundle.
	StateAwaitingBundle

	// StateAuthorized: Valid bundle imported. Ready to join the P2P network.
	StateAuthorized

	// StateAdminReady: StateAuthorized AND has Root Admin private key.
	// Enables --create-bundle and admin operations.
	StateAdminReady
)

func (s AppState) String() string {
	switch s {
	case StateUninitialized:
		return "UNINITIALIZED"
	case StateAwaitingBundle:
		return "AWAITING_BUNDLE"
	case StateAuthorized:
		return "AUTHORIZED"
	case StateAdminReady:
		return "ADMIN_READY"
	default:
		return "UNKNOWN"
	}
}

// DetermineAppState inspects the database to derive the current onboarding stage.
func DetermineAppState(database *store.Database) (AppState, error) {
	hasIdentity, err := database.HasMLSIdentity()
	if err != nil {
		return StateUninitialized, err
	}
	if !hasIdentity {
		return StateUninitialized, nil
	}

	hasBundle, err := database.HasAuthBundle()
	if err != nil {
		return StateAwaitingBundle, err
	}
	if !hasBundle {
		return StateAwaitingBundle, nil
	}

	bundle, err := database.GetAuthBundle()
	if err != nil {
		return StateAwaitingBundle, err
	}
	if time.Now().Unix() > bundle.TokenExpiresAt {
		return StateAwaitingBundle, nil
	}

	hasAdminKey, err := database.HasConfig(admin.AdminKeyConfigKey)
	if err != nil {
		return StateAuthorized, err
	}
	if hasAdminKey {
		return StateAdminReady, nil
	}

	return StateAuthorized, nil
}
