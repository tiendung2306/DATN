package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/coordination"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

const blindStoreTopic = "/org/offline-store/v1"
const (
	blindStoreObjectEnvelope         = "group-envelope"
	blindStoreObjectKeyPackage       = "key-package"
	blindStoreObjectWelcome          = "welcome"
	blindStoreObjectReplicatedRecord = "replicated-record"
)

type blindStoreEnvelopeV1 struct {
	V              int      `json:"v"`
	PublishedAt    int64    `json:"published_at"`
	ObjectType     string   `json:"object_type"`
	GroupID        string   `json:"group_id"`
	MsgType        string   `json:"msg_type"`
	Envelope       []byte   `json:"envelope"`
	EnvelopeHash   []byte   `json:"envelope_hash"`
	PeerID         string   `json:"peer_id,omitempty"`
	PublicKP       []byte   `json:"public_kp,omitempty"`
	InviteePeerID  string   `json:"invitee_peer_id,omitempty"`
	GroupType      string   `json:"group_type,omitempty"`
	CategoryID     string   `json:"category_id,omitempty"`
	Welcome        []byte   `json:"welcome,omitempty"`
	ReplicaTargets []string `json:"replica_targets"`

	RecordNamespace string `json:"record_namespace,omitempty"`
	RecordKey       string `json:"record_key,omitempty"`
	RecordWireJSON  []byte `json:"record_wire_json,omitempty"`
	RecordSignature []byte `json:"record_signature,omitempty"`
	RecordBlob      []byte `json:"record_blob,omitempty"`
}

type blindStoreLayer struct {
	rt          *Runtime
	topic       *pubsub.Topic
	sub         *pubsub.Subscription
	cancel      context.CancelFunc
	participant bool
	storeNode   bool
	replicaK    int
}

func (r *Runtime) initBlindStoreLocked(nodeCtx context.Context) error {
	if r.node == nil || r.cfg == nil {
		return nil
	}
	topic, err := r.node.PubSub.Join(blindStoreTopic)
	if err != nil {
		return fmt.Errorf("blind-store join topic: %w", err)
	}

	layer := &blindStoreLayer{
		rt:    r,
		topic: topic,
		// Regular nodes retain only objects targeted to them; store nodes retain all.
		participant: r.cfg.BlindStoreParticipant,
		storeNode:   r.cfg.StoreNode,
		replicaK:    r.cfg.OfflineReplicaK,
	}
	r.blindStore = layer

	if layer.participant || layer.storeNode {
		sub, err := topic.Subscribe()
		if err != nil {
			_ = topic.Close()
			r.blindStore = nil
			return fmt.Errorf("blind-store subscribe: %w", err)
		}
		ctx, cancel := context.WithCancel(nodeCtx)
		layer.sub = sub
		layer.cancel = cancel
		go layer.readLoop(ctx)
		slog.Info("Blind-store subscriber enabled", "store_node", layer.storeNode, "selective_replica_mode", layer.participant && !layer.storeNode)
		return nil
	}

	slog.Info("Blind-store publish-only mode enabled")
	return nil
}

func (b *blindStoreLayer) Close() {
	if b.cancel != nil {
		b.cancel()
	}
	if b.sub != nil {
		b.sub.Cancel()
	}
	if b.topic != nil {
		_ = b.topic.Close()
	}
}

func (b *blindStoreLayer) readLoop(ctx context.Context) {
	for {
		msg, err := b.sub.Next(ctx)
		if err != nil {
			return
		}
		b.rt.mu.RLock()
		node := b.rt.node
		localID := peer.ID("")
		if node != nil {
			localID = node.Host.ID()
		}
		b.rt.mu.RUnlock()
		if localID == "" {
			continue
		}
		if msg.ReceivedFrom == localID {
			continue
		}
		b.handleInbound(msg.ReceivedFrom, msg.Data)
	}
}

