//go:build !windows

package sidecar

import (
	"os/exec"
)

// setSysProcAttr is a no-op on non-Windows platforms.
func setSysProcAttr(cmd *exec.Cmd) {
	// No-op for macOS, Linux, etc.
}

// trackProcess is a no-op on non-Windows platforms.
func trackProcess(pid int) {
	// No-op for macOS, Linux, etc.
}
