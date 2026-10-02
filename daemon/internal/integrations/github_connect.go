package integrations

// This file owns the person-level GitHub connection: the sign-in through the public Marshal GitHub
// App (device flow) and the pasted personal token. Both end in one keychain entry under the
// "github" id, and both are used the same way: GitHubClient answers a client for whichever is saved.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ghclient "github.com/khanblair/marshal/daemon/internal/github"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// DefaultGitHubClientID is the public client id of the Marshal Kanban GitHub App. It is not a secret.
const DefaultGitHubClientID = "Iv23limyHH9MJX4VKiNa"

// DefaultGitHubAppSlug is the address name of the Marshal Kanban GitHub App.
const DefaultGitHubAppSlug = "marshal-kanban"

const (
	githubModeOAuth = "oauth"
	githubModeToken = "token"

	// refreshMargin is how early before expiry a sign-in token is renewed.
	refreshMargin = time.Minute
	// installCheckEvery is how often a sign-in waiting for the App's install looks again.
	installCheckEvery = 3 * time.Second
	// installationsTTL is how long the installed-accounts list is reused when nothing is waiting.
	installationsTTL = 30 * time.Second

	reconnectSentence = "GitHub access has expired. Reconnect GitHub in Settings."
)

// ErrGitHubReconnect means the saved sign-in can no longer be renewed, so the person must sign in
// again. The connection's row reads "needs attention" until they do.
var ErrGitHubReconnect = errors.New("the saved GitHub sign-in expired and must be renewed")

// GitHubClient is what Marshal calls GitHub with, however GitHub was connected.
type GitHubClient interface {
	ghclient.Client
	ListWorkflowRuns(ctx context.Context, repo ghclient.Repository, branch string) ([]ghclient.WorkflowRun, error)
	RerunFailedJobs(ctx context.Context, repo ghclient.Repository, runID int64) error
	FailedLog(ctx context.Context, repo ghclient.Repository, runID int64, maxBytes int) (string, error)
}

// githubUserState is everything the person-level connection keeps in memory.
type githubUserState struct {
	clientID, appSlug string
	apiBase, authBase string
	httpOverride      *http.Client
	flowMu            sync.Mutex
	flow              *githubFlow
	tokMu             sync.Mutex
	secrets           *githubSecrets
	broken            atomic.Bool
	instMu            sync.Mutex
	instAt            time.Time
	instCache         []githubapp.UserInstallation
	instCacheValid    bool
	defaultHTTP       *http.Client
}

func (g *githubUserState) configure(opts Options) {
	g.clientID, g.appSlug = opts.GitHubClientID, opts.GitHubAppSlug
	if g.clientID == "" {
		g.clientID = DefaultGitHubClientID
	}
	if g.appSlug == "" {
		g.appSlug = DefaultGitHubAppSlug
	}
	g.apiBase, g.authBase, g.httpOverride = opts.GitHubAPIBaseURL, opts.GitHubAuthBaseURL, opts.GitHubHTTPClient
	g.defaultHTTP = &http.Client{Timeout: githubapp.DefaultTimeout}
}

// httpClient is the client for GitHub's sign-in and user calls.
func (g *githubUserState) httpClient() *http.Client {
	if g.httpOverride != nil {
		return g.httpOverride
	}
	return g.defaultHTTP
}

// githubFlow is one sign-in in progress, or one that ended and has not been read away yet.
type githubFlow struct {
	state       protocol.GitHubConnectState
	code        githubapp.DeviceCode
	expiresAt   time.Time
	interval    time.Duration
	nextPoll    time.Time
	nextInstall time.Time
	message     string
	login       string
}

func (s *Service) deviceFlow() (*githubapp.DeviceFlow, error) {
	return githubapp.NewDeviceFlow(s.gh.clientID, s.gh.authBase, s.gh.httpClient(), s.now)
}

func (s *Service) userAPI() *githubapp.UserAPI {
	return githubapp.NewUserAPI(s.gh.apiBase, s.gh.httpClient())
}

func (s *Service) githubInstallURL() string {
	base := s.gh.authBase
	if base == "" {
		base = githubapp.DefaultAuthBaseURL
	}
	return strings.TrimRight(base, "/") + "/apps/" + s.gh.appSlug + "/installations/new"
}

func (s *Service) githubNeedsReconnect() bool { return s.gh.broken.Load() }

