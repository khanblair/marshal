package claude

import (
	"encoding/json"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// toolCallEvent turns the start of a tool call (an assistant message's tool_use block) into an
// event. Diffs are left empty: Claude Code's tool results are plain text, not the structured
// diffs that the ACP adapter gets from agents that speak that protocol.
func toolCallEvent(b contentBlock) agents.ToolCall {
	path := pathFromInput(b.Input)
	command := commandFromInput(b.Input)
	return agents.ToolCall{
		ID: b.ID, Title: toolTitle(b.Name, path, command), Kind: kindFor(b.Name),
		Status: agents.StatusPending, Path: path, Command: command,
	}
}

// toolResultEvent turns a tool_result block, echoed back on a "user" message, into the update
// that closes out the matching ToolCall.
func toolResultEvent(b contentBlock) agents.ToolCallUpdate {
	text, truncated := agents.Truncate(toolResultText(b.Content), agents.MaxContentBytes)
	status := agents.StatusCompleted
	if b.IsError {
		status = agents.StatusFailed
	}
	return agents.ToolCallUpdate{ID: b.ToolUseID, Status: status, Content: text, Truncated: truncated}
}

// toolTitle is the short label a chat view shows for a tool call: the tool's name, and the file
// or command it names when it names one.
func toolTitle(name, path, command string) string {
	switch {
	case path != "":
		return name + " " + path
	case command != "":
		text, _ := agents.Truncate(command, maxTitleCommandBytes)
		return name + ": " + text
	default:
		return name
	}
}

// maxTitleCommandBytes bounds how much of a command goes into a tool call's title, so a long
// shell one-liner does not dominate the chat view's tool call row.
const maxTitleCommandBytes = 80

// kindFor is a best-effort grouping of Claude Code's built-in tool names into the kinds that
// agents.ToolCall.Kind documents. A tool this adapter does not recognize, including any MCP tool,
// is "other".
func kindFor(name string) string {
	switch name {
	case "Read", "NotebookRead":
		return "read"
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		return "edit"
	case "Bash", "BashOutput", "KillShell":
		return "execute"
	case "Grep", "Glob":
		return "search"
	case "WebFetch":
		return "fetch"
	case "WebSearch":
		return "search"
	default:
		return "other"
	}
}

// inputFields reads a tool_use block's input as a plain map. The input is whatever the model
// sent, so anything that does not decode this way is treated as having no fields.
func inputFields(input json.RawMessage) map[string]any {
	if len(input) == 0 {
		return nil
	}
	var fields map[string]any
	if err := json.Unmarshal(input, &fields); err != nil {
		return nil
	}
	return fields
}

// pathFromInput reads the file a tool call is about, under the first key that Claude Code's
// built-in file tools use for it.
func pathFromInput(input json.RawMessage) string {
	return stringField(inputFields(input), "file_path", "path", "notebook_path")
}

// commandFromInput reads the command line of a Bash tool call.
func commandFromInput(input json.RawMessage) string {
	return stringField(inputFields(input), "command")
}

// stringField returns the first of the keys that names a non-empty string value.
func stringField(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := fields[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// turnReason maps a result line's subtype and stop reason to a agents.TurnEnded reason.
// "error_max_turns" is documented; every other non-success subtype is a generic failure, since
// Claude Code's exact set of error subtypes is not documented beyond that one (see the report).
func turnReason(m resultLine) string {
	switch m.Subtype {
	case "success", "":
		return endReason(m.StopReason)
	case "error_max_turns":
		return agents.TurnMaxRequests
	default:
		return agents.TurnError
	}
}

// endReason maps the stop reason of a turn that ended without Claude Code calling it an error.
func endReason(stopReason string) string {
	switch stopReason {
	case "max_tokens":
		return agents.TurnMaxTokens
	case "refusal":
		return agents.TurnRefusal
	default:
		return agents.TurnEndTurn
	}
}

// resultFailureMessage is the plain sentence for a Failed event from a result line that Claude
// Code marked as an error.
func resultFailureMessage(m resultLine) string {
	if m.Result != "" {
		return m.Result
	}
	return "The agent could not finish its turn."
}

// resultFailureDetail is the log-and-fold detail for a Failed event from an error result.
func resultFailureDetail(m resultLine) string {
	if m.Subtype != "" {
		return m.Subtype
	}
	return "the agent reported an error"
}
