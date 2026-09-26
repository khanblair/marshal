package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/coder/websocket"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// eventEnvelopeBytes is what an event adds to its data on the wire: the keys, the topic, the
// type, the time, and the number. It is an upper bound, used to keep a frame under its size limit
// without encoding every event twice.
const eventEnvelopeBytes = 192

// errStreamEnded says the subscription ended for a reason that is not a resync, which is the bus
// closing during shutdown.
var errStreamEnded = errors.New("the event source closed")

// pump moves events from the bus to one client. Its goroutine is the only writer of event frames,
// so the client sees them in order. It holds events for one flush interval so that a burst goes
// out in a few frames, and it never holds more than a frame's limit.
type pump struct {
	st     *stream
	ws     *websocket.Conn
	bus    *events.Bus
	limits Limits
	sub    *events.Subscription
	topics map[protocol.Topic]struct{}

	batch      []protocol.Event
	batchBytes int
	timer      *time.Timer
	armed      bool
}

// newPump subscribes for the Hello and sends the first frames: a Resync when the client's
// position cannot be replayed, or the events it missed.
func newPump(ctx context.Context, st *stream, ws *websocket.Conn, hello protocol.Hello) (*pump, error) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	p := &pump{
		st: st, ws: ws, bus: st.hub.bus, limits: st.hub.limits,
		topics: topicSet(hello.Subscribe), timer: timer,
	}
	resume := p.bus.SubscribeSince(p.filter(), hello.Epoch, hello.SinceSeq)
	p.sub = resume.Subscription
	if err := p.begin(ctx, resume); err != nil {
		p.close()
		return nil, err
	}
	return p, nil
}

func topicSet(topics []protocol.Topic) map[protocol.Topic]struct{} {
	set := make(map[protocol.Topic]struct{}, len(topics))
	for _, topic := range topics {
		set[topic] = struct{}{}
	}
	return set
}

func (p *pump) filter() events.Filter {
	names := make([]string, 0, len(p.topics))
	for topic := range p.topics {
		names = append(names, string(topic))
	}
	return events.Topics(names...)
}

// close releases the subscription and the timer.
func (p *pump) close() {
	p.timer.Stop()
	p.sub.Close()
}

// begin sends what a new or replaced subscription starts with. When the replay did not work the
// client gets a Resync and reloads; when it did, it gets the events it missed, in order.
func (p *pump) begin(ctx context.Context, resume events.Resume) error {
	if reason, needed := resume.Replay.ResyncReason(); needed {
		return p.sendResync(ctx, reason, resume.Newest)
	}
	for _, ev := range resume.Events {
		if err := p.enqueue(ctx, ev); err != nil {
			return err
		}
	}
	return p.flush(ctx)
}

// run is the pump's loop. It takes events from the subscription, flushes on the timer, applies the
// client's later messages (a new Hello, or a terminal message), and reports a subscriber that fell
// behind.
func (p *pump) run(ctx context.Context, messages <-chan clientMessage) error {
	for {
		var err error
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, open := <-p.sub.C():
			err = p.onEvent(ctx, ev, open)
		case <-p.timer.C:
			err = p.onTick(ctx)
		case msg := <-messages:
			err = p.onMessage(ctx, msg)
		}
		if err != nil {
			return err
		}
	}
}

// onEvent handles one value from the subscription channel: a real event, or the channel closing
// because the subscription needs to be replaced.
func (p *pump) onEvent(ctx context.Context, ev events.Event, open bool) error {
	if !open {
		return p.resubscribe(ctx)
	}
	return p.receive(ctx, ev)
}

// onTick flushes the batch that has been waiting for the flush interval.
func (p *pump) onTick(ctx context.Context) error {
	p.armed = false
	return p.flush(ctx)
}

// receive queues one event from the subscription. An event for a topic that a later Hello dropped
// is skipped, because the subscription may still have had it queued.
func (p *pump) receive(ctx context.Context, ev events.Event) error {
	if _, followed := p.topics[protocol.Topic(ev.Topic)]; followed {
		if err := p.enqueue(ctx, ev); err != nil {
			return err
		}
	}
	if p.sub.TakeLagged() {
		// The subscription dropped events for a slow client. Tell it to reload.
		return p.sendResync(ctx, protocol.ResyncReasonTooFarBehind, p.bus.Seq())
	}
	return nil
}

