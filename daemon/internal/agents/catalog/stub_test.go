package catalog

import (
	"fmt"
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
	if !list.ServerTime.Time().Equal(now) || len(list.Agents) != 4 {
		t.Fatalf("catalog = %d agents at %v, want 4 at %v", len(list.Agents), list.ServerTime.Time(), now)
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
	// The AGENTS table of apps/web/src/mock/constants.ts. The built-in agent's two models without a
	// thinking setting are the ones the picker leaves out of its thinking choices.
	want := map[protocol.AgentKind][]string{
		protocol.AgentKindClaude:  {"claude-sonnet-4-5", "claude-opus-4-1", "claude-haiku-4-5"},
		protocol.AgentKindCodex:   {"gpt-5-codex", "gpt-5", "gpt-5-mini"},
		protocol.AgentKindGemini:  {"gemini-2.5-pro", "gemini-2.5-flash"},
		protocol.AgentKindBuiltin: {"claude-sonnet-4-5", "gpt-5-mini", "deepseek-chat", "gemini-2.5-flash", "qwen2.5-coder:32b"},
	}
	noThink := map[string]bool{"deepseek-chat": true, "qwen2.5-coder:32b": true}
	for _, agent := range list.Agents {
		got := modelIDs(agent.Models)
		for _, m := range agent.Models {
			// Every model of the three CLI agents thinks; the built-in agent has the design's two
			// that do not.
			if m.Thinking == noThink[m.ID] {
				t.Errorf("%s model %s thinking = %v, which is not the prototype's", agent.Kind, m.ID, m.Thinking)
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(want[agent.Kind]) {
			t.Errorf("%s models = %v, want %v", agent.Kind, got, want[agent.Kind])
		}
	}
}

func TestStubMissingMakesKindsMissing(t *testing.T) {
	stub := NewStub(StubMissing(protocol.AgentKindCodex, protocol.AgentKindGemini))
	list, err := stub.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	askedFor := map[protocol.AgentKind]bool{
		protocol.AgentKindCodex: true, protocol.AgentKindGemini: true,
	}
	for _, agent := range list.Agents {
		missing := askedFor[agent.Kind]
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
		if d.Startable != !askedFor[d.Kind] {
			t.Errorf("%s startable = %v, want %v", d.Kind, d.Startable, !askedFor[d.Kind])
		}
	}
}

// Marshal's own agent is never one of the ones that can be missing, whatever a test asks for: there
// is nothing on the machine to find or not find. Only the kinds Marshal has to look for can be
// asked for as missing.
func TestStubMissingNeverTouchesTheBuiltInAgent(t *testing.T) {
	stub := NewStub(StubMissing(Kinds()...))
	list, err := stub.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range list.Agents {
		if agent.Kind == protocol.AgentKindBuiltin {
			if agent.Status != protocol.AgentStatusSupported || agent.Version != StubVersion {
				t.Errorf("builtin = %q at %q, want supported whatever was asked for", agent.Status, agent.Version)
			}
			continue
		}
		if agent.Status != protocol.AgentStatusMissing {
			t.Errorf("%s status = %q, want missing", agent.Kind, agent.Status)
		}
	}
}

func TestStubRefreshIsList(t *testing.T) {
	stub := NewStub()
	refreshed, err := stub.Refresh(t.Context())
	if err != nil || len(refreshed.Agents) != 4 {
		t.Errorf("Refresh = %d agents, %v, want 4 and no error", len(refreshed.Agents), err)
	}
}
