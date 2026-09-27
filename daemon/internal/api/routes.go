package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// routeNeeds names the services a route calls. A route is registered only when the server has all
// of the services it needs, the same way the event stream is registered only when there is a bus.
type routeNeeds uint32

const (
	needsProjects routeNeeds = 1 << iota
	needsSessions
	needsCatalog
	needsDashboard
	needsHistory
	needsDiff
	needsChats
	needsSearch
	needsAccounts
	needsAudit
	// needsProviders registers the routes that list the model providers and store, replace, and
	// remove their keys (N18). The key itself never comes back from any of them.
	needsProviders
	// needsCostLimits registers the routes that read and write the cost and awake limits (B4.5).
	needsCostLimits
	// needsConnectionTests registers the route that runs a connection test (B4.6, section 18). It
	// is a bit of its own rather than part of needsProviders because the machinery is shared: the
	// integrations and MCP servers of later phases get the same route over the same Runner, and
	// only the Tester differs.
	needsConnectionTests
	// needsRoles registers the routes that list the role templates and add, edit, delete, reset,
	// and override them (B5.1, N18).
	needsRoles
	// needsPullRequests registers the route that opens a card's branch as a real pull request
	// (B5.4, build-plan 5.6). It is registered only when a forge token is set up.
	needsPullRequests
	// needsIntegrator registers the merge-queue route (B5.5, build-plan 5.8).
	needsIntegrator
	// needsReview registers the route that runs the Reviewer over a card's pull request (B5.4,
	// build-plan 5.7). It is registered only when a forge token is set up and the Reviewer role
	// can be read.
	needsReview
	// needsSleepSettings registers the routes that read and write the sleep settings (B5.6, N5).
	// It is a bit of its own and not part of needsSessions because the settings outlive every
	// session: they are the numbers the idle timer is driven by, and a daemon that had no session
	// manager would still have them.
	needsSleepSettings
	// needsQuality registers the routes that read a card's code-smell findings, ask the card's
	// agent to fix one, dismiss one, and read and write a project's smell profile (B5.8, section
	// 17). It needs the projects service too, because a card and a project are read through it.
	needsQuality
	// needsIntegrations registers the routes that list the connections Marshal is set up with
	// apart from model providers, save one, remove one, and run one's connection test (B6.1, B6.7,
	// section 18). It is a bit of its own rather than part of needsConnectionTests because the two
	// are needed together for exactly one route: the test, which needs the runner (the cooldown and
	// the saved result) and the service (what the test asks).
	needsIntegrations
	// needsCI registers the route that reads every project's CI health (B6.2, section 11.1). It is a
	// bit of its own and not part of needsProjects because the CI monitor outlives any one project:
	// it is the module that watches the forge, and a daemon whose projects cannot be read still
	// answers the runs it holds. It also carries the route that simulates a failure, which needs the
	// dev-mode bit beside it (B6.4).
	needsCI
	// needsDevMode limits a route to a dev daemon: a normal daemon has no such address at all.
	needsDevMode
	// needsLocalCI registers the route that runs a card's own workflow steps on this machine (B6.5,
	// section 15.3). It carries needsProjects beside it, because a card's worktree is read through
	// the projects service.
	needsLocalCI
	// needsPreview registers the routes that run and stop a card's dev server, read its preview,
	// and take and serve the before and after screenshots (B6.6, build-plan 6.6 and 6.7, section
	// 11.2). It is a bit of its own and not part of needsProjects because the preview outlives any
	// one look at a project: it is one dev server per card, on its own port, in its own browser
	// profile. It carries needsProjects beside it, because a card's worktree and its project's dev
	// command are read through the projects service.
	needsPreview
	// needsMemory registers the two routes that read and write a card's note (B7.4, N11, build-plan
	// task 7.12). It carries needsProjects beside it, because the card a note belongs to is read
	// through the projects service - the note's file is found from the card's number and title.
	needsMemory
	// rawBody marks a route that reads its body as it is and not as JSON, such as an image upload.
	// It is not a service, and has ignores it.
	rawBody
)

// routeSpec is one domain route: its method and path, the services it needs, and its handler.
type routeSpec struct {
	pattern string
	needs   routeNeeds
	handle  func(*Server, http.ResponseWriter, *http.Request)
}

