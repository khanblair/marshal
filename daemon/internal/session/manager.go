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
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// ErrNoLiveSession means a card has no session running in this process: it was never started, it
// already stopped, or the daemon has not resumed it yet.
var ErrNoLiveSession = errors.New("no live session for this card")

// Manager owns the life cycle of every card's session: starting, sending, stopping, and resuming
// (docs/architecture.md section 5). It is safe for use by many goroutines.
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

	mu       sync.Mutex
	closed   bool
	sessions map[string]*liveSession // cardID -> live session
	pending  map[string]struct{}     // cardID -> a Start or Resume is in flight for it

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
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store: st, bus: bus, projects: proj, registry: registry, git: git, cfg: cfg, log: cfg.Logger,
		ctx: ctx, cancel: cancel,
		sessions: make(map[string]*liveSession), pending: make(map[string]struct{}),
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
	m.mu.Lock()
	ls, ok := m.sessions[cardID]
	m.mu.Unlock()
	if !ok {
		return nil, protocol.Refused("This card has no agent running.").With("cardId", cardID).WithCause(ErrNoLiveSession)
	}
	return ls, nil
}

// forget removes a card's live session from the map, but only if it is still the one given: a
// session that has already been replaced (or removed by something else) is left alone.
func (m *Manager) forget(cardID string, ls *liveSession) {
	m.mu.Lock()
	if m.sessions[cardID] == ls {
		delete(m.sessions, cardID)
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
				"card_id", ls.cardID, "error", err)
		}
		return protocol.Unavailable("Marshal is shutting down.")
	}
	m.sessions[ls.cardID] = ls
	m.pumpWG.Add(1)
	m.mu.Unlock()
	go m.pump(ls)
	return nil
}

// startedAgent is an agent together with the handle Start or Resume returned for it. The two
// always travel together from the moment an agent session exists, so bundling them keeps the
// functions that pass a freshly started or resumed session around (startAgent, resumeRow, and
// what they call) within the parameter-count guideline of code-standards.md section 12.
type startedAgent struct {
	agent  agents.Agent
	handle agents.SessionHandle
}

// newLiveSession opens a session's on-disk log and ring and builds its bookkeeping.
func (m *Manager) newLiveSession(card protocol.Card, sessionRowID string, sa startedAgent) (*liveSession, error) {
	events := sa.agent.Events(sa.handle)
	diskLog, err := newSessionLog(m.cfg.DataDir, sessionRowID, m.cfg.LogSegmentBytes)
	if err != nil {
		return nil, err
	}
	return &liveSession{
		cardID: card.ID, projectID: card.ProjectID, sessionRowID: sessionRowID,
		agent: sa.agent, handle: sa.handle, events: events,
		structured: sa.handle.Capabilities.StructuredEvents,
		diskLog:    diskLog, ring: newEntryRing(m.cfg.RingBytes),
	}, nil
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
		return fmt.Errorf("record that the session of card %s moved to %s: %w", ls.cardID, state, err)
	}
	return nil
}

// publishState publishes session.state_changed. State changes are critical: the event bus never
// drops them.
func (m *Manager) publishState(ls *liveSession, state protocol.SessionState, reason string) {
	m.bus.Publish(string(protocol.CardTopic(ls.cardID)), string(protocol.EventTypeSessionStateChanged),
		protocol.SessionStateChangedEventData{CardID: ls.cardID, SessionID: ls.sessionRowID, State: state, Reason: reason}, true)
}

// thinkingOrEmpty reads a card's thinking setting, which is a pointer because it may be unset.
func thinkingOrEmpty(t *protocol.ThinkingMode) string {
	if t == nil {
		return ""
	}
	return string(*t)
}
