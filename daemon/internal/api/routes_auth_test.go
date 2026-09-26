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
	sampleDiffPath  = "src/util.js"
	unauthorizedMsg = "Sign in again. This device's token is missing or no longer valid."
	nothingThereMsg = "Marshal has nothing at that address. Check the address and try again."
)

// concretePath fills the ids of a route pattern with ids of the right shape, so the request gets
// as far as the token check the way a real one does.
func concretePath(pattern string) (method, path string) {
	method, path, _ = strings.Cut(pattern, " ")
	if strings.HasPrefix(path, "/v1/projects/{id}") {
		path = strings.Replace(path, "{id}", sampleProjectID, 1)
	} else {
		path = strings.Replace(path, "{id}", sampleCardID, 1)
	}
	// A route that names a file inside a card carries the rest of the address as its path.
	return method, strings.Replace(path, "{path...}", sampleDiffPath, 1)
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
	sessionRoutes := []string{
		"POST /v1/cards/{id}/messages", "POST /v1/cards/{id}/stop", "POST /v1/cards/{id}/resume", "POST /v1/cards/{id}/view",
	}
	holdRoutes := []string{
		"POST /v1/cards/{id}/pause", "POST /v1/cards/{id}/unpause",
		"POST /v1/cards/{id}/sleep", "POST /v1/cards/{id}/wake",
		"POST /v1/cards/{id}/pin", "POST /v1/cards/{id}/unpin",
	}
	cardRoutes := []string{"POST /v1/cards/{id}/move", "PATCH /v1/cards/{id}", "DELETE /v1/cards/{id}", "POST /v1/cards/{id}/fork"}
	labelRoutes := []string{"GET /v1/projects/{id}/labels", "POST /v1/projects/{id}/labels", "PATCH /v1/labels/{id}", "DELETE /v1/labels/{id}"}
	homeRoutes := []string{"GET /v1/home/dashboard", "GET /v1/home/activity"}
	historyRoutes := []string{
		"GET /v1/cards/{id}/messages", "GET /v1/cards/{id}/messages/{messageId}",
		"GET /v1/cards/{id}/activity",
		"GET /v1/chats/{id}/messages", "GET /v1/chats/{id}/messages/{messageId}",
	}
	diffRoutes := []string{"GET /v1/cards/{id}/diff", "GET /v1/cards/{id}/diff/{path...}"}
	chatRoutes := []string{
		"GET /v1/projects/{id}/chats", "POST /v1/projects/{id}/chats",
		"PATCH /v1/chats/{id}", "POST /v1/chats/{id}/archive", "POST /v1/chats/{id}/restore",
		"DELETE /v1/chats/{id}", "POST /v1/chats/{id}/messages",
	}
	searchRoutes := []string{"GET /v1/search"}
	startRoute := []string{"POST /v1/cards/{id}/start"}
	agentRoutes := []string{"GET /v1/agents", "POST /v1/agents/refresh"}
	accountRoutes := []string{
		"GET /v1/me", "PATCH /v1/me", "POST /v1/me/avatar", "DELETE /v1/me/avatar", "GET /v1/users",
		"GET /v1/users/{id}/avatar", "GET /v1/me/progress", "PATCH /v1/me/progress",
		"GET /v1/me/preferences", "PATCH /v1/me/preferences",
	}
	savedViewRoutes := []string{
		"GET /v1/projects/{id}/saved-views", "POST /v1/projects/{id}/saved-views",
		"PATCH /v1/saved-views/{id}", "DELETE /v1/saved-views/{id}",
	}
	devRoutes := []string{"POST /v1/dev/reset-first-launch"}
	groups := map[string][]string{
		"projects": projectRoutes, "sessions": sessionRoutes, "hold": holdRoutes, "start": startRoute,
		"cards": cardRoutes, "labels": labelRoutes, "home": homeRoutes,
		"agents": agentRoutes, "history": historyRoutes, "diff": diffRoutes, "chats": chatRoutes,
		"search": searchRoutes, "accounts": accountRoutes, "saved views": savedViewRoutes, "dev": devRoutes,
	}
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	if count != len(api.RoutePatterns()) {
		t.Fatalf("this test knows %d routes and the server has %d: add the new one to the right group", count, len(api.RoutePatterns()))
	}
	// present names the groups a stack built with these options has. Every other group is a set of
	// addresses that must not exist, so a route whose service is missing cannot be reached at all.
	tests := []struct {
		name    string
		opts    []stackOption
		present []string
	}{
		{"everything", nil, []string{"projects", "sessions", "hold", "start", "cards", "labels", "home", "agents", "history", "diff", "chats", "search"}},
		{"no projects", []stackOption{withoutProjects()}, []string{"sessions", "hold", "home", "agents", "history", "chats"}},
		{"no sessions", []stackOption{withoutSessions()}, []string{"projects", "cards", "labels", "home", "agents", "history", "diff", "chats", "search"}},
		{"no catalog", []stackOption{withoutCatalog()}, []string{"projects", "sessions", "hold", "start", "cards", "labels", "home", "history", "diff", "chats", "search"}},
		{"no dashboard", []stackOption{withoutDashboard()}, []string{"projects", "sessions", "hold", "start", "cards", "labels", "agents", "history", "diff", "chats", "search"}},
		{"no history", []stackOption{withoutHistory()}, []string{"projects", "sessions", "hold", "start", "cards", "labels", "home", "agents", "diff", "chats", "search"}},
		{"no diff", []stackOption{withoutDiff()}, []string{"projects", "sessions", "hold", "start", "cards", "labels", "home", "agents", "history", "chats", "search"}},
		{"no chats", []stackOption{withoutChats()}, []string{"projects", "sessions", "hold", "start", "cards", "labels", "home", "agents", "history", "diff"}},
		{"no search", []stackOption{withoutSearch()}, []string{"projects", "sessions", "hold", "start", "cards", "labels", "home", "agents", "history", "diff", "chats"}},
		{"nothing", []stackOption{withoutProjects(), withoutSessions(), withoutCatalog(), withoutDashboard(), withoutHistory(), withoutDiff(), withoutChats(), withoutSearch()}, nil},
	}
	// A stack without the accounts service, and a normal daemon, have every other group.
	for name, opt := range map[string]stackOption{"no accounts": withoutAccounts(), "a normal daemon": normalDaemon()} {
		tests = append(tests, struct {
			name    string
			opts    []stackOption
			present []string
		}{name, []stackOption{opt}, tests[0].present})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t, tc.opts...)
			have := map[string]bool{}
			for _, name := range tc.present {
				have[name] = true
			}
			// The accounts routes follow the accounts service, the saved view routes follow the
			// projects service, and the dev route needs the accounts service and a dev daemon. The
			// stack knows which it was built with, so the rows above do not name these groups.
			have["accounts"] = !st.cfg.noAccounts
			have["saved views"] = !st.cfg.noProjects
			have["dev"] = !st.cfg.noAccounts && !st.cfg.normal
			for name, group := range groups {
				for _, pattern := range group {
					method, path := concretePath(pattern)
					got := st.do(method, path, nil)
					// A 405 means the address exists for another method (the two
					// /v1/cards/{id}/messages routes), so this route is not registered.
					missing := got.Status == http.StatusNotFound && strings.Contains(string(got.Body), nothingThereMsg)
					registered := got.Status != http.StatusMethodNotAllowed && !missing
					if registered != have[name] {
						t.Errorf("%s is registered %v, want %v (group %s)", pattern, registered, have[name], name)
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
