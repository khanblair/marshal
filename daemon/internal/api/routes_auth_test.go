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
	sampleProjectID    = "sample"
	sampleCardID       = "01M3C107JB041061050R3GG28A"
	sampleCheckpointID = "01M3C107JB041061050R3GG28B"
	sampleFindingID    = "01M3C107JB041061050R3GG28C"
	sampleDiffPath     = "src/util.js"
	unauthorizedMsg    = "Sign in again. This device's token is missing or no longer valid."
	nothingThereMsg    = "Marshal has nothing at that address. Check the address and try again."
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
	path = strings.Replace(path, "{path...}", sampleDiffPath, 1)
	// A limits route names the scope and the kind, so they are filled with a pair that exists.
	path = strings.Replace(path, "{scope}", "global", 1)
	// A roles route names a role, so it is filled with one of Marshal's own.
	path = strings.Replace(path, "{name}", "Worker", 1)
	// A checkpoint restore names the checkpoint, so it is filled with an id of the right shape.
	path = strings.Replace(path, "{cp}", sampleCheckpointID, 1)
	// A call that acts on a finding names the finding, so it is filled with an id of the right shape.
	path = strings.Replace(path, "{findingId}", sampleFindingID, 1)
	// A preview screenshot route names the image file, so it is filled with one of the two kinds
	// Marshal takes. Whether that card has a shot is the service's business, not the router's.
	path = strings.Replace(path, "{file}", "before.png", 1)
	return method, strings.Replace(path, "{kind}", "cost-day", 1)
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
		if method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut {
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
	projectRoutes := []string{"GET /v1/folders",
		"GET /v1/projects", "POST /v1/projects", "GET /v1/projects/{id}", "PATCH /v1/projects/{id}",
		"DELETE /v1/projects/{id}", "GET /v1/projects/{id}/board", "POST /v1/projects/{id}/cards", "GET /v1/cards/{id}",
	}
	sessionRoutes := []string{
		"POST /v1/cards/{id}/messages", "POST /v1/cards/{id}/stop", "POST /v1/cards/{id}/resume", "POST /v1/cards/{id}/view",
		"POST /v1/approvals/{id}",
		"POST /v1/cards/{id}/bypass", "DELETE /v1/cards/{id}/bypass",
	}
	holdRoutes := []string{
		"POST /v1/cards/{id}/pause", "POST /v1/cards/{id}/unpause",
		"POST /v1/cards/{id}/sleep", "POST /v1/cards/{id}/wake",
		"POST /v1/cards/{id}/pin", "POST /v1/cards/{id}/unpin",
	}
	// The plan routes need both projects and sessions: the session manager holds the plan, and the
	// card it belongs to is read through the projects service.
	planRoutes := []string{
		"POST /v1/cards/{id}/plan/approve", "POST /v1/cards/{id}/plan/reject", "PUT /v1/cards/{id}/plan",
	}
	// The checkpoint routes need both services too: the session manager holds the card's worktree,
	// and the card itself is read through the projects service.
	checkpointRoutes := []string{
		"GET /v1/cards/{id}/checkpoints", "POST /v1/cards/{id}/checkpoints/{cp}/restore",
	}
	// Handing a card off needs both services as well: the session manager holds the session being
	// replaced and the worktree the new agent starts in, and the card the handoff continues is read
	// through the projects service.
	handoffRoutes := []string{"POST /v1/cards/{id}/handoff"}
	// The two note routes need the projects service, which reads the card a note's file is found
	// from, and the memory module, which owns the vault the note is written in.
	noteRoutes := []string{"GET /v1/cards/{id}/note", "PUT /v1/cards/{id}/note"}
	// The lesson routes need the same pair as the note routes, and for the same reason: the memory
	// module owns the vault a lesson's file lives in, and the projects service is what its project
	// id is checked against.
	lessonRoutes := []string{
		"GET /v1/projects/{id}/lessons", "POST /v1/projects/{id}/lessons",
		"GET /v1/projects/{id}/lessons/{slug}", "PUT /v1/projects/{id}/lessons/{slug}",
		"DELETE /v1/projects/{id}/lessons/{slug}",
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
	agentRoutes := []string{"GET /v1/agents", "POST /v1/agents/refresh", "POST /v1/agents/{id}/test"}
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
	auditRoutes := []string{"GET /v1/audit", "GET /v1/audit/search", "GET /v1/audit/export"}
	providerRoutes := []string{"GET /v1/providers", "PUT /v1/providers/{id}", "DELETE /v1/providers/{id}"}
	connectionTestRoutes := []string{"POST /v1/providers/{id}/test"}
	// The connection routes list, save, and remove the connections Marshal is set up with: they
	// follow the connections service the GitHub app is reached through.
	integrationRoutes := []string{"POST /v1/integrations/telegram/detect-chat",
		"GET /v1/integrations", "PUT /v1/integrations/{id}", "DELETE /v1/integrations/{id}",
		"GET /v1/integrations/gcal/authorize", "GET /v1/integrations/gmail/authorize", "GET /v1/integrations/{id}/authorize",
		"GET /v1/google/files", "POST /v1/google/docs", "POST /v1/google/sheets", "POST /v1/google/slides",
		"POST /v1/google/drive/files", "POST /v1/google/read",
		"GET /v1/integrations/gcal/client", "GET /v1/integrations/gcal/calendars", "PUT /v1/integrations/gcal/calendars",
		"POST /v1/integrations/github/connect", "GET /v1/integrations/github/connect",
		"DELETE /v1/integrations/github/connect", "PUT /v1/integrations/github/token",
		"POST /v1/integrations/github/token/test",
	}
	// Asking a connection to test itself needs the connections service and the runner that runs it.
	integrationTestRoutes := []string{"POST /v1/integrations/{id}/test"}
	limitRoutes := []string{"GET /v1/limits", "PUT /v1/limits/{scope}/{kind}", "DELETE /v1/limits/{scope}/{kind}"}
	roleRoutes := []string{
		"GET /v1/roles", "POST /v1/roles", "GET /v1/roles/{name}", "PATCH /v1/roles/{name}",
		"DELETE /v1/roles/{name}", "POST /v1/roles/{name}/reset", "PUT /v1/roles/{name}/override",
	}
	// The pull-request route needs both projects and a GitHub client, so it follows the pair.
	pullRequestRoutes := []string{"POST /v1/cards/{id}/pull-request"}
	// The review route needs the projects, the review service, and the roles the Reviewer reads.
	reviewRoutes := []string{"POST /v1/cards/{id}/review"}
	// The merge route needs both projects and the merge queue.
	integratorRoutes := []string{"POST /v1/cards/{id}/merge"}
	// The Integration view's routes need the merge queue's reader. Showing a card's worktree needs
	// only the projects service, which owns the card's folder.
	mergeStateRoutes := []string{
		"GET /v1/projects/{id}/integration", "POST /v1/projects/{id}/integration/pause",
		"POST /v1/projects/{id}/integration/resume", "POST /v1/cards/{id}/merge/retry",
		"POST /v1/cards/{id}/merge/undo",
	}
	worktreeRoutes := []string{"POST /v1/cards/{id}/worktree/open"}
	// The notice routes are the session manager's: a sleep notice names live sessions and the
	// moment they sleep, so they follow the sessions service and need nothing else.
	noticeRoutes := []string{
		"GET /v1/notices", "POST /v1/notices/{id}/actions", "DELETE /v1/notices/{id}",
	}
	// The sleep settings are their own service: they are the numbers the idle timer is driven by,
	// and they outlive every session.
	sleepSettingsRoutes := []string{"GET /v1/settings/sleep", "PUT /v1/settings/sleep"}
	// The alert settings are the notification router's, kept in the settings service's table.
	alertRoutes := []string{"GET /v1/settings/alerts", "PUT /v1/settings/alerts"}
	// The card panel's routes are its own service's: a card's acceptance checks, checklists, comments
	// with their attachments, and the people on it.
	panelRoutes := []string{
		"GET /v1/cards/{id}/checks", "POST /v1/cards/{id}/checks", "POST /v1/cards/{id}/checks/run",
		"DELETE /v1/cards/{id}/checks/{check}",
		"GET /v1/cards/{id}/checklists", "POST /v1/cards/{id}/checklists",
		"PATCH /v1/cards/{id}/checklists/{list}", "DELETE /v1/cards/{id}/checklists/{list}",
		"POST /v1/cards/{id}/checklists/{list}/items", "PUT /v1/cards/{id}/checklists/{list}/items/{item}",
		"DELETE /v1/cards/{id}/checklists/{list}/items/{item}",
		"GET /v1/cards/{id}/comments", "POST /v1/cards/{id}/comments",
		"DELETE /v1/cards/{id}/comments/{comment}", "GET /v1/cards/{id}/attachments/{attachment}",
		"GET /v1/cards/{id}/members", "PUT /v1/cards/{id}/members/{user}",
		"DELETE /v1/cards/{id}/members/{user}",
	}
	// The quality routes need both the projects service, which owns the card and the project the
	// findings and the profile belong to, and the quality module, which owns the checks.
	qualityRoutes := []string{
		"GET /v1/cards/{id}/findings", "POST /v1/cards/{id}/findings/{findingId}/fix",
		"POST /v1/cards/{id}/findings/{findingId}/dismiss",
		"GET /v1/projects/{id}/smell-profile", "PUT /v1/projects/{id}/smell-profile",
	}
	// The CI route needs the CI monitor, which reads the projects a run belongs to and the roles
	// whose ceilings the fix loop counts rounds against.
	ciRoutes := []string{"GET /v1/ci"}
	// Simulating a failure needs the same monitor and, in its real mode, spends Actions minutes on
	// the person's own GitHub account, so it exists only on a dev daemon - the same reason the dev
	// reset route does.
	ciSimulateRoutes := []string{"POST /v1/cards/{id}/ci-failure"}
	// Running a card's workflow steps locally needs the projects service, which reads the worktree
	// the steps run in. Nothing else: a local run keeps no state and sends no event.
	localCIRoutes := []string{"POST /v1/cards/{id}/local-ci"}
	// The schedule routes are their own service's: a schedule outlives any one project, so they
	// follow the scheduler rather than the projects service.
	scheduleRoutes := []string{
		"GET /v1/schedules", "GET /v1/schedules/catalog", "POST /v1/schedules", "PUT /v1/schedules/{id}", "DELETE /v1/schedules/{id}",
		"GET /v1/schedules/{id}/runs", "POST /v1/schedules/{id}/run", "GET /v1/schedules/{id}/preview",
	}
	// The calendar route needs both the scheduler and the projects service, for the schedules and
	// the due cards; Google Calendar's own events are read only when integrations is also there.
	calendarRoutes := []string{"GET /v1/calendar"}
	// The preview routes need the projects service, which owns the card, its worktree, and its
	// project's dev command, and the preview module, which owns the dev server and its screenshots.
	previewRoutes := []string{
		"GET /v1/cards/{id}/preview", "POST /v1/cards/{id}/preview/start",
		"POST /v1/cards/{id}/preview/stop", "POST /v1/cards/{id}/preview/shots",
		"GET /v1/cards/{id}/preview/shots/{file}",
	}
	// The paired-device routes are the devices service's own: a device outlives any one project,
	// and the code that pairs a new one is a daemon-wide secret rather than a person's. The route
	// that exchanges a code for a token takes no token and so is not a domain route at all: it is
	// registered beside health and checked in routes_devices_test.go.
	deviceRoutes := []string{
		"GET /v1/me/devices", "POST /v1/me/devices/pairing-code", "DELETE /v1/me/devices/{id}",
	}
	// The tailnet status needs no service - the server holds the node or holds nothing - so it is
	// registered on every stack that has a store at all.
	tailnetRoutes := []string{"GET /v1/tailnet", "GET /v1/tailnet/peers"}
	groups := map[string][]string{
		"projects": projectRoutes, "sessions": sessionRoutes, "hold": holdRoutes, "start": startRoute,
		"cards": cardRoutes, "labels": labelRoutes, "home": homeRoutes,
		"agents": agentRoutes, "history": historyRoutes, "diff": diffRoutes, "chats": chatRoutes,
		"search": searchRoutes, "accounts": accountRoutes, "saved views": savedViewRoutes, "dev": devRoutes,
		"audit": auditRoutes, "providers": providerRoutes, "limits": limitRoutes,
		"connection tests": connectionTestRoutes, "roles": roleRoutes, "plans": planRoutes,
		"integrations": integrationRoutes, "integration tests": integrationTestRoutes,
		"checkpoints": checkpointRoutes, "pull requests": pullRequestRoutes,
		"handoffs": handoffRoutes, "notes": noteRoutes, "lessons": lessonRoutes,
		"integrator": integratorRoutes, "review": reviewRoutes,
		"merge state": mergeStateRoutes, "worktree": worktreeRoutes,
		"notices": noticeRoutes, "sleep settings": sleepSettingsRoutes, "alerts": alertRoutes, "card panel": panelRoutes, "quality": qualityRoutes,
		"ci": ciRoutes, "ci simulation": ciSimulateRoutes, "local ci": localCIRoutes,
		"preview": previewRoutes, "schedules": scheduleRoutes, "calendar": calendarRoutes, "devices": deviceRoutes,
		"tailnet": tailnetRoutes,
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
		// A stack with no memory module has no search either: two of the kinds search answers with
		// are read through it.
		{"no memory", []stackOption{withoutMemory()}, []string{"projects", "sessions", "hold", "start", "cards", "labels", "home", "agents", "history", "diff", "chats"}},
		{"nothing", []stackOption{withoutProjects(), withoutSessions(), withoutCatalog(), withoutDashboard(), withoutHistory(), withoutDiff(), withoutChats(), withoutSearch()}, nil},
	}
	// A stack without the accounts service, without the audit-log service, and a normal daemon, have
	// every other group.
	for name, opt := range map[string]stackOption{
		"no accounts": withoutAccounts(), "no audit": withoutAudit(), "a normal daemon": normalDaemon(), "no connection tests": withoutConnectionTests(), "no roles": withoutRoles(),
		"no pull requests": withoutPullRequests(), "no integrator": withoutIntegrator(),
		"no review": withoutReview(), "no sleep settings": withoutSleepSettings(),
		"no quality": withoutQuality(), "no integrations": withoutIntegrations(), "no merge state": withoutIntegration(),
		"no ci": withoutCI(), "no local ci": withoutLocalCI(),
		"no preview": withoutPreview(), "no schedules": withoutSchedules(),
		"no devices": withoutDevices(), "no alerts": withoutAlerts(), "no card panel": withoutCardPanel(),
	} {
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
			// projects service, the dev route needs the accounts service and a dev daemon, the audit
			// routes follow the audit-log service, and the provider, limits, connection-test, and role
			// routes follow their own services. The stack knows which it was built with, so the rows
			// above do not name these groups.
			have["accounts"] = !st.cfg.noAccounts
			have["saved views"] = !st.cfg.noProjects
			have["dev"] = !st.cfg.noAccounts && !st.cfg.normal
			have["audit"] = !st.cfg.noAudit
			have["providers"] = !st.cfg.noProviders
			have["limits"] = !st.cfg.noCostLimits
			have["connection tests"] = !st.cfg.noConnectionTests
			have["roles"] = !st.cfg.noRoles
			// The connection routes follow the connections service, and asking one to test itself
			// also needs the runner that runs the test.
			have["integrations"] = !st.cfg.noIntegrations
			have["integration tests"] = !st.cfg.noIntegrations && !st.cfg.noConnectionTests
			// The pull-request route needs both services and a GitHub client, so it follows the pair.
			have["pull requests"] = !st.cfg.noPullRequests && !st.cfg.noProjects
			// The review route follows the review service, which the stack builds only with the
			// projects and the roles its Reviewer reads.
			have["review"] = !st.cfg.noReview && !st.cfg.noProjects && !st.cfg.noRoles
			// The merge route needs both services too.
			have["integrator"] = !st.cfg.noIntegrator && !st.cfg.noProjects
			// The merge queue's reader stands alone; opening a worktree follows the projects service.
			have["merge state"] = !st.cfg.noIntegration
			have["worktree"] = !st.cfg.noProjects
			// The notice routes are the session manager's own.
			have["notices"] = !st.cfg.noSessions
			// The sleep settings follow their own service.
			have["sleep settings"] = !st.cfg.noSleepSettings
			// The alert settings are built beside the sleep settings, over the same service.
			have["alerts"] = !st.cfg.noAlerts && !st.cfg.noSleepSettings
			// The card panel is built over the projects service, which finds a card's worktree.
			have["card panel"] = !st.cfg.noCardPanel && !st.cfg.noProjects
			// The quality routes need the quality module and the projects service it reads a card
			// and a project through.
			have["quality"] = !st.cfg.noQuality && !st.cfg.noProjects
			// The plan routes need both services, so they follow the pair rather than either one.
			have["plans"] = !st.cfg.noProjects && !st.cfg.noSessions
			// The checkpoint routes need both services too, for the same reason as the plans.
			have["checkpoints"] = !st.cfg.noProjects && !st.cfg.noSessions
			// The handoff route needs both services as well: the session being replaced and the
			// card that continues on the new agent live in different services.
			have["handoffs"] = !st.cfg.noProjects && !st.cfg.noSessions
			// The note routes need the memory module, which owns the vault, and the projects
			// service, which reads the card a note is found from.
			have["notes"] = !st.cfg.noMemory && !st.cfg.noProjects
			// The lesson routes follow the same pair, for the same reason.
			have["lessons"] = !st.cfg.noMemory && !st.cfg.noProjects
			// The CI route follows the monitor, which the stack builds only with the projects the
			// runs belong to and the roles whose ceilings the fix loop counts against.
			have["ci"] = !st.cfg.noCI && !st.cfg.noProjects && !st.cfg.noRoles
			// Simulating a failure needs the same monitor and a dev daemon, because its real mode
			// spends Actions minutes on the person's own GitHub account.
			have["ci simulation"] = have["ci"] && !st.cfg.normal
			// The local-CI route reads the card's worktree through the projects service, which is
			// the only thing it needs besides itself.
			have["local ci"] = !st.cfg.noLocalCI && !st.cfg.noProjects
			// The preview routes need the preview module and the projects service it reads a card, its
			// worktree, and its project's dev command through.
			have["preview"] = !st.cfg.noPreview && !st.cfg.noProjects
			// The schedule routes need only the scheduler: nothing about them reads a project or
			// any other service.
			have["schedules"] = !st.cfg.noSchedules
			// The calendar route needs both the scheduler and the projects service.
			have["calendar"] = !st.cfg.noSchedules && !st.cfg.noProjects
			// The paired-device routes follow the devices service, the same way every other group
			// follows the service it calls.
			have["devices"] = !st.cfg.noDevices
			// The tailnet status needs no service at all.
			have["tailnet"] = true
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
