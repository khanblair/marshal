package integrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ghclient "github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeGitHubUser answers GitHub's sign-in endpoints and the user calls Marshal makes, with no network.
type fakeGitHubUser struct {
	srv *httptest.Server

	mu              sync.Mutex
	polls           int
	refreshes       int
	approveOnPoll   int
	deny            bool
	expiresIn       int
	login           string
	scopes          string
	rejectRefresh   bool
	validAccess     map[string]bool
	validRefresh    string
	installs        []map[string]any
	repoCount       int
	tokenSeq        int
	lastAuth        string
	secretOnRefresh bool
}

func newFakeGitHubUser(t *testing.T) *fakeGitHubUser {
	t.Helper()
	f := &fakeGitHubUser{approveOnPoll: 1, expiresIn: 28800, login: "octo", repoCount: 3, validAccess: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"device_code": "dc-1", "user_code": "ABCD-1234", "verification_uri": f.srv.URL + "/login/device",
			"expires_in": 900, "interval": 5,
		})
	})
	mux.HandleFunc("POST /login/oauth/access_token", f.token)
	mux.HandleFunc("GET /user", f.user)
	mux.HandleFunc("GET /user/installations", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"total_count": len(f.installs), "installations": f.installs})
	})
	mux.HandleFunc("GET /user/installations/{id}/repositories", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"total_count": f.repoCount})
	})
	mux.HandleFunc("GET /user/repos", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, []map[string]any{{"full_name": "octo/app"}})
	})
	mux.HandleFunc("GET /repos/acme/web/commits/main/check-runs", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"total_count": 0, "check_runs": []any{}})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeGitHubUser) mintLocked() map[string]any {
	f.tokenSeq++
	access, refresh := fmt.Sprintf("ghu_%d", f.tokenSeq), fmt.Sprintf("ghr_%d", f.tokenSeq)
	f.validAccess[access], f.validRefresh = true, refresh
	answer := map[string]any{"access_token": access, "token_type": "bearer", "scope": ""}
	if f.expiresIn > 0 {
		answer["expires_in"], answer["refresh_token"], answer["refresh_token_expires_in"] = f.expiresIn, refresh, 15897600
	}
	return answer
}

func (f *fakeGitHubUser) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Form.Get("grant_type") {
	case "refresh_token":
		f.refreshes++
		f.secretOnRefresh = f.secretOnRefresh || r.Form.Has("client_secret")
		if f.rejectRefresh || r.Form.Get("refresh_token") != f.validRefresh {
			writeJSON(w, http.StatusOK, map[string]any{"error": "bad_refresh_token"})
			return
		}
		writeJSON(w, http.StatusOK, f.mintLocked())
	default:
		f.polls++
		switch {
		case f.deny:
			writeJSON(w, http.StatusOK, map[string]any{"error": "access_denied"})
		case f.polls < f.approveOnPoll:
			writeJSON(w, http.StatusOK, map[string]any{"error": "authorization_pending"})
		default:
			writeJSON(w, http.StatusOK, f.mintLocked())
		}
	}
}

func (f *fakeGitHubUser) authorized(w http.ResponseWriter, r *http.Request) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	f.mu.Lock()
	f.lastAuth = r.Header.Get("Authorization")
	ok := f.validAccess[token]
	f.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "Bad credentials"})
	}
	return ok
}

func (f *fakeGitHubUser) user(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	if f.scopes != "" {
		w.Header().Set("X-OAuth-Scopes", f.scopes)
	}
	writeJSON(w, http.StatusOK, map[string]any{"login": f.login})
}

func (f *fakeGitHubUser) install(account, kind string, perms map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installs = append(f.installs, map[string]any{
		"id": 100 + len(f.installs), "account": map[string]any{"login": account, "type": kind},
		"repository_selection": "all", "permissions": perms,
	})
}

func (f *fakeGitHubUser) allow(token string) {
	f.mu.Lock()
	f.validAccess[token] = true
	f.mu.Unlock()
}

func fullPermissions() map[string]string {
	return map[string]string{
		"issues": "write", "pull_requests": "write", "actions": "write", "contents": "write", "checks": "read",
	}
}

// clock is a settable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newSignInFixture(t *testing.T) (*fixture, *fakeGitHubUser, *clock) {
	t.Helper()
	fake, clk := newFakeGitHubUser(t), &clock{t: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
	f := newFixture(t, func(o *integrations.Options) {
		o.GitHubAPIBaseURL, o.GitHubAuthBaseURL, o.Now = fake.srv.URL, fake.srv.URL, clk.now
	})
	return f, fake, clk
}

