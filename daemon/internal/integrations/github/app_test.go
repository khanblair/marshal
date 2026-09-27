package github_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	ghclient "github.com/khanblair/marshal/daemon/internal/github"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
)

// testKey makes a throwaway RSA private key in the PEM form GitHub generates, so the App transport
// can sign a JWT without a real App.
func testKey(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate an RSA key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// appTransport stands in for GitHub at the transport level: it answers ghinstallation's token
// exchange, and records the Authorization header of every other call the App makes.
type appTransport struct {
	paths []string
	auth  []string
}

func (a *appTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	a.paths = append(a.paths, req.URL.Path)
	if strings.HasSuffix(req.URL.Path, "/access_tokens") {
		return jsonReply(201, map[string]any{
			"token":      "ghs_installation_token",
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}), nil
	}
	a.auth = append(a.auth, req.Header.Get("Authorization"))
	return jsonReply(200, map[string]any{
		"number": 7, "html_url": "https://github.com/o/r/pull/7", "state": "open", "title": "T",
	}), nil
}

func jsonReply(status int, value any) *http.Response {
	body, _ := json.Marshal(value)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func TestTheAppClientSignsEveryCallWithAnInstallationToken(t *testing.T) {
	tr := &appTransport{}
	client, err := githubapp.NewAppClient(githubapp.AppConfig{
		AppID:          42,
		InstallationID: 99,
		PrivateKey:     testKey(t),
		Transport:      tr,
	}, ghclient.WithBaseURL("https://api.github.invalid"))
	if err != nil {
		t.Fatalf("NewAppClient: %v", err)
	}

	pr, err := client.CreatePullRequest(context.Background(), ghclient.NewPullRequest{
		Repo:  ghclient.Repository{Owner: "o", Name: "r"},
		Title: "T", Head: "card-1", Base: "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.Number != 7 {
		t.Fatalf("the pull request number = %d, want 7", pr.Number)
	}
	// ghinstallation authorizes an installation call with "token <value>", not "Bearer".
	if len(tr.auth) != 1 || tr.auth[0] != "token ghs_installation_token" {
		t.Fatalf("the call carried %q, want an installation token", tr.auth)
	}
	// The exchange happens before the call that needs it: the App mints a token, then uses it.
	if len(tr.paths) != 2 || !strings.HasSuffix(tr.paths[0], "/access_tokens") {
		t.Fatalf("the transport saw %v, want the token exchange first", tr.paths)
	}
	if !strings.HasSuffix(tr.paths[1], "/pulls") {
		t.Fatalf("the second call went to %q, want the pulls endpoint", tr.paths[1])
	}
}

func TestTheAppClientIsAClient(t *testing.T) {
	// The whole point of Phase 6's App work is that it is the same interface Phase 5 defined, so a
	// caller that holds a github.Client cannot tell the two apart.
	tr := &appTransport{}
	client, err := githubapp.NewAppClient(githubapp.AppConfig{
		AppID: 1, InstallationID: 2, PrivateKey: testKey(t), Transport: tr,
	})
	if err != nil {
		t.Fatalf("NewAppClient: %v", err)
	}
	var _ ghclient.Client = client
}

func TestAnAppClientNeedsItsIdsAndKey(t *testing.T) {
	key := testKey(t)
	cases := []struct {
		name string
		cfg  githubapp.AppConfig
	}{
		{"no app id", githubapp.AppConfig{InstallationID: 2, PrivateKey: key}},
		{"no installation id", githubapp.AppConfig{AppID: 1, PrivateKey: key}},
		{"no key", githubapp.AppConfig{AppID: 1, InstallationID: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := githubapp.NewAppClient(tc.cfg); err == nil {
				t.Fatalf("NewAppClient with %s should fail", tc.name)
			}
		})
	}
}

func TestATransportClientNeedsAnHTTPClient(t *testing.T) {
	if _, err := ghclient.NewTransportClient(nil); err == nil {
		t.Fatal("NewTransportClient with no HTTP client should fail")
	}
	if _, err := ghclient.NewTransportClient(&http.Client{}); err != nil {
		t.Fatalf("NewTransportClient with an HTTP client: %v", err)
	}
}

// installationTransport answers the App-signed installation read and the installation-signed
// repository list, recording the Authorization header that arrived on each. It is how a test can
// prove the two calls are signed as different principals.
type installationTransport struct {
	seen map[string]string
}

func (i *installationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if i.seen == nil {
		i.seen = map[string]string{}
	}
	if strings.HasSuffix(req.URL.Path, "/access_tokens") {
		return jsonReply(201, map[string]any{
			"token":      "ghs_installation_token",
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}), nil
	}
	i.seen[req.URL.Path] = req.Header.Get("Authorization")
	switch {
	case strings.HasPrefix(req.URL.Path, "/app/installations/"):
		return jsonReply(200, map[string]any{
			"account":              map[string]string{"login": "acme"},
			"permissions":          map[string]string{"issues": "write", "pull_requests": "write", "actions": "read"},
			"repository_selection": "all",
		}), nil
	case req.URL.Path == "/installation/repositories":
		return jsonReply(200, map[string]any{
			"total_count":  2,
			"repositories": []map[string]string{{"full_name": "acme/one"}, {"full_name": "acme/two"}},
		}), nil
	}
	return jsonReply(404, map[string]string{"message": "not found"}), nil
}

func TestTheInstallationReadIsSignedAsTheAppNotAsTheInstallation(t *testing.T) {
	tr := &installationTransport{}
	app, err := githubapp.NewApp(githubapp.AppConfig{
		AppID: 42, InstallationID: 99, PrivateKey: testKey(t), Transport: tr,
	}, ghclient.WithBaseURL("https://api.github.invalid"))
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	install, err := app.Installation(context.Background())
	if err != nil {
		t.Fatalf("Installation: %v", err)
	}
	if install.Account != "acme" {
		t.Fatalf("account = %q, want acme", install.Account)
	}
	if install.Permissions["issues"] != "write" || install.Permissions["actions"] != "read" {
		t.Fatalf("permissions = %v, want issues write and actions read", install.Permissions)
	}
	if !install.AllRepositories {
		t.Fatal("repository_selection all should mean every repository is visible")
	}
	// The App-signed call carries a JWT, which GitHub only accepts on this route. An installation
	// token could not read it, which is what makes a matching answer proof the App's key is right.
	got := tr.seen["/app/installations/99"]
	if !strings.HasPrefix(got, "Bearer ") || strings.Count(got, ".") != 2 {
		t.Fatalf("the installation read carried %q, want a Bearer JWT", got)
	}
	if _, exchanged := tr.seen["/app/installations/99"]; exchanged && strings.Contains(got, "ghs_") {
		t.Fatalf("the installation read used an installation token: %q", got)
	}
}

func TestTheRepositoriesReadIsSignedAsTheInstallation(t *testing.T) {
	tr := &installationTransport{}
	app, err := githubapp.NewApp(githubapp.AppConfig{
		AppID: 42, InstallationID: 99, PrivateKey: testKey(t), Transport: tr,
	}, ghclient.WithBaseURL("https://api.github.invalid"))
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	repos, err := app.Repositories(context.Background())
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	if len(repos) != 2 || repos[0] != "acme/one" || repos[1] != "acme/two" {
		t.Fatalf("repositories = %v, want acme/one and acme/two", repos)
	}
	// Listing repositories needs a real installation token, so this call must have exchanged one.
	got := tr.seen["/installation/repositories"]
	if got != "token ghs_installation_token" {
		t.Fatalf("the repository list carried %q, want an installation token", got)
	}
}

func TestARefusedInstallationReadNamesWhatGitHubSaid(t *testing.T) {
	tr := &refusingTransport{status: 401}
	app, err := githubapp.NewApp(githubapp.AppConfig{
		AppID: 42, InstallationID: 99, PrivateKey: testKey(t), Transport: tr,
	}, ghclient.WithBaseURL("https://api.github.invalid"))
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	_, err = app.Installation(context.Background())
	if err == nil {
		t.Fatal("a refused installation read should fail")
	}
	var apiErr *ghclient.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("error = %v, want an api error with status 401", err)
	}
}

// refusingTransport answers every call with one status and a GitHub-shaped body.
type refusingTransport struct{ status int }

func (r *refusingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return jsonReply(r.status, map[string]string{"message": "Bad credentials"}), nil
}
