package service

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/admin"

	"github.com/libp2p/go-libp2p/core/peer"
)

// errReplicationStaleProfile is returned when an incoming profile record is older than local state.
var errReplicationStaleProfile = errors.New("replication: stale profile revision")

// errProfileUnknownPublicKey means we cannot verify a profile until peer_directory has its MLS key.
var errProfileUnknownPublicKey = errors.New("profile: unknown MLS public key")

const profileWireVersion = 1

// UserProfileInfo is returned to the frontend for local and peer profiles.
type UserProfileInfo struct {
	PeerID         string `json:"peer_id"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email,omitempty"`
	Phone          string `json:"phone,omitempty"`
	AvatarHash     string `json:"avatar_hash,omitempty"`
	AvatarDataURL  string `json:"avatar_data_url,omitempty"`
	ProfileUpdated int64  `json:"updated_at"`
}

// UpdateUserProfileRequest updates optional contact fields only.
type UpdateUserProfileRequest struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type profileWireV1 struct {
	V               int      `json:"v"`
	PeerID          string   `json:"peer_id"`
	DisplayName     string   `json:"display_name"`
	Email           string   `json:"email"`
	Phone           string   `json:"phone"`
	AvatarHash      string   `json:"avatar_hash"`
	AvatarMime      string   `json:"avatar_mime"`
	AvatarUpdatedAt int64    `json:"avatar_updated_at"`
	ProfileRevision int64    `json:"profile_revision"`
	ClearedFields   []string `json:"cleared_fields,omitempty"`
}

func normalizeMLSEd25519PrivateKey(signingKey []byte) (ed25519.PrivateKey, error) {
	switch len(signingKey) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(signingKey), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(signingKey), nil
	default:
		return nil, fmt.Errorf("invalid MLS ed25519 private key size: %d", len(signingKey))
	}
}

func marshalProfileWire(w profileWireV1) ([]byte, error) {
	return json.Marshal(w)
}

func signProfileWire(priv ed25519.PrivateKey, wire []byte) []byte {
	return ed25519.Sign(priv, wire)
}

func verifyProfileWire(pub ed25519.PublicKey, wire []byte, sig []byte) error {
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length")
	}
	if !ed25519.Verify(pub, wire, sig) {
		return fmt.Errorf("invalid profile signature")
	}
	return nil
}

