package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

const (
	guiBuildJobID      = "build-gui-exe"
	headlessBuildJobID = "build-headless-image"
	defaultDemoGroupID = "demo"
)

type App struct {
	ctx               context.Context
	mu                sync.Mutex
	root              string
	workspace         DemoWorkspace
	procs             map[string]*exec.Cmd
	errors            map[string]string
	warnings          map[string]string
	workspaceWarning  []string
	demoTargetNodeIDs []string
	scenario          ScenarioRunState
	jobs              *jobRunner
}

func NewApp() *App {
	return &App{
		procs:    make(map[string]*exec.Cmd),
		errors:   make(map[string]string),
		warnings: make(map[string]string),
		jobs:     newJobRunner(),
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
	ws, warnings, err := loadOrCreateWorkspace(repoRoot)
	if err != nil {
		a.errors["workspace"] = err.Error()
		return
	}
	a.workspace = ws
	a.workspaceWarning = warnings
}

func (a *App) Shutdown(_ context.Context) {
	_ = a.StopAll()
}

func (a *App) GetSnapshot() (ControlSnapshot, error) {
	a.mu.Lock()
	ws := a.workspace
	targetNodeIDs := append([]string(nil), a.demoTargetNodeIDs...)
	a.mu.Unlock()

	guiStatuses := a.refreshStatusesForProfiles(ws.GuiLane.Instances, ws.Token)
	headlessStatuses := a.refreshStatusesForProfiles(ws.HeadlessLane.Instances, ws.Token)

	return ControlSnapshot{
		Workspace: ws,
		GuiLane: GuiLaneSnapshot{
			Instances:  guiStatuses,
			Preflight:  a.preflightGUI(ws),
			BuildJobID: guiBuildJobID,
		},
		HeadlessLane: HeadlessLaneSnapshot{
			Instances:   headlessStatuses,
			Topology:    a.getNetworkTopologyForProfiles(ws.HeadlessLane.Instances),
			DemoCluster: a.buildDemoClusterState(ws, headlessStatuses, targetNodeIDs),
			Preflight:   a.preflightHeadless(),
			Warnings:    a.workspaceWarnings(),
			BuildJobID:  headlessBuildJobID,
		},
		Jobs:      a.jobs.snapshot(),
		Scenarios: builtInScenarios(),
	}, nil
}

func (a *App) PreflightGUI() (PreflightResult, error) {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	return a.preflightGUI(ws), nil
}

func (a *App) PreflightHeadless() (PreflightResult, error) {
	return a.preflightHeadless(), nil
}

func (a *App) BuildGuiDemo() error {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	cryptoDir := filepath.Join(ws.RepoRoot, "crypto-engine")
	appExe := ws.AppExe
	if appExe == "" {
		appExe = filepath.Join(ws.AppDir, "build", "bin", "SecureP2P.exe")
	}
	appExeDir := filepath.Dir(appExe)
	cryptoExe := filepath.Join(cryptoDir, "target", "release", "crypto-engine.exe")
	command := fmt.Sprintf(
		`taskkill /F /IM crypto-engine.exe 2>nul & taskkill /F /IM SecureP2P.exe 2>nul & cd /d "%s" && cargo build --release && cd /d "%s" && wails build && if not exist "%s" mkdir "%s" && copy /Y "%s" "%s\crypto-engine.exe"`,
		cryptoDir,
		ws.AppDir,
		appExeDir,
		appExeDir,
		cryptoExe,
		appExeDir,
	)
	a.Notify("Opening build terminal for GUI EXE...")
	return openBuildTerminal(a.root, "GUI Demo Build", command)
}

func (a *App) BuildHeadlessImage() error {
	a.mu.Lock()
	repoRoot := a.workspace.RepoRoot
	a.mu.Unlock()
	command := fmt.Sprintf(`cd /d "%s" && docker build -t secure-p2p:latest .`, repoRoot)
	a.Notify("Opening build terminal for headless Docker image...")
	return openBuildTerminal(repoRoot, "Headless Docker Build", command)
}

func (a *App) StartInstance(id string) error {
	a.mu.Lock()
	profile, _, ok := a.profileLocked(id)
	ws := a.workspace
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	if a.isInstanceRunning(id) {
		return nil
	}

	seedWarning, err := ensureRuntimeSeeded(profile)
	if err != nil {
		a.setInstanceError(profile.ID, err.Error())
		return err
	}
	if seedWarning != "" {
		a.setInstanceWarning(profile.ID, seedWarning)
	} else {
		a.setInstanceWarning(profile.ID, "")
	}

	if err := os.MkdirAll(profile.RuntimeDir, 0o700); err != nil {
		a.setInstanceError(profile.ID, err.Error())
		return err
	}

	if profile.LaunchMode == "exe" {
		return a.startExeInstance(ws, profile)
	}
	return a.startDockerInstance(ws, profile)
}

func (a *App) startExeInstance(ws DemoWorkspace, profile DemoInstanceProfile) error {
	appExe := ws.AppExe
	if appExe == "" {
		appExe = filepath.Join(ws.AppDir, "build", "bin", "SecureP2P.exe")
	}
	args := []string{
		"-db", profile.DBPath,
		"-runtime-dir", profile.RuntimeDir,
		"-p2p-port", fmt.Sprint(profile.P2PPort),
		"-control-port", fmt.Sprint(profile.ControlPort),
		"-control-token", ws.Token,
		"-instance-label", profile.Label,
	}
	if profile.StoreNode {
		args = append(args, "-store-node")
	}
	if profile.Bootstrap != "" {
		args = append(args, "-bootstrap", profile.Bootstrap)
	}
	if profile.Headless {
		args = append(args, "-headless")
	}

	cmd := exec.Command(appExe, args...)
	cmd.Dir = exeWorkingDir(appExe, ws.AppDir)

	logPath := filepath.Join(profile.RuntimeDir, "stdout.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		a.setInstanceError(profile.ID, "log file: "+err.Error())
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		a.setInstanceError(profile.ID, err.Error())
		return err
	}

	a.mu.Lock()
	a.procs[profile.ID] = cmd
	a.errors[profile.ID] = ""
	a.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
	}()
	return nil
}

