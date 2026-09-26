// Package pty is the adapter that runs a command line agent in a pseudo-terminal, through
// go-pty (a Unix pseudo-terminal, and ConPTY on Windows). It is for agents that have no
// structured mode and for the terminal view, where a person types to the program and watches its
// screen. It implements agents.Agent, but a terminal has no turns, tool calls, or permission
// requests: the only events are agents.TerminalOutput and agents.Exited, and Respond always
// fails.
//
// Besides the interface, the adapter has the calls that a terminal view needs: WriteRaw for what
// the person types, Resize for the size of the view, and Snapshot for a view that opens late.
package pty

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"

	gopty "github.com/aymanbagabas/go-pty"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

// Adapter runs sessions of one CLI in terminals. One Adapter can run many sessions at once, each
// with its own process.
type Adapter struct {
	cfg Config
	log *slog.Logger
	tee *teeWriter

	mu       sync.Mutex
	sessions map[string]*session
}

var (
	_ agents.Agent    = (*Adapter)(nil)
	_ agents.Terminal = (*Adapter)(nil)
)

// New returns an adapter for the program in cfg. It does not start anything.
func New(cfg Config) (*Adapter, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	a := &Adapter{cfg: cfg, log: cfg.Logger, sessions: make(map[string]*session)}
	if cfg.Tee != nil {
		a.tee = &teeWriter{w: cfg.Tee}
	}
	return a, nil
}

// Start starts the program in a new terminal, in spec.Cwd. The session id is made up here, and
// handed to Config.StartArgs when there is one. The model, thinking mode, and permission mode of
// the spec are not applied: a terminal program has no control for them, and the handle says so.
func (a *Adapter) Start(ctx context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	id, err := newSessionID()
	if err != nil {
		return agents.SessionHandle{}, err
	}
	args := a.cfg.Args
	if a.cfg.StartArgs != nil {
		args = a.cfg.StartArgs(id)
	}
	if a.cfg.InstructionArgs != nil && spec.Instructions != "" {
		args = append(slices.Clone(args), a.cfg.InstructionArgs(spec.Instructions)...)
	}
	return a.launch(ctx, id, args, spec)
}

// Resume starts the program again with the arguments that Config.ResumeArgs gives for the session
// id. It returns agents.ErrCannotResume when the config has no such arguments.
func (a *Adapter) Resume(
	ctx context.Context, sessionID string, spec agents.StartSpec,
) (agents.SessionHandle, error) {
	if a.cfg.ResumeArgs == nil {
		return agents.SessionHandle{}, fmt.Errorf("%w: this program has no way to resume", agents.ErrCannotResume)
	}
	if sessionID == "" {
		return agents.SessionHandle{}, fmt.Errorf("%w: there is no session id to resume", agents.ErrCannotResume)
	}
	return a.launch(ctx, sessionID, a.cfg.ResumeArgs(sessionID), spec)
}

// Send types the text and then Config.LineEnd into the terminal. A terminal has no turns, so it
// never returns agents.ErrBusy.
func (a *Adapter) Send(ctx context.Context, h agents.SessionHandle, msg agents.UserMessage) error {
	s, err := a.find(h)
	if err != nil {
		return err
	}
	return s.write(ctx, []byte(msg.Text+a.cfg.LineEnd))
}

// WriteRaw types bytes into the terminal exactly as they are, for a terminal view that forwards
// what the person types. It returns agents.ErrUnknownSession for a session that is not running.
func (a *Adapter) WriteRaw(ctx context.Context, h agents.SessionHandle, data []byte) error {
	s, err := a.find(h)
	if err != nil {
		return err
	}
	return s.write(ctx, data)
}

