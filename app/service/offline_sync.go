package service

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (r *Runtime) registerOfflineSyncHandlers() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(p2p.OfflineSyncProtocol, func(s network.Stream) {
		go r.handleOfflineSyncStream(s)
	})
	r.node.Host.SetStreamHandler(p2p.OfflineDeliveryAckProtocol, func(s network.Stream) {
		go r.handleOfflineDeliveryAckStream(s)
	})
	slog.Info("Offline sync handlers registered")
}

func (r *Runtime) removeOfflineSyncHandlers() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(p2p.OfflineSyncProtocol)
	r.node.Host.RemoveStreamHandler(p2p.OfflineDeliveryAckProtocol)
}

func (r *Runtime) handleOfflineSyncStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(90 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	ap := r.node.AuthProtocol
	r.mu.Unlock()
	if ap != nil && !ap.IsVerified(remote) {
		slog.Warn("offline-sync: unverified peer", "peer", remote)
		return
	}

	r.mu.Lock()
	cs := r.coordStorage
	r.mu.Unlock()
	if cs == nil {
		return
	}

	var req p2p.OfflineSyncRequestV1
	if err := p2p.ReadOfflineJSONFrame(s, &req); err != nil || req.V != 1 {
		slog.Warn("offline-sync: bad request", "from", remote, "err", err)
		return
	}

	for _, g := range req.Groups {
		if g.GroupID == "" {
			continue
		}
		after := g.AfterSeq
		for {
			recs, err := cs.GetEnvelopesSince(g.GroupID, after, 50)
			if err != nil {
				slog.Warn("offline-sync: GetEnvelopesSince", "err", err)
				break
			}
			entries := make([]p2p.OfflineSyncEntryV1, 0, len(recs))
			for _, rec := range recs {
				entries = append(entries, p2p.OfflineSyncEntryV1{Seq: rec.Seq, Envelope: rec.Envelope})
			}
			batch := p2p.OfflineSyncBatchV1{
				GroupID: g.GroupID,
				Entries: entries,
				HasMore: len(recs) == 50,
			}
			if err := p2p.WriteOfflineJSONFrame(s, &batch); err != nil {
				return
			}
			if len(recs) == 0 {
				break
			}
			last := recs[len(recs)-1].Seq
			if len(recs) < 50 {
				break
			}
			after = last
		}
	}

	if err := p2p.WriteOfflineEndMarker(s); err != nil {
		return
	}

	var ack p2p.OfflineSyncAckV1
	if err := p2p.ReadOfflineJSONFrame(s, &ack); err != nil || ack.V != 1 {
		slog.Debug("offline-sync: missing or bad ack", "from", remote, "err", err)
		return
	}
	for _, ag := range ack.Groups {
		if ag.GroupID == "" {
			continue
		}
		if err := cs.RecordSyncAck(remote.String(), ag.GroupID, ag.AckedSeq); err != nil {
			slog.Warn("offline-sync: RecordSyncAck", "err", err)
		}
	}
}

func (r *Runtime) handleOfflineDeliveryAckStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	ap := r.node.AuthProtocol
	r.mu.Unlock()
	if ap != nil && !ap.IsVerified(remote) {
		slog.Warn("offline-delivery-ack: unverified peer", "peer", remote)
		return
	}

	var ack p2p.OfflineDeliveryAckV1
	if err := p2p.ReadOfflineJSONFrame(s, &ack); err != nil || ack.V != 1 {
		return
	}
	if ack.Recipient != remote.String() {
		slog.Warn("offline-delivery-ack: recipient mismatch", "peer", remote)
		return
	}

	r.mu.Lock()
	cs := r.coordStorage
	r.mu.Unlock()
	if cs == nil {
		return
	}
	for _, g := range ack.Groups {
		if g.GroupID == "" {
			continue
		}
		_ = cs.RecordSyncAck(ack.Recipient, g.GroupID, g.AckedSeq)
	}
}

func (r *Runtime) pullOfflineSyncFromPeer(remote peer.ID) {
	if err := r.pullOfflineSyncFromPeerOnce(remote); err != nil {
		slog.Debug("offline-sync: NewStream", "peer", remote, "err", err)
	}
}

// scheduleOfflineSyncPull retries a few times to handle the common race where
// the remote has connected but has not finished registering stream handlers yet.
func (r *Runtime) scheduleOfflineSyncPull(remote peer.ID) {
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

		err := r.pullOfflineSyncFromPeerOnce(remote)
		if err == nil {
			return
		}
		// Protocol negotiation failure is expected during early identify race.
		if strings.Contains(err.Error(), "protocols not supported") {
			continue
		}
		slog.Debug("offline-sync: pull attempt failed", "peer", remote, "err", err)
		return
	}
}

