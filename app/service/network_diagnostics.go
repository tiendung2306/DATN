package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

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
	defer r.mu.RUnlock()

	out := NetworkSettings{
		BootstrapAddr: strings.TrimSpace(r.cfg.BootstrapAddr),
	}
	if r.node == nil {
		return out, nil
	}

	out.LocalPeerID = r.node.Host.ID().String()
	if addrs := r.node.Host.Addrs(); len(addrs) > 0 {
		out.LocalMultiaddr = fmt.Sprintf("%s/p2p/%s", addrs[0].String(), out.LocalPeerID)
	}
	peers := r.node.Host.Network().Peers()
	out.ConnectedPeers = len(peers)

	if r.node.AuthProtocol != nil {
		verified := 0
		for _, pid := range peers {
			if r.node.AuthProtocol.IsVerified(pid) {
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

type DiagnosticsGroupSnapshot struct {
	GroupID       string `json:"group_id"`
	Epoch         uint64 `json:"epoch"`
	TokenHolder   string `json:"token_holder"`
	ActiveMembers int    `json:"active_members"`
	TreeHashShort string `json:"tree_hash_short,omitempty"`
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

	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := DiagnosticsSnapshot{
		TimestampMs:               time.Now().UnixMilli(),
		AppState:                  r.getAppStateUnlocked(),
		LocalPeerID:               settings.LocalPeerID,
		ConnectedPeers:            settings.ConnectedPeers,
		VerifiedPeers:             settings.VerifiedPeers,
		BootstrapAddr:             settings.BootstrapAddr,
		OfflineSync:               offline,
		RuntimeHealth:             r.GetRuntimeHealth(),
		BlindStoreActive:          r.blindStore != nil,
		RuntimeEventReplayEnabled: r.cfg != nil && r.cfg.RuntimeEventReplay,
	}
	if r.db != nil {
		if seq, seqErr := r.db.GetLatestRuntimeSeq(); seqErr == nil {
			snapshot.RuntimeEventCursor = seq
		}
	}
	if r.coordinators == nil {
		return snapshot, nil
	}

	groupIDs := make([]string, 0, len(r.coordinators))
	for gid := range r.coordinators {
		groupIDs = append(groupIDs, gid)
	}
	sort.Strings(groupIDs)

	for _, gid := range groupIDs {
		coord := r.coordinators[gid]
		snapshot.Groups = append(snapshot.Groups, DiagnosticsGroupSnapshot{
			GroupID:       gid,
			Epoch:         coord.CurrentEpoch(),
			TokenHolder:   map[bool]string{true: "self", false: "other"}[coord.IsTokenHolder()],
			ActiveMembers: len(coord.ActiveMembers()),
		})
	}
	return snapshot, nil
}

func (r *Runtime) ExportDiagnostics() (string, error) {
	snapshot, err := r.GetDiagnosticsSnapshot()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(".local", 0700); err != nil {
		return "", err
	}
	outPath := filepath.Join(".local", fmt.Sprintf("diagnostics-%d.json", time.Now().Unix()))
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
	target := ".local"
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}
