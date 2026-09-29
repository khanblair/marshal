package api_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
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
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/devices"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/preview"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/schedules"
	"github.com/khanblair/marshal/daemon/internal/security"
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
	// noDashboard leaves the dashboard service out, so the Home route is not registered.
	noDashboard bool
	// noHistory leaves the history store and the service that reads it out, so the routes that
	// page a card's chat and activity are not registered.
	noHistory bool
	// noDiff leaves the diff service out, so the routes that draw a card's Diff tab are not
	// registered.
	noDiff bool
	// noChats leaves the chats service out, so the project chat routes are not registered.
	noChats bool
	// noSearch leaves the search service out, so the search route is not registered.
	noSearch bool
	// noAccounts leaves the accounts service out, so the profile, avatar, progress, and preferences
	// routes, and the dev reset, are not registered.
	noAccounts bool
	// noAudit leaves the audit-log read service out, so the audit list, search, and export routes
	// are not registered.
	noAudit bool
	// noProviders leaves the model provider service out, so the routes that list providers and
	// store, replace, and remove their keys are not registered.
	noProviders bool
	// noCostLimits leaves the limits service out, so the routes that read and write the cost and
	// awake limits are not registered.
	noCostLimits bool
	// noConnectionTests leaves the connection-test runner out, so the route that tests a provider
	// on demand is not registered. The provider rows then carry no last test either.
	noConnectionTests bool
	// noRoles leaves the roles service out, so the routes that list Marshal's role templates and
	// add, edit, delete, reset, and override them are not registered.
	noRoles bool
	// noPullRequests leaves the pull-request service out, so the route that opens a card's branch
	// as a real pull request is not registered. This is the usual case: it is unset until a GitHub
	// token is saved.
	noPullRequests bool
	// noIntegrator leaves the merge queue out, so the route that merges a card is not registered.
	noIntegrator bool
	// noReview leaves the review service out, so the route that runs the Reviewer over a card's
	// pull request is not registered. This is the usual case: it is unset until a GitHub token is
	// saved.
	noReview bool
	// noSleepSettings leaves the settings service out, so the routes that read and write the sleep
	// settings are not registered. This is not a case the daemon ships in: the settings are rows in
	// the store, and cmd/marshald always builds the service. It is here because every service the
	// server can be given has a stack without it, so a route that needs one is proven to follow it.
	noSleepSettings bool
	// noQuality leaves the quality module out, so the routes that read a card's findings, act on
	// one, and read and write a project's smell profile are not registered. This is not a case the
	// daemon ships in: the module needs only the store, the projects service, and Git, and
	// cmd/marshald always builds it. It is here because every service the server can be given has a
	// stack without it, so a route that needs one is proven to follow it.
	noQuality bool
	// noIntegrations leaves the connections service out, so the routes that list the connections
	// Marshal is set up with, save one, remove one, and test one are not registered, and there is
	// no webhook receiver either. This is not a case the daemon ships in: cmd/marshald always
	// builds the service. It is here because every service the server can be given has a stack
	// without it, so a route that needs one is proven to follow it.
	noIntegrations bool
	// noCI leaves the CI monitor out, so the route that reads every project's CI health is not
	// registered. This is not a case the daemon ships in: the monitor needs only the store, the
	// projects service, the roles service, and Git, and cmd/marshald always builds it. It is here
	// because every service the server can be given has a stack without it, so a route that needs
	// one is proven to follow it.
	noCI bool
	// noLocalCI leaves the local-CI runner out, so the route that runs a card's workflow steps
	// locally is not registered. This is not a case the daemon ships in: the module needs only the
	// projects service and Git, and cmd/marshald always builds it. It is here because every service
	// the server can be given has a stack without it, so a route that needs one is proven to follow
	// it.
	noLocalCI bool
	// localCIRunner, when it is set, replaces the real command runner behind the local-CI route, so
	// an API test proves the wire without starting a process. Nil runs steps with the real runner.
	localCIRunner localci.Runner
	// noPreview leaves the preview module out, so the routes that read a card's preview, start and
	// stop it, take a screenshot, and serve one are not registered. This is not a case the daemon
	// ships in: the module needs only the projects service and the daemon's data folder, and
	// cmd/marshald always builds it. It is here because every service the server can be given has a
	// stack without it, so a route that needs one is proven to follow it.
	noPreview bool
	// noSchedules leaves the schedules service out, so the routes that list the scheduled jobs and
	// briefs and add, edit, and delete one are not registered. This is not a case the daemon ships
	// in: cmd/marshald always builds the service. It is here because every service the server can be
	// given has a stack without it, so a route that needs one is proven to follow it.
	noSchedules bool
	// noDevices leaves the paired-devices service out, so the list, code, and revoke routes are not
	// registered and no device can be paired at all. This is not a case the daemon ships in:
	// cmd/marshald always builds the service. It is here for the same reason as every other
	// no-service option: a route that needs one is proven to follow it.
	noDevices bool
	// tailnet, when it is set, is the node the server is given, so a test can drive the tailnet
	// listener, the tailnet origin rule, and the Funnel handler with no Tailscale anywhere. Nil,
	// which is the default, is a daemon reachable on this machine only.
	tailnet api.TailnetNode
	// funnel asks the server to expose /hooks/* to the public internet through Funnel. It does
	// nothing without a node above it, exactly as in a real daemon.
	funnel bool
	// noMemory leaves the memory module out, so the routes that read and write a card's note are not
	// registered and there is no search either (the search answers the session and note kinds through
	// the same module). This is not a case the daemon ships in: the module needs only the store, the
	// projects service, and the data folder, and cmd/marshald always builds it. It is here for the
	// same reason the other no-service options are: every service the server can be given has a
	// stack without it, so a route that needs one is proven to follow it.
	noMemory bool
	// previewOptions, when it is set, replaces the seams the preview module starts a dev server and
	// takes a screenshot through, so an API test proves the wire without starting a process or
	// launching a browser. Nil leaves every seam at the real default, which is what the daemon uses.
	previewOptions preview.Options
	// webhookSecret, when it is set, saves a GitHub App connection with that webhook secret before
	// the stack serves, so a test can sign a delivery with a secret the daemon actually holds. It
	// is the same thing the Settings screen does through PUT /v1/integrations/github.
	webhookSecret string
	// integrationsTester, when it is set, replaces the real GitHub connection test, so an API test
	// that presses Test never dials GitHub.
	integrationsTester func(ctx context.Context, info integrations.Info) (protocol.TestResult, error)
	// trelloBaseURL, when it is set, points the Trello connection at a fake server, so an API test
	// that saves a Trello connection and presses Test never dials Trello.
	trelloBaseURL string
	// providerFailure is what every provider call answers with, standing in for a provider that
	// refused the key or could not be reached. Nil is a provider that works.
	providerFailure error
	resumeMode      session.ResumeMode
	stubSpeed       string
	// terminals are the agents that run a card's CLI in a terminal (the terminal view). Nil leaves
	// every card without a terminal view.
	terminals *agents.Registry
	// secondAgent registers a second agent kind (gemini) with the same stub-agent factory as the
	// default card agent, so a card can be handed off from one agent to another over the wire.
	secondAgent bool
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
func withoutDashboard() stackOption              { return func(c *stackConfig) { c.noDashboard = true } }
func withoutHistory() stackOption                { return func(c *stackConfig) { c.noHistory = true } }
func withoutDiff() stackOption                   { return func(c *stackConfig) { c.noDiff = true } }
func withoutChats() stackOption                  { return func(c *stackConfig) { c.noChats = true } }
func withoutSearch() stackOption                 { return func(c *stackConfig) { c.noSearch = true } }
func withoutAccounts() stackOption               { return func(c *stackConfig) { c.noAccounts = true } }
func withoutAudit() stackOption                  { return func(c *stackConfig) { c.noAudit = true } }
func withoutProviders() stackOption              { return func(c *stackConfig) { c.noProviders = true } }
func withoutCostLimits() stackOption             { return func(c *stackConfig) { c.noCostLimits = true } }
func withoutConnectionTests() stackOption        { return func(c *stackConfig) { c.noConnectionTests = true } }
func withoutRoles() stackOption                  { return func(c *stackConfig) { c.noRoles = true } }
func withoutPullRequests() stackOption           { return func(c *stackConfig) { c.noPullRequests = true } }
func withoutIntegrator() stackOption             { return func(c *stackConfig) { c.noIntegrator = true } }
func withoutReview() stackOption                 { return func(c *stackConfig) { c.noReview = true } }
func withoutSleepSettings() stackOption          { return func(c *stackConfig) { c.noSleepSettings = true } }
func withoutQuality() stackOption                { return func(c *stackConfig) { c.noQuality = true } }
func withoutIntegrations() stackOption           { return func(c *stackConfig) { c.noIntegrations = true } }
func withoutCI() stackOption                     { return func(c *stackConfig) { c.noCI = true } }
func withoutLocalCI() stackOption                { return func(c *stackConfig) { c.noLocalCI = true } }
func withLocalCIRunner(r localci.Runner) stackOption {
	return func(c *stackConfig) { c.localCIRunner = r }
}
func withoutPreview() stackOption { return func(c *stackConfig) { c.noPreview = true } }