// domainRoutes lists every route for projects, boards, cards, sessions, and agents. It is the only
// place they are named, both for registering them and for the test that checks each one asks for
// a token, so a new route cannot be left out of that test.
func domainRoutes() []routeSpec {
	return []routeSpec{
		{"GET /v1/projects", needsProjects, (*Server).listProjects},
		{"POST /v1/projects", needsProjects, (*Server).createProject},
		{"GET /v1/projects/{id}", needsProjects, (*Server).getProject},
		{"PATCH /v1/projects/{id}", needsProjects, (*Server).updateProject},
		{"DELETE /v1/projects/{id}", needsProjects, (*Server).removeProject},
		{"GET /v1/projects/{id}/board", needsProjects, (*Server).getBoard},
		{"POST /v1/projects/{id}/cards", needsProjects, (*Server).createCard},
		{"GET /v1/cards/{id}", needsProjects, (*Server).getCard},
		{"POST /v1/cards/{id}/start", needsProjects | needsSessions, (*Server).startCard},
		{"POST /v1/cards/{id}/move", needsProjects, (*Server).moveCard},
		{"PATCH /v1/cards/{id}", needsProjects, (*Server).updateCard},
		{"DELETE /v1/cards/{id}", needsProjects, (*Server).deleteCard},
		{"POST /v1/cards/{id}/fork", needsProjects, (*Server).forkCard},
		{"POST /v1/cards/{id}/messages", needsSessions, (*Server).sendMessage},
		{"POST /v1/cards/{id}/stop", needsSessions, (*Server).stopCard},
		{"POST /v1/cards/{id}/resume", needsSessions, (*Server).resumeCard},
		{"POST /v1/cards/{id}/pause", needsSessions, (*Server).pauseCard},
		{"POST /v1/cards/{id}/unpause", needsSessions, (*Server).unpauseCard},
		{"POST /v1/cards/{id}/sleep", needsSessions, (*Server).sleepCard},
		{"POST /v1/cards/{id}/wake", needsSessions, (*Server).wakeCard},
		{"POST /v1/cards/{id}/pin", needsSessions, (*Server).pinCard},
		{"POST /v1/cards/{id}/unpin", needsSessions, (*Server).unpinCard},
		{"POST /v1/cards/{id}/view", needsSessions, (*Server).setCardView},
		{"POST /v1/cards/{id}/handoff", needsProjects | needsSessions, (*Server).handoffCard},
		{"POST /v1/cards/{id}/bypass", needsSessions, (*Server).setCardBypass},
		{"DELETE /v1/cards/{id}/bypass", needsSessions, (*Server).clearCardBypass},
		{"GET /v1/cards/{id}/note", needsProjects | needsMemory, (*Server).getNote},
		{"PUT /v1/cards/{id}/note", needsProjects | needsMemory, (*Server).saveNote},
		{"GET /v1/projects/{id}/lessons", needsProjects | needsMemory, (*Server).listLessons},
		{"POST /v1/projects/{id}/lessons", needsProjects | needsMemory, (*Server).saveLesson},
		{"GET /v1/projects/{id}/lessons/{slug}", needsProjects | needsMemory, (*Server).getLesson},
		{"PUT /v1/projects/{id}/lessons/{slug}", needsProjects | needsMemory, (*Server).saveLesson},
		{"DELETE /v1/projects/{id}/lessons/{slug}", needsProjects | needsMemory, (*Server).deleteLesson},
		{"POST /v1/cards/{id}/plan/approve", needsProjects | needsSessions, (*Server).approvePlan},
		{"POST /v1/cards/{id}/plan/reject", needsProjects | needsSessions, (*Server).rejectPlan},
		{"PUT /v1/cards/{id}/plan", needsProjects | needsSessions, (*Server).editPlan},
		{"GET /v1/cards/{id}/checkpoints", needsProjects | needsSessions, (*Server).listCheckpoints},
		{"POST /v1/cards/{id}/checkpoints/{cp}/restore", needsProjects | needsSessions, (*Server).restoreCheckpoint},
		{"POST /v1/approvals/{id}", needsSessions, (*Server).decideApproval},
		{"GET /v1/cards/{id}/messages", needsHistory, (*Server).listMessages},
		{"GET /v1/cards/{id}/messages/{messageId}", needsHistory, (*Server).getMessage},
		{"GET /v1/cards/{id}/activity", needsHistory, (*Server).listActivity},
		{"GET /v1/cards/{id}/diff", needsDiff, (*Server).getCardDiff},
		{"GET /v1/cards/{id}/diff/{path...}", needsDiff, (*Server).getFileHunks},
		{"GET /v1/home/dashboard", needsDashboard, (*Server).home},
		{"GET /v1/home/activity", needsDashboard, (*Server).homeActivity},
		{"GET /v1/agents", needsCatalog, (*Server).listAgents},
		{"GET /v1/projects/{id}/labels", needsProjects, (*Server).listLabels},
		{"POST /v1/projects/{id}/labels", needsProjects, (*Server).createLabel},
		{"PATCH /v1/labels/{id}", needsProjects, (*Server).updateLabel},
		{"DELETE /v1/labels/{id}", needsProjects, (*Server).deleteLabel},
		{"POST /v1/agents/refresh", needsCatalog, (*Server).refreshAgents},
		{"GET /v1/projects/{id}/chats", needsChats, (*Server).listChats},
		{"POST /v1/projects/{id}/chats", needsChats, (*Server).createChat},
		{"PATCH /v1/chats/{id}", needsChats, (*Server).updateChat},
		{"POST /v1/chats/{id}/archive", needsChats, (*Server).archiveChat},
		{"POST /v1/chats/{id}/restore", needsChats, (*Server).restoreChat},
		{"DELETE /v1/chats/{id}", needsChats, (*Server).deleteChat},
		{"POST /v1/chats/{id}/messages", needsChats, (*Server).sendChatMessage},
		{"GET /v1/chats/{id}/messages", needsHistory, (*Server).listChatMessages},
		{"GET /v1/chats/{id}/messages/{messageId}", needsHistory, (*Server).getChatMessage},
		{"GET /v1/search", needsSearch, (*Server).getSearch},
		{"GET /v1/me", needsAccounts, (*Server).getMe},
		{"PATCH /v1/me", needsAccounts, (*Server).updateMe},
		{"POST /v1/me/avatar", needsAccounts | rawBody, (*Server).setAvatar},
		{"DELETE /v1/me/avatar", needsAccounts, (*Server).removeAvatar},
		{"GET /v1/users", needsAccounts, (*Server).listUsers},
		{"GET /v1/users/{id}/avatar", needsAccounts, (*Server).getAvatar},
		{"GET /v1/me/progress", needsAccounts, (*Server).getProgress},
		{"PATCH /v1/me/progress", needsAccounts, (*Server).updateProgress},
		{"GET /v1/me/preferences", needsAccounts, (*Server).getPreferences},
		{"PATCH /v1/me/preferences", needsAccounts, (*Server).updatePreferences},
		{"POST /v1/dev/reset-first-launch", needsAccounts | needsDevMode, (*Server).resetFirstLaunch},
		{"GET /v1/projects/{id}/saved-views", needsProjects, (*Server).listSavedViews},
		{"POST /v1/projects/{id}/saved-views", needsProjects, (*Server).createSavedView},
		{"PATCH /v1/saved-views/{id}", needsProjects, (*Server).updateSavedView},
		{"DELETE /v1/saved-views/{id}", needsProjects, (*Server).deleteSavedView},
		{"GET /v1/audit", needsAudit, (*Server).listAudit},
		{"GET /v1/audit/search", needsAudit, (*Server).searchAudit},
		{"GET /v1/audit/export", needsAudit, (*Server).exportAudit},
		{"GET /v1/providers", needsProviders, (*Server).listProviders},
		{"PUT /v1/providers/{id}", needsProviders, (*Server).saveProvider},
		{"DELETE /v1/providers/{id}", needsProviders, (*Server).removeProvider},
		{"POST /v1/providers/{id}/test", needsProviders | needsConnectionTests, (*Server).testProvider},
		{"GET /v1/limits", needsCostLimits, (*Server).listLimits},
		{"PUT /v1/limits/{scope}/{kind}", needsCostLimits, (*Server).setLimit},
		{"DELETE /v1/limits/{scope}/{kind}", needsCostLimits, (*Server).deleteLimit},
		{"GET /v1/roles", needsRoles, (*Server).listRoles},
		{"POST /v1/roles", needsRoles, (*Server).createRole},
		{"GET /v1/roles/{name}", needsRoles, (*Server).getRole},
		{"PATCH /v1/roles/{name}", needsRoles, (*Server).updateRole},
		{"DELETE /v1/roles/{name}", needsRoles, (*Server).deleteRole},
		{"POST /v1/roles/{name}/reset", needsRoles, (*Server).resetRole},
		{"PUT /v1/roles/{name}/override", needsRoles, (*Server).setRoleOverride},
		{"POST /v1/cards/{id}/pull-request", needsProjects | needsPullRequests, (*Server).openPullRequest},
		{"POST /v1/cards/{id}/merge", needsProjects | needsIntegrator, (*Server).mergeCard},
		{"POST /v1/cards/{id}/review", needsProjects | needsReview, (*Server).reviewCard},
		{"GET /v1/notices", needsSessions, (*Server).listNotices},
		{"POST /v1/notices/{id}/actions", needsSessions, (*Server).noticeAction},
		{"DELETE /v1/notices/{id}", needsSessions, (*Server).dismissNotice},
		{"GET /v1/settings/sleep", needsSleepSettings, (*Server).getSleepSettings},
		{"PUT /v1/settings/sleep", needsSleepSettings, (*Server).setSleepSettings},
		{"GET /v1/cards/{id}/findings", needsProjects | needsQuality, (*Server).cardFindings},
		{"POST /v1/cards/{id}/findings/{findingId}/fix", needsProjects | needsQuality, (*Server).fixFinding},
		{"POST /v1/cards/{id}/findings/{findingId}/dismiss", needsProjects | needsQuality, (*Server).dismissFinding},
		{"GET /v1/projects/{id}/smell-profile", needsProjects | needsQuality, (*Server).getSmellProfile},
		{"PUT /v1/projects/{id}/smell-profile", needsProjects | needsQuality, (*Server).setSmellProfile},
		{"GET /v1/ci", needsCI, (*Server).getCI},
		{"POST /v1/cards/{id}/ci-failure", needsCI | needsDevMode, (*Server).simulateCIFailure},
		{"POST /v1/cards/{id}/local-ci", needsProjects | needsLocalCI, (*Server).runLocalCI},
		{"GET /v1/cards/{id}/preview", needsProjects | needsPreview, (*Server).getPreview},
		{"POST /v1/cards/{id}/preview/start", needsProjects | needsPreview, (*Server).startPreview},
		{"POST /v1/cards/{id}/preview/stop", needsProjects | needsPreview, (*Server).stopPreview},
		{"POST /v1/cards/{id}/preview/shots", needsProjects | needsPreview, (*Server).takePreviewShot},
		{"GET /v1/cards/{id}/preview/shots/{file}", needsProjects | needsPreview, (*Server).getPreviewShot},
		{"GET /v1/integrations", needsIntegrations, (*Server).listIntegrations},
		{"PUT /v1/integrations/{id}", needsIntegrations, (*Server).saveIntegration},
		{"DELETE /v1/integrations/{id}", needsIntegrations, (*Server).removeIntegration},
		{"POST /v1/integrations/{id}/test", needsIntegrations | needsConnectionTests, (*Server).testIntegration},
	}
}

