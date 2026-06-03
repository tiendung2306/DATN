package service

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

const bootstrapOverrideConfigKey = "runtime_bootstrap_override"

type NetworkSettings struct {
	LocalPeerID    string `json:"local_peer_id"`
	LocalMultiaddr string `json:"local_multiaddr"`
	BootstrapAddr  string `json:"bootstrap_addr"`
	ConnectedPeers int    `json:"connected_peers"`
	VerifiedPeers  int    `json:"verified_peers"`
}

func (r *Runtime) ValidateMultiaddr(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fmt.Errorf("bootstrap address is required")
	}
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}
	info, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("multiaddr must include /p2p/PeerID: %w", err)
	}
	if info.ID == "" {
		return fmt.Errorf("multiaddr must include /p2p/PeerID")
	}
	return nil
}

func (r *Runtime) GetNetworkSettings() (NetworkSettings, error) {
	r.mu.RLock()
	bootstrapAddr := ""
	if r.cfg != nil {
		bootstrapAddr = strings.TrimSpace(r.cfg.BootstrapAddr)
	}
	node := r.node
	r.mu.RUnlock()

	out := NetworkSettings{
		BootstrapAddr: bootstrapAddr,
	}
	if node == nil {
		return out, nil
	}

	out.LocalPeerID = node.Host.ID().String()
	if addrs := node.Host.Addrs(); len(addrs) > 0 {
		out.LocalMultiaddr = fmt.Sprintf("%s/p2p/%s", addrs[0].String(), out.LocalPeerID)
	}
	peers := node.Host.Network().Peers()
	out.ConnectedPeers = len(peers)

	if node.AuthProtocol != nil {
		verified := 0
		for _, pid := range peers {
			if node.AuthProtocol.IsVerified(pid) {
				verified++
			}
		}
		out.VerifiedPeers = verified
	}
	return out, nil
}

func (r *Runtime) SetBootstrapAddress(addr string) error {
	if err := r.ValidateMultiaddr(addr); err != nil {
		return err
	}
	addr = strings.TrimSpace(addr)

	r.mu.Lock()
	r.cfg.BootstrapAddr = addr
	db := r.db
	r.mu.Unlock()

	if db != nil {
		if err := db.SetConfig(bootstrapOverrideConfigKey, []byte(addr)); err != nil {
			return fmt.Errorf("persist bootstrap override: %w", err)
		}
	}
	return nil
}

func (r *Runtime) ReconnectP2P() error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	r.mu.Lock()
	r.stopCoordinatorsLocked()
	r.stopNetworkLocked()
	r.mu.Unlock()
	if err := r.launchP2PNode(); err != nil {
		return err
	}
	r.setP2PStatus(true, "P2P node reconnected")
	return nil
}

func (r *Runtime) DisconnectP2P() error {
	r.mu.Lock()
	r.stopCoordinatorsLocked()
	r.stopNetworkLocked()
	r.mu.Unlock()
	r.setP2PStatus(false, "P2P node disconnected by operator")
	return nil
}

func (r *Runtime) ResumeP2P() error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	if err := r.launchP2PNode(); err != nil {
		return err
	}
	r.setP2PStatus(true, "P2P node resumed")
	return nil
}

func (r *Runtime) SetBlockedPeers(peerIDs []string) error {
	r.mu.Lock()
	node := r.node
	r.mu.Unlock()
	if node == nil {
		return fmt.Errorf("P2P node is not running")
	}

	var list []peer.ID
	for _, s := range peerIDs {
		pid, err := peer.Decode(s)
		if err != nil {
			return fmt.Errorf("invalid peer ID %q: %w", s, err)
		}
		list = append(list, pid)
	}

	node.SetBlockedPeers(list)

	// Force disconnect from any currently connected peers that are now blocked
	for _, pid := range list {
		_ = node.Host.Network().ClosePeer(pid)
	}

	return nil
}


type DiagnosticsGroupSnapshot struct {
	GroupID           string         `json:"group_id"`
	Epoch             uint64         `json:"epoch"`
	TokenHolder       string         `json:"token_holder"`
	TokenHolderPeerID string         `json:"token_holder_peer_id,omitempty"`
	ActiveMembers     int            `json:"active_members"`
	ActiveView        []string       `json:"active_view,omitempty"`
	TreeHashHex       string         `json:"tree_hash_hex,omitempty"`
	TreeHashShort     string         `json:"tree_hash_short,omitempty"`
	IsHealing         bool           `json:"is_healing"`
	OperationalMode   string         `json:"operational_mode,omitempty"`
	SyncRetryAttempts int            `json:"sync_retry_attempts"`
	PendingEnvelopes  int            `json:"pending_envelopes"`
	ReplayStateCounts map[string]int `json:"replay_state_counts,omitempty"`
	PendingOperations int            `json:"pending_operations"`
	OperationStatuses map[string]int `json:"operation_statuses,omitempty"`
}

