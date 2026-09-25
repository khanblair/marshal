package events

import "sync"

// Subscription is one subscriber's view of the bus. Read events from C. Close it when done, or
// its goroutine keeps running until the bus closes.
//
// Each subscription has two queues, one for ordinary events and one for critical events, and one
// goroutine that merges them into C by sequence number. The queues are bounded, so a subscriber
// that does not read cannot grow the daemon's memory or slow the publisher:
//
//   - When the ordinary queue is full, the oldest ordinary event is dropped and Lagged reports
//     true. The consumer should reload what it shows from the store and call TakeLagged.
//   - Critical events are never dropped. If the critical queue overflows too, the subscription
//     closes with ClosedForResync and the consumer must reload from the store and subscribe again.
//
// Besides the two queues, at most one event is held by the goroutine while it waits for the
// reader.
type Subscription struct {
	bus      *Bus
	out      chan Event
	wake     chan struct{} // holds one token: "the queues changed"
	done     chan struct{} // closed when the subscription ends
	finished chan struct{} // closed when the goroutine has exited

	mu            sync.Mutex
	filter        Filter
	normal        queue
	critical      queue
	normalLimit   int
	criticalLimit int
	lagged        bool
	closed        bool
	closedReason  CloseReason
}

func newSubscription(bus *Bus, filter Filter) *Subscription {
	return &Subscription{
		bus:           bus,
		out:           make(chan Event),
		wake:          make(chan struct{}, 1),
		done:          make(chan struct{}),
		finished:      make(chan struct{}),
		filter:        filter.clone(),
		normalLimit:   bus.bufferSize,
		criticalLimit: bus.criticalSize,
	}
}

// C is the channel of events, in sequence order. It is closed when the subscription ends. An
// event may still arrive shortly after Close.
func (s *Subscription) C() <-chan Event { return s.out }

// Lagged reports whether an ordinary event was dropped since the last TakeLagged. It stays true
// for good on a subscription that was closed for a resync.
func (s *Subscription) Lagged() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.laggedLocked()
}

// TakeLagged reports Lagged and clears it, in one step, so a drop that happens just after the
// call is reported by the next call and never lost.
func (s *Subscription) TakeLagged() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	lagged := s.laggedLocked()
	s.lagged = false
	return lagged
}

func (s *Subscription) laggedLocked() bool {
	return s.lagged || s.closedReason == ClosedForResync
}

// Reason says why the subscription ended, or StillOpen.
func (s *Subscription) Reason() CloseReason {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closedReason
}

// Add follows more topics. Events published before the call are not delivered. It does nothing
// on a subscription that follows every topic.
func (s *Subscription) Add(topics ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filter.add(topics...)
}

// Remove stops following topics. Events already queued for them are still delivered.
func (s *Subscription) Remove(topics ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filter.remove(topics...)
}

// Close ends the subscription, discards its queued events, closes C, and waits for its goroutine
// to exit. It is safe to call more than once and from any goroutine.
func (s *Subscription) Close() {
	s.bus.forget(s)
	s.finish(ClosedByCaller)
	<-s.finished
}

// finish ends the subscription for a reason. Only the first call has an effect.
func (s *Subscription) finish(reason CloseReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked(reason)
}

// offer queues an event if the subscription follows its topic. It never blocks. It reports false
// when the subscription ended because the critical queue overflowed.
func (s *Subscription) offer(ev Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.filter.Matches(ev.Topic) {
		return true
	}
	if ev.Critical {
		if s.critical.len() >= s.criticalLimit {
			s.closeLocked(ClosedForResync)
			return false
		}
		s.critical.push(ev)
	} else {
		if s.normal.len() >= s.normalLimit {
			s.normal.pop()
			s.lagged = true
		}
		s.normal.push(ev)
	}
	select {
	case s.wake <- struct{}{}:
	default: // A token is already waiting, and it covers this event too.
	}
	return true
}

// closeLocked is finish for a caller that already holds the lock.
func (s *Subscription) closeLocked(reason CloseReason) {
	if s.closed {
		return
	}
	s.closed = true
	s.closedReason = reason
	s.normal.clear()
	s.critical.clear()
	close(s.done)
}

// next takes the queued event with the lowest sequence number.
func (s *Subscription) next() (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Event{}, false
	}
	ordinary, hasOrdinary := s.normal.peek()
	critical, hasCritical := s.critical.peek()
	if hasOrdinary && (!hasCritical || ordinary.Seq < critical.Seq) {
		return s.normal.pop()
	}
	if hasCritical {
		return s.critical.pop()
	}
	return Event{}, false
}

// run is the subscription's only goroutine. It hands events to C one at a time and stops when
// the subscription ends. It never holds the lock while it waits.
func (s *Subscription) run() {
	defer close(s.finished)
	defer close(s.out)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		if ev, ok := s.next(); ok {
			select {
			case s.out <- ev:
			case <-s.done:
				return
			}
			continue
		}
		select {
		case <-s.wake:
		case <-s.done:
			return
		}
	}
}
