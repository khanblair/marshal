package session

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Attacher gives a card's session the two things the daemon adds to it, and takes them back when
// the session ends:
//
//   - The internal MCP server (docs/architecture.md section 11.4), as the list of servers the
//     agent is given. It is the daemon serving tools to its own agents, so it needs the whole
//     daemon behind it - the projects service, the memory module, the session manager itself - and
//     none of that is built before the manager is.
//   - The context a session starts with, in the order architecture section 7 gives it: the role's
//     instructions, the project's memory, the card's task and its pinned files, a board-awareness
//     summary, and the list of tools the agent has.
//
// It is one seam and not two because both are built by the same module from the same card, and
// because they are given up at the same moment: a server that outlived its session would be a
// server no agent can reach, and a release that ran while its server was still being handed out
// would leave a live agent with a tool that answers nothing.
//
// The manager names only what it needs, the way it names RoleLimitsReader and HistoryRecorder, and
// the module that implements it (internal/mcpserver, wired in cmd/marshald) is the only one that
// knows how a server is built.
type Attacher interface {
	// Attach builds what this card's session is given, and answers it. It is called once, just
	// before a fresh session's agent is started.
	Attach(ctx context.Context, card protocol.Card) (Attachment, error)
	// Detach releases what Attach built for a card, when the card's session ends. It is called for
	// every session that ends, including one that crashed, and it does nothing for a card that was
	// never attached - which is what lets it be called without asking whether Attach ever ran.
	Detach(ctx context.Context, cardID string)
}

// Attachment is what a card's session is given beyond its own settings: the MCP servers its agent
// starts, and the context its first message carries.
type Attachment struct {
	// Servers are the MCP servers the agent is given. Empty for an agent that takes none.
	Servers []agents.MCPServer
	// Instructions is the context the session starts with (architecture section 7). It is empty
	// when there is nothing to say, and is never sent again when a session is resumed.
	Instructions string
}

// SetAttacher gives the manager the module that attaches the internal server and the starting
// context to a card's session. It is set after the manager is built, because that module is built
// after it, and it is safe to call while sessions are running: the next session to start reads it.
func (m *Manager) SetAttacher(a Attacher) {
	m.attachMu.Lock()
	m.attachModule = a
	m.attachMu.Unlock()
}

// attacher returns the module, or nil when none was set.
func (m *Manager) attacher() Attacher {
	m.attachMu.RLock()
	defer m.attachMu.RUnlock()
	return m.attachModule
}

// attach builds what a card's session is given, and remembers that the card's session must give it
// up. It answers an empty Attachment - never an error - when nothing is set up or when the module
// fails: a knowledge base that cannot be read is a session without it, not a card that cannot
// start. The failure is logged, because a card running without its memory should be visible.
//
// The card id is remembered rather than the attachment, because Detach runs at the other end of the
// session, on the pump's goroutine, and only the id is needed to give it up.
func (m *Manager) attach(ctx context.Context, card protocol.Card) Attachment {
	module := m.attacher()
	if module == nil {
		return Attachment{}
	}
	got, err := module.Attach(ctx, card)
	if err != nil {
		m.log.Error("could not attach the internal server and context to a card's session",
			"card_id", card.ID, "error", err)
		return Attachment{}
	}
	return got
}

// detach gives up what a card's session was given. It runs for every session that ends, and it is
// safe to call for a card that never attached anything.
func (m *Manager) detach(ls *liveSession) {
	if ls.isChat() {
		return
	}
	m.detachCard(ls.cardID)
}

// detachCard gives up what a card was given, by card id. It is what the two ends of a session use:
// the pump, when the session's channel closes, and a start that failed after the card was attached,
// where there is no live session to hand over.
func (m *Manager) detachCard(cardID string) {
	module := m.attacher()
	if module == nil {
		return
	}
	// The context is the daemon's own, not the one that ended the session: a release must happen
	// even when the reason for the ending was the daemon shutting down.
	module.Detach(context.WithoutCancel(m.ctx), cardID)
}
