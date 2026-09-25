// Package gitx runs the Git command line. Every other module reaches Git through it, and
// always with arguments as a list, never through a shell.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Git runs Git commands.
type Git struct {
	bin string
	env []string
}

// Option changes how a Git runs.
type Option func(*Git)

// WithEnv adds environment settings (as KEY=value) to every command. Tests use it to give
// commits an identity, since a CI machine has none.
func WithEnv(pairs ...string) Option {
	return func(g *Git) { g.env = append(g.env, pairs...) }
}

// New returns a Git that uses the git program on the PATH. Git never asks questions, and always
// answers in English, so its output can be read reliably.
func New(opts ...Option) *Git {
	g := &Git{bin: "git", env: []string{"GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Error is a Git command that failed.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	detail := strings.TrimSpace(e.Stderr)
	if detail == "" {
		detail = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), detail)
}

func (e *Error) Unwrap() error { return e.Err }

// Run runs `git <args>` in a folder and returns what it printed, without the trailing newline.
func (g *Git) Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, g.bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), g.env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("Git is not installed, or is not on the PATH")
		}
		return "", &Error{Args: args, Stderr: stderr.String(), Err: err}
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}
