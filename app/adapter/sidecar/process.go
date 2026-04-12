package sidecar

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
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

	ctx, cancel := context.WithCancel(context.Background())
	pm.cancelFunc = cancel

	cmd := exec.CommandContext(ctx, binPath, "--port", fmt.Sprintf("%d", port))

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

// StopCryptoEngine stops the sidecar process.
func (pm *ProcessManager) StopCryptoEngine() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.cancelFunc != nil {
		pm.cancelFunc()
		slog.Info("Crypto Engine stopped")
	}
}
