package api

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// getCard is GET /v1/cards/{id}.
func (s *Server) getCard(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.Card]{cardIDOf, s.projects.Card}.serve(s, w, r)
}

// startCard is POST /v1/cards/{id}/start: make the card's worktree, start its agent, and move the
// card to working. It takes no body, and a body that is sent is not read. It can take a minute or
// more, so it gets a longer time to answer. When the client goes away the request's context ends
// and the session manager undoes what it made.
func (s *Server) startCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.allowSlowAnswer(w)
	card, err := s.sessions.Start(r.Context(), id)
	if err != nil {
		s.writeError(w, startError(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// updateCard is PATCH /v1/cards/{id}: change the fields a person set, and leave the rest as they
// are. A field that is not in the body is not touched, so a client sends only what changed.
func (s *Server) updateCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.UpdateCardRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.projects.UpdateCard(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// deleteCard is DELETE /v1/cards/{id}: remove the card and everything Marshal made for it. It
// stops the card's session, removes its worktree and its branch, and deletes its session logs. It
// never touches the repository folder.
func (s *Server) deleteCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.projects.DeleteCard(r.Context(), id); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}

// forkCard is POST /v1/cards/{id}/fork: add a card in the backlog that starts from this card's
// latest commit, with a copy of its settings and labels. A card that never started has no commit
// to fork from, and is refused with a plain sentence.
func (s *Server) forkCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.projects.ForkCard(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	w.Header().Set("Location", "/v1/cards/"+card.ID)
	s.writeJSON(w, http.StatusCreated, card)
}

// moveCard is POST /v1/cards/{id}/move: move a card by hand, checked by the rules of
// architecture.md section 6.1. A refused move answers 422 with the reason and the sentence the app
// shows, and the card is left as it was.
func (s *Server) moveCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.MoveCardRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.projects.MoveCard(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}

// sendMessage is POST /v1/cards/{id}/messages: put a message into the card's running session. The
// answer to it arrives on the event stream.
func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.SendMessageRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if err := checkMessage(req.Text); err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.sessions.Send(r.Context(), id, req.Text); err != nil {
		s.writeError(w, s.sessionError(r.Context(), id, err))
		return
	}
	s.writeNoContent(w)
}

// checkMessage refuses a message that is empty, or only spaces, or longer than the limit. The
// text itself is never trimmed: people paste code, and its spaces matter.
func checkMessage(text string) error {
	if strings.TrimSpace(text) == "" {
		return protocol.InvalidArgument("Write a message first.")
	}
	if utf8.RuneCountInString(text) > protocol.MaxMessageChars {
		return protocol.InvalidArgument("That message is too long. Send at most 100,000 characters at a time.")
	}
	return nil
}

// stopCard is POST /v1/cards/{id}/stop: end the card's agent.
func (s *Server) stopCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.sessions.Stop(r.Context(), id); err != nil {
		s.writeError(w, s.sessionError(r.Context(), id, err))
		return
	}
	s.writeNoContent(w)
}

// resumeCard is POST /v1/cards/{id}/resume: start the agent of a card again with the session it
// had, for a card whose session was left over from an earlier run and is not running yet. Like
// starting a card, it can take a minute or more.
func (s *Server) resumeCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.allowSlowAnswer(w)
	if err := s.sessions.Resume(r.Context(), id); err != nil {
		s.writeError(w, s.resumeError(r.Context(), id, err))
		return
	}
	s.writeNoContent(w)
}
