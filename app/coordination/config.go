package coordination

import (
	"fmt"
	"time"
)

// CoordinatorConfig holds all tuneable parameters for the coordination protocol.
// Every timing-sensitive value is configurable so that tests can use aggressive
// timeouts (milliseconds) while production uses conservative defaults (seconds).
type CoordinatorConfig struct {
	// TokenHolderTimeout is how long nodes wait for the current Token Holder
	// to emit a Commit before evicting it from ActiveView and electing a new one.
	TokenHolderTimeout time.Duration

	// HeartbeatInterval is how often each node broadcasts a liveness heartbeat
	// and GroupStateAnnouncement to its group topic.
	HeartbeatInterval time.Duration

	// PeerDeadAfter is the number of consecutive missed heartbeats before a
	// peer is removed from ActiveView. Effective dead-peer timeout is
	// HeartbeatInterval * PeerDeadAfter.
	PeerDeadAfter int

	// MaxBatchedProposals caps how many Proposals the Token Holder bundles into
	// a single Commit. Prevents unbounded Commit size under high proposal load.
	MaxBatchedProposals int

	// KeyRotationInterval controls how often automatic Update Proposals are
	// generated for continuous key rotation (PCS). Set to 0 to disable
	// automatic rotation (useful in tests for manual control).
	KeyRotationInterval time.Duration

	// MetricsEnabled toggles recording of coordination metrics.
	// Should be true for benchmarks and evaluation; may be false in production
	// to reduce overhead if metrics are not needed.
	MetricsEnabled bool
}

// DefaultConfig returns production-ready defaults optimized for LAN/intranet
// where network latency is typically <1ms.
func DefaultConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		TokenHolderTimeout:  4 * time.Second,
		HeartbeatInterval:   5 * time.Second,
		PeerDeadAfter:       3,
		MaxBatchedProposals: 10,
		KeyRotationInterval: 5 * time.Minute,
		MetricsEnabled:      true,
	}
}

// TestConfig returns aggressive defaults for deterministic testing.
// Short timeouts ensure tests complete quickly; key rotation is disabled
// so tests control epoch transitions explicitly.
func TestConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		TokenHolderTimeout:  100 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
		PeerDeadAfter:       3,
		MaxBatchedProposals: 10,
		KeyRotationInterval: 0,
		MetricsEnabled:      true,
	}
}

// Validate checks that all configuration values are within acceptable bounds.
// Returns ErrInvalidConfig (wrapped with details) on failure.
func (c *CoordinatorConfig) Validate() error {
	if c.TokenHolderTimeout <= 0 {
		return fmt.Errorf("%w: TokenHolderTimeout must be positive, got %v",
			ErrInvalidConfig, c.TokenHolderTimeout)
	}
	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("%w: HeartbeatInterval must be positive, got %v",
			ErrInvalidConfig, c.HeartbeatInterval)
	}
	if c.PeerDeadAfter < 1 {
		return fmt.Errorf("%w: PeerDeadAfter must be >= 1, got %d",
			ErrInvalidConfig, c.PeerDeadAfter)
	}
	if c.MaxBatchedProposals < 1 {
		return fmt.Errorf("%w: MaxBatchedProposals must be >= 1, got %d",
			ErrInvalidConfig, c.MaxBatchedProposals)
	}
	if c.KeyRotationInterval < 0 {
		return fmt.Errorf("%w: KeyRotationInterval must be >= 0, got %v",
			ErrInvalidConfig, c.KeyRotationInterval)
	}
	return nil
}
