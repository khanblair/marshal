package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The checkpoint routes (docs/backend-checklist.md B5.3, build-plan 5.21, docs/architecture.md
// section 10): a card's restore points, and putting a card back to one. The restore route is named
// in the architecture doc's route list (section 11.3): POST /v1/cards/{id}/checkpoints/{cp}/restore.
//
// There is no route that makes one by hand: Marshal makes a checkpoint before every turn, and a
// person's own checkpoint is a later phase. The list is read-only, newest first, and only the restore
// route changes anything. Both routes need the projects service, which owns the card, and the
// session manager, which owns the card's worktree: a restore is a Git operation on the worktree the
// manager started.
//
// Every answer is the shape the screen needs: the list of restore points, or the card as it now
// stands after a restore.

// listCheckpoints is GET /v1/cards/{id}/checkpoints: a card's restore points, newest first.
// A card that has never started has none, which is an empty list and not an error.
func (s *Server) listCheckpoints(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	list, err := s.sessions.ListCheckpoints(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// restoreCheckpoint is POST /v1/cards/{id}/checkpoints/{cp}/restore: put the card's worktree and
// branch back to one of its restore points, and answer the card as it now is.
//
// The body is optional. An absent body, and a body with no fields, both mean a worktree restore; a
// body that sets conversation asks for the card's own conversation to be put back as well. A
// checkpoint that is not on this card is not found, an id that cannot exist is not found the same
// way, and a card whose agent is mid-turn is refused, because a restore would race the writes the
// turn is making.
func (s *Server) restoreCheckpoint(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	checkpointID := r.PathValue("cp")
	if !protocol.ValidID(checkpointID) {
		s.writeError(w, notFoundID("checkpoint", checkpointID))
		return
	}
	var req protocol.RestoreCheckpointRequest
	if r.ContentLength != 0 {
		if err := s.decodeJSON(r, &req); err != nil {
			s.writeError(w, err)
			return
		}
	}
	card, err := s.sessions.RestoreCheckpoint(r.Context(), id, checkpointID, req.Conversation)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}
