//go:build business_integration

// Sprint 2 — BI-108–BI-112 (cross-cutting errors, concurrency, events).

package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"app/adapter/store"
	"app/coordination"
)

func TestBusinessP1_Sprint2_BI108_GroupNotFound_SendAndMembers(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	err := rt.SendGroupMessage("ghost-group-108", "hi")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, coordination.ErrGroupNotFound) && !strings.Contains(err.Error(), "group metadata unavailable") {
		t.Fatalf("SendGroupMessage err=%v", err)
	}
	_, err = rt.GetGroupMembers("ghost-group-108")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not in group") {
		t.Fatalf("GetGroupMembers err=%v", err)
	}
}

func TestBusinessP1_Sprint2_BI109_RuntimeNotInitialized(t *testing.T) {
	dir := t.TempDir()
	dbPath := businessDBPath(dir)
	businessSeedAuthorizedUser(t, dbPath)
	cfg := businessDefaultConfig(dbPath)
	rt := NewRuntime(cfg)
	rt.SetContext(context.Background())
	// Intentionally no Startup — db/coordination stack not bound.
	err := rt.RemoveMemberFromGroup("any", testPeerID(t))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), ErrCodeRuntimeNotInitialized) {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint2_BI110_ConcurrentAcceptInvite(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	kp, err := rt.GenerateKeyPackage()
	if err != nil {
		t.Fatalf("GenerateKeyPackage: %v", err)
	}
	pubKP, err := hex.DecodeString(kp.PublicHex)
	if err != nil {
		t.Fatal(err)
	}
	privKP, err := hex.DecodeString(kp.BundlePrivateHex)
	if err != nil {
		t.Fatal(err)
	}
	info, err := rt.GetOnboardingInfo()
	if err != nil {
		t.Fatal(err)
	}
	rt.mu.RLock()
	database := rt.db
	rt.mu.RUnlock()
	if database == nil {
		t.Fatal("nil db")
	}
	if err := database.SaveKPBundle(info.PeerID, pubKP, privKP); err != nil {
		t.Fatalf("SaveKPBundle: %v", err)
	}
	gid := "grp-bi110"
	state := bizMockGroupState{
		GroupID:  gid,
		Epoch:    1,
		TreeHash: hex.EncodeToString(bizMockTreeHash(1)),
	}
	welcomeBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SavePendingInvite(&store.PendingInvite{
		GroupID:      gid,
		GroupType:    "group",
		WelcomeBytes: welcomeBytes,
	}); err != nil {
		t.Fatalf("SavePendingInvite: %v", err)
	}
	invID := store.PendingInviteID(gid, welcomeBytes)

	var err1, err2 error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err1 = rt.AcceptInvite(invID)
	}()
	go func() {
		defer wg.Done()
		err2 = rt.AcceptInvite(invID)
	}()
	wg.Wait()
	if err1 != nil && err2 != nil {
		t.Fatalf("both AcceptInvite failed: %v, %v", err1, err2)
	}
	has, err := database.HasGroup(gid)
	if err != nil || !has {
		t.Fatalf("expected joined group has=%v err=%v", has, err)
	}
}

func TestBusinessP1_Sprint2_BI111_ConcurrentCreateSameGroup(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "race-111"
	var ok, fail atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := rt.CreateGroupChat(gid, "group", "")
			if err == nil {
				ok.Add(1)
				return
			}
			if strings.Contains(err.Error(), "already in group") {
				fail.Add(1)
				return
			}
			t.Errorf("unexpected error: %v", err)
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || fail.Load() != 1 {
		t.Fatalf("ok=%d fail=%d want 1 and 1", ok.Load(), fail.Load())
	}
}

type sprint2CaptureSink struct {
	mu    sync.Mutex
	names []string
}

func (s *sprint2CaptureSink) Emit(_ context.Context, event string, _ map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = append(s.names, event)
}

func (s *sprint2CaptureSink) has(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.names {
		if strings.HasPrefix(n, prefix) || strings.Contains(n, prefix) {
			return true
		}
	}
	return false
}

func TestBusinessP1_Sprint2_BI112_RuntimeEvents_GroupLifecycle(t *testing.T) {
	sink := &sprint2CaptureSink{}
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	rt.SetEventSink(sink)

	gid := "evt-112"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	if err := rt.SendGroupMessage(gid, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := rt.LeaveGroup(gid); err != nil {
		t.Fatal(err)
	}

	if !sink.has("group") {
		t.Fatalf("expected group-related events, got %#v", sink.names)
	}
}
