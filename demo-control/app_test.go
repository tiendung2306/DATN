package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDemoClusterTargetSetSelectedNodesOnly(t *testing.T) {
	ws := DemoWorkspace{
		HeadlessLane: DemoLaneConfig{
			DemoOwnerNode: "node-01",
		},
	}
	statuses := []InstanceStatus{
		testStatus("node-01", true, "AUTHORIZED", "peer-01"),
		testStatus("node-02", true, "ADMIN_READY", "peer-02"),
		testStatus("node-03", true, "AUTHORIZED", "peer-03"),
		testStatus("node-04", false, "", ""),
		testStatus("node-05", false, "", ""),
	}

	target := resolveDemoClusterTargetSet(ws, statuses, []string{"node-01", "node-02", "node-03"})
	if target.LastError != "" {
		t.Fatalf("expected no error, got %q", target.LastError)
	}
	if got := strings.Join(target.TargetNodeIDs, ","); got != "node-01,node-02,node-03" {
		t.Fatalf("unexpected target nodes: %s", got)
	}
	if len(target.ExpectedPeers) != 3 || target.EligibleCount != 3 {
		t.Fatalf("expected 3 eligible peers, got peers=%d eligible=%d", len(target.ExpectedPeers), target.EligibleCount)
	}
}

func TestResolveDemoClusterTargetSetFallsBackToRunningNodes(t *testing.T) {
	ws := DemoWorkspace{}
	statuses := []InstanceStatus{
		testStatus("node-01", true, "AUTHORIZED", "peer-01"),
		testStatus("node-02", false, "", ""),
		testStatus("node-03", true, "AUTHORIZED", "peer-03"),
	}

	target := resolveDemoClusterTargetSet(ws, statuses, nil)
	if target.LastError != "" {
		t.Fatalf("expected no error, got %q", target.LastError)
	}
	if got := strings.Join(target.TargetNodeIDs, ","); got != "node-01,node-03" {
		t.Fatalf("unexpected fallback target nodes: %s", got)
	}
}

func TestResolveDemoClusterTargetSetBlocksSelectedNodeOnly(t *testing.T) {
	ws := DemoWorkspace{}
	statuses := []InstanceStatus{
		testStatus("node-01", true, "AUTHORIZED", "peer-01"),
		testStatus("node-02", false, "", ""),
		testStatus("node-03", true, "AUTHORIZED", "peer-03"),
	}

	target := resolveDemoClusterTargetSet(ws, statuses, []string{"node-02"})
	if !strings.Contains(target.LastError, "node-02 offline") {
		t.Fatalf("expected node-02 blocking error, got %q", target.LastError)
	}
	if strings.Contains(target.LastError, "node-03") {
		t.Fatalf("unexpected non-selected node in error: %q", target.LastError)
	}
}

func TestChooseDemoOwnerRejectsOwnerOutsideTargetSet(t *testing.T) {
	ws := DemoWorkspace{
		HeadlessLane: DemoLaneConfig{DemoOwnerNode: "node-01"},
	}
	target := demoClusterTargetSet{
		ExpectedPeers: map[string]string{
			"node-01": "peer-01",
		},
		TargetStatuses: []InstanceStatus{testStatus("node-01", true, "AUTHORIZED", "peer-01")},
	}

	if _, err := chooseDemoOwner(ws, target, "node-02"); err == nil {
		t.Fatal("expected owner validation error")
	}
}

