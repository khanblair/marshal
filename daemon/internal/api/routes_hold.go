package api

import "net/http"

// The session hold routes: POST /v1/cards/{id}/pause, /unpause, /sleep, /wake, /pin, and /unpin.
// They are the four controls a person presses, plus the call that undoes a pause and the one that
// undoes a pin, under the rules of docs/architecture.md section 5.1 (checklist item B2.15). None
// takes a body, and a body that is sent is not read, the way stopping a card ignores one.
//
// The rules and the refusal sentences live in the session manager (internal/session), because it
// owns what a session's state means. A refusal is a 422 with the code "refused", the stable
// details.reason of protocol.HoldRefusalReason, and the sentence the app shows, and it leaves the
// card and the session exactly as they were, the same shape a refused card move uses.

// pauseCard is POST /v1/cards/{id}/pause: hold a working card between turns. The turn that is
// running finishes, and a message sent meanwhile waits until the card is resumed. A card that is
// not working is refused.
func (s *Server) pauseCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.Pause(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// unpauseCard is POST /v1/cards/{id}/unpause: release a pause and deliver the message it was
// holding. A card that is not paused is left alone, so a client may call it twice.
func (s *Server) unpauseCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.Unpause(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// sleepCard is POST /v1/cards/{id}/sleep: stop the card's agent process, keep the session id, and
// record that the session is asleep, so Wake or Start brings the same conversation back. The card
// answers with no content: what changed is the session, and the session.state_changed event says
// so.
func (s *Server) sleepCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.sessions.Sleep(r.Context(), id); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}

// wakeCard is POST /v1/cards/{id}/wake: resume the card's sleeping session through its saved id,
// recording it as waking and then awake. Like resuming a card, it can take a minute or more, so
// it gets the longer time to answer.
func (s *Server) wakeCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.allowSlowAnswer(w)
	if err := s.sessions.Wake(r.Context(), id); err != nil {
		s.writeError(w, s.resumeError(r.Context(), id, err))
		return
	}
	s.writeNoContent(w)
}

// pinCard is POST /v1/cards/{id}/pin: keep a card from sleeping on its own. Automatic sleep is
// Phase 5, so today this records the person's choice and publishes the card.
func (s *Server) pinCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.Pin(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// unpinCard is POST /v1/cards/{id}/unpin: let a card sleep on its own again.
func (s *Server) unpinCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.sessions.Unpin(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}
