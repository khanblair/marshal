package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The GitHub sign-in and pasted-token routes. The sign-in is read by polling: each GET moves it on
// as far as GitHub allows, so no background loop is needed.

// startGitHubConnect is POST /v1/integrations/github/connect: ask GitHub for a code to show.
func (s *Server) startGitHubConnect(w http.ResponseWriter, r *http.Request) {
	view, err := s.integrations.StartGitHubConnect(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, view)
}

// readGitHubConnect is GET /v1/integrations/github/connect: advance the sign-in and say where it is.
// The connection test runs once, on the read that finished it.
func (s *Server) readGitHubConnect(w http.ResponseWriter, r *http.Request) {
	view, finished, err := s.integrations.ReadGitHubConnect(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	if finished {
		s.integrationsTestAfterConnect(r.Context(), integrations.GitHubID)
	}
	s.writeJSON(w, http.StatusOK, view)
}

// cancelGitHubConnect is DELETE /v1/integrations/github/connect: forget a sign-in in progress.
func (s *Server) cancelGitHubConnect(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.integrations.CancelGitHubConnect(r.Context()))
}

// saveGitHubToken is PUT /v1/integrations/github/token: check and store a pasted token.
func (s *Server) saveGitHubToken(w http.ResponseWriter, r *http.Request) {
	var req protocol.SaveGitHubTokenRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, translate(err))
		return
	}
	if err := s.integrations.SaveGitHubToken(r.Context(), req); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.integrationsTestAfterConnect(r.Context(), integrations.GitHubID)
	s.writeIntegrations(w, r)
}

// testGitHubToken is POST /v1/integrations/github/token/test: check a pasted token, saving nothing.
func (s *Server) testGitHubToken(w http.ResponseWriter, r *http.Request) {
	var req protocol.SaveGitHubTokenRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, translate(err))
		return
	}
	result, err := s.integrations.TestGitHubToken(r.Context(), req)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}
