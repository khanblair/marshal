// Command marshald is the Marshal daemon. It owns all state and does all the work, and keeps
// running when the app is closed.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/acp"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/agents/claude"
	"github.com/khanblair/marshal/daemon/internal/agents/gemini"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/cardhistory"
	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/diff"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/fixture"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/search"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store"
)

const (
	dataDirMode  = 0o700
	databaseFile = "marshal.db"
	lockFileName = "marshald.lock"
	// Exit codes. exitAlreadyRunning is distinct from exitFailed so a caller (the desktop app,
	// or a service manager retrying a crash) can tell "another daemon already owns this data
	// folder" apart from every other kind of start-up failure.
	exitOK             = 0
	exitFailed         = 1
	exitBadInput       = 2
	exitAlreadyRunning = 3

	// stubAgentEnv overrides where the stub agent binary is. The default is a file next to the
	// running marshald executable, built by the same `pnpm build` that builds marshald.
	stubAgentEnv = "MARSHAL_STUB_AGENT"
	// stubAgentBinary is the name of the stub agent program (plus ".exe" on Windows).
	stubAgentBinary = "stub-agent"
	// stubAgentSpeed is the stub agent's pause multiplier in dev mode: its own normal default (a
	// background daemon has no reason to rush turns the way tests, which use "0", do).
	stubAgentSpeed = "1"
	// restoreAllTimeout bounds RestoreAll's own background run (see restoreSessions), so a stuck
	// agent program at start-up can never keep the daemon from shutting down cleanly either.
	restoreAllTimeout = 2 * time.Minute
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run starts the daemon and returns the process exit code. It is separate from main so tests
// can call it.
func run(args []string, stdout, stderr io.Writer) int {
	env, err := platform.CurrentEnv()
	if err != nil {
		say(stderr, "%v", err)
		return exitFailed
	}
	settings, err := config.Load(args, env, stderr)
	switch {
	case errors.Is(err, config.ErrHelp):
		return exitOK
	case err != nil:
		say(stderr, "%v", err)
		return exitBadInput
	}
	if settings.ShowVersion {
		say(stdout, "%s", buildinfo.Version)
		return exitOK
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: settings.LogLevel}))
	if err := serve(ctx, settings, env, log); err != nil {
		if errors.Is(err, platform.ErrAlreadyRunning) {
			// A plain sentence for the person, not the wrapped OS error: that detail is already
			// in the structured log line serve wrote before returning.
			say(stderr, "Marshal is already running for this user.")
			return exitAlreadyRunning
		}
		say(stderr, "%v", err)
		return exitFailed
	}
	return exitOK
}

