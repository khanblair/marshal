package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The bypass routes: POST /v1/cards/{id}/bypass turns bypass permissions on for a card, and DELETE
// /v1/cards/{id}/bypass turns it off (docs/backend-checklist.md B3.2, inventory N8).
//
// Bypass is not a field of a card that an edit sets: it is granted through these two calls, because
// granting it carries the acknowledgement a person gave (the body of the POST) and the project's
// bypass lock. Both answers are the card as it is afterwards, so a client that turned it on from a
// settings menu sees the mode it landed in without a second read.
//
// Turning it on is announced to every view the card has, because the card itself is what changed:
// the session manager writes the mode through the projects service, which publishes card.updated on
// the card's own topic (critical, so no client keeps drawing a card whose rules changed).

// setCardBypass is POST /v1/cards/{id}/bypass. The body is {"acknowledged":true}; anything else is
// refused with the reason "unacknowledged", so bypass cannot be turned on by a body that omits it.
func (s *Server) setCardBypass(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.BypassRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.SetBypass(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// clearCardBypass is DELETE /v1/cards/{id}/bypass: turn bypass off, leaving the card in full auto.
// It has no body: turning bypass off is always allowed, including for a card that was never in it.
func (s *Server) clearCardBypass(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.ClearBypass(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}
