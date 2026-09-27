package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The approval route: POST /v1/approvals/{id} answers one permission request an agent is blocked
// on (docs/architecture.md 11.4, checklist item B3.4, N7). The request is announced by the session
// pump on the card's or the chat's topic when the agent asks (approval.requested); this route is
// the one answer. It writes nothing else: the answer is announced as approval.resolved, and every
// view of the same approval follows it.
//
// A body of {"decision":"approved"} or {"decision":"denied"} is required, with an optional
// "optionId" to pick the exact option the agent offered (allow_always rather than allow_once, for
// example). The decision is delivered to the agent's own protocol unchanged, so the person's answer
// is what the agent acts on.

// approvalIDOf reads the approval id of the address. An approval id is opaque, like a card's, so an
// id that is not the right shape is not found rather than passed on.
func approvalIDOf(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if !protocol.ValidID(id) {
		return "", notFoundID("approval", id)
	}
	return id, nil
}

// decideApproval is POST /v1/approvals/{id}: answer a waiting approval. A request nobody is waiting
// on any more is refused (409 when it was already answered, 404 when there is no such request).
func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request) {
	id, err := approvalIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.DecideApprovalRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.sessions.Respond(r.Context(), id, req.Decision, req.OptionID, audit.ActorPerson); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}
