package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestBoardStatusDescribesTheOtherCards(t *testing.T) {
	f := newFixture(t)
	out := callOK[boardStatusOut](t, f.client(t), "board_status", map[string]any{})

	if out.ProjectID != projectID {
		t.Errorf("the answer is about project %q, want %q", out.ProjectID, projectID)
	}
	if got, want := len(out.Cards), 2; got != want {
		t.Fatalf("the answer has %d cards, want %d: %+v", got, want, out.Cards)
	}
	for _, card := range out.Cards {
		if card.Key == projectID+"#1" {
			t.Errorf("the answer includes the asking card: %+v", card)
		}
	}
	board := out.Cards[0]
	if board.Key != projectID+"#2" || board.Title != "Ship the board" {
		t.Errorf("the first other card is %+v", board)
	}
	if board.Owner != "Reviewer" {
		t.Errorf("the card's owner is %q, want its role %q", board.Owner, "Reviewer")
	}
	if board.Goal != "The board should show claims." {
		t.Errorf("the card's goal is %q", board.Goal)
	}
	if board.DoingNow != "reading the diff" {
		t.Errorf("the card's doing-now line is %q", board.DoingNow)
	}
	if len(board.Claims) != 1 || board.Claims[0] != "src/board.ts" {
		t.Errorf("the card's claims are %q, want src/board.ts", board.Claims)
	}
	// The card with no description has no goal, and its owner is its agent program because it has no
	// role.
	third := out.Cards[1]
	if third.Goal != "" {
		t.Errorf("a card with no description has goal %q", third.Goal)
	}
	if third.Owner != string(protocol.AgentKindGemini) {
		t.Errorf("a card with no role is owned by %q, want its agent %q", third.Owner, protocol.AgentKindGemini)
	}
	if !strings.Contains(out.Summary, "2 other cards") {
		t.Errorf("the summary is %q, which does not count the other cards", out.Summary)
	}
	if !strings.Contains(out.Summary, "backlog") || !strings.Contains(out.Summary, "working") {
		t.Errorf("the summary is %q, which does not count the states", out.Summary)
	}
}

func TestBoardStatusCountsAndCutsALongBoard(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < boardCardLimit+5; i++ {
		f.cards.add(protocol.Card{
			ID: fmt.Sprintf("extra-%d", i), ProjectID: projectID, Number: 50 + i,
			Key:   protocol.CardKey{ProjectID: projectID, Number: 50 + i}.String(),
			Title: "More work", State: protocol.CardStateBacklog, Agent: protocol.AgentKindClaude,
		})
	}
	out := callOK[boardStatusOut](t, f.client(t), "board_status", map[string]any{})
	if !out.Truncated {
		t.Error("a board longer than the limit was not reported as cut")
	}
	if len(out.Cards) != boardCardLimit {
		t.Errorf("the answer has %d cards, want %d", len(out.Cards), boardCardLimit)
	}
	if !strings.Contains(out.Summary, fmt.Sprintf("%d other cards", boardCardLimit+7)) {
		t.Errorf("the summary is %q, which does not count the whole board", out.Summary)
	}
	if !strings.Contains(out.Summary, fmt.Sprintf("first %d", boardCardLimit)) {
		t.Errorf("the summary is %q, which does not say the list was cut", out.Summary)
	}
}

func TestBoardStatusOnAnEmptyBoardSaysSo(t *testing.T) {
	f := newFixture(t)
	f.cards.cards = []protocol.Card{{
		ID: "card-1", ProjectID: projectID, Number: 1, Key: projectID + "#1", Title: "Alone",
	}}
	out := callOK[boardStatusOut](t, f.client(t), "board_status", map[string]any{})
	if !strings.Contains(out.Summary, "No other cards") {
		t.Errorf("the summary of an empty board is %q", out.Summary)
	}
	if len(out.Cards) != 0 {
		t.Errorf("an empty board answered with %+v", out.Cards)
	}
	if out.Cards == nil {
		t.Error("the answer is null rather than an empty list")
	}
}

