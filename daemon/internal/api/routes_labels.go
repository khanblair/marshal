package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// listLabels is GET /v1/projects/{id}/labels: a project's managed labels, by name.
func (s *Server) listLabels(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.LabelSnapshot]{projectIDOf, s.projects.Labels}.serve(s, w, r)
}

// createLabel is POST /v1/projects/{id}/labels. A name the project already uses is a conflict.
func (s *Server) createLabel(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.CreateLabelRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	label, err := s.projects.CreateLabel(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusCreated, label)
}

// updateLabel is PATCH /v1/labels/{id}: rename a label, recolor it, or both.
func (s *Server) updateLabel(w http.ResponseWriter, r *http.Request) {
	id, err := labelIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.UpdateLabelRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	label, err := s.projects.UpdateLabel(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, label)
}

// deleteLabel is DELETE /v1/labels/{id}: remove the label and take it off every card that had it.
func (s *Server) deleteLabel(w http.ResponseWriter, r *http.Request) {
	id, err := labelIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.projects.DeleteLabel(r.Context(), id); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}
