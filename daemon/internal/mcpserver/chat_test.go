package mcpserver

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A project chat's server (docs/architecture.md 16.2): the identity is a chat and not a card, and
// the tools it serves are those of its kind.

// fakeMerge is the integrator.MergeTools a chat's server routes the Integrator's three tools to.
type fakeMerge struct {
	task      integrator.MergeTask
	taskErr   error
	reportErr error
	reports   []integrator.Verdict
	reportIDs []string
	asks      []string
}

func (f *fakeMerge) Context(_ context.Context, taskID string) (integrator.MergeTask, error) {
	if f.taskErr != nil {
		return integrator.MergeTask{}, f.taskErr
	}
	task := f.task
	task.ID = taskID
	return task, nil
}

func (f *fakeMerge) Report(_ context.Context, taskID string, verdict integrator.Verdict) error {
	if f.reportErr != nil {
		return f.reportErr
	}
	f.reportIDs = append(f.reportIDs, taskID)
	f.reports = append(f.reports, verdict)
	return nil
}

func (f *fakeMerge) Ask(_ context.Context, _, question string) error {
	f.asks = append(f.asks, question)
	return nil
}

// chatFixture is a chat's server over the fixture's fakes. merge is what the merge tools are
// answered by, and may be changed after the server is built, as the daemon sets it late.
type chatFixture struct {
	*fixture
	merge integrator.MergeTools
}

// newChatFixture builds a server for a chat of a kind, whose permission rules say mode.
func newChatFixture(t *testing.T, kind ChatKind, mode protocol.PermissionMode) *chatFixture {
	t.Helper()
	base := newFixture(t)
	base.mode = mode
	cf := &chatFixture{fixture: base}
	server, err := New(
		Deps{
			Cards: base.cards, Notes: base.notes, Claims: base.claims, Codebase: base.code,
			Harness:    base.rules,
			MergeTools: func() integrator.MergeTools { return cf.merge },
		},
		Identity{ChatID: "chat-1", ChatKind: kind, ProjectID: projectID, Role: "Orchestrator"},
	)
	if err != nil {
		t.Fatalf("build the chat's server: %v", err)
	}
	base.server = server
	return cf
}

// toolsOf lists the tool names a client sees.
func toolsOf(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	var names []string
	for tool, err := range cs.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list the tools: %v", err)
		}
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func sorted(names ...string) []string {
	slices.Sort(names)
	return names
}

func TestAChatServerServesTheToolsOfItsKindAndNoneOfACardsOwn(t *testing.T) {
	tests := []struct {
		kind ChatKind
		want []string
	}{
		{ChatKindOrchestrator, sorted("board_status", "create_card", "search_memory", "search_codebase")},
		{ChatKindIntegrator, sorted("board_status", "merge_context", "merge_report", "ask_owner")},
		{ChatKindOther, sorted("board_status")},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			f := newChatFixture(t, tc.kind, protocol.PermissionModeAutoEdits)
			if got := toolsOf(t, f.client(t)); !slices.Equal(got, tc.want) {
				t.Errorf("tools = %v, want %v", got, tc.want)
			}
			if got := f.server.ToolNames(); len(got) != len(tc.want) {
				t.Errorf("ToolNames = %v, want %d tools", got, len(tc.want))
			}
		})
	}
}

func TestANewServerNeedsACardOrAChatAndNotBoth(t *testing.T) {
	f := newFixture(t)
	deps := Deps{Cards: f.cards, Notes: f.notes, Claims: f.claims, Agents: f.agents, Harness: f.rules}
	tests := []struct {
		name string
		id   Identity
		want string
	}{
		{"neither", Identity{ProjectID: projectID}, "a card id is required"},
		{"both", Identity{CardID: "c", ChatID: "x", ChatKind: ChatKindOther, ProjectID: projectID}, "not both"},
		{"a chat with no kind", Identity{ChatID: "x", ProjectID: projectID}, "needs a kind"},
		{"a chat with an unknown kind", Identity{ChatID: "x", ChatKind: "gardener", ProjectID: projectID}, "needs a kind"},
		{"a card with a chat kind", Identity{CardID: "c", ChatKind: ChatKindOther, ProjectID: projectID}, "no chat kind"},
		{"a chat with no project", Identity{ChatID: "x", ChatKind: ChatKindOther}, "a project id is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(deps, tc.id)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("New = %v, want an error saying %q", err, tc.want)
			}
		})
	}
}

