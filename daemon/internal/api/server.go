// Package api serves the HTTP and WebSocket API under /v1. It listens on the local machine only.
package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/errgroup"

	"github.com/khanblair/marshal/daemon/internal/accounts"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/auditlog"
	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/cardhistory"
	"github.com/khanblair/marshal/daemon/internal/cardpanel"
	"github.com/khanblair/marshal/daemon/internal/chats"
	ci "github.com/khanblair/marshal/daemon/internal/ci"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/connectiontest"
	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/devices"
	"github.com/khanblair/marshal/daemon/internal/diff"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/mcpserver"
	"github.com/khanblair/marshal/daemon/internal/memory"
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
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/settings"
	"github.com/khanblair/marshal/daemon/internal/store"
)

const (
	// loopback is the address the daemon always listens on: this machine, and only this machine.
	// The tailnet listener (Deps.Tailnet) is a second, additive one; nothing ever listens on every
	// interface (docs/architecture.md section 13).
	loopback          = "127.0.0.1"
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// Deps is what the server needs from the rest of the daemon. Every field is optional: a route
// whose dependency is missing is not registered, so a server with less than everything still
// starts, and a test wires only what it uses. It grows as modules are added.
type Deps struct {
	// Store holds the devices that tokens are checked against. Without it no route needs a token,
	// so the server has only the public routes.
	Store *store.Store
	// Bus feeds the event stream. Without it there is no /v1/events.
	Bus *events.Bus
	// Dev makes a dev daemon accept its dev token from this machine. A normal daemon ignores it.
	Dev *DevAccess
	// Projects manages projects, boards, and cards. When it is set, the routes for projects, boards,
	// and cards are registered, and so is starting a card when Sessions is set as well.
	Projects *projects.Service
	// Sessions starts, sends to, stops, and resumes a card's agent session. When it is set, the
	// routes to send a message to a card, stop it, and resume it are registered.
	Sessions *session.Manager
	// Catalog lists the agents this daemon can start: the real Catalog in real mode, the Stub in
	// dev mode. When it is set, the routes to list and refresh the agents are registered.
	Catalog catalog.Source
	// Dashboard builds Home's answer. When it is set, the routes that read Home are registered.
	Dashboard *dashboard.Service
	// History serves a card's chat and its activity. When it is set, the routes that page a card's
	// chat, page its activity, and read one message's tool detail are registered.
	History *cardhistory.Service
	// Diff serves a card's changed files with their counts, and one file's hunks on demand. When
	// it is set, the two routes that draw the card's Diff tab are registered.
	Diff *diff.Service
	// Chats serves a project's chats: listing them, creating one, renaming one, archiving and
	// restoring one, and deleting one. When it is set, and Projects is too, the chat routes are
	// registered.
	Chats *chats.Service
	// Search answers the command palette's search over projects, cards, chats, past sessions, and
	// notes. When it is set, the search route is registered. It answers the session and note kinds
	// through the memory module, which is wired into it where the daemon is built.
	Search *search.Service
	// Memory owns the vault: a card's note, the files it claims, and the search over notes and past
	// sessions (B7.2, B7.4, build-plan tasks 7.3, 7.6, and 7.12). When it is set, and Projects is
	// too, the two routes that read and write a card's note are registered. The internal MCP
	// server's tools reach the same module, which is wired where the daemon is built.
	Memory *memory.Service
	// Accounts serves the person's profile, avatar, progress, and preferences, and the users list.
	// When it is set, those routes are registered, and so is the dev-only reset of first-launch
	// progress on a dev daemon.
	Accounts *accounts.Service
	// Auditlog reads the audit log back for the read-only list, search, and export routes
	// (B3.5). When it is set, those routes are registered.
	Auditlog *auditlog.Service
	// Providers lists the model providers Marshal knows and stores, replaces, and removes their
	// keys (N18). When it is set, those three routes are registered. The key itself is never in an
	// answer: only a masked value is.
	Providers *providers.Service
	// CostLimits reads and writes the cost and awake limits (B4.5). When it is set, those routes are
	// registered. It is separate from Providers because a ceiling needs the store and not the
	// keychain: a daemon sets limits whether or not a key has been saved.
	CostLimits *providers.Limits
	// ConnectionTests runs a connection's test and remembers its result (B4.6, section 18). When it
	// is set, POST /v1/providers/{id}/test is registered, and every provider row carries the last
	// test that was saved for it. It is separate from Providers because the runner belongs to the
	// daemon and not to a provider: later phases' integrations and MCP servers are tested through
	// the same one.
	ConnectionTests *connectiontest.Runner
	// Roles lists Marshal's role templates and adds, edits, deletes, resets, and overrides them
	// (B5.1, N18). When it is set, the role routes are registered. It needs the store and not the
	// keychain: a role is a name and a spec, and nothing in it is a secret.
	Roles *roles.Service
	// PullRequests opens a card's branch as a real pull request on the project's forge (B5.4,
	// build-plan 5.6). When it is set, and Projects is too, POST /v1/cards/{id}/pull-request is
	// registered. It stays unset until a forge token is saved, so on a machine with no GitHub
	// connection the route does not exist at all.
	PullRequests *pullrequest.Service
	// Integrator runs the merge queue (B5.5, build-plan 5.8). When it is set, and Projects is too,
	// POST /v1/cards/{id}/merge is registered: a card in Ready to merge is merged into the
	// project's default branch, one card at a time per project.
	Integrator *integrator.Service
	// Integration is the merge queue as the Integration view reads it. When it is set, the routes
	// that read a project's merge state and press pause, resume, retry, and undo are registered.
	// Never put a typed nil pointer here: leave the field unset instead.
	Integration integrator.Reader
	// Opener shows a card's worktree in the file manager or the editor. Nil means SystemOpener,
	// which starts this machine's own programs. POST /v1/cards/{id}/worktree/open is registered
	// when Projects is set, and answers only requests that come from this machine.
	Opener Opener
	// Review runs the Reviewer role over a card's pull request (B5.4, build-plan 5.7). When it is
	// set, and Projects is too, POST /v1/cards/{id}/review is registered. It stays unset until a
	// forge token is saved, the same as PullRequests, and it is wired as that service's own
	// "after a pull request is opened" step, so every pull request is read without a person asking.
	Review *review.Service
	// SleepSettings reads and writes the settings the automatic sleep is driven by (B5.6, N5): the
	// idle time, the warning time, the keep-awake time, what happens at restart, and where a warning
	// goes. When it is set, GET and PUT /v1/settings/sleep are registered. The notices themselves
	// come from Sessions, which owns the groups, so the notice routes need no service of their own.
	SleepSettings *settings.Service
	// Alerts reads and writes which channel each kind of alert goes to (B9.4). Nil leaves the alert
	// settings routes out.
	Alerts AlertSettings
	// CardPanel serves a card's acceptance checks, checklists, comments with attachments, and members
	// (B10.2, B10.5, B10.6). Nil leaves those routes out.
	CardPanel *cardpanel.Service
	// Quality reads a card's code-smell findings, asks the card's agent to fix one, dismisses one
	// with a reason, and reads and writes a project's smell profile (B5.8, architecture.md section
	// 17). When it is set, and Projects is too, the five quality routes are registered.
	Quality *quality.Service
	// Integrations owns the connections Marshal is set up with apart from model providers: today
	// the GitHub App, in later phases Trello, a calendar, Gmail, Telegram, Discord, and a vault
	// (B6.1, B6.7, section 18). When it is set, GET /v1/integrations and the save, remove, and test
	// routes beneath it are registered.
	Integrations *integrations.Service
	// CI is the CI monitor: it turns a workflow run on a card's branch into a card's CI state and
	// runs the failure loop of section 9 (B6.2, B6.3, B6.4). When it is set, GET /v1/ci is
	// registered, and on a dev daemon so is POST /v1/cards/{id}/ci-failure, the simulated failure.
	// The monitor is also the sink the webhook route's deliveries reach, which is wired where the
	// daemon is built and not here.
	CI *ci.Service
	// Webhooks receives GitHub's deliveries for the App (B6.1, build-plan 6.1). When it is set,
	// POST /hooks/github is registered. It is the one route with no bearer token: GitHub cannot
	// send one, so the delivery is authorized by its own signature instead (the signedWebhook
	// category). A delivery is refused while no webhook secret is saved, so an unconfigured daemon
	// answers 401 to every delivery rather than believing one.
	Webhooks *githubapp.Receiver
	// LocalCI runs a card's own workflow steps on this machine (B6.5, build-plan 6.5). When it is
	// set, and Projects is too, POST /v1/cards/{id}/local-ci is registered. It is a service of its
	// own rather than part of CI because the two know nothing of each other: the CI monitor watches
	// the forge, and this runs a project's own workflow files in a worktree.
	LocalCI *localci.Service
	// Preview runs one dev server per card and takes its before and after screenshots (B6.6,
	// build-plan 6.6 and 6.7, section 11.2). When it is set, and Projects is too, the five preview
	// routes are registered: read a card's preview, start it, stop it, take a screenshot, and serve
	// one. It is a service of its own because a preview is a dev server on the person's machine, not
	// a look at a project.
	Preview *preview.Service
	// Schedules owns the scheduled jobs and briefs and the cron that runs them (B8.1, build-plan
	// 8.1). When it is set, GET /v1/schedules and the routes that add, edit, and delete one are
	// registered. It is a service of its own and not part of Projects because a schedule outlives
	// any one project: a brief spans every project, and one cron runs the whole daemon's work.
	Schedules *schedules.Service
	// MCP is the internal MCP server endpoint, served over the daemon's own listener to the agents
	// of live cards (docs/architecture.md section 11.4). It carries its own per-card authentication,
	// so it is mounted as a stream route and not behind the owner-token check. Nil when the daemon
	// has no memory module, and then no agent is given a server.
	MCP http.Handler
	// WebUI is the built web app (internal/webui.FS()), served for any address that is not a
	// route above. Without it, an address nothing else matches answers not_found, as it always
	// did before this field existed.
	WebUI fs.FS
	// Devices owns the paired clients: the short-lived code that pairs a new device, the device
	// rows, and revoking one (B9.2). When it is set, the list, code, and revoke routes are
	// registered, and POST /v1/devices/pair - the one route that mints a token with none of its
	// own - is registered beside health, with no token check. Nil keeps every device address
	// closed and pairing impossible.
	Devices *devices.Service
	// Tailnet is this daemon's own node on the person's tailnet (B9.1). Nil, which is the
	// default, is a daemon reachable on this machine only: no tailnet listener is opened and no
	// path is opened to the public internet. Set, it is a second listener beside loopback, never
	// instead of it (docs/architecture.md section 13).
	Tailnet TailnetNode
	// Funnel asks Tailscale to expose this daemon to the public internet. Only /hooks/* is served
	// on it, and every request there is still signature-verified the way Phase 8 built it. It does
	// nothing without a Tailnet: there is no node to expose.
	Funnel bool
	// Limits are the sizes and times. A zero field takes its default.
	Limits Limits
}

// Server is the daemon's HTTP server.
type Server struct {
	settings config.Settings
	log      *slog.Logger
	now      func() time.Time
	limits   Limits
	auth     *authenticator // nil without a store
	hub      *hub           // nil without a store and a bus
	// The services that the domain routes call. Each is nil when its dependency was not given, and
	// a route that needs one is then not registered. The server only holds them: the rules are in
	// the services.
	projects  *projects.Service
	sessions  *session.Manager
	catalog   catalog.Source
	dashboard *dashboard.Service
	history   *cardhistory.Service
	diff      *diff.Service
	chats     *chats.Service
	search    *search.Service
	memory    *memory.Service
	cardPanel *cardpanel.Service
	accounts  *accounts.Service
	auditlog  *auditlog.Service

	providers  *providers.Service
	costLimits *providers.Limits

	connectionTests *connectiontest.Runner

	roles        *roles.Service
	pullRequests *pullrequest.Service
	integrator   *integrator.Service
	integration  integrator.Reader
	opener       Opener
	review       *review.Service

	sleepSettings *settings.Service
	alerts        AlertSettings
	quality       *quality.Service
	integrations  *integrations.Service
	webhooks      *githubapp.Receiver
	ci            *ci.Service
	localCI       *localci.Service
	preview       *preview.Service
	schedules     *schedules.Service
	webUI         fs.FS
	// devices owns pairing, the device rows, and revoking one (B9.2). Nil means no device address
	// exists at all.
	devices *devices.Service
	// tailnet is the node on the person's tailnet (B9.1). Nil is a daemon on this machine only.
	tailnet TailnetNode
	// funnel is whether /hooks/* was asked to be exposed to the public internet.
	funnel bool
	// mcp is the daemon's internal MCP server endpoint (docs/architecture.md section 11.4): one
	// server per live card, reached only by the card's own agent. It lives outside the domain routes
	// because it authenticates itself with a per-card secret rather than the daemon's owner token.
	mcp http.Handler
}

// New makes a server. `now` is the clock, so tests can fix the time.
func New(settings config.Settings, log *slog.Logger, now func() time.Time, deps Deps) *Server {
	s := &Server{
		settings: settings, log: log, now: now, limits: deps.Limits.withDefaults(),
		projects: deps.Projects, sessions: deps.Sessions, catalog: deps.Catalog,
		dashboard: deps.Dashboard, history: deps.History, diff: deps.Diff, chats: deps.Chats,
		search: deps.Search, memory: deps.Memory, accounts: deps.Accounts, auditlog: deps.Auditlog,
		providers: deps.Providers, costLimits: deps.CostLimits,
		connectionTests: deps.ConnectionTests,
		roles:           deps.Roles,
		pullRequests:    deps.PullRequests,
		integrator:      deps.Integrator,
		integration:     deps.Integration,
		opener:          deps.Opener,
		review:          deps.Review,
		sleepSettings:   deps.SleepSettings,
		alerts:          deps.Alerts,
		cardPanel:       deps.CardPanel,
		quality:         deps.Quality,
		integrations:    deps.Integrations,
		webhooks:        deps.Webhooks,
		ci:              deps.CI,
		localCI:         deps.LocalCI,
		preview:         deps.Preview,
		schedules:       deps.Schedules,
		webUI:           deps.WebUI,
		devices:         deps.Devices,
		tailnet:         deps.Tailnet,
		funnel:          deps.Funnel && deps.Tailnet != nil,
		mcp:             deps.MCP,
	}
	if s.opener == nil {
		s.opener = SystemOpener{}
	}
	if deps.Store == nil {
		return s
	}
	var dev *devCredential
	if settings.Dev() && deps.Dev != nil && deps.Dev.Token != "" {
		dev = &devCredential{hash: store.HashToken(deps.Dev.Token), caller: deps.Dev.Caller}
	}
	s.auth = newAuthenticator(deps.Store, log, now, dev)
	if deps.Bus != nil {
		s.hub = newHub(deps.Bus, s.limits, log, now)
		if deps.Sessions != nil {
			s.hub.terminals = deps.Sessions
		}
	}
	return s
}

// Handler returns the routes. Tests call it directly.
func (s *Server) Handler() http.Handler {
	return s.handler()
}

// handler builds the routes and wraps them in the middleware. extra adds routes before the
// wrapping, which is how the tests add a protected route of their own.
func (s *Server) handler(extra ...func(*router)) http.Handler {
	routes := newRouter(s)
	routes.public("GET /v1/health", s.health)
	// Pairing is the one address that mints a token with no token of its own: a device being
	// paired has none yet, which is the whole point. What authorizes it is the short-lived,
	// single-use code read off a device that is already signed in (internal/devices), and the
	// Funnel handler refuses it along with everything outside /hooks/.
	if s.devices != nil {
		routes.public("POST /v1/devices/pair", s.pairDevice)
	}
	// The webhook route is registered outside the token check on purpose: GitHub has no token, and
	// its deliveries are authorized by a signature over their raw body instead (B6.1).
	if s.webhooks != nil {
		routes.signedWebhook("POST /hooks/github", s.githubWebhook)
	}
	// Trello's deliveries are authorized the same way, against the secret and callback URL of the
	// stored Trello connection (B8.2). The routes service is the receiver here rather than a
	// separate object: there is nothing for a verified Trello delivery to reach yet, so the whole
	// of checking one is reading the two halves out of the keychain and the row.
	if s.integrations != nil {
		routes.signedWebhook("POST /hooks/trello", s.trelloWebhook)
		// Google's consent page sends the owner's browser back here, and a browser visit carries no
		// token, so this one cannot sit behind the token check. What authorizes it is the single-use
		// state that only a signed-in owner's own authorize call mints (integrations.FinishGoogle).
		routes.public("GET /v1/integrations/gcal/callback", s.callbackGoogleCalendar)
	}
	if s.auth != nil {
		routes.protected("GET /v1/auth/whoami", s.whoami)
		if s.hub != nil {
			routes.stream("GET /v1/events", s.eventStream)
		}
		s.addDomainRoutes(routes)
	}
	// The internal MCP server is mounted as a stream route (a raw handler with no body rules and no
	// owner-token check): it authenticates each request with the per-card secret minted when that
	// card's session started, so an agent reaches its own card's tools and nothing else.
	if s.mcp != nil {
		routes.stream(mcpserver.PathPrefix, s.mcp.ServeHTTP)
	}
	for _, add := range extra {
		add(routes)
	}
	// The first wrapper is the outermost: the id is set first so every later log line has it, the
	// access log sees the final status, and recovery sits closest to the handlers.
	return s.withRequestID(s.withAccessLog(s.withRecovery(routes)))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, protocol.Health{
		Status:     "ok",
		Version:    buildinfo.Version,
		Mode:       string(s.settings.Mode),
		ServerTime: protocol.NewTimestamp(s.now()),
	})
}

