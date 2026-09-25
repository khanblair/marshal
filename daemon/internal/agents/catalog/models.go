package catalog

import "github.com/khanblair/marshal/daemon/internal/protocol"

// modelsFor lists the models of an agent, in the order to show them. The first is the default when
// a card switches to the agent.
//
// The ids are what each agent takes as its model setting, so they can go to the agent as they are:
// Claude Code takes its aliases and full names, Gemini CLI its model names, and Codex its model
// names. Thinking says that the model has a thinking setting; whether Marshal can set it is the
// agent's own Thinking capability (see specs.go), and a picker needs both.
//
// A later change can refresh these from the agent itself: an ACP agent lists its models when a
// session starts, and that list could be merged over this table. Nothing does that yet, so this
// table is the one place to edit when a model appears or goes.
func modelsFor(kind protocol.AgentKind) []protocol.AgentModel {
	switch kind {
	case protocol.AgentKindClaude:
		return claudeModels()
	case protocol.AgentKindGemini:
		return []protocol.AgentModel{
			{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Thinking: true},
			{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Thinking: true},
		}
	case protocol.AgentKindCodex:
		return []protocol.AgentModel{
			{ID: "gpt-5-codex", Name: "GPT-5 Codex", Thinking: true},
			{ID: "gpt-5", Name: "GPT-5", Thinking: true},
			{ID: "gpt-5-mini", Name: "GPT-5 mini", Thinking: true},
		}
	}
	return []protocol.AgentModel{}
}

// claudeModels are Claude Code's aliases, which follow the newest model of each kind, and then the
// current full names, which stay on one model. Haiku has no effort setting, so it cannot think on
// demand; the other current models can.
func claudeModels() []protocol.AgentModel {
	return []protocol.AgentModel{
		{ID: "sonnet", Name: "Sonnet (latest)", Thinking: true},
		{ID: "opus", Name: "Opus (latest)", Thinking: true},
		{ID: "haiku", Name: "Haiku (latest)", Thinking: false},
		{ID: "fable", Name: "Fable (latest)", Thinking: true},
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Thinking: true},
		{ID: "claude-opus-5-5", Name: "Claude Opus 5.5", Thinking: true},
		{ID: "claude-fable-5-1", Name: "Claude Fable 5.1", Thinking: true},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Thinking: false},
	}
}
