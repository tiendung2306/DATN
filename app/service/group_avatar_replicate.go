package service

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
)

// errReplicationStaleGroupAvatar is returned when an incoming group avatar record is older than local state.
var errReplicationStaleGroupAvatar = errors.New("replication: stale group avatar revision")

const groupAvatarWireVersion = 1

type groupAvatarWireV1 struct {
	V               int      `json:"v"`
	GroupID         string   `json:"group_id"`
	CreatorPeerID   string   `json:"creator_peer_id"`
	AvatarHash      string   `json:"avatar_hash"`
	AvatarMime      string   `json:"avatar_mime"`
	AvatarUpdatedAt int64    `json:"avatar_updated_at"`
	Revision        int64    `json:"revision"`
	ClearedFields   []string `json:"cleared_fields,omitempty"`
}

func groupAvatarBlobRefsFromWire(wireJSON []byte) []store.ReplicatedBlobRef {
	var w groupAvatarWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return nil
	}
	hash := strings.TrimSpace(strings.ToLower(w.AvatarHash))
	if hash == "" {
		return nil
	}
	return []store.ReplicatedBlobRef{{Hash: hash, Required: true}}
}

func (r *Runtime) nextGroupAvatarReplicationRevision(groupID string) (int64, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return 0, fmt.Errorf("group id required")
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return 0, fmt.Errorf("app not initialized")
	}
	row, err := db.GetReplicatedRecord(store.NamespaceGroupAvatarV1, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 1, nil
		}
		return 0, err
	}
	return row.Revision + 1, nil
}

func (r *Runtime) packSignedGroupAvatarPushPayload(groupID string) (wireJSON, sig, avatarBlob []byte, err error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, nil, nil, fmt.Errorf("group id required")
	}
	r.mu.RLock()
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if db == nil || priv == nil {
		return nil, nil, nil, fmt.Errorf("app not initialized")
	}
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return nil, nil, nil, err
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return nil, nil, nil, err
	}
	rev, err := r.nextGroupAvatarReplicationRevision(groupID)
	if err != nil {
		return nil, nil, nil, err
	}
	hash, mime, at, err := db.GetGroupChatAvatarMeta(groupID)
	if err != nil {
		return nil, nil, nil, err
	}
	hash = strings.TrimSpace(strings.ToLower(hash))
	mime = strings.TrimSpace(mime)
	var clears []string
	if hash == "" {
		clears = []string{"avatar"}
	}
	wire := groupAvatarWireV1{
		V:               groupAvatarWireVersion,
		GroupID:         groupID,
		CreatorPeerID:   info.PeerID,
		AvatarHash:      hash,
		AvatarMime:      mime,
		AvatarUpdatedAt: at,
		Revision:        rev,
		ClearedFields:   clears,
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, nil, nil, err
	}
	mlsPriv, err := normalizeMLSEd25519PrivateKey(identity.SigningKeyPrivate)
	if err != nil {
		return nil, nil, nil, err
	}
	sigBytes := signProfileWire(mlsPriv, raw)
	if hash != "" {
		if _, b, err := db.GetAvatarBlob(hash); err == nil && len(b) > 0 {
			avatarBlob = append([]byte(nil), b...)
		}
	}
	return raw, sigBytes, avatarBlob, nil
}

func (r *Runtime) persistOwnReplicatedGroupAvatar(wireJSON, signature []byte) error {
	var w groupAvatarWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return fmt.Errorf("group avatar wire: %w", err)
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("app not initialized")
	}
	identity, err := db.GetMLSIdentity()
	if err != nil {
		return err
	}
	pubHex := strings.ToLower(hex.EncodeToString(identity.PublicKey))
	body := string(wireJSON)
	h := store.ReplicatedBodyHash(body)
	refs := groupAvatarBlobRefsFromWire(wireJSON)
	return db.PutReplicatedRecordForce(
		store.NamespaceGroupAvatarV1, w.GroupID, w.CreatorPeerID,
		w.Revision, 1, body, h, signature, pubHex, 0, refs,
	)
}

func (r *Runtime) replicateGroupChatAvatarAfterLocalSave(groupID string) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return
	}
	wire, sig, blob, err := r.packSignedGroupAvatarPushPayload(groupID)
	if err != nil || len(wire) == 0 || len(sig) == 0 {
		slog.Debug("replicated group avatar: pack failed", "group", groupID, "err", err)
		return
	}
	if err := r.persistOwnReplicatedGroupAvatar(wire, sig); err != nil {
		slog.Debug("replicated group avatar: persist failed", "group", groupID, "err", err)
		return
	}
	r.publishBlindStoreReplicatedGroupAvatar(wire, sig, blob)
	r.mu.RLock()
	node := r.node
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if node == nil || node.Host == nil || node.AuthProtocol == nil || db == nil || priv == nil {
		return
	}
	meta := p2p.ReplicaPushMetaV1{
		Namespace: store.NamespaceGroupAvatarV1,
		RecordKey: groupID,
	}
	self := node.Host.ID()
	for _, pid := range node.AuthProtocol.VerifiedPeerIDs() {
		if pid == self {
			continue
		}
		to := pid
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			if err := p2p.PushReplicaStoreRecord(ctx, node.Host, to, meta, wire, sig, blob); err != nil {
				slog.Debug("replica-store group avatar push failed", "peer", to, "group", groupID, "err", err)
			}
		}()
	}
}