// forgetGitHubUser ends any sign-in in progress and drops what is cached about the connection.
func (s *Service) forgetGitHubUser() {
	s.gh.flowMu.Lock()
	s.gh.flow = nil
	s.gh.flowMu.Unlock()
	s.resetGitHubUserCache()
}

func (s *Service) resetGitHubUserCache() {
	s.gh.tokMu.Lock()
	s.gh.secrets = nil
	s.gh.tokMu.Unlock()
	s.gh.instMu.Lock()
	s.gh.instCache, s.gh.instCacheValid = nil, false
	s.gh.instMu.Unlock()
	s.gh.broken.Store(false)
}

// StartGitHubConnect asks GitHub for a sign-in code and begins waiting for it to be approved. A
// sign-in already waiting is replaced.
func (s *Service) StartGitHubConnect(ctx context.Context) (protocol.GitHubConnect, error) {
	device, err := s.deviceFlow()
	if err != nil {
		return protocol.GitHubConnect{}, err
	}
	code, err := device.Start(ctx)
	switch {
	case errors.Is(err, githubapp.ErrDeviceFlowDisabled):
		return protocol.GitHubConnect{}, protocol.Unavailable(
			"GitHub sign-in is switched off for the Marshal GitHub App. Paste a token instead.")
	case err != nil:
		s.log.Warn("could not start the GitHub sign-in", "err", err)
		return protocol.GitHubConnect{}, protocol.Unavailable(
			"Marshal could not reach GitHub. Check the connection and try again.")
	}
	now := s.now()
	flow := &githubFlow{
		state: protocol.GitHubConnectStatePending, code: code, expiresAt: now.Add(code.ExpiresIn),
		interval: code.Interval, nextPoll: now.Add(code.Interval),
	}
	s.gh.flowMu.Lock()
	s.gh.flow = flow
	view := s.flowView(flow)
	s.gh.flowMu.Unlock()
	return view, nil
}

// ReadGitHubConnect moves the sign-in on as far as GitHub allows and answers where it is. The second
// result is true exactly once, on the read that finished the connection, so the caller can test it.
func (s *Service) ReadGitHubConnect(ctx context.Context) (protocol.GitHubConnect, bool, error) {
	s.gh.flowMu.Lock()
	defer s.gh.flowMu.Unlock()
	flow := s.gh.flow
	if flow == nil {
		return s.storedGitHubConnect(ctx), false, nil
	}
	if s.advanceGitHubFlow(ctx, flow) {
		s.gh.flow = nil
		return s.storedGitHubConnect(ctx), true, nil
	}
	return s.flowView(flow), false, nil
}

// CancelGitHubConnect forgets a sign-in in progress and answers what is connected now.
func (s *Service) CancelGitHubConnect(ctx context.Context) protocol.GitHubConnect {
	s.gh.flowMu.Lock()
	defer s.gh.flowMu.Unlock()
	s.gh.flow = nil
	return s.storedGitHubConnect(ctx)
}

// advanceGitHubFlow polls GitHub for the approval, then looks for the App's installation. It reports
// true once the person is signed in and the App is installed somewhere.
func (s *Service) advanceGitHubFlow(ctx context.Context, f *githubFlow) bool {
	now := s.now()
	if f.state == protocol.GitHubConnectStatePending {
		if !now.Before(f.expiresAt) {
			f.state, f.message = protocol.GitHubConnectStateExpired, "The code expired. Try again."
			return false
		}
		if now.Before(f.nextPoll) {
			return false
		}
		s.pollGitHubApproval(ctx, f, now)
	}
	if f.state != protocol.GitHubConnectStateNeedsInstall || now.Before(f.nextInstall) {
		return false
	}
	f.nextInstall = now.Add(installCheckEvery)
	installs, err := s.freshInstallations(ctx)
	if err != nil {
		s.log.Warn("could not list the GitHub App's installations", "err", err)
		return false
	}
	return len(installs) > 0
}

