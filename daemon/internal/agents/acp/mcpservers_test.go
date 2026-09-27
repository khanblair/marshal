package acp

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// The list a session is given is the protocol's own shape: a name, a program, its arguments, and
// its environment. A session that is given nothing is told so with an empty list rather than with a
// missing field, because the protocol asks for the field on every request.
func TestMCPServersCarriesWhatASessionIsGiven(t *testing.T) {
	got := mcpServers([]agents.MCPServer{{
		Name:    "marshal",
		Command: "/usr/local/bin/marshald",
		Args:    []string{"mcp"},
		Env:     []string{"MARSHAL_MCP_TOKEN=abc", "MARSHAL_MODE=dev", "MARSHAL_EMPTY="},
	}})
	if len(got) != 1 {
		t.Fatalf("%d servers were carried, want 1", len(got))
	}
	if got[0].Stdio == nil {
		t.Fatal("the server is not carried over stdio, which is the one transport every agent must support")
	}
	stdio := got[0].Stdio
	if stdio.Name != "marshal" || stdio.Command != "/usr/local/bin/marshald" {
		t.Errorf("the server is %+v", stdio)
	}
	if !slices.Equal(stdio.Args, []string{"mcp"}) {
		t.Errorf("the arguments are %q", stdio.Args)
	}
	if len(stdio.Env) != 3 {
		t.Fatalf("the environment is %+v, want three entries", stdio.Env)
	}
	for i, want := range []struct{ name, value string }{
		{"MARSHAL_MCP_TOKEN", "abc"}, {"MARSHAL_MODE", "dev"}, {"MARSHAL_EMPTY", ""},
	} {
		if stdio.Env[i].Name != want.name || stdio.Env[i].Value != want.value {
			t.Errorf("entry %d is %+v, want %s=%q", i, stdio.Env[i], want.name, want.value)
		}
	}
}

// A server with no arguments and no environment is still valid JSON for the protocol: the arrays
// encode as [] and never as null.
func TestMCPServersEncodesEmptyListsAsArrays(t *testing.T) {
	got := mcpServers([]agents.MCPServer{{Name: "marshal", Command: "/bin/true"}})
	raw, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("encode the server: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode the server: %v", err)
	}
	for _, field := range []string{"args", "env"} {
		if string(decoded[field]) != "[]" {
			t.Errorf("%s encodes as %s, want []", field, decoded[field])
		}
	}
	if _, ok := decoded["type"]; ok {
		t.Errorf("a stdio server carries a type field it should not: %s", raw)
	}
}

// A session that is given no servers is told so plainly.
func TestMCPServersIsEmptyAndNotNilForNothing(t *testing.T) {
	got := mcpServers(nil)
	if got == nil {
		t.Fatal("the list is nil, and the protocol asks for it to exist")
	}
	if len(got) != 0 {
		t.Errorf("an empty list carried %+v", got)
	}
}
