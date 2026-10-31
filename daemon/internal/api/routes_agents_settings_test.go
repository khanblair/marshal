package api_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Turning an agent off is kept, and the list shows it at once.
func TestTurningAnAgentOffIsKeptAndListed(t *testing.T) {
	st := newStack(t, withRealCatalog(&countingProbe{}))
	saved := st.do(http.MethodPut, "/v1/agents/claude/settings", map[string]any{"path": "", "off": true}).want(t, http.StatusOK)
	if agent := decode[protocol.Agent](t, saved); !agent.Off || agent.Kind != protocol.AgentKindClaude {
		t.Errorf("saved claude = %+v, want it off", agent)
	}
	list := decode[protocol.AgentCatalog](t, st.do(http.MethodGet, "/v1/agents", nil).want(t, http.StatusOK))
	for _, agent := range list.Agents {
		if agent.Kind == protocol.AgentKindClaude && !agent.Off {
			t.Errorf("listed claude = %+v, want it off", agent)
		}
	}
}

// A path that is not a file the agent can run is refused with the reason, and nothing is kept.
func TestAChosenPathThatIsNotTheAgentIsRefused(t *testing.T) {
	st := newStack(t, withRealCatalog(&countingProbe{}))
	notAgent := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(notAgent, []byte("#!/bin/sh\necho 1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path string }{
		{"a relative path", "claude"},
		{"a file that is not Claude Code", notAgent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st.do(http.MethodPut, "/v1/agents/claude/settings", map[string]any{"path": tc.path}).want(t, http.StatusBadRequest)
		})
	}
	list := decode[protocol.AgentCatalog](t, st.do(http.MethodGet, "/v1/agents", nil).want(t, http.StatusOK))
	for _, agent := range list.Agents {
		if agent.Kind == protocol.AgentKindClaude && agent.CustomPath != "" {
			t.Errorf("claude kept a path after a refusal: %+v", agent)
		}
	}
}

func TestAgentSettingsAnswerForWhatCannotBeSet(t *testing.T) {
	st := newStack(t, withRealCatalog(&countingProbe{}))
	st.do(http.MethodPut, "/v1/agents/nope/settings", map[string]any{"off": true}).want(t, http.StatusNotFound)
	st.do(http.MethodPut, "/v1/agents/builtin/settings", map[string]any{"off": true}).want(t, http.StatusBadRequest)
}
