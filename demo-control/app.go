package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	mu        sync.Mutex
	root      string
	workspace DemoWorkspace
	procs     map[string]*exec.Cmd
	errors    map[string]string
	scenario  ScenarioRunState
}

func NewApp() *App {
	return &App{
		procs:  make(map[string]*exec.Cmd),
		errors: make(map[string]string),
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	repoRoot, err := findRepoRoot()
	if err != nil {
		a.errors["workspace"] = err.Error()
		return
	}
	a.root = repoRoot
	ws, err := loadOrCreateWorkspace(repoRoot)
	if err != nil {
		a.errors["workspace"] = err.Error()
		return
	}
	a.workspace = ws
}

func (a *App) Shutdown(_ context.Context) {
	a.StopAll()
}

func (a *App) GetSnapshot() (ControlSnapshot, error) {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	statuses := a.refreshStatuses(ws)
	rules, _ := listFirewallRules()
	return ControlSnapshot{
		Workspace: ws,
		Instances: statuses,
		Firewall:  rules,
		Scenarios: builtInScenarios(),
	}, nil
}

func (a *App) Preflight() (PreflightResult, error) {
	a.mu.Lock()
	ws := a.workspace
	procs := make(map[string]*exec.Cmd)
	for k, v := range a.procs {
		procs[k] = v
	}
	a.mu.Unlock()

	out := PreflightResult{OK: true}
	if _, err := os.Stat(ws.AppDir); err != nil {
		out.OK = false
		out.Errors = append(out.Errors, "app dir not found: "+ws.AppDir)
	}
	if _, err := os.Stat(ws.AppExe); err != nil {
		out.Warnings = append(out.Warnings, "built exe not found, go_run fallback will be used: "+ws.AppExe)
	}
	for _, p := range ws.Instances {
		// Only check for port conflicts if the instance is NOT currently running under this controller!
		running := false
		if cmd := procs[p.ID]; cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
			running = true
		}

		if !running {
			if portBusy(p.ControlPort) {
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s control port %d is already in use", p.ID, p.ControlPort))
			}
			if portBusy(p.P2PPort) {
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s p2p port %d is already in use", p.ID, p.P2PPort))
			}
		}
	}
	if runtime.GOOS != "windows" {
		out.Warnings = append(out.Warnings, "firewall partition controls are Windows-only")
	} else {
		// Check if running as administrator on Windows
		cmd := exec.Command("net", "session")
		if err := cmd.Run(); err != nil {
			out.Warnings = append(out.Warnings, "Administrator privileges not detected. Firewall partitions will fail. Please run Demo Control as Administrator.")
		}
	}
	return out, nil
}

func (a *App) StartInstance(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	profile, ok := a.profileLocked(id)
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	if cmd := a.procs[id]; cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
		return nil
	}
	if err := os.MkdirAll(profile.RuntimeDir, 0o700); err != nil {
		return err
	}
	args := []string{
		"-runtime-dir", profile.RuntimeDir,
		"-db", profile.DBPath,
		"-p2p-port", fmt.Sprint(profile.P2PPort),
		"-control-port", fmt.Sprint(profile.ControlPort),
		"-control-token", a.workspace.Token,
		"-instance-label", profile.Label,
	}
	if profile.Bootstrap != "" {
		args = append(args, "-bootstrap", profile.Bootstrap)
	}
	if profile.Headless {
		args = append(args, "-headless")
	}
	if profile.StoreNode {
		args = append(args, "-store-node")
	}

	var cmd *exec.Cmd
	if profile.LaunchMode == "go_run" || !fileExists(a.workspace.AppExe) {
		cmd = exec.Command("go", append([]string{"run", "."}, args...)...)
		cmd.Dir = a.workspace.AppDir
	} else {
		cmd = exec.Command(a.workspace.AppExe, args...)
		cmd.Dir = a.workspace.AppDir
	}
	
	logPath := filepath.Join(profile.RuntimeDir, "stdout.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		a.errors[id] = "log file: " + err.Error()
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		a.errors[id] = err.Error()
		return err
	}
	a.procs[id] = cmd
	a.errors[id] = ""
	go func() {
		err := cmd.Wait()
		logFile.Close()
		a.mu.Lock()
		if err != nil {
			a.errors[id] = err.Error()
		}
		a.mu.Unlock()
	}()
	return nil
}

