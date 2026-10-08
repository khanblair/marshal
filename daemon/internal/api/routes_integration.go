package api

import (
	"net/http"
	"os"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The Integration tab's routes: where finished cards are headed, the controls on the Integrator,
// and showing a card's worktree on this machine. The merge queue itself is behind
// integrator.Reader; these routes only read it and press its controls.

const (
	messageOnlyOnThisMachine = "Marshal opens a folder only on the computer it runs on. " +
		"Use this from that computer, not from a phone or another machine."
	messageChooseOpenWith = `Choose "finder" or "editor" to open the folder with.`
	messageNoWorktree     = "This card has no worktree yet. Start the card first, then open its folder."
	messageWorktreeGone   = "This card's worktree folder is no longer there."
	messageCouldNotOpen   = "Marshal could not open the folder on this computer. Try again."
)

// integrationState is GET /v1/projects/{id}/integration: the merge queue, what was delivered, and
// what the Integrator is doing.
func (s *Server) integrationState(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.IntegrationState]{projectIDOf, s.integration.State}.serve(s, w, r)
}

// pauseIntegration is POST /v1/projects/{id}/integration/pause: stop taking cards off the queue.
func (s *Server) pauseIntegration(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.IntegrationState]{projectIDOf, s.integration.Pause}.serve(s, w, r)
}

// resumeIntegration is POST /v1/projects/{id}/integration/resume: take cards off the queue again.
func (s *Server) resumeIntegration(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.IntegrationState]{projectIDOf, s.integration.Resume}.serve(s, w, r)
}

// retryMerge is POST /v1/cards/{id}/merge/retry: run a card's merge or delivery again after a stop
// that needed the owner.
func (s *Server) retryMerge(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.Card]{cardIDOf, s.integration.Retry}.serve(s, w, r)
}

// undoMerge is POST /v1/cards/{id}/merge/undo: put the integration branch back to where it was
// before the card's merge.
func (s *Server) undoMerge(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.Card]{cardIDOf, s.integration.Undo}.serve(s, w, r)
}

// sendToMerge is POST /v1/cards/{id}/send-to-merge: a person says a card's committed work is
// finished, for a project with no GitHub origin, where no pull request will ever send it to Ready to
// merge. The answer is the card as it now is.
func (s *Server) sendToMerge(w http.ResponseWriter, r *http.Request) {
	lookup[protocol.Card]{cardIDOf, s.integration.SendToMerge}.serve(s, w, r)
}

// openWorktree is POST /v1/cards/{id}/worktree/open: show the card's worktree in the file manager or
// the editor of the machine the daemon runs on. Only a request from this machine is answered, and
// the only thing the request chooses is how to show the folder: the folder is the card's own.
func (s *Server) openWorktree(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		s.writeError(w, protocol.Forbidden(messageOnlyOnThisMachine))
		return
	}
	id, err := cardIDOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	var req protocol.OpenWorktreeRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if req.With != openWithFinder && req.With != openWithEditor {
		s.writeError(w, protocol.InvalidArgument(messageChooseOpenWith))
		return
	}
	path, _, err := s.projects.Worktree(r.Context(), id)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	if err := checkWorktreeFolder(path); err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.opener.Open(r.Context(), path, req.With); err != nil {
		s.writeError(w, protocol.Unavailable(messageCouldNotOpen).WithCause(err))
		return
	}
	s.writeNoContent(w)
}

// checkWorktreeFolder is the refusal for a card that has no worktree folder to show, or nil.
func checkWorktreeFolder(path string) *protocol.Error {
	if path == "" {
		return protocol.Refused(messageNoWorktree)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return protocol.Refused(messageWorktreeGone)
	}
	return nil
}
