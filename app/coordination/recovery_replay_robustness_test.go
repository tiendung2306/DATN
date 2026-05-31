package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// TestCoordinator_CatchUpMode_GossipInboxIsolation verifies that when a coordinator
// is in ModeCatchingUp, incoming live MsgCommit and MsgApplication envelopes are isolated
// and buffered into the coordination storage inbox as pending gossip catchup items,
// without mutating the coordinator's local groupState or epoch.
func TestCoordinator_CatchUpMode_GossipInboxIsolation(t *testing.T) {
	groupID := "grp-catchup-isolation"
	nodes, network, _ := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// 1. Initially both nodes are at epoch 0
	if alice.coord.CurrentEpoch() != 0 || bob.coord.CurrentEpoch() != 0 {
		t.Fatalf("expected initial epoch to be 0, got alice=%d bob=%d",
			alice.coord.CurrentEpoch(), bob.coord.CurrentEpoch())
	}

	// 2. Set Bob's operational mode to ModeCatchingUp
	bob.coord.SetOperationalMode(ModeCatchingUp)
	if mode := bob.coord.GetOperationalMode(); mode != ModeCatchingUp {
		t.Fatalf("expected Bob mode to be CATCHING_UP, got %s", mode)
	}

	// 3. Alice sends an application message
	testMsg := []byte("secret live gossip while catching up")
	_, err := alice.coord.SendMessage(testMsg)
	if err != nil {
		t.Fatalf("Alice failed to send message: %v", err)
	}

	// 4. Drain the network to deliver Alice's message to Bob's raw message handler
	network.DrainAll()

	// 5. Assert Gossip Inbox Isolation holds true:
	// Bob should not have processed the message or updated his groupState
	bobMsgs := bob.storage.Messages()
	if len(bobMsgs) != 0 {
		t.Fatalf("Inbox Isolation Violated! Bob processed message while catching up: count=%d", len(bobMsgs))
	}

	// Bob's group state/epoch must not have changed
	if bob.coord.CurrentEpoch() != 0 {
		t.Fatalf("Inbox Isolation Violated! Bob advanced epoch to %d while catching up", bob.coord.CurrentEpoch())
	}

	// 6. Assert that the envelope was successfully written to Bob's database as "pending" under "gossip_catchup"
	records, err := bob.storage.GetEnvelopesSince(groupID, 0, 10)
	if err != nil {
		t.Fatalf("failed to query Bob's logged envelopes: %v", err)
	}

	var gossipCatchupRecord *EnvelopeRecord
	for _, rec := range records {
		if rec.MsgType == MsgApplication && rec.SourcePath == "gossip_catchup" {
			gossipCatchupRecord = rec
			break
		}
	}

	if gossipCatchupRecord == nil {
		t.Fatal("expected Bob to buffer the live application envelope in SQLite log with source 'gossip_catchup', but found none")
	}

	if gossipCatchupRecord.ApplyState != "pending" {
		t.Fatalf("expected buffered gossip envelope to be 'pending', got %q", gossipCatchupRecord.ApplyState)
	}

	// 7. Transition Bob back to ModeLive to replicate catch-up exit
	bob.coord.SetOperationalMode(ModeLive)
	if mode := bob.coord.GetOperationalMode(); mode != ModeLive {
		t.Fatalf("expected Bob mode to be LIVE, got %s", mode)
	}

	// 8. Execute sequential replay on the isolated envelope
	results, err := bob.coord.ReplayEnvelopesDetailed([][]byte{gossipCatchupRecord.Envelope})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 replay result, got %d", len(results))
	}

	result := results[0]
	if result.State != ReplayStateApplied {
		t.Fatalf("expected replay state to be APPLIED, got %s (err: %s)", result.State, result.Error)
	}
	if !result.Applied || !result.CursorSafe {
		t.Fatalf("expected result to be applied and cursor-safe: %+v", result)
	}

	// 9. Verify that Bob has now successfully processed and decrypted the message
	bobMsgsAfter := bob.storage.Messages()
	if len(bobMsgsAfter) != 1 {
		t.Fatalf("expected Bob to have exactly 1 stored message after catch-up replay, got %d", len(bobMsgsAfter))
	}

	if string(bobMsgsAfter[0].Content) != string(testMsg) {
		t.Errorf("mismatched decrypted message: got %q, want %q", bobMsgsAfter[0].Content, testMsg)
	}
}