func (b *blindStoreLayer) handleInbound(from peer.ID, data []byte) {
	rt := b.rt
	rt.mu.RLock()
	node := rt.node
	cs := rt.coordStorage
	db := rt.db
	rt.mu.RUnlock()
	if node == nil || db == nil {
		return
	}
	if node.AuthProtocol != nil && !node.AuthProtocol.IsVerified(from) {
		return
	}

	var msg blindStoreEnvelopeV1
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	if msg.V != 1 || msg.ObjectType == "" {
		return
	}

	if !b.storeNode {
		localID := node.Host.ID().String()
		targeted := false
		for _, t := range msg.ReplicaTargets {
			if t == localID {
				targeted = true
				break
			}
		}
		if !targeted {
			return
		}
	}

	switch msg.ObjectType {
	case blindStoreObjectEnvelope:
		if cs == nil {
			return
		}
		if msg.GroupID == "" || len(msg.Envelope) == 0 {
			return
		}
		var env coordination.Envelope
		if err := json.Unmarshal(msg.Envelope, &env); err != nil {
			return
		}
		if env.GroupID != msg.GroupID {
			return
		}
		if env.Type != coordination.MsgApplication && env.Type != coordination.MsgCommit {
			return
		}
		seq, err := cs.AppendEnvelopeWithSource(env.GroupID, env.Type, env.Epoch, env.Timestamp, msg.Envelope, "blind_store")
		if err != nil {
			slog.Debug("blind-store: append envelope failed", "err", err)
			return
		}
		if seq > 0 {
			go rt.replayPendingEnvelopesForGroup(msg.GroupID, "blind_store")
		}
	case blindStoreObjectKeyPackage:
		if msg.PeerID == "" || len(msg.PublicKP) == 0 || msg.PublishedAt <= 0 {
			return
		}
		_ = db.SaveStoredKeyPackageIfNewer(msg.PeerID, msg.PublicKP, from.String(), msg.PublishedAt)
	case blindStoreObjectWelcome:
		if msg.InviteePeerID == "" || msg.GroupID == "" || len(msg.Welcome) == 0 || msg.PublishedAt <= 0 {
			return
		}
		_ = db.SaveStoredWelcomeIfNewer(msg.InviteePeerID, msg.GroupID, msg.GroupType, msg.CategoryID, msg.Welcome, from.String(), msg.PublishedAt)
		if msg.InviteePeerID == node.Host.ID().String() {
			_ = rt.savePendingInviteFromWelcome(msg.GroupID, msg.GroupType, msg.CategoryID, msg.Welcome, from.String(), false)
		}
	case blindStoreObjectReplicatedRecord:
		ns := strings.TrimSpace(msg.RecordNamespace)
		switch ns {
		case store.NamespaceUserProfileV1:
			ownerKey := strings.TrimSpace(msg.RecordKey)
			if ownerKey == "" || len(msg.RecordWireJSON) == 0 || len(msg.RecordSignature) == 0 {
				return
			}
			wire := append([]byte(nil), msg.RecordWireJSON...)
			sig := append([]byte(nil), msg.RecordSignature...)
			blob := append([]byte(nil), msg.RecordBlob...)
			if err := rt.applySignedRemoteProfilePush(ownerKey, wire, sig, blob); err != nil && !errors.Is(err, errReplicationStaleProfile) {
				slog.Debug("blind-store: replicated record rejected", "owner", ownerKey, "err", err)
			}
		case store.NamespaceGroupAvatarV1:
			gid := strings.TrimSpace(msg.RecordKey)
			if gid == "" || len(msg.RecordWireJSON) == 0 || len(msg.RecordSignature) == 0 {
				return
			}
			wire := append([]byte(nil), msg.RecordWireJSON...)
			sig := append([]byte(nil), msg.RecordSignature...)
			blob := append([]byte(nil), msg.RecordBlob...)
			var w groupAvatarWireV1
			if err := json.Unmarshal(wire, &w); err != nil {
				return
			}
			if strings.TrimSpace(w.GroupID) != gid {
				slog.Debug("blind-store: group avatar record key mismatch", "record_key", gid, "wire_group", w.GroupID)
				return
			}
			creator := strings.TrimSpace(w.CreatorPeerID)
			if creator == "" {
				return
			}
			if err := rt.applySignedRemoteGroupAvatarPush(creator, wire, sig, blob); err != nil && !errors.Is(err, errReplicationStaleGroupAvatar) {
				slog.Debug("blind-store: group avatar replicated record rejected", "group", gid, "err", err)
			}
		case store.NamespaceGroupRoleV1:
			recordKey := strings.TrimSpace(msg.RecordKey)
			if recordKey == "" || len(msg.RecordWireJSON) == 0 || len(msg.RecordSignature) == 0 {
				return
			}
			wire := append([]byte(nil), msg.RecordWireJSON...)
			sig := append([]byte(nil), msg.RecordSignature...)
			var w groupRoleWireV1
			if err := json.Unmarshal(wire, &w); err != nil {
				return
			}
			if groupRoleRecordKey(w.GroupID, w.TargetPeerID) != recordKey {
				slog.Debug("blind-store: group role record key mismatch", "record_key", recordKey, "group", w.GroupID)
				return
			}
			actor := strings.TrimSpace(w.ActorPeerID)
			if actor == "" {
				return
			}
			if err := rt.applySignedRemoteGroupRolePush(actor, wire, sig); err != nil && !errors.Is(err, errReplicationStaleGroupRole) {
				slog.Debug("blind-store: group role replicated record rejected", "record_key", recordKey, "err", err)
			}
		case store.NamespaceGroupInvitePolicyV1:
			gid := strings.TrimSpace(msg.RecordKey)
			if gid == "" || len(msg.RecordWireJSON) == 0 || len(msg.RecordSignature) == 0 {
				return
			}
			wire := append([]byte(nil), msg.RecordWireJSON...)
			sig := append([]byte(nil), msg.RecordSignature...)
			var w groupInvitePolicyWireV1
			if err := json.Unmarshal(wire, &w); err != nil {
				return
			}
			if strings.TrimSpace(w.GroupID) != gid {
				slog.Debug("blind-store: group invite policy record key mismatch", "record_key", gid, "group", w.GroupID)
				return
			}
			actor := strings.TrimSpace(w.ActorPeerID)
			if actor == "" {
				return
			}
			if err := rt.applySignedRemoteGroupInvitePolicyPush(actor, wire, sig); err != nil && !errors.Is(err, errReplicationStaleGroupInvitePolicy) {
				slog.Debug("blind-store: group invite policy replicated record rejected", "group", gid, "err", err)
			}
		default:
			return
		}
	}
}