// The board carries the plan: which other card waits for which, so a card reading the board sees the
// order the work has to be done in and not only what is being done (migration 0020, task 7.4).
func TestBoardStatusCarriesWhatEachCardWaitsFor(t *testing.T) {
	f := newFixture(t)
	f.cards.add(protocol.Card{
		ID: "card-4", ProjectID: projectID, Number: 4, Key: projectID + "#4",
		Title: "On its own", State: protocol.CardStateBacklog, Agent: protocol.AgentKindClaude,
	})
	// Named out of order on purpose: the answer is in number order, so the plan reads back the way it
	// was numbered however it was written.
	f.cards.addDep("card-3", key(3), key(2))
	f.cards.addDep("card-2", key(3))
	out := callOK[boardStatusOut](t, f.client(t), "board_status", map[string]any{})

	byKey := map[string]boardCard{}
	for _, card := range out.Cards {
		byKey[card.Key] = card
	}
	third, ok := byKey[projectID+"#3"]
	if !ok {
		t.Fatalf("the board has no %s card: %+v", projectID+"#3", out.Cards)
	}
	want := []string{projectID + "#2", projectID + "#3"}
	if len(third.DependsOn) != len(want) {
		t.Fatalf("card 3 waits for %q, want %q", third.DependsOn, want)
	}
	for i := range want {
		if third.DependsOn[i] != want[i] {
			t.Fatalf("card 3 waits for %q, want %q", third.DependsOn, want)
		}
	}
	if got := byKey[projectID+"#2"].DependsOn; len(got) != 1 || got[0] != projectID+"#3" {
		t.Errorf("card 2 waits for %q, want [%s]", got, projectID+"#3")
	}
	// A card that waits for nothing says nothing about dependencies, rather than carrying an empty
	// list a reader has to tell from a missing one.
	if got := byKey[projectID+"#4"].DependsOn; got != nil {
		t.Errorf("card 4 waits for nothing and answered with %q", got)
	}
}

// A board whose dependencies cannot be read is a board without them, not a failed call: what a card
// is doing matters more than the order it is in, and the failure is left in the log.
func TestBoardStatusKeepsTheBoardWhenTheDependenciesCannotBeRead(t *testing.T) {
	f := newFixture(t)
	f.cards.depsErr = errors.New("the edge table is unreadable")
	out := callOK[boardStatusOut](t, f.client(t), "board_status", map[string]any{})
	if len(out.Cards) != 2 {
		t.Fatalf("the board answered with %d cards, want both: %+v", len(out.Cards), out.Cards)
	}
	for _, card := range out.Cards {
		if card.DependsOn != nil {
			t.Errorf("card %s answered with dependencies %q it could not read", card.Key, card.DependsOn)
		}
	}
	if said := f.logs.String(); !strings.Contains(said, "could not read the board's dependencies") {
		t.Errorf("the failure is not in the log: %q", said)
	}
}

// key is a card key in the fixture's project.
func key(number int) protocol.CardKey {
	return protocol.CardKey{ProjectID: projectID, Number: number}
}

func TestClaimFilesRecordsTheClaimAndWarnsAboutAnOverlap(t *testing.T) {
	f := newFixture(t)
	f.claims.conflicts = []memory.Conflict{{
		PathOrPackage: "src/health.go", CardID: "card-2", ClaimedAt: fixtureNoteTime,
	}}
	out := callOK[claimFilesOut](t, f.client(t), "claim_files",
		map[string]any{"paths": []string{"src/health.go", "src/router.ts"}})

	if len(out.Claims) != 2 {
		t.Errorf("the answer claims %q, want both paths", out.Claims)
	}
	if len(out.Warnings) != 1 {
		t.Fatalf("the answer has %d warnings, want 1: %q", len(out.Warnings), out.Warnings)
	}
	for _, want := range []string{projectID + "#2", "Ship the board", "src/health.go", "not a lock"} {
		if !strings.Contains(out.Warnings[0], want) {
			t.Errorf("the warning %q does not say %q", out.Warnings[0], want)
		}
	}
	if len(f.claims.held["card-1"]) != 2 {
		t.Errorf("the claims recorded are %+v", f.claims.held["card-1"])
	}
}

