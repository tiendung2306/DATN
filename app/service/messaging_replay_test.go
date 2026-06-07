package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"app/adapter/store"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

type replayCaptureSink struct {
	events []string
	data   []map[string]interface{}
}

func (s *replayCaptureSink) Emit(_ context.Context, event string, payload map[string]interface{}) {
	s.events = append(s.events, event)
	s.data = append(s.data, payload)
}

func setupReplayRuntime(t *testing.T) (*Runtime, *store.SQLiteCoordinationStorage) {
	t.Helper()
	db, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cs := store.NewSQLiteCoordinationStorage(db)
	return &Runtime{
		ctx:            context.Background(),
		coordStorage:   cs,
		eventRevisions: map[string]int64{},
	}, cs
}

func makeReplayApplicationEvent(eventID, jobID, groupID string, originalHash, replayedHash []byte) *coordination.ApplicationEvent {
	payloadHash := sha256.Sum256([]byte(eventID + ":" + groupID))
	return &coordination.ApplicationEvent{
		EventID:              eventID,
		JobID:                jobID,
		GroupID:              groupID,
		EnvelopeHash:         originalHash,
		PayloadHash:          payloadHash[:],
		HlcWallTimeMs:        1000,
		HlcCounter:           1,
		HlcNodeID:            "peer-a",
		Status:               "REPLAYED",
		ReplayedEnvelopeHash: replayedHash,
		CreatedAtMs:          1000,
		UpdatedAtMs:          1000,
	}
}

func TestMapStoredMessagesToMessageInfo_ResolvesReplaySupersedesMessageID(t *testing.T) {
	rt, cs := setupReplayRuntime(t)
	originalHash := sha256.Sum256([]byte("original"))
	replayedHash := sha256.Sum256([]byte("replayed"))
	replayedAt := int64(1234)
	if err := cs.SaveApplicationEvent(makeReplayApplicationEvent("ev-map-replay", "job-map-replay", "g-map-replay", originalHash[:], replayedHash[:])); err != nil {
		t.Fatalf("SaveApplicationEvent: %v", err)
	}

	info := rt.mapStoredMessagesToMessageInfo([]*coordination.StoredMessage{{
		MessageID:    hex.EncodeToString(replayedHash[:]),
		GroupID:      "g-map-replay",
		SenderID:     peer.ID("peer-a"),
		Content:      []byte("hello"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: 10, Counter: 1, NodeID: "peer-a"},
		EnvelopeHash: replayedHash[:],
		ReplayedAt:   &replayedAt,
	}})

	if len(info) != 1 {
		t.Fatalf("mapped len=%d want 1", len(info))
	}
	if info[0].SupersedesMessageID != hex.EncodeToString(originalHash[:]) {
		t.Fatalf("SupersedesMessageID=%q want %q", info[0].SupersedesMessageID, hex.EncodeToString(originalHash[:]))
	}
	if info[0].ReplayedAt == nil || *info[0].ReplayedAt != replayedAt {
		t.Fatalf("ReplayedAt=%v want %d", info[0].ReplayedAt, replayedAt)
	}
}

func TestMapStoredMessagesToMessageInfo_ResolvesReplaySupersedesMessageID_MultiHop(t *testing.T) {
	rt, cs := setupReplayRuntime(t)
	originalHash := sha256.Sum256([]byte("original-multi"))
	replay1Hash := sha256.Sum256([]byte("replay-multi-1"))
	replay2Hash := sha256.Sum256([]byte("replay-multi-2"))
	replayedAt := int64(4321)

	if err := cs.SaveApplicationEvent(makeReplayApplicationEvent("ev-map-replay-1", "job-map-replay-multi", "g-map-replay-multi", originalHash[:], replay1Hash[:])); err != nil {
		t.Fatalf("SaveApplicationEvent 1: %v", err)
	}
	if err := cs.SaveApplicationEvent(makeReplayApplicationEvent("ev-map-replay-2", "job-map-replay-multi", "g-map-replay-multi", replay1Hash[:], replay2Hash[:])); err != nil {
		t.Fatalf("SaveApplicationEvent 2: %v", err)
	}

	info := rt.mapStoredMessagesToMessageInfo([]*coordination.StoredMessage{{
		MessageID:    hex.EncodeToString(replay2Hash[:]),
		GroupID:      "g-map-replay-multi",
		SenderID:     peer.ID("peer-a"),
		Content:      []byte("hello-again"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: 20, Counter: 1, NodeID: "peer-a"},
		EnvelopeHash: replay2Hash[:],
		ReplayedAt:   &replayedAt,
	}})

	if len(info) != 1 {
		t.Fatalf("mapped len=%d want 1", len(info))
	}
	if info[0].SupersedesMessageID != hex.EncodeToString(originalHash[:]) {
		t.Fatalf("SupersedesMessageID=%q want %q", info[0].SupersedesMessageID, hex.EncodeToString(originalHash[:]))
	}
}

