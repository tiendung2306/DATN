package service

import (
	"testing"

	"app/coordination"
)

func TestOfflineSyncShouldAdvanceCursorForTerminalReplayStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		state coordination.ReplayEnvelopeState
		want  bool
	}{
		{state: coordination.ReplayStateApplied, want: true},
		{state: coordination.ReplayStateDuplicateApplied, want: true},
		{state: coordination.ReplayStateStaleEpoch, want: true},
		{state: coordination.ReplayStateDecryptFailed, want: true},
		{state: coordination.ReplayStateBlockedStaleRequiresSnapshot, want: true},
		{state: coordination.ReplayStateBlockedDecryptFailed, want: true},
		{state: coordination.ReplayStateFutureEpoch, want: false},
		{state: coordination.ReplayStateBlockedMissingPriorEpoch, want: false},
		{state: coordination.ReplayStateInvalid, want: false},
	}

	for _, tc := range cases {
		if got := offlineSyncShouldAdvanceCursor(tc.state); got != tc.want {
			t.Fatalf("state %q => %v, want %v", tc.state, got, tc.want)
		}
	}
}
