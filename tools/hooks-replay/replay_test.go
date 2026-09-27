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

// The signature the daemon checks is the HMAC of the exact body under the webhook secret. This is
// the same vector the daemon-side fixture test uses, so if either side's HMAC changes shape the
// pair stops agreeing here.
const (
	pingBody      = `{"zen":"Keep it logically awesome.","hook_id":1,"hook":{"type":"App","events":["push","pull_request","check_run"]}}`
	pingSecret    = "s3cr3t"
	pingSignature = "sha256=79582579274f1b605ec5f9eb889ba26b1e9d6f4875b58474130cc4114e956bc4"
)

func TestSignMatchesTheDaemonsOwnHmac(t *testing.T) {
	if got := sign(pingSecret, []byte(pingBody)); got != pingSignature {
		t.Errorf("sign = %s, want %s", got, pingSignature)
	}
}

func TestReplaySignsTheBodyWhenASecretIsGiven(t *testing.T) {
	// The recording carries a stale signature on purpose: a replay signed with the daemon's own
	// secret must replace it, or a fixture recorded under another secret could never be replayed.
	dir := writeRecording(t, "github", "ping",
		`{"headers": {"X-GitHub-Event": "ping", "X-Hub-Signature-256": "sha256=stale"}, "body": `+pingBody+`}`)
	var gotSignature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get(SignatureHeader)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	var out, errOut bytes.Buffer
	code := run([]string{"-url", server.URL, "-dir", dir, "-secret", pingSecret, "github", "ping"}, &out, &errOut)
	if code != exitOK {
		t.Fatalf("exit code = %d (%s)", code, errOut.String())
	}
	if gotSignature != pingSignature {
		t.Errorf("signature = %q, want %q", gotSignature, pingSignature)
	}
}

func TestTheRecordedPingFixtureLoadsAndSigns(t *testing.T) {
	// The real fixture, at the default folder: this proves the tool and the recording agree about
	// the shape of a saved delivery, not just a string written inside a test.
	rec, err := loadRecording(defaultDir, "github", "ping")
	if err != nil {
		t.Fatalf("load the recorded ping fixture: %v", err)
	}
	if rec.Headers["X-GitHub-Event"] != "ping" {
		t.Errorf("the fixture's event header = %q, want ping", rec.Headers["X-GitHub-Event"])
	}
	if string(rec.Body) != pingBody {
		t.Errorf("the fixture's body = %s, want %s", rec.Body, pingBody)
	}
	if got := sign(defaultDirSecret, rec.Body); got != defaultDirSignature {
		t.Errorf("the fixture signed with %q = %s, want %s", defaultDirSecret, got, defaultDirSignature)
	}
}

const (
	defaultDirSecret    = "a-webhook-secret"
	defaultDirSignature = "sha256=7c716fde08d6f4dc080d65e67155883c9f08b3b56a9a4a087999254dfb88fde6"
	// The signature of the recorded workflow_run fixture under the same secret. It pins the
	// fixture's body bytes: a re-format that changed them would stop matching here and in the
	// daemon's own replay test, which signs the same bytes with the same vector.
	defaultDirWorkflowRunSignature = "sha256=792f8f6a9eb575b2041c5ba0be1773745089110c0d7a8da8161c6cd7a58eeba9"
)

func TestTheRecordedWorkflowRunFixtureLoadsAndSigns(t *testing.T) {
	// The CI monitor's fixture: a recorded workflow_run failure, replayed at the route the way the
	// ping is. It is what proves the route and the monitor read a delivery GitHub actually sends.
	rec, err := loadRecording(defaultDir, "github", "workflow-run")
	if err != nil {
		t.Fatalf("load the recorded workflow_run fixture: %v", err)
	}
	if rec.Headers["X-GitHub-Event"] != "workflow_run" {
		t.Errorf("the fixture's event header = %q, want workflow_run", rec.Headers["X-GitHub-Event"])
	}
	if got := sign(defaultDirSecret, rec.Body); got != defaultDirWorkflowRunSignature {
		t.Errorf("the fixture signed with %q = %s, want %s", defaultDirSecret, got, defaultDirWorkflowRunSignature)
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
	_, err := send(ctx, http.DefaultClient, target{baseURL: "http://127.0.0.1:1", provider: "github"}, recording{Body: json.RawMessage(rawBody)}, "")
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
