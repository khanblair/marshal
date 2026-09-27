package search_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/search"
)

// The search answers over what the projects, chats, and memory services give it, so these tests
// give it small fakes of them and read the answers. The real services behind the real route are in
// internal/api's routes_search_test.go.

var base = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// fakeProjects is the projects the search reads: a list, and each project's cards.
type fakeProjects struct {
	projects  []protocol.Project
	cards     map[string][]protocol.Card
	cardsErr  map[string]error // what Cards answers for a project, instead of its cards
	listErr   error
	listCalls int
}

func (f *fakeProjects) List(context.Context) (protocol.ProjectListSnapshot, error) {
	f.listCalls++
	return protocol.ProjectListSnapshot{Projects: f.projects}, f.listErr
}

func (f *fakeProjects) Cards(_ context.Context, projectID string) ([]protocol.Card, error) {
	if err := f.cardsErr[projectID]; err != nil {
		return nil, err
	}
	return f.cards[projectID], nil
}

// fakeChats is the chats the search reads, and which halves of the list it was asked for.
type fakeChats struct {
	chats    map[string][]protocol.Chat
	archived []bool
}

func (f *fakeChats) List(_ context.Context, projectID string, archived bool) (protocol.ChatListSnapshot, error) {
	f.archived = append(f.archived, archived)
	return protocol.ChatListSnapshot{ProjectID: projectID, Chats: f.chats[projectID]}, nil
}

// fakeSessions is the past session events the search reads through the memory module, keyed by
// project, and the queries it was asked for.
type fakeSessions struct {
	hits    map[string][]protocol.SessionHit
	err     error
	queries []string
}

func (f *fakeSessions) SearchSessions(_ context.Context, projectID, query string, _ int) ([]protocol.SessionHit, error) {
	f.queries = append(f.queries, query)
	if f.err != nil {
		return nil, f.err
	}
	return f.hits[projectID], nil
}

// fakeNotes is the card notes the search reads through the memory module, keyed by project.
type fakeNotes struct {
	notes map[string][]protocol.Note
	err   error
}

func (f *fakeNotes) SearchNotes(_ context.Context, projectID, query string, _ int) ([]protocol.Note, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.notes[projectID], nil
}

// world is the fakes and the service over them.
type world struct {
	projects *fakeProjects
	chats    *fakeChats
	sessions *fakeSessions
	notes    *fakeNotes
	svc      *search.Service
}

func newWorld(t *testing.T, projects []protocol.Project) *world {
	t.Helper()
	w := &world{
		projects: &fakeProjects{
			projects: projects, cards: map[string][]protocol.Card{}, cardsErr: map[string]error{},
		},
		chats:    &fakeChats{chats: map[string][]protocol.Chat{}},
		sessions: &fakeSessions{hits: map[string][]protocol.SessionHit{}},
		notes:    &fakeNotes{notes: map[string][]protocol.Note{}},
	}
	svc, err := search.New(search.Deps{
		Projects: w.projects, Chats: w.chats, Sessions: w.sessions, Notes: w.notes,
	}, search.WithClock(func() time.Time { return base }))
	if err != nil {
		t.Fatal(err)
	}
	w.svc = svc
	return w
}

// project makes a project with a name and a folder, and the language the palette shows.
func project(id, name, path string) protocol.Project {
	return protocol.Project{ID: id, Name: name, Path: path, Language: "Go"}
}

// card adds a card to a project. The minute says how recently it changed, later meaning newer.
func (w *world) card(projectID string, number int, title, body string, minute int) protocol.Card {
	c := protocol.Card{
		ID: fmt.Sprintf("card-%s-%d", projectID, number), ProjectID: projectID, Number: number,
		Key: fmt.Sprintf("%s#%d", projectID, number), Title: title, Body: body,
		State:     protocol.CardStateBacklog,
		UpdatedAt: protocol.NewTimestamp(base.Add(time.Duration(minute) * time.Minute)),
	}
	w.projects.cards[projectID] = append(w.projects.cards[projectID], c)
	return c
}

// chat adds a live chat to a project.
func (w *world) chat(projectID, id, title string, minute int) {
	w.chats.chats[projectID] = append(w.chats.chats[projectID], protocol.Chat{
		ID: id, ProjectID: projectID, Title: title,
		LastActiveAt: protocol.NewTimestamp(base.Add(time.Duration(minute) * time.Minute)),
	})
}