func (s *Service) pollGitHubApproval(ctx context.Context, f *githubFlow, now time.Time) {
	device, err := s.deviceFlow()
	if err != nil {
		f.state, f.message = protocol.GitHubConnectStateFailed, err.Error()
		return
	}
	set, err := device.Poll(ctx, f.code.DeviceCode)
	f.nextPoll = now.Add(f.interval)
	var slow *githubapp.SlowDownError
	switch {
	case err == nil:
		s.finishGitHubSignIn(ctx, f, set)
	case errors.Is(err, githubapp.ErrAuthorizationPending):
	case errors.As(err, &slow):
		f.interval += 5 * time.Second
		if slow.Interval > 0 {
			f.interval = slow.Interval
		}
		f.nextPoll = now.Add(f.interval)
	case errors.Is(err, githubapp.ErrExpiredCode):
		f.state, f.message = protocol.GitHubConnectStateExpired, "The code expired. Try again."
	case errors.Is(err, githubapp.ErrAccessDenied):
		f.state, f.message = protocol.GitHubConnectStateDenied, "You refused the code on GitHub. Try again to sign in."
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		f.message = "Marshal could not reach GitHub, so it is trying again."
	default:
		s.log.Warn("the GitHub sign-in could not be finished", "err", err)
		f.state, f.message = protocol.GitHubConnectStateFailed, "GitHub did not finish the sign-in. Try again."
	}
}

// finishGitHubSignIn learns who signed in and stores the token. The token is only stored once it is
// known to work.
func (s *Service) finishGitHubSignIn(ctx context.Context, f *githubFlow, set githubapp.TokenSet) {
	who, err := s.userAPI().Whoami(ctx, set.AccessToken)
	if err != nil {
		s.log.Warn("GitHub issued a token that could not be used", "err", err)
		f.state, f.message = protocol.GitHubConnectStateFailed, "GitHub signed you in, but Marshal could not use the token. Try again."
		return
	}
	secrets := githubSecrets{
		Mode: githubModeOAuth, Token: set.AccessToken, RefreshToken: set.RefreshToken,
		ExpiresAt: millis(set.ExpiresAt), RefreshExpiresAt: millis(set.RefreshExpiresAt),
	}
	if err := s.persistGitHubUser(ctx, secrets, who.Login); err != nil {
		s.log.Error("could not save the GitHub sign-in", "err", err)
		f.state, f.message = protocol.GitHubConnectStateFailed, "Marshal could not save the sign-in. Try again."
		return
	}
	f.state, f.login, f.message = protocol.GitHubConnectStateNeedsInstall, who.Login, ""
}

func millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMillis(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// persistGitHubUser saves a person-level connection, secret first and settings second, and drops the
// caches. It takes the token lock so a renewal in flight cannot write an older token over it.
func (s *Service) persistGitHubUser(ctx context.Context, secrets githubSecrets, login string) error {
	s.gh.tokMu.Lock()
	defer s.gh.tokMu.Unlock()
	raw, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("write the GitHub connection's secrets: %w", err)
	}
	if err := s.keys.Set(GitHubID, string(raw)); err != nil {
		return fmt.Errorf("save the GitHub connection's secret in the keychain: %w", err)
	}
	config, err := json.Marshal(githubConfig{Mode: secrets.Mode, Login: login})
	if err != nil {
		return fmt.Errorf("write the GitHub connection's settings: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID: GitHubID, Kind: KindGitHub, ConfigJSON: string(config), KeychainRef: GitHubID,
		})
	})
	if err != nil {
		return fmt.Errorf("save the GitHub connection's settings: %w", err)
	}
	s.gh.secrets = &secrets
	s.forgetGitHub()
	s.gh.instMu.Lock()
	s.gh.instCache, s.gh.instCacheValid = nil, false
	s.gh.instMu.Unlock()
	s.gh.broken.Store(false)
	return nil
}

// githubSecretsLocked reads the keychain entry once and keeps it. The caller holds tokMu.
func (s *Service) githubSecretsLocked() (githubSecrets, error) {
	if s.gh.secrets != nil {
		return *s.gh.secrets, nil
	}
	raw, err := s.keys.Get(GitHubID)
	if err != nil && !errors.Is(err, security.ErrNoKey) {
		return githubSecrets{}, fmt.Errorf("read the GitHub connection's secret from the keychain: %w", err)
	}
	secrets, err := parseGitHubSecrets(raw)
	if err != nil {
		return githubSecrets{}, err
	}
	s.gh.secrets = &secrets
	return secrets, nil
}

