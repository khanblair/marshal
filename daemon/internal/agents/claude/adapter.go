package claude

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// Adapter runs Claude Code sessions. It implements agents.Agent, and one Adapter can run many
// sessions at once, each with its own process.
type Adapter struct {
	cfg Config
	log *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session
}

var _ agents.Agent = (*Adapter)(nil)

// New returns an adapter for the claude program at cfg.Path. It starts nothing.
func New(cfg Config) (agents.Agent, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	return &Adapter{cfg: cfg, log: cfg.Logger, sessions: make(map[string]*session)}, nil
}

// Factory returns the factory that the session manager registers for the claude kind.
func Factory(cfg Config) agents.Factory {
	return func() (agents.Agent, error) { return New(cfg) }
}

// Start starts a new Claude Code process and a session in it.
func (a *Adapter) Start(ctx context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	return a.open(ctx, spec, "")
}

// Resume starts a new Claude Code process with --resume and picks up the session with the given
// id.
func (a *Adapter) Resume(
	ctx context.Context, sessionID string, spec agents.StartSpec,
) (agents.SessionHandle, error) {
	if sessionID == "" {
		return agents.SessionHandle{}, fmt.Errorf("%w: there is no session id to resume", agents.ErrCannotResume)
	}
	return a.open(ctx, spec, sessionID)
}

// Send starts a turn and returns at once. It returns agents.ErrBusy while a turn is running.
func (a *Adapter) Send(ctx context.Context, h agents.SessionHandle, msg agents.UserMessage) error {
	s, err := a.usable(ctx, h, "send a message")
	if err != nil {
		return err
	}
	return s.send(msg)
}

// Interrupt asks Claude Code to stop the turn in flight and keeps the session. See turn.go for
// how it is done and what happens when Claude Code does not answer in time.
func (a *Adapter) Interrupt(_ context.Context, h agents.SessionHandle) error {
	s, err := a.find(h)
	if err != nil {
		return err
	}
	return s.interrupt()
}

// Events returns the session's event channel, closed after the Exited event. Take it once, right
// after Start or Resume. For a session that is not running it returns a channel that is already
// closed and empty.
func (a *Adapter) Events(h agents.SessionHandle) <-chan agents.AgentEvent {
	s, err := a.find(h)
	if err != nil {
		closed := make(chan agents.AgentEvent)
		close(closed)
		return closed
	}
	return s.sink.C()
}

// Respond always fails: Claude Code's streaming JSON mode has no way to ask the person about a
// tool yet (see catalog's claudeSpec, whose comment this mirrors), so nothing is ever waiting to
// be answered.
func (a *Adapter) Respond(context.Context, agents.SessionHandle, agents.ApprovalResponse) error {
	return fmt.Errorf("answer claude code: %w", agents.ErrUnknownRequest)
}

// Stop ends the session and its process. It is safe to call more than once, and a session that is
// already gone is not an error.
func (a *Adapter) Stop(ctx context.Context, h agents.SessionHandle) error {
	s, err := a.find(h)
	if err != nil {
		return nil
	}
	return s.stop(ctx)
}

// Capabilities says what this adapter can do. Claude Code's streaming JSON mode reports messages
// and tool calls as structured events, lets Marshal pick the model and the effort level, and
// resumes a session by id; it has no permission-request channel yet (see Respond), which is why
// this struct has no field to say so: agents.Capabilities only carries what an agent offers, and
// Approvals is not one of its fields (see the report's Ruling on this).
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{Resume: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, MCP: true}
}

// usable checks that the caller's context is still alive, and returns the running session that a
// handle names. The action says what was being done, for the error.
func (a *Adapter) usable(ctx context.Context, h agents.SessionHandle, action string) (*session, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	return a.find(h)
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

// register records a session that is ready. Two running sessions cannot share an id.
func (a *Adapter) register(id string, s *session) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, taken := a.sessions[id]; taken {
		return fmt.Errorf("session %q is already running", id)
	}
	a.sessions[id] = s
	return nil
}

// forget removes a session that has ended.
func (a *Adapter) forget(id string, s *session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessions[id] == s {
		delete(a.sessions, id)
	}
}

// appliedOf says which of a start spec's settings this adapter always takes: Claude Code's
// --model, --effort, and --permission-mode flags cover all three whenever they are asked for.
func appliedOf(spec agents.StartSpec) agents.Applied {
	return agents.Applied{
		Model: spec.Model != "", Thinking: spec.Thinking != "", PermissionMode: spec.PermissionMode != "",
	}
}

// absoluteCwd checks that a start spec names its working folder as an absolute path, which
// StartSpec.Cwd's own doc comment requires.
func absoluteCwd(spec agents.StartSpec) error {
	if !filepath.IsAbs(spec.Cwd) {
		return fmt.Errorf("the working folder %q must be an absolute path", spec.Cwd)
	}
	return nil
}
