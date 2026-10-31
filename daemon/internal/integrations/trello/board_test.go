package trello_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
)

// The calls the Trello sync makes beyond the first ones, against a fake server that records each
// request. The key and the token are easy to spot strings, so a test can prove they never come back
// out in an error.

const (
	testKey   = "KEY-abc123"
	testToken = "TOKEN-xyz789"
)

// seen is one request the fake server got.
type seen struct {
	method string
	path   string
	// query is the address's own options, which on a write holds only the credential.
	query url.Values
	// form is the body's fields, which is where a write puts its data.
	form        url.Values
	contentType string
}

// recorder is a fake Trello that answers every request with the same status and body and keeps what
// it was asked.
type recorder struct {
	*httptest.Server
	mu     sync.Mutex
	status int
	body   string
	reqs   []seen
}

func newRecorder(t *testing.T, body string) *recorder {
	t.Helper()
	rec := &recorder{status: http.StatusOK, body: body}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, seen{
			method: r.Method, path: r.URL.Path, query: r.URL.Query(), form: r.PostForm,
			contentType: r.Header.Get("Content-Type"),
		})
		status, answer := rec.status, rec.body
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(rec.Close)
	return rec
}

func (r *recorder) client() *trello.Client {
	return trello.NewClient(testKey, testToken, trello.WithBaseURL(r.URL))
}

// only returns the one request the server saw.
func (r *recorder) only(t *testing.T) seen {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) != 1 {
		t.Fatalf("the server saw %d requests, want 1: %+v", len(r.reqs), r.reqs)
	}
	return r.reqs[0]
}

func (s seen) wantCredential(t *testing.T) {
	t.Helper()
	if s.query.Get("key") != testKey || s.query.Get("token") != testToken {
		t.Errorf("the request carried key %q and token %q, want the client's own", s.query.Get("key"), s.query.Get("token"))
	}
}

// wantWrite checks a write: the method, the path, the credential in the address, and every field in
// the body and not in the address.
func (s seen) wantWrite(t *testing.T, method, path string, fields url.Values) {
	t.Helper()
	if s.method != method || s.path != path {
		t.Errorf("request = %s %s, want %s %s", s.method, s.path, method, path)
	}
	s.wantCredential(t)
	if s.contentType != "application/x-www-form-urlencoded" {
		t.Errorf("content type = %q, want a form", s.contentType)
	}
	for name, want := range fields {
		if got := s.form[name]; len(got) != 1 || got[0] != want[0] {
			t.Errorf("body field %q = %v, want %v", name, got, want)
		}
		if _, inAddress := s.query[name]; inAddress {
			t.Errorf("field %q was sent in the address, want the body", name)
		}
	}
	if len(s.form) != len(fields) {
		t.Errorf("body fields = %v, want exactly %v", s.form, fields)
	}
}

func TestBoardCardsReadsTheOpenCardsWithTheirChecklists(t *testing.T) {
	rec := newRecorder(t, `[
		{"id":"c1","name":"One","desc":"first","idList":"l1","closed":false,"url":"https://trello.com/c/1",
		 "dateLastActivity":"2026-10-01T10:00:00.000Z",
		 "checklists":[{"id":"cl1","name":"Steps","checkItems":[
			{"id":"i1","name":"write it","state":"complete"},
			{"id":"i2","name":"test it","state":"incomplete"}]}]},
		{"id":"c2","name":"Two","idList":"l2"}]`)

	cards, err := rec.client().BoardCards(context.Background(), "board 1")
	if err != nil {
		t.Fatalf("BoardCards: %v", err)
	}

	got := rec.only(t)
	if got.method != http.MethodGet || got.path != "/boards/board 1/cards/open" {
		t.Errorf("request = %s %s, want GET of the board's open cards, with the id escaped", got.method, got.path)
	}
	got.wantCredential(t)
	if got.query.Get("checklists") != "all" {
		t.Errorf("checklists option = %q, want all", got.query.Get("checklists"))
	}
	if fields := got.query.Get("fields"); !strings.Contains(fields, "idList") || !strings.Contains(fields, "closed") {
		t.Errorf("fields = %q, want the list and the closed flag asked for", fields)
	}
	if len(cards) != 2 || cards[0].Name != "One" || cards[1].IDList != "l2" {
		t.Fatalf("cards = %+v, want the two the server held", cards)
	}
	items := cards[0].Checklists[0].Items
	if len(items) != 2 || !items[0].Done() || items[1].Done() {
		t.Errorf("items = %+v, want the first ticked and the second open", items)
	}
}