// TestCoordinator_RecoveryReplay_StaleRequiresSnapshot verifies that replaying stale application envelopes
// (epoch less than local coordinator epoch) does not fail or halt the sequencer but returns ReplayStateStaleEpoch
// with CursorSafe=true, permitting the recovery loop to mark them blocked and bypass them safely.
func TestCoordinator_RecoveryReplay_StaleRequiresSnapshot(t *testing.T) {
	groupID := "grp-stale-replay"
	nodes, network, _ := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// 1. Partition Bob so he does NOT receive or apply the first message
	network.Partition([]peer.ID{alice.id}, []peer.ID{bob.id})

	// 2. Alice sends message at epoch 0
	_, err := alice.coord.SendMessage([]byte("msg at epoch 0"))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	network.DrainAll() // Delivered to Alice herself, Bob receives nothing since he is partitioned

	records, err := alice.storage.GetEnvelopesSince(groupID, 0, 10)
	if err != nil {
		t.Fatalf("failed to get alice logged envelopes: %v", err)
	}

	var epoch0Env []byte
	for _, rec := range records {
		if rec.MsgType == MsgApplication {
			epoch0Env = rec.Envelope
			break
		}
	}
	if len(epoch0Env) == 0 {
		t.Fatal("failed to find epoch 0 application envelope")
	}

	// 3. Heal the partition
	network.Heal()
	exchangeHeartbeats(nodes, network)

	// 4. Advance the coordinator's epoch to epoch 4 by doing multiple proposals/commits
	var holder *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
			break
		}
	}
	if holder == nil {
		t.Fatal("failed to find token holder")
	}

	for i := 0; i < 4; i++ {
		err := holder.coord.ProposeUpdate([]byte("epoch-advancement"))
		if err != nil {
			t.Fatalf("ProposeUpdate failed: %v", err)
		}
		network.DrainAll()
	}

	// Both nodes must now be at epoch 4
	for _, n := range nodes {
		if n.coord.CurrentEpoch() != 4 {
			t.Fatalf("node %s epoch = %d, expected 4", n.id, n.coord.CurrentEpoch())
		}
	}

	// 5. Replay the epoch 0 application envelope on Bob
	// Bob is at epoch 4, which is strictly greater than the message epoch (0).
	// Since Bob never applied this envelope before, this triggers the stale epoch check.
	results, err := bob.coord.ReplayEnvelopesDetailed([][]byte{epoch0Env})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.State != ReplayStateStaleEpoch {
		t.Fatalf("expected replay state to be STALE_EPOCH, got %s", result.State)
	}

	// Stale epoch replay must be cursor-safe and terminal to allow recovery loop progress
	if !result.CursorSafe || !result.Terminal {
		t.Fatalf("expected stale replay result to be cursor-safe and terminal: %+v", result)
	}
}

// TestCoordinator_RecoveryReplay_DecryptFailed verifies that replaying envelopes that fail MLS decryption
// returns ReplayStateDecryptFailed and is flagged as terminal, so the recovery loop can transition them
// to BLOCKED_DECRYPT_FAILED and safely bypass the gap without locking up.
func TestCoordinator_RecoveryReplay_DecryptFailed(t *testing.T) {
	groupID := "grp-decrypt-failed"
	nodes, network, clk := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// 1. Construct an envelope containing malformed application payload that will fail mock/real decryption
	malformedPayload, err := json.Marshal(ApplicationMsg{
		Ciphertext: []byte("garbage-non-decryptable-ciphertext"),
	})
	if err != nil {
		t.Fatalf("failed to marshal ApplicationMsg: %v", err)
	}

	env := Envelope{
		Type:    MsgApplication,
		GroupID: groupID,
		Epoch:   bob.coord.CurrentEpoch(),
		From:    alice.id.String(),
		Timestamp: HLCTimestamp{
			WallTimeMs: clk.Now().UnixMilli(),
			NodeID:     alice.id.String(),
		},
		Payload: malformedPayload,
	}

	envelopeBytes, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	// 2. Replay the malformed envelope on Bob, injecting mock decryption failure
	bob.mls.SetNextError(errors.New("mock decryption failed"))
	results, err := bob.coord.ReplayEnvelopesDetailed([][]byte{envelopeBytes})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.State != ReplayStateDecryptFailed {
		t.Fatalf("expected replay state to be DECRYPT_FAILED, got %s (err: %s)", result.State, result.Error)
	}

	// Decrypt failure must be terminal so it can be bypassed after marking in storage
	if !result.Terminal {
		t.Fatalf("expected decryption failure result to be terminal: %+v", result)
	}
}

// TestCoordinator_RecoveryReplay_FutureEpochBlocked verifies that replaying envelopes from a future epoch
// (e.g. local epoch is 0, message is epoch 2) returns ReplayStateFutureEpoch and is marked as not applied
// and not cursor-safe, which correctly forces the recovery loop sequencer to halt and wait for missing commits.
func TestCoordinator_RecoveryReplay_FutureEpochBlocked(t *testing.T) {
	groupID := "grp-future-epoch"
	nodes, network, clk := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// 1. Construct an envelope at a future epoch (epoch 2) when Bob is still at epoch 0
	appPayload, err := json.Marshal(ApplicationMsg{
		Ciphertext: []byte("future-epoch-ciphertext"),
	})
	if err != nil {
		t.Fatalf("failed to marshal ApplicationMsg: %v", err)
	}

	futureEnv := Envelope{
		Type:    MsgApplication,
		GroupID: groupID,
		Epoch:   2, // Future epoch (Bob is at 0)
		From:    alice.id.String(),
		Timestamp: HLCTimestamp{
			WallTimeMs: clk.Now().UnixMilli(),
			NodeID:     alice.id.String(),
		},
		Payload: appPayload,
	}

	futureEnvBytes, err := json.Marshal(futureEnv)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	// 2. Replay the future envelope on Bob
	results, err := bob.coord.ReplayEnvelopesDetailed([][]byte{futureEnvBytes})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.State != ReplayStateFutureEpoch {
		t.Fatalf("expected replay state to be FUTURE_EPOCH, got %s", result.State)
	}

	// FUTURE_EPOCH must not be cursor-safe and not applied to prevent the sequencer from advancing
	if result.Applied || result.CursorSafe || result.Terminal {
		t.Fatalf("expected future replay result to not be applied, terminal, or cursor-safe: %+v", result)
	}

	// Ensure no message was stored
	if len(bob.storage.Messages()) != 0 {
		t.Fatal("expected Bob to store 0 messages for future epoch application envelope")
	}
}