func avatarDataURL(mime string, data []byte) string {
	if mime == "" || len(data) == 0 {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func (r *Runtime) buildUserProfileInfo(peerID, displayName string, row *store.PeerDirectoryProfileRow, updatedAt int64) (UserProfileInfo, error) {
	peerID = strings.TrimSpace(peerID)
	displayName = strings.TrimSpace(displayName)
	out := UserProfileInfo{
		PeerID:         peerID,
		DisplayName:    displayName,
		ProfileUpdated: updatedAt,
	}
	if row != nil {
		if row.Email.Valid {
			out.Email = row.Email.String
		}
		if row.Phone.Valid {
			out.Phone = row.Phone.String
		}
		if row.AvatarHash.Valid && strings.TrimSpace(row.AvatarHash.String) != "" {
			out.AvatarHash = strings.TrimSpace(row.AvatarHash.String)
			// Always resolve from avatar_blobs when we have a hash. peer_directory.avatar_mime
			// can be NULL on older rows or partial merges while hash still points at a blob.
			r.mu.RLock()
			db := r.db
			r.mu.RUnlock()
			if db != nil {
				mime, blob, err := db.GetAvatarBlob(out.AvatarHash)
				if err == nil && len(blob) > 0 {
					m := strings.TrimSpace(mime)
					if m == "" && row.AvatarMime.Valid {
						m = strings.TrimSpace(row.AvatarMime.String)
					}
					out.AvatarDataURL = avatarDataURL(m, blob)
				}
			}
		}
	}
	return out, nil
}

func peerRowFromLocal(l *store.LocalProfileRow) *store.PeerDirectoryProfileRow {
	if l == nil {
		return nil
	}
	return &store.PeerDirectoryProfileRow{
		Email:           l.Email,
		Phone:           l.Phone,
		AvatarHash:      l.AvatarHash,
		AvatarMime:      l.AvatarMime,
		AvatarUpdatedAt: l.AvatarUpdatedAt,
		ProfileRevision: l.ProfileRevision,
		UpdatedAtUnix:   l.UpdatedAtUnix,
	}
}

func (r *Runtime) ensureLocalProfileSeeded() error {
	r.mu.RLock()
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if db == nil || priv == nil {
		return fmt.Errorf("app not initialized")
	}
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return err
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return err
	}
	if err := db.EnsureLocalProfileRow(info.PeerID, identity.DisplayName); err != nil {
		return err
	}
	if err := db.EnsureLocalProfileRevisionFloor(1); err != nil {
		return err
	}
	if err := db.SyncLocalProfileIdentity(info.PeerID, identity.DisplayName); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) nextLocalProfileRevision() (int64, error) {
	row, err := r.db.GetLocalProfile()
	if err != nil {
		return 0, err
	}
	return row.ProfileRevision + 1, nil
}

func computeProfileClears(before, after *store.LocalProfileRow) []string {
	if before == nil || after == nil {
		return nil
	}
	var out []string
	if before.Email.Valid && !after.Email.Valid {
		out = append(out, "email")
	}
	if before.Phone.Valid && !after.Phone.Valid {
		out = append(out, "phone")
	}
	if before.AvatarHash.Valid && !after.AvatarHash.Valid {
		out = append(out, "avatar")
	}
	return out
}

func (r *Runtime) refreshSignedSelfPeerDirectory(clearedFields []string) error {
	r.mu.RLock()
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if db == nil || priv == nil {
		return fmt.Errorf("app not initialized")
	}
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return err
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return err
	}
	local, err := db.GetLocalProfile()
	if err != nil {
		return err
	}
	pubHex := strings.ToLower(hex.EncodeToString(identity.PublicKey))
	_ = db.UpsertPeerProfileWithKey(info.PeerID, identity.DisplayName, pubHex)

	email := ""
	phone := ""
	avatarHash := ""
	avatarMime := ""
	var avatarAt int64
	if local.Email.Valid {
		email = local.Email.String
	}
	if local.Phone.Valid {
		phone = local.Phone.String
	}
	if local.AvatarHash.Valid {
		avatarHash = strings.TrimSpace(strings.ToLower(local.AvatarHash.String))
	}
	if local.AvatarMime.Valid {
		avatarMime = strings.TrimSpace(local.AvatarMime.String)
	}
	avatarAt = local.AvatarUpdatedAt

	wire := profileWireV1{
		V:               profileWireVersion,
		PeerID:          info.PeerID,
		DisplayName:     identity.DisplayName,
		Email:           email,
		Phone:           phone,
		AvatarHash:      avatarHash,
		AvatarMime:      avatarMime,
		AvatarUpdatedAt: avatarAt,
		ProfileRevision: local.ProfileRevision,
		ClearedFields:   append([]string(nil), clearedFields...),
	}
	raw, err := marshalProfileWire(wire)
	if err != nil {
		return err
	}
	mlsPriv, err := normalizeMLSEd25519PrivateKey(identity.SigningKeyPrivate)
	if err != nil {
		return err
	}
	sig := signProfileWire(mlsPriv, raw)

	var emailNS, phoneNS, hashNS, mimeNS sql.NullString
	if email != "" {
		emailNS = sql.NullString{String: email, Valid: true}
	}
	if phone != "" {
		phoneNS = sql.NullString{String: phone, Valid: true}
	}
	if avatarHash != "" {
		hashNS = sql.NullString{String: avatarHash, Valid: true}
	}
	if avatarMime != "" {
		mimeNS = sql.NullString{String: avatarMime, Valid: true}
	}
	if err := db.UpsertPeerDirectorySigned(
		info.PeerID, identity.DisplayName, pubHex,
		emailNS, phoneNS, hashNS, mimeNS, avatarAt, local.ProfileRevision, hex.EncodeToString(sig),
	); err != nil {
		return err
	}
	r.updateAuthHandshakeProfileAnnex(raw, sig)
	return nil
}

