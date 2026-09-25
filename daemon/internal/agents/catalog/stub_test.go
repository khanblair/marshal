package catalog

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestStubReportsEveryAgentSupported(t *testing.T) {
	now := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	stub := NewStub(StubClock(func() time.Time { return now }))
	list, err := stub.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !list.ServerTime.Time().Equal(now) || len(list.Agents) != 3 {
		t.Fatalf("catalog = %d agents at %v, want 3 at %v", len(list.Agents), list.ServerTime.Time(), now)
	}
	for _, agent := range list.Agents {
		if agent.Status != protocol.AgentStatusSupported || agent.Version != StubVersion || agent.Warning != "" {
			t.Errorf("%s = %q %q %q, want supported at version %q", agent.Kind, agent.Status, agent.Version,
				agent.Warning, StubVersion)
		}
		if agent.Capabilities != everything() {
			t.Errorf("%s capabilities = %+v, want all on", agent.Kind, agent.Capabilities)
		}
		if len(agent.Models) == 0 {
			t.Errorf("%s has no models", agent.Kind)
		}
	}
}

func TestStubHasThePrototypesModels(t *testing.T) {
	list, err := NewStub().List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// The AGENTS table of apps/web/src/mock/constants.ts.
	want := map[protocol.AgentKind][]string{
		protocol.AgentKindClaude: {"claude-sonnet-4-5", "claude-opus-4-1", "claude-haiku-4-5"},
		protocol.AgentKindCodex:  {"gpt-5-codex", "gpt-5", "gpt-5-mini"},
		protocol.AgentKindGemini: {"gemini-2.5-pro", "gemini-2.5-flash"},
	}
	for _, agent := range list.Agents {
		var got []string
		for _, m := range agent.Models {
			got = append(got, m.ID)
			if !m.Thinking {
				t.Errorf("%s model %s cannot think, but the prototype lets every one of these think", agent.Kind, m.ID)
			}
		}
		if len(got) != len(want[agent.Kind]) {
			t.Errorf("%s models = %v, want %v", agent.Kind, got, want[agent.Kind])
			continue
		}
		for i := range got {
			if got[i] != want[agent.Kind][i] {
				t.Errorf("%s models = %v, want %v", agent.Kind, got, want[agent.Kind])
			}
		}
	}
}

func TestStubMissingMakesKindsMissing(t *testing.T) {
	stub := NewStub(StubMissing(protocol.AgentKindCodex, protocol.AgentKindGemini))
	list, err := stub.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range list.Agents {
		missing := agent.Kind != protocol.AgentKindClaude
		if (agent.Status == protocol.AgentStatusMissing) != missing {
			t.Errorf("%s status = %q, want missing: %v", agent.Kind, agent.Status, missing)
		}
		if missing && (agent.InstallHint == "" || agent.Version != "") {
			t.Errorf("%s = hint %q version %q, want an install hint and no version", agent.Kind, agent.InstallHint, agent.Version)
		}
		if !missing && agent.InstallHint != "" {
			t.Errorf("%s has an install hint though it is present", agent.Kind)
		}
	}
	detected, err := stub.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range detected {
		if d.Startable != (d.Kind == protocol.AgentKindClaude) {
			t.Errorf("%s startable = %v", d.Kind, d.Startable)
		}
	}
}

func TestStubRefreshIsList(t *testing.T) {
	stub := NewStub()
	refreshed, err := stub.Refresh(t.Context())
	if err != nil || len(refreshed.Agents) != 3 {
		t.Errorf("Refresh = %d agents, %v, want 3 and no error", len(refreshed.Agents), err)
	}
}
