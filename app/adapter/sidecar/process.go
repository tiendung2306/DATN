package sidecar

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ProcessManager manages the Rust crypto-engine child process.
type ProcessManager struct {
	port       int
	cmd        *exec.Cmd
	mu         sync.Mutex
	cancelFunc context.CancelFunc
}

// NewProcessManager creates a process manager.
func NewProcessManager() *ProcessManager {
	return &ProcessManager{}
}

// GetFreePort asks the OS for an unused TCP port.
func (pm *ProcessManager) GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// StartEngine finds and starts the Rust sidecar binary.
func (pm *ProcessManager) StartEngine() (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	port, err := pm.GetFreePort()
	if err != nil {
		return 0, fmt.Errorf("failed to get free port: %w", err)
	}
	pm.port = port

	executable := "crypto-engine"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}

	cwd, _ := os.Getwd()
	possiblePaths := []string{
		filepath.Join(cwd, executable),
		filepath.Join(cwd, "..", "crypto-engine", "target", "debug", executable),
		filepath.Join(cwd, "..", "crypto-engine", "target", "release", executable),
		filepath.Join(cwd, "..", "..", "crypto-engine", "target", "debug", executable),
		filepath.Join(cwd, "..", "..", "crypto-engine", "target", "release", executable),
	}

	var binPath string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			binPath = p
			break
		}
	}

	if binPath == "" {
		return 0, fmt.Errorf("crypto-engine binary not found. Searched in: %v. Please build the rust project.", possiblePaths)
	}
	if err := ensureBinaryFresh(binPath); err != nil {
		return 0, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	pm.cancelFunc = cancel

	cmd := exec.CommandContext(ctx, binPath, "--port", fmt.Sprintf("%d", port))
	setSysProcAttr(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return 0, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return 0, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return 0, fmt.Errorf("failed to start crypto-engine: %w", err)
	}

	trackProcess(cmd.Process.Pid)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			slog.Info("Rust Engine", "msg", scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			slog.Warn("Rust Engine", "msg", scanner.Text())
		}
	}()

	pm.cmd = cmd
	slog.Info("Crypto Engine started", "port", port, "path", binPath)

	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Warn("Crypto Engine exited", "error", err)
		} else {
			slog.Info("Crypto Engine exited cleanly")
		}
	}()

	return port, nil
}

func ensureBinaryFresh(binPath string) error {
	binInfo, err := os.Stat(binPath)
	if err != nil {
		return fmt.Errorf("stat crypto-engine binary %q: %w", binPath, err)
	}
	latestSourcePath, latestSourceMod, ok, err := newestRustSourceArtifact(binPath)
	if err != nil || !ok {
		return err
	}
	if latestSourceMod.After(binInfo.ModTime()) {
		return fmt.Errorf(
			"crypto-engine binary is stale: binary=%q modified=%s, source=%q modified=%s. Rebuild with `cd crypto-engine && cargo build`",
			binPath,
			binInfo.ModTime().Format(time.RFC3339),
			latestSourcePath,
			latestSourceMod.Format(time.RFC3339),
		)
	}
	return nil
}

func newestRustSourceArtifact(binPath string) (string, time.Time, bool, error) {
	crateRoot, ok := rustCrateRootFromBinary(binPath)
	if !ok {
		return "", time.Time{}, false, nil
	}

	candidates := []string{
		filepath.Join(crateRoot, "Cargo.toml"),
		filepath.Join(crateRoot, "Cargo.lock"),
	}
	srcRoot := filepath.Join(crateRoot, "src")

	var (
		latestPath string
		latestMod  time.Time
		found      bool
	)
	recordNewest := func(path string, info fs.FileInfo) {
		if info == nil {
			return
		}
		if !found || info.ModTime().After(latestMod) {
			found = true
			latestMod = info.ModTime()
			latestPath = path
		}
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			recordNewest(candidate, info)
		}
	}

	if info, err := os.Stat(srcRoot); err == nil && info.IsDir() {
		walkErr := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			recordNewest(path, info)
			return nil
		})
		if walkErr != nil {
			return "", time.Time{}, false, fmt.Errorf("scan rust source tree %q: %w", srcRoot, walkErr)
		}
	}

	return latestPath, latestMod, found, nil
}

func rustCrateRootFromBinary(binPath string) (string, bool) {
	buildDir := filepath.Dir(binPath)
	base := filepath.Base(buildDir)
	if !strings.EqualFold(base, "debug") && !strings.EqualFold(base, "release") {
		return "", false
	}
	targetDir := filepath.Dir(buildDir)
	if !strings.EqualFold(filepath.Base(targetDir), "target") {
		return "", false
	}
	return filepath.Dir(targetDir), true
}

// StopCryptoEngine stops the sidecar process.
func (pm *ProcessManager) StopCryptoEngine() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.cancelFunc != nil {
		pm.cancelFunc()
		slog.Info("Crypto Engine stopped")
	}
}
