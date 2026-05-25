package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (r *Runtime) persistOwnReplicatedProfile(wireJSON, signature []byte) error {
	var w profileWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return fmt.Errorf("profile wire: %w", err)
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
	refs := profileBlobRefsFromWire(wireJSON)
	return db.PutReplicatedRecordForce(
		store.NamespaceUserProfileV1, w.PeerID, w.PeerID,
		w.ProfileRevision, 1, body, h, signature, pubHex, 0, refs,
	)
}

func (r *Runtime) replicateLocalProfileNow(clearedFields []string) {
	wire, sig, blob, err := r.packSignedProfilePushPayload(clearedFields)
	if err != nil || len(wire) == 0 || len(sig) == 0 {
		return
	}
	if err := r.persistOwnReplicatedProfile(wire, sig); err != nil {
		slog.Debug("replicated profile: persist failed", "err", err)
		return
	}
	r.publishBlindStoreReplicatedProfile(wire, sig, blob)
	r.mu.RLock()
	node := r.node
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if node == nil || node.Host == nil || node.AuthProtocol == nil || db == nil || priv == nil {
		return
	}
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return
	}
	self := node.Host.ID()
	meta := p2p.ReplicaPushMetaV1{
		Namespace: store.NamespaceUserProfileV1,
		RecordKey: info.PeerID,
	}
	for _, pid := range node.AuthProtocol.VerifiedPeerIDs() {
		if pid == self {
			continue
		}
		to := pid
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			if err := p2p.PushReplicaStoreRecord(ctx, node.Host, to, meta, wire, sig, blob); err != nil {
				slog.Debug("replica-store push failed", "peer", to, "err", err)
			}
		}()
	}
}

func (r *Runtime) replicateLocalProfilePushToPeer(remote peer.ID) {
	wire, sig, blob, err := r.packSignedProfilePushPayload(nil)
	if err != nil || len(wire) == 0 || len(sig) == 0 {
		return
	}
	if err := r.persistOwnReplicatedProfile(wire, sig); err != nil {
		slog.Debug("replicated profile: persist before push failed", "err", err)
		return
	}
	r.mu.RLock()
	node := r.node
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if node == nil || node.Host == nil || db == nil || priv == nil {
		return
	}
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return
	}
	meta := p2p.ReplicaPushMetaV1{Namespace: store.NamespaceUserProfileV1, RecordKey: info.PeerID}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := p2p.PushReplicaStoreRecord(ctx, node.Host, remote, meta, wire, sig, blob); err != nil {
		slog.Debug("replica-store push to peer failed", "peer", remote, "err", err)
	}
}

