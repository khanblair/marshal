package session_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// TestMain runs the suite, then removes the built stub agent binary that the real, end-to-end
// tests share (testutil.StubAgent builds it once for the whole process), and only then checks for
// leaked goroutines: a goleak.VerifyTestMain call cannot be followed by more cleanup, since it
// exits the process itself.
func TestMain(m *testing.M) {
	code := m.Run()
	testutil.CleanStubAgent()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// eventTimeout is how long a test waits for an event that should already be on its way.
const eventTimeout = 5 * time.Second

// env is a session Manager on a real store, bus, and projects service, with one fake agent
// behind the default card agent kind (claude), shared by every Manager env.reopen builds, so a
// test can simulate a restart that keeps the database and the data folder.
type env struct {
	mgr      *session.Manager
	store    *store.Store
	bus      *events.Bus
	proj     *projects.Service
	git      *gitx.Git
	registry *agents.Registry
	agent    *fakeAgent
	dataDir  string
	sub      *events.Subscription
}

// newEnv builds a fresh store, bus, projects service, and session Manager, with a subscriber on
// every topic.
func newEnv(t *testing.T, mutate ...func(*session.Config)) *env {
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
	agent := newFakeAgent(agents.Capabilities{Resume: true, StructuredEvents: true})
	registry := agents.NewRegistry()
	if err := registry.Register(protocol.AgentKindClaude, func() (agents.Agent, error) { return agent, nil }); err != nil {
		t.Fatalf("register the fake agent: %v", err)
	}
	e := &env{
		store: st, bus: bus, proj: proj, git: git, registry: registry, agent: agent, dataDir: dataDir,
		sub: bus.Subscribe(events.AllTopics()),
	}
	e.mgr = e.reopen(t, mutate...)
	return e
}

// reopen builds a new Manager over the same store, bus, git, projects service, and agent
// registry, simulating a restart that keeps the database and the data folder but starts a fresh
// process. It does not replace e.mgr; callers that want a real restart reassign it themselves.
func (e *env) reopen(t *testing.T, mutate ...func(*session.Config)) *session.Manager {
	t.Helper()
	cfg := session.Config{DataDir: e.dataDir}
	for _, m := range mutate {
		m(&cfg)
	}
	mgr, err := session.NewManager(e.store, e.bus, e.proj, e.registry, e.git, cfg)
	if err != nil {
		t.Fatalf("make the manager: %v", err)
	}
	return mgr
}

// project makes a project from a fresh copy of a fixture repository.
func (e *env) project(t *testing.T, fixture string) protocol.Project {
	t.Helper()
	path := testutil.Fixture(t, fixture)
	p, err := e.proj.Create(context.Background(), protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path})
	if err != nil {
		t.Fatalf("create a project from %s: %v", fixture, err)
	}
	return p
}

// card makes a card in a project.
func (e *env) card(t *testing.T, projectID, title string) protocol.Card {
	t.Helper()
	c, err := e.proj.CreateCard(context.Background(), projectID, protocol.CreateCardRequest{Title: title})
	if err != nil {
		t.Fatalf("create the card %q: %v", title, err)
	}
	return c
}

// untilType reads events until one of the given type arrives, and returns it.
func (e *env) untilType(t *testing.T, typ protocol.EventType) events.Event {
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
