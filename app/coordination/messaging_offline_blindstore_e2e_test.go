package coordination

import (
	"context"
	"testing"
	"time"
)

func TestE2E_MessagingOfflineSyncAndBlindStoreReplay(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	aliceID := peerID("alice")
	bobID := peerID("bob")
	aliceTransport := network.AddNode(aliceID)
	bobTransport := network.AddNode(bobID)

	aliceMLS := NewMockMLSEngine()
	bobMLS := NewMockMLSEngine()
	aliceStorage := NewMockStorage()
	bobStorage := NewMockStorage()

	var blindStoreReplicas [][]byte
	aliceCoord, err := NewCoordinator(CoordinatorOpts{
		Config:    TestConfig(),
		Transport: aliceTransport,
		Clock:     clk,
		MLS:       aliceMLS,
		Storage:   aliceStorage,
		LocalID:   aliceID,
		GroupID:   "grp-e2e-offline",
		OnEnvelopeBroadcast: func(mt MessageType, _ string, env []byte) {
			if mt != MsgApplication {
				return
			}
			cp := make([]byte, len(env))
			copy(cp, env)
			blindStoreReplicas = append(blindStoreReplicas, cp)
		},
	})
	if err != nil {
		t.Fatalf("NewCoordinator alice: %v", err)
	}

	bobCoord, err := NewCoordinator(CoordinatorOpts{
		Config:    TestConfig(),
		Transport: bobTransport,
		Clock:     clk,
		MLS:       bobMLS,
		Storage:   bobStorage,
		LocalID:   bobID,
		GroupID:   "grp-e2e-offline",
	})
	if err != nil {
		t.Fatalf("NewCoordinator bob: %v", err)
	}

	if err := aliceCoord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup alice: %v", err)
	}
	bobCoord.InitializeGroup(aliceCoord.GetGroupState(), aliceCoord.CurrentEpoch(), aliceCoord.treeHash)

	ctx := context.Background()
	if err := aliceCoord.Start(ctx); err != nil {
		t.Fatalf("Start alice: %v", err)
	}
	t.Cleanup(aliceCoord.Stop)
	if err := bobCoord.Start(ctx); err != nil {
		t.Fatalf("Start bob: %v", err)
	}
	t.Cleanup(bobCoord.Stop)

	if _, err := aliceCoord.SendMessage([]byte("m1-offline")); err != nil {
		t.Fatalf("SendMessage #1: %v", err)
	}
	if _, err := aliceCoord.SendMessage([]byte("m2-offline")); err != nil {
		t.Fatalf("SendMessage #2: %v", err)
	}

	if got := len(blindStoreReplicas); got != 2 {
		t.Fatalf("blind-store captured %d envelopes, want 2", got)
	}

	// Bob was effectively offline (no network drain). First, recover one envelope
	// via blind-store replication.
	appliedBlind, err := bobCoord.ReplayEnvelopes([][]byte{blindStoreReplicas[0]})
	if err != nil {
		t.Fatalf("ReplayEnvelopes blind-store: %v", err)
	}
	if appliedBlind != 1 {
		t.Fatalf("blind-store applied=%d, want 1", appliedBlind)
	}

	// Then run offline sync replay from Alice's envelope log. The first envelope
	// is replayed again, so dedup must keep total messages correct.
	recs, err := aliceStorage.GetEnvelopesSince("grp-e2e-offline", 0, 50)
	if err != nil {
		t.Fatalf("GetEnvelopesSince: %v", err)
	}
	offlineBatch := make([][]byte, 0, len(recs))
	for _, r := range recs {
		offlineBatch = append(offlineBatch, r.Envelope)
	}
	appliedOffline, err := bobCoord.ReplayEnvelopes(offlineBatch)
	if err != nil {
		t.Fatalf("ReplayEnvelopes offline-sync: %v", err)
	}
	if appliedOffline != 1 {
		t.Fatalf("offline-sync applied=%d, want 1 (second message only)", appliedOffline)
	}

	msgs := bobStorage.Messages()
	if len(msgs) != 2 {
		t.Fatalf("bob stored %d messages, want 2", len(msgs))
	}
	if string(msgs[0].Content) != "m1-offline" || string(msgs[1].Content) != "m2-offline" {
		t.Fatalf("unexpected message contents: got [%q, %q]", msgs[0].Content, msgs[1].Content)
	}
}

