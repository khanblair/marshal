package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/auditlog"
	"github.com/khanblair/marshal/daemon/internal/briefs"
	"github.com/khanblair/marshal/daemon/internal/cardhistory"
	"github.com/khanblair/marshal/daemon/internal/chats"
	"github.com/khanblair/marshal/daemon/internal/ci"
	"github.com/khanblair/marshal/daemon/internal/codemap"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/connectiontest"
	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/diff"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/mcpattach"
	"github.com/khanblair/marshal/daemon/internal/mcpserver"
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
	"github.com/khanblair/marshal/daemon/internal/schedules"
	"github.com/khanblair/marshal/daemon/internal/search"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/session"
	sleepsettings "github.com/khanblair/marshal/daemon/internal/settings"
	"github.com/khanblair/marshal/daemon/internal/store"
)

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
	// trelloOutbound watches Marshal's own card.moved events and mirrors a card that reaches done
	// onto its linked Trello card (B8.2, the Marshal-to-Trello direction). It is always built -
	// nothing here needs Trello to be connected, only the event bus - and a card that is not linked
	// to Trello is the ordinary path its own apply takes, not an error.
	trelloOutbound *integrations.OutboundSync
	// gmailPoller reads Gmail's watched label on a timer and turns a message into a card, once
	// (B8.3). It is always built - nothing here needs Gmail to be connected, only a store to poll
	// against - and every poll with nothing set up answers nothing, quietly.
	gmailPoller *integrations.GmailPoller
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
	// memory is the knowledge base: the card notes, the file claims, and the vault they live in
	// (B7.2, B7.4, architecture.md section 12). It is the reader and writer the note routes and the
	// internal MCP server's note and claim tools use.
	memory *memory.Service
	// mcpHost serves the internal MCP server to live cards' agents (B7.1, section 11.4). It is
	// handed to the API as the endpoint that mounts it, and the session manager's attacher reaches it
	// to add and remove a card's server.
	mcpHost *mcpserver.Host
	// codemap is the codebase map: the light per-project index of files and symbols that lets an
	// agent ask where a name is (B7.5, build-plan task 7.9). It is what the internal MCP server's
	// search_codebase answers from.
	codemap *codemap.Map
	// schedules owns the scheduled jobs and briefs and the cron that runs them (B8.1, build-plan
	// 8.1). It is always built - its rows are the store's and its clock is the daemon's - and it is
	// started only once every module a run can reach exists.
	schedules *schedules.Service
}

// coreDaemonModules is git, the project and session machinery, and the recorders every later
// module needs - buildModules' first stage.
type coreDaemonModules struct {
	git              *gitx.Git
	lateMem          *lateMemory
	proj             *projects.Service
	providerSvc      *providers.Service
	costLimits       *providers.Limits
	sleepSettingsSvc *sleepsettings.Service
	connectionTests  *connectiontest.Runner
	catalogSrc       catalog.Source
	historyStore     *history.Store
	auditRec         *audit.Recorder
	sessions         *session.Manager
}

