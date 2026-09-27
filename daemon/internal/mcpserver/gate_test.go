package mcpserver

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// touched is every fake's memory of what it was asked to change. A refusal must leave all of it
// empty: a tool that answers "no" and writes anyway is the bug this whole file is about.
type touched struct {
	cards  int
	notes  int
	claims int
	agents int
}

func nothingTouched(f *fixture) touched {
	return touched{
		cards:  len(f.cards.created) + len(f.cards.updated),
		notes:  len(f.notes.saved),
		claims: len(f.claims.held["card-1"]) + len(f.claims.released),
		agents: len(f.agents.sent),
	}
}

// mutating is every tool that changes something, with arguments that would change it. Some of them
// speak for parts of Marshal that are not built yet; they are listed anyway, because the mode must
// refuse them before the missing part is ever reached.
func mutating() map[string]map[string]any {
	return map[string]map[string]any{
		"claim_files":         {"paths": []string{"src/health.go"}},
		"release_files":       {"paths": []string{"src/board.ts"}},
		"post_note":           {"body": "a note"},
		"ask_agent":           {"cardKey": projectID + "#2", "question": "what are you on?"},
		"create_card":         {"title": "New work"},
		"report_progress":     {"doingNow": "writing the route"},
		"tick_checklist_item": {"itemId": "item-1", "done": true, "evidence": "commit abc"},
		"post_comment":        {"body": "done"},
	}
}

func TestPlanModeRefusesEveryToolThatChangesSomething(t *testing.T) {
	for name, args := range mutating() {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.mode = protocol.PermissionModePlan
			cs := f.client(t)
			before := nothingTouched(f)
			said := callRefused(t, cs, name, args)
			if after := nothingTouched(f); after != before {
				t.Errorf("%s was refused and changed something: %+v, was %+v", name, after, before)
			}
			if !strings.Contains(said, string(protocol.PermissionModePlan)) {
				t.Errorf("the refusal does not name the mode that made it: %q", said)
			}
			if !strings.Contains(said, name) {
				t.Errorf("the refusal does not name the tool it refused: %q", said)
			}
			// The mode decides before a tool is reached, so even the tools whose part of Marshal is
			// missing answer with the mode's refusal and not with the missing part's.
			if strings.Contains(said, "does not keep") || strings.Contains(said, "has not built") {
				t.Errorf("%s answered about a missing part before the mode refused it: %q", name, said)
			}
		})
	}
}

func TestReadingIsAllowedInEveryMode(t *testing.T) {
	reads := map[string]map[string]any{
		"board_status":  {},
		"read_notes":    {},
		"search_memory": {"query": "route"},
	}
	for _, mode := range protocol.PermissionModeValues() {
		t.Run(string(mode), func(t *testing.T) {
			for name, args := range reads {
				f := newFixture(t)
				f.mode = mode
				cs := f.client(t)
				if said, isError := call(t, cs, name, args); isError {
					t.Errorf("%s is a read and %s mode refused it: %s", name, mode, said)
				}
			}
		})
	}
}

func TestAutoEditsAllowsAnEditAndLeavesANewCardToTheOwner(t *testing.T) {
	f := newFixture(t)
	f.mode = protocol.PermissionModeAutoEdits
	cs := f.client(t)

	if _, isError := call(t, cs, "report_progress", map[string]any{"doingNow": "writing the route"}); isError {
		t.Error("auto-accept edits refused to change the card's own progress line")
	}
	said := callRefused(t, cs, "create_card", map[string]any{"title": "New work"})
	if !strings.Contains(said, "owner") {
		t.Errorf("a new card was left to nobody in particular: %q", said)
	}
	if len(f.cards.created) != 0 {
		t.Errorf("a card was created anyway: %+v", f.cards.created)
	}
}

func TestFullAutoAndBypassCreateTheCard(t *testing.T) {
	for _, mode := range []protocol.PermissionMode{
		protocol.PermissionModeFullAuto, protocol.PermissionModeBypass,
	} {
		t.Run(string(mode), func(t *testing.T) {
			f := newFixture(t)
			f.mode = mode
			cs := f.client(t)
			out := callOK[createCardOut](t, cs, "create_card", map[string]any{"title": "New work"})
			if out.State != protocol.CardStateBacklog {
				t.Errorf("a proposed card starts in %q, want the backlog", out.State)
			}
			if len(f.cards.created) != 1 {
				t.Fatalf("%d cards were created, want 1", len(f.cards.created))
			}
			if f.cards.created[0].Title != "New work" {
				t.Errorf("the card was created as %+v", f.cards.created[0])
			}
		})
	}
}

func TestAskModeLeavesAnEditToTheOwner(t *testing.T) {
	f := newFixture(t)
	f.mode = protocol.PermissionModeAsk
	cs := f.client(t)
	said := callRefused(t, cs, "post_note", map[string]any{"body": "a note"})
	if !strings.Contains(said, "owner") {
		t.Errorf("a refusal in ask mode does not say whose it is: %q", said)
	}
	if len(f.notes.saved) != 0 {
		t.Errorf("the note was written anyway: %+v", f.notes.saved)
	}
}

func TestASessionWithNoModeLeavesTheCallToTheOwner(t *testing.T) {
	f := newFixture(t)
	f.noMode = true
	cs := f.client(t)
	said := callRefused(t, cs, "board_status", map[string]any{})
	if !strings.Contains(said, "could not tell") {
		t.Errorf("a session with no mode does not say so: %q", said)
	}
}

// The rules are read for each call and not kept, so a person who turns a card's mode up while its
// agent is working changes what that agent's next call may do.
func TestThePermissionRulesAreReadForEachCall(t *testing.T) {
	f := newFixture(t)
	f.mode = protocol.PermissionModePlan
	cs := f.client(t)
	callRefused(t, cs, "claim_files", map[string]any{"paths": []string{"src/health.go"}})

	f.mode = protocol.PermissionModeFullAuto
	callOK[claimFilesOut](t, cs, "claim_files", map[string]any{"paths": []string{"src/health.go"}})
	if len(f.claims.held["card-1"]) != 1 {
		t.Errorf("the claim was not recorded after the mode was turned up: %+v", f.claims.held["card-1"])
	}
}

// A refusal is an answer the model reads, not a broken call: the client's own call succeeds, the
// result says it was an error, and the text says what happened and what to do about it.
func TestARefusalIsAToolErrorWithSomethingToRead(t *testing.T) {
	f := newFixture(t)
	f.mode = protocol.PermissionModePlan
	res, err := f.client(t).CallTool(t.Context(), &mcp.CallToolParams{
		Name: "post_note", Arguments: map[string]any{"body": "x"},
	})
	if err != nil {
		t.Fatalf("a refusal broke the connection: %v", err)
	}
	if !res.IsError {
		t.Error("a refused call did not come back as an error")
	}
	said := textOf(res)
	for _, want := range []string{"plan", "post_note", "not done"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal %q does not say %q", said, want)
		}
	}
}

func TestARefusalIsNotedInTheLog(t *testing.T) {
	f := newFixture(t)
	f.mode = protocol.PermissionModePlan
	callRefused(t, f.client(t), "post_note", map[string]any{"body": "x"})
	logged := f.logs.String()
	for _, want := range []string{"post_note", "card-1", "deny", "mode"} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log does not say %q: %s", want, logged)
		}
	}
}
