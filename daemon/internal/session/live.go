package session

import (
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// liveSession is what the Manager keeps in memory for one card's running session, on top of what
// is in the database. It is reached through Manager.sessions, guarded by Manager.mu; its own
// fields below are guarded by its own locks, since the pump goroutine and callers of Send both
// touch the turn bookkeeping independently of the sessions map.
type liveSession struct {
	cardID string
	// projectID is the project of the card, kept so a project can find its own sessions without
	// asking the database.
	projectID    string
	sessionRowID string
	agent        agents.Agent
	handle       agents.SessionHandle
	// events is the handle's event channel, taken once right after Start or Resume succeeded, and
	// read only by the pump goroutine. Taking it once, early, means a process that dies before the
	// pump's first read still delivers Failed and Exited on it: Agent.Events only returns an
	// already-closed, empty channel for a session that is not running, so it must not be called
	// again later to "get" the same events.
	events <-chan agents.AgentEvent
	// structured is Capabilities().StructuredEvents. Only a structured agent ever sends TurnEnded,
	// so only a structured agent's "is a turn running" is tracked here; a PTY-style agent (not
	// reachable through a card's chat in Phase 1) is never treated as busy by this bookkeeping,
	// since gating its future Send on TurnEnded would mean it could never send again.
	structured bool

	diskLog *sessionLog
	ring    *entryRing

	turnMu sync.Mutex
	busy   bool
	queue  []string

	stopMu        sync.Mutex
	stopRequested bool
}

// claimTurn tries to mark the session busy for a new turn. It returns true when the caller may
// call agent.Send: either no turn was running, or the session is not one this bookkeeping tracks
// busy-ness for at all (see the structured field).
func (ls *liveSession) claimTurn() bool {
	if !ls.structured {
		return true
	}
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	if ls.busy {
		return false
	}
	ls.busy = true
	return true
}

// enqueue adds a message to the queue, bounded to maxQueuedMessages. It reports whether there was
// room.
func (ls *liveSession) enqueue(text string) bool {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	if len(ls.queue) >= maxQueuedMessages {
		return false
	}
	ls.queue = append(ls.queue, text)
	return true
}

// forceBusy marks the session busy after the agent itself reported ErrBusy for a Send this
// bookkeeping thought was free to make, so the message that just failed is queued instead of lost.
func (ls *liveSession) forceBusy() {
	ls.turnMu.Lock()
	ls.busy = true
	ls.turnMu.Unlock()
}

// release marks the session free after a Send failed for a reason other than ErrBusy.
func (ls *liveSession) release() {
	ls.turnMu.Lock()
	ls.busy = false
	ls.turnMu.Unlock()
}

// next pops the oldest queued message, if any. When the queue is empty it marks the session free
// and reports false.
func (ls *liveSession) next() (string, bool) {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	if len(ls.queue) == 0 {
		ls.busy = false
		return "", false
	}
	text := ls.queue[0]
	ls.queue = ls.queue[1:]
	return text, true
}

// setStopRequested marks that Stop or Close asked this session to end, so the pump's Exited
// handling knows the exit was expected and must not treat it as a crash.
func (ls *liveSession) setStopRequested() {
	ls.stopMu.Lock()
	ls.stopRequested = true
	ls.stopMu.Unlock()
}

func (ls *liveSession) wasStopRequested() bool {
	ls.stopMu.Lock()
	defer ls.stopMu.Unlock()
	return ls.stopRequested
}