func killProcessTree(pid int) {
	if runtime.GOOS == "windows" {
		// /F forces termination, /T terminates specified process and any child processes
		_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(pid)).Run()
	} else {
		// On non-Windows platforms, we send SIGKILL to the process
		// We can also kill the process group if we set PGID, but usually Process.Kill() is standard.
	}
}

func (a *App) StopInstance(id string) error {
	graceful := false
	if err := a.controlAction(id, "shutdown"); err == nil {
		graceful = true
	}

	a.mu.Lock()
	cmd := a.procs[id]
	a.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		pid := cmd.Process.Pid
		if graceful {
			// Poll for up to 1.5s for graceful exit
			for i := 0; i < 15; i++ {
				a.mu.Lock()
				runningCmd := a.procs[id]
				a.mu.Unlock()
				if runningCmd == nil || runningCmd.ProcessState != nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Forcefully kill process tree (SecureP2P and its crypto-engine sidecar)
		killProcessTree(pid)
		_ = cmd.Process.Kill()
		time.Sleep(300 * time.Millisecond)
	}

	a.mu.Lock()
	delete(a.procs, id)
	a.mu.Unlock()
	return nil
}

func (a *App) KillInstance(id string) error {
	a.mu.Lock()
	cmd := a.procs[id]
	delete(a.procs, id)
	a.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		killProcessTree(cmd.Process.Pid)
		_ = cmd.Process.Kill()
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func (a *App) RestartInstance(id string) error {
	_ = a.StopInstance(id)
	time.Sleep(1000 * time.Millisecond)
	return a.StartInstance(id)
}

func (a *App) StartAll() error {
	for _, p := range a.workspace.Instances {
		if err := a.StartInstance(p.ID); err != nil {
			return err
		}
		time.Sleep(120 * time.Millisecond)
	}
	return nil
}

func (a *App) StopAll() error {
	// 1. Try to stop all instances gracefully
	for _, p := range a.workspace.Instances {
		_ = a.controlAction(p.ID, "shutdown")
	}

	// Wait up to 800ms for graceful shutdown
	time.Sleep(800 * time.Millisecond)

	// 2. Kill remaining managed process trees
	a.mu.Lock()
	for id, cmd := range a.procs {
		if cmd != nil && cmd.Process != nil {
			killProcessTree(cmd.Process.Pid)
			_ = cmd.Process.Kill()
		}
		delete(a.procs, id)
	}
	a.mu.Unlock()

	// 3. System-wide deep sweep: clean up any remaining orphaned SecureP2P or crypto-engine processes
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/IM", "SecureP2P.exe").Run()
		_ = exec.Command("taskkill", "/F", "/IM", "crypto-engine.exe").Run()
	} else {
		_ = exec.Command("killall", "-9", "SecureP2P").Run()
		_ = exec.Command("killall", "-9", "crypto-engine").Run()
	}

	return nil
}

func (a *App) ResetInstance(id string) error {
	a.mu.Lock()
	profile, ok := a.profileLocked(id)
	running := false
	if cmd := a.procs[id]; cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
		running = true
	}
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	if running {
		return fmt.Errorf("stop %s before reset", id)
	}
	if err := ensureDemoPath(a.root, profile.RuntimeDir); err != nil {
		return err
	}
	if err := os.RemoveAll(profile.RuntimeDir); err != nil {
		return err
	}
	if profile.TemplateDir != "" && fileExists(profile.TemplateDir) {
		if err := copyDir(profile.TemplateDir, profile.RuntimeDir); err != nil {
			return err
		}
	}
	return os.MkdirAll(profile.RuntimeDir, 0o700)
}