func exeWorkingDir(appExe string, fallback string) string {
	if dir := strings.TrimSpace(filepath.Dir(appExe)); dir != "" && dir != "." {
		return dir
	}
	return fallback
}

func (a *App) startDockerInstance(ws DemoWorkspace, profile DemoInstanceProfile) error {
	if err := a.ensureSharedNetwork(); err != nil {
		a.setInstanceError(profile.ID, err.Error())
		return err
	}
	_ = exec.Command("docker", "rm", "-f", profile.ID).Run()

	dockerArgs := []string{
		"run", "--rm",
		"--name", profile.ID,
		"--network", sharedDockerNetwork,
		"--ip", profile.BindIP,
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", profile.ControlPort, profile.ControlPort),
		"-v", fmt.Sprintf("%s:/data", profile.RuntimeDir),
		"secure-p2p:latest",
		"-db", "/data/app.db",
		"-runtime-dir", "/data",
		"-p2p-port", fmt.Sprint(profile.P2PPort),
		"-bind-ip", "0.0.0.0",
		"-control-port", fmt.Sprint(profile.ControlPort),
		"-control-token", ws.Token,
		"-instance-label", profile.Label,
		"-headless",
	}
	if profile.StoreNode {
		dockerArgs = append(dockerArgs, "-store-node")
	}
	if profile.Bootstrap != "" {
		dockerArgs = append(dockerArgs, "-bootstrap", profile.Bootstrap)
	}

	cmd := exec.Command("docker", dockerArgs...)
	logPath := filepath.Join(profile.RuntimeDir, "stdout.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		a.setInstanceError(profile.ID, "log file: "+err.Error())
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		a.setInstanceError(profile.ID, err.Error())
		return err
	}

	a.mu.Lock()
	a.procs[profile.ID] = cmd
	a.errors[profile.ID] = ""
	a.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
	}()
	return nil
}

