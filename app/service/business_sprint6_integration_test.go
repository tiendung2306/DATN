//go:build business_integration

// Sprint 6 — BI-088, BI-093–BI-097 subset (offline sync + file transfer). Blind-store BI-091–092 deferred without harness hooks.

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBusinessP1_Sprint6_BI088_TriggerOfflineSync_NoCrash(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if err := rt.TriggerOfflineSync(); err != nil {
		t.Fatalf("TriggerOfflineSync: %v", err)
	}
}

func TestBusinessP1_Sprint6_BI093_PrepareOutgoingFileTransfer(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "dm-file-prep"
	if err := rt.CreateGroupChat(gid, "dm", ""); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(path, []byte("integration-file-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	dto, err := rt.PrepareOutgoingFileTransfer(gid, path)
	if err != nil {
		t.Fatalf("PrepareOutgoingFileTransfer: %v", err)
	}
	if dto.FileID == "" || dto.PlaintextSize <= 0 {
		t.Fatalf("dto=%+v", dto)
	}
}

func TestBusinessP1_Sprint6_BI094_PrepareOutgoing_MissingPath(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "dm-file-miss"
	if err := rt.CreateGroupChat(gid, "dm", ""); err != nil {
		t.Fatal(err)
	}
	_, err := rt.PrepareOutgoingFileTransfer(gid, filepath.Join(t.TempDir(), "nonexistent.bin"))
	if err == nil || !strings.Contains(err.Error(), "stat") {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint6_BI094_PrepareOutgoing_EmptyFileRejected(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "dm-file-empty"
	if err := rt.CreateGroupChat(gid, "dm", ""); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := rt.PrepareOutgoingFileTransfer(gid, path)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint6_BI097_PullFileTransferFromPeer_NotFound(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "dm-file-dl"
	if err := rt.CreateGroupChat(gid, "dm", ""); err != nil {
		t.Fatal(err)
	}
	sender := testPeerID(t)
	dest := filepath.Join(t.TempDir(), "out.bin")
	err := rt.PullFileTransferFromPeer(gid, "no-such-file-id", sender, dest)
	if err == nil {
		t.Fatal("expected error")
	}
}