func (r *Runtime) publishBlindStoreEnvelope(msgType coordination.MessageType, groupID string, wire []byte) {
	r.mu.RLock()
	layer := r.blindStore
	node := r.node
	r.mu.RUnlock()
	if layer == nil || node == nil || len(wire) == 0 {
		return
	}
	if msgType != coordination.MsgApplication && msgType != coordination.MsgCommit {
		return
	}

	sum := sha256.Sum256(wire)
	r.publishBlindStoreEnvelopeToTargets(msgType, groupID, wire, layer.selectReplicaTargets(node.Host.ID(), "env:"+groupID+":"+fmt.Sprintf("%x", sum[:])))
}

func (r *Runtime) publishBlindStoreEnvelopeToTargets(msgType coordination.MessageType, groupID string, wire []byte, targets []string) {
	if len(targets) == 0 || len(wire) == 0 {
		return
	}
	sum := sha256.Sum256(wire)
	frame := blindStoreEnvelopeV1{
		V:              1,
		PublishedAt:    time.Now().UnixMilli(),
		ObjectType:     blindStoreObjectEnvelope,
		GroupID:        groupID,
		MsgType:        string(msgType),
		Envelope:       wire,
		EnvelopeHash:   sum[:],
		ReplicaTargets: targets,
	}
	r.publishBlindStoreFrame(frame)
}

