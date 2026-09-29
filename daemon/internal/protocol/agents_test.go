package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

var agentsNow = time.Date(2026, time.September, 25, 10, 20, 0, 0, time.UTC)

// sampleAgents has one agent of each status, so the golden file shows every field in use, and the
// built-in agent, which is the one kind that is never missing and has no version to report.
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
		{
			Kind: protocol.AgentKindBuiltin, Name: "Built-in agent", Status: protocol.AgentStatusSupported,
			// The built-in agent's models are the models of the providers that are set up, so a
			// sample shows the ones a person with a key for each would have.
			Models: []protocol.AgentModel{
				{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5", Thinking: true},
				{ID: "gpt-5-mini", Name: "GPT-5 mini", Thinking: true},
				{ID: "deepseek-chat", Name: "DeepSeek Chat", Thinking: false},
				{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Thinking: true},
				{ID: "qwen2.5-coder:32b", Name: "Qwen2.5 Coder 32B", Thinking: false},
			},
			Capabilities: protocol.AgentCapabilities{
				Resume: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, Approvals: true,
			},
		},
	}
}

func TestAgentCatalogGolden(t *testing.T) {
	testutil.Golden(t, "agents", protocol.NewAgentCatalog(sampleAgents(), agentsNow).WithTools([]protocol.AgentTool{
		{ID: "qwen", Name: "Qwen Code", Version: "0.15.6", Interface: protocol.AgentToolInterfaceACP,
			Note: "It speaks the Agent Client Protocol. Marshal has no adapter switched on for it yet."},
	}))
}

func TestAgentCatalogNeverEncodesAListAsNull(t *testing.T) {
	empty, err := json.Marshal(protocol.NewAgentCatalog(nil, agentsNow))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"agents":[],"tools":[],"serverTime":"2026-09-25T10:20:00.000Z"}`
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
