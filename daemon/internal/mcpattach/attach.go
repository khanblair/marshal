// Package mcpattach gives a card's session the internal MCP server and the context it starts from,
// and takes them back when the session ends. It is the module docs/architecture.md section 11.4 says
// the daemon serves to its own agents, and it is the session manager's Attacher
// (internal/session/attach.go).
//
// # Why it is a package of its own
//
// The session manager names the Attacher as an interface so it never has to import this one, and the
// mcpserver package must not import internal/session, or the two would be an import cycle. This is
// the one place that knows both, so it is where they meet: it builds a server, hands it to the host
// that serves it, and answers the session the two things it was asked for.
//
// # The context order
//
// The context a session starts with is built in the order docs/architecture.md section 7 gives it -
// the role's instructions, the project's memory, the card's task, a short board-awareness summary,
// and the list of tools the agent has. See context.go. It is given to a fresh session only: an agent
// resuming a conversation is not told it a second time.
package mcpattach

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/mcpserver"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// serverName is what the agent knows the internal server as.
const serverName = "marshal"

// TokenEnv is the environment variable the daemon's `mcp` mode reads the card's secret from. The
// value is set on the server the agent is given, so the command it runs carries the secret without
// it appearing in the command line, where any process listing could read it.
const TokenEnv = "MARSHAL_MCP_TOKEN"

// Roles is what the context order needs from the roles module: the role a card runs under, for the
// instructions the role's agent is given.
type Roles interface {
	Role(ctx context.Context, name, projectID string) (protocol.Role, error)
}

// Deps is what an Attacher is built from. Host, Cards, Notes, Claims, Agents, Harness, Command, and
// Address are required; the rest may be left out.
type Deps struct {
	// Host is where a card's server is served while its session lives.
	Host *mcpserver.Host
	// Cards, Notes, Claims, and Agents are the services the server's tools call.
	Cards  mcpserver.Cards
	Notes  mcpserver.Notes
	Claims mcpserver.Claims
	Agents mcpserver.Agents
	// Panel is the card panel behind the checklist, comment, and attachment tools. It may be nil.
	Panel mcpserver.Panel
	// Codebase is the codebase map, nil until internal/codemap exists (slice 4). The tool that needs
	// it refuses until then.
	Codebase mcpserver.Codebase
	// Roles answers a card's role for the context order. It may be nil, and then a card's role
	// instructions are left out.
	Roles Roles
	// Harness reads a card's permission rules as they are now, by card id. It is the session
	// manager's HarnessConfigFor, so a tool call is decided exactly as the session's own requests
	// are.
	Harness func(cardID string) (harness.Config, bool)
	// Command is the daemon's own executable, and Address the loopback address it is listening on.
	// Together they are what the agent is told to run to reach this daemon's `mcp` mode. With either
	// empty no server is given and a session gets only its context.
	Command string
	Address string
	// Logger is where a failed attachment is noted. The default logs nothing.
	Logger *slog.Logger
	// Now is the clock the server stamps times with. The default is time.Now.
	Now func() time.Time
}

// Attacher is the session manager's Attacher (internal/session/attach.go), built from the daemon's
// services.
type Attacher struct {
	deps Deps
}

// New builds the attacher. Every required part must be there: a server built without one of them
// would fail on the first tool call rather than here. Command and Address are not required - a
// daemon that could not resolve its own executable still starts, and its sessions simply get the
// context and no server (Attach says so).
func New(deps Deps) (*Attacher, error) {
	switch {
	case deps.Host == nil:
		return nil, errors.New("mcpattach: a host is required")
	case deps.Cards == nil:
		return nil, errors.New("mcpattach: the cards are required")
	case deps.Notes == nil:
		return nil, errors.New("mcpattach: the notes are required")
	case deps.Claims == nil:
		return nil, errors.New("mcpattach: the claims are required")
	case deps.Agents == nil:
		return nil, errors.New("mcpattach: the agents are required")
	case deps.Harness == nil:
		return nil, errors.New("mcpattach: the permission rules are required")
	}
	return &Attacher{deps: deps}, nil
}

// Attach builds this card's server, hosts it, and answers the session that reaches it: the command
// the agent runs and the context the session starts from. A failure here is a session without its
// memory, and the manager logs it and starts the card anyway (internal/session/attach.go).
//
// A daemon that could not resolve its own executable, or has no address to be reached on, gives the
// session its context and no server rather than failing it: an agent that can read the board and
// its own card but cannot call the tools is better off than one that does not start.
func (a *Attacher) Attach(ctx context.Context, card protocol.Card) (session.Attachment, error) {
	if a.deps.Command == "" || a.deps.Address == "" {
		return session.Attachment{Instructions: a.context(ctx, card, nil)}, nil
	}
	server, err := mcpserver.New(a.serverDeps(card), mcpserver.Identity{
		CardID: card.ID, ProjectID: card.ProjectID, Role: card.Role,
	})
	if err != nil {
		return session.Attachment{}, err
	}
	secret, err := a.deps.Host.Add(card.ID, server)
	if err != nil {
		return session.Attachment{}, err
	}
	return session.Attachment{
		Servers:      []agents.MCPServer{a.command(card, secret)},
		Instructions: a.context(ctx, card, server.ToolNames()),
	}, nil
}

// Detach takes the card's server away when its session ends. It does nothing for a card that was
// never attached, which is what lets the manager call it for every ending.
func (a *Attacher) Detach(_ context.Context, cardID string) {
	a.deps.Host.Remove(cardID)
}

// serverDeps is what one card's server is built from.
func (a *Attacher) serverDeps(card protocol.Card) mcpserver.Deps {
	cardID := card.ID
	return mcpserver.Deps{
		Cards: a.deps.Cards, Notes: a.deps.Notes, Claims: a.deps.Claims,
		Agents: a.deps.Agents, Panel: a.deps.Panel, Codebase: a.deps.Codebase,
		Harness: func() (harness.Config, bool) { return a.deps.Harness(cardID) },
		Now:     a.deps.Now, Logger: a.deps.Logger,
	}
}

// command is the server the agent is given: the daemon's own program in its `mcp` mode, told which
// card it speaks for and where the daemon is, with the card's secret in its environment.
func (a *Attacher) command(card protocol.Card, secret string) agents.MCPServer {
	return agents.MCPServer{
		Name:    serverName,
		Command: a.deps.Command,
		Args:    []string{"mcp", "--address", a.deps.Address, "--card", card.ID},
		Env:     []string{TokenEnv + "=" + secret},
	}
}