func (r *Runtime) updateAuthHandshakeProfileAnnex(wireJSON, signature []byte) {
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil || node.AuthProtocol == nil {
		return
	}
	node.AuthProtocol.SetLocalProfileAnnex(wireJSON, signature)
}

// GetMyProfile returns the local user's profile for Settings and UI.
func (r *Runtime) GetMyProfile() (UserProfileInfo, error) {
	if err := r.ensureSessionActive(); err != nil {
		return UserProfileInfo{}, err
	}
	r.mu.RLock()
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if db == nil || priv == nil {
		return UserProfileInfo{}, fmt.Errorf("app not initialized")
	}
	if err := r.ensureLocalProfileSeeded(); err != nil {
		return UserProfileInfo{}, err
	}
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return UserProfileInfo{}, err
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return UserProfileInfo{}, err
	}
	row, rowErr := db.GetPeerDirectoryProfile(info.PeerID)
	if rowErr != nil && !errors.Is(rowErr, sql.ErrNoRows) {
		return UserProfileInfo{}, rowErr
	}
	local, err := db.GetLocalProfile()
	if err != nil {
		return UserProfileInfo{}, err
	}
	var pr *store.PeerDirectoryProfileRow
	updatedAt := local.UpdatedAtUnix
	if errors.Is(rowErr, sql.ErrNoRows) {
		pr = peerRowFromLocal(local)
	} else {
		pr = row
		updatedAt = row.UpdatedAtUnix
	}
	return r.buildUserProfileInfo(info.PeerID, identity.DisplayName, pr, updatedAt)
}

// SaveMyProfile persists email, phone, and optional avatar in one step.
// avatarChange: 0 = leave avatar unchanged, 1 = replace with avatarImageBytes, 2 = remove avatar.
func (r *Runtime) SaveMyProfile(req UpdateUserProfileRequest, avatarImageBytes []byte, avatarChange int) (UserProfileInfo, error) {
	if err := r.ensureSessionActive(); err != nil {
		return UserProfileInfo{}, err
	}
	r.mu.RLock()
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if db == nil || priv == nil {
		return UserProfileInfo{}, fmt.Errorf("app not initialized")
	}
	if err := r.ensureLocalProfileSeeded(); err != nil {
		return UserProfileInfo{}, err
	}
	before, err := db.GetLocalProfile()
	if err != nil {
		return UserProfileInfo{}, err
	}
	nextRev, err := r.nextLocalProfileRevision()
	if err != nil {
		return UserProfileInfo{}, err
	}
	email := strings.TrimSpace(req.Email)
	phone := strings.TrimSpace(req.Phone)
	var emailNS, phoneNS sql.NullString
	if email != "" {
		emailNS = sql.NullString{String: email, Valid: true}
	}
	if phone != "" {
		phoneNS = sql.NullString{String: phone, Valid: true}
	}
	if err := db.SaveLocalProfileContacts(emailNS, phoneNS, nextRev); err != nil {
		return UserProfileInfo{}, err
	}
	switch avatarChange {
	case 1:
		if len(avatarImageBytes) == 0 {
			return UserProfileInfo{}, fmt.Errorf("avatar image bytes required when replacing avatar")
		}
		mime, err := validateAvatarImageBytes(avatarImageBytes)
		if err != nil {
			return UserProfileInfo{}, err
		}
		hash := store.AvatarContentHash(avatarImageBytes)
		if err := db.UpsertAvatarBlob(hash, mime, avatarImageBytes); err != nil {
			return UserProfileInfo{}, err
		}
		now := time.Now().Unix()
		if err := db.SaveLocalProfileAvatar(hash, mime, now, nextRev); err != nil {
			return UserProfileInfo{}, err
		}
	case 2:
		if err := db.ClearLocalProfileAvatar(nextRev); err != nil {
			return UserProfileInfo{}, err
		}
	case 0:
		// leave avatar as-is
	default:
		return UserProfileInfo{}, fmt.Errorf("invalid avatarChange %d", avatarChange)
	}
	after, err := db.GetLocalProfile()
	if err != nil {
		return UserProfileInfo{}, err
	}
	clears := computeProfileClears(before, after)
	if err := r.refreshSignedSelfPeerDirectory(clears); err != nil {
		return UserProfileInfo{}, err
	}
	r.replicateLocalProfileNow(clears)
	return r.GetMyProfile()
}

