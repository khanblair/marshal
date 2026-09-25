//go:build windows

package claude

import "errors"

// sigintPid has no equivalent on Windows: there is no portable way to deliver Ctrl-C to a process
// this one did not start in its own console process group. The caller (see forceCancelFallback in
// turn.go) treats the error as "SIGINT is not available here" and moves straight to stopping the
// process and resuming it on the next Send.
func sigintPid(int) error {
	return errors.New("interrupting a single process by signal is not supported on windows")
}
