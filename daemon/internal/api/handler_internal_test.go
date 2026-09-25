package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// syncBuffer is a log destination that the server writes to from its own goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func internalServer(logs *syncBuffer, settings config.Settings) *Server {
	log := slog.New(slog.NewTextHandler(logs, nil))
	return New(settings, log, time.Now, Deps{})
}

// A handler that panics gives the client the one generic answer. The panic and its stack go to the
// log, and nothing of them to the client.
func TestAPanicInAHandlerBecomesAnInternalAnswer(t *testing.T) {
	logs := &syncBuffer{}
	s := internalServer(logs, config.Settings{Mode: platform.ModeDev})
	handler := s.handler(func(r *router) {
		r.public("GET /v1/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom with a secret detail") })
		r.public("GET /v1/halfway", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			panic("after the answer started")
		})
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/boom", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var got protocol.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Error.Code != protocol.ErrorCodeInternal {
		t.Fatalf("body = %s (error %v), want the internal answer", rec.Body, err)
	}
	if strings.Contains(rec.Body.String(), "kaboom") {
		t.Error("the panic value reached the client")
	}
	if !strings.Contains(logs.String(), "kaboom with a secret detail") || !strings.Contains(logs.String(), "a handler panicked") {
		t.Errorf("the panic is not in the log:\n%s", logs)
	}

	// Once the answer has started, its status is already sent and no second answer is written.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/halfway", nil))
	if rec.Code != http.StatusAccepted || strings.Contains(rec.Body.String(), "internal") {
		t.Errorf("a panic after the answer started gave %d %s", rec.Code, rec.Body)
	}
}

// A handler that asks the server to drop the connection is not turned into an answer.
func TestAnAbortedHandlerIsPassedOn(t *testing.T) {
	s := internalServer(&syncBuffer{}, config.Settings{Mode: platform.ModeDev})
	handler := s.handler(func(r *router) {
		r.public("GET /v1/abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	})
	defer func() {
		value := recover()
		err, ok := value.(error)
		if !ok || !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler to be passed on", value)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/abort", nil))
}

var requestIDShape = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

// Every answer carries an id the daemon made, whatever the client sent.
func TestEveryAnswerHasARequestIDTheDaemonMade(t *testing.T) {
	s := internalServer(&syncBuffer{}, config.Settings{Mode: platform.ModeDev})
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set(requestIDHeader, "chosen-by-the-client")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get(requestIDHeader); !requestIDShape.MatchString(got) {
		t.Errorf("request id = %q, want an id that the daemon made", got)
	}
}

// Run listens on the port and serves until the context ends.
func TestRunServesUntilTheContextEnds(t *testing.T) {
	logs := &syncBuffer{}
	s := internalServer(logs, config.Settings{Mode: platform.ModeDev, Port: 0})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "daemon listening") {
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("Run never started listening:\n%s", logs)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v after the context ended", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

// Run says so when the port cannot be opened.
func TestRunReportsAPortThatIsTaken(t *testing.T) {
	first := internalServer(&syncBuffer{}, config.Settings{Mode: platform.ModeDev, Port: 0})
	listener, err := first.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	taken := internalServer(&syncBuffer{}, config.Settings{Mode: platform.ModeDev, Port: port})
	if err := taken.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded on a port that is already in use")
	}
}
