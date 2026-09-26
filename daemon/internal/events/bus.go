// Package events is the daemon's in-process publish and subscribe. Modules publish small events
// such as "card.moved"; other modules and the WebSocket layer subscribe to the topics they care
// about. There is no external queue.
//
// The bus gives four guarantees:
//
//   - Publish never blocks and never waits for a subscriber. It assigns the event its sequence
//     number before it returns.
//   - Every subscriber sees events in sequence order. A subscriber that follows some topics sees
//     gaps in the numbers, which is normal.
//   - A subscriber that reads too slowly loses its oldest ordinary events (and is told, see
//     Subscription.Lagged). Critical events are never dropped: if even their queue overflows, the
//     subscription is closed so the consumer reloads from the store.
//   - The bus keeps the most recent events in a ring, so a client that reconnects can be replayed
//     what it missed (Since and SubscribeSince), or be told to reload (Replay). An event
//     published with PublishLive is not kept in the ring, so a stream of bytes such as a
//     terminal's output cannot push the events that matter out of it.
//
// Bounds, all configurable: 256 ordinary events (DefaultBufferSize) and 1,024 critical events
// (DefaultCriticalBufferSize) per subscriber, and 2,000 events in the ring
// (protocol.ReplayBufferSize). There is one goroutine per subscription and none per event.
package events

import (
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// DefaultBufferSize is how many ordinary events a subscriber may have waiting.
	DefaultBufferSize = 256
	// DefaultCriticalBufferSize is how many critical events a subscriber may have waiting.
	DefaultCriticalBufferSize = 1024
	// DefaultReplaySize is how many recent events the bus keeps for replay.
	DefaultReplaySize = protocol.ReplayBufferSize
)

// Bus is the event bus. It is safe for use by many goroutines.
type Bus struct {
	now          func() time.Time
	epoch        string
	bufferSize   int
	criticalSize int
	replaySize   int

	mu  sync.Mutex // guards everything below
	seq uint64
	// ring holds the newest replayable events. A live-only event (PublishLive) takes a sequence
	// number and never enters it, so the ring's sequence numbers are increasing but not always
	// contiguous.
	ring queue
	// evicted is the sequence number of the newest replayable event that is no longer in the ring
	// (0 while none has left it). A client that has applied everything up to evicted can be
	// replayed the rest from the ring, and one that is behind it cannot.
	evicted uint64
	subs    map[*Subscription]struct{}
	closed  bool
}

// Option changes how New works.
type Option func(*config)

type config struct {
	now          func() time.Time
	epoch        string
	entropy      io.Reader
	bufferSize   int
	criticalSize int
	replaySize   int
}

// WithClock sets the clock that stamps events. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(c *config) { c.now = now }
}

// WithEpoch sets the epoch id. The default is a new id from protocol.NewID.
func WithEpoch(epoch string) Option {
	return func(c *config) { c.epoch = epoch }
}

// WithBufferSize sets how many ordinary events each subscriber may have waiting. A value below 1
// is treated as 1.
func WithBufferSize(n int) Option {
	return func(c *config) { c.bufferSize = max(n, 1) }
}

// WithCriticalBufferSize sets how many critical events each subscriber may have waiting. A value
// below 1 is treated as 1.
func WithCriticalBufferSize(n int) Option {
	return func(c *config) { c.criticalSize = max(n, 1) }
}

// WithReplaySize sets how many recent events the bus keeps for replay. A value below 1 is treated
// as 1.
func WithReplaySize(n int) Option {
	return func(c *config) { c.replaySize = max(n, 1) }
}

