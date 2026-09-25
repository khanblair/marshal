package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// newTestServer returns a server whose log lines go to the returned buffer.
func newTestServer() (*Server, *bytes.Buffer) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	settings := config.Settings{Mode: platform.ModeDev, Port: config.DevPort}
	return New(settings, log, time.Now, Deps{}), &logs
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) protocol.Error {
	t.Helper()
	var body protocol.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not the error shape: %v\n%s", err, rec.Body.String())
	}
	return body.Error
}

func TestStatusTable(t *testing.T) {
	want := map[protocol.ErrorCode]int{
		protocol.ErrorCodeInvalidArgument:  400,
		protocol.ErrorCodeUnauthorized:     401,
		protocol.ErrorCodeForbidden:        403,
		protocol.ErrorCodeNotFound:         404,
		protocol.ErrorCodeMethodNotAllowed: 405,
		protocol.ErrorCodeConflict:         409,
		protocol.ErrorCodeRefused:          422,
		protocol.ErrorCodeUnsupported:      501,
		protocol.ErrorCodeUnavailable:      503,
		protocol.ErrorCodeInternal:         500,
	}
	for _, code := range protocol.ErrorCodeValues() {
		status, ok := want[code]
		if !ok {
			t.Errorf("the status table in this test has no row for %q", code)
			continue
		}
		if got := statusOf(code); got != status {
			t.Errorf("statusOf(%q) = %d, want %d", code, got, status)
		}
	}
	if got := statusOf("made_up"); got != http.StatusInternalServerError {
		t.Errorf("statusOf(unknown) = %d, want 500", got)
	}
}

func TestWriteErrorSendsAProtocolError(t *testing.T) {
	server, logs := newTestServer()
	rec := httptest.NewRecorder()
	server.writeError(rec, protocol.NotFound("card").With("id", "abc"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != jsonContentType {
		t.Errorf("content type = %q", got)
	}
	got := decodeError(t, rec)
	if got.Code != protocol.ErrorCodeNotFound || got.Details["id"] != "abc" {
		t.Errorf("error = %+v", got)
	}
	if logs.Len() != 0 {
		t.Errorf("a 404 is the client's mistake and should not be logged: %s", logs.String())
	}
}

func TestWriteErrorFindsAWrappedProtocolError(t *testing.T) {
	server, _ := newTestServer()
	rec := httptest.NewRecorder()
	server.writeError(rec, fmt.Errorf("move the card: %w", protocol.Refused("A card in Done cannot move back. Create a new card instead.")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	if got := decodeError(t, rec); got.Code != protocol.ErrorCodeRefused {
		t.Errorf("code = %q, want refused", got.Code)
	}
}

func TestWriteErrorHidesUnknownErrors(t *testing.T) {
	server, logs := newTestServer()
	rec := httptest.NewRecorder()
	server.writeError(rec, errors.New("open /Users/me/.config/marshal/secret.db: permission denied"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	got := decodeError(t, rec)
	want := protocol.Internal()
	if got.Code != protocol.ErrorCodeInternal || got.Message != want.Message {
		t.Errorf("error = %+v, want the generic internal answer", got)
	}
	if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "permission") {
		t.Errorf("the reason leaked to the client: %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "permission denied") {
		t.Errorf("the reason was not logged: %q", logs.String())
	}
}

func TestWriteErrorDoesNotReportAHungUpClientAsAFailure(t *testing.T) {
	server, logs := newTestServer()
	rec := httptest.NewRecorder()
	server.writeError(rec, fmt.Errorf("list projects: %w", context.Canceled))
	if strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("a cancelled request was logged as an error: %q", logs.String())
	}
	if !strings.Contains(logs.String(), "request cancelled") {
		t.Errorf("the cancelled request was not logged at all: %q", logs.String())
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want the unchanged 500", rec.Code)
	}
}

func TestWriteErrorLogsAnInternalProtocolErrorWithItsCause(t *testing.T) {
	server, logs := newTestServer()
	rec := httptest.NewRecorder()
	server.writeError(rec, protocol.Internal().WithCause(errors.New("disk is full")))
	if !strings.Contains(logs.String(), "disk is full") || strings.Contains(rec.Body.String(), "disk") {
		t.Errorf("log = %q, body = %s", logs.String(), rec.Body.String())
	}
}

func TestWriteJSONTurnsAnEncodeFailureIntoAnInternalError(t *testing.T) {
	server, logs := newTestServer()
	rec := httptest.NewRecorder()
	// A zero timestamp does not encode, so this body cannot be sent.
	server.writeJSON(rec, http.StatusOK, protocol.Health{Status: "ok"})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	got := decodeError(t, rec)
	if got.Code != protocol.ErrorCodeInternal {
		t.Errorf("code = %q, want internal", got.Code)
	}
	if !strings.Contains(logs.String(), "encode a response") {
		t.Errorf("the encode failure was not logged: %q", logs.String())
	}
}

func TestFallbackBodyIsTheInternalError(t *testing.T) {
	want, err := json.Marshal(protocol.ErrorResponse{Error: *protocol.Internal()})
	if err != nil {
		t.Fatal(err)
	}
	if fallbackErrorBody != string(want)+"\n" {
		t.Errorf("fallbackErrorBody = %s, want %s", fallbackErrorBody, want)
	}
}