func TestAChatServerNeedsNeitherNotesNorAgents(t *testing.T) {
	f := newFixture(t)
	_, err := New(Deps{Cards: f.cards, Claims: f.claims, Harness: f.rules},
		Identity{ChatID: "x", ChatKind: ChatKindOrchestrator, ProjectID: projectID})
	if err != nil {
		t.Fatalf("a chat's server without notes or agents: %v", err)
	}
	server, _ := New(Deps{Cards: f.cards, Claims: f.claims, Harness: f.rules},
		Identity{ChatID: "x", ChatKind: ChatKindOrchestrator, ProjectID: projectID})
	if slices.Contains(server.ToolNames(), "search_memory") {
		t.Errorf("tools = %v: search_memory needs the notes, so it is left out without them", server.ToolNames())
	}
	if server.ChatID() != "x" || server.CardID() != "" {
		t.Errorf("ids = %q / %q, want a chat's server", server.ChatID(), server.CardID())
	}
}

func TestAPlanOnlyOrchestratorChatStillReadsTheBoardAndCreatesCards(t *testing.T) {
	// Plan-only is about the owner's folder. The board is Marshal's own record, so the Orchestrator
	// can still plan on it: that is what the chat is for.
	f := newChatFixture(t, ChatKindOrchestrator, protocol.PermissionModePlan)
	cs := f.client(t)

	board := callOK[boardStatusOut](t, cs, "board_status", nil)
	if len(board.Cards) != 3 {
		t.Errorf("board_status listed %d cards, want the project's three (a chat has no card of its own to leave out)", len(board.Cards))
	}
	made := callOK[createCardOut](t, cs, "create_card", map[string]any{
		"title": "Export as CSV", "body": "Let people download the table.", "role": "Worker",
		"dependsOn": []string{projectID + "#3"},
	})
	if made.Key == "" || made.State != protocol.CardStateBacklog {
		t.Errorf("create_card = %+v, want a card in the backlog", made)
	}
	if len(f.cards.created) != 1 || f.cards.created[0].Title != "Export as CSV" {
		t.Errorf("created = %+v, want the card the chat planned", f.cards.created)
	}
}

func TestEveryModeLetsTheOrchestratorChatCreateCards(t *testing.T) {
	for _, mode := range []protocol.PermissionMode{
		protocol.PermissionModeAsk, protocol.PermissionModeAutoEdits, protocol.PermissionModePlan,
		protocol.PermissionModeFullAuto, protocol.PermissionModeBypass,
	} {
		t.Run(string(mode), func(t *testing.T) {
			f := newChatFixture(t, ChatKindOrchestrator, mode)
			callOK[createCardOut](t, f.client(t), "create_card", map[string]any{"title": "New work"})
			if len(f.cards.created) != 1 {
				t.Errorf("created %d cards in %s, want 1", len(f.cards.created), mode)
			}
		})
	}
}

func TestAChatServerRefusesInTheWordsOfAChatWhenItCannotReadItsMode(t *testing.T) {
	f := newChatFixture(t, ChatKindOther, protocol.PermissionModeAsk)
	f.noMode = true
	said := callRefused(t, f.client(t), "board_status", nil)
	if !strings.Contains(said, "this chat is in") || strings.Contains(said, "card") {
		t.Errorf("the refusal = %q, want it to speak of the chat and not a card", said)
	}
}

func TestAnotherChatsBoardStatusIsGatedByItsMode(t *testing.T) {
	// Reading is allowed in every careful mode, so the board is there for a plan-only chat too.
	f := newChatFixture(t, ChatKindOther, protocol.PermissionModePlan)
	callOK[boardStatusOut](t, f.client(t), "board_status", nil)
}

