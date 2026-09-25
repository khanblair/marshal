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
	// Env is added to the agent's environment as KEY=value entries, for example a provider key.
	// The agent gets nothing else from the daemon's environment beyond the short list that
	// internal/proc lets through.
	Env []string
	// Label is a name the caller picks for logs and errors, for example the card id. It is never
	// interpreted.
	Label string
}

// UserMessage is what the user says to the agent.
type UserMessage struct {
	Text string
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