// signIn runs the whole sign-in against the fake: code, approval, and an installed App.
func signIn(t *testing.T, f *fixture, fake *fakeGitHubUser, clk *clock) {
	t.Helper()
	ctx := context.Background()
	fake.install("octo", "User", fullPermissions())
	if _, err := f.svc.StartGitHubConnect(ctx); err != nil {
		t.Fatalf("StartGitHubConnect: %v", err)
	}
	clk.advance(5 * time.Second)
	view, connected, err := f.svc.ReadGitHubConnect(ctx)
	if err != nil || !connected || view.State != protocol.GitHubConnectStateConnected {
		t.Fatalf("sign-in did not finish: %+v connected=%v err=%v", view, connected, err)
	}
}

func TestSignInShowsACodeThenAsksForTheInstallThenConnects(t *testing.T) {
	f, fake, clk := newSignInFixture(t)
	ctx := context.Background()

	view, err := f.svc.StartGitHubConnect(ctx)
	if err != nil {
		t.Fatalf("StartGitHubConnect: %v", err)
	}
	if view.State != protocol.GitHubConnectStatePending || view.UserCode != "ABCD-1234" ||
		!strings.HasSuffix(view.VerificationURI, "/login/device") || view.ExpiresAt == nil {
		t.Fatalf("pending view = %+v", view)
	}

	// Asking again inside GitHub's interval must not poll GitHub again.
	if _, _, err := f.svc.ReadGitHubConnect(ctx); err != nil {
		t.Fatalf("ReadGitHubConnect: %v", err)
	}
	if fake.polls != 0 {
		t.Fatalf("polled GitHub %d times inside the interval, want 0", fake.polls)
	}

	clk.advance(5 * time.Second)
	view, connected, err := f.svc.ReadGitHubConnect(ctx)
	if err != nil || connected {
		t.Fatalf("after approval: connected=%v err=%v", connected, err)
	}
	if view.State != protocol.GitHubConnectStateNeedsInstall || view.Login != "octo" ||
		!strings.HasSuffix(view.InstallURL, "/apps/marshal-kanban/installations/new") {
		t.Fatalf("needs-install view = %+v", view)
	}

	fake.install("octo", "User", fullPermissions())
	fake.install("acme", "Organization", fullPermissions())
	clk.advance(3 * time.Second)
	view, connected, err = f.svc.ReadGitHubConnect(ctx)
	if err != nil || !connected || view.State != protocol.GitHubConnectStateConnected || len(view.Installations) != 2 {
		t.Fatalf("connected view = %+v connected=%v err=%v", view, connected, err)
	}
	if view.Installations[1].Kind != "organization" || view.Mode != "oauth" {
		t.Fatalf("installations = %+v mode=%q", view.Installations, view.Mode)
	}
	// The connected signal is given once, so two screens polling run one test.
	if _, again, _ := f.svc.ReadGitHubConnect(ctx); again {
		t.Fatal("the connection was announced twice")
	}

	config, ref, found := f.row(integrations.GitHubID)
	raw, _ := f.keys.Get(integrations.GitHubID)
	if !found || ref != integrations.GitHubID || !strings.Contains(raw, `"mode":"oauth"`) ||
		strings.Contains(config, "ghu_") || !strings.Contains(config, `"login":"octo"`) {
		t.Fatalf("stored row=%q ref=%q keychain=%q", config, ref, raw)
	}
	list, _ := f.svc.List(ctx)
	if list[0].Status != protocol.IntegrationStatusConnected {
		t.Fatalf("the GitHub row reads %q, want connected", list[0].Status)
	}
}

func TestSignInEndsWhenRefusedOrExpired(t *testing.T) {
	f, fake, clk := newSignInFixture(t)
	ctx := context.Background()
	fake.deny = true
	_, _ = f.svc.StartGitHubConnect(ctx)
	clk.advance(5 * time.Second)
	view, _, _ := f.svc.ReadGitHubConnect(ctx)
	if view.State != protocol.GitHubConnectStateDenied {
		t.Fatalf("a refused code reads %q, want denied", view.State)
	}

	fake.deny = false
	fake.approveOnPoll = 99
	_, _ = f.svc.StartGitHubConnect(ctx)
	clk.advance(16 * time.Minute)
	view, _, _ = f.svc.ReadGitHubConnect(ctx)
	if view.State != protocol.GitHubConnectStateExpired {
		t.Fatalf("an old code reads %q, want expired", view.State)
	}
	if view = f.svc.CancelGitHubConnect(ctx); view.State != protocol.GitHubConnectStateIdle {
		t.Fatalf("after cancel the state is %q, want idle", view.State)
	}
}

