package cardhistory_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/cardhistory"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// testTime is the clock every test works from, so a stored time is exact.
var testTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// env is the service on a real database, with one card and its session row, which the stored
// events' own foreign keys need.
type env struct {
	store   *store.Store
	history *history.Store
	service *cardhistory.Service
	cardID  string
	card2   string
	session string
}

// newEnv opens a store, writes a card with its session, and builds the service over them.
func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cardID, other, sessionID := addCards(t, st)
	hist, err := history.New(st, history.WithClock(func() time.Time { return testTime }))
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	svc, err := cardhistory.New(cardhistory.Deps{Store: st, History: hist},
		cardhistory.WithClock(func() time.Time { return testTime }))
	if err != nil {
		t.Fatalf("make the service: %v", err)
	}
	return &env{store: st, history: hist, service: svc, cardID: cardID, card2: other, session: sessionID}
}

// addCards writes a project, a board, two cards, and a session row, and returns the two card ids
// and the session id.
func addCards(t *testing.T, st *store.Store) (first, second, session string) {
	t.Helper()
	ctx := context.Background()
	now := testTime.UnixMilli()
	first, second, session = "card-1", "card-2", "session-1"
	err := st.Write(ctx, func(q *db.Queries) error {
		if err := q.CreateProject(ctx, db.CreateProjectParams{
			ID: "api", Name: "api", RepoPath: "/tmp/api", DefaultBranch: "main",
			PackagesJSON: "[]", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		if err := q.CreateBoard(ctx, db.CreateBoardParams{ID: "board-1", ProjectID: "api", ColumnsJSON: "[]"}); err != nil {
			return err
		}
		for i, id := range []string{first, second} {
			if err := q.CreateCard(ctx, db.CreateCardParams{
				ID: id, ProjectID: "api", Number: int64(i + 1), BoardID: "board-1", Title: "A card",
				State: "working", AgentKind: "claude", PermissionMode: "auto-edits", CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		return q.CreateCardSession(ctx, db.CreateCardSessionParams{
			ID: session, CardID: first, AgentKind: "claude", State: "awake",
			LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("write the cards and the session: %v", err)
	}
	return first, second, session
}

// append stores records on the first card.
func (e *env) append(t *testing.T, records ...history.Record) {
	t.Helper()
	if err := e.history.Append(context.Background(), e.cardID, e.session, records); err != nil {
		t.Fatalf("append: %v", err)
	}
}

// chatRecords is a run of stored events of the kinds a card's history holds.
func chatRecords(count int) []history.Record {
	records := make([]history.Record, 0, count)
	for i := range count {
		records = append(records, history.Record{Kind: history.KindAgent, Summary: fmt.Sprintf("chunk %d", i)})
	}
	return records
}

// toolDetail is the payload a stored tool call carries.
func toolDetail(id, toolKind, command, content string) string {
	return fmt.Sprintf(`{"id":%q,"toolKind":%q,"command":%q,"content":%q}`, id, toolKind, command, content)
}

func TestNewRefusesMissingParts(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hist, err := history.New(st)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	for name, deps := range map[string]cardhistory.Deps{
		"no store":   {History: hist},
		"no history": {Store: st},
	} {
		if _, err := cardhistory.New(deps); err == nil {
			t.Errorf("New with %s = nil error, want one", name)
		}
	}
}

func TestOptionsIgnoreNilValues(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hist, err := history.New(st)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	if _, err := cardhistory.New(cardhistory.Deps{Store: st, History: hist},
		cardhistory.WithClock(nil), cardhistory.WithLogger(nil)); err != nil {
		t.Fatalf("New with nil options: %v", err)
	}
}

func TestMessagesPagesNewestFirst(t *testing.T) {
	e := newEnv(t)
	e.append(t,
		history.Record{Kind: history.KindUser, Summary: "please"},
		history.Record{Kind: history.KindAgent, Summary: "one"},
		history.Record{Kind: history.KindAgent, Summary: "two"},
		history.Record{Kind: history.KindToolCall, Summary: "Edited conn.go", State: history.StateOK,
			Detail: toolDetail("c1", "edit", "", "done")},
	)
	ctx := context.Background()

	first, err := e.service.Messages(ctx, e.cardID, 0, 2)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(first.Items) != 2 || !first.More {
		t.Fatalf("first page = %d items, More %v; want 2 and More", len(first.Items), first.More)
	}
	if first.Items[0].Text != "" || first.Items[0].Kind != protocol.ChatMessageKindTool {
		t.Errorf("the newest message = %+v, want the tool call", first.Items[0])
	}
	if first.Items[0].Tool == nil || first.Items[0].Tool.Title != "Edited conn.go" {
		t.Errorf("the newest message has no tool call: %+v", first.Items[0])
	}
	if first.Items[1].Text != "two" || first.Items[1].Kind != protocol.ChatMessageKindAgent {
		t.Errorf("the second message = %+v", first.Items[1])
	}
	if first.Cursor != 3 {
		t.Errorf("cursor = %d, want the oldest sequence on the page, 3", first.Cursor)
	}

	second, err := e.service.Messages(ctx, e.cardID, first.Cursor, 2)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(second.Items) != 2 || second.More {
		t.Fatalf("second page = %d items, More %v; want 2 and no more", len(second.Items), second.More)
	}
	if second.Items[0].Text != "one" || second.Items[1].Text != "please" {
		t.Errorf("second page = %+v", second.Items)
	}

	empty, err := e.service.Messages(ctx, e.cardID, second.Cursor, 2)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(empty.Items) != 0 || empty.More || empty.Items == nil {
		t.Errorf("the page past the end = %+v, want an empty list and no more", empty)
	}
}

func TestMessagesOfACardWithNoHistoryIsAnEmptyPage(t *testing.T) {
	e := newEnv(t)
	page, err := e.service.Messages(context.Background(), e.cardID, 0, 50)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || page.More {
		t.Errorf("page = %+v, want an empty list that is not nil", page)
	}
}

func TestMessagesOfAnUnknownCardIsNotFound(t *testing.T) {
	e := newEnv(t)
	_, err := e.service.Messages(context.Background(), "no-such-card", 0, 50)
	assertCode(t, err, protocol.ErrorCodeNotFound)
}

func TestMessagesUsesTheDefaultSizeAndTheCap(t *testing.T) {
	e := newEnv(t)
	e.append(t, chatRecords(cardhistory.MaxPageSize+10)...)
	ctx := context.Background()

	big, err := e.service.Messages(ctx, e.cardID, 0, 10_000)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(big.Items) != cardhistory.MaxPageSize {
		t.Errorf("an oversized page gave %d items, want the cap of %d", len(big.Items), cardhistory.MaxPageSize)
	}
	standard, err := e.service.Messages(ctx, e.cardID, 0, 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(standard.Items) != cardhistory.DefaultPageSize {
		t.Errorf("a page with no size gave %d items, want %d", len(standard.Items), cardhistory.DefaultPageSize)
	}
}

func TestActivitySkipsTheChat(t *testing.T) {
	e := newEnv(t)
	e.append(t,
		history.Record{Kind: history.KindUser, Summary: "please"},
		history.Record{Kind: history.KindToolCall, Summary: "Edited conn.go", State: history.StateOK,
			Detail: `{"id":"c1","toolKind":"edit","path":"internal/upstream/conn.go"}`},
		history.Record{Kind: history.KindAgent, Summary: "there"},
		history.Record{Kind: history.KindToolCall, Summary: "Bash: go test ./...", State: history.StateRunning,
			Detail: toolDetail("c2", "execute", "go test ./...", "")},
		history.Record{Kind: history.KindApproval, Summary: "Asked to run rm -rf build", State: history.StateWaiting,
			Detail: `{"requestId":"r1","command":"rm -rf build"}`},
		history.Record{Kind: history.KindSystem, Summary: "The agent exited.", State: history.StateFailed},
	)

	page, err := e.service.Activity(context.Background(), e.cardID, "", 0, 50)
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	want := []struct {
		kind protocol.ActivityKind
		seq  int64
	}{
		{protocol.ActivityKindTool, 6},
		{protocol.ActivityKindApproval, 5},
		{protocol.ActivityKindTest, 4},
		{protocol.ActivityKindFile, 2},
	}
	if len(page.Items) != len(want) {
		t.Fatalf("the activity list = %+v, want %d entries", page.Items, len(want))
	}
	for i, expect := range want {
		if page.Items[i].Kind != expect.kind || page.Items[i].Seq != expect.seq {
			t.Errorf("entry %d = %+v, want kind %q at sequence %d", i, page.Items[i], expect.kind, expect.seq)
		}
	}
	if page.Items[3].State != protocol.ActivityStateOK {
		t.Errorf("the file entry's state = %q, want ok", page.Items[3].State)
	}
}

func TestActivityFiltersByKind(t *testing.T) {
	e := newEnv(t)
	e.append(t,
		history.Record{Kind: history.KindToolCall, Summary: "Edited conn.go", State: history.StateOK,
			Detail: `{"id":"c1","toolKind":"edit","path":"conn.go"}`},
		history.Record{Kind: history.KindToolCall, Summary: "Bash: go build ./...", State: history.StateOK,
			Detail: toolDetail("c2", "execute", "go build ./...", "")},
		history.Record{Kind: history.KindToolCall, Summary: "Bash: go test ./...", State: history.StateOK,
			Detail: toolDetail("c3", "execute", "go test ./...", "ok")},
		history.Record{Kind: history.KindApproval, Summary: "Asked to run rm -rf build", State: history.StateWaiting},
	)
	ctx := context.Background()
	for kind, want := range map[protocol.ActivityKind]string{
		protocol.ActivityKindFile:     "Edited conn.go",
		protocol.ActivityKindCommand:  "Bash: go build ./...",
		protocol.ActivityKindTest:     "Bash: go test ./...",
		protocol.ActivityKindApproval: "Asked to run rm -rf build",
	} {
		page, err := e.service.Activity(ctx, e.cardID, kind, 0, 50)
		if err != nil {
			t.Fatalf("Activity(%s): %v", kind, err)
		}
		if len(page.Items) != 1 || page.Items[0].Text != want {
			t.Errorf("Activity(%s) = %+v, want only %q", kind, page.Items, want)
		}
	}
	// No tool entry is a plain tool when it neither ran nor touched a file.
	if _, err := e.service.Activity(ctx, e.cardID, protocol.ActivityKindTool, 0, 50); err != nil {
		t.Fatalf("Activity(tool): %v", err)
	}
}

func TestActivityOfAnUnknownCardIsNotFound(t *testing.T) {
	e := newEnv(t)
	_, err := e.service.Activity(context.Background(), "no-such-card", "", 0, 50)
	assertCode(t, err, protocol.ErrorCodeNotFound)
}

func TestActivityRefusesAnUnknownKind(t *testing.T) {
	e := newEnv(t)
	if _, err := e.service.Activity(context.Background(), e.cardID, "nonsense", 0, 50); err == nil {
		t.Fatal("Activity with an unknown kind = nil error, want one")
	}
}

// A page reads a bounded number of stored events, so a card whose history is a long conversation
// still answers when it is asked for its activity: the first pages come back empty with a cursor
// that has moved, and the page where the conversation ends holds the activity.
func TestActivityReadsPastALongRunOfChat(t *testing.T) {
	e := newEnv(t)
	e.append(t, history.Record{Kind: history.KindToolCall, Summary: "Bash: go build ./...", State: history.StateOK,
		Detail: toolDetail("c1", "execute", "go build ./...", "")})
	e.append(t, chatRecords(500)...)
	ctx := context.Background()

	cursor, pages := int64(0), 0
	for {
		page, err := e.service.Activity(ctx, e.cardID, "", cursor, 1)
		if err != nil {
			t.Fatalf("Activity: %v", err)
		}
		pages++
		if pages > 20 {
			t.Fatal("walking the activity list never reached the end")
		}
		if len(page.Items) > 0 {
			if pages < 2 {
				t.Error("the first page read the whole history instead of stopping at its bound")
			}
			if page.Items[0].Seq != 1 {
				t.Fatalf("the entry = %+v, want the one tool call at sequence 1", page.Items[0])
			}
			return
		}
		if !page.More {
			t.Fatal("the walk ended without the tool call")
		}
		if page.Cursor == cursor || page.Cursor == 0 {
			t.Fatalf("the cursor did not move: %d", page.Cursor)
		}
		cursor = page.Cursor
	}
}

func TestMessageDetailReturnsTheToolInFull(t *testing.T) {
	e := newEnv(t)
	e.append(t, history.Record{Kind: history.KindToolCall, Summary: "Edited conn.go", State: history.StateOK,
		Detail: `{"id":"c1","title":"Edited conn.go","toolKind":"edit","path":"conn.go",` +
			`"content":"Replaced Dial.","diffs":[{"path":"conn.go","oldText":"a","newText":"b"}],"truncated":true}`})
	page, err := e.service.Messages(context.Background(), e.cardID, 0, 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	detail, err := e.service.MessageDetail(context.Background(), e.cardID, page.Items[0].ID)
	if err != nil {
		t.Fatalf("MessageDetail: %v", err)
	}
	if detail.Tool == nil {
		t.Fatal("the tool detail is missing")
	}
	if detail.Tool.Content != "Replaced Dial." || len(detail.Tool.Diffs) != 1 || !detail.Tool.Truncated {
		t.Errorf("tool detail = %+v", *detail.Tool)
	}
	if detail.Message.ID != page.Items[0].ID || detail.Message.Kind != protocol.ChatMessageKindTool {
		t.Errorf("the message in the detail = %+v", detail.Message)
	}
	if !detail.ServerTime.Time().Equal(testTime) {
		t.Errorf("serverTime = %s, want %s", detail.ServerTime.Time(), testTime)
	}
}

func TestMessageDetailOfAMessageWithNoToolDetail(t *testing.T) {
	e := newEnv(t)
	e.append(t, history.Record{Kind: history.KindUser, Summary: "please"})
	page, err := e.service.Messages(context.Background(), e.cardID, 0, 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	detail, err := e.service.MessageDetail(context.Background(), e.cardID, page.Items[0].ID)
	if err != nil {
		t.Fatalf("MessageDetail: %v", err)
	}
	if detail.Tool != nil {
		t.Errorf("a user message carries a tool detail: %+v", detail.Tool)
	}
	if detail.Message.Text != "please" {
		t.Errorf("message = %+v", detail.Message)
	}
}

func TestMessageDetailIsNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.append(t, history.Record{Kind: history.KindAgent, Summary: "hello"})
	page, err := e.service.Messages(ctx, e.cardID, 0, 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	for name, tc := range map[string]struct{ cardID, messageID string }{
		"an id that is not there":  {e.cardID, "01M3C107JB041061050R3GG28Z"},
		"no id at all":             {e.cardID, ""},
		"another card's message":   {e.card2, page.Items[0].ID},
		"a card that is not there": {"no-such-card", page.Items[0].ID},
	} {
		_, err := e.service.MessageDetail(ctx, tc.cardID, tc.messageID)
		if name == "a card that is not there" {
			assertCode(t, err, protocol.ErrorCodeNotFound)
			continue
		}
		assertCode(t, err, protocol.ErrorCodeNotFound)
	}
}

// A very long id is cut before it is echoed back, so a hostile address is not repeated in full.
func TestMessageDetailCutsAVeryLongId(t *testing.T) {
	e := newEnv(t)
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'a'
	}
	_, err := e.service.MessageDetail(context.Background(), e.cardID, string(long))
	assertCode(t, err, protocol.ErrorCodeNotFound)
}

// assertCode fails unless the error is an answer with the code.
func assertCode(t *testing.T, err error, code protocol.ErrorCode) {
	t.Helper()
	var answer *protocol.Error
	if !errors.As(err, &answer) {
		t.Fatalf("err = %v, want an answer with the code %s", err, code)
	}
	if answer.Code != code {
		t.Fatalf("code = %s, want %s (%s)", answer.Code, code, answer.Message)
	}
}
