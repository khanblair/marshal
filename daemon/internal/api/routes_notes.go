package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's note over the API (docs/architecture.md sections 10 and 12; docs/backend-checklist.md
// B7.4 and N11; build-plan task 7.12). It is the Notes tab's two calls: read the card's note, and
// write it.
//
// The note is a markdown file in the vault rather than a row, so these routes are thin on purpose:
// where it lives, what it says when nothing has been saved, and what the file and the row mean when
// the two disagree are all decided in internal/memory. A route reads the card's id, calls the
// module, and writes what it answered.

// getNote is GET /v1/cards/{id}/note: the card's note, whole. A card nothing has been saved for
// answers the note the daemon would start one from, with no save time, and reading never writes
// anything: the file appears on the first save. The author is "person" for a note the owner has not
// touched yet, which is what the tab shows.
func (s *Server) getNote(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	note, err := s.memory.Note(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, note)
}

// saveNote is PUT /v1/cards/{id}/note: replace the card's note with the body. It answers the note as
// a read would, so a client that saves and draws the answer shows exactly what the next read shows.
//
// The body is the whole note and not a patch, which is what one file per card means. It is written
// by the owner: the author is "person", and a note an agent wrote through its own tool keeps saying
// "agent" until a person saves over it.
func (s *Server) saveNote(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.SaveNoteRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	note, err := s.memory.SaveNote(r.Context(), id, req.Body, protocol.NoteAuthorPerson)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, note)
}