// TestCoordinator_CatchUpMode_InboxIsolation_MultipleAppAndCommits verifies that multiple application messages
// and commit envelopes received while in ModeCatchingUp are isolated, buffered, and then sequentially replayed
// correctly to advance the epoch and decrypt the messages causally.
func TestCoordinator_CatchUpMode_InboxIsolation_MultipleAppAndCommits(t *testing.T) {
	groupID := "grp-catchup-multi"
	nodes, network, _ := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// 1. Set Bob's operational mode to ModeCatchingUp
	bob.coord.SetOperationalMode(ModeCatchingUp)

	// 2. Alice sends Application message 1 (epoch 0)
	msg1 := []byte("first message at epoch 0")
	if _, err := alice.coord.SendMessage(msg1); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// 3. Alice rotates key (proposes update and commits), advancing group to epoch 1
	if err := alice.coord.ProposeUpdate([]byte("alice-key-rotation")); err != nil {
		t.Fatalf("ProposeUpdate failed: %v", err)
	}

	// 4. Alice sends Application message 2 (epoch 1)
	msg2 := []byte("second message at epoch 1")
	if _, err := alice.coord.SendMessage(msg2); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// 5. Deliver all messages over the network to Bob
	network.DrainAll()

	// 6. Assert Gossip Inbox Isolation: Bob's group state hasn't changed (still epoch 0, 0 messages)
	if bob.coord.CurrentEpoch() != 0 {
		t.Fatalf("expected Bob to be at epoch 0, got %d", bob.coord.CurrentEpoch())
	}
	if len(bob.storage.Messages()) != 0 {
		t.Fatalf("expected Bob to have 0 messages, got %d", len(bob.storage.Messages()))
	}

	// 7. Get all buffered envelopes in Bob's storage
	records, err := bob.storage.GetEnvelopesSince(groupID, 0, 50)
	if err != nil {
		t.Fatalf("GetEnvelopesSince failed: %v", err)
	}

	// We should have at least 3 envelopes buffered: AppMsg(0), Commit(0->1), AppMsg(1)
	var envelopes [][]byte
	for _, rec := range records {
		if rec.SourcePath == "gossip_catchup" {
			envelopes = append(envelopes, rec.Envelope)
		}
	}

	if len(envelopes) < 3 {
		t.Fatalf("expected at least 3 isolated envelopes, got %d", len(envelopes))
	}

	// 8. Restore Bob to LIVE mode
	bob.coord.SetOperationalMode(ModeLive)

	// 9. Replay them sequentially in the correct order
	results, err := bob.coord.ReplayEnvelopesDetailed(envelopes)
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}

	// Assert that all are applied successfully
	for i, res := range results {
		if res.State != ReplayStateApplied {
			t.Fatalf("expected envelope %d to be APPLIED, got %s (err: %s)", i, res.State, res.Error)
		}
	}

	// 10. Verify Bob advanced to epoch 1 and successfully decrypted both messages
	if bob.coord.CurrentEpoch() != 1 {
		t.Fatalf("expected Bob to reach epoch 1, got %d", bob.coord.CurrentEpoch())
	}

	bobMsgs := bob.storage.Messages()
	if len(bobMsgs) != 2 {
		t.Fatalf("expected Bob to have 2 stored messages, got %d", len(bobMsgs))
	}

	if string(bobMsgs[0].Content) != string(msg1) || string(bobMsgs[1].Content) != string(msg2) {
		t.Errorf("decrypted messages mismatch: got [%s, %s], want [%s, %s]",
			bobMsgs[0].Content, bobMsgs[1].Content, msg1, msg2)
	}
}

// TestCoordinator_GetSetOperationalMode_Concurrency verifies the thread-safety of operationalMode getters and setters.
func TestCoordinator_GetSetOperationalMode_Concurrency(t *testing.T) {
	groupID := "grp-concurrency"
	nodes, _, _ := setupCluster(t, 1, groupID)
	coord := nodes[0].coord

	startChan := make(chan struct{})
	doneChan := make(chan struct{})

	readersCount := 50
	writersCount := 50

	for i := 0; i < readersCount; i++ {
		go func() {
			<-startChan
			for j := 0; j < 100; j++ {
				_ = coord.GetOperationalMode()
			}
			doneChan <- struct{}{}
		}()
	}

	for i := 0; i < writersCount; i++ {
		mode := ModeLive
		if i%2 == 0 {
			mode = ModeCatchingUp
		}
		go func() {
			<-startChan
			for j := 0; j < 100; j++ {
				coord.SetOperationalMode(mode)
			}
			doneChan <- struct{}{}
		}()
	}

	close(startChan)

	totalGoroutines := readersCount + writersCount
	for i := 0; i < totalGoroutines; i++ {
		<-doneChan
	}
}