// githubToken answers the token to call GitHub with and how it was connected. A sign-in token that is
// about to expire is renewed first, by one caller at a time: GitHub rotates the refresh token on
// every use, so two renewals at once would leave one of them holding a dead token.
func (s *Service) githubToken(ctx context.Context) (token, mode string, err error) {
	s.gh.tokMu.Lock()
	defer s.gh.tokMu.Unlock()
	secrets, err := s.githubSecretsLocked()
	if err != nil {
		return "", "", err
	}
	if secrets.Token == "" {
		return "", "", ErrNotConnected
	}
	if secrets.Mode != githubModeOAuth || secrets.ExpiresAt == 0 ||
		s.now().Add(refreshMargin).Before(fromMillis(secrets.ExpiresAt)) {
		return secrets.Token, secrets.Mode, nil
	}
	renewed, err := s.renewGitHubToken(ctx, &secrets)
	if err != nil {
		return "", "", err
	}
	return renewed, secrets.Mode, nil
}

// renewGitHubToken trades the refresh token for a new pair and saves it. A pair the keychain would
// not take is kept in memory, because the old refresh token is already spent. The caller holds tokMu.
func (s *Service) renewGitHubToken(ctx context.Context, secrets *githubSecrets) (string, error) {
	spent := s.gh.broken.Load() || secrets.RefreshToken == "" ||
		(secrets.RefreshExpiresAt != 0 && !s.now().Before(fromMillis(secrets.RefreshExpiresAt)))
	if spent {
		s.gh.broken.Store(true)
		return "", ErrGitHubReconnect
	}
	device, err := s.deviceFlow()
	if err != nil {
		return "", err
	}
	set, err := device.Refresh(ctx, secrets.RefreshToken)
	if errors.Is(err, githubapp.ErrRefreshRejected) {
		s.gh.broken.Store(true)
		return "", ErrGitHubReconnect
	}
	if err != nil {
		return "", fmt.Errorf("renew the GitHub sign-in: %w", err)
	}
	secrets.Token, secrets.RefreshToken = set.AccessToken, set.RefreshToken
	secrets.ExpiresAt, secrets.RefreshExpiresAt = millis(set.ExpiresAt), millis(set.RefreshExpiresAt)
	s.gh.secrets = secrets
	raw, err := json.Marshal(secrets)
	if err == nil {
		err = s.keys.Set(GitHubID, string(raw))
	}
	if err != nil {
		s.log.Error("could not save the renewed GitHub sign-in", "err", err)
	}
	return secrets.Token, nil
}

// GitHubClient answers a client for the connected GitHub, whichever way it was connected: a sign-in,
// a pasted token, or a GitHub App saved by hand. ErrNotConnected means none is saved.
func (s *Service) GitHubClient(ctx context.Context) (GitHubClient, error) {
	token, _, err := s.githubToken(ctx)
	if errors.Is(err, ErrNotConnected) {
		app, appErr := s.App(ctx)
		if appErr != nil {
			return nil, appErr
		}
		return app.Client(), nil
	}
	if err != nil {
		return nil, err
	}
	opts := []ghclient.Option{ghclient.WithBaseURL(s.gh.apiBase)}
	if s.gh.httpOverride != nil {
		opts = append(opts, ghclient.WithHTTPClient(s.gh.httpOverride))
	}
	return ghclient.NewTokenClient(token, opts...)
}

// installations lists the accounts the App is installed on for the signed-in user.
func (s *Service) installations(ctx context.Context) ([]githubapp.UserInstallation, error) {
	token, _, err := s.githubToken(ctx)
	if err != nil {
		return nil, err
	}
	return s.userAPI().Installations(ctx, token)
}

// freshInstallations reads the installations from GitHub and refreshes the cache.
func (s *Service) freshInstallations(ctx context.Context) ([]githubapp.UserInstallation, error) {
	installs, err := s.installations(ctx)
	if err != nil {
		return nil, err
	}
	s.gh.instMu.Lock()
	s.gh.instCache, s.gh.instCacheValid, s.gh.instAt = installs, true, s.now()
	s.gh.instMu.Unlock()
	return installs, nil
}

func (s *Service) cachedInstallations(ctx context.Context) ([]githubapp.UserInstallation, error) {
	s.gh.instMu.Lock()
	if s.gh.instCacheValid && s.now().Sub(s.gh.instAt) < installationsTTL {
		out := s.gh.instCache
		s.gh.instMu.Unlock()
		return out, nil
	}
	s.gh.instMu.Unlock()
	return s.freshInstallations(ctx)
}

