package history

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The mapping from a stored event to the wire types a screen draws (docs/backend-inventory.md 4.3
// and 4.4, N13 and N14). It lives beside the internal kinds, not in internal/protocol, because the
// protocol package must not know what this package stores.
//
// Every kind this package can store maps to a chat message, and the test
// TestEveryStoredKindHasAWireMessage walks KindValues and fails when one does not. Nothing is
// skipped quietly: a kind that cannot be drawn is an error here, so the day a kind is added the
// wire side stops compiling its tests instead of dropping rows.

// ChatMessageOf turns one stored event into the chat message the API serves. A kind that has no
// wire message, and a detail that is not the JSON this package wrote, are errors.
func ChatMessageOf(ev Event) (protocol.ChatMessage, error) {
	kind, err := chatMessageKindOf(ev.Kind)
	if err != nil {
		return protocol.ChatMessage{}, err
	}
	state, err := ActivityStateOf(ev.State)
	if err != nil {
		return protocol.ChatMessage{}, fmt.Errorf("event %s of %s: %w", ev.ID, ev.ownerLabel(), err)
	}
	message := protocol.ChatMessage{
		ID: ev.ID, Kind: kind, Seq: ev.Seq, At: protocol.NewTimestamp(ev.At),
	}
	switch kind {
	case protocol.ChatMessageKindTool:
		call, err := toolCallOf(ev)
		if err != nil {
			return protocol.ChatMessage{}, err
		}
		message.Tool = &call
	case protocol.ChatMessageKindDiff:
		summary, err := diffSummaryOf(ev)
		if err != nil {
			return protocol.ChatMessage{}, err
		}
		message.Text, message.Diff = ev.Summary, &summary
	case protocol.ChatMessageKindPlan:
		plan, err := planOf(ev)
		if err != nil {
			return protocol.ChatMessage{}, err
		}
		message.Text, message.Plan = ev.Summary, &plan
	case protocol.ChatMessageKindApproval:
		approval, err := approvalOf(ev, state)
		if err != nil {
			return protocol.ChatMessage{}, err
		}
		message.Text, message.Approval = ev.Summary, &approval
	case protocol.ChatMessageKindCard:
		reference, err := cardRefOf(ev)
		if err != nil {
			return protocol.ChatMessage{}, err
		}
		message.Text, message.Card = ev.Summary, &reference
	default:
		// A user message, the agent's answer, or a system note: the line is the message.
		message.Text = ev.Summary
	}
	return message, nil
}

// ActivityItemOf turns one stored event into the item a card's Activity tab shows. ok is false
// when the event is a chat message rather than an activity item: an entry of the activity list
// always ended somehow, and a row with no state never did (kinds.go, State).
func ActivityItemOf(ev Event) (item protocol.ActivityItem, ok bool, err error) {
	if ev.State == "" {
		return protocol.ActivityItem{}, false, nil
	}
	state, err := ActivityStateOf(ev.State)
	if err != nil {
		return protocol.ActivityItem{}, false, fmt.Errorf("event %s of %s: %w", ev.ID, ev.ownerLabel(), err)
	}
	kind, err := ActivityKindOf(ev)
	if err != nil {
		return protocol.ActivityItem{}, false, err
	}
	return protocol.ActivityItem{
		ID: ev.ID, Kind: kind, Seq: ev.Seq, At: protocol.NewTimestamp(ev.At),
		Text: ev.Summary, Result: activityResultOf(ev), State: state,
	}, true, nil
}

// ActivityKindOf says which activity kind one stored event is drawn as. It is the derivation the
// prototype's own activity list uses: a tool call that runs something is a command (or a test,
// when the command is one), a tool call that touches a file is a file, an approval is an approval,
// and anything else is a tool.
func ActivityKindOf(ev Event) (protocol.ActivityKind, error) {
	switch ev.Kind {
	case KindToolCall, KindToolCallUpdate:
		detail, err := storedToolDetail(ev)
		if err != nil {
			return "", err
		}
		return toolActivityKind(detail), nil
	case KindApproval:
		return protocol.ActivityKindApproval, nil
	case KindDiffSummary:
		return protocol.ActivityKindFile, nil
	case KindSystem, KindPlan, KindCardReference, KindUser, KindAgent, KindThought:
		// A plan, a system note, or a card reference is "any other tool call or system note"
		// (ActivityKindTool). A person's message, the agent's answer, and its reasoning carry no
		// state, so they are never served as activity items; they still have a kind here so that
		// no stored kind can be dropped without a test failing.
		return protocol.ActivityKindTool, nil
	default:
		return "", fmt.Errorf("%s: the event kind %q has no activity kind", ev.ownerLabel(), ev.Kind)
	}
}

// ActivityStateOf maps the state a stored event carries to the wire state. The empty state maps to
// the empty state and no error: it means the event is not an activity item.
func ActivityStateOf(state State) (protocol.ActivityState, error) {
	switch state {
	case StateOK:
		return protocol.ActivityStateOK, nil
	case StateRunning:
		return protocol.ActivityStateRunning, nil
	case StateFailed:
		return protocol.ActivityStateFailed, nil
	case StateWaiting:
		return protocol.ActivityStateWaiting, nil
	case "":
		return "", nil
	default:
		return "", fmt.Errorf("the stored state %q has no activity state", state)
	}
}

// chatMessageKindOf maps one stored kind to the message a screen draws. The agent's reasoning is
// stored as its own kind and drawn as part of the agent's message: the chat has no separate
// thought block (docs/backend-inventory.md 4.3).
func chatMessageKindOf(kind Kind) (protocol.ChatMessageKind, error) {
	switch kind {
	case KindUser:
		return protocol.ChatMessageKindUser, nil
	case KindAgent, KindThought:
		return protocol.ChatMessageKindAgent, nil
	case KindToolCall, KindToolCallUpdate:
		return protocol.ChatMessageKindTool, nil
	case KindSystem:
		return protocol.ChatMessageKindSystem, nil
	case KindDiffSummary:
		return protocol.ChatMessageKindDiff, nil
	case KindPlan:
		return protocol.ChatMessageKindPlan, nil
	case KindApproval:
		return protocol.ChatMessageKindApproval, nil
	case KindCardReference:
		return protocol.ChatMessageKindCard, nil
	default:
		return "", fmt.Errorf("the stored kind %q has no chat message kind", kind)
	}
}

// toolCallOf builds the tool call block of one stored event. The line the block shows is the
// event's summary, which is the title the agent gave the call; an update that carries no title
// keeps the one it had, so it falls back to the title in the detail.
func toolCallOf(ev Event) (protocol.ChatToolCall, error) {
	detail, err := storedToolDetail(ev)
	if err != nil {
		return protocol.ChatToolCall{}, err
	}
	state, err := ActivityStateOf(ev.State)
	if err != nil {
		return protocol.ChatToolCall{}, err
	}
	var pointer *protocol.ActivityState
	if state != "" {
		value := state
		pointer = &value
	}
	return protocol.ChatToolCall{
		ID: detail.ID, Title: firstText(ev.Summary, detail.Title), ToolKind: detail.ToolKind,
		State: pointer, HasDetail: detail.Content != "" || len(detail.Diffs) > 0,
	}, nil
}

// ToolDetailOf reads the whole of one stored tool call, for the detail a chat block opens on
// demand: the output the agent kept and the files it changed. A kind that has no such detail comes
// back empty rather than as an error, so a caller has one shape to draw.
func ToolDetailOf(ev Event) (protocol.ChatToolDetail, error) {
	detail, err := storedToolDetail(ev)
	if err != nil {
		return protocol.ChatToolDetail{}, err
	}
	diffs := make([]protocol.FileDiff, 0, len(detail.Diffs))
	for _, diff := range detail.Diffs {
		diffs = append(diffs, protocol.FileDiff{
			Path: diff.Path, OldText: diff.OldText, NewText: diff.NewText,
		})
	}
	return protocol.ChatToolDetail{
		ID: detail.ID, Title: firstText(detail.Title, ev.Summary), ToolKind: detail.ToolKind,
		Path: detail.Path, Command: detail.Command, Content: detail.Content,
		Diffs: diffs, Truncated: detail.Truncated,
	}, nil
}

// diffSummaryOf builds the diff summary of one stored event: how many files a turn changed and how
// many lines went in and out. Nothing writes this kind yet (N15's turn-end summary does), so the
// stored shape is the one that slice will use.
func diffSummaryOf(ev Event) (protocol.ChatDiffSummary, error) {
	var stored diffSummaryDetail
	if err := decodeDetail(ev, &stored); err != nil {
		return protocol.ChatDiffSummary{}, err
	}
	return protocol.ChatDiffSummary{
		Files: stored.Files, Additions: stored.Additions, Deletions: stored.Deletions,
	}, nil
}

// planOf builds the plan block of one stored event. Nothing writes a plan message yet (Phase 5,
// B5.2), so a plan is drawn as waiting and only its steps come from what is stored.
func planOf(ev Event) (protocol.ChatPlan, error) {
	var stored planDetail
	if err := decodeDetail(ev, &stored); err != nil {
		return protocol.ChatPlan{}, err
	}
	steps := make([]string, 0, len(stored.Steps))
	for _, step := range stored.Steps {
		steps = append(steps, step.Text)
	}
	return protocol.ChatPlan{
		State: protocol.ChatPlanStateWaiting, Steps: steps,
		Files: []string{}, Risks: []string{}, Checks: []string{},
	}, nil
}

// approvalOf builds the approval block of one stored event. Nothing writes an approval message yet
// (Phase 3, B3.4), and the request an agent asks for today is only logged, so a stored approval is
// drawn as waiting until the approval flow answers it.
func approvalOf(ev Event, state protocol.ActivityState) (protocol.ChatApproval, error) {
	var stored approvalDetail
	if err := decodeDetail(ev, &stored); err != nil {
		return protocol.ChatApproval{}, err
	}
	return protocol.ChatApproval{
		State: approvalState(state), Command: stored.Command, Reason: stored.Title,
	}, nil
}

// approvalState maps how an approval ended to where the block stands. A state the approval flow
// cannot produce reads as waiting, which is the state a request nobody answered is in.
func approvalState(state protocol.ActivityState) protocol.ChatApprovalState {
	switch state {
	case protocol.ActivityStateOK:
		return protocol.ChatApprovalStateApproved
	case protocol.ActivityStateFailed:
		return protocol.ChatApprovalStateDenied
	default:
		return protocol.ChatApprovalStateWaiting
	}
}

// cardRefOf builds the card reference of one stored event: the cards the message names.
func cardRefOf(ev Event) (protocol.ChatCardRef, error) {
	var stored cardRefDetail
	if err := decodeDetail(ev, &stored); err != nil {
		return protocol.ChatCardRef{}, err
	}
	cards := stored.Cards
	if cards == nil {
		cards = []protocol.CardKey{}
	}
	return protocol.ChatCardRef{Cards: cards}, nil
}

// storedToolDetail reads the structured payload of a tool call event. An event with no detail is
// not an error: the one line says everything the daemon kept.
func storedToolDetail(ev Event) (toolDetail, error) {
	var detail toolDetail
	if err := decodeDetail(ev, &detail); err != nil {
		return toolDetail{}, err
	}
	return detail, nil
}

// decodeDetail reads one event's stored payload. The daemon wrote it, so a payload it cannot read
// is a problem to report rather than a row to skip.
func decodeDetail(ev Event, into any) error {
	if ev.Detail == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(ev.Detail), into); err != nil {
		return fmt.Errorf("read the detail of event %s of %s: %w", ev.ID, ev.ownerLabel(), err)
	}
	return nil
}

