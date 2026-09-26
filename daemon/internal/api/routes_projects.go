package api

import (
	"context"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// listProjects is GET /v1/projects: every project with its badges.
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	list, err := s.projects.List(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// createProject is POST /v1/projects: add a repository, from a folder or by cloning it.
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req protocol.CreateProjectRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if req.Source == protocol.ProjectSourceClone {
		// A clone of a large repository can take longer than the server's usual time to answer.
		s.allowSlowAnswer(w)
	}
	project, err := s.projects.Create(r.Context(), req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	w.Header().Set("Location", "/v1/projects/"+project.ID)
	s.writeJSON(w, http.StatusCreated, project)
}

// getProject is GET /v1/projects/{id}.
func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.Project]{projectIDOf, s.projects.Get}.serve(s, w, r)
}

// updateProject is PATCH /v1/projects/{id}: change the fields that are sent.
func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.UpdateProjectRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	project, err := s.projects.Update(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, project)
}

// removeProject is DELETE /v1/projects/{id}: stop managing a project. Its body is optional, and
// an empty one keeps nothing. The repository folder is never deleted.
func (s *Server) removeProject(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var opts protocol.RemoveProjectRequest
	if r.ContentLength != 0 {
		if err := s.decodeJSON(r, &opts); err != nil {
			s.writeError(w, err)
			return
		}
	}
	if err := s.projects.Remove(r.Context(), id, opts); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}

// getBoard is GET /v1/projects/{id}/board: the columns and every card.
func (s *Server) getBoard(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.BoardSnapshot]{projectIDOf, s.projects.Board}.serve(s, w, r)
}

// createCard is POST /v1/projects/{id}/cards: add a card to the backlog. The card is marked as
// made by the person whose token was used, never by anything in the body.
func (s *Server) createCard(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	caller, ok := Principal(r.Context())
	if !ok {
		s.writeError(w, errUnauthorized())
		return
	}
	var req protocol.CreateCardRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	// A card is always written in the backlog. When it is asked for in planning or working, its
	// session is started straight after, so a card is never shown as working before its agent
	// exists. This route stays registered without a session manager (the registry test asks for
	// that), so a start state it cannot honour is refused here, before anything is written.
	needsStart := req.StartState != "" && req.StartState != protocol.CardStateBacklog
	if needsStart && s.sessions == nil {
		s.writeError(w, protocol.Unavailable("Marshal cannot start an agent right now. Add the card to the backlog instead."))
		return
	}
	card, err := s.projects.CreateCard(r.Context(), id, req, projects.WithCreatedBy(caller.UserID))
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	if needsStart {
		// Starting an agent can take a minute or more, so this answer gets the longer window.
		s.allowSlowAnswer(w)
		if card, err = s.startNewCard(r.Context(), card, req.StartState); err != nil {
			s.writeError(w, err)
			return
		}
	}
	w.Header().Set("Location", "/v1/cards/"+card.ID)
	s.writeJSON(w, http.StatusCreated, card)
}

// startNewCard starts the session of a card that was just added, and leaves it in the state the
// person asked for. It takes a while, so its request gets the longer window (allowSlowAnswer).
// When the agent cannot start, the card is removed again: adding a card either works or leaves
// nothing behind, and the person can add it to the backlog instead.
func (s *Server) startNewCard(ctx context.Context, card protocol.Card, state protocol.CardState) (protocol.Card, error) {
	started, err := s.sessions.Start(ctx, card.ID)
	if err != nil {
		if undoErr := s.projects.DeleteCard(ctx, card.ID); undoErr != nil {
			s.log.Error("could not remove a card whose agent would not start",
				"card_id", card.ID, "error", undoErr)
		}
		return protocol.Card{}, startError(err)
	}
	if state == protocol.CardStatePlanning {
		// The session is running: the card waits in Planning while its agent writes the plan.
		return s.projects.SetState(ctx, card.ID, protocol.CardStatePlanning)
	}
	return started, nil
}