func TestTheChatRefusalNamesTheChatsOwner(t *testing.T) {
	said := refusalFor("chat", protocol.PermissionModePlan, "post_note", harness.Outcome{Decision: harness.DecisionDeny, Rule: harness.RuleMode})
	if !strings.Contains(said, "The chat's permission mode is plan") || !strings.Contains(said, "the chat's owner") {
		t.Errorf("refusalFor = %q", said)
	}
	if card := refusal(protocol.PermissionModePlan, "post_note", harness.Outcome{Decision: harness.DecisionDeny, Rule: harness.RuleMode}); !strings.Contains(card, "The card's permission mode") {
		t.Errorf("refusal for a card = %q, want it unchanged", card)
	}
}

func TestTheMergeToolsRouteToTheImplementationAndAnswerInTheAgentsOwnWords(t *testing.T) {
	f := newChatFixture(t, ChatKindIntegrator, protocol.PermissionModePlan)
	merge := &fakeMerge{task: integrator.MergeTask{
		ProjectID: projectID, Kind: integrator.MergeTaskCard, Worktree: "/work/integrator", Target: "development",
		Conflicts: []string{"src/a.go"},
		Cards:     []integrator.CardContext{{CardID: "card-1", Key: projectID + "#1", Title: "Add a health check", Branch: "marshal/1"}},
	}}
	f.merge = merge
	cs := f.client(t)

	view := callOK[integrator.MergeTaskView](t, cs, "merge_context", map[string]any{"task_id": "t-1"})
	if view.TaskID != "t-1" || view.Target != "development" || len(view.Conflicts) != 1 || len(view.Cards) != 1 || view.Cards[0].Key != projectID+"#1" {
		t.Errorf("merge_context = %+v", view)
	}

	reported := callOK[mergeReportOut](t, cs, "merge_report", map[string]any{
		"task_id": "t-1", "resolved": true, "confident": false, "summary": "kept both sides",
		"files": []string{"src/a.go"}, "questions": []string{"is the new flag meant to default on?"},
	})
	if !reported.Recorded || len(merge.reports) != 1 || merge.reportIDs[0] != "t-1" {
		t.Fatalf("merge_report = %+v, reports = %+v, want the verdict recorded for t-1", reported, merge.reports)
	}
	got := merge.reports[0]
	if !got.Resolved || got.Confident || got.Summary != "kept both sides" || len(got.Files) != 1 || len(got.Questions) != 1 {
		t.Errorf("verdict = %+v", got)
	}

	asked := callOK[askOwnerOut](t, cs, "ask_owner", map[string]any{"task_id": "t-1", "question": "Which flag wins?"})
	if !asked.Asked || len(merge.asks) != 1 || merge.asks[0] != "Which flag wins?" {
		t.Errorf("ask_owner = %+v, asks = %v", asked, merge.asks)
	}
}

func TestTheMergeToolsAreNotGatedByTheFolderMode(t *testing.T) {
	for _, mode := range []protocol.PermissionMode{protocol.PermissionModePlan, protocol.PermissionModeAsk} {
		f := newChatFixture(t, ChatKindIntegrator, mode)
		f.noMode = true
		f.merge = &fakeMerge{task: integrator.MergeTask{ProjectID: projectID}}
		cs := f.client(t)
		callOK[mergeReportOut](t, cs, "merge_report", map[string]any{"task_id": "t", "resolved": true, "confident": true})
		callOK[askOwnerOut](t, cs, "ask_owner", map[string]any{"task_id": "t", "question": "q?"})
	}
}

func TestTheMergeToolsSayTheyAreNotAvailableUntilSomethingImplementsThem(t *testing.T) {
	f := newChatFixture(t, ChatKindIntegrator, protocol.PermissionModeAutoEdits)
	cs := f.client(t)
	calls := map[string]map[string]any{
		"merge_context": {"task_id": "t-1"},
		"merge_report":  {"task_id": "t-1", "resolved": true, "confident": true},
		"ask_owner":     {"task_id": "t-1", "question": "q?"},
	}
	for name, args := range calls {
		if said := callRefused(t, cs, name, args); !strings.Contains(said, "the merge tools are not available") {
			t.Errorf("%s with nothing set said %q", name, said)
		}
	}
	// A getter that answers a nil interface is the same as none.
	f.merge = nil
	if said := callRefused(t, cs, "merge_context", calls["merge_context"]); !strings.Contains(said, "not available") {
		t.Errorf("a nil implementation said %q", said)
	}
	// It may be set after the server was built, and the next call finds it.
	f.merge = &fakeMerge{task: integrator.MergeTask{ProjectID: projectID}}
	callOK[integrator.MergeTaskView](t, cs, "merge_context", calls["merge_context"])
}