// addDomainRoutes registers the routes whose services the server has. Every one is protected.
func (s *Server) addDomainRoutes(r *router) {
	for _, spec := range domainRoutes() {
		if !s.has(spec.needs) {
			continue
		}
		register := r.protected
		if spec.needs&rawBody != 0 {
			register = r.upload
		}
		register(spec.pattern, func(w http.ResponseWriter, req *http.Request) {
			spec.handle(s, w, req)
		})
	}
}

// has reports whether the server has every service in needs.
func (s *Server) has(needs routeNeeds) bool {
	switch {
	case needs&needsProjects != 0 && s.projects == nil:
		return false
	case needs&needsSessions != 0 && s.sessions == nil:
		return false
	case needs&needsCatalog != 0 && s.catalog == nil:
		return false
	case needs&needsDashboard != 0 && s.dashboard == nil:
		return false
	case needs&needsHistory != 0 && s.history == nil:
		return false
	case needs&needsDiff != 0 && s.diff == nil:
		return false
	case needs&needsChats != 0 && s.chats == nil:
		return false
	case needs&needsSearch != 0 && s.search == nil:
		return false
	case needs&needsAccounts != 0 && s.accounts == nil:
		return false
	case needs&needsAudit != 0 && s.auditlog == nil:
		return false
	case needs&needsProviders != 0 && s.providers == nil:
		return false
	case needs&needsCostLimits != 0 && s.costLimits == nil:
		return false
	case needs&needsConnectionTests != 0 && s.connectionTests == nil:
		return false
	case needs&needsRoles != 0 && s.roles == nil:
		return false
	case needs&needsPullRequests != 0 && s.pullRequests == nil:
		return false
	case needs&needsIntegrator != 0 && s.integrator == nil:
		return false
	case needs&needsReview != 0 && s.review == nil:
		return false
	case needs&needsSleepSettings != 0 && s.sleepSettings == nil:
		return false
	case needs&needsQuality != 0 && s.quality == nil:
		return false
	case needs&needsIntegrations != 0 && s.integrations == nil:
		return false
	case needs&needsCI != 0 && s.ci == nil:
		return false
	case needs&needsLocalCI != 0 && s.localCI == nil:
		return false
	case needs&needsPreview != 0 && s.preview == nil:
		return false
	case needs&needsMemory != 0 && s.memory == nil:
		return false
	case needs&needsDevMode != 0 && !s.settings.Dev():
		return false
	}
	return true
}

