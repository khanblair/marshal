package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

var fixedNow = time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)

func newServer() *api.Server {
	settings := config.Settings{Mode: platform.ModeDev, Port: config.DevPort}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.New(settings, log, func() time.Time { return fixedNow })
}

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got protocol.Health
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "ok" || got.Mode != "dev" || !got.ServerTime.Equal(fixedNow) {
		t.Errorf("health = %+v", got)
	}
}

func TestOnlyGetIsAllowedOnHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/health", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestServeStopsWhenTheContextEnds(t *testing.T) {
	defer goleak.VerifyNone(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- newServer().Serve(ctx, listener) }()

	resp, err := http.Get("http://" + listener.Addr().String() + "/v1/health")
	if err != nil {
		t.Fatalf("GET health: %v", err)
	}
	_ = resp.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v after cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop after the context ended")
	}
}

func TestListensOnLoopbackOnly(t *testing.T) {
	settings := config.Settings{Mode: platform.ModeDev, Port: 0}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	listener, err := api.New(settings, log, time.Now).Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	tcp, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !tcp.IP.IsLoopback() {
		t.Errorf("address %v is not a loopback address", listener.Addr())
	}
}
