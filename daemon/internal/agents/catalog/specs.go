package catalog

import "github.com/khanblair/marshal/daemon/internal/protocol"

// BuiltinName is the built-in agent as the screens name it. It is here, next to the other names,
// so the daemon and the design's fixtures cannot drift apart.
const BuiltinName = "Built-in agent"

// spec is what Marshal knows about one kind of agent before it looks at the machine.
type spec struct {
	kind protocol.AgentKind
	// name is the words shown to people.
	name string
	// program is the name of the command that starts the agent. It is empty for the built-in
	// agent, which is part of Marshal and has no program to look for.
	program string
	// installHint is the plain sentence for the disabled row of a missing agent.
	installHint string
	// startable is false for an agent that Marshal can find but cannot start sessions with yet.
	startable bool
	// builtIn is true for the agent Marshal runs itself. There is nothing to look for on the
	// machine and nothing to test against a version, so it is always present and always
	// supported, and it is the one kind detection does not probe.
	builtIn bool
	// capabilities is what Marshal can do with the agent through its adapter.
	capabilities protocol.AgentCapabilities
}

// Kinds lists the kinds of agent that the catalog reports, in the order the pickers show them. The
// built-in agent is last: the design lists the agents Marshal found first, and Marshal's own agent
// after them.
func Kinds() []protocol.AgentKind {
	return []protocol.AgentKind{
		protocol.AgentKindClaude, protocol.AgentKindGemini, protocol.AgentKindCodex, protocol.AgentKindBuiltin,
	}
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
	case protocol.AgentKindBuiltin:
		return builtinSpec()
	}
	return spec{kind: kind}
}

// builtinSpec is Marshal's own agent (agents/builtin). It is always there, so it is never missing
// and needs no install hint. Its capabilities are the adapter's own: it resumes by replaying the
// stored conversation, it reports messages and tool calls as events, a card chooses its model and
// its thinking mode, and it asks the person before a tool that the permission rules send to them.
// It does not take Marshal's MCP servers yet (agents.Capabilities.MCP is false), so MCP is false.
func builtinSpec() spec {
	return spec{
		kind: protocol.AgentKindBuiltin, name: BuiltinName, startable: true, builtIn: true,
		capabilities: protocol.AgentCapabilities{
			Resume: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, Approvals: true,
		},
	}
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
