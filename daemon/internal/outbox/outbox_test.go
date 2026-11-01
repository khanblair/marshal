package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/netwatch"
	"github.com/khanblair/marshal/daemon/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newBox(t *testing.T) (*Service, *clock) {
	t.Helper()
	return newBoxWith(t, nil)
}

// newBoxWith makes an outbox that asks the given monitor whether the internet is reachable.
func newBoxWith(t *testing.T, network *netwatch.Monitor) (*Service, *clock) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	c := &clock{t: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}
	return New(st, Options{Now: c.now, Logger: quiet(), Network: network}), c
}

// monitorWith is a monitor whose check answers what the test sets: the internet is reachable or not.
func monitorWith(t *testing.T, reachable *atomic.Bool) *netwatch.Monitor {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return netwatch.New(ctx, netwatch.Options{
		Probe:  func(context.Context) bool { return reachable.Load() },
		Sleep:  func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
		Logger: quiet(),
	})
}

// sink records what a handler was given, in order, and answers each call from a list of errors.
type sink struct {
	mu   sync.Mutex
	got  []string
	errs []error
}

func (s *sink) handler(_ context.Context, item Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var text string
	_ = json.Unmarshal(item.Payload, &text)
	s.got = append(s.got, text)
	if len(s.errs) == 0 {
		return nil
	}
	err := s.errs[0]
	s.errs = s.errs[1:]
	return err
}

func (s *sink) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.got...)
}

var errNet = &net.OpError{Op: "dial", Err: errors.New("network is unreachable")}

func TestAFlushSendsWhatWasSavedOldestFirstAndEmptiesTheOutbox(t *testing.T) {
	box, c := newBox(t)
	ctx := context.Background()
	s := &sink{}
	box.Register(KindNotice, s.handler)
	for _, text := range []string{"first", "second", "third"} {
		if err := box.Enqueue(ctx, KindNotice, "ntfy", text); err != nil {
			t.Fatal(err)
		}
		c.advance(time.Second)
	}
	if n, _ := box.Pending(ctx); n != 3 {
		t.Fatalf("pending = %d, want 3", n)
	}
	result, err := box.Flush(ctx)
	if err != nil || result.Sent != 3 || result.Left != 0 {
		t.Fatalf("flush = %+v, %v; want 3 sent and none left", result, err)
	}
	if fmt.Sprint(s.sent()) != "[first second third]" {
		t.Errorf("sent in order %v, want oldest first", s.sent())
	}
}

func TestAFlushStopsAtTheFirstNetworkFailureAndKeepsTheRest(t *testing.T) {
	box, c := newBox(t)
	ctx := context.Background()
	s := &sink{errs: []error{nil, fmt.Errorf("send: %w", errNet)}}
	box.Register(KindNotice, s.handler)
	for _, text := range []string{"a", "b", "c"} {
		_ = box.Enqueue(ctx, KindNotice, "ntfy", text)
		c.advance(time.Second)
	}
	result, err := box.Flush(ctx)
	if err != nil || result.Sent != 1 || result.Left != 2 {
		t.Fatalf("flush = %+v, %v; want 1 sent and 2 left", result, err)
	}
	if got := s.sent(); fmt.Sprint(got) != "[a b]" {
		t.Errorf("the handler saw %v, want it to stop after the network failure on b", got)
	}
	// The internet is back: the rest go out, and the one that failed is sent again, in order.
	result, _ = box.Flush(ctx)
	if result.Sent != 2 || result.Left != 0 || fmt.Sprint(s.sent()) != "[a b b c]" {
		t.Errorf("second flush = %+v with %v, want b and c sent", result, s.sent())
	}
}

// A network-looking failure with the internet reachable is one server being down, not the outage. It
// counts against that message and the others still go out, instead of the whole outbox waiting on it.
func TestAServerThatIsDownWhileTheInternetIsFineDoesNotBlockTheOtherMessages(t *testing.T) {
	var up atomic.Bool
	up.Store(true)
	box, c := newBoxWith(t, monitorWith(t, &up))
	ctx := context.Background()
	s := &sink{errs: []error{fmt.Errorf("send: %w", errNet)}}
	box.Register(KindNotice, s.handler)
	for _, text := range []string{"stuck", "fine"} {
		_ = box.Enqueue(ctx, KindNotice, "ntfy", text)
		c.advance(time.Second)
	}
	result, err := box.Flush(ctx)
	if err != nil || result.Sent != 1 || result.Left != 1 {
		t.Fatalf("flush = %+v, %v; want the second sent and the first kept", result, err)
	}
	if fmt.Sprint(s.sent()) != "[stuck fine]" {
		t.Errorf("handler saw %v, want both tried once", s.sent())
	}
}

// When the internet really is down, the flush stops, and the same message goes first once it is back.
func TestAFlushStopsWhileTheInternetIsDownAndSendsInOrderWhenItIsBack(t *testing.T) {
	var up atomic.Bool
	box, c := newBoxWith(t, monitorWith(t, &up))
	ctx := context.Background()
	s := &sink{errs: []error{fmt.Errorf("send: %w", errNet)}}
	box.Register(KindNotice, s.handler)
	for _, text := range []string{"first", "second"} {
		_ = box.Enqueue(ctx, KindNotice, "ntfy", text)
		c.advance(time.Second)
	}
	result, _ := box.Flush(ctx)
	if result.Sent != 0 || result.Left != 2 || fmt.Sprint(s.sent()) != "[first]" {
		t.Fatalf("flush while down = %+v with %v, want it to stop at the first", result, s.sent())
	}
	up.Store(true)
	result, _ = box.Flush(ctx)
	if result.Sent != 2 || result.Left != 0 || fmt.Sprint(s.sent()) != "[first first second]" {
		t.Errorf("flush when back = %+v with %v, want both sent in order", result, s.sent())
	}
}

