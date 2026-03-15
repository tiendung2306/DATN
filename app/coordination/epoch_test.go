package coordination

import (
	"testing"
)

func TestValidateEpoch_Match(t *testing.T) {
	if got := ValidateEpoch(5, 5); got != ActionProcess {
		t.Errorf("same epoch -> ActionProcess, got %d", got)
	}
}

func TestValidateEpoch_Stale(t *testing.T) {
	if got := ValidateEpoch(3, 5); got != ActionRejectStale {
		t.Errorf("sender behind -> ActionRejectStale, got %d", got)
	}
}

func TestValidateEpoch_Future(t *testing.T) {
	if got := ValidateEpoch(7, 5); got != ActionBufferFuture {
		t.Errorf("sender ahead -> ActionBufferFuture, got %d", got)
	}
}

func TestEpochTracker_Advance_ReturnsBuffered(t *testing.T) {
	et := NewEpochTracker(1, []byte("hash-1"))

	et.BufferFuture(2, []byte("msg-for-epoch-2-a"))
	et.BufferFuture(2, []byte("msg-for-epoch-2-b"))
	et.BufferFuture(3, []byte("msg-for-epoch-3"))

	if et.FutureBufferSize() != 3 {
		t.Errorf("expected 3 buffered, got %d", et.FutureBufferSize())
	}

	buffered := et.Advance(2, []byte("hash-2"))
	if len(buffered) != 2 {
		t.Errorf("Advance(2) should return 2 buffered msgs, got %d", len(buffered))
	}

	// epoch-3 messages should still be buffered
	if et.FutureBufferSize() != 1 {
		t.Errorf("expected 1 remaining, got %d", et.FutureBufferSize())
	}
}

func TestEpochTracker_Advance_DiscardsOldEpochs(t *testing.T) {
	et := NewEpochTracker(1, []byte("hash-1"))

	et.BufferFuture(2, []byte("msg-2"))
	et.BufferFuture(3, []byte("msg-3"))
	et.BufferFuture(5, []byte("msg-5"))

	// Jump directly to epoch 4 — epoch 2 and 3 become stale
	buffered := et.Advance(4, []byte("hash-4"))
	if len(buffered) != 0 {
		t.Errorf("no messages buffered for epoch 4, should get 0, got %d", len(buffered))
	}
	if et.FutureBufferSize() != 1 {
		t.Errorf("only epoch-5 should remain, got %d", et.FutureBufferSize())
	}
}

func TestEpochTracker_BufferFuture_IgnoresStale(t *testing.T) {
	et := NewEpochTracker(5, []byte("hash-5"))

	et.BufferFuture(3, []byte("stale-msg")) // epoch 3 < current 5
	et.BufferFuture(5, []byte("current"))   // epoch 5 == current 5

	if et.FutureBufferSize() != 0 {
		t.Errorf("stale/current messages should not be buffered, got %d", et.FutureBufferSize())
	}
}

func TestEpochTracker_Validate(t *testing.T) {
	et := NewEpochTracker(5, []byte("hash-5"))

	tests := []struct {
		name     string
		msgEpoch uint64
		want     EpochAction
	}{
		{"match", 5, ActionProcess},
		{"stale", 4, ActionRejectStale},
		{"future", 6, ActionBufferFuture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := et.Validate(tt.msgEpoch); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEpochTracker_TreeHash_DefensiveCopy(t *testing.T) {
	original := []byte("original-hash")
	et := NewEpochTracker(1, original)

	original[0] = 'X' // mutate caller's slice
	if et.TreeHash()[0] != 'o' {
		t.Error("constructor should make a defensive copy")
	}

	retrieved := et.TreeHash()
	retrieved[0] = 'Y' // mutate returned slice
	if et.TreeHash()[0] != 'o' {
		t.Error("TreeHash() should return a defensive copy")
	}
}

func TestEpochTracker_BufferFuture_DefensiveCopy(t *testing.T) {
	et := NewEpochTracker(1, []byte("h"))

	data := []byte("message-data")
	et.BufferFuture(2, data)
	data[0] = 'X'

	buffered := et.Advance(2, []byte("h2"))
	if buffered[0][0] != 'm' {
		t.Error("BufferFuture should make a defensive copy")
	}
}