// buildCoreModules makes git, projects, the agent registry and catalog, and the session manager,
// in the order each depends on the last.
func buildCoreModules(st *store.Store, bus *events.Bus, settings config.Settings, env platform.Env, log *slog.Logger) (coreDaemonModules, error) {
	git := gitx.New()
	late := &lateSessions{}
	lateMem := &lateMemory{}
	// A card's session state is read from the stored session rows, not from the manager, so the
	// projects service can have it before the manager is built.
	states, err := session.NewStoredStates(st)
	if err != nil {
		return coreDaemonModules{}, fmt.Errorf("start the session states: %w", err)
	}
	proj, err := projects.New(projects.Deps{Store: st, Bus: bus, Git: git, DataDir: settings.DataDir},
		projects.WithLogger(log), projects.WithSessionStopper(late), projects.WithAwakeCounter(late),
		projects.WithSessionStates(states), projects.WithMemoryRemover(lateMem))
	if err != nil {
		return coreDaemonModules{}, fmt.Errorf("start the projects module: %w", err)
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
		return coreDaemonModules{}, fmt.Errorf("start the model providers: %w", err)
	}
	costLimits := providers.NewLimits(st)
	// The sleep settings are the numbers the automatic sleep is driven by (B5.6, N5): the idle
	// time, the warning time, the keep-awake time, what happens to awake cards at restart, and
	// where a warning goes. They live in the `settings` table under one key, the way a limit lives
	// in the `limits` table, because they are edited on a screen while the daemon runs.
	sleepSettingsSvc, err := sleepsettings.New(st)
	if err != nil {
		return coreDaemonModules{}, fmt.Errorf("start the settings module: %w", err)
	}
	// One runner tests every connection Marshal can be set up with (docs/architecture.md section
	// 18): a model provider today, and later phases' integrations and MCP servers through the same
	// ones, because the cooldown, the time limit, and the saved result are the same for all of them.
	connectionTests := connectiontest.New(st, connectiontest.Options{Logger: log, Now: time.Now})
	registry, catalogSrc, err := buildAgents(settings, env, log, providerSvc)
	if err != nil {
		return coreDaemonModules{}, err
	}
	terminals, err := buildTerminals(settings, env, catalogSrc, log)
	if err != nil {
		return coreDaemonModules{}, err
	}
	// The history is built first and given to both: the session manager writes a card's events
	// through it as they happen, and the API layer pages them back for the card's chat and activity.
	historyStore, err := history.New(st)
	if err != nil {
		return coreDaemonModules{}, fmt.Errorf("start the history store: %w", err)
	}
	// One audit recorder serves the whole daemon (docs/architecture.md section 10). The session
	// manager writes a decision on an approval, a bypass toggle, a checkpoint restore, and a card
	// stopped by its limits through it; the CI monitor writes a simulated failure. It is stateless
	// and safe for many goroutines, so one is shared rather than each module making its own.
	auditRec, err := audit.New(st, time.Now, nil, log)
	if err != nil {
		return coreDaemonModules{}, fmt.Errorf("start the audit recorder: %w", err)
	}
	sessions, err := session.NewManager(st, bus, proj, registry, git, session.Config{
		DataDir: settings.DataDir, Logger: log, History: historyStore, Plans: historyStore,
		Terminals: terminals, Audit: auditRec,
	})
	if err != nil {
		return coreDaemonModules{}, fmt.Errorf("start the session manager: %w", err)
	}
	late.manager.Store(sessions)
	return coreDaemonModules{
		git: git, lateMem: lateMem, proj: proj, providerSvc: providerSvc, costLimits: costLimits,
		sleepSettingsSvc: sleepSettingsSvc, connectionTests: connectionTests, catalogSrc: catalogSrc,
		historyStore: historyStore, auditRec: auditRec, sessions: sessions,
	}, nil
}

// cardDaemonModules is what a card is read and searched through, and what the session manager's
// role and sleep readers are - buildModules' second stage, built from its first.
type cardDaemonModules struct {
	home         *dashboard.Service
	homeSub      *dashboard.Subscriber
	cards        *cardhistory.Service
	cardDiff     *diff.Service
	projectChats *chats.Service
	mem          *memory.Service
	finder       *search.Service
	you          *accounts.Service
	auditRead    *auditlog.Service
	roleSvc      *roles.Service
}

