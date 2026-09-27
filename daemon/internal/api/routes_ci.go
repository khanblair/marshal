package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// CI health (docs/architecture.md sections 9 and 11.1, docs/backend-checklist.md B6.2, B6.4, and
// B6.6, build-plan 6.2 and 6.10). One route reads CI: every project Marshal holds a workflow run
// for, with each project's runs newest first. The card's own badge is not read here - it travels on
// the card the board already fetches, in `Card.CI` - and the events that move both are published by
// internal/ci itself.
//
// The read answers the same shape the `ci.updated` event carries on the home topic, so a screen that
// draws the CI health page can apply an event and a first read with one piece of code. The second
// route, on a dev daemon only, makes a failure happen on one card's branch.

// getCI is GET /v1/ci: every project's CI health, stamped with the daemon's time.
func (s *Server) getCI(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.ci.Snapshot(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, snapshot)
}

// simulateCIFailure is POST /v1/cards/{id}/ci-failure: make a CI failure happen on one card's branch,
// which is the card menu's "Simulate CI failure" (B6.4, build-plan 6.10, N28, decision D5).
//
// Both modes answer the same shape, so the screen that shows which one ran does not branch:
//
//   - synthetic makes up a failed run and injects it through the monitor's own path, so nothing
//     outside the daemon changes and no forge is asked for anything.
//   - real commits a deliberately failing workflow file to the card's branch and pushes it, so the
//     forge's Actions really run and the failure arrives later as an ordinary delivery.
//
// The mode is not checked here: the monitor owns the two modes and answers a mode it does not know
// with the sentence that names both. The route itself exists only on a dev daemon (needsDevMode),
// because the real mode spends Actions minutes on the person's own GitHub account and a normal
// install must have no address for it.
func (s *Server) simulateCIFailure(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.SimulateCIFailureRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	result, err := s.ci.Simulate(r.Context(), id, req.Mode)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}
