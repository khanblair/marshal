package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeGitHubSignIn answers GitHub's sign-in endpoints and the user calls, so a route test never
// dials GitHub. The code is approved on the first poll, and one installation exists once installed.
type fakeGitHubSignIn struct {
	mu        sync.Mutex
	installed bool
	srv       *httptest.Server
}

func newFakeGitHubSignIn(t *testing.T) *fakeGitHubSignIn {
	t.Helper()
	f := &fakeGitHubSignIn{}
	reply := func(w http.ResponseWriter, body any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, map[string]any{
			"device_code": "dc", "user_code": "WXYZ-9876", "verification_uri": f.srv.URL + "/login/device",
			"expires_in": 900, "interval": 5,
		})
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, map[string]any{"access_token": "ghu_route", "token_type": "bearer"})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); tok != "ghu_route" && tok != "ghp_route" {
			w.WriteHeader(http.StatusUnauthorized)
			reply(w, map[string]any{"message": "Bad credentials"})
			return
		}
		w.Header().Set("X-OAuth-Scopes", "repo")
		reply(w, map[string]any{"login": "octo"})
	})
	mux.HandleFunc("GET /user/installations", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		list := []map[string]any{}
		if f.installed {
			list = append(list, map[string]any{
				"id": 7, "account": map[string]any{"login": "octo", "type": "User"},
				"repository_selection": "all", "permissions": map[string]string{},
			})
		}
		reply(w, map[string]any{"total_count": len(list), "installations": list})
	})
	mux.HandleFunc("GET /user/installations/7/repositories", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, map[string]any{"total_count": 2})
	})
	mux.HandleFunc("GET /user/repos", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, []map[string]any{{"full_name": "octo/app"}})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func TestTheGitHubSignInRoutesTakeACodeThroughToConnected(t *testing.T) {
	fake := newFakeGitHubSignIn(t)
	st := newStack(t, withIntegrationsTester(passingTester), withGitHubBaseURL(fake.srv.URL))

	pending := decode[protocol.GitHubConnect](t, st.do(http.MethodPost, "/v1/integrations/github/connect", nil).want(t, http.StatusOK))
	if pending.State != protocol.GitHubConnectStatePending || pending.UserCode != "WXYZ-9876" || pending.ExpiresAt == nil {
		t.Fatalf("start answered %+v", pending)
	}

	st.advance(5 * time.Second)
	needs := decode[protocol.GitHubConnect](t, st.do(http.MethodGet, "/v1/integrations/github/connect", nil).want(t, http.StatusOK))
	if needs.State != protocol.GitHubConnectStateNeedsInstall || needs.Login != "octo" || needs.InstallURL == "" {
		t.Fatalf("after approval the read answered %+v", needs)
	}

	fake.mu.Lock()
	fake.installed = true
	fake.mu.Unlock()
	st.advance(3 * time.Second)
	done := decode[protocol.GitHubConnect](t, st.do(http.MethodGet, "/v1/integrations/github/connect", nil).want(t, http.StatusOK))
	if done.State != protocol.GitHubConnectStateConnected || len(done.Installations) != 1 {
		t.Fatalf("after the install the read answered %+v", done)
	}

	list := decode[protocol.IntegrationList](t, st.do(http.MethodGet, "/v1/integrations", nil).want(t, http.StatusOK))
	if list.Integrations[0].Status != protocol.IntegrationStatusConnected || list.Integrations[0].LastTest == nil {
		t.Fatalf("the GitHub row is %+v, want connected with a test run after the connect", list.Integrations[0])
	}
	for _, body := range [][]byte{st.do(http.MethodGet, "/v1/integrations", nil).Body, st.do(http.MethodGet, "/v1/integrations/github/connect", nil).Body} {
		if strings.Contains(string(body), "ghu_route") {
			t.Fatalf("a token came back from a route: %s", body)
		}
	}
}

func TestThePastedTokenRoutesTestWithoutSavingThenSave(t *testing.T) {
	fake := newFakeGitHubSignIn(t)
	st := newStack(t, withIntegrationsTester(passingTester), withGitHubBaseURL(fake.srv.URL))

	result := decode[protocol.TestResult](t, st.do(http.MethodPost, "/v1/integrations/github/token/test",
		protocol.SaveGitHubTokenRequest{Token: "ghp_route"}).want(t, http.StatusOK))
	if result.Checks[0].State != protocol.CheckStatePassed {
		t.Fatalf("the token test answered %+v", result.Checks)
	}
	list := decode[protocol.IntegrationList](t, st.do(http.MethodGet, "/v1/integrations", nil).want(t, http.StatusOK))
	if list.Integrations[0].Status != protocol.IntegrationStatusNone {
		t.Fatalf("testing a token connected GitHub: %q", list.Integrations[0].Status)
	}

	st.do(http.MethodPut, "/v1/integrations/github/token", protocol.SaveGitHubTokenRequest{Token: "ghp_wrong"}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	list = decode[protocol.IntegrationList](t, st.do(http.MethodPut, "/v1/integrations/github/token",
		protocol.SaveGitHubTokenRequest{Token: "ghp_route"}).want(t, http.StatusOK))
	if list.Integrations[0].Status != protocol.IntegrationStatusConnected {
		t.Fatalf("saving a token left GitHub %q", list.Integrations[0].Status)
	}
	state := decode[protocol.GitHubConnect](t, st.do(http.MethodGet, "/v1/integrations/github/connect", nil).want(t, http.StatusOK))
	if state.Mode != "token" || state.Login != "octo" {
		t.Fatalf("the connection reads %+v", state)
	}
}
