package coordination

import (
	"sync"
	"time"
)

// FakeClock is a test-only Clock implementation that gives the test full
// control over time. Advance() moves the clock forward and triggers any
// pending After() timers whose deadline has been reached.
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

type waiter struct {
	deadline time.Time
	ch       chan time.Time
}

func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *FakeClock) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan time.Time, 1)
	deadline := f.now.Add(d)

	if !f.now.Before(deadline) {
		ch <- f.now
		return ch
	}

	f.waiters = append(f.waiters, waiter{deadline: deadline, ch: ch})
	return ch
}

// Advance moves the fake clock forward by d and fires any After() timers
// whose deadline has been reached.
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = f.now.Add(d)

	remaining := f.waiters[:0]
	for _, w := range f.waiters {
		if !f.now.Before(w.deadline) {
			w.ch <- f.now
		} else {
			remaining = append(remaining, w)
		}
	}
	f.waiters = remaining
}

// Set jumps the fake clock to an absolute time.
func (f *FakeClock) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = t

	remaining := f.waiters[:0]
	for _, w := range f.waiters {
		if !f.now.Before(w.deadline) {
			w.ch <- f.now
		} else {
			remaining = append(remaining, w)
		}
	}
	f.waiters = remaining
}