// toolActivityKind is the kind of activity a tool call is: a command or a test when it runs
// something, a file when it touches one, and a tool otherwise. The tool's own kind is the agent's
// word for it (agents.ToolCall.Kind).
func toolActivityKind(detail toolDetail) protocol.ActivityKind {
	if detail.Command != "" || detail.ToolKind == toolKindExecute {
		if isTestCommand(firstText(detail.Command, detail.Title)) {
			return protocol.ActivityKindTest
		}
		return protocol.ActivityKindCommand
	}
	switch detail.ToolKind {
	case toolKindRead, toolKindEdit, toolKindDelete, toolKindMove:
		return protocol.ActivityKindFile
	}
	if detail.Path != "" || len(detail.Diffs) > 0 {
		return protocol.ActivityKindFile
	}
	return protocol.ActivityKindTool
}

// isTestCommand reports whether a command line is a test or a type check, the rule the prototype's
// activity list uses: the words "test" and "tsc" anywhere in the command. It is deliberately
// simple, because the daemon does not parse shell command lines.
func isTestCommand(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, "test") || strings.Contains(lower, "tsc")
}

// activityResultOf is the short line an activity entry shows beside its one line: the last thing
// the tool said. Empty when the tool said nothing, or when the event is not a tool call.
func activityResultOf(ev Event) string {
	detail, err := storedToolDetail(ev)
	if err != nil {
		return ""
	}
	return lastLine(detail.Content)
}

