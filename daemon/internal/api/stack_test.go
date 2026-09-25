package api_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/acp"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// stackConfig is what a test changes about the stack it builds. The default is a dev daemon with
// every service, the real stub agent behind the default card agent, and the stub catalog.
type stackConfig struct {
	normal     bool // a normal daemon: the token comes from the owner token file and the database
	limits     api.Limits
	agent      agents.Factory // replaces the stub agent behind claude
	catalog    catalog.Source
	noProjects bool
	noSessions bool
	noCatalog  bool
	resumeMode session.ResumeMode
	stubSpeed  string
}

type stackOption func(*stackConfig)

func normalDaemon() stackOption           { return func(c *stackConfig) { c.normal = true } }
func withLimits(l api.Limits) stackOption { return func(c *stackConfig) { c.limits = l } }
func withAgent(f agents.Factory) stackOption {
	return func(c *stackConfig) { c.agent = f }
}
func withCatalog(src catalog.Source) stackOption { return func(c *stackConfig) { c.catalog = src } }
func withoutProjects() stackOption               { return func(c *stackConfig) { c.noProjects = true } }
func withoutSessions() stackOption               { return func(c *stackConfig) { c.noSessions = true } }
func withoutCatalog() stackOption                { return func(c *stackConfig) { c.noCatalog = true } }
func withResumeMode(m session.ResumeMode) stackOption {
	return func(c *stackConfig) { c.resumeMode = m }
}

// stack is a daemon in a test: a real store, bus, Git, projects service, session manager over the
// real stub agent through the real ACP adapter, and the API server listening on a free port. It
// is built the way cmd/marshald builds the real one.
type stack struct {
	t        *testing.T
	cfg      stackConfig
	dir      string // the temp folder of the whole stack
	dataDir  string // the data folder that worktrees and session logs go under
	stateDir string // where the stub agent keeps its sessions, so a new process can resume them
	store    *store.Store
	bus      *events.Bus
	git      *gitx.Git
	proj     *projects.Service
	reg      *agents.Registry
	log      *slog.Logger
	logs     *lockedBuffer
	settings config.Settings
	dev      *api.DevAccess
	token    string

	wires      []*wire // the event stream clients that are open, closed when the server restarts
	clockMu    sync.Mutex
	skew       time.Duration // how far the server's clock is ahead of the real one
	mgr        *session.Manager
	closeMgr   func()
	base       string // http://127.0.0.1:<port>
	stopServer func()
	client     *http.Client
}

