package port

import (
	"app/coordination"
	"app/domain"
)

// IdentityRepo persists local MLS identity rows.
type IdentityRepo interface {
	SaveIdentity(id *domain.Identity) error
	GetIdentity() (*domain.Identity, error)
	HasIdentity() (bool, error)
	UpdateDisplayName(name string) error
}

// ConfigRepo is generic key-value config (libp2p key, admin key, session flags).
type ConfigRepo interface {
	SetConfig(key string, value []byte) error
	GetConfig(key string) ([]byte, error)
	HasConfig(key string) (bool, error)
	DeleteConfig(key string) error
}

// InviteRepo stores auth bundle, KeyPackage bundles, and pending welcomes.
type InviteRepo interface {
	SaveAuthBundle(b *domain.AuthBundle) error
	GetAuthBundle() (*domain.AuthBundle, error)
	HasAuthBundle() (bool, error)
	SaveKPBundle(peerID string, publicKP, privateBundle []byte) error
	GetKPBundle(peerID string) (publicKP, privateBundle []byte, err error)
	SavePendingWelcome(targetPeerID, groupID string, welcome []byte) error
	GetPendingWelcomesFor(targetPeerID string) ([]domain.PendingWelcome, error)
	MarkWelcomeDelivered(id int64) error
}

// CoordinationStore persists MLS group state, coordination metadata, and messages.
type CoordinationStore interface {
	coordination.CoordinationStorage
}
