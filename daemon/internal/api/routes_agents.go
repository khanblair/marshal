package api

import (
	"errors"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
)

// listAgents is GET /v1/agents: the agents this daemon can start, from the catalog's kept answer.
func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.catalog.List(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// refreshAgents is POST /v1/agents/refresh: look for the agents again, whatever is kept.
func (s *Server) refreshAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.catalog.Refresh(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// testAgent is POST /v1/agents/{id}/test: look at one agent program again and say what was found,
// without sending any prompt. The id is an agent kind or one of the other tools the catalog lists.
func (s *Server) testAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := s.catalog.Test(r.Context(), id)
	if errors.Is(err, catalog.ErrUnknownAgent) {
		s.writeError(w, notFoundID("agent", id))
		return
	}
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}