// search runs a query and fails the test on an error.
func (w *world) search(t *testing.T, query string) protocol.SearchSnapshot {
	t.Helper()
	got, err := w.svc.Search(context.Background(), query)
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	return got
}

// cardKeys lists the keys of the cards of an answer, in the order they were ranked.
func cardKeys(s protocol.SearchSnapshot) []string {
	keys := make([]string, len(s.Cards))
	for i, c := range s.Cards {
		keys[i] = c.Key
	}
	return keys
}

func wantKeys(t *testing.T, s protocol.SearchSnapshot, want ...string) {
	t.Helper()
	if got := cardKeys(s); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("cards for %q = %v, want %v", s.Query, got, want)
	}
}

// Each kind is found by what the palette shows for it, and each hit carries what opens it and
// the project it belongs to.
func TestEachKindIsFoundWithWhatOpensIt(t *testing.T) {
	w := newWorld(t, []protocol.Project{
		project("api", "api-gateway", "/home/ada/code/api-gateway"),
		project("web", "web-dashboard", "/home/ada/code/web-dashboard"),
	})
	w.card("api", 41, "Refresh the token before it expires", "", 5)
	w.card("web", 7, "Show the session timeout", "The token lives for an hour.", 1)
	w.card("web", 8, "Unrelated", "", 1)
	w.chat("api", "chat-1", "Token rotation question", 3)
	w.chat("web", "chat-2", "Unrelated chat", 2)

	got := w.search(t, "token")
	if len(got.Projects) != 0 {
		t.Errorf("projects = %+v, want none", got.Projects)
	}
	wantKeys(t, got, "api#41", "web#7")
	first := got.Cards[0]
	if first.CardID != "card-api-41" || first.Number != 41 || first.Title != "Refresh the token before it expires" ||
		first.State != protocol.CardStateBacklog || first.ProjectID != "api" || first.ProjectName != "api-gateway" {
		t.Errorf("card hit = %+v", first)
	}
	if len(got.Chats) != 1 || got.Chats[0].ChatID != "chat-1" || got.Chats[0].ProjectID != "api" ||
		got.Chats[0].ProjectName != "api-gateway" || got.Chats[0].Title != "Token rotation question" ||
		got.Chats[0].LastActiveAt != protocol.NewTimestamp(base.Add(3*time.Minute)) {
		t.Errorf("chat hits = %+v", got.Chats)
	}
	if got.Totals != (protocol.SearchTotals{Projects: 0, Cards: 2, Chats: 1}) {
		t.Errorf("totals = %+v", got.Totals)
	}
	if got.Query != "token" || got.ServerTime != protocol.NewTimestamp(base) {
		t.Errorf("query = %q, time = %v", got.Query, got.ServerTime)
	}

	// A project is found by its name, and by its folder.
	byName := w.search(t, "dashboard")
	if len(byName.Projects) != 1 || byName.Projects[0] != (protocol.ProjectHit{
		ProjectID: "web", Name: "web-dashboard", Path: "/home/ada/code/web-dashboard", Language: "Go",
	}) {
		t.Errorf("projects for a name = %+v", byName.Projects)
	}
	byPath := w.search(t, "ada/code")
	if len(byPath.Projects) != 2 {
		t.Errorf("projects for a folder = %+v, want both", byPath.Projects)
	}
	// A card is found by its description and its key as well as its title.
	wantKeys(t, w.search(t, "an hour"), "web#7")
	wantKeys(t, w.search(t, "web#8"), "web#8")
}

// A search ignores case, and matches inside a word as well as at the start of one.
func TestMatchingIgnoresCaseAndFindsPartsOfWords(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "Api-Gateway", "/code/api")})
	w.card("api", 1, "Refresh the TOKEN", "", 1)
	w.chat("api", "chat-1", "Rotate Tokens", 1)
	for _, query := range []string{"token", "TOKEN", "ToKeN", "oken", "tok"} {
		got := w.search(t, query)
		if len(got.Cards) != 1 || len(got.Chats) != 1 {
			t.Errorf("%q found %d cards and %d chats, want 1 and 1", query, len(got.Cards), len(got.Chats))
		}
	}
	if got := w.search(t, "GATEWAY"); len(got.Projects) != 1 {
		t.Errorf("a name in another case found %d projects, want 1", len(got.Projects))
	}
	// Accented and non-Latin text folds too.
	w.card("api", 2, "Überprüfung der Rechte", "", 1)
	wantKeys(t, w.search(t, "überprüfung"), "api#2")
	wantKeys(t, w.search(t, "ÜBERPRÜFUNG"), "api#2")
}

