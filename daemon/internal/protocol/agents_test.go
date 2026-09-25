package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

var agentsNow = time.Date(2026, time.September, 25, 10, 20, 0, 0, time.UTC)

// sampleAgents has one agent of each status, so the golden file shows every field in use.
func sampleAgents() []protocol.Agent {
	return []protocol.Agent{
		{
			Kind: protocol.AgentKindClaude, Name: "Claude Code", Version: "2.1.282",
			Status: protocol.AgentStatusSupported,
			Models: []protocol.AgentModel{
				{ID: "sonnet", Name: "Sonnet (latest)", Thinking: true},
				{ID: "haiku", Name: "Haiku (latest)", Thinking: false},
			},
			Capabilities: protocol.AgentCapabilities{
				Resume: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, MCP: true,
			},
		},
		{
			Kind: protocol.AgentKindGemini, Name: "Gemini CLI", Version: "0.36.0",
			Status:  protocol.AgentStatusUntested,
			Warning: "Marshal has not been tested with Gemini CLI 0.36.0. It usually works, but if something looks wrong, try version 0.35.1.",
			Models:  []protocol.AgentModel{{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Thinking: true}},
			Capabilities: protocol.AgentCapabilities{
				Resume: true, StructuredEvents: true, ModelSwitching: true, MCP: true, Approvals: true,
			},
		},
		{
			Kind: protocol.AgentKindCodex, Name: "Codex", Status: protocol.AgentStatusMissing,
			InstallHint: "Codex is not installed. Install it with: npm install -g @openai/codex",
			Models:      []protocol.AgentModel{{ID: "gpt-5-codex", Name: "GPT-5 Codex", Thinking: true}},
		},
	}
}

func TestAgentCatalogGolden(t *testing.T) {
	testutil.Golden(t, "agents", protocol.NewAgentCatalog(sampleAgents(), agentsNow))
}

func TestAgentCatalogNeverEncodesAListAsNull(t *testing.T) {
	empty, err := json.Marshal(protocol.NewAgentCatalog(nil, agentsNow))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"agents":[],"serverTime":"2026-09-25T10:20:00.000Z"}`
	if string(empty) != want {
		t.Errorf("got %s\nwant %s", empty, want)
	}
	// An agent with no models still sends a list.
	noModels, err := json.Marshal(protocol.NewAgentCatalog([]protocol.Agent{{Kind: protocol.AgentKindCodex}}, agentsNow))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Agents []map[string]json.RawMessage `json:"agents"`
	}
	if err := json.Unmarshal(noModels, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := string(decoded.Agents[0]["models"]); got != "[]" {
		t.Errorf("models = %s, want []", got)
	}
}

func TestAgentCatalogDoesNotChangeTheCallersList(t *testing.T) {
	agents := []protocol.Agent{{Kind: protocol.AgentKindCodex}}
	protocol.NewAgentCatalog(agents, agentsNow)
	if agents[0].Models != nil {
		t.Error("NewAgentCatalog changed the list it was given")
	}
}

func TestAgentCatalogCarriesNoPath(t *testing.T) {
	body, err := json.Marshal(protocol.NewAgentCatalog(sampleAgents(), agentsNow))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Agents []map[string]json.RawMessage `json:"agents"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, agent := range decoded.Agents {
		for key := range agent {
			if key == "path" || key == "command" {
				t.Errorf("the catalog sends %q, which would show the person's folders", key)
			}
		}
	}
}
