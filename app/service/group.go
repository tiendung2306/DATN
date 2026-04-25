package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/sidecar"
	"app/adapter/store"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ─── Types exposed to the frontend via Wails ─────────────────────────────────

// MessageInfo is a single chat message returned to the frontend.
type MessageInfo struct {
	GroupID   string `json:"group_id"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
	IsMine    bool   `json:"is_mine"`
}

// GroupInfo is a summary of a joined group returned to the frontend.
type GroupInfo struct {
	GroupID string `json:"group_id"`
	Epoch   uint64 `json:"epoch"`
	MyRole  string `json:"my_role"`
}

// MemberInfo describes a peer in the coordination active view with liveness.
type MemberInfo struct {
	PeerID   string `json:"peer_id"`
	IsOnline bool   `json:"is_online"`
}

// KeyPackageResult is returned by GenerateKeyPackage for Wails/TS bindings.
type KeyPackageResult struct {
	PublicHex        string `json:"public_hex"`
	BundlePrivateHex string `json:"bundle_private_hex"`
}

// ─── Group chat operations ───────────────────────────────────────────────────

// CreateGroupChat creates a new MLS group, starts the Coordinator, and
// subscribes to the group's GossipSub topic. The group ID must be unique.
func (r *Runtime) CreateGroupChat(groupID string) error {
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.node == nil {
		return fmt.Errorf("P2P node not running")
	}
	if r.mlsEngine == nil {
		return fmt.Errorf("crypto engine not available — build the Rust project first")
	}
	if r.coordinators == nil {
		return fmt.Errorf("coordination stack not initialized")
	}
	if _, exists := r.coordinators[groupID]; exists {
		return fmt.Errorf("already in group %q", groupID)
	}

	identity, err := r.db.GetMLSIdentity()
	if err != nil {
		return fmt.Errorf("get MLS identity: %w", err)
	}

	coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
		Config:        coordination.DefaultConfig(),
		Transport:     r.transport,
		Clock:         coordination.RealClock{},
		MLS:           r.mlsEngine,
		Storage:       r.coordStorage,
		LocalID:       r.node.Host.ID(),
		GroupID:       groupID,
		SigningKey:    identity.SigningKeyPrivate,
		OnMessage:     r.makeMessageHandler(groupID),
		OnEpochChange: r.makeEpochHandler(groupID),
		OnEnvelopeBroadcast: func(mt coordination.MessageType, gid string, wire []byte) {
			r.publishBlindStoreEnvelope(mt, gid, wire)
		},
	})
	if err != nil {
		return fmt.Errorf("create coordinator: %w", err)
	}

	if err := coord.CreateGroup(); err != nil {
		return fmt.Errorf("create group: %w", err)
	}
	if err := coord.Start(r.ctx); err != nil {
		return fmt.Errorf("start coordinator: %w", err)
	}

	r.coordinators[groupID] = coord
	slog.Info("Group chat created", "group_id", groupID)
	return nil
}

// GetGroups returns all groups the local node has joined.
func (r *Runtime) GetGroups() ([]GroupInfo, error) {
	if r.coordStorage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	records, err := r.coordStorage.ListGroups()
	if err != nil {
		return nil, err
	}

	result := make([]GroupInfo, len(records))
	for i, r := range records {
		result[i] = GroupInfo{
			GroupID: r.GroupID,
			Epoch:   r.Epoch,
			MyRole:  string(r.MyRole),
		}
	}
	return result, nil
}

// GenerateKeyPackage builds an MLS KeyPackage for the local identity.
// Public hex is shared OOB with the group creator; bundle private hex must be
// kept locally until JoinGroupWithWelcome.
func (r *Runtime) GenerateKeyPackage() (KeyPackageResult, error) {
	if r.mlsEngine == nil {
		return KeyPackageResult{}, fmt.Errorf("crypto engine not available — build the Rust project first")
	}
	identity, err := r.db.GetMLSIdentity()
	if err != nil {
		return KeyPackageResult{}, fmt.Errorf("get MLS identity: %w", err)
	}
	kp, bundle, err := r.mlsEngine.GenerateKeyPackage(context.Background(), identity.SigningKeyPrivate)
	if err != nil {
		return KeyPackageResult{}, err
	}
	return KeyPackageResult{
		PublicHex:        hex.EncodeToString(kp),
		BundlePrivateHex: hex.EncodeToString(bundle),
	}, nil
}

// AddMemberToGroup runs MLS AddMembers as the Token Holder and returns the
// Welcome message as hex for out-of-band delivery to the invitee.
func (r *Runtime) AddMemberToGroup(groupID, newMemberPeerID, keyPackageHex string) (welcomeHex string, err error) {
	raw, err := hex.DecodeString(strings.TrimSpace(keyPackageHex))
	if err != nil {
		return "", fmt.Errorf("decode key package hex: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.coordinators == nil {
		return "", fmt.Errorf("coordination stack not initialized")
	}
	coord, ok := r.coordinators[groupID]
	if !ok {
		return "", fmt.Errorf("not in group %q", groupID)
	}

	welcome, err := coord.AddMember(peer.ID(newMemberPeerID), raw)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(welcome), nil
}

// JoinGroupWithWelcome joins an existing group using a Welcome message and the
// private KeyPackage bundle from [App.GenerateKeyPackage] for this invite flow.
func (r *Runtime) JoinGroupWithWelcome(groupID, welcomeHex, keyPackageBundlePrivateHex string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}
	welcomeRaw, err := hex.DecodeString(strings.TrimSpace(welcomeHex))
	if err != nil {
		return fmt.Errorf("decode welcome hex: %w", err)
	}
	bundleRaw, err := hex.DecodeString(strings.TrimSpace(keyPackageBundlePrivateHex))
	if err != nil {
		return fmt.Errorf("decode key package bundle hex: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.node == nil {
		return fmt.Errorf("P2P node not running")
	}
	if r.mlsEngine == nil {
		return fmt.Errorf("crypto engine not available — build the Rust project first")
	}
	if r.coordStorage == nil {
		return fmt.Errorf("storage not initialized")
	}
	if _, exists := r.coordinators[groupID]; exists {
		return fmt.Errorf("already in group %q", groupID)
	}

	identity, err := r.db.GetMLSIdentity()
	if err != nil {
		return fmt.Errorf("get MLS identity: %w", err)
	}

	groupState, treeHash, epoch, err := r.mlsEngine.ProcessWelcome(context.Background(), welcomeRaw, identity.SigningKeyPrivate, bundleRaw)
	if err != nil {
		return err
	}

	now := time.Now()
	if err := r.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    groupID,
		GroupState: groupState,
		Epoch:      epoch,
		TreeHash:   treeHash,
		MyRole:     coordination.RoleMember,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		return fmt.Errorf("save group record: %w", err)
	}

	coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
		Config:        coordination.DefaultConfig(),
		Transport:     r.transport,
		Clock:         coordination.RealClock{},
		MLS:           r.mlsEngine,
		Storage:       r.coordStorage,
		LocalID:       r.node.Host.ID(),
		GroupID:       groupID,
		SigningKey:    identity.SigningKeyPrivate,
		OnMessage:     r.makeMessageHandler(groupID),
		OnEpochChange: r.makeEpochHandler(groupID),
		OnEnvelopeBroadcast: func(mt coordination.MessageType, gid string, wire []byte) {
			r.publishBlindStoreEnvelope(mt, gid, wire)
		},
	})
	if err != nil {
		return fmt.Errorf("create coordinator: %w", err)
	}
	if err := coord.Start(r.ctx); err != nil {
		return fmt.Errorf("start coordinator: %w", err)
	}
	r.coordinators[groupID] = coord
	slog.Info("Joined group via Welcome", "group_id", groupID, "epoch", epoch)
	return nil
}

// GetGroupMembers returns active-view members with coarse online status from libp2p.
func (r *Runtime) GetGroupMembers(groupID string) ([]MemberInfo, error) {
	r.mu.Lock()
	coord, ok := r.coordinators[groupID]
	tr := r.transport
	r.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("not in group %q", groupID)
	}
	if tr == nil {
		return nil, fmt.Errorf("transport not initialized")
	}

	online := make(map[string]struct{})
	for _, p := range tr.ConnectedPeers() {
		online[p.String()] = struct{}{}
	}

	ids := coord.ActiveMembers()
	out := make([]MemberInfo, 0, len(ids))
	for _, id := range ids {
		_, isOn := online[id.String()]
		out = append(out, MemberInfo{PeerID: id.String(), IsOnline: isOn})
	}
	return out, nil
}

// GetGroupStatus returns live status for a specific group.
func (r *Runtime) GetGroupStatus(groupID string) map[string]interface{} {
	r.mu.Lock()
	coord, ok := r.coordinators[groupID]
	r.mu.Unlock()

	if !ok {
		return map[string]interface{}{"error": "not in group"}
	}

	snap := coord.GetMetrics()
	return map[string]interface{}{
		"group_id":            groupID,
		"epoch":               coord.CurrentEpoch(),
		"is_token_holder":     coord.IsTokenHolder(),
		"active_members":      len(coord.ActiveMembers()),
		"commits_issued":      snap.CommitsIssued,
		"proposals_received":  snap.ProposalsReceived,
		"messages_encrypted":  snap.CommitBytesTotal,
		"partitions_detected": snap.PartitionsDetected,
	}
}

// ─── Coordination stack initialization ───────────────────────────────────────

// initCoordinationStackLocked sets up the shared transport, storage, and MLS
// engine after the P2P node is running. Must be called with r.mu held.
func (r *Runtime) initCoordinationStackLocked() {
	if r.node == nil || r.db == nil {
		return
	}

	r.transport = p2p.NewLibP2PTransport(r.node.Host, r.node.PubSub)
	r.transport.SetDirectMessageHandler(r.dispatchDirectCoordination)
	r.coordStorage = store.NewSQLiteCoordinationStorage(r.db)

	if r.mlsClient != nil {
		r.mlsEngine = sidecar.NewMLSEngine(r.mlsClient)
	}

	r.coordinators = make(map[string]*coordination.Coordinator)
	r.loadExistingGroupsLocked()
}

// loadExistingGroupsLocked restores Coordinators for groups persisted in SQLite.
// Must be called with r.mu held.
func (r *Runtime) loadExistingGroupsLocked() {
	if r.coordStorage == nil || r.node == nil {
		return
	}

	groups, err := r.coordStorage.ListGroups()
	if err != nil || len(groups) == 0 {
		return
	}

	identity, err := r.db.GetMLSIdentity()
	if err != nil {
		slog.Warn("Cannot load existing groups: no MLS identity", "error", err)
		return
	}

	for _, rec := range groups {
		coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
			Config:        coordination.DefaultConfig(),
			Transport:     r.transport,
			Clock:         coordination.RealClock{},
			MLS:           r.mlsEngine,
			Storage:       r.coordStorage,
			LocalID:       r.node.Host.ID(),
			GroupID:       rec.GroupID,
			SigningKey:    identity.SigningKeyPrivate,
			OnMessage:     r.makeMessageHandler(rec.GroupID),
			OnEpochChange: r.makeEpochHandler(rec.GroupID),
			OnEnvelopeBroadcast: func(mt coordination.MessageType, gid string, wire []byte) {
				r.publishBlindStoreEnvelope(mt, gid, wire)
			},
		})
		if err != nil {
			slog.Warn("Failed to create coordinator for existing group",
				"group", rec.GroupID, "error", err)
			continue
		}
		if err := coord.Start(r.ctx); err != nil {
			slog.Warn("Failed to start coordinator for existing group",
				"group", rec.GroupID, "error", err)
			continue
		}
		r.coordinators[rec.GroupID] = coord
		slog.Info("Restored group from DB", "group_id", rec.GroupID, "epoch", rec.Epoch)
	}
}

// ─── Event handlers ──────────────────────────────────────────────────────────

func (r *Runtime) makeMessageHandler(groupID string) func(*coordination.StoredMessage) {
	return func(msg *coordination.StoredMessage) {
		var isMine bool
		r.mu.Lock()
		if r.node != nil {
			isMine = msg.SenderID == r.node.Host.ID()
		}
		r.mu.Unlock()

		r.emit("group:message", map[string]interface{}{
			"group_id":  msg.GroupID,
			"sender":    msg.SenderID.String(),
			"content":   string(msg.Content),
			"timestamp": msg.Timestamp.WallTimeMs,
			"is_mine":   isMine,
		})
	}
}

func (r *Runtime) makeEpochHandler(groupID string) func(uint64) {
	return func(epoch uint64) {
		r.emit("group:epoch", map[string]interface{}{
			"group_id": groupID,
			"epoch":    epoch,
		})
	}
}

// ─── Teardown helpers ────────────────────────────────────────────────────────

// stopCoordinatorsLocked stops all running coordinators and closes transport.
// Must be called with r.mu held.
func (r *Runtime) stopCoordinatorsLocked() {
	for id, coord := range r.coordinators {
		coord.Stop()
		slog.Info("Stopped coordinator", "group_id", id)
	}
	r.coordinators = nil

	if r.transport != nil {
		r.transport.Close()
		r.transport = nil
	}
}

// Ensure imports are used by the compiler.
var (
	_ = (*store.SQLiteCoordinationStorage)(nil)
	_ = (*p2p.LibP2PTransport)(nil)
)
