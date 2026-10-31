package api

import (
	"errors"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The Trello apply routes (docs/marshal-product-scope.md section 19.2). Reading the board and what is
// pending never writes to Trello. Only apply does, and only the changes the person names.

func trelloRoutes() []routeSpec {
	return []routeSpec{
		{"GET /v1/projects/{id}/trello/pending", needsIntegrations | needsProjects, (*Server).trelloPending},
		{"POST /v1/projects/{id}/trello/apply", needsIntegrations | needsProjects, (*Server).trelloApply},
		{"POST /v1/projects/{id}/trello/dismiss", needsIntegrations | needsProjects, (*Server).trelloDismiss},
		{"POST /v1/projects/{id}/trello/refresh", needsIntegrations | needsProjects, (*Server).trelloRefresh},
	}
}

// trelloError turns what the Trello sync answered into a refusal a person can act on.
func (s *Server) trelloError(err error) error {
	switch {
	case errors.Is(err, integrations.ErrTrelloNotLinked):
		return protocol.Conflict("This project is not linked to a Trello board. Link one in Settings, under Integrations.")
	case errors.Is(err, integrations.ErrNotConnected):
		return protocol.Refused("Trello is not connected yet. Connect it in Settings, under Integrations.")
	}
	return translate(err)
}

// trelloPending is GET /v1/projects/{id}/trello/pending: what Marshal would change in Trello. A
// project with no board answers linked false and nothing else.
func (s *Server) trelloPending(w http.ResponseWriter, r *http.Request) {
	pending, err := s.integrations.PendingTrello(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, s.trelloError(err))
		return
	}
	s.writeJSON(w, http.StatusOK, pending)
}

// trelloApply is POST /v1/projects/{id}/trello/apply: write the named changes to Trello.
func (s *Server) trelloApply(w http.ResponseWriter, r *http.Request) {
	var req protocol.ApplyTrelloRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	result, err := s.integrations.ApplyTrello(r.Context(), r.PathValue("id"), req)
	if err != nil {
		s.writeError(w, s.trelloError(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}

// trelloDismiss is POST /v1/projects/{id}/trello/dismiss: stop showing the named changes.
func (s *Server) trelloDismiss(w http.ResponseWriter, r *http.Request) {
	var req protocol.DismissTrelloRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	id := r.PathValue("id")
	if err := s.integrations.DismissTrello(r.Context(), id, req.ChangeIDs); err != nil {
		s.writeError(w, s.trelloError(err))
		return
	}
	pending, err := s.integrations.PendingTrello(r.Context(), id)
	if err != nil {
		s.writeError(w, s.trelloError(err))
		return
	}
	s.writeJSON(w, http.StatusOK, pending)
}

// trelloRefresh is POST /v1/projects/{id}/trello/refresh: read the board now. It changes Marshal and
// never Trello.
func (s *Server) trelloRefresh(w http.ResponseWriter, r *http.Request) {
	var req protocol.RefreshTrelloRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	id := r.PathValue("id")
	counts, err := s.integrations.RefreshTrello(r.Context(), id, req.KeepTrelloCardIDs)
	if err != nil {
		s.writeError(w, s.trelloError(err))
		return
	}
	pending, err := s.integrations.PendingTrello(r.Context(), id)
	if err != nil {
		s.writeError(w, s.trelloError(err))
		return
	}
	s.writeJSON(w, http.StatusOK, protocol.RefreshTrelloResult{
		Imported: counts.Imported, Updated: counts.Updated, Pending: pending,
	})
}