func TestEnsureRuntimeSeededCopiesTemplateOnce(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	templateDir := filepath.Join(root, "template")
	if err := os.MkdirAll(templateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	templateDB := filepath.Join(templateDir, "app.db")
	if err := os.WriteFile(templateDB, []byte("seeded"), 0o600); err != nil {
		t.Fatal(err)
	}

	warning, err := ensureRuntimeSeeded(DemoInstanceProfile{
		ID:          "node-01",
		RuntimeDir:  runtimeDir,
		TemplateDir: templateDir,
		DBPath:      filepath.Join(runtimeDir, "app.db"),
	})
	if err != nil {
		t.Fatalf("ensureRuntimeSeeded failed: %v", err)
	}
	if !strings.Contains(warning, "auto-seeded") {
		t.Fatalf("expected auto-seeded warning, got %q", warning)
	}
	raw, err := os.ReadFile(filepath.Join(runtimeDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "seeded" {
		t.Fatalf("unexpected runtime db content: %q", string(raw))
	}
}

func TestEnsureRuntimeSeededPreservesExistingRuntime(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	templateDir := filepath.Join(root, "template")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(templateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "app.db"), []byte("runtime"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "app.db"), []byte("template"), 0o600); err != nil {
		t.Fatal(err)
	}

	warning, err := ensureRuntimeSeeded(DemoInstanceProfile{
		ID:          "node-01",
		RuntimeDir:  runtimeDir,
		TemplateDir: templateDir,
		DBPath:      filepath.Join(runtimeDir, "app.db"),
	})
	if err != nil {
		t.Fatalf("ensureRuntimeSeeded failed: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	raw, err := os.ReadFile(filepath.Join(runtimeDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "runtime" {
		t.Fatalf("expected existing runtime db to win, got %q", string(raw))
	}
}

func TestEnsureRuntimeSeededWarnsWithoutTemplate(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")

	warning, err := ensureRuntimeSeeded(DemoInstanceProfile{
		ID:         "node-01",
		RuntimeDir: runtimeDir,
		DBPath:     filepath.Join(runtimeDir, "app.db"),
	})
	if err != nil {
		t.Fatalf("ensureRuntimeSeeded failed: %v", err)
	}
	if !strings.Contains(warning, "no template") {
		t.Fatalf("expected missing template warning, got %q", warning)
	}
}

func TestNormalizeWorkspaceMigratesLegacyHeadlessPaths(t *testing.T) {
	repoRoot := t.TempDir()
	legacyRuntime := filepath.Join(repoRoot, ".demo-control", "runtimes", "node-01")
	legacyTemplate := filepath.Join(repoRoot, ".demo-control", "templates", "node-01")
	if err := os.MkdirAll(legacyRuntime, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyTemplate, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRuntime, "app.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyTemplate, "seed.txt"), []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}

	ws := DemoWorkspace{
		HeadlessLane: DemoLaneConfig{
			Instances: []DemoInstanceProfile{
				{
					ID:          "node-01",
					RuntimeDir:  legacyRuntime,
					TemplateDir: legacyTemplate,
					DBPath:      filepath.Join(legacyRuntime, "app.db"),
				},
			},
		},
	}

	warnings := normalizeWorkspace(&ws, repoRoot)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	expectedRuntime := filepath.Join(repoRoot, ".demo-control", "headless", "runtimes", "node-01")
	expectedTemplate := filepath.Join(repoRoot, ".demo-control", "headless", "templates", "node-01")
	if ws.HeadlessLane.Instances[0].RuntimeDir != expectedRuntime {
		t.Fatalf("unexpected runtime dir: %s", ws.HeadlessLane.Instances[0].RuntimeDir)
	}
	if ws.HeadlessLane.Instances[0].TemplateDir != expectedTemplate {
		t.Fatalf("unexpected template dir: %s", ws.HeadlessLane.Instances[0].TemplateDir)
	}
	if !fileExists(filepath.Join(expectedRuntime, "app.db")) {
		t.Fatal("expected migrated runtime db to exist")
	}
	if !fileExists(filepath.Join(expectedTemplate, "seed.txt")) {
		t.Fatal("expected migrated template file to exist")
	}
}

func TestNormalizeWorkspaceWarnsOnLegacyConflict(t *testing.T) {
	repoRoot := t.TempDir()
	legacyRuntime := filepath.Join(repoRoot, ".demo-control", "runtimes", "node-01")
	newRuntime := filepath.Join(repoRoot, ".demo-control", "headless", "runtimes", "node-01")
	if err := os.MkdirAll(legacyRuntime, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newRuntime, 0o700); err != nil {
		t.Fatal(err)
	}

	ws := DemoWorkspace{
		HeadlessLane: DemoLaneConfig{
			Instances: []DemoInstanceProfile{
				{
					ID:         "node-01",
					RuntimeDir: legacyRuntime,
					DBPath:     filepath.Join(legacyRuntime, "app.db"),
				},
			},
		},
	}

	warnings := normalizeWorkspace(&ws, repoRoot)
	if len(warnings) == 0 {
		t.Fatal("expected migration warning")
	}
	if ws.HeadlessLane.Instances[0].RuntimeDir != legacyRuntime {
		t.Fatalf("expected runtime dir to stay on legacy path during conflict, got %s", ws.HeadlessLane.Instances[0].RuntimeDir)
	}
}

func TestExeWorkingDirUsesExeFolder(t *testing.T) {
	got := exeWorkingDir(`E:\Projects\DATN\app\build\bin\SecureP2P.exe`, `E:\Projects\DATN\app`)
	want := `E:\Projects\DATN\app\build\bin`
	if got != want {
		t.Fatalf("expected exe working dir %q, got %q", want, got)
	}
}

func TestExeWorkingDirFallsBackWhenExeMissing(t *testing.T) {
	got := exeWorkingDir("", `E:\Projects\DATN\app`)
	want := `E:\Projects\DATN\app`
	if got != want {
		t.Fatalf("expected fallback working dir %q, got %q", want, got)
	}
}

func testStatus(id string, running bool, appState string, peerID string) InstanceStatus {
	return InstanceStatus{
		Profile:  DemoInstanceProfile{ID: id},
		Running:  running,
		AppState: appState,
		PeerID:   peerID,
	}
}
