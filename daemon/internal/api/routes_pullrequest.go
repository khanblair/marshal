package api

import (
	"net/http"
)

// The pull-request route (docs/backend-checklist.md B5.4, build-plan 5.6): open a card's branch as
// a real pull request on the project's forge. It answers the card as it now stands, with the pull
// request link recorded and the card moved to In review; a card that already has a pull request is
// returned unchanged.
//
// The route needs the projects service, which owns the card, and a configured pull-request service,
// which is what talks to the forge. On a machine with no forge token the route is not registered at
// all (see the needsPullRequests route bit).
func (s *Server) openPullRequest(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	card, err := s.pullRequests.Open(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, card)
}
