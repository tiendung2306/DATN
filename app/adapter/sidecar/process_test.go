package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRustCrateRootFromBinary_TargetLayout(t *testing.T) {
	binPath := filepath.Join("repo", "crypto-engine", "target", "debug", "crypto-engine.exe")
	root, ok := rustCrateRootFromBinary(binPath)
	if !ok {
		t.Fatal("expected target-layout binary to resolve crate root")
	}
	if want := filepath.Join("repo", "crypto-engine"); root != want {
		t.Fatalf("crate root=%q want %q", root, want)
	}
}

func TestEnsureBinaryFresh_RejectsStaleBinary(t *testing.T) {
	crateRoot := t.TempDir()
	srcRoot := filepath.Join(crateRoot, "src")
	binPath := filepath.Join(crateRoot, "target", "debug", "crypto-engine.exe")
	if err := os.MkdirAll(srcRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll src: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatalf("MkdirAll bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(crateRoot, "Cargo.toml"), []byte("[package]\nname='crypto-engine'\n"), 0o644); err != nil {
		t.Fatalf("WriteFile Cargo.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "mls.rs"), []byte("fn main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile mls.rs: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("bin"), 0o644); err != nil {
		t.Fatalf("WriteFile bin: %v", err)
	}

	binTime := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	srcTime := binTime.Add(2 * time.Hour)
	for _, path := range []string{binPath, filepath.Join(crateRoot, "Cargo.toml")} {
		if err := os.Chtimes(path, binTime, binTime); err != nil {
			t.Fatalf("Chtimes %q: %v", path, err)
		}
	}
	if err := os.Chtimes(filepath.Join(srcRoot, "mls.rs"), srcTime, srcTime); err != nil {
		t.Fatalf("Chtimes src: %v", err)
	}

	err := ensureBinaryFresh(binPath)
	if err == nil {
		t.Fatal("expected stale binary error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "stale") || !strings.Contains(msg, "cargo build") {
		t.Fatalf("unexpected error: %q", msg)
	}
}

func TestEnsureBinaryFresh_AllowsFreshBinary(t *testing.T) {
	crateRoot := t.TempDir()
	srcRoot := filepath.Join(crateRoot, "src")
	binPath := filepath.Join(crateRoot, "target", "debug", "crypto-engine.exe")
	if err := os.MkdirAll(srcRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll src: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatalf("MkdirAll bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "mls.rs"), []byte("fn main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("bin"), 0o644); err != nil {
		t.Fatalf("WriteFile bin: %v", err)
	}

	srcTime := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	binTime := srcTime.Add(2 * time.Hour)
	if err := os.Chtimes(filepath.Join(srcRoot, "mls.rs"), srcTime, srcTime); err != nil {
		t.Fatalf("Chtimes src: %v", err)
	}
	if err := os.Chtimes(binPath, binTime, binTime); err != nil {
		t.Fatalf("Chtimes bin: %v", err)
	}

	if err := ensureBinaryFresh(binPath); err != nil {
		t.Fatalf("ensureBinaryFresh returned error: %v", err)
	}
}
