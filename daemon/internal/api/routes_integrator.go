package api

import (
	"net/http"
)

// The merge-queue route (docs/backend-checklist.md B5.5, build-plan 5.8): send a card that is in
// Ready to merge through the Integrator. It answers the Integrator's result: whether the target
// branch moved forward and the merge commit, or, when it did not, the plain sentence the card was
// sent to Needs you with.
//
// The route needs the projects service, which owns the card, and a configured Integrator. It is
// registered whenever the daemon has both; a daemon built without a merge queue has no such address.
func (s *Server) mergeCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	result, err := s.integrator.Merge(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}
