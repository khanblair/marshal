package agents

import "sync"

// EventSink delivers one session's events to its one reader. Every adapter needs the same rules
// around its event channel: a sender waits for room but gives up once the session is releasing
// blocked senders, a final event is still sent when the channel has room even after that release,
// and the channel closes exactly once. Session types in agents/acp and agents/claude hold one and
// wrap its methods under their own unexported names.
type EventSink struct {
	events    chan AgentEvent
	sendMu    sync.RWMutex
	closed    bool
	abort     chan struct{}
	abortOnce sync.Once
}

// NewEventSink makes a sink whose channel holds up to buffer events before a sender waits for a
// reader.
func NewEventSink(buffer int) *EventSink {
	return &EventSink{
		events: make(chan AgentEvent, buffer),
		abort:  make(chan struct{}),
	}
}

// C returns the channel a caller reads. It is the same channel for the sink's whole life, and it
// closes after the last event.
func (s *EventSink) C() <-chan AgentEvent { return s.events }

// Emit puts an event on the channel. It waits for room, and gives up when the sink is released or
// closed, so a late event is dropped rather than sent on a closed channel.
func (s *EventSink) Emit(ev AgentEvent) {
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.closed {
		return
	}
	select {
	case s.events <- ev:
	case <-s.abort:
	}
}

// EmitFinal sends an event that must not be lost when there is room for it, even after senders
// have been released. It only waits for a reader when the channel is full.
func (s *EventSink) EmitFinal(ev AgentEvent) {
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.closed {
		return
	}
	select {
	case s.events <- ev:
		return
	default:
	}
	select {
	case s.events <- ev:
	case <-s.abort:
	}
}

// Release lets every blocked Emit and EmitFinal give up.
func (s *EventSink) Release() { s.abortOnce.Do(func() { close(s.abort) }) }

// Close closes the event channel once, after every sender is done or released.
func (s *EventSink) Close() {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.events)
	}
}
