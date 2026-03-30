package main

import (
	"fmt"
	"log/slog"

	"app/coordination"
	"app/db"
	"app/p2p"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

// ─── Group chat operations ───────────────────────────────────────────────────

// CreateGroupChat creates a new MLS group, starts the Coordinator, and
// subscribes to the group's GossipSub topic. The group ID must be unique.
func (a *App) CreateGroupChat(groupID string) error {
	if groupID == "" {
		return fmt.Errorf("group ID is required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.node == nil {
		return fmt.Errorf("P2P node not running")
	}
	if a.mlsEngine == nil {
		return fmt.Errorf("crypto engine not available — build the Rust project first")
	}
	if a.coordinators == nil {
		return fmt.Errorf("coordination stack not initialized")
	}
	if _, exists := a.coordinators[groupID]; exists {
		return fmt.Errorf("already in group %q", groupID)
	}

	identity, err := a.db.GetMLSIdentity()
	if err != nil {
		return fmt.Errorf("get MLS identity: %w", err)
	}

	coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
		Config:        coordination.DefaultConfig(),
		Transport:     a.transport,
		Clock:         coordination.RealClock{},
		MLS:           a.mlsEngine,
		Storage:       a.coordStorage,
		LocalID:       a.node.Host.ID(),
		GroupID:       groupID,
		SigningKey:    identity.SigningKeyPrivate,
		OnMessage:     a.makeMessageHandler(groupID),
		OnEpochChange: a.makeEpochHandler(groupID),
	})
	if err != nil {
		return fmt.Errorf("create coordinator: %w", err)
	}

	if err := coord.CreateGroup(); err != nil {
		return fmt.Errorf("create group: %w", err)
	}
	if err := coord.Start(a.ctx); err != nil {
		return fmt.Errorf("start coordinator: %w", err)
	}

	a.coordinators[groupID] = coord
	slog.Info("Group chat created", "group_id", groupID)
	return nil
}

// SendGroupMessage encrypts and broadcasts a text message to the group.
func (a *App) SendGroupMessage(groupID string, text string) error {
	if text == "" {
		return nil
	}

	slog.Info("Sending group message", "group", groupID, "len", len(text))

	a.mu.Lock()
	coord, ok := a.coordinators[groupID]
	a.mu.Unlock()

	if !ok {
		return fmt.Errorf("not in group %q", groupID)
	}

	_, err := coord.SendMessage([]byte(text))
	return err
}

// GetGroupMessages returns all stored messages for a group, sorted by HLC.
func (a *App) GetGroupMessages(groupID string) ([]MessageInfo, error) {
	if a.coordStorage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	msgs, err := a.coordStorage.GetMessagesSince(groupID, coordination.HLCTimestamp{})
	if err != nil {
		return nil, err
	}

	var localID string
	a.mu.Lock()
	if a.node != nil {
		localID = string(a.node.Host.ID())
	}
	a.mu.Unlock()

	result := make([]MessageInfo, len(msgs))
	for i, m := range msgs {
		result[i] = MessageInfo{
			GroupID:   m.GroupID,
			Sender:    string(m.SenderID),
			Content:   string(m.Content),
			Timestamp: m.Timestamp.WallTimeMs,
			IsMine:    string(m.SenderID) == localID,
		}
	}
	return result, nil
}

