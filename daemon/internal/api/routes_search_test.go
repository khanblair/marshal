package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The search route (docs/backend-checklist.md B2.11, N23) over the real server, the real projects
// and chats services, and the real store. The matching and ranking rules are tested in
// internal/search; these tests are about the route: what it reads, what it answers, that it sees
// what the other routes just changed, and that it needs a token (TestEveryRouteRequiresAToken).

// searchFor runs a search through the API and returns the answer and its raw body.
func (st *stack) searchFor(query string) (protocol.SearchSnapshot, []byte) {
	st.t.Helper()
	r := st.do(http.MethodGet, "/v1/search?q="+url.QueryEscape(query), nil).want(st.t, http.StatusOK)
	return decode[protocol.SearchSnapshot](st.t, r), r.Body
}

// searchWorld is two projects, one of them the sample, with a few cards and chats to find.
type searchWorld struct {
	small, sample protocol.Project
	tokenCard     protocol.Card
	healthCard    protocol.Card
	healthChat    protocol.Chat
}

func (st *stack) makeSearchWorld() searchWorld {
	st.t.Helper()
	var w searchWorld
	w.small, _ = st.addProject("small-repo")
	r := st.do(http.MethodPost, "/v1/projects", protocol.CreateProjectRequest{Source: protocol.ProjectSourceSample}).
		want(st.t, http.StatusCreated)
	w.sample = decode[protocol.Project](st.t, r)
	w.tokenCard = st.addCard(w.small.ID, "Refresh the token before it expires")
	w.healthCard = st.addCard(w.sample.ID, "Add a health check endpoint")
	st.addCard(w.sample.ID, "Fix the typo in the README")
	w.healthChat = st.newChat(w.sample.ID, protocol.CreateChatRequest{Title: "Health check question"})
	st.newChat(w.small.ID, protocol.CreateChatRequest{Title: "Unrelated"})
	return w
}

func TestSearchThroughHTTP(t *testing.T) {
	st := newStack(t)
	w := st.makeSearchWorld()

	// One answer has every kind, in the shape the golden file has, with what opens each hit.
	got, body := st.searchFor("health")
	sameShape(t, "search", body)
	if got.Query != "health" || len(got.Projects) != 0 {
		t.Errorf("answer = %+v, want the query and no project", got)
	}
	if len(got.Cards) != 1 || got.Cards[0].CardID != w.healthCard.ID || got.Cards[0].Key != w.healthCard.Key ||
		got.Cards[0].Number != w.healthCard.Number || got.Cards[0].ProjectID != w.sample.ID ||
		got.Cards[0].ProjectName != w.sample.Name || got.Cards[0].State != protocol.CardStateBacklog {
		t.Errorf("cards = %+v, want the sample's health card", got.Cards)
	}
	if len(got.Chats) != 1 || got.Chats[0].ChatID != w.healthChat.ID || got.Chats[0].ProjectID != w.sample.ID ||
		got.Chats[0].ProjectName != w.sample.Name {
		t.Errorf("chats = %+v, want the sample's health chat", got.Chats)
	}
	if got.Totals != (protocol.SearchTotals{Cards: 1, Chats: 1}) || got.ServerTime.Time().IsZero() {
		t.Errorf("totals = %+v, serverTime = %v", got.Totals, got.ServerTime)
	}

	// A project is found by its name, with the language the palette shows.
	byName, _ := st.searchFor("sample")
	if len(byName.Projects) != 1 || byName.Projects[0].ProjectID != w.sample.ID ||
		byName.Projects[0].Name != w.sample.Name || byName.Projects[0].Path != w.sample.Path ||
		byName.Projects[0].Language != w.sample.Language {
		t.Errorf("projects = %+v, want the sample project", byName.Projects)
	}

	// "#1" is card 1 of every project, and each says which project it is in.
	numbered, _ := st.searchFor("#1")
	if len(numbered.Cards) != 2 || numbered.Cards[0].Number != 1 || numbered.Cards[1].Number != 1 ||
		numbered.Cards[0].ProjectID == numbered.Cards[1].ProjectID {
		t.Errorf("cards for #1 = %+v, want card 1 of both projects", numbered.Cards)
	}
	if one, _ := st.searchFor(w.tokenCard.Key); len(one.Cards) != 1 || one.Cards[0].CardID != w.tokenCard.ID {
		t.Errorf("cards for the key %s = %+v", w.tokenCard.Key, one.Cards)
	}
}

