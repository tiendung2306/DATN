package coordination

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// MaxClockDriftMs defines the maximum tolerable physical clock drift in milliseconds (5 seconds).
const MaxClockDriftMs = 5000

// ErrClockDrift is returned when the physical component of a received HLC timestamp
// exceeds the local physical time by more than MaxClockDriftMs, indicating clock poisoning.
var ErrClockDrift = errors.New("hlc: clock drift limit exceeded")

// HLC implements a Hybrid Logical Clock as described by Kulkarni et al. (2014).
//
// It combines a physical wall-clock component with a logical counter to provide
// causally consistent, totally ordered timestamps without requiring synchronized
// clocks across nodes. This is critical for air-gapped networks where NTP may
// not be available.
//
// The HLC uses the injectable Clock interface, making it fully deterministic
// when paired with a FakeClock in tests.
//
// Thread-safe: all methods may be called concurrently.
type HLC struct {
	clock Clock
	mu    sync.Mutex
	l     int64  // latest known wall time (unix milliseconds)
	c     uint32 // logical counter for events at the same l
	id    string // node identifier for deterministic tiebreaking
}

// NewHLC creates a new Hybrid Logical Clock for the given node.
// The clock parameter provides wall-clock time; nodeID is typically peer.ID.String().
func NewHLC(clock Clock, nodeID string) *HLC {
	return &HLC{
		clock: clock,
		id:    nodeID,
	}
}

// Now generates an HLC timestamp for a local event (e.g., sending a message).
func (h *HLC) Now() HLCTimestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	pt := h.clock.Now().UnixMilli()

	if pt > h.l {
		h.l = pt
		h.c = 0
	} else {
		// Enforce Logical Counter Overflow Protection
		if h.c >= 0xFFFFFFFE { // Counter close to max
			// Sleep for 1ms to let physical time tick forward
			time.Sleep(1 * time.Millisecond)
			pt = h.clock.Now().UnixMilli()
			h.l = max2(h.l+1, pt) // guarantee l increases
			h.c = 0
		} else {
			h.c++
		}
	}

	return HLCTimestamp{
		WallTimeMs: h.l,
		Counter:    h.c,
		NodeID:     h.id,
	}
}

// Update merges a received HLC timestamp into the local clock state and
// returns the resulting timestamp for the receive event.
func (h *HLC) Update(received HLCTimestamp) (HLCTimestamp, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	pt := h.clock.Now().UnixMilli()
	msgL := received.WallTimeMs

	// Enforce Clock Drift Boundary Check to prevent clock poisoning
	if msgL-pt > MaxClockDriftMs {
		return HLCTimestamp{}, fmt.Errorf("%w: received %d is %dms ahead of physical %d",
			ErrClockDrift, msgL, msgL-pt, pt)
	}

	// Enforce Logical Counter Overflow Protection in Update
	if h.c >= 0xFFFFFFFE {
		time.Sleep(1 * time.Millisecond)
		pt = h.clock.Now().UnixMilli()
		h.l = max2(h.l+1, pt)
		h.c = 0
	}

	newL := max3(h.l, msgL, pt)

	switch {
	case newL == h.l && newL == msgL:
		h.c = maxU32(h.c, received.Counter) + 1
	case newL == h.l:
		h.c++
	case newL == msgL:
		h.c = received.Counter + 1
	default:
		h.c = 0
	}

	h.l = newL

	return HLCTimestamp{
		WallTimeMs: h.l,
		Counter:    h.c,
		NodeID:     h.id,
	}, nil
}

func max2(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}

func max3(a, b, c int64) int64 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func maxU32(a, b uint32) uint32 {
	if b > a {
		return b
	}
	return a
}
