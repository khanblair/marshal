package protocol

import "time"

// AgentModel is one model that an agent can run. The pickers list it, and ID is the value that
// goes back to the daemon when a card is created.
type AgentModel struct {
	// ID is the value the agent takes as its model setting, such as "sonnet" or "gemini-2.5-pro".
	ID string `json:"id"`
	// Name is the words shown to people.
	Name string `json:"name"`
	// Thinking says whether the model has a thinking setting at all. The thinking picker is only
	// offered when it is true and the agent's own Thinking capability is true as well, because
	// an agent may have models that think and still give Marshal no way to set how hard.
	Thinking bool `json:"thinking"`
}

// AgentCapabilities says what Marshal can do with an agent, as far as it is known before a
// session starts. It describes the agent as Marshal drives it, so a feature the agent has but
// Marshal does not use yet is false.
type AgentCapabilities struct {
	// Resume is true when a stopped session can be picked up again with its session id.
	Resume bool `json:"resume"`
	// StructuredEvents is true when the agent reports messages and tool calls as events, so the
	// chat view works. An agent without it is only shown as a terminal.
	StructuredEvents bool `json:"structuredEvents"`
	// ModelSwitching is true when a card can choose the model when its session starts.
	ModelSwitching bool `json:"modelSwitching"`
	// Thinking is true when a card can choose a thinking mode when its session starts.
	Thinking bool `json:"thinking"`
	// MCP is true when the agent accepts MCP servers from Marshal.
	MCP bool `json:"mcp"`
	// Approvals is true when the agent can ask the person before it uses a tool. When it is
	// false, the "Ask" permission mode makes the agent skip what needs approval instead of
	// asking, and the screens should say so.
	Approvals bool `json:"approvals"`
}

// Agent is one entry of the agent catalog: a kind of coding agent, and whether it can be used on
// this machine. The place of the program on disk is not sent, because it names the person's
// folders.
type Agent struct {
	// Kind says which agent it is.
	Kind AgentKind `json:"kind"`
	// Name is the words shown to people, such as "Claude Code".
	Name string `json:"name"`
	// Version is the installed version, or empty when the agent is missing.
	Version string `json:"version"`
	// Status says whether the agent can be used here.
	Status AgentStatus `json:"status"`
	// Warning is one plain sentence for the screens when there is something to know, such as an
	// untested version. It is empty when there is nothing to say.
	Warning string `json:"warning"`
	// InstallHint is one plain sentence with the install command, for the disabled row of a
	// missing agent. It is empty when the agent is installed.
	InstallHint string `json:"installHint"`
	// Models are the models the agent can run, in the order to show them. The first one is the
	// default when a card switches to this agent.
	Models []AgentModel `json:"models"`
	// Capabilities says what Marshal can do with the agent.
	Capabilities AgentCapabilities `json:"capabilities"`
}

// AgentCatalog is the answer to GET /v1/agents.
type AgentCatalog struct {
	// Agents has one entry for each kind of agent that a card can use, whether or not it is
	// installed. The built-in agent is not listed here.
	Agents []Agent `json:"agents"`
	// ServerTime is the daemon's time when the answer was made. The catalog itself may come from
	// a check that was made a few minutes earlier.
	ServerTime Timestamp `json:"serverTime"`
}

// NewAgentCatalog makes a catalog stamped with the daemon's time. It turns a nil list into an
// empty one, for the catalog and for each agent's models, so the JSON has [] and never null.
func NewAgentCatalog(agents []Agent, now time.Time) AgentCatalog {
	out := make([]Agent, len(agents))
	for i, agent := range agents {
		if agent.Models == nil {
			agent.Models = []AgentModel{}
		}
		out[i] = agent
	}
	return AgentCatalog{Agents: out, ServerTime: NewTimestamp(now)}
}