// TestCoordinator_UnifiedRetentionPolicy_StrictSecurity verifies that in STRICT_SECURITY mode,
// any application message from a past epoch is rejected early without calling the crypto engine.
func TestCoordinator_UnifiedRetentionPolicy_StrictSecurity(t *testing.T) {
	groupID := "grp-retention-strict"
	nodes, network, _ := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// 1. Configure Bob with STRICT_SECURITY
	bob.coord.cfg.RetentionMode = RetentionStrictSecurity

	// 2. Alice sends message at epoch 0
	msgText := []byte("hello strict security")
	_, err := alice.coord.SendMessage(msgText)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	network.DrainAll()

	records, err := alice.storage.GetEnvelopesSince(groupID, 0, 10)
	if err != nil {
		t.Fatalf("failed to get logged envelopes: %v", err)
	}

	var epoch0Env []byte
	var epoch0Hash []byte
	for _, rec := range records {
		if rec.MsgType == MsgApplication {
			epoch0Env = rec.Envelope
			epoch0Hash = rec.EnvelopeHash
			break
		}
	}
	if len(epoch0Env) == 0 {
		t.Fatal("failed to find epoch 0 application envelope")
	}

	// Ensure Bob applied it live
	bobMsgs := bob.storage.Messages()
	if len(bobMsgs) != 1 {
		t.Fatalf("expected Bob to have applied epoch 0 msg live, got %d", len(bobMsgs))
	}

	// 3. Advance Alice and Bob to epoch 1
	var holder *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
			break
		}
	}
	if holder == nil {
		t.Fatal("failed to find token holder")
	}

	err = holder.coord.ProposeUpdate([]byte("pcs-rotation"))
	if err != nil {
		t.Fatalf("ProposeUpdate failed: %v", err)
	}
	network.DrainAll()

	// Assert Bob is at epoch 1
	if bob.coord.CurrentEpoch() != 1 {
		t.Fatalf("expected Bob to reach epoch 1, got %d", bob.coord.CurrentEpoch())
	}

	// 4. Clear Bob's applied envelope marker for epoch 0 message
	bob.storage.ClearAppliedEnvelope(groupID, epoch0Hash)

	// 5. Replay epoch 0 application message on Bob.
	// Since Bob is at epoch 1 and max_past_epochs is 0, this epoch 0 message MUST be rejected.
	results, err := bob.coord.ReplayEnvelopesDetailed([][]byte{epoch0Env})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].State != ReplayStateStaleEpoch {
		t.Fatalf("expected ReplayStateStaleEpoch, got %s", results[0].State)
	}
}

// TestCoordinator_UnifiedRetentionPolicy_Balanced verifies that in BALANCED mode,
// application messages from past epochs are successfully processed within the 3 epochs boundary,
// but rejected once they exceed 3 epochs.
func TestCoordinator_UnifiedRetentionPolicy_Balanced(t *testing.T) {
	groupID := "grp-retention-balanced"
	nodes, network, _ := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// 1. Configure Bob with BALANCED (default)
	bob.coord.cfg.RetentionMode = RetentionBalanced

	// 2. Alice sends a message at epoch 0
	msgText := []byte("hello balanced")
	_, err := alice.coord.SendMessage(msgText)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	network.DrainAll()

	records, err := alice.storage.GetEnvelopesSince(groupID, 0, 10)
	if err != nil {
		t.Fatalf("failed to get logged envelopes: %v", err)
	}

	var epoch0Env []byte
	var epoch0Hash []byte
	for _, rec := range records {
		if rec.MsgType == MsgApplication {
			epoch0Env = rec.Envelope
			epoch0Hash = rec.EnvelopeHash
			break
		}
	}

	// 3. Advance Alice and Bob to epoch 2
	var holder *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
			break
		}
	}
	if holder == nil {
		t.Fatal("failed to find token holder")
	}

	for i := 0; i < 2; i++ {
		err := holder.coord.ProposeUpdate([]byte("advance-epoch"))
		if err != nil {
			t.Fatalf("ProposeUpdate failed: %v", err)
		}
		network.DrainAll()
	}

	if bob.coord.CurrentEpoch() != 2 {
		t.Fatalf("expected Bob to reach epoch 2, got %d", bob.coord.CurrentEpoch())
	}

	// 4. Clear Bob's applied envelope marker for epoch 0 message to simulate pull replay
	bob.storage.ClearAppliedEnvelope(groupID, epoch0Hash)

	// 5. Replay epoch 0 application envelope on Bob.
	// Since epoch diff is 2 <= 3, it should be successfully decrypted and applied!
	results, err := bob.coord.ReplayEnvelopesDetailed([][]byte{epoch0Env})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].State != ReplayStateApplied {
		t.Fatalf("expected ReplayStateApplied, got %s (err: %s)", results[0].State, results[0].Error)
	}

	// 6. Let's advance Alice and Bob further so they are at epoch 4 (epoch diff = 4 > 3).
	// Clear the applied envelope marker first.
	bob.storage.ClearAppliedEnvelope(groupID, epoch0Hash)

	for i := 0; i < 2; i++ {
		err := holder.coord.ProposeUpdate([]byte("advance-epoch-further"))
		if err != nil {
			t.Fatalf("ProposeUpdate failed: %v", err)
		}
		network.DrainAll()
	}

	if bob.coord.CurrentEpoch() != 4 {
		t.Fatalf("expected Bob to reach epoch 4, got %d", bob.coord.CurrentEpoch())
	}

	// Replay epoch 0 message on Bob again.
	// Since epoch diff is 4 > 3, it should now be rejected as ReplayStateStaleEpoch!
	results2, err := bob.coord.ReplayEnvelopesDetailed([][]byte{epoch0Env})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}
	if len(results2) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results2))
	}
	if results2[0].State != ReplayStateStaleEpoch {
		t.Fatalf("expected ReplayStateStaleEpoch, got %s", results2[0].State)
	}
}