// Every word of the query has to match, in any order, and a word may match a different field from
// the one before.
func TestEveryWordHasToMatch(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.card("api", 1, "Refresh the token", "Expires after an hour.", 1)
	w.card("api", 2, "Refresh the cache", "", 1)
	w.card("api", 3, "Rotate keys", "The token is refreshed.", 1)
	wantKeys(t, w.search(t, "refresh token"), "api#1", "api#3")
	wantKeys(t, w.search(t, "token refresh"), "api#1", "api#3")
	wantKeys(t, w.search(t, "refresh token zebra"))
	wantKeys(t, w.search(t, "refresh hour"), "api#1")
}

// Better matches come first: a title over a description, a whole title over the start of one over
// the start of a word over the middle of one, and among equals the one touched most recently.
func TestBetterMatchesRankFirst(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.card("api", 1, "Rotate keys", "Mentions cache once.", 9)
	w.card("api", 2, "Uncached reads", "", 1)
	w.card("api", 3, "Warm the cache", "", 2)
	w.card("api", 4, "Cache invalidation", "", 3)
	w.card("api", 5, "cache", "", 4)
	w.card("api", 6, "Warm the cache", "", 8)
	wantKeys(t, w.search(t, "cache"),
		"api#5",          // the whole title
		"api#4",          // the title starts with it
		"api#6", "api#3", // it starts a word of the title, newest first
		"api#2", // it is inside a word of the title
		"api#1") // only the description has it

	// Two of the same score are told apart by when they last changed.
	w = newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.card("api", 1, "Same words", "", 1)
	w.card("api", 2, "Same words", "", 5)
	w.card("api", 3, "Same words", "", 3)
	wantKeys(t, w.search(t, "same"), "api#2", "api#3", "api#1")
}

// Projects rank by name over id over folder, and a whole name over a start over the middle.
func TestProjectsRankByNameFirst(t *testing.T) {
	w := newWorld(t, []protocol.Project{
		project("in-path", "Zebra", "/code/token/zebra"),
		project("mid", "My token tool", "/code/mid"),
		project("start", "Token service", "/code/start"),
		project("whole", "token", "/code/whole"),
		project("by-id", "Other", "/code/other"),
	})
	w.projects.projects[4].ID = "token-id"
	got := w.search(t, "token")
	var ids []string
	for _, p := range got.Projects {
		ids = append(ids, p.ProjectID)
	}
	want := []string{"whole", "start", "mid", "token-id", "in-path"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("projects = %v, want %v", ids, want)
	}
}

// A chat is found by its title, best match first and the most recent first among equals.
func TestChatsRankByTitleThenRecency(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api"), project("web", "web", "/code/web")})
	w.chat("api", "a", "Plan the release", 1)
	w.chat("web", "b", "Release", 2)
	w.chat("api", "c", "Plan the release", 6)
	w.chat("web", "d", "Ship it", 9)
	got := w.search(t, "release")
	var ids []string
	for _, c := range got.Chats {
		ids = append(ids, c.ChatID)
	}
	if strings.Join(ids, ",") != "b,c,a" {
		t.Errorf("chats = %v, want [b c a]", ids)
	}
}

