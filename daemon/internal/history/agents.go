package history

import (
	"encoding/json"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// RecordsOf converts one agent event into the records it adds to a card's history, in order. An
// event that is not history — a turn ending, a piece of terminal output, the process exiting —
// adds none, so the caller appends nothing for it.
//
// The records carry no time and no id: the store gives each one both, so a batch that came from
// one event shares one time. An error means a record's detail could not be encoded, which a
// caller logs and otherwise ignores: the event itself already happened.
func RecordsOf(ev agents.AgentEvent) ([]Record, error) {
	switch e := ev.(type) {
	case agents.MessageChunk:
		return []Record{{Kind: KindAgent, Summary: e.Text}}, nil
	case agents.ThoughtChunk:
		return []Record{{Kind: KindThought, Summary: e.Text}}, nil
	case agents.ToolCall:
		return oneDetail(KindToolCall, e.Title, toolState(e.Status), toolDetailOf(e))
	case agents.ToolCallUpdate:
		return oneDetail(KindToolCallUpdate, e.Title, toolState(e.Status), toolDetailOfUpdate(e))
	case agents.PlanUpdate:
		return oneDetail(KindPlan, planSummary(e.Steps), "", planDetailOf(e.Steps))
	case agents.PermissionRequested:
		return oneDetail(KindApproval, e.Title, StateWaiting, approvalDetailOf(e))
	case agents.Failed:
		return oneDetail(KindSystem, e.Message, StateFailed, failureDetailOf(e))
	default:
		return nil, nil
	}
}

// oneDetail builds the single record these events add: one summary line, one state, and the
// structured payload a reader opens on demand.
func oneDetail(kind Kind, summary string, state State, detail any) ([]Record, error) {
	encoded, err := encodeDetail(detail)
	if err != nil {
		return nil, err
	}
	return []Record{{Kind: kind, State: state, Summary: summary, Detail: encoded}}, nil
}

// encodeDetail encodes a record's structured payload. The field names match the session log line
// and the wire's agent tool call (internal/session/log.go), so one client mapper reads all three.
func encodeDetail(detail any) (string, error) {
	data, err := json.Marshal(detail)
	if err != nil {
		return "", fmt.Errorf("encode a history detail: %w", err)
	}
	return string(data), nil
}

// planSummary is the one line a plan adds to the history and the activity list; the steps
// themselves are in the record's detail.
func planSummary(steps []agents.PlanStep) string {
	if len(steps) == 1 {
		return "Plan with 1 step"
	}
	return fmt.Sprintf("Plan with %d steps", len(steps))
}

// toolState maps a tool call's own status word to the activity state a card's list draws
// (docs/backend-inventory.md 4.4, N14). Any other status, including the empty one a
// ToolCallUpdate leaves when it does not mention a status, has no state: the call's state did not
// change.
func toolState(status string) State {
	switch status {
	case agents.StatusCompleted:
		return StateOK
	case agents.StatusFailed:
		return StateFailed
	case agents.StatusPending, agents.StatusInProgress:
		return StateRunning
	default:
		return ""
	}
}

// toolDetail is the structured payload stored for a tool call: the call's own fields, with the
// same JSON names the session log line uses.
type toolDetail struct {
	ID        string     `json:"id"`
	Title     string     `json:"title,omitempty"`
	ToolKind  string     `json:"toolKind,omitempty"`
	Status    string     `json:"status,omitempty"`
	Path      string     `json:"path,omitempty"`
	Command   string     `json:"command,omitempty"`
	Content   string     `json:"content,omitempty"`
	Diffs     []fileDiff `json:"diffs,omitempty"`
	Truncated bool       `json:"truncated,omitempty"`
}

// fileDiff is one change a tool made to a file, as stored with a tool call.
type fileDiff struct {
	Path    string `json:"path"`
	OldText string `json:"oldText,omitempty"`
	NewText string `json:"newText,omitempty"`
}

// planDetail is a whole plan, as stored with a plan update.
type planDetail struct {
	Steps []planStep `json:"steps"`
}

// planStep is one line of a stored plan.
type planStep struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

// approvalDetail is a permission request, as stored. It holds the fields the session log line
// keeps; the answers and the options the person can give belong to the approvals work.
type approvalDetail struct {
	RequestID   string `json:"requestId"`
	Title       string `json:"title,omitempty"`
	RequestKind string `json:"requestKind,omitempty"`
	Path        string `json:"path,omitempty"`
	Command     string `json:"command,omitempty"`
}

// failureDetail is an agent failure, as stored: a plain sentence and the lines behind it.
type failureDetail struct {
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// toolDetailOf turns a starting tool call into its stored detail.
func toolDetailOf(call agents.ToolCall) toolDetail {
	return toolDetail{
		ID: call.ID, Title: call.Title, ToolKind: call.Kind, Status: call.Status,
		Path: call.Path, Command: call.Command, Content: call.Content,
		Diffs: fileDiffsOf(call.Diffs), Truncated: call.Truncated,
	}
}

// toolDetailOfUpdate turns an update to a tool call into its stored detail. Empty fields stay
// empty: the update did not change them.
func toolDetailOfUpdate(update agents.ToolCallUpdate) toolDetail {
	return toolDetail{
		ID: update.ID, Title: update.Title, Status: update.Status,
		Content: update.Content, Diffs: fileDiffsOf(update.Diffs), Truncated: update.Truncated,
	}
}

// planDetailOf turns the agent's plan into its stored detail.
func planDetailOf(steps []agents.PlanStep) planDetail {
	stored := make([]planStep, len(steps))
	for i, step := range steps {
		stored[i] = planStep{Text: step.Text, Status: step.Status}
	}
	return planDetail{Steps: stored}
}

// approvalDetailOf turns a permission request into its stored detail.
func approvalDetailOf(request agents.PermissionRequested) approvalDetail {
	return approvalDetail{
		RequestID: request.RequestID, Title: request.Title, RequestKind: request.Kind,
		Path: request.Path, Command: request.Command,
	}
}

// failureDetailOf turns an agent failure into its stored detail.
func failureDetailOf(failed agents.Failed) failureDetail {
	return failureDetail{Message: failed.Message, Detail: failed.Detail}
}

// fileDiffsOf converts the agent's file diffs to the stored shape.
func fileDiffsOf(diffs []agents.FileDiff) []fileDiff {
	if len(diffs) == 0 {
		return nil
	}
	stored := make([]fileDiff, len(diffs))
	for i, diff := range diffs {
		stored[i] = fileDiff{Path: diff.Path, OldText: diff.OldText, NewText: diff.NewText}
	}
	return stored
}