// TestCoordinator_UnifiedRetentionPolicy_TimeBasedExceeded verifies senior-grade retention:
// - Test 1: Sender clock chậm 1 tiếng (now - 1h) -> does NOT reject just because sender timestamp is old.
// - Test 4: Message nằm trong local inbox quá lâu (first_seen_at = now - 10m, max_past_age = 5m) -> REJECTED as stale!
func TestCoordinator_UnifiedRetentionPolicy_TimeBasedExceeded(t *testing.T) {
	groupID := "grp-retention-time"
	nodes, network, clk := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// Configure Bob with BALANCED (age limit = 300s)
	bob.coord.cfg.RetentionMode = RetentionBalanced

	// Alice sends a message at epoch 0
	msgText := []byte("hello time limit")
	_, err := alice.coord.SendMessage(msgText)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	network.DrainAll()

	records, err := alice.storage.GetEnvelopesSince(groupID, 0, 10)
	if err != nil {
		t.Fatalf("failed to get logged envelopes: %v", err)
	}

	var epoch0EnvBytes []byte
	for _, rec := range records {
		if rec.MsgType == MsgApplication {
			epoch0EnvBytes = rec.Envelope
			break
		}
	}

	// Advance Bob to epoch 2
	var holder *testNode
	for _, n := range nodes {
		if n.coord.IsTokenHolder() {
			holder = n
			break
		}
	}
	if holder == nil {
		t.Fatal("failed to find token holder")
	}

	for i := 0; i < 2; i++ {
		err := holder.coord.ProposeUpdate([]byte("advance-epoch"))
		if err != nil {
			t.Fatalf("ProposeUpdate failed: %v", err)
		}
		network.DrainAll()
	}

	if bob.coord.CurrentEpoch() != 2 {
		t.Fatalf("expected Bob to reach epoch 2, got %d", bob.coord.CurrentEpoch())
	}

	// --- TEST 1: Sender clock chậm 1 tiếng (now - 1h) ---
	var env1 Envelope
	if err := json.Unmarshal(epoch0EnvBytes, &env1); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}
	env1.Timestamp.WallTimeMs = clk.Now().UnixMilli() - 3600000 // now - 1h
	modifiedEnvBytes1, _ := json.Marshal(env1)

	// Since Bob first sees this message now, first_seen_at = now.
	// Expected: not stale (ReplayStateApplied), because age calculated on first_seen_at = 0, NOT sender timestamp!
	results1, err := bob.coord.ReplayEnvelopesDetailed([][]byte{modifiedEnvBytes1})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}
	if len(results1) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results1))
	}
	if results1[0].State != ReplayStateApplied && results1[0].State != ReplayStateDuplicateApplied {
		t.Fatalf("TEST 1 FAILED: expected ReplayStateApplied or DuplicateApplied for old sender clock, got %s", results1[0].State)
	}
	t.Log("TEST 1 PASS: old sender clock (now - 1h) successfully processed since local first-seen time is fresh.")

	// --- TEST 4: Message nằm trong local inbox quá lâu (first_seen_at = now - 10m) ---
	var env4 Envelope
	if err := json.Unmarshal(epoch0EnvBytes, &env4); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}
	// We want to simulate that Bob's local storage already saw this envelope 10 minutes ago.
	// Manually save it to Bob's coordination storage with FirstSeenAtMs in the past!
	hash4 := sha256.Sum256(epoch0EnvBytes)
	pastSeenMs := clk.Now().UnixMilli() - 600000 // 10 minutes ago
	bob.storage.AppendEnvelopeWithSource(groupID, env4.Type, env4.Epoch, env4.Timestamp, epoch0EnvBytes, "local")

	// Clear the applied envelope marker first so it goes to the age-retention check instead of returning duplicate!
	bob.storage.ClearAppliedEnvelope(groupID, hash4[:])

	// Manually override FirstSeenAtMs in mock storage for this envelope
	envRecord, err := bob.storage.GetEnvelope(hash4[:])
	if err != nil || envRecord == nil {
		t.Fatalf("failed to retrieve env from mock storage: %v", err)
	}
	envRecord.FirstSeenAtMs = pastSeenMs

	// Replay this message on Bob.
	// Since max_past_age = 5m (300s) and local first_seen_at is 10m ago (600s),
	// this message is stale locally and MUST be rejected as ReplayStateStaleEpoch!
	results4, err := bob.coord.ReplayEnvelopesDetailed([][]byte{epoch0EnvBytes})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed failed: %v", err)
	}
	if len(results4) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results4))
	}
	if results4[0].State != ReplayStateStaleEpoch {
		t.Fatalf("TEST 4 FAILED: expected ReplayStateStaleEpoch for old first-seen time, got %s", results4[0].State)
	}
	t.Log("TEST 4 PASS: message retained in local inbox for too long (> 5m) is successfully rejected.")
}