// Resize sets the size of the terminal, and the program is told about it.
func (a *Adapter) Resize(ctx context.Context, h agents.SessionHandle, cols, rows int) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("resize the terminal: %w", err)
	}
	if cols < 1 || rows < 1 || cols > MaxSize || rows > MaxSize {
		return fmt.Errorf("the terminal size %dx%d is out of range", cols, rows)
	}
	s, err := a.find(h)
	if err != nil {
		return err
	}
	return s.resize(cols, rows)
}

// Size returns the size the terminal has now, which is the configured size until the first Resize.
// It returns agents.ErrUnknownSession for a session that is not running.
func (a *Adapter) Size(h agents.SessionHandle) (cols, rows int, err error) {
	s, err := a.find(h)
	if err != nil {
		return 0, 0, err
	}
	cols, rows = s.size()
	return cols, rows, nil
}

// Interrupt types Ctrl-C, which the terminal turns into an interrupt for the program in front.
func (a *Adapter) Interrupt(ctx context.Context, h agents.SessionHandle) error {
	s, err := a.find(h)
	if err != nil {
		return err
	}
	return s.interrupt(ctx)
}

// Events returns the session's event channel: TerminalOutput while the program prints, then
// Exited, and then it is closed. Take it once, right after Start or Resume, and keep reading until
// it closes: a full channel slows the program down. For a session that is not running it returns
// a channel that is already closed and empty.
func (a *Adapter) Events(h agents.SessionHandle) <-chan agents.AgentEvent {
	s, err := a.find(h)
	if err != nil {
		closed := make(chan agents.AgentEvent)
		close(closed)
		return closed
	}
	return s.events
}

// Snapshot returns the last RingBytes of output of a running session, oldest first, so that a
// viewer that opens late can paint the screen. It may begin in the middle of an escape sequence,
// and it may be ahead of the events that were sent so far, since those are batched. It returns
// nil for a session that is not running.
func (a *Adapter) Snapshot(h agents.SessionHandle) []byte {
	s, err := a.find(h)
	if err != nil {
		return nil
	}
	return s.ring.snapshot()
}

// Respond always fails: a terminal has no structured permission requests, so none is waiting.
func (a *Adapter) Respond(_ context.Context, _ agents.SessionHandle, _ agents.ApprovalResponse) error {
	return fmt.Errorf("respond: %w", agents.ErrUnknownRequest)
}

// Stop ends the session. It closes in steps: a polite request, a wait of Config.StopGrace, and
// then the kill of the whole process tree. It is safe to call more than once, and a session that
// is already gone is not an error.
func (a *Adapter) Stop(ctx context.Context, h agents.SessionHandle) error {
	s, err := a.find(h)
	if err != nil {
		// A session that is not running is already stopped.
		return nil
	}
	return s.stop(ctx)
}

// Capabilities says what this adapter can do: resume when the config has resume arguments, and no
// structured events.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{Resume: a.cfg.ResumeArgs != nil}
}

// launch starts the program in a terminal and registers the session.
func (a *Adapter) launch(
	ctx context.Context, id string, args []string, spec agents.StartSpec,
) (agents.SessionHandle, error) {
	if err := ctx.Err(); err != nil {
		return agents.SessionHandle{}, fmt.Errorf("start %s: %w", a.cfg.Path, err)
	}
	if info, err := os.Stat(spec.Cwd); err != nil || !info.IsDir() {
		return agents.SessionHandle{}, fmt.Errorf(
			"start %s: the working folder %q is not a folder that exists", a.cfg.Path, spec.Cwd)
	}
	if a.isRunning(id) {
		return agents.SessionHandle{}, fmt.Errorf("session %q is already running", id)
	}
	if a.cfg.SpecArgs != nil {
		args = slices.Concat(args, a.cfg.SpecArgs(spec))
	}
	s, err := a.spawn(id, args, spec)
	if err != nil {
		return agents.SessionHandle{}, err
	}
	// The session is registered before it begins, so that a program that ends at once is
	// forgotten by its own supervisor and not left in the table.
	regErr := a.register(id, s)
	s.begin()
	if regErr != nil {
		// Two starts raced for one id. The newcomer must not run beside the first.
		if stopErr := s.stop(context.Background()); stopErr != nil {
			s.log.Warn("stopping a duplicate session failed", "err", stopErr)
		}
		return agents.SessionHandle{}, regErr
	}
	s.log.Info("terminal session started", "program", a.cfg.Path, "pid", s.pid)
	return agents.SessionHandle{ID: id, Label: spec.Label, Model: spec.Model, Thinking: spec.Thinking,
		PermissionMode: spec.PermissionMode, Capabilities: a.Capabilities()}, nil
}