func (a *App) ResetAll() error {
	for _, p := range a.workspace.Instances {
		if err := a.ResetInstance(p.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) TriggerOfflineSync(id string) error { return a.controlAction(id, "trigger-offline-sync") }
func (a *App) ExportDiagnostics(id string) error  { return a.controlAction(id, "export-diagnostics") }
func (a *App) IsolateNode(id string) error {
	return a.ApplyPartition(PartitionSpec{ID: "isolate-" + id, Label: "Isolate " + id, Clusters: [][]string{{id}, a.otherIDs(id)}, Active: true})
}
func (a *App) HealAll() error                    { return healAllFirewallRules() }
func (a *App) OpenRuntimeFolder(id string) error { return a.openInstancePath(id, "runtime") }
func (a *App) OpenInstanceLog(id string) error {
	a.mu.Lock()
	profile, ok := a.profileLocked(id)
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	logPath := filepath.Join(profile.RuntimeDir, "stdout.log")
	if !fileExists(logPath) {
		return fmt.Errorf("no logs generated yet. please start the node first.")
	}
	return openPath(logPath)
}

func (a *App) ReadInstanceLogTail(id string, limit int) (string, error) {
	a.mu.Lock()
	profile, ok := a.profileLocked(id)
	a.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("unknown instance %s", id)
	}
	logPath := filepath.Join(profile.RuntimeDir, "stdout.log")
	if !fileExists(logPath) {
		return "No logs generated yet. Please start the node first.", nil
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) ApplyPartition(spec PartitionSpec) error {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	return applyFirewallPartition(ws.Instances, spec)
}

func (a *App) RunScenario(id string) error {
	scenarios := builtInScenarios()
	var selected *ScenarioSpec
	for i := range scenarios {
		if scenarios[i].ID == id {
			selected = &scenarios[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("unknown scenario %s", id)
	}
	go a.runScenario(*selected)
	return nil
}

func (a *App) GetScenarioState() ScenarioRunState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.scenario
}

func (a *App) openInstancePath(id string, kind string) error {
	a.mu.Lock()
	profile, ok := a.profileLocked(id)
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	target := profile.RuntimeDir
	if kind == "db" {
		target = filepath.Dir(profile.DBPath)
	}
	return openPath(target)
}

func (a *App) controlAction(id string, action string) error {
	a.mu.Lock()
	profile, ok := a.profileLocked(id)
	token := a.workspace.Token
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/v1/actions/%s", profile.ControlPort, action), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Demo-Token", token)
	client := http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", action, resp.Status)
	}
	return nil
}

func (a *App) refreshStatuses(ws DemoWorkspace) []InstanceStatus {
	a.mu.Lock()
	procs := make(map[string]*exec.Cmd, len(a.procs))
	for k, v := range a.procs {
		procs[k] = v
	}
	errs := make(map[string]string, len(a.errors))
	for k, v := range a.errors {
		errs[k] = v
	}
	a.mu.Unlock()

	out := make([]InstanceStatus, 0, len(ws.Instances))
	for _, p := range ws.Instances {
		st := InstanceStatus{Profile: p, LastError: errs[p.ID]}
		if cmd := procs[p.ID]; cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
			st.Running = true
			st.PID = cmd.Process.Pid
		}
		a.fillRemoteStatus(&st, ws.Token)
		out = append(out, st)
	}
	return out
}