func (r *Runtime) handleReplicaStorePush(remote peer.ID, meta p2p.ReplicaPushMetaV1, wireJSON, signature, blob []byte) error {
	if meta.V != 1 {
		return fmt.Errorf("unsupported replica meta version %d", meta.V)
	}
	ns := strings.TrimSpace(meta.Namespace)
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil || node.AuthProtocol == nil || !node.AuthProtocol.IsVerified(remote) {
		return fmt.Errorf("replica push: peer not verified")
	}
	switch ns {
	case store.NamespaceUserProfileV1:
		owner := strings.TrimSpace(meta.RecordKey)
		if owner == "" || owner != remote.String() {
			return fmt.Errorf("replica push record_key must match remote peer")
		}
		err := r.applySignedRemoteProfilePush(owner, wireJSON, signature, blob)
		if err != nil && errors.Is(err, errReplicationStaleProfile) {
			return nil
		}
		return err
	case store.NamespaceGroupAvatarV1:
		gid := strings.TrimSpace(meta.RecordKey)
		if gid == "" {
			return fmt.Errorf("replica push: group record_key required")
		}
		var w groupAvatarWireV1
		if err := json.Unmarshal(wireJSON, &w); err != nil {
			return fmt.Errorf("group avatar wire: %w", err)
		}
		if strings.TrimSpace(w.GroupID) != gid {
			return fmt.Errorf("replica push: group_id mismatch")
		}
		if strings.TrimSpace(w.CreatorPeerID) != remote.String() {
			return fmt.Errorf("replica push: creator must match remote peer")
		}
		err := r.applySignedRemoteGroupAvatarPush(remote.String(), wireJSON, signature, blob)
		if err != nil && errors.Is(err, errReplicationStaleGroupAvatar) {
			return nil
		}
		return err
	case store.NamespaceGroupRoleV1:
		recordKey := strings.TrimSpace(meta.RecordKey)
		if recordKey == "" {
			return fmt.Errorf("replica push: group role record_key required")
		}
		var w groupRoleWireV1
		if err := json.Unmarshal(wireJSON, &w); err != nil {
			return fmt.Errorf("group role wire: %w", err)
		}
		if groupRoleRecordKey(w.GroupID, w.TargetPeerID) != recordKey {
			return fmt.Errorf("replica push: group role key mismatch")
		}
		actor := strings.TrimSpace(w.ActorPeerID)
		if actor == "" {
			return fmt.Errorf("replica push: group role actor required")
		}
		err := r.applySignedRemoteGroupRolePush(actor, wireJSON, signature)
		if err != nil && errors.Is(err, errReplicationStaleGroupRole) {
			return nil
		}
		return err
	case store.NamespaceGroupInvitePolicyV1:
		gid := strings.TrimSpace(meta.RecordKey)
		if gid == "" {
			return fmt.Errorf("replica push: group invite policy record_key required")
		}
		var w groupInvitePolicyWireV1
		if err := json.Unmarshal(wireJSON, &w); err != nil {
			return fmt.Errorf("group invite policy wire: %w", err)
		}
		if strings.TrimSpace(w.GroupID) != gid {
			return fmt.Errorf("replica push: group invite policy key mismatch")
		}
		actor := strings.TrimSpace(w.ActorPeerID)
		if actor == "" {
			return fmt.Errorf("replica push: group invite policy actor required")
		}
		err := r.applySignedRemoteGroupInvitePolicyPush(actor, wireJSON, signature)
		if err != nil && errors.Is(err, errReplicationStaleGroupInvitePolicy) {
			return nil
		}
		return err
	default:
		return fmt.Errorf("unsupported namespace %q", meta.Namespace)
	}
}