// spawn opens a terminal at the configured size and starts the program in it. It returns a
// session that has not begun yet.
func (a *Adapter) spawn(id string, args []string, spec agents.StartSpec) (*session, error) {
	p, err := gopty.New()
	if err != nil {
		return nil, fmt.Errorf("open a terminal for %s: %w", a.cfg.Path, err)
	}
	if err := p.Resize(a.cfg.Cols, a.cfg.Rows); err != nil {
		a.closeQuietly(p)
		return nil, fmt.Errorf("size the terminal for %s: %w", a.cfg.Path, err)
	}
	cmd := p.Command(a.cfg.Path, args...)
	cmd.Dir = spec.Cwd
	cmd.Env = a.environment(spec)
	if err := cmd.Start(); err != nil {
		a.closeQuietly(p)
		return nil, fmt.Errorf("start %s: %w", a.cfg.Path, err)
	}
	if err := releaseChildEnd(p); err != nil {
		// Only the detection of the program's end gets slower, and the drain wait covers that.
		a.log.Warn("could not release the child end of the terminal", "session_id", id, "err", err)
	}
	return newSession(a, launched{id: id, label: spec.Label, pty: p, cmd: cmd}), nil
}

// closeQuietly closes a terminal on an error path, where the original error is the one to report.
func (a *Adapter) closeQuietly(p gopty.Pty) {
	if err := p.Close(); err != nil {
		a.log.Debug("closing a terminal that did not start", "err", err)
	}
}

// environment is what the program sees: the allowed part of the daemon's environment, TERM, the
// config's entries, and then the session's own, each replacing the one before.
func (a *Adapter) environment(spec agents.StartSpec) []string {
	extra := make([]string, 0, 1+len(a.cfg.Env)+len(spec.Env))
	extra = append(extra, "TERM="+terminalName)
	extra = append(extra, a.cfg.Env...)
	extra = append(extra, spec.Env...)
	return proc.Environ(extra)
}

// find returns the running session that a handle names.
func (a *Adapter) find(h agents.SessionHandle) (*session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[h.ID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", agents.ErrUnknownSession, h.ID)
	}
	return s, nil
}

// isRunning says whether a session with the id is running.
func (a *Adapter) isRunning(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.sessions[id]
	return ok
}

// register records a session that is running. Two running sessions cannot share an id.
func (a *Adapter) register(id string, s *session) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, taken := a.sessions[id]; taken {
		return fmt.Errorf("session %q is already running", id)
	}
	a.sessions[id] = s
	return nil
}

// forget drops a session that has ended. It leaves the entry alone when it belongs to another
// session with the same id.
func (a *Adapter) forget(id string, s *session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessions[id] == s {
		delete(a.sessions, id)
	}
}

// The bits of a version 4 UUID that are not random.
const (
	versionByte    = 6
	variantByte    = 8
	lowNibble      = 0x0f
	lowSixBits     = 0x3f
	version4       = 0x40
	variantRFC4122 = 0x80
)

// newSessionID makes a random id in the form of a version 4 UUID, which the CLIs that let the
// caller choose a session id accept.
func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("make a session id: %w", err)
	}
	b[versionByte] = b[versionByte]&lowNibble | version4
	b[variantByte] = b[variantByte]&lowSixBits | variantRFC4122
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
