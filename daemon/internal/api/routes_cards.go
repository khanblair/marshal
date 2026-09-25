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
