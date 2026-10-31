package integrations_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/cardpanel"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The setup the Trello sync tests share: a daemon with a board linked, a fake Trello behind it
// (trello_fake_test.go), and the real card panel for checklists and comments. The tests are in
// trello_engine_test.go, trello_pending_test.go, and trello_apply_test.go.

const (
	engineProject = "small-repo"
	engineOwner   = "owner-1"
)

func ptr[T any](v T) *T { return &v }

// fakeBoard is the board module the sync works through: cards in memory, with a row in the store for
// each so the sync's own tables have a card to point at.
type fakeBoard struct {
	t     *testing.T
	store *store.Store

	mu    sync.Mutex
	cards map[string]*protocol.Card
	order []string
	// calls lists what the sync asked of the board: "update card-1" and "move card-1 done".
	calls []string
}

func newFakeBoard(t *testing.T, st *store.Store) *fakeBoard {
	return &fakeBoard{t: t, store: st, cards: map[string]*protocol.Card{}}
}

func (b *fakeBoard) CreateCard(ctx context.Context, projectID string, in protocol.CreateCardRequest) (protocol.Card, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	number := len(b.order) + 1
	id := fmt.Sprintf("card-%d", number)
	now := time.Now().UnixMilli()
	err := b.store.Write(ctx, func(q *db.Queries) error {
		return q.CreateCard(ctx, db.CreateCardParams{
			ID: id, ProjectID: projectID, Number: int64(number), BoardID: "board-" + projectID,
			Title: in.Title, State: string(protocol.CardStateBacklog), AgentKind: "claude",
			PermissionMode: "auto-edits", CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card := &protocol.Card{
		ID: id, ProjectID: projectID, Number: number, Title: in.Title, Body: in.Body, State: protocol.CardStateBacklog,
	}
	b.cards[id] = card
	b.order = append(b.order, id)
	return *card, nil
}

func (b *fakeBoard) MoveCard(_ context.Context, id string, in protocol.MoveCardRequest) (protocol.Card, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, fmt.Sprintf("move %s %s", id, in.State))
	b.cards[id].State = in.State
	return *b.cards[id], nil
}

func (b *fakeBoard) Card(_ context.Context, id string) (protocol.Card, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	card, ok := b.cards[id]
	if !ok {
		return protocol.Card{}, protocol.NotFound("card").With("id", id)
	}
	return *card, nil
}

func (b *fakeBoard) Cards(_ context.Context, projectID string) ([]protocol.Card, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []protocol.Card{}
	for _, id := range b.order {
		if b.cards[id].ProjectID == projectID {
			out = append(out, *b.cards[id])
		}
	}
	return out, nil
}

func (b *fakeBoard) UpdateCard(_ context.Context, id string, in protocol.UpdateCardRequest) (protocol.Card, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "update "+id)
	if in.Title != nil {
		b.cards[id].Title = *in.Title
	}
	if in.Body != nil {
		b.cards[id].Body = *in.Body
	}
	return *b.cards[id], nil
}

// The methods below are what a person does in Marshal. They are not recorded in calls.

// add makes a card in Marshal that Trello has never heard of.
func (b *fakeBoard) add(title, body string) string {
	b.t.Helper()
	card, err := b.CreateCard(context.Background(), engineProject, protocol.CreateCardRequest{Title: title, Body: body})
	if err != nil {
		b.t.Fatalf("add a card: %v", err)
	}
	return card.ID
}

func (b *fakeBoard) edit(id string, title, body *string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if title != nil {
		b.cards[id].Title = *title
	}
	if body != nil {
		b.cards[id].Body = *body
	}
}

func (b *fakeBoard) setState(id string, state protocol.CardState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cards[id].State = state
}

func (b *fakeBoard) get(id string) protocol.Card {
	card, err := b.Card(context.Background(), id)
	if err != nil {
		b.t.Fatalf("read card %s: %v", id, err)
	}
	return card
}

func (b *fakeBoard) callList() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string{}, b.calls...)
}

// trelloPanel is the real card panel, adapted the way cmd/marshald adapts it for the sync.
type trelloPanel struct{ svc *cardpanel.Service }

func (p trelloPanel) Checklists(ctx context.Context, cardID string) (protocol.ChecklistList, error) {
	return p.svc.Checklists(ctx, cardID)
}

func (p trelloPanel) CreateChecklist(ctx context.Context, cardID, name string) (protocol.ChecklistList, error) {
	return p.svc.CreateChecklist(ctx, cardID, protocol.CreateChecklistRequest{Name: name})
}

func (p trelloPanel) AddItem(ctx context.Context, cardID, listID, text string) (protocol.ChecklistList, error) {
	return p.svc.AddItem(ctx, cardID, listID, protocol.AddChecklistItemRequest{Text: text})
}

func (p trelloPanel) SetItem(ctx context.Context, cardID, listID, itemID string, done bool, userID string) (protocol.ChecklistList, error) {
	return p.svc.Tick(ctx, cardID, listID, itemID, done, cardpanel.Actor{UserID: userID})
}

