package api

import "net/http"

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
