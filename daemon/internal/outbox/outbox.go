// Package outbox keeps what could not be sent because the internet was down, and sends it when the
// internet is back. A notice to a chat and a brief by email both go through it: the sender tries
// once, and if the failure was the network, the message is saved here instead of being dropped.
//
// The outbox sends oldest first, stops at the first network failure so it does not hammer a dead
// connection, and drops a message after a day, or after a service has refused it several times, so a
// long outage or a broken channel never ends in a flood. A message that goes out late says so.
package outbox

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/netwatch"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The kinds of message the outbox holds.
const (
	// KindNotice is a notice to a chat service or an email.
	KindNotice = "notice"
	// KindBrief is a brief sent to a channel.
	KindBrief = "brief"
)

// Limits. They are fields of Options so a test can shrink them.
const (
	// MaxAge is how old a message may be when its turn comes. Older than this is dropped.
	MaxAge = 24 * time.Hour
	// MaxAttempts is how many sends may fail for a reason other than the internet being down before the
	// message is dropped. With flushEvery between tries it is about an hour of a service refusing it.
	MaxAttempts = 10
	// MaxQueued is the most messages kept. When it is full the oldest are dropped.
	MaxQueued = 500
	// lateAfter is how long a message waits before it is called late.
	lateAfter = time.Minute
	// flushEvery is how often a stuck message is tried again while the internet is up.
	flushEvery = 5 * time.Minute
	// batch is how many messages one flush reads at a time.
	batch = 100
)

// Item is one saved message.
type Item struct {
	ID string
	// Kind says what the payload is, and so which handler sends it.
	Kind string
	// Target is the channel it was going to.
	Target string
	// Payload is what the handler decodes.
	Payload json.RawMessage
	// Attempts counts the failed sends that were not the network.
	Attempts int
	// CreatedAt is when it was first meant to be sent.
	CreatedAt time.Time
}

// Late reports how long the message waited, and whether that is long enough to say so.
func (i Item) Late(now time.Time) (time.Duration, bool) {
	waited := now.Sub(i.CreatedAt)
	return waited, waited >= lateAfter
}

// Handler sends one saved message. It answers a network error unchanged, so the outbox knows to wait
// for the internet, and any other error means the service refused it.
type Handler func(ctx context.Context, item Item) error

// Options sets up the outbox. Every field is optional.
type Options struct {
	// Network is told of a network failure and says when the internet is back. Nil means it never is
	// told and a flush runs on its timer only.
	Network *netwatch.Monitor
	// Now is the clock. The default is time.Now.
	Now func() time.Time
	// Entropy makes ids. The default is crypto/rand.
	Entropy io.Reader
	// Logger receives one line for each message saved, sent late, or dropped.
	Logger *slog.Logger
}

// Service is the outbox. It is safe for concurrent use.
type Service struct {
	store   *store.Store
	net     *netwatch.Monitor
	now     func() time.Time
	entropy io.Reader
	log     *slog.Logger

	flushMu  sync.Mutex
	mu       sync.Mutex
	handlers map[string]Handler
}

// New makes an outbox over the store.
func New(st *store.Store, opts Options) *Service {
	s := &Service{
		store: st, net: opts.Network, now: opts.Now, entropy: opts.Entropy, log: opts.Logger,
		handlers: map[string]Handler{},
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.entropy == nil {
		s.entropy = rand.Reader
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	return s
}

// Register says how a kind of message is sent. A message of a kind with no handler is dropped when
// its turn comes.
func (s *Service) Register(kind string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[kind] = h
}

// Enqueue saves a message to send later. The payload is whatever the kind's handler decodes. When the
// outbox is full the oldest messages are dropped to make room.
func (s *Service) Enqueue(ctx context.Context, kind, target string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("save a message to send later: %w", err)
	}
	id, err := protocol.NewID(s.now(), s.entropy)
	if err != nil {
		return fmt.Errorf("make an id for a message to send later: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		if err := q.InsertOutbox(ctx, db.InsertOutboxParams{
			ID: id, Kind: kind, Target: target, PayloadJSON: string(raw), CreatedAt: s.now().UnixMilli(),
		}); err != nil {
			return err
		}
		return q.TrimOutbox(ctx, MaxQueued)
	})
	if err != nil {
		return fmt.Errorf("save a message to send later: %w", err)
	}
	s.log.Info("saved a message to send when the internet is back", "kind", kind, "target", target)
	return nil
}

// Pending is how many messages are waiting.
func (s *Service) Pending(ctx context.Context) (int, error) {
	n, err := s.store.Queries().CountOutbox(ctx)
	if err != nil {
		return 0, fmt.Errorf("count the messages waiting to be sent: %w", err)
	}
	return int(n), nil
}

// Result is what one flush did.
type Result struct {
	Sent, Dropped, Left int
}

