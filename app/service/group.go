package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
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
	MessageID         string `json:"message_id"`
	GroupID           string `json:"group_id"`
	Sender            string `json:"sender"`
	SenderDisplayName string `json:"sender_display_name"`
	Content           string `json:"content"`
	Timestamp         int64  `json:"timestamp"`
	IsMine            bool   `json:"is_mine"`
	Status            string `json:"status"`
	CommentCount      int    `json:"comment_count"`
	// ReplayedAt is the unix-ms timestamp at which Autonomous Replay re-broadcast
	// this message after a fork heal. Frontend uses this to suppress the original
	// row once the replay copy is received and stored as a new row, preventing
	// duplicate display. Nil for normal (non-replayed) messages.
	ReplayedAt *int64 `json:"replayed_at,omitempty"`
}

// GroupInfo is a summary of a joined group returned to the frontend.
type GroupInfo struct {
	GroupID            string `json:"group_id"`
	Epoch              uint64 `json:"epoch"`
	MyRole             string `json:"my_role"`
	GroupType          string `json:"group_type"`
	CategoryID         string `json:"category_id,omitempty"`
	ConversationTitle  string `json:"conversation_title"`
	ConversationSub    string `json:"conversation_subtitle,omitempty"`
	ConversationAvatar string `json:"conversation_avatar_type"`
	CounterpartyPeerID string `json:"counterparty_peer_id,omitempty"`
	IsCounterpartyOn   bool   `json:"is_counterparty_online"`
	LastActivityAt     int64  `json:"last_activity_at"`
}

// MemberInfo describes a peer in the coordination active view with liveness.
type MemberInfo struct {
	PeerID      string `json:"peer_id"`
	DisplayName string `json:"display_name"`
	IsOnline    bool   `json:"is_online"`
}

// KeyPackageResult is returned by GenerateKeyPackage for Wails/TS bindings.
type KeyPackageResult struct {
	PublicHex        string `json:"public_hex"`
	BundlePrivateHex string `json:"bundle_private_hex"`
}

func normalizeGroupTypeRuntime(groupType string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(groupType))
	if normalized == "" {
		return "channel", nil
	}
	if normalized != "channel" && normalized != "dm" && normalized != "group" {
		return "", fmt.Errorf("invalid group type %q: must be one of [channel, group, dm]", groupType)
	}
	return normalized, nil
}

func canonicalDMGroupID(peerA, peerB string) string {
	ids := []string{strings.TrimSpace(peerA), strings.TrimSpace(peerB)}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(ids[0] + ":" + ids[1]))
	return "dm-" + hex.EncodeToString(sum[:8])
}

// ─── Group chat operations ───────────────────────────────────────────────────

