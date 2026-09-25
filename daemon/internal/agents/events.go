package agents

// MaxContentBytes is the most text one event carries from a tool: its output and the text of its
// diffs together. Events stay small because they cross the event bus, and what does not fit is
// cut off with the Truncated flag set. The full output belongs in the session logs.
const MaxContentBytes = 16 << 10

// Values of ToolCall.Status and ToolCallUpdate.Status.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// Values of PlanStep.Status. They are the same words as the tool statuses, except that a plan
// step has no failed state.
const (
	PlanPending    = StatusPending
	PlanInProgress = StatusInProgress
	PlanCompleted  = StatusCompleted
)

// Values of TurnEnded.Reason.
const (
	// TurnEndTurn is a turn that finished normally.
	TurnEndTurn = "end_turn"
	// TurnCancelled is a turn that Interrupt or the agent stopped.
	TurnCancelled = "cancelled"
	// TurnMaxTokens is a turn that ran out of room to answer.
	TurnMaxTokens = "max_tokens"
	// TurnMaxRequests is a turn that hit the agent's limit on model requests.
	TurnMaxRequests = "max_turn_requests"
	// TurnRefusal is a turn where the model declined to continue.
	TurnRefusal = "refusal"
	// TurnError is a turn that failed. A Failed event came just before it.
	TurnError = "error"
)

// Values of PermissionOption.Kind.
const (
	OptionAllowOnce    = "allow_once"
	OptionAllowAlways  = "allow_always"
	OptionRejectOnce   = "reject_once"
	OptionRejectAlways = "reject_always"
)

// AgentEvent is something that happened in a session. It is one of the structs in this file, and
// no other type implements it, so a type switch over them is complete. The events of a session
// arrive in order on one channel. A turn ends with TurnEnded, or with Exited if the process ends
// first.
type AgentEvent interface {
	agentEvent()
}

// MessageChunk is a piece of the agent's answer. Pieces of one answer join into the message.
type MessageChunk struct {
	Text string
}

// ThoughtChunk is a piece of the agent's reasoning, when the agent shares it.
type ThoughtChunk struct {
	Text string
}

// ToolCall announces that the agent started using a tool.
type ToolCall struct {
	// ID names the call inside the session. Later ToolCallUpdate events use it.
	ID    string
	Title string
	// Kind is the agent's word for the kind of tool: read, edit, delete, move, search, execute,
	// think, fetch, switch_mode, or other.
	Kind string
	// Status is one of the Status constants.
	Status string
	// Path is the file the call is about, when it has one.
	Path string
	// Command is the command line of an execute call.
	Command string
	// Content is the text the tool has produced so far, cut to MaxContentBytes.
	Content string
	Diffs   []FileDiff
	// Truncated says that Content or Diffs were cut short.
	Truncated bool
}

// ToolCallUpdate reports progress on a tool call. Empty fields did not change.
type ToolCallUpdate struct {
	ID        string
	Title     string
	Status    string
	Content   string
	Diffs     []FileDiff
	Truncated bool
}

// FileDiff is a change that a tool made to a file. OldText is empty for a new file.
type FileDiff struct {
	Path    string
	OldText string
	NewText string
}

// PlanStep is one line of the agent's plan.
type PlanStep struct {
	Text string
	// Status is one of the Plan constants.
	Status string
}

// PlanUpdate replaces the agent's whole plan.
type PlanUpdate struct {
	Steps []PlanStep
}

// PermissionOption is one answer the user can give to a permission request.
type PermissionOption struct {
	ID   string
	Name string
	// Kind is one of the Option constants.
	Kind string
}

// PermissionRequested says that the agent waits for the user before it goes on. The turn stays
// blocked until Respond is called, or until Interrupt or Stop answers "cancelled".
type PermissionRequested struct {
	// RequestID names this request. Respond needs it.
	RequestID string
	// ToolCallID is the ID of the ToolCall that the request is about, when the agent says.
	ToolCallID string
	Title      string
	Kind       string
	Path       string
	Command    string
	Options    []PermissionOption
}

// TurnEnded says that a turn is over and the agent waits for the next message.
type TurnEnded struct {
	// Reason is one of the Turn constants.
	Reason string
}

// Failed says that the agent broke. Message is a plain sentence, and Detail is for logs and for a
// "show details" fold: it may hold the last lines that the agent printed. If the process is still
// alive, a TurnEnded with the reason "error" follows. If it died, an Exited follows.
type Failed struct {
	Message string
	Detail  string
}

// Exited is the last event. The process is gone and the channel closes after it.
type Exited struct {
	// Code is the exit code, or -1 when the process was killed or ended by a signal.
	Code int
	// Err is nil after a clean exit.
	Err error
}

// TerminalOutput is a piece of what a terminal session printed: raw bytes with the escape
// sequences still in them, for a terminal view to draw. Only the PTY adapter sends it, since
// a terminal has no turns and no tool calls. Data holds at most 16 KiB and is not reused, so the
// receiver may keep it.
type TerminalOutput struct {
	Data []byte
}

func (TerminalOutput) agentEvent()      {}
func (MessageChunk) agentEvent()        {}
func (ThoughtChunk) agentEvent()        {}
func (ToolCall) agentEvent()            {}
func (ToolCallUpdate) agentEvent()      {}
func (PlanUpdate) agentEvent()          {}
func (PermissionRequested) agentEvent() {}
func (TurnEnded) agentEvent()           {}
func (Failed) agentEvent()              {}
func (Exited) agentEvent()              {}
