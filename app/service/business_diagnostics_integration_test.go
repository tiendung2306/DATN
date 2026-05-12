//go:build business_integration

// Sprint 5 — BI-102–BI-106 network diagnostics.

package service

import (
	"os"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestBusinessP1_Sprint5_BI102_ValidateMultiaddr_Valid(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	_, pub, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	addr := "/ip4/127.0.0.1/tcp/4001/p2p/" + pid.String()
	if err := rt.ValidateMultiaddr(addr); err != nil {
		t.Fatal(err)
	}
}

func TestBusinessP1_Sprint5_BI103_ValidateMultiaddr_Invalid(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if err := rt.ValidateMultiaddr("not-a-multiaddr"); err == nil {
		t.Fatal("expected error")
	}
	if err := rt.ValidateMultiaddr("/ip4/127.0.0.1/tcp/4001"); err == nil {
		t.Fatal("expected error without /p2p/ peer id")
	}
}

func TestBusinessP1_Sprint5_BI104_SetBootstrapAddress_Persists(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	_, pub, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	addr := "/ip4/127.0.0.1/tcp/9999/p2p/" + pid.String()
	if err := rt.SetBootstrapAddress(addr); err != nil {
		t.Fatalf("SetBootstrapAddress: %v", err)
	}
	st, err := rt.GetNetworkSettings()
	if err != nil {
		t.Fatal(err)
	}
	if st.BootstrapAddr != addr {
		t.Fatalf("settings.bootstrap=%q want %q", st.BootstrapAddr, addr)
	}
	rt.mu.RLock()
	d := rt.db
	rt.mu.RUnlock()
	raw, err := d.GetConfig(bootstrapOverrideConfigKey)
	if err != nil || string(raw) != addr {
		t.Fatalf("persisted bootstrap: err=%v raw=%q", err, string(raw))
	}
}

func TestBusinessP1_Sprint5_BI105_ReconnectP2P_NoPanic(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if err := rt.ReconnectP2P(); err != nil {
		t.Fatalf("ReconnectP2P: %v", err)
	}
}

func TestBusinessP1_Sprint5_BI106_ExportDiagnostics_WritesJSON(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	path, err := rt.ExportDiagnostics()
	if err != nil {
		t.Fatalf("ExportDiagnostics: %v", err)
	}
	if path == "" {
		t.Fatal("empty path")
	}
	if !strings.HasSuffix(path, ".json") {
		t.Fatalf("path=%q", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"app_state"`) {
		snippet := string(b)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		t.Fatalf("unexpected diagnostics content: %s", snippet)
	}
	_ = os.Remove(path)
}