// CreateGroupChat creates a new MLS group, starts the Coordinator, and
// subscribes to the group's GossipSub topic. The group ID must be unique.
func (r *Runtime) CreateGroupChat(groupID string, groupType string, categoryID string) error {
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}
	normalizedGroupType, err := normalizeGroupTypeRuntime(groupType)
	if err != nil {
		return err
	}
	categoryID = strings.TrimSpace(categoryID)
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	if normalizedGroupType == "channel" {
		if categoryID == "" {
			return fmt.Errorf("ERR_CATEGORY_REQUIRED: channel category is required")
		}
		if _, err := db.GetChannelCategory(categoryID); err != nil {
			return fmt.Errorf("ERR_CATEGORY_NOT_FOUND: %w", err)
		}
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}

	r.mu.Lock()
	emitMembersChanged := false
	emitCategoryAssigned := false
	defer func() {
		r.mu.Unlock()
		if emitCategoryAssigned {
			r.emit("channel_categories:changed", map[string]interface{}{
				"reason":      "assigned",
				"channel_id":  groupID,
				"category_id": categoryID,
			})
			r.broadcastChannelCategoryFrame(p2p.ChannelCategorySyncFrameV1{
				V:          1,
				Type:       "assign_channel",
				EventID:    newCategoryEventID("assign", groupID),
				ChannelID:  groupID,
				CategoryID: categoryID,
			})
		}
		if emitMembersChanged {
			r.emit("group:members_changed", map[string]interface{}{
				"group_id": groupID,
				"reason":   "created",
			})
		}
	}()

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
		Config:           coordination.DefaultConfig(),
		Transport:        r.transport,
		Clock:            coordination.RealClock{},
		MLS:              r.mlsEngine,
		Storage:          r.coordStorage,
		LocalID:          r.node.Host.ID(),
		GroupID:          groupID,
		SigningKey:       identity.SigningKeyPrivate,
		GroupInfoFetcher: r.fetchGroupInfoForHeal,
		OnMessage:        r.makeMessageHandler(groupID),
		OnEpochChange:    r.makeEpochHandler(groupID),
		OnAccessLost:     r.makeAccessLostHandler(groupID),
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

	rec, err := r.coordStorage.GetGroupRecord(groupID)
	if err != nil {
		return fmt.Errorf("group record after create: %w", err)
	}
	rec.GroupType = normalizedGroupType
	if normalizedGroupType == "channel" {
		rec.CategoryID = categoryID
	} else {
		rec.CategoryID = ""
	}
	if err := r.coordStorage.SaveGroupRecord(rec); err != nil {
		return fmt.Errorf("save group metadata: %w", err)
	}
	if normalizedGroupType == "channel" && categoryID != "" {
		if err := r.db.AssignCategoryToGroup(groupID, categoryID); err != nil {
			return err
		}
		emitCategoryAssigned = true
	}

	if err := coord.Start(r.ctx); err != nil {
		return fmt.Errorf("start coordinator: %w", err)
	}

	r.coordinators[groupID] = coord
	_ = r.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     groupID,
		PeerID:      r.node.Host.ID().String(),
		DisplayName: strings.TrimSpace(identity.DisplayName),
		Role:        "creator",
		Status:      store.GroupMemberStatusActive,
		Source:      "create",
		UpdatedAt:   time.Now().Unix(),
	})
	emitMembersChanged = true
	slog.Info("Group chat created", "group_id", groupID, "type", normalizedGroupType)
	return nil
}