// buildCardModules makes the dashboard, a card's history and diff, a project's chats, the memory
// module, search, accounts, the audit log's reader, and roles - and gives the session manager the
// role and sleep readers it needs once they exist. ctx is the daemon's own context, which the
// vault watcher this starts stops with.
func buildCardModules(ctx context.Context, st *store.Store, bus *events.Bus, settings config.Settings, log *slog.Logger, core coreDaemonModules) (cardDaemonModules, error) {
	home, err := dashboard.New(dashboard.Deps{Store: st}, dashboard.WithLogger(log))
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the dashboard module: %w", err)
	}
	// The dashboard's subscriber keeps the stored daily numbers and the activity stream current from
	// the events the daemon publishes. It is built here and started by the caller, which is also
	// where it is closed: its goroutine must stop before the bus and the store close.
	homeSub, err := dashboard.NewSubscriber(home, bus)
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the dashboard subscriber: %w", err)
	}
	cards, err := cardhistory.New(cardhistory.Deps{Store: st, History: core.historyStore}, cardhistory.WithLogger(log))
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the card history module: %w", err)
	}
	cardDiff, err := diff.New(diff.Deps{Projects: core.proj, Git: core.git}, diff.WithLogger(log))
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the card diff module: %w", err)
	}
	// The chats service owns a project's chats and their own sessions. It is given the session
	// manager, which starts a chat's agent when its first message is sent, puts it to sleep when the
	// chat is archived, and stops it and removes its logs when the chat is deleted. The manager never
	// reads the chats table, so the two need no late binding: the manager is built first.
	projectChats, err := chats.New(chats.Deps{Store: st, Bus: bus, Sessions: core.sessions, Titles: core.providerSvc},
		chats.WithLogger(log))
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the chats module: %w", err)
	}
	// The memory module (B7.2, B7.4, B7.5, architecture.md section 12): the one note each card keeps,
	// the files a card has claimed, and the search over them. Its vault is `<data>/vault`, so the
	// whole knowledge base is one folder a person can open in Obsidian, and the note files - not the
	// rows - are what that person edits.
	mem, err := memory.New(memory.Deps{Store: st, Cards: core.proj, Root: vaultRoot(settings.DataDir)},
		memory.WithClock(time.Now))
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the memory module: %w", err)
	}
	// The projects service is told where a removed project's memory goes. It is set after the module
	// is built because the two need each other: memory reads a project's cards, and the project's
	// removal deletes its memory folder.
	core.lateMem.svc.Store(mem)
	// The vault watcher brings the index up to date after a note is edited in Obsidian (B7.4, task
	// 7.8). It stops with the daemon's own context, so nothing sweeps a vault after the store closes.
	mem.StartVaultWatch(ctx)
	// Search reads the projects, their cards, their chats, their past sessions, and their notes
	// through the services that own them, never their tables: the memory module answers the two
	// kinds that are searched through a full-text index rather than read whole.
	finder, err := search.New(search.Deps{Projects: core.proj, Chats: projectChats, Sessions: mem, Notes: mem})
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the search module: %w", err)
	}
	// The accounts service owns the person's profile, avatar, progress, and preferences. It asks the
	// projects module only whether a project and a saved view are there.
	you, err := accounts.New(accounts.Deps{Store: st, Bus: bus, Projects: core.proj, DataDir: settings.DataDir},
		accounts.WithLogger(log))
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the accounts module: %w", err)
	}
	// The audit log's read service backs the read-only list, search, and export routes (B3.5). The
	// rows themselves are written by internal/audit as things happen; this only reads them back.
	auditRead, err := auditlog.New(auditlog.Deps{Store: st}, auditlog.WithLogger(log))
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the audit log module: %w", err)
	}
	// The roles module owns Marshal's role templates and a project's overrides of them (B5.1, N18).
	// It needs only the store. Its starter roles are added by EnsureStarters, which the caller runs
	// before anything is served; adding one that is already there does nothing.
	roleSvc, err := roles.New(roles.Deps{Store: st}, roles.WithLogger(log))
	if err != nil {
		return cardDaemonModules{}, fmt.Errorf("start the roles module: %w", err)
	}
	// The roles module is the session manager's reader of a card's role ceilings, so the harness can
	// stop a card that passes the role it runs under (B5.3, build-plan 5.3). It is set here rather
	// than handed to NewManager because it is built after the manager; the manager reads it when a
	// card's turn ends, so a card already running picks it up.
	core.sessions.SetRoleLimits(roleSvc)
	// The automatic sleep's two readers (B5.6): the settings above, which decide the idle time, the
	// warning, and the keep-awake length, and the cost limits service, which answers a project's
	// awake ceiling. Both are set here rather than handed to NewManager because both are built
	// after it; the manager reads them on each sweep, so a setting a person saves is in force on
	// the next pass with no restart.
	core.sessions.SetSleepSettings(core.sleepSettingsSvc)
	core.sessions.SetAwakeLimits(core.costLimits)
	return cardDaemonModules{
		home: home, homeSub: homeSub, cards: cards, cardDiff: cardDiff, projectChats: projectChats,
		mem: mem, finder: finder, you: you, auditRead: auditRead, roleSvc: roleSvc,
	}, nil
}