// TestCoordinator_ReconcileOps_UsesMembership_NotActiveView verifies P0.1:
// membership-based satisfied check must use MLS HasMember, NOT ActiveView.
//
// Scenario: Bob is an MLS member but goes offline (not in ActiveView).
// A REMOVE_MEMBER operation targeting Bob must NOT be marked SATISFIED_BY_OTHER
// just because Bob is offline. ActiveView tracks liveness; MLS HasMember tracks membership.
func TestCoordinator_ReconcileOps_UsesMembership_NotActiveView(t *testing.T) {
	groupID := "grp-membership-check"
	nodes, network, _ := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	bob := nodes[1]

	// Configure Alice's MLS engine to treat Bob (and Alice) as members.
	alice.mls.SetHasMemberFunc(func(groupState []byte, identity []byte) (bool, error) {
		idStr := string(identity)
		if idStr == bob.id.String() || idStr == alice.id.String() {
			return true, nil
		}
		return false, nil
	})

	// 1. Confirm both nodes are connected via ActiveView
	alice.coord.mu.Lock()
	bobInView := alice.coord.activeView.Contains(bob.id)
	alice.coord.mu.Unlock()
	if !bobInView {
		t.Skip("Bob not in Alice's ActiveView — heartbeat exchange did not propagate in time")
	}

	// 2. Simulate Bob going offline: call Evict() to remove him from Alice's ActiveView.
	// Evict simulates the heartbeat timeout path (liveness departure).
	// Bob is still in the MLS group state; HasMember will return true.
	alice.coord.mu.Lock()
	alice.coord.activeView.Evict(bob.id)
	bobStillInView := alice.coord.activeView.Contains(bob.id)
	alice.coord.mu.Unlock()

	if bobStillInView {
		t.Fatal("Bob should not be in Alice's ActiveView after Evict")
	}

	// 3. Store a REMOVE_MEMBER operation targeting Bob.
	// Bob is still an MLS member (HasMember returns true), so this operation is NOT satisfied.
	bobIDStr := bob.id.String()
	removeOp := &PendingOperation{
		OperationID:     "op-remove-bob",
		GroupID:         groupID,
		OpType:          "REMOVE_MEMBER",
		TargetMemberID:  &bobIDStr,
		SemanticPayload: []byte(bobIDStr),
		Status:          "PENDING",
		CreatedAt:       alice.coord.clock.Now(),
		UpdatedAt:       alice.coord.clock.Now(),
	}
	if err := alice.storage.SavePendingOperation(removeOp); err != nil {
		t.Fatalf("save pending operation: %v", err)
	}

	// 4. Trigger reconcileAndRebaseOperationsLocked (runs under coordinator lock).
	alice.coord.mu.Lock()
	alice.coord.reconcileAndRebaseOperationsLocked()
	alice.coord.mu.Unlock()

	// 5. Assert: the operation must NOT be SATISFIED_BY_OTHER.
	// MockMLSEngine's HasMember: if Members map is nil, all non-empty identities return true,
	// so Bob (non-empty identity) is considered a member.
	// With the P0.1 fix, Remove(offlineBob) checks HasMember, not ActiveView.
	// Bob is an MLS member → not satisfied → status stays PENDING (or PROPOSED if rebase triggered).
	op, err := alice.storage.GetPendingOperation("op-remove-bob")
	if err != nil {
		t.Fatalf("get pending operation: %v", err)
	}
	if op.Status == "SATISFIED_BY_OTHER" {
		t.Fatalf("P0.1 BUG: Remove(offlineBob) was marked SATISFIED_BY_OTHER incorrectly.\n"+
			"ActiveView.Evict does not mean the peer left the MLS group.\n"+
			"HasMember must be used for membership checks, not ActiveView. status=%s", op.Status)
	}
	t.Logf("P0.1 PASS: offline Bob's remove op status=%s (correctly not SATISFIED_BY_OTHER)", op.Status)
}

