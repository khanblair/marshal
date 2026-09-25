//go:build unix

package proc

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// foldEnvNames is false because environment variable names are case sensitive here.
const foldEnvNames = false

// configure puts the child in a process group of its own, so a signal to the group reaches the
// child and everything it starts, and nothing else.
func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// polite asks the process group to end.
func polite(pid int) error { return signalGroup(pid, syscall.SIGTERM) }

// killTree kills the whole process group.
func killTree(pid int) error { return signalGroup(pid, syscall.SIGKILL) }

// sweep kills what is left in the group after its leader has been collected. A child that
// outlives its leader is an orphan, and it may hold the output pipe open.
func sweep(pid int) {
	// Nothing is left to report if the group is already empty.
	_ = signalGroup(pid, syscall.SIGKILL)
}

// signalGroup sends a signal to every process in the group that the pid leads. A group that is
// already gone is not an error.
func signalGroup(pid int, sig syscall.Signal) error {
	err := syscall.Kill(-pid, sig)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return fmt.Errorf("signal process group %d: %w", pid, err)
}
