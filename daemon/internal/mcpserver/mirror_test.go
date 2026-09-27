package mcpserver

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ForwardTools is what puts a card's tools in front of a CLI agent: `marshald mcp` speaks MCP to the
// agent, speaks the same protocol to the daemon that holds the card, and copies the tool list across
// (cmd/marshald/mcp.go). These tests drive the whole pair over real in-memory transports - a client
// talking to the daemon's server, a second server carrying the forwarded tools, and a client talking
// to that one - so a tool the copy drops, or an answer that stops on the way, fails here and not in
// an agent's session.

// mirrored builds the two ends a forwarding command builds, and answers the client talking to the
// daemon (the upstream) and the client talking to the copy (the agent).
func mirrored(t *testing.T, f *fixture) (upstream, agent *mcp.ClientSession) {
	t.Helper()
	ctx := t.Context()
	upstream = f.client(t)

	// The copy carries the daemon's instructions, exactly as `forward` does, so a test can check the
	// handshake the agent reads is about Marshal's tools rather than about a forwarder.
	instructions := ""
	if init := upstream.InitializeResult(); init != nil {
		instructions = init.Instructions
	}
	local := mcp.NewServer(
		&mcp.Implementation{Name: "marshal", Version: "test"},
		&mcp.ServerOptions{Instructions: instructions},
	)
	if err := ForwardTools(ctx, upstream, local); err != nil {
		t.Fatalf("forward the daemon's tools: %v", err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	session, err := local.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect the copy: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil)
	agent, err = client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect the agent: %v", err)
	}
	t.Cleanup(func() { _ = agent.Close() })
	return upstream, agent
}

// TestForwardToolsPutsEveryToolInFrontOfTheAgent: the agent sees the daemon's whole tool list, in the
// same order and with the same descriptions, and reads the same instructions on the handshake. A
// tool missing from the copy is a tool no CLI agent can ever call.
func TestForwardToolsPutsEveryToolInFrontOfTheAgent(t *testing.T) {
	f := newFixture(t)
	upstream, agent := mirrored(t, f)

	remote, err := upstream.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list the daemon's tools: %v", err)
	}
	local, err := agent.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list the forwarded tools: %v", err)
	}
	if len(local.Tools) != len(remote.Tools) {
		t.Fatalf("the agent sees %d tools, want the %d the daemon serves", len(local.Tools), len(remote.Tools))
	}
	for i, tool := range local.Tools {
		if tool.Name != remote.Tools[i].Name {
			t.Errorf("tool %d is %q, want %q", i, tool.Name, remote.Tools[i].Name)
		}
		if tool.Description != remote.Tools[i].Description {
			t.Errorf("%s reached the agent with a different description", tool.Name)
		}
	}

	if got, want := agent.InitializeResult().Instructions, upstream.InitializeResult().Instructions; got != want {
		t.Errorf("the agent read instructions\n%q\nwant the daemon's\n%q", got, want)
	}
}

// TestForwardToolsPassesACallThroughUnchanged: a call the agent makes reaches the daemon's own
// handler with the arguments as they arrived, so the answer is the one a local agent gets, and the
// card's state really changed.
func TestForwardToolsPassesACallThroughUnchanged(t *testing.T) {
	f := newFixture(t)
	upstream, agent := mirrored(t, f)
	args := map[string]any{"paths": []any{"src/route.ts", "src/route_test.ts"}}

	direct, refused := call(t, upstream, "claim_files", args)
	if refused {
		t.Fatalf("the daemon refused the claim: %s", direct)
	}
	forwarded, refused := call(t, agent, "claim_files", args)
	if refused {
		t.Fatalf("the forwarded claim was refused: %s", forwarded)
	}
	if forwarded != direct {
		t.Errorf("the forwarded call answered\n%q\nwant the daemon's own answer\n%q", forwarded, direct)
	}

	// And the call did the work rather than only answering: the paths are held, through the module
	// and not a copy of its output.
	var held []string
	for _, claim := range f.claims.held["card-1"] {
		held = append(held, claim.PathOrPackage)
	}
	for _, path := range []string{"src/route.ts", "src/route_test.ts"} {
		if !slicesContains(held, path) {
			t.Errorf("after the forwarded call the card holds %q, which is missing %q", held, path)
		}
	}
}

// TestForwardToolsHandsBackARefusal: a refused call comes back to the agent explained, the same
// sentence a local agent reads, and not as a broken connection. A forwarder that turned a refusal
// into an error would leave the model with nothing to act on (docs/architecture.md section 11.4).
func TestForwardToolsHandsBackARefusal(t *testing.T) {
	f := newFixture(t)
	upstream, agent := mirrored(t, f)

	direct, directRefused := call(t, upstream, "list_checklists", map[string]any{})
	if !directRefused {
		t.Fatalf("the daemon answered list_checklists instead of refusing: %s", direct)
	}
	forwarded, refused := call(t, agent, "list_checklists", map[string]any{})
	if !refused {
		t.Fatalf("the forwarded refusal arrived as a success: %s", forwarded)
	}
	if forwarded != direct {
		t.Errorf("the agent was refused with\n%q\nwant\n%q", forwarded, direct)
	}
	if !strings.Contains(forwarded, "checklists") {
		t.Errorf("the refusal %q does not say what it refuses", forwarded)
	}

	// The connection is still good after a refusal: the next call is answered.
	if text, isError := call(t, agent, "board_status", map[string]any{}); isError {
		t.Errorf("board_status was refused after a refusal went through: %s", text)
	}
}

// slicesContains is strings.Contains for a small list of paths, kept here rather than pulling slices
// in for one use.
func slicesContains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
