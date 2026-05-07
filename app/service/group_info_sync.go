package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (r *Runtime) registerGroupInfoHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(p2p.GroupInfoProtocol, func(s network.Stream) {
		go r.handleGroupInfoStream(s)
	})
	slog.Info("Group-info handler registered")
}

func (r *Runtime) removeGroupInfoHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(p2p.GroupInfoProtocol)
}

func (r *Runtime) handleGroupInfoStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	ap := r.node.AuthProtocol
	r.mu.Unlock()
	if ap != nil && !ap.IsVerified(remote) {
		slog.Warn("group-info: unverified peer", "peer", remote)
		return
	}

	var req p2p.GroupInfoRequestV1
	if err := p2p.ReadGroupInfoJSONFrame(s, &req); err != nil || req.V != 1 || strings.TrimSpace(req.GroupID) == "" {
		slog.Warn("group-info: bad request", "from", remote, "err", err)
		return
	}

	resp, err := r.exportLocalGroupInfo(req.GroupID, req.WithRatchetTree)
	if err != nil {
		resp = &p2p.GroupInfoResponseV1{
			V:       1,
			GroupID: req.GroupID,
			Error:   err.Error(),
		}
	}
	if err := p2p.WriteGroupInfoJSONFrame(s, resp); err != nil {
		slog.Debug("group-info: write response failed", "to", remote, "err", err)
	}
}

// exportLocalGroupInfo snapshots local branch data and asks MLSEngine to export
// verifiable GroupInfo. Used by the group-info stream handler (Sprint 2C) and
// by the future heal orchestrator caller path (Sprint 2D).
func (r *Runtime) exportLocalGroupInfo(groupID string, withRatchetTree bool) (*p2p.GroupInfoResponseV1, error) {
	snap, err := r.snapshotGroupForExport(groupID)
	if err != nil {
		return nil, err
	}
	groupInfo, err := snap.mls.ExportGroupInfo(context.Background(), snap.groupState, withRatchetTree)
	if err != nil {
		return nil, fmt.Errorf("ExportGroupInfo: %w", err)
	}
	return &p2p.GroupInfoResponseV1{
		V:         1,
		GroupID:   groupID,
		Epoch:     snap.epoch,
		TreeHash:  snap.treeHash,
		GroupInfo: groupInfo,
	}, nil
}

// requestGroupInfoFromPeer asks remote for GroupInfo over /app/group-info/1.0.0.
// Sprint 2D heal orchestrator will call this before ExternalJoin.
func (r *Runtime) requestGroupInfoFromPeer(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*p2p.GroupInfoResponseV1, error) {
	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil {
		return nil, errors.New("p2p node not ready")
	}

	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(ctx, remote)
	}
	s, err := node.Host.NewStream(ctx, remote, p2p.GroupInfoProtocol)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))

	req := p2p.GroupInfoRequestV1{
		V:               1,
		GroupID:         strings.TrimSpace(groupID),
		WithRatchetTree: withRatchetTree,
	}
	if req.GroupID == "" {
		return nil, errors.New("group_id is required")
	}
	if err := p2p.WriteGroupInfoJSONFrame(s, &req); err != nil {
		return nil, err
	}

	var resp p2p.GroupInfoResponseV1
	if err := p2p.ReadGroupInfoJSONFrame(s, &resp); err != nil {
		return nil, err
	}
	if resp.V != 1 {
		return nil, fmt.Errorf("unsupported response version: %d", resp.V)
	}
	if strings.TrimSpace(resp.Error) != "" {
		return nil, fmt.Errorf("remote group-info error: %s", resp.Error)
	}
	if strings.TrimSpace(resp.GroupID) != req.GroupID {
		return nil, fmt.Errorf("group-info group mismatch: got=%q want=%q", resp.GroupID, req.GroupID)
	}
	if len(resp.GroupInfo) == 0 {
		return nil, errors.New("empty group_info response")
	}
	return &resp, nil
}

type groupInfoExportSnapshot struct {
	mls        mlsExportClient
	groupState []byte
	treeHash   []byte
	epoch      uint64
}

type mlsExportClient interface {
	ExportGroupInfo(ctx context.Context, groupState []byte, withRatchetTree bool) (groupInfo []byte, err error)
}

func (r *Runtime) snapshotGroupForExport(groupID string) (*groupInfoExportSnapshot, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("group_id is required")
	}

	r.mu.RLock()
	coord := r.coordinators[groupID]
	mls := r.mlsEngine
	r.mu.RUnlock()

	if coord == nil {
		return nil, ErrGroupNotFound
	}
	if mls == nil {
		return nil, errors.New("mls engine not ready")
	}
	return &groupInfoExportSnapshot{
		mls:        mls,
		groupState: coord.GetGroupState(),
		treeHash:   append([]byte(nil), coord.GetTreeHash()...),
		epoch:      coord.CurrentEpoch(),
	}, nil
}

func (r *Runtime) fetchGroupInfoForHeal(ctx context.Context, remote peer.ID, groupID string, withRatchetTree bool) (*coordination.GroupInfoFetchResult, error) {
	resp, err := r.requestGroupInfoFromPeer(ctx, remote, groupID, withRatchetTree)
	if err != nil {
		return nil, err
	}
	return &coordination.GroupInfoFetchResult{
		GroupInfo: append([]byte(nil), resp.GroupInfo...),
		Epoch:     resp.Epoch,
		TreeHash:  append([]byte(nil), resp.TreeHash...),
	}, nil
}
