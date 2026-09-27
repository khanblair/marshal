package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Local CI (docs/architecture.md sections 9 and 11.1, docs/backend-checklist.md B6.5, build-plan
// 6.5, docs/marshal-product-scope.md 15.3): the project's own workflow files, read one step at a
// time, with what Marshal ran in the card's worktree and what it refused to run.
//
// The route is the daemon's side of "run the same checks as GitHub Actions on your machine, before
// pushing". It changes nothing outside the worktree: a step that would deploy or publish is
// reported rather than run, so a person can press this without anything leaving their machine.

// runLocalCI is POST /v1/cards/{id}/local-ci: run the workflow steps of one card's worktree. The
// body names one workflow file, or every workflow the worktree has when it names none.
func (s *Server) runLocalCI(w http.ResponseWriter, r *http.Request) {
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.LocalCIRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	result, err := s.localCI.Run(r.Context(), id, req.Workflow)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}