func TestGetCardReadsOneCardWithItsChecklists(t *testing.T) {
	rec := newRecorder(t, `{"id":"c1","name":"One","idList":"l1","closed":true,
		"checklists":[{"id":"cl1","name":"Steps","checkItems":[]}]}`)

	card, err := rec.client().GetCard(context.Background(), "c1")
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}

	got := rec.only(t)
	if got.method != http.MethodGet || got.path != "/cards/c1" {
		t.Errorf("request = %s %s, want GET /cards/c1", got.method, got.path)
	}
	got.wantCredential(t)
	if got.query.Get("checklists") != "all" {
		t.Errorf("checklists option = %q, want all", got.query.Get("checklists"))
	}
	if !card.Closed || len(card.Checklists) != 1 {
		t.Errorf("card = %+v, want the closed flag and the checklist read", card)
	}
}

func TestBoardCommentsReadsTheCommentActions(t *testing.T) {
	rec := newRecorder(t, `[
		{"id":"a2","date":"2026-10-02T10:00:00.000Z","data":{"text":"second","card":{"id":"c1"}},
		 "memberCreator":{"fullName":"Ada Lovelace","username":"ada"}},
		{"id":"a1","date":"2026-10-01T10:00:00.000Z","data":{"text":"first","card":{"id":"c2"}},
		 "memberCreator":{"fullName":"","username":"grace"}}]`)

	comments, err := rec.client().BoardComments(context.Background(), "b1")
	if err != nil {
		t.Fatalf("BoardComments: %v", err)
	}

	got := rec.only(t)
	if got.path != "/boards/b1/actions" || got.query.Get("filter") != "commentCard" || got.query.Get("limit") != "1000" {
		t.Errorf("request = %s %v, want the comment actions of the board, a thousand at a time", got.path, got.query)
	}
	if got.query.Has("before") {
		t.Errorf("the first page asked for before=%q, want no cursor", got.query.Get("before"))
	}
	// The answer keeps Trello's order, which is newest first.
	if len(comments) != 2 || comments[0].ID != "a2" || comments[1].ID != "a1" {
		t.Fatalf("comments = %+v, want both, in the order served", comments)
	}
	if c := comments[0]; c.CardID != "c1" || c.Text != "second" || c.Author != "Ada Lovelace" || c.Date != "2026-10-02T10:00:00.000Z" {
		t.Errorf("first comment = %+v, want its card, text, author, and date", c)
	}
	if comments[1].Author != "grace" {
		t.Errorf("author = %q, want the username when the full name is empty", comments[1].Author)
	}
}

// pagedComments serves comment actions the way Trello pages them: a full page while there are more,
// each page starting after the id the caller named in `before`.
func pagedComments(t *testing.T, total int) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var befores []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		befores = append(befores, r.URL.Query().Get("before"))
		mu.Unlock()
		start := 0
		if before := r.URL.Query().Get("before"); before != "" {
			if _, err := fmt.Sscanf(before, "a%d", &start); err != nil {
				http.Error(w, "bad cursor", http.StatusBadRequest)
				return
			}
			start++
		}
		end := min(start+1000, total)
		page := make([]map[string]any, 0, end-start)
		for i := start; i < end; i++ {
			page = append(page, map[string]any{
				"id": fmt.Sprintf("a%d", i), "date": "2026-10-01T10:00:00.000Z",
				"data": map[string]any{"text": "hi", "card": map[string]any{"id": "c1"}},
			})
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	t.Cleanup(server.Close)
	return server, &befores
}

func TestBoardCommentsFollowsPagesUntilAShortOne(t *testing.T) {
	server, befores := pagedComments(t, 1500)
	client := trello.NewClient(testKey, testToken, trello.WithBaseURL(server.URL))

	comments, err := client.BoardComments(context.Background(), "b1")
	if err != nil {
		t.Fatalf("BoardComments: %v", err)
	}

	if len(comments) != 1500 {
		t.Fatalf("got %d comments, want all 1500", len(comments))
	}
	if comments[0].ID != "a0" || comments[1000].ID != "a1000" || comments[1499].ID != "a1499" {
		t.Errorf("order broke across the pages: %s, %s, %s", comments[0].ID, comments[1000].ID, comments[1499].ID)
	}
	// The second page continues after the last comment of the first.
	if want := []string{"", "a999"}; strings.Join(*befores, ",") != strings.Join(want, ",") {
		t.Errorf("cursors = %q, want %q", *befores, want)
	}
}

func TestBoardCommentsStopsAtTheFivePageCap(t *testing.T) {
	server, befores := pagedComments(t, 9000)
	client := trello.NewClient(testKey, testToken, trello.WithBaseURL(server.URL))

	comments, err := client.BoardComments(context.Background(), "b1")
	if err != nil {
		t.Fatalf("BoardComments: %v", err)
	}

	if len(*befores) != 5 || len(comments) != 5000 {
		t.Errorf("read %d pages and %d comments, want 5 pages and 5000 comments", len(*befores), len(comments))
	}
}

