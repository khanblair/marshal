package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/keyedlock"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// ErrNoLiveSession means a card has no session running in this process: it was never started, it
// already stopped, or the daemon has not resumed it yet.
var ErrNoLiveSession = errors.New("no live session for this card")

// Manager owns the life cycle of every session, a card's and a project chat's: starting, sending,
// stopping, sleeping, and resuming (docs/architecture.md section 5). A card's session is started by
// a person and follows the card; a chat's session starts on demand, when its first message is sent,
// and belongs to no card. It is safe for use by many goroutines.
type Manager struct {
	store    *store.Store
	bus      *events.Bus
	projects *projects.Service
	registry *agents.Registry
	git      *gitx.Git
	cfg      Config
	log      *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	closed    bool
	sessions  map[string]*liveSession // card or chat id -> live session
	pending   map[string]struct{}     // cardID -> a Start or Resume is in flight for it
	switching map[string]struct{}     // cardID -> a switch of its view is in flight

	// chatLocks serializes what is done to one chat's session: sending to it, and putting it to
	// sleep. A chat that is not running is started by its next message, and two messages must not
	// each start a process.
	chatLocks keyedlock.Locks
	// viewLocks serializes what is done to a card's session while its view is switched: the switch
	// stops the process and starts another, and a message, a stop, a sleep, or a delete that arrived in
	// between would find no session. They wait for the switch instead and then act on the new session.
	viewLocks keyedlock.Locks

	pumpWG sync.WaitGroup
}

// NewManager builds a Manager. Every dependency is required.
func NewManager(st *store.Store, bus *events.Bus, proj *projects.Service, registry *agents.Registry, git *gitx.Git, cfg Config) (*Manager, error) {
	if st == nil || bus == nil || proj == nil || registry == nil || git == nil {
		return nil, errors.New("the session manager needs the store, the event bus, projects, the agent registry, and git")
	}
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	if cfg.History == nil {
		recorder, err := history.New(st)
		if err != nil {
			return nil, fmt.Errorf("make the history store: %w", err)
		}
		cfg.History = recorder
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store: st, bus: bus, projects: proj, registry: registry, git: git, cfg: cfg, log: cfg.Logger,
		ctx: ctx, cancel: cancel,
		sessions: make(map[string]*liveSession), pending: make(map[string]struct{}),
		switching: make(map[string]struct{}),
	}, nil
}

// RecentOutput returns a copy of the recent structured log entries for a card's live session, for
// a later "what happened before I opened this card" API route. It is nil for a card with no live
// session.
func (m *Manager) RecentOutput(cardID string) []LogEntry {
	m.mu.Lock()
	ls, ok := m.sessions[cardID]
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return ls.ring.snapshot()
}

// reserve claims cardID for a Start or Resume in progress, refusing when the card already has a
// live session, is already being started or resumed, or the manager is shutting down. Call
// release when done, however it ends.
func (m *Manager) reserve(cardID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return protocol.Unavailable("Marshal is shutting down.")
	}
	if _, switching := m.switching[cardID]; switching {
		return refusedView(protocol.ViewRefusalReasonSwitching).With("cardId", cardID)
	}
	if _, live := m.sessions[cardID]; live {
		return protocol.Refused("This card's agent is already running.").With("cardId", cardID)
	}
	if _, pending := m.pending[cardID]; pending {
		return protocol.Refused("This card is already being started.").With("cardId", cardID)
	}
	m.pending[cardID] = struct{}{}
	return nil
}

func (m *Manager) release(cardID string) {
	m.mu.Lock()
	delete(m.pending, cardID)
	m.mu.Unlock()
}

// live returns the live session of a card, or ErrNoLiveSession wrapped in a plain protocol.Refused.
func (m *Manager) live(cardID string) (*liveSession, error) {
	ls := m.liveOf(cardID)
	if ls == nil {
		return nil, protocol.Refused("This card has no agent running.").With("cardId", cardID).WithCause(ErrNoLiveSession)
	}
	return ls, nil
}

// liveOf returns a card's live session in this process, or nil when it has none.
func (m *Manager) liveOf(cardID string) *liveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[cardID]
}

// cardPaused reports whether a card is held by a pause, for the paths that must let a held
// message wait (Send) or keep it waiting (a turn ending). A card that cannot be read is not
// treated as paused: those paths have their own answer for a card that is gone, and delivering a
// message is the behavior they had before a pause existed.
func (m *Manager) cardPaused(ctx context.Context, cardID string) bool {
	card, err := m.projects.Card(ctx, cardID)
	if err != nil {
		m.log.Warn("could not read a card to see whether it is paused", "card_id", cardID, "error", err)
		return false
	}
	return card.Paused
}

// forget removes a live session from the map, but only if it is still the one given: a session that
// has already been replaced (or removed by something else) is left alone. The key is the card's or
// the chat's id.
func (m *Manager) forget(key string, ls *liveSession) {
	m.mu.Lock()
	if m.sessions[key] == ls {
		delete(m.sessions, key)
	}
	m.mu.Unlock()
}

// goLive registers a freshly started or resumed session as live and starts its pump goroutine,
// unless the manager has been closed in the meantime, in which case it stops the session's agent
// at once instead, so a resume that finishes during shutdown never orphans a process.
func (m *Manager) goLive(ls *liveSession) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), closeStopTimeout)
		defer cancel()
		if err := ls.agent.Stop(ctx, ls.handle); err != nil {
			m.log.Error("could not stop a session that finished starting during shutdown",
				ls.noun()+"_id", ls.key(), "error", err)
		}
		// No pump will ever run for it, so nothing else would close its log.
		if err := ls.diskLog.close(); err != nil {
			m.log.Error("could not close the log of a session that finished starting during shutdown",
				ls.noun()+"_id", ls.key(), "error", err)
		}
		return protocol.Unavailable("Marshal is shutting down.")
	}
	m.sessions[ls.key()] = ls
	m.pumpWG.Add(1)
	if ls.term != nil {
		m.pumpWG.Add(1)
	}
	m.mu.Unlock()
	go m.pump(ls)
	if ls.term != nil {
		go m.pumpTerminalInput(ls)
	}
	return nil
}

