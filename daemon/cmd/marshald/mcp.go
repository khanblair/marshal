package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/mcpattach"
	"github.com/khanblair/marshal/daemon/internal/mcpserver"
)

// This file is the daemon's own binary in its `mcp` mode: the stdio MCP server an agent runs to
// reach the daemon's tools (docs/architecture.md section 11.4, "over stdio for CLI agents").
//
// # Why a forwarder and not the server itself
//
// The tools act on the whole daemon - the board, the card's note and claims, and, for ask_agent,
// another card's live session. None of that is here in a process the agent started, so this command
// does not serve the tools itself: it speaks MCP over stdio to the agent and the same protocol over
// HTTP to the daemon that does (internal/mcpserver's Host). The agent sees an ordinary stdio server
// and never holds a URL or a daemon token.
//
// # What it is given
//
// The address and the card come in as arguments and the card's secret in the environment (its name
// is mcpattach.TokenEnv). The secret is a per-card value the daemon mints when the card's session
// starts and drops when it ends, not the owner's token: an agent that could read a file on this
// machine still cannot act as the owner through here.

// runMCP serves one card's tools to an agent over stdio until the agent goes away, and answers the
// process exit code.
func runMCP(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	address := fs.String("address", "", "the daemon's loopback address, as host:port")
	card := fs.String("card", "", "the card whose tools are served")
	if err := fs.Parse(args); err != nil {
		return exitBadInput
	}
	secret := strings.TrimSpace(os.Getenv(mcpattach.TokenEnv))
	switch {
	case *address == "":
		say(stderr, "mcp: --address is required")
		return exitBadInput
	case *card == "":
		say(stderr, "mcp: --card is required")
		return exitBadInput
	case secret == "":
		say(stderr, "mcp: the card's secret is missing from the environment (%s)", mcpattach.TokenEnv)
		return exitBadInput
	}
	// The agent owns this process: when it closes the pipe or kills us, the context ends and the
	// forwarder returns. There is no signal handling of our own to do - the process is short-lived
	// and holds nothing.
	if err := forward(context.Background(), forwardConfig{
		address: *address, card: *card, secret: secret,
	}, &mcp.StdioTransport{}); err != nil {
		say(stderr, "mcp: %v", err)
		return exitFailed
	}
	_ = stdout
	return exitOK
}

// forwardConfig is what the forwarder needs: where the daemon is, which card it speaks for, and the
// secret that reaches it.
type forwardConfig struct {
	address string
	card    string
	secret  string
}

// forward reaches the daemon, mirrors its tools onto the server the agent talks to, and serves them
// until the agent's end of the pipe closes.
func forward(ctx context.Context, cfg forwardConfig, transport mcp.Transport) error {
	client := mcp.NewClient(&mcp.Implementation{Name: "marshal", Version: buildinfo.Version}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             "http://" + cfg.address + mcpserver.PathPrefix + cfg.card,
		DisableStandaloneSSE: true,
		HTTPClient: &http.Client{Transport: mcpAuthTransport{
			secret: cfg.secret, base: http.DefaultTransport,
		}},
	}, nil)
	if err != nil {
		return fmt.Errorf("reach the daemon at %s: %w", cfg.address, err)
	}
	defer func() { _ = session.Close() }()

	// The server the agent sees carries the same instructions the daemon's own does, so the handshake
	// it reads is about Marshal's tools and not about a forwarder.
	instructions := ""
	if init := session.InitializeResult(); init != nil {
		instructions = init.Instructions
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "marshal", Version: buildinfo.Version},
		&mcp.ServerOptions{Instructions: instructions},
	)
	if err := mcpserver.ForwardTools(ctx, session, server); err != nil {
		return err
	}
	return server.Run(ctx, transport)
}

// mcpAuthTransport adds the card's secret to every request to the daemon. It stands in for a header
// the SDK client has no field for.
type mcpAuthTransport struct {
	secret string
	base   http.RoundTripper
}

func (t mcpAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.secret)
	return t.base.RoundTrip(clone)
}
