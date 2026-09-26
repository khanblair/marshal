package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A project's saved views (docs/backend-checklist.md B2.5). The rules (a name that is not empty and
// not taken, filters and a swimlane the board knows, a limit on how many) are in internal/projects;
// these handlers read the address and the body and write the answer.

// listSavedViews is GET /v1/projects/{id}/saved-views: a project's views, the one saved longest ago
// first. A project with none has an empty list, never null.
func (s *Server) listSavedViews(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.SavedViewListSnapshot]{projectIDOf, s.projects.SavedViews}.serve(s, w, r)
}

// createSavedView is POST /v1/projects/{id}/saved-views. A name the project already uses replaces
// that view and keeps its id, so the answer is 200; a view that is new is 201 with its address.
func (s *Server) createSavedView(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.CreateSavedViewRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	view, created, err := s.projects.CreateSavedView(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	if !created {
		s.writeJSON(w, http.StatusOK, view)
		return
	}
	w.Header().Set("Location", "/v1/saved-views/"+view.ID)
	s.writeJSON(w, http.StatusCreated, view)
}

// updateSavedView is PATCH /v1/saved-views/{id}: rename a view, or change its filters or swimlane.
func (s *Server) updateSavedView(w http.ResponseWriter, r *http.Request) {
	id, err := savedViewIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.UpdateSavedViewRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	view, err := s.projects.UpdateSavedView(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, view)
}

// deleteSavedView is DELETE /v1/saved-views/{id}.
func (s *Server) deleteSavedView(w http.ResponseWriter, r *http.Request) {
	id, err := savedViewIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.projects.DeleteSavedView(r.Context(), id); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}

// savedViewIDOf reads a saved view id from the address. A saved view id is opaque, like a card's,
// so an id that is not the right shape is not found rather than passed on.
func savedViewIDOf(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if !protocol.ValidID(id) {
		return "", notFoundID("saved view", id)
	}
	return id, nil
}