func (a *App) fillRemoteStatus(st *InstanceStatus, token string) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/v1/status", st.Profile.ControlPort), nil)
	if err != nil {
		return
	}
	req.Header.Set("X-Demo-Token", token)
	client := http.Client{Timeout: 900 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return
	}
	st.LastSeenMs = time.Now().UnixMilli()
	if appState, _ := raw["app_state"].(string); appState != "" {
		st.AppState = appState
	}
	if health, _ := raw["health"].(map[string]interface{}); health != nil {
		st.StartupStage, _ = health["startup_stage"].(string)
		st.P2PReady, _ = health["p2p_running"].(bool)
		st.CryptoReady, _ = health["crypto_ready"].(bool)
	}
	if network, _ := raw["network"].(map[string]interface{}); network != nil {
		st.PeerID, _ = network["local_peer_id"].(string)
		st.PeerCount = intFromAny(network["connected_peers"])
	}
	if diag, _ := raw["diagnostics"].(map[string]interface{}); diag != nil {
		if groups, ok := diag["groups"].([]interface{}); ok {
			st.GroupCount = len(groups)
			for _, item := range groups {
				g, _ := item.(map[string]interface{})
				if g == nil {
					continue
				}
				st.Groups = append(st.Groups, InstanceGroup{
					GroupID:         stringFromAny(g["group_id"]),
					Epoch:           uint64(intFromAny(g["epoch"])),
					TokenHolder:     stringFromAny(g["token_holder"]),
					TokenHolderPeer: stringFromAny(g["token_holder_peer_id"]),
					ActiveMembers:   intFromAny(g["active_members"]),
					TreeHashShort:   stringFromAny(g["tree_hash_short"]),
					IsHealing:       boolFromAny(g["is_healing"]),
				})
			}
		}
	}
}

func (a *App) profileLocked(id string) (DemoInstanceProfile, bool) {
	for _, p := range a.workspace.Instances {
		if p.ID == id {
			return p, true
		}
	}
	return DemoInstanceProfile{}, false
}

func (a *App) otherIDs(id string) []string {
	out := make([]string, 0, len(a.workspace.Instances)-1)
	for _, p := range a.workspace.Instances {
		if p.ID != id {
			out = append(out, p.ID)
		}
	}
	return out
}

func loadOrCreateWorkspace(repoRoot string) (DemoWorkspace, error) {
	controlRoot := filepath.Join(repoRoot, ".demo-control")
	if err := os.MkdirAll(controlRoot, 0o700); err != nil {
		return DemoWorkspace{}, err
	}
	path := filepath.Join(controlRoot, "workspace.json")
	if fileExists(path) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return DemoWorkspace{}, err
		}
		var ws DemoWorkspace
		if err := json.Unmarshal(raw, &ws); err != nil {
			return DemoWorkspace{}, err
		}
		normalizeWorkspace(&ws, repoRoot)
		return ws, nil
	}
	ws := defaultWorkspace(repoRoot)
	raw, _ := json.MarshalIndent(ws, "", "  ")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return DemoWorkspace{}, err
	}
	return ws, nil
}

func defaultWorkspace(repoRoot string) DemoWorkspace {
	token := randomToken()
	ws := DemoWorkspace{
		Name:     "default",
		RepoRoot: repoRoot,
		AppDir:   filepath.Join(repoRoot, "app"),
		AppExe:   filepath.Join(repoRoot, "app", "build", "bin", "SecureP2P.exe"),
		Token:    token,
	}
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("node-%02d", i)
		runtimeDir := filepath.Join(repoRoot, ".demo-control", "runtimes", id)
		ws.Instances = append(ws.Instances, DemoInstanceProfile{
			ID:          id,
			Label:       fmt.Sprintf("Node %02d", i),
			LaunchMode:  "exe",
			RuntimeDir:  runtimeDir,
			TemplateDir: filepath.Join(repoRoot, ".demo-control", "templates", id),
			DBPath:      filepath.Join(runtimeDir, "app.db"),
			P2PPort:     4100 + i,
			ControlPort: 5100 + i,
			Headless:    false,
		})
	}
	return ws
}

func normalizeWorkspace(ws *DemoWorkspace, repoRoot string) {
	if ws.RepoRoot == "" {
		ws.RepoRoot = repoRoot
	}
	if ws.AppDir == "" {
		ws.AppDir = filepath.Join(ws.RepoRoot, "app")
	}
	if ws.AppExe == "" {
		ws.AppExe = filepath.Join(ws.AppDir, "build", "bin", "SecureP2P.exe")
	}
	if ws.Token == "" {
		ws.Token = randomToken()
	}
	for i := range ws.Instances {
		if ws.Instances[i].DBPath == "" {
			ws.Instances[i].DBPath = filepath.Join(ws.Instances[i].RuntimeDir, "app.db")
		}
	}
}

