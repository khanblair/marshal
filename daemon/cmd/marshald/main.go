// Command marshald is the Marshal daemon. It owns all state and does all the work, and keeps
// running when the app is closed.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/acp"
	"github.com/khanblair/marshal/daemon/internal/agents/builtin"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/agents/claude"
	"github.com/khanblair/marshal/daemon/internal/agents/gemini"
	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/chatcmd"
	"github.com/khanblair/marshal/daemon/internal/ci"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/devices"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/fixture"
	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/notify"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/pullrequest"
	"github.com/khanblair/marshal/daemon/internal/review"
	"github.com/khanblair/marshal/daemon/internal/roles"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/session"
	sleepsettings "github.com/khanblair/marshal/daemon/internal/settings"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/tailnet"
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
	// `mcp` is the same binary in another mode: the stdio server an agent runs to reach the daemon's
	// tools (mcp.go). It is handled before the daemon's own flags, because everything below opens the
	// data folder, takes the run lock, and serves - none of which a short-lived forwarder does.
	if len(args) > 0 && args[0] == "mcp" {
		return runMCP(args[1:], stdout, stderr)
	}
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
	state, err := openDaemonState(ctx, settings, log)
	if err != nil {
		return err
	}
	settings, st, bus := state.settings, state.store, state.bus
	defer func() {
		if closeErr := state.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	mods, err := buildModules(ctx, st, bus, settings, env, log)
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
	// The scheduler, the Marshal-to-Trello move sync, and the Gmail poller (B8.1-B8.3): started
	// together after every module a run can reach exists, stopped together in reverse order before
	// the session manager, the bus, and the store.
	stopAutomation, err := startAutomationWatchers(ctx, mods)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stopAutomation(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	// The paired-devices service (B9.1, B9.2): the code a phone types in, the device rows, and
	// revoking one. It needs only the store and the clock, so it is built here beside the accounts
	// it pairs for rather than in buildModules.
	pairedDevices := devices.NewService(st, devices.WithLogger(log), devices.WithClock(time.Now)) // The tailnet node (B9.1): a second, additive listener beside the loopback one, off unless it
	// was asked for. It is built before the server so the server can serve on it, and closed after
	// the server has stopped accepting - see the defer just below.
	node := buildTailnetNode(ctx, settings, st, log)
	defer closeTailnet(node, &err)

	// The notification router (B9.4): which chat each kind of event reaches, grouped so a burst of
	// things is one message, and an approval sent at once. It talks to no service itself - the
	// connections service builds the bot for the channel - so a daemon with no chat connected routes
	// to nowhere rather than failing, and it runs for as long as the daemon does.
	alerts := buildNotifications(ctx, log, mods.integrations, bus, mods.sleepSettings, linkBase(node, settings.Port))

	// The chat receive loops (B9.3): Telegram and Discord can answer an approval the same way a
	// screen does. They start the first time a connection for them is saved, so a person who adds a
	// bot from Settings does not have to restart the daemon, and each one runs for as long as it does.
	if err := startChatCommands(ctx, log, mods); err != nil {
		return err
	}

	dev, err := api.EnsureAccounts(ctx, st, api.AccountsConfig{
		DataDir: settings.DataDir, Dev: settings.Dev(), Now: time.Now, Log: log,
	})
	if err != nil {
		return err
	}
	// The fixture loads before the restore below starts, and before Run serves anything, so the
	// restore and the first requests never see a half-made fixture.
	startCardWork(ctx, settings, mods, log)
	// A nil node must stay a nil interface: a typed nil would look like a node the server can call.
	var tailnetNode api.TailnetNode
	if node != nil {
		tailnetNode = node
	}
	deps := moduleDeps(mods)
	deps.Store, deps.Bus, deps.Dev, deps.Alerts = st, bus, dev, alerts
	deps.Devices, deps.Tailnet, deps.Funnel, deps.WebUI = pairedDevices, tailnetNode, settings.Funnel, webUIFS()
	return api.New(settings, log, time.Now, deps).Run(ctx)
}

// moduleDeps is the part of the server's dependencies that comes from the modules built at start.
// Every module the server has a route group for belongs here: a module left out leaves its routes
// unregistered, and they answer "nothing at that address" with no other sign (the card notes did).
func moduleDeps(mods daemonModules) api.Deps {
	return api.Deps{
		Projects: mods.proj, Sessions: mods.sessions, Catalog: mods.catalog,
		Dashboard: mods.dashboard, History: mods.history, Diff: mods.diff, Chats: mods.chats,
		Search: mods.search, Memory: mods.memory, Accounts: mods.accounts, Auditlog: mods.auditlog,
		Providers: mods.providers, CostLimits: mods.costLimits, ConnectionTests: mods.connectionTests,
		Roles: mods.roles, PullRequests: mods.pullRequests, Review: mods.review,
		Integrator: mods.integrator, SleepSettings: mods.sleepSettings, Quality: mods.quality,
		Integrations: mods.integrations, Webhooks: mods.integrations.Receiver(), CI: mods.ci,
		LocalCI: mods.localCI, CardPanel: mods.cardPanel, Preview: mods.preview,
		Schedules: mods.schedules, MCP: mods.mcpHost,
	}
}

// acquireDaemonLock takes the run lock for a data folder. It logs the one failure serve treats as
// special - another daemon already holds it - before answering it, since every other failure to
// start is already visible in the error serve returns.
func acquireDaemonLock(dataDir string, log *slog.Logger) (*platform.Lock, error) {
	lock, err := platform.AcquireLock(filepath.Join(dataDir, lockFileName))
	if err != nil {
		if errors.Is(err, platform.ErrAlreadyRunning) {
			log.Error("cannot start: another daemon already holds the lock", "data_dir", dataDir, "error", err)
		}
		return nil, err
	}
	return lock, nil
}

// daemonState is what serve opens before building any module: the data folder resolved to an
// absolute path, the run lock that folder is held under, the database, and the event bus. Close
// releases them in the reverse order Open acquired them, joining every failure into one error the
// way serve's own defers used to.
type daemonState struct {
	settings config.Settings
	lock     *platform.Lock
	store    *store.Store
	bus      *events.Bus
}

// openDaemonState resolves the data folder, makes it if it is missing, and opens the lock, the
// database, and the event bus in that order - the order a daemon that lost the race for this data
// folder, or whose database will not open, must touch nothing further for. A step that fails closes
// whatever the steps before it opened before answering the error.
func openDaemonState(ctx context.Context, settings config.Settings, log *slog.Logger) (*daemonState, error) {
	dataDir, err := filepath.Abs(settings.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve the data folder %s: %w", settings.DataDir, err)
	}
	settings.DataDir = dataDir
	if err := os.MkdirAll(settings.DataDir, dataDirMode); err != nil {
		return nil, fmt.Errorf("make the data folder %s: %w", settings.DataDir, err)
	}
	lock, err := acquireDaemonLock(settings.DataDir, log)
	if err != nil {
		return nil, err
	}
	log.Info("starting", "version", buildinfo.Version, "mode", settings.Mode, "data_dir", settings.DataDir)
	st, err := store.Open(ctx, filepath.Join(settings.DataDir, databaseFile), store.WithLogger(log))
	if err != nil {
		_ = lock.Release()
		return nil, fmt.Errorf("open the database: %w", err)
	}
	bus, err := events.New()
	if err != nil {
		_ = st.Close()
		_ = lock.Release()
		return nil, fmt.Errorf("start the event bus: %w", err)
	}
	return &daemonState{settings: settings, lock: lock, store: st, bus: bus}, nil
}

// Close releases a daemonState's lock, database, and event bus, in the reverse order Open acquired
// them: the bus first, then the database, then the lock, joining every failure into one error.
func (d *daemonState) Close() error {
	var err error
	d.bus.Close()
	if closeErr := d.store.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close the database: %w", closeErr))
	}
	if releaseErr := d.lock.Release(); releaseErr != nil {
		err = errors.Join(err, fmt.Errorf("release the lock: %w", releaseErr))
	}
	return err
}