// UpdateMyProfile updates email and phone only; display_name stays locked.
func (r *Runtime) UpdateMyProfile(req UpdateUserProfileRequest) error {
	_, err := r.SaveMyProfile(req, nil, 0)
	return err
}

func validateAvatarImageBytes(data []byte) (mime string, err error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty image")
	}
	if len(data) > store.MaxAvatarBytes {
		return "", fmt.Errorf(
			"avatar image exceeds storage/sync limit (%d KiB); the UI must compress before upload, or choose a smaller image",
			store.MaxAvatarBytes/1024,
		)
	}
	switch {
	case len(data) >= 8 && bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}) && bytes.Equal(data[4:8], []byte{0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png", nil
	case len(data) >= 7 && bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}) && bytes.Equal(data[4:7], []byte{0x0A, 0x1A, 0x0A}):
		// De-facto variant: LF-only after "PNG" (some exporters); not in the strict PNG spec.
		return "image/png", nil
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg", nil
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", nil
	default:
		return "", fmt.Errorf("unsupported image format (allowed: png, jpeg, webp)")
	}
}

// ClearMyAvatar removes the local avatar blob reference.
func (r *Runtime) ClearMyAvatar() error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	r.mu.RLock()
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if db == nil || priv == nil {
		return fmt.Errorf("app not initialized")
	}
	if err := r.ensureLocalProfileSeeded(); err != nil {
		return err
	}
	before, err := db.GetLocalProfile()
	if err != nil {
		return err
	}
	nextRev, err := r.nextLocalProfileRevision()
	if err != nil {
		return err
	}
	if err := db.ClearLocalProfileAvatar(nextRev); err != nil {
		return err
	}
	after, err := db.GetLocalProfile()
	if err != nil {
		return err
	}
	clears := computeProfileClears(before, after)
	if err := r.refreshSignedSelfPeerDirectory(clears); err != nil {
		return err
	}
	r.replicateLocalProfileNow(clears)
	return nil
}

// GetPeerProfile returns another user's profile from peer_directory + avatar blob store.
func (r *Runtime) GetPeerProfile(peerID string) (UserProfileInfo, error) {
	if err := r.ensureSessionActive(); err != nil {
		return UserProfileInfo{}, err
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return UserProfileInfo{}, fmt.Errorf("peer_id is required")
	}
	if _, err := peer.Decode(peerID); err != nil {
		return UserProfileInfo{}, fmt.Errorf("invalid peer_id: %w", err)
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return UserProfileInfo{}, fmt.Errorf("app not initialized")
	}
	row, err := db.GetPeerDirectoryProfile(peerID)
	if err != nil {
		return UserProfileInfo{}, err
	}
	return r.buildUserProfileInfo(peerID, row.DisplayName, row, row.UpdatedAtUnix)
}

// ApplySignedPeerProfile verifies MLS-signed profile wire JSON and merges into peer_directory.
func (r *Runtime) ApplySignedPeerProfile(peerID string, wireJSON []byte, signature []byte) error {
	return r.applySignedRemoteProfilePush(peerID, wireJSON, signature, nil)
}