func (r *Runtime) publishBlindStoreKeyPackage(peerID string, publicKP []byte) {
	r.mu.RLock()
	layer := r.blindStore
	node := r.node
	r.mu.RUnlock()
	if layer == nil || node == nil || peerID == "" || len(publicKP) == 0 {
		return
	}
	frame := blindStoreEnvelopeV1{
		V:              1,
		PublishedAt:    time.Now().UnixMilli(),
		ObjectType:     blindStoreObjectKeyPackage,
		PeerID:         peerID,
		PublicKP:       publicKP,
		ReplicaTargets: layer.selectReplicaTargets(node.Host.ID(), "kp:"+peerID),
	}
	r.publishBlindStoreFrame(frame)
}

func (r *Runtime) publishBlindStoreWelcome(inviteePeerID, groupID, groupType, categoryID string, welcome []byte) {
	r.mu.RLock()
	layer := r.blindStore
	node := r.node
	r.mu.RUnlock()
	if layer == nil || node == nil || inviteePeerID == "" || groupID == "" || len(welcome) == 0 {
		return
	}
	frame := blindStoreEnvelopeV1{
		V:              1,
		PublishedAt:    time.Now().UnixMilli(),
		ObjectType:     blindStoreObjectWelcome,
		GroupID:        groupID,
		InviteePeerID:  inviteePeerID,
		GroupType:      groupType,
		CategoryID:     categoryID,
		Welcome:        welcome,
		ReplicaTargets: layer.selectReplicaTargets(node.Host.ID(), "welcome:"+inviteePeerID+":"+groupID),
	}
	r.publishBlindStoreFrame(frame)
}

func (r *Runtime) replicateRecentEnvelopesToStorePeer(storePeer peer.ID) {
	r.mu.RLock()
	node := r.node
	cs := r.coordStorage
	groupIDs := make([]string, 0, len(r.coordinators))
	for gid := range r.coordinators {
		groupIDs = append(groupIDs, gid)
	}
	r.mu.RUnlock()
	if node == nil || cs == nil || storePeer == "" {
		return
	}
	if node.AuthProtocol != nil && !node.AuthProtocol.IsVerified(storePeer) {
		return
	}
	sort.Strings(groupIDs)
	targets := []string{storePeer.String()}
	for _, gid := range groupIDs {
		recs, err := cs.GetEnvelopesSince(gid, 0, 500)
		if err != nil {
			continue
		}
		for _, rec := range recs {
			if rec.MsgType != coordination.MsgApplication && rec.MsgType != coordination.MsgCommit {
				continue
			}
			r.publishBlindStoreEnvelopeToTargets(rec.MsgType, rec.GroupID, rec.Envelope, targets)
		}
	}
}

func (r *Runtime) publishBlindStoreReplicatedProfile(wire, sig, blob []byte) {
	r.mu.RLock()
	layer := r.blindStore
	node := r.node
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if layer == nil || node == nil || db == nil || priv == nil || len(wire) == 0 {
		return
	}
	info, err := p2p.GetOnboardingInfo(db, priv)
	if err != nil {
		return
	}
	key := info.PeerID
	sum := sha256.Sum256(wire)
	routing := "replica:" + store.NamespaceUserProfileV1 + ":" + key + ":" + fmt.Sprintf("%x", sum[:])
	targets := layer.selectReplicaTargets(node.Host.ID(), routing)
	frame := blindStoreEnvelopeV1{
		V:               1,
		PublishedAt:     time.Now().UnixMilli(),
		ObjectType:      blindStoreObjectReplicatedRecord,
		RecordNamespace: store.NamespaceUserProfileV1,
		RecordKey:       key,
		RecordWireJSON:  append([]byte(nil), wire...),
		RecordSignature: append([]byte(nil), sig...),
		RecordBlob:      append([]byte(nil), blob...),
		ReplicaTargets:  targets,
	}
	r.publishBlindStoreFrame(frame)
}

