package session_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// recorder is a HistoryRecorder that keeps what it is handed, for tests that check what the
// manager gives the store it was wired with, and that can be made to fail.
type recorder struct {
	mu    sync.Mutex
	calls []recorderCall
	err   error
}

// recorderCall is one Append or AppendChat the manager made.
type recorderCall struct {
	cardID    string
	chatID    string
	sessionID string
	records   []history.Record
}

func (r *recorder) Append(_ context.Context, cardID, sessionID string, records []history.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recorderCall{cardID: cardID, sessionID: sessionID, records: records})
	return r.err
}

func (r *recorder) AppendChat(_ context.Context, chatID, sessionID string, records []history.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recorderCall{chatID: chatID, sessionID: sessionID, records: records})
	return r.err
}

// snapshot copies the calls made so far.
func (r *recorder) snapshot() []recorderCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recorderCall(nil), r.calls...)
}

// historyOf reads a card's stored history through the history module's own store, the way the API
// layer will.
func (e *env) historyOf(t *testing.T, cardID string) []history.Event {
	t.Helper()
	h, err := history.New(e.store)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	page, err := h.Page(context.Background(), cardID, 0, 100)
	if err != nil {
		t.Fatalf("read the history of card %s: %v", cardID, err)
	}
	return page.Events
}

// liveFakeSession returns the session row id and the fake agent session behind a card's live
// session.
func (e *env) liveFakeSession(t *testing.T, cardID string) (string, *fakeSession) {
	t.Helper()
	row, err := e.store.Queries().GetSessionByCard(context.Background(), cardID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	fs, err := e.agent.find(row.AgentSessionID)
	if err != nil {
		t.Fatalf("find the fake session: %v", err)
	}
	return row.ID, fs
}

func TestASessionsOutputIsStoredAsCardHistory(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	sessionID, fs := e.liveFakeSession(t, card.ID)

	fs.sink.Emit(agents.MessageChunk{Text: "working on it"})
	e.untilType(t, protocol.EventTypeSessionOutput)
	fs.sink.Emit(agents.ToolCall{
		ID: "t1", Title: "Edit main.go", Kind: "edit", Status: agents.StatusInProgress, Path: "main.go",
	})
	e.untilType(t, protocol.EventTypeSessionToolCall)

	events := e.historyOf(t, card.ID)
	if len(events) != 2 {
		t.Fatalf("stored %d events, want the message and the tool call: %+v", len(events), events)
	}
	tool, message := events[0], events[1]
	if tool.Kind != history.KindToolCall || tool.Seq != 2 || tool.Summary != "Edit main.go" {
		t.Errorf("the newest event = %+v, want the tool call at sequence 2", tool)
	}
	if tool.State != history.StateRunning || !strings.Contains(tool.Detail, `"toolKind":"edit"`) {
		t.Errorf("the tool call = %+v, want a running state and its tool kind in the detail", tool)
	}
	if message.Kind != history.KindAgent || message.Seq != 1 || message.Summary != "working on it" {
		t.Errorf("the oldest event = %+v, want the message at sequence 1", message)
	}
	if tool.CardID != card.ID || tool.SessionID != sessionID || tool.At.IsZero() {
		t.Errorf("the tool call = %+v, want the card, the session, and a time", tool)
	}
}

func TestAMessageSentToASessionIsStoredAsHistory(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "please fix the tests"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// The message is stored before Send returns, so the card's history has it whether or not the
	// agent's answer has arrived yet.
	sent := oneKind(t, e.historyOf(t, card.ID), history.KindUser)
	if sent.Seq != 1 || sent.Summary != "please fix the tests" {
		t.Errorf("the sent message = %+v, want the person's words at sequence 1", sent)
	}

	e.untilType(t, protocol.EventTypeSessionOutput)
	answered := oneKind(t, e.historyOf(t, card.ID), history.KindAgent)
	if !strings.Contains(answered.Summary, "please fix the tests") {
		t.Errorf("the stored answer = %q, want the agent's own words", answered.Summary)
	}
}

func TestAQueuedMessageJoinsTheHistoryWhenItIsDelivered(t *testing.T) {
	e := newEnv(t)
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold); _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilType(t, protocol.EventTypeSessionStateChanged) // the "awake" from Start

	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send(first): %v", err)
	}
	// A turn is running (held by e.agent.hold), so this one waits in the queue and is not history
	// yet.
	if err := e.mgr.Send(context.Background(), card.ID, "second"); err != nil {
		t.Fatalf("Send(second): %v", err)
	}
	if stored := e.historyOf(t, card.ID); len(stored) != 1 {
		t.Fatalf("stored %d events while the second message waits, want only the first: %+v", len(stored), stored)
	}

	e.agent.hold <- struct{}{} // let the first turn finish, which delivers the queued message
	waitForHistory(t, e, card.ID, 3)

	sent := kindEvents(t, e.historyOf(t, card.ID), history.KindUser)
	if len(sent) != 2 || sent[0].Summary != "second" || sent[1].Summary != "first" {
		t.Errorf("the stored messages = %+v, want the queued one newest and the first one oldest", sent)
	}
}

