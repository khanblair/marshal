package catalog

import (
	"context"
	"slices"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// StubVersion is the version that the stub catalog reports for every agent it lists as present.
const StubVersion = "stub"

// Stub is the catalog of dev mode (MARSHAL_AGENT=stub) and of tests that run offline. It reports
// every agent as supported, at the version "stub", with the models that the prototype's pickers
// show and every capability on, so the pickers and the end-to-end tests work with no agent
// installed. The session manager starts the scripted stub agent behind each kind.
type Stub struct {
	now     func() time.Time
	missing []protocol.AgentKind
}

var _ Source = (*Stub)(nil)

// StubOption changes how a Stub answers.
type StubOption func(*Stub)

// StubMissing makes the given kinds appear as missing, for tests of the disabled picker row. Such
// an agent has an install hint and no version, like a real one that is not installed.
func StubMissing(kinds ...protocol.AgentKind) StubOption {
	return func(s *Stub) { s.missing = append(s.missing, kinds...) }
}

// StubClock sets the clock of the stub, so a test can fix the server time.
func StubClock(now func() time.Time) StubOption {
	return func(s *Stub) { s.now = now }
}

// NewStub returns a stub catalog.
func NewStub(opts ...StubOption) *Stub {
	s := &Stub{now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// List returns the stub catalog.
func (s *Stub) List(context.Context) (protocol.AgentCatalog, error) {
	agents := make([]protocol.Agent, 0, len(Kinds()))
	for _, d := range s.detected() {
		agents = append(agents, s.agentOf(d))
	}
	return protocol.NewAgentCatalog(agents, s.now()), nil
}

// Refresh returns the same as List: there is nothing to look at again.
func (s *Stub) Refresh(ctx context.Context) (protocol.AgentCatalog, error) {
	return s.List(ctx)
}

// Detect returns the stub agents. They have no place on disk, so Path is empty, and every one that
// is present can be started.
func (s *Stub) Detect(context.Context) ([]Detected, error) {
	return s.detected(), nil
}

// detected lists the stub agents in the order of Kinds.
func (s *Stub) detected() []Detected {
	out := make([]Detected, 0, len(Kinds()))
	for _, kind := range Kinds() {
		d := Detected{Kind: kind, Status: protocol.AgentStatusSupported, Version: StubVersion, Startable: true}
		if slices.Contains(s.missing, kind) {
			d = Detected{Kind: kind, Status: protocol.AgentStatusMissing}
		}
		out = append(out, d)
	}
	return out
}

// agentOf makes the wire entry of a stub agent.
func (s *Stub) agentOf(d Detected) protocol.Agent {
	sp := specFor(d.Kind)
	agent := protocol.Agent{
		Kind: d.Kind, Name: sp.name, Version: d.Version, Status: d.Status,
		Models: prototypeModels(d.Kind), Capabilities: everything(),
	}
	if d.Status == protocol.AgentStatusMissing {
		agent.InstallHint = sp.installHint
	}
	return agent
}

// everything is a set of capabilities with all of them on, as the prototype shows.
func everything() protocol.AgentCapabilities {
	return protocol.AgentCapabilities{
		Resume: true, StructuredEvents: true, ModelSwitching: true, Thinking: true, MCP: true, Approvals: true,
	}
}

// prototypeModels are the models that the prototype's pickers list for each agent (the AGENTS table
// in apps/web/src/mock/constants.ts), so that the screens look the same in dev mode. The scripted
// stub agent takes any model.
func prototypeModels(kind protocol.AgentKind) []protocol.AgentModel {
	switch kind {
	case protocol.AgentKindClaude:
		return []protocol.AgentModel{
			{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5", Thinking: true},
			{ID: "claude-opus-4-1", Name: "Claude Opus 4.1", Thinking: true},
			{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Thinking: true},
		}
	case protocol.AgentKindGemini, protocol.AgentKindCodex:
		return modelsFor(kind)
	}
	return []protocol.AgentModel{}
}
