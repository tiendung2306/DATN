package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
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

const generateKeyPackageTimeout = 20 * time.Second

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
	// CounterpartyAvatarDataURL is set for DM groups when an avatar exists for the other participant.
	CounterpartyAvatarDataURL string `json:"counterparty_avatar_data_url,omitempty"`
	// GroupAvatarDataURL is set for non-channel group chats when a local group image exists.
	GroupAvatarDataURL string `json:"group_avatar_data_url,omitempty"`
	IsCounterpartyOn   bool   `json:"is_counterparty_online"`
	LastActivityAt     int64  `json:"last_activity_at"`
}

// MemberInfo describes a peer in the coordination active view with liveness.
type MemberInfo struct {
	PeerID         string `json:"peer_id"`
	DisplayName    string `json:"display_name"`
	AvatarDataURL  string `json:"avatar_data_url,omitempty"`
	IsOnline       bool   `json:"is_online"`
	Role           string `json:"role"`
	IsAdmin        bool   `json:"is_admin"`
	IsCreator      bool   `json:"is_creator"`
	CanManageAdmin bool   `json:"can_manage_admin"`
	CanRemove      bool   `json:"can_remove"`
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
		Config:               coordination.DefaultConfig(),
		Transport:            r.transport,
		Clock:                coordination.RealClock{},
		MLS:                  r.mlsEngine,
		Storage:              r.coordStorage,
		LocalID:              r.node.Host.ID(),
		GroupID:              groupID,
		SigningKey:           identity.SigningKeyPrivate,
		GroupInfoFetcher:     r.fetchGroupInfoForHeal,
		AuthorizedCommitters: r.authorizedCommittersProvider(db),
		InitialActiveView:    []peer.ID{r.node.Host.ID()},
		OnMessage:            r.makeMessageHandler(groupID),
		OnEpochChange:        r.makeEpochHandler(groupID),
		OnAccessLost:         r.makeAccessLostHandler(groupID),
		OnEnvelopeBroadcast: func(mt coordination.MessageType, gid string, wire []byte) {
			r.publishBlindStoreEnvelope(mt, gid, wire)
		},
		OnAddCommitted:     r.makeAddCommittedHandler(groupID),
		OnPeerObserved:     r.makePeerObservedHandler(groupID),
		OnProposalObserved: r.makeProposalAuditHandler(groupID),
		OnCommitIssued:     r.makeCommitAuditHandler(groupID),
		OnForkHealEvent:    r.makeForkHealAuditHandler(groupID),
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
	localPeerID := r.node.Host.ID().String()
	_ = r.db.SetGroupCreatorPeerID(groupID, localPeerID)
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
		PeerID:      localPeerID,
		DisplayName: strings.TrimSpace(identity.DisplayName),
		Role:        "creator",
		Status:      store.GroupMemberStatusActive,
		Source:      "create",
		UpdatedAt:   time.Now().Unix(),
	})
	emitMembersChanged = true
	r.appendGroupEvent(groupID, groupEventTypeCreated, localPeerID, "", rec.Epoch, map[string]any{
		"creator_peer_id": localPeerID,
		"group_type":      normalizedGroupType,
		"category_id":     rec.CategoryID,
		"initial_epoch":   rec.Epoch,
	})
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
	if joined {
		active, err := database.IsGroupActive(groupID)
		if err != nil {
			return "", err
		}
		if !active {
			slog.Info("DM group was previously left. Purging stale metadata to recreate DM conversation", "group_id", groupID)
			if err := database.PurgeGroupMetadata(groupID); err != nil {
				return "", fmt.Errorf("purge stale DM metadata: %w", err)
			}
			joined = false
		}
	}
	if !joined {
		if err := r.CreateGroupChat(groupID, "dm", ""); err != nil && !strings.Contains(err.Error(), "already in group") {
			return "", err
		}
	}
	if err := database.SetDMCounterpartyPeerID(groupID, targetID.String()); err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("StartDirectMessage: persist DM counterparty failed", "group_id", groupID, "target", targetID.String(), "err", err)
	}

	r.mu.RLock()
	coord := r.coordinators[groupID]
	r.mu.RUnlock()
	if coord != nil {
		for _, member := range coord.ActiveMembers() {
			if member.String() == targetID.String() {
				_ = r.resendPendingWelcome(targetID, groupID, "dm", "")
				return groupID, nil
			}
		}
	}

	members, err := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
	if err == nil {
		for _, rec := range members {
			if rec.PeerID == targetID.String() {
				if resent := r.resendPendingWelcome(targetID, groupID, "dm", ""); resent {
					return groupID, nil
				}
				slog.Info("resendPendingWelcome failed, performing fresh InvitePeerToGroup", "group", groupID, "target", targetID.String())
				break
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
			counterpartyPeerID = strings.TrimSpace(rec.DMCounterpartyPeerID)
			if counterpartyPeerID == localPeerID {
				counterpartyPeerID = ""
			}
			memberDisplayName := ""
			members, err := database.ListGroupMembers(rec.GroupID, store.GroupMemberStatusActive)
			if err == nil {
				for _, m := range members {
					if strings.TrimSpace(m.PeerID) == "" {
						continue
					}
					if m.PeerID == localPeerID {
						continue
					}
					if counterpartyPeerID == "" {
						counterpartyPeerID = m.PeerID
						_ = database.SetDMCounterpartyPeerID(rec.GroupID, counterpartyPeerID)
					}
					if m.PeerID != counterpartyPeerID {
						continue
					}
					displayName := strings.TrimSpace(m.DisplayName)
					memberDisplayName = displayName
					break
				}
			}
			if counterpartyPeerID != "" {
				if memberDisplayName == "" {
					memberDisplayName = r.resolveDisplayNameForPeer(counterpartyPeerID)
				}
				title = memberDisplayName
				subtitle = counterpartyPeerID
				_, isCounterpartyOnline = online[counterpartyPeerID]
			}
		}

		counterpartyAvatar := ""
		if normalizedType == "dm" && counterpartyPeerID != "" {
			counterpartyAvatar = r.memberAvatarDataURL(counterpartyPeerID)
		}

		groupAvatarURL := ""
		if normalizedType == "group" {
			groupAvatarURL = r.groupChatAvatarDataURL(rec.GroupID)
		}

		var lastActivityAt int64
		if msgs, err := r.coordStorage.GetMessagesPaginated(rec.GroupID, 1, 0); err == nil && len(msgs) > 0 {
			lastActivityAt = msgs[0].Timestamp.WallTimeMs
		}

		result[i] = GroupInfo{
			GroupID:                   rec.GroupID,
			Epoch:                     rec.Epoch,
			MyRole:                    string(rec.MyRole),
			GroupType:                 normalizedType,
			CategoryID:                rec.CategoryID,
			ConversationTitle:         title,
			ConversationSub:           subtitle,
			ConversationAvatar:        avatarType,
			CounterpartyPeerID:        counterpartyPeerID,
			CounterpartyAvatarDataURL: counterpartyAvatar,
			GroupAvatarDataURL:        groupAvatarURL,
			IsCounterpartyOn:          isCounterpartyOnline,
			LastActivityAt:            lastActivityAt,
		}
	}
	return result, nil
}

