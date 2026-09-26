package history_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// testTime is the clock every test store is given, so a stored time is exact.
var testTime = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

// env is a history store on a real database, with one card and its session row, which the stored
// events' own foreign keys need.
type env struct {
	store     *store.Store
	history   *history.Store
	cardID    string
	sessionID string
}

// newEnv opens the daemon's store and a history store on it, and writes one card with its session.
func newEnv(t *testing.T, opts ...history.Option) *env {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	cardID, sessionID := addCard(t, st, "1")
	h, err := history.New(st, append([]history.Option{history.WithClock(func() time.Time { return testTime })}, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &env{store: st, history: h, cardID: cardID, sessionID: sessionID}
}

// addCard writes a project, a board, a card, and a session row, and returns the card and session
// ids.
func addCard(t *testing.T, st *store.Store, suffix string) (string, string) {
	t.Helper()
	ctx := context.Background()
	now := testTime.UnixMilli()
	projectID, boardID, cardID, sessionID := "p"+suffix, "b"+suffix, "c"+suffix, "s"+suffix
	err := st.Write(ctx, func(q *db.Queries) error {
		if err := q.CreateProject(ctx, db.CreateProjectParams{
			ID: projectID, Name: "Project " + suffix, RepoPath: "/tmp/" + suffix,
			DefaultBranch: "main", PackagesJSON: "[]", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		if err := q.CreateBoard(ctx, db.CreateBoardParams{ID: boardID, ProjectID: projectID, ColumnsJSON: "[]"}); err != nil {
			return err
		}
		if err := q.CreateCard(ctx, db.CreateCardParams{
			ID: cardID, ProjectID: projectID, Number: 1, BoardID: boardID, Title: "A card",
			State: "working", AgentKind: "claude", PermissionMode: "auto-edits", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return q.CreateCardSession(ctx, db.CreateCardSessionParams{
			ID: sessionID, CardID: cardID, AgentKind: "claude", State: "awake",
			LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("write the card and its session: %v", err)
	}
	return cardID, sessionID
}

// appends stores one agent record per summary, in one batch.
func appends(t *testing.T, h *history.Store, cardID, sessionID string, summaries ...string) {
	t.Helper()
	records := make([]history.Record, len(summaries))
	for i, summary := range summaries {
		records[i] = history.Record{Kind: history.KindAgent, Summary: summary}
	}
	if err := h.Append(context.Background(), cardID, sessionID, records); err != nil {
		t.Fatalf("Append(%q): %v", summaries, err)
	}
}

func TestNewRefusesAMissingStore(t *testing.T) {
	if _, err := history.New(nil); err == nil {
		t.Fatal("New(nil) = nil error, want one")
	}
}

func TestAppendNumbersACardsEventsFromOne(t *testing.T) {
	e := newEnv(t)
	appends(t, e.history, e.cardID, e.sessionID, "one", "two", "three")

	page, err := e.history.Page(context.Background(), e.cardID, 0, 10)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if page.More {
		t.Error("More = true, want false on the only page")
	}
	want := []string{"three", "two", "one"}
	if len(page.Events) != len(want) {
		t.Fatalf("got %d events, want %d", len(page.Events), len(want))
	}
	seen := map[string]bool{}
	for i, ev := range page.Events {
		if ev.Summary != want[i] {
			t.Errorf("event %d = %q, want %q", i, ev.Summary, want[i])
		}
		if ev.Seq != int64(len(want)-i) {
			t.Errorf("event %q has seq %d, want %d", ev.Summary, ev.Seq, len(want)-i)
		}
		if ev.CardID != e.cardID || ev.SessionID != e.sessionID || ev.Kind != history.KindAgent {
			t.Errorf("event %q = %+v", ev.Summary, ev)
		}
		if !ev.At.Equal(testTime) {
			t.Errorf("event %q happened at %s, want %s", ev.Summary, ev.At, testTime)
		}
		if seen[ev.ID] {
			t.Errorf("two events share the id %s", ev.ID)
		}
		seen[ev.ID] = true
	}
	if page.Cursor != 1 {
		t.Errorf("Cursor = %d, want the oldest sequence, 1", page.Cursor)
	}
}

func TestAppendKeepsEachCardsSequenceItsOwn(t *testing.T) {
	e := newEnv(t)
	other, otherSession := addCard(t, e.store, "2")
	appends(t, e.history, e.cardID, e.sessionID, "a", "b")
	appends(t, e.history, other, otherSession, "c")

	page, err := e.history.Page(context.Background(), other, 0, 10)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0].Seq != 1 {
		t.Errorf("the second card's events = %+v, want one event at seq 1", page.Events)
	}
}

func TestConcurrentAppendsNeverReuseASequence(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				err := e.history.Append(context.Background(), e.cardID, e.sessionID,
					[]history.Record{{Kind: history.KindAgent, Summary: "tick"}})
				if err != nil {
					t.Errorf("Append: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	page, err := e.history.Page(context.Background(), e.cardID, 0, 100)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(page.Events) != 20 {
		t.Fatalf("got %d events, want 20", len(page.Events))
	}
	for i, ev := range page.Events {
		if wantSeq := int64(len(page.Events) - i); ev.Seq != wantSeq {
			t.Fatalf("event %d has seq %d, want %d: the sequence has a gap or a repeat", i, ev.Seq, wantSeq)
		}
	}
}

func TestAppendRefusesRecordsItCouldNotReadBack(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for name, records := range map[string][]history.Record{
		"an unknown kind":  {{Kind: "nonsense", Summary: "x"}},
		"an unknown state": {{Kind: history.KindToolCall, State: "nonsense", Summary: "x"}},
	} {
		if err := e.history.Append(ctx, e.cardID, e.sessionID, records); err == nil {
			t.Errorf("Append with %s = nil error, want one", name)
		}
	}
	page, err := e.history.Page(ctx, e.cardID, 0, 10)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(page.Events) != 0 {
		t.Errorf("a refused append stored %d events", len(page.Events))
	}
}

func TestAppendRefusesMissingArgumentsAndEmptyBatches(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	records := []history.Record{{Kind: history.KindAgent, Summary: "x"}}
	if err := e.history.Append(ctx, e.cardID, e.sessionID, nil); err != nil {
		t.Errorf("Append with no records = %v, want nil", err)
	}
	if err := e.history.Append(ctx, "", e.sessionID, records); err == nil {
		t.Error("Append with no card = nil error, want one")
	}
	if err := e.history.Append(ctx, e.cardID, "", records); err == nil {
		t.Error("Append with no session = nil error, want one")
	}
}

func TestAppendReportsACardOrSessionThatIsNotThere(t *testing.T) {
	e := newEnv(t)
	record := []history.Record{{Kind: history.KindAgent, Summary: "x"}}
	if err := e.history.Append(context.Background(), "no-such-card", e.sessionID, record); err == nil {
		t.Error("Append for a card that does not exist = nil error, want one")
	}
	if err := e.history.Append(context.Background(), e.cardID, "no-such-session", record); err == nil {
		t.Error("Append for a session that does not exist = nil error, want one")
	}
}

func TestAppendReportsACancelledContext(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := e.history.Append(ctx, e.cardID, e.sessionID, []history.Record{{Kind: history.KindAgent, Summary: "x"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Append with a cancelled context = %v, want a cancelled error", err)
	}
}

func TestAppendReportsAnEntropyReaderThatFails(t *testing.T) {
	e := newEnv(t, history.WithEntropy(bytes.NewReader(nil)))
	err := e.history.Append(context.Background(), e.cardID, e.sessionID,
		[]history.Record{{Kind: history.KindAgent, Summary: "x"}})
	if err == nil {
		t.Fatal("Append with an empty entropy reader = nil error, want one")
	}
}

func TestOptionsIgnoreNilValues(t *testing.T) {
	e := newEnv(t, history.WithClock(nil), history.WithEntropy(nil))
	appends(t, e.history, e.cardID, e.sessionID, "still works")
	page, err := e.history.Page(context.Background(), e.cardID, 0, 10)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(page.Events) != 1 {
		t.Errorf("got %d events, want 1", len(page.Events))
	}
}

func TestPageWalksBackwardsByCursor(t *testing.T) {
	e := newEnv(t)
	appends(t, e.history, e.cardID, e.sessionID, "one", "two", "three", "four", "five")
	ctx := context.Background()

	first, err := e.history.Page(ctx, e.cardID, 0, 2)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if !first.More || first.Cursor != 4 {
		t.Fatalf("first page = %+v, want More and cursor 4", first)
	}
	second, err := e.history.Page(ctx, e.cardID, first.Cursor, 2)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if !second.More || second.Cursor != 2 {
		t.Fatalf("second page = %+v, want More and cursor 2", second)
	}
	third, err := e.history.Page(ctx, e.cardID, second.Cursor, 2)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if third.More || third.Cursor != 1 || len(third.Events) != 1 || third.Events[0].Summary != "one" {
		t.Fatalf("third page = %+v, want the last event with no more", third)
	}
	empty, err := e.history.Page(ctx, e.cardID, third.Cursor, 2)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(empty.Events) != 0 || empty.More || empty.Cursor != 1 {
		t.Fatalf("the page past the end = %+v, want nothing and the cursor it was given", empty)
	}

	want := []string{"five", "four", "three", "two", "one"}
	var got []string
	cursor := int64(0)
	for {
		page, err := e.history.Page(ctx, e.cardID, cursor, 2)
		if err != nil {
			t.Fatalf("Page: %v", err)
		}
		for _, ev := range page.Events {
			got = append(got, ev.Summary)
		}
		if !page.More {
			break
		}
		cursor = page.Cursor
	}
	if len(got) != len(want) {
		t.Fatalf("paging through the whole history gave %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paging through the whole history gave %q, want %q", got, want)
		}
	}
}

func TestPageSizeFallsBackAndIsCapped(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	summaries := make([]string, history.DefaultPageSize+1)
	for i := range summaries {
		summaries[i] = "agent text"
	}
	appends(t, e.history, e.cardID, e.sessionID, summaries...)
	page, err := e.history.Page(ctx, e.cardID, 0, 0)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(page.Events) != history.DefaultPageSize || !page.More {
		t.Fatalf("a page with no size gave %d events, More = %v; want %d and More",
			len(page.Events), page.More, history.DefaultPageSize)
	}

	big, bigSession := addCard(t, e.store, "2")
	appends(t, e.history, big, bigSession, summaries...)
	for range history.MaxPageSize {
		appends(t, e.history, big, bigSession, "more")
	}
	page, err = e.history.Page(ctx, big, 0, history.MaxPageSize*10)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	if len(page.Events) != history.MaxPageSize {
		t.Fatalf("an oversized page gave %d events, want the cap of %d", len(page.Events), history.MaxPageSize)
	}
}

func TestPageRefusesAMissingCard(t *testing.T) {
	e := newEnv(t)
	if _, err := e.history.Page(context.Background(), "", 0, 10); err == nil {
		t.Fatal("Page with no card = nil error, want one")
	}
}

func TestPageByKindReturnsOneKindsEvents(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	records := []history.Record{
		{Kind: history.KindUser, Summary: "please"},
		{Kind: history.KindToolCall, Summary: "Edit", State: history.StateRunning},
		{Kind: history.KindAgent, Summary: "done"},
		{Kind: history.KindToolCall, Summary: "Run", State: history.StateOK},
	}
	if err := e.history.Append(ctx, e.cardID, e.sessionID, records); err != nil {
		t.Fatalf("Append: %v", err)
	}
	page, err := e.history.PageByKind(ctx, e.cardID, history.KindToolCall, 0, 10)
	if err != nil {
		t.Fatalf("PageByKind: %v", err)
	}
	if len(page.Events) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(page.Events))
	}
	if page.Events[0].Summary != "Run" || page.Events[0].State != history.StateOK || page.Events[0].Seq != 4 {
		t.Errorf("the newest tool call = %+v", page.Events[0])
	}
	if page.Events[1].State != history.StateRunning || page.Events[1].Seq != 2 {
		t.Errorf("the older tool call = %+v", page.Events[1])
	}
	if err := e.history.Append(ctx, e.cardID, e.sessionID, []history.Record{{Kind: history.KindAgent, Summary: "later"}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	older, err := e.history.PageByKind(ctx, e.cardID, history.KindToolCall, 4, 10)
	if err != nil {
		t.Fatalf("PageByKind: %v", err)
	}
	if len(older.Events) != 1 || older.Events[0].Summary != "Edit" {
		t.Errorf("the page before sequence 4 = %+v, want only the older tool call", older.Events)
	}
}

func TestPageByKindRefusesAnUnknownKind(t *testing.T) {
	e := newEnv(t)
	if _, err := e.history.PageByKind(context.Background(), e.cardID, "nonsense", 0, 10); err == nil {
		t.Fatal("PageByKind with an unknown kind = nil error, want one")
	}
}

func TestEventReturnsOneEventInFull(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	detail := `{"id":"t1","title":"Edit main.go"}`
	err := e.history.Append(ctx, e.cardID, e.sessionID, []history.Record{
		{Kind: history.KindToolCall, Summary: "Edit main.go", Detail: detail, State: history.StateRunning},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	page, err := e.history.Page(ctx, e.cardID, 0, 10)
	if err != nil {
		t.Fatalf("Page: %v", err)
	}
	got, err := e.history.Event(ctx, e.cardID, page.Events[0].ID)
	if err != nil {
		t.Fatalf("Event: %v", err)
	}
	if got.Detail != detail || got.Summary != "Edit main.go" || got.State != history.StateRunning {
		t.Errorf("Event = %+v, want the stored detail, summary, and state", got)
	}
	other, _ := addCard(t, e.store, "2")
	if _, err := e.history.Event(ctx, other, page.Events[0].ID); !store.IsNotFound(err) {
		t.Errorf("Event for another card's id = %v, want not found", err)
	}
	if _, err := e.history.Event(ctx, e.cardID, "no-such-event"); !store.IsNotFound(err) {
		t.Errorf("Event for an id that is not there = %v, want not found", err)
	}
}

func TestEventRefusesMissingArguments(t *testing.T) {
	e := newEnv(t)
	if _, err := e.history.Event(context.Background(), "", "e1"); err == nil {
		t.Error("Event with no card = nil error, want one")
	}
	if _, err := e.history.Event(context.Background(), "c1", ""); err == nil {
		t.Error("Event with no id = nil error, want one")
	}
}

func TestKindsAndStatesAreTheirOwnFixedLists(t *testing.T) {
	for _, kind := range history.KindValues() {
		if !kind.Valid() {
			t.Errorf("%q is in KindValues but not valid", kind)
		}
	}
	if history.Kind("nonsense").Valid() {
		t.Error("an unknown kind is valid")
	}
	for _, state := range history.StateValues() {
		if !state.Valid() {
			t.Errorf("%q is in StateValues but not valid", state)
		}
	}
	if !history.State("").Valid() {
		t.Error("the empty state is invalid, but it means the entry is not an activity item")
	}
	if history.State("nonsense").Valid() {
		t.Error("an unknown state is valid")
	}
}