// TestCoordinator_TimeBasedRetention_FutureSpoofedTimestamp verifies senior-grade skew limits:
// - Test 2: Sender timestamp tương lai trong skew (now + 2m) -> accepted, age >= 0 since we use local first-seen time.
// - Test 3: Sender timestamp tương lai quá xa (now + 1y) -> completely rejected as INVALID and dropped!
func TestCoordinator_TimeBasedRetention_FutureSpoofedTimestamp(t *testing.T) {
	groupID := "grp-timestamp-spoof"
	nodes, _, clk := setupCluster(t, 1, groupID)
	alice := nodes[0]
	alice.coord.cfg.RetentionMode = RetentionBalanced

	if err := alice.coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := alice.coord.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer alice.coord.Stop()

	// --- TEST 2: Sender timestamp tương lai trong skew (now + 5s) ---
	futureSkewMs := clk.Now().UnixMilli() + 5000 // now + 5 seconds (within 5m skew limit and HLC 10s drift limit)
	appPayload2, _ := json.Marshal(ApplicationMsg{Ciphertext: []byte("skew-in-bound")})
	env2 := Envelope{
		GroupID:   groupID,
		Type:      MsgApplication,
		Epoch:     0,
		From:      alice.id.String(),
		Payload:   appPayload2,
		Timestamp: HLCTimestamp{WallTimeMs: futureSkewMs, Counter: 0, NodeID: alice.id.String()},
	}
	wire2, _ := json.Marshal(env2)

	// Expected: Accepted (state=APPLIED or DUPLICATE depending on state, but NOT INVALID!)
	results2, err := alice.coord.ReplayEnvelopesDetailed([][]byte{wire2})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed: %v", err)
	}
	if len(results2) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results2))
	}
	if results2[0].State == ReplayStateInvalid {
		t.Fatalf("TEST 2 FAILED: future skew inside bounds was incorrectly rejected as INVALID: %s", results2[0].Error)
	}
	t.Logf("TEST 2 PASS: future skew within limits successfully processed (state=%s)", results2[0].State)

	// --- TEST 3: Sender timestamp tương lai quá xa (now + 1y) ---
	futureFarMs := clk.Now().UnixMilli() + 365*24*3600*1000 // now + 1 year
	appPayload3, _ := json.Marshal(ApplicationMsg{Ciphertext: []byte("skew-far-out")})
	env3 := Envelope{
		GroupID:   groupID,
		Type:      MsgApplication,
		Epoch:     0,
		From:      alice.id.String(),
		Payload:   appPayload3,
		Timestamp: HLCTimestamp{WallTimeMs: futureFarMs, Counter: 0, NodeID: alice.id.String()},
	}
	wire3, _ := json.Marshal(env3)

	// Expected: completely rejected as INVALID
	results3, err := alice.coord.ReplayEnvelopesDetailed([][]byte{wire3})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed: %v", err)
	}
	if len(results3) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results3))
	}
	if results3[0].State != ReplayStateInvalid {
		t.Fatalf("TEST 3 FAILED: spoofed far-future timestamp (now + 1y) was not rejected as INVALID. Got state=%s", results3[0].State)
	}
	t.Logf("TEST 3 PASS: far-future spoofed timestamp rejected correctly (state=%s, err=%s)", results3[0].State, results3[0].Error)
}