func (r *Runtime) groupChatAvatarDataURL(groupID string) string {
	r.mu.RLock()
	database := r.db
	r.mu.RUnlock()
	if database == nil {
		return ""
	}
	hash, mime, _, err := database.GetGroupChatAvatarMeta(groupID)
	if err != nil || strings.TrimSpace(hash) == "" {
		return ""
	}
	blobMime, data, err := database.GetAvatarBlob(hash)
	if err != nil || len(data) == 0 {
		return ""
	}
	outMime := strings.TrimSpace(mime)
	if outMime == "" {
		outMime = blobMime
	}
	return avatarDataURL(outMime, data)
}

// SaveGroupChatAvatar updates the local image for a group-type MLS chat.
// avatarChange: 0 = no image change, 1 = replace with imageBytes, 2 = remove image.
func (r *Runtime) SaveGroupChatAvatar(groupID string, imageBytes []byte, avatarChange int) error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}
	r.mu.RLock()
	cs := r.coordStorage
	database := r.db
	r.mu.RUnlock()
	if cs == nil || database == nil {
		return fmt.Errorf("database not initialized")
	}
	rec, err := cs.GetGroupRecord(groupID)
	if err != nil {
		return fmt.Errorf("group not found: %w", err)
	}
	normalizedType, nerr := normalizeGroupTypeRuntime(rec.GroupType)
	if nerr != nil {
		return nerr
	}
	if normalizedType != "group" {
		return fmt.Errorf("group avatar is only supported for group chats, not %q", normalizedType)
	}
	if _, _, err := r.requireGroupPermission(groupID, permissionChangeGroupSetting); err != nil {
		return err
	}
	switch avatarChange {
	case 0:
		return nil
	case 1:
		if len(imageBytes) == 0 {
			return fmt.Errorf("avatar image bytes required when replacing group avatar")
		}
		mime, err := validateAvatarImageBytes(imageBytes)
		if err != nil {
			return err
		}
		hash := store.AvatarContentHash(imageBytes)
		if err := database.UpsertAvatarBlob(hash, mime, imageBytes); err != nil {
			return err
		}
		now := time.Now().Unix()
		if err := database.SetGroupChatAvatar(groupID, hash, mime, now); err != nil {
			return err
		}
	case 2:
		if err := database.ClearGroupChatAvatar(groupID); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid avatarChange %d", avatarChange)
	}
	go r.replicateGroupChatAvatarAfterLocalSave(groupID)
	go r.emitAllGroupsMembersChanged("group_avatar")
	return nil
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
	ctx, cancel := context.WithTimeout(context.Background(), generateKeyPackageTimeout)
	defer cancel()
	kp, bundle, err := r.mlsEngine.GenerateKeyPackage(ctx, identity.SigningKeyPrivate)
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

	// Do not hold r.mu Lock across coord.AddMember: the coordinator may call
	// OnEnvelopeBroadcast → publishBlindStoreEnvelope, which takes r.mu RLock.
	// Lock→RLock on the same goroutine deadlocks (sync.RWMutex is not re-entrant).
	r.mu.RLock()
	if r.coordinators == nil {
		r.mu.RUnlock()
		return "", fmt.Errorf("coordination stack not initialized")
	}
	coord, ok := r.coordinators[groupID]
	if !ok {
		r.mu.RUnlock()
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
					r.mu.RUnlock()
					return "", fmt.Errorf("direct message already has two members")
				}
			}
		}
	}
	r.mu.RUnlock()

	newMemberPeerID = strings.TrimSpace(newMemberPeerID)
	targetID, decErr := peer.Decode(newMemberPeerID)
	if decErr != nil {
		return "", fmt.Errorf("invalid peer ID %q: %w", newMemberPeerID, decErr)
	}
	kpHash := computeKeyPackageHash(raw)
	opID := coordinationAddOpID(groupID, newMemberPeerID, kpHash)
	result, err := coord.AddMember(coordination.AddMemberRequest{
		TargetPeerID:    targetID,
		KeyPackageBytes: raw,
		OperationID:     opID,
		KeyPackageHash:  hexDecodedOrNil(kpHash),
	})
	if err != nil {
		return "", err
	}
	if newMemberPeerID != "" {
		_ = r.upsertGroupMember(groupID, newMemberPeerID, "member", "add")
	}
	// On the deferred path this node was NOT the Token Holder. We never
	// fabricate a Welcome locally — that would require ephemeral key
	// material we do not own. Caller (legacy diagnostic UI / tests) gets
	// an empty welcomeHex meaning "proposal broadcast; Welcome will arrive
	// from the actual Token Holder out-of-band".
	return hex.EncodeToString(result.Welcome), nil
}

