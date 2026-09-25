package catalog

import "github.com/khanblair/marshal/daemon/internal/protocol"

// spec is what Marshal knows about one kind of agent before it looks at the machine.
type spec struct {
	kind protocol.AgentKind
	// name is the words shown to people.
	name string
	// program is the name of the command that starts the agent.
	program string
	// installHint is the plain sentence for the disabled row of a missing agent.
	installHint string
	// startable is false for an agent that Marshal can find but cannot start sessions with yet.
	startable bool
	// capabilities is what Marshal can do with the agent through its adapter.
	capabilities protocol.AgentCapabilities
}

// Kinds lists the kinds of agent that the catalog reports, in the order the pickers show them.
// The built-in agent is not here: it has no program to look for.
func Kinds() []protocol.AgentKind {
	return []protocol.AgentKind{protocol.AgentKindClaude, protocol.AgentKindGemini, protocol.AgentKindCodex}
}

// specFor returns what is known about a kind. A kind that the catalog does not report gets an
// empty spec, which nothing asks for because callers go through Kinds.
func specFor(kind protocol.AgentKind) spec {
	switch kind {
	case protocol.AgentKindClaude:
		return claudeSpec()
	case protocol.AgentKindGemini:
		return geminiSpec()
	case protocol.AgentKindCodex:
		return codexSpec()
	}
	return spec{kind: kind}
}

// claudeSpec is Claude Code. Its adapter (agents/claude) reads the CLI's streaming JSON mode, which
// has no way to ask the person about a tool yet, so Approvals is false. MCP is what the agent
// accepts; the adapter does not hand it Marshal's servers yet.
func claudeSpec() spec {
	return spec{
		kind: protocol.AgentKindClaude, name: "Claude Code", program: "claude", startable: true,
		installHint: "Claude Code is not installed. Install it with: curl -fsSL https://claude.ai/install.sh | bash",
		capabilities: protocol.AgentCapabilities{
			Resume: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, MCP: true,
		},
	}
}

// geminiSpec is Gemini CLI, driven through ACP. It has no setting for how hard a model thinks,
// so Thinking is false.
func geminiSpec() spec {
	return spec{
		kind: protocol.AgentKindGemini, name: "Gemini CLI", program: "gemini", startable: true,
		installHint: "Gemini CLI is not installed. Install it with: npm install -g @google/gemini-cli",
		capabilities: protocol.AgentCapabilities{
			Resume: true, StructuredEvents: true, ModelSwitching: true, MCP: true, Approvals: true,
		},
	}
}

// codexSpec is Codex. Marshal finds it and lists it, but has no adapter for it yet (see
// agents/codex), so it is not startable and every capability is false.
func codexSpec() spec {
	return spec{
		kind: protocol.AgentKindCodex, name: "Codex", program: "codex", startable: false,
		installHint: "Codex is not installed. Install it with: npm install -g @openai/codex",
	}
}