// A query that reads as a card number finds cards by number: "#41" the card 41 of every project,
// first, before cards whose number only starts with 41, and never by a 41 inside a title.
func TestAHashAndANumberFindsCardsByNumber(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api"), project("web", "web", "/code/web")})
	w.card("api", 4, "Four", "", 1)
	w.card("api", 41, "Forty one", "", 1)
	w.card("api", 410, "Four hundred and ten", "", 1)
	w.card("web", 41, "Also forty one", "", 2)
	w.card("web", 5, "Retry 41 times", "", 1)
	w.card("web", 14, "Fourteen", "", 1)
	w.card("web", 6, "Fixes #41 in the api", "", 1)

	wantKeys(t, w.search(t, "#41"), "web#41", "api#41", "api#410", "web#6")
	// Being typed: "#4" finds the cards whose number starts with 4, the whole number first.
	wantKeys(t, w.search(t, "#4"), "api#4", "web#41", "api#41", "api#410", "web#6")
	// A whole key names one card.
	wantKeys(t, w.search(t, "api#41"), "api#41", "api#410")
	wantKeys(t, w.search(t, "API#41"), "api#41", "api#410")
	// Digits on their own are that number first, then a title that has them, then a number that
	// starts with them.
	wantKeys(t, w.search(t, "41"), "web#41", "api#41", "web#5", "web#6", "api#410")
	// A number joins other words: the card has to match both.
	wantKeys(t, w.search(t, "#41 also"), "web#41")
	wantKeys(t, w.search(t, "#999"))
}

// A hash alone is a way to browse cards: it matches every card, the most recently changed first.
func TestAHashAloneListsCardsByRecency(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.card("api", 1, "Old", "", 1)
	w.card("api", 2, "New", "", 9)
	wantKeys(t, w.search(t, "#"), "api#2", "api#1")
}

// A list is cut to the most hits of one kind, the best of them, and the total says how many there
// were before the cut.
func TestEachListIsCutAndTheTotalIsTheWholeCount(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	const many = 20
	for i := 1; i <= many; i++ {
		w.card("api", i, fmt.Sprintf("Fix crash %d", i), "", i)
		w.chat("api", fmt.Sprintf("chat-%02d", i), fmt.Sprintf("Crash notes %d", i), i)
	}
	for i := range many {
		w.projects.projects = append(w.projects.projects, project(fmt.Sprintf("crash-%d", i), "Crash "+fmt.Sprint(i), "/code/c"))
	}
	got := w.search(t, "crash")
	if len(got.Projects) != protocol.SearchHitsPerKind || len(got.Cards) != protocol.SearchHitsPerKind ||
		len(got.Chats) != protocol.SearchHitsPerKind {
		t.Errorf("lengths = %d, %d, %d, want %d each", len(got.Projects), len(got.Cards), len(got.Chats), protocol.SearchHitsPerKind)
	}
	if got.Totals != (protocol.SearchTotals{Projects: many, Cards: many, Chats: many}) {
		t.Errorf("totals = %+v, want %d of each", got.Totals, many)
	}
	// The newest cards are the ones kept, and the cut never reorders what is left.
	wantKeys(t, got, "api#20", "api#19", "api#18", "api#17", "api#16", "api#15", "api#14", "api#13")
	if got.Chats[0].ChatID != "chat-20" || got.Chats[len(got.Chats)-1].ChatID != "chat-13" {
		t.Errorf("chats = %+v", got.Chats)
	}
}

// A search that finds nothing is three empty lists, never null, with the query it looked for.
func TestNoMatchIsEmptyListsNotNull(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.card("api", 1, "Something", "", 1)
	got := w.search(t, "zzzz")
	if got.Projects == nil || got.Cards == nil || got.Chats == nil {
		t.Errorf("a list is nil: %+v", got)
	}
	if len(got.Projects)+len(got.Cards)+len(got.Chats) != 0 || got.Totals != (protocol.SearchTotals{}) || got.Query != "zzzz" {
		t.Errorf("answer = %+v", got)
	}
	// A daemon with nothing in it answers the same.
	empty := newWorld(t, nil).search(t, "anything")
	if empty.Projects == nil || empty.Cards == nil || empty.Chats == nil {
		t.Errorf("a list is nil: %+v", empty)
	}
}

// An empty query, or one of only spaces, matches nothing and reads nothing.
func TestAnEmptyQueryMatchesNothingAndReadsNothing(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.card("api", 1, "Something", "", 1)
	for _, query := range []string{"", "   ", "\t\n"} {
		got := w.search(t, query)
		if got.Query != "" || len(got.Projects)+len(got.Cards)+len(got.Chats) != 0 ||
			got.Projects == nil || got.Cards == nil || got.Chats == nil {
			t.Errorf("%q answered %+v", query, got)
		}
	}
	if w.projects.listCalls != 0 || len(w.chats.archived) != 0 {
		t.Errorf("an empty query read the projects %d times and the chats %d times", w.projects.listCalls, len(w.chats.archived))
	}
}

