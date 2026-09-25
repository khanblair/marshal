package session_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/acp"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// stubEnv is like env (helpers_test.go), but its registry starts a real stub-agent process
// through the real agents/acp adapter, for end-to-end confidence that the manager works with a
// real Agent implementation and not only the fake one every other test in this package uses. A
// fresh *acp.Adapter is made on every registry.New call, matching production (see the report's
// Ruling on not caching an Agent per kind): the stub agent's own --state-dir, shared by every
// Manager an env builds, is what lets a new process continue an old session, exactly like a real
// agent's resume would.
type stubEnv struct {
	mgr     *session.Manager
	store   *store.Store
	bus     *events.Bus
	proj    *projects.Service
	git     *gitx.Git
	reg     *agents.Registry
	dataDir string
	sub     *events.Subscription
}

func newStubEnv(t *testing.T, mutate ...func(*session.Config)) *stubEnv {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	bus, err := events.New(events.WithEpoch("test-epoch"))
	if err != nil {
		t.Fatalf("make the bus: %v", err)
	}
	t.Cleanup(bus.Close)
	git := testutil.Git()
	dataDir := filepath.Join(dir, "data")
	proj, err := projects.New(projects.Deps{Store: st, Bus: bus, Git: git, DataDir: dataDir}, projects.WithLocalClones(true))
	if err != nil {
		t.Fatalf("make the projects service: %v", err)
	}
	stateDir := filepath.Join(dataDir, "dev-agent-state")
	stubPath := testutil.StubAgent(t)
	reg := agents.NewRegistry()
	factory := func() (agents.Agent, error) {
		return acp.New(acp.Config{
			Path: stubPath, Args: []string{"--state-dir", stateDir, "--speed", "0"},
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}
	if err := reg.Register(protocol.AgentKindClaude, factory); err != nil {
		t.Fatalf("register the stub agent: %v", err)
	}
	e := &stubEnv{store: st, bus: bus, proj: proj, git: git, reg: reg, dataDir: dataDir, sub: bus.Subscribe(events.AllTopics())}
	e.mgr = e.reopen(t, mutate...)
	return e
}

func (e *stubEnv) reopen(t *testing.T, mutate ...func(*session.Config)) *session.Manager {
	t.Helper()
	cfg := session.Config{DataDir: e.dataDir}
	for _, m := range mutate {
		m(&cfg)
	}
	mgr, err := session.NewManager(e.store, e.bus, e.proj, e.reg, e.git, cfg)
	if err != nil {
		t.Fatalf("make the manager: %v", err)
	}
	return mgr
}

func (e *stubEnv) project(t *testing.T, fixture string) protocol.Project {
	t.Helper()
	path := testutil.Fixture(t, fixture)
	p, err := e.proj.Create(context.Background(), protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path})
	if err != nil {
		t.Fatalf("create a project from %s: %v", fixture, err)
	}
	return p
}

func (e *stubEnv) card(t *testing.T, projectID, title string) protocol.Card {
	t.Helper()
	c, err := e.proj.CreateCard(context.Background(), projectID, protocol.CreateCardRequest{Title: title})
	if err != nil {
		t.Fatalf("create the card %q: %v", title, err)
	}
	return c
}

func (e *stubEnv) untilType(t *testing.T, typ protocol.EventType) events.Event {
	t.Helper()
	timeout := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				t.Fatalf("the subscription closed waiting for %s", typ)
			}
			if ev.Type == string(typ) {
				return ev
			}
		case <-timeout:
			t.Fatalf("no %s event arrived", typ)
			return events.Event{}
		}
	}
}

// collectUntil reads every event, of any type, keeping all of them, until one matches stop, and
// returns the whole batch including that one. Unlike untilType this never silently discards an
// event of a type it was not looking for, which matters here: the real stub agent runs fast
// enough (--speed 0) that several session.output events can already be queued by the time a test
// gets around to waiting for the next session.state_changed.
func (e *stubEnv) collectUntil(t *testing.T, stop func(events.Event) bool) []events.Event {
	t.Helper()
	var got []events.Event
	timeout := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				t.Fatalf("the subscription closed after %d events", len(got))
			}
			got = append(got, ev)
			if stop(ev) {
				return got
			}
		case <-timeout:
			t.Fatalf("no matching event arrived; got %d events first", len(got))
			return got
		}
	}
}

// nthAwake matches the nth time cardID's session reaches session.state_changed(awake), counting
// from the start of whatever batch it is used on.
func nthAwake(cardID string, n int) func(events.Event) bool {
	count := 0
	return func(ev events.Event) bool {
		if ev.Type != string(protocol.EventTypeSessionStateChanged) {
			return false
		}
		data, ok := ev.Data.(protocol.SessionStateChangedEventData)
		if !ok || data.CardID != cardID || data.State != protocol.SessionStateAwake {
			return false
		}
		count++
		return count >= n
	}
}

