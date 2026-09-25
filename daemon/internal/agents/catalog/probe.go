package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/khanblair/marshal/daemon/internal/proc"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	defaultVersionTimeout = 5 * time.Second
	// versionOutputBytes bounds what is read from a program's answer to --version. A version is a
	// few words, so a program that prints more is not answering the question.
	versionOutputBytes = 4 << 10
	// versionStopGrace is how long a program that has already answered gets to exit before it is
	// killed.
	versionStopGrace = 200 * time.Millisecond
	// versionStopTimeout bounds waiting for that exit; it is longer than the grace period itself
	// so the kill has time to land before the context gives up on it.
	versionStopTimeout = 2 * versionStopGrace
)

// ErrNotFound means the agent's program is not installed anywhere that the probe looks.
var ErrNotFound = errors.New("the program is not installed")

// ErrUnreadable means the program is there but did not give a version: it failed, hung, or printed
// something that holds no version.
var ErrUnreadable = errors.New("the program did not give a version")

// Found is what a probe learned about an installed agent.
type Found struct {
	// Path is where the program is. It stays on the daemon's side and is never sent to clients.
	Path    string
	Version Semver
}

// Probe looks for one kind of agent on the machine. The catalog calls one probe for each kind at
// the same time, so an implementation must be safe for concurrent use, and it must give up when
// the context ends. Tests pass a fake.
type Probe interface {
	// Probe returns what it found. It returns an error that wraps ErrNotFound when the agent is not
	// installed, and one that wraps ErrUnreadable, together with the path, when the program is
	// there but gives no version.
	Probe(ctx context.Context, kind protocol.AgentKind) (Found, error)
}

// ProbeOptions sets how a ProgramProbe looks for programs.
type ProbeOptions struct {
	// Dirs are searched, in order, after the daemon's PATH. A daemon started as a user service
	// has a short PATH, so the folders where agents usually install go here (see KnownDirs).
	Dirs []string
	// LookPath finds a program by name, or checks that a path can be run. The default is
	// exec.LookPath.
	LookPath func(name string) (string, error)
	// VersionTimeout is how long a program gets to answer --version. The default is 5 seconds.
	VersionTimeout time.Duration
}

// ProgramProbe is the probe for real machines: it finds the program, and asks it for its version
// through internal/proc.
type ProgramProbe struct {
	dirs    []string
	look    func(string) (string, error)
	timeout time.Duration
}

var _ Probe = (*ProgramProbe)(nil)

// NewProbe returns a probe that searches the given folders.
func NewProbe(opts ProbeOptions) *ProgramProbe {
	p := &ProgramProbe{dirs: opts.Dirs, look: opts.LookPath, timeout: opts.VersionTimeout}
	if p.look == nil {
		p.look = exec.LookPath
	}
	if p.timeout <= 0 {
		p.timeout = defaultVersionTimeout
	}
	return p
}

// DefaultProbe returns the probe that the daemon uses: the PATH, then the well-known folders.
func DefaultProbe() *ProgramProbe {
	return NewProbe(ProbeOptions{Dirs: KnownDirs()})
}

// Probe finds the program of a kind and reads its version.
func (p *ProgramProbe) Probe(ctx context.Context, kind protocol.AgentKind) (Found, error) {
	program := specFor(kind).program
	if program == "" {
		return Found{}, fmt.Errorf("%w: there is no program for %q", ErrNotFound, kind)
	}
	path, ok := p.locate(program)
	if !ok {
		return Found{}, fmt.Errorf("%w: %s", ErrNotFound, program)
	}
	version, err := p.readVersion(ctx, path)
	if err != nil {
		return Found{Path: path}, fmt.Errorf("read the version of %s: %w: %w", program, ErrUnreadable, err)
	}
	return Found{Path: path, Version: version}, nil
}

// locate looks for a program on PATH first, and then in the well-known folders.
func (p *ProgramProbe) locate(program string) (string, bool) {
	if path, err := p.look(program); err == nil {
		return path, true
	}
	for _, dir := range p.dirs {
		if path, err := p.look(filepath.Join(dir, program)); err == nil {
			return path, true
		}
	}
	return "", false
}

// readVersion runs the program with --version and reads the answer. The answer is bounded, the
// wait is bounded, and a program that is still running afterwards is stopped with everything it
// started, so a hanging program leaves nothing behind.
func (p *ProgramProbe) readVersion(ctx context.Context, path string) (Semver, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	child, err := proc.Start(ctx, proc.Spec{Path: path, Args: []string{"--version"}, Env: ProgramEnv(path)})
	if err != nil {
		return Semver{}, fmt.Errorf("start it: %w", err)
	}
	out, _ := io.ReadAll(io.LimitReader(child.Stdout, versionOutputBytes))
	// Only the reader closes its end. The output that was read is all that is wanted from here.
	_ = child.Stdout.Close()
	stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), versionStopTimeout)
	defer stopCancel()
	if err := child.Stop(stopCtx, versionStopGrace); err != nil {
		return Semver{}, fmt.Errorf("stop it after the answer: %w", err)
	}
	exit := child.Wait()
	if err := ctx.Err(); err != nil {
		return Semver{}, fmt.Errorf("it did not answer in time: %w", err)
	}
	if exit.Err != nil {
		return Semver{}, fmt.Errorf("it exited with an error: %w", exit.Err)
	}
	version, ok := ParseSemver(string(out))
	if !ok {
		return Semver{}, errors.New("its answer holds no version")
	}
	return version, nil
}
