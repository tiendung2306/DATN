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
	// to its group topic. Used by ActiveView to detect dead peers.
	HeartbeatInterval time.Duration

	// AnnounceInterval is how often each node broadcasts a
	// GroupStateAnnouncement (TreeHash + MemberCount + CommitHash) so that
	// peers can detect partitions via the ForkDetector. Set to 0 to disable
	// automatic announcing (tests may drive announces manually via
	// Coordinator.BroadcastAnnounce). Runs on an independent ticker so its
	// cadence is decoupled from HeartbeatInterval.
	AnnounceInterval time.Duration

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

	// ReplayThrottleMs is the delay (in milliseconds) between consecutive
	// re-broadcasts during Autonomous Replay after fork healing. Lower values
	// finish replay faster but burst the network. Set to 0 to disable
	// throttling. Tunable so Phase 9.2 evaluation can sweep different load
	// profiles without recompiling.
	ReplayThrottleMs int

	// MetricsEnabled toggles recording of coordination metrics.
	// Should be true for benchmarks and evaluation; may be false in production
	// to reduce overhead if metrics are not needed.
	MetricsEnabled bool

	// OfflineSyncEnabled logs ciphertext envelopes for store-and-forward replay.
	OfflineSyncEnabled bool

	// EnvelopeLogTTL is how long envelope_log rows are retained (unix age prune).
	EnvelopeLogTTL time.Duration

	// EnvelopeLogMaxPerGroup caps rows per group_id after TTL prune.
	EnvelopeLogMaxPerGroup int

	// BatchingDelay is how long the Token Holder waits after receiving the first
	// proposal before executing a commit, allowing concurrent proposals to collect.
	BatchingDelay time.Duration

	// ViewBootstrapGrace is a short safety window after a coordinator starts.
	// During this window a singleton ActiveView (local-only) is not allowed to
	// issue a Commit if application policy says there are other authorized
	// committers in the group. This prevents discovery-delay forks immediately
	// after startup/join while preserving liveness after the grace expires.
	ViewBootstrapGrace time.Duration

	// MLSOperationTimeout bounds a single Rust sidecar MLS RPC that sits on a
	// user-facing or protocol-critical path (AddMembers, CreateCommit,
	// Encrypt/Decrypt, ProcessCommit, ...). Without a per-call deadline, a stalled
	// sidecar RPC can wedge the caller forever because the coordinator lifetime
	// context is only cancelled on app shutdown.
	MLSOperationTimeout time.Duration

	// ApplicationAckTimeout is how long a sender waits for a direct delivery ACK
	// for one MsgApplication envelope before retrying over the direct stream.
	// GossipSub remains the primary path; this timeout is only a repair trigger.
	ApplicationAckTimeout time.Duration

	// ApplicationDirectRetryLimit bounds how many timed direct retries the
	// coordinator attempts for one outstanding application envelope before giving
	// up and waiting for a future reconnect/offline-sync repair.
	ApplicationDirectRetryLimit int

	// RetentionMode controls the secret key retention policy.
	// STRICT_SECURITY (max_past_epochs = 0, age = 0)
	// BALANCED (max_past_epochs = 3, age = 5m) [default]
	// HIGH_AVAILABILITY (max_past_epochs = 10, age = 1h)
	RetentionMode MLSRetentionMode
}

type MLSRetentionMode string

const (
	RetentionStrictSecurity    MLSRetentionMode = "STRICT_SECURITY"
	RetentionBalanced          MLSRetentionMode = "BALANCED"
	RetentionHighAvailability MLSRetentionMode = "HIGH_AVAILABILITY"
)

// GetMaxPastEpochs returns the OpenMLS past epoch secret tree key retention limit.
func (c *CoordinatorConfig) GetMaxPastEpochs() uint32 {
	switch c.RetentionMode {
	case RetentionStrictSecurity:
		return 0
	case RetentionHighAvailability:
		return 10
	case RetentionBalanced:
		fallthrough
	default:
		return 3
	}
}

// GetMaxPastAgeSeconds returns the maximum causal age allowed for processing old messages in Go.
func (c *CoordinatorConfig) GetMaxPastAgeSeconds() int64 {
	switch c.RetentionMode {
	case RetentionStrictSecurity:
		return 0
	case RetentionHighAvailability:
		return 3600 // 1 hour
	case RetentionBalanced:
		fallthrough
	default:
		return 300 // 5 minutes
	}
}

