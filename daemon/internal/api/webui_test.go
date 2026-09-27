package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/platform"
)

func newServerWithWebUI() *api.Server {
	settings := config.Settings{Mode: platform.ModeDev, Port: config.DevPort}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	fs := fstest.MapFS{
		"index.html":    {Data: []byte("<html>the app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	return api.New(settings, log, func() time.Time { return fixedNow }, api.Deps{WebUI: fs})
}

func TestWebUIServesAFileThatExists(t *testing.T) {
	rec := httptest.NewRecorder()
	newServerWithWebUI().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "console.log(1)" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestWebUIFallsBackToIndexForAClientSideRoute(t *testing.T) {
	rec := httptest.NewRecorder()
	newServerWithWebUI().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/board", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "<html>the app</html>" {
		t.Errorf("body = %q, want the index page", rec.Body.String())
	}
}

func TestWebUIServesIndexAtTheRoot(t *testing.T) {
	rec := httptest.NewRecorder()
	newServerWithWebUI().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>the app</html>" {
		t.Errorf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}

func TestWebUINeverShadowsTheAPIOrTheWebhook(t *testing.T) {
	rec := httptest.NewRecorder()
	newServerWithWebUI().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/nothing-here", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (not the index page)", rec.Code)
	}
	if rec.Body.String() == "<html>the app</html>" {
		t.Error("the web UI answered an address under /v1, which it must never do")
	}
}

func TestWithoutAWebUITheOldNotFoundAnswerIsUnchanged(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/board", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no web UI was given", rec.Code)
	}
}
