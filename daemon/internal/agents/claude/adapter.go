package claude

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// Adapter runs Claude Code sessions. It implements agents.Agent, and one Adapter can run many
// sessions at once, each with its own process.
type Adapter struct {
	cfg Config
	log *slog.Logger

	sessions *agents.SessionRegistry[session]
}

var _ agents.Agent = (*Adapter)(nil)

// New returns an adapter for the claude program at cfg.Path. It starts nothing.
func New(cfg Config) (agents.Agent, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	return &Adapter{cfg: cfg, log: cfg.Logger, sessions: agents.NewSessionRegistry[session]()}, nil
}

// Factory returns the factory that the session manager registers for the claude kind.
func Factory(cfg Config) agents.Factory {
	return func() (agents.Agent, error) { return New(cfg) }
}

// Start starts a new Claude Code process and a session in it.
func (a *Adapter) Start(ctx context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	return a.open(ctx, spec, "", "")
}

// Resume starts a new Claude Code process with --resume and picks up the session with the given
// id.
func (a *Adapter) Resume(
	ctx context.Context, sessionID string, spec agents.StartSpec,
) (agents.SessionHandle, error) {
	if sessionID == "" {
		return agents.SessionHandle{}, fmt.Errorf("%w: there is no session id to resume", agents.ErrCannotResume)
	}
	if !looksLikeUUID(sessionID) {
		return agents.SessionHandle{}, notASessionID(sessionID)
	}
	// Claude Code only saves a conversation once it has a first message. A session that was
	// started and never spoken to has nothing to resume, so it starts again under its own id. The
	// file is looked for first because Claude Code's own "no conversation" answer can come after
	// the process has already counted as started.
	if !Saved(sessionID) {
		return a.startAgain(ctx, spec, sessionID)
	}
	h, err := a.open(ctx, spec, sessionID, "")
	if err != nil && strings.Contains(err.Error(), noConversation) {
		return a.startAgain(ctx, spec, sessionID)
	}
	return h, err
}

// startAgain begins a new conversation under a session id that has none saved.
func (a *Adapter) startAgain(ctx context.Context, spec agents.StartSpec, sessionID string) (agents.SessionHandle, error) {
	a.log.Info("no saved conversation to resume, starting the session again", "session_id", sessionID)
	h, err := a.open(ctx, spec, "", sessionID)
	h.Restarted = err == nil
	return h, err
}

// noConversation is what Claude Code prints when --resume names a session it never saved.
const noConversation = "No conversation found"

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
	return a.sessions.Find(h)
}

// register records a session that is ready. Two running sessions cannot share an id.
func (a *Adapter) register(id string, s *session) error {
	return a.sessions.Register(id, s)
}

// forget removes a session that has ended.
func (a *Adapter) forget(id string, s *session) {
	a.sessions.Forget(id, s)
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
