//go:build windows

package cli

import "os/exec"

// setProcessGroup is a no-op on Windows: POSIX process groups don't apply,
// and psql/redis-cli don't spawn further children, so killing the immediate
// process is sufficient.
func setProcessGroup(cmd *exec.Cmd) {}

// terminateProcessGroup kills the immediate process (best effort on Windows).
func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// killProcessGroupNow kills the immediate process (same as terminate on Windows).
func killProcessGroupNow(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