func TestAServerBuiltWithNoMergeToolsGetterAnswersThatTheyAreNotAvailable(t *testing.T) {
	f := newFixture(t)
	server, err := New(Deps{Cards: f.cards, Claims: f.claims, Harness: f.rules},
		Identity{ChatID: "x", ChatKind: ChatKindIntegrator, ProjectID: projectID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f.server = server
	if said := callRefused(t, f.client(t), "merge_context", map[string]any{"task_id": "t"}); !strings.Contains(said, "not available") {
		t.Errorf("said %q", said)
	}
}

func TestOneProjectsIntegratorCannotReadOrReportAnotherProjectsMerge(t *testing.T) {
	f := newChatFixture(t, ChatKindIntegrator, protocol.PermissionModeAutoEdits)
	merge := &fakeMerge{task: integrator.MergeTask{ProjectID: otherProject}}
	f.merge = merge
	cs := f.client(t)
	for name, args := range map[string]map[string]any{
		"merge_context": {"task_id": "t-9"},
		"merge_report":  {"task_id": "t-9", "resolved": true, "confident": true},
		"ask_owner":     {"task_id": "t-9", "question": "q?"},
	} {
		if said := callRefused(t, cs, name, args); !strings.Contains(said, "another project") {
			t.Errorf("%s said %q, want a refusal to reach across projects", name, said)
		}
	}
	if len(merge.reports) != 0 || len(merge.asks) != 0 {
		t.Errorf("a refused call still reached the merge: reports %v, asks %v", merge.reports, merge.asks)
	}
}

func TestTheMergeToolsPassTheImplementationsErrorOnToTheAgent(t *testing.T) {
	f := newChatFixture(t, ChatKindIntegrator, protocol.PermissionModeAutoEdits)
	merge := &fakeMerge{task: integrator.MergeTask{ProjectID: projectID}, reportErr: errors.New("this task already has a verdict")}
	f.merge = merge
	said := callRefused(t, f.client(t), "merge_report", map[string]any{"task_id": "t", "resolved": true, "confident": true})
	if !strings.Contains(said, "already has a verdict") {
		t.Errorf("said %q, want the implementation's own sentence", said)
	}
	merge.taskErr = errors.New("no task t is waiting")
	if said := callRefused(t, f.client(t), "merge_context", map[string]any{"task_id": "t"}); !strings.Contains(said, "no task t is waiting") {
		t.Errorf("said %q", said)
	}
}

func TestEachKindsServerIntroducesItselfInItsOwnWords(t *testing.T) {
	want := map[ChatKind]string{
		ChatKindOrchestrator: "Orchestrator chat",
		ChatKindIntegrator:   "the Integrator",
		ChatKindOther:        "this chat",
	}
	for kind, phrase := range want {
		f := newChatFixture(t, kind, protocol.PermissionModeAutoEdits)
		cs := f.client(t)
		if got := cs.InitializeResult().Instructions; !strings.Contains(got, phrase) {
			t.Errorf("%s: instructions = %q, want them to mention %q", kind, got, phrase)
		}
	}
}

func TestAChatServerIsHostedUnderTheChatId(t *testing.T) {
	f := newChatFixture(t, ChatKindOrchestrator, protocol.PermissionModePlan)
	host := NewHost()
	secret, err := host.Add(f.server.identity.key(), f.server)
	if err != nil || secret == "" {
		t.Fatalf("Add = %q, %v", secret, err)
	}
	if !host.Has("chat-1") {
		t.Error("the host does not hold the chat's server under its chat id")
	}
	host.Remove("chat-1")
	if host.Has("chat-1") {
		t.Error("the chat's server is still hosted after Remove")
	}
}
