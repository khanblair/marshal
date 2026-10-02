package claude

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestEachToolRuleIsItsOwnArgumentAfterItsFlag(t *testing.T) {
	got := toolRuleArgs(agents.StartSpec{
		AllowedTools:    []string{"mcp__marshal", "Bash(git add:*)"},
		DisallowedTools: []string{"Bash(git push:*)"},
	})
	want := []string{"--allowedTools", "mcp__marshal", "Bash(git add:*)", "--disallowedTools", "Bash(git push:*)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("toolRuleArgs = %q, want %q", got, want)
	}
	if got := toolRuleArgs(agents.StartSpec{}); len(got) != 0 {
		t.Errorf("a spec with no rules gave %q, want no flags", got)
	}
}

func TestEveryMCPServerTheSessionIsGivenIsAllowedAsAWhole(t *testing.T) {
	servers := []agents.MCPServer{{Name: "marshal", Command: "/bin/marshald"}}
	got := toolRuleArgs(agents.StartSpec{MCPServers: servers, AllowedTools: []string{"Bash(git add:*)"}})
	if want := []string{"--allowedTools", "Bash(git add:*)", "mcp__marshal"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("toolRuleArgs = %q, want %q", got, want)
	}
	// A rule that is already there is not written twice.
	got = toolRuleArgs(agents.StartSpec{MCPServers: servers, AllowedTools: []string{"mcp__marshal"}})
	if want := []string{"--allowedTools", "mcp__marshal"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("toolRuleArgs = %q, want %q", got, want)
	}
}

func TestAStartPassesItsToolRulesOnTheCommandLine(t *testing.T) {
	a := fakeAdapter(t, "showargs")
	spec := startSpec(t)
	spec.AllowedTools = []string{"mcp__marshal"}
	spec.DisallowedTools = []string{"Bash(git push:*)", "Bash(git rebase:*)"}
	h := start(t, a, spec)
	text := send(t, a, h, "hi").text()
	for _, want := range []string{
		"|--allowedTools|mcp__marshal|", "|--disallowedTools|Bash(git push:*)|Bash(git rebase:*)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the command line %q lacks %s", text, want)
		}
	}
}

func TestMCPServersReachClaudeThroughAFileOnlyTheOwnerCanRead(t *testing.T) {
	a := fakeAdapter(t, "showargs")
	spec := startSpec(t)
	spec.MCPServers = []agents.MCPServer{{
		Name: "marshal", Command: "/bin/marshald", Args: []string{"mcp", "--card", "c1"},
		Env: []string{"MARSHAL_MCP_TOKEN=s3cret"},
	}}
	h := start(t, a, spec)
	text := send(t, a, h, "hi").text()

	_, after, found := strings.Cut(text, "|--mcp-config|")
	if !found {
		t.Fatalf("the command line %q has no --mcp-config", text)
	}
	path, _, _ := strings.Cut(after, "|")
	path = strings.Fields(path)[0]
	if strings.Contains(text, "s3cret") {
		t.Errorf("the secret is on the command line: %q", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the MCP config file is missing: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("the MCP config file has mode %v, want the owner alone", info.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the MCP config: %v", err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("the MCP config is not JSON: %v", err)
	}
	got := cfg.Servers["marshal"]
	if got.Command != "/bin/marshald" || strings.Join(got.Args, " ") != "mcp --card c1" || got.Env["MARSHAL_MCP_TOKEN"] != "s3cret" {
		t.Errorf("the marshal server = %+v", got)
	}

	stopAndDrain(t, a, h)
	deadline := time.Now().Add(eventTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("the MCP config file %s was not removed when the session ended", path)
}

func TestASpecWithNoServersWritesNoConfigFile(t *testing.T) {
	path, err := writeMCPConfig(nil)
	if err != nil || path != "" {
		t.Errorf("writeMCPConfig(nil) = %q, %v, want nothing", path, err)
	}
}