// DefaultConfig returns production-ready defaults optimized for LAN/intranet
// where network latency is typically <1ms.
func DefaultConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		TokenHolderTimeout:          4 * time.Second,
		HeartbeatInterval:           5 * time.Second,
		AnnounceInterval:            10 * time.Second,
		PeerDeadAfter:               3,
		MaxBatchedProposals:         10,
		KeyRotationInterval:         5 * time.Minute,
		ReplayThrottleMs:            100,
		MetricsEnabled:              true,
		OfflineSyncEnabled:          true,
		EnvelopeLogTTL:              7 * 24 * time.Hour,
		EnvelopeLogMaxPerGroup:      10000,
		BatchingDelay:               1 * time.Second,
		ViewBootstrapGrace:          2 * time.Second,
		MLSOperationTimeout:         20 * time.Second,
		ApplicationAckTimeout:       10 * time.Second,
		ApplicationDirectRetryLimit: 2,
		RetentionMode:               RetentionBalanced,
	}
}

// TestConfig returns aggressive defaults for deterministic testing.
// Short timeouts ensure tests complete quickly; key rotation is disabled
// so tests control epoch transitions explicitly.
func TestConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		TokenHolderTimeout:          100 * time.Millisecond,
		HeartbeatInterval:           50 * time.Millisecond,
		AnnounceInterval:            0,
		PeerDeadAfter:               3,
		MaxBatchedProposals:         10,
		KeyRotationInterval:         0,
		ReplayThrottleMs:            0,
		MetricsEnabled:              true,
		OfflineSyncEnabled:          true,
		EnvelopeLogTTL:              7 * 24 * time.Hour,
		EnvelopeLogMaxPerGroup:      10000,
		BatchingDelay:               0,
		ViewBootstrapGrace:          0,
		MLSOperationTimeout:         250 * time.Millisecond,
		ApplicationAckTimeout:       50 * time.Millisecond,
		ApplicationDirectRetryLimit: 2,
		RetentionMode:               RetentionBalanced,
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
	if c.AnnounceInterval < 0 {
		return fmt.Errorf("%w: AnnounceInterval must be >= 0, got %v",
			ErrInvalidConfig, c.AnnounceInterval)
	}
	if c.ReplayThrottleMs < 0 {
		return fmt.Errorf("%w: ReplayThrottleMs must be >= 0, got %d",
			ErrInvalidConfig, c.ReplayThrottleMs)
	}
	if c.EnvelopeLogTTL < 0 {
		return fmt.Errorf("%w: EnvelopeLogTTL must be >= 0, got %v",
			ErrInvalidConfig, c.EnvelopeLogTTL)
	}
	if c.EnvelopeLogMaxPerGroup < 1 {
		return fmt.Errorf("%w: EnvelopeLogMaxPerGroup must be >= 1, got %d",
			ErrInvalidConfig, c.EnvelopeLogMaxPerGroup)
	}
	if c.BatchingDelay < 0 {
		return fmt.Errorf("%w: BatchingDelay must be >= 0, got %v",
			ErrInvalidConfig, c.BatchingDelay)
	}
	if c.ViewBootstrapGrace < 0 {
		return fmt.Errorf("%w: ViewBootstrapGrace must be >= 0, got %v",
			ErrInvalidConfig, c.ViewBootstrapGrace)
	}
	if c.MLSOperationTimeout <= 0 {
		return fmt.Errorf("%w: MLSOperationTimeout must be positive, got %v",
			ErrInvalidConfig, c.MLSOperationTimeout)
	}
	if c.ApplicationAckTimeout <= 0 {
		return fmt.Errorf("%w: ApplicationAckTimeout must be positive, got %v",
			ErrInvalidConfig, c.ApplicationAckTimeout)
	}
	if c.ApplicationDirectRetryLimit < 0 {
		return fmt.Errorf("%w: ApplicationDirectRetryLimit must be >= 0, got %d",
			ErrInvalidConfig, c.ApplicationDirectRetryLimit)
	}
	return nil
}
