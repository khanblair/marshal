package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ForwardTools makes one MCP server's tools reachable through another. It lists every tool the
// remote session serves and registers a handler for each on the local server, so a call the local
// side receives is passed on unchanged and the remote's answer - including a refusal, which comes
// back as a tool error rather than a broken connection - is returned as it is.
//
// # Where this is used
//
// The daemon serves a card's tools itself, in the process that holds the card's note, its claims, and
// the other agents (Host, above). A CLI agent cannot reach that process directly: the agent is told,
// over ACP, to run a stdio command. `marshald mcp` is that command, and this is what it does - it
// speaks MCP over stdio to the agent and the same protocol over HTTP to the daemon, mirroring the
// tools across (cmd/marshald/mcp.go).
//
// The arguments are forwarded as they arrived and are unmarshalled and validated on the daemon side,
// so a call reaches the same handler a local agent's call would, with the same schemas.
func ForwardTools(ctx context.Context, remote *mcp.ClientSession, local *mcp.Server) error {
	cursor := ""
	for {
		list, err := remote.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return fmt.Errorf("read the tools to forward: %w", err)
		}
		for _, tool := range list.Tools {
			name := tool.Name
			local.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return remote.CallTool(ctx, &mcp.CallToolParams{
					Name: name, Arguments: req.Params.Arguments,
				})
			})
		}
		if list.NextCursor == "" {
			return nil
		}
		cursor = list.NextCursor
	}
}
