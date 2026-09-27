// Command marshald is the Marshal daemon. It owns all state and does all the work, and keeps
// running when the app is closed.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/acp"
	"github.com/khanblair/marshal/daemon/internal/agents/builtin"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/agents/claude"
	"github.com/khanblair/marshal/daemon/internal/agents/gemini"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/auditlog"
	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/cardhistory"
	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/ci"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/connectiontest"
	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/diff"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/fixture"
	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/localci"
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
	sleepsettings "github.com/khanblair/marshal/daemon/internal/settings"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/webui"
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
	// Marshal's starter roles are added before anything is served: a role that is missing is added,
	// and a role a person has edited is left exactly as it is.
	if err := mods.roles.EnsureStarters(ctx); err != nil {
		return fmt.Errorf("add the starter roles: %w", err)
	}
	// The "after a restart" setting is read before anything is restored, so the cards come back the
	// way the person chose on Settings > Sleep (B5.6). The config's mode is the default until the
	// screen has ever been saved.
	applyResumeSetting(ctx, mods.sleepSettings, mods.sessions, log)
	// The idle timer starts before the restore, so a session the restore brings back is watched from
	// its first idle minute like any other. It stops with the manager, in Manager.Close.
	mods.sessions.StartSleepWatch()
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
	// The CI polling backup covers the deliveries GitHub did not send (architecture.md section 9).
	// It starts after the modules exist and stops before the daemon returns, so a sweep never runs
	// against a half-built daemon or after the store is closed.
	mods.ci.Start(ctx)
	defer mods.ci.Stop()
	// Every dev server the preview module started is stopped before the daemon returns, so a
	// person's machine is not left with one running after Marshal is gone. It stops before the
	// session manager, the bus, and the store, because it is registered last of the three.
	defer mods.preview.Close()

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
		Auditlog: mods.auditlog, Providers: mods.providers, CostLimits: mods.costLimits,
		ConnectionTests: mods.connectionTests, Roles: mods.roles,
		PullRequests: mods.pullRequests, Review: mods.review, Integrator: mods.integrator,
		SleepSettings: mods.sleepSettings, Quality: mods.quality,
		Integrations: mods.integrations, Webhooks: mods.integrations.Receiver(),
		CI: mods.ci, LocalCI: mods.localCI, Preview: mods.preview,
		WebUI: webUIFS(),
	}).Run(ctx)
}