// A search is never behind: it reads what the other routes wrote a moment ago, so a card that was
// edited, deleted, or moved, a chat that was archived, and a project that was removed all show at
// once. There is no index to catch up.
func TestSearchSeesChangesAtOnce(t *testing.T) {
	st := newStack(t)
	w := st.makeSearchWorld()

	title := "Add a readiness probe"
	st.do(http.MethodPatch, "/v1/cards/"+w.healthCard.ID, protocol.UpdateCardRequest{Title: &title}).want(t, http.StatusOK)
	if got, _ := st.searchFor("health"); len(got.Cards) != 0 || len(got.Chats) != 1 {
		t.Errorf("after the rename, health finds %d cards and %d chats, want 0 and 1", len(got.Cards), len(got.Chats))
	}
	if got, _ := st.searchFor("readiness"); len(got.Cards) != 1 {
		t.Errorf("after the rename, readiness finds %d cards, want 1", len(got.Cards))
	}

	st.do(http.MethodPost, "/v1/cards/"+w.healthCard.ID+"/move", protocol.MoveCardRequest{State: protocol.CardStatePlanning}).
		want(t, http.StatusOK)
	if got, _ := st.searchFor("readiness"); len(got.Cards) != 1 || got.Cards[0].State != protocol.CardStatePlanning {
		t.Errorf("after the move, the hit is %+v, want it in planning", got.Cards)
	}

	st.do(http.MethodDelete, "/v1/cards/"+w.healthCard.ID, nil).want(t, http.StatusNoContent)
	if got, _ := st.searchFor("readiness"); len(got.Cards) != 0 {
		t.Errorf("a deleted card is still found: %+v", got.Cards)
	}

	// An archived chat leaves the search, as it leaves the main list, and comes back with it.
	st.do(http.MethodPost, "/v1/chats/"+w.healthChat.ID+"/archive", nil).want(t, http.StatusOK)
	if got, _ := st.searchFor("health"); len(got.Chats) != 0 {
		t.Errorf("an archived chat is found: %+v", got.Chats)
	}
	st.do(http.MethodPost, "/v1/chats/"+w.healthChat.ID+"/restore", nil).want(t, http.StatusOK)
	if got, _ := st.searchFor("health"); len(got.Chats) != 1 {
		t.Errorf("a restored chat is not found: %+v", got.Chats)
	}

	// Removing a project takes it, its cards, and its chats out of every answer.
	st.do(http.MethodDelete, "/v1/projects/"+w.sample.ID, nil).want(t, http.StatusNoContent)
	gone, _ := st.searchFor("sample")
	if len(gone.Projects)+len(gone.Cards)+len(gone.Chats) != 0 {
		t.Errorf("a removed project is still found: %+v", gone)
	}
	if got, _ := st.searchFor("health"); got.Totals != (protocol.SearchTotals{}) {
		t.Errorf("the removed project's chat is still found: %+v", got)
	}
}

// A board of many cards is cut to the most hits of one kind, and the total says how many matched.
func TestSearchCutsLongLists(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	const cards = 50
	for i := range cards {
		st.addCard(project.ID, "Fix crash "+strings.Repeat("x", i%3))
	}
	got, _ := st.searchFor("crash")
	if len(got.Cards) != protocol.SearchHitsPerKind || got.Totals.Cards != cards {
		t.Errorf("cards = %d (total %d), want %d of %d", len(got.Cards), got.Totals.Cards, protocol.SearchHitsPerKind, cards)
	}
}