// serve opens what the daemon owns, serves until the context ends, and closes it in the reverse
// order: the server stops accepting and closes its event streams first (Run does that), then the
// session manager stops every live agent, then the event bus closes, then the database.
func serve(ctx context.Context, settings config.Settings, env platform.Env, log *slog.Logger) (err error) {
	dataDir, err := filepath.Abs(settings.DataDir)
	if err != nil {
		return fmt.Errorf("resolve the data folder %s: %w", settings.DataDir, err)
	}
	settings.DataDir = dataDir
	if err := os.MkdirAll(settings.DataDir, dataDirMode); err != nil {
		return fmt.Errorf("make the data folder %s: %w", settings.DataDir, err)
	}
	// The lock is acquired before anything below touches the database or opens a listener: a
	// daemon that lost the race for this data folder must do neither.
	lock, err := platform.AcquireLock(filepath.Join(settings.DataDir, lockFileName))
	if err != nil {
		if errors.Is(err, platform.ErrAlreadyRunning) {
			log.Error("cannot start: another daemon already holds the lock", "data_dir", settings.DataDir, "error", err)
		}
		return err
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release the lock: %w", releaseErr))
		}
	}()
	log.Info("starting", "version", buildinfo.Version, "mode", settings.Mode, "data_dir", settings.DataDir)
	st, err := store.Open(ctx, filepath.Join(settings.DataDir, databaseFile), store.WithLogger(log))
	if err != nil {
		return fmt.Errorf("open the database: %w", err)
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close the database: %w", closeErr))
		}
	}()
	bus, err := events.New()
	if err != nil {
		return fmt.Errorf("start the event bus: %w", err)
	}
	defer bus.Close()

	mods, err := buildModules(st, bus, settings, env, log)
	if err != nil {
		return err
	}
	// Sessions must stop before the bus and the store close: this defer runs before theirs
	// (defers run last registered first), so every agent process is asked to end, and every
	// session's log file is closed, while the bus and the store it writes through still work.
	defer func() {
		if closeErr := mods.sessions.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close the session manager: %w", closeErr))
		}
	}()
	// The Home subscriber stops before the session manager, the bus, and the store: it is registered
	// last, so it runs first, and a card finish it is still writing lands while the store still
	// works. It is started before the fixture loads, so a card the fixture creates in the done state
	// is counted like any other.
	defer func() {
		if closeErr := mods.homeSub.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("stop the Home subscriber: %w", closeErr))
		}
	}()
	if err := mods.homeSub.Start(ctx); err != nil {
		return fmt.Errorf("start the Home subscriber: %w", err)
	}

	dev, err := api.EnsureAccounts(ctx, st, api.AccountsConfig{
		DataDir: settings.DataDir, Dev: settings.Dev(), Now: time.Now, Log: log,
	})
	if err != nil {
		return err
	}
	// The fixture loads before the restore below starts, and before Run serves anything, so the
	// restore and the first requests never see a half-made fixture.
	loaded := loadFixture(ctx, settings, mods.proj, mods.sessions, log)
	if loaded {
		// A fixture writes session rows so the screens have something to draw, and those sessions
		// have no process behind them. Restoring them would start an agent for every one of the
		// prototype's cards at every daemon start, which is neither wanted nor cheap, so a loaded
		// fixture leaves them for a person to resume (the manual side of architecture.md 5.3).
		log.Info("the fixture is loaded, so its sessions are left for a person to resume")
	} else {
		// Restoring sessions runs in its own goroutine, bounded by its own timeout, so a slow or
		// stuck agent program never delays Run below from serving the health endpoint.
		go restoreSessions(ctx, mods.sessions, log)
	}
	return api.New(settings, log, time.Now, api.Deps{
		Store: st, Bus: bus, Dev: dev, Projects: mods.proj, Sessions: mods.sessions,
		Catalog: mods.catalog, Dashboard: mods.dashboard, History: mods.history,
		Diff: mods.diff, Chats: mods.chats, Search: mods.search, Accounts: mods.accounts,
	}).Run(ctx)
}

// daemonModules are the parts of the daemon that buildModules makes together, once the store and
// the bus are ready.
type daemonModules struct {
	proj      *projects.Service
	sessions  *session.Manager
	catalog   catalog.Source
	dashboard *dashboard.Service
	homeSub   *dashboard.Subscriber
	history   *cardhistory.Service
	diff      *diff.Service
	chats     *chats.Service
	search    *search.Service
	accounts  *accounts.Service
}

// buildModules makes git, projects, the agent registry and catalog, and the session manager, in
// the order each depends on the last.
func buildModules(st *store.Store, bus *events.Bus, settings config.Settings, env platform.Env, log *slog.Logger) (daemonModules, error) {
	git := gitx.New()
	late := &lateSessions{}
	// A card's session state is read from the stored session rows, not from the manager, so the
	// projects service can have it before the manager is built.
	states, err := session.NewStoredStates(st)
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the session states: %w", err)
	}
	proj, err := projects.New(projects.Deps{Store: st, Bus: bus, Git: git, DataDir: settings.DataDir},
		projects.WithLogger(log), projects.WithSessionStopper(late), projects.WithAwakeCounter(late),
		projects.WithSessionStates(states))
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the projects module: %w", err)
	}
	registry, catalogSrc, err := buildAgents(settings, env, log)
	if err != nil {
		return daemonModules{}, err
	}
	terminals, err := buildTerminals(settings, env, catalogSrc, log)
	if err != nil {
		return daemonModules{}, err
	}
	// The history is built first and given to both: the session manager writes a card's events
	// through it as they happen, and the API layer pages them back for the card's chat and activity.
	historyStore, err := history.New(st)
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the history store: %w", err)
	}
	sessions, err := session.NewManager(st, bus, proj, registry, git, session.Config{
		DataDir: settings.DataDir, Logger: log, History: historyStore, Terminals: terminals,
	})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the session manager: %w", err)
	}
	late.manager.Store(sessions)
	home, err := dashboard.New(dashboard.Deps{Store: st}, dashboard.WithLogger(log))
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the dashboard module: %w", err)
	}
	// The dashboard's subscriber keeps the stored daily numbers and the activity stream current from
	// the events the daemon publishes. It is built here and started by the caller, which is also
	// where it is closed: its goroutine must stop before the bus and the store close.
	homeSub, err := dashboard.NewSubscriber(home, bus)
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the dashboard subscriber: %w", err)
	}
	cards, err := cardhistory.New(cardhistory.Deps{Store: st, History: historyStore}, cardhistory.WithLogger(log))
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the card history module: %w", err)
	}
	cardDiff, err := diff.New(diff.Deps{Projects: proj, Git: git}, diff.WithLogger(log))
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the card diff module: %w", err)
	}
	// The chats service owns a project's chats and their own sessions. It is given the session
	// manager, which starts a chat's agent when its first message is sent, puts it to sleep when the
	// chat is archived, and stops it and removes its logs when the chat is deleted. The manager never
	// reads the chats table, so the two need no late binding: the manager is built first.
	projectChats, err := chats.New(chats.Deps{Store: st, Bus: bus, Sessions: sessions}, chats.WithLogger(log))
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the chats module: %w", err)
	}
	// Search reads the projects, their cards, and their chats through the two services that own
	// them, never their tables.
	finder, err := search.New(search.Deps{Projects: proj, Chats: projectChats})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the search module: %w", err)
	}
	// The accounts service owns the person's profile, avatar, progress, and preferences. It asks the
	// projects module only whether a project and a saved view are there.
	you, err := accounts.New(accounts.Deps{Store: st, Bus: bus, Projects: proj, DataDir: settings.DataDir},
		accounts.WithLogger(log))
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the accounts module: %w", err)
	}
	return daemonModules{
		proj: proj, sessions: sessions, catalog: catalogSrc, dashboard: home, homeSub: homeSub,
		history: cards, diff: cardDiff, chats: projectChats, search: finder, accounts: you,
	}, nil
}