func TestE2E_OfflineSyncReplayHonorsCursor(t *testing.T) {
	network := NewFakeNetwork()
	clk := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	aliceID := peerID("alice")
	bobID := peerID("bob")
	aliceTransport := network.AddNode(aliceID)
	bobTransport := network.AddNode(bobID)

	aliceStorage := NewMockStorage()
	bobStorage := NewMockStorage()

	aliceCoord, err := NewCoordinator(CoordinatorOpts{
		Config:    TestConfig(),
		Transport: aliceTransport,
		Clock:     clk,
		MLS:       NewMockMLSEngine(),
		Storage:   aliceStorage,
		LocalID:   aliceID,
		GroupID:   "grp-e2e-cursor",
	})
	if err != nil {
		t.Fatalf("NewCoordinator alice: %v", err)
	}
	bobCoord, err := NewCoordinator(CoordinatorOpts{
		Config:    TestConfig(),
		Transport: bobTransport,
		Clock:     clk,
		MLS:       NewMockMLSEngine(),
		Storage:   bobStorage,
		LocalID:   bobID,
		GroupID:   "grp-e2e-cursor",
	})
	if err != nil {
		t.Fatalf("NewCoordinator bob: %v", err)
	}

	if err := aliceCoord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup alice: %v", err)
	}
	bobCoord.InitializeGroup(aliceCoord.GetGroupState(), aliceCoord.CurrentEpoch(), aliceCoord.treeHash)

	ctx := context.Background()
	if err := aliceCoord.Start(ctx); err != nil {
		t.Fatalf("Start alice: %v", err)
	}
	t.Cleanup(aliceCoord.Stop)
	if err := bobCoord.Start(ctx); err != nil {
		t.Fatalf("Start bob: %v", err)
	}
	t.Cleanup(bobCoord.Stop)

	if _, err := aliceCoord.SendMessage([]byte("c1")); err != nil {
		t.Fatalf("SendMessage c1: %v", err)
	}
	if _, err := aliceCoord.SendMessage([]byte("c2")); err != nil {
		t.Fatalf("SendMessage c2: %v", err)
	}
	if _, err := aliceCoord.SendMessage([]byte("c3")); err != nil {
		t.Fatalf("SendMessage c3: %v", err)
	}

	firstPull, err := aliceStorage.GetEnvelopesSince("grp-e2e-cursor", 0, 2)
	if err != nil {
		t.Fatalf("GetEnvelopesSince first pull: %v", err)
	}
	if len(firstPull) != 2 {
		t.Fatalf("first pull size=%d, want 2", len(firstPull))
	}
	firstBatch := [][]byte{firstPull[0].Envelope, firstPull[1].Envelope}
	if applied, err := bobCoord.ReplayEnvelopes(firstBatch); err != nil || applied != 2 {
		t.Fatalf("Replay first pull applied=%d err=%v, want 2 nil", applied, err)
	}
	if err := bobStorage.SetOfflinePullCursor("grp-e2e-cursor", aliceID.String(), firstPull[1].Seq); err != nil {
		t.Fatalf("SetOfflinePullCursor: %v", err)
	}

	cursor, err := bobStorage.GetOfflinePullCursor("grp-e2e-cursor", aliceID.String())
	if err != nil {
		t.Fatalf("GetOfflinePullCursor: %v", err)
	}
	secondPull, err := aliceStorage.GetEnvelopesSince("grp-e2e-cursor", cursor, 50)
	if err != nil {
		t.Fatalf("GetEnvelopesSince second pull: %v", err)
	}
	if len(secondPull) != 1 {
		t.Fatalf("second pull size=%d, want 1", len(secondPull))
	}
	if applied, err := bobCoord.ReplayEnvelopes([][]byte{secondPull[0].Envelope}); err != nil || applied != 1 {
		t.Fatalf("Replay second pull applied=%d err=%v, want 1 nil", applied, err)
	}

	msgs := bobStorage.Messages()
	if len(msgs) != 3 {
		t.Fatalf("bob stored %d messages, want 3", len(msgs))
	}
	if string(msgs[0].Content) != "c1" || string(msgs[1].Content) != "c2" || string(msgs[2].Content) != "c3" {
		t.Fatalf("unexpected message contents: got [%q, %q, %q]", msgs[0].Content, msgs[1].Content, msgs[2].Content)
	}
}