// Flush sends what is waiting, oldest first. It stops at the first failure that is the internet being
// down and leaves the rest, and it drops a message that is too old or that a service keeps refusing.
// Each message is tried at most once per flush. Two flushes never run at once.
//
// Delivery is at least once: a message can arrive twice when a service accepted it but the answer was
// lost, or when the daemon stopped between sending it and forgetting it.
func (s *Service) Flush(ctx context.Context) (Result, error) {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	var out Result
	tried := map[string]bool{}
	for {
		rows, err := s.store.Queries().ListOutbox(ctx, batch)
		if err != nil {
			return out, fmt.Errorf("read the messages waiting to be sent: %w", err)
		}
		if len(rows) == 0 {
			return out, nil
		}
		progressed := false
		for _, row := range rows {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			if tried[row.ID] {
				continue
			}
			tried[row.ID] = true
			done, stop, err := s.deliver(ctx, row)
			if err != nil {
				return out, err
			}
			switch done {
			case sentOne:
				out.Sent++
				progressed = true
			case droppedOne:
				out.Dropped++
				progressed = true
			}
			if stop {
				n, _ := s.Pending(ctx)
				out.Left = n
				return out, nil
			}
		}
		if !progressed || len(rows) < batch || len(tried) >= MaxQueued*2 {
			n, _ := s.Pending(ctx)
			out.Left = n
			return out, nil
		}
	}
}

type outcome int

const (
	keptOne outcome = iota
	sentOne
	droppedOne
)

// deliver handles one row. It answers what happened to it, and whether the flush should stop because
// the network failed.
func (s *Service) deliver(ctx context.Context, row db.Outbox) (outcome, bool, error) {
	item := Item{
		ID: row.ID, Kind: row.Kind, Target: row.Target, Payload: json.RawMessage(row.PayloadJSON),
		Attempts: int(row.Attempts), CreatedAt: time.UnixMilli(row.CreatedAt),
	}
	if s.now().Sub(item.CreatedAt) > MaxAge {
		s.log.Warn("dropped a message that waited more than a day", "kind", item.Kind, "target", item.Target)
		return droppedOne, false, s.remove(ctx, item.ID)
	}
	s.mu.Lock()
	handler := s.handlers[item.Kind]
	s.mu.Unlock()
	if handler == nil {
		s.log.Warn("dropped a saved message of a kind nothing sends", "kind", item.Kind)
		return droppedOne, false, s.remove(ctx, item.ID)
	}
	err := handler(ctx, item)
	switch {
	case err == nil:
		if waited, late := item.Late(s.now()); late {
			s.log.Info("sent a message that was waiting for the internet", "kind", item.Kind, "target", item.Target, "waited", waited.Round(time.Second))
		}
		// A send that worked proves the internet is there.
		s.net.MarkOnline()
		// The message is gone from the outbox even if the daemon is shutting down, or it would be sent
		// again at the next start.
		return sentOne, false, s.remove(context.WithoutCancel(ctx), item.ID)
	case netwatch.IsNetworkError(err) && !s.reachable(ctx):
		// The internet is down: the rest would fail the same way, so the flush stops here and the
		// monitor's return starts it again.
		return keptOne, true, nil
	default:
		if item.Attempts+1 >= MaxAttempts {
			s.log.Warn("dropped a message a service kept refusing", "kind", item.Kind, "target", item.Target, "err", err)
			return droppedOne, false, s.remove(ctx, item.ID)
		}
		failErr := s.store.Write(ctx, func(q *db.Queries) error {
			return q.FailOutbox(ctx, db.FailOutboxParams{LastError: truncate(err.Error()), ID: item.ID})
		})
		return keptOne, false, failErr
	}
}

// reachable asks the monitor whether the internet is there. With no monitor a network-looking failure
// is taken at its word, so nothing is reachable.
func (s *Service) reachable(ctx context.Context) bool {
	return s.net != nil && s.net.Check(ctx)
}

func (s *Service) remove(ctx context.Context, id string) error {
	err := s.store.Write(ctx, func(q *db.Queries) error { return q.DeleteOutbox(ctx, id) })
	if err != nil {
		return fmt.Errorf("take a sent message off the outbox: %w", err)
	}
	return nil
}

// truncate keeps the last error short enough for a row.
func truncate(text string) string {
	if r := []rune(text); len(r) > 300 {
		return string(r[:300])
	}
	return text
}

// Run flushes the outbox when the daemon starts, each time the internet comes back, and every few
// minutes after, until ctx ends. It is meant to run on its own goroutine.
func (s *Service) Run(ctx context.Context) {
	back := make(chan struct{}, 1)
	s.net.Subscribe(func(status netwatch.Status) {
		if status.Online {
			select {
			case back <- struct{}{}:
			default:
			}
		}
	})
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()
	s.flushQuietly(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-back:
		case <-ticker.C:
		}
		s.flushQuietly(ctx)
	}
}

func (s *Service) flushQuietly(ctx context.Context) {
	result, err := s.Flush(ctx)
	if err != nil && ctx.Err() == nil {
		s.log.Warn("could not send the messages waiting for the internet", "err", err)
		return
	}
	if result.Sent > 0 || result.Dropped > 0 {
		s.log.Info("flushed the outbox", "sent", result.Sent, "dropped", result.Dropped, "left", result.Left)
	}
}
