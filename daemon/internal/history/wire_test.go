package history_test

import (
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The mapping from a stored event to the wire. The two tests that walk KindValues are the loud
// ones: the day this package stores a kind the wire cannot draw, they fail here instead of the API
// quietly skipping rows.

// event builds one stored event with the kind, state, summary, and detail a test needs.
func event(kind history.Kind, state history.State, summary, detail string) history.Event {
	return history.Event{
		ID: "01M3C107JB041061050R3GG28A", CardID: "card-1", SessionID: "session-1", Seq: 7,
		Kind: kind, State: state, Summary: summary, Detail: detail,
		At: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
	}
}

// toolDetailJSON is the payload this package stores for a tool call (agents.go, toolDetail).
const toolDetailJSON = `{"id":"call_1","title":"Edited internal/upstream/conn.go","toolKind":"edit",` +
	`"status":"completed","path":"internal/upstream/conn.go","content":"Replaced Dial.\nok",` +
	`"diffs":[{"path":"internal/upstream/conn.go","oldText":"a","newText":"b"}],"truncated":true}`

// TestEveryStoredKindHasAWireMessage walks the kinds this package can store and requires a chat
// message for each. A kind without one is a failure here, not a skipped row on the wire.
func TestEveryStoredKindHasAWireMessage(t *testing.T) {
	for _, kind := range history.KindValues() {
		message, err := history.ChatMessageOf(event(kind, "", "a line", ""))
		if err != nil {
			t.Errorf("kind %q has no chat message: %v", kind, err)
			continue
		}
		if !message.Kind.Valid() {
			t.Errorf("kind %q maps to the message kind %q, which is not one of the fixed list", kind, message.Kind)
		}
		if message.ID == "" || message.Seq == 0 || message.At.Time().IsZero() {
			t.Errorf("kind %q lost its id, its place, or its time: %+v", kind, message)
		}
	}
}

// TestEveryStoredKindHasAnActivityKind is the same guarantee for the activity list.
func TestEveryStoredKindHasAnActivityKind(t *testing.T) {
	for _, kind := range history.KindValues() {
		got, err := history.ActivityKindOf(event(kind, "", "a line", ""))
		if err != nil {
			t.Errorf("kind %q has no activity kind: %v", kind, err)
			continue
		}
		if !got.Valid() {
			t.Errorf("kind %q maps to the activity kind %q, which is not one of the fixed list", kind, got)
		}
	}
}

func TestChatMessageOfMapsEveryKind(t *testing.T) {
	tests := []struct {
		name       string
		ev         history.Event
		wantKind   protocol.ChatMessageKind
		wantText   string
		checkExtra func(t *testing.T, message protocol.ChatMessage)
	}{
		{
			name: "a person's message", ev: event(history.KindUser, "", "Upgrade the library", ""),
			wantKind: protocol.ChatMessageKindUser, wantText: "Upgrade the library",
		},
		{
			name: "the agent's answer", ev: event(history.KindAgent, "", "I read the callers.", ""),
			wantKind: protocol.ChatMessageKindAgent, wantText: "I read the callers.",
		},
		{
			name:     "the agent's reasoning is drawn as part of its message",
			ev:       event(history.KindThought, "", "Maybe the dialer is nil.", ""),
			wantKind: protocol.ChatMessageKindAgent, wantText: "Maybe the dialer is nil.",
		},
		{
			name: "a system note", ev: event(history.KindSystem, history.StateFailed, "The agent exited.", `{"message":"boom"}`),
			wantKind: protocol.ChatMessageKindSystem, wantText: "The agent exited.",
		},
		{
			name: "a tool call with its detail", ev: event(history.KindToolCall, history.StateOK, "Edited conn.go", toolDetailJSON),
			wantKind: protocol.ChatMessageKindTool,
			checkExtra: func(t *testing.T, message protocol.ChatMessage) {
				t.Helper()
				if message.Tool == nil {
					t.Fatal("a tool message has no tool call")
				}
				if message.Tool.ID != "call_1" || message.Tool.Title != "Edited conn.go" || message.Tool.ToolKind != "edit" {
					t.Errorf("tool call = %+v", *message.Tool)
				}
				if message.Tool.State == nil || *message.Tool.State != protocol.ActivityStateOK {
					t.Errorf("tool state = %v, want ok", message.Tool.State)
				}
				if !message.Tool.HasDetail {
					t.Error("a tool call with content and diffs says it has no detail")
				}
			},
		},
		{
			name:     "an update that reports no state keeps the state it had",
			ev:       event(history.KindToolCallUpdate, "", "Bash: go build ./...", `{"id":"call_2","content":"building"}`),
			wantKind: protocol.ChatMessageKindTool,
			checkExtra: func(t *testing.T, message protocol.ChatMessage) {
				t.Helper()
				if message.Tool == nil || message.Tool.State != nil {
					t.Errorf("an update with no state = %+v, want a null state", message.Tool)
				}
				if message.Tool != nil && message.Tool.Title != "Bash: go build ./..." {
					t.Errorf("the title = %q, want the event's own line", message.Tool.Title)
				}
			},
		},
		{
			name: "a diff summary", ev: event(history.KindDiffSummary, "", "3 files changed", `{"files":3,"additions":42,"deletions":7}`),
			wantKind: protocol.ChatMessageKindDiff, wantText: "3 files changed",
			checkExtra: func(t *testing.T, message protocol.ChatMessage) {
				t.Helper()
				want := protocol.ChatDiffSummary{Files: 3, Additions: 42, Deletions: 7}
				if message.Diff == nil || *message.Diff != want {
					t.Errorf("diff = %+v, want %+v", message.Diff, want)
				}
			},
		},
		{
			name:     "a plan",
			ev:       event(history.KindPlan, "", "Plan with 2 steps", `{"steps":[{"text":"Read","status":"completed"},{"text":"Change","status":"pending"}]}`),
			wantKind: protocol.ChatMessageKindPlan, wantText: "Plan with 2 steps",
			checkExtra: func(t *testing.T, message protocol.ChatMessage) {
				t.Helper()
				if message.Plan == nil || len(message.Plan.Steps) != 2 || message.Plan.Steps[0] != "Read" {
					t.Fatalf("plan = %+v", message.Plan)
				}
				if message.Plan.Files == nil || message.Plan.Risks == nil || message.Plan.Checks == nil {
					t.Error("the lists of a plan must be empty, never null")
				}
			},
		},
		{
			name:     "an approval",
			ev:       event(history.KindApproval, history.StateWaiting, "Asked to run rm -rf build", `{"requestId":"r1","title":"Clean the build folder","command":"rm -rf build"}`),
			wantKind: protocol.ChatMessageKindApproval, wantText: "Asked to run rm -rf build",
			checkExtra: func(t *testing.T, message protocol.ChatMessage) {
				t.Helper()
				if message.Approval == nil {
					t.Fatal("an approval message has no approval block")
				}
				want := protocol.ChatApproval{
					State: protocol.ChatApprovalStateWaiting, Command: "rm -rf build", Reason: "Clean the build folder",
				}
				if *message.Approval != want {
					t.Errorf("approval = %+v, want %+v", *message.Approval, want)
				}
			},
		},
		{
			name:     "a card reference",
			ev:       event(history.KindCardReference, "", "Made a card.", `{"cards":[{"projectId":"api","number":43}]}`),
			wantKind: protocol.ChatMessageKindCard, wantText: "Made a card.",
			checkExtra: func(t *testing.T, message protocol.ChatMessage) {
				t.Helper()
				if message.Card == nil || len(message.Card.Cards) != 1 {
					t.Fatalf("card reference = %+v", message.Card)
				}
				if message.Card.Cards[0] != (protocol.CardKey{ProjectID: "api", Number: 43}) {
					t.Errorf("card reference = %+v", message.Card.Cards)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			message, err := history.ChatMessageOf(tc.ev)
			if err != nil {
				t.Fatalf("ChatMessageOf: %v", err)
			}
			if message.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", message.Kind, tc.wantKind)
			}
			if message.Text != tc.wantText {
				t.Errorf("text = %q, want %q", message.Text, tc.wantText)
			}
			if !message.At.Time().Equal(tc.ev.At) {
				t.Errorf("at = %s, want %s", message.At.Time(), tc.ev.At)
			}
			if tc.checkExtra != nil {
				tc.checkExtra(t, message)
			}
		})
	}
}

// A card reference with no payload still answers with an empty list, never null.
func TestChatMessageOfKeepsAnEmptyCardList(t *testing.T) {
	message, err := history.ChatMessageOf(event(history.KindCardReference, "", "No cards.", ""))
	if err != nil {
		t.Fatalf("ChatMessageOf: %v", err)
	}
	if message.Card == nil || message.Card.Cards == nil {
		t.Fatalf("card reference = %+v, want an empty list", message.Card)
	}
}

// A message with no detail has nothing to open: HasDetail stays false.
func TestChatMessageOfAToolCallWithNoDetail(t *testing.T) {
	message, err := history.ChatMessageOf(event(history.KindToolCall, history.StateRunning, "Bash: go build ./...", ""))
	if err != nil {
		t.Fatalf("ChatMessageOf: %v", err)
	}
	if message.Tool == nil || message.Tool.HasDetail {
		t.Errorf("tool call = %+v, want no detail", message.Tool)
	}
}

func TestChatMessageOfRefusesWhatItCannotRead(t *testing.T) {
	if _, err := history.ChatMessageOf(event("nonsense", "", "x", "")); err == nil {
		t.Error("an unknown kind = nil error, want one")
	}
	if _, err := history.ChatMessageOf(event(history.KindToolCall, "", "x", "not json")); err == nil {
		t.Error("a detail that is not JSON = nil error, want one")
	}
	if _, err := history.ChatMessageOf(event(history.KindToolCall, history.State("nonsense"), "x", "")); err == nil {
		t.Error("an unknown state = nil error, want one")
	}
}

func TestActivityItemOfSkipsTheChat(t *testing.T) {
	item, ok, err := history.ActivityItemOf(event(history.KindAgent, "", "a piece of the answer", ""))
	if err != nil || ok {
		t.Fatalf("a chat message = %+v, ok %v, err %v; want no item and no error", item, ok, err)
	}
}

func TestActivityItemOfMapsEveryActivityKind(t *testing.T) {
	tests := []struct {
		name     string
		ev       history.Event
		wantKind protocol.ActivityKind
		want     string
	}{
		{
			name: "a file a tool edited",
			ev: event(history.KindToolCall, history.StateOK, "Edited conn.go",
				`{"id":"c1","toolKind":"edit","path":"internal/upstream/conn.go"}`),
			wantKind: protocol.ActivityKindFile,
		},
		{
			name: "a command a tool ran",
			ev: event(history.KindToolCall, history.StateRunning, "Bash: go build ./...",
				`{"id":"c2","toolKind":"execute","command":"go build ./..."}`),
			wantKind: protocol.ActivityKindCommand,
		},
		{
			name: "a test a tool ran, with the last thing it said",
			ev: event(history.KindToolCallUpdate, history.StateOK, "Bash: go test ./...",
				`{"id":"c3","toolKind":"execute","command":"go test ./...","content":"ok  daemon\n"}`),
			wantKind: protocol.ActivityKindTest, want: "ok  daemon",
		},
		{
			name: "a type check is a test too",
			ev: event(history.KindToolCall, history.StateOK, "Bash: tsc --noEmit",
				`{"id":"c4","toolKind":"execute","command":"tsc --noEmit"}`),
			wantKind: protocol.ActivityKindTest,
		},
		{
			name: "a tool that neither ran nor touched a file",
			ev: event(history.KindToolCall, history.StateOK, "Searched the repository",
				`{"id":"c5","toolKind":"search"}`),
			wantKind: protocol.ActivityKindTool,
		},
		{
			name:     "a note the daemon wrote",
			ev:       event(history.KindSystem, history.StateFailed, "The agent exited.", ""),
			wantKind: protocol.ActivityKindTool,
		},
		{
			name:     "a permission the agent asked for",
			ev:       event(history.KindApproval, history.StateWaiting, "Asked to run rm -rf build", ""),
			wantKind: protocol.ActivityKindApproval,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item, ok, err := history.ActivityItemOf(tc.ev)
			if err != nil || !ok {
				t.Fatalf("ActivityItemOf = %+v, ok %v, err %v", item, ok, err)
			}
			if item.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", item.Kind, tc.wantKind)
			}
			if item.Result != tc.want {
				t.Errorf("result = %q, want %q", item.Result, tc.want)
			}
			if !item.State.Valid() {
				t.Errorf("state = %q, which is not one of the fixed list", item.State)
			}
		})
	}
}

