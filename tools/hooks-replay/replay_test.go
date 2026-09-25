package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rawBody = `{"zen":"Keep it logically awesome.",  "hook_id":1}`

// writeRecording saves recording text exactly as given, under a temp hooks folder, and returns
// the folder. It writes the text itself rather than encoding a value, because encoding would
// tidy the body's spacing, and the point of the test is that spacing survives.
func writeRecording(t *testing.T, provider, name, text string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, provider), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, provider, name+".json"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReplaySendsTheRecordedBytesAndHeaders(t *testing.T) {
	dir := writeRecording(t, "github", "ping",
		`{"headers": {"X-GitHub-Event": "ping", "X-Hub-Signature-256": "sha256=abc"}, "body": `+rawBody+`}`)
	var gotBody []byte
	var gotHeader http.Header
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotHeader, gotPath, gotMethod = r.Header.Clone(), r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := run([]string{"-url", server.URL, "-dir", dir, "github", "ping"}, &out, &errOut)
	if code != exitOK {
		t.Fatalf("exit code = %d (%s)", code, errOut.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/hooks/github" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if string(gotBody) != rawBody {
		t.Errorf("body was changed:\n got %s\nwant %s", gotBody, rawBody)
	}
	if gotHeader.Get("X-GitHub-Event") != "ping" || gotHeader.Get("X-Hub-Signature-256") != "sha256=abc" {
		t.Errorf("headers = %v", gotHeader)
	}
	if !strings.Contains(out.String(), "204") {
		t.Errorf("output %q does not show the status", out.String())
	}
}

func TestReplayReportsANon2xxAnswer(t *testing.T) {
	dir := writeRecording(t, "github", "ping", `{"body": `+rawBody+`}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := run([]string{"-url", server.URL, "-dir", dir, "github", "ping"}, &out, &errOut)
	if code != exitFailed || !strings.Contains(errOut.String(), "401") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestLoadRecordingRefusesNamesThatEscapeTheFolder(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range [][2]string{{"..", "ping"}, {"github", "../secret"}, {"git/hub", "ping"}, {"GitHub", "ping"}, {"", "ping"}} {
		if _, err := loadRecording(dir, bad[0], bad[1]); !errors.Is(err, errBadName) {
			t.Errorf("loadRecording(%q, %q) = %v, want errBadName", bad[0], bad[1], err)
		}
	}
}

func TestLoadRecordingNeedsABody(t *testing.T) {
	dir := writeRecording(t, "github", "empty", `{"headers": {}}`)
	if _, err := loadRecording(dir, "github", "empty"); err == nil {
		t.Error("a recording with no body was accepted")
	}
}

func TestSendNeedsAReachableDaemon(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	_, err := send(ctx, http.DefaultClient, target{baseURL: "http://127.0.0.1:1", provider: "github"}, recording{Body: json.RawMessage(rawBody)})
	if err == nil {
		t.Error("send to a closed port returned no error")
	}
}

func TestUsageWhenArgumentsAreMissing(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"github"}, &out, &errOut); code != exitBadInput || !strings.Contains(errOut.String(), "Usage") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}