// storedGitHubConnect answers the saved connection as the screen shows it: connected with who and
// where, waiting for the App to be installed, or idle when nothing is saved.
func (s *Service) storedGitHubConnect(ctx context.Context) protocol.GitHubConnect {
	view := protocol.GitHubConnect{State: protocol.GitHubConnectStateIdle, Installations: []protocol.GitHubInstallation{}}
	s.gh.tokMu.Lock()
	secrets, err := s.githubSecretsLocked()
	s.gh.tokMu.Unlock()
	if err != nil {
		s.log.Warn("could not read the GitHub connection", "err", err)
		return view
	}
	if secrets.Token == "" {
		return view
	}
	view.Mode, view.Login = secrets.Mode, s.githubLogin(ctx)
	view.State = protocol.GitHubConnectStateConnected
	if s.githubNeedsReconnect() {
		view.State, view.Message = protocol.GitHubConnectStateFailed, reconnectSentence
		return view
	}
	if secrets.Mode != githubModeOAuth {
		return view
	}
	installs, err := s.cachedInstallations(ctx)
	switch {
	case errors.Is(err, ErrGitHubReconnect):
		view.State, view.Message = protocol.GitHubConnectStateFailed, reconnectSentence
	case err != nil:
		s.log.Warn("could not list the GitHub App's installations", "err", err)
		view.Message = "Marshal could not list the accounts the app is installed on."
	default:
		for _, in := range installs {
			view.Installations = append(view.Installations, protocol.GitHubInstallation{
				Account: in.Account, Kind: in.Kind, AllRepositories: in.AllRepositories,
			})
		}
		view.InstallURL = s.githubInstallURL()
		if len(installs) == 0 {
			view.State = protocol.GitHubConnectStateNeedsInstall
		}
	}
	return view
}

func (s *Service) flowView(f *githubFlow) protocol.GitHubConnect {
	view := protocol.GitHubConnect{
		State: f.state, Login: f.login, Message: f.message, Installations: []protocol.GitHubInstallation{},
	}
	switch f.state {
	case protocol.GitHubConnectStatePending:
		expires := protocol.NewTimestamp(f.expiresAt)
		view.UserCode, view.VerificationURI, view.ExpiresAt = f.code.UserCode, f.code.VerificationURI, &expires
	case protocol.GitHubConnectStateNeedsInstall:
		view.InstallURL = s.githubInstallURL()
	}
	return view
}

// githubLogin is who the saved connection is signed in as, from its row.
func (s *Service) githubLogin(ctx context.Context) string {
	var config githubConfig
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, GitHubID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && row.ConfigJSON == "") {
			return nil
		}
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(row.ConfigJSON), &config)
	})
	if err != nil {
		s.log.Warn("could not read the GitHub login", "err", err)
	}
	return config.Login
}

// SaveGitHubToken stores a pasted personal token, replacing whatever GitHub connection was there. The
// token is checked against GitHub first, so one GitHub would refuse is never saved.
func (s *Service) SaveGitHubToken(ctx context.Context, req protocol.SaveGitHubTokenRequest) error {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return protocol.InvalidArgument("Paste a GitHub personal access token.")
	}
	who, err := s.userAPI().Whoami(ctx, token)
	if err != nil {
		var apiErr *ghclient.APIError
		if errors.As(err, &apiErr) && apiErr.Unauthorized() {
			return protocol.InvalidArgument("GitHub did not accept that token. Copy the whole token and try again.")
		}
		s.log.Warn("could not check a pasted GitHub token", "err", err)
		return protocol.Unavailable("Marshal could not reach GitHub to check the token. Try again.")
	}
	if err := s.persistGitHubUser(ctx, githubSecrets{Mode: githubModeToken, Token: token}, who.Login); err != nil {
		return err
	}
	s.gh.flowMu.Lock()
	s.gh.flow = nil
	s.gh.flowMu.Unlock()
	return nil
}

// TestGitHubToken checks a pasted token without saving it.
func (s *Service) TestGitHubToken(ctx context.Context, req protocol.SaveGitHubTokenRequest) (protocol.TestResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return protocol.TestResult{}, protocol.InvalidArgument("Paste a GitHub personal access token to test it.")
	}
	return s.testGitHubToken(ctx, token), nil
}

