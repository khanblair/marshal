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

// The search answers over what the projects and chats services give it, so these tests give it
// small fakes of the two and read the answers. The real services behind the real route are in
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

// world is the fakes and the service over them.
type world struct {
	projects *fakeProjects
	chats    *fakeChats
	svc      *search.Service
}

func newWorld(t *testing.T, projects []protocol.Project) *world {
	t.Helper()
	w := &world{
		projects: &fakeProjects{
			projects: projects, cards: map[string][]protocol.Card{}, cardsErr: map[string]error{},
		},
		chats: &fakeChats{chats: map[string][]protocol.Chat{}},
	}
	svc, err := search.New(search.Deps{Projects: w.projects, Chats: w.chats}, search.WithClock(func() time.Time { return base }))
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

func TestNewNeedsBothReaders(t *testing.T) {
	if _, err := search.New(search.Deps{Chats: &fakeChats{}}); err == nil {
		t.Error("a service with no projects was built")
	}
	if _, err := search.New(search.Deps{Projects: &fakeProjects{}}); err == nil {
		t.Error("a service with no chats was built")
	}
}