func TestClaimFilesWithNoOverlapSaysNothingAboutOne(t *testing.T) {
	f := newFixture(t)
	out := callOK[claimFilesOut](t, f.client(t), "claim_files", map[string]any{"paths": []string{"a.go"}})
	if len(out.Warnings) != 0 {
		t.Errorf("an unobstructed claim answered with warnings: %q", out.Warnings)
	}
}

// A refusal the memory module makes itself - a path that climbs out of the repository, say - reaches
// the model as the sentence it is, rather than being swallowed or turned into "no".
func TestClaimFilesPassesOnWhatTheMemoryModuleRefused(t *testing.T) {
	f := newFixture(t)
	f.claims.err = protocol.InvalidArgument("a claim must name a path inside the repository").
		With("path", "../secrets")
	said, isError := call(t, f.client(t), "claim_files", map[string]any{"paths": []string{"../secrets"}})
	if !isError {
		t.Fatalf("claiming a path outside the repository was allowed: %s", said)
	}
	if !strings.Contains(said, "inside the repository") {
		t.Errorf("the refusal is %q, which loses what the memory module said", said)
	}
	if len(f.claims.held["card-1"]) != 0 {
		t.Errorf("the claim was recorded anyway: %+v", f.claims.held["card-1"])
	}
}

func TestReleaseFilesAnswersWhatIsLeft(t *testing.T) {
	f := newFixture(t)
	f.claims.held["card-1"] = []memory.Claim{
		{CardID: "card-1", ProjectID: projectID, PathOrPackage: "a.go"},
		{CardID: "card-1", ProjectID: projectID, PathOrPackage: "b.go"},
	}
	out := callOK[releaseFilesOut](t, f.client(t), "release_files", map[string]any{"paths": []string{"a.go"}})
	if len(out.Released) != 1 || out.Released[0] != "a.go" {
		t.Errorf("the answer released %q", out.Released)
	}
	if len(out.Remaining) != 1 || out.Remaining[0] != "b.go" {
		t.Errorf("the answer left %q, want b.go", out.Remaining)
	}
}

func TestPostNoteWritesTheCardsOwnNoteAsTheAgent(t *testing.T) {
	f := newFixture(t)
	out := callOK[noteOut](t, f.client(t), "post_note", map[string]any{"body": "I added the route."})

	if out.Body != "I added the route." {
		t.Errorf("the note reads %q", out.Body)
	}
	if out.Author != protocol.NoteAuthorAgent {
		t.Errorf("the note is written by %q, want %q", out.Author, protocol.NoteAuthorAgent)
	}
	if out.CardKey != projectID+"#1" {
		t.Errorf("the note is on card %q, want the asking card", out.CardKey)
	}
	if out.Path != notePathOf("card-1") {
		t.Errorf("the note lives at %q", out.Path)
	}
	if out.UpdatedAt == "" {
		t.Error("a saved note has no time")
	}
	if got := f.notes.saved["card-1"]; got != "I added the route." {
		t.Errorf("the note written was %q", got)
	}
}

// Reading a card nothing has been saved for answers the note Marshal would start one from, with no
// time at all - and it does not write one.
func TestReadNotesReadsTheCardWithoutWritingOne(t *testing.T) {
	f := newFixture(t)
	out := callOK[noteOut](t, f.client(t), "read_notes", map[string]any{})
	if out.Body == "" {
		t.Error("an unsaved note answered with nothing")
	}
	if out.UpdatedAt != "" {
		t.Errorf("an unsaved note answered with the time %q", out.UpdatedAt)
	}
	if len(f.notes.saved) != 0 {
		t.Errorf("reading a note saved one: %+v", f.notes.saved)
	}
}

func TestReadNotesReadsAnotherCardsNoteByKey(t *testing.T) {
	f := newFixture(t)
	f.notes.saved = map[string]string{"card-2": "The board needs the claim column."}
	f.notes.authors = map[string]protocol.NoteAuthor{"card-2": protocol.NoteAuthorAgent}

	out := callOK[noteOut](t, f.client(t), "read_notes", map[string]any{"cardKey": projectID + "#2"})
	if out.CardKey != projectID+"#2" {
		t.Errorf("the note read is on card %q", out.CardKey)
	}
	if out.Body != "The board needs the claim column." {
		t.Errorf("the note read is %q", out.Body)
	}
}