func (r *Runtime) isActiveGroupCreatorPeer(groupID, peerID string) (bool, error) {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" || peerID == "" {
		return false, nil
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return false, fmt.Errorf("app not initialized")
	}
	creatorPeerID, err := db.GetGroupCreatorPeerID(groupID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	creatorPeerID = strings.TrimSpace(creatorPeerID)
	if creatorPeerID != "" {
		return creatorPeerID == peerID, nil
	}

	// Legacy fallback for groups created/joined before creator_peer_id existed.
	members, err := db.ListGroupMembers(groupID, store.GroupMemberStatusActive)
	if err != nil {
		return false, err
	}
	for _, m := range members {
		if strings.TrimSpace(m.PeerID) == peerID && strings.TrimSpace(m.Role) == "creator" {
			return true, nil
		}
	}
	// Transitional fallback for old data where neither mls_groups creator
	// column nor roster creator role are available yet.
	if hinted, err := db.GetGroupInviteCreatorHint(groupID); err == nil && strings.TrimSpace(hinted) == peerID {
		return true, nil
	}
	return false, nil
}

func (r *Runtime) applySignedRemoteGroupAvatarPush(signerPeerID string, wireJSON, signature, avatarBlob []byte) error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	signerPeerID = strings.TrimSpace(signerPeerID)
	if signerPeerID == "" {
		return fmt.Errorf("creator_peer_id is required")
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("app not initialized")
	}
	existing, err := db.GetPeerDirectoryProfile(signerPeerID)
	if err != nil {
		return err
	}
	pubHex := strings.TrimSpace(existing.PublicKeyHex)
	if pubHex == "" {
		return fmt.Errorf("%w: peer %q", errProfileUnknownPublicKey, signerPeerID)
	}
	pubBytes, err := hex.DecodeString(pubHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid stored public key for peer")
	}
	signingPubHex := strings.ToLower(hex.EncodeToString(pubBytes))
	if err := verifyProfileWire(ed25519.PublicKey(pubBytes), wireJSON, signature); err != nil {
		return err
	}
	var w groupAvatarWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return fmt.Errorf("group avatar wire: %w", err)
	}
	if w.V != groupAvatarWireVersion {
		return fmt.Errorf("unsupported group avatar wire version %d", w.V)
	}
	gid := strings.TrimSpace(w.GroupID)
	if gid == "" {
		return fmt.Errorf("group_id required in wire")
	}
	if strings.TrimSpace(w.CreatorPeerID) != signerPeerID {
		return fmt.Errorf("creator_peer_id mismatch in signed payload")
	}
	has, err := db.HasGroup(gid)
	if err != nil {
		return err
	}
	if !has {
		return fmt.Errorf("group avatar push: not a member of group %q", gid)
	}
	ok, err := r.isActiveGroupCreatorPeer(gid, signerPeerID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("group avatar push: peer %q is not the active creator for group %q", signerPeerID, gid)
	}
	clearAvatar := false
	for _, f := range w.ClearedFields {
		if strings.EqualFold(strings.TrimSpace(f), "avatar") {
			clearAvatar = true
			break
		}
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
		store.NamespaceGroupAvatarV1, gid, signerPeerID,
		w.Revision, 1,
		body, bodyHash, signature, signingPubHex, 0, groupAvatarBlobRefsFromWire(wireJSON),
	); err != nil {
		if errors.Is(err, store.ErrReplicatedStaleRevision) {
			return errReplicationStaleGroupAvatar
		}
		return err
	}
	if clearAvatar {
		if err := db.ClearGroupChatAvatar(gid); err != nil {
			return err
		}
	} else {
		h := strings.TrimSpace(strings.ToLower(w.AvatarHash))
		m := strings.TrimSpace(w.AvatarMime)
		if h == "" || m == "" {
			return fmt.Errorf("avatar hash and mime required when not clearing")
		}
		if err := db.SetGroupChatAvatar(gid, h, m, w.AvatarUpdatedAt); err != nil {
			return err
		}
	}
	go r.emitAllGroupsMembersChanged("group_avatar")
	return nil
}

func (r *Runtime) avatarBlobAttachmentForGroupAvatarWire(wireJSON []byte) []byte {
	var w groupAvatarWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return nil
	}
	h := strings.TrimSpace(strings.ToLower(w.AvatarHash))
	if h == "" {
		return nil
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return nil
	}
	_, b, err := db.GetAvatarBlob(h)
	if err != nil || len(b) == 0 {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