// StartDirectMessage creates or reuses a deterministic DM conversation for local peer and target peer.
func (r *Runtime) StartDirectMessage(peerID string) (string, error) {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return "", fmt.Errorf("peer ID is required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return "", err
	}
	targetID, err := peer.Decode(peerID)
	if err != nil {
		return "", fmt.Errorf("invalid peer ID %q: %w", peerID, err)
	}

	r.mu.RLock()
	node := r.node
	database := r.db
	r.mu.RUnlock()
	if node == nil || database == nil {
		return "", fmt.Errorf("runtime not initialized")
	}
	if targetID == node.Host.ID() {
		return "", fmt.Errorf("cannot create direct message with yourself")
	}
	groupID := canonicalDMGroupID(node.Host.ID().String(), targetID.String())

	joined, err := database.HasGroup(groupID)
	if err != nil {
		return "", err
	}
	if !joined {
		if err := r.CreateGroupChat(groupID, "dm", ""); err != nil && !strings.Contains(err.Error(), "already in group") {
			return "", err
		}
	}

	r.mu.RLock()
	coord := r.coordinators[groupID]
	r.mu.RUnlock()
	if coord != nil {
		for _, member := range coord.ActiveMembers() {
			if member.String() == targetID.String() {
				return groupID, nil
			}
		}
	}

	members, err := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
	if err == nil {
		for _, rec := range members {
			if rec.PeerID == targetID.String() {
				return groupID, nil
			}
		}
	}
	if err := r.InvitePeerToGroup(targetID.String(), groupID); err != nil {
		return "", err
	}
	return groupID, nil
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

	r.mu.RLock()
	database := r.db
	node := r.node
	tr := r.transport
	r.mu.RUnlock()

	localPeerID := ""
	if node != nil {
		localPeerID = node.Host.ID().String()
	}
	if localPeerID == "" {
		if info, err := r.GetOnboardingInfo(); err == nil && info != nil {
			localPeerID = strings.TrimSpace(info.PeerID)
		}
	}
	online := map[string]struct{}{}
	if tr != nil {
		for _, pid := range tr.ConnectedPeers() {
			online[pid.String()] = struct{}{}
		}
	}
	if node != nil {
		online[node.Host.ID().String()] = struct{}{}
	}

	result := make([]GroupInfo, len(records))
	for i, rec := range records {
		normalizedType, _ := normalizeGroupTypeRuntime(rec.GroupType)
		title := rec.GroupID
		subtitle := ""
		counterpartyPeerID := ""
		avatarType := normalizedType
		isCounterpartyOnline := false

		if normalizedType == "dm" && database != nil {
			title = ""
			members, err := database.ListGroupMembers(rec.GroupID, store.GroupMemberStatusActive)
			if err == nil {
				for _, m := range members {
					if strings.TrimSpace(m.PeerID) == "" {
						continue
					}
					if m.PeerID == localPeerID {
						continue
					}
					counterpartyPeerID = m.PeerID
					displayName := strings.TrimSpace(m.DisplayName)
					if displayName == "" {
						displayName = r.resolveDisplayNameForPeer(counterpartyPeerID)
					}
					if displayName != "" {
						title = displayName
					}
					subtitle = counterpartyPeerID
					_, isCounterpartyOnline = online[counterpartyPeerID]
					break
				}
			}
		}

		var lastActivityAt int64
		if msgs, err := r.coordStorage.GetMessagesPaginated(rec.GroupID, 1, 0); err == nil && len(msgs) > 0 {
			lastActivityAt = msgs[0].Timestamp.WallTimeMs
		}

		result[i] = GroupInfo{
			GroupID:            rec.GroupID,
			Epoch:              rec.Epoch,
			MyRole:             string(rec.MyRole),
			GroupType:          normalizedType,
			CategoryID:         rec.CategoryID,
			ConversationTitle:  title,
			ConversationSub:    subtitle,
			ConversationAvatar: avatarType,
			CounterpartyPeerID: counterpartyPeerID,
			IsCounterpartyOn:   isCounterpartyOnline,
			LastActivityAt:     lastActivityAt,
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
	if err := r.ensureSessionActive(); err != nil {
		return "", err
	}
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
	if rec, recErr := r.coordStorage.GetGroupRecord(groupID); recErr == nil {
		if normalized, normErr := normalizeGroupTypeRuntime(rec.GroupType); normErr == nil && normalized == "dm" {
			members, listErr := r.db.ListGroupMembers(groupID, store.GroupMemberStatusActive)
			if listErr == nil {
				activeCount := 0
				for _, member := range members {
					if strings.TrimSpace(member.PeerID) != "" {
						activeCount++
					}
				}
				if activeCount >= 2 {
					return "", fmt.Errorf("direct message already has two members")
				}
			}
		}
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
	return r.joinGroupWithWelcome(groupID, welcomeHex, keyPackageBundlePrivateHex, "")
}

func (r *Runtime) joinGroupWithWelcome(groupID, welcomeHex, keyPackageBundlePrivateHex, groupType string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}
	normalizedGroupType, err := normalizeGroupTypeRuntime(groupType)
	if err != nil {
		return err
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
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
	emitMembersChanged := false
	defer func() {
		r.mu.Unlock()
		if emitMembersChanged {
			r.emit("group:members_changed", map[string]interface{}{
				"group_id": groupID,
				"reason":   "joined",
			})
		}
	}()

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
		GroupType:  normalizedGroupType,
		CategoryID: "",
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		return fmt.Errorf("save group record: %w", err)
	}

	coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
		Config:           coordination.DefaultConfig(),
		Transport:        r.transport,
		Clock:            coordination.RealClock{},
		MLS:              r.mlsEngine,
		Storage:          r.coordStorage,
		LocalID:          r.node.Host.ID(),
		GroupID:          groupID,
		SigningKey:       identity.SigningKeyPrivate,
		GroupInfoFetcher: r.fetchGroupInfoForHeal,
		OnMessage:        r.makeMessageHandler(groupID),
		OnEpochChange:    r.makeEpochHandler(groupID),
		OnAccessLost:     r.makeAccessLostHandler(groupID),
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
	_ = r.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     groupID,
		PeerID:      r.node.Host.ID().String(),
		DisplayName: strings.TrimSpace(identity.DisplayName),
		Role:        "member",
		Status:      store.GroupMemberStatusActive,
		Source:      "welcome",
		UpdatedAt:   time.Now().Unix(),
	})
	emitMembersChanged = true
	slog.Info("Joined group via Welcome", "group_id", groupID, "epoch", epoch)
	return nil
}