// withoutSchedules leaves the schedules service out.
func withoutSchedules() stackOption { return func(c *stackConfig) { c.noSchedules = true } }

// withoutDevices leaves the paired-devices service out.
func withoutDevices() stackOption { return func(c *stackConfig) { c.noDevices = true } }

// withTailnet hands the server a node on a tailnet.
func withTailnet(node api.TailnetNode) stackOption {
	return func(c *stackConfig) { c.tailnet = node }
}

// withFunnel asks for /hooks/* to be exposed publicly, which needs withTailnet to mean anything.
func withFunnel() stackOption { return func(c *stackConfig) { c.funnel = true } }

// withoutMemory leaves the memory module out.
func withoutMemory() stackOption { return func(c *stackConfig) { c.noMemory = true } }
func withPreviewOptions(o preview.Options) stackOption {
	return func(c *stackConfig) { c.previewOptions = o }
}
func withTerminals(reg *agents.Registry) stackOption {
	return func(c *stackConfig) { c.terminals = reg }
}
func withSecondAgent() stackOption { return func(c *stackConfig) { c.secondAgent = true } }
func withResumeMode(m session.ResumeMode) stackOption {
	return func(c *stackConfig) { c.resumeMode = m }
}
func withWebhooks(secret string) stackOption {
	return func(c *stackConfig) { c.webhookSecret = secret }
}
func withIntegrationsTester(fn func(ctx context.Context, info integrations.Info) (protocol.TestResult, error)) stackOption {
	return func(c *stackConfig) { c.integrationsTester = fn }
}
func withTrelloBaseURL(url string) stackOption {
	return func(c *stackConfig) { c.trelloBaseURL = url }
}

