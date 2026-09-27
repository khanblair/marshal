package api

import (
	"context"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/connectiontest"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The connections Marshal is set up with, apart from model providers (docs/architecture.md section
// 18, B6.1 and B6.7, build-plan 6.11). GitHub is the one this phase builds; the rest are listed so
// the settings screen shows every row and read as "not connected" until their own phase.
//
// Saving is a whole-connection call - the App's id, its installation, its private key, and its
// webhook secret arrive together - because none of them is any use alone, and the row's status is
// about all of them at once. Nothing secret comes back from any of these: the wire shape carries the
// id, the kind, the status, and the last test's own answer, and no key.

// listIntegrations is GET /v1/integrations: every connection Marshal knows, whether or not it is set
// up, with each one's last test folded into its status.
func (s *Server) listIntegrations(w http.ResponseWriter, r *http.Request) {
	s.writeIntegrations(w, r)
}

// saveIntegration is PUT /v1/integrations/{id}: store or replace one connection. The body is the
// GitHub App's shape today; a later phase's connection will have its own, and the id in the address
// says which one is being saved, so an id with no save shape yet is refused rather than silently
// accepted.
//
// The test that follows a save runs without the cooldown, because the connection has just changed
// and the last result is about something that is no longer stored (section 18).
func (s *Server) saveIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id != integrations.GitHubID {
		s.writeError(w, protocol.NotFound("connection").With("id", id))
		return
	}
	var req protocol.SaveGitHubRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.integrations.SaveGitHub(r.Context(), req); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.integrationsTestAfterConnect(r.Context(), id)
	s.writeIntegrations(w, r)
}

// removeIntegration is DELETE /v1/integrations/{id}: forget a connection's settings and its secret.
// A connection that was never set up is not an error - the answer is the same list either way.
func (s *Server) removeIntegration(w http.ResponseWriter, r *http.Request) {
	if err := s.integrations.Remove(r.Context(), r.PathValue("id")); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeIntegrations(w, r)
}

// testIntegration is POST /v1/integrations/{id}/test: run one connection's test now and answer with
// its result. It answers the same shape a provider's test does (protocol.TestResult), so a screen
// shows both kinds of test the same way.
//
// A test that ran and found something wrong is a 200 with a failed check - that is the answer the
// person asked for. The call fails only when the test could not be run at all, or when it is asked
// for again inside the cooldown, which is a conflict with how long to wait.
func (s *Server) testIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := integrations.Lookup(id); !ok {
		s.writeError(w, protocol.NotFound("connection").With("id", id))
		return
	}
	result, err := s.connectionTests.Run(r.Context(), s.integrationKind(id), id,
		connectiontest.TesterFunc(func(callCtx context.Context) (protocol.TestResult, error) {
			return s.integrations.Test(callCtx, id)
		}))
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}

// integrationKind is the kind one connection's test is filed under, which is what keys its cooldown
// and its saved result. The connection's own info owns it, so a later phase's connection answers
// here without this file knowing its name.
func (s *Server) integrationKind(id string) string {
	if info, ok := integrations.Lookup(id); ok {
		return info.Kind
	}
	return id
}

// integrationsTestAfterConnect runs the test a save is followed by, ignoring the cooldown. It is
// deliberately quiet: the save has already happened, so a test that could not be run must not turn
// the answer into a failure. The result, if there is one, is saved under the connection's id and
// comes back in the list this handler writes next.
func (s *Server) integrationsTestAfterConnect(ctx context.Context, id string) {
	if s.connectionTests == nil {
		return
	}
	_, err := s.connectionTests.RunAfterConnect(ctx, s.integrationKind(id), id,
		connectiontest.TesterFunc(func(callCtx context.Context) (protocol.TestResult, error) {
			return s.integrations.Test(callCtx, id)
		}))
	if err != nil && s.log != nil {
		s.log.Warn("a connection was saved but its connection test could not be run",
			"connection", id, "err", err)
	}
}

// writeIntegrations sends the whole list, stamped with the daemon's time. Every connection route but
// the test answers with it, so a change and a read are the same shape and a screen redraws from one
// answer whatever changed.
func (s *Server) writeIntegrations(w http.ResponseWriter, r *http.Request) {
	list, err := s.integrations.List(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, protocol.NewIntegrationList(list, s.now()))
}