// lateSessions lets the projects service reach the session manager, which is built after it
// because the manager needs the projects service. It does nothing until the manager is set.
type lateSessions struct {
	manager atomic.Pointer[session.Manager]
}

func (l *lateSessions) StopCardSession(ctx context.Context, cardID string) error {
	if m := l.manager.Load(); m != nil {
		return m.StopCardSession(ctx, cardID)
	}
	return nil
}

func (l *lateSessions) RemoveCardLogs(ctx context.Context, cardID string) error {
	if m := l.manager.Load(); m != nil {
		return m.RemoveCardLogs(ctx, cardID)
	}
	return nil
}

func (l *lateSessions) StopProjectSessions(ctx context.Context, projectID string) error {
	if m := l.manager.Load(); m != nil {
		return m.StopProjectSessions(ctx, projectID)
	}
	return nil
}

func (l *lateSessions) AwakeCards(ctx context.Context, projectID string) (int, error) {
	if m := l.manager.Load(); m != nil {
		return m.AwakeCards(ctx, projectID)
	}
	return 0, nil
}

// loadFixture loads the fixture named by --fixture or MARSHAL_FIXTURE, and says whether one was
// loaded. A fixture that is not asked for, or that cannot be loaded, is not fatal: the daemon
// starts without it and the log says so.
func loadFixture(ctx context.Context, settings config.Settings, proj fixture.Cards, sessions fixture.Sessions, log *slog.Logger) bool {
	if settings.Fixture != "" && !settings.Dev() {
		// Sample projects must never land in the data folder of a real install.
		log.Warn("fixtures load only in dev mode, so nothing was loaded", "fixture", settings.Fixture)
		return false
	}
	switch settings.Fixture {
	case "":
		return false
	case fixture.PrototypeName:
		if err := fixture.LoadPrototype(ctx, settings.DataDir, proj,
			fixture.WithLogger(log), fixture.WithSessions(sessions)); err != nil {
			log.Warn("could not load the prototype fixture, so starting without all of it", "error", err)
			return false
		}
		return true
	default:
		log.Warn("unknown fixture, so nothing was loaded", "fixture", settings.Fixture, "known", fixture.PrototypeName)
		return false
	}
}

// restoreSessions resumes the sessions left over from the last run (docs/architecture.md 5.3). It
// logs its own errors instead of returning them: nothing above is waiting for it.
func restoreSessions(ctx context.Context, sessions *session.Manager, log *slog.Logger) {
	rctx, cancel := context.WithTimeout(ctx, restoreAllTimeout)
	defer cancel()
	if err := sessions.RestoreAll(rctx); err != nil {
		log.Error("could not restore agent sessions", "error", err)
	}
}