// stack is a daemon in a test: a real store, bus, Git, projects service, session manager over the
// real stub agent through the real ACP adapter, and the API server listening on a free port. It
// is built the way cmd/marshald builds the real one.
type stack struct {
	t        *testing.T
	cfg      stackConfig
	dir      string // the temp folder of the whole stack
	dataDir  string // the data folder that worktrees and session logs go under
	vault    string // the vault folder the memory module writes under, inside dataDir
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

	wires   []*wire // the event stream clients that are open, closed when the server restarts
	clockMu sync.Mutex
	skew    time.Duration // how far the server's clock is ahead of the real one
	mgr     *session.Manager
	hist    *history.Store
	// mem is the memory module, which owns the vault under the stack's data folder. It is nil when
	// the stack was built without it (withoutMemory, or withoutProjects, which it needs).
	mem *memory.Service
	// webhookSink records the deliveries that reach the daemon's webhook receiver, so a test can
	// prove a verified event arrived and that an unverified one did not.
	webhookSink *webhookRecorder
	// keychain is the in-memory keychain the connections service and the providers service both
	// keep their secrets in, so a test never reads or writes this machine's real keychain and can
	// prove a secret the daemon stores never comes back out.
	keychain     security.Keychain
	integrations *integrations.Service
	// schedules is the scheduler the stack's own routes run through, so a test can save a schedule
	// and read it back the way the daemon does. It is nil when the stack was built without it.
	schedules *schedules.Service
	// devices is the paired-devices service, so a test can pair a device with a code it issued
	// itself. It is nil when the stack was built without it.
	devices *devices.Service
	// audit is the one audit recorder the stack's modules write rows through, built the way
	// cmd/marshald builds the daemon's, so a test can prove a row was written by reading it back
	// through GET /v1/audit.
	audit    *audit.Recorder
	closeMgr func()
	// server is the API server the stack serves with, so a test can reach the handlers that are
	// not on a listener - the Funnel handler, for one.
	server     *api.Server
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
	st.vault = filepath.Join(st.dataDir, "vault")
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
	if cfg.secondAgent {
		// A second kind behind the same program, so "hand this card off to another agent" has
		// somewhere to go without a second real agent being installed.
		if err := st.reg.Register(protocol.AgentKindGemini, factory); err != nil {
			t.Fatalf("register the second agent: %v", err)
		}
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
	// One in-memory keychain serves every service that keeps a secret, so a test never reads or
	// writes this machine's real keychain.
	st.keychain = security.NewMemoryKeychain()
	deps := projects.Deps{Store: st.store, Bus: st.bus, Git: st.git, DataDir: st.dataDir}
	// The session states come from the stored rows, as they do in cmd/marshald, so the projects
	// service built once here keeps reading them while restart replaces the session manager.
	states, err := session.NewStoredStates(st.store)
	if err != nil {
		t.Fatalf("make the session states: %v", err)
	}
	if st.proj, err = projects.New(deps, projects.WithLogger(st.log), projects.WithSessionStates(states)); err != nil {
		t.Fatalf("make the projects service: %v", err)
	}
	// The one audit recorder, on the stack's own clock so a row's time moves with st.advance.
	if st.audit, err = audit.New(st.store, st.now, nil, st.log); err != nil {
		t.Fatalf("make the audit recorder: %v", err)
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
