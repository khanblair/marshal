//go:build unix

package claude

import "syscall"

// sigintPid sends SIGINT to the process: Claude Code's own documentation says this is how to end
// a turn in flight without leaving it "unfinished" for a later --resume to replay, unlike SIGTERM
// (see the report's Ruling on the interrupt fallback). It does not start, stop, or wait for the
// process; internal/proc still owns all of that.
func sigintPid(pid int) error {
	return syscall.Kill(pid, syscall.SIGINT)
}