func (r *Runtime) applySignedRemoteProfilePush(peerID string, wireJSON, signature, avatarBlob []byte) error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return fmt.Errorf("peer_id is required")
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("app not initialized")
	}
	existing, err := db.GetPeerDirectoryProfile(peerID)
	if err != nil {
		return err
	}
	pubHex := strings.TrimSpace(existing.PublicKeyHex)
	if pubHex == "" {
		return fmt.Errorf("%w: peer %q", errProfileUnknownPublicKey, peerID)
	}
	pubBytes, err := hex.DecodeString(pubHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid stored public key for peer")
	}
	if err := verifyProfileWire(ed25519.PublicKey(pubBytes), wireJSON, signature); err != nil {
		return err
	}
	var w profileWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return fmt.Errorf("profile wire: %w", err)
	}
	if w.V != profileWireVersion {
		return fmt.Errorf("unsupported profile wire version %d", w.V)
	}
	if strings.TrimSpace(w.PeerID) != peerID {
		return fmt.Errorf("peer_id mismatch in wire payload")
	}
	if strings.TrimSpace(w.DisplayName) != strings.TrimSpace(existing.DisplayName) {
		return fmt.Errorf("display_name mismatch in signed profile")
	}
	if len(avatarBlob) > 0 {
		if strings.TrimSpace(w.AvatarHash) == "" {
			return fmt.Errorf("unexpected avatar blob without hash in wire")
		}
		want := strings.ToLower(strings.TrimSpace(w.AvatarHash))
		if store.AvatarContentHash(avatarBlob) != want {
			return fmt.Errorf("avatar content hash mismatch")
		}
		mime, err := validateAvatarImageBytes(avatarBlob)
		if err != nil {
			return err
		}
		if em := strings.TrimSpace(w.AvatarMime); em != "" && em != mime {
			return fmt.Errorf("avatar mime mismatch")
		}
		if err := db.UpsertAvatarBlob(want, mime, avatarBlob); err != nil {
			return err
		}
	}
	body := string(wireJSON)
	bodyHash := store.ReplicatedBodyHash(body)
	if err := db.TryMergeReplicatedRecord(
		store.NamespaceUserProfileV1, peerID, peerID,
		w.ProfileRevision, 1,
		body, bodyHash, signature, pubHex, 0, profileBlobRefsFromWire(wireJSON),
	); err != nil {
		if errors.Is(err, store.ErrReplicatedStaleRevision) {
			return errReplicationStaleProfile
		}
		return err
	}
	if err := db.MergePeerDirectoryProfile(
		peerID, w.ProfileRevision,
		w.Email, w.Phone, w.AvatarHash, w.AvatarMime, w.AvatarUpdatedAt,
		signature,
		w.ClearedFields,
	); err != nil {
		return err
	}
	go r.emitAllGroupsMembersChanged("profile_push")
	return nil
}

// fillAuthHandshakeProfileAnnexLocked attaches signed profile wire to the outbound auth payload.
// Caller must hold r.mu (e.g. launchP2PNode) and must not invoke other methods that lock r.mu.
func (r *Runtime) fillAuthHandshakeProfileAnnexLocked(hs *p2p.AuthHandshakeMsg) {
	if hs == nil || r.db == nil || r.privKey == nil {
		return
	}
	db, priv := r.db, r.privKey
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return
	}
	if err := db.EnsureLocalProfileRow(info.PeerID, identity.DisplayName); err != nil {
		return
	}
	if err := db.SyncLocalProfileIdentity(info.PeerID, identity.DisplayName); err != nil {
		return
	}
	local, err := db.GetLocalProfile()
	if err != nil {
		return
	}
	email, phone, avatarHash, avatarMime := "", "", "", ""
	var avatarAt int64
	if local.Email.Valid {
		email = local.Email.String
	}
	if local.Phone.Valid {
		phone = local.Phone.String
	}
	if local.AvatarHash.Valid {
		avatarHash = strings.TrimSpace(strings.ToLower(local.AvatarHash.String))
	}
	if local.AvatarMime.Valid {
		avatarMime = strings.TrimSpace(local.AvatarMime.String)
	}
	avatarAt = local.AvatarUpdatedAt
	wire := profileWireV1{
		V:               profileWireVersion,
		PeerID:          info.PeerID,
		DisplayName:     identity.DisplayName,
		Email:           email,
		Phone:           phone,
		AvatarHash:      avatarHash,
		AvatarMime:      avatarMime,
		AvatarUpdatedAt: avatarAt,
		ProfileRevision: local.ProfileRevision,
	}
	raw, err := marshalProfileWire(wire)
	if err != nil {
		return
	}
	mlsPriv, err := normalizeMLSEd25519PrivateKey(identity.SigningKeyPrivate)
	if err != nil {
		return
	}
	sig := signProfileWire(mlsPriv, raw)
	hs.ProfileWireJSON = append([]byte(nil), raw...)
	hs.ProfileSignature = append([]byte(nil), sig...)
}