// New makes a bus. It fails only if it cannot make an epoch id.
func New(opts ...Option) (*Bus, error) {
	cfg := config{
		now:          time.Now,
		entropy:      rand.Reader,
		bufferSize:   DefaultBufferSize,
		criticalSize: DefaultCriticalBufferSize,
		replaySize:   DefaultReplaySize,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.epoch == "" {
		epoch, err := protocol.NewID(cfg.now(), cfg.entropy)
		if err != nil {
			return nil, fmt.Errorf("make the event epoch: %w", err)
		}
		cfg.epoch = epoch
	}
	return &Bus{
		now:          cfg.now,
		epoch:        cfg.epoch,
		bufferSize:   cfg.bufferSize,
		criticalSize: cfg.criticalSize,
		replaySize:   cfg.replaySize,
		subs:         make(map[*Subscription]struct{}),
	}, nil
}

// Epoch is the id of this run of the bus. Sequence numbers are only meaningful inside it.
func (b *Bus) Epoch() string { return b.epoch }

// Seq is the sequence number of the newest event, or 0 if nothing was published yet.
func (b *Bus) Seq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

// Publish sends an event to every subscriber that follows its topic and returns its sequence
// number. It never blocks. It returns 0 if the bus is closed. Publish after the change is
// committed to the store, so a subscriber that reloads sees it.
func (b *Bus) Publish(topic, eventType string, data any, critical bool) uint64 {
	return b.publish(topic, eventType, data, critical, true)
}

// PublishLive sends an event that is for the subscribers of this moment only, and never enters the
// replay ring. It is for a stream whose bytes must not be applied twice or out of place, and that
// would push every other event out of the ring: a terminal's output (docs/architecture.md 11.2). A
// client that reconnects is not replayed it and asks for the current state instead.
//
// It is an ordinary event in every other way. It has the next sequence number, so every subscriber
// still sees events in sequence order, and it is not critical, so a subscriber that reads too
// slowly loses its oldest ordinary events and is told to reload, exactly as for any ordinary
// event. It returns 0 if the bus is closed.
func (b *Bus) PublishLive(topic, eventType string, data any) uint64 {
	return b.publish(topic, eventType, data, false, false)
}

// publish is Publish and PublishLive: replayable says whether the event enters the ring.
func (b *Bus) publish(topic, eventType string, data any, critical, replayable bool) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0
	}
	b.seq++
	ev := Event{Seq: b.seq, Topic: topic, Type: eventType, At: b.now(), Critical: critical, Data: data}
	if replayable {
		if b.ring.len() >= b.replaySize {
			oldest, _ := b.ring.pop()
			b.evicted = oldest.Seq
		}
		b.ring.push(ev)
	}
	for sub := range b.subs {
		if !sub.offer(ev) {
			delete(b.subs, sub)
		}
	}
	return ev.Seq
}

// Subscribe starts a subscription that receives events published from now on.
func (b *Bus) Subscribe(filter Filter) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.subscribeLocked(filter)
}

// Since returns the events after seq, on every topic, if the ring still holds them all. Use
// Filter.Apply to keep the topics a client follows. Prefer SubscribeSince when the client will
// also follow live events: calling Since and then Subscribe leaves a gap.
func (b *Bus) Since(epoch string, seq uint64) ([]Event, Replay) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sinceLocked(epoch, seq)
}

// Resume is what SubscribeSince returns.
type Resume struct {
	// Subscription follows live events. It starts right after Newest, so nothing is missed and
	// nothing is repeated between it and Events.
	Subscription *Subscription
	// Events are the missed events on the followed topics, in order. Send them before anything
	// from Subscription. Empty unless Replay is ReplayOK.
	Events []Event
	// Replay says whether the replay worked. When it did not, send a Resync frame with Newest.
	Replay Replay
	// Newest is the sequence number of the newest event at the moment of the call, or 0.
	Newest uint64
}

// SubscribeSince subscribes and replays what the client missed as one step, so no event falls
// between the replay and the live events.
func (b *Bus) SubscribeSince(filter Filter, epoch string, seq uint64) Resume {
	b.mu.Lock()
	defer b.mu.Unlock()
	missed, replay := b.sinceLocked(epoch, seq)
	return Resume{
		Subscription: b.subscribeLocked(filter),
		Events:       filter.Apply(missed),
		Replay:       replay,
		Newest:       b.seq,
	}
}

// Close ends every subscription and stops accepting events. It is safe to call more than once.
func (b *Bus) Close() {
	b.mu.Lock()
	b.closed = true
	subs := make([]*Subscription, 0, len(b.subs))
	for sub := range b.subs {
		sub.finish(ClosedByBus)
		subs = append(subs, sub)
	}
	clear(b.subs)
	b.ring.clear()
	b.evicted = b.seq
	b.mu.Unlock()
	for _, sub := range subs {
		<-sub.finished
	}
}

func (b *Bus) subscribeLocked(filter Filter) *Subscription {
	sub := newSubscription(b, filter)
	if b.closed {
		sub.finish(ClosedByBus)
	} else {
		b.subs[sub] = struct{}{}
	}
	go sub.run()
	return sub
}

// forget stops publishing to a subscription that has been closed.
func (b *Bus) forget(sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, sub)
}

// sinceLocked decides whether the ring covers a client's position. Coverage is judged on the
// sequence numbers of all replayable events, before any topic filter: the client needs every one
// after seq, so the replay works only while none of them has left the ring. A live-only event is
// never replayed, so it does not count against that.
func (b *Bus) sinceLocked(epoch string, seq uint64) ([]Event, Replay) {
	switch {
	case epoch != b.epoch:
		return nil, ReplayOtherEpoch
	case seq > b.seq:
		return nil, ReplayAhead
	case seq == b.seq:
		return nil, ReplayOK
	case seq < b.evicted:
		return nil, ReplayTooOld
	}
	// The ring's sequence numbers increase, so the missed events are its tail.
	first := b.ring.len()
	for first > 0 && b.ring.at(first-1).Seq > seq {
		first--
	}
	missed := make([]Event, 0, b.ring.len()-first)
	for i := first; i < b.ring.len(); i++ {
		missed = append(missed, b.ring.at(i))
	}
	return missed, ReplayOK
}