// lastLine is the last line of text that is not blank, cut to a short bound so one long line
// cannot fill the activity list.
func lastLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return cutText(line, maxResultChars)
		}
	}
	return ""
}

// cutText cuts text to at most limit characters, so a line stays a line.
func cutText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return strings.TrimSpace(text[:limit])
}

// firstText is the first of two lines that is not empty.
func firstText(first, second string) string {
	if first != "" {
		return first
	}
	return second
}

const (
	// maxResultChars bounds the result line an activity entry shows.
	maxResultChars = 60
	// toolKindExecute is the agent's word for a tool that runs a command (agents.ToolCall.Kind).
	toolKindExecute = "execute"
	// toolKindRead and its siblings are the agent's words for a tool that touches a file.
	toolKindRead   = "read"
	toolKindEdit   = "edit"
	toolKindDelete = "delete"
	toolKindMove   = "move"
)

// diffSummaryDetail is the stored payload of a diff summary (KindDiffSummary): how many files a
// turn changed and how many lines went in and out. Nothing writes it yet; the turn-end diff
// summary of a later slice (N15) writes these names.
type diffSummaryDetail struct {
	Files     int `json:"files"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// cardRefDetail is the stored payload of a card reference (KindCardReference): the cards a message
// made or named. Nothing writes it yet; the card tool of the internal MCP server (Phase 7) does.
type cardRefDetail struct {
	Cards []protocol.CardKey `json:"cards"`
}
