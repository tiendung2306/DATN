package coordination

import (
	"sync"
)

// ValidateEpoch compares an incoming message's epoch against the local epoch
// and returns the action the caller should take:
//
//   - ActionProcess:      msgEpoch == localEpoch → process normally
//   - ActionRejectStale:  msgEpoch <  localEpoch → sender is behind, notify them
//   - ActionBufferFuture: msgEpoch >  localEpoch → sender is ahead, buffer message
func ValidateEpoch(msgEpoch, localEpoch uint64) EpochAction {
	switch {
	case msgEpoch == localEpoch:
		return ActionProcess
	case msgEpoch < localEpoch:
		return ActionRejectStale
	default:
		return ActionBufferFuture
	}
}

// EpochTracker manages the local node's epoch state for a single group.
//
// It stores the current epoch number and the tree hash of the MLS group state
// at that epoch. After each successful Commit, the coordinator calls Advance()
// to update the tracker.
//
// Thread-safe: all methods may be called concurrently.
type EpochTracker struct {
	mu       sync.RWMutex
	epoch    uint64
	treeHash []byte

	// futureBuffer stores messages from higher epochs until we catch up.
	// Key = epoch number; value = list of raw envelopes.
	futureBuffer map[uint64][][]byte
}

// NewEpochTracker creates a tracker initialized at the given epoch.
func NewEpochTracker(initialEpoch uint64, treeHash []byte) *EpochTracker {
	th := make([]byte, len(treeHash))
	copy(th, treeHash)
	return &EpochTracker{
		epoch:        initialEpoch,
		treeHash:     th,
		futureBuffer: make(map[uint64][][]byte),
	}
}

// Current returns the current epoch number.
func (et *EpochTracker) Current() uint64 {
	et.mu.RLock()
	defer et.mu.RUnlock()
	return et.epoch
}

// TreeHash returns a copy of the current tree hash.
func (et *EpochTracker) TreeHash() []byte {
	et.mu.RLock()
	defer et.mu.RUnlock()
	cp := make([]byte, len(et.treeHash))
	copy(cp, et.treeHash)
	return cp
}

// Advance moves the tracker to a new epoch with a new tree hash.
// Returns any buffered messages for the new epoch (which can now be processed).
func (et *EpochTracker) Advance(newEpoch uint64, newTreeHash []byte) [][]byte {
	et.mu.Lock()
	defer et.mu.Unlock()

	et.epoch = newEpoch
	et.treeHash = make([]byte, len(newTreeHash))
	copy(et.treeHash, newTreeHash)

	buffered := et.futureBuffer[newEpoch]
	delete(et.futureBuffer, newEpoch)

	// Discard anything older than the new epoch
	for e := range et.futureBuffer {
		if e <= newEpoch {
			delete(et.futureBuffer, e)
		}
	}

	return buffered
}

// Validate checks an incoming message epoch against the current epoch
// and returns the appropriate action. If the action is ActionBufferFuture,
// the caller should use BufferFuture to store the raw envelope.
func (et *EpochTracker) Validate(msgEpoch uint64) EpochAction {
	et.mu.RLock()
	defer et.mu.RUnlock()
	return ValidateEpoch(msgEpoch, et.epoch)
}

// BufferFuture stores a raw message envelope for a future epoch.
// It is a no-op if the epoch is not actually ahead of the current one.
func (et *EpochTracker) BufferFuture(epoch uint64, rawEnvelope []byte) {
	et.mu.Lock()
	defer et.mu.Unlock()

	if epoch <= et.epoch {
		return
	}

	cp := make([]byte, len(rawEnvelope))
	copy(cp, rawEnvelope)
	et.futureBuffer[epoch] = append(et.futureBuffer[epoch], cp)
}

// FutureBufferSize returns the total number of messages buffered across all
// future epochs (useful for metrics and tests).
func (et *EpochTracker) FutureBufferSize() int {
	et.mu.RLock()
	defer et.mu.RUnlock()
	total := 0
	for _, msgs := range et.futureBuffer {
		total += len(msgs)
	}
	return total
}
