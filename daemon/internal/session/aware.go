package session

import (
	"context"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// The board-awareness summary a card's agent is given at the start of every turn: who else is
// working in its project, on what, and the files they hold (docs/marshal-product-scope.md section
// 11.3, docs/backend-checklist.md B7.2, build-plan task 7.2).
//
// # Why it is a seam of its own
//
// It is not a third method on Attacher, though the same module happens to implement both, for two
// reasons. It is needed when nothing is attached: a daemon that could not resolve its own
// executable, or has no loopback address, gives a session no server at all and still wants its
// agents told what the board looks like. And its lifetime is different - Attach builds once, just
// before a fresh session's agent starts, while this is read as every turn begins, because the board
// an agent is working against an hour into a card's life is not the board it started on.
//
// An empty summary is always an acceptable answer, and a module that is not set at all is the
// ordinary case in a daemon built without one: an agent that is told nothing about the board is
// better off than one whose message never arrives.
type Awareness interface {
	// TurnAwareness answers the summary for one card, or the empty string when there is nothing to
	// say. It is called as a turn begins, so it must stay cheap enough to run on every turn, and it
	// must not block on anything the session holds.
	TurnAwareness(ctx context.Context, cardID string) string
}

// SetAwareness gives the manager the module that builds a card's per-turn summary. It is set after
// the manager is built, the same way and for the same reason SetAttacher is: that module needs the
// services the manager is built from. It is safe to call while sessions are running - the next turn
// reads it.
func (m *Manager) SetAwareness(a Awareness) {
	m.awareMu.Lock()
	m.awareModule = a
	m.awareMu.Unlock()
}

// awareness returns the module, or nil when none was set.
func (m *Manager) awareness() Awareness {
	m.awareMu.RLock()
	defer m.awareMu.RUnlock()
	return m.awareModule
}

// sendTurn hands the agent the message that begins a turn. It is the one place a turn's message
// reaches an agent - Send's direct path and the pump's queued path both come through it - so the
// awareness summary is put in front of a turn's message exactly once, and only when the turn
// actually starts.
//
// That placement is the point. The text a queue holds is what the person typed: a message that had
// to wait is not given the summary where it was accepted, because it is not starting a turn there,
// and giving it one then would mean the summary was built against a board that had already moved on
// and then carried into a turn that added a second one.
func (m *Manager) sendTurn(ctx context.Context, ls *liveSession, text string) error {
	return ls.agent.Send(ctx, ls.handle, agents.UserMessage{Text: m.turnText(ctx, ls, text)})
}

// turnText puts a card's board-awareness summary in front of a turn's message, separated by a blank
// line so the person's own words are still easy to find in the transcript.
//
// A chat has no board to be aware of, and a summary that cannot be built is left out rather than
// failing the send: the send is the thing that matters, and a missing summary is a turn that runs
// with less context, not one that does not run.
func (m *Manager) turnText(ctx context.Context, ls *liveSession, text string) string {
	if ls.isChat() {
		return text
	}
	module := m.awareness()
	if module == nil {
		return text
	}
	summary := strings.TrimSpace(module.TurnAwareness(ctx, ls.cardID))
	if summary == "" {
		return text
	}
	return summary + "\n\n" + text
}
