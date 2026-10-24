//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// setProcessGroup is intentionally a no-op: psql/redis-cli don't spawn
// further children, and putting them in a separate process group breaks
// their own terminal job control - e.g. the tcsetattr an interactive
// password prompt needs (to disable echo) raises SIGTTOU, and the default
// action for a background process group is to stop, not run, which looks
// exactly like a hang. Signaling the single child PID directly, keeping it
// in our own (foreground) process group, is sufficient and correct here.
func setProcessGroup(cmd *exec.Cmd) {}

// terminateProcessGroup asks cmd's process to exit.
func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
}

// killProcessGroupNow forces cmd's process to exit immediately.
func killProcessGroupNow(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
