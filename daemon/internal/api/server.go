// Package api serves the HTTP and WebSocket API under /v1. It listens on the local machine only.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store"
)

const (
	// loopback is the only address the daemon listens on until a tailnet address is added.
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
	projects *projects.Service
	sessions *session.Manager
	catalog  catalog.Source
}

// New makes a server. `now` is the clock, so tests can fix the time.
func New(settings config.Settings, log *slog.Logger, now func() time.Time, deps Deps) *Server {
	s := &Server{
		settings: settings, log: log, now: now, limits: deps.Limits.withDefaults(),
		projects: deps.Projects, sessions: deps.Sessions, catalog: deps.Catalog,
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
	if s.auth != nil {
		routes.protected("GET /v1/auth/whoami", s.whoami)
		if s.hub != nil {
			routes.stream("GET /v1/events", s.eventStream)
		}
		s.addDomainRoutes(routes)
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
func (s *Server) Run(ctx context.Context) error {
	listener, err := s.Listen()
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

// Serve serves on a listener that the caller opened. When the context ends it stops accepting,
// closes every open event stream with "going away", waits for requests in progress, and returns,
// all within the shutdown window.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(),
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
