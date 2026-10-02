// Package agents defines what every kind of coding agent looks like to the rest of the daemon: one
// interface, one small set of events, and a registry that picks an implementation by kind. The
// adapters (agents/acp, later agents/pty and agents/builtin) live in the packages below it.
package agents

import "context"

// Agent is one kind of coding agent. A single Agent value can run many sessions at once, and each
// session is named by the SessionHandle that Start or Resume returned. All methods are safe to
// call from several goroutines.
type Agent interface {
	// Start starts a new session in a new process. The context only bounds the start: the
	// session lives on after Start returns, until Stop or until its process ends.
	Start(ctx context.Context, spec StartSpec) (SessionHandle, error)
	// Resume starts a new process and picks up the session that an earlier process had. It
	// returns ErrCannotResume when the agent cannot do that, and the caller then follows the
	// restore rules in architecture section 5.3.
	Resume(ctx context.Context, sessionID string, spec StartSpec) (SessionHandle, error)
	// Send starts one turn and returns at once. What happens next arrives on Events, and the
	// turn ends with a TurnEnded event, except for an agent whose Capabilities says
	// StructuredEvents is false (the PTY adapter), which never sends one and never returns
	// ErrBusy either. It returns ErrBusy while a turn is running, and the caller queues the
	// message.
	Send(ctx context.Context, h SessionHandle, msg UserMessage) error
	// Interrupt stops the running turn and keeps the session, so the next Send continues it.
	// The turn ends with TurnEnded and the reason "cancelled". It does nothing when no turn runs.
	Interrupt(ctx context.Context, h SessionHandle) error
	// Events returns the session's event channel. Take it once, right after Start or Resume, and
	// treat its closing as the end of the session. While the session runs it is the same
	// channel on every call, and it is closed after the last event, which is Exited. The caller
	// must keep reading until then, because the agent waits when the channel is full, and after
	// Stop the Exited event is only certain to arrive if the reader keeps reading. For a session
	// that is not running, whether it ended by Stop or by a crash, it returns a channel that is
	// already closed and holds no events, so a caller that asks late learns nothing about how
	// the session ended.
	Events(h SessionHandle) <-chan AgentEvent
	// Respond answers a PermissionRequested event.
	Respond(ctx context.Context, h SessionHandle, r ApprovalResponse) error
	// Stop ends the session and its process, and everything the process started. It is safe to
	// call more than once. The event channel receives Exited and is closed.
	Stop(ctx context.Context, h SessionHandle) error
	// Capabilities says what the agent can do, as far as it is known. It takes no handle, so it
	// describes the latest session the agent has started. Before the first session it holds the
	// values that every ACP agent has.
	Capabilities() Capabilities
}

// StartSpec says how to start or resume a session.
type StartSpec struct {
	// Cwd is the working folder of the agent, as an absolute path. It is normally a card's
	// worktree.
	Cwd string
	// Model is the model to use. Empty means the agent's own default.
	Model string
	// Thinking is the value of a protocol.ThinkingMode. Empty means the agent's own default.
	Thinking string
	// PermissionMode is the value of a protocol.PermissionMode. Empty means the agent's own
	// default.
	PermissionMode string
	// Instructions are the role instructions. They go to the agent with the first message of a
	// new session, and are not sent again when a session is resumed.
	Instructions string
	// MCPServers are the MCP servers this session is given, each served over stdio: the agent runs
	// the command itself. Marshal's internal server (docs/architecture.md section 11.4) is the one
	// every card's session gets. An agent that accepts no servers, or an adapter whose protocol
	// carries them another way, ignores this list; the request that carries it is built by the
	// adapter, not here.
	MCPServers []MCPServer
	// AllowedTools and DisallowedTools are tool rules the agent program enforces itself, written in
	// that program's own syntax (Claude Code's, "Bash(git push:*)"). A disallowed rule wins over an
	// allowed one and over the permission mode. An adapter whose program has no such rules ignores
	// them, so they are a second line behind the harness and never the only one.
	AllowedTools    []string
	DisallowedTools []string
	// Env is added to the agent's environment as KEY=value entries, for example a provider key.
	// The agent gets nothing else from the daemon's environment beyond the short list that
	// internal/proc lets through.
	Env []string
	// Label is a name the caller picks for logs and errors, for example the card id. It is never
	// interpreted.
	Label string
}