func TestBoardCommentsStopsAfterAnExactPageThatIsLast(t *testing.T) {
	// A board with exactly 1000 comments costs one extra, empty, request to know it is done.
	server, befores := pagedComments(t, 1000)
	client := trello.NewClient(testKey, testToken, trello.WithBaseURL(server.URL))

	comments, err := client.BoardComments(context.Background(), "b1")
	if err != nil {
		t.Fatalf("BoardComments: %v", err)
	}

	if len(comments) != 1000 || len(*befores) != 2 {
		t.Errorf("got %d comments in %d requests, want 1000 in 2", len(comments), len(*befores))
	}
}

func TestUpdateCardSendsOnlyTheFieldsItWasGiven(t *testing.T) {
	name, desc, empty := "New name", "New description", ""
	tests := []struct {
		name   string
		fields trello.UpdateCardFields
		want   url.Values
	}{
		{"a name", trello.UpdateCardFields{Name: &name}, url.Values{"name": {name}}},
		{"a description", trello.UpdateCardFields{Desc: &desc}, url.Values{"desc": {desc}}},
		{"both", trello.UpdateCardFields{Name: &name, Desc: &desc}, url.Values{"name": {name}, "desc": {desc}}},
		{"a description cleared", trello.UpdateCardFields{Desc: &empty}, url.Values{"desc": {""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := newRecorder(t, `{}`)
			if err := rec.client().UpdateCard(context.Background(), "c1", tt.fields); err != nil {
				t.Fatalf("UpdateCard: %v", err)
			}
			rec.only(t).wantWrite(t, http.MethodPut, "/cards/c1", tt.want)
		})
	}
}

func TestAddCommentPostsTheTextInTheBodyAndAnswersTheCommentId(t *testing.T) {
	rec := newRecorder(t, `{"id":"action-77","date":"2026-10-03T10:00:00.000Z","data":{"text":"ok"}}`)
	long := strings.Repeat("a long comment & more = ", 2000)

	id, err := rec.client().AddComment(context.Background(), "c1", long)
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	if id != "action-77" {
		t.Errorf("id = %q, want the id Trello answered", id)
	}
	rec.only(t).wantWrite(t, http.MethodPost, "/cards/c1/actions/comments", url.Values{"text": {long}})
}

func TestCreateChecklistAddsAnEmptyListToTheCard(t *testing.T) {
	rec := newRecorder(t, `{"id":"cl9","name":"Release steps","checkItems":[]}`)

	list, err := rec.client().CreateChecklist(context.Background(), "c1", "Release steps")
	if err != nil {
		t.Fatalf("CreateChecklist: %v", err)
	}

	if list.ID != "cl9" || list.Name != "Release steps" {
		t.Errorf("checklist = %+v, want the one Trello answered", list)
	}
	rec.only(t).wantWrite(t, http.MethodPost, "/checklists", url.Values{"idCard": {"c1"}, "name": {"Release steps"}})
}

func TestAddCheckItemSaysWhetherTheLineStartsTicked(t *testing.T) {
	for _, done := range []bool{true, false} {
		t.Run(fmt.Sprint("done=", done), func(t *testing.T) {
			state := "incomplete"
			if done {
				state = "complete"
			}
			rec := newRecorder(t, fmt.Sprintf(`{"id":"i5","name":"ship it","state":%q}`, state))

			item, err := rec.client().AddCheckItem(context.Background(), "cl9", "ship it", done)
			if err != nil {
				t.Fatalf("AddCheckItem: %v", err)
			}

			if item.ID != "i5" || item.Done() != done {
				t.Errorf("item = %+v, want i5 with done = %v", item, done)
			}
			rec.only(t).wantWrite(t, http.MethodPost, "/checklists/cl9/checkItems",
				url.Values{"name": {"ship it"}, "checked": {fmt.Sprint(done)}})
		})
	}
}

func TestSetCheckItemTicksAndReopensTheLine(t *testing.T) {
	for state, done := range map[string]bool{"complete": true, "incomplete": false} {
		t.Run(state, func(t *testing.T) {
			rec := newRecorder(t, `{}`)
			if err := rec.client().SetCheckItem(context.Background(), "c1", "i5", done); err != nil {
				t.Fatalf("SetCheckItem: %v", err)
			}
			rec.only(t).wantWrite(t, http.MethodPut, "/cards/c1/checkItem/i5", url.Values{"state": {state}})
		})
	}
}

// newCalls is every new call, each as a function of one client, so a table can drive them all.
func newCalls() map[string]func(context.Context, *trello.Client) error {
	text := "x"
	return map[string]func(context.Context, *trello.Client) error{
		"BoardCards": func(ctx context.Context, c *trello.Client) error { _, err := c.BoardCards(ctx, "b1"); return err },
		"GetCard":    func(ctx context.Context, c *trello.Client) error { _, err := c.GetCard(ctx, "c1"); return err },
		"BoardComments": func(ctx context.Context, c *trello.Client) error {
			_, err := c.BoardComments(ctx, "b1")
			return err
		},
		"UpdateCard": func(ctx context.Context, c *trello.Client) error {
			return c.UpdateCard(ctx, "c1", trello.UpdateCardFields{Name: &text})
		},
		"AddComment": func(ctx context.Context, c *trello.Client) error { _, err := c.AddComment(ctx, "c1", "x"); return err },
		"CreateChecklist": func(ctx context.Context, c *trello.Client) error {
			_, err := c.CreateChecklist(ctx, "c1", "x")
			return err
		},
		"AddCheckItem": func(ctx context.Context, c *trello.Client) error {
			_, err := c.AddCheckItem(ctx, "cl1", "x", false)
			return err
		},
		"SetCheckItem": func(ctx context.Context, c *trello.Client) error { return c.SetCheckItem(ctx, "c1", "i1", true) },
	}
}

func wantNoCredential(t *testing.T, err error) {
	t.Helper()
	for _, secret := range []string{testKey, testToken, "key=", "token="} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("the error %q holds %q", err, secret)
		}
	}
}

