package api

import (
	"net/http"
)

// The review route (docs/backend-checklist.md B5.4, build-plan 5.7): run the Reviewer role over a
// card's pull request. It answers what the review did: whether the pull request was approved, where
// the card went, how many comments were posted, and whether the agent that wrote the branch was
// told. A verdict that is not in yet ("the checks are still running") answers 200 with Waiting true
// and leaves the card exactly as it was.
//
// The route needs the review service, which needs a forge token and the Reviewer role. On a machine
// with no forge token it is not registered at all (see the needsReview route bit).
func (s *Server) reviewCard(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	result, err := s.review.Review(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}
