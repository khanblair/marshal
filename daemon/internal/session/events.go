package session

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// LogEntry is one recorded moment of a session: an agent event, with when it happened and its
// kind. RecentOutput returns these for a card, for a later "what happened before I opened this
// card" API route (none calls it yet in Phase 1).
type LogEntry struct {
	At    time.Time
	Kind  string
	Event agents.AgentEvent
}

// eventKind names an agents.AgentEvent for the "kind" field of a log line and a LogEntry. The
// agents.AgentEvent set is closed (its own doc comment says so), so every case here is exhaustive
// except the default, kept only in case the agents package ever grows a kind this code has not
// been taught yet.
func eventKind(ev agents.AgentEvent) string {
	switch ev.(type) {
	case agents.MessageChunk:
		return "message"
	case agents.ThoughtChunk:
		return "thought"
	case agents.ToolCall:
		return "tool_call"
	case agents.ToolCallUpdate:
		return "tool_call_update"
	case agents.PlanUpdate:
		return "plan"
	case agents.PermissionRequested:
		return "permission_requested"
	case agents.TurnEnded:
		return "turn_ended"
	case agents.Failed:
		return "failed"
	case agents.Exited:
		return "exited"
	case agents.TerminalOutput:
		return "terminal_output"
	default:
		return "unknown"
	}
}

// logCommon is embedded in every typed log-line struct, so every line carries "at" and "kind"
// before its own fields.
type logCommon struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
}

type logText struct {
	logCommon
	Text string `json:"text"`
}

type logFileDiff struct {
	Path    string `json:"path"`
	OldText string `json:"oldText,omitempty"`
	NewText string `json:"newText,omitempty"`
}

func logDiffsOf(diffs []agents.FileDiff) []logFileDiff {
	if len(diffs) == 0 {
		return nil
	}
	out := make([]logFileDiff, len(diffs))
	for i, d := range diffs {
		out[i] = logFileDiff{Path: d.Path, OldText: d.OldText, NewText: d.NewText}
	}
	return out
}

type logToolCall struct {
	logCommon
	ID        string        `json:"id"`
	Title     string        `json:"title,omitempty"`
	ToolKind  string        `json:"toolKind,omitempty"`
	Status    string        `json:"status,omitempty"`
	Path      string        `json:"path,omitempty"`
	Command   string        `json:"command,omitempty"`
	Content   string        `json:"content,omitempty"`
	Diffs     []logFileDiff `json:"diffs,omitempty"`
	Truncated bool          `json:"truncated,omitempty"`
}

type logToolCallUpdate struct {
	logCommon
	ID        string        `json:"id"`
	Title     string        `json:"title,omitempty"`
	Status    string        `json:"status,omitempty"`
	Content   string        `json:"content,omitempty"`
	Diffs     []logFileDiff `json:"diffs,omitempty"`
	Truncated bool          `json:"truncated,omitempty"`
}

type logPlanStep struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

type logPlan struct {
	logCommon
	Steps []logPlanStep `json:"steps"`
}

type logPermissionRequested struct {
	logCommon
	RequestID string `json:"requestId"`
	Title     string `json:"title,omitempty"`
	// RequestKind is agents.PermissionRequested.Kind. It cannot be named Kind: that would shadow
	// logCommon.Kind, the log line's own "permission_requested" (a JSON field is written once, at
	// the shallowest depth that has it, and this struct embeds logCommon).
	RequestKind string `json:"requestKind,omitempty"`
	Path        string `json:"path,omitempty"`
	Command     string `json:"command,omitempty"`
}

type logTurnEnded struct {
	logCommon
	Reason string `json:"reason"`
}