// lookup is a route that reads one thing by the id in its address: how to read the id, and how to
// ask the service for the thing.
type lookup[T any] struct {
	idOf  func(*http.Request) (string, error)
	fetch func(context.Context, string) (T, error)
}

// serve reads the id, asks the service, and sends the thing with a 200. It is the whole body of
// the plain GET routes.
func (l lookup[T]) serve(s *Server, w http.ResponseWriter, r *http.Request) {
	id, err := l.idOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	value, err := l.fetch(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, value)
}

// maxEchoedIDBytes cuts an id from the address before it is sent back in an error, so a very long
// address is not repeated in full.
const maxEchoedIDBytes = 64

// notFoundID is the answer for a project or a card that cannot be found. A malformed id gets the
// same answer as an unknown one, because it can never exist and the two must not be told apart.
// It is built the way the services build theirs, so the two are the same.
func notFoundID(kind, id string) *protocol.Error {
	if len(id) > maxEchoedIDBytes {
		id = id[:maxEchoedIDBytes]
	}
	return protocol.NotFound(kind).With("id", id)
}

// projectIDOf reads the project id of the address. An id that is not the shape of a project id is
// not found, without asking the service.
func projectIDOf(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if !protocol.ValidProjectID(id) {
		return "", notFoundID("project", id)
	}
	return id, nil
}

