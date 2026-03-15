package coordination

import (
	"testing"
	"time"
)

func TestMetrics_Snapshot_IsImmutableCopy(t *testing.T) {
	m := NewMetrics()
	m.IncrCommitsIssued()
	m.RecordEpochFinalization(10 * time.Millisecond)

	snap := m.Snapshot()

	m.IncrCommitsIssued()
	m.RecordEpochFinalization(20 * time.Millisecond)

	if snap.CommitsIssued != 1 {
		t.Errorf("snapshot CommitsIssued should be 1, got %d", snap.CommitsIssued)
	}
	if len(snap.EpochFinalizations) != 1 {
		t.Errorf("snapshot should have 1 finalization, got %d", len(snap.EpochFinalizations))
	}
}

func TestMetrics_Reset_ClearsEverything(t *testing.T) {
	m := NewMetrics()
	m.IncrCommitsIssued()
	m.IncrProposalsReceived()
	m.IncrDuplicateEpochDetected()
	m.IncrPartitionsDetected()
	m.IncrForkHealingsAttempted()
	m.IncrForkHealingsSucceeded()
	m.AddCommitBytes(1024)
	m.RecordEpochFinalization(5 * time.Millisecond)
	m.RecordExternalJoin(10 * time.Millisecond)
	m.RecordTokenElection(2 * time.Millisecond)

	m.Reset()
	snap := m.Snapshot()

	if snap.CommitsIssued != 0 || snap.ProposalsReceived != 0 ||
		snap.DuplicateEpochDetected != 0 || snap.CommitBytesTotal != 0 ||
		snap.PartitionsDetected != 0 || snap.ForkHealingsAttempted != 0 ||
		snap.ForkHealingsSucceeded != 0 {
		t.Error("Reset should clear all counters")
	}
	if len(snap.EpochFinalizations) != 0 || len(snap.ExternalJoins) != 0 ||
		len(snap.TokenElections) != 0 {
		t.Error("Reset should clear all latency slices")
	}
}

func TestMetrics_Counters_Increment(t *testing.T) {
	m := NewMetrics()

	m.IncrCommitsIssued()
	m.IncrCommitsIssued()
	m.IncrProposalsReceived()
	m.AddCommitBytes(100)
	m.AddCommitBytes(200)

	snap := m.Snapshot()
	if snap.CommitsIssued != 2 {
		t.Errorf("CommitsIssued: want 2, got %d", snap.CommitsIssued)
	}
	if snap.ProposalsReceived != 1 {
		t.Errorf("ProposalsReceived: want 1, got %d", snap.ProposalsReceived)
	}
	if snap.CommitBytesTotal != 300 {
		t.Errorf("CommitBytesTotal: want 300, got %d", snap.CommitBytesTotal)
	}
}

func TestMetrics_LatencySamples_Accumulate(t *testing.T) {
	m := NewMetrics()

	m.RecordEpochFinalization(1 * time.Millisecond)
	m.RecordEpochFinalization(2 * time.Millisecond)
	m.RecordEpochFinalization(3 * time.Millisecond)

	snap := m.Snapshot()
	if len(snap.EpochFinalizations) != 3 {
		t.Errorf("want 3 samples, got %d", len(snap.EpochFinalizations))
	}
	if snap.EpochFinalizations[0] != 1*time.Millisecond {
		t.Errorf("first sample: want 1ms, got %v", snap.EpochFinalizations[0])
	}
}