// startedAgent is an agent together with the handle Start or Resume returned for it. The two
// always travel together from the moment an agent session exists, so bundling them keeps the
// functions that pass a freshly started or resumed session around (startAgent, resumeRow, and
// what they call) within the parameter-count guideline of code-standards.md section 12.
type startedAgent struct {
	agent  agents.Agent
	handle agents.SessionHandle
	// view is the view the agent was started in. The zero value is the chat view, which is what every
	// session starts in.
	view protocol.CardViewMode
}

// newLiveSession opens a session's on-disk log and ring and builds its bookkeeping.
func (m *Manager) newLiveSession(o owner, sessionRowID string, sa startedAgent) (*liveSession, error) {
	term, err := newTerminal(sa)
	if err != nil {
		return nil, err
	}
	events := sa.agent.Events(sa.handle)
	diskLog, err := newSessionLog(m.cfg.DataDir, sessionRowID, m.cfg.LogSegmentBytes)
	if err != nil {
		return nil, err
	}
	return &liveSession{
		owner: o, sessionRowID: sessionRowID,
		agent: sa.agent, handle: sa.handle, events: events,
		structured: sa.handle.Capabilities.StructuredEvents,
		diskLog:    diskLog, ring: newEntryRing(m.cfg.RingBytes), term: term,
	}, nil
}

// newTerminal makes the terminal of a session started in the terminal view, and nil for one started
// in the chat view. An agent that was started for the terminal view but cannot be driven as a
// terminal is a mistake in how it was registered, and is reported as one.
func newTerminal(sa startedAgent) (*terminalSession, error) {
	if sa.view != protocol.CardViewModeTerminal {
		return nil, nil
	}
	agent, ok := sa.agent.(agents.Terminal)
	if !ok {
		return nil, fmt.Errorf("the agent registered for the terminal view is not a terminal: %T", sa.agent)
	}
	return newTerminalSession(agent, sa.handle), nil
}

// setSessionState writes a session row's state, agent session id, and last-active time. Errors are
// logged and swallowed by callers that cannot usefully return them (the pump goroutine); callers
// on a request path return them.
func (m *Manager) setSessionState(ctx context.Context, ls *liveSession, state protocol.SessionState) error {
	now := m.cfg.Now()
	err := m.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(ctx, db.UpdateSessionRuntimeParams{
			State: string(state), AgentSessionID: ls.handle.ID,
			LastActiveAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(), ID: ls.sessionRowID,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("record that the session of %s %s moved to %s: %w", ls.noun(), ls.key(), state, err)
	}
	return nil
}

// setRowState writes a session row's state, keeping its agent session id, and moves its
// last-active time to now. It is for the moves a session makes while it has no live counterpart
// in this process: a sleep, and the "waking" half of a wake.
func (m *Manager) setRowState(ctx context.Context, row db.Session, state protocol.SessionState) error {
	now := m.cfg.Now()
	err := m.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(ctx, db.UpdateSessionRuntimeParams{
			State: string(state), AgentSessionID: row.AgentSessionID,
			LastActiveAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(), ID: row.ID,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("record that session %s moved to %s: %w", row.ID, state, err)
	}
	return nil
}

// publishState publishes session.state_changed for a live session, a card's or a chat's. State
// changes are critical: the event bus never drops them.
func (m *Manager) publishState(ls *liveSession, state protocol.SessionState, reason string) {
	if ls.isChat() {
		m.publishChatState(ls.chatID, ls.sessionRowID, state, reason)
		return
	}
	m.publishSessionState(ls.cardID, ls.sessionRowID, state, reason)
}

// publishChatState announces that a chat's session moved to a state, on the chat's own topic. A
// chat has no card on any board, so nothing else follows it: the project topic hears about a chat
// through the chats module's own events.
func (m *Manager) publishChatState(chatID, sessionRowID string, state protocol.SessionState, reason string) {
	m.bus.Publish(string(protocol.ChatTopic(chatID)), string(protocol.EventTypeSessionStateChanged),
		protocol.SessionStateChangedEventData{ChatID: chatID, SessionID: sessionRowID, State: state, Reason: reason}, true)
}

// publishSessionState announces that a card's session moved to a state, for a session named by the
// ids of its card and its own row, whether or not it is live. It is the one place a card's session
// state change is announced, so each change is announced exactly once on each of two topics:
// session.state_changed on the card's own topic, for an open card, and card.updated with the card
// as it now is on the project topic, because the wire card carries its session state and the board
// and Home follow only the project topic. The caller has already stored the state, which is what
// the card is read back with.
func (m *Manager) publishSessionState(cardID, sessionRowID string, state protocol.SessionState, reason string) {
	m.bus.Publish(string(protocol.CardTopic(cardID)), string(protocol.EventTypeSessionStateChanged),
		protocol.SessionStateChangedEventData{CardID: cardID, SessionID: sessionRowID, State: state, Reason: reason}, true)
	m.projects.SessionChanged(m.ctx, cardID)
}

// thinkingOrEmpty reads a card's thinking setting, which is a pointer because it may be unset.
func thinkingOrEmpty(t *protocol.ThinkingMode) string {
	if t == nil {
		return ""
	}
	return string(*t)
}
