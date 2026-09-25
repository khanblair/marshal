package acp

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

const (
	// eventBuffer is how many events wait in a session's channel. The agent slows down when the
	// reader falls this far behind.
	eventBuffer = 256
	// connectionDrainTimeout is how long the adapter waits for the protocol connection to notice
	// that the process is gone, before it closes the pipe itself.
	connectionDrainTimeout = 5 * time.Second
	// inboundDrainTimeout is how long the adapter waits for the protocol library to hand over the
	// messages it had already read. The library gives up on them after 5 seconds as well.
	inboundDrainTimeout = 6 * time.Second
)

// session is one running agent process and the protocol connection to it. The supervise goroutine
// owns its end of life: it is the only one that closes the event channel.
type session struct {
	adapter *Adapter
	log     *slog.Logger
	label   string

	// life is the lifetime of the process. It is not tied to the request that started the
	// session, and it ends when the session is over.
	life       context.Context
	lifeCancel context.CancelFunc

	proc           *proc.Process
	conn           *sdk.ClientSideConnection
	closeSupported bool

	// sink delivers events to the caller. Every send goes through emit or emitFinal below, and
	// only closeEvents closes it.
	sink *agents.EventSink

	mu           sync.Mutex
	id           sdk.SessionId
	accepting    bool
	instructions string
	turnActive   bool
	interrupted  bool
	turnCancel   context.CancelFunc
	interruptTmr *time.Timer
	pending      map[string]*pendingPermission
	requests     int
	stopping     bool
	ended        bool
	endedInTurn  bool
	inboundCtx   context.Context

	turns sync.WaitGroup
	// processEnded closes when the process has exited, and wakes what waits on the agent.
	processEnded chan struct{}
	// done closes when the session is completely over and its event channel is closed.
	done chan struct{}
}

// newSession makes the session for one start. The process comes later, in attach.
func newSession(a *Adapter, spec agents.StartSpec, life context.Context, cancel context.CancelFunc) *session {
	return &session{
		adapter:      a,
		log:          a.log.With("label", spec.Label),
		label:        spec.Label,
		life:         life,
		lifeCancel:   cancel,
		sink:         agents.NewEventSink(eventBuffer),
		instructions: spec.Instructions,
		pending:      make(map[string]*pendingPermission),
		processEnded: make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// emit puts an event on the channel. It waits for room, and gives up when the sender is released
// or the channel has been closed, so a late event is dropped rather than sent on a closed channel.
func (s *session) emit(ev agents.AgentEvent) { s.sink.Emit(ev) }

// emitFinal sends an event that must not be lost when there is room for it, even after senders
// have been released. It only waits for a reader when the channel is full.
func (s *session) emitFinal(ev agents.AgentEvent) { s.sink.EmitFinal(ev) }

// releaseSenders lets every blocked emit give up.
func (s *session) releaseSenders() { s.sink.Release() }

// closeEvents closes the event channel once, after every sender is done or released.
func (s *session) closeEvents() { s.sink.Close() }

// supervise owns the end of the session: it waits for the process, lets the connection and the
// running turn finish, reports what happened, and closes the event channel.
func (s *session) supervise() {
	defer close(s.done)
	select {
	case <-s.proc.Done():
	case <-s.conn.Done():
		// The agent closed its output but did not exit. The session cannot work any more.
		if err := s.proc.Stop(s.life, s.adapter.cfg.StopGrace); err != nil {
			s.log.Warn("could not stop an agent that closed its output", "err", err)
		}
	}
	exit := s.proc.Wait()
	s.markEnded()
	s.waitConnection()
	s.turns.Wait()
	s.report(exit)
	s.releaseSenders()
	s.closeEvents()
	s.adapter.forget(s.sessionID(), s)
	s.lifeCancel()
}

// markEnded records that the process is gone, so that nothing new starts and waiting requests
// give up.
func (s *session) markEnded() {
	s.mu.Lock()
	s.ended = true
	s.endedInTurn = s.endedInTurn || s.turnActive
	s.mu.Unlock()
	close(s.processEnded)
}

// waitConnection waits until the protocol connection has read what the process left in the pipe,
// and handed all of it to the client. Only then are the events complete, and Exited can follow.
func (s *session) waitConnection() {
	timer := time.NewTimer(connectionDrainTimeout)
	defer timer.Stop()
	select {
	case <-s.conn.Done():
	case <-timer.C:
		s.log.Warn("the agent's output did not end after the process did")
	}
	// Closing the read end lets the connection's reader finish if it was still blocked.
	if err := s.proc.Stdout.Close(); err != nil {
		s.log.Debug("close the agent's output", "err", err)
	}
	s.waitInboundDrain()
}

// noteInbound remembers the context that the protocol library gives to notification handlers. It
// ends once the library has handed over every message it read before the connection closed.
func (s *session) noteInbound(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inboundCtx == nil {
		s.inboundCtx = ctx
	}
}

// waitInboundDrain waits for the library to finish handing over messages.
func (s *session) waitInboundDrain() {
	s.mu.Lock()
	ctx := s.inboundCtx
	s.mu.Unlock()
	if ctx == nil {
		return
	}
	timer := time.NewTimer(inboundDrainTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// report sends the last events: a Failed event when the process ended without being asked to, and
// then Exited.
func (s *session) report(exit proc.Exit) {
	s.mu.Lock()
	expected := s.stopping
	inTurn := s.endedInTurn
	s.mu.Unlock()
	if expected {
		s.log.Info("agent session ended", "session_id", s.sessionID(), "exit_code", exit.Code)
	} else {
		s.log.Warn("agent process ended without being stopped", "session_id", s.sessionID(),
			"exit_code", exit.Code)
		s.emitFinal(agents.Failed{Message: crashMessage(inTurn), Detail: crashDetail(exit, s.proc.StderrTail())})
	}
	s.emitFinal(agents.Exited{Code: exit.Code, Err: exit.Err})
}

// sessionID returns the agent's session id.
func (s *session) sessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.id)
}

// setAccepting says whether session updates become events. They do not during the handshake, and
// they do not while an old session is replayed.
func (s *session) setAccepting(on bool) {
	s.mu.Lock()
	s.accepting = on
	s.mu.Unlock()
}

// acceptsUpdatesFor says whether an update for the given session id should become an event.
func (s *session) acceptsUpdatesFor(id sdk.SessionId) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepting && (s.id == "" || s.id == id)
}

// crashMessage is the plain sentence for a process that ended without being asked to.
func crashMessage(inTurn bool) string {
	if inTurn {
		return "The agent stopped in the middle of its work."
	}
	return "The agent stopped unexpectedly."
}

// crashDetail describes how the process ended, and what it printed last.
func crashDetail(exit proc.Exit, stderrTail string) string {
	detail := "exit code " + strconv.Itoa(exit.Code)
	if exit.Err != nil {
		detail = exit.Err.Error()
	}
	if stderrTail != "" {
		detail += "\n" + stderrTail
	}
	return detail
}
