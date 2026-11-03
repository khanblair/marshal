package codex

import (
	"encoding/json"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// MCP output is part of the transcript. Keep errors visible as well as successful
// text results, rather than showing an anonymous failed tool with no explanation.
func mcpDetails(item map[string]any) (string, string) {
	server, tool := str(item["server"]), str(item["tool"])
	title := server + "." + tool
	if server == "" && tool == "" {
		title = "MCP tool"
	}
	if failure, ok := item["error"].(map[string]any); ok {
		if message := str(failure["message"]); message != "" {
			return title, message
		}
	}
	result, ok := item["result"].(map[string]any)
	if !ok {
		return title, ""
	}
	blocks, _ := result["content"].([]any)
	var text []string
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if ok && str(block["type"]) == "text" {
			text = append(text, str(block["text"]))
		}
	}
	if len(text) == 0 && result["structuredContent"] != nil {
		data, _ := json.Marshal(result["structuredContent"])
		text = append(text, string(data))
	}
	joined, _ := agents.Truncate(strings.Join(text, "\n"), agents.MaxContentBytes)
	return title, joined
}
