package builtin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
)

// Adapter runs built-in agent sessions. It implements agents.Agent, and one Adapter can run many
// sessions at once, each with its own history.
type Adapter struct {
	cfg Config
	log *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session
}

var (
	_ agents.Agent           = (*Adapter)(nil)
	_ agents.SettingsApplier = (*Adapter)(nil)
)

// New returns a built-in agent.
func New(cfg Config) (agents.Agent, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	return &Adapter{cfg: cfg, log: cfg.Logger, sessions: make(map[string]*session)}, nil
}

// Factory returns the factory that the session manager registers for the builtin kind.
func Factory(cfg Config) agents.Factory {
	return func() (agents.Agent, error) { return New(cfg) }
}

// Start starts a new session with no history.
func (a *Adapter) Start(ctx context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	return a.open(ctx, spec, "")
}

// Resume starts a session that continues the conversation stored under sessionID. The built-in
// agent has no process holding a session id, so it replays what the daemon persisted
// (Config.History) instead of handing the id anywhere: see the phase report's Resume ruling.
func (a *Adapter) Resume(
	ctx context.Context, sessionID string, spec agents.StartSpec,
) (agents.SessionHandle, error) {
	if sessionID == "" {
		return agents.SessionHandle{}, fmt.Errorf("%w: there is no session id to resume", agents.ErrCannotResume)
	}
	return a.open(ctx, spec, sessionID)
}

// open builds and registers a session, resolving the model into a provider first.
func (a *Adapter) open(ctx context.Context, spec agents.StartSpec, resumeID string) (agents.SessionHandle, error) {
	if err := ctx.Err(); err != nil {
		return agents.SessionHandle{}, err
	}
	resolved, err := a.cfg.Resolver.Resolve(spec.Model)
	if err != nil {
		return agents.SessionHandle{}, fmt.Errorf("find a provider for the model %q: %w", spec.Model, err)
	}
	history, err := a.historyOf(resumeID)
	if err != nil {
		return agents.SessionHandle{}, err
	}
	life, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := newSession(a, spec, resolved, history, life, cancel)
	s.resumed = resumeID != ""
	if err := a.register(s.id, s); err != nil {
		cancel()
		return agents.SessionHandle{}, err
	}
	a.log.Info("built-in agent session started", "provider", resolved.ProviderID, "model", resolved.Model, "resumed", resumeID != "")
	return agents.SessionHandle{
		ID: s.id, Label: spec.Label, Model: spec.Model, Thinking: spec.Thinking, PermissionMode: spec.PermissionMode,
		Applied: appliedOf(spec), Capabilities: a.Capabilities(),
	}, nil
}

// historyOf returns the stored conversation for a resume, or nothing for a fresh start. A history
// hook that fails is not fatal: the session starts with what is known.
func (a *Adapter) historyOf(resumeID string) ([]providers.Message, error) {
	if resumeID == "" || a.cfg.History == nil {
		return nil, nil
	}
	history, err := a.cfg.History(resumeID)
	if err != nil {
		a.log.Warn("could not read a session's history to resume it", "session_id", resumeID, "err", err)
		return nil, nil
	}
	return history, nil
}

// Send starts a turn and returns at once. It returns agents.ErrBusy while a turn is running.
func (a *Adapter) Send(_ context.Context, h agents.SessionHandle, msg agents.UserMessage) error {
	s, err := a.find(h)
	if err != nil {
		return err
	}
	ctx, err := s.takeTurn()
	if err != nil {
		return err
	}
	instructions := ""
	if !s.resumed {
		instructions = s.instructions
	}
	go s.runTurn(ctx, instructions, msg.Text)
	return nil
}

// Interrupt stops the running turn and keeps the session. The turn ends with TurnEnded and the
// reason "cancelled". It does nothing when no turn runs.
func (a *Adapter) Interrupt(_ context.Context, h agents.SessionHandle) error {
	s, err := a.find(h)
	if err != nil {
		return err
	}
	return s.interrupt()
}

// Events returns the session's event channel, closed after the Exited event. For a session that is
// not running it returns a channel that is already closed and empty.
func (a *Adapter) Events(h agents.SessionHandle) <-chan agents.AgentEvent {
	s, err := a.find(h)
	if err != nil {
		closed := make(chan agents.AgentEvent)
		close(closed)
		return closed
	}
	return s.sink.C()
}

// Respond answers a PermissionRequested event.
func (a *Adapter) Respond(_ context.Context, h agents.SessionHandle, r agents.ApprovalResponse) error {
	s, err := a.find(h)
	if err != nil {
		return err
	}
	return s.respond(r)
}

// Stop ends the session. It is safe to call more than once, and a session that is already gone is
// not an error.
func (a *Adapter) Stop(ctx context.Context, h agents.SessionHandle) error {
	s, err := a.find(h)
	if err != nil {
		return nil
	}
	return s.stop(ctx)
}

// Capabilities says what the built-in agent can do. It reports structured events, model and
// thinking switching, and approvals (it asks through PermissionRequested). Resume and LoadSession
// are both true: a session is picked back up by replaying its stored conversation. It takes no MCP
// servers yet.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{
		Resume: true, LoadSession: true, StructuredEvents: true, ModelSwitching: true, Thinking: true,
		MCP: false,
	}
}

// ApplySettings changes a running session's settings, so a change a person makes while the session
// is live is in force for the turn that follows it. A setting the agent cannot take is reported as
// not applied rather than failing the caller.
func (a *Adapter) ApplySettings(
	_ context.Context, h agents.SessionHandle, settings agents.SessionSettings,
) (agents.Applied, error) {
	s, err := a.find(h)
	if err != nil {
		return agents.Applied{}, err
	}
	return s.applySettings(settings), nil
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

// appliedOf says which of a start spec's settings this adapter takes: all three, whenever they are
// asked for. The provider's own default stands in for an empty one.
func appliedOf(spec agents.StartSpec) agents.Applied {
	return agents.Applied{
		Model: spec.Model != "", Thinking: spec.Thinking != "", PermissionMode: spec.PermissionMode != "",
	}
}

// newSessionID makes a fresh session id. It is Marshal's own id; nothing outside needs to
// understand it.
func newSessionID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "builtin"
	}
	return hex.EncodeToString(buf)
}

// validPermissionMode reports whether a permission mode is one the daemon knows.
func validPermissionMode(mode string) bool {
	return mode == "" || protocol.PermissionMode(mode).Valid()
}
