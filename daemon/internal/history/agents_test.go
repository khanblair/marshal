package history_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/history"
)

func TestRecordsOfTurnsAnAgentEventIntoOneRecord(t *testing.T) {
	type testCase struct {
		event   agents.AgentEvent
		kind    history.Kind
		state   history.State
		summary string
		detail  string
	}
	cases := map[string]testCase{
		"a message chunk": {
			event:   agents.MessageChunk{Text: "hello"},
			kind:    history.KindAgent,
			summary: "hello",
		},
		"a thought chunk": {
			event:   agents.ThoughtChunk{Text: "let me see"},
			kind:    history.KindThought,
			summary: "let me see",
		},
		"a tool call": {
			event: agents.ToolCall{
				ID: "t1", Title: "Edit main.go", Kind: "edit", Status: agents.StatusInProgress,
				Path: "main.go", Content: "new text",
				Diffs:     []agents.FileDiff{{Path: "main.go", OldText: "old", NewText: "new"}},
				Truncated: true,
			},
			kind:    history.KindToolCall,
			state:   history.StateRunning,
			summary: "Edit main.go",
			detail: `{"id":"t1","title":"Edit main.go","toolKind":"edit","status":"in_progress",` +
				`"path":"main.go","content":"new text",` +
				`"diffs":[{"path":"main.go","oldText":"old","newText":"new"}],"truncated":true}`,
		},
		"a tool call that finished": {
			event: agents.ToolCallUpdate{ID: "t1", Status: agents.StatusCompleted, Content: "done"},
			kind:  history.KindToolCallUpdate,
			state: history.StateOK,
			// A ToolCall only names a call the first time, so an update has no summary line and no
			// tool kind of its own.
			detail: `{"id":"t1","status":"completed","content":"done"}`,
		},
		"a tool call that failed": {
			event:  agents.ToolCall{ID: "t2", Status: agents.StatusFailed},
			kind:   history.KindToolCall,
			state:  history.StateFailed,
			detail: `{"id":"t2","status":"failed"}`,
		},
		"an update that does not mention a status": {
			event:  agents.ToolCallUpdate{ID: "t1", Content: "more"},
			kind:   history.KindToolCallUpdate,
			state:  "",
			detail: `{"id":"t1","content":"more"}`,
		},
		"a plan": {
			event: agents.PlanUpdate{Steps: []agents.PlanStep{
				{Text: "Read the router", Status: agents.PlanCompleted},
				{Text: "Add the route", Status: agents.PlanInProgress},
			}},
			kind:    history.KindPlan,
			summary: "Plan with 2 steps",
			detail:  `{"steps":[{"text":"Read the router","status":"completed"},{"text":"Add the route","status":"in_progress"}]}`,
		},
		"a one-step plan": {
			event:   agents.PlanUpdate{Steps: []agents.PlanStep{{Text: "Only one", Status: agents.PlanPending}}},
			kind:    history.KindPlan,
			summary: "Plan with 1 step",
			detail:  `{"steps":[{"text":"Only one","status":"pending"}]}`,
		},
		"an empty plan": {
			event:   agents.PlanUpdate{},
			kind:    history.KindPlan,
			summary: "Plan with 0 steps",
			detail:  `{"steps":[]}`,
		},
		"a permission request": {
			event: agents.PermissionRequested{
				RequestID: "r1", Title: "Run the tests", Kind: "execute", Path: "main.go", Command: "go test ./...",
			},
			kind:    history.KindApproval,
			state:   history.StateWaiting,
			summary: "Run the tests",
			detail:  `{"requestId":"r1","title":"Run the tests","requestKind":"execute","path":"main.go","command":"go test ./..."}`,
		},
		"a failure": {
			event:   agents.Failed{Message: "the agent broke", Detail: "the last lines"},
			kind:    history.KindSystem,
			state:   history.StateFailed,
			summary: "the agent broke",
			detail:  `{"message":"the agent broke","detail":"the last lines"}`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			records, err := history.RecordsOf(tc.event)
			if err != nil {
				t.Fatalf("RecordsOf: %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("got %d records, want 1: %+v", len(records), records)
			}
			got := records[0]
			if got.Kind != tc.kind || got.State != tc.state || got.Summary != tc.summary || got.Detail != tc.detail {
				t.Errorf("record = %+v, want kind %q, state %q, summary %q, detail %q",
					got, tc.kind, tc.state, tc.summary, tc.detail)
			}
		})
	}
}

func TestRecordsOfIgnoresEventsThatAreNotHistory(t *testing.T) {
	cases := map[string]agents.AgentEvent{
		"a turn ending":  agents.TurnEnded{Reason: agents.TurnEndTurn},
		"an exit":        agents.Exited{Code: 0},
		"terminal bytes": agents.TerminalOutput{Data: []byte("$ ls")},
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			records, err := history.RecordsOf(event)
			if err != nil {
				t.Fatalf("RecordsOf: %v", err)
			}
			if len(records) != 0 {
				t.Errorf("got %d records for %s, want none: %+v", len(records), name, records)
			}
		})
	}
}
