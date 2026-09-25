package session_test

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// fakeAgent is a plain, scriptable implementation of agents.Agent, for unit tests that must never
// start a real process. A turn runs on its own goroutine and never emits onto its session's event
// channel from the caller's own goroutine, so a Send that happens to be called from the pump
// goroutine itself (delivering a queued message) can never deadlock against the pump reading that
// same channel.
type fakeAgent struct {
	caps agents.Capabilities

	mu        sync.Mutex
	sessions  map[string]*fakeSession
	startErr  error
	resumeErr error
	// sendErr, when set, is what Send returns instead of starting a turn (for a reason other than
	// the fake's own busy bookkeeping, which returns agents.ErrBusy on its own).
	sendErr error
	// stopErr, when set, is what Stop returns instead of ending the session normally.
	stopErr error
	nextID  int
	// hold, when set, makes every turn wait for a receive on it before sending TurnEnded, so a
	// test can control exactly when a turn ends.
	hold chan struct{}
	// startHold, when set, makes Start wait for a receive on it before returning, so a test can
	// create a reliable window during which two Start calls for the same card overlap.
	startHold chan struct{}
	// beforeSend, when set, is called synchronously at the very start of every Send, before this
	// fake decides busy or not, so a test can observe exactly what the caller (the Manager) had
	// already done by the time it asked the agent to start a turn.
	beforeSend func()
}

// fakeSession is one session a fakeAgent is running.
type fakeSession struct {
	id   string
	sink *agents.EventSink

	mu    sync.Mutex
	busy  bool
	turns int
}

func newFakeAgent(caps agents.Capabilities) *fakeAgent {
	return &fakeAgent{caps: caps, sessions: make(map[string]*fakeSession)}
}

func (a *fakeAgent) Start(_ context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	a.mu.Lock()
	hold := a.startHold
	a.mu.Unlock()
	if hold != nil {
		<-hold
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.startErr != nil {
		return agents.SessionHandle{}, a.startErr
	}
	a.nextID++
	id := fmt.Sprintf("fake-%d", a.nextID)
	a.sessions[id] = &fakeSession{id: id, sink: agents.NewEventSink(256)}
	return a.handleFor(id, spec), nil
}

func (a *fakeAgent) Resume(_ context.Context, sessionID string, spec agents.StartSpec) (agents.SessionHandle, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.resumeErr != nil {
		return agents.SessionHandle{}, a.resumeErr
	}
	s, ok := a.sessions[sessionID]
	if !ok {
		return agents.SessionHandle{}, fmt.Errorf("%w: %s", agents.ErrCannotResume, sessionID)
	}
	// A resumed process has a fresh event channel, but the fake keeps the turn count, the way the
	// real stub agent's own saved session does.
	s.mu.Lock()
	s.sink, s.busy = agents.NewEventSink(256), false
	s.mu.Unlock()
	return a.handleFor(sessionID, spec), nil
}

func (a *fakeAgent) handleFor(id string, spec agents.StartSpec) agents.SessionHandle {
	return agents.SessionHandle{
		ID: id, Label: spec.Label, Model: spec.Model, Thinking: spec.Thinking, PermissionMode: spec.PermissionMode,
		Capabilities: a.caps,
	}
}

func (a *fakeAgent) Send(_ context.Context, h agents.SessionHandle, msg agents.UserMessage) error {
	a.mu.Lock()
	before := a.beforeSend
	a.mu.Unlock()
	if before != nil {
		before()
	}
	s, err := a.find(h.ID)
	if err != nil {
		return err
	}
	a.mu.Lock()
	sendErr := a.sendErr
	a.mu.Unlock()
	if sendErr != nil {
		return sendErr
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return agents.ErrBusy
	}
	s.busy = true
	s.mu.Unlock()
	go a.runTurn(s, msg)
	return nil
}

// runTurn plays one turn: a message naming its turn number (so a test can prove a resumed session
// remembers earlier ones, the same way the real stub agent's "Turn N, I remember M earlier turns"
// line does) and then TurnEnded. When a.hold is set, the whole turn waits for a receive on it
// before emitting anything, so a test can make a turn's events arrive at an exact, known moment
// instead of racing whatever the test does right after Send returns.
func (a *fakeAgent) runTurn(s *fakeSession, msg agents.UserMessage) {
	a.mu.Lock()
	hold := a.hold
	a.mu.Unlock()
	if hold != nil {
		<-hold
	}
	s.mu.Lock()
	s.turns++
	n := s.turns
	s.mu.Unlock()
	s.sink.Emit(agents.MessageChunk{Text: fmt.Sprintf("turn %d remembers %d earlier turns: %s", n, n-1, msg.Text)})
	s.sink.Emit(agents.TurnEnded{Reason: agents.TurnEndTurn})
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

// setSendErr changes what Send returns for every session, safe to call while a pump goroutine may
// be concurrently calling Send (unlike a direct field write, which the race detector rightly
// flags: Send reads sendErr under a.mu, so a test that mutates it after a session has gone live
// must go through a.mu too).
func (a *fakeAgent) setSendErr(err error) {
	a.mu.Lock()
	a.sendErr = err
	a.mu.Unlock()
}

func (a *fakeAgent) Interrupt(context.Context, agents.SessionHandle) error { return nil }

func (a *fakeAgent) Events(h agents.SessionHandle) <-chan agents.AgentEvent {
	s, err := a.find(h.ID)
	if err != nil {
		ch := make(chan agents.AgentEvent)
		close(ch)
		return ch
	}
	return s.sink.C()
}

func (*fakeAgent) Respond(context.Context, agents.SessionHandle, agents.ApprovalResponse) error {
	return agents.ErrUnknownRequest
}

func (a *fakeAgent) Stop(_ context.Context, h agents.SessionHandle) error {
	a.mu.Lock()
	stopErr := a.stopErr
	a.mu.Unlock()
	if stopErr != nil {
		return stopErr
	}
	s, err := a.find(h.ID)
	if err != nil {
		return nil // already gone: Stop is idempotent
	}
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	if busy {
		// Mirrors a real adapter's graceful stop: a turn in flight ends with TurnEnded(cancelled)
		// before the process actually exits (docs/architecture.md 4.1's Interrupt rule; a plain
		// Stop takes a similar path when it has to end a turn to get there). This is what
		// onTurnEnded's own stopRequested check exists to survive.
		s.sink.Emit(agents.TurnEnded{Reason: agents.TurnCancelled})
	}
	s.sink.EmitFinal(agents.Exited{Code: 0})
	s.sink.Release()
	s.sink.Close()
	return nil
}

func (a *fakeAgent) Capabilities() agents.Capabilities {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.caps
}

func (a *fakeAgent) find(id string) (*fakeSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[id]
	if !ok {
		return nil, agents.ErrUnknownSession
	}
	return s, nil
}

// crash ends a session the way a process that died on its own would: Failed then Exited, with no
// Stop ever asked for.
func (a *fakeAgent) crash(id, message string) {
	s, err := a.find(id)
	if err != nil {
		return
	}
	s.sink.Emit(agents.Failed{Message: message})
	s.sink.EmitFinal(agents.Exited{Code: -1, Err: errors.New(message)})
	s.sink.Release()
	s.sink.Close()
}

var _ agents.Agent = (*fakeAgent)(nil)