// startAutomationWatchers starts the scheduler, the Marshal-to-Trello move sync, and the Gmail
// poller (B8.1-B8.3), in that order, and returns how to stop all three in reverse order. The
// scheduler is the one of the three whose own rows can fire immediately once it starts, so it
// starts first and stops last, the same "last started, first stopped" order the modules it depends
// on already follow.
func startAutomationWatchers(ctx context.Context, mods daemonModules) (func() error, error) {
	if err := mods.schedules.Start(ctx); err != nil {
		return nil, fmt.Errorf("start the scheduler: %w", err)
	}
	if err := mods.trelloOutbound.Start(ctx); err != nil {
		mods.schedules.Stop()
		return nil, fmt.Errorf("start the Trello outbound sync: %w", err)
	}
	mods.gmailPoller.Start(ctx)
	return func() error {
		var stopErr error
		if closeErr := mods.gmailPoller.Close(); closeErr != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("stop the Gmail poller: %w", closeErr))
		}
		if closeErr := mods.trelloOutbound.Close(); closeErr != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("stop the Trello outbound sync: %w", closeErr))
		}
		mods.schedules.Stop()
		return stopErr
	}, nil
}

// startChatCommands starts the chat command service and its receive-loop watcher (B9.3), when the
// daemon has a session manager to run a command against. A daemon built in stub mode still has one,
// so this only ever skips when sessions itself failed to build.
func startChatCommands(ctx context.Context, log *slog.Logger, mods daemonModules) error {
	if mods.sessions == nil {
		return nil
	}
	commands, err := chatcmd.New(mods.sessions, log)
	if err != nil {
		return fmt.Errorf("start chat commands: %w", err)
	}
	if mods.proj != nil {
		commands.WithCards(chatCards{svc: mods.proj})
	}
	go watchChats(ctx, log, mods.integrations, commands)
	return nil
}

