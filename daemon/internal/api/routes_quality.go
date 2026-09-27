package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The quality routes (docs/backend-checklist.md B5.8, build-plan 5.12 to 5.15, docs/architecture.md
// section 17, docs/ui-rules.md section 14): a card's code-smell findings, and one project's smell
// profile.
//
// A finding is a fact about the commit it was found in, so the two calls that act on one ask the
// daemon and write down what happened there rather than being worked out on the client: asking the
// card's agent to fix a finding sends it a message and marks the finding as handed over, and
// dismissing one keeps the reason. Neither call moves the card: a blocking finding is what the move
// to In review consults (section 17.1), and the move itself is still the projects module's.
//
// The read route runs nothing. The checks are run when a card moves to In review, and by the
// turns that changed code (sections 17.1 and 17.2), never while a screen is being drawn: a GET
// that started a project's linter would hold a request open for as long as the linter takes.

// cardFindings is GET /v1/cards/{id}/findings: the findings of one card as of its current commit,
// grouped by the client. It answers what the checks last saved and runs nothing, so it is as fast
// as any other read; a card that has not been checked yet, or a card that never started, answers an
// empty list with no checked time, which is the empty state the card's checks panel draws.
func (s *Server) cardFindings(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	list, err := s.quality.Findings(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// fixFinding is POST /v1/cards/{id}/findings/{findingId}/fix: ask the card's agent to fix one
// finding, and answer the finding as it now stands.
//
// The agent is told in the card's own session, which wakes it if it is asleep. A finding that was
// handed over reads "fixed" whether or not the code has been changed yet: the next check of a newer
// commit is what says whether the smell is really gone. A card with no agent to ask is refused with
// its own reason, and a finding that is not on this card is not found, as is an id that cannot
// exist.
func (s *Server) fixFinding(w http.ResponseWriter, r *http.Request) {
	id, findingID, err := findingAddress(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	finding, err := s.quality.Fix(r.Context(), id, findingID)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, finding)
}

// dismissFinding is POST /v1/cards/{id}/findings/{findingId}/dismiss: wave one finding away with a
// reason, and answer the finding as it now stands. The reason is required, because a dismissed
// finding keeps why the code is fine as it is, and the reasons are what a later phase's automatic
// lessons learn from (build-plan 5.15).
func (s *Server) dismissFinding(w http.ResponseWriter, r *http.Request) {
	id, findingID, err := findingAddress(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.DismissFindingRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	finding, err := s.quality.Dismiss(r.Context(), id, findingID, req.Reason)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, finding)
}

// findingAddress reads the card and the finding ids of a finding route. Both are opaque ids, so one
// of the wrong shape is not found rather than passed on to the service.
func findingAddress(r *http.Request) (cardID, findingID string, err error) {
	cardID, err = cardIDOf(r)
	if err != nil {
		return "", "", err
	}
	findingID = r.PathValue("findingId")
	if !protocol.ValidID(findingID) {
		return "", "", notFoundID("finding", findingID)
	}
	return cardID, findingID, nil
}

// getSmellProfile is GET /v1/projects/{id}/smell-profile: one project's smell profile, with every
// threshold and every check filled in. A project that has never been edited answers the defaults,
// so the screen has numbers to draw before anything is saved.
func (s *Server) getSmellProfile(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	profile, err := s.quality.Profile(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, profile)
}

// setSmellProfile is PUT /v1/projects/{id}/smell-profile: save one project's smell profile, and
// answer it resolved. The profile is one document and is replaced as a whole, because a screen
// edits it as a whole. Thresholds outside the allowed range, a check Marshal does not know, named
// twice, and a linter with no command are all refused with the field that is wrong.
func (s *Server) setSmellProfile(w http.ResponseWriter, r *http.Request) {
	id, err := projectIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var in protocol.SmellProfile
	if err := s.decodeJSON(r, &in); err != nil {
		s.writeError(w, err)
		return
	}
	profile, err := s.quality.SetProfile(r.Context(), id, in)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, profile)
}