func TestReadNotesRefusesACardInAnotherProject(t *testing.T) {
	f := newFixture(t)
	said := callRefused(t, f.client(t), "read_notes", map[string]any{"cardKey": otherProject + "#1"})
	if !strings.Contains(said, "another project") {
		t.Errorf("the refusal is %q, which does not say why", said)
	}
}

func TestReadNotesRefusesAKeyThatIsNotOne(t *testing.T) {
	f := newFixture(t)
	if _, isError := call(t, f.client(t), "read_notes", map[string]any{"cardKey": "nonsense"}); !isError {
		t.Error("a card key that is not one was accepted")
	}
}

func TestSearchMemoryAnswersAnExcerptOfEachHit(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("route ", 100)
	f.notes.index = []protocol.Note{
		protocol.NewNote("card-2", projectID, notePathOf("card-2"), long,
			protocol.NoteAuthorAgent, fixtureNoteTime),
	}
	out := callOK[searchMemoryOut](t, f.client(t), "search_memory", map[string]any{"query": "route"})

	if out.Query != "route" {
		t.Errorf("the answer says it searched for %q", out.Query)
	}
	if len(out.Notes) != 1 {
		t.Fatalf("the answer has %d notes, want 1", len(out.Notes))
	}
	hit := out.Notes[0]
	if hit.CardKey != projectID+"#2" {
		t.Errorf("the hit names card %q", hit.CardKey)
	}
	if len([]rune(hit.Excerpt)) > noteExcerptRun+1 {
		t.Errorf("the excerpt is %d runes long, want at most %d", len([]rune(hit.Excerpt)), noteExcerptRun)
	}
	if !strings.HasSuffix(hit.Excerpt, ellipsis) {
		t.Errorf("the excerpt %q does not say it was cut", hit.Excerpt)
	}
	if out.Searched == "" {
		t.Error("the answer does not say what it searched")
	}
}

func TestSearchMemoryWithNoWordsSearchesNothing(t *testing.T) {
	f := newFixture(t)
	out := callOK[searchMemoryOut](t, f.client(t), "search_memory", map[string]any{"query": "   "})
	if len(out.Notes) != 0 {
		t.Errorf("a blank query answered with %+v", out.Notes)
	}
	if len(f.notes.queries) != 0 {
		t.Errorf("a blank query was passed on: %q", f.notes.queries)
	}
	if out.Notes == nil {
		t.Error("the answer is null rather than an empty list")
	}
	if out.Lessons == nil {
		t.Error("the lessons answer is null rather than an empty list")
	}
}

// search_memory searches a project's lessons alongside its cards' notes (task 7.6), so an agent
// that asks it one question gets both without knowing the project has learned anything at all.
func TestSearchMemoryAnswersLessonsToo(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("flaky ", 100)
	f.notes.lessonIndex = []protocol.Lesson{
		{
			ProjectID: projectID, Slug: "ci-is-flaky", Title: "CI is flaky",
			Path: projectID + "/lessons/ci-is-flaky.md", Body: long,
			Author: protocol.NoteAuthorAgent, UpdatedAt: protocol.NewTimestamp(fixtureNoteTime),
		},
	}
	out := callOK[searchMemoryOut](t, f.client(t), "search_memory", map[string]any{"query": "flaky"})

	if len(out.Lessons) != 1 {
		t.Fatalf("the answer has %d lessons, want 1", len(out.Lessons))
	}
	hit := out.Lessons[0]
	if hit.Title != "CI is flaky" {
		t.Errorf("the hit names %q", hit.Title)
	}
	if !strings.HasSuffix(hit.Excerpt, ellipsis) {
		t.Errorf("the excerpt %q does not say it was cut", hit.Excerpt)
	}
}