func (r *Runtime) serveReplicaStoreP2P(remote peer.ID, req *p2p.ReplicaPullRequestV1, emit p2p.ReplicaStoreSyncEmitFunc) error {
	if req.V != 1 {
		return fmt.Errorf("unsupported pull version %d", req.V)
	}
	r.mu.RLock()
	node := r.node
	db := r.db
	r.mu.RUnlock()
	if node == nil || db == nil {
		return fmt.Errorf("app not ready")
	}
	if node.AuthProtocol == nil || !node.AuthProtocol.IsVerified(remote) {
		return fmt.Errorf("replica pull: peer not verified")
	}
	ns := strings.TrimSpace(req.Namespace)
	switch ns {
	case store.NamespaceUserProfileV1:
		if len(req.Keys) > 256 {
			return fmt.Errorf("replica pull: too many keys")
		}
		for _, key := range req.Keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			row, err := db.GetReplicatedRecord(ns, key)
			if err != nil {
				continue
			}
			cur := int64(0)
			if req.Cursors != nil {
				cur = req.Cursors[key]
			}
			if row.Revision <= cur {
				continue
			}
			blob := r.avatarBlobAttachmentForProfileWire([]byte(row.BodyJSON))
			hdr := p2p.ReplicaPullRecordHeaderV1{Key: key, Revision: row.Revision}
			if err := emit(hdr, []byte(row.BodyJSON), row.Signature, blob); err != nil {
				return err
			}
		}
		return nil
	case store.NamespaceGroupAvatarV1:
		if len(req.Keys) > 256 {
			return fmt.Errorf("replica pull: too many keys")
		}
		for _, key := range req.Keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			row, err := db.GetReplicatedRecord(ns, key)
			if err != nil {
				continue
			}
			cur := int64(0)
			if req.Cursors != nil {
				cur = req.Cursors[key]
			}
			if row.Revision <= cur {
				continue
			}
			blob := r.avatarBlobAttachmentForGroupAvatarWire([]byte(row.BodyJSON))
			hdr := p2p.ReplicaPullRecordHeaderV1{Key: key, Revision: row.Revision}
			if err := emit(hdr, []byte(row.BodyJSON), row.Signature, blob); err != nil {
				return err
			}
		}
		return nil
	case store.NamespaceGroupRoleV1:
		if len(req.Keys) > 512 {
			return fmt.Errorf("replica pull: too many keys")
		}
		for _, key := range req.Keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			row, err := db.GetReplicatedRecord(ns, key)
			if err != nil {
				continue
			}
			cur := int64(0)
			if req.Cursors != nil {
				cur = req.Cursors[key]
			}
			if row.Revision <= cur {
				continue
			}
			hdr := p2p.ReplicaPullRecordHeaderV1{Key: key, Revision: row.Revision}
			if err := emit(hdr, []byte(row.BodyJSON), row.Signature, nil); err != nil {
				return err
			}
		}
		return nil
	case store.NamespaceGroupInvitePolicyV1:
		if len(req.Keys) > 256 {
			return fmt.Errorf("replica pull: too many keys")
		}
		for _, key := range req.Keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			row, err := db.GetReplicatedRecord(ns, key)
			if err != nil {
				continue
			}
			cur := int64(0)
			if req.Cursors != nil {
				cur = req.Cursors[key]
			}
			if row.Revision <= cur {
				continue
			}
			hdr := p2p.ReplicaPullRecordHeaderV1{Key: key, Revision: row.Revision}
			if err := emit(hdr, []byte(row.BodyJSON), row.Signature, nil); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func (r *Runtime) avatarBlobAttachmentForProfileWire(wireJSON []byte) []byte {
	var w profileWireV1
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

func (r *Runtime) scheduleReplicatedProfilePull(remote peer.ID) {
	backoffs := []time.Duration{300 * time.Millisecond, 1 * time.Second, 2 * time.Second}
	for _, d := range backoffs {
		time.Sleep(d)

		r.mu.Lock()
		node := r.node
		r.mu.Unlock()
		if node == nil {
			return
		}
		if node.Host.Network().Connectedness(remote) != network.Connected {
			return
		}

		err := r.pullReplicatedProfilesFromPeerOnce(remote)
		if err == nil {
			return
		}
		if strings.Contains(err.Error(), "protocols not supported") {
			continue
		}
		slog.Debug("replica-store: pull attempt failed", "peer", remote, "err", err)
		return
	}
}

func (r *Runtime) pullReplicatedProfilesFromPeerOnce(remote peer.ID) error {
	r.mu.RLock()
	node := r.node
	db := r.db
	r.mu.RUnlock()
	if node == nil || db == nil || node.AuthProtocol == nil {
		return nil
	}
	if !node.AuthProtocol.IsVerified(remote) {
		return fmt.Errorf("peer not verified")
	}
	req, err := r.buildReplicatedProfilePullRequest(remote, node.Host.ID(), db)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.appCtx(), 30*time.Second)
	defer cancel()
	if err := p2p.PullReplicaStoreRecords(ctx, node.Host, remote, req, func(k string, rev int64, wire, sig, blob []byte) error {
		if err := r.applySignedRemoteProfilePush(k, wire, sig, blob); err != nil {
			if errors.Is(err, errReplicationStaleProfile) {
				return db.UpsertReplicatedPullCursor(remote.String(), store.NamespaceUserProfileV1, k, rev)
			}
			if errors.Is(err, errProfileUnknownPublicKey) {
				slog.Debug("replica-store: profile skipped until MLS public key is known", "owner", k, "replica", remote)
				return nil
			}
			slog.Debug("replica-store: profile record rejected", "owner", k, "replica", remote, "err", err)
			return nil
		}
		return db.UpsertReplicatedPullCursor(remote.String(), store.NamespaceUserProfileV1, k, rev)
	}); err != nil {
		return err
	}
	greq, err := r.buildReplicatedGroupAvatarPullRequest(remote, db)
	if err != nil {
		return err
	}
	if greq != nil && len(greq.Keys) > 0 {
		ctx2, cancel2 := context.WithTimeout(r.appCtx(), 30*time.Second)
		err = p2p.PullReplicaStoreRecords(ctx2, node.Host, remote, greq, func(k string, rev int64, wire, sig, blob []byte) error {
			var w groupAvatarWireV1
			if err := json.Unmarshal(wire, &w); err != nil {
				slog.Debug("replica-store: group avatar wire invalid", "replica", remote, "err", err)
				return nil
			}
			creator := strings.TrimSpace(w.CreatorPeerID)
			if creator == "" {
				return nil
			}
			if err := r.applySignedRemoteGroupAvatarPush(creator, wire, sig, blob); err != nil {
				if errors.Is(err, errReplicationStaleGroupAvatar) {
					return db.UpsertReplicatedPullCursor(remote.String(), store.NamespaceGroupAvatarV1, k, rev)
				}
				if errors.Is(err, errProfileUnknownPublicKey) {
					slog.Debug("replica-store: group avatar skipped until MLS public key is known", "creator", creator, "replica", remote)
					return nil
				}
				slog.Debug("replica-store: group avatar record rejected", "group", k, "replica", remote, "err", err)
				return nil
			}
			return db.UpsertReplicatedPullCursor(remote.String(), store.NamespaceGroupAvatarV1, k, rev)
		})
		cancel2()
		if err != nil {
			return err
		}
	}
	rreq, err := r.buildReplicatedGroupRolePullRequest(remote, db)
	if err != nil {
		return err
	}
	if rreq != nil && len(rreq.Keys) > 0 {
		ctx3, cancel3 := context.WithTimeout(r.appCtx(), 30*time.Second)
		err = p2p.PullReplicaStoreRecords(ctx3, node.Host, remote, rreq, func(k string, rev int64, wire, sig, _ []byte) error {
			var w groupRoleWireV1
			if err := json.Unmarshal(wire, &w); err != nil {
				slog.Debug("replica-store: group role wire invalid", "replica", remote, "err", err)
				return nil
			}
			actor := strings.TrimSpace(w.ActorPeerID)
			if actor == "" {
				return nil
			}
			if err := r.applySignedRemoteGroupRolePush(actor, wire, sig); err != nil {
				if errors.Is(err, errReplicationStaleGroupRole) {
					return db.UpsertReplicatedPullCursor(remote.String(), store.NamespaceGroupRoleV1, k, rev)
				}
				if errors.Is(err, errProfileUnknownPublicKey) {
					slog.Debug("replica-store: group role skipped until MLS public key is known", "actor", actor, "replica", remote)
					return nil
				}
				slog.Debug("replica-store: group role record rejected", "record_key", k, "replica", remote, "err", err)
				return nil
			}
			return db.UpsertReplicatedPullCursor(remote.String(), store.NamespaceGroupRoleV1, k, rev)
		})
		cancel3()
		if err != nil {
			return err
		}
	}
	preq, err := r.buildReplicatedGroupInvitePolicyPullRequest(remote, db)
	if err != nil {
		return err
	}
	if preq == nil || len(preq.Keys) == 0 {
		return nil
	}
	ctx4, cancel4 := context.WithTimeout(r.appCtx(), 30*time.Second)
	defer cancel4()
	return p2p.PullReplicaStoreRecords(ctx4, node.Host, remote, preq, func(k string, rev int64, wire, sig, _ []byte) error {
		var w groupInvitePolicyWireV1
		if err := json.Unmarshal(wire, &w); err != nil {
			slog.Debug("replica-store: group invite policy wire invalid", "replica", remote, "err", err)
			return nil
		}
		actor := strings.TrimSpace(w.ActorPeerID)
		if actor == "" {
			return nil
		}
		if err := r.applySignedRemoteGroupInvitePolicyPush(actor, wire, sig); err != nil {
			if errors.Is(err, errReplicationStaleGroupInvitePolicy) {
				return db.UpsertReplicatedPullCursor(remote.String(), store.NamespaceGroupInvitePolicyV1, k, rev)
			}
			if errors.Is(err, errProfileUnknownPublicKey) {
				slog.Debug("replica-store: group invite policy skipped until MLS public key is known", "actor", actor, "replica", remote)
				return nil
			}
			slog.Debug("replica-store: group invite policy record rejected", "group", k, "replica", remote, "err", err)
			return nil
		}
		return db.UpsertReplicatedPullCursor(remote.String(), store.NamespaceGroupInvitePolicyV1, k, rev)
	})
}