func (r *Runtime) packSignedProfilePushPayload(clearedFields []string) (wireJSON, sig []byte, avatarBlob []byte, err error) {
	r.mu.RLock()
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if db == nil || priv == nil {
		return nil, nil, nil, fmt.Errorf("app not initialized")
	}
	if err := r.ensureLocalProfileSeeded(); err != nil {
		return nil, nil, nil, err
	}
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return nil, nil, nil, err
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return nil, nil, nil, err
	}
	local, err := db.GetLocalProfile()
	if err != nil {
		return nil, nil, nil, err
	}
	email, phone, avatarHash, avatarMime := "", "", "", ""
	var avatarAt int64
	if local.Email.Valid {
		email = local.Email.String
	}
	if local.Phone.Valid {
		phone = local.Phone.String
	}
	if local.AvatarHash.Valid {
		avatarHash = strings.TrimSpace(strings.ToLower(local.AvatarHash.String))
	}
	if local.AvatarMime.Valid {
		avatarMime = strings.TrimSpace(local.AvatarMime.String)
	}
	avatarAt = local.AvatarUpdatedAt
	wire := profileWireV1{
		V:               profileWireVersion,
		PeerID:          info.PeerID,
		DisplayName:     identity.DisplayName,
		Email:           email,
		Phone:           phone,
		AvatarHash:      avatarHash,
		AvatarMime:      avatarMime,
		AvatarUpdatedAt: avatarAt,
		ProfileRevision: local.ProfileRevision,
		ClearedFields:   append([]string(nil), clearedFields...),
	}
	raw, err := marshalProfileWire(wire)
	if err != nil {
		return nil, nil, nil, err
	}
	mlsPriv, err := normalizeMLSEd25519PrivateKey(identity.SigningKeyPrivate)
	if err != nil {
		return nil, nil, nil, err
	}
	sigBytes := signProfileWire(mlsPriv, raw)
	if avatarHash != "" {
		if _, b, err := db.GetAvatarBlob(avatarHash); err == nil && len(b) > 0 {
			avatarBlob = append([]byte(nil), b...)
		}
	}
	return raw, sigBytes, avatarBlob, nil
}

func (r *Runtime) pushLocalUserProfileToPeer(remote peer.ID) {
	r.replicateLocalProfilePushToPeer(remote)
}

// handleUserProfilePush is registered as the libp2p stream handler callback.
func (r *Runtime) handleUserProfilePush(remote peer.ID, wireJSON, signature, avatarBlob []byte) error {
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil || node.AuthProtocol == nil || !node.AuthProtocol.IsVerified(remote) {
		return fmt.Errorf("profile push: peer not verified")
	}
	err := r.applySignedRemoteProfilePush(remote.String(), wireJSON, signature, avatarBlob)
	if err != nil && errors.Is(err, errReplicationStaleProfile) {
		return nil
	}
	return err
}

// handleAuthProfileAnnex applies optional signed profile metadata from the auth handshake.
func (r *Runtime) handleAuthProfileAnnex(remote peer.ID, tok *admin.InvitationToken, wireJSON, signature []byte) {
	if tok == nil || len(tok.PublicKey) == 0 {
		return
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return
	}
	pubHex := strings.ToLower(hex.EncodeToString(tok.PublicKey))
	_ = db.UpsertPeerProfileWithKey(remote.String(), tok.DisplayName, pubHex)
	if err := r.ApplySignedPeerProfile(remote.String(), wireJSON, signature); err != nil {
		slog.Debug("auth profile annex rejected", "peer", remote, "err", err)
	}
}

func (r *Runtime) memberAvatarDataURL(peerID string) string {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return ""
	}
	row, err := db.GetPeerDirectoryProfile(peerID)
	if err != nil {
		return ""
	}
	info, err := r.buildUserProfileInfo(peerID, row.DisplayName, row, row.UpdatedAtUnix)
	if err != nil {
		return ""
	}
	return info.AvatarDataURL
}
