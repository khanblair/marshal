// Package mcpserver serves Marshal's internal MCP server to one card's agent: every tool in
// docs/architecture.md section 11.4, over stdio for a CLI agent and in process for the built-in one
// (docs/backend-checklist.md B7.1, build-plan task 7.1).
//
// # One server per agent session
//
// A server is built for one card's session and its handlers close over that session's identity, so a
// tool call can never be made "as" another card. The SDK has nowhere to hang per-session state
// (v1.6.0's ServerSessionState holds the initialize handshake and the log level and nothing a caller
// may add), so binding the identity at construction is the shape the library leaves; a second agent
// gets a second server over its own transport.
//
// # The daemon's services, and not another copy of them
//
// Every tool is written against the same service interfaces the routes use (Cards, Notes, Claims,
// Agents, Codebase), so a tool call does what the equivalent route does - including the events the
// service publishes itself. This package must not import internal/session: session imports this one
// to hand a server to a starting agent, and the two directions would be an import cycle. `Agents` is
// therefore the single method of the session manager this package needs.
//
// # Every call is permission-checked
//
// Before a tool does anything it puts the call through harness.Config.Decide, the same gate
// docs/architecture.md section 13 describes ("permissions are checked by the harness before every
// tool call"). A tool says whether it only reads Marshal or changes it; a call the mode does not
// allow comes back as a refusal that says why, in the words the model reads (see gate.go). Marshal's
// own record is what these tools change, so the harness's file and command rules have nothing to say
// about them and the mode is what decides - which is the honest reading of §13's "every tool call".
//
// The rules are read for each call and not once when the server is built, because the session reads
// them that way for its own permission requests (internal/session's harnessConfig): a person who
// turns a card's mode up while an agent is working changes what that agent's next tool call may do.
//
// # The tools whose domains do not exist yet
//
// Five tools name domains that arrive in Phase 10 (§19.1's checklists, §19.2's comments and
// attachments): list_checklists, tick_checklist_item, read_comments, post_comment, and
// read_attachment. They are registered anyway, and each one answers with a sentence saying that part
// of Marshal is not built yet - because an agent that calls one and is left guessing, or is told
// nothing, is worse off than one that is told plainly (see pending.go). Phase 10 fills each in by
// handing its service to Deps.
package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/codemap"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// serverName is what the server calls itself in the MCP handshake.
	serverName = "marshal"
	// serverVersion is the version the handshake carries. It tracks the daemon's own version string
	// rather than a separate number, so there is one version to keep true.
	serverVersion = "0.1.0"
	// boardCardLimit caps how many cards board_status answers with. The awareness summary exists to
	// tell an agent who else is working and on what, not to hand it the board: past this many cards
	// the answer is cut and says so, which is what keeps the call inside a token budget
	// (build-plan task 7.2).
	boardCardLimit = 40
)

// Identity is the agent session a server is served to. Every tool reads the card from here and never
// from the call, so an agent cannot act as another card.
type Identity struct {
	// CardID is the card whose agent the server is for.
	CardID string
	// ProjectID is that card's project.
	ProjectID string
	// Role is the role's name, for the tool list the context order names. It may be empty.
	Role string
}

// Cards is what the tools need from the projects service. It is exactly the part of that service
// these tools call, so the projects service satisfies it as it stands and no adapter is needed.
type Cards interface {
	Card(ctx context.Context, id string) (protocol.Card, error)
	CardByKey(ctx context.Context, key protocol.CardKey) (protocol.Card, error)
	Cards(ctx context.Context, projectID string) ([]protocol.Card, error)
	CreateCard(ctx context.Context, projectID string, in protocol.CreateCardRequest, opts ...projects.CardOption) (protocol.Card, error)
	UpdateCard(ctx context.Context, id string, in protocol.UpdateCardRequest) (protocol.Card, error)
	// ProjectDependencies answers every edge of a project's plan, grouped by the card that waits and
	// named by the keys of the cards it waits for (internal/projects/dependencies.go, migration 0020,
	// build-plan task 7.4). It is one read for the whole board.
	ProjectDependencies(ctx context.Context, projectID string) (map[string][]protocol.CardKey, error)
}

// Notes is what the tools need from the memory module's notes and lessons.
type Notes interface {
	Note(ctx context.Context, cardID string) (protocol.Note, error)
	SaveNote(ctx context.Context, cardID, body string, author protocol.NoteAuthor) (protocol.Note, error)
	SearchNotes(ctx context.Context, projectID, query string, limit int) ([]protocol.Note, error)
	// SearchLessons is search_memory's other half (task 7.6): what search_notes is to a card's
	// notes, this is to a project's lessons.
	SearchLessons(ctx context.Context, projectID, query string, limit int) ([]protocol.Lesson, error)
}

// Claims is what the tools need from the memory module's file claims.
type Claims interface {
	Claim(ctx context.Context, cardID string, paths []string) (memory.ClaimResult, error)
	Release(ctx context.Context, cardID string, paths []string) error
	Claims(ctx context.Context, cardID string) ([]memory.Claim, error)
	ProjectClaims(ctx context.Context, projectID string) ([]memory.Claim, error)
}

// Panel is what the checklist, comment, and attachment tools need from the card panel
// (internal/cardpanel), which this package never imports. Nil leaves those tools answering that
// the card's panel is not available.
type Panel interface {
	Checklists(ctx context.Context, cardID string) (protocol.ChecklistList, error)
	AgentTick(ctx context.Context, cardID, itemID string, done bool, evidence string) (protocol.ChecklistList, error)
	UnreadComments(ctx context.Context, cardID string) ([]protocol.Comment, error)
	PostAsAgent(ctx context.Context, cardID, body string) (protocol.Comment, error)
	ReadAttachment(ctx context.Context, cardID, name string) (string, error)
}

