package api

import "net/http"

// getSearch is GET /v1/search?q=: what the command palette and the top bar look for. One answer
// holds every kind, each kind in its own list, best match first. The rules (an empty query
// matches nothing, a query too long is refused) are in internal/search; this handler only reads
// the query and writes the answer.
func (s *Server) getSearch(w http.ResponseWriter, r *http.Request) {
	answer, err := s.search.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}
