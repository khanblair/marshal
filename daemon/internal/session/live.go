package session

import (
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/harness"
)

// liveSession is what the Manager keeps in memory for one running session, a card's or a chat's,
// on top of what is in the database. It is reached through Manager.sessions, guarded by
// Manager.mu; its own fields below are guarded by its own locks, since the pump goroutine and
// callers of Send both touch the turn bookkeeping independently of the sessions map.
type liveSession struct {
	// owner is whose session it is. Its cardID, chatID, and projectID are read straight from here.
	owner
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

	// term is set while the card's agent runs in a terminal (the terminal view): its screen, its input
	// queue, and the calls that reach the pseudo-terminal. It is nil for a session in the chat view,
	// and for every chat's session, which has no terminal.
	term *terminalSession

	turnMu sync.Mutex
	busy   bool
	queue  []string

	// settings is the model, thinking mode, and permission mode the session is running with, and
	// settingsMu guards it. It starts as what Start or Resume was given and moves on when a card's
	// setting change is given to the running agent before the next turn (see the manager's
	// applyCardSettings, B3.6). It is never written into handle, which other goroutines read as a
	// plain value.
	settingsMu sync.Mutex
	settings   sessionSettings

	stopMu        sync.Mutex
	stopRequested bool

	// scannedThrough is the last commit the secret scanner has read on this card's branch (B3.5).
	// It is read and written only by the pump goroutine, in the turn-end scan, so it needs no lock.
	scannedThrough string

	// The fields below are the harness's own bookkeeping for a card (B5.3): the stuck detector, the
	// reason it last gave, and the file each tool call is about. The detector and its reason are
	// read and written only by the pump goroutine; the reason is guarded by turnMu because the
	// turn-end check reads it from the pump while a turn is being timed from a request.
	loop  *harness.StuckDetector
	stuck *harness.Reason

	// turnStartedAt is when the current turn began, for the role's own time ceiling, and toolPaths
	// remembers the file each tool call is about so a failed update can say where it happened. Both
	// are guarded by turnMu.
	turnStartedAt time.Time
	toolPaths     map[string]string
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

// isBusy says whether a turn is running, as far as this bookkeeping tracks it. A session that does
// not report turns is never busy.
func (ls *liveSession) isBusy() bool {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	return ls.busy
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

// release marks the session free: after a Send failed for a reason other than ErrBusy, and after
// a turn ended while a pause held the next message, where the queue is not empty but no turn is
// running.
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

// takeIfIdle pops the oldest queued message when no turn is running, and marks the session busy
// for the turn it is about to start. It is how a pause that was holding messages lets them go
// when the person resumes the card: a turn that is already running is left to the pump's
// TurnEnded path, which delivers the next message when that turn really ends.
func (ls *liveSession) takeIfIdle() (string, bool) {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	if ls.busy || len(ls.queue) == 0 {
		return "", false
	}
	text := ls.queue[0]
	ls.queue = ls.queue[1:]
	ls.busy = true
	return text, true
}

// waiting says how many messages are queued for this session.
func (ls *liveSession) waiting() int {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	return len(ls.queue)
}

// setStopRequested marks that Stop, Close, or Sleep asked this session to end, so the pump's
// Exited handling knows the exit was expected and must not treat it as a crash. What the row is
// left reading is the caller's business: Stop writes "stopped" before asking, Sleep writes
// "asleep" after, and Close deliberately writes nothing.
func (ls *liveSession) setStopRequested() {
	ls.stopMu.Lock()
	ls.stopRequested = true
	ls.stopMu.Unlock()
}

// clearStopRequested takes back setStopRequested when the stop it announced never happened (the
// agent refused to end), so a session that is still running is not treated as one on its way out.
func (ls *liveSession) clearStopRequested() {
	ls.stopMu.Lock()
	ls.stopRequested = false
	ls.stopMu.Unlock()
}

func (ls *liveSession) wasStopRequested() bool {
	ls.stopMu.Lock()
	defer ls.stopMu.Unlock()
	return ls.stopRequested
}

// sessionSettings is the model, thinking mode, and permission mode a session is running with.
type sessionSettings struct {
	model          string
	thinking       string
	permissionMode string
}

// settingsNow returns the settings the session is running with.
func (ls *liveSession) settingsNow() sessionSettings {
	ls.settingsMu.Lock()
	defer ls.settingsMu.Unlock()
	return ls.settings
}

// setSettings records the settings the session is running with now, after the agent took a change
// or after it started with them.
func (ls *liveSession) setSettings(s sessionSettings) {
	ls.settingsMu.Lock()
	ls.settings = s
	ls.settingsMu.Unlock()
}
