package coordination

import (
	"sort"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ActiveView tracks the set of online peers for a group.
//
// Peers are added when they send a heartbeat and evicted after missing
// PeerDeadAfter consecutive liveness checks. The local node is always
// present and cannot be evicted.
//
// When the view changes (member added or evicted), the onChange callback
// fires so that the Token Holder can be recomputed.
//
// Thread-safe: all methods may be called concurrently.
type ActiveView struct {
	mu       sync.RWMutex
	clock    Clock
	cfg      *CoordinatorConfig
	localID  peer.ID
	members  map[peer.ID]*memberState
	onChange func(members []peer.ID) // called with lock released
}

type memberState struct {
	missedChecks int
}

// NewActiveView creates an ActiveView containing only the local node.
// onChange is invoked (in a separate goroutine) whenever the member set changes;
// it receives the current sorted member list. May be nil if no callback is needed.
func NewActiveView(clock Clock, cfg *CoordinatorConfig, localID peer.ID, onChange func([]peer.ID)) *ActiveView {
	av := &ActiveView{
		clock:    clock,
		cfg:      cfg,
		localID:  localID,
		members:  make(map[peer.ID]*memberState),
		onChange: onChange,
	}
	av.members[localID] = &memberState{}
	return av
}

// RecordHeartbeat marks a peer as alive and resets its missed-check counter.
// If the peer was not previously in the view, it is added and onChange fires.
func (av *ActiveView) RecordHeartbeat(id peer.ID) {
	av.mu.Lock()
	ms, exists := av.members[id]
	if exists {
		ms.missedChecks = 0
		av.mu.Unlock()
		return
	}
	av.members[id] = &memberState{}
	members := av.sortedMembersLocked()
	av.mu.Unlock()

	if av.onChange != nil {
		av.onChange(members)
	}
}

// CheckLiveness increments the missed-check counter for all remote peers
// and evicts those exceeding PeerDeadAfter. Returns the list of evicted peers.
//
// Should be called periodically (e.g., every HeartbeatInterval).
// The local node is never evicted.
func (av *ActiveView) CheckLiveness() []peer.ID {
	av.mu.Lock()
	var evicted []peer.ID
	for id, ms := range av.members {
		if id == av.localID {
			continue
		}
		ms.missedChecks++
		if ms.missedChecks >= av.cfg.PeerDeadAfter {
			delete(av.members, id)
			evicted = append(evicted, id)
		}
	}

	var members []peer.ID
	if len(evicted) > 0 {
		members = av.sortedMembersLocked()
	}
	av.mu.Unlock()

	if len(evicted) > 0 && av.onChange != nil {
		av.onChange(members)
	}
	return evicted
}

// Evict forcibly removes a peer from the view (e.g., Token Holder timeout).
// No-op if the peer is not present or is the local node.
func (av *ActiveView) Evict(id peer.ID) {
	if id == av.localID {
		return
	}
	av.mu.Lock()
	_, exists := av.members[id]
	if !exists {
		av.mu.Unlock()
		return
	}
	delete(av.members, id)
	members := av.sortedMembersLocked()
	av.mu.Unlock()

	if av.onChange != nil {
		av.onChange(members)
	}
}

// Members returns a sorted snapshot of the current member peer IDs.
func (av *ActiveView) Members() []peer.ID {
	av.mu.RLock()
	defer av.mu.RUnlock()
	return av.sortedMembersLocked()
}

// Contains returns true if the peer is in the current view.
func (av *ActiveView) Contains(id peer.ID) bool {
	av.mu.RLock()
	defer av.mu.RUnlock()
	_, ok := av.members[id]
	return ok
}

// Size returns the number of peers in the current view.
func (av *ActiveView) Size() int {
	av.mu.RLock()
	defer av.mu.RUnlock()
	return len(av.members)
}

// sortedMembersLocked returns sorted peer IDs. Caller must hold at least a read lock.
// Sorting ensures deterministic iteration order across all nodes.
func (av *ActiveView) sortedMembersLocked() []peer.ID {
	result := make([]peer.ID, 0, len(av.members))
	for id := range av.members {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}