func (a *App) StopInstance(id string) error {
	_ = a.controlAction(id, "shutdown")
	for i := 0; i < 15; i++ {
		if !a.isInstanceRunning(id) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	a.mu.Lock()
	cmd := a.procs[id]
	delete(a.procs, id)
	a.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = exec.Command("docker", "stop", id).Run()
	_ = exec.Command("docker", "rm", "-f", id).Run()
	a.cleanupUnusedPartitionNetworks()
	return nil
}

func (a *App) KillInstance(id string) error {
	a.mu.Lock()
	cmd := a.procs[id]
	delete(a.procs, id)
	a.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = exec.Command("docker", "rm", "-f", id).Run()
	a.cleanupUnusedPartitionNetworks()
	return nil
}

func (a *App) RestartInstance(id string) error {
	if err := a.StopInstance(id); err != nil {
		return err
	}
	time.Sleep(900 * time.Millisecond)
	return a.StartInstance(id)
}

func (a *App) StartGuiLane() error {
	return a.startLane("gui")
}

func (a *App) StartHeadlessLane() error {
	return a.startLane("headless")
}

func (a *App) StopGuiLane() error {
	return a.stopLane("gui")
}

func (a *App) StopHeadlessLane() error {
	return a.stopLane("headless")
}

func (a *App) ResetGuiLane() error {
	return a.resetLane("gui")
}

func (a *App) ResetHeadlessLane() error {
	return a.resetLane("headless")
}

func (a *App) StartAll() error {
	if err := a.StartGuiLane(); err != nil {
		return err
	}
	return a.StartHeadlessLane()
}

func (a *App) StopAll() error {
	if err := a.StopGuiLane(); err != nil {
		return err
	}
	return a.StopHeadlessLane()
}

func (a *App) ResetAll() error {
	if err := a.ResetGuiLane(); err != nil {
		return err
	}
	return a.ResetHeadlessLane()
}

func (a *App) startLane(laneID string) error {
	profiles := a.laneProfiles(laneID)
	for _, profile := range profiles {
		if err := a.StartInstance(profile.ID); err != nil {
			return err
		}
		time.Sleep(120 * time.Millisecond)
	}
	return nil
}

func (a *App) stopLane(laneID string) error {
	profiles := a.laneProfiles(laneID)
	for _, profile := range profiles {
		_ = a.controlAction(profile.ID, "shutdown")
	}
	time.Sleep(800 * time.Millisecond)
	for _, profile := range profiles {
		if err := a.StopInstance(profile.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) resetLane(laneID string) error {
	profiles := a.laneProfiles(laneID)
	for _, profile := range profiles {
		if err := a.ResetInstance(profile.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) ResetInstance(id string) error {
	a.mu.Lock()
	profile, _, ok := a.profileLocked(id)
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	if a.isInstanceRunning(id) {
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

func (a *App) CaptureRuntimeAsTemplate(id string) error {
	a.mu.Lock()
	profile, _, ok := a.profileLocked(id)
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	if a.isInstanceRunning(id) {
		return fmt.Errorf("stop %s before capturing template", id)
	}
	if err := ensureDemoPath(a.root, profile.RuntimeDir); err != nil {
		return err
	}
	if err := ensureDemoPath(a.root, profile.TemplateDir); err != nil {
		return err
	}
	if err := os.RemoveAll(profile.TemplateDir); err != nil {
		return err
	}
	if !fileExists(profile.RuntimeDir) {
		return fmt.Errorf("runtime dir %s does not exist", profile.RuntimeDir)
	}
	return copyDir(profile.RuntimeDir, profile.TemplateDir)
}

func (a *App) TriggerOfflineSync(id string) error { return a.controlAction(id, "trigger-offline-sync") }
func (a *App) ExportDiagnostics(id string) error  { return a.controlAction(id, "export-diagnostics") }

func (a *App) SendDemoMessage(nodeIDs []string, message string) error {
	a.mu.Lock()
	groupID := strings.TrimSpace(a.workspace.HeadlessLane.DemoGroupID)
	a.mu.Unlock()
	if groupID == "" {
		groupID = defaultDemoGroupID
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Errorf("message is required")
	}
	if len(nodeIDs) == 0 {
		return fmt.Errorf("select at least one node to send from")
	}
	for _, nodeID := range nodeIDs {
		if err := a.controlDemoSendMessage(nodeID, groupID, message); err != nil {
			return fmt.Errorf("%s: %w", nodeID, err)
		}
	}
	return nil
}

func (a *App) PrepareDemoCluster(ownerID string, nodeIDs []string) error {
	a.mu.Lock()
	ws := a.workspace
	a.demoTargetNodeIDs = append([]string(nil), nodeIDs...)
	a.mu.Unlock()

	groupID := strings.TrimSpace(ws.HeadlessLane.DemoGroupID)
	if groupID == "" {
		groupID = defaultDemoGroupID
	}
	statuses := a.refreshStatusesForProfiles(ws.HeadlessLane.Instances, ws.Token)
	target := resolveDemoClusterTargetSet(ws, statuses, nodeIDs)
	if target.LastError != "" {
		return errors.New(target.LastError)
	}
	ownerID, err := chooseDemoOwner(ws, target, strings.TrimSpace(ownerID))
	if err != nil {
		return err
	}

	groups, err := a.controlDemoGroups(ownerID)
	if err != nil {
		return err
	}
	groupExists := false
	for _, group := range groups {
		if strings.TrimSpace(stringFromAny(group["group_id"])) == groupID {
			groupExists = true
			break
		}
	}
	if !groupExists {
		if err := a.controlDemoCreateGroup(ownerID, groupID, "group"); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") {
			return err
		}
	}

	members, err := a.controlDemoGroupMembers(ownerID, groupID)
	if err != nil {
		members = nil
	}
	memberSet := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberSet[member.PeerID] = struct{}{}
	}
	for nodeID, peerID := range target.ExpectedPeers {
		if nodeID == ownerID {
			continue
		}
		if _, ok := memberSet[peerID]; ok {
			continue
		}
		if err := a.controlDemoInvitePeer(ownerID, groupID, peerID); err != nil {
			return fmt.Errorf("invite %s failed: %w", nodeID, err)
		}
	}

	if err := a.waitForDemoClusterReady(ownerID, groupID, target.ExpectedPeers, 45*time.Second); err != nil {
		return err
	}
	return nil
}

func (a *App) waitForDemoClusterReady(ownerID string, groupID string, expectedPeers map[string]string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		members, err := a.controlDemoGroupMembers(ownerID, groupID)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		memberSet := make(map[string]struct{}, len(members))
		for _, member := range members {
			memberSet[member.PeerID] = struct{}{}
		}

		allReady := true
		for _, peerID := range expectedPeers {
			if _, ok := memberSet[peerID]; !ok {
				allReady = false
				break
			}
		}
		if allReady {
			for nodeID := range expectedPeers {
				groups, err := a.controlDemoGroups(nodeID)
				if err != nil || !remoteGroupsContain(groups, groupID) {
					allReady = false
					break
				}
			}
		}
		if allReady {
			return nil
		}
		time.Sleep(1200 * time.Millisecond)
	}
	return fmt.Errorf("demo cluster did not become ready within %s", timeout)
}

func remoteGroupsContain(groups []map[string]interface{}, groupID string) bool {
	for _, group := range groups {
		if strings.TrimSpace(stringFromAny(group["group_id"])) == strings.TrimSpace(groupID) {
			return true
		}
	}
	return false
}

type demoClusterTargetSet struct {
	RequestedNodeIDs []string
	TargetStatuses   []InstanceStatus
	TargetNodeIDs    []string
	ExpectedPeers    map[string]string
	EligibleCount    int
	BlockingNodes    []string
	LastError        string
}

func resolveDemoClusterTargetSet(ws DemoWorkspace, statuses []InstanceStatus, requestedNodeIDs []string) demoClusterTargetSet {
	index := make(map[string]InstanceStatus, len(statuses))
	for _, status := range statuses {
		index[status.Profile.ID] = status
	}

	out := demoClusterTargetSet{
		RequestedNodeIDs: append([]string(nil), requestedNodeIDs...),
		ExpectedPeers:    make(map[string]string),
	}

	requested := make([]string, 0, len(requestedNodeIDs))
	seen := make(map[string]struct{}, len(requestedNodeIDs))
	for _, id := range requestedNodeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		requested = append(requested, id)
	}

	if len(requested) > 0 {
		for _, id := range requested {
			status, ok := index[id]
			if !ok {
				out.BlockingNodes = append(out.BlockingNodes, fmt.Sprintf("%s unknown", id))
				continue
			}
			out.TargetStatuses = append(out.TargetStatuses, status)
		}
	} else {
		for _, status := range statuses {
			if status.Running {
				out.TargetStatuses = append(out.TargetStatuses, status)
			}
		}
	}

	for _, status := range out.TargetStatuses {
		out.TargetNodeIDs = append(out.TargetNodeIDs, status.Profile.ID)
		if !status.Running {
			out.BlockingNodes = append(out.BlockingNodes, fmt.Sprintf("%s offline", status.Profile.ID))
			continue
		}
		if !isEligibleDemoState(status.AppState) {
			out.BlockingNodes = append(out.BlockingNodes, fmt.Sprintf("%s unauthorized (%s)", status.Profile.ID, status.AppState))
			continue
		}
		peerID := strings.TrimSpace(status.PeerID)
		if peerID == "" {
			out.BlockingNodes = append(out.BlockingNodes, fmt.Sprintf("%s missing peer id", status.Profile.ID))
			continue
		}
		out.ExpectedPeers[status.Profile.ID] = peerID
		out.EligibleCount++
	}

	switch {
	case len(out.TargetStatuses) == 0 && len(requested) > 0:
		out.LastError = "no selected headless nodes could be resolved"
	case len(out.TargetStatuses) == 0:
		out.LastError = "no running headless nodes selected"
	case len(out.BlockingNodes) > 0:
		out.LastError = "cannot prepare demo cluster: " + strings.Join(out.BlockingNodes, "; ")
	}

	return out
}

func chooseDemoOwner(ws DemoWorkspace, target demoClusterTargetSet, explicitOwner string) (string, error) {
	explicitOwner = strings.TrimSpace(explicitOwner)
	if explicitOwner != "" {
		if _, ok := target.ExpectedPeers[explicitOwner]; !ok {
			return "", fmt.Errorf("owner %s is not eligible for demo cluster target set", explicitOwner)
		}
		return explicitOwner, nil
	}

	workspaceOwner := strings.TrimSpace(ws.HeadlessLane.DemoOwnerNode)
	if workspaceOwner != "" {
		if _, ok := target.ExpectedPeers[workspaceOwner]; ok {
			return workspaceOwner, nil
		}
	}

	for _, status := range target.TargetStatuses {
		if _, ok := target.ExpectedPeers[status.Profile.ID]; ok {
			return status.Profile.ID, nil
		}
	}

	return "", fmt.Errorf("no eligible headless owner node configured")
}

func (a *App) IsolateNode(id string) error {
	a.mu.Lock()
	profile, _, ok := a.profileLocked(id)
	headless := a.workspace.HeadlessLane.Instances
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	if profile.LaunchMode != "docker" {
		return fmt.Errorf("network isolation is only supported for docker-managed instances")
	}
	if !isContainerRunning(id) {
		return fmt.Errorf("instance %s is not running", id)
	}
	if err := a.ensurePartitionNetwork(partitionNetworkName(0), 0); err != nil {
		return err
	}
	if err := a.moveContainerToPartitionNetwork(profile, partitionNetworkName(0), partitionIPAddress(profile, 0)); err != nil {
		return err
	}
	for _, other := range headless {
		if other.ID == id || other.LaunchMode != "docker" || !isContainerRunning(other.ID) {
			continue
		}
		if err := a.healContainerToSharedNetwork(other); err != nil {
			return err
		}
	}
	a.cleanupUnusedPartitionNetworks()
	return nil
}

func (a *App) HealAll() error {
	headless := a.laneProfiles("headless")
	for _, profile := range headless {
		if err := a.healContainerToSharedNetwork(profile); err != nil {
			return err
		}
	}
	a.cleanupUnusedPartitionNetworks()
	return nil
}

func (a *App) ApplyPartition(spec PartitionSpec) error {
	headless := a.laneProfiles("headless")
	seen := make(map[string]struct{})
	nonEmptyClusters := 0
	for i, cluster := range spec.Clusters {
		if len(cluster) == 0 {
			continue
		}
		nonEmptyClusters++
		networkName := partitionNetworkName(i)
		if err := a.ensurePartitionNetwork(networkName, i); err != nil {
			return err
		}
		for _, nodeID := range cluster {
			profile, ok := findProfileInSlice(headless, nodeID)
			if !ok {
				return fmt.Errorf("unknown headless instance %s", nodeID)
			}
			if profile.LaunchMode != "docker" {
				return fmt.Errorf("network partition is only supported for docker-managed instances (%s)", nodeID)
			}
			if !isContainerRunning(nodeID) {
				continue
			}
			if err := a.moveContainerToPartitionNetwork(profile, networkName, partitionIPAddress(profile, i)); err != nil {
				return err
			}
			seen[nodeID] = struct{}{}
		}
	}
	if nonEmptyClusters < 2 {
		return fmt.Errorf("partition requires at least two non-empty clusters")
	}
	for _, profile := range headless {
		if profile.LaunchMode != "docker" || !isContainerRunning(profile.ID) {
			continue
		}
		if _, ok := seen[profile.ID]; ok {
			continue
		}
		if err := a.healContainerToSharedNetwork(profile); err != nil {
			return err
		}
	}
	a.cleanupUnusedPartitionNetworks()
	return nil
}

func (a *App) OpenRuntimeFolder(id string) error { return a.openInstancePath(id, "runtime") }

func (a *App) OpenInstanceLog(id string) error {
	a.mu.Lock()
	profile, _, ok := a.profileLocked(id)
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown instance %s", id)
	}
	logPath := filepath.Join(profile.RuntimeDir, "stdout.log")
	if !fileExists(logPath) {
		return fmt.Errorf("no logs generated yet. please start the node first")
	}
	return openPath(logPath)
}

func (a *App) ReadInstanceLogTail(id string, limit int) (string, error) {
	a.mu.Lock()
	profile, _, ok := a.profileLocked(id)
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

func (a *App) runScenario(spec ScenarioSpec) {
	a.mu.Lock()
	a.scenario = ScenarioRunState{
		Running:     true,
		ScenarioID:  spec.ID,
		StartedAtMs: time.Now().UnixMilli(),
	}
	a.mu.Unlock()

	for i, step := range spec.Steps {
		a.mu.Lock()
		a.scenario.StepIndex = i
		a.scenario.CurrentStep = step.Action
		a.mu.Unlock()

		var err error
		switch step.Action {
		case "start-headless":
			err = a.StartHeadlessLane()
		case "stop-headless":
			err = a.StopHeadlessLane()
		case "reset-headless":
			err = a.ResetHeadlessLane()
		case "prepare-demo":
			err = a.PrepareDemoCluster(step.InstanceID, nil)
		case "heal":
			err = a.HealAll()
		case "isolate":
			err = a.IsolateNode(step.InstanceID)
		case "sync":
			err = a.TriggerOfflineSync(step.InstanceID)
		case "partition":
			err = a.ApplyPartition(PartitionSpec{ID: spec.ID, Label: spec.Name, Clusters: step.Partition, Active: true})
		case "send-demo":
			err = a.SendDemoMessage([]string{step.InstanceID}, step.Message)
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

func builtInScenarios() []ScenarioSpec {
	return []ScenarioSpec{
		{
			ID:          "headless-bring-up",
			Name:        "Headless Bring-Up",
			Description: "Start headless lane and prepare the shared Demo group.",
			Steps: []ScenarioStep{
				{Action: "heal"},
				{Action: "start-headless"},
				{Action: "wait", Milliseconds: 5000},
				{Action: "prepare-demo"},
			},
		},
		{
			ID:          "offline-node",
			Name:        "Offline Recovery Demo",
			Description: "Prepare Demo group, isolate node-03, heal the network, then trigger offline sync.",
			Steps: []ScenarioStep{
				{Action: "prepare-demo"},
				{Action: "send-demo", InstanceID: "node-01", Message: "Pre-isolation checkpoint"},
				{Action: "isolate", InstanceID: "node-03"},
				{Action: "wait", Milliseconds: 5000},
				{Action: "heal"},
				{Action: "sync", InstanceID: "node-03"},
			},
		},
		{
			ID:          "fork-heal",
			Name:        "Fork Healing Partition",
			Description: "Split 1-3 from 4-6, send messages on both sides, then heal.",
			Steps: []ScenarioStep{
				{Action: "prepare-demo"},
				{Action: "send-demo", InstanceID: "node-01", Message: "Cluster ready before split"},
				{Action: "partition", Partition: [][]string{{"node-01", "node-02", "node-03"}, {"node-04", "node-05", "node-06"}}},
				{Action: "wait", Milliseconds: 3000},
				{Action: "send-demo", InstanceID: "node-01", Message: "Cluster A says hello during split"},
				{Action: "send-demo", InstanceID: "node-04", Message: "Cluster B says hello during split"},
				{Action: "wait", Milliseconds: 3000},
				{Action: "heal"},
				{Action: "wait", Milliseconds: 3000},
				{Action: "sync", InstanceID: "node-01"},
			},
		},
	}
}

func (a *App) openInstancePath(id string, kind string) error {
	a.mu.Lock()
	profile, _, ok := a.profileLocked(id)
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

func (a *App) refreshStatusesForProfiles(profiles []DemoInstanceProfile, token string) []InstanceStatus {
	a.mu.Lock()
	errs := make(map[string]string, len(a.errors))
	for k, v := range a.errors {
		errs[k] = v
	}
	warnings := make(map[string]string, len(a.warnings))
	for k, v := range a.warnings {
		warnings[k] = v
	}
	a.mu.Unlock()

	runningContainers, _ := getRunningContainers()
	out := make([]InstanceStatus, 0, len(profiles))
	for _, p := range profiles {
		st := InstanceStatus{Profile: p, LastError: errs[p.ID], LastWarning: warnings[p.ID]}
		if p.LaunchMode == "exe" {
			a.mu.Lock()
			cmd := a.procs[p.ID]
			a.mu.Unlock()
			if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
				st.Running = true
				st.PID = cmd.Process.Pid
			}
		} else if runningContainers != nil && runningContainers[p.ID] {
			st.Running = true
			st.PID = getContainerPID(p.ID)
		}
		a.fillRemoteStatus(&st, token)
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
					ActiveView:      stringSliceFromAny(g["active_view"]),
					TreeHashShort:   stringFromAny(g["tree_hash_short"]),
					IsHealing:       boolFromAny(g["is_healing"]),
				})
			}
		}
	}
}

func (a *App) buildDemoClusterState(ws DemoWorkspace, statuses []InstanceStatus, requestedNodeIDs []string) DemoClusterState {
	groupID := strings.TrimSpace(ws.HeadlessLane.DemoGroupID)
	if groupID == "" {
		groupID = defaultDemoGroupID
	}

	target := resolveDemoClusterTargetSet(ws, statuses, requestedNodeIDs)
	state := DemoClusterState{
		GroupID:       groupID,
		TargetNodeIDs: append([]string(nil), target.TargetNodeIDs...),
		TargetCount:   len(target.TargetNodeIDs),
		EligibleCount: target.EligibleCount,
		BlockingNodes: append([]string(nil), target.BlockingNodes...),
	}
	for _, peerID := range target.ExpectedPeers {
		state.TargetPeerIDs = append(state.TargetPeerIDs, peerID)
	}
	sort.Strings(state.TargetPeerIDs)
	state.Eligible = len(target.TargetNodeIDs) > 0 && len(target.BlockingNodes) == 0
	if target.LastError != "" {
		state.LastError = target.LastError
	}

	ownerID, err := chooseDemoOwner(ws, target, "")
	if err != nil {
		if state.LastError == "" {
			state.LastError = err.Error()
		}
		return state
	}
	state.OwnerNodeID = ownerID

	groups, err := a.controlDemoGroups(ownerID)
	if err != nil {
		if state.LastError == "" {
			state.LastError = err.Error()
		}
		return state
	}
	if !remoteGroupsContain(groups, groupID) {
		if state.LastError == "" {
			state.LastError = "Demo group not prepared yet"
		}
		return state
	}

	members, err := a.controlDemoGroupMembers(ownerID, groupID)
	if err == nil {
		state.Members = members
		state.MemberCount = len(members)
	}
	groupStatus, err := a.controlDemoGroupStatus(ownerID, groupID)
	if err == nil {
		state.GroupStatusDigest = groupStatus
	}
	messages, err := a.controlDemoGroupMessages(ownerID, groupID, 16)
	if err == nil {
		state.RecentMessages = messages
	}

	memberSet := make(map[string]struct{}, len(state.Members))
	for _, member := range state.Members {
		memberSet[member.PeerID] = struct{}{}
	}
	state.Ready = state.Eligible
	if state.Ready {
		for _, peerID := range target.ExpectedPeers {
			if _, ok := memberSet[peerID]; !ok {
				state.Ready = false
				break
			}
		}
	}
	if !state.Ready && state.LastError == "" {
		state.LastError = "demo group exists but roster is incomplete for the selected target nodes"
	}
	return state
}

func (a *App) profileLocked(id string) (DemoInstanceProfile, string, bool) {
	if profile, ok := findProfileInSlice(a.workspace.GuiLane.Instances, id); ok {
		return profile, a.workspace.GuiLane.ID, true
	}
	if profile, ok := findProfileInSlice(a.workspace.HeadlessLane.Instances, id); ok {
		return profile, a.workspace.HeadlessLane.ID, true
	}
	return DemoInstanceProfile{}, "", false
}

func (a *App) laneProfiles(laneID string) []DemoInstanceProfile {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch laneID {
	case "gui":
		return append([]DemoInstanceProfile(nil), a.workspace.GuiLane.Instances...)
	case "headless":
		return append([]DemoInstanceProfile(nil), a.workspace.HeadlessLane.Instances...)
	default:
		return nil
	}
}

func (a *App) lookupProfileAndToken(id string) (DemoInstanceProfile, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	profile, _, ok := a.profileLocked(id)
	if !ok {
		return DemoInstanceProfile{}, "", fmt.Errorf("unknown instance %s", id)
	}
	return profile, a.workspace.Token, nil
}

func (a *App) setInstanceError(id string, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.errors[id] = message
}

func (a *App) setInstanceWarning(id string, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.warnings[id] = message
}

func (a *App) workspaceWarnings() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.workspaceWarning...)
}