// webUIFS is the built web app, or nil when the build that made this binary never copied one in
// (scripts/copy-web-dist.mjs runs before every real build; a bare `go build` in daemon/ for a Go
// test or a quick check does not, and the daemon still answers every /v1 route the same either
// way, it just has no page to show at "/").
func webUIFS() fs.FS {
	if !webui.Available() {
		return nil
	}
	return webui.FS()
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
	auditlog  *auditlog.Service
	// providers holds the keychain and the keys in it, and costLimits the cost and awake ceilings.
	// They are separate services: a ceiling needs only the store, so limits work with no key saved.
	providers  *providers.Service
	costLimits *providers.Limits
	// connectionTests runs a connection's test and remembers its result, for every kind of
	// connection Marshal is set up with (docs/architecture.md section 18).
	connectionTests *connectiontest.Runner
	// roles owns Marshal's role templates and a project's overrides of them. Its starter roles are
	// added by EnsureStarters, which Run calls before anything is served.
	roles *roles.Service
	// pullRequests opens a card's branch as a real pull request on GitHub (B5.4, build-plan 5.6).
	// It is nil when no GitHub token is saved, so its route is not registered and the section
	// stays "Not connected".
	pullRequests *pullrequest.Service
	// review runs the Reviewer role over a card's pull request (B5.4, build-plan 5.7). It is nil
	// for the same reason as pullRequests, and its route follows it.
	review *review.Service
	// integrator runs the merge queue (B5.5, build-plan 5.8). It is always built: a clean merge
	// needs only Git and the store, and its tests are supplied by the local CI of a later phase.
	integrator *integrator.Service
	// sleepSettings reads and writes the numbers the automatic sleep is driven by (B5.6, N5). It is
	// always built: the settings are rows in the store, and the screen that edits them needs no
	// other service.
	sleepSettings *sleepsettings.Service
	// quality checks a card's own changes for code smells (B5.8, architecture.md section 17). It is
	// always built: it needs only the store, the projects service, Git, and the session manager as
	// the way it reaches a card's agent. It is also the projects service's review gate, so a card
	// whose changes have a blocking smell stays in Working until it is fixed or dismissed.
	quality *quality.Service
	// integrations owns the connections Marshal is set up with apart from model providers: the
	// GitHub App today, and later phases' services (B6.1, B6.7, architecture.md section 18). It
	// holds the GitHub delivery receiver, which is what the webhook route hands deliveries to.
	integrations *integrations.Service
	// ci is the CI monitor: it turns a workflow run on a card's branch into a card's CI state and
	// runs the failure loop of section 9 (B6.2, B6.3). It is always built - it needs the store, the
	// projects service, Git, and the roles service, all of which exist whether or not GitHub is
	// connected - and it is the sink the delivery receiver hands verified deliveries to. Without a
	// GitHub connection its forge resolves to nothing, so only a simulated failure moves a run.
	ci *ci.Service
	// localCI runs a card's own workflow steps on this machine (B6.5, build-plan 6.5, section
	// 15.3): "run the same checks as GitHub Actions on your machine, before pushing". It is always
	// built - it needs only the projects service, to find the card's worktree - and it needs no
	// GitHub connection at all: the whole point is to catch a failure before the push.
	localCI *localci.Service
	// preview runs one dev server per card and takes the before and after screenshots of it (B6.6,
	// build-plan 6.6 and 6.7, architecture.md section 11.2). It is always built: a preview needs the
	// projects service for the card's worktree and the project's dev command, and the daemon's data
	// folder for the shots and the browser profiles. Its dev servers are stopped when the daemon
	// returns, so a person's machine is not left running one.
	preview *preview.Service
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
	// The model providers own the keychain and the keys in it (Slice 4). They are built before the
	// agents because the built-in agent resolves every model through them and the catalog lists the
	// models of the providers that are set up.
	//
	// The notice writer is deliberately left out: telling a person about a fallback needs the
	// notices surface, which is Phase 5's (B5.6). A fallback is logged either way
	// (providers.noticeFallback), so nothing happens silently in the meantime.
	providerSvc, err := providers.New(security.NewOSKeychain(platform.AppName(settings.Mode)), providers.Options{
		Logger:   log,
		Recorder: providers.NewStoreRecorder(st, rand.Reader),
		Now:      time.Now,
	})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the model providers: %w", err)
	}
	costLimits := providers.NewLimits(st)
	// The sleep settings are the numbers the automatic sleep is driven by (B5.6, N5): the idle
	// time, the warning time, the keep-awake time, what happens to awake cards at restart, and
	// where a warning goes. They live in the `settings` table under one key, the way a limit lives
	// in the `limits` table, because they are edited on a screen while the daemon runs.
	sleepSettingsSvc, err := sleepsettings.New(st)
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the settings module: %w", err)
	}
	// One runner tests every connection Marshal can be set up with (docs/architecture.md section
	// 18): a model provider today, and later phases' integrations and MCP servers through the same
	// ones, because the cooldown, the time limit, and the saved result are the same for all of them.
	connectionTests := connectiontest.New(st, connectiontest.Options{Logger: log, Now: time.Now})
	registry, catalogSrc, err := buildAgents(settings, env, log, providerSvc)
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
	// One audit recorder serves the whole daemon (docs/architecture.md section 10). The session
	// manager writes a decision on an approval, a bypass toggle, a checkpoint restore, and a card
	// stopped by its limits through it; the CI monitor writes a simulated failure. It is stateless
	// and safe for many goroutines, so one is shared rather than each module making its own.
	auditRec, err := audit.New(st, time.Now, nil, log)
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the audit recorder: %w", err)
	}
	sessions, err := session.NewManager(st, bus, proj, registry, git, session.Config{
		DataDir: settings.DataDir, Logger: log, History: historyStore, Plans: historyStore,
		Terminals: terminals, Audit: auditRec,
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
	projectChats, err := chats.New(chats.Deps{Store: st, Bus: bus, Sessions: sessions, Titles: providerSvc},
		chats.WithLogger(log))
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
	// The audit log's read service backs the read-only list, search, and export routes (B3.5). The
	// rows themselves are written by internal/audit as things happen; this only reads them back.
	auditRead, err := auditlog.New(auditlog.Deps{Store: st}, auditlog.WithLogger(log))
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the audit log module: %w", err)
	}
	// The roles module owns Marshal's role templates and a project's overrides of them (B5.1, N18).
	// It needs only the store. Its starter roles are added by EnsureStarters, which the caller runs
	// before anything is served; adding one that is already there does nothing.
	roleSvc, err := roles.New(roles.Deps{Store: st}, roles.WithLogger(log))
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the roles module: %w", err)
	}
	// The roles module is the session manager's reader of a card's role ceilings, so the harness can
	// stop a card that passes the role it runs under (B5.3, build-plan 5.3). It is set here rather
	// than handed to NewManager because it is built after the manager; the manager reads it when a
	// card's turn ends, so a card already running picks it up.
	sessions.SetRoleLimits(roleSvc)
	// The automatic sleep's two readers (B5.6): the settings above, which decide the idle time, the
	// warning, and the keep-awake length, and the cost limits service, which answers a project's
	// awake ceiling. Both are set here rather than handed to NewManager because both are built
	// after it; the manager reads them on each sweep, so a setting a person saves is in force on
	// the next pass with no restart.
	sessions.SetSleepSettings(sleepSettingsSvc)
	sessions.SetAwakeLimits(costLimits)
	// The pull-request service opens a card's branch as a real pull request on GitHub (B5.4,
	// build-plan 5.6), and the review service runs the Reviewer role over every pull request that
	// is opened (B5.4, build-plan 5.7). Both are built only when a GitHub token has been saved in
	// the keychain; with no token they are left out, so POST /v1/cards/{id}/pull-request and
	// POST /v1/cards/{id}/review do not exist and nothing is pretended. Phase 6 adds the GitHub App
	// and the screen that saves the token.
	pullReq, reviewSvc, err := buildForge(settings, proj, git, roleSvc, sessions, log)
	if err != nil {
		return daemonModules{}, err
	}
	// The merge queue (B5.5, build-plan 5.8). Its tests are nil for now: a clean merge with no
	// test runner moves the target forward, and Phase 6's local CI supplies the runner that makes
	// the queue wait for a real test run.
	mergeQueue, err := integrator.New(integrator.Deps{
		Cards: proj, Projects: proj, Git: git, DataDir: settings.DataDir, Log: log,
	})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the merge queue: %w", err)
	}
	// The code-smell module (B5.8, architecture.md section 17). It is built last of the modules
	// that touch a card, because it reads cards and projects through the projects service and
	// reaches a card's agent through the session manager, and only then is it handed back to the
	// projects service as its review gate: a card whose changes have a blocking smell is kept in
	// Working and the finding goes back to the agent that wrote the code.
	//
	// Its linters are real child programs run through internal/proc, started in the card's own
	// worktree with the project's own configuration; its built-in checks need no program at all.
	qualitySvc, err := quality.New(quality.Deps{
		Store: st, Cards: proj, Projects: proj, Git: git, Worker: sessions, Bus: bus, Log: log,
	})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the code-smell module: %w", err)
	}
	proj.SetReviewGate(qualitySvc)
	// The connections Marshal is set up with apart from model providers (B6.1, B6.7, architecture.md
	// section 18): the GitHub App today. It owns the GitHub delivery receiver the webhook route
	// hands deliveries to, and it reads the webhook secret from the keychain on every delivery, so
	// saving the App in Settings takes effect without a restart - which is why the receiver exists
	// even on a daemon nobody has connected GitHub to. Until then every delivery is refused.
	integrationSvc, err := integrations.New(st, security.NewOSKeychain(platform.AppName(settings.Mode)),
		integrations.Options{Logger: log, Now: time.Now})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the integrations module: %w", err)
	}
	// The CI monitor (B6.2, B6.3, architecture.md section 9) watches the GitHub App's workflow runs.
	// Its forge is an adapter rather than a client, because the App is saved and removed while the
	// daemon runs: every call asks the connections service for the App as it is now, and answers
	// ci.ErrNoForge while nobody has connected GitHub, which is what makes the monitor harmless on a
	// machine that has never signed in.
	ciSvc, err := ci.New(ci.Deps{
		Store: st, Cards: proj, Projects: proj, Git: git, Roles: roleSvc,
		Worker: sessions, Bus: bus, Audit: auditRec,
		Options: ci.Options{Logger: log, Now: time.Now, Forge: appForge{svc: integrationSvc}},
	})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the CI monitor: %w", err)
	}
	// Every verified delivery the receiver accepts reaches the monitor, which is what turns a
	// replayed webhook into a card's CI state.
	integrationSvc.SetMonitor(ciSvc)
	// Local CI (B6.5, build-plan 6.5): the project's own workflow files, run in the card's worktree.
	// It reads the worktree through the projects service and starts each step through internal/proc,
	// so nothing here needs a forge, a token, or a network.
	localCISvc, err := localci.New(localci.Deps{Projects: proj, Log: log, Now: time.Now})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the local CI module: %w", err)
	}
	// Live preview (B6.6, build-plan 6.6 and 6.7): one dev server per card, on a port picked for it,
	// in the card's own worktree, with the before and after screenshots kept under the daemon's data
	// folder. Every seam is left at its real default - the command runner, the browser lookup, the
	// port picker, the prober, and the chromedp shooter - so pressing Start in the Preview tab runs
	// the project's own dev command, and pressing Take screenshot drives the person's own Chrome or
	// Edge. Nothing is started here: the service starts a dev server only when it is asked to.
	previewSvc, err := preview.New(preview.Deps{
		Projects: proj, Bus: bus, DataDir: settings.DataDir, Log: log, Now: time.Now,
	})
	if err != nil {
		return daemonModules{}, fmt.Errorf("start the preview module: %w", err)
	}
	return daemonModules{
		proj: proj, sessions: sessions, catalog: catalogSrc, dashboard: home, homeSub: homeSub,
		history: cards, diff: cardDiff, chats: projectChats, search: finder, accounts: you,
		auditlog: auditRead, providers: providerSvc, costLimits: costLimits,
		connectionTests: connectionTests, roles: roleSvc, pullRequests: pullReq,
		review: reviewSvc, integrator: mergeQueue, sleepSettings: sleepSettingsSvc,
		quality: qualitySvc, integrations: integrationSvc, ci: ciSvc, localCI: localCISvc,
		preview: previewSvc,
	}, nil
}

