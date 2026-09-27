package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A project's lessons over the API (docs/architecture.md sections 10 and 12; docs/backend-checklist.
// md B7.4, B7.6; build-plan task 7.13). Where a card has one note, a project has many lessons, so
// these four routes are the list-get-save-delete shape N-lists elsewhere in this package use, rather
// than the card note's two-route read-and-replace.
//
// Every route reads and writes through internal/memory, the same as the card note routes do, and for
// the same reason: the lesson is a markdown file in the vault, and the module is what decides where
// it lives and what the file and the row mean when the two disagree.

// listLessons is GET /v1/projects/{id}/lessons: a project's lessons, newest first, for the lessons
// screen (build-plan task 7.13).
func (s *Server) listLessons(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	lessons, err := s.memory.ListLessons(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, lessons)
}

// getLesson is GET /v1/projects/{id}/lessons/{slug}: one lesson, whole. Unlike a card note, there is
// no placeholder for a slug nobody has saved a lesson under - it answers not found.
func (s *Server) getLesson(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	slug, err := lessonSlugOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	lesson, err := s.memory.Lesson(r.Context(), id, slug)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, lesson)
}

// saveLesson answers two routes with the one shape a save needs: POST /v1/projects/{id}/lessons
// makes a lesson the first time, when the screen has no slug yet to put anything at, and PUT
// /v1/projects/{id}/lessons/{slug} edits one it already has. Both read the same body and write the
// same way; the slug in a PUT's address is informational only - the slug that is actually saved to
// is made fresh from the title in the body, the same way a save always has been - so retitling a
// lesson moves it to a new slug rather than failing on a mismatched address. A screen that wants to
// keep editing the same lesson after a retitle reads the slug back from the answer.
func (s *Server) saveLesson(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.SaveLessonRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	lesson, err := s.memory.SaveLesson(r.Context(), id, req.Title, req.Body, protocol.NoteAuthorPerson)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, lesson)
}

// deleteLesson is DELETE /v1/projects/{id}/lessons/{slug}: remove a lesson outright, file and row.
func (s *Server) deleteLesson(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	slug, err := lessonSlugOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.memory.DeleteLesson(r.Context(), id, slug); err != nil {
		s.writeError(w, translate(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// lessonSlugOf reads a lesson's slug from the address. It is not an opaque id like a card's, so it
// is checked for shape rather than looked up: the letters, digits, and hyphens noteSlug ever writes,
// and nothing that could walk a path (migration 0019's slug column, internal/memory.noteSlug).
func lessonSlugOf(r *http.Request) (string, error) {
	slug := r.PathValue("slug")
	if slug == "" || len(slug) > maxEchoedIDBytes {
		return "", notFoundID("lesson", slug)
	}
	for _, c := range slug {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
		default:
			return "", notFoundID("lesson", slug)
		}
	}
	return slug, nil
}
