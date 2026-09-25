package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	sampleProjectID = "sample"
	sampleCardID    = "01M3C107JB041061050R3GG28A"
	unauthorizedMsg = "Sign in again. This device's token is missing or no longer valid."
	nothingThereMsg = "Marshal has nothing at that address. Check the address and try again."
)

// concretePath fills the ids of a route pattern with ids of the right shape, so the request gets
// as far as the token check the way a real one does.
func concretePath(pattern string) (method, path string) {
	method, path, _ = strings.Cut(pattern, " ")
	if strings.HasPrefix(path, "/v1/projects/{id}") {
		return method, strings.Replace(path, "{id}", sampleProjectID, 1)
	}
	return method, strings.Replace(path, "{id}", sampleCardID, 1)
}

// Every domain route refuses a request without a valid token, in the one error shape, before it
// reads the body or asks a service. The list of routes is the one that registers them, so a route
// that is added later is checked here without anyone remembering to.
func TestEveryRouteRequiresAToken(t *testing.T) {
	patterns := api.RoutePatterns()
	if len(patterns) == 0 {
		t.Fatal("there are no domain routes to check")
	}
	st := newStack(t)
	for _, pattern := range patterns {
		method, path := concretePath(pattern)
		var body any
		if method == http.MethodPost || method == http.MethodPatch {
			body = `{}` // a valid body, so only the missing token can be the reason for the refusal
		}
		tests := []struct {
			name   string
			header string
		}{
			{"no Authorization header", ""},
			{"a wrong token", "Bearer not-the-token"},
			{"a token that is empty", "Bearer "},
			{"a scheme that is not Bearer", "Basic " + st.token},
			{"the token with no scheme", st.token},
		}
		for _, tc := range tests {
			t.Run(pattern+" with "+tc.name, func(t *testing.T) {
				req := st.newRequest(method, path, body)
				if tc.header != "" {
					req.Header.Set("Authorization", tc.header)
				}
				r := st.send(req)
				got := r.apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
				if got.Message != unauthorizedMsg {
					t.Errorf("message = %q, want %q", got.Message, unauthorizedMsg)
				}
				if r.Header.Get("WWW-Authenticate") != "Bearer" {
					t.Errorf("WWW-Authenticate = %q, want Bearer", r.Header.Get("WWW-Authenticate"))
				}
			})
		}
	}
}

// The token is never in what the daemon logs, including when it was refused.
func TestATokenIsNeverLogged(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodGet, "/v1/projects", nil).want(t, http.StatusOK)
	st.doWith("a-wrong-token-that-someone-tried", http.MethodGet, "/v1/projects", nil).want(t, http.StatusUnauthorized)
	st.mustNotLog(st.token)
	st.mustNotLog("a-wrong-token-that-someone-tried")
	if !strings.Contains(st.logText(), `path=/v1/projects`) || strings.Contains(st.logText(), "Authorization") {
		t.Errorf("the access log should name the path and never a header:\n%s", st.logText())
	}
}

// A route is registered only when the service it needs is there. One that is not registered is
// an address that does not exist, and no token can change that.
func TestARouteIsRegisteredOnlyWhenItsServiceIsThere(t *testing.T) {
	projectRoutes := []string{
		"GET /v1/projects", "POST /v1/projects", "GET /v1/projects/{id}", "PATCH /v1/projects/{id}",
		"DELETE /v1/projects/{id}", "GET /v1/projects/{id}/board", "POST /v1/projects/{id}/cards", "GET /v1/cards/{id}",
	}
	sessionRoutes := []string{"POST /v1/cards/{id}/messages", "POST /v1/cards/{id}/stop", "POST /v1/cards/{id}/resume"}
	startRoute := []string{"POST /v1/cards/{id}/start"}
	agentRoutes := []string{"GET /v1/agents", "POST /v1/agents/refresh"}
	all := append(append(append(append([]string{}, projectRoutes...), sessionRoutes...), startRoute...), agentRoutes...)
	if len(all) != len(api.RoutePatterns()) {
		t.Fatalf("this test knows %d routes and the server has %d: add the new one to the right group", len(all), len(api.RoutePatterns()))
	}
	tests := []struct {
		name    string
		opts    []stackOption
		present [][]string
		absent  [][]string
	}{
		{"everything", nil, [][]string{projectRoutes, sessionRoutes, startRoute, agentRoutes}, nil},
		{"no projects", []stackOption{withoutProjects()}, [][]string{sessionRoutes, agentRoutes}, [][]string{projectRoutes, startRoute}},
		{"no sessions", []stackOption{withoutSessions()}, [][]string{projectRoutes, agentRoutes}, [][]string{sessionRoutes, startRoute}},
		{"no catalog", []stackOption{withoutCatalog()}, [][]string{projectRoutes, sessionRoutes, startRoute}, [][]string{agentRoutes}},
		{"nothing", []stackOption{withoutProjects(), withoutSessions(), withoutCatalog()}, nil, [][]string{projectRoutes, sessionRoutes, startRoute, agentRoutes}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t, tc.opts...)
			for _, group := range tc.present {
				for _, pattern := range group {
					method, path := concretePath(pattern)
					if got := st.do(method, path, nil); got.Status == http.StatusNotFound && strings.Contains(string(got.Body), nothingThereMsg) {
						t.Errorf("%s should be registered", pattern)
					}
				}
			}
			for _, group := range tc.absent {
				for _, pattern := range group {
					method, path := concretePath(pattern)
					got := st.do(method, path, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
					if got.Message != nothingThereMsg {
						t.Errorf("%s should not be registered, but answered %q", pattern, got.Message)
					}
				}
			}
		})
	}
}

// Without a store there is nothing to check a token against, so no domain route exists, whatever
// services are given.
func TestNoDomainRoutesWithoutAStore(t *testing.T) {
	st := newStack(t)
	server := api.New(st.settings, st.log, time.Now, api.Deps{Projects: st.proj, Sessions: st.mgr})
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/projects", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