// lockedBuffer is a log destination that several goroutines write to and a test reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newStack builds and starts a daemon. Its cleanups run in the order the daemon closes in: the
// server first, then the session manager, then the bus, and the store last.
func newStack(t *testing.T, opts ...stackOption) *stack {
	t.Helper()
	var cfg stackConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	st := &stack{t: t, cfg: cfg, dir: t.TempDir(), logs: &lockedBuffer{}}
	st.dataDir = filepath.Join(st.dir, "data")
	st.stateDir = filepath.Join(st.dataDir, "dev-agent-state")
	st.log = slog.New(slog.NewTextHandler(st.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	st.settings = config.Settings{Mode: platform.ModeDev, Port: 0, Agent: config.AgentStub}
	if cfg.normal {
		st.settings.Mode = platform.ModeNormal
		st.settings.Agent = config.AgentReal
	}
	st.openCore()
	st.makeAccounts()
	st.reg = agents.NewRegistry()
	factory := cfg.agent
	if factory == nil {
		factory = st.stubFactory()
	}
	if err := st.reg.Register(protocol.AgentKindClaude, factory); err != nil {
		t.Fatalf("register the agent: %v", err)
	}
	st.startModules()
	return st
}

// openCore opens what everything else stands on: the store, the bus, Git, and the projects
// service.
func (st *stack) openCore() {
	t := st.t
	t.Helper()
	ctx := context.Background()
	var err error
	if st.store, err = store.Open(ctx, filepath.Join(st.dir, "marshal.db")); err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.store.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	if st.bus, err = events.New(events.WithEpoch("test-epoch")); err != nil {
		t.Fatalf("make the bus: %v", err)
	}
	t.Cleanup(st.bus.Close)
	st.git = testutil.Git()
	deps := projects.Deps{Store: st.store, Bus: st.bus, Git: st.git, DataDir: st.dataDir}
	if st.proj, err = projects.New(deps, projects.WithLogger(st.log)); err != nil {
		t.Fatalf("make the projects service: %v", err)
	}
}

// makeAccounts makes the owner and the token, the way the daemon does on its first start.
func (st *stack) makeAccounts() {
	t := st.t
	t.Helper()
	ctx := context.Background()
	dev, err := api.EnsureAccounts(ctx, st.store, api.AccountsConfig{
		DataDir: st.dir, Dev: !st.cfg.normal, Now: time.Now, Log: st.log,
	})
	if err != nil {
		t.Fatalf("make the accounts: %v", err)
	}
	st.dev = dev
	if dev != nil {
		st.token = dev.Token
		return
	}
	token, err := platform.ReadTokenFile(filepath.Join(st.dir, platform.OwnerTokenFile))
	if err != nil {
		t.Fatalf("read the owner token: %v", err)
	}
	st.token = token
}

// stubFactory makes agents that are the real stub-agent program behind the real ACP adapter, as
// internal/session's own tests do. A new process is made for every session, and the stub's own
// state folder is shared, so a later process can continue an earlier session.
func (st *stack) stubFactory() agents.Factory {
	path := testutil.StubAgent(st.t)
	speed := st.cfg.stubSpeed
	if speed == "" {
		speed = "0"
	}
	return func() (agents.Agent, error) {
		return acp.New(acp.Config{
			Path: path, Args: []string{"--state-dir", st.stateDir, "--speed", speed},
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}
}

// startModules builds the session manager and the server over what is already open, and starts
// serving. restart calls it again over the same store and data folder.
func (st *stack) startModules() {
	t := st.t
	t.Helper()
	deps := api.Deps{Store: st.store, Bus: st.bus, Dev: st.dev, Limits: st.cfg.limits}
	if !st.cfg.noProjects {
		deps.Projects = st.proj
	}
	if !st.cfg.noSessions {
		mgr, err := session.NewManager(st.store, st.bus, st.proj, st.reg, st.git, session.Config{
			DataDir: st.dataDir, Logger: st.log, ResumeMode: st.cfg.resumeMode,
		})
		if err != nil {
			t.Fatalf("make the session manager: %v", err)
		}
		st.mgr = mgr
		st.closeMgr = sync.OnceFunc(func() {
			if err := mgr.Close(); err != nil {
				t.Errorf("close the session manager: %v", err)
			}
		})
		t.Cleanup(st.closeMgr)
		deps.Sessions = mgr
	}
	if !st.cfg.noCatalog {
		deps.Catalog = st.cfg.catalog
		if deps.Catalog == nil {
			deps.Catalog = catalog.NewStub()
		}
	}
	server := api.New(st.settings, st.log, st.now, deps)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	st.stopServer = sync.OnceFunc(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
		st.client.CloseIdleConnections()
	})
	t.Cleanup(st.stopServer)
	st.base = "http://" + listener.Addr().String()
	st.client = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 30 * time.Second}
}

// restart closes the session manager and the server, as a daemon stopping does, and builds new
// ones over the same store, bus, projects service, and data folder, as a daemon starting again
// does. It does not restore sessions: the test calls RestoreAll, as the daemon does after it is
// up.
func (st *stack) restart() {
	st.t.Helper()
	// A client that is not reading cannot answer the server's close, so the tests close their own
	// side first, as a person closing the app would.
	for _, w := range st.wires {
		_ = w.conn.CloseNow()
	}
	st.wires = nil
	st.stopServer()
	if st.closeMgr != nil {
		st.closeMgr()
	}
	st.startModules()
}

// worktreeFolders lists the folders under the project's worktrees folder.
func (st *stack) worktreeFolders(projectID string) []string {
	st.t.Helper()
	entries, err := os.ReadDir(projects.WorktreesDir(st.dataDir, projectID))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		st.t.Fatalf("read the worktrees folder: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// logText is everything the stack has logged so far.
func (st *stack) logText() string { return st.logs.String() }

// mustNotLog fails the test if the log holds the text. It is how the tests check that a token or
// a message never reaches a log.
func (st *stack) mustNotLog(secret string) {
	st.t.Helper()
	if secret != "" && strings.Contains(st.logText(), secret) {
		st.t.Errorf("the log holds %q, which must never be logged", secret)
	}
}

// now is the server's clock: the real one, plus whatever the test has moved it forward.
func (st *stack) now() time.Time {
	st.clockMu.Lock()
	defer st.clockMu.Unlock()
	return time.Now().Add(st.skew)
}

// advance moves the server's clock forward.
func (st *stack) advance(d time.Duration) {
	st.clockMu.Lock()
	defer st.clockMu.Unlock()
	st.skew += d
}

// openStore opens a store of its own in a temp folder, for a test that needs one that nothing else
// has written to.
func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open a store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	return st
}