// GetGroupMembers returns group roster members with online presence overlay.
func (r *Runtime) GetGroupMembers(groupID string) ([]MemberInfo, error) {
	r.mu.RLock()
	tr := r.transport
	database := r.db
	node := r.node
	r.mu.RUnlock()

	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	hasGroup, err := database.HasGroup(groupID)
	if err != nil {
		return nil, err
	}
	if !hasGroup {
		return nil, fmt.Errorf("not in group %q", groupID)
	}
	// Always backfill roster from strong local evidence (self + stored senders),
	// not only when roster is empty. This prevents long-lived divergent panels.
	r.ensureGroupRosterBackfilled(groupID)
	members, err := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
	if err != nil {
		return nil, err
	}

	online := make(map[string]struct{})
	if tr != nil {
		for _, p := range tr.ConnectedPeers() {
			online[p.String()] = struct{}{}
		}
	}
	if node != nil {
		online[node.Host.ID().String()] = struct{}{}
	}

	out := make([]MemberInfo, 0, len(members))
	for _, rec := range members {
		if !isValidPeerID(rec.PeerID) {
			continue
		}
		displayName := strings.TrimSpace(rec.DisplayName)
		if displayName == "" {
			displayName = r.resolveDisplayNameForPeer(rec.PeerID)
			if displayName != "" && displayName != rec.DisplayName {
				_ = database.UpsertGroupMember(store.GroupMemberRecord{
					GroupID:     rec.GroupID,
					PeerID:      rec.PeerID,
					DisplayName: displayName,
					Role:        rec.Role,
					Status:      rec.Status,
					Source:      "profile-refresh",
					JoinedAt:    rec.JoinedAt,
					LeftAt:      rec.LeftAt,
					UpdatedAt:   time.Now().Unix(),
				})
			}
		}
		_, isOn := online[rec.PeerID]
		out = append(out, MemberInfo{
			PeerID:      rec.PeerID,
			DisplayName: displayName,
			IsOnline:    isOn,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PeerID < out[j].PeerID
	})
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
			Config:           coordination.DefaultConfig(),
			Transport:        r.transport,
			Clock:            coordination.RealClock{},
			MLS:              r.mlsEngine,
			Storage:          r.coordStorage,
			LocalID:          r.node.Host.ID(),
			GroupID:          rec.GroupID,
			SigningKey:       identity.SigningKeyPrivate,
			GroupInfoFetcher: r.fetchGroupInfoForHeal,
			OnMessage:        r.makeMessageHandler(rec.GroupID),
			OnEpochChange:    r.makeEpochHandler(rec.GroupID),
			OnAccessLost:     r.makeAccessLostHandler(rec.GroupID),
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
		if !isMine {
			if err := r.upsertGroupMember(groupID, msg.SenderID.String(), "member", "message"); err == nil {
				r.emit("group:members_changed", map[string]interface{}{
					"group_id": groupID,
					"reason":   "message_sender",
				})
			}
		}

		r.emit("group:message", map[string]interface{}{
			"message_id": msg.MessageID,
			"group_id":   msg.GroupID,
			"sender":     msg.SenderID.String(),
			"content":    string(msg.Content),
			"timestamp":  msg.Timestamp.WallTimeMs,
			"is_mine":    isMine,
			"status":     "published",
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

func (r *Runtime) makeAccessLostHandler(groupID string) func(string, uint64, string) {
	return func(_ string, epoch uint64, reason string) {
		if reason == "" {
			reason = "removed"
		}

		r.mu.Lock()
		var coordToStop interface{ Stop() }
		if r.coordinators != nil {
			if coord := r.coordinators[groupID]; coord != nil {
				coordToStop = coord
				delete(r.coordinators, groupID)
			}
		}
		database := r.db
		node := r.node
		r.mu.Unlock()

		if coordToStop != nil {
			coordToStop.Stop()
		}
		if database != nil {
			_ = database.MarkGroupLeft(groupID)
			localPeerID := ""
			if node != nil {
				localPeerID = node.Host.ID().String()
			} else if info, err := r.GetOnboardingInfo(); err == nil && info != nil {
				localPeerID = strings.TrimSpace(info.PeerID)
			}
			if localPeerID != "" {
				_ = database.MarkGroupMemberLeft(groupID, localPeerID, 0)
			}
		}

		r.emit("group:left", map[string]interface{}{
			"group_id": groupID,
			"reason":   reason,
			"epoch":    epoch,
		})
		r.emit("group:members_changed", map[string]interface{}{
			"group_id": groupID,
			"reason":   "removed_self",
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