func loadOrCreateWorkspace(repoRoot string) (DemoWorkspace, []string, error) {
	controlRoot := filepath.Join(repoRoot, ".demo-control")
	if err := os.MkdirAll(controlRoot, 0o700); err != nil {
		return DemoWorkspace{}, nil, err
	}
	path := filepath.Join(controlRoot, "workspace.json")
	if fileExists(path) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return DemoWorkspace{}, nil, err
		}
		var ws DemoWorkspace
		if err := json.Unmarshal(raw, &ws); err != nil {
			return DemoWorkspace{}, nil, err
		}
		warnings := normalizeWorkspace(&ws, repoRoot)
		_ = saveWorkspace(path, ws)
		return ws, warnings, nil
	}
	ws := defaultWorkspace(repoRoot)
	if err := saveWorkspace(path, ws); err != nil {
		return DemoWorkspace{}, nil, err
	}
	return ws, nil, nil
}

func defaultWorkspace(repoRoot string) DemoWorkspace {
	token := randomToken()
	guiRoot := filepath.Join(repoRoot, ".demo-control", "gui")
	headlessRoot := filepath.Join(repoRoot, ".demo-control", "headless")
	ws := DemoWorkspace{
		Name:     "default",
		RepoRoot: repoRoot,
		AppDir:   filepath.Join(repoRoot, "app"),
		AppExe:   filepath.Join(repoRoot, "app", "build", "bin", "SecureP2P.exe"),
		Token:    token,
		GuiLane: DemoLaneConfig{
			ID:           "gui",
			Label:        "GUI Demo",
			Description:  "Windows GUI lane for live product walkthroughs",
			RuntimeRoot:  filepath.Join(guiRoot, "runtimes"),
			TemplateRoot: filepath.Join(guiRoot, "templates"),
		},
		HeadlessLane: DemoLaneConfig{
			ID:            "headless",
			Label:         "Headless Demo",
			Description:   "Docker headless lane for network partition and replay demos",
			RuntimeRoot:   filepath.Join(headlessRoot, "runtimes"),
			TemplateRoot:  filepath.Join(headlessRoot, "templates"),
			DemoGroupID:   defaultDemoGroupID,
			DemoOwnerNode: "node-01",
		},
	}

	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("gui-%02d", i)
		runtimeDir := filepath.Join(ws.GuiLane.RuntimeRoot, id)
		ws.GuiLane.Instances = append(ws.GuiLane.Instances, DemoInstanceProfile{
			ID:          id,
			Label:       fmt.Sprintf("GUI Node %02d", i),
			LaunchMode:  "exe",
			RuntimeDir:  runtimeDir,
			TemplateDir: filepath.Join(ws.GuiLane.TemplateRoot, id),
			DBPath:      filepath.Join(runtimeDir, "app.db"),
			BindIP:      "127.0.0.1",
			P2PPort:     4200 + i,
			ControlPort: 5200 + i,
			Headless:    false,
		})
	}

	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("node-%02d", i)
		runtimeDir := filepath.Join(ws.HeadlessLane.RuntimeRoot, id)
		ws.HeadlessLane.Instances = append(ws.HeadlessLane.Instances, DemoInstanceProfile{
			ID:          id,
			Label:       fmt.Sprintf("Node %02d", i),
			LaunchMode:  "docker",
			RuntimeDir:  runtimeDir,
			TemplateDir: filepath.Join(ws.HeadlessLane.TemplateRoot, id),
			DBPath:      filepath.Join(runtimeDir, "app.db"),
			BindIP:      fmt.Sprintf("10.20.30.%d", 10+i),
			P2PPort:     4100 + i,
			ControlPort: 5100 + i,
			Headless:    true,
		})
	}
	return ws
}