func TestSearchCodebaseRefusesWhileTheMapIsUnbuilt(t *testing.T) {
	f := newFixture(t)
	// A server built without a map at all. The daemon gives every session the real one
	// (build-plan task 7.9), so this is the defensive path - and it refuses rather than answering
	// with no matches, which would read as "that name is not in this project".
	f.server.deps.Codebase = nil
	said := callRefused(t, f.client(t), "search_codebase", map[string]any{"query": "Session"})
	if !strings.Contains(said, "not built the codebase map") {
		t.Errorf("the refusal is %q", said)
	}
	if !strings.Contains(said, "not a permission refusal") {
		t.Errorf("the refusal does not rule out a permission problem: %q", said)
	}
}

func TestSearchCodebaseAnswersTheMapsMatches(t *testing.T) {
	f := newFixture(t)
	f.code.matches = []CodeMatch{
		{Path: "daemon/internal/session/send.go", Line: 27, Kind: "function", Name: "Send", Parent: "Manager"},
	}
	out := callOK[searchCodebaseOut](t, f.client(t), "search_codebase", map[string]any{"query": "Send"})
	if len(out.Matches) != 1 {
		t.Fatalf("the answer has %d matches, want 1", len(out.Matches))
	}
	if out.Matches[0].Path != "daemon/internal/session/send.go" || out.Matches[0].Line != 27 {
		t.Errorf("the match is %+v", out.Matches[0])
	}
	if len(f.code.queried) != 1 || f.code.queried[0] != "Send" {
		t.Errorf("the map was asked for %q", f.code.queried)
	}
}

// TestSearchCodebaseSaysWhenTheMapIsBlind: a machine with no universal ctags still gets an answer,
// and the answer says which kind of answer it is. Without this the agent reads a file-name match as
// a symbol match, or reads no matches as "that name is not here".
func TestSearchCodebaseSaysWhenTheMapIsBlind(t *testing.T) {
	f := newFixture(t)
	f.code.notice = "Marshal cannot read symbols on this machine, because universal ctags is not installed"
	f.code.matches = []CodeMatch{{Path: "daemon/internal/session/send.go", Kind: "file", Name: "send.go"}}

	out := callOK[searchCodebaseOut](t, f.client(t), "search_codebase", map[string]any{"query": "send"})
	if out.Notice != f.code.notice {
		t.Errorf("the answer's notice is %q, want the map's %q", out.Notice, f.code.notice)
	}
	if len(out.Matches) != 1 {
		t.Fatalf("the answer has %d matches, want the file the map could still see", len(out.Matches))
	}
}

// TestSearchCodebaseIsQuietWhenTheMapCanReadSymbols: the ordinary answer carries no notice, so a
// notice in an answer always means something.
func TestSearchCodebaseIsQuietWhenTheMapCanReadSymbols(t *testing.T) {
	f := newFixture(t)
	f.code.matches = []CodeMatch{{Path: "daemon/internal/session/send.go", Line: 27, Kind: "method", Name: "Send"}}

	out := callOK[searchCodebaseOut](t, f.client(t), "search_codebase", map[string]any{"query": "Send"})
	if out.Notice != "" {
		t.Errorf("the answer carries the notice %q although the map can read symbols", out.Notice)
	}
}

func TestSearchCodebasePassesOnAMapThatFails(t *testing.T) {
	f := newFixture(t)
	f.code.err = errors.New("ctags is not installed")
	said, isError := call(t, f.client(t), "search_codebase", map[string]any{"query": "Send"})
	if !isError {
		t.Fatal("a map that failed answered as if it had found nothing")
	}
	if !strings.Contains(said, "ctags is not installed") {
		t.Errorf("the failure is reported as %q, which loses what the map said", said)
	}
}

func TestReportProgressSetsTheCardsOwnLine(t *testing.T) {
	f := newFixture(t)
	out := callOK[reportProgressOut](t, f.client(t), "report_progress",
		map[string]any{"doingNow": "writing the route"})
	if out.DoingNow != "writing the route" {
		t.Errorf("the card's line is %q", out.DoingNow)
	}
	if out.Key != projectID+"#1" {
		t.Errorf("the answer names card %q", out.Key)
	}
	if len(f.cards.updated) != 1 || f.cards.updated[0].DoingNow == nil ||
		*f.cards.updated[0].DoingNow != "writing the route" {
		t.Errorf("the update sent was %+v", f.cards.updated)
	}
}

