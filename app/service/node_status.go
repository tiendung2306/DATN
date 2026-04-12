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
				}
			}
			status.ConnectedPeers = append(status.ConnectedPeers, peer)
		}
	}

	return status
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
