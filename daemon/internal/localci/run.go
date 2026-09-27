package localci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/proc"
)

// The runner is the one thing in this package that starts a program: a step's command, run through
// a shell in the card's worktree, with the workflow's own environment. It sits behind the Runner
// interface so the engine never depends on starting a process, and a test drives the whole engine
// with a fake and never runs anybody's test suite.
//
// A shell is what makes the run the same as GitHub's: a `run:` block is shell script, several
// commands to a line, and the file may name `bash`. Marshal runs /bin/sh -e or /bin/bash -e -o
// pipefail, which is GitHub's own default for a Linux job, so an early failing command fails the
// step here too rather than being passed over.

const (
	// DefaultStepTimeout bounds one step's run. A step that has not answered by then is killed,
	// because a check that hangs is worse than a check that was skipped.
	DefaultStepTimeout = 10 * time.Minute
	// DefaultOutputBytes is how much of one step's output is kept. The end is kept rather than the
	// start, because that is where a failing test says what went wrong.
	DefaultOutputBytes = 64 << 10
	// DefaultOutputLines is how many lines of one step's output are kept, counted from the end.
	DefaultOutputLines = 200
	// stopGrace is how long a step is given to stop politely before its whole tree is killed.
	stopGrace = 2 * time.Second
)

// RunRequest is what a runner is given: the worktree to run in, the shell, the step's command, and
// the environment the workflow gave it.
type RunRequest struct {
	// Dir is the card's worktree, which the command runs in.
	Dir string
	// Shell is the program to run the command with, such as "/bin/sh". It is chosen by shellPath.
	Shell string
	// Command is the step's `run:` text, which the shell reads as a script.
	Command string
	// Env are the workflow's, the job's, and the step's own environment entries, as KEY=value, in
	// the order they were declared, so the step's own entry wins over the job's.
	Env []string
}

// RunResult is what a run produced. A step that exits non-zero is not an error: failing is how a
// step says it failed. An error is a step that could not be started or read at all.
type RunResult struct {
	// Output is the end of what the command printed, trimmed.
	Output string
	// Failed says the command exited non-zero.
	Failed bool
	// Took is how long the command ran.
	Took time.Duration
}

// Runner runs one step's command. CommandRunner is the real one; a test gives a fake that never
// starts a process.
type Runner interface {
	// Run runs a step and answers what it printed. It answers an error only when the command could
	// not be started or its output could not be read.
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

// CommandRunner runs a step's command as a child process through internal/proc, which puts the
// child in its own process group, gives it a filtered environment, and always reaps it. Stopping a
// step that ran past its time limit ends the whole tree, so a command that started a helper quits
// too.
type CommandRunner struct {
	timeout  time.Duration
	maxBytes int
	maxLines int
}

// NewCommandRunner builds the real runner. A timeout or a size left at zero takes its default.
func NewCommandRunner(timeout time.Duration, maxBytes, maxLines int) *CommandRunner {
	if timeout <= 0 {
		timeout = DefaultStepTimeout
	}
	if maxBytes <= 0 {
		maxBytes = DefaultOutputBytes
	}
	if maxLines <= 0 {
		maxLines = DefaultOutputLines
	}
	return &CommandRunner{timeout: timeout, maxBytes: maxBytes, maxLines: maxLines}
}

// Run runs one step's command with a time limit and reads the end of what it printed.
func (c *CommandRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if strings.TrimSpace(req.Command) == "" {
		return RunResult{}, nil
	}
	shell := req.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	started := time.Now()
	child, err := proc.Start(ctx, proc.Spec{
		Path: shell, Args: shellArgs(shell, req.Command), Dir: req.Dir, Env: req.Env,
	})
	if err != nil {
		return RunResult{Took: time.Since(started)}, fmt.Errorf("start the step: %w", err)
	}
	tail := &tailWriter{limit: c.maxBytes}
	_, readErr := io.Copy(tail, child.Stdout)
	// Stop ends the whole tree, which is what makes a command that spawned a helper quit too. It is
	// safe after a clean exit, and it is what bounds a command that keeps printing past the read.
	_ = child.Stop(context.WithoutCancel(ctx), stopGrace)
	exit := child.Wait()
	out := tailLines(tail.String(), c.maxLines)
	took := time.Since(started)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return RunResult{Output: out, Took: took}, fmt.Errorf("read what the step printed: %w", readErr)
	}
	return RunResult{Output: out, Failed: exit.Code != 0, Took: took}, nil
}

// shellArgs is how a step's command is handed to its shell: `-e` so an early failing command fails
// the step, which is GitHub's own default, and `-o pipefail` for bash so a failure anywhere in a
// pipeline is seen rather than only the last command's.
func shellArgs(shell, command string) []string {
	if strings.HasSuffix(shell, "bash") {
		return []string{"-e", "-o", "pipefail", "-c", command}
	}
	return []string{"-e", "-c", command}
}

// tailWriter keeps the end of what a step printed. A step that prints without end must not fill the
// daemon's memory, and the last few lines are the ones that say what went wrong, so the start of a
// long log is dropped rather than the end.
type tailWriter struct {
	limit int
	buf   []byte
}

// Write appends what was printed and keeps only the last limit bytes.
func (t *tailWriter) Write(p []byte) (int, error) {
	n := len(p)
	if t.limit <= 0 {
		return n, nil
	}
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.limit {
		t.buf = append(t.buf[:0], t.buf[len(t.buf)-t.limit:]...)
	}
	return n, nil
}

// String answers what was kept.
func (t *tailWriter) String() string { return string(t.buf) }

// tailLines keeps the last n lines of a text and trims the whitespace around the whole answer, so
// a step's output reads as the end of a log rather than as a slab of blank lines.
func tailLines(text string, n int) string {
	text = strings.TrimRight(text, "\n")
	if n > 0 {
		if lines := strings.Split(text, "\n"); len(lines) > n {
			text = strings.Join(lines[len(lines)-n:], "\n")
		}
	}
	return strings.TrimRight(text, " \t\n")
}