func normalizeWorkspace(ws *DemoWorkspace, repoRoot string) []string {
	var warnings []string
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

	if len(ws.GuiLane.Instances) == 0 && len(ws.HeadlessLane.Instances) == 0 && len(ws.Instances) > 0 {
		legacy := append([]DemoInstanceProfile(nil), ws.Instances...)
		legacyToken := ws.Token
		legacyAppDir := ws.AppDir
		legacyAppExe := ws.AppExe
		legacyName := ws.Name
		*ws = defaultWorkspace(repoRoot)
		if legacyName != "" {
			ws.Name = legacyName
		}
		if legacyToken != "" {
			ws.Token = legacyToken
		}
		if legacyAppDir != "" {
			ws.AppDir = legacyAppDir
		}
		if legacyAppExe != "" {
			ws.AppExe = legacyAppExe
		}
		ws.Instances = nil
		ws.HeadlessLane.Instances = nil
		for i := range legacy {
			legacy[i].LaunchMode = "docker"
			legacy[i].Headless = true
			if legacy[i].TemplateDir == "" {
				legacy[i].TemplateDir = filepath.Join(repoRoot, ".demo-control", "templates", legacy[i].ID)
			}
			ws.HeadlessLane.Instances = append(ws.HeadlessLane.Instances, legacy[i])
		}
	}

	if ws.GuiLane.ID == "" {
		ws.GuiLane.ID = "gui"
	}
	if ws.GuiLane.Label == "" {
		ws.GuiLane.Label = "GUI Demo"
	}
	if ws.HeadlessLane.ID == "" {
		ws.HeadlessLane.ID = "headless"
	}
	if ws.HeadlessLane.Label == "" {
		ws.HeadlessLane.Label = "Headless Demo"
	}
	if ws.GuiLane.RuntimeRoot == "" {
		ws.GuiLane.RuntimeRoot = filepath.Join(repoRoot, ".demo-control", "gui", "runtimes")
	}
	if ws.GuiLane.TemplateRoot == "" {
		ws.GuiLane.TemplateRoot = filepath.Join(repoRoot, ".demo-control", "gui", "templates")
	}
	if ws.HeadlessLane.RuntimeRoot == "" {
		ws.HeadlessLane.RuntimeRoot = filepath.Join(repoRoot, ".demo-control", "headless", "runtimes")
	}
	if ws.HeadlessLane.TemplateRoot == "" {
		ws.HeadlessLane.TemplateRoot = filepath.Join(repoRoot, ".demo-control", "headless", "templates")
	}
	if ws.HeadlessLane.DemoGroupID == "" {
		ws.HeadlessLane.DemoGroupID = defaultDemoGroupID
	}
	if ws.HeadlessLane.DemoOwnerNode == "" && len(ws.HeadlessLane.Instances) > 0 {
		ws.HeadlessLane.DemoOwnerNode = ws.HeadlessLane.Instances[0].ID
	}

	warnings = append(warnings, migrateLegacyHeadlessPaths(ws, repoRoot)...)

	normalizeLaneProfiles(ws.GuiLane.Instances, ws.GuiLane.RuntimeRoot, ws.GuiLane.TemplateRoot, "exe", false, 4201, 5201, false)
	normalizeLaneProfiles(ws.HeadlessLane.Instances, ws.HeadlessLane.RuntimeRoot, ws.HeadlessLane.TemplateRoot, "docker", true, 4101, 5101, true)
	ensureGuiLaneNodeCount(ws)
	ws.Instances = nil
	return warnings
}