// The result line is the last thing the tool said, and a very long line is cut so it stays a line.
func TestActivityItemOfCutsALongResult(t *testing.T) {
	long := strings.Repeat("x", 500)
	ev := event(history.KindToolCall, history.StateOK, "Bash: go build ./...",
		`{"id":"c1","toolKind":"execute","command":"go build ./...","content":"`+long+`"}`)
	item, ok, err := history.ActivityItemOf(ev)
	if err != nil || !ok {
		t.Fatalf("ActivityItemOf = %+v, ok %v, err %v", item, ok, err)
	}
	if len(item.Result) > 60 {
		t.Errorf("the result is %d characters, want at most 60", len(item.Result))
	}
}

// Blank lines at the end of a tool's output are not the result.
func TestActivityItemOfTakesTheLastLineThatSaysSomething(t *testing.T) {
	ev := event(history.KindToolCall, history.StateOK, "Bash: go test ./...",
		`{"id":"c1","toolKind":"execute","command":"go test ./...","content":"231 passed\n\n"}`)
	item, _, err := history.ActivityItemOf(ev)
	if err != nil {
		t.Fatalf("ActivityItemOf: %v", err)
	}
	if item.Result != "231 passed" {
		t.Errorf("result = %q, want the last line with something on it", item.Result)
	}
}