// MCPServer is one MCP server a session is given, served over stdio: the agent starts Command
// itself and speaks the protocol over the process's own standard input and output.
//
// It is deliberately not the ACP SDK's own server type. The agents package describes what a session
// is given in its own terms, and each adapter turns that into whatever its protocol carries - the
// ACP adapter into an sdk.McpServer, and a future adapter into its own shape - so a change in the
// SDK does not reach this package or the daemon above it.
type MCPServer struct {
	// Name is the server's name, as the agent knows it, for example "marshal".
	Name string
	// Command is the program to run, as an absolute path where the caller can give one.
	Command string
	// Args are the arguments that program is given, in order.
	Args []string
	// Env is the environment the program is run in, as KEY=value entries, added to the agent's
	// own. It is how a server is told which session it speaks for.
	Env []string
}

// UserMessage is what the user says to the agent.
type UserMessage struct {
	Text string
}

// SessionSettings are the settings a session can be given without starting another process: the
// ones a card's settings row offers (docs/ui-rules.md 3.4). Each field holds the same value and
// means the same thing as the field of the same name in StartSpec.
type SessionSettings struct {
	// Model is the model to use. Empty means the agent's own default.
	Model string
	// Thinking is the value of a protocol.ThinkingMode. Empty means the agent's own default.
	Thinking string
	// PermissionMode is the value of a protocol.PermissionMode. Empty means the agent's own
	// default.
	PermissionMode string
}

// SettingsApplier is the optional half of an Agent: an agent that can change the settings of a
// session that is already running, so a change a person makes while the session is live is in
// force for the turn that follows it, without a restart (docs/backend-checklist.md B3.6, N8).
//
// It is deliberately not part of Agent. An agent whose program only takes these settings on the
// command line has nowhere to put a change: making every adapter carry a method it cannot honour
// would say more than the truth. The session manager asks for this interface and, when the agent
// does not implement it, keeps the session as it started and lets the next Start or Resume read
// the card again - which is where a change takes effect for such an agent.
type SettingsApplier interface {
	// ApplySettings gives the running session the settings asked for, and reports which of them
	// the agent took, the way Start's own Applied does. A field that is empty is left alone. An
	// agent that is not running, or does not offer a control for a setting, reports it as not
	// applied rather than failing the caller's turn.
	ApplySettings(ctx context.Context, h SessionHandle, settings SessionSettings) (Applied, error)
}

// ApprovalResponse answers a permission request.
type ApprovalResponse struct {
	// RequestID is the RequestID of the PermissionRequested event.
	RequestID string
	// OptionID is the ID of the PermissionOption the user chose. It is ignored when Cancelled is
	// set.
	OptionID string
	// Cancelled says that the user dismissed the request without choosing.
	Cancelled bool
}

// Capabilities says what an agent can do.
type Capabilities struct {
	// Resume is true when a stopped session can be picked up again by a new process.
	Resume bool
	// LoadSession is true when the agent can replay a saved session, the older way to resume.
	LoadSession bool
	// StructuredEvents is true when the agent reports messages and tool calls as events, rather
	// than as terminal output.
	StructuredEvents bool
	// ModelSwitching is true when the agent lets the client choose the model.
	ModelSwitching bool
	// Thinking is true when the agent lets the client choose how hard it thinks.
	Thinking bool
	// MCP is true when the agent accepts MCP servers from the client.
	MCP bool
}

// SessionHandle names one running session. It is a plain value: keep it, pass it back, and use
// ID to resume the session later.
type SessionHandle struct {
	// ID is the agent's own session id. It is what Resume needs.
	ID string
	// Label is the label from the StartSpec.
	Label string
	// Model, Thinking, and PermissionMode are what the session was asked to run with. Applied
	// says which of them the agent really took: an agent that does not offer a control keeps the
	// value here without acting on it.
	Model          string
	Thinking       string
	PermissionMode string
	Applied        Applied
	// Capabilities is what this session's agent said it can do.
	Capabilities Capabilities
}

// Applied says which requested settings the agent took.
type Applied struct {
	Model          bool
	Thinking       bool
	PermissionMode bool
}
