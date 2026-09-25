// Package api serves the HTTP and WebSocket API under /v1. It listens on the local machine only.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// loopback is the only address the daemon listens on until a tailnet address is added.
	loopback          = "127.0.0.1"
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 5 * time.Second
	jsonContentType   = "application/json; charset=utf-8"
)

// Server is the daemon's HTTP server.
type Server struct {
	settings config.Settings
	log      *slog.Logger
	now      func() time.Time
}

// New makes a server. `now` is the clock, so tests can fix the time.
func New(settings config.Settings, log *slog.Logger, now func() time.Time) *Server {
	return &Server{settings: settings, log: log, now: now}
}

// Handler returns the routes. Tests call it directly.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, protocol.Health{
		Status:     "ok",
		Version:    buildinfo.Version,
		Mode:       string(s.settings.Mode),
		ServerTime: s.now().UTC(),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write a response", "error", err)
	}
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

// Serve serves on a listener that the caller opened.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: readHeaderTimeout}
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
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down: %w", err)
	}
	return <-done
}