func (p trelloPanel) Comments(ctx context.Context, cardID string) (protocol.CommentList, error) {
	return p.svc.Comments(ctx, cardID)
}

func (p trelloPanel) ImportComment(ctx context.Context, cardID, body, userID string, at time.Time) (string, error) {
	return p.svc.ImportComment(ctx, cardID, body, userID, at)
}

// recordingAgent is the card's agent as the panel sees it. A test that expects it never to be reached
// reads sent.
type recordingAgent struct {
	mu   sync.Mutex
	sent []string
}

func (a *recordingAgent) Send(_ context.Context, _, text string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sent = append(a.sent, text)
	return nil
}

func (a *recordingAgent) messages() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.sent...)
}

// testLog writes a log line as part of the test's own output.
type testLog struct{ t *testing.T }

func (l testLog) Write(p []byte) (int, error) {
	l.t.Logf("%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// engine is a daemon with a Trello board linked to the project small-repo, a fake Trello behind it,
// and the real card panel for checklists and comments.
type engine struct {
	t        *testing.T
	f        *fixture
	trello   *fakeTrello
	board    *fakeBoard
	panel    *cardpanel.Service
	agent    *recordingAgent
	notified chan string
}

// newEngine builds the engine. A caller may change the saved link before it is saved.
func newEngine(t *testing.T, link ...func(*protocol.SaveTrelloRequest)) *engine {
	t.Helper()
	tr := newFakeTrello(t)
	logger := slog.New(slog.NewTextHandler(testLog{t}, &slog.HandlerOptions{Level: slog.LevelWarn}))
	f := newFixture(t, func(o *integrations.Options) {
		o.TrelloBaseURL = tr.URL
		o.Logger = logger
	})
	e := &engine{t: t, f: f, trello: tr, agent: &recordingAgent{}, notified: make(chan string, 64)}
	e.seedStore()
	save := trelloSave()
	for _, change := range link {
		change(&save)
	}
	if err := f.svc.SaveTrello(context.Background(), save); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	e.board = newFakeBoard(t, f.store)
	panel, err := cardpanel.New(cardpanel.Deps{Store: f.store, Agent: e.agent, Log: logger})
	if err != nil {
		t.Fatalf("make the card panel: %v", err)
	}
	e.panel = panel
	f.svc.SetTrelloCards(e.board)
	f.svc.SetTrelloPanel(trelloPanel{svc: panel})
	f.svc.SetTrelloNotifier(func(projectID string) {
		select {
		case e.notified <- projectID:
		default:
		}
	})
	f.svc.SetTrelloPaceForTest(0)
	return e
}

// seedStore makes the owner, the project, and its board, which the sync's own rows point at.
func (e *engine) seedStore() {
	e.t.Helper()
	ctx := context.Background()
	now := time.Now().UnixMilli()
	err := e.f.store.Write(ctx, func(q *db.Queries) error {
		if _, err := q.CreateUser(ctx, db.CreateUserParams{ID: engineOwner, Name: "Blair", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if err := q.CreateProject(ctx, db.CreateProjectParams{
			ID: engineProject, Name: engineProject, RepoPath: "/code/" + engineProject, DefaultBranch: "main",
			PackagesJSON: "[]", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return q.CreateBoard(ctx, db.CreateBoardParams{
			ID: "board-" + engineProject, ProjectID: engineProject, ColumnsJSON: `["backlog","working","needs","done"]`,
		})
	})
	if err != nil {
		e.t.Fatalf("seed the store: %v", err)
	}
}

func (e *engine) refresh(keep ...string) integrations.TrelloRefreshCounts {
	e.t.Helper()
	counts, err := e.f.svc.RefreshTrello(context.Background(), engineProject, keep)
	if err != nil {
		e.t.Fatalf("RefreshTrello: %v", err)
	}
	return counts
}

func (e *engine) pending() protocol.TrelloPending {
	e.t.Helper()
	pending, err := e.f.svc.PendingTrello(context.Background(), engineProject)
	if err != nil {
		e.t.Fatalf("PendingTrello: %v", err)
	}
	return pending
}

func (e *engine) apply(req protocol.ApplyTrelloRequest) protocol.ApplyTrelloResult {
	e.t.Helper()
	result, err := e.f.svc.ApplyTrello(context.Background(), engineProject, req)
	if err != nil {
		e.t.Fatalf("ApplyTrello: %v", err)
	}
	return result
}

// applyAll applies every change that is pending now, with no overrides.
func (e *engine) applyAll() protocol.ApplyTrelloResult {
	e.t.Helper()
	return e.apply(protocol.ApplyTrelloRequest{ChangeIDs: changeIDs(e.pending().Changes)})
}

func changeIDs(changes []protocol.TrelloChange) []string {
	ids := make([]string, 0, len(changes))
	for _, c := range changes {
		ids = append(ids, c.ID)
	}
	return ids
}

func changeKinds(changes []protocol.TrelloChange) []protocol.TrelloChangeKind {
	kinds := make([]protocol.TrelloChangeKind, 0, len(changes))
	for _, c := range changes {
		kinds = append(kinds, c.Kind)
	}
	return kinds
}

// changeOf finds the one pending change of a kind on a card. It fails the test if there is not one.
func (e *engine) changeOf(pending protocol.TrelloPending, kind protocol.TrelloChangeKind, cardID string) protocol.TrelloChange {
	e.t.Helper()
	var found []protocol.TrelloChange
	for _, c := range pending.Changes {
		if c.Kind == kind && c.CardID == cardID {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		e.t.Fatalf("pending holds %d %s changes on %s, want 1: %+v", len(found), kind, cardID, pending.Changes)
	}
	return found[0]
}

// marshalCard is the Marshal card a Trello card was imported as.
func (e *engine) marshalCard(trelloID string) string {
	e.t.Helper()
	id, ok, err := e.f.svc.ExternalCard(context.Background(), "trello", trelloID)
	if err != nil || !ok {
		e.t.Fatalf("ExternalCard(%s) = %q, %v, %v, want a linked card", trelloID, id, ok, err)
	}
	return id
}

func (e *engine) checklists(cardID string) []protocol.Checklist {
	e.t.Helper()
	lists, err := e.panel.Checklists(context.Background(), cardID)
	if err != nil {
		e.t.Fatalf("Checklists: %v", err)
	}
	return lists.Checklists
}

func (e *engine) comments(cardID string) []protocol.Comment {
	e.t.Helper()
	list, err := e.panel.Comments(context.Background(), cardID)
	if err != nil {
		e.t.Fatalf("Comments: %v", err)
	}
	return list.Comments
}

// addChecklist is a person adding a checklist with lines in Marshal. It answers the list's id and the
// lines' ids in order.
func (e *engine) addChecklist(cardID, name string, lines ...string) (string, []string) {
	e.t.Helper()
	ctx := context.Background()
	lists, err := e.panel.CreateChecklist(ctx, cardID, protocol.CreateChecklistRequest{Name: name})
	if err != nil {
		e.t.Fatalf("CreateChecklist: %v", err)
	}
	listID := lists.Checklists[len(lists.Checklists)-1].ID
	for _, line := range lines {
		if _, err := e.panel.AddItem(ctx, cardID, listID, protocol.AddChecklistItemRequest{Text: line}); err != nil {
			e.t.Fatalf("AddItem: %v", err)
		}
	}
	var ids []string
	for _, l := range e.checklists(cardID) {
		if l.ID == listID {
			for _, it := range l.Items {
				ids = append(ids, it.ID)
			}
		}
	}
	return listID, ids
}

func (e *engine) addLine(cardID, listID, text string) {
	e.t.Helper()
	if _, err := e.panel.AddItem(context.Background(), cardID, listID, protocol.AddChecklistItemRequest{Text: text}); err != nil {
		e.t.Fatalf("AddItem: %v", err)
	}
}

func (e *engine) tick(cardID, listID, itemID string, done bool) {
	e.t.Helper()
	if _, err := e.panel.Tick(context.Background(), cardID, listID, itemID, done, cardpanel.Actor{UserID: engineOwner}); err != nil {
		e.t.Fatalf("Tick: %v", err)
	}
}

// say is the person commenting in Marshal. The text must not ask the agent anything.
func (e *engine) say(cardID, body string) {
	e.t.Helper()
	if _, err := e.panel.Post(context.Background(), cardID, protocol.PostCommentRequest{Body: body}, cardpanel.Actor{UserID: engineOwner}); err != nil {
		e.t.Fatalf("Post: %v", err)
	}
}

// agentSays is the card's agent commenting in Marshal.
func (e *engine) agentSays(cardID, body string) {
	e.t.Helper()
	if _, err := e.panel.Post(context.Background(), cardID, protocol.PostCommentRequest{Body: body}, cardpanel.Actor{Agent: true}); err != nil {
		e.t.Fatalf("Post as the agent: %v", err)
	}
}

// seedFixLogin puts one card on the fake board the way a team might keep it: a description, a
// checklist with one line done, and two comments.
func seedFixLogin(tr *fakeTrello) {
	tr.addCard("tc-1", "list-todo", "Fix login", "Users cannot log in")
	tr.addChecklist("tc-1", "tcl-1", "Steps", checkItem("tci-1", "reproduce", false), checkItem("tci-2", "write test", true))
	tr.comment("tc-1", "Ada Lovelace", "Looks like a cookie issue")
	tr.comment("tc-1", "Grace Hopper", "Same here")
}

// importedFixLogin seeds the card and reads the board once. It answers the Marshal card's id.
func (e *engine) importedFixLogin() string {
	e.t.Helper()
	seedFixLogin(e.trello)
	e.refresh()
	return e.marshalCard("tc-1")
}
