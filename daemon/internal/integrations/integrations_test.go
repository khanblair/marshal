package integrations_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	ghclient "github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// testKey makes a throwaway RSA private key in the PEM form GitHub generates. Nothing is ever sent
// with it: it only has to parse, so the App can be built from it.
var testKey = func() []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}()

// fixture is a connections service over a real store and an in-memory keychain, the way the daemon
// builds it and a test never touches the machine's keychain.
type fixture struct {
	t     *testing.T
	store *store.Store
	keys  *security.MemoryKeychain
	svc   *integrations.Service
}

func newFixture(t *testing.T, opts ...func(*integrations.Options)) *fixture {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &fixture{t: t, store: st, keys: security.NewMemoryKeychain()}
	var o integrations.Options
	for _, opt := range opts {
		opt(&o)
	}
	svc, err := integrations.New(st, f.keys, o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f.svc = svc
	return f
}

// connected saves a GitHub App connection with a throwaway key, so a test starts from a daemon the
// owner has set up.
func (f *fixture) connected(secret string) {
	f.t.Helper()
	err := f.svc.SaveGitHub(context.Background(), protocol.SaveGitHubRequest{
		AppID: 42, InstallationID: 99,
		PrivateKey:    string(testKey),
		WebhookSecret: secret,
	})
	if err != nil {
		f.t.Fatalf("SaveGitHub: %v", err)
	}
}

// row reads a connection's stored row straight from the database, so a test can see what was
// actually written rather than what a route reported.
func (f *fixture) row(id string) (config, keychainRef string, found bool) {
	f.t.Helper()
	err := f.store.Read(context.Background(), func(q *db.Queries) error {
		row, err := q.GetIntegration(context.Background(), id)
		if err != nil {
			return err
		}
		config, keychainRef, found = row.ConfigJSON, row.KeychainRef, true
		return nil
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		f.t.Fatalf("read the %q row: %v", id, err)
	}
	return config, keychainRef, found
}

func TestListShowsEveryConnectionAndOnlyGitHubCanBeSetUp(t *testing.T) {
	f := newFixture(t)
	list, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 7 {
		t.Fatalf("List answered %d connections, want the 7 the settings screen shows", len(list))
	}
	want := []string{"github", "trello", "gcal", "gmail", "telegram", "discord", "obsidian"}
	for i, id := range want {
		if list[i].ID != id {
			t.Fatalf("connection %d is %q, want %q", i, list[i].ID, id)
		}
		if list[i].Status != protocol.IntegrationStatusNone {
			t.Fatalf("%s reads %q with nothing stored, want none", id, list[i].Status)
		}
		if list[i].LastTest != nil {
			t.Fatalf("%s has a last test with nothing ever run", id)
		}
	}
	if list[0].Kind != integrations.KindGitHub {
		t.Fatalf("the GitHub row's kind is %q, want %q", list[0].Kind, integrations.KindGitHub)
	}
}

func TestSavingTheAppConnectsItAndKeepsTheSecretOutOfTheRow(t *testing.T) {
	const secret = "s3cr3t"
	f := newFixture(t)
	f.connected(secret)

	list, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].Status != protocol.IntegrationStatusConnected {
		t.Fatalf("GitHub reads %q after being saved, want connected", list[0].Status)
	}
	config, ref, found := f.row(integrations.GitHubID)
	if !found {
		t.Fatal("saving the App wrote no row")
	}
	// The row carries the two ids and the name of the keychain entry, and neither secret.
	if strings.Contains(config, secret) || strings.Contains(config, "PRIVATE KEY") {
		t.Fatalf("the row holds a secret: %s", config)
	}
	var stored struct {
		AppID          int64 `json:"appId"`
		InstallationID int64 `json:"installationId"`
	}
	if err := json.Unmarshal([]byte(config), &stored); err != nil {
		t.Fatalf("the row's config is not the App's settings: %v", err)
	}
	if stored.AppID != 42 || stored.InstallationID != 99 {
		t.Fatalf("the row holds %+v, want app 42 installation 99", stored)
	}
	if ref != integrations.GitHubID {
		t.Fatalf("the row's keychain reference is %q, want %q", ref, integrations.GitHubID)
	}
	// Both secrets are in the keychain, under the connection's own id, and the row never held them.
	raw, err := f.keys.Get(integrations.GitHubID)
	if err != nil {
		t.Fatalf("the secrets are not in the keychain: %v", err)
	}
	if !strings.Contains(raw, secret) || !strings.Contains(raw, "PRIVATE KEY") {
		t.Fatalf("the keychain entry does not hold both secrets")
	}
}

func TestSavingRefusesAnAppThatCouldNotWork(t *testing.T) {
	cases := []struct {
		name string
		req  protocol.SaveGitHubRequest
	}{
		{"no app id", protocol.SaveGitHubRequest{InstallationID: 1, PrivateKey: string(testKey), WebhookSecret: "s"}},
		{"no installation id", protocol.SaveGitHubRequest{AppID: 1, PrivateKey: string(testKey), WebhookSecret: "s"}},
		{"no key", protocol.SaveGitHubRequest{AppID: 1, InstallationID: 1, WebhookSecret: "s"}},
		{"no webhook secret", protocol.SaveGitHubRequest{AppID: 1, InstallationID: 1, PrivateKey: string(testKey)}},
		{"a blank webhook secret", protocol.SaveGitHubRequest{AppID: 1, InstallationID: 1, PrivateKey: string(testKey), WebhookSecret: "  "}},
		{"a key that is not a key", protocol.SaveGitHubRequest{AppID: 1, InstallationID: 1, PrivateKey: "not a pem", WebhookSecret: "s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			err := f.svc.SaveGitHub(context.Background(), tc.req)
			if err == nil {
				t.Fatal("saving an App that could not work should fail")
			}
			// Nothing is written: a refused save leaves the daemon exactly as it was.
			if _, _, found := f.row(integrations.GitHubID); found {
				t.Fatal("a refused save wrote a row")
			}
			if _, err := f.keys.Get(integrations.GitHubID); !errors.Is(err, security.ErrNoKey) {
				t.Fatal("a refused save wrote a keychain entry")
			}
		})
	}
}

func TestSavingAgainReplacesTheConnection(t *testing.T) {
	f := newFixture(t)
	f.connected("first")
	f.connected("second")
	// One row, not two, and the keychain holds the newest secret alone: saving twice is what
	// rotating a key does.
	config, _, found := f.row(integrations.GitHubID)
	if !found || config == "" {
		t.Fatal("saving again lost the row")
	}
	raw, err := f.keys.Get(integrations.GitHubID)
	if err != nil {
		t.Fatalf("read the secrets: %v", err)
	}
	if strings.Contains(raw, "first") || !strings.Contains(raw, "second") {
		t.Fatalf("the keychain holds the old secret: %s", raw)
	}
}

func TestRemovingForgetsTheSettingsAndTheSecret(t *testing.T) {
	f := newFixture(t)
	f.connected("s3cr3t")
	if err := f.svc.Remove(context.Background(), integrations.GitHubID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, _, found := f.row(integrations.GitHubID); found {
		t.Fatal("removing left the row behind")
	}
	if _, err := f.keys.Get(integrations.GitHubID); !errors.Is(err, security.ErrNoKey) {
		t.Fatal("removing left the keychain entry behind")
	}
	// Removing again is not an error: the answer is the same list either way.
	if err := f.svc.Remove(context.Background(), integrations.GitHubID); err != nil {
		t.Fatalf("removing a connection with nothing stored = %v, want nil", err)
	}
	// A connection Marshal does not know is not found.
	if err := f.svc.Remove(context.Background(), "myspace"); err == nil {
		t.Fatal("removing an unknown connection should fail")
	}
}

func TestReadingTheAppWithNothingStoredIsNotConnected(t *testing.T) {
	f := newFixture(t)
	app, err := f.svc.App(context.Background())
	if !errors.Is(err, integrations.ErrNotConnected) {
		t.Fatalf("App with nothing stored = %v, want ErrNotConnected", err)
	}
	if app != nil {
		t.Fatal("App with nothing stored answered a client")
	}
}

func TestTheWebhookSecretIsReadFreshFromTheKeychain(t *testing.T) {
	f := newFixture(t)
	// Before anything is saved the receiver has no secret, so a delivery is refused.
	body := []byte(`{}`)
	h := http.Header{}
	h.Set(githubapp.EventHeader, "ping")
	if err := f.svc.Receiver().Receive(context.Background(), h, body); !errors.Is(err, githubapp.ErrNoSecret) {
		t.Fatalf("Receive with nothing saved = %v, want ErrNoSecret", err)
	}
	f.connected("s3cr3t")
	signed := http.Header{}
	signed.Set(githubapp.EventHeader, "ping")
	signed.Set(githubapp.SignatureHeader, sign("s3cr3t", body))
	if err := f.svc.Receiver().Receive(context.Background(), signed, body); err != nil {
		t.Fatalf("Receive after the App was saved: %v", err)
	}
	// Removing the connection stops the secret being trusted at once, without a restart.
	if err := f.svc.Remove(context.Background(), integrations.GitHubID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := f.svc.Receiver().Receive(context.Background(), signed, body); !errors.Is(err, githubapp.ErrNoSecret) {
		t.Fatalf("Receive after the App was removed = %v, want ErrNoSecret", err)
	}
}

func TestAVerifiedDeliveryReachesTheMonitor(t *testing.T) {
	f := newFixture(t)
	f.connected("s3cr3t")
	monitor := &recordingMonitor{}
	f.svc.SetMonitor(monitor)

	body := []byte(`{"action":"completed"}`)
	h := http.Header{}
	h.Set(githubapp.EventHeader, "check_run")
	h.Set(githubapp.SignatureHeader, sign("s3cr3t", body))
	if err := f.svc.Receiver().Receive(context.Background(), h, body); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(monitor.events) != 1 || monitor.events[0].Kind != "check_run" {
		t.Fatalf("the monitor got %+v, want the one check_run delivery", monitor.events)
	}
}

// recordingMonitor remembers every verified delivery it is handed.
type recordingMonitor struct{ events []githubapp.Event }

func (m *recordingMonitor) Delivery(_ context.Context, event githubapp.Event) error {
	m.events = append(m.events, event)
	return nil
}

// sign makes the signature GitHub would send for a body with a secret.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// fakeGitHub answers the three routes the connection test reads, and refuses whichever one the test
// asks it to, so every check can be driven without a live App.
type fakeGitHub struct {
	account     string
	selection   string
	permissions map[string]string
	repos       []string
	refuse      map[string]int // path suffix -> status to answer
	seen        []string
}

func (f *fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	f.seen = append(f.seen, req.URL.Path)
	if strings.HasSuffix(req.URL.Path, "/access_tokens") {
		return jsonReply(201, map[string]any{"token": "ghs_x", "expires_at": "2100-01-01T00:00:00Z"}), nil
	}
	for path, status := range f.refuse {
		if strings.Contains(req.URL.Path, path) {
			return jsonReply(status, map[string]string{"message": "nope"}), nil
		}
	}
	switch {
	case strings.HasPrefix(req.URL.Path, "/app/installations/"):
		return jsonReply(200, map[string]any{
			"account":              map[string]string{"login": f.account},
			"permissions":          f.permissions,
			"repository_selection": f.selection,
		}), nil
	case req.URL.Path == "/installation/repositories":
		repos := make([]map[string]string, 0, len(f.repos))
		for _, name := range f.repos {
			repos = append(repos, map[string]string{"full_name": name})
		}
		return jsonReply(200, map[string]any{"total_count": len(repos), "repositories": repos}), nil
	}
	return jsonReply(404, map[string]string{"message": "not found"}), nil
}

func jsonReply(status int, value any) *http.Response {
	body, _ := json.Marshal(value)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

// withFakeGitHub points the connection test at a fake server, so its checks run for real without a
// network.
func withFakeGitHub(fake *fakeGitHub) func(*integrations.Options) {
	return func(o *integrations.Options) {
		o.App = func(context.Context) (*githubapp.App, error) {
			return githubapp.NewApp(githubapp.AppConfig{
				AppID: 42, InstallationID: 99, PrivateKey: testKey, Transport: fake,
			}, ghclient.WithBaseURL("https://api.github.invalid"))
		}
	}
}

// healthy is a fake GitHub the App can do everything with.
func healthy() *fakeGitHub {
	return &fakeGitHub{
		account:   "acme",
		selection: "all",
		permissions: map[string]string{
			"issues": "write", "pull_requests": "write", "actions": "write",
		},
		repos: []string{"acme/one", "acme/two"},
	}
}

// checkNamed finds one check by its label.
func checkNamed(t *testing.T, result protocol.TestResult, name string) protocol.TestCheck {
	t.Helper()
	for _, check := range result.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("the result has no %q check: %+v", name, result.Checks)
	return protocol.TestCheck{}
}

func TestTheTestReportsAWorkingApp(t *testing.T) {
	fake := healthy()
	f := newFixture(t, withFakeGitHub(fake))
	f.connected("s3cr3t")

	result, err := f.svc.Test(context.Background(), integrations.GitHubID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Fatalf("a working App did not pass: %+v", result.Checks)
	}
	if got := checkNamed(t, result, integrations.CheckAppInstalled); got.State != protocol.CheckStatePassed ||
		!strings.Contains(got.Message, "acme") {
		t.Fatalf("the installed check reads %+v, want a pass naming acme", got)
	}
	if got := checkNamed(t, result, integrations.CheckRepositories); !strings.Contains(got.Message, "2 repositories") {
		t.Fatalf("the repositories check reads %+v, want a pass naming 2 repositories", got)
	}
	// No delivery has reached the daemon, which is a warning and not a failure: GitHub sends a ping
	// only when the webhook is set up, and an App that works is not broken for not being pinged.
	if got := checkNamed(t, result, integrations.CheckWebhook); got.State != protocol.CheckStateWarning {
		t.Fatalf("the webhook check reads %+v, want a warning with nothing delivered yet", got)
	}
	if got := checkNamed(t, result, integrations.CheckSummary); got.State != protocol.CheckStateWarning {
		t.Fatalf("the summary reads %+v, want a warning while a check is only a warning", got)
	}
}

func TestTheTestCatchesAMissingPermission(t *testing.T) {
	fake := healthy()
	delete(fake.permissions, "pull_requests")
	f := newFixture(t, withFakeGitHub(fake))
	f.connected("s3cr3t")

	result, err := f.svc.Test(context.Background(), integrations.GitHubID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Fatal("an App without the pull_requests permission passed the test")
	}
	got := checkNamed(t, result, integrations.CheckPermissions)
	if got.State != protocol.CheckStateFailed || !strings.Contains(got.Message, "pull_requests") {
		t.Fatalf("the permissions check reads %+v, want a failure naming pull_requests", got)
	}
	// The summary is what a connection row shows, and it is the first failure.
	if summary := checkNamed(t, result, integrations.CheckSummary); summary.State != protocol.CheckStateFailed ||
		summary.Message != got.Message {
		t.Fatalf("the summary reads %+v, want the permissions failure", summary)
	}
}

func TestTheTestWarnsWhenAPermissionIsOnlyReadable(t *testing.T) {
	fake := healthy()
	fake.permissions["actions"] = "read"
	f := newFixture(t, withFakeGitHub(fake))
	f.connected("s3cr3t")

	result, err := f.svc.Test(context.Background(), integrations.GitHubID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	got := checkNamed(t, result, integrations.CheckPermissions)
	if got.State != protocol.CheckStateWarning || !strings.Contains(got.Message, "actions") {
		t.Fatalf("the permissions check reads %+v, want a warning naming actions", got)
	}
	if !result.OK {
		t.Fatal("a read-only permission should warn, not fail")
	}
}

func TestTheTestCatchesABlockedWebhook(t *testing.T) {
	f := newFixture(t, withFakeGitHub(healthy()))
	f.connected("s3cr3t")
	// A delivery signed with the wrong secret is exactly what a mismatched webhook secret looks
	// like, and it is what the check exists to catch.
	body := []byte(`{}`)
	h := http.Header{}
	h.Set(githubapp.EventHeader, "ping")
	h.Set(githubapp.SignatureHeader, sign("the-wrong-secret", body))
	if err := f.svc.Receiver().Receive(context.Background(), h, body); err == nil {
		t.Fatal("a wrongly signed delivery should be refused")
	}

	result, err := f.svc.Test(context.Background(), integrations.GitHubID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	got := checkNamed(t, result, integrations.CheckWebhook)
	if got.State != protocol.CheckStateFailed {
		t.Fatalf("the webhook check reads %+v, want a failure after a refused delivery", got)
	}
	if result.OK {
		t.Fatal("a blocked webhook passed the test")
	}
}

func TestTheTestCatchesARefusedApp(t *testing.T) {
	fake := healthy()
	fake.refuse = map[string]int{"/app/installations/": 401}
	f := newFixture(t, withFakeGitHub(fake))
	f.connected("s3cr3t")

	result, err := f.svc.Test(context.Background(), integrations.GitHubID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	got := checkNamed(t, result, integrations.CheckAppInstalled)
	if got.State != protocol.CheckStateFailed {
		t.Fatalf("the installed check reads %+v, want a failure for a refused App", got)
	}
}

func TestTheTestFailsWhenNoAppIsSaved(t *testing.T) {
	f := newFixture(t)
	result, err := f.svc.Test(context.Background(), integrations.GitHubID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Fatal("testing with no App saved passed")
	}
	got := checkNamed(t, result, integrations.CheckSummary)
	if got.State != protocol.CheckStateFailed || got.Fix == "" {
		t.Fatalf("the summary reads %+v, want a failure with a fix", got)
	}
}

func TestAnUnknownConnectionIsNotFound(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Test(context.Background(), "myspace"); err == nil {
		t.Fatal("testing an unknown connection should fail")
	}
}
