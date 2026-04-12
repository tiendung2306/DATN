package domain

// AuthBundle is the stored invitation material after import.
type AuthBundle struct {
	DisplayName    string
	PeerID         string
	PublicKey      []byte
	TokenIssuedAt  int64
	TokenExpiresAt int64
	TokenSignature []byte
	BootstrapAddr  string
	RootPublicKey  []byte
}

// PendingWelcome is an undelivered Welcome on the creator side.
type PendingWelcome struct {
	ID           int64
	TargetPeerID string
	GroupID      string
	WelcomeBytes []byte
}

// KPStatus reports whether a KeyPackage is advertised for this node.
type KPStatus struct {
	Advertised bool
	PeerID     string
}

// CreateBundleRequest holds parameters for admin bundle creation (GUI).
type CreateBundleRequest struct {
	DisplayName     string
	PeerID          string
	PublicKeyHex    string
	AdminPassphrase string
}
