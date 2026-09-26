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
type routeNeeds uint16

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
	// needsDevMode limits a route to a dev daemon: a normal daemon has no such address at all.
	needsDevMode
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
