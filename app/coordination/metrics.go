package coordination

import (
	"sync"
	"time"
)

// Metrics collects coordination protocol measurements for evaluation (Phase 7).
//
// All fields are unexported and accessed through thread-safe methods.
// Use Snapshot() to obtain an immutable copy for assertions and reporting.
type Metrics struct {
	mu sync.Mutex

	// Correctness (Phase 7.1)
	commitsIssued          int64
	proposalsReceived      int64
	duplicateEpochDetected int64

	// Latency samples (Phase 7.3) — append-only; compute percentiles externally
	epochFinalizations []time.Duration // time from first Proposal to all nodes at E+1
	externalJoins      []time.Duration // time from partition detection to External Join complete
	tokenElections     []time.Duration // time from Token Holder eviction to new Commit emitted

	// Scalability (Phase 7.3)
	commitBytesTotal int64

	// Partition (Phase 7.2)
	partitionsDetected    int64
	forkHealingsAttempted int64
	forkHealingsSucceeded int64
}

// NewMetrics creates a zero-initialized Metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// ─── Recording methods ───────────────────────────────────────────────────────

// IncrCommitsIssued records that this node issued a Commit as Token Holder.
func (m *Metrics) IncrCommitsIssued() {
	m.mu.Lock()
	m.commitsIssued++
	m.mu.Unlock()
}

// IncrProposalsReceived records receipt of a Proposal from any source.
func (m *Metrics) IncrProposalsReceived() {
	m.mu.Lock()
	m.proposalsReceived++
	m.mu.Unlock()
}

// IncrDuplicateEpochDetected records detection of a Commit for an epoch that
// was already processed. This should be zero if the Single-Writer invariant holds.
func (m *Metrics) IncrDuplicateEpochDetected() {
	m.mu.Lock()
	m.duplicateEpochDetected++
	m.mu.Unlock()
}

// RecordEpochFinalization records the time from Proposal submission until all
// nodes have advanced to the new epoch.
func (m *Metrics) RecordEpochFinalization(d time.Duration) {
	m.mu.Lock()
	m.epochFinalizations = append(m.epochFinalizations, d)
	m.mu.Unlock()
}

// RecordExternalJoin records the time for a fork healing External Join operation.
func (m *Metrics) RecordExternalJoin(d time.Duration) {
	m.mu.Lock()
	m.externalJoins = append(m.externalJoins, d)
	m.mu.Unlock()
}

// RecordTokenElection records the time from Token Holder eviction to the first
// Commit emitted by the new holder.
func (m *Metrics) RecordTokenElection(d time.Duration) {
	m.mu.Lock()
	m.tokenElections = append(m.tokenElections, d)
	m.mu.Unlock()
}

// AddCommitBytes adds to the running total of Commit bytes sent (bandwidth proxy).
func (m *Metrics) AddCommitBytes(n int64) {
	m.mu.Lock()
	m.commitBytesTotal += n
	m.mu.Unlock()
}

// IncrPartitionsDetected records that a network partition was detected via
// divergent TreeHash in GroupStateAnnouncement.
func (m *Metrics) IncrPartitionsDetected() {
	m.mu.Lock()
	m.partitionsDetected++
	m.mu.Unlock()
}

// IncrForkHealingsAttempted records that a fork healing procedure was initiated.
func (m *Metrics) IncrForkHealingsAttempted() {
	m.mu.Lock()
	m.forkHealingsAttempted++
	m.mu.Unlock()
}

// IncrForkHealingsSucceeded records that a fork healing completed successfully.
func (m *Metrics) IncrForkHealingsSucceeded() {
	m.mu.Lock()
	m.forkHealingsSucceeded++
	m.mu.Unlock()
}

// ─── Snapshot ────────────────────────────────────────────────────────────────

// MetricsSnapshot is an immutable point-in-time copy of all metrics.
// Exported fields are intended for use in test assertions and benchmark reports.
type MetricsSnapshot struct {
	CommitsIssued          int64
	ProposalsReceived      int64
	DuplicateEpochDetected int64

	EpochFinalizations []time.Duration
	ExternalJoins      []time.Duration
	TokenElections     []time.Duration

	CommitBytesTotal int64

	PartitionsDetected    int64
	ForkHealingsAttempted int64
	ForkHealingsSucceeded int64
}

// Snapshot returns an immutable copy of the current metrics state.
// Safe to call concurrently with recording methods.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := MetricsSnapshot{
		CommitsIssued:          m.commitsIssued,
		ProposalsReceived:      m.proposalsReceived,
		DuplicateEpochDetected: m.duplicateEpochDetected,
		CommitBytesTotal:       m.commitBytesTotal,
		PartitionsDetected:     m.partitionsDetected,
		ForkHealingsAttempted:  m.forkHealingsAttempted,
		ForkHealingsSucceeded:  m.forkHealingsSucceeded,
	}

	if len(m.epochFinalizations) > 0 {
		s.EpochFinalizations = make([]time.Duration, len(m.epochFinalizations))
		copy(s.EpochFinalizations, m.epochFinalizations)
	}
	if len(m.externalJoins) > 0 {
		s.ExternalJoins = make([]time.Duration, len(m.externalJoins))
		copy(s.ExternalJoins, m.externalJoins)
	}
	if len(m.tokenElections) > 0 {
		s.TokenElections = make([]time.Duration, len(m.tokenElections))
		copy(s.TokenElections, m.tokenElections)
	}

	return s
}

// Reset clears all metrics to zero. Useful between benchmark iterations.
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.commitsIssued = 0
	m.proposalsReceived = 0
	m.duplicateEpochDetected = 0
	m.epochFinalizations = nil
	m.externalJoins = nil
	m.tokenElections = nil
	m.commitBytesTotal = 0
	m.partitionsDetected = 0
	m.forkHealingsAttempted = 0
	m.forkHealingsSucceeded = 0
}
