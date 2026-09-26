package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// setCardView is POST /v1/cards/{id}/view: switch a card's agent between the chat view and the
// terminal view (docs/architecture.md 4.3). It stops the agent's process and resumes the same
// session id in the other mode, so the conversation carries over. The answer is the view the card is
// in and the state of its session; asking for the view the card is already in changes nothing.
//
// The rules and their sentences live in the session manager, which owns what a session's process
// is. A refusal is a 422 with the code "refused", the stable details.reason of
// protocol.ViewRefusalReason, and the sentence the app shows, and it leaves the card and its session
// as they were, except one that says the new process could not pick the session up, which moves the
// card to Needs you (architecture.md 5.3). A card that does not exist is not found.
//
// Like starting a card, it starts a process, so it can take longer than an ordinary request and it
// gets the longer time to answer.
func (s *Server) setCardView(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.SetViewRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if !req.Mode.Valid() {
		s.writeError(w, protocol.InvalidArgument("The view must be chat or terminal."))
		return
	}
	s.allowSlowAnswer(w)
	view, err := s.sessions.SwitchView(r.Context(), id, req.Mode)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, view)
}
