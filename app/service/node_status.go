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
	r.mu.Lock()
	defer r.mu.Unlock()

	status := &NodeStatus{
		State:          r.getAppStateUnlocked(),
		IsRunning:      r.node != nil,
		ConnectedPeers: []PeerInfo{},
	}

	if r.db != nil && r.privKey != nil {
		if info, err := p2p.GetOnboardingInfo(r.db, r.privKey); err == nil {
			status.PeerID = info.PeerID
		}
		if identity, err := r.db.GetMLSIdentity(); err == nil {
			status.DisplayName = identity.DisplayName
		}
	}

	if r.node != nil {
		for _, pid := range r.node.Host.Network().Peers() {
			peer := PeerInfo{ID: pid.String()}
			if r.node.AuthProtocol != nil {
				peer.Verified = r.node.AuthProtocol.IsVerified(pid)
				if tok := r.node.AuthProtocol.GetVerifiedToken(pid); tok != nil {
					peer.DisplayName = tok.DisplayName
					if r.db != nil {
						_ = r.db.UpsertPeerProfile(pid.String(), tok.DisplayName)
						_ = r.db.UpdateGroupMemberDisplayNameByPeer(pid.String(), tok.DisplayName)
					}
				}
			}
			status.ConnectedPeers = append(status.ConnectedPeers, peer)
		}
	}

	return status
}

// GetKnownPeers returns a list of all historically encountered and currently connected peers.
func (r *Runtime) GetKnownPeers() []PeerInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	outMap := make(map[string]PeerInfo)

	// 1. Load from DB persistent directory
	if r.db != nil {
		profiles, _ := r.db.GetAllPeerProfiles()
		for pid, name := range profiles {
			outMap[pid] = PeerInfo{
				ID:          pid,
				DisplayName: name,
				Verified:    false,
			}
		}
	}

	// 2. Overlay active connected peers
	if r.node != nil {
		for _, pid := range r.node.Host.Network().Peers() {
			verified := false
			displayName := ""
			if r.node.AuthProtocol != nil {
				verified = r.node.AuthProtocol.IsVerified(pid)
				if tok := r.node.AuthProtocol.GetVerifiedToken(pid); tok != nil {
					displayName = tok.DisplayName
					if r.db != nil && displayName != "" {
						_ = r.db.UpsertPeerProfile(pid.String(), displayName)
						_ = r.db.UpdateGroupMemberDisplayNameByPeer(pid.String(), displayName)
					}
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
