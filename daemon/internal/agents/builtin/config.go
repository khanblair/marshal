// Package builtin is Marshal's own coding agent: the one that talks to a model provider API
// directly, with the owner's own key, instead of shelling out to a CLI the way agents/claude and
// agents/gemini do. It implements the same agents.Agent interface, so the session manager, the
// event bus, and the chat view need to know nothing about it beyond its kind.
//
// It is a read/edit/run/tools loop: one turn is one call to the provider, the model's streamed text
// and tool calls become the same events the CLI adapters emit, each tool call is checked against the
// permission harness before it runs, the results go back to the model, and the loop repeats until
// the model answers without asking for a tool (or a limit stops it).
//
// Two rules shape it. Every provider call goes through providers.Client, which is where the queue,
// retries, usage, and fallback join later; this package never touches a provider SDK. And every tool
// call goes through harness.Decide before it runs, so the permission mode, the profile, the command
// blocklist, and the card's worktree rule hold for Marshal's own agent exactly as they hold for a
// CLI one.
package builtin

import (
	"errors"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/security"
)

const (
	// defaultMaxTurns bounds how many model calls one turn may make, so a model that asks for a
	// tool forever is stopped. It is reported as TurnMaxRequests, the same reason a CLI agent
	// uses when it hits its own turn limit.
	defaultMaxTurns = 40
	// defaultMaxTokens caps how much the model may answer with in one call.
	defaultMaxTokens = 8192
	// defaultCommandTimeout bounds one run_command call.
	defaultCommandTimeout = 120 * time.Second
	// defaultToolOutputBytes is how much of a tool's own output is kept before it is cut. It is
	// larger than agents.MaxContentBytes because the model needs more of it than the chat view
	// shows; the event is cut to MaxContentBytes afterwards.
	defaultToolOutputBytes = 64 << 10
	// defaultSystemPrompt is the base instruction every built-in session starts with, before the
	// role instructions a card carries.
	defaultSystemPrompt = "You are Marshal's built-in coding agent. You work inside one card's " +
		"worktree. Use the tools you are given to read, change, and run what the task needs. " +
		"Prefer the smallest change that works, and say what you did when you are done."
)

// Resolved is one provider and model a session has been pointed at, with the client that talks to
// it. The client is what the session calls; the ids are what it reports in usage rows.
//
// It is an alias, not a second struct with the same fields: providers.Resolved is the same thing
// seen from the provider service's side, and the two must stay identical for the service to satisfy
// Resolver at all. Aliasing makes that a fact the compiler checks instead of a habit someone
// remembers, so the key store the screens write is the resolver the agent reads with nothing in
// between.
type Resolved = providers.Resolved

// Resolver turns a model id from a card into the provider and model to call. The session manager
// wires the real one over the stored keys; a test gives its own.
type Resolver interface {
	// Resolve returns the provider and model for a model id. An empty model id resolves to the
	// built-in agent's default model.
	Resolve(model string) (Resolved, error)
}

// History returns the conversation a session already had, oldest first, so Resume can continue it
// instead of starting from nothing. The session manager wires this to the events the daemon already
// persists (session_events); a test gives its own, or none.
//
// This is how the built-in agent satisfies Resume: it has no external process holding a session id,
// so it replays what the daemon recorded rather than passing an id anywhere.
type History func(sessionID string) ([]providers.Message, error)

// Config says how a built-in agent runs. Resolver is required.
type Config struct {
	// Resolver turns a model id into a provider and model. Required.
	Resolver Resolver
	// History returns a resumed session's earlier conversation. Nil means a resumed session
	// starts with no history, which is what happens when nothing has been persisted yet.
	History History
	// System is added in front of every session's system prompt. Empty uses the package default.
	System string
	// Profile is what the built-in agent may do at all, whatever its permission mode. The zero
	// value refuses everything the harness covers, so a caller that wants the shipped behaviour
	// passes security.DefaultProfile().
	Profile security.Profile
	// Blocklist is the command blocklist. A nil list has nothing to say.
	Blocklist *security.Blocklist
	// Containment builds the worktree rule for a session's working folder. Nil uses a rule that
	// keeps the session inside its own folder and says nothing about branches, which is what a
	// chat with no worktree needs.
	Containment func(cwd string) gitx.Containment
	// MaxTurns bounds the model calls in one turn. Zero uses the default.
	MaxTurns int
	// MaxTokens caps one model answer. Zero uses the default.
	MaxTokens int64
	// CommandTimeout bounds one run_command call. Zero uses the default.
	CommandTimeout time.Duration
	// ToolOutputBytes is how much of a tool's own output is kept. Zero uses the default.
	ToolOutputBytes int
	// Logger receives the agent's log lines. Nil uses slog.Default().
	Logger *slog.Logger
}

// withDefaults checks the config and fills in what was left out.
func (c Config) withDefaults() (Config, error) {
	if c.Resolver == nil {
		return Config{}, errors.New("builtin agent: a provider resolver is required")
	}
	if c.System == "" {
		c.System = defaultSystemPrompt
	}
	if c.Blocklist == nil {
		c.Blocklist = security.DefaultBlocklist()
	}
	if c.Containment == nil {
		c.Containment = func(cwd string) gitx.Containment { return gitx.NewContainment(cwd, cwd, "", "") }
	}
	if c.MaxTurns <= 0 {
		c.MaxTurns = defaultMaxTurns
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = defaultMaxTokens
	}
	if c.CommandTimeout <= 0 {
		c.CommandTimeout = defaultCommandTimeout
	}
	if c.ToolOutputBytes <= 0 {
		c.ToolOutputBytes = defaultToolOutputBytes
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c, nil
}