// startCardWork loads the fixture named by --fixture or MARSHAL_FIXTURE, or, when none was asked
// for, restores the sessions left over from the last run (docs/architecture.md 5.3). A fixture's
// sessions have no process behind them, so a loaded fixture leaves them for a person to resume by
// hand instead of restoring them.
func startCardWork(ctx context.Context, settings config.Settings, mods daemonModules, log *slog.Logger) {
	if loadFixture(ctx, settings, mods.proj, mods.sessions, log) {
		log.Info("the fixture is loaded, so its sessions are left for a person to resume")
		return
	}
	// Restoring sessions runs in its own goroutine, bounded by its own timeout, so a slow or
	// stuck agent program never delays Run below from serving the health endpoint.
	go restoreSessions(ctx, mods.sessions, log)
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

// tailnetStateDir is where the node's own Tailscale state lives: one folder under the daemon's
// data folder, so signing in once survives a restart and nothing is written outside the data
// folder the daemon owns.
const tailnetStateDir = "tailnet"

// buildTailnetNode makes the node the daemon joins, or nil for a daemon that was not asked to join
// a tailnet - which is every daemon until someone switches it on. The node is made here rather than
// in serve so that serve stays a list of what it opens and closes rather than a list of how.
//
// Starting the identity watcher is part of making the node: it reads the node's own status until
// the daemon stops, so it has to begin before the server starts serving on the tailnet.
func buildTailnetNode(ctx context.Context, settings config.Settings, st *store.Store, log *slog.Logger) *tailnet.Node {
	if !settings.Tailnet {
		return nil
	}
	node := tailnet.New(tailnet.Config{
		Dir:      filepath.Join(settings.DataDir, tailnetStateDir),
		Hostname: settings.TailnetHostname,
		Logger:   log,
	})
	go watchTailnetIdentity(ctx, node, st, log)
	return node
}

// closeTailnet closes the node after the server has stopped serving on it, and joins any failure
// into the daemon's own error, the way every other close in serve does. A daemon that was never
// asked to join a tailnet has no node and closes nothing.
func closeTailnet(node *tailnet.Node, join *error) {
	if node == nil {
		return
	}
	if err := node.Close(); err != nil {
		*join = errors.Join(*join, fmt.Errorf("close the tailnet node: %w", err))
	}
}

// tailnetStatusSource is what the identity watcher reads: the node's own view of itself. It is
// an interface so the watcher can be driven by a test without a node that can join anything.
type tailnetStatusSource interface {
	Status() protocol.TailnetStatus
}

// watchTailnetIdentity writes the Tailscale account the daemon's node joined as into the owner's
// row, so the profile's Tailscale identity (B9.1) comes from the node itself. It reads the node's
// status rather than waiting on the join, because a node that nobody has signed in to has no
// identity yet and a daemon that never joins has nothing to write. It stops with the daemon.
func watchTailnetIdentity(ctx context.Context, node tailnetStatusSource, st *store.Store, log *slog.Logger) {
	owner, err := st.Queries().GetOwner(ctx)
	if err != nil {
		log.Warn("could not find the owner to record a Tailscale identity for", "error", err)
		return
	}
	ticker := time.NewTicker(tailnetIdentityInterval)
	defer ticker.Stop()
	for {
		if identity := node.Status().Identity; identity != "" && identity != owner.TailnetIdentity {
			if err := st.SetTailnetIdentity(ctx, owner.ID, identity); err != nil {
				log.Warn("could not record the Tailscale account this machine joined as", "error", err)
			} else {
				owner.TailnetIdentity = identity
				log.Info("recorded the Tailscale account", "identity", identity)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// tailnetIdentityInterval is how often the node is asked whether it has signed in yet.
const tailnetIdentityInterval = 10 * time.Second

// buildNotifications is the notification router (B9.4, build-plan 9.7), built once and running
// against the event bus until the daemon stops.
//
// It sends through whichever chat connection a person saved. The router itself knows nothing about
// Telegram or Discord: the connections service builds the bot for the channel, and a chat nobody has
// connected means the notice goes nowhere, which is a logged nothing rather than an error - so a
// daemon with no chat set up still serves the app exactly as it did before.
func buildNotifications(
	ctx context.Context, log *slog.Logger, links *integrations.Service, bus *events.Bus,
	saved notify.RouteStore, base func() string,
) api.AlertSettings {
	sender := notify.SenderFunc(func(callCtx context.Context, channel notify.Channel, notice chatbot.Notice) error {
		if links == nil {
			return nil
		}
		bot, err := links.ChatBot(callCtx, chatbot.Kind(channel))
		if err != nil {
			if errors.Is(err, integrations.ErrNotConnected) {
				return nil
			}
			return err
		}
		defer func() { _ = bot.Close() }()
		return bot.Notify(callCtx, notice)
	})
	router, err := notify.New(sender, notify.Options{Logger: log, LinkBase: base})
	if err != nil {
		// notify.New only ever refuses a nil sender, and this one is not nil.
		log.Warn("the notification router could not be built", "error", err)
		return nil
	}
	go router.Follow(ctx, bus)
	return alertSettings(ctx, log, router, links, saved)
}

// alertSettings makes the router's routing table something a screen can read and change, and
// applies the choices saved before the last restart. A daemon with nowhere to keep choices still
// routes with the defaults, so it answers no settings rather than failing to start.
func alertSettings(
	ctx context.Context, log *slog.Logger, router *notify.Service, links *integrations.Service, saved notify.RouteStore,
) api.AlertSettings {
	alerts, err := notify.NewAlerts(router, saved, func(callCtx context.Context) map[notify.Channel]bool {
		return connectedChannels(callCtx, links)
	}, time.Now)
	if err != nil {
		log.Warn("the alert settings could not be built", "error", err)
		return nil
	}
	if err := alerts.Load(ctx); err != nil {
		log.Warn("the saved alert choices could not be read, so the defaults stand", "error", err)
	}
	return alerts
}

// connectedChannels says which alert channels have a connection that is set up.
func connectedChannels(ctx context.Context, links *integrations.Service) map[notify.Channel]bool {
	up := map[notify.Channel]bool{}
	if links == nil {
		return up
	}
	list, err := links.List(ctx)
	if err != nil {
		return up
	}
	for _, row := range list {
		switch row.ID {
		case integrations.TelegramID, integrations.DiscordID, integrations.NtfyID:
			up[notify.Channel(row.ID)] = row.Status == protocol.IntegrationStatusConnected
		}
	}
	return up
}

// linkBase is the address a phone opens Marshal at: this node's MagicDNS name on the daemon's port,
// once the node is online. Before that, or on a daemon with no tailnet, it is empty and a notice
// carries no link to a card, which is better than one that cannot open.
func linkBase(node *tailnet.Node, port int) func() string {
	return func() string {
		if node == nil {
			return ""
		}
		status := node.Status()
		if status.State != tailnet.StateOnline || status.DNSName == "" {
			return ""
		}
		return "http://" + net.JoinHostPort(status.DNSName, strconv.Itoa(port))
	}
}

// chatRecheckInterval is how often the daemon looks for a chat connection that has been set up
// since it last looked. Nothing announces a connection as "ready for a receive loop", so a watcher
// is what turns a token pasted in Settings into a bot that starts answering.
const chatRecheckInterval = 10 * time.Second

// watchChats starts each chat bot's receive loop the first time a connection for it is saved, and
// leaves it running (B9.3, build-plan 9.5 and 9.6).
//
// It is a watcher rather than one call at start-up because a bot is most often connected from
// Settings after the daemon is already up, and a person who has just pasted a token should not have
// to restart anything for it to work. Each kind is started at most once: the loop the bot started
// keeps running, and a second bot of the same kind would answer the same messages twice.
func watchChats(ctx context.Context, log *slog.Logger, links *integrations.Service, commands *chatcmd.Service) {
	if links == nil {
		return
	}
	started := map[chatbot.Kind]bool{}
	for {
		for _, kind := range []chatbot.Kind{chatbot.KindTelegram, chatbot.KindDiscord} {
			if started[kind] {
				continue
			}
			bot, err := links.ChatBot(ctx, kind)
			if err != nil {
				// No connection saved for this service yet, which is the ordinary state of a
				// daemon nobody has set a bot up on. Nothing is wrong and nothing is logged.
				continue
			}
			started[kind] = true
			log.Info("chat commands are on", "service", kind)
			go func(bot chatbot.Bot) {
				if err := commands.Run(ctx, bot); err != nil && ctx.Err() == nil {
					log.Warn("a chat bot stopped receiving", "service", bot.Kind(), "error", err)
				}
			}(bot)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(chatRecheckInterval):
		}
	}
}

// vaultRoot is where the memory module keeps a person's knowledge base: one `vault` folder under the
// daemon's data folder (docs/architecture.md section 12).
func vaultRoot(dataDir string) string {
	return filepath.Join(dataDir, "vault")
}

// loopbackAddress is where the daemon is reached on this machine: the address its own `mcp` mode and
// the agent it serves connect back to.
func loopbackAddress(port int) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

// daemonExecutable is the absolute path of the running daemon, which is the command an agent runs as
// `mcp`. A daemon that cannot resolve its own path logs it and serves no tools rather than failing
// to start: everything else it does still works.
func daemonExecutable(log *slog.Logger) string {
	exe, err := os.Executable()
	if err != nil {
		log.Warn("could not resolve the daemon's own path, so no agent is given the internal tools", "error", err)
		return ""
	}
	return exe
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

// lateMemory lets the projects service reach the memory module, which is built after it because the
// module reads a project's cards through the projects service. It does nothing until the module is
// set, which means a project removed before then leaves its memory folder alone - a folder the
// person can delete themselves, where deleting it on a guess is not something to do.
type lateMemory struct {
	svc atomic.Pointer[memory.Service]
}

func (l *lateMemory) RemoveProjectMemory(ctx context.Context, projectID string) error {
	if s := l.svc.Load(); s != nil {
		return s.RemoveProjectMemory(ctx, projectID)
	}
	return nil
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

// chatCards is the card work a chat can ask for, answered by the projects service. It is an adapter
// for the same reason trelloCards is: the two signatures do not meet without one.
type chatCards struct {
	svc *projects.Service
}

func (c chatCards) Projects(ctx context.Context) ([]chatcmd.Project, error) {
	list, err := c.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]chatcmd.Project, len(list.Projects))
	for i, project := range list.Projects {
		out[i] = chatcmd.Project{ID: project.ID, Name: project.Name}
	}
	return out, nil
}

func (c chatCards) CreateCard(ctx context.Context, projectID string, in protocol.CreateCardRequest) (protocol.Card, error) {
	return c.svc.CreateCard(ctx, projectID, in)
}