func TestActivityStateOfMapsEveryState(t *testing.T) {
	tests := map[history.State]protocol.ActivityState{
		history.StateOK:      protocol.ActivityStateOK,
		history.StateRunning: protocol.ActivityStateRunning,
		history.StateFailed:  protocol.ActivityStateFailed,
		history.StateWaiting: protocol.ActivityStateWaiting,
		"":                   "",
	}
	for state, want := range tests {
		got, err := history.ActivityStateOf(state)
		if err != nil || got != want {
			t.Errorf("ActivityStateOf(%q) = %q, %v; want %q", state, got, err, want)
		}
	}
	if _, err := history.ActivityStateOf("nonsense"); err == nil {
		t.Error("ActivityStateOf of an unknown state = nil error, want one")
	}
}

func TestStateValuesAllMap(t *testing.T) {
	for _, state := range history.StateValues() {
		if _, err := history.ActivityStateOf(state); err != nil {
			t.Errorf("state %q has no wire state: %v", state, err)
		}
	}
}

func TestToolDetailOfReturnsTheWholeCall(t *testing.T) {
	detail, err := history.ToolDetailOf(event(history.KindToolCall, history.StateOK, "Edited conn.go", toolDetailJSON))
	if err != nil {
		t.Fatalf("ToolDetailOf: %v", err)
	}
	if detail.Path != "internal/upstream/conn.go" || detail.Content == "" || len(detail.Diffs) != 1 || !detail.Truncated {
		t.Errorf("detail = %+v", detail)
	}
	if got := detail.Diffs[0]; got.Path != "internal/upstream/conn.go" || got.OldText != "a" || got.NewText != "b" {
		t.Errorf("diff = %+v", got)
	}
	// A kind with no tool detail answers with an empty one rather than an error.
	empty, err := history.ToolDetailOf(event(history.KindSystem, history.StateOK, "a note", ""))
	if err != nil || empty.ID != "" || empty.Diffs == nil {
		t.Errorf("ToolDetailOf of a note = %+v, %v; want an empty detail with an empty diff list", empty, err)
	}
}

func TestToolDetailOfRefusesADetailItCannotRead(t *testing.T) {
	if _, err := history.ToolDetailOf(event(history.KindToolCall, history.StateOK, "x", "not json")); err == nil {
		t.Error("ToolDetailOf with a detail that is not JSON = nil error, want one")
	}
}