// enqueue adds an event to the frame being built. A full frame goes out first, so no frame is
// over its limits, and a frame that reaches the event limit goes out at once. An event that cannot
// be encoded is logged and skipped: one bad payload must not end a client's stream.
func (p *pump) enqueue(ctx context.Context, ev events.Event) error {
	wire, size, ok := p.encode(ev)
	if !ok {
		return nil
	}
	if len(p.batch) > 0 && p.batchBytes+size > p.limits.MaxBatchBytes {
		if err := p.flush(ctx); err != nil {
			return err
		}
	}
	p.batch = append(p.batch, wire)
	p.batchBytes += size
	if len(p.batch) >= p.limits.MaxBatchEvents {
		return p.flush(ctx)
	}
	if !p.armed {
		p.timer.Reset(p.limits.FlushInterval)
		p.armed = true
	}
	return nil
}

func (p *pump) encode(ev events.Event) (protocol.Event, int, bool) {
	data, err := json.Marshal(ev.Data)
	if err != nil || ev.At.IsZero() {
		// The payload is never logged, only what identifies the event.
		p.st.log.Warn("skip an event that cannot be sent", "seq", ev.Seq, "topic", ev.Topic, "type", ev.Type, "error", err)
		return protocol.Event{}, 0, false
	}
	wire := protocol.Event{
		Seq:   ev.Seq,
		Topic: protocol.Topic(ev.Topic),
		Type:  protocol.EventType(ev.Type),
		At:    protocol.NewTimestamp(ev.At),
		Data:  data,
	}
	return wire, len(data) + eventEnvelopeBytes, true
}

// flush sends the events held so far as one frame.
func (p *pump) flush(ctx context.Context) error {
	if p.armed {
		p.timer.Stop()
		p.armed = false
	}
	if len(p.batch) == 0 {
		return nil
	}
	frame := protocol.EventBatch{Epoch: p.bus.Epoch(), Events: p.batch}
	err := p.st.send(ctx, p.ws, frame)
	p.batch, p.batchBytes = p.batch[:0], 0
	return err
}

// sendResync sends events that are waiting first, so the order the client sees is the order they
// happened in, and then the Resync.
func (p *pump) sendResync(ctx context.Context, reason protocol.ResyncReason, seq uint64) error {
	if err := p.flush(ctx); err != nil {
		return err
	}
	return p.st.send(ctx, p.ws, protocol.Resync{Epoch: p.bus.Epoch(), Reason: reason, Seq: seq})
}

// resubscribe handles a subscription whose channel closed. A subscription that was closed for a
// resync (its critical queue overflowed) is replaced by a new one that starts at the newest event,
// and the client is told to reload. Only that reason is answered this way: after the bus itself is
// closed, subscribing again would only hand back a closed subscription, so the stream ends.
func (p *pump) resubscribe(ctx context.Context) error {
	if p.sub.Reason() != events.ClosedForResync {
		return errStreamEnded
	}
	since := p.bus.Seq()
	if err := p.sendResync(ctx, protocol.ResyncReasonTooFarBehind, since); err != nil {
		return err
	}
	resume := p.bus.SubscribeSince(p.filter(), p.bus.Epoch(), since)
	p.sub = resume.Subscription
	return p.begin(ctx, resume)
}

// replace applies a later Hello. Without an epoch only the topics change, for events from now on.
// With an epoch, the client also says where it left off, so the subscription is made again and
// what it missed is replayed, in the same way as for the first Hello.
func (p *pump) replace(ctx context.Context, hello protocol.Hello) error {
	next := topicSet(hello.Subscribe)
	if hello.Epoch == "" {
		p.changeTopics(next)
		return nil
	}
	if err := p.flush(ctx); err != nil {
		return err
	}
	p.topics = next
	resume := p.bus.SubscribeSince(p.filter(), hello.Epoch, hello.SinceSeq)
	old := p.sub
	p.sub = resume.Subscription
	old.Close()
	return p.begin(ctx, resume)
}

// changeTopics follows and unfollows topics on the live subscription.
func (p *pump) changeTopics(next map[protocol.Topic]struct{}) {
	var added, removed []string
	for topic := range next {
		if _, had := p.topics[topic]; !had {
			added = append(added, string(topic))
		}
	}
	for topic := range p.topics {
		if _, keeps := next[topic]; !keeps {
			removed = append(removed, string(topic))
		}
	}
	p.sub.Add(added...)
	p.sub.Remove(removed...)
	p.topics = next
}