func (r *Runtime) publishBlindStoreReplicatedGroupAvatar(wire, sig, blob []byte) {
	r.mu.RLock()
	layer := r.blindStore
	node := r.node
	db := r.db
	priv := r.privKey
	r.mu.RUnlock()
	if layer == nil || node == nil || db == nil || priv == nil || len(wire) == 0 {
		return
	}
	var w groupAvatarWireV1
	if err := json.Unmarshal(wire, &w); err != nil {
		return
	}
	gid := strings.TrimSpace(w.GroupID)
	if gid == "" {
		return
	}
	sum := sha256.Sum256(wire)
	routing := "replica:" + store.NamespaceGroupAvatarV1 + ":" + gid + ":" + fmt.Sprintf("%x", sum[:])
	targets := layer.selectReplicaTargets(node.Host.ID(), routing)
	frame := blindStoreEnvelopeV1{
		V:               1,
		PublishedAt:     time.Now().UnixMilli(),
		ObjectType:      blindStoreObjectReplicatedRecord,
		RecordNamespace: store.NamespaceGroupAvatarV1,
		RecordKey:       gid,
		RecordWireJSON:  append([]byte(nil), wire...),
		RecordSignature: append([]byte(nil), sig...),
		RecordBlob:      append([]byte(nil), blob...),
		ReplicaTargets:  targets,
	}
	r.publishBlindStoreFrame(frame)
}

func (r *Runtime) publishBlindStoreReplicatedGroupRole(wire, sig []byte) {
	r.mu.RLock()
	layer := r.blindStore
	node := r.node
	r.mu.RUnlock()
	if layer == nil || node == nil || len(wire) == 0 {
		return
	}
	var w groupRoleWireV1
	if err := json.Unmarshal(wire, &w); err != nil {
		return
	}
	key := groupRoleRecordKey(w.GroupID, w.TargetPeerID)
	if strings.TrimSpace(w.GroupID) == "" || strings.TrimSpace(w.TargetPeerID) == "" || key == "|" {
		return
	}
	sum := sha256.Sum256(wire)
	routing := "replica:" + store.NamespaceGroupRoleV1 + ":" + key + ":" + fmt.Sprintf("%x", sum[:])
	targets := layer.selectReplicaTargets(node.Host.ID(), routing)
	frame := blindStoreEnvelopeV1{
		V:               1,
		PublishedAt:     time.Now().UnixMilli(),
		ObjectType:      blindStoreObjectReplicatedRecord,
		RecordNamespace: store.NamespaceGroupRoleV1,
		RecordKey:       key,
		RecordWireJSON:  append([]byte(nil), wire...),
		RecordSignature: append([]byte(nil), sig...),
		ReplicaTargets:  targets,
	}
	r.publishBlindStoreFrame(frame)
}

func (r *Runtime) publishBlindStoreReplicatedGroupInvitePolicy(wire, sig []byte) {
	r.mu.RLock()
	layer := r.blindStore
	node := r.node
	r.mu.RUnlock()
	if layer == nil || node == nil || len(wire) == 0 {
		return
	}
	var w groupInvitePolicyWireV1
	if err := json.Unmarshal(wire, &w); err != nil {
		return
	}
	gid := strings.TrimSpace(w.GroupID)
	if gid == "" {
		return
	}
	sum := sha256.Sum256(wire)
	routing := "replica:" + store.NamespaceGroupInvitePolicyV1 + ":" + gid + ":" + fmt.Sprintf("%x", sum[:])
	targets := layer.selectReplicaTargets(node.Host.ID(), routing)
	frame := blindStoreEnvelopeV1{
		V:               1,
		PublishedAt:     time.Now().UnixMilli(),
		ObjectType:      blindStoreObjectReplicatedRecord,
		RecordNamespace: store.NamespaceGroupInvitePolicyV1,
		RecordKey:       gid,
		RecordWireJSON:  append([]byte(nil), wire...),
		RecordSignature: append([]byte(nil), sig...),
		ReplicaTargets:  targets,
	}
	r.publishBlindStoreFrame(frame)
}

func (r *Runtime) publishBlindStoreFrame(frame blindStoreEnvelopeV1) {
	r.mu.RLock()
	layer := r.blindStore
	r.mu.RUnlock()
	if layer == nil {
		return
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		return
	}
	baseCtx := r.appCtx()
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()
	if err := layer.topic.Publish(ctx, payload); err != nil {
		slog.Debug("blind-store: publish failed", "object_type", frame.ObjectType, "group", frame.GroupID, "err", err)
	}
}