// cardIDOf reads the card id of the address. An id that is not the shape of an opaque id is not
// found, without asking the service.
func cardIDOf(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if !protocol.ValidID(id) {
		return "", notFoundID("card", id)
	}
	return id, nil
}

// labelIDOf reads a label id from the address. A label id is opaque, like a card's, so an id that
// is not the right shape is not found rather than passed on.
func labelIDOf(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if !protocol.ValidID(id) {
		return "", notFoundID("label", id)
	}
	return id, nil
}

// chatIDOf reads a chat id from the address. A chat id is opaque, like a card's, so an id that is
// not the right shape is not found rather than passed on.
func chatIDOf(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if !protocol.ValidID(id) {
		return "", notFoundID("chat", id)
	}
	return id, nil
}

// slowAnswerWindow is how long starting or resuming a card, or cloning a repository for a new
// project, may take before its answer is written.
// It makes a worktree and then starts an agent program that has a start time limit of its own, so
// it can take a minute or more, and the server's usual write limit of 60 seconds would cut the
// connection while the work is still going on. Only the two routes that need it get this.
const slowAnswerWindow = 3 * time.Minute

// allowSlowAnswer gives this one request the longer window to write its answer in. The server's
// read limit needs no such care, because Go stops applying it once the request has been read and
// does not end the request's context when it runs out, and a test keeps that true.
func (s *Server) allowSlowAnswer(w http.ResponseWriter) {
	// The deadline is on the connection, so it is made from the wall clock. The injected clock
	// is for the times that people read, and a test that fixes it in the past would end every
	// slow request at once.
	err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(slowAnswerWindow))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		s.log.Warn("could not give a slow request more time to answer", "error", err)
	}
}
