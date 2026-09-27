package mcpserver

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// section11_4 is every tool of docs/architecture.md section 11.4, in the order that table lists
// them. The whole point of this file is that this list is written out by hand and not read from the
// server: a test that asked the server what it serves would agree with the server however wrong both
// were.
var section11_4 = []string{
	"board_status",
	"claim_files",
	"release_files",
	"post_note",
	"read_notes",
	"ask_agent",
	"create_card",
	"search_memory",
	"search_codebase",
	"report_progress",
	"list_checklists",
	"tick_checklist_item",
	"read_comments",
	"read_attachment",
	"post_comment",
}

func TestEveryToolOfSection11_4IsRegisteredInTheTablesOrder(t *testing.T) {
	f := newFixture(t)
	if got := f.server.ToolNames(); !slices.Equal(got, section11_4) {
		t.Errorf("the tools are\n%q\nwant\n%q", got, section11_4)
	}
	if got, want := f.server.ToolCount(), len(section11_4); got != want {
		t.Errorf("%d tools are served, want %d", got, want)
	}
}

func TestToolNamesAnswersACopy(t *testing.T) {
	f := newFixture(t)
	names := f.server.ToolNames()
	if len(names) == 0 {
		t.Fatal("the server serves no tools")
	}
	names[0] = "tampered"
	if got := f.server.ToolNames()[0]; got != section11_4[0] {
		t.Errorf("the first tool is now %q: ToolNames handed out the server's own list", got)
	}
}

func TestTheCardIDIsTheOneTheServerWasBuiltFor(t *testing.T) {
	f := newFixture(t)
	if got := f.server.CardID(); got != "card-1" {
		t.Errorf("the server serves card %q, want %q", got, "card-1")
	}
}

// A tool is only usable if a client can be told what it is, what it takes, and what it answers. The
// SDK builds the argument schema from the Go type, so this is where a type that cannot be described
// shows up.
func TestTheHandshakeOffersEveryToolDescribedAndWithArguments(t *testing.T) {
	f := newFixture(t)
	cs := f.client(t)
	listed, err := cs.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list the tools: %v", err)
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("tool %s has no description for a model to read", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %s has no argument schema", tool.Name)
		}
	}
	// The order the tools are registered in is asserted above, against the server itself. A client
	// is offered them sorted, so this side is about which tools a client can see at all.
	got, want := slices.Sorted(slices.Values(names)), slices.Sorted(slices.Values(section11_4))
	if !slices.Equal(got, want) {
		t.Errorf("a client is offered\n%q\nwant\n%q", got, want)
	}
	instructions := cs.InitializeResult().Instructions
	if !strings.Contains(instructions, "Marshal") {
		t.Errorf("the server tells a client %q, which does not name Marshal", instructions)
	}
}

func TestNewRefusesAServerMissingAPart(t *testing.T) {
	base := func() (Deps, Identity) {
		f := newFixture(t)
		return Deps{
			Cards: f.cards, Notes: f.notes, Claims: f.claims, Agents: f.agents, Harness: f.rules,
		}, Identity{
			CardID: "card-1", ProjectID: projectID,
		}
	}
	tests := []struct {
		name   string
		broken func(*Deps, *Identity)
		want   string
	}{
		{"no cards", func(d *Deps, _ *Identity) { d.Cards = nil }, "cards"},
		{"no notes", func(d *Deps, _ *Identity) { d.Notes = nil }, "notes"},
		{"no claims", func(d *Deps, _ *Identity) { d.Claims = nil }, "claims"},
		{"no agents", func(d *Deps, _ *Identity) { d.Agents = nil }, "agents"},
		{"no permission rules", func(d *Deps, _ *Identity) { d.Harness = nil }, "permission rules"},
		{"no card", func(_ *Deps, i *Identity) { i.CardID = "" }, "card id"},
		{"no project", func(_ *Deps, i *Identity) { i.ProjectID = "" }, "project id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps, identity := base()
			test.broken(&deps, &identity)
			_, err := New(deps, identity)
			if err == nil {
				t.Fatal("the server was built with a part missing")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("the error says %q, which does not name %q", err, test.want)
			}
		})
	}
}

func TestABuiltServerNeedsNoCodebaseMap(t *testing.T) {
	f := newFixture(t)
	deps := Deps{Cards: f.cards, Notes: f.notes, Claims: f.claims, Agents: f.agents, Harness: f.rules}
	if _, err := New(deps, Identity{CardID: "card-1", ProjectID: projectID}); err != nil {
		t.Errorf("a server cannot be built before the codebase map exists: %v", err)
	}
}

func TestWithClockIsUsed(t *testing.T) {
	f := newFixture(t)
	server, err := New(
		Deps{Cards: f.cards, Notes: f.notes, Claims: f.claims, Agents: f.agents, Harness: f.rules},
		Identity{CardID: "card-1", ProjectID: projectID},
		WithClock(func() time.Time { return fixtureNoteTime }),
	)
	if err != nil {
		t.Fatalf("build the server: %v", err)
	}
	if !server.now().Equal(fixtureNoteTime) {
		t.Errorf("the clock reads %s, want %s", server.now(), fixtureNoteTime)
	}
}
