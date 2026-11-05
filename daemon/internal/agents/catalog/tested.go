package catalog

import "github.com/khanblair/marshal/daemon/internal/protocol"

// testedVersions lists, for each kind of agent, the exact versions that Marshal's tests have run
// against. An installed agent at one of these versions is "supported"; any other version is
// "untested", which works with a warning.
//
// CI updates this list: when the agent compatibility job passes on a new version, it adds the
// version here, and it drops one that is no longer checked. Do not add a version by hand unless a
// person has run the adapter's tests against it on a real machine.
//
// The 2026-11-03 real lifecycle check drove Claude and Codex through the built
// daemon, including MCP, approval/denial, file effects, and stop/resume.
func testedVersions(kind protocol.AgentKind) []string {
	switch kind {
	case protocol.AgentKindClaude:
		return []string{"2.1.282", "2.1.299"}
	case protocol.AgentKindGemini:
		return []string{"0.35.1"}
	case protocol.AgentKindCodex:
		return []string{"0.166.0-alpha.5.1", "0.166.0-alpha.6"}
	}
	return nil
}