func normalizeLaneProfiles(profiles []DemoInstanceProfile, runtimeRoot string, templateRoot string, defaultMode string, defaultHeadless bool, p2pBase int, controlBase int, dockerBind bool) {
	for i := range profiles {
		if profiles[i].RuntimeDir == "" {
			profiles[i].RuntimeDir = filepath.Join(runtimeRoot, profiles[i].ID)
		}
		if profiles[i].TemplateDir == "" {
			profiles[i].TemplateDir = filepath.Join(templateRoot, profiles[i].ID)
		}
		if profiles[i].DBPath == "" {
			profiles[i].DBPath = filepath.Join(profiles[i].RuntimeDir, "app.db")
		}
		if profiles[i].LaunchMode == "" {
			profiles[i].LaunchMode = defaultMode
		}
		if profiles[i].P2PPort == 0 {
			profiles[i].P2PPort = p2pBase + i
		}
		if profiles[i].ControlPort == 0 {
			profiles[i].ControlPort = controlBase + i
		}
		if profiles[i].Label == "" {
			profiles[i].Label = profiles[i].ID
		}
		if profiles[i].LaunchMode == "docker" {
			profiles[i].Headless = true
		} else {
			profiles[i].Headless = defaultHeadless
		}
		if profiles[i].BindIP == "" {
			if dockerBind {
				profiles[i].BindIP = fmt.Sprintf("10.20.30.%d", 11+i)
			} else {
				profiles[i].BindIP = "127.0.0.1"
			}
		}
	}
}