type DiagnosticsSnapshot struct {
	TimestampMs               int64                      `json:"timestamp_ms"`
	AppState                  string                     `json:"app_state"`
	LocalPeerID               string                     `json:"local_peer_id,omitempty"`
	ConnectedPeers            int                        `json:"connected_peers"`
	VerifiedPeers             int                        `json:"verified_peers"`
	BootstrapAddr             string                     `json:"bootstrap_addr,omitempty"`
	OfflineSync               []OfflineSyncGroupStatus   `json:"offline_sync,omitempty"`
	Groups                    []DiagnosticsGroupSnapshot `json:"groups,omitempty"`
	RuntimeHealth             RuntimeHealth              `json:"runtime_health"`
	BlindStoreActive          bool                       `json:"blind_store_active"`
	RuntimeEventReplayEnabled bool                       `json:"runtime_event_replay_enabled"`
	RuntimeEventCursor        int64                      `json:"runtime_event_cursor"`
}

func (r *Runtime) GetDiagnosticsSnapshot() (DiagnosticsSnapshot, error) {
	settings, err := r.GetNetworkSettings()
	if err != nil {
		return DiagnosticsSnapshot{}, err
	}
	offline, _ := r.GetOfflineSyncStatus()
	runtimeHealth := r.GetRuntimeHealth()

	r.mu.RLock()
	db := r.db
	coordStorage := r.coordStorage
	blindStoreActive := r.blindStore != nil
	replayEnabled := r.cfg != nil && r.cfg.RuntimeEventReplay
	coords := make(map[string]*coordination.Coordinator, len(r.coordinators))
	for gid, coord := range r.coordinators {
		coords[gid] = coord
	}
	r.mu.RUnlock()

	appState := "ERROR"
	if db != nil {
		if state, err := DetermineAppState(db); err == nil {
			appState = state.String()
		}
	}

	snapshot := DiagnosticsSnapshot{
		TimestampMs:               time.Now().UnixMilli(),
		AppState:                  appState,
		LocalPeerID:               settings.LocalPeerID,
		ConnectedPeers:            settings.ConnectedPeers,
		VerifiedPeers:             settings.VerifiedPeers,
		BootstrapAddr:             settings.BootstrapAddr,
		OfflineSync:               offline,
		RuntimeHealth:             runtimeHealth,
		BlindStoreActive:          blindStoreActive,
		RuntimeEventReplayEnabled: replayEnabled,
	}
	if db != nil {
		if seq, seqErr := db.GetLatestRuntimeSeq(); seqErr == nil {
			snapshot.RuntimeEventCursor = seq
		}
	}
	if len(coords) == 0 {
		return snapshot, nil
	}

	groupIDs := make([]string, 0, len(coords))
	for gid := range coords {
		groupIDs = append(groupIDs, gid)
	}
	sort.Strings(groupIDs)

	for _, gid := range groupIDs {
		coord := coords[gid]
		if coord == nil {
			continue
		}

		tokenHolderID := ""
		if holder, err := coord.CurrentTokenHolder(); err == nil {
			tokenHolderID = holder.String()
		}

		var activeViewList []string
		for _, pid := range coord.ActiveMembers() {
			activeViewList = append(activeViewList, pid.String())
		}

		treeHashHex := hex.EncodeToString(coord.GetTreeHash())
		treeHashShort := ""
		if len(treeHashHex) > 8 {
			treeHashShort = treeHashHex[:8]
		} else {
			treeHashShort = treeHashHex
		}

		replayStateCounts := map[string]int{}
		pendingEnvelopes := 0
		if coordStorage != nil {
			if counts, countsErr := coordStorage.ListEnvelopeStateCounts(gid); countsErr == nil {
				replayStateCounts = counts
				for state, count := range counts {
					if coordination.IsPendingApplyState(state) {
						pendingEnvelopes += count
					}
				}
			}
		}

		operationStatuses := map[string]int{}
		pendingOperations := 0
		if coordStorage != nil {
			if ops, opsErr := coordStorage.ListPendingOperations(gid); opsErr == nil {
				pendingOperations = len(ops)
				for _, op := range ops {
					operationStatuses[op.Status]++
				}
			}
		}

		snapshot.Groups = append(snapshot.Groups, DiagnosticsGroupSnapshot{
			GroupID:           gid,
			Epoch:             coord.CurrentEpoch(),
			TokenHolder:       map[bool]string{true: "self", false: "other"}[coord.IsTokenHolder()],
			TokenHolderPeerID: tokenHolderID,
			ActiveMembers:     len(coord.ActiveMembers()),
			ActiveView:        activeViewList,
			TreeHashHex:       treeHashHex,
			TreeHashShort:     treeHashShort,
			IsHealing:         coord.IsHealing(),
			OperationalMode:   string(coord.GetOperationalMode()),
			SyncRetryAttempts: coord.GetSyncRetryAttempts(),
			PendingEnvelopes:  pendingEnvelopes,
			ReplayStateCounts: replayStateCounts,
			PendingOperations: pendingOperations,
			OperationStatuses: operationStatuses,
		})
	}
	return snapshot, nil
}

func (r *Runtime) ExportDiagnostics() (string, error) {
	snapshot, err := r.GetDiagnosticsSnapshot()
	if err != nil {
		return "", err
	}
	localDir := r.localDir()
	if err := os.MkdirAll(localDir, 0700); err != nil {
		return "", err
	}
	outPath := filepath.Join(localDir, fmt.Sprintf("diagnostics-%d.json", time.Now().Unix()))
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(outPath, raw, 0600); err != nil {
		return "", err
	}
	return outPath, nil
}

func (r *Runtime) OpenLogFolder() error {
	target := r.localDir()
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}
