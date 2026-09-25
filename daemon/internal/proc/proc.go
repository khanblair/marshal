// Package proc is the one place that starts child processes. A program is always started with a
// list of arguments, never through a shell, in its own process group, with a filtered
// environment, and it is always reaped, so no zombie is left behind. Stopping asks politely
// first and then kills the whole process tree.
package proc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	// StderrTailBytes is how much of a child's standard error is kept, counted from the end. The
	// tail is for error messages, so it stays small however much the child prints.
	StderrTailBytes = 8 << 10

	// waitDelay bounds how long Wait lingers on the copy of standard error after the child has
	// gone. A grandchild that still holds the pipe open would otherwise block it forever.
	waitDelay = 2 * time.Second
	// killWait is how long Stop waits for the process to disappear after the hard kill.
	killWait = 5 * time.Second
)

// Spec says what to start.
type Spec struct {
	// Path is the program. A bare name is looked up on the daemon's PATH.
	Path string
	// Args are the arguments, without the program name. They are never passed through a shell.
	Args []string
	// Dir is the working folder. Empty means the daemon's own.
	Dir string
	// Env is added to the filtered environment as KEY=value entries, and wins over it. It is the
	// way to hand the child a MARSHAL_* setting or a provider variable: nothing of the sort is
	// inherited from the daemon. An agent that needs a login finds it in the home folder (HOME
	// or USERPROFILE), which is inherited.
	Env []string
	// Stdin asks for a pipe to the child's standard input. Without it the child reads nothing.
	Stdin bool
}

// Exit is how a process ended.
type Exit struct {
	// Code is the exit code, or -1 when a signal or a kill ended the process.
	Code int
	// Err is nil after a clean exit. Otherwise it says how the process ended.
	Err error
}

// Process is a running child. Its standard output is read from Stdout, and the caller owns
// closing it: that keeps the last lines readable after the child has exited.
type Process struct {
	// Stdin writes to the child. It is nil unless Spec.Stdin was set. Stop closes it.
	Stdin io.WriteCloser
	// Stdout reads what the child prints. It reaches end of file when the child and everything it
	// started has closed its end, which is why Stop kills the whole tree.
	Stdout io.ReadCloser

	cmd    *exec.Cmd
	pid    int
	stderr *tailBuffer
	done   chan struct{}
	exit   Exit

	closeStdin sync.Once
}

// Start starts a program. The context is the lifetime of the process: when it ends, the whole
// process tree is killed, so a caller that wants the process to outlive one request must pass a
// context that is not tied to that request.
func Start(ctx context.Context, spec Spec) (*Process, error) {
	if spec.Path == "" {
		return nil, errors.New("proc: no program to start")
	}
	if spec.Dir != "" {
		if info, err := os.Stat(spec.Dir); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("start %s: the working folder %s is not a folder that exists", spec.Path, spec.Dir)
		}
	}
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = buildEnv(os.Environ(), spec.Env)
	cmd.WaitDelay = waitDelay
	cmd.Cancel = func() error { return killTree(cmd.Process.Pid) }
	configure(cmd)
	tail := &tailBuffer{limit: StderrTailBytes}
	cmd.Stderr = tail

	pipes, err := openPipes(cmd, spec.Stdin)
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		pipes.closeAll()
		return nil, fmt.Errorf("start %s: %w", spec.Path, err)
	}
	// The child holds its own copies now. Closing ours is what lets the reader see end of file.
	pipes.closeChildEnds()

	p := &Process{
		Stdin: pipes.stdinWriterOrNil(), Stdout: pipes.stdoutReader,
		cmd: cmd, pid: cmd.Process.Pid, stderr: tail, done: make(chan struct{}),
	}
	go p.reap()
	return p, nil
}

// reap waits for the child exactly once. It is the only caller of cmd.Wait, so the child is
// always collected, and the result is kept for Wait and Stop to read.
func (p *Process) reap() {
	err := p.cmd.Wait()
	// Whatever the child left running in its group is ours to clean up as well.
	sweep(p.pid)
	p.exit = Exit{Code: exitCode(p.cmd, err), Err: err}
	close(p.done)
}

// exitCode reads the code from the finished command, or -1 when it has none.
func exitCode(cmd *exec.Cmd, err error) int {
	if cmd.ProcessState == nil {
		return -1
	}
	if err == nil {
		return 0
	}
	return cmd.ProcessState.ExitCode()
}

// Pid returns the process id.
func (p *Process) Pid() int { return p.pid }

// Done is closed once the process has exited and been collected.
func (p *Process) Done() <-chan struct{} { return p.done }

// Wait blocks until the process has exited and returns how it ended. It can be called from any
// number of goroutines. It only returns when the process ends, so a caller that needs a bound
// uses Done in a select, or Stop.
func (p *Process) Wait() Exit {
	<-p.done
	return p.exit
}

// StderrTail returns the last StderrTailBytes of what the child wrote to standard error.
func (p *Process) StderrTail() string { return p.stderr.String() }

// Stop ends the process and everything it started. It first asks politely (it closes standard
// input, and on Unix sends SIGTERM to the process group) and waits up to grace for the process to
// go. Then it kills the whole tree. A cancelled context skips the rest of the grace period. Stop
// is safe to call more than once and from several goroutines.
func (p *Process) Stop(ctx context.Context, grace time.Duration) error {
	if p.finished() {
		return nil
	}
	p.closeInput()
	// A failed polite request only means that the hard kill comes sooner.
	_ = polite(p.pid)
	if p.waitFor(ctx, grace) {
		return nil
	}
	killErr := killTree(p.pid)
	if p.waitFor(context.Background(), killWait) {
		return nil
	}
	if killErr != nil {
		return fmt.Errorf("stop process %d: %w", p.pid, killErr)
	}
	return fmt.Errorf("stop process %d: it did not exit after being killed", p.pid)
}

// finished says whether the process has been collected already.
func (p *Process) finished() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// closeInput closes standard input once.
func (p *Process) closeInput() {
	p.closeStdin.Do(func() {
		if p.Stdin != nil {
			// Closing a pipe the child may already have closed can fail, and nothing depends on it.
			_ = p.Stdin.Close()
		}
	})
}

// waitFor reports whether the process ended before d passed or ctx ended.
func (p *Process) waitFor(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-p.done:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return p.finished()
	}
}