func migrateLegacyHeadlessPaths(ws *DemoWorkspace, repoRoot string) []string {
	if ws == nil {
		return nil
	}
	legacyRuntimeRoot := filepath.Join(repoRoot, ".demo-control", "runtimes")
	legacyTemplateRoot := filepath.Join(repoRoot, ".demo-control", "templates")
	var warnings []string

	for i := range ws.HeadlessLane.Instances {
		profile := &ws.HeadlessLane.Instances[i]
		targetRuntime := filepath.Join(ws.HeadlessLane.RuntimeRoot, profile.ID)
		targetTemplate := filepath.Join(ws.HeadlessLane.TemplateRoot, profile.ID)

		if migrated, warning := migrateLegacyNodePath(profile.ID, profile.RuntimeDir, targetRuntime, legacyRuntimeRoot); warning != "" {
			warnings = append(warnings, warning)
		} else if migrated != "" {
			profile.RuntimeDir = migrated
			if profile.DBPath == "" || isPathWithin(profile.DBPath, legacyRuntimeRoot) || isPathWithin(profile.DBPath, profile.RuntimeDir) {
				profile.DBPath = filepath.Join(profile.RuntimeDir, "app.db")
			}
		}

		if migrated, warning := migrateLegacyNodePath(profile.ID, profile.TemplateDir, targetTemplate, legacyTemplateRoot); warning != "" {
			warnings = append(warnings, warning)
		} else if migrated != "" {
			profile.TemplateDir = migrated
		}
	}

	return warnings
}

func migrateLegacyNodePath(nodeID string, currentPath string, targetPath string, legacyRoot string) (string, string) {
	currentPath = strings.TrimSpace(currentPath)
	if currentPath == "" || !isPathWithin(currentPath, legacyRoot) {
		return "", ""
	}
	if pathsEqual(currentPath, targetPath) {
		return targetPath, ""
	}

	srcExists := fileExists(currentPath)
	dstExists := fileExists(targetPath)
	if srcExists && dstExists {
		return "", fmt.Sprintf("%s migration conflict: both %s and %s exist", nodeID, currentPath, targetPath)
	}
	if srcExists {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			return "", fmt.Sprintf("%s migration failed creating %s: %v", nodeID, filepath.Dir(targetPath), err)
		}
		if err := os.Rename(currentPath, targetPath); err != nil {
			return "", fmt.Sprintf("%s migration failed moving %s to %s: %v", nodeID, currentPath, targetPath, err)
		}
	}
	return targetPath, ""
}

func ensureRuntimeSeeded(profile DemoInstanceProfile) (string, error) {
	if err := os.MkdirAll(profile.RuntimeDir, 0o700); err != nil {
		return "", err
	}

	dbPath := profile.DBPath
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join(profile.RuntimeDir, "app.db")
	}
	if fileExists(dbPath) {
		return "", nil
	}

	templateDir := strings.TrimSpace(profile.TemplateDir)
	if templateDir == "" || !fileExists(templateDir) {
		return "runtime has no app.db and no template was found; node will boot from an empty runtime", nil
	}
	if err := copyDir(templateDir, profile.RuntimeDir); err != nil {
		return "", fmt.Errorf("seed runtime %s from template %s: %w", profile.RuntimeDir, templateDir, err)
	}
	return "runtime was auto-seeded from template before start", nil
}