func TestReportProgressWithNoWordsClearsTheLine(t *testing.T) {
	f := newFixture(t)
	out := callOK[reportProgressOut](t, f.client(t), "report_progress", map[string]any{"doingNow": ""})
	if out.DoingNow != "" {
		t.Errorf("the line is %q, want it cleared", out.DoingNow)
	}
}

func TestAskAgentNamesTheAskerAndWhereTheAnswerStays(t *testing.T) {
	f := newFixture(t)
	out := callOK[askAgentOut](t, f.client(t), "ask_agent",
		map[string]any{"cardKey": projectID + "#2", "question": "Are you editing src/board.ts?"})

	if !out.Sent {
		t.Error("the question was not sent")
	}
	if out.CardKey != projectID+"#2" || out.Title != "Ship the board" {
		t.Errorf("the answer names %+v", out)
	}
	if len(f.agents.sent) != 1 {
		t.Fatalf("%d questions were sent, want 1", len(f.agents.sent))
	}
	sent := f.agents.sent[0]
	if sent.cardID != "card-2" {
		t.Errorf("the question went to %q", sent.cardID)
	}
	for _, want := range []string{"card " + projectID + "#1", "Add a health check", "Are you editing src/board.ts?", "does not carry"} {
		if !strings.Contains(sent.text, want) {
			t.Errorf("the question as sent (%q) does not say %q", sent.text, want)
		}
	}
}

func TestAskAgentRefusesItsOwnCardAndAnotherProject(t *testing.T) {
	f := newFixture(t)
	cs := f.client(t)
	mine := callRefused(t, cs, "ask_agent", map[string]any{"cardKey": projectID + "#1", "question": "hello?"})
	if !strings.Contains(mine, "this card") {
		t.Errorf("asking yourself is refused as %q", mine)
	}
	elsewhere := callRefused(t, cs, "ask_agent",
		map[string]any{"cardKey": otherProject + "#1", "question": "hello?"})
	if !strings.Contains(elsewhere, "another project") {
		t.Errorf("asking across projects is refused as %q", elsewhere)
	}
	if len(f.agents.sent) != 0 {
		t.Errorf("a question was sent anyway: %+v", f.agents.sent)
	}
}

func TestAskAgentNeedsAQuestion(t *testing.T) {
	f := newFixture(t)
	if _, isError := call(t, f.client(t), "ask_agent",
		map[string]any{"cardKey": projectID + "#2", "question": "  "}); !isError {
		t.Error("an empty question was sent")
	}
}

func TestCreateCardNeedsATitle(t *testing.T) {
	f := newFixture(t)
	f.mode = protocol.PermissionModeFullAuto
	if _, isError := call(t, f.client(t), "create_card", map[string]any{"title": " "}); !isError {
		t.Error("a card with no title was created")
	}
	if len(f.cards.created) != 0 {
		t.Errorf("a card was created anyway: %+v", f.cards.created)
	}
}

// create_card answers with the card it made and the plan that card carries, in number order, so the
// agent building a plan one card at a time can read back each key it just named and use it in the
// next card (docs/marshal-product-scope.md section 10.2, build-plan task 7.4).
func TestCreateCardCarriesThePlanItWasGiven(t *testing.T) {
	f := newFixture(t)
	f.mode = protocol.PermissionModeFullAuto
	out := callOK[createCardOut](t, f.client(t), "create_card", map[string]any{
		"title": "Ship it",
		// Out of order, with one repeated: the answer is the plan, in number order, with one edge per
		// card.
		"dependsOn": []string{projectID + "#3", projectID + "#2", projectID + "#3"},
	})
	if out.Key == "" || out.Title != "Ship it" || out.State != protocol.CardStateBacklog {
		t.Errorf("the answer is %+v, want the card that was made", out)
	}
	want := []string{projectID + "#2", projectID + "#3"}
	if len(out.DependsOn) != len(want) {
		t.Fatalf("the answer names %q, want %q", out.DependsOn, want)
	}
	for i := range want {
		if out.DependsOn[i] != want[i] {
			t.Fatalf("the answer names %q, want %q", out.DependsOn, want)
		}
	}
	if len(f.cards.created) != 1 || f.cards.created[0].Title != "Ship it" {
		t.Errorf("the card that was created is %+v", f.cards.created)
	}
}