// testGitHubUser is the stored connection's test, for a sign-in or a pasted token.
func (s *Service) testGitHubUser(ctx context.Context, token, mode string) protocol.TestResult {
	if mode == githubModeToken {
		return s.testGitHubToken(ctx, token)
	}
	checks := make([]protocol.TestCheck, 0, 5)
	api := s.userAPI()
	who, whoErr := api.Whoami(ctx, token)
	checks = append(checks, signedInCheck(who, whoErr))
	installs, instErr := api.Installations(ctx, token)
	checks = append(checks, installedCheck(installs, instErr, s.githubInstallURL()))
	repos, reposErr := countInstallationRepositories(ctx, api, token, installs, instErr)
	checks = append(checks, repositoriesCountCheck(repos, reposErr, instErr, installs))
	checks = append(checks, userPermissionsCheck(installs, instErr))
	works := fmt.Sprintf("Connected as @%s. Marshal can see %s on %s.", who.Login,
		plural(repos, "repository", "repositories"), plural(len(installs), "account", "accounts"))
	return protocol.NewTestResult(GitHubID, withSummary(checks, works,
		"GitHub is connected, with something to check."), s.now())
}

// testGitHubToken runs the checks for a personal token, saved or not.
func (s *Service) testGitHubToken(ctx context.Context, token string) protocol.TestResult {
	api := s.userAPI()
	who, whoErr := api.Whoami(ctx, token)
	checks := []protocol.TestCheck{signedInCheck(who, whoErr)}
	if whoErr == nil {
		checks = append(checks, tokenScopesCheck(who))
		has, err := api.HasRepositories(ctx, token)
		checks = append(checks, tokenRepositoriesCheck(has, err))
	}
	works := fmt.Sprintf("Connected as @%s with a personal access token.", who.Login)
	return protocol.NewTestResult(GitHubID, withSummary(checks, works,
		"The token works, with something to check."), s.now())
}

func withSummary(checks []protocol.TestCheck, works, partly string) []protocol.TestCheck {
	return append([]protocol.TestCheck{summaryCheck(checks, works, partly)}, checks...)
}

// CheckSignedIn and its siblings name the checks a person-level GitHub test reports.
const (
	CheckSignedIn   = "Signed in"
	CheckInstalled  = "App installed"
	CheckTokenScope = "Token access"
)

// userGitHubFailure words a failed call for a person-level connection. A refused token means sign in
// again; anything else reads as it does for the App test.
func userGitHubFailure(err error, what string) (message, fix string) {
	var apiErr *ghclient.APIError
	if errors.As(err, &apiErr) && apiErr.Unauthorized() {
		return "GitHub does not accept the saved sign-in or token.",
			"Choose Connect GitHub and sign in again, or paste a new token."
	}
	return githubFailure(err, what)
}

func signedInCheck(who githubapp.Identity, err error) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckSignedIn}
	if err != nil {
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = userGitHubFailure(err, "Marshal could not read your GitHub account.")
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = fmt.Sprintf("Signed in as @%s.", who.Login)
	return check
}

func installedCheck(installs []githubapp.UserInstallation, err error, installURL string) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckInstalled}
	switch {
	case err != nil:
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = userGitHubFailure(err, "Marshal could not list where the app is installed.")
	case len(installs) == 0:
		check.State = protocol.CheckStateFailed
		check.Message = "The Marshal GitHub App is not installed on any of your accounts."
		check.Fix = "Install Marshal Kanban, choosing All repositories: " + installURL
	default:
		names := make([]string, 0, len(installs))
		for _, in := range installs {
			names = append(names, "@"+in.Account)
		}
		check.State = protocol.CheckStatePassed
		check.Message = "The app is installed on " + joinWords(names) + "."
	}
	return check
}

func countInstallationRepositories(
	ctx context.Context, api *githubapp.UserAPI, token string, installs []githubapp.UserInstallation, instErr error,
) (int, error) {
	if instErr != nil {
		return 0, instErr
	}
	total := 0
	for _, in := range installs {
		n, err := api.InstallationRepositories(ctx, token, in.ID)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

func repositoriesCountCheck(repos int, err, instErr error, installs []githubapp.UserInstallation) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckRepositories}
	switch {
	case instErr != nil:
		check.State = protocol.CheckStateWarning
		check.Message = "Marshal could not count repositories, because it could not list the installations."
	case err != nil:
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = userGitHubFailure(err, "Marshal could not list your repositories.")
	case repos == 0:
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = noRepositoriesSentence(installs)
	default:
		check.State = protocol.CheckStatePassed
		check.Message = fmt.Sprintf("Marshal can see %s.", plural(repos, "repository", "repositories"))
	}
	return check
}