// messageTexts pulls the text of every message-kind session.output event for cardID out of batch,
// in order.
func messageTexts(cardID string, batch []events.Event) []string {
	var out []string
	for _, ev := range batch {
		if ev.Type != string(protocol.EventTypeSessionOutput) {
			continue
		}
		if data, ok := ev.Data.(protocol.SessionOutputEventData); ok && data.CardID == cardID && data.Kind == "message" {
			out = append(out, data.Text)
		}
	}
	return out
}

// TestStubAgentStartSendAndStop covers, against a real stub-agent process through the real ACP
// adapter: Start makes a worktree and moves the card to working, sending a message while a turn
// is running queues it and delivers it once the turn ends, and Stop ends the process and marks
// the session stopped.
func TestStubAgentStartSendAndStop(t *testing.T) {
	e := newStubEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")

	updated, err := e.mgr.Start(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if updated.State != protocol.CardStateWorking {
		t.Fatalf("card state = %s, want working", updated.State)
	}
	path, _, err := e.proj.Worktree(context.Background(), card.ID)
	if err != nil || path == "" {
		t.Fatalf("Worktree = %q, %v, want a path", path, err)
	}

	if err := e.mgr.Send(context.Background(), card.ID, "please fix the token bug"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Nothing here waits for the first Send's turn to reach the agent before the second: the
	// manager's own busy bookkeeping is set the moment the first Send is accepted, synchronously,
	// so this always exercises the queue regardless of how fast the real agent answers.
	if err := e.mgr.Send(context.Background(), card.ID, "and one more thing"); err != nil {
		t.Fatalf("Send while busy: %v", err)
	}
	// Start's own "awake" is the 1st; the first turn ending is the 2nd; the queued message's turn
	// (delivered automatically) ending is the 3rd. Collecting through it keeps every event in
	// between instead of discarding whichever ones a narrower wait would skip past.
	batch := e.collectUntil(t, nthAwake(card.ID, 3))
	texts := messageTexts(card.ID, batch)
	if len(texts) == 0 || !strings.Contains(texts[0], "Turn 1") {
		t.Fatalf("message texts = %v, want the first to start the conversation at turn 1", texts)
	}
	if turn2 := strings.Join(texts, " | "); !strings.Contains(turn2, "Turn 2") {
		t.Errorf("message texts = %v, want the queued message to run as turn 2", texts)
	}

	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	stopped := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := stopped.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateStopped {
		t.Fatalf("state after Stop = %+v, want stopped", stopped.Data)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateStopped) {
		t.Errorf("session row state = %s, want stopped", row.State)
	}
}

// TestStubAgentResumesAfterClose proves that closing the session manager and building a fresh one
// against the same store and data folder, then RestoreAll, continues the same session: the stub
// agent's own "Turn N, I remember M earlier turns" line keeps counting across the restart.
func TestStubAgentResumesAfterClose(t *testing.T) {
	e := newStubEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "first question"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Start's own "awake" is the 1st; the turn ending, which leaves the session idle before
	// closing rather than mid-turn, is the 2nd.
	firstBatch := e.collectUntil(t, nthAwake(card.ID, 2))
	firstTexts := messageTexts(card.ID, firstBatch)
	if len(firstTexts) == 0 || !strings.Contains(firstTexts[0], "Turn 1") {
		t.Fatalf("first turn texts = %v, want the first to start the conversation at turn 1", firstTexts)
	}

	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fresh := e.reopen(t)
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if err := fresh.Send(context.Background(), card.ID, "second question"); err != nil {
		t.Fatalf("Send after RestoreAll: %v", err)
	}
	// RestoreAll's own resume already published one "awake" before Send even started a turn; the
	// new turn's own "awake" is the next one.
	secondBatch := e.collectUntil(t, nthAwake(card.ID, 2))
	secondTexts := messageTexts(card.ID, secondBatch)
	if len(secondTexts) == 0 || !strings.Contains(secondTexts[0], "Turn 2") ||
		!strings.Contains(secondTexts[0], "I remember 1 earlier turns") {
		t.Errorf("texts after resume = %v, want the first to continue at turn 2 and remember turn 1", secondTexts)
	}
}

// TestStubAgentCannotResumeMovesToNeeds proves that a card whose session the agent cannot resume
// (here, because its recorded agent session id names a session the stub agent has never heard of)
// moves to needs you, matching docs/architecture.md 5.3.
func TestStubAgentCannotResumeMovesToNeeds(t *testing.T) {
	e := newStubEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()
	row, err := e.store.Queries().GetSessionByCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if err := e.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(ctx, db.UpdateSessionRuntimeParams{
			State: row.State, AgentSessionID: "no-such-session-on-disk",
			LastActiveAt: row.LastActiveAt, UpdatedAt: row.UpdatedAt, ID: row.ID,
		})
		return err
	}); err != nil {
		t.Fatalf("corrupt the agent session id: %v", err)
	}

	fresh := e.reopen(t)
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(ctx); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	updated, err := e.proj.Card(ctx, card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if updated.State != protocol.CardStateNeeds {
		t.Errorf("card state = %s, want %s", updated.State, protocol.CardStateNeeds)
	}
	after, err := e.store.Queries().GetSessionByCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if after.State != string(protocol.SessionStateStopped) {
		t.Errorf("session row state = %s, want stopped", after.State)
	}
}