func TestPastedTokenIsCheckedThenSavedAndUsed(t *testing.T) {
	f, fake, _ := newSignInFixture(t)
	ctx := context.Background()

	err := f.svc.SaveGitHubToken(ctx, protocol.SaveGitHubTokenRequest{Token: "ghp_wrong"})
	var perr *protocol.Error
	if !errors.As(err, &perr) || !strings.Contains(perr.Error(), "did not accept that token") {
		t.Fatalf("a refused token answered %v", err)
	}
	if _, _, found := f.row(integrations.GitHubID); found {
		t.Fatal("a token GitHub refused was saved")
	}

	fake.allow("ghp_good")
	fake.scopes = "repo"
	if err := f.svc.SaveGitHubToken(ctx, protocol.SaveGitHubTokenRequest{Token: " ghp_good "}); err != nil {
		t.Fatalf("SaveGitHubToken: %v", err)
	}
	view, _, _ := f.svc.ReadGitHubConnect(ctx)
	if view.State != protocol.GitHubConnectStateConnected || view.Mode != "token" || view.Login != "octo" ||
		len(view.Installations) != 0 {
		t.Fatalf("view after saving a token = %+v", view)
	}
	client, err := f.svc.GitHubClient(ctx)
	if err != nil {
		t.Fatalf("GitHubClient: %v", err)
	}
	if _, err := client.ListChecks(ctx, ghclient.Repository{Owner: "acme", Name: "web"}, "main"); err != nil {
		t.Fatalf("ListChecks: %v", err)
	}
	if fake.lastAuth != "Bearer ghp_good" {
		t.Fatalf("GitHub saw %q, want the trimmed token", fake.lastAuth)
	}
	result, err := f.svc.Test(ctx, integrations.GitHubID)
	if err != nil || result.Checks[0].Name != "Summary" ||
		result.Checks[0].Message != "Connected as @octo with a personal access token." {
		t.Fatalf("stored test = %+v err=%v", result, err)
	}
}

func TestTestingATokenSavesNothingAndNamesWhatIsMissing(t *testing.T) {
	f, fake, _ := newSignInFixture(t)
	fake.allow("ghp_noscope")
	fake.scopes = "read:user"
	result, err := f.svc.TestGitHubToken(context.Background(), protocol.SaveGitHubTokenRequest{Token: "ghp_noscope"})
	if err != nil {
		t.Fatalf("TestGitHubToken: %v", err)
	}
	summary := result.Checks[0]
	if summary.State != protocol.CheckStateFailed || !strings.Contains(summary.Message, "repo scope") {
		t.Fatalf("summary = %+v, want a failure naming the repo scope", summary)
	}
	if _, _, found := f.row(integrations.GitHubID); found {
		t.Fatal("testing a token saved it")
	}

	fake.scopes = "repo, workflow"
	result, _ = f.svc.TestGitHubToken(context.Background(), protocol.SaveGitHubTokenRequest{Token: "ghp_noscope"})
	if result.Checks[0].State != protocol.CheckStatePassed {
		t.Fatalf("a classic token with repo reads %+v, want passed", result.Checks[0])
	}
}

func TestStoredSignInTestNamesAccountsRepositoriesAndMissingPermissions(t *testing.T) {
	f, fake, clk := newSignInFixture(t)
	signIn(t, f, fake, clk)
	result, err := f.svc.Test(context.Background(), integrations.GitHubID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	want := "Connected as @octo. Marshal can see 3 repositories on 1 account."
	if result.Checks[0].State != protocol.CheckStatePassed || result.Checks[0].Message != want {
		t.Fatalf("summary = %+v, want %q", result.Checks[0], want)
	}

	fake.mu.Lock()
	fake.installs[0]["permissions"] = map[string]string{"issues": "write"}
	fake.mu.Unlock()
	result, _ = f.svc.Test(context.Background(), integrations.GitHubID)
	if result.Checks[0].State != protocol.CheckStateFailed || !strings.Contains(result.Checks[0].Message, "pull_requests") {
		t.Fatalf("summary = %+v, want a failure naming pull_requests", result.Checks[0])
	}
}

func TestSignInTokenIsRenewedOnceWhenManyCallersAskAtOnce(t *testing.T) {
	f, fake, clk := newSignInFixture(t)
	fake.expiresIn = 3600
	signIn(t, f, fake, clk)
	clk.advance(59*time.Minute + 30*time.Second)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, err := f.svc.GitHubClient(context.Background())
			if err == nil {
				_, err = client.ListChecks(context.Background(), ghclient.Repository{Owner: "acme", Name: "web"}, "main")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a caller failed during the renewal: %v", err)
		}
	}
	if fake.refreshes != 1 {
		t.Fatalf("GitHub was asked to renew %d times, want 1", fake.refreshes)
	}
	if fake.secretOnRefresh {
		t.Fatal("the renewal sent a client secret")
	}
	raw, _ := f.keys.Get(integrations.GitHubID)
	if !strings.Contains(raw, "ghr_2") {
		t.Fatalf("the renewed refresh token was not saved: %s", raw)
	}
}

