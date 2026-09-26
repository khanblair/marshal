package api

import (
	"net/http"
)

// A card's diff (docs/backend-checklist.md B2.9, docs/backend-inventory.md N15). The list route
// answers with the changed files and their counts and never with hunks; one file's hunks come from
// its own route, which the screen calls when that file is opened. Both answers are bounded in
// internal/diff, so a huge diff cannot be returned whole and a large file cannot stall the screen.

// getCardDiff is GET /v1/cards/{id}/diff: the card's changed files with their counts, and no
// hunks. An unknown card is not found; a card that never started has an empty diff, not an error.
func (s *Server) getCardDiff(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	answer, err := s.diff.Files(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// getFileHunks is GET /v1/cards/{id}/diff/{path}: one changed file's hunks, loaded when the screen
// opens that file. The path is the rest of the address, so a file inside a folder is one address
// with its slashes, and a path that is not part of the card's diff is not found.
func (s *Server) getFileHunks(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	answer, err := s.diff.Hunks(r.Context(), id, r.PathValue("path"))
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}