// Agents is what ask_agent needs: the session manager's one way to put a question to another card's
// agent. It is deliberately one method, so this package never imports internal/session.
type Agents interface {
	Send(ctx context.Context, cardID, text string) error
}

// Codebase is the codebase map (internal/codemap). A match is one place a name is written.
type Codebase interface {
	Search(ctx context.Context, projectID, query string, limit int) ([]CodeMatch, error)
	// Notice is what to tell the agent when the map cannot read symbols on this machine - universal
	// ctags not being installed - and is answering from file names alone. Empty when the map reads
	// symbols.
	Notice() string
}

// CodeMatch is one place the codebase map found a name. It is the map's own Match: the map's Search
// answers these as they stand, so no adapter stands between the two.
type CodeMatch = codemap.Match

// Deps are what a server is built from. Cards, Notes, Claims, and Agents are required; the rest may
// be left out, and the tool that needs a missing one refuses with a sentence naming it.
type Deps struct {
	// Cards resolves cards and their project, and creates and updates them.
	Cards Cards
	// Notes reads and writes the card's note.
	Notes Notes
	// Claims reads and writes the files a card holds.
	Claims Claims
	// Agents puts a question to another card's agent.
	Agents Agents
	// Panel is the card's checklists, comments, and attachments. It may be nil.
	Panel Panel
	// Codebase is the codebase map. The daemon gives every session the real map
	// (build-plan task 7.9); a server built without one refuses search_codebase with a sentence
	// saying so.
	Codebase Codebase
	// Harness reads the card's permission rules as they are now: the session's own harnessConfig,
	// which is the card's mode, Marshal's profile and blocklist, and the card's worktree rule. It is
	// read for every call rather than kept, so turning a card's mode up changes what its agent's next
	// call may do. Build it the way internal/session does - in particular the profile must be
	// security.DefaultProfile() or the shipped profile's own refusals apply.
	//
	// It reports false when there is no mode to decide with, which is a session whose permission row
	// could not be read; a call then needs the card's owner, exactly as the session's own permission
	// requests do.
	Harness func() (harness.Config, bool)
	// Now is the clock. The default is time.Now.
	Now func() time.Time
	// Logger is where a refused or failed call is noted. The default logs nothing.
	Logger *slog.Logger
}

// Server is one card's MCP server.
type Server struct {
	deps     Deps
	identity Identity
	now      func() time.Time
	logger   *slog.Logger
	impl     *mcp.Server
	names    []string
}

// New builds the server for one agent session and registers every tool.
func New(deps Deps, identity Identity, opts ...Option) (*Server, error) {
	switch {
	case deps.Cards == nil:
		return nil, errors.New("mcpserver: cards are required")
	case deps.Notes == nil:
		return nil, errors.New("mcpserver: the notes are required")
	case deps.Claims == nil:
		return nil, errors.New("mcpserver: the claims are required")
	case deps.Agents == nil:
		return nil, errors.New("mcpserver: the agents are required")
	case deps.Harness == nil:
		return nil, errors.New("mcpserver: the permission rules are required")
	case identity.CardID == "":
		return nil, errors.New("mcpserver: a card id is required")
	case identity.ProjectID == "":
		return nil, errors.New("mcpserver: a project id is required")
	}
	s := &Server{deps: deps, identity: identity, now: time.Now, logger: deps.Logger}
	for _, opt := range opts {
		opt(s)
	}
	s.impl = mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		&mcp.ServerOptions{
			Instructions: s.serverInstructions(),
			Logger:       s.logger,
		},
	)
	if err := s.register(); err != nil {
		return nil, err
	}
	return s, nil
}

// Option changes how New builds a server.
type Option func(*Server)

// WithClock sets the clock the tools stamp times with. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Server) {
		if now != nil {
			s.now = now
		}
	}
}

// serverInstructions is what the server tells a client about itself at the handshake. It is one
// short paragraph: the tools carry their own descriptions, and this is the line that says what the
// set of them is for.
func (s *Server) serverInstructions() string {
	return "Marshal's tools for this card. They read and write the card's own record - its notes, " +
		"the files it claims, its progress line, its checklists and comments - and the board and " +
		"memory around it. Every call is checked against the card's permission mode."
}

// CardID is the card the server is served to.
func (s *Server) CardID() string { return s.identity.CardID }

// ToolNames lists the tools in the order they were registered, which is the order the context order
// of docs/architecture.md section 7 names them in.
func (s *Server) ToolNames() []string {
	out := make([]string, len(s.names))
	copy(out, s.names)
	return out
}

// ToolCount is how many tools the server serves.
func (s *Server) ToolCount() int { return len(s.names) }

// Run serves the server over one transport until the context is cancelled or the client goes away.
// It is what a stdio subprocess calls with mcp.StdioTransport.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	return s.impl.Run(ctx, transport)
}

// Connect attaches the server to one transport and answers the session, for a caller that wants to
// keep several agent sessions on one transport or to drive the server from a test.
func (s *Server) Connect(ctx context.Context, transport mcp.Transport) (*mcp.ServerSession, error) {
	return s.impl.Connect(ctx, transport, nil)
}

// add registers one tool and remembers its name.
func add[In, Out any](s *Server, name, description string, handler mcp.ToolHandlerFor[In, Out]) {
	s.names = append(s.names, name)
	mcp.AddTool(s.impl, &mcp.Tool{Name: name, Description: description}, handler)
}