func TestAnExpiredSignInAsksToReconnect(t *testing.T) {
	f, fake, clk := newSignInFixture(t)
	fake.expiresIn = 3600
	signIn(t, f, fake, clk)
	fake.rejectRefresh = true
	clk.advance(59*time.Minute + 30*time.Second)

	_, err := f.svc.GitHubClient(context.Background())
	if !errors.Is(err, integrations.ErrGitHubReconnect) {
		t.Fatalf("GitHubClient answered %v, want ErrGitHubReconnect", err)
	}
	list, _ := f.svc.List(context.Background())
	if list[0].Status != protocol.IntegrationStatusError || !strings.Contains(list[0].Detail, "Reconnect GitHub") {
		t.Fatalf("the row reads %q %q, want needs attention", list[0].Status, list[0].Detail)
	}
	result, _ := f.svc.Test(context.Background(), integrations.GitHubID)
	if result.Checks[0].State != protocol.CheckStateFailed {
		t.Fatalf("test = %+v, want a failure", result.Checks[0])
	}
}

func TestATokenSavedByHandBeforeConnectionsStillWorks(t *testing.T) {
	f, fake, _ := newSignInFixture(t)
	fake.allow("ghp_legacy")
	if err := f.keys.Set(integrations.GitHubID, "ghp_legacy"); err != nil {
		t.Fatalf("keychain: %v", err)
	}
	client, err := f.svc.GitHubClient(context.Background())
	if err != nil {
		t.Fatalf("GitHubClient with a raw token: %v", err)
	}
	if _, err := client.ListChecks(context.Background(), ghclient.Repository{Owner: "acme", Name: "web"}, "main"); err != nil {
		t.Fatalf("ListChecks: %v", err)
	}
}

func TestNothingConnectedAnswersNotConnected(t *testing.T) {
	f, _, _ := newSignInFixture(t)
	if _, err := f.svc.GitHubClient(context.Background()); !errors.Is(err, integrations.ErrNotConnected) {
		t.Fatalf("GitHubClient answered %v, want ErrNotConnected", err)
	}
	view, _, _ := f.svc.ReadGitHubConnect(context.Background())
	if view.State != protocol.GitHubConnectStateIdle || view.Installations == nil {
		t.Fatalf("idle view = %+v", view)
	}
}

func TestRemovingGitHubEndsASignInButRemovingAnotherConnectionDoesNot(t *testing.T) {
	f, _, _ := newSignInFixture(t)
	ctx := context.Background()
	_, _ = f.svc.StartGitHubConnect(ctx)
	if err := f.svc.Remove(ctx, integrations.TelegramID); err != nil {
		t.Fatalf("Remove telegram: %v", err)
	}
	if view, _, _ := f.svc.ReadGitHubConnect(ctx); view.State != protocol.GitHubConnectStatePending {
		t.Fatalf("removing Telegram changed the GitHub sign-in to %q", view.State)
	}
	if err := f.svc.Remove(ctx, integrations.GitHubID); err != nil {
		t.Fatalf("Remove github: %v", err)
	}
	if view, _, _ := f.svc.ReadGitHubConnect(ctx); view.State != protocol.GitHubConnectStateIdle {
		t.Fatalf("after removing GitHub the state is %q, want idle", view.State)
	}
}

func TestAnAppThatSeesNoRepositoriesSaysWhyForEachCause(t *testing.T) {
	f, fake, clk := newSignInFixture(t)
	fake.repoCount = 0
	signIn(t, f, fake, clk)

	result, _ := f.svc.Test(context.Background(), integrations.GitHubID)
	summary := result.Checks[0]
	if summary.State != protocol.CheckStateFailed || !strings.Contains(summary.Message, "have no repositories yet") ||
		!strings.Contains(summary.Fix, "owns your repositories") {
		t.Fatalf("an empty account reads %+v, want the no-repositories sentence", summary)
	}

	fake.mu.Lock()
	fake.installs[0]["repository_selection"] = "selected"
	fake.mu.Unlock()
	result, _ = f.svc.Test(context.Background(), integrations.GitHubID)
	summary = result.Checks[0]
	if !strings.Contains(summary.Message, "none are selected") || !strings.Contains(summary.Fix, "select the repositories") {
		t.Fatalf("a selected-repositories install reads %+v, want the none-selected sentence", summary)
	}
}
