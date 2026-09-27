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

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/acp"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/auditlog"
	"github.com/khanblair/marshal/daemon/internal/cardhistory"
	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/ci"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/connectiontest"
	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/diff"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/preview"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/pullrequest"
	"github.com/khanblair/marshal/daemon/internal/quality"
	"github.com/khanblair/marshal/daemon/internal/review"
	"github.com/khanblair/marshal/daemon/internal/roles"
	"github.com/khanblair/marshal/daemon/internal/search"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/settings"
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

// withoutMemory leaves the memory module out.
func withoutMemory() stackOption { return func(c *stackConfig) { c.noMemory = true } }
func withPreviewOptions(o preview.Options) stackOption {
	return func(c *stackConfig) { c.previewOptions = o }
}
func withProviderFailure(err error) stackOption {
	return func(c *stackConfig) { c.providerFailure = err }
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
	// audit is the one audit recorder the stack's modules write rows through, built the way
	// cmd/marshald builds the daemon's, so a test can prove a row was written by reading it back
	// through GET /v1/audit.
	audit      *audit.Recorder
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

// startModules builds the session manager and the server over what is already open, and starts
// serving. restart calls it again over the same store and data folder.
func (st *stack) startModules() {
	t := st.t
	t.Helper()
	deps := api.Deps{Store: st.store, Bus: st.bus, Dev: st.dev, Limits: st.cfg.limits}
	if !st.cfg.noIntegrations {
		// The connections Marshal is set up with apart from model providers, built the way
		// cmd/marshald builds it. Its receiver is the stack's webhook route, and its sink is the
		// recorder, so a test can prove a verified delivery reached the daemon. The secret is read
		// from the keychain on every delivery, so the connection is saved below and not configured
		// here.
		if st.webhookSink == nil {
			st.webhookSink = &webhookRecorder{}
		}
		opts := integrations.Options{Logger: st.log, Now: st.now, Tester: st.cfg.integrationsTester}
		svc, err := integrations.New(st.store, st.keychain, opts)
		if err != nil {
			t.Fatalf("make the integrations service: %v", err)
		}
		svc.SetMonitor(st.webhookSink)
		st.integrations = svc
		deps.Integrations = svc
		deps.Webhooks = svc.Receiver()
		if st.cfg.webhookSecret != "" {
			st.connectGitHub(t, st.cfg.webhookSecret)
		}
	}
	if !st.cfg.noHistory {
		// One history store is shared by the session manager that writes it and the service that
		// reads it back, the way cmd/marshald shares one.
		if st.hist == nil {
			hist, err := history.New(st.store, history.WithClock(st.now))
			if err != nil {
				t.Fatalf("make the history store: %v", err)
			}
			st.hist = hist
		}
		histSvc, err := cardhistory.New(cardhistory.Deps{Store: st.store, History: st.hist})
		if err != nil {
			t.Fatalf("make the card history service: %v", err)
		}
		deps.History = histSvc
	}
	if !st.cfg.noDashboard {
		home, err := dashboard.New(dashboard.Deps{Store: st.store}, dashboard.WithLogger(st.log))
		if err != nil {
			t.Fatalf("make the dashboard service: %v", err)
		}
		deps.Dashboard = home
	}
	if !st.cfg.noProjects {
		deps.Projects = st.proj
	}
	// The memory module (B7.2, B7.4, B7.5): the one note each card keeps in the vault, the files a
	// card has claimed, and the search over the notes and the past sessions. Its vault is a folder
	// under the stack's own data folder, which is a temporary directory - a test never writes into
	// the real `<data>/vault`.
	if !st.cfg.noMemory && !st.cfg.noProjects {
		mem, err := memory.New(memory.Deps{Store: st.store, Cards: st.proj, Root: st.vault},
			memory.WithClock(st.now))
		if err != nil {
			t.Fatalf("make the memory module: %v", err)
		}
		deps.Memory = mem
		st.mem = mem
	}
	if !st.cfg.noDiff && !st.cfg.noProjects {
		// The diff module reads a card's worktree through the projects module, so a stack without
		// projects has no diff service either.
		cardDiff, err := diff.New(diff.Deps{Projects: st.proj, Git: st.git}, diff.WithLogger(st.log))
		if err != nil {
			t.Fatalf("make the card diff service: %v", err)
		}
		deps.Diff = cardDiff
	}
	if !st.cfg.noSessions {
		// One audit recorder serves the session manager and the CI monitor, as cmd/marshald wires
		// it. It is stateless, so one is shared rather than each module making its own.
		cfg := session.Config{
			DataDir: st.dataDir, Logger: st.log, ResumeMode: st.cfg.resumeMode, History: st.hist,
			Terminals: st.cfg.terminals, Audit: st.audit,
			// The session manager reads the stack's own clock, as the history store, the accounts
			// service, and the server already do, so st.advance moves everyone's idea of "now" at
			// once. That is what lets a test drive the idle timer of Phase 5 without waiting it out.
			Now: st.now,
		}
		// A stack with no history has no plan store either: plans are messages in the history, so
		// the manager is left to make its own from the store it already has rather than handed a
		// nil one through the interface.
		if st.hist != nil {
			cfg.Plans = st.hist
		}
		mgr, err := session.NewManager(st.store, st.bus, st.proj, st.reg, st.git, cfg)
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
	if !st.cfg.noChats {
		// The chats service is given the session manager when there is one, as cmd/marshald gives
		// it: the manager starts a chat's agent on its first message, and puts it to sleep or ends
		// it when the chat is archived or deleted.
		chatDeps := chats.Deps{Store: st.store, Bus: st.bus}
		if deps.Sessions != nil {
			chatDeps.Sessions = deps.Sessions
		}
		projectChats, err := chats.New(chatDeps, chats.WithLogger(st.log))
		if err != nil {
			t.Fatalf("make the chats service: %v", err)
		}
		deps.Chats = projectChats
		if !st.cfg.noSearch && !st.cfg.noProjects && st.mem != nil {
			// Search reads through the projects, chats, and memory services, so a stack without any
			// of them has no search either.
			finder, err := search.New(search.Deps{
				Projects: st.proj, Chats: projectChats, Sessions: st.mem, Notes: st.mem,
			})
			if err != nil {
				t.Fatalf("make the search service: %v", err)
			}
			deps.Search = finder
		}
	}
	if !st.cfg.noAccounts {
		you, err := accounts.New(accounts.Deps{Store: st.store, Bus: st.bus, Projects: st.proj, DataDir: st.dataDir},
			accounts.WithLogger(st.log), accounts.WithClock(st.now))
		if err != nil {
			t.Fatalf("make the accounts service: %v", err)
		}
		deps.Accounts = you
	}
	if !st.cfg.noAudit {
		auditRead, err := auditlog.New(auditlog.Deps{Store: st.store}, auditlog.WithClock(st.now))
		if err != nil {
			t.Fatalf("make the audit log service: %v", err)
		}
		deps.Auditlog = auditRead
	}
	if !st.cfg.noCatalog {
		deps.Catalog = st.cfg.catalog
		if deps.Catalog == nil {
			deps.Catalog = catalog.NewStub()
		}
	}
	if !st.cfg.noProviders {
		// The keychain is an in-memory one: a test never reads or writes this machine's real
		// keychain. The client factory is a fake, and that is what keeps a provider call out of
		// these tests: saving a key tests it, and so does pressing Test, so a real factory would
		// dial the provider named in the request. The usage recorder is wired as the daemon wires
		// it, over a fixed reader so a row's id is reproducible.
		svc, err := providers.New(st.keychain, providers.Options{
			Logger: st.log, Recorder: providers.NewStoreRecorder(st.store, bytes.NewReader(make([]byte, 64))),
			Now: st.now, Build: fakeProviderBuild(st.cfg.providerFailure),
		})
		if err != nil {
			t.Fatalf("make the provider service: %v", err)
		}
		deps.Providers = svc
	}
	if !st.cfg.noCostLimits {
		deps.CostLimits = providers.NewLimits(st.store)
	}
	if !st.cfg.noConnectionTests {
		deps.ConnectionTests = connectiontest.New(st.store,
			connectiontest.Options{Logger: st.log, Now: st.now})
	}
	if !st.cfg.noRoles {
		// The roles service is built and seeded the way cmd/marshald does it, so the API tests see
		// Marshal's eight starter roles without asking for them.
		roleSvc, err := roles.New(roles.Deps{Store: st.store}, roles.WithLogger(st.log), roles.WithClock(st.now))
		if err != nil {
			t.Fatalf("make the roles service: %v", err)
		}
		if err := roleSvc.EnsureStarters(context.Background()); err != nil {
			t.Fatalf("add the starter roles: %v", err)
		}
		deps.Roles = roleSvc
		// The roles service is the manager's reader of a card's role ceilings, as cmd/marshald
		// wires it, so an API test can drive the harness by naming a role on a card.
		if st.mgr != nil {
			st.mgr.SetRoleLimits(roleSvc)
		}
	}
	if !st.cfg.noCI && !st.cfg.noProjects && !st.cfg.noRoles {
		// The CI monitor is built the way cmd/marshald builds it: over the store, the projects
		// service, the roles service whose ceilings it counts rounds against, and real Git, over a
		// forge client with a token. No forge call is made in the auth test, so the address the
		// client points at never matters here.
		client, err := github.NewTokenClient("test-token", github.WithBaseURL("http://127.0.0.1:0"))
		if err != nil {
			t.Fatalf("make the GitHub client: %v", err)
		}
		ciDeps := ci.Deps{
			Store: st.store, Cards: st.proj, Projects: st.proj, Git: st.git,
			Roles: deps.Roles, Bus: st.bus, Audit: st.audit,
			Options: ci.Options{Logger: st.log, Now: st.now, Forge: client},
		}
		// The worker is set only when there is a session manager: a nil *session.Manager in that
		// interface would be non-nil to the monitor and a call on it would panic.
		if st.mgr != nil {
			ciDeps.Worker = st.mgr
		}
		ciSvc, err := ci.New(ciDeps)
		if err != nil {
			t.Fatalf("make the CI monitor: %v", err)
		}
		deps.CI = ciSvc
		// A verified delivery reaches the CI monitor through the connections service, as
		// cmd/marshald wires it. The recorder keeps its own copy for the webhook tests.
		if st.webhookSink != nil {
			st.webhookSink.setInner(ciSvc)
		}
	}
	if !st.cfg.noLocalCI && !st.cfg.noProjects {
		// The local-CI runner is built the way cmd/marshald builds it: over the projects service,
		// which reads the card's worktree. Its command runner is left at its default, so a test
		// that does not want a child process gives a fake through withLocalCIRunner.
		svc, err := localci.New(localci.Deps{
			Projects: st.proj, Log: st.log, Now: st.now, Runner: st.cfg.localCIRunner,
		})
		if err != nil {
			t.Fatalf("make the local CI service: %v", err)
		}
		deps.LocalCI = svc
	}
	if !st.cfg.noPreview && !st.cfg.noProjects {
		// The preview module is built the way cmd/marshald builds it: over the projects service,
		// which reads the card's worktree and the project's dev command, and the daemon's data
		// folder, where the shots and the browser profiles live. Every seam is left at its real
		// default, so a test that does not want a dev server or a browser gives fakes through
		// withPreviewOptions.
		svc, err := preview.New(preview.Deps{
			Projects: st.proj, Bus: st.bus, DataDir: st.dataDir, Log: st.log, Now: st.now,
			Options: st.cfg.previewOptions,
		})
		if err != nil {
			t.Fatalf("make the preview module: %v", err)
		}
		deps.Preview = svc
		// The dev servers stop once the server has stopped answering, and before the bus and the
		// store close, as the daemon stops them when it returns.
		t.Cleanup(svc.Close)
	}
	if !st.cfg.noPullRequests && !st.cfg.noProjects {
		// The pull-request service is built the way cmd/marshald builds it, over a client with a
		// token. No call is made in the auth test, so the address the client points at never
		// matters here.
		client, err := github.NewTokenClient("test-token", github.WithBaseURL("http://127.0.0.1:0"))
		if err != nil {
			t.Fatalf("make the GitHub client: %v", err)
		}
		prSvc, err := pullrequest.New(pullrequest.Deps{
			Client: client, Cards: st.proj, Projects: st.proj, Git: st.git, Log: st.log,
		})
		if err != nil {
			t.Fatalf("make the pull-request service: %v", err)
		}
		deps.PullRequests = prSvc
	}
	if !st.cfg.noReview && !st.cfg.noProjects && !st.cfg.noRoles {
		// The review service is built the way cmd/marshald builds it: the ChecksReviewer over a
		// client with a token, reading the Reviewer role the stack just seeded. No forge call is
		// made in the auth test, so the address the client points at never matters here.
		client, err := github.NewTokenClient("test-token", github.WithBaseURL("http://127.0.0.1:0"))
		if err != nil {
			t.Fatalf("make the GitHub client: %v", err)
		}
		checker, err := review.NewChecksReviewer(client)
		if err != nil {
			t.Fatalf("make the checks reviewer: %v", err)
		}
		reviewSvc, err := review.New(review.Deps{
			Cards: st.proj, Projects: st.proj, Roles: deps.Roles, Git: st.git,
			Forge: client, Reviews: checker, Log: st.log,
		})
		if err != nil {
			t.Fatalf("make the review service: %v", err)
		}
		deps.Review = reviewSvc
	}
	if !st.cfg.noIntegrator && !st.cfg.noProjects {
		// The merge queue is built the way cmd/marshald builds it, with no test runner.
		queue, err := integrator.New(integrator.Deps{
			Cards: st.proj, Projects: st.proj, Git: st.git, DataDir: st.dataDir, Log: st.log,
		})
		if err != nil {
			t.Fatalf("make the merge queue: %v", err)
		}
		deps.Integrator = queue
	}
	if !st.cfg.noSleepSettings {
		// The sleep settings service is built the way cmd/marshald builds it: over the store, and
		// handed to the session manager as its reader of the idle, warning, and keep-awake times, so
		// the idle timer and the settings screen read the same rows.
		sleepSvc, err := settings.New(st.store)
		if err != nil {
			t.Fatalf("make the settings service: %v", err)
		}
		deps.SleepSettings = sleepSvc
		if st.mgr != nil {
			st.mgr.SetSleepSettings(sleepSvc)
		}
	}
	if !st.cfg.noQuality && !st.cfg.noProjects {
		// The quality module is built the way cmd/marshald builds it: over the store, the projects
		// service, real Git, and the session manager as its way of reaching a card's agent. Its
		// linter builder is left at its default, so a test that drives a real check without
		// wanting a child process passes one of its own through the module.
		//
		// The worker is set only when there is a session manager: a nil *session.Manager in that
		// interface would be non-nil to the module and a call on it would panic, so a stack built
		// without sessions is the one stack that really has nobody to ask and answers the
		// "no agent" refusal instead.
		qualityDeps := quality.Deps{
			Store: st.store, Cards: st.proj, Projects: st.proj, Git: st.git,
			Bus: st.bus, Log: st.log,
		}
		if st.mgr != nil {
			qualityDeps.Worker = st.mgr
		}
		qualitySvc, err := quality.New(qualityDeps)
		if err != nil {
			t.Fatalf("make the quality module: %v", err)
		}
		deps.Quality = qualitySvc
		// The module is the projects service's review gate, as cmd/marshald wires it, so an API test
		// can drive the move to review through a card whose changes have a blocking smell.
		st.proj.SetReviewGate(qualitySvc)
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
