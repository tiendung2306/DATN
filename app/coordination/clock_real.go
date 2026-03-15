package coordination

import "time"

// RealClock implements Clock using the standard library time package.
// Used in production; tests use FakeClock instead.
type RealClock struct{}

func (RealClock) Now() time.Time                         { return time.Now() }
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
