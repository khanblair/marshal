package codex

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestMCPTransportApprovalIsLimitedToMarshal(t *testing.T) {
	servers := mcp([]agents.MCPServer{
		{Name: "marshal", Command: "marshald", Env: []string{"MARSHAL_TOKEN=fixture"}},
		{Name: "external", Command: "other-server"},
	})
	internal := servers["marshal"].(map[string]any)
	if internal["default_tools_approval_mode"] != "approve" {
		t.Fatal("Marshal's permission-checked server must be reachable without another consent layer")
	}
	if internal["env"].(map[string]string)["MARSHAL_TOKEN"] != "fixture" {
		t.Fatal("MCP credentials must remain in the server's RPC configuration")
	}
	if _, ok := servers["external"].(map[string]any)["default_tools_approval_mode"]; ok {
		t.Fatal("external MCP servers must retain their native consent policy")
	}
}

func TestMCPDetailsPreservesFailureAndSuccessfulContent(t *testing.T) {
	title, content := mcpDetails(map[string]any{
		"server": "marshal", "tool": "board_status",
		"error": map[string]any{"message": "user rejected MCP tool call"},
	})
	if title != "marshal.board_status" || content != "user rejected MCP tool call" {
		t.Fatalf("failure detail lost: %q %q", title, content)
	}
	_, content = mcpDetails(map[string]any{
		"server": "marshal", "tool": "board_status",
		"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "board ready"}}},
	})
	if content != "board ready" {
		t.Fatalf("successful MCP result lost: %q", content)
	}
}