// coordinationAddOpID is a thin wrapper that mirrors ComputeAddOperationID in
// invite.go so callers in group.go don't need to reach into invite-internal
// helpers. Kept package-private to discourage random callers from inventing
// new operation ids.
func coordinationAddOpID(groupID, targetPeerID, kpHash string) string {
	return ComputeAddOperationID(groupID, targetPeerID, kpHash)
}

// JoinGroupWithWelcome joins an existing group using a Welcome message and the
// private KeyPackage bundle from [App.GenerateKeyPackage] for this invite flow.
func (r *Runtime) JoinGroupWithWelcome(groupID, welcomeHex, keyPackageBundlePrivateHex string) error {
	return r.joinGroupWithWelcome(groupID, welcomeHex, keyPackageBundlePrivateHex, "", "")
}

// resolveChannelCategoryForWelcomeJoin returns category_id already associated
// with this exact Welcome bytes on the invitee (pending_invites or
// stored_welcomes). Used when JoinGroupWithWelcome runs without an inline hint
// so replicated invite metadata still lands in mls_groups on first save.
func (r *Runtime) resolveChannelCategoryForWelcomeJoin(groupID string, welcomeRaw []byte, normalizedGroupType string) string {
	if !strings.EqualFold(strings.TrimSpace(normalizedGroupType), "channel") {
		return ""
	}
	if r.db == nil || r.node == nil {
		return ""
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || len(welcomeRaw) == 0 {
		return ""
	}
	id := store.PendingInviteID(groupID, welcomeRaw)
	if inv, err := r.db.GetPendingInvite(id); err == nil && inv != nil {
		if c := strings.TrimSpace(inv.CategoryID); c != "" {
			return c
		}
	}
	localPeer := r.node.Host.ID().String()
	wb, _, cat, _, err := r.db.GetStoredWelcome(localPeer, groupID)
	if err != nil || len(wb) == 0 {
		return ""
	}
	if !bytes.Equal(wb, welcomeRaw) {
		return ""
	}
	return strings.TrimSpace(cat)
}

func (r *Runtime) joinGroupWithWelcome(groupID, welcomeHex, keyPackageBundlePrivateHex, groupType, categoryIDHint string) error {
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
	if r.db != nil {
		if active, err := r.db.IsGroupActive(groupID); err == nil && !active {
			slog.Info("joinGroupWithWelcome: group was previously left. Purging stale metadata.", "group_id", groupID)
			_ = r.db.PurgeGroupMetadata(groupID)
		}
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
	emitCategoryAfterUnlock := ""
	runLeafBackfill := false
	defer func() {
		r.mu.Unlock()
		if emitMembersChanged {
			r.emit("group:members_changed", map[string]interface{}{
				"group_id": groupID,
				"reason":   "joined",
			})
		}
		if emitCategoryAfterUnlock != "" {
			r.emit("channel_categories:changed", map[string]interface{}{
				"reason":      "welcome_join",
				"channel_id":  groupID,
				"category_id": emitCategoryAfterUnlock,
			})
		}
		// MLS leaf enumeration runs after r.mu is released because the
		// helper takes its own RLock (sync.RWMutex is not re-entrant).
		// This guarantees Tester2 sees Tester1 (and every other leaf) in
		// the roster the instant JoinGroupWithWelcome returns, without
		// waiting for any of them to send a message.
		if runLeafBackfill {
			r.backfillMLSLeafRoster(groupID)
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

	hint := strings.TrimSpace(categoryIDHint)
	resolvedCat := hint
	if resolvedCat == "" && strings.EqualFold(normalizedGroupType, "channel") {
		resolvedCat = r.resolveChannelCategoryForWelcomeJoin(groupID, welcomeRaw, normalizedGroupType)
	}

	now := time.Now()
	if err := r.coordStorage.SaveGroupRecord(&coordination.GroupRecord{
		GroupID:    groupID,
		GroupState: groupState,
		Epoch:      epoch,
		TreeHash:   treeHash,
		MyRole:     coordination.RoleMember,
		GroupType:  normalizedGroupType,
		CategoryID: resolvedCat,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		return fmt.Errorf("save group record: %w", err)
	}
	if err := r.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     groupID,
		PeerID:      r.node.Host.ID().String(),
		DisplayName: strings.TrimSpace(identity.DisplayName),
		Role:        "member",
		Status:      store.GroupMemberStatusActive,
		Source:      "welcome",
		UpdatedAt:   time.Now().Unix(),
	}); err != nil {
		slog.Warn("Failed to persist local group member before coordinator start", "group", groupID, "error", err)
	}
	if _, err := r.reconcileGroupRosterWithMLS(groupID); err != nil {
		slog.Warn("Failed to backfill MLS roster before coordinator start", "group", groupID, "error", err)
	}

	coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
		Config:               coordination.DefaultConfig(),
		Transport:            r.transport,
		Clock:                coordination.RealClock{},
		MLS:                  r.mlsEngine,
		Storage:              r.coordStorage,
		LocalID:              r.node.Host.ID(),
		GroupID:              groupID,
		SigningKey:           identity.SigningKeyPrivate,
		GroupInfoFetcher:     r.fetchGroupInfoForHeal,
		AuthorizedCommitters: r.authorizedCommittersProvider(r.db),
		InitialActiveView:    r.initialActiveViewForGroupLocked(groupID),
		OnMessage:            r.makeMessageHandler(groupID),
		OnEpochChange:        r.makeEpochHandler(groupID),
		OnAccessLost:         r.makeAccessLostHandler(groupID),
		OnEnvelopeBroadcast: func(mt coordination.MessageType, gid string, wire []byte) {
			r.publishBlindStoreEnvelope(mt, gid, wire)
		},
		OnAddCommitted:     r.makeAddCommittedHandler(groupID),
		OnPeerObserved:     r.makePeerObservedHandler(groupID),
		OnProposalObserved: r.makeProposalAuditHandler(groupID),
		OnCommitIssued:     r.makeCommitAuditHandler(groupID),
		OnForkHealEvent:    r.makeForkHealAuditHandler(groupID),
	})
	if err != nil {
		return fmt.Errorf("create coordinator: %w", err)
	}
	if err := coord.Start(r.ctx); err != nil {
		return fmt.Errorf("start coordinator: %w", err)
	}
	r.coordinators[groupID] = coord
	if err := r.db.UpsertGroupMember(store.GroupMemberRecord{
		GroupID:     groupID,
		PeerID:      r.node.Host.ID().String(),
		DisplayName: strings.TrimSpace(identity.DisplayName),
		Role:        "member",
		Status:      store.GroupMemberStatusActive,
		Source:      "welcome",
		UpdatedAt:   time.Now().Unix(),
	}); err != nil {
		// Log but don't fail join
	}
	r.appendGroupEvent(groupID, groupEventTypeMemberJoined, r.node.Host.ID().String(), r.node.Host.ID().String(), epoch, map[string]any{
		"peer_id":      r.node.Host.ID().String(),
		"source":       "welcome",
		"group_type":   normalizedGroupType,
		"category_id":  resolvedCat,
		"joined_epoch": epoch,
	})
	emitMembersChanged = true
	runLeafBackfill = true
	if resolvedCat != "" && strings.EqualFold(normalizedGroupType, "channel") && hint == "" {
		emitCategoryAfterUnlock = resolvedCat
	}
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
	localPeerID := ""
	if node != nil {
		localPeerID = node.Host.ID().String()
	}
	localRole := store.GroupMemberRoleMember
	for _, rec := range members {
		if rec.PeerID == localPeerID && rec.Status == store.GroupMemberStatusActive {
			localRole = rec.Role
			break
		}
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
				// Profile refresh: caller is updating display_name only,
				// not asserting anything about role. Use the preserving
				// variant so we cannot ever clobber a creator row even by
				// echoing rec.Role back into the upsert.
				_ = database.UpsertGroupMemberPreservingRole(store.GroupMemberRecord{
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
		avatarURL := r.memberAvatarDataURL(rec.PeerID)
		role := r.resolveGroupMemberRole(groupID, rec.PeerID)
		if role != rec.Role {
			rec.Role = role
		}
		isCreator := isCreatorRole(rec.Role)
		isAdmin := isAdminRole(rec.Role)
		out = append(out, MemberInfo{
			PeerID:         rec.PeerID,
			DisplayName:    displayName,
			AvatarDataURL:  avatarURL,
			IsOnline:       isOn,
			Role:           rec.Role,
			IsAdmin:        isAdmin,
			IsCreator:      isCreator,
			CanManageAdmin: isCreatorRole(localRole) && !isCreator,
			CanRemove:      canRemoveMemberByRole(localRole, rec.Role, localPeerID == rec.PeerID),
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
		"commit_bytes_total":  snap.CommitBytesTotal,
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
	r.seedSelfPeerDirectoryRowLocked()
	r.loadExistingGroupsLocked()
}

// seedSelfPeerDirectoryRowLocked persists this node's own (peer_id,
// display_name, public_key_hex) tuple into peer_directory so MLS leaf
// enumeration can resolve the local leaf back to a peer.ID without falling
// through to a heartbeat self-observation (impossible — the node never
// heartbeats itself). This is idempotent: callers may invoke it on every
// coordination-stack init without churning the directory.
//
// Must be called with r.mu held.
func (r *Runtime) seedSelfPeerDirectoryRowLocked() {
	if r.db == nil || r.node == nil {
		return
	}
	identity, err := r.db.GetMLSIdentity()
	if err != nil || identity == nil {
		return
	}
	pubHex := ""
	if len(identity.PublicKey) > 0 {
		pubHex = hex.EncodeToString(identity.PublicKey)
	}
	_ = r.db.UpsertPeerProfileWithKey(
		r.node.Host.ID().String(),
		strings.TrimSpace(identity.DisplayName),
		pubHex,
	)
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
		r.ensureGroupRosterBackfilled(rec.GroupID)
		coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
			Config:               coordination.DefaultConfig(),
			Transport:            r.transport,
			Clock:                coordination.RealClock{},
			MLS:                  r.mlsEngine,
			Storage:              r.coordStorage,
			LocalID:              r.node.Host.ID(),
			GroupID:              rec.GroupID,
			SigningKey:           identity.SigningKeyPrivate,
			GroupInfoFetcher:     r.fetchGroupInfoForHeal,
			AuthorizedCommitters: r.authorizedCommittersProvider(r.db),
			InitialActiveView:    r.initialActiveViewForGroupLocked(rec.GroupID),
			OnMessage:            r.makeMessageHandler(rec.GroupID),
			OnEpochChange:        r.makeEpochHandler(rec.GroupID),
			OnAccessLost:         r.makeAccessLostHandler(rec.GroupID),
			OnEnvelopeBroadcast: func(mt coordination.MessageType, gid string, wire []byte) {
				r.publishBlindStoreEnvelope(mt, gid, wire)
			},
			OnAddCommitted:     r.makeAddCommittedHandler(rec.GroupID),
			OnPeerObserved:     r.makePeerObservedHandler(rec.GroupID),
			OnProposalObserved: r.makeProposalAuditHandler(rec.GroupID),
			OnCommitIssued:     r.makeCommitAuditHandler(rec.GroupID),
			OnForkHealEvent:    r.makeForkHealAuditHandler(rec.GroupID),
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

	r.recoverStaleAddOperationsLocked()
}

// initialActiveViewForGroupLocked seeds a coordinator with peers that are both
// transport-authenticated and active members of the MLS group. This avoids a
// misleading local-only ActiveView immediately after startup/join, before the
// first group heartbeat arrives.
//
// Must be called with r.mu held.
func (r *Runtime) initialActiveViewForGroupLocked(groupID string) []peer.ID {
	if r.db == nil || r.node == nil {
		return nil
	}

	verified := make(map[string]struct{})
	if r.node.AuthProtocol != nil {
		for _, pid := range r.node.AuthProtocol.VerifiedPeerIDs() {
			verified[pid.String()] = struct{}{}
		}
	}
	if len(verified) == 0 && r.transport != nil {
		for _, pid := range r.transport.ConnectedPeers() {
			if r.node.AuthProtocol != nil && !r.node.AuthProtocol.IsVerified(pid) {
				continue
			}
			verified[pid.String()] = struct{}{}
		}
	}

	local := r.node.Host.ID()
	out := []peer.ID{local}
	seen := map[peer.ID]struct{}{local: struct{}{}}
	members, err := r.db.ListGroupMembers(groupID, store.GroupMemberStatusActive)
	if err != nil {
		return out
	}
	for _, rec := range members {
		if _, ok := verified[rec.PeerID]; !ok {
			continue
		}
		pid, err := peer.Decode(rec.PeerID)
		if err != nil || pid == "" {
			continue
		}
		if _, exists := seen[pid]; exists {
			continue
		}
		seen[pid] = struct{}{}
		out = append(out, pid)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (r *Runtime) observeVerifiedPeerForGroups(pid peer.ID) {
	r.mu.RLock()
	database := r.db
	coords := make(map[string]*coordination.Coordinator, len(r.coordinators))
	for groupID, coord := range r.coordinators {
		coords[groupID] = coord
	}
	r.mu.RUnlock()

	if database == nil || pid == "" || len(coords) == 0 {
		return
	}
	for groupID, coord := range coords {
		r.ensureGroupRosterBackfilled(groupID)
		members, err := database.ListGroupMembers(groupID, store.GroupMemberStatusActive)
		if err != nil {
			continue
		}
		for _, rec := range members {
			if rec.PeerID == pid.String() {
				coord.ObservePeerAlive(pid)
				break
			}
		}
	}
}

// recoverStaleAddOperationsLocked surfaces any Add operation that was
// mid-flight when the process exited (proposal_broadcast / commit_observed /
// welcome_queued for more than the recovery threshold).
//
// We intentionally do NOT silently re-broadcast a stale ProposalAdd: the
// proposal bytes were authored against the pre-restart MLS epoch and would
// fail validation under the new epoch on every other peer. Instead we emit
// a diagnostic event so the UI can prompt the operator (or higher-level
// retry logic) to issue a fresh invite. Pending Welcomes still in the
// outbox (pending_welcomes_out) are retried by the existing connect-time
// retry path; this recovery loop only ensures the audit row stays in sync.
//
// Must be called with r.mu held.
func (r *Runtime) recoverStaleAddOperationsLocked() {
	if r.db == nil {
		return
	}
	const stallThresholdSeconds = 60
	ops, err := r.db.ListRecoverableAddOperations(stallThresholdSeconds)
	if err != nil || len(ops) == 0 {
		return
	}
	for _, op := range ops {
		slog.Warn("Stale group_add_operation observed at startup",
			"operation_id", op.OperationID,
			"group_id", op.GroupID,
			"target", op.TargetPeerID,
			"status", op.Status,
			"updated_at", op.UpdatedAt,
		)
		// Detach the emit from the locked section.
		op := op
		go r.emit("group:add_operation_stale", map[string]interface{}{
			"operation_id": op.OperationID,
			"group_id":     op.GroupID,
			"target":       op.TargetPeerID,
			"status":       op.Status,
		})
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
			// Observation only: the sender exists in the group. We have no
			// authoritative information about their role, so route through
			// the preserving variant — never overwrite a creator row that
			// might be authored for this peer locally.
			if err := r.upsertGroupMemberFromRosterSync(groupID, msg.SenderID.String(), "message"); err == nil {
				r.emit("group:members_changed", map[string]interface{}{
					"group_id": groupID,
					"reason":   "message_sender",
				})
			}
		}

		// Generate notifications for mentions and replies
		r.processNotificationsForMessage(msg)

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
		if changed, err := r.reconcileGroupRosterWithMLS(groupID); err == nil && changed {
			r.emit("group:members_changed", map[string]interface{}{
				"group_id": groupID,
				"reason":   "epoch_reconcile",
			})
		}
		r.emit("group:epoch", map[string]interface{}{
			"group_id": groupID,
			"epoch":    epoch,
		})
	}
}

// makeAddCommittedHandler wires the per-group OnAddCommitted callback.
//
// The callback fires in two distinct scenarios with different responsibilities:
//
//   - welcome != nil: the local node ran CreateCommit (Token Holder path).
//     It now owns the only copy of the ephemeral keys that authored the
//     Welcome bytes. The runtime persists pending_welcomes_out, replicates
//     to verified store peers, and fires direct-stream delivery so the
//     invitee can auto-join.
//
//   - welcome == nil: the local node observed a remote Commit referencing
//     an AddCommitDelivery on the group topic (any non-holder receiver).
//     We MUST NOT attempt to reconstruct a Welcome here — only the node
//     that ran CreateCommit has the ephemeral material. We only advance
//     the local group_add_operations row to commit_observed for audit/UI.
//
// Per the Single-Writer Protocol research notes: the Welcome is generated
// by the node that runs CreateCommit, and only that node may send it.
func (r *Runtime) makeAddCommittedHandler(groupID string) func(coordination.AddCommitDelivery, uint64, []byte) {
	return func(delivery coordination.AddCommitDelivery, commitEpoch uint64, welcome []byte) {
		r.mu.RLock()
		database := r.db
		r.mu.RUnlock()
		if database == nil || strings.TrimSpace(delivery.OperationID) == "" {
			return
		}

		welcomeHashHex := ""
		if len(delivery.WelcomeHash) > 0 {
			welcomeHashHex = hex.EncodeToString(delivery.WelcomeHash)
		}
		commitHash := []byte(nil)
		if len(delivery.WelcomeHash) > 0 {
			commitHash = nil // commit_hash is reserved for raw MLS commit fingerprint
		}

		if mErr := database.MarkAddCommitObserved(delivery.OperationID, commitEpoch, commitHash); mErr != nil &&
			!errors.Is(mErr, store.ErrAddOperationTerminal) {
			slog.Warn("MarkAddCommitObserved failed",
				"operation", delivery.OperationID, "group", groupID, "err", mErr)
		}
		role := "observer"
		if len(welcome) > 0 {
			role = "token_holder"
		}
		r.appendGroupEvent(groupID, groupEventTypeAddCommitObserved, "", delivery.TargetPeerID, commitEpoch, map[string]any{
			"operation_id":   delivery.OperationID,
			"target_peer_id": delivery.TargetPeerID,
			"request_id":     delivery.RequestID,
			"group_type":     delivery.GroupType,
			"category_id":    delivery.CategoryID,
			"welcome_hash":   welcomeHashHex,
			"role":           role,
		})

		// Observer path: nothing more to do beyond audit. The Token Holder
		// will deliver the Welcome out-of-band to the invitee.
		if len(welcome) == 0 {
			r.emit("group:add_committed", map[string]interface{}{
				"group_id":     groupID,
				"operation_id": delivery.OperationID,
				"target":       delivery.TargetPeerID,
				"epoch":        commitEpoch,
				"welcome_hash": welcomeHashHex,
				"role":         "observer",
			})
			return
		}

		// Token Holder path: dispatch Welcome through the outbox helper.
		r.dispatchTokenHolderWelcome(database, groupID, delivery, welcome)
		r.emit("group:add_committed", map[string]interface{}{
			"group_id":     groupID,
			"operation_id": delivery.OperationID,
			"target":       delivery.TargetPeerID,
			"epoch":        commitEpoch,
			"welcome_hash": welcomeHashHex,
			"role":         "token_holder",
		})
	}
}

// makePeerObservedHandler wires the per-group OnPeerObserved callback fired by
// the coordinator the first time a remote peer transitions absent→present in
// the ActiveView via a heartbeat (Phase A: heartbeat-driven roster sync).
//
// We use this callback as a self-healing complement to MLS leaf enumeration:
//   - When Welcome only persists (self, inviter), or when the joiner cannot
//     resolve every MLS leaf via the peer_directory (e.g. the peer has not
//     yet handshaked / verified), every heartbeat from that peer still gives
//     us a verified, online peer.ID we can upsert into group_members.
//   - We only fire once per (group, peer) transition — Phase A's job is not
//     to spam every ~5s heartbeat, only to surface fresh observations.
//
// The handler runs in a goroutine spawned by the coordinator, so it is safe
// to perform DB writes here without blocking heartbeat processing.
func (r *Runtime) makePeerObservedHandler(groupID string) func(string, peer.ID, time.Time) {
	return func(_ string, peerID peer.ID, _ time.Time) {
		peerStr := peerID.String()
		if peerStr == "" {
			return
		}
		r.mu.RLock()
		node := r.node
		r.mu.RUnlock()
		if node != nil && peerID == node.Host.ID() {
			return
		}
		// Heartbeat is a pure liveness signal. Caller has no opinion on the
		// peer's role, so use the preserving variant.
		if err := r.upsertGroupMemberFromRosterSync(groupID, peerStr, "heartbeat"); err != nil {
			slog.Debug("OnPeerObserved upsert failed",
				"group_id", groupID, "peer", peerStr, "err", err)
			return
		}
		r.emit("group:members_changed", map[string]interface{}{
			"group_id": groupID,
			"reason":   "peer_observed",
		})
	}
}

func (r *Runtime) makeAccessLostHandler(groupID string) func(string, uint64, string) {
	return func(_ string, epoch uint64, reason string) {
		if reason == "" {
			reason = "removed"
		}
		localPeerID := ""

		r.mu.Lock()
		var coordToStop interface{ Stop() }
		if r.coordinators != nil {
			if coord := r.coordinators[groupID]; coord != nil {
				coordToStop = coord
				delete(r.coordinators, groupID)
			}
		}
		r.mu.Unlock()

		if coordToStop != nil {
			coordToStop.Stop()
		}

		// Perform DB updates AFTER Stop() to ensure GossipSub topic is released
		// before we allow any re-join (which typically waits for IsGroupActive=false).
		r.mu.Lock()
		if r.db != nil {
			_ = r.db.MarkGroupLeft(groupID)
			if r.node != nil {
				localPeerID = r.node.Host.ID().String()
			} else if info, err := r.GetOnboardingInfo(); err == nil && info != nil {
				localPeerID = strings.TrimSpace(info.PeerID)
			}
			if localPeerID != "" {
				_ = r.db.MarkGroupMemberLeft(groupID, localPeerID, 0)
			}
		}
		r.mu.Unlock()

		r.emit("group:left", map[string]interface{}{
			"group_id": groupID,
			"reason":   reason,
			"epoch":    epoch,
		})
		if localPeerID != "" {
			r.appendGroupEvent(groupID, groupEventTypeMemberLeft, localPeerID, localPeerID, epoch, map[string]any{
				"peer_id": localPeerID,
				"reason":  "access_lost",
			})
		}
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
