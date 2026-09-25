package acp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// Adapter runs ACP agent sessions. It implements agents.Agent, and one Adapter can run many
// sessions, each with its own process.
type Adapter struct {
	cfg Config
	log *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session
	caps     agents.Capabilities
}

var _ agents.Agent = (*Adapter)(nil)

// New returns an adapter for the agent program in cfg. It does not start anything.
func New(cfg Config) (*Adapter, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	return &Adapter{
		cfg:      cfg,
		log:      cfg.Logger,
		sessions: make(map[string]*session),
		// What every ACP agent gives: events with structure. The rest is learned from a session.
		caps: agents.Capabilities{StructuredEvents: true},
	}, nil
}

// Start starts a new agent process and a session in it.
func (a *Adapter) Start(ctx context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	return a.open(ctx, spec, "")
}

// Resume starts a new agent process and picks up the session with the given id. It returns
// agents.ErrCannotResume when the agent cannot do that.
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

// Interrupt stops the running turn and keeps the session.
func (a *Adapter) Interrupt(ctx context.Context, h agents.SessionHandle) error {
	s, err := a.find(h)
	if err != nil {
		return err
	}
	return s.interrupt(ctx)
}

// Events returns the session's event channel, which is closed after the Exited event. Take it
// once, right after Start or Resume. For a session that is not running it returns a channel that
// is already closed and empty.
func (a *Adapter) Events(h agents.SessionHandle) <-chan agents.AgentEvent {
	s, err := a.find(h)
	if err != nil {
		closed := make(chan agents.AgentEvent)
		close(closed)
		return closed
	}
	return s.sink.C()
}

// Respond answers a permission request.
func (a *Adapter) Respond(ctx context.Context, h agents.SessionHandle, r agents.ApprovalResponse) error {
	s, err := a.usable(ctx, h, "answer a permission request")
	if err != nil {
		return err
	}
	return s.respond(r)
}

// Stop ends the session and its process. It is safe to call more than once, and a session that is
// already gone is not an error.
func (a *Adapter) Stop(ctx context.Context, h agents.SessionHandle) error {
	s, err := a.find(h)
	if err != nil {
		// A session that is not running is already stopped.
		return nil
	}
	return s.stop(ctx)
}

// Capabilities describes the agent as the latest session found it.
func (a *Adapter) Capabilities() agents.Capabilities {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.caps
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

// register records a session that is ready. Two running sessions cannot share an id, because the
// agent's state for one id would be shared by two processes.
func (a *Adapter) register(id string, s *session, caps agents.Capabilities) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, taken := a.sessions[id]; taken {
		return fmt.Errorf("session %q is already running", id)
	}
	a.sessions[id] = s
	a.caps = caps
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