func (b *blindStoreLayer) selectReplicaTargets(local peer.ID, routingKey string) []string {
	if b.replicaK <= 0 || b.topic == nil {
		return nil
	}
	rt := b.rt
	candidates := b.topic.ListPeers()
	if len(candidates) == 0 {
		return nil
	}

	rt.mu.RLock()
	node := rt.node
	rt.mu.RUnlock()
	if node == nil {
		return nil
	}

	eligible := make([]peer.ID, 0, len(candidates))
	for _, pid := range candidates {
		if pid == local {
			continue
		}
		if node.AuthProtocol != nil && !node.AuthProtocol.IsVerified(pid) {
			continue
		}
		eligible = append(eligible, pid)
	}
	if len(eligible) == 0 {
		return nil
	}

	ordered := b.closestByRoutingKey(eligible, routingKey)
	if len(ordered) > b.replicaK {
		ordered = ordered[:b.replicaK]
	}
	out := make([]string, 0, len(ordered))
	for _, pid := range ordered {
		out = append(out, pid.String())
	}
	return out
}

func (b *blindStoreLayer) closestByRoutingKey(eligible []peer.ID, routingKey string) []peer.ID {
	rt := b.rt
	rt.mu.RLock()
	node := rt.node
	rt.mu.RUnlock()
	if node == nil {
		return nil
	}

	candidates := make(map[peer.ID]struct{}, len(eligible))
	for _, pid := range eligible {
		candidates[pid] = struct{}{}
	}

	ordered := make([]peer.ID, 0, len(eligible))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if node.DHT != nil && routingKey != "" {
		if dhtPeers, err := node.DHT.GetClosestPeers(ctx, routingKey); err == nil {
			for _, pid := range dhtPeers {
				if _, ok := candidates[pid]; ok {
					ordered = append(ordered, pid)
					delete(candidates, pid)
				}
			}
		}
	}
	if len(candidates) > 0 {
		rest := make([]peer.ID, 0, len(candidates))
		for pid := range candidates {
			rest = append(rest, pid)
		}
		sortByXORDistance(rest, routingKey)
		ordered = append(ordered, rest...)
	}
	return ordered
}

func sortByXORDistance(peers []peer.ID, routingKey string) {
	keyHash := sha256.Sum256([]byte(routingKey))
	sort.Slice(peers, func(i, j int) bool {
		ih := sha256.Sum256([]byte(peers[i].String()))
		jh := sha256.Sum256([]byte(peers[j].String()))
		return xorDistance(keyHash[:], ih[:]).Cmp(xorDistance(keyHash[:], jh[:])) < 0
	})
}

// blindStoreFetchCandidates returns verified blind-store peers ordered by the
// same routing logic used during publish, so fetch paths align with write paths.
func (r *Runtime) blindStoreFetchCandidates(local peer.ID, routingKey string) []peer.ID {
	r.mu.RLock()
	layer := r.blindStore
	node := r.node
	r.mu.RUnlock()
	if layer == nil || layer.topic == nil || node == nil {
		return nil
	}
	candidates := layer.topic.ListPeers()
	if len(candidates) == 0 {
		return nil
	}
	eligible := make([]peer.ID, 0, len(candidates))
	for _, pid := range candidates {
		if pid == local {
			continue
		}
		if node.AuthProtocol != nil && !node.AuthProtocol.IsVerified(pid) {
			continue
		}
		eligible = append(eligible, pid)
	}
	if len(eligible) == 0 {
		return nil
	}
	return layer.closestByRoutingKey(eligible, routingKey)
}

func xorDistance(a, b []byte) *big.Int {
	if len(a) != len(b) {
		return big.NewInt(0)
	}
	buf := make([]byte, len(a))
	for i := range a {
		buf[i] = a[i] ^ b[i]
	}
	return new(big.Int).SetBytes(buf)
}