// Nothing is indexed, so a search reads what the sidebar and the board read on every request. That
// stays quick with a big board: three projects, 100 cards and 15 chats are searched 30 times, and
// the mean is logged (run with -v to see it: about 3 ms, and about 60 ms under the race detector).
// The limit is far above that, and only there to catch a search that starts to cost a read for
// every card.
func TestSearchIsQuickOnABigBoard(t *testing.T) {
	const (
		searches = 30
		limit    = time.Second
	)
	st := newStack(t)
	small, _ := st.addProject("small-repo")
	mono, _ := st.addProject("monorepo")
	sampleProject := decode[protocol.Project](t, st.do(http.MethodPost, "/v1/projects",
		protocol.CreateProjectRequest{Source: protocol.ProjectSourceSample}).want(t, http.StatusCreated))
	for i, project := range []protocol.Project{small, mono, sampleProject} {
		cards := 50
		if i > 0 {
			cards = 25
		}
		for n := range cards {
			st.addCard(project.ID, fmt.Sprintf("Fix crash %d in the %s service", n, project.Name))
		}
		for n := range 5 {
			st.newChat(project.ID, protocol.CreateChatRequest{Title: fmt.Sprintf("Crash question %d", n)})
		}
	}
	started := time.Now()
	for range searches {
		got, _ := st.searchFor("crash service")
		if got.Totals.Cards != 100 || got.Totals.Chats != 0 {
			t.Fatalf("totals = %+v, want 100 cards and no chat (a chat has no service)", got.Totals)
		}
	}
	mean := time.Since(started) / searches
	t.Logf("a search over 3 projects, 100 cards, and 15 chats takes %s on average, through HTTP", mean)
	if mean > limit {
		t.Errorf("a search takes %s on average, want under %s", mean, limit)
	}
}

// Without a query, or with only spaces, the answer is the three empty lists and no error. A query
// that is too long is refused with the daemon's sentence and changes nothing.
func TestSearchQueryRules(t *testing.T) {
	st := newStack(t)
	st.makeSearchWorld()
	for _, path := range []string{"/v1/search", "/v1/search?q=", "/v1/search?q=%20%20%20"} {
		r := st.do(http.MethodGet, path, nil).want(t, http.StatusOK)
		sameShape(t, "search-empty", r.Body)
		got := decode[protocol.SearchSnapshot](t, r)
		if got.Query != "" || got.Totals != (protocol.SearchTotals{}) {
			t.Errorf("%s answered %+v, want nothing", path, got)
		}
	}

	over := strings.Repeat("a", protocol.MaxSearchQueryChars+1)
	refusal := st.do(http.MethodGet, "/v1/search?q="+over, nil).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if refusal.Message != "Search for 200 characters or fewer." {
		t.Errorf("message = %q", refusal.Message)
	}
	// Text that is not valid UTF-8 is refused too, not searched as it is.
	st.do(http.MethodGet, "/v1/search?q=%ff%fe", nil).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	// The query is not written to the log: it is what a person is looking for.
	st.searchFor("quokka-needle")
	st.mustNotLog("quokka-needle")
}

// Search reads only, so it takes no other method.
func TestSearchTakesOnlyGet(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodPost, "/v1/search?q=x", `{}`).apiError(t, http.StatusMethodNotAllowed, protocol.ErrorCodeMethodNotAllowed)
}

// The search reads through the projects and the chats, so the route exists only with the search
// service, which the stack builds only over both of those. Any of the three missing is an address
// that does not exist, not an answer with a kind left out.
func TestSearchRouteNeedsTheSearchServiceAndWhatItReadsThrough(t *testing.T) {
	for name, opt := range map[string]stackOption{
		"no search service": withoutSearch(), "no projects": withoutProjects(), "no chats": withoutChats(),
	} {
		t.Run(name, func(t *testing.T) {
			st := newStack(t, opt)
			got := st.do(http.MethodGet, "/v1/search?q=x", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
			if got.Message != nothingThereMsg {
				t.Errorf("message = %q, want the nothing-at-that-address sentence", got.Message)
			}
		})
	}
}