func TestAManagerUsesTheRecorderItWasWiredWith(t *testing.T) {
	rec := &recorder{}
	e := newEnv(t, func(cfg *session.Config) { cfg.History = rec })
	// Held so the turn the Send starts adds nothing: this test is about what the manager hands the
	// recorder for the message itself.
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold); _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	calls := rec.snapshot()
	if len(calls) != 1 {
		t.Fatalf("the recorder got %d calls, want the one message: %+v", len(calls), calls)
	}
	call := calls[0]
	if call.cardID != card.ID || call.sessionID == "" {
		t.Errorf("the recorder was given card %q and session %q, want the card and its session", call.cardID, call.sessionID)
	}
	if len(call.records) != 1 || call.records[0].Kind != history.KindUser || call.records[0].Summary != "hello" {
		t.Errorf("the recorder was given %+v, want the person's message", call.records)
	}
}

func TestARecorderThatFailsDoesNotStopTheSession(t *testing.T) {
	rec := &recorder{err: errors.New("the disk is full")}
	e := newEnv(t, func(cfg *session.Config) { cfg.History = rec })
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send with a recorder that fails: %v", err)
	}
	// The turn the message started still publishes, so a history write that failed costs the
	// history, not the chat. Two calls are the person's message and the agent's answer; the turn
	// ending is not history.
	ev := e.untilType(t, protocol.EventTypeSessionOutput)
	if data, _ := ev.Data.(protocol.SessionOutputEventData); !strings.Contains(data.Text, "hello") {
		t.Errorf("output = %+v, want the agent's words", ev.Data)
	}
	if got := len(rec.snapshot()); got != 2 {
		t.Errorf("the recorder got %d calls, want both the message and the output", got)
	}
}

// oneKind returns the only stored event of one kind, and fails the test unless there is exactly
// one.
func oneKind(t *testing.T, events []history.Event, kind history.Kind) history.Event {
	t.Helper()
	found := kindEvents(t, events, kind)
	if len(found) != 1 {
		t.Fatalf("got %d %s events, want 1: %+v", len(found), kind, events)
	}
	return found[0]
}

// kindEvents returns the stored events of one kind, newest first.
func kindEvents(t *testing.T, events []history.Event, kind history.Kind) []history.Event {
	t.Helper()
	var found []history.Event
	for _, ev := range events {
		if ev.Kind == kind {
			found = append(found, ev)
		}
	}
	return found
}

// waitForHistory waits until a card has at least count stored events, which is how a test waits
// for the pump goroutine without reading the database in a loop forever.
func waitForHistory(t *testing.T, e *env, cardID string, count int) {
	t.Helper()
	deadline := time.Now().Add(eventTimeout)
	for time.Now().Before(deadline) {
		if len(e.historyOf(t, cardID)) >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("card %s stored fewer than %d events before the deadline", cardID, count)
}
