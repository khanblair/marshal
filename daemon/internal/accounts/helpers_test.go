package accounts_test

import (
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// eventTimeout is how long a test waits for an event that should already have been published.
const eventTimeout = 5 * time.Second

// quiet is how long a test waits before it decides that no event is coming.
const quiet = 50 * time.Millisecond

// ticker is a clock that moves one second on every reading, so the times of two changes differ and
// are in order.
type ticker struct {
	mu   sync.Mutex
	next time.Time
}

func (c *ticker) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next = c.next.Add(time.Second)
	return c.next
}

// env is the service on a real store, a real bus, and a real projects service, with a subscriber on
// every topic and the owner as the user.
type env struct {
	svc      *accounts.Service
	projects *projects.Service
	store    *store.Store
	bus      *events.Bus
	sub      *events.Subscription
	dbPath   string
	dataDir  string
	clock    *ticker
	userID   string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{
		dbPath: filepath.Join(dir, "marshal.db"), dataDir: filepath.Join(dir, "data"),
		clock: &ticker{next: time.Date(2026, time.September, 30, 9, 0, 0, 0, time.UTC)},
	}
	bus, err := events.New(events.WithEpoch("test-epoch"))
	if err != nil {
		t.Fatalf("make the bus: %v", err)
	}
	t.Cleanup(bus.Close)
	e.bus = bus
	e.sub = bus.Subscribe(events.AllTopics())
	e.open(t)
	owner, err := e.store.EnsureOwner(context.Background(), db.CreateUserParams{
		ID: "01M3C107JB041061050R3GG2U1", Name: "Owner",
		CreatedAt: store.Millis(e.clock.now()), UpdatedAt: store.Millis(e.clock.now()),
	})
	if err != nil {
		t.Fatalf("make the owner: %v", err)
	}
	e.userID = owner.ID
	return e
}

// open opens the store and builds the services over it. restart calls it again over the same file
// and data folder, as a daemon starting again does.
func (e *env) open(t *testing.T) {
	t.Helper()
	st, err := store.Open(context.Background(), e.dbPath)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e.store = st
	e.projects, err = projects.New(projects.Deps{Store: st, Bus: e.bus, Git: testutil.Git(), DataDir: e.dataDir},
		projects.WithClock(e.clock.now))
	if err != nil {
		t.Fatalf("make the projects service: %v", err)
	}
	e.svc, err = accounts.New(accounts.Deps{Store: st, Bus: e.bus, Projects: e.projects, DataDir: e.dataDir},
		accounts.WithClock(e.clock.now))
	if err != nil {
		t.Fatalf("make the accounts service: %v", err)
	}
}

// restart closes the store and opens it again with new services over it: whatever the next reads
// find was written to the file.
func (e *env) restart(t *testing.T) {
	t.Helper()
	if err := e.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	e.open(t)
}

// project adds a project from a fresh copy of a fixture repository.
func (e *env) project(t *testing.T, fixture string) protocol.Project {
	t.Helper()
	project, err := e.projects.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: testutil.Fixture(t, fixture),
	})
	if err != nil {
		t.Fatalf("create a project from %s: %v", fixture, err)
	}
	return project
}

// savedView saves a view in a project.
func (e *env) savedView(t *testing.T, projectID, name string) protocol.SavedView {
	t.Helper()
	view, _, err := e.projects.CreateSavedView(context.Background(), projectID, protocol.CreateSavedViewRequest{Name: name})
	if err != nil {
		t.Fatalf("save the view %q: %v", name, err)
	}
	return view
}

// secondUser adds another person to the database, as Phase 9 will, and returns their id.
func (e *env) secondUser(t *testing.T, name string) string {
	t.Helper()
	id, err := protocol.NewID(e.clock.now(), rand.Reader)
	if err != nil {
		t.Fatalf("make an id: %v", err)
	}
	now := store.Millis(e.clock.now())
	err = e.store.Write(context.Background(), func(q *db.Queries) error {
		_, err := q.CreateUser(context.Background(), db.CreateUserParams{
			ID: id, Name: name, CreatedAt: now, UpdatedAt: now,
		})
		return err
	})
	if err != nil {
		t.Fatalf("add a user: %v", err)
	}
	return id
}

// next returns the next event, or fails the test.
func (e *env) next(t *testing.T) events.Event {
	t.Helper()
	select {
	case ev, ok := <-e.sub.C():
		if !ok {
			t.Fatal("the subscription closed, want an event")
		}
		return ev
	case <-time.After(eventTimeout):
		t.Fatal("no event arrived")
		return events.Event{}
	}
}

// nextMe returns the next me.updated event and its data, and checks that it is critical and on the
// me topic.
func (e *env) nextMe(t *testing.T) protocol.MeUpdatedEventData {
	t.Helper()
	ev := e.next(t)
	if ev.Type != string(protocol.EventTypeMeUpdated) || ev.Topic != string(protocol.MeTopic) || !ev.Critical {
		t.Fatalf("event = %s on %s (critical %v), want a critical me.updated on the me topic", ev.Type, ev.Topic, ev.Critical)
	}
	data, ok := ev.Data.(protocol.MeUpdatedEventData)
	if !ok {
		t.Fatalf("event data is %T, want protocol.MeUpdatedEventData", ev.Data)
	}
	return data
}

// noEvent fails the test if an event arrives soon.
func (e *env) noEvent(t *testing.T) {
	t.Helper()
	select {
	case ev, ok := <-e.sub.C():
		t.Fatalf("unexpected event %+v (open %v)", ev, ok)
	case <-time.After(quiet):
	}
}

// drainEvents throws away the events that are already waiting.
func (e *env) drainEvents() {
	for {
		select {
		case <-e.sub.C():
		case <-time.After(quiet):
			return
		}
	}
}

// wantCode fails the test unless err is a protocol error with this code and this message.
func wantCode(t *testing.T, err error, code protocol.ErrorCode, message string) {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want a protocol error with code %s", err, code)
	}
	if perr.Code != code || perr.Message != message {
		t.Fatalf("error = %s %q, want %s %q", perr.Code, perr.Message, code, message)
	}
}
