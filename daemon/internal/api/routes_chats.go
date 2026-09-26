package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A project's chats (docs/backend-checklist.md B2.10). The list, the create, the four changes to one
// chat, and the message a person sends it are all thin: the rules (a name that is not empty, a
// target that is a real target, a chat that exists and is not archived) are in internal/chats, and
// these handlers only read the address, the query, and the body, and write the answer. A chat's
// history is paged by the same service that pages a card's, through routes_history.go.

// listChats is GET /v1/projects/{id}/chats: one project's chats, most recently active first. The
// main list is the live chats; `archived=true` asks for the archived ones instead. An unknown
// project is not found, and a project with no chats is an empty list, never null.
func (s *Server) listChats(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	archived, err := archivedFilter(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	snapshot, err := s.chats.List(r.Context(), id, archived)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, snapshot)
}

// createChat is POST /v1/projects/{id}/chats: make a chat in this project, with its own session.
func (s *Server) createChat(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.CreateChatRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	chat, err := s.chats.Create(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	w.Header().Set("Location", "/v1/chats/"+chat.ID)
	s.writeJSON(w, http.StatusCreated, chat)
}

// updateChat is PATCH /v1/chats/{id}: rename a chat. A body with no title leaves it as it is, and a
// name that is empty or only spaces is refused.
func (s *Server) updateChat(w http.ResponseWriter, r *http.Request) {
	id, err := chatIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.UpdateChatRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	chat, err := s.chats.Update(r.Context(), id, req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, chat)
}

// archiveChat is POST /v1/chats/{id}/archive: the chat leaves the main list and its session is put
// to sleep.
func (s *Server) archiveChat(w http.ResponseWriter, r *http.Request) {
	id, err := chatIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	chat, err := s.chats.Archive(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, chat)
}

// restoreChat is POST /v1/chats/{id}/restore: the chat comes back to the main list.
func (s *Server) restoreChat(w http.ResponseWriter, r *http.Request) {
	id, err := chatIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	chat, err := s.chats.Restore(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, chat)
}

// deleteChat is DELETE /v1/chats/{id}: the chat's session is stopped, its logs are deleted, and the
// chat goes. Cards the chat made stay on the board.
func (s *Server) deleteChat(w http.ResponseWriter, r *http.Request) {
	id, err := chatIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.chats.Remove(r.Context(), id); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeNoContent(w)
}

// archivedFilter reads the optional `archived` parameter of the chat list. Absent or "false" is the
// main list; "true" is the archived one. Anything else is refused rather than read as false, so a
// client that misspells it is told.
func archivedFilter(r *http.Request) (bool, error) {
	switch r.URL.Query().Get("archived") {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, protocol.InvalidArgument("The archived filter is true or false.")
	}
}

// sendChatMessage is POST /v1/chats/{id}/messages: put a message into the chat's own session,
// starting the session when it is not running. The body is the one a card's message has, and the
// answer arrives on the event stream, on the topic chat:<id>. Starting the agent can take a minute
// or more, so the route gets the longer time to answer, as starting a card does. A chat that is
// archived is refused, and one that is not there is not found.
func (s *Server) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	id, err := chatIDOf(r)
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
	s.allowSlowAnswer(w)
	if err := s.chats.Send(r.Context(), id, req.Text); err != nil {
		s.writeError(w, startError(err))
		return
	}
	s.writeNoContent(w)
}