func (r *Runtime) buildReplicatedProfilePullRequest(remote, self peer.ID, db *store.Database) (*p2p.ReplicaPullRequestV1, error) {
	key := remote.String()
	keys := []string{key}
	known, err := db.ListKnownProfilePeerIDs(self.String(), 256)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{key: {}}
	for _, k := range known {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	cursors := make(map[string]int64, len(keys))
	for _, k := range keys {
		cur, err := db.GetReplicatedPullCursor(remote.String(), store.NamespaceUserProfileV1, k)
		if err != nil {
			return nil, err
		}
		cursors[k] = cur
	}
	return &p2p.ReplicaPullRequestV1{
		Namespace: store.NamespaceUserProfileV1,
		Keys:      keys,
		Cursors:   cursors,
	}, nil
}

func (r *Runtime) buildReplicatedGroupAvatarPullRequest(remote peer.ID, db *store.Database) (*p2p.ReplicaPullRequestV1, error) {
	keys, err := db.ListJoinedGroupChatIDsForReplication(256)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	cursors := make(map[string]int64, len(keys))
	for _, k := range keys {
		cur, err := db.GetReplicatedPullCursor(remote.String(), store.NamespaceGroupAvatarV1, k)
		if err != nil {
			return nil, err
		}
		cursors[k] = cur
	}
	return &p2p.ReplicaPullRequestV1{
		Namespace: store.NamespaceGroupAvatarV1,
		Keys:      keys,
		Cursors:   cursors,
	}, nil
}

func (r *Runtime) buildReplicatedGroupRolePullRequest(remote peer.ID, db *store.Database) (*p2p.ReplicaPullRequestV1, error) {
	groupIDs, err := db.ListJoinedGroupChatIDsForReplication(256)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	keys := make([]string, 0)
	for _, groupID := range groupIDs {
		members, err := db.ListGroupMembers(groupID)
		if err != nil {
			continue
		}
		for _, member := range members {
			if member.PeerID == "" || member.Status != store.GroupMemberStatusActive {
				continue
			}
			key := groupRoleRecordKey(groupID, member.PeerID)
			if key == "|" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	cursors := make(map[string]int64, len(keys))
	for _, k := range keys {
		cur, err := db.GetReplicatedPullCursor(remote.String(), store.NamespaceGroupRoleV1, k)
		if err != nil {
			return nil, err
		}
		cursors[k] = cur
	}
	return &p2p.ReplicaPullRequestV1{
		Namespace: store.NamespaceGroupRoleV1,
		Keys:      keys,
		Cursors:   cursors,
	}, nil
}

func (r *Runtime) buildReplicatedGroupInvitePolicyPullRequest(remote peer.ID, db *store.Database) (*p2p.ReplicaPullRequestV1, error) {
	keys, err := db.ListJoinedGroupChatIDsForReplication(256)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	cursors := make(map[string]int64, len(keys))
	for _, k := range keys {
		cur, err := db.GetReplicatedPullCursor(remote.String(), store.NamespaceGroupInvitePolicyV1, k)
		if err != nil {
			return nil, err
		}
		cursors[k] = cur
	}
	return &p2p.ReplicaPullRequestV1{
		Namespace: store.NamespaceGroupInvitePolicyV1,
		Keys:      keys,
		Cursors:   cursors,
	}, nil
}

func (r *Runtime) replicatedProfileRepairLoop(ctx context.Context) {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.mu.RLock()
			node := r.node
			db := r.db
			r.mu.RUnlock()
			if node == nil || node.AuthProtocol == nil {
				continue
			}
			self := node.Host.ID()
			for _, pid := range node.AuthProtocol.VerifiedPeerIDs() {
				if pid == self {
					continue
				}
				p := pid
				go func() { _ = r.pullReplicatedProfilesFromPeerOnce(p) }()
			}
			if db != nil {
				cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
				if n, err := db.GCUnreferencedReplicatedBlobs(cutoff, 128); err != nil {
					slog.Debug("replica-store: replicated blob gc failed", "err", err)
				} else if n > 0 {
					slog.Debug("replica-store: replicated blob gc", "deleted", n)
				}
				if n, err := db.GCUnreferencedAvatarBlobs(cutoff, 128); err != nil {
					slog.Debug("replica-store: avatar blob gc failed", "err", err)
				} else if n > 0 {
					slog.Debug("replica-store: avatar blob gc", "deleted", n)
				}
			}
		}
	}
}

func profileBlobRefsFromWire(wireJSON []byte) []store.ReplicatedBlobRef {
	var w profileWireV1
	if err := json.Unmarshal(wireJSON, &w); err != nil {
		return nil
	}
	hash := strings.TrimSpace(strings.ToLower(w.AvatarHash))
	if hash == "" {
		return nil
	}
	return []store.ReplicatedBlobRef{{Hash: hash, Required: true}}
}