// GetGroups returns all groups the local node has joined.
func (a *App) GetGroups() ([]GroupInfo, error) {
	if a.coordStorage == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	records, err := a.coordStorage.ListGroups()
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

// GetGroupStatus returns live status for a specific group.
func (a *App) GetGroupStatus(groupID string) map[string]interface{} {
	a.mu.Lock()
	coord, ok := a.coordinators[groupID]
	a.mu.Unlock()

	if !ok {
		return map[string]interface{}{"error": "not in group"}
	}

	snap := coord.GetMetrics()
	return map[string]interface{}{
		"group_id":             groupID,
		"epoch":                coord.CurrentEpoch(),
		"is_token_holder":      coord.IsTokenHolder(),
		"active_members":       len(coord.ActiveMembers()),
		"commits_issued":       snap.CommitsIssued,
		"proposals_received":   snap.ProposalsReceived,
		"messages_encrypted":   snap.CommitBytesTotal,
		"partitions_detected":  snap.PartitionsDetected,
	}
}

// ─── Coordination stack initialization ───────────────────────────────────────

// initCoordinationStackLocked sets up the shared transport, storage, and MLS
// engine after the P2P node is running. Must be called with a.mu held.
func (a *App) initCoordinationStackLocked() {
	if a.node == nil || a.db == nil {
		return
	}

	a.transport = p2p.NewLibP2PTransport(a.node.Host, a.node.PubSub)
	a.coordStorage = db.NewSQLiteCoordinationStorage(a.db)

	if a.mlsClient != nil {
		a.mlsEngine = coordination.NewGrpcMLSEngine(a.mlsClient)
	}

	a.coordinators = make(map[string]*coordination.Coordinator)
	a.loadExistingGroupsLocked()
}

// loadExistingGroupsLocked restores Coordinators for groups persisted in SQLite.
// Must be called with a.mu held.
func (a *App) loadExistingGroupsLocked() {
	if a.coordStorage == nil || a.node == nil {
		return
	}

	groups, err := a.coordStorage.ListGroups()
	if err != nil || len(groups) == 0 {
		return
	}

	identity, err := a.db.GetMLSIdentity()
	if err != nil {
		slog.Warn("Cannot load existing groups: no MLS identity", "error", err)
		return
	}

	for _, rec := range groups {
		coord, err := coordination.NewCoordinator(coordination.CoordinatorOpts{
			Config:        coordination.DefaultConfig(),
			Transport:     a.transport,
			Clock:         coordination.RealClock{},
			MLS:           a.mlsEngine,
			Storage:       a.coordStorage,
			LocalID:       a.node.Host.ID(),
			GroupID:       rec.GroupID,
			SigningKey:    identity.SigningKeyPrivate,
			OnMessage:     a.makeMessageHandler(rec.GroupID),
			OnEpochChange: a.makeEpochHandler(rec.GroupID),
		})
		if err != nil {
			slog.Warn("Failed to create coordinator for existing group",
				"group", rec.GroupID, "error", err)
			continue
		}
		if err := coord.Start(a.ctx); err != nil {
			slog.Warn("Failed to start coordinator for existing group",
				"group", rec.GroupID, "error", err)
			continue
		}
		a.coordinators[rec.GroupID] = coord
		slog.Info("Restored group from DB", "group_id", rec.GroupID, "epoch", rec.Epoch)
	}
}

// ─── Event handlers ──────────────────────────────────────────────────────────

func (a *App) makeMessageHandler(groupID string) func(*coordination.StoredMessage) {
	return func(msg *coordination.StoredMessage) {
		var isMine bool
		a.mu.Lock()
		if a.node != nil {
			isMine = msg.SenderID == a.node.Host.ID()
		}
		a.mu.Unlock()

		wailsRuntime.EventsEmit(a.ctx, "group:message", map[string]interface{}{
			"group_id":  msg.GroupID,
			"sender":    string(msg.SenderID),
			"content":   string(msg.Content),
			"timestamp": msg.Timestamp.WallTimeMs,
			"is_mine":   isMine,
		})
	}
}

func (a *App) makeEpochHandler(groupID string) func(uint64) {
	return func(epoch uint64) {
		wailsRuntime.EventsEmit(a.ctx, "group:epoch", map[string]interface{}{
			"group_id": groupID,
			"epoch":    epoch,
		})
	}
}

// ─── Teardown helpers ────────────────────────────────────────────────────────

// stopCoordinatorsLocked stops all running coordinators and closes transport.
// Must be called with a.mu held.
func (a *App) stopCoordinatorsLocked() {
	for id, coord := range a.coordinators {
		coord.Stop()
		slog.Info("Stopped coordinator", "group_id", id)
	}
	a.coordinators = nil

	if a.transport != nil {
		a.transport.Close()
		a.transport = nil
	}
}

// Ensure imports are used by the compiler.
var (
	_ = (*db.SQLiteCoordinationStorage)(nil)
	_ = (*p2p.LibP2PTransport)(nil)
)
