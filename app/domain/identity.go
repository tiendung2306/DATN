package domain

// Identity is the local MLS signing identity (at most one per device).
type Identity struct {
	DisplayName       string
	PublicKey         []byte
	SigningKeyPrivate []byte
	Credential        []byte
}

// AppState is the onboarding / authorization lifecycle state.
type AppState int

const (
	StateUninitialized AppState = iota
	StateAwaitingBundle
	StateAuthorized
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
		return "ERROR"
	}
}

// OnboardingInfo is what a user sends to Admin out-of-band before receiving a bundle.
type OnboardingInfo struct {
	PeerID       string
	PublicKeyHex string
}