// appForge is the CI monitor's forge, answered from the GitHub App saved in Settings (B6.2,
// architecture.md section 9). It resolves the App on every call rather than holding a client, so
// connecting, replacing, or removing GitHub in Settings takes effect without a restart, and a daemon
// nobody has connected answers ci.ErrNoForge - which the monitor treats as "there is nothing to ask"
// rather than as a failure.
type appForge struct {
	svc *integrations.Service
}

// client answers the GitHub client of the stored App, or ci.ErrNoForge when none is set up.
func (f appForge) client(ctx context.Context) (*github.TransportClient, error) {
	app, err := f.svc.App(ctx)
	if err != nil {
		if errors.Is(err, integrations.ErrNotConnected) {
			return nil, ci.ErrNoForge
		}
		return nil, err
	}
	return app.Client(), nil
}

// ListWorkflowRuns lists one branch's workflow runs, newest first.
func (f appForge) ListWorkflowRuns(ctx context.Context, repo github.Repository, branch string) ([]github.WorkflowRun, error) {
	client, err := f.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListWorkflowRuns(ctx, repo, branch)
}

// RerunFailedJobs asks GitHub to run one run's failed jobs again.
func (f appForge) RerunFailedJobs(ctx context.Context, repo github.Repository, runID int64) error {
	client, err := f.client(ctx)
	if err != nil {
		return err
	}
	return client.RerunFailedJobs(ctx, repo, runID)
}

