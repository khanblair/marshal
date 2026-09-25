package proc

import "os"

// The functions below are for code that has to start its child by another route than Start, for
// example inside a pseudo-terminal (internal/agents/pty), and still wants the same environment
// rules and the same way of ending a process tree.

// Environ returns the environment for a child that the caller starts itself: the allowed part of
// the daemon's own environment, then the extra KEY=value entries, which win over it.
func Environ(extra []string) []string {
	return buildEnv(os.Environ(), extra)
}

// Polite asks the process group led by pid to end, as far as the platform can. It is SIGTERM to
// the group on Unix and nothing on Windows.
func Polite(pid int) error { return polite(pid) }

// KillTree kills the process pid leads and everything it started.
func KillTree(pid int) error { return killTree(pid) }

// Sweep kills what is left of the process group that pid led, once its leader has been collected.
func Sweep(pid int) { sweep(pid) }
