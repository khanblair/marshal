//go:build windows

package proc

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

// foldEnvNames is true because Windows treats environment variable names as case insensitive.
const foldEnvNames = true

// configure gives the child its own process group and keeps a console window from appearing.
func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
}

// polite does nothing here. A console program cannot be asked to close with taskkill and
// without /F, so the polite request is closing standard input, which Stop does first.
func polite(int) error { return nil }

// killTree kills the process and everything it started.
func killTree(pid int) error {
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill process %d: %w (%s)", pid, err, out)
	}
	return nil
}

// sweep does nothing here: once the leader has exited, taskkill can no longer find its children.
func sweep(int) {}