// A message is tried once per flush, even with more waiting than one batch holds and some kept.
func TestEachMessageIsTriedOnceInOneFlushEvenPastABatch(t *testing.T) {
	box, c := newBox(t)
	ctx := context.Background()
	calls := map[string]int{}
	var mu sync.Mutex
	box.Register(KindNotice, func(_ context.Context, item Item) error {
		mu.Lock()
		defer mu.Unlock()
		calls[item.ID]++
		if calls[item.ID] == 1 && item.Attempts == 0 && len(calls)%2 == 0 {
			return errors.New("refused")
		}
		return nil
	})
	for i := range batch + 20 {
		_ = box.Enqueue(ctx, KindNotice, "ntfy", fmt.Sprint(i))
		c.advance(time.Millisecond)
	}
	if _, err := box.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	for id, n := range calls {
		if n != 1 {
			t.Errorf("message %s was tried %d times in one flush, want once", id, n)
		}
	}
}

func TestAMessageOlderThanADayIsDroppedNotSent(t *testing.T) {
	box, c := newBox(t)
	ctx := context.Background()
	s := &sink{}
	box.Register(KindNotice, s.handler)
	_ = box.Enqueue(ctx, KindNotice, "ntfy", "old")
	c.advance(MaxAge + time.Minute)
	_ = box.Enqueue(ctx, KindNotice, "ntfy", "new")
	result, err := box.Flush(ctx)
	if err != nil || result.Dropped != 1 || result.Sent != 1 || fmt.Sprint(s.sent()) != "[new]" {
		t.Errorf("flush = %+v, %v with %v; want the old one dropped and the new one sent", result, err, s.sent())
	}
}

func TestAMessageAServiceKeepsRefusingIsDroppedAfterAFewTries(t *testing.T) {
	box, _ := newBox(t)
	ctx := context.Background()
	refuse := errors.New("the bot token is wrong")
	s := &sink{}
	for range MaxAttempts + 1 {
		s.errs = append(s.errs, refuse)
	}
	box.Register(KindNotice, s.handler)
	_ = box.Enqueue(ctx, KindNotice, "telegram", "x")
	for i := range MaxAttempts {
		result, _ := box.Flush(ctx)
		if i < MaxAttempts-1 && result.Left != 1 {
			t.Fatalf("flush %d left %d, want the message kept", i+1, result.Left)
		}
	}
	if n, _ := box.Pending(ctx); n != 0 {
		t.Errorf("pending = %d after %d refusals, want it dropped", n, MaxAttempts)
	}
}

func TestAMessageOfAKindNothingSendsIsDropped(t *testing.T) {
	box, _ := newBox(t)
	ctx := context.Background()
	_ = box.Enqueue(ctx, KindBrief, "resend", "x")
	result, err := box.Flush(ctx)
	if err != nil || result.Dropped != 1 {
		t.Errorf("flush = %+v, %v; want the message dropped", result, err)
	}
}

func TestTheOutboxHoldsAtMostItsLimitAndDropsTheOldest(t *testing.T) {
	box, c := newBox(t)
	ctx := context.Background()
	for i := range MaxQueued + 5 {
		if err := box.Enqueue(ctx, KindNotice, "ntfy", fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
		c.advance(time.Millisecond)
	}
	if n, _ := box.Pending(ctx); n != MaxQueued {
		t.Errorf("pending = %d, want the limit %d", n, MaxQueued)
	}
	s := &sink{}
	box.Register(KindNotice, s.handler)
	_, _ = box.Flush(ctx)
	if got := s.sent(); got[0] != "5" {
		t.Errorf("the oldest kept message is %q, want 5: the first five were dropped", got[0])
	}
}

func TestAMessageSaysItWasLateOnlyAfterAMinute(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	item := Item{CreatedAt: now}
	if _, late := item.Late(now.Add(30 * time.Second)); late {
		t.Error("a message that waited 30 seconds is called late")
	}
	if waited, late := item.Late(now.Add(5 * time.Minute)); !late || waited != 5*time.Minute {
		t.Errorf("a message that waited 5 minutes: late = %v, waited %v", late, waited)
	}
}

func TestFlushesNeverRunAtOnce(t *testing.T) {
	box, _ := newBox(t)
	ctx := context.Background()
	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	box.Register(KindNotice, func(context.Context, Item) error {
		entered <- struct{}{}
		<-release
		return nil
	})
	_ = box.Enqueue(ctx, KindNotice, "ntfy", "x")
	done := make(chan struct{}, 2)
	for range 2 {
		go func() { _, _ = box.Flush(ctx); done <- struct{}{} }()
	}
	<-entered
	select {
	case <-entered:
		t.Fatal("two flushes sent at once")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-done
	<-done
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