type logFailed struct {
	logCommon
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// logTerminalOutput is a piece of what a terminal printed. The bytes are raw, so they are written as
// base64 (the way encoding/json writes a byte slice), not as text that could split a character.
type logTerminalOutput struct {
	logCommon
	Data []byte `json:"data"`
}

type logExited struct {
	logCommon
	Code int    `json:"code"`
	Err  string `json:"err,omitempty"`
}

// marshalLogLine builds one JSON-lines line for ev: a typed struct per kind, not a generic map, so
// a line is never malformed (Kind names the struct, and its own fields cannot fail to match it).
func marshalLogLine(at time.Time, ev agents.AgentEvent) ([]byte, error) {
	common := logCommon{At: at.UTC(), Kind: eventKind(ev)}
	line, err := json.Marshal(logValue(common, ev))
	if err != nil {
		return nil, fmt.Errorf("encode a session log line: %w", err)
	}
	return append(line, '\n'), nil
}

// logValue builds the typed value marshalLogLine encodes, split out to keep marshalLogLine short.
//
//nolint:cyclop // one line per closed AgentEvent kind
func logValue(common logCommon, ev agents.AgentEvent) any {
	switch e := ev.(type) {
	case agents.MessageChunk:
		return logText{common, e.Text}
	case agents.ThoughtChunk:
		return logText{common, e.Text}
	case agents.ToolCall:
		return logToolCall{
			common, e.ID, e.Title, e.Kind, e.Status, e.Path, e.Command, e.Content, logDiffsOf(e.Diffs), e.Truncated,
		}
	case agents.ToolCallUpdate:
		return logToolCallUpdate{common, e.ID, e.Title, e.Status, e.Content, logDiffsOf(e.Diffs), e.Truncated}
	case agents.PlanUpdate:
		steps := make([]logPlanStep, len(e.Steps))
		for i, s := range e.Steps {
			steps[i] = logPlanStep{Text: s.Text, Status: s.Status}
		}
		return logPlan{common, steps}
	case agents.PermissionRequested:
		return logPermissionRequested{common, e.RequestID, e.Title, e.Kind, e.Path, e.Command}
	case agents.TurnEnded:
		return logTurnEnded{common, e.Reason}
	case agents.Failed:
		return logFailed{common, e.Message, e.Detail}
	case agents.Exited:
		return logExited{common, e.Code, errText(e.Err)}
	case agents.TerminalOutput:
		return logTerminalOutput{common, e.Data}
	default:
		return common
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// isStateChanging says whether ev should flush the session log at once instead of waiting for the
// flush timer: TurnEnded and Exited are the moments after which a crash should not be allowed to
// lose the last second of chat text.
func isStateChanging(ev agents.AgentEvent) bool {
	switch ev.(type) {
	case agents.TurnEnded, agents.Exited:
		return true
	default:
		return false
	}
}

// wirePlanSteps converts agents.PlanStep values to the wire type.
func wirePlanSteps(steps []agents.PlanStep) []protocol.PlanStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]protocol.PlanStep, len(steps))
	for i, s := range steps {
		out[i] = protocol.PlanStep{Text: s.Text, Status: s.Status}
	}
	return out
}

// wireFileDiffs converts agents.FileDiff values to the wire type.
func wireFileDiffs(diffs []agents.FileDiff) []protocol.FileDiff {
	if len(diffs) == 0 {
		return nil
	}
	out := make([]protocol.FileDiff, len(diffs))
	for i, d := range diffs {
		out[i] = protocol.FileDiff{Path: d.Path, OldText: d.OldText, NewText: d.NewText}
	}
	return out
}

// wireToolCall converts a starting tool call to the wire type.
func wireToolCall(e agents.ToolCall) protocol.AgentToolCall {
	return protocol.AgentToolCall{
		ID: e.ID, Title: e.Title, ToolKind: e.Kind, Status: e.Status, Path: e.Path, Command: e.Command,
		Content: e.Content, Diffs: wireFileDiffs(e.Diffs), Truncated: e.Truncated,
	}
}

// wireToolCallUpdate converts a tool call update to the wire type.
func wireToolCallUpdate(e agents.ToolCallUpdate) protocol.AgentToolCall {
	return protocol.AgentToolCall{
		ID: e.ID, Title: e.Title, Status: e.Status, Content: e.Content,
		Diffs: wireFileDiffs(e.Diffs), Truncated: e.Truncated,
	}
}
