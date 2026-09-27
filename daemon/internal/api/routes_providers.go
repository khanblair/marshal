package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/connectiontest"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// The model providers and the ceilings on what they may cost (docs/backend-checklist.md B4.5, B4.6,
// and N18, build-plan tasks 4.9 and 4.10). The rules are in internal/providers. These handlers read
// the path and the body, call the service, and write the answer. A provider's key is never in an
// answer: only the masked form the service makes is.
//
// Saving a key and testing one are separate calls. Saving stores the key and then runs one test, so a
// person who has just typed a key is told whether it works without pressing anything (section 18,
// "a test runs automatically right after a connection is added"); testing runs one on demand, with a
// cooldown, and is the only provider route that ever calls a provider.

// listProviders is GET /v1/providers: every provider Marshal knows, whether or not a key is stored,
// with a stored key shown only as a masked value.
func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	s.writeProviders(w, r)
}

// saveProvider is PUT /v1/providers/{id}: store a key, or the address of a local provider's server,
// replacing whatever was there. The whole list comes back, so a screen redraws from one answer and
// the key itself is never sent.
//
// The test that follows a save is part of the same call, so the answer carries what the test found:
// a key the provider refused comes back as an invalid row with the sentence to show, and a good key
// as a tested row. A test that the daemon could not run at all is logged and does not fail the save:
// the key is stored, which is what the request was for, and the row still says what is known.
func (s *Server) saveProvider(w http.ResponseWriter, r *http.Request) {
	var req protocol.SaveProviderRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	id := r.PathValue("id")
	if err := s.providers.Save(id, req.Key); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.testAfterConnect(r.Context(), id)
	s.writeProviders(w, r)
}

// testProvider is POST /v1/providers/{id}/test: run one connection test now and answer with its
// result. It answers the same shape every connection's test does (protocol.TestResult), so a screen
// shows a provider's test and a later phase's integration test the same way.
//
// A test that ran and found a bad key is a 200 with a failed check - that is the answer the person
// asked for. The call fails only when the test could not be run at all, or when it is asked for
// again inside the cooldown, which is a conflict with how long to wait.
func (s *Server) testProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := s.connectionTests.Run(r.Context(), connectiontest.KindProvider, id,
		connectiontest.TesterFunc(func(callCtx context.Context) (protocol.TestResult, error) {
			return s.providers.Test(callCtx, id)
		}))
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}

// removeProvider is DELETE /v1/providers/{id}: forget a stored key. A provider that had none is not
// an error - there was nothing to remove and the answer is the same list either way, which is how
// removing an avatar that is not there behaves.
func (s *Server) removeProvider(w http.ResponseWriter, r *http.Request) {
	err := s.providers.Remove(r.PathValue("id"))
	if err != nil && !errors.Is(err, security.ErrNoKey) {
		s.writeError(w, translate(err))
		return
	}
	s.writeProviders(w, r)
}

// testAfterConnect runs the test a save is followed by, ignoring the cooldown (the key has just
// changed, so the last result is about a key that is no longer stored). It is deliberately quiet: the
// save has already happened, so a test that could not be run must not turn the answer into a failure.
// The result, if there is one, is saved under the provider's id and comes back in the list this
// handler writes next.
func (s *Server) testAfterConnect(ctx context.Context, id string) {
	if s.connectionTests == nil {
		return
	}
	_, err := s.connectionTests.RunAfterConnect(ctx, connectiontest.KindProvider, id,
		connectiontest.TesterFunc(func(callCtx context.Context) (protocol.TestResult, error) {
			return s.providers.Test(callCtx, id)
		}))
	if err != nil && s.log != nil {
		s.log.Warn("a provider was saved but its connection test could not be run",
			"provider", id, "err", err)
	}
}

// writeProviders sends the list, stamped with the daemon's time. It is the whole answer of every
// provider route but the test, so a change and a read are the same shape.
//
// Each row is filled with the last saved test of that provider before it is sent, and a row whose
// last test failed its key check is reported as invalid with the sentence to show. That is what makes
// the invalid state reachable: nothing else in the daemon knows a key is bad, because nothing else
// ever asks the provider.
func (s *Server) writeProviders(w http.ResponseWriter, r *http.Request) {
	list, err := s.providers.List()
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	if s.connectionTests != nil {
		for i := range list {
			last, ok, err := s.connectionTests.Last(r.Context(), list[i].ID)
			if err != nil {
				s.writeError(w, translate(err))
				return
			}
			if !ok {
				continue
			}
			list[i].LastTest = &last
			applyProviderTest(&list[i])
		}
	}
	s.writeJSON(w, http.StatusOK, protocol.NewProviderList(list, s.now()))
}

// applyProviderTest folds what a test found into the row a screen reads. A test that failed any check
// makes a provider with a stored key invalid, and the first failed check's fix - or, without one, its
// message - becomes the one sentence the row shows. A test that only warned leaves the row saved: the
// key works, which is what the row's state is about.
func applyProviderTest(p *protocol.Provider) {
	if p.LastTest == nil || p.LastTest.OK || p.Status != protocol.ProviderStatusSaved {
		return
	}
	failed, ok := p.LastTest.FirstFailed()
	if !ok {
		return
	}
	p.Status = protocol.ProviderStatusInvalid
	p.Error = failed.Fix
	if p.Error == "" {
		p.Error = failed.Message
	}
}

// listLimits is GET /v1/limits: every cost and awake ceiling that is set, global ones first.
func (s *Server) listLimits(w http.ResponseWriter, r *http.Request) {
	s.writeLimits(w, r)
}

// setLimit is PUT /v1/limits/{scope}/{kind}: set or replace one ceiling. Which ceiling it is comes
// from the path and the number from the body, so one handler serves every scope and every kind. The
// kind is passed on as the path wrote it, even when it names nothing: the service owns the list of
// kinds and refuses an unknown one with a sentence, and repeating the list here would let the two
// drift apart.
func (s *Server) setLimit(w http.ResponseWriter, r *http.Request) {
	var req protocol.SetLimitRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	scope, kind := r.PathValue("scope"), protocol.LimitKind(r.PathValue("kind"))
	if _, err := s.costLimits.Set(r.Context(), scope, kind, req.Value); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeLimits(w, r)
}

// deleteLimit is DELETE /v1/limits/{scope}/{kind}: remove one ceiling. A scope that had none of that
// kind is not an error - the answer is the same list either way.
func (s *Server) deleteLimit(w http.ResponseWriter, r *http.Request) {
	scope, kind := r.PathValue("scope"), protocol.LimitKind(r.PathValue("kind"))
	if _, err := s.costLimits.Delete(r.Context(), scope, kind); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeLimits(w, r)
}

// writeLimits sends the whole list of ceilings. Every limits route answers with it, so a screen
// redraws its form from one answer whatever changed.
func (s *Server) writeLimits(w http.ResponseWriter, r *http.Request) {
	list, err := s.costLimits.List(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}
