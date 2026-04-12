package store

import (
	"app/domain"
	"app/port"
)

// Repos bundles outbound repository ports backed by one SQLite database.
type Repos struct {
	Identity       port.IdentityRepo
	Config         port.ConfigRepo
	Invite         port.InviteRepo
	Coordination   port.CoordinationStore
	DB             *Database
	CoordinationDB *SQLiteCoordinationStorage
}

// NewRepos wires all repository ports for the given database.
func NewRepos(d *Database) *Repos {
	coord := NewSQLiteCoordinationStorage(d)
	return &Repos{
		Identity:       identityRepo{d},
		Config:         configRepo{d},
		Invite:         inviteRepo{d},
		Coordination:   coord,
		DB:             d,
		CoordinationDB: coord,
	}
}

type identityRepo struct{ d *Database }

func (r identityRepo) SaveIdentity(id *domain.Identity) error {
	return r.d.SaveMLSIdentity(&MLSIdentity{
		DisplayName:       id.DisplayName,
		PublicKey:         id.PublicKey,
		SigningKeyPrivate: id.SigningKeyPrivate,
		Credential:        id.Credential,
	})
}

func (r identityRepo) GetIdentity() (*domain.Identity, error) {
	id, err := r.d.GetMLSIdentity()
	if err != nil {
		return nil, err
	}
	return &domain.Identity{
		DisplayName:       id.DisplayName,
		PublicKey:         id.PublicKey,
		SigningKeyPrivate: id.SigningKeyPrivate,
		Credential:        id.Credential,
	}, nil
}

func (r identityRepo) HasIdentity() (bool, error) {
	return r.d.HasMLSIdentity()
}

func (r identityRepo) UpdateDisplayName(name string) error {
	return r.d.UpdateMLSDisplayName(name)
}

type configRepo struct{ d *Database }

func (r configRepo) SetConfig(key string, value []byte) error { return r.d.SetConfig(key, value) }
func (r configRepo) GetConfig(key string) ([]byte, error)     { return r.d.GetConfig(key) }
func (r configRepo) HasConfig(key string) (bool, error)       { return r.d.HasConfig(key) }
func (r configRepo) DeleteConfig(key string) error           { return r.d.DeleteConfig(key) }

type inviteRepo struct{ d *Database }

func (r inviteRepo) SaveAuthBundle(b *domain.AuthBundle) error {
	return r.d.SaveAuthBundle(&StoredAuthBundle{
		DisplayName:    b.DisplayName,
		PeerID:         b.PeerID,
		PublicKey:      b.PublicKey,
		TokenIssuedAt:  b.TokenIssuedAt,
		TokenExpiresAt: b.TokenExpiresAt,
		TokenSignature: b.TokenSignature,
		BootstrapAddr:  b.BootstrapAddr,
		RootPublicKey:  b.RootPublicKey,
	})
}

func (r inviteRepo) GetAuthBundle() (*domain.AuthBundle, error) {
	b, err := r.d.GetAuthBundle()
	if err != nil {
		return nil, err
	}
	return &domain.AuthBundle{
		DisplayName:    b.DisplayName,
		PeerID:         b.PeerID,
		PublicKey:      b.PublicKey,
		TokenIssuedAt:  b.TokenIssuedAt,
		TokenExpiresAt: b.TokenExpiresAt,
		TokenSignature: b.TokenSignature,
		BootstrapAddr:  b.BootstrapAddr,
		RootPublicKey:  b.RootPublicKey,
	}, nil
}

func (r inviteRepo) HasAuthBundle() (bool, error) {
	return r.d.HasAuthBundle()
}

func (r inviteRepo) SaveKPBundle(peerID string, publicKP, privateBundle []byte) error {
	return r.d.SaveKPBundle(peerID, publicKP, privateBundle)
}

func (r inviteRepo) GetKPBundle(peerID string) ([]byte, []byte, error) {
	return r.d.GetKPBundle(peerID)
}

func (r inviteRepo) SavePendingWelcome(targetPeerID, groupID string, welcome []byte) error {
	return r.d.SavePendingWelcome(targetPeerID, groupID, welcome)
}

func (r inviteRepo) GetPendingWelcomesFor(targetPeerID string) ([]domain.PendingWelcome, error) {
	rows, err := r.d.GetPendingWelcomesFor(targetPeerID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PendingWelcome, 0, len(rows))
	for _, pw := range rows {
		out = append(out, domain.PendingWelcome{
			ID:           pw.ID,
			TargetPeerID: pw.TargetPeerID,
			GroupID:      pw.GroupID,
			WelcomeBytes: pw.WelcomeBytes,
		})
	}
	return out, nil
}

func (r inviteRepo) MarkWelcomeDelivered(id int64) error {
	return r.d.MarkWelcomeDelivered(id)
}

var (
	_ port.IdentityRepo       = identityRepo{}
	_ port.ConfigRepo         = configRepo{}
	_ port.InviteRepo         = inviteRepo{}
	_ port.CoordinationStore  = (*SQLiteCoordinationStorage)(nil)
)
