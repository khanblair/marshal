package protocol

// The payloads (Event.Data) of the events the session manager (internal/session) publishes.
// Modeled closely on the closed set of agents.AgentEvent (internal/agents/events.go), so the
// mapping from a Go event to a wire event stays close to mechanical.
//
// Two wire event types cover the five kinds of agent event that carry content:
//
//   - session.output carries a message chunk, a thought chunk, or a plan update: text the chat
//     view streams in as it arrives.
//   - session.tool_call carries a tool call starting or an update to one already running: the
//     chat view renders these as their own blocks (docs/architecture.md section 2.2, S8a), not as
//     text, which is why they are not folded into session.output even though every other kind of
//     output is. The reserved name and doc comment of EventTypeSessionToolCall (event_types.go)
//     already say "sent when an agent calls a tool", not "sent as part of session output", so this
//     keeps the two constants doing what their names say.
//
// A project chat's session (docs/backend-checklist.md B2.10) publishes the same three events, with
// the same payloads, on the topic chat:<id> instead of card:<id>. Each carries chatId beside cardId,
// and exactly one of the two is set: cardId is empty in a chat's event, and chatId is left out of a
// card's, so a client tells them apart by which is there.
//
// Neither payload is sent for a PermissionRequested event: that belongs to the approvals flow of
// architecture.md section 11.4 and Phase 3 (B3.4), which does not exist yet, so a permission
// request is only logged for now (see internal/session's report).

// SessionOutputEventData is the payload of session.output: a piece of the agent's answer, a piece
// of its reasoning, or a full replacement of its plan.
type SessionOutputEventData struct {
	// CardID is the card whose session produced this output. It is empty when a chat's did.
	CardID string `json:"cardId"`
	// ChatID is the chat whose session produced this output. It is left out when a card's did.
	ChatID string `json:"chatId,omitempty"`
	// Kind is "message", "thought", or "plan".
	Kind string `json:"kind"`
	// Text is the chunk of text, for "message" and "thought". It is cut with agents.Truncate the
	// same way tool output already is, so one event never carries more than agents.MaxContentBytes:
	// the full text belongs in the session log (docs/architecture.md section 4.1). Empty for "plan".
	Text string `json:"text,omitempty"`
	// Plan is the agent's whole plan, for "plan". Nil for every other kind.
	Plan []PlanStep `json:"plan,omitempty"`
}

// PlanStep is one line of an agent's plan, on the wire.
type PlanStep struct {
	Text string `json:"text"`
	// Status is "pending", "in_progress", or "completed" (agents.PlanStep's own Status constants).
	Status string `json:"status"`
}

// SessionToolCallEventData is the payload of session.tool_call: a tool call starting, or an
// update to one that is already running.
type SessionToolCallEventData struct {
	// CardID is the card whose session made this tool call. It is empty when a chat's did.
	CardID string `json:"cardId"`
	// ChatID is the chat whose session made this tool call. It is left out when a card's did.
	ChatID string `json:"chatId,omitempty"`
	// Kind is "tool_call" or "tool_call_update".
	Kind     string        `json:"kind"`
	ToolCall AgentToolCall `json:"toolCall"`
}

// AgentToolCall is a tool call, or an update to one, on the wire. A field that an update did not
// change is empty (see agents.ToolCallUpdate); Title, ToolKind, Path, and Command are only ever
// set by the call that starts the tool, so an update leaves them out.
type AgentToolCall struct {
	// ID names the call inside the session. An update shares the ID of the call it updates.
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	// ToolKind is the agent's word for the kind of tool: read, edit, delete, move, search,
	// execute, think, fetch, switch_mode, or other. Only set when the call starts.
	ToolKind string `json:"toolKind,omitempty"`
	// Status is "pending", "in_progress", "completed", or "failed" (agents.StatusPending and its
	// siblings).
	Status string `json:"status,omitempty"`
	// Path is the file the call is about, when it has one. Only set when the call starts.
	Path string `json:"path,omitempty"`
	// Command is the command line of an execute call. Only set when the call starts.
	Command string `json:"command,omitempty"`
	// Content is the text the tool has produced so far, cut to agents.MaxContentBytes.
	Content string     `json:"content,omitempty"`
	Diffs   []FileDiff `json:"diffs,omitempty"`
	// Truncated says that Content or Diffs were cut short. The full text belongs in the session log.
	Truncated bool `json:"truncated,omitempty"`
}

// FileDiff is a change a tool made to a file, on the wire. OldText is empty for a new file.
type FileDiff struct {
	Path    string `json:"path"`
	OldText string `json:"oldText,omitempty"`
	NewText string `json:"newText,omitempty"`
}

// SessionStateChangedEventData is the payload of session.state_changed.
type SessionStateChangedEventData struct {
	// CardID is the card the session belongs to. It is empty for a chat's session.
	CardID string `json:"cardId"`
	// ChatID is the chat the session belongs to. It is left out for a card's session.
	ChatID string `json:"chatId,omitempty"`
	// SessionID is the session's own opaque id (not the agent's session id).
	SessionID string `json:"sessionId"`
	// State is the state the session moved to.
	State SessionState `json:"state"`
	// Reason is a plain sentence, set when the move needs an explanation (for example, a resume
	// that failed). Empty for an ordinary move such as a turn ending.
	Reason string `json:"reason,omitempty"`
}