func TestAnUnreachableTrelloErrorHoldsNoCredential(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	address := closed.URL
	closed.Close()
	client := trello.NewClient(testKey, testToken, trello.WithBaseURL(address))

	for name, call := range newCalls() {
		t.Run(name, func(t *testing.T) {
			err := call(context.Background(), client)
			if err == nil {
				t.Fatal("a call to a closed server succeeded")
			}
			wantNoCredential(t, err)
			if !strings.Contains(err.Error(), "reach Trello") {
				t.Errorf("error = %q, want it to say Trello could not be reached", err)
			}
		})
	}
}

func TestAnErrorFromTrelloIsAnAPIErrorWithNoCredential(t *testing.T) {
	rec := newRecorder(t, "invalid token")
	rec.status = http.StatusUnauthorized
	client := rec.client()

	for name, call := range newCalls() {
		t.Run(name, func(t *testing.T) {
			err := call(context.Background(), client)
			var apiErr *trello.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want an *APIError", err)
			}
			if apiErr.Status != http.StatusUnauthorized || apiErr.Body != "invalid token" {
				t.Errorf("APIError = %+v, want the status and what Trello said", apiErr)
			}
			if strings.Contains(apiErr.Path, "?") {
				t.Errorf("APIError path = %q, want the path with no query", apiErr.Path)
			}
			wantNoCredential(t, err)
		})
	}
}

func TestAnAPIErrorKeepsOnlyTheStartOfALongBody(t *testing.T) {
	rec := newRecorder(t, strings.Repeat("x", 5000))
	rec.status = http.StatusBadGateway

	err := rec.client().SetCheckItem(context.Background(), "c1", "i1", true)

	var apiErr *trello.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an *APIError", err)
	}
	if len(apiErr.Body) != 512 {
		t.Errorf("body holds %d bytes, want it cut to 512", len(apiErr.Body))
	}
}

func TestIsNotFoundOnlyAnswersForAMissingThing(t *testing.T) {
	rec := newRecorder(t, "the card is gone")
	rec.status = http.StatusNotFound
	notFound := rec.client().SetCheckItem(context.Background(), "c1", "i1", true)

	rec.mu.Lock()
	rec.status = http.StatusTooManyRequests
	rec.mu.Unlock()
	tooMany := rec.client().SetCheckItem(context.Background(), "c1", "i1", true)

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"a 404", notFound, true},
		{"a 404 wrapped", fmt.Errorf("write the tick: %w", notFound), true},
		{"a 429", tooMany, false},
		{"an error that is not Trello's", errors.New("a 404 in the words only"), false},
		{"no error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trello.IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnAnswerThatIsNotJSONIsAnErrorWithNoCredential(t *testing.T) {
	rec := newRecorder(t, "<html>not json</html>")

	for name, call := range newCalls() {
		if name == "UpdateCard" || name == "SetCheckItem" {
			continue // These read no answer, so there is nothing to misread.
		}
		t.Run(name, func(t *testing.T) {
			err := call(context.Background(), rec.client())
			if err == nil {
				t.Fatal("an answer that is not JSON was accepted")
			}
			wantNoCredential(t, err)
		})
	}
}

func TestACanceledCallKeepsTheCancellationVisible(t *testing.T) {
	rec := newRecorder(t, `[]`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rec.client().SetCheckItem(ctx, "c1", "i1", true)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to wrap context.Canceled, which is how Apply knows to stop", err)
	}
	wantNoCredential(t, err)
}