func (r *Runtime) pullOfflineSyncFromPeerOnce(remote peer.ID) error {
	r.mu.Lock()
	cs := r.coordStorage
	var groupIDs []string
	for gid := range r.coordinators {
		groupIDs = append(groupIDs, gid)
	}
	node := r.node
	r.mu.Unlock()

	if cs == nil || node == nil || len(groupIDs) == 0 {
		return nil
	}
	sort.Strings(groupIDs)

	req := p2p.OfflineSyncRequestV1{V: 1}
	for _, gid := range groupIDs {
		after, _ := cs.GetOfflinePullCursor(gid, remote.String())
		req.Groups = append(req.Groups, p2p.OfflineSyncRequestGroup{
			GroupID:  gid,
			AfterSeq: after,
		})
	}

	ctx, cancel := context.WithTimeout(r.appCtx(), 90*time.Second)
	defer cancel()

	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(ctx, remote)
	}

	s, err := node.Host.NewStream(ctx, remote, p2p.OfflineSyncProtocol)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(90 * time.Second))

	if err := p2p.WriteOfflineJSONFrame(s, &req); err != nil {
		return err
	}

	maxByGroup := make(map[string]int64)
	pendingBlobs := make(map[string][][]byte)

	for {
		var batch p2p.OfflineSyncBatchV1
		if err := p2p.ReadOfflineJSONFrame(s, &batch); err != nil {
			if errors.Is(err, p2p.ErrOfflineStreamEnd) {
				break
			}
			return err
		}
		for _, e := range batch.Entries {
			if e.Seq > maxByGroup[batch.GroupID] {
				maxByGroup[batch.GroupID] = e.Seq
			}
			pendingBlobs[batch.GroupID] = append(pendingBlobs[batch.GroupID], e.Envelope)
		}
	}

	r.mu.Lock()
	coords := make(map[string]*coordination.Coordinator, len(r.coordinators))
	for k, v := range r.coordinators {
		coords[k] = v
	}
	r.mu.Unlock()

	for gid, blobs := range pendingBlobs {
		coord := coords[gid]
		if coord == nil {
			continue
		}
		if _, err := coord.ReplayEnvelopes(blobs); err != nil {
			slog.Warn("offline-sync: ReplayEnvelopes", "group", gid, "err", err)
		}
		if m := maxByGroup[gid]; m > 0 {
			_ = cs.SetOfflinePullCursor(gid, remote.String(), m)
		}
	}

	ack := p2p.OfflineSyncAckV1{V: 1}
	for gid, m := range maxByGroup {
		if m <= 0 {
			continue
		}
		ack.Groups = append(ack.Groups, p2p.OfflineSyncAckGroupV1{GroupID: gid, AckedSeq: m})
	}
	sort.Slice(ack.Groups, func(i, j int) bool { return ack.Groups[i].GroupID < ack.Groups[j].GroupID })
	_ = p2p.WriteOfflineJSONFrame(s, &ack)
	return nil
}

func (r *Runtime) flushPendingDeliveryAcksTo(remote peer.ID) {
	r.mu.Lock()
	cs := r.coordStorage
	node := r.node
	r.mu.Unlock()
	if cs == nil || node == nil {
		return
	}
	rows, err := cs.ListPendingDeliveryAcksForTarget(remote.String())
	if err != nil || len(rows) == 0 {
		return
	}

	localID := node.Host.ID().String()
	ack := p2p.OfflineDeliveryAckV1{V: 1, Recipient: localID}
	for _, row := range rows {
		ack.Groups = append(ack.Groups, p2p.OfflineDeliveryAckGroupV1{
			GroupID:  row.GroupID,
			AckedSeq: row.AckedSeq,
		})
	}

	ctx, cancel := context.WithTimeout(r.appCtx(), 30*time.Second)
	defer cancel()
	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(ctx, remote)
	}
	s, err := node.Host.NewStream(ctx, remote, p2p.OfflineDeliveryAckProtocol)
	if err != nil {
		return
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(30 * time.Second))
	if err := p2p.WriteOfflineJSONFrame(s, &ack); err != nil {
		return
	}
	for _, row := range rows {
		_ = cs.DeletePendingDeliveryAck(row.ID)
	}
}

func (r *Runtime) offlineEnvelopeGCLoop(ctx context.Context) {
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	cfg := coordination.DefaultConfig()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.runOfflineEnvelopeGC(cfg)
		}
	}
}

func (r *Runtime) runOfflineEnvelopeGC(cfg *coordination.CoordinatorConfig) {
	r.mu.Lock()
	cs := r.coordStorage
	r.mu.Unlock()
	if cs == nil || cfg == nil {
		return
	}
	cutoff := time.Now().Add(-cfg.EnvelopeLogTTL).Unix()
	n, err := cs.PruneEnvelopes(cutoff, cfg.EnvelopeLogMaxPerGroup)
	if err != nil {
		slog.Warn("offline envelope GC", "err", err)
		return
	}
	if n > 0 {
		slog.Info("offline envelope GC pruned rows", "count", n)
	}
}

// TriggerOfflineSync pulls missed envelopes from all currently connected peers.
func (r *Runtime) TriggerOfflineSync() error {
	r.mu.Lock()
	tr := r.transport
	r.mu.Unlock()
	if tr == nil {
		return errors.New("transport not ready")
	}
	for _, p := range tr.ConnectedPeers() {
		go r.pullOfflineSyncFromPeer(p)
	}
	return nil
}

// OfflineSyncGroupStatus is per-group sync metadata for the UI.
type OfflineSyncGroupStatus struct {
	GroupID   string `json:"group_id"`
	LatestSeq int64  `json:"latest_seq"`
}

// GetOfflineSyncStatus returns envelope log heads (best-effort).
func (r *Runtime) GetOfflineSyncStatus() ([]OfflineSyncGroupStatus, error) {
	r.mu.Lock()
	cs := r.coordStorage
	var gids []string
	for gid := range r.coordinators {
		gids = append(gids, gid)
	}
	r.mu.Unlock()
	if cs == nil {
		return nil, errors.New("storage not ready")
	}
	sort.Strings(gids)
	out := make([]OfflineSyncGroupStatus, 0, len(gids))
	for _, gid := range gids {
		latest, _ := cs.GetLatestSeq(gid)
		out = append(out, OfflineSyncGroupStatus{GroupID: gid, LatestSeq: latest})
	}
	return out, nil
}
