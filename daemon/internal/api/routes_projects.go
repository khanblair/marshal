package api

import (
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
	card, err := s.projects.CreateCard(r.Context(), id, req, projects.WithCreatedBy(caller.UserID))
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	w.Header().Set("Location", "/v1/cards/"+card.ID)
	s.writeJSON(w, http.StatusCreated, card)
}
