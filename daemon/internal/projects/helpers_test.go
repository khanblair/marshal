package projects_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// eventTimeout is how long a test waits for an event that should already have been published.
const eventTimeout = 5 * time.Second

// quiet is how long a test waits before it decides that no event is coming.
const quiet = 50 * time.Millisecond

// env is a service on a real store and a real bus, with a subscriber on every topic.
type env struct {
	svc     *projects.Service
	store   *store.Store
	bus     *events.Bus
	git     *gitx.Git
	dataDir string
	dbPath  string
	sub     *events.Subscription
}

// ticker is a clock that moves one second on every reading, so times are distinct and in order.
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

func newEnv(t *testing.T, opts ...projects.Option) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "marshal.db")
	st, err := store.Open(ctx, dbPath)
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
	clock := &ticker{next: time.Date(2026, time.September, 25, 9, 0, 0, 0, time.UTC)}
	all := append([]projects.Option{projects.WithClock(clock.now), projects.WithLocalClones(true)}, opts...)
	svc, err := projects.New(projects.Deps{Store: st, Bus: bus, Git: git, DataDir: dataDir}, all...)
	if err != nil {
		t.Fatalf("make the service: %v", err)
	}
	return &env{svc: svc, store: st, bus: bus, git: git, dataDir: dataDir, dbPath: dbPath, sub: bus.Subscribe(events.AllTopics())}
}

// folder makes a project from a fresh copy of a fixture repository.
func (e *env) folder(t *testing.T, fixture string, opts ...projects.CreateOption) protocol.Project {
	t.Helper()
	path := testutil.Fixture(t, fixture)
	project, err := e.svc.Create(context.Background(),
		protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path}, opts...)
	if err != nil {
		t.Fatalf("create a project from %s: %v", fixture, err)
	}
	return project
}

// card makes a card in a project.
func (e *env) card(t *testing.T, projectID, title string) protocol.Card {
	t.Helper()
	card, err := e.svc.CreateCard(context.Background(), projectID, protocol.CreateCardRequest{Title: title})
	if err != nil {
		t.Fatalf("create the card %q: %v", title, err)
	}
	return card
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

// nextType returns the next event and checks its type and topic.
func (e *env) nextType(t *testing.T, typ protocol.EventType, topic protocol.Topic) events.Event {
	t.Helper()
	ev := e.next(t)
	if ev.Type != string(typ) || ev.Topic != string(topic) {
		t.Fatalf("event = %s on %s, want %s on %s", ev.Type, ev.Topic, typ, topic)
	}
	return ev
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

// wantCode fails the test unless err is a protocol error with this code, and returns it.
func wantCode(t *testing.T, err error, code protocol.ErrorCode) *protocol.Error {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want a protocol error with code %s", err, code)
	}
	if perr.Code != code {
		t.Fatalf("error code = %s (%s), want %s", perr.Code, perr.Message, code)
	}
	return perr
}

// sameFolder reports whether two paths lead to the same folder, however they are spelled.
func sameFolder(t *testing.T, a, b string) bool {
	t.Helper()
	infoA, err := os.Stat(a)
	if err != nil {
		t.Fatalf("stat %s: %v", a, err)
	}
	infoB, err := os.Stat(b)
	if err != nil {
		t.Fatalf("stat %s: %v", b, err)
	}
	return os.SameFile(infoA, infoB)
}
