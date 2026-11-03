// Package codex adapts Codex's JSON-RPC app-server to agents.Agent.
package codex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

const (
	defaultRequestTimeout = 60 * time.Second
	defaultStopGrace      = 5 * time.Second
)

// Config controls the Codex app-server executable and its process lifetime.
type Config struct {
	// Path is the absolute path to the codex executable.
	Path string
	// Env is added after the filtered program environment.
	Env []string
	// Logger receives adapter log output.
	Logger *slog.Logger
	// RequestTimeout bounds RPC startup and turn-start requests; zero uses 60 seconds.
	RequestTimeout time.Duration
	// StopGrace bounds graceful process shutdown; zero uses five seconds.
	StopGrace time.Duration
}

// Adapter runs one app-server process for each Codex session.
type Adapter struct {
	cfg      Config
	sessions *agents.SessionRegistry[session]
}

var _ agents.Agent = (*Adapter)(nil)

// New returns a Codex app-server adapter without starting a process.
func New(cfg Config) (agents.Agent, error) {
	if cfg.Path == "" {
		return nil, errors.New("codex adapter: the path of the codex program is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	if cfg.StopGrace <= 0 {
		cfg.StopGrace = defaultStopGrace
	}
	return &Adapter{cfg: cfg, sessions: agents.NewSessionRegistry[session]()}, nil
}

// Factory returns the registry factory for a Codex adapter.
func Factory(cfg Config) agents.Factory { return func() (agents.Agent, error) { return New(cfg) } }

// Start creates a new Codex thread and its long-lived app-server process.
func (a *Adapter) Start(ctx context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	return a.open(ctx, spec, "")
}

// Resume starts an app-server process attached to the requested thread.
func (a *Adapter) Resume(ctx context.Context, id string, spec agents.StartSpec) (agents.SessionHandle, error) {
	if id == "" {
		return agents.SessionHandle{}, fmt.Errorf("%w: missing thread id", agents.ErrCannotResume)
	}
	return a.open(ctx, spec, id)
}
func (a *Adapter) open(ctx context.Context, spec agents.StartSpec, resume string) (agents.SessionHandle, error) {
	if err := ctx.Err(); err != nil {
		return agents.SessionHandle{}, err
	}
	if !filepath.IsAbs(spec.Cwd) {
		return agents.SessionHandle{}, fmt.Errorf("working folder %q must be absolute", spec.Cwd)
	}
	threadSettings, configSettings, err := settings(spec)
	if err != nil {
		return agents.SessionHandle{}, err
	}
	s, err := newSession(ctx, a, spec, threadSettings, configSettings, resume)
	if err != nil {
		return agents.SessionHandle{}, err
	}
	if err = a.sessions.Register(s.threadID(), s); err != nil {
		_ = s.stop(context.Background())
		return agents.SessionHandle{}, err
	}
	select {
	case <-s.done:
		a.forget(s)
		return agents.SessionHandle{}, errors.New("codex app-server exited while the session was starting")
	default:
	}
	return s.handle(), nil
}

// Send starts a turn on the session's existing thread.
func (a *Adapter) Send(ctx context.Context, h agents.SessionHandle, msg agents.UserMessage) error {
	s, e := a.usable(ctx, h)
	if e != nil {
		return e
	}
	return s.send(ctx, msg)
}

// Interrupt stops the current turn while keeping the Codex thread available.
func (a *Adapter) Interrupt(ctx context.Context, h agents.SessionHandle) error {
	s, e := a.find(h)
	if e != nil {
		return e
	}
	return s.interrupt(ctx)
}

// Events returns the channel carrying structured events for a session.
func (a *Adapter) Events(h agents.SessionHandle) <-chan agents.AgentEvent {
	s, e := a.find(h)
	if e != nil {
		c := make(chan agents.AgentEvent)
		close(c)
		return c
	}
	return s.sink.C()
}

// Respond answers an outstanding Codex approval request.
func (a *Adapter) Respond(ctx context.Context, h agents.SessionHandle, r agents.ApprovalResponse) error {
	s, e := a.usable(ctx, h)
	if e != nil {
		return e
	}
	return s.respond(ctx, r)
}

// Stop ends the app-server process and closes the session event stream.
func (a *Adapter) Stop(ctx context.Context, h agents.SessionHandle) error {
	s, e := a.find(h)
	if e != nil {
		return nil
	}
	return s.stop(ctx)
}

// Capabilities reports the settings and structured events supported by Codex.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{Resume: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, MCP: true}
}
func (a *Adapter) find(h agents.SessionHandle) (*session, error) { return a.sessions.Find(h) }
func (a *Adapter) usable(ctx context.Context, h agents.SessionHandle) (*session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.find(h)
}
func (a *Adapter) forget(s *session) {
	a.sessions.Forget(s.threadID(), s)
}