// A card that waits for nothing says nothing about dependencies.
func TestCreateCardWithoutAPlanNamesNone(t *testing.T) {
	f := newFixture(t)
	f.mode = protocol.PermissionModeFullAuto
	out := callOK[createCardOut](t, f.client(t), "create_card", map[string]any{"title": "Ship it"})
	if out.DependsOn != nil {
		t.Errorf("a card made with no plan answered with %q", out.DependsOn)
	}
}

// A key the tool cannot read refuses the whole call, before any card is made: a plan the tool cannot
// read is a mistake, not a plan.
func TestCreateCardRefusesAKeyItCannotRead(t *testing.T) {
	tests := []struct {
		name string
		key  string
		says string
	}{
		{"no number", projectID + "#", "the number must start at 1"},
		{"a number that is not a number", projectID + "#two", "invalid syntax"},
		{"a number starting at zero", projectID + "#0", "the number must start at 1"},
		{"no project or number", "not-a-key", "want <project id>#<number>"},
		{"a project id that is not one", "Small Repo#1", "want <project id>#<number>"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.mode = protocol.PermissionModeFullAuto
			said := callRefused(t, f.client(t), "create_card", map[string]any{
				"title": "Ship it", "dependsOn": []string{test.key},
			})
			for _, want := range []string{"dependsOn", test.key, test.says} {
				if !strings.Contains(said, want) {
					t.Errorf("the refusal %q does not say %q", said, want)
				}
			}
			if len(f.cards.created) != 0 {
				t.Errorf("a card was created anyway: %+v", f.cards.created)
			}
		})
	}
}

// Every tool whose part of Marshal is not built yet answers with a sentence that says what is
// missing, rules out a permission problem, and says what to do instead.
func TestTheToolsWhosePartIsNotBuiltSaySo(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
		want string
		says string
	}{
		{"list_checklists", map[string]any{}, "checklists", "does not keep checklists"},
		{"tick_checklist_item", map[string]any{"itemId": "item-1", "done": true, "evidence": "c"}, "checklists", "does not keep checklists"},
		{"read_comments", map[string]any{}, "comments", "does not keep comments"},
		{"post_comment", map[string]any{"body": "done"}, "comments", "does not keep comments"},
		{"read_attachment", map[string]any{"path": "notes.txt"}, "attachments", "does not hold attachments"},
	}
	for _, test := range tests {
		t.Run(test.tool, func(t *testing.T) {
			f := newFixture(t)
			said := callRefused(t, f.client(t), test.tool, test.args)
			for _, want := range []string{test.says, test.want, "not a permission refusal"} {
				if !strings.Contains(said, want) {
					t.Errorf("the answer %q does not say %q", said, want)
				}
			}
		})
	}
}

// The arguments and the answer of a tool that is not built yet are described to a client all the
// same, so the tool the agents already know does not change shape when the work lands.
func TestAToolThatIsNotBuiltIsStillDescribed(t *testing.T) {
	f := newFixture(t)
	cs := f.client(t)
	listed, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list the tools: %v", err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != "tick_checklist_item" {
			continue
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("read the argument schema: %v", err)
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("read the argument schema as JSON: %v", err)
		}
		for _, property := range []string{"itemId", "done", "evidence"} {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("the argument schema has no %q: %s", property, raw)
			}
		}
		if len(schema.Required) != 2 {
			t.Errorf("the required arguments are %q, want itemId and done", schema.Required)
		}
		return
	}
	t.Fatal("tick_checklist_item is not offered to a client")
}
