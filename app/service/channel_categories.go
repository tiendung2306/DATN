package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const defaultChannelCategoryID = "cat-general"

type ChannelCategoryInfo struct {
	CategoryID string `json:"category_id"`
	Name       string `json:"name"`
	SortOrder  int    `json:"sort_order"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

func (r *Runtime) ListChannelCategories() ([]ChannelCategoryInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := r.ensureChannelCategoryBaselineLocked(); err != nil {
		return nil, err
	}
	rows, err := r.db.ListChannelCategories()
	if err != nil {
		return nil, err
	}
	out := make([]ChannelCategoryInfo, 0, len(rows))
	for _, rec := range rows {
		out = append(out, toCategoryInfo(rec))
	}
	return out, nil
}

func (r *Runtime) CreateChannelCategory(name string) (ChannelCategoryInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ChannelCategoryInfo{}, fmt.Errorf("category name is required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return ChannelCategoryInfo{}, err
	}

	r.mu.Lock()
	locked := true
	defer func() {
		if locked {
			r.mu.Unlock()
		}
	}()
	if r.db == nil {
		return ChannelCategoryInfo{}, fmt.Errorf("database not initialized")
	}
	if err := r.ensureChannelCategoryBaselineLocked(); err != nil {
		return ChannelCategoryInfo{}, err
	}

	id, err := newCategoryID()
	if err != nil {
		return ChannelCategoryInfo{}, err
	}
	localPeerID := ""
	if r.node != nil {
		localPeerID = r.node.Host.ID().String()
	}
	now := time.Now().Unix()
	rec := store.ChannelCategoryRecord{
		CategoryID: id,
		Name:       name,
		SortOrder:  int(now),
		CreatedBy:  localPeerID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := r.db.UpsertChannelCategory(rec); err != nil {
		return ChannelCategoryInfo{}, err
	}
	locked = false
	r.mu.Unlock()

	r.emit("channel_categories:changed", map[string]interface{}{"reason": "created", "category_id": id})
	r.broadcastChannelCategoryFrame(p2p.ChannelCategorySyncFrameV1{
		V:       1,
		Type:    "upsert_category",
		EventID: newCategoryEventID("upsert", id),
		Category: &p2p.ChannelCategoryWire{
			CategoryID: rec.CategoryID,
			Name:       rec.Name,
			SortOrder:  rec.SortOrder,
			UpdatedAt:  rec.UpdatedAt,
			CreatedBy:  rec.CreatedBy,
		},
	})
	return toCategoryInfo(rec), nil
}

func (r *Runtime) DeleteChannelCategory(categoryID string) error {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return fmt.Errorf("category ID is required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}

	r.mu.Lock()
	if r.db == nil {
		r.mu.Unlock()
		return fmt.Errorf("database not initialized")
	}
	count, err := r.db.CountActiveChannelsInCategory(categoryID)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	if count > 0 {
		r.mu.Unlock()
		return fmt.Errorf("ERR_CATEGORY_NOT_EMPTY: category still has active channels")
	}
	if err := r.db.DeleteChannelCategory(categoryID); err != nil {
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()

	r.emit("channel_categories:changed", map[string]interface{}{"reason": "deleted", "category_id": categoryID})
	r.broadcastChannelCategoryFrame(p2p.ChannelCategorySyncFrameV1{
		V:          1,
		Type:       "delete_category",
		EventID:    newCategoryEventID("delete", categoryID),
		CategoryID: categoryID,
	})
	return nil
}

func (r *Runtime) AssignChannelCategory(channelID, categoryID string) error {
	channelID = strings.TrimSpace(channelID)
	categoryID = strings.TrimSpace(categoryID)
	if channelID == "" {
		return fmt.Errorf("channel ID is required")
	}
	if categoryID == "" {
		return fmt.Errorf("category ID is required")
	}
	if err := r.ensureSessionActive(); err != nil {
		return err
	}

	r.mu.Lock()
	if r.db == nil {
		r.mu.Unlock()
		return fmt.Errorf("database not initialized")
	}
	if _, err := r.db.GetChannelCategory(categoryID); err != nil {
		r.mu.Unlock()
		return fmt.Errorf("ERR_CATEGORY_NOT_FOUND: %w", err)
	}
	if err := r.db.AssignCategoryToGroupWhenReady(channelID, categoryID); err != nil {
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()

	r.emit("channel_categories:changed", map[string]interface{}{"reason": "assigned", "category_id": categoryID, "channel_id": channelID})
	r.broadcastChannelCategoryFrame(p2p.ChannelCategorySyncFrameV1{
		V:          1,
		Type:       "assign_channel",
		EventID:    newCategoryEventID("assign", channelID),
		ChannelID:  channelID,
		CategoryID: categoryID,
	})
	return nil
}

func (r *Runtime) ensureChannelCategoryBaselineLocked() error {
	if r.db == nil {
		return nil
	}
	rows, err := r.db.ListChannelCategories()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		localPeerID := ""
		if r.node != nil {
			localPeerID = r.node.Host.ID().String()
		}
		now := time.Now().Unix()
		if err := r.db.UpsertChannelCategory(store.ChannelCategoryRecord{
			CategoryID: defaultChannelCategoryID,
			Name:       "General",
			SortOrder:  0,
			CreatedBy:  localPeerID,
			CreatedAt:  now,
			UpdatedAt:  now,
		}); err != nil {
			return err
		}
	}
	legacyChannels, err := r.db.ListActiveChannelsWithoutCategory()
	if err != nil {
		return err
	}
	for _, groupID := range legacyChannels {
		if err := r.db.AssignCategoryToGroupWhenReady(groupID, defaultChannelCategoryID); err != nil {
			slog.Warn("Failed to assign default category", "group_id", groupID, "error", err)
		}
	}
	return nil
}

func (r *Runtime) registerChannelCategorySyncHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(p2p.ChannelCategorySyncProtocol, func(s network.Stream) {
		go r.handleChannelCategorySyncStream(s)
	})
}

func (r *Runtime) removeChannelCategorySyncHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(p2p.ChannelCategorySyncProtocol)
}

func (r *Runtime) handleChannelCategorySyncStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(20 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.RLock()
	node := r.node
	db := r.db
	r.mu.RUnlock()
	if node == nil || db == nil {
		return
	}
	if node.AuthProtocol != nil && !node.AuthProtocol.IsVerified(remote) {
		return
	}

	var frame p2p.ChannelCategorySyncFrameV1
	if err := p2p.ReadChannelCategoryJSONFrame(s, &frame); err != nil {
		return
	}
	if frame.V != 1 {
		return
	}
	switch frame.Type {
	case "request_snapshot":
		cats, err := db.ListChannelCategories()
		if err != nil {
			return
		}
		assignments, err := db.ListChannelAssignments()
		if err != nil {
			return
		}
		out := p2p.ChannelCategorySyncFrameV1{
			V:    1,
			Type: "snapshot",
		}
		for _, rec := range cats {
			out.Categories = append(out.Categories, p2p.ChannelCategoryWire{
				CategoryID: rec.CategoryID,
				Name:       rec.Name,
				SortOrder:  rec.SortOrder,
				UpdatedAt:  rec.UpdatedAt,
				CreatedBy:  rec.CreatedBy,
			})
		}
		for _, rec := range assignments {
			out.Assignments = append(out.Assignments, p2p.ChannelAssignmentWire{
				ChannelID:  rec.ChannelID,
				CategoryID: rec.CategoryID,
			})
		}
		_ = p2p.WriteChannelCategoryJSONFrame(s, &out)
	case "snapshot":
		changed := false
		for _, item := range frame.Categories {
			if strings.TrimSpace(item.CategoryID) == "" || strings.TrimSpace(item.Name) == "" {
				continue
			}
			if err := db.UpsertChannelCategory(store.ChannelCategoryRecord{
				CategoryID: item.CategoryID,
				Name:       item.Name,
				SortOrder:  item.SortOrder,
				CreatedBy:  item.CreatedBy,
				UpdatedAt:  item.UpdatedAt,
				CreatedAt:  item.UpdatedAt,
			}); err == nil {
				changed = true
			}
		}
		for _, a := range frame.Assignments {
			if strings.TrimSpace(a.ChannelID) == "" || strings.TrimSpace(a.CategoryID) == "" {
				continue
			}
			if err := db.AssignCategoryToGroupWhenReady(a.ChannelID, a.CategoryID); err != nil {
				slog.Warn("category snapshot assign failed", "channel_id", a.ChannelID, "category_id", a.CategoryID, "err", err)
				continue
			}
			changed = true
		}
		if changed {
			r.emit("channel_categories:changed", map[string]interface{}{"reason": "snapshot_sync"})
		}
	case "assign_channel":
		if strings.TrimSpace(frame.EventID) == "" {
			return
		}
		if err := db.AssignCategoryToGroupWhenReady(frame.ChannelID, frame.CategoryID); err != nil {
			slog.Warn("category sync assign_channel failed", "channel_id", frame.ChannelID, "category_id", frame.CategoryID, "err", err)
			return
		}
		already, err := db.MarkCategorySyncEventApplied(frame.EventID)
		if err != nil || already {
			return
		}
		r.emit("channel_categories:changed", map[string]interface{}{"reason": "peer_sync"})
	case "upsert_category", "delete_category":
		if strings.TrimSpace(frame.EventID) == "" {
			return
		}
		already, err := db.MarkCategorySyncEventApplied(frame.EventID)
		if err != nil || already {
			return
		}
		switch frame.Type {
		case "upsert_category":
			if frame.Category == nil {
				return
			}
			_ = db.UpsertChannelCategory(store.ChannelCategoryRecord{
				CategoryID: frame.Category.CategoryID,
				Name:       frame.Category.Name,
				SortOrder:  frame.Category.SortOrder,
				CreatedBy:  frame.Category.CreatedBy,
				UpdatedAt:  frame.Category.UpdatedAt,
				CreatedAt:  frame.Category.UpdatedAt,
			})
		case "delete_category":
			_ = db.DeleteChannelCategory(frame.CategoryID)
		}
		r.emit("channel_categories:changed", map[string]interface{}{"reason": "peer_sync"})
	}
}

func (r *Runtime) scheduleChannelCategorySync(remote peer.ID) {
	backoffs := []time.Duration{250 * time.Millisecond, 900 * time.Millisecond, 1500 * time.Millisecond}
	for _, d := range backoffs {
		time.Sleep(d)
		r.mu.RLock()
		node := r.node
		r.mu.RUnlock()
		if node == nil {
			return
		}
		if node.Host.Network().Connectedness(remote) != network.Connected {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err := r.requestChannelCategorySnapshot(ctx, remote)
		cancel()
		if err == nil {
			return
		}
	}
}

func (r *Runtime) requestChannelCategorySnapshot(ctx context.Context, remote peer.ID) error {
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil {
		return fmt.Errorf("p2p node not ready")
	}
	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(ctx, remote)
	}
	s, err := node.Host.NewStream(ctx, remote, p2p.ChannelCategorySyncProtocol)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(20 * time.Second))
	if err := p2p.WriteChannelCategoryJSONFrame(s, &p2p.ChannelCategorySyncFrameV1{
		V:    1,
		Type: "request_snapshot",
	}); err != nil {
		return err
	}
	var resp p2p.ChannelCategorySyncFrameV1
	if err := p2p.ReadChannelCategoryJSONFrame(s, &resp); err != nil {
		return err
	}
	if resp.V != 1 || resp.Type != "snapshot" {
		return fmt.Errorf("invalid category snapshot response")
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	changed := false
	for _, item := range resp.Categories {
		if strings.TrimSpace(item.CategoryID) == "" || strings.TrimSpace(item.Name) == "" {
			continue
		}
		if err := db.UpsertChannelCategory(store.ChannelCategoryRecord{
			CategoryID: item.CategoryID,
			Name:       item.Name,
			SortOrder:  item.SortOrder,
			CreatedBy:  item.CreatedBy,
			UpdatedAt:  item.UpdatedAt,
			CreatedAt:  item.UpdatedAt,
		}); err == nil {
			changed = true
		}
	}
	for _, a := range resp.Assignments {
		if strings.TrimSpace(a.ChannelID) == "" || strings.TrimSpace(a.CategoryID) == "" {
			continue
		}
		if err := db.AssignCategoryToGroupWhenReady(a.ChannelID, a.CategoryID); err != nil {
			slog.Warn("category snapshot pull assign failed", "channel_id", a.ChannelID, "category_id", a.CategoryID, "err", err)
			continue
		}
		changed = true
	}
	if changed {
		r.emit("channel_categories:changed", map[string]interface{}{"reason": "snapshot_pull"})
	}
	return nil
}

func (r *Runtime) broadcastChannelCategoryFrame(frame p2p.ChannelCategorySyncFrameV1) {
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil {
		return
	}
	peers := node.Host.Network().Peers()
	for _, pid := range peers {
		go func(target peer.ID) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			if node.AuthProtocol != nil {
				node.AuthProtocol.InitiateHandshake(ctx, target)
			}
			s, err := node.Host.NewStream(ctx, target, p2p.ChannelCategorySyncProtocol)
			if err != nil {
				return
			}
			defer s.Close()
			_ = s.SetDeadline(time.Now().Add(20 * time.Second))
			_ = p2p.WriteChannelCategoryJSONFrame(s, &frame)
		}(pid)
	}
}

func toCategoryInfo(rec store.ChannelCategoryRecord) ChannelCategoryInfo {
	return ChannelCategoryInfo{
		CategoryID: rec.CategoryID,
		Name:       rec.Name,
		SortOrder:  rec.SortOrder,
		CreatedBy:  rec.CreatedBy,
		CreatedAt:  rec.CreatedAt,
		UpdatedAt:  rec.UpdatedAt,
	}
}

func newCategoryID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate category id: %w", err)
	}
	return "cat-" + hex.EncodeToString(b[:]), nil
}

func newCategoryEventID(prefix, entityID string) string {
	return fmt.Sprintf("%s:%s:%d", prefix, strings.TrimSpace(entityID), time.Now().UnixNano())
}
