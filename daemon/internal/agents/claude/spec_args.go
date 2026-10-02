package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// toolRuleArgs turns a spec's tool rules into Claude Code's own flags. Claude Code documents both
// flags as taking each rule as its own argument ("--allowedTools" "Bash(git log *)" "Read"), so a
// rule that holds spaces stays whole, and the next "--" flag ends the list. A disallowed rule wins
// over an allowed one. Both lists are empty for most sessions, and then no flag is added.
//
// Every MCP server the spec gives the session is allowed as a whole ("mcp__marshal"). Marshal built
// that server itself and decides each call it gets (internal/mcpserver), and Claude Code runs with
// its permission prompts off, so a tool of it that is not allowed here is refused without asking.
func toolRuleArgs(spec agents.StartSpec) []string {
	allowed := slices.Clone(spec.AllowedTools)
	for _, server := range spec.MCPServers {
		if rule := "mcp__" + server.Name; !slices.Contains(allowed, rule) {
			allowed = append(allowed, rule)
		}
	}
	var args []string
	if len(allowed) > 0 {
		args = append(append(args, "--allowedTools"), allowed...)
	}
	if len(spec.DisallowedTools) > 0 {
		args = append(append(args, "--disallowedTools"), spec.DisallowedTools...)
	}
	return args
}

// mcpConfig is the file Claude Code's --mcp-config reads: the servers it starts for the session.
type mcpConfig struct {
	Servers map[string]mcpConfigServer `json:"mcpServers"`
}

type mcpConfigServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// writeMCPConfig writes the session's MCP servers to a file only its owner can read, because a
// server's environment carries a secret that a command line would show to every process on the
// machine. It answers the file's path, which the caller removes when the session ends. A spec with
// no servers writes nothing and answers an empty path.
func writeMCPConfig(servers []agents.MCPServer) (string, error) {
	if len(servers) == 0 {
		return "", nil
	}
	cfg := mcpConfig{Servers: make(map[string]mcpConfigServer, len(servers))}
	for _, server := range servers {
		env := make(map[string]string, len(server.Env))
		for _, pair := range server.Env {
			if name, value, ok := strings.Cut(pair, "="); ok {
				env[name] = value
			}
		}
		cfg.Servers[server.Name] = mcpConfigServer{Command: server.Command, Args: server.Args, Env: env}
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encode the MCP servers: %w", err)
	}
	// CreateTemp makes the file readable by its owner alone.
	file, err := os.CreateTemp("", "marshal-mcp-*.json")
	if err != nil {
		return "", fmt.Errorf("make the MCP config file: %w", err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("write the MCP config file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("close the MCP config file: %w", err)
	}
	return file.Name(), nil
}

// removeUnless removes an MCP config file when a start did not go on to keep it.
func removeUnless(path string, keep *bool) {
	if path != "" && !*keep {
		_ = os.Remove(path)
	}
}

// removeWhenDone removes an MCP config file once its session has finished. The file has to outlive
// the process that first reads it, because an interrupted turn starts the process again with the
// same arguments.
func removeWhenDone(path string, done <-chan struct{}) {
	if path == "" {
		return
	}
	<-done
	_ = os.Remove(path)
}