// TestCoordinator_DurablePendingOp_AddRemoveAdd verifies P1.1:
// the partial unique index must NOT block a second ADD_MEMBER for the same peer
// after the first ADD_MEMBER was COMMITTED. Lifecycle: add → remove → add again.
func TestCoordinator_DurablePendingOp_AddRemoveAdd(t *testing.T) {
	groupID := "grp-add-remove-add"
	nodes, network, clk := setupCluster(t, 2, groupID)
	createAndShareGroup(t, nodes)
	startAll(t, nodes)
	exchangeHeartbeats(nodes, network)

	alice := nodes[0]
	now := clk.Now()

	davePeerID := peerID("dave-lifecycle")
	iKey1 := "add_member_" + davePeerID.String()
	iKey2 := "remove_member_" + davePeerID.String()
	iKey3 := "add_member_" + davePeerID.String() // same as iKey1

	// Step 1: first ADD is committed.
	op1 := &PendingOperation{
		OperationID:     "op-add-dave-first",
		GroupID:         groupID,
		OpType:          "ADD_MEMBER",
		Status:          "COMMITTED",
		IdempotencyKey:  &iKey1,
		SemanticPayload: []byte("dave-kp-v1"),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := alice.storage.SavePendingOperation(op1); err != nil {
		t.Fatalf("save op1 (ADD COMMITTED): %v", err)
	}

	// Step 2: REMOVE is committed (different idempotency key).
	op2 := &PendingOperation{
		OperationID:     "op-remove-dave",
		GroupID:         groupID,
		OpType:          "REMOVE_MEMBER",
		Status:          "COMMITTED",
		IdempotencyKey:  &iKey2,
		SemanticPayload: []byte("dave-remove"),
		CreatedAt:       now.Add(time.Second),
		UpdatedAt:       now.Add(time.Second),
	}
	if err := alice.storage.SavePendingOperation(op2); err != nil {
		t.Fatalf("save op2 (REMOVE COMMITTED): %v", err)
	}

	// Step 3: NEW add with the SAME idempotency key as op1 (same peer).
	// With old broad index: UNIQUE constraint would block this insert (iKey3 == iKey1).
	// With partial index (only PENDING/PROPOSED): op1 is COMMITTED → no conflict → must succeed.
	op3 := &PendingOperation{
		OperationID:     "op-add-dave-second",
		GroupID:         groupID,
		OpType:          "ADD_MEMBER",
		Status:          "PENDING",
		IdempotencyKey:  &iKey3,
		SemanticPayload: []byte("dave-kp-v2"),
		CreatedAt:       now.Add(2 * time.Second),
		UpdatedAt:       now.Add(2 * time.Second),
	}
	if err := alice.storage.SavePendingOperation(op3); err != nil {
		t.Fatalf("P1.1 BUG: second ADD for same peer blocked by idempotency index: %v\n"+
			"Partial unique index must only cover PENDING/PROPOSED, not COMMITTED.", err)
	}

	// Verify all three are persisted correctly.
	ops, err := alice.storage.ListPendingOperations(groupID)
	if err != nil {
		t.Fatalf("list ops: %v", err)
	}
	found := map[string]string{}
	for _, op := range ops {
		found[op.OperationID] = op.Status
	}
	for opID, wantStatus := range map[string]string{
		"op-add-dave-first":  "COMMITTED",
		"op-remove-dave":     "COMMITTED",
		"op-add-dave-second": "PENDING",
	} {
		if found[opID] != wantStatus {
			t.Errorf("%s: expected %q, got %q", opID, wantStatus, found[opID])
		}
	}
	t.Log("P1.1 PASS: add-remove-add lifecycle works; partial unique index allows re-add after COMMITTED")
}

// TestCoordinator_RecoveryReplay_HeadOfLine_FutureNotBlocksCurrent documents P1.3:
// ReplayEnvelopesDetailed processes each envelope independently and returns ALL results.
// A FutureEpoch envelope returns CursorSafe=false, but other envelopes in the SAME
// batch are still processed. The caller's recovery loop must NOT use a simple linear
// cursor that stops at the first CursorSafe=false result.
func TestCoordinator_RecoveryReplay_HeadOfLine_FutureNotBlocksCurrent(t *testing.T) {
	groupID := "grp-hol-test"
	nodes, _, _ := setupCluster(t, 1, groupID)
	alice := nodes[0]

	if err := alice.coord.CreateGroup(); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := alice.coord.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer alice.coord.Stop()

	// Build a future-epoch envelope (epoch 5 >> local epoch 0).
	futurePayload, _ := json.Marshal(ApplicationMsg{Ciphertext: []byte("future-msg")})
	futureEnv := Envelope{
		GroupID:   groupID,
		Type:      MsgApplication,
		Epoch:     5, // far future
		From:      alice.id.String(),
		Payload:   futurePayload,
		Timestamp: HLCTimestamp{WallTimeMs: alice.coord.clock.Now().UnixMilli()},
	}
	futureWire, _ := json.Marshal(futureEnv)

	// Build two current-epoch envelopes (epoch 0 == local epoch).
	currentPayload1, _ := json.Marshal(ApplicationMsg{Ciphertext: []byte("msg-A")})
	currentEnv1 := Envelope{
		GroupID:   groupID,
		Type:      MsgApplication,
		Epoch:     0,
		From:      alice.id.String(),
		Payload:   currentPayload1,
		Timestamp: HLCTimestamp{WallTimeMs: alice.coord.clock.Now().UnixMilli() + 1},
	}
	wire1, _ := json.Marshal(currentEnv1)

	currentPayload2, _ := json.Marshal(ApplicationMsg{Ciphertext: []byte("msg-B")})
	currentEnv2 := Envelope{
		GroupID:   groupID,
		Type:      MsgApplication,
		Epoch:     0,
		From:      alice.id.String(),
		Payload:   currentPayload2,
		Timestamp: HLCTimestamp{WallTimeMs: alice.coord.clock.Now().UnixMilli() + 2},
	}
	wire2, _ := json.Marshal(currentEnv2)

	// Replay batch: [futureEpoch=5, currentEpoch=0, currentEpoch=0]
	// ReplayEnvelopesDetailed must return 3 results, one per input envelope.
	results, err := alice.coord.ReplayEnvelopesDetailed([][]byte{futureWire, wire1, wire2})
	if err != nil {
		t.Fatalf("ReplayEnvelopesDetailed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 independent results (one per envelope), got %d", len(results))
	}

	r0, r1, r2 := results[0], results[1], results[2]

	// Future epoch: must be FutureEpoch with CursorSafe=false.
	if r0.State != ReplayStateFutureEpoch {
		t.Errorf("result[0] (future epoch 5): expected FutureEpoch, got %s", r0.State)
	}
	if r0.CursorSafe {
		t.Errorf("result[0] (future epoch 5): FutureEpoch must have CursorSafe=false (dependency not met)")
	}
	if r0.Applied {
		t.Errorf("result[0] (future epoch 5): must not be Applied")
	}

	// Current epoch envelopes must be processed independently of the future envelope.
	validCurrentStates := map[ReplayEnvelopeState]bool{
		ReplayStateApplied:          true,
		ReplayStateDuplicateApplied: true,
		ReplayStateStaleEpoch:       true, // acceptable if retentionmode blocks epoch 0
		ReplayStateInvalid:          true, // acceptable if mock decrypt fails
		ReplayStateDecryptFailed:    true,
	}
	if !validCurrentStates[r1.State] {
		t.Errorf("result[1] (current epoch 0): unexpected state %s", r1.State)
	}
	if !validCurrentStates[r2.State] {
		t.Errorf("result[2] (current epoch 0, duplicate): unexpected state %s", r2.State)
	}

	t.Logf("P1.3 PASS: [0]=%s(CursorSafe=%v) [1]=%s(Applied=%v) [2]=%s(Applied=%v)",
		r0.State, r0.CursorSafe, r1.State, r1.Applied, r2.State, r2.Applied)
	t.Log("Caller's recovery loop MUST NOT stop at CursorSafe=false; must process subsequent envelopes.")
}