// noRepositoriesSentence names why the app sees nothing: an install limited to selected
// repositories with none picked, or an account that simply has no repositories yet.
func noRepositoriesSentence(installs []githubapp.UserInstallation) (message, fix string) {
	for _, in := range installs {
		if !in.AllRepositories {
			return "The app is installed with selected repositories, and none are selected.",
				"Open the installation on GitHub and select the repositories Marshal should see, or choose All repositories."
		}
	}
	return "The accounts the app is installed on have no repositories yet.",
		"Sign in with the GitHub account that owns your repositories, or create a repository first."
}

// userRequiredPermissions is what Marshal needs of the App on a signed-in connection.
func userRequiredPermissions() []struct{ Name, Level string } {
	return []struct{ Name, Level string }{
		{"issues", permWrite}, {"pull_requests", permWrite}, {"actions", permWrite},
		{"contents", permWrite}, {"checks", "read"},
	}
}

func userPermissionsCheck(installs []githubapp.UserInstallation, err error) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckPermissions}
	if err != nil || len(installs) == 0 {
		check.State = protocol.CheckStateWarning
		check.Message = "Marshal could not read the app's permissions, because it has no installation to read."
		return check
	}
	var lacking []string
	for _, want := range userRequiredPermissions() {
		for _, in := range installs {
			if !permissionAtLeast(in.Permissions[want.Name], want.Level) {
				lacking = append(lacking, fmt.Sprintf("%s (%s)", want.Name, want.Level))
				break
			}
		}
	}
	if len(lacking) > 0 {
		check.State = protocol.CheckStateFailed
		check.Message = "The app is missing " + joinWords(lacking) + " on an installation."
		check.Fix = "Open the installation on GitHub and accept the app's updated permissions."
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = "The app may read and write code, issues, pull requests, and Actions."
	return check
}

func tokenScopesCheck(who githubapp.Identity) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckTokenScope}
	if !who.ScopesKnown {
		check.State = protocol.CheckStateWarning
		check.Message = "GitHub does not list this token's permissions, so Marshal finds out when it first uses them."
		check.Fix = "Give a fine-grained token read and write access to Contents, Pull requests, Issues and Actions."
		return check
	}
	if !who.HasScope("repo") {
		check.State = protocol.CheckStateFailed
		check.Message = "The token does not have the repo scope."
		check.Fix = "Create a classic token with the repo scope, then paste it here."
		return check
	}
	check.State = protocol.CheckStatePassed
	check.Message = "The token has the repo scope."
	return check
}

func tokenRepositoriesCheck(has bool, err error) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckRepositories}
	switch {
	case err != nil:
		check.State = protocol.CheckStateFailed
		check.Message, check.Fix = userGitHubFailure(err, "Marshal could not list your repositories.")
	case !has:
		check.State = protocol.CheckStateFailed
		check.Message = "The token can see no repositories."
		check.Fix = "Give the token access to at least one repository."
	default:
		check.State = protocol.CheckStatePassed
		check.Message = "Marshal can list your repositories."
	}
	return check
}

// githubNotConnectedTest is the failed result a test answers when nothing is saved.
func (s *Service) githubNotConnectedTest() protocol.TestResult {
	return protocol.NewTestResult(GitHubID, []protocol.TestCheck{{
		Name: CheckSummary, State: protocol.CheckStateFailed,
		Message: "GitHub is not connected, so Marshal cannot use it.",
		Fix:     "Choose Connect GitHub in Settings, under Integrations.",
	}}, s.now())
}

// githubReconnectTest is the failed result a test answers when the saved sign-in expired.
func (s *Service) githubReconnectTest() protocol.TestResult {
	return protocol.NewTestResult(GitHubID, []protocol.TestCheck{{
		Name: CheckSummary, State: protocol.CheckStateFailed,
		Message: "The saved GitHub sign-in expired.",
		Fix:     "Choose Connect GitHub and sign in again.",
	}}, s.now())
}