// Listen opens the port on the local machine only. A port of 0 lets the system pick a free one.
func (s *Server) Listen() (net.Listener, error) {
	addr := net.JoinHostPort(loopback, strconv.Itoa(s.settings.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return listener, nil
}

// Run listens and serves until the context is cancelled, then finishes open requests and returns.
//
// With no tailnet node it serves on loopback, which is the daemon of every phase before this one.
// With one it also serves on the tailnet: the loopback listener is opened first and left alone,
// the node is joined in the background, and only then does a second listener open on the tailnet
// address. Nothing ever replaces the loopback bind, and nothing ever listens on every interface
// (docs/architecture.md section 13). A tailnet that cannot be joined is logged and the daemon
// keeps serving on this machine, because a phone being unreachable is not a reason for the
// desktop app to be unreachable too.
func (s *Server) Run(ctx context.Context) error {
	listener, err := s.Listen()
	if err != nil {
		return err
	}
	if s.tailnet == nil {
		return s.Serve(ctx, listener)
	}
	group, gctx := errgroup.WithContext(ctx)
	group.Go(func() error { return s.Serve(gctx, listener) })
	group.Go(func() error { return s.serveTailnet(gctx) })
	return group.Wait()
}

// serveTailnet joins the tailnet and serves on it, alongside the loopback listener. Every failure
// that is about the tailnet is logged and answered with nil: the daemon is already serving on this
// machine, and a tailnet nobody has signed in to is a feature that is off, not a daemon that is
// down.
func (s *Server) serveTailnet(ctx context.Context) error {
	go s.tailnet.Watch(ctx)
	if _, err := s.tailnet.Up(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		s.log.Error("Marshal is reachable on this machine only", "error", err)
		return nil
	}
	group, gctx := errgroup.WithContext(ctx)
	listener, err := s.tailnet.Listen("tcp", s.tailnetAddr())
	if err != nil {
		s.log.Error("could not listen on the tailnet", "error", err)
		return nil
	}
	group.Go(func() error { return s.Serve(gctx, listener) })
	if s.funnel {
		if err := s.openFunnel(gctx, group); err != nil {
			s.log.Error("could not open Funnel", "error", err)
		}
	}
	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}

// openFunnel opens the one public listener the daemon ever has, and serves only the webhook
// routes on it. The rest of the daemon - the UI, every /v1 route, and the pairing route - is
// reached on the tailnet, never from the public internet (docs/architecture.md section 13).
func (s *Server) openFunnel(ctx context.Context, group *errgroup.Group) error {
	listener, err := s.tailnet.ListenFunnel("tcp", funnelAddr)
	if err != nil {
		return err
	}
	s.log.Info("Funnel is open for webhooks only", "address", funnelAddr)
	group.Go(func() error { return s.ServeOn(ctx, listener, s.FunnelHandler()) })
	return nil
}

// Serve serves on a listener that the caller opened. When the context ends it stops accepting,
// closes every open event stream with "going away", waits for requests in progress, and returns,
// all within the shutdown window.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	return s.ServeOn(ctx, listener, s.Handler())
}

// ServeOn serves a specific handler on a listener the caller opened, with the same shutdown
// behaviour as Serve. It exists so the two listeners this daemon can have - loopback and the
// tailnet, plus Funnel's - share one shutdown path, and so the public one serves a handler that
// refuses every address outside /hooks/.
func (s *Server) ServeOn(ctx context.Context, listener net.Listener, handler http.Handler) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       s.limits.ReadTimeout,
		WriteTimeout:      s.limits.WriteTimeout,
		IdleTimeout:       s.limits.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}
	done := make(chan error, 1)
	go func() {
		if err := srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			done <- err
			return
		}
		done <- nil
	}()
	s.log.Info("daemon listening", "address", listener.Addr().String(), "mode", s.settings.Mode)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	return s.shutdown(ctx, srv, done)
}

// shutdown ends the server in order. The event streams are told first so no new one starts, then
// the listener stops, then the handlers of the streams are waited for: net/http does not track
// a connection that a handler has taken over, so the hub does.
func (s *Server) shutdown(ctx context.Context, srv *http.Server, done <-chan error) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if s.hub != nil {
		s.hub.closeAll(websocket.StatusGoingAway, "Marshal is shutting down.")
	}
	var problems []error
	if err := srv.Shutdown(shutdownCtx); err != nil {
		problems = append(problems, fmt.Errorf("shut down: %w", err))
	}
	if s.hub != nil {
		if err := s.hub.wait(shutdownCtx); err != nil {
			problems = append(problems, err)
		}
	}
	if err := <-done; err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}
