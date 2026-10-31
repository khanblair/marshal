package integrations_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
)

// fakeTrello is a Trello board in memory behind an httptest server. A test changes it as a person in
// Trello would, and the sync reads and writes it through the real client. Every request is recorded,
// and one with the wrong key or token is refused, so a test also proves who a write was made as.

const (
	fakeBoardID = "board-1"
	fakeKey     = "a-key"
	fakeToken   = "a-token"
	// fakeTokenOwner is who a comment posted with the token is shown as in Trello.
	fakeTokenOwner = "Blair Khan"
)

// trelloRequest is one request the fake saw. A read has no form.
type trelloRequest struct {
	method string
	path   string
	form   url.Values
}

type fakeComment struct {
	id, cardID, text, author string
	at                       time.Time
}

type fakeTrello struct {
	*httptest.Server
	t *testing.T

	mu        sync.Mutex
	boardName string
	lists     []trello.List
	cards     []*trello.Card
	comments  []fakeComment
	counter   int
	log       []trelloRequest
	// fail answers a request with a status instead of doing it. The key is "METHOD /path".
	fail map[string]int
}

func newFakeTrello(t *testing.T) *fakeTrello {
	t.Helper()
	f := &fakeTrello{
		t: t, boardName: "Sprint Board", fail: map[string]int{},
		lists: []trello.List{{ID: "list-todo", Name: "To do"}, {ID: "list-doing", Name: "Doing"}, {ID: "list-done", Name: "Done"}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boards/{board}", f.wrap(f.getBoard))
	mux.HandleFunc("GET /boards/{board}/lists", f.wrap(f.getLists))
	mux.HandleFunc("GET /boards/{board}/cards/open", f.wrap(f.getOpenCards))
	mux.HandleFunc("GET /boards/{board}/actions", f.wrap(f.getComments))
	mux.HandleFunc("GET /cards/{card}", f.wrap(f.getCard))
	mux.HandleFunc("PUT /cards/{card}", f.wrap(f.putCard))
	mux.HandleFunc("POST /cards", f.wrap(f.postCard))
	mux.HandleFunc("POST /cards/{card}/actions/comments", f.wrap(f.postComment))
	mux.HandleFunc("PUT /cards/{card}/checkItem/{item}", f.wrap(f.putCheckItem))
	mux.HandleFunc("POST /checklists", f.wrap(f.postChecklist))
	mux.HandleFunc("POST /checklists/{list}/checkItems", f.wrap(f.postCheckItem))
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

// wrap records a request, checks who it is made as, applies an injected failure, and runs the handler
// with the lock held.
func (f *fakeTrello) wrap(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		f.log = append(f.log, trelloRequest{method: r.Method, path: r.URL.Path, form: r.PostForm})
		// Trello takes the credential from the address or from a form body, so both are read.
		if r.Form.Get("key") != fakeKey || r.Form.Get("token") != fakeToken {
			f.t.Errorf("%s %s was made with key %q and token %q, want the linked person's",
				r.Method, r.URL.Path, r.Form.Get("key"), r.Form.Get("token"))
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if status, ok := f.fail[r.Method+" "+r.URL.Path]; ok {
			http.Error(w, "injected failure", status)
			return
		}
		handler(w, r)
	}
}

func (f *fakeTrello) nextID(prefix string) string {
	f.counter++
	return fmt.Sprintf("%s-%d", prefix, f.counter)
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func copyCard(c *trello.Card) trello.Card {
	out := *c
	out.Checklists = make([]trello.Checklist, len(c.Checklists))
	for i, cl := range c.Checklists {
		out.Checklists[i] = cl
		out.Checklists[i].Items = append([]trello.CheckItem{}, cl.Items...)
	}
	return out
}

func (f *fakeTrello) findCard(id string) *trello.Card {
	for _, c := range f.cards {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func (f *fakeTrello) getBoard(w http.ResponseWriter, _ *http.Request) {
	reply(w, trello.Board{ID: fakeBoardID, Name: f.boardName, URL: "https://trello.com/b/" + fakeBoardID})
}

func (f *fakeTrello) getLists(w http.ResponseWriter, _ *http.Request) {
	reply(w, f.lists)
}

func (f *fakeTrello) getOpenCards(w http.ResponseWriter, _ *http.Request) {
	out := []trello.Card{}
	for _, c := range f.cards {
		if !c.Closed {
			out = append(out, copyCard(c))
		}
	}
	reply(w, out)
}

func (f *fakeTrello) getCard(w http.ResponseWriter, r *http.Request) {
	c := f.findCard(r.PathValue("card"))
	if c == nil {
		http.Error(w, "that card is gone", http.StatusNotFound)
		return
	}
	reply(w, copyCard(c))
}

func (f *fakeTrello) putCard(w http.ResponseWriter, r *http.Request) {
	c := f.findCard(r.PathValue("card"))
	if c == nil {
		http.Error(w, "that card is gone", http.StatusNotFound)
		return
	}
	if r.Form.Has("name") {
		c.Name = r.Form.Get("name")
	}
	if r.Form.Has("desc") {
		c.Desc = r.Form.Get("desc")
	}
	if r.Form.Has("idList") {
		c.IDList = r.Form.Get("idList")
	}
	reply(w, copyCard(c))
}

func (f *fakeTrello) postCard(w http.ResponseWriter, r *http.Request) {
	// The prefix keeps a card Marshal makes from sharing an id with one a test seeded by hand.
	id := f.nextID("made")
	c := &trello.Card{
		ID: id, Name: r.Form.Get("name"), Desc: r.Form.Get("desc"), IDList: r.Form.Get("idList"),
		URL: "https://trello.com/c/" + id,
	}
	f.cards = append(f.cards, c)
	reply(w, copyCard(c))
}

func (f *fakeTrello) postComment(w http.ResponseWriter, r *http.Request) {
	cardID := r.PathValue("card")
	if f.findCard(cardID) == nil {
		http.Error(w, "that card is gone", http.StatusNotFound)
		return
	}
	id := f.addCommentLocked(cardID, fakeTokenOwner, r.Form.Get("text"))
	reply(w, map[string]string{"id": id})
}

func (f *fakeTrello) postChecklist(w http.ResponseWriter, r *http.Request) {
	c := f.findCard(r.Form.Get("idCard"))
	if c == nil {
		http.Error(w, "that card is gone", http.StatusNotFound)
		return
	}
	list := trello.Checklist{ID: f.nextID("cl"), Name: r.Form.Get("name"), Items: []trello.CheckItem{}}
	c.Checklists = append(c.Checklists, list)
	reply(w, list)
}

func (f *fakeTrello) postCheckItem(w http.ResponseWriter, r *http.Request) {
	for _, c := range f.cards {
		for i := range c.Checklists {
			if c.Checklists[i].ID != r.PathValue("list") {
				continue
			}
			item := trello.CheckItem{ID: f.nextID("ci"), Name: r.Form.Get("name"), State: "incomplete"}
			if r.Form.Get("checked") == "true" {
				item.State = "complete"
			}
			c.Checklists[i].Items = append(c.Checklists[i].Items, item)
			reply(w, item)
			return
		}
	}
	http.Error(w, "that checklist is gone", http.StatusNotFound)
}

func (f *fakeTrello) putCheckItem(w http.ResponseWriter, r *http.Request) {
	c := f.findCard(r.PathValue("card"))
	if c != nil {
		for i := range c.Checklists {
			for j := range c.Checklists[i].Items {
				if c.Checklists[i].Items[j].ID == r.PathValue("item") {
					c.Checklists[i].Items[j].State = r.Form.Get("state")
					reply(w, c.Checklists[i].Items[j])
					return
				}
			}
		}
	}
	http.Error(w, "that item is gone", http.StatusNotFound)
}

// getComments answers a board's comment actions newest first, after the one named by before.
func (f *fakeTrello) getComments(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	skipping := r.URL.Query().Get("before") != ""
	for i := len(f.comments) - 1; i >= 0; i-- {
		c := f.comments[i]
		if skipping {
			skipping = c.id != r.URL.Query().Get("before")
			continue
		}
		out = append(out, map[string]any{
			"id": c.id, "date": c.at.UTC().Format("2006-01-02T15:04:05.000Z"),
			"data":          map[string]any{"text": c.text, "card": map[string]any{"id": c.cardID}},
			"memberCreator": map[string]any{"fullName": c.author, "username": "user"},
		})
	}
	reply(w, out)
}

func (f *fakeTrello) addCommentLocked(cardID, author, text string) string {
	id := f.nextID("ac")
	at := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC).Add(time.Duration(len(f.comments)) * time.Minute)
	f.comments = append(f.comments, fakeComment{id: id, cardID: cardID, text: text, author: author, at: at})
	return id
}

// The methods below are what a test does to the board, as a person in Trello would.

func (f *fakeTrello) setLists(lists ...trello.List) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = lists
}

func (f *fakeTrello) addCard(id, listID, name, desc string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cards = append(f.cards, &trello.Card{
		ID: id, Name: name, Desc: desc, IDList: listID, URL: "https://trello.com/c/" + id,
	})
}

// edit changes a card the way a person in Trello would. It fails the test when the card is not there.
func (f *fakeTrello) edit(id string, change func(c *trello.Card)) {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.findCard(id)
	if c == nil {
		f.t.Fatalf("the fake Trello has no card %q", id)
	}
	change(c)
}

func (f *fakeTrello) addChecklist(cardID, id, name string, items ...trello.CheckItem) {
	f.edit(cardID, func(c *trello.Card) {
		c.Checklists = append(c.Checklists, trello.Checklist{ID: id, Name: name, Items: items})
	})
}

func checkItem(id, name string, done bool) trello.CheckItem {
	state := "incomplete"
	if done {
		state = "complete"
	}
	return trello.CheckItem{ID: id, Name: name, State: state}
}

func (f *fakeTrello) tick(cardID, itemID string, done bool) {
	f.edit(cardID, func(c *trello.Card) {
		for i := range c.Checklists {
			for j := range c.Checklists[i].Items {
				if c.Checklists[i].Items[j].ID == itemID {
					c.Checklists[i].Items[j] = checkItem(itemID, c.Checklists[i].Items[j].Name, done)
				}
			}
		}
	})
}

func (f *fakeTrello) comment(cardID, author, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCommentLocked(cardID, author, text)
}

func (f *fakeTrello) remove(cardID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, c := range f.cards {
		if c.ID == cardID {
			f.cards = append(f.cards[:i], f.cards[i+1:]...)
			return
		}
	}
}

func (f *fakeTrello) failWith(method, path string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[method+" "+path] = status
}

// card is a copy of what Trello holds for a card now.
func (f *fakeTrello) card(id string) trello.Card {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.findCard(id)
	if c == nil {
		f.t.Fatalf("the fake Trello has no card %q", id)
	}
	return copyCard(c)
}

func (f *fakeTrello) cardCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.cards)
}

func (f *fakeTrello) commentTexts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, c := range f.comments {
		out = append(out, c.text)
	}
	return out
}

// writes are the requests that changed something, in order.
func (f *fakeTrello) writes() []trelloRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []trelloRequest{}
	for _, r := range f.log {
		if r.method != http.MethodGet {
			out = append(out, r)
		}
	}
	return out
}

// count is how many requests of a method and path were made.
func (f *fakeTrello) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.log {
		if r.method == method && r.path == path {
			n++
		}
	}
	return n
}

func (f *fakeTrello) clearLog() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = nil
}

// cardNamed is a copy of the card Trello holds with a name. It fails the test when there is none.
func (f *fakeTrello) cardNamed(name string) trello.Card {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cards {
		if c.Name == name {
			return copyCard(c)
		}
	}
	f.t.Fatalf("the fake Trello has no card named %q", name)
	return trello.Card{}
}

// writeCounts is how many times each write was made, keyed "METHOD /path".
func (f *fakeTrello) writeCounts() map[string]int {
	out := map[string]int{}
	for _, r := range f.writes() {
		out[r.method+" "+r.path]++
	}
	return out
}
