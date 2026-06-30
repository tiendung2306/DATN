package domain

// Identity is the local MLS signing identity (at most one per device).
type Identity struct {
	DisplayName       string
	PublicKey         []byte
	SigningKeyPrivate []byte
	Credential        []byte
}

// OnboardingInfo is what a user sends to Admin out-of-band before receiving a bundle.
type OnboardingInfo struct {
	PeerID       string
	PublicKeyHex string
}