func saveWorkspace(path string, ws DemoWorkspace) error {
	raw, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func (a *App) preflightGUI(ws DemoWorkspace) PreflightResult {
	out := PreflightResult{OK: true}
	if _, err := exec.LookPath("cargo"); err != nil {
		out.OK = false
		out.Errors = append(out.Errors, "cargo is not installed or not on PATH")
	}
	if _, err := exec.LookPath("wails"); err != nil {
		out.OK = false
		out.Errors = append(out.Errors, "wails CLI is not installed or not on PATH")
	}
	if !fileExists(filepath.Join(ws.RepoRoot, "crypto-engine", "Cargo.toml")) {
		out.OK = false
		out.Errors = append(out.Errors, "crypto-engine/Cargo.toml was not found")
	}
	if !fileExists(filepath.Join(ws.AppDir, "wails.json")) {
		out.OK = false
		out.Errors = append(out.Errors, "app/wails.json was not found")
	}
	if !fileExists(ws.AppExe) {
		out.Warnings = append(out.Warnings, "SecureP2P.exe not found yet. Run Build EXE to create it.")
	}
	return out
}

func (a *App) preflightHeadless() PreflightResult {
	out := PreflightResult{OK: true}
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		out.OK = false
		out.Errors = append(out.Errors, "Docker daemon is not running or Docker CLI is not installed.")
		return out
	}
	imageCheck := exec.Command("docker", "image", "inspect", "secure-p2p:latest")
	if err := imageCheck.Run(); err != nil {
		out.Warnings = append(out.Warnings, "Docker image 'secure-p2p:latest' not found. Use Build Image in Headless Demo.")
	}
	if !dockerNetworkExists(sharedDockerNetwork) {
		out.Warnings = append(out.Warnings, "Shared Docker network datn_p2p_net does not exist yet; it will be created on first start.")
	}
	return out
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
	case int64:
		return int(n)
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

func stringSliceFromAny(v interface{}) []string {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
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

func openBuildTerminal(repoRoot string, title string, windowsCommand string) error {
	switch runtime.GOOS {
	case "windows":
		fullCommand := fmt.Sprintf("title %s && %s", title, windowsCommand)
		psCommand := fmt.Sprintf(
			`Start-Process -FilePath 'cmd.exe' -WorkingDirectory '%s' -ArgumentList '/k', '%s'`,
			powerShellQuote(filepath.Clean(repoRoot)),
			powerShellQuote(fullCommand),
		)
		return exec.Command("powershell.exe", "-NoProfile", "-Command", psCommand).Start()
	case "darwin":
		script := fmt.Sprintf(`tell application "Terminal" to do script "cd %s && %s"`, shellQuote(filepath.Clean(repoRoot)), strings.ReplaceAll(windowsCommand, `"`, `\"`))
		return exec.Command("osascript", "-e", script).Start()
	default:
		return exec.Command("x-terminal-emulator", "-e", "sh", "-lc", windowsCommand).Start()
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func powerShellQuote(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func ensureGuiLaneNodeCount(ws *DemoWorkspace) {
	if ws == nil {
		return
	}
	existing := make(map[string]struct{}, len(ws.GuiLane.Instances))
	maxP2P := 4200
	maxControl := 5200
	for _, profile := range ws.GuiLane.Instances {
		existing[profile.ID] = struct{}{}
		if profile.P2PPort > maxP2P {
			maxP2P = profile.P2PPort
		}
		if profile.ControlPort > maxControl {
			maxControl = profile.ControlPort
		}
	}
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("gui-%02d", i)
		if _, ok := existing[id]; ok {
			continue
		}
		runtimeDir := filepath.Join(ws.GuiLane.RuntimeRoot, id)
		ws.GuiLane.Instances = append(ws.GuiLane.Instances, DemoInstanceProfile{
			ID:          id,
			Label:       fmt.Sprintf("GUI Node %02d", i),
			LaunchMode:  "exe",
			RuntimeDir:  runtimeDir,
			TemplateDir: filepath.Join(ws.GuiLane.TemplateRoot, id),
			DBPath:      filepath.Join(runtimeDir, "app.db"),
			BindIP:      "127.0.0.1",
			P2PPort:     maxP2P + 1,
			ControlPort: maxControl + 1,
			Headless:    false,
		})
		maxP2P++
		maxControl++
	}
	sort.Slice(ws.GuiLane.Instances, func(i, j int) bool {
		return ws.GuiLane.Instances[i].ID < ws.GuiLane.Instances[j].ID
	})
}

func ensureDemoPath(root string, target string) error {
	absRoot, _ := filepath.Abs(filepath.Join(root, ".demo-control"))
	absTarget, _ := filepath.Abs(target)
	if !strings.HasPrefix(strings.ToLower(absTarget), strings.ToLower(absRoot)) {
		return fmt.Errorf("refusing to reset path outside .demo-control: %s", target)
	}
	return nil
}

func isPathWithin(target string, root string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(absTarget), strings.ToLower(absRoot))
}

func pathsEqual(left string, right string) bool {
	absLeft, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	absRight, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	return strings.EqualFold(absLeft, absRight)
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

func getRunningContainers() (map[string]bool, error) {
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}").CombinedOutput()
	if err != nil {
		return nil, err
	}
	running := make(map[string]bool)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" {
			running[name] = true
		}
	}
	return running, nil
}

func isContainerRunning(id string) bool {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", id).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func getContainerPID(id string) int {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Pid}}", id).CombinedOutput()
	if err != nil {
		return 0
	}
	var pid int
	_, _ = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &pid)
	return pid
}

func (a *App) isInstanceRunning(id string) bool {
	a.mu.Lock()
	cmd := a.procs[id]
	profile, _, hasProfile := a.profileLocked(id)
	a.mu.Unlock()
	if hasProfile && profile.LaunchMode == "exe" {
		return cmd != nil && cmd.Process != nil && cmd.ProcessState == nil
	}
	return isContainerRunning(id)
}

func findProfileInSlice(profiles []DemoInstanceProfile, id string) (DemoInstanceProfile, bool) {
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return DemoInstanceProfile{}, false
}

func findStatusByID(statuses []InstanceStatus, id string) (InstanceStatus, bool) {
	for _, status := range statuses {
		if status.Profile.ID == id {
			return status, true
		}
	}
	return InstanceStatus{}, false
}

func isEligibleDemoState(appState string) bool {
	switch strings.ToUpper(strings.TrimSpace(appState)) {
	case "AUTHORIZED", "ADMIN_READY":
		return true
	default:
		return false
	}
}

func (a *App) Notify(message string) {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "demo:notice", message)
	}
}