func builtInScenarios() []ScenarioSpec {
	return []ScenarioSpec{
		{ID: "bring-up", Name: "Normal Cluster Bring-Up", Description: "Start all configured nodes and wait for runtime readiness.", Steps: []ScenarioStep{{Action: "heal"}, {Action: "start-all"}, {Action: "wait", Milliseconds: 5000}}},
		{ID: "offline-node", Name: "Offline Messaging Recovery Prep", Description: "Isolate node-03, then heal and trigger sync.", Steps: []ScenarioStep{{Action: "isolate", InstanceID: "node-03"}, {Action: "wait", Milliseconds: 5000}, {Action: "heal"}, {Action: "sync", InstanceID: "node-03"}}},
		{ID: "fork-heal", Name: "Fork Healing Partition", Description: "Split 1-3 from 4-6, then heal.", Steps: []ScenarioStep{{Action: "partition", Partition: [][]string{{"node-01", "node-02", "node-03"}, {"node-04", "node-05", "node-06"}}}, {Action: "wait", Milliseconds: 8000}, {Action: "heal"}}},
		{ID: "reset-clean", Name: "Reset To Known Good State", Description: "Stop all, heal firewall, reset runtimes, start all.", Steps: []ScenarioStep{{Action: "stop-all"}, {Action: "heal"}, {Action: "reset-all"}, {Action: "start-all"}}},
	}
}

func (a *App) runScenario(spec ScenarioSpec) {
	a.mu.Lock()
	a.scenario = ScenarioRunState{Running: true, ScenarioID: spec.ID, StartedAtMs: time.Now().UnixMilli()}
	a.mu.Unlock()
	for i, step := range spec.Steps {
		a.mu.Lock()
		a.scenario.StepIndex = i
		a.scenario.CurrentStep = step.Action
		a.mu.Unlock()
		var err error
		switch step.Action {
		case "start-all":
			err = a.StartAll()
		case "stop-all":
			err = a.StopAll()
		case "reset-all":
			err = a.ResetAll()
		case "heal":
			err = a.HealAll()
		case "isolate":
			err = a.IsolateNode(step.InstanceID)
		case "sync":
			err = a.TriggerOfflineSync(step.InstanceID)
		case "partition":
			err = a.ApplyPartition(PartitionSpec{ID: spec.ID, Label: spec.Name, Clusters: step.Partition, Active: true})
		case "wait":
			time.Sleep(time.Duration(step.Milliseconds) * time.Millisecond)
		}
		if err != nil {
			a.mu.Lock()
			a.scenario.Running = false
			a.scenario.LastError = err.Error()
			a.scenario.EndedAtMs = time.Now().UnixMilli()
			a.mu.Unlock()
			return
		}
	}
	a.mu.Lock()
	a.scenario.Running = false
	a.scenario.EndedAtMs = time.Now().UnixMilli()
	a.mu.Unlock()
}

func randomToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func intFromAny(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func stringFromAny(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolFromAny(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func openPath(path string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func portBusy(port int) bool {
	client := http.Client{Timeout: 120 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err == nil {
		_ = resp.Body.Close()
		return true
	}
	return false
}

func ensureDemoPath(root string, target string) error {
	absRoot, _ := filepath.Abs(filepath.Join(root, ".demo-control"))
	absTarget, _ := filepath.Abs(target)
	if !strings.HasPrefix(strings.ToLower(absTarget), strings.ToLower(absRoot)) {
		return fmt.Errorf("refusing to reset path outside .demo-control: %s", target)
	}
	return nil
}

func findRepoRoot() (string, error) {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}
	for _, start := range candidates {
		dir, _ := filepath.Abs(start)
		for {
			if fileExists(filepath.Join(dir, "PROJECT_PLAN.md")) && fileExists(filepath.Join(dir, "app", "wails.json")) {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("repo root not found; start the controller from inside the DATN repository")
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o600)
	})
}

func sortStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func (a *App) Notify(message string) {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "demo:notice", message)
	}
}