// lateDaemonModules is the GitHub-backed and card-quality services built once a card can already
// be read, diffed, and remembered - buildModules' third and last stage.
type lateDaemonModules struct {
	pullReq        *pullrequest.Service
	reviewSvc      *review.Service
	mergeQueue     *integrator.Service
	qualitySvc     *quality.Service
	integrationSvc *integrations.Service
	trelloOutbound *integrations.OutboundSync
	gmailPoller    *integrations.GmailPoller
	ciSvc          *ci.Service
	localCISvc     *localci.Service
	previewSvc     *preview.Service
	codeMap        *codemap.Map
	mcpHost        *mcpserver.Host
	schedSvc       *schedules.Service
}

// buildLateModules makes the GitHub pull-request and review services, the merge queue, the
// code-smell gate, the GitHub App and Obsidian connections, the CI monitor, local CI, live
// preview, the codebase map, and the internal MCP server - and gives the session manager the
// attacher and awareness reader it needs once they exist.
func buildLateModules(st *store.Store, bus *events.Bus, settings config.Settings, log *slog.Logger, core coreDaemonModules, cm cardDaemonModules) (lateDaemonModules, error) {
	// The pull-request service opens a card's branch as a real pull request on GitHub (B5.4,
	// build-plan 5.6), and the review service runs the Reviewer role over every pull request that
	// is opened (B5.4, build-plan 5.7). Both are built only when a GitHub token has been saved in
	// the keychain; with no token they are left out, so POST /v1/cards/{id}/pull-request and
	// POST /v1/cards/{id}/review do not exist and nothing is pretended. Phase 6 adds the GitHub App
	// and the screen that saves the token.
	pullReq, reviewSvc, err := buildForge(settings, core.proj, core.git, cm.roleSvc, core.sessions, log)
	if err != nil {
		return lateDaemonModules{}, err
	}
	// The merge queue (B5.5, build-plan 5.8). Its tests are nil for now: a clean merge with no
	// test runner moves the target forward, and Phase 6's local CI supplies the runner that makes
	// the queue wait for a real test run.
	mergeQueue, err := integrator.New(integrator.Deps{
		Cards: core.proj, Projects: core.proj, Git: core.git, DataDir: settings.DataDir, Log: log,
	})
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the merge queue: %w", err)
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
		Store: st, Cards: core.proj, Projects: core.proj, Git: core.git, Worker: core.sessions, Bus: bus, Log: log,
	})
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the code-smell module: %w", err)
	}
	core.proj.SetReviewGate(qualitySvc)
	// The connections Marshal is set up with apart from model providers (B6.1, B6.7, B7.4,
	// architecture.md section 18): the GitHub App today, and the Obsidian vault, which is a folder
	// rather than a service. It owns the GitHub delivery receiver the webhook route hands deliveries
	// to, and it reads the webhook secret from the keychain on every delivery, so saving the App in
	// Settings takes effect without a restart - which is why the receiver exists even on a daemon
	// nobody has connected GitHub to. Until then every delivery is refused. The vault root is handed
	// in from the memory module, so the Obsidian row and the test name the one folder Marshal writes
	// memory in rather than a second copy of the same path.
	gcalRedirect := fmt.Sprintf("http://%s/v1/integrations/gcal/callback", loopbackAddress(settings.Port))
	integrationSvc, err := integrations.New(st, security.NewOSKeychain(platform.AppName(settings.Mode)),
		integrations.Options{
			Logger: log, Now: time.Now, VaultRoot: cm.mem.Root(), GCalRedirectURL: gcalRedirect,
		})
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the integrations module: %w", err)
	}
	// The CI monitor (B6.2, B6.3, architecture.md section 9) watches the GitHub App's workflow runs.
	// Its forge is an adapter rather than a client, because the App is saved and removed while the
	// daemon runs: every call asks the connections service for the App as it is now, and answers
	// ci.ErrNoForge while nobody has connected GitHub, which is what makes the monitor harmless on a
	// machine that has never signed in.
	ciSvc, err := ci.New(ci.Deps{
		Store: st, Cards: core.proj, Projects: core.proj, Git: core.git, Roles: cm.roleSvc,
		Worker: core.sessions, Bus: bus, Audit: core.auditRec,
		Options: ci.Options{Logger: log, Now: time.Now, Forge: appForge{svc: integrationSvc}},
	})
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the CI monitor: %w", err)
	}
	// Every verified delivery the receiver accepts reaches the monitor, which is what turns a
	// replayed webhook into a card's CI state.
	integrationSvc.SetMonitor(ciSvc)
	// The board work a believed Trello delivery is applied through (B8.2, architecture.md section
	// 19.4): the projects service makes and moves the cards, adapted here so the connections package
	// never depends on the board module. Without it a believed delivery is logged and nothing else,
	// which is why attaching it is not optional in a real daemon.
	integrationSvc.SetTrelloCards(trelloCards{svc: core.proj})
	// The Marshal-to-Trello half of the move sync (B8.2): watches card.moved on the event bus and
	// mirrors a card reaching done onto its linked Trello card, once Start runs (main.go).
	trelloOutbound, err := integrations.NewOutboundSync(integrationSvc, bus)
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the Trello outbound sync: %w", err)
	}
	// Gmail's inbound half (B8.3): a labeled message becomes a card, on a timer (docs/marshal-
	// product-scope.md 19.1's "cheap polling as a backup" - Gmail has no webhook Marshal can take).
	gmailPoller, err := integrations.NewGmailPoller(integrationSvc)
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the Gmail poller: %w", err)
	}
	// Local CI (B6.5, build-plan 6.5): the project's own workflow files, run in the card's worktree.
	// It reads the worktree through the projects service and starts each step through internal/proc,
	// so nothing here needs a forge, a token, or a network.
	localCISvc, err := localci.New(localci.Deps{Projects: core.proj, Log: log, Now: time.Now})
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the local CI module: %w", err)
	}
	// Live preview (B6.6, build-plan 6.6 and 6.7): one dev server per card, on a port picked for it,
	// in the card's own worktree, with the before and after screenshots kept under the daemon's data
	// folder. Every seam is left at its real default - the command runner, the browser lookup, the
	// port picker, the prober, and the chromedp shooter - so pressing Start in the Preview tab runs
	// the project's own dev command, and pressing Take screenshot drives the person's own Chrome or
	// Edge. Nothing is started here: the service starts a dev server only when it is asked to.
	previewSvc, err := preview.New(preview.Deps{
		Projects: core.proj, Bus: bus, DataDir: settings.DataDir, Log: log, Now: time.Now,
	})
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the preview module: %w", err)
	}
	// The codebase map (B7.5, build-plan task 7.9): a light index of each project's files and
	// symbols, so an agent asks where a name is instead of reading the tree to find it. Symbols come
	// from universal ctags when the machine has it, and from file names alone when it does not
	// (internal/codemap says so in every answer).
	codeMap, err := codemap.New(codemap.Deps{
		Roots: func(ctx context.Context, projectID string) (string, error) {
			project, err := core.proj.Get(ctx, projectID)
			if err != nil {
				return "", err
			}
			return project.Path, nil
		},
		Logger: log,
	})
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the codebase map: %w", err)
	}
	// The internal MCP server (B7.1, architecture.md section 11.4): one server per live card, served
	// over the daemon's own listener and reached by that card's own agent through `marshald mcp`. The
	// host holds the servers; the attacher builds one when a session starts and gives it up when the
	// session ends (internal/session's Attacher).
	mcpHost := mcpserver.NewHost(mcpserver.WithHostLogger(log))
	attacher, err := mcpattach.New(mcpattach.Deps{
		Host: mcpHost, Cards: core.proj, Notes: cm.mem, Claims: cm.mem, Agents: core.sessions,
		Codebase: codeMap, Roles: cm.roleSvc, Harness: core.sessions.HarnessConfigFor,
		Command: daemonExecutable(log), Address: loopbackAddress(settings.Port),
		Logger: log, Now: time.Now,
	})
	if err != nil {
		return lateDaemonModules{}, fmt.Errorf("start the internal MCP server: %w", err)
	}
	core.sessions.SetAttacher(attacher)
	// The same module builds the board-awareness summary a card's agent is given at the start of
	// every turn (internal/session's Awareness). It is a seam of its own rather than a third method
	// on the attacher because a daemon that could not resolve its own executable still gives its
	// sessions the summary, and because it is read per turn rather than once (session/aware.go).
	core.sessions.SetAwareness(attacher)
	// The scheduler (B8.1, build-plan 8.1): the scheduled jobs and briefs, and the cron that runs
	// them. It needs only the store - the rows are what it runs - and it is built here so that the
	// routes that edit a schedule and the cron that runs one are the same object.
	schedSvc := schedules.NewService(st, log)
	// Briefs (B8.5, build-plan task 8.7): gathered from the same projects service every card route
	// reads, never a query of its own. Registered under the schedule's Kind, not its Action -
	// every brief's Action is a person's own free-text sentence describing what happens, not a
	// dispatchable key (executeSchedule's fallback, internal/schedules/service.go).
	schedSvc.RegisterHandler("brief", briefs.New(core.proj).Handle)
	return lateDaemonModules{
		pullReq: pullReq, reviewSvc: reviewSvc, mergeQueue: mergeQueue, qualitySvc: qualitySvc,
		integrationSvc: integrationSvc, trelloOutbound: trelloOutbound, gmailPoller: gmailPoller,
		ciSvc: ciSvc, localCISvc: localCISvc, previewSvc: previewSvc,
		codeMap: codeMap, mcpHost: mcpHost, schedSvc: schedSvc,
	}, nil
}