// FailedLog reads the log of one run's first failed job, cut to maxBytes.
func (f appForge) FailedLog(ctx context.Context, repo github.Repository, runID int64, maxBytes int) (string, error) {
	client, err := f.client(ctx)
	if err != nil {
		return "", err
	}
	return client.FailedLog(ctx, repo, runID, maxBytes)
}

// buildForge builds the two services that talk to GitHub from the token saved in the keychain: the
// pull-request service, which opens a card's branch as a pull request, and the review service,
// which runs the Reviewer role over every pull request. Both answer with a nil service and no error
// on a machine with no token (the usual case until the owner signs in): not being signed in is not
// a failure to start.
//
// The review is wired into the pull-request service's After hook, so a pull request is read as soon
// as it exists (build-plan 5.7). The hook runs the review service, whose Reviewer is the
// ChecksReviewer over the same client, and whose worker is the session manager, so a request for
// changes reaches the agent that wrote the branch.
func buildForge(settings config.Settings, proj *projects.Service, git *gitx.Git, roleSvc *roles.Service, sessions *session.Manager, log *slog.Logger) (*pullrequest.Service, *review.Service, error) {
	keys := security.NewOSKeychain(platform.AppName(settings.Mode))
	token, err := keys.Get(githubTokenProvider)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, nil, nil
	}
	client, err := github.NewTokenClient(token)
	if err != nil {
		return nil, nil, fmt.Errorf("build the GitHub client: %w", err)
	}
	checker, err := review.NewChecksReviewer(client)
	if err != nil {
		return nil, nil, fmt.Errorf("start the review module: %w", err)
	}
	reviewSvc, err := review.New(review.Deps{
		Cards: proj, Projects: proj, Roles: roleSvc, Git: git, Forge: client,
		Reviews: checker, Worker: sessions, Log: log,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start the review service: %w", err)
	}
	svc, err := pullrequest.New(pullrequest.Deps{
		Client: client, Cards: proj, Projects: proj, Git: git, Log: log,
		After: func(ctx context.Context, cardID string) error {
			_, err := reviewSvc.Review(ctx, cardID)
			return err
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start the pull-request service: %w", err)
	}
	log.Info("the pull-request and review services are ready")
	return svc, reviewSvc, nil
}

// githubTokenProvider is the keychain entry the GitHub personal token is filed under. Two shapes
// use it: Phase 5's personal token and, in Phase 6, the GitHub App's own credentials, both under
// this one name because only one GitHub connection exists at a time.
const githubTokenProvider = "github"

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

// applyResumeSetting gives the session manager the "After a restart" setting a person saved on
// Settings > Sleep (B5.6), which decides what RestoreAll does with the sessions that were awake
// when the daemon last stopped (docs/architecture.md 5.3). It outranks the flag the daemon was
// started with: the flag is the default until the screen is saved.
//
// A settings row that cannot be read leaves the manager on that default. A database that cannot
// answer is not a reason to change how a person's cards come back, and the log says so.
func applyResumeSetting(ctx context.Context, svc *sleepsettings.Service, sessions *session.Manager, log *slog.Logger) {
	cfg, err := svc.Sleep(ctx)
	if err != nil {
		log.Warn("could not read the sleep settings, so the resume mode from the flags stands", "error", err)
		return
	}
	if cfg.Restore == protocol.SleepRestoreManual {
		sessions.SetResumeMode(session.ResumeModeManual)
		return
	}
	sessions.SetResumeMode(session.ResumeModeAuto)
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
// every kind is registered behind the stub agent binary. In real mode the three kinds with a real
// adapter get a factory: claude and gemini are CLI programs the catalog finds on the machine
// (agents/claude, agents/gemini), and builtin is Marshal's own agent, which runs here in the daemon
// (agents/builtin) and so has no program to find. Codex is left unregistered, so choosing it gives
// agents.ErrUnknownKind, which is the correct, honest answer until agents/codex has a real adapter.
func buildAgents(settings config.Settings, env platform.Env, log *slog.Logger, providerSvc *providers.Service) (*agents.Registry, catalog.Source, error) {
	registry := agents.NewRegistry()
	if settings.Agent == config.AgentStub {
		return registerStubAgents(registry, settings, env, log)
	}
	// BuiltinModels is the key store's own list: the built-in agent can run the models of exactly the
	// providers that have a key saved, so its row is asked again on every List and shows a key the
	// person saved a moment ago.
	cat := catalog.New(catalog.Options{Logger: log, BuiltinModels: providerSvc.Models})
	for _, kind := range []protocol.AgentKind{protocol.AgentKindClaude, protocol.AgentKindGemini} {
		if err := registry.Register(kind, realAgentFactory(cat, kind, log)); err != nil {
			return nil, nil, fmt.Errorf("register the %s agent: %w", kind, err)
		}
	}
	if err := registry.Register(protocol.AgentKindBuiltin, builtinAgentFactory(log, providerSvc)); err != nil {
		return nil, nil, fmt.Errorf("register the built-in agent: %w", err)
	}
	return registry, cat, nil
}

// builtinAgentFactory returns the factory for Marshal's own agent. It runs in the daemon, so there
// is no path to resolve and nothing to detect: what it needs is a model provider.
//
// The resolver is the provider service itself, which is the one place that knows which providers
// have a key saved: builtin.Resolver and providers.Service.Resolve have the same shape on purpose,
// so the key store the screens write and the resolver the agent reads are the same object. A model
// no saved key can run is reported as one nobody can run, in plain words, rather than pretending to
// start.
func builtinAgentFactory(log *slog.Logger, resolver builtin.Resolver) agents.Factory {
	return builtin.Factory(builtin.Config{
		Resolver: resolver,
		Profile:  security.DefaultProfile(),
		Logger:   log,
	})
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