// The answer says which query it searched: trimmed, with each run of spaces made one.
func TestTheQueryIsCleaned(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.card("api", 1, "Refresh the token", "", 1)
	got := w.search(t, "  refresh \t  the\n token  ")
	if got.Query != "refresh the token" {
		t.Errorf("query = %q", got.Query)
	}
	wantKeys(t, got, "api#1")
}

// A query longer than the limit is refused with a sentence, counted in characters and not bytes,
// and one at the limit is answered. Spaces that are trimmed or collapsed are not counted.
func TestAQueryTooLongIsRefused(t *testing.T) {
	w := newWorld(t, nil)
	ctx := context.Background()
	atLimit := strings.Repeat("é", protocol.MaxSearchQueryChars) // 400 bytes, 200 characters
	for name, query := range map[string]string{
		"a query at the limit":       atLimit,
		"a query with padding":       strings.Repeat(" ", 500) + "token" + strings.Repeat(" ", 500),
		"words with long gaps":       strings.Repeat("a"+strings.Repeat(" ", 50), 100),
		"a hundred one-letter words": strings.Repeat("a ", 100),
	} {
		if _, err := w.svc.Search(ctx, query); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
	for name, query := range map[string]string{
		"one character over": atLimit + "x",
		"words that add up":  strings.Repeat("a ", 101),
	} {
		_, err := w.svc.Search(ctx, query)
		var refusal *protocol.Error
		if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeInvalidArgument ||
			refusal.Message != "Search for 200 characters or fewer." {
			t.Errorf("%s: err = %v, want the invalid_argument sentence", name, err)
		}
	}
	_, err := w.svc.Search(ctx, "\xff\xfe")
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeInvalidArgument {
		t.Errorf("a query that is not text: err = %v, want invalid_argument", err)
	}
}

// Archived chats stay behind the Archived toggle: the search asks for the live half only.
func TestArchivedChatsAreNotSearched(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api"), project("web", "web", "/code/web")})
	w.search(t, "anything")
	if len(w.chats.archived) != 2 {
		t.Fatalf("the chats were read %d times, want once for each project", len(w.chats.archived))
	}
	for _, archived := range w.chats.archived {
		if archived {
			t.Error("the search asked for archived chats")
		}
	}
}

// A project that is removed while the search is running is left out, and the rest is answered.
func TestAProjectRemovedMidSearchIsSkipped(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api"), project("gone", "gone", "/code/gone")})
	w.card("api", 1, "Match me", "", 1)
	w.projects.cardsErr["gone"] = protocol.NotFound("project")
	got := w.search(t, "match")
	wantKeys(t, got, "api#1")
}

// A read that fails for any other reason fails the search: a half answer would look like no
// matches.
func TestAReadThatFailsFailsTheSearch(t *testing.T) {
	boom := errors.New("disk on fire")
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.projects.cardsErr["api"] = boom
	if _, err := w.svc.Search(context.Background(), "x"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the read's own error", err)
	}
	w = newWorld(t, nil)
	w.projects.listErr = boom
	if _, err := w.svc.Search(context.Background(), "x"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the read's own error", err)
	}
}

// A search whose request was cancelled stops rather than reading on.
func TestACancelledSearchStops(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.svc.Search(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// sessionOn records a stored session event the memory module would answer with for a card: the
// summary the index matched, and when it happened. The card's own fields are what the search uses to
// name the hit.
func (w *world) sessionOn(card protocol.Card, excerpt string, minute int) {
	w.sessions.hits[card.ProjectID] = append(w.sessions.hits[card.ProjectID], protocol.SessionHit{
		CardID: card.ID, Key: card.Key, Title: card.Title, Excerpt: excerpt,
		At: protocol.NewTimestamp(base.Add(time.Duration(minute) * time.Minute)),
	})
}

// noteOn records a card note the memory module would answer with, with the whole body the index
// matched and the time it was last saved.
func (w *world) noteOn(card protocol.Card, path, body, author string, minute int) {
	at := protocol.NewTimestamp(base.Add(time.Duration(minute) * time.Minute))
	w.notes.notes[card.ProjectID] = append(w.notes.notes[card.ProjectID], protocol.Note{
		CardID: card.ID, ProjectID: card.ProjectID, Path: path, Body: body,
		Author: protocol.NoteAuthor(author), UpdatedAt: &at,
	})
}

// The two kinds the search does not read whole - a project's past sessions and its notes - are named
// by their card and carry what opens them and the project they belong to, exactly as a card hit does.
func TestSessionAndNoteHitsAreNamedByTheirCard(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api-gateway", "/home/ada/code/api-gateway")})
	card := w.card("api", 41, "Refresh the token before it expires", "", 5)
	w.sessionOn(card, "Refreshed the token in the middleware", 10)
	w.noteOn(card, "api/cards/41-refresh-the-token.md", "# Refresh the token\n\nGoal: refresh it.", "agent", 11)

	got := w.search(t, "token")
	if len(got.Sessions) != 1 {
		t.Fatalf("sessions = %+v, want one hit", got.Sessions)
	}
	session := got.Sessions[0]
	if session.CardID != card.ID || session.Key != "api#41" || session.Title != "Refresh the token before it expires" ||
		session.ProjectID != "api" || session.ProjectName != "api-gateway" ||
		session.Excerpt != "Refreshed the token in the middleware" ||
		session.At != protocol.NewTimestamp(base.Add(10*time.Minute)) {
		t.Errorf("session hit = %+v", session)
	}
	if len(got.Notes) != 1 {
		t.Fatalf("notes = %+v, want one hit", got.Notes)
	}
	note := got.Notes[0]
	if note.CardID != card.ID || note.Key != "api#41" || note.Title != "Refresh the token before it expires" ||
		note.Path != "api/cards/41-refresh-the-token.md" || note.Author != protocol.NoteAuthorAgent ||
		note.ProjectID != "api" || note.ProjectName != "api-gateway" ||
		note.At != protocol.NewTimestamp(base.Add(11*time.Minute)) {
		t.Errorf("note hit = %+v", note)
	}
	if !strings.HasPrefix(note.Excerpt, "# Refresh the token") {
		t.Errorf("note excerpt = %q, want the beginning of the note", note.Excerpt)
	}
	if got.Totals != (protocol.SearchTotals{Cards: 1, Sessions: 1, Notes: 1}) {
		t.Errorf("totals = %+v, want the card and one of each of the two kinds behind it", got.Totals)
	}

	// A session hit is named by the card's own title, so a word in the title is enough: the index
	// matched the summary, and the search scores the title too.
	if hits, _ := w.svc.Search(context.Background(), "expires"); len(hits.Sessions) != 1 {
		t.Errorf("a word only the card's title holds found %d sessions, want 1", len(hits.Sessions))
	}
}

// A note's answer carries only the beginning of it (see excerpt), because a palette needs a pointer
// at the note and not the note. The whole note goes to the caller that scores it, so a word further
// in than the excerpt reaches still ranks the hit.
func TestANoteHitExcerptIsTheBeginningOfTheNote(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	long := w.card("api", 1, "Write the runbook", "", 1)
	short := w.card("api", 2, "Write the runbook", "", 1)
	body := strings.Repeat("é", 150) + " tailmarker" // 300 bytes of text, then a word at the end
	w.noteOn(long, "api/cards/1-write-the-runbook.md", body, "person", 1)
	w.noteOn(short, "api/cards/2-write-the-runbook.md", "a short tailmarker note", "person", 2)

	got := w.search(t, "tailmarker")
	if len(got.Notes) != 2 {
		t.Fatalf("a word past the excerpt did not rank the note: %+v", got.Notes)
	}
	for _, note := range got.Notes {
		switch note.CardID {
		case long.ID:
			if len(note.Excerpt) >= len(body) {
				t.Errorf("the long note's excerpt is the whole note: %d bytes of %d", len(note.Excerpt), len(body))
			}
			if !strings.HasSuffix(note.Excerpt, "…") {
				t.Errorf("a cut excerpt = %q, want it to end with an ellipsis", note.Excerpt)
			}
			if strings.ContainsRune(note.Excerpt, '\uFFFD') {
				t.Errorf("the excerpt cut a character in half: %q", note.Excerpt)
			}
		case short.ID:
			if note.Excerpt != "a short tailmarker note" {
				t.Errorf("a short note's excerpt = %q, want the note whole and uncut", note.Excerpt)
			}
		default:
			t.Errorf("an unexpected note hit: %+v", note)
		}
	}
}

// A session or a note the index answered but this project's cards no longer hold is skipped rather
// than shown as a dead link: the card was deleted between the two reads.
func TestASessionOrNoteHitForAGoneCardIsSkipped(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	gone := w.card("api", 1, "Refresh the token", "", 1)
	w.sessionOn(gone, "the token work", 2)
	w.noteOn(gone, "api/cards/1-refresh-the-token.md", "the token note", "agent", 2)
	// A card the project still has, so one hit of each kind survives the read.
	survivor := w.card("api", 2, "Rotate the token", "", 1)
	w.sessionOn(survivor, "the token work again", 3)
	w.noteOn(survivor, "api/cards/2-rotate-the-token.md", "the token note again", "agent", 3)

	// The project's cards no longer include the first card, which is what a delete mid-search looks
	// like to the search: the index answered before the card went.
	w.projects.cards["api"] = []protocol.Card{survivor}
	got := w.search(t, "token")
	if len(got.Sessions) != 1 || got.Sessions[0].CardID != survivor.ID {
		t.Errorf("sessions = %+v, want only the card that is still there", got.Sessions)
	}
	if len(got.Notes) != 1 || got.Notes[0].CardID != survivor.ID {
		t.Errorf("notes = %+v, want only the card that is still there", got.Notes)
	}
}

// A hit the index answered that does not match the words is dropped rather than shown: the memory
// module's index matches a prefix of a word, and the ordering pass here decides what really matches.
func TestASessionOrNoteHitThatDoesNotMatchIsDropped(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	card := w.card("api", 1, "Unrelated title", "", 1)
	w.sessionOn(card, "nothing to do with the query", 2)
	w.noteOn(card, "api/cards/1-unrelated-title.md", "nothing to do with the query", "agent", 2)

	got := w.search(t, "token")
	if len(got.Sessions) != 0 || len(got.Notes) != 0 {
		t.Errorf("hits that do not match the words were shown: %+v / %+v", got.Sessions, got.Notes)
	}
}

// The memory module is searched with the query's words joined, once per project per kind, so the
// index and this package look for the same thing.
func TestSessionAndNoteSearchesAreGivenTheWordsAndOneProjectAtATime(t *testing.T) {
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api"), project("web", "web", "/code/web")})
	w.card("api", 1, "Something", "", 1)
	w.card("web", 1, "Something", "", 1)
	w.search(t, "  Token   ROTATION ")
	if strings.Join(w.sessions.queries, ",") != "token rotation,token rotation" {
		t.Errorf("the sessions were searched for %v, want the words joined once per project", w.sessions.queries)
	}
}

// A read of the past sessions or the notes that fails fails the search: a half answer would look
// like a kind with no matches.
func TestASessionOrNoteReadThatFailsFailsTheSearch(t *testing.T) {
	boom := errors.New("disk on fire")
	w := newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.sessions.err = boom
	if _, err := w.svc.Search(context.Background(), "x"); !errors.Is(err, boom) {
		t.Errorf("a failing session read: err = %v, want the read's own error", err)
	}
	w = newWorld(t, []protocol.Project{project("api", "api", "/code/api")})
	w.notes.err = boom
	if _, err := w.svc.Search(context.Background(), "x"); !errors.Is(err, boom) {
		t.Errorf("a failing note read: err = %v, want the read's own error", err)
	}
}

func TestNewNeedsEveryReader(t *testing.T) {
	all := func() search.Deps {
		return search.Deps{
			Projects: &fakeProjects{}, Chats: &fakeChats{},
			Sessions: &fakeSessions{}, Notes: &fakeNotes{},
		}
	}
	// Every reader is needed: an answer with a kind missing would look like a kind with no matches.
	// Each is left out once, and the service must refuse to be built each time.
	leave := map[string]func(*search.Deps){
		"projects": func(d *search.Deps) { d.Projects = nil },
		"chats":    func(d *search.Deps) { d.Chats = nil },
		"sessions": func(d *search.Deps) { d.Sessions = nil },
		"notes":    func(d *search.Deps) { d.Notes = nil },
	}
	for name, drop := range leave {
		deps := all()
		drop(&deps)
		if _, err := search.New(deps); err == nil {
			t.Errorf("a service with no %s was built", name)
		}
	}
	if _, err := search.New(all()); err != nil {
		t.Errorf("a service with every reader was not built: %v", err)
	}
}