// trelloCards is the board work the Trello sync writes through, answered by the projects service.
// It exists because the two signatures differ in a way Go will not adapt: the projects service's
// CreateCard takes its own options variadically, so it does not structurally satisfy the sync's
// interface, and an adapter is clearer than changing either side for the other's convenience.
type trelloCards struct {
	svc *projects.Service
}

func (t trelloCards) CreateCard(ctx context.Context, projectID string, in protocol.CreateCardRequest) (protocol.Card, error) {
	return t.svc.CreateCard(ctx, projectID, in)
}

func (t trelloCards) MoveCard(ctx context.Context, id string, in protocol.MoveCardRequest) (protocol.Card, error) {
	return t.svc.MoveCard(ctx, id, in)
}

func (t trelloCards) Card(ctx context.Context, id string) (protocol.Card, error) {
	return t.svc.Card(ctx, id)
}

// buildModules makes git, projects, the agent registry and catalog, and the session manager, in
// the order each depends on the last, in three stages kept as separate functions so each stays
// readable on its own. ctx is the daemon's own context, which the long-lived watchers it starts -
// the vault watcher - stop with.
func buildModules(ctx context.Context, st *store.Store, bus *events.Bus, settings config.Settings, env platform.Env, log *slog.Logger) (daemonModules, error) {
	core, err := buildCoreModules(st, bus, settings, env, log)
	if err != nil {
		return daemonModules{}, err
	}
	cm, err := buildCardModules(ctx, st, bus, settings, log, core)
	if err != nil {
		return daemonModules{}, err
	}
	late, err := buildLateModules(st, bus, settings, log, core, cm)
	if err != nil {
		return daemonModules{}, err
	}
	return daemonModules{
		proj: core.proj, sessions: core.sessions, catalog: core.catalogSrc, dashboard: cm.home, homeSub: cm.homeSub,
		history: cm.cards, diff: cm.cardDiff, chats: cm.projectChats, search: cm.finder, accounts: cm.you,
		auditlog: cm.auditRead, providers: core.providerSvc, costLimits: core.costLimits,
		connectionTests: core.connectionTests, roles: cm.roleSvc, pullRequests: late.pullReq,
		review: late.reviewSvc, integrator: late.mergeQueue, sleepSettings: core.sleepSettingsSvc,
		quality: late.qualitySvc, integrations: late.integrationSvc, trelloOutbound: late.trelloOutbound,
		gmailPoller: late.gmailPoller,
		ci:          late.ciSvc, localCI: late.localCISvc,
		preview: late.previewSvc, memory: cm.mem, mcpHost: late.mcpHost, codemap: late.codeMap,
		schedules: late.schedSvc,
	}, nil
}