// buildAgents makes the agent registry the session manager starts processes through, and the
// catalog the API layer will list agents from. In stub mode (settings.Agent == config.AgentStub)
// every kind is registered behind the stub agent binary. In real mode, only claude and gemini get
// a factory, since those are the only kinds with a real adapter today (agents/claude,
// agents/gemini); codex is left unregistered, so choosing it gives agents.ErrUnknownKind, which is
// the correct, honest answer until agents/codex has a real adapter.
func buildAgents(settings config.Settings, env platform.Env, log *slog.Logger) (*agents.Registry, catalog.Source, error) {
	registry := agents.NewRegistry()
	if settings.Agent == config.AgentStub {
		return registerStubAgents(registry, settings, env, log)
	}
	cat := catalog.New(catalog.Options{Logger: log})
	for _, kind := range []protocol.AgentKind{protocol.AgentKindClaude, protocol.AgentKindGemini} {
		if err := registry.Register(kind, realAgentFactory(cat, kind, log)); err != nil {
			return nil, nil, fmt.Errorf("register the %s agent: %w", kind, err)
		}
	}
	return registry, cat, nil
}

// registerStubAgents registers every kind catalog.Kinds() lists behind the same agents/acp
// adapter configuration, pointed at the built stub agent binary (catalog.Stub's own doc comment:
// "The session manager starts the scripted stub agent behind each kind"). Its --state-dir is a
// fixed folder under the data directory, so its own sessions resume across a daemon restart the
// same way real ones do.
func registerStubAgents(registry *agents.Registry, settings config.Settings, env platform.Env, log *slog.Logger) (*agents.Registry, catalog.Source, error) {
	path, err := stubAgentPath(env)
	if err != nil {
		return nil, nil, err
	}
	stateDir := filepath.Join(settings.DataDir, "dev-agent-state")
	args := []string{"--state-dir", stateDir, "--speed", stubAgentSpeed}
	factory := func() (agents.Agent, error) {
		return acp.New(acp.Config{Path: path, Args: args, Logger: log})
	}
	for _, kind := range catalog.Kinds() {
		if err := registry.Register(kind, factory); err != nil {
			return nil, nil, fmt.Errorf("register the stub agent for %s: %w", kind, err)
		}
	}
	return registry, catalog.NewStub(), nil
}

// stubAgentPath resolves the stub agent binary: the MARSHAL_STUB_AGENT environment override if
// set, else a file named stub-agent (stub-agent.exe on Windows) in the same folder as the running
// marshald executable. It fails with a clear, actionable message if neither is found: the stub
// agent is never built at runtime.
func stubAgentPath(env platform.Env) (string, error) {
	if override := env.Getenv(stubAgentEnv); override != "" {
		return override, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find the running program: %w", err)
	}
	name := stubAgentBinary
	if env.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(filepath.Dir(exe), name)
	if _, statErr := os.Stat(path); statErr != nil {
		return "", fmt.Errorf(
			"find the stub agent at %s: run `pnpm build` first, or set %s to its path: %w", path, stubAgentEnv, statErr)
	}
	return path, nil
}

// realAgentFactory returns a Factory that detects the installed agents the first time it is
// called, never at start-up (the catalog package's own doc comment: detection "never runs when
// the daemon starts"), and starts the real adapter for kind if it is installed and startable.
func realAgentFactory(cat catalog.Source, kind protocol.AgentKind, log *slog.Logger) agents.Factory {
	return func() (agents.Agent, error) {
		detected, err := cat.Detect(context.Background())
		if err != nil {
			return nil, fmt.Errorf("detect installed agents: %w", err)
		}
		for _, d := range detected {
			if d.Kind == kind {
				return realAgentOf(kind, d, log)
			}
		}
		return nil, fmt.Errorf("%w: %s", agents.ErrUnknownKind, kind)
	}
}

// realAgentOf starts the real adapter for a detected agent, or reports agents.ErrUnknownKind when
// it is not installed or Marshal cannot start it yet.
func realAgentOf(kind protocol.AgentKind, d catalog.Detected, log *slog.Logger) (agents.Agent, error) {
	if !d.Startable {
		return nil, fmt.Errorf("%w: %s is not installed, or Marshal cannot start it yet", agents.ErrUnknownKind, kind)
	}
	switch kind {
	case protocol.AgentKindClaude:
		return claude.New(claude.Config{Path: d.Path, Logger: log})
	case protocol.AgentKindGemini:
		return gemini.New(gemini.Config{Path: d.Path, Logger: log})
	default:
		return nil, fmt.Errorf("%w: %s", agents.ErrUnknownKind, kind)
	}
}
