package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestAgentsThroughHTTP(t *testing.T) {
	st := newStack(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/agents"},
		{http.MethodPost, "/v1/agents/refresh"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			r := st.do(tc.method, tc.path, nil).want(t, http.StatusOK)
			sameShape(t, "agents", r.Body)
			list := decode[protocol.AgentCatalog](t, r)
			kinds := make([]protocol.AgentKind, len(list.Agents))
			for i, agent := range list.Agents {
				kinds[i] = agent.Kind
				if agent.Models == nil || agent.Status != protocol.AgentStatusSupported || agent.Version != catalog.StubVersion {
					t.Errorf("agent %s is %+v, want the stub's supported agent with a list of models", agent.Kind, agent)
				}
			}
			want := []protocol.AgentKind{protocol.AgentKindClaude, protocol.AgentKindGemini, protocol.AgentKindCodex}
			if len(kinds) != len(want) || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] {
				t.Errorf("kinds = %v, want %v", kinds, want)
			}
		})
	}
}

func TestAMissingAgentHasAnInstallHint(t *testing.T) {
	st := newStack(t, withCatalog(catalog.NewStub(catalog.StubMissing(protocol.AgentKindCodex))))
	list := decode[protocol.AgentCatalog](t, st.do(http.MethodGet, "/v1/agents", nil).want(t, http.StatusOK))
	codex := list.Agents[2]
	if codex.Kind != protocol.AgentKindCodex || codex.Status != protocol.AgentStatusMissing || codex.InstallHint == "" || codex.Version != "" {
		t.Errorf("codex = %+v, want a missing agent with an install hint", codex)
	}
}

// countingProbe finds Claude and nothing else, and counts how often it was asked.
type countingProbe struct {
	mu    sync.Mutex
	calls int
}

func (p *countingProbe) Probe(_ context.Context, kind protocol.AgentKind) (catalog.Found, error) {
	if kind != protocol.AgentKindClaude {
		return catalog.Found{}, catalog.ErrNotFound
	}
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	version, _ := catalog.ParseSemver("2.1.282")
	return catalog.Found{Path: "/opt/tools/claude/bin/claude", Version: version}, nil
}

func (p *countingProbe) asked() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// Listing uses the answer that is kept, refreshing looks again, and the place of a program on the
// machine is never in what a client is sent.
func TestListingUsesTheKeptAnswerAndRefreshingLooksAgain(t *testing.T) {
	probe := &countingProbe{}
	kept := catalog.New(catalog.Options{Probe: probe})
	st := newStack(t, withCatalog(kept))

	first := st.do(http.MethodGet, "/v1/agents", nil).want(t, http.StatusOK)
	st.do(http.MethodGet, "/v1/agents", nil).want(t, http.StatusOK)
	if probe.asked() != 1 {
		t.Errorf("two lists looked %d times, want 1", probe.asked())
	}
	refreshed := st.do(http.MethodPost, "/v1/agents/refresh", nil).want(t, http.StatusOK)
	if probe.asked() != 2 {
		t.Errorf("a refresh made it %d looks in all, want 2", probe.asked())
	}
	for _, r := range []reply{first, refreshed} {
		if strings.Contains(string(r.Body), "/opt/tools") {
			t.Errorf("the answer names a folder on the machine: %s", r.Body)
		}
		claude := decode[protocol.AgentCatalog](t, r).Agents[0]
		if claude.Version != "2.1.282" || claude.Status == protocol.AgentStatusMissing {
			t.Errorf("claude = %+v, want the version that was found", claude)
		}
	}
}

// failingSource is a catalog that cannot answer.
type failingSource struct {
	catalog.Source
	err error
}

func (f failingSource) List(context.Context) (protocol.AgentCatalog, error) {
	return protocol.AgentCatalog{}, f.err
}
func (f failingSource) Refresh(context.Context) (protocol.AgentCatalog, error) {
	return protocol.AgentCatalog{}, f.err
}

// A catalog that cannot answer is the daemon's own problem, except when it ran out of time, which
// the person can try again.
func TestACatalogThatCannotAnswer(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		code    protocol.ErrorCode
		message string
	}{
		{"an unexpected failure", errors.New("the probe exploded at /opt/tools"), http.StatusInternalServerError,
			protocol.ErrorCodeInternal, protocol.Internal().Message},
		{"a check that took too long", context.DeadlineExceeded, http.StatusServiceUnavailable,
			protocol.ErrorCodeUnavailable, "Marshal took too long to finish that. Try again."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t, withCatalog(failingSource{err: tc.err}))
			for _, route := range []struct{ method, path string }{{http.MethodGet, "/v1/agents"}, {http.MethodPost, "/v1/agents/refresh"}} {
				got := st.do(route.method, route.path, nil).apiError(t, tc.status, tc.code)
				if got.Message != tc.message {
					t.Errorf("%s %s said %q, want %q", route.method, route.path, got.Message, tc.message)
				}
			}
		})
	}
}