func TestMakeMessageHandler_EmitsReplayMetadataConsistently(t *testing.T) {
	rt, cs := setupReplayRuntime(t)
	sink := &replayCaptureSink{}
	rt.SetEventSink(sink)

	originalHash := sha256.Sum256([]byte("original-event"))
	replayedHash := sha256.Sum256([]byte("replayed-event"))
	replayedAt := int64(5678)
	if err := cs.SaveApplicationEvent(makeReplayApplicationEvent("ev-emit-replay", "job-emit-replay", "g-emit-replay", originalHash[:], replayedHash[:])); err != nil {
		t.Fatalf("SaveApplicationEvent: %v", err)
	}

	handler := rt.makeMessageHandler("g-emit-replay")
	handler(&coordination.StoredMessage{
		MessageID:    hex.EncodeToString(replayedHash[:]),
		GroupID:      "g-emit-replay",
		SenderID:     peer.ID("peer-a"),
		Content:      []byte("hello"),
		Timestamp:    coordination.HLCTimestamp{WallTimeMs: 42, Counter: 0, NodeID: "peer-a"},
		EnvelopeHash: replayedHash[:],
		ReplayedAt:   &replayedAt,
	})

	if len(sink.events) == 0 {
		t.Fatal("expected emitted event")
	}
	var payload map[string]interface{}
	for i, event := range sink.events {
		if event == "group:message" {
			payload = sink.data[i]
			break
		}
	}
	if payload == nil {
		t.Fatal("group:message payload not captured")
	}
	if got := payload["supersedes_message_id"]; got != hex.EncodeToString(originalHash[:]) {
		t.Fatalf("supersedes_message_id=%v want %s", got, hex.EncodeToString(originalHash[:]))
	}
	if got := payload["replayed_at"]; got != replayedAt {
		t.Fatalf("replayed_at=%v want %d", got, replayedAt)
	}
}

func TestMakeMessageHandler_EmitsLocalEchoToken(t *testing.T) {
	rt, _ := setupReplayRuntime(t)
	sink := &replayCaptureSink{}
	rt.SetEventSink(sink)

	handler := rt.makeMessageHandler("g-local-echo")
	handler(&coordination.StoredMessage{
		MessageID:      "msg-local-echo",
		GroupID:        "g-local-echo",
		SenderID:       peer.ID("peer-a"),
		Content:        []byte("hello-local"),
		Timestamp:      coordination.HLCTimestamp{WallTimeMs: 99, Counter: 0, NodeID: "peer-a"},
		LocalEchoToken: "token-123",
	})

	var payload map[string]interface{}
	for i, event := range sink.events {
		if event == "group:message" {
			payload = sink.data[i]
			break
		}
	}
	if payload == nil {
		t.Fatal("group:message payload not captured")
	}
	if got := payload["local_echo_token"]; got != "token-123" {
		t.Fatalf("local_echo_token=%v want token-123", got)
	}
}

func TestEmitReplayBlocked_SuppressesLateJoinHistoryGap(t *testing.T) {
	rt, _ := setupReplayRuntime(t)
	sink := &replayCaptureSink{}
	rt.SetEventSink(sink)
	blockedHash := sha256.Sum256([]byte("blocked"))

	rt.emitReplayBlocked(
		"g-replay-blocked",
		"stale_epoch_requires_recovery_snapshot",
		&coordination.EnvelopeRecord{Seq: 7},
		coordination.ReplayEnvelopeResult{
			GroupID:      "g-replay-blocked",
			State:        coordination.ReplayStateBlockedStaleRequiresSnapshot,
			LocalEpoch:   4,
			MsgEpoch:     1,
			EnvelopeHash: blockedHash[:],
		},
	)

	if len(sink.data) == 0 {
		t.Fatal("expected replay_blocked event")
	}
	payload := sink.data[0]
	if got := payload["user_visible"]; got != false {
		t.Fatalf("user_visible=%v want false", got)
	}
}
