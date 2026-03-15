package coordination

import (
	"testing"
	"time"
)

func TestHLC_Now_MonotonicallyIncreasing(t *testing.T) {
	clk := NewFakeClock(time.UnixMilli(1000))
	h := NewHLC(clk, "node-A")

	ts1 := h.Now()
	ts2 := h.Now()
	ts3 := h.Now()

	if !ts1.Before(ts2) {
		t.Errorf("ts1 should be before ts2: %+v vs %+v", ts1, ts2)
	}
	if !ts2.Before(ts3) {
		t.Errorf("ts2 should be before ts3: %+v vs %+v", ts2, ts3)
	}
}

func TestHLC_Now_CounterResets_WhenWallTimeAdvances(t *testing.T) {
	clk := NewFakeClock(time.UnixMilli(1000))
	h := NewHLC(clk, "node-A")

	_ = h.Now() // c=0
	_ = h.Now() // c=1
	ts3 := h.Now()
	if ts3.Counter != 2 {
		t.Errorf("expected counter=2, got %d", ts3.Counter)
	}

	clk.Advance(1 * time.Millisecond) // wall time moves forward
	ts4 := h.Now()
	if ts4.Counter != 0 {
		t.Errorf("counter should reset to 0 when wall time advances, got %d", ts4.Counter)
	}
	if ts4.WallTimeMs != 1001 {
		t.Errorf("expected wall time 1001, got %d", ts4.WallTimeMs)
	}
}

func TestHLC_Update_CausalOrder_ClockBehind(t *testing.T) {
	clkA := NewFakeClock(time.UnixMilli(1000))
	clkB := NewFakeClock(time.UnixMilli(500)) // B's clock is 500ms behind

	hlcA := NewHLC(clkA, "node-A")
	hlcB := NewHLC(clkB, "node-B")

	tsA := hlcA.Now()
	tsB := hlcB.Update(tsA) // B receives A's message

	if !tsA.Before(tsB) {
		t.Errorf("B's receive ts must be after A's send ts:\n  A=%+v\n  B=%+v", tsA, tsB)
	}
	if tsB.WallTimeMs < tsA.WallTimeMs {
		t.Errorf("B's wall time should be >= A's: B.L=%d, A.L=%d",
			tsB.WallTimeMs, tsA.WallTimeMs)
	}
}

func TestHLC_Update_CausalOrder_ClockAhead(t *testing.T) {
	clkA := NewFakeClock(time.UnixMilli(500))
	clkB := NewFakeClock(time.UnixMilli(1000)) // B's clock is ahead

	hlcA := NewHLC(clkA, "node-A")
	hlcB := NewHLC(clkB, "node-B")

	tsA := hlcA.Now()
	tsB := hlcB.Update(tsA) // B receives A's message

	if !tsA.Before(tsB) {
		t.Errorf("B's receive ts must be after A's send ts:\n  A=%+v\n  B=%+v", tsA, tsB)
	}
	if tsB.WallTimeMs != 1000 {
		t.Errorf("B's wall time should use its own physical time: got %d", tsB.WallTimeMs)
	}
}

func TestHLC_Update_ThreeNodeCausalChain(t *testing.T) {
	clkA := NewFakeClock(time.UnixMilli(100))
	clkB := NewFakeClock(time.UnixMilli(200))
	clkC := NewFakeClock(time.UnixMilli(50))

	hlcA := NewHLC(clkA, "A")
	hlcB := NewHLC(clkB, "B")
	hlcC := NewHLC(clkC, "C")

	ts1 := hlcA.Now()       // A sends
	ts2 := hlcB.Update(ts1) // B receives from A, then replies
	ts3 := hlcB.Now()       // B sends reply
	ts4 := hlcC.Update(ts3) // C receives from B

	if !ts1.Before(ts2) {
		t.Error("ts2 should be after ts1")
	}
	if !ts2.Before(ts3) {
		t.Error("ts3 should be after ts2")
	}
	if !ts3.Before(ts4) {
		t.Error("ts4 should be after ts3")
	}
}

func TestHLCTimestamp_Before_Lexicographic(t *testing.T) {
	tests := []struct {
		name string
		a, b HLCTimestamp
		want bool
	}{
		{
			name: "different wall time",
			a:    HLCTimestamp{WallTimeMs: 100, Counter: 5, NodeID: "Z"},
			b:    HLCTimestamp{WallTimeMs: 200, Counter: 0, NodeID: "A"},
			want: true,
		},
		{
			name: "same wall time, different counter",
			a:    HLCTimestamp{WallTimeMs: 100, Counter: 0, NodeID: "Z"},
			b:    HLCTimestamp{WallTimeMs: 100, Counter: 1, NodeID: "A"},
			want: true,
		},
		{
			name: "same wall+counter, different nodeID",
			a:    HLCTimestamp{WallTimeMs: 100, Counter: 0, NodeID: "A"},
			b:    HLCTimestamp{WallTimeMs: 100, Counter: 0, NodeID: "B"},
			want: true,
		},
		{
			name: "equal timestamps",
			a:    HLCTimestamp{WallTimeMs: 100, Counter: 0, NodeID: "A"},
			b:    HLCTimestamp{WallTimeMs: 100, Counter: 0, NodeID: "A"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Before(tt.b); got != tt.want {
				t.Errorf("(%+v).Before(%+v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestHLCTimestamp_Equal(t *testing.T) {
	a := HLCTimestamp{WallTimeMs: 100, Counter: 1, NodeID: "X"}
	b := HLCTimestamp{WallTimeMs: 100, Counter: 1, NodeID: "X"}
	c := HLCTimestamp{WallTimeMs: 100, Counter: 2, NodeID: "X"}

	if !a.Equal(b) {
		t.Error("identical timestamps should be equal")
	}
	if a.Equal(c) {
		t.Error("different timestamps should not be equal")
	}
}

func TestHLCTimestamp_IsZero(t *testing.T) {
	var zero HLCTimestamp
	if !zero.IsZero() {
		t.Error("zero-value timestamp should be IsZero")
	}
	nonZero := HLCTimestamp{WallTimeMs: 1}
	if nonZero.IsZero() {
		t.Error("non-zero timestamp should not be IsZero")
	}
}
