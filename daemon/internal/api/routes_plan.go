package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The plan routes: the answers to the plan a card waits on in plan-first mode
// (docs/backend-checklist.md B5.2, inventory N6). A person approves the plan, rejects it, or changes
// its steps and leaves it waiting.
//
// A plan is a message in the card's chat and not a table of its own
// (docs/marshal-product-scope.md 10.3), so there is no route that reads one back on its own: the
// plan arrives with the chat it is in, and the newest plan message of a card is the one a person is
// answering (GET /v1/cards/{id}/messages).
//
// Every answer is the card as it now stands, the way a move answers, and the answer is announced as
// plan.updated on the card's topic, so every view of the same plan follows one decision without
// reading the chat back. A card with no plan, and a plan that has already been answered, are
// refused before anything is written (404 and 409).

// approvePlan is POST /v1/cards/{id}/plan/approve: accept the plan a card waits on. It starts the
// work and takes the card out of plan-only, which is what lets the agent change files rather than
// plan them, so a card waits in Planning until its plan is approved.
func (s *Server) approvePlan(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.ApprovePlan(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// rejectPlan is POST /v1/cards/{id}/plan/reject: send the plan back, so the agent writes another
// one. The card returns to Planning.
func (s *Server) rejectPlan(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.RejectPlan(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// editPlan is PUT /v1/cards/{id}/plan: replace the steps of the plan a card waits on with the ones
// a person left, and leave the plan waiting. Only the steps change: the files, the risks, and the
// checks are the agent's own reading of the work.
func (s *Server) editPlan(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.EditPlanRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.EditPlan(r.Context(), id, req.Steps)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}
