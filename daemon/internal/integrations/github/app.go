package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"

	ghclient "github.com/khanblair/marshal/daemon/internal/github"
)

// DefaultTimeout bounds every call the App makes. The same twenty seconds the personal-token client
// uses: a GitHub that has not answered in that long is not about to, and the person should be told
// rather than left watching a spinner.
const DefaultTimeout = 20 * time.Second

// maxReadBody bounds how much of an answer is read, so a server that answers with a whole document
// cannot be made to fill memory. It is larger than the client's own error ceiling because a
// repository list is a real answer that has to be decoded.
const maxReadBody = 64 << 10

// AppConfig is one GitHub App installation: the App's numeric id, the id of its installation on the
// owner's account, and the App's private key in PEM form. It is what the owner saves when they set
// the App up (docs/backend-checklist.md section 3); nothing in Marshal creates one.
type AppConfig struct {
	// AppID is the App's own numeric id, from its settings page.
	AppID int64
	// InstallationID is the numeric id of the installation on the owner's account or organization.
	// One App can be installed in more than one place, and this is the one Marshal acts as.
	InstallationID int64
	// PrivateKey is the App's private key, in PEM form, exactly as GitHub generated it.
	PrivateKey []byte
	// BaseURL is the API's own address. Empty means GitHub's public API; a test points it at a
	// fake server.
	BaseURL string
	// Transport is the HTTP transport the App's calls are made over, so a test can point them at a
	// fake server. Nil means http.DefaultTransport.
	Transport http.RoundTripper
}

// Validate refuses an App that cannot be used, naming the one thing that is missing, so a save can
// tell a person which field to fill in rather than failing at the first call.
func (cfg AppConfig) Validate() error {
	if cfg.AppID <= 0 {
		return errors.New("a GitHub App needs its app id")
	}
	if cfg.InstallationID <= 0 {
		return errors.New("a GitHub App needs its installation id")
	}
	if len(cfg.PrivateKey) == 0 {
		return errors.New("a GitHub App needs its private key")
	}
	return nil
}

// App is one GitHub App installation as Marshal uses it: the client that opens pull requests, reads
// reviews, and lists checks, plus the two authorized HTTP clients a connection test needs - one
// signed as the App itself, one signed as the installation.
//
// It is one type rather than two clients because all three must agree about which App they are: a
// test that checked a different App's installation than the one Marshal acts as would be worse than
// no test.
type App struct {
	client         *ghclient.TransportClient
	asApp          *http.Client
	asInstall      *http.Client
	baseURL        string
	installationID int64
}

// NewApp builds one App. The client it returns is what Phase 5's github.Client interface was
// defined for: a caller that holds a client cannot tell an App from a personal token, which is the
// point.
//
// The App never sends a bearer token of its own. ghinstallation signs each request with a JWT made
// from the App's key, exchanges it for a short-lived installation token, and caches that token
// until it is nearly expired, so a long-running daemon keeps working across token rollovers.
func NewApp(cfg AppConfig, opts ...ghclient.Option) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	base := cfg.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	install, err := ghinstallation.New(base, cfg.AppID, cfg.InstallationID, cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("build the GitHub App's transport: %w", err)
	}
	// The second transport signs as the App itself. GitHub answers /app/installations/{id} only to
	// that one, and it is where the installation's permissions come from.
	asApp, err := ghinstallation.NewAppsTransport(base, cfg.AppID, cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("build the GitHub App's own transport: %w", err)
	}
	client, err := ghclient.NewTransportClient(
		&http.Client{Transport: install, Timeout: DefaultTimeout}, opts...)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(cfg.BaseURL, "/")
	if url == "" {
		url = ghclient.DefaultBaseURL
	}
	return &App{
		client:         client,
		asApp:          &http.Client{Transport: asApp, Timeout: DefaultTimeout},
		asInstall:      &http.Client{Transport: install, Timeout: DefaultTimeout},
		baseURL:        url,
		installationID: cfg.InstallationID,
	}, nil
}

// NewAppClient builds the App's client alone, for a caller that only needs to talk to GitHub.
func NewAppClient(cfg AppConfig, opts ...ghclient.Option) (*ghclient.TransportClient, error) {
	app, err := NewApp(cfg, opts...)
	if err != nil {
		return nil, err
	}
	return app.client, nil
}

// Client is the App's github.Client, for the pull-request, review, and check services.
func (a *App) Client() *ghclient.TransportClient { return a.client }

// Installation is what GitHub says about one installation: the account it is on, what it is allowed
// to do, and whether it can see every repository or only the ones picked for it.
type Installation struct {
	// Account is the user or organization the App is installed on.
	Account string
	// Permissions is what the installation may do, by GitHub's own names: "issues",
	// "pull_requests", "actions", "contents", and the rest, each "read", "write", or absent.
	Permissions map[string]string
	// AllRepositories is true when the installation can see every repository the account owns.
	AllRepositories bool
}

// Installation reads the App's own installation. It is signed as the App and not as the
// installation, which is what makes it proof that the App's id and key are right: an installation
// token cannot read this route.
func (a *App) Installation(ctx context.Context) (Installation, error) {
	var answer struct {
		Account struct {
			Login string `json:"login"`
		} `json:"account"`
		Permissions      map[string]string `json:"permissions"`
		RepositorySelect string            `json:"repository_selection"`
	}
	path := "/app/installations/" + strconv.FormatInt(a.installationID, 10)
	if err := a.get(ctx, a.asApp, path, &answer); err != nil {
		return Installation{}, err
	}
	return Installation{
		Account:         answer.Account.Login,
		Permissions:     answer.Permissions,
		AllRepositories: answer.RepositorySelect == "all",
	}, nil
}

// Repositories lists the repositories the installation can see, which is what proves an
// installation token could be minted and that the person picked the right installation.
func (a *App) Repositories(ctx context.Context) ([]string, error) {
	var answer struct {
		TotalCount   int `json:"total_count"`
		Repositories []struct {
			FullName string `json:"full_name"`
		} `json:"repositories"`
	}
	if err := a.get(ctx, a.asInstall, "/installation/repositories", &answer); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(answer.Repositories))
	for _, repo := range answer.Repositories {
		if repo.FullName != "" {
			names = append(names, repo.FullName)
		}
	}
	return names, nil
}

// get makes one authorized read and decodes it. A refused answer becomes an error naming what
// GitHub said, because the connection test turns that into the sentence a person reads.
func (a *App) get(ctx context.Context, client *http.Client, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build the request for %s: %w", path, err)
	}
	req.Header.Set("Accept", ghclient.AcceptHeader)
	req.Header.Set("X-GitHub-Api-Version", ghclient.APIVersion)
	reply, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = reply.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(reply.Body, maxReadBody))
	if err != nil {
		return fmt.Errorf("read GitHub's answer: %w", err)
	}
	if reply.StatusCode < 200 || reply.StatusCode > 299 {
		return &ghclient.APIError{
			Status:    reply.StatusCode,
			Message:   messageOf(body),
			RequestID: reply.Header.Get("X-GitHub-Request-Id"),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("read GitHub's answer: %w", err)
	}
	return nil
}

// messageOf pulls the sentence out of a failed answer: GitHub's own "message" when it gave one, the
// whole body when it did not, and a stand-in when even that is empty. It is the same reading the
// personal-token client does, so both clients report a refusal the same way.
func messageOf(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &payload)
	if message := strings.TrimSpace(payload.Message); message != "" {
		return message
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		return text
	}
	return "no message"
}
