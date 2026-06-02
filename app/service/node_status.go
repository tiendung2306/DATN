package service

import (
	"app/adapter/p2p"
)

// PeerInfo holds display information about a single connected peer.
type PeerInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Verified    bool   `json:"verified"`
}

// NodeStatus holds the full runtime status of the local P2P node.
type NodeStatus struct {
	State          string     `json:"state"`
	PeerID         string     `json:"peer_id"`
	DisplayName    string     `json:"display_name"`
	IsRunning      bool       `json:"is_running"`
	ConnectedPeers []PeerInfo `json:"connected_peers"`
}

// GetNodeStatus returns the current runtime status of the P2P node.
func (r *Runtime) GetNodeStatus() *NodeStatus {
	r.mu.RLock()
	db := r.db
	privKey := r.privKey
	node := r.node
	r.mu.RUnlock()

	status := &NodeStatus{
		State:          "ERROR",
		IsRunning:      node != nil,
		ConnectedPeers: []PeerInfo{},
	}

	if db != nil {
		if state, err := DetermineAppState(db); err == nil {
			status.State = state.String()
		}
	}

	if db != nil && privKey != nil {
		if info, err := p2p.GetOnboardingInfo(db, privKey); err == nil {
			status.PeerID = info.PeerID
		}
		if identity, err := db.GetMLSIdentity(); err == nil {
			status.DisplayName = identity.DisplayName
		}
	}

	if node != nil {
		for _, pid := range node.Host.Network().Peers() {
			peer := PeerInfo{ID: pid.String()}
			if node.AuthProtocol != nil {
				peer.Verified = node.AuthProtocol.IsVerified(pid)
				if tok := node.AuthProtocol.GetVerifiedToken(pid); tok != nil {
					peer.DisplayName = tok.DisplayName
				}
			}
			status.ConnectedPeers = append(status.ConnectedPeers, peer)
		}
	}

	return status
}

// GetKnownPeers returns a list of all historically encountered and currently connected peers.
func (r *Runtime) GetKnownPeers() []PeerInfo {
	r.mu.RLock()
	db := r.db
	node := r.node
	r.mu.RUnlock()

	outMap := make(map[string]PeerInfo)

	// 1. Load from DB persistent directory
	if db != nil {
		profiles, _ := db.GetAllPeerProfiles()
		for pid, name := range profiles {
			outMap[pid] = PeerInfo{
				ID:          pid,
				DisplayName: name,
				Verified:    false,
			}
		}
	}

	// 2. Overlay active connected peers
	if node != nil {
		for _, pid := range node.Host.Network().Peers() {
			verified := false
			displayName := ""
			if node.AuthProtocol != nil {
				verified = node.AuthProtocol.IsVerified(pid)
				if tok := node.AuthProtocol.GetVerifiedToken(pid); tok != nil {
					displayName = tok.DisplayName
				}
			}

			existing, exists := outMap[pid.String()]
			if exists {
				existing.Verified = verified
				if displayName != "" {
					existing.DisplayName = displayName
				}
				outMap[pid.String()] = existing
			} else {
				outMap[pid.String()] = PeerInfo{
					ID:          pid.String(),
					DisplayName: displayName,
					Verified:    verified,
				}
			}
		}
	}

	var list []PeerInfo
	for _, p := range outMap {
		list = append(list, p)
	}
	return list
}

func (r *Runtime) emitNodeStatusChanged(reason string) {
	status := r.GetNodeStatus()
	if status == nil {
		return
	}
	r.emit("node:status", map[string]interface{}{
		"reason":          reason,
		"state":           status.State,
		"peer_id":         status.PeerID,
		"display_name":    status.DisplayName,
		"is_running":      status.IsRunning,
		"connected_peers": status.ConnectedPeers,
	})
}

func (r *Runtime) emitAllGroupsMembersChanged(reason string) {
	r.mu.RLock()
	cs := r.coordStorage
	r.mu.RUnlock()
	if cs == nil {
		return
	}
	groups, err := cs.ListGroups()
	if err != nil {
		return
	}
	for _, g := range groups {
		r.emit("group:members_changed", map[string]interface{}{
			"group_id": g.GroupID,
			"reason":   reason,
		})
	}
}

func (r *Runtime) getAppStateUnlocked() string {
	if r.db == nil {
		return "ERROR"
	}
	state, err := DetermineAppState(r.db)
	if err != nil {
		return "ERROR"
	}
	return state.String()
}
