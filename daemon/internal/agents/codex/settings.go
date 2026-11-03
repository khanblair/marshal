package codex

import (
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func settings(s agents.StartSpec) (map[string]any, map[string]any, error) {
	approval, sandbox := "untrusted", "read-only"
	switch s.PermissionMode {
	case "", "ask":
	case "plan":
		approval = "never"
	case "auto-edits":
		sandbox = "workspace-write"
	case "full-auto":
		sandbox = "workspace-write"
		approval = "never"
	case "bypass":
		sandbox = "danger-full-access"
		approval = "never"
	case "default":
	default:
		return nil, nil, fmt.Errorf("%w: permission mode %q", agents.ErrUnsupportedSetting, s.PermissionMode)
	}
	thread := map[string]any{"approvalPolicy": approval, "sandbox": sandbox, "approvalsReviewer": "user"}
	config := map[string]any{"mcp_servers": mcp(s.MCPServers)}
	if s.Thinking != "" {
		switch s.Thinking {
		case "low", "medium", "high", "xhigh":
			config["model_reasoning_effort"] = s.Thinking
		default:
			return nil, nil, fmt.Errorf("%w: thinking %q", agents.ErrUnsupportedSetting, s.Thinking)
		}
	}
	return thread, config, nil
}
func mcp(in []agents.MCPServer) map[string]any {
	out := map[string]any{}
	for _, v := range in {
		env := map[string]string{}
		for _, e := range v.Env {
			for i := 0; i < len(e); i++ {
				if e[i] == '=' {
					env[e[:i]] = e[i+1:]
					break
				}
			}
		}
		server := map[string]any{"command": v.Command, "args": v.Args, "env": env}
		if v.Name == "marshal" {
			// Marshal's injected server enforces the session's tool permissions itself.
			// Allow its transport, as the Claude adapter does with mcp__marshal;
			// native file/command approvals and other MCP servers keep their policy.
			server["default_tools_approval_mode"] = "approve"
		}
		out[v.Name] = server
	}
	return out
}
