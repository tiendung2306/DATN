package main

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

type ProcessManager struct {
	port       int
	cmd        *exec.Cmd
	mu         sync.Mutex
	cancelFunc context.CancelFunc
}

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

// StartCryptoEngine finds and starts the Rust sidecar.
func (pm *ProcessManager) StartCryptoEngine() (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	port, err := pm.GetFreePort()
	if err != nil {
		return 0, fmt.Errorf("failed to get free port: %w", err)
	}
	pm.port = port

	// Determine binary path (looking in crypto-engine/target/debug)
	executable := "crypto-engine"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}

	// For development, we look into the Rust build directory
	cwd, _ := os.Getwd()
	binPath := filepath.Join(cwd, "..", "crypto-engine", "target", "debug", executable)

	// Fallback check if binary exists
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return 0, fmt.Errorf("crypto-engine binary not found at %s. Please run 'cargo build' in crypto-engine directory", binPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	pm.cancelFunc = cancel

	cmd := exec.CommandContext(ctx, binPath, "--port", fmt.Sprintf("%d", port))

	// Capture stdout/stderr and pipe to slog
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			slog.Info("Rust Engine", "msg", scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			slog.Error("Rust Engine", "err", scanner.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		cancel()
		return 0, fmt.Errorf("failed to start crypto-engine: %w", err)
	}

	pm.cmd = cmd
	slog.Info("Crypto Engine started", "port", port, "path", binPath)

	return port, nil
}

func (pm *ProcessManager) StopCryptoEngine() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.cancelFunc != nil {
		pm.cancelFunc()
		slog.Info("Crypto Engine stopped")
	}
}
