package providers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/security"
)

// testNow is the instant a test's receipts are stamped with, so an assertion can name the exact time
// rather than accepting whatever the clock said.
var testNow = time.Date(2026, time.September, 27, 14, 30, 0, 0, time.UTC)

// The synthetic keys a test stores. None is a real key and none is used for anything: a stored key
// only has to pass the shape check to reach a provider's client, and every client here is a stub.
const (
	anthropicTestKey = "sk-ant-api03-not-real-4f2a"
	openAITestKey    = "sk-proj-not-real-6b7c"
)

// errBurst is what a script client answers with when it is handed more calls at once than a real
// provider would allow on one key: a rate limit, which is the thing the queue exists to avoid.
var errBurst = fmt.Errorf("%w: too many calls at once for one key", ErrRateLimited)

// scriptClient is a Client a test scripts. It reaches nothing and knows no provider; it records
// every request it was given, answers with the reply and the failures the test set, and can be told
// to refuse a burst the way a provider refuses one, so a test can prove the queue holds the line.
type scriptClient struct {
	id string

	mu       sync.Mutex
	requests []Request
	peak     int
	inFlight int
	refused  int

	// limit is this provider's own idea of "too many at once": above it a call is refused with
	// errBurst rather than answered. Zero means this provider never complains.
	limit int
	// ready, when set, holds every call until the test closes it, so a test can pile calls up inside
	// the client and watch how many the queue really let through at once.
	ready chan struct{}

	// answer is what a call answers with once the scripted failures and replies run out.
	answer Reply
	// always, when set, is the failure every call answers with; it is how a test makes a provider
	// stay broken across every retry.
	always error
	// errs and replies are answered one per call, in order, before answer is used.
	errs    []error
	replies []Reply

	// streamErr is what Stream answers with as long as it is set, so a test can make a provider's
	// stream fail at its very start on every attempt.
	streamErr error
	// streamEvents is what a stream that starts answers with, in order.
	streamEvents []Event
	// streamFailAfter, when set, is what the stream answers once its events have been read: a stream
	// that failed after it had already delivered something.
	streamFailAfter error
}

func (c *scriptClient) ID() string { return c.id }

func (c *scriptClient) Complete(ctx context.Context, req Request) (Reply, error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	c.inFlight++
	if c.inFlight > c.peak {
		c.peak = c.inFlight
	}
	over := c.limit > 0 && c.inFlight > c.limit
	if over {
		c.refused++
	}
	ready := c.ready
	c.mu.Unlock()

	if ready != nil {
		select {
		case <-ready:
		case <-ctx.Done():
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.inFlight--
	if over {
		return Reply{}, errBurst
	}
	return c.next()
}

func (c *scriptClient) Stream(_ context.Context, req Request) (Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	if c.streamErr != nil {
		return nil, c.streamErr
	}
	return &scriptStream{
		events: append([]Event(nil), c.streamEvents...),
		after:  c.streamFailAfter,
	}, nil
}

// next picks this call's answer. The caller holds the lock.
func (c *scriptClient) next() (Reply, error) {
	if len(c.errs) > 0 {
		err := c.errs[0]
		c.errs = c.errs[1:]
		if err != nil {
			return Reply{}, err
		}
	}
	if len(c.replies) > 0 {
		reply := c.replies[0]
		c.replies = c.replies[1:]
		return reply, nil
	}
	if c.always != nil {
		return Reply{}, c.always
	}
	return c.answer, nil
}

// scriptStream is a stream a test scripts: its events, then either a failure or the end.
type scriptStream struct {
	events []Event
	after  error
	at     int
}

func (s *scriptStream) Recv() (Event, error) {
	if s.at < len(s.events) {
		ev := s.events[s.at]
		s.at++
		return ev, nil
	}
	if s.after != nil {
		return Event{}, s.after
	}
	return Event{}, io.EOF
}

func (s *scriptStream) Close() error { return nil }

// fakeFactory builds a script client per provider id and keeps it, so a test can script one provider
// and read what it was sent. A seeded client exists before its provider is resolved, which is how a
// test scripts the backup of a fallback before the call that needs it.
type fakeFactory struct {
	mu      sync.Mutex
	clients map[string]*scriptClient
}

func newFakeFactory() *fakeFactory {
	return &fakeFactory{clients: map[string]*scriptClient{}}
}

func (f *fakeFactory) build(info Info, _ string) (Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.clients[info.ID]; ok {
		return c, nil
	}
	c := &scriptClient{id: info.ID}
	f.clients[info.ID] = c
	return c, nil
}

// seed registers a script client for a provider before anything resolves it.
func (f *fakeFactory) seed(id string) *scriptClient {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := &scriptClient{id: id}
	f.clients[id] = c
	return c
}

// get returns the client a provider was given, failing the test when the provider was never built.
func (f *fakeFactory) get(t *testing.T, id string) *scriptClient {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.clients[id]
	if !ok {
		t.Fatalf("%s has no client: the test must save its key and resolve a model it serves first", id)
	}
	return c
}

// requestsOf returns what a provider's client was sent. The caller reads it after the calls are
// over, so taking the lock here is enough.
func (c *scriptClient) requestsOf() []Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Request(nil), c.requests...)
}

// stats returns the concurrency numbers a queue test asserts on.
func (c *scriptClient) stats() (peak, refused, inFlight int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peak, c.refused, c.inFlight
}

// fakeRecorder collects the receipts a call would have filed, so a test can read what the daemon
// would have written without a database.
type fakeRecorder struct {
	mu      sync.Mutex
	records []UsageRecord
	err     error
}

func (r *fakeRecorder) Record(_ context.Context, rec UsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec)
	return r.err
}

func (r *fakeRecorder) all() []UsageRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]UsageRecord(nil), r.records...)
}

// managedService returns a service over an in-memory keychain and script clients, with the waiting
// and the clock stilled unless the test asked for its own. Nothing it builds can reach a provider.
func managedService(t *testing.T, opts Options) (*Service, *fakeFactory) {
	t.Helper()
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.Sleep == nil {
		opts.Sleep = func(context.Context, time.Duration) error { return nil }
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return testNow }
	}
	f := newFakeFactory()
	s, err := New(security.NewMemoryKeychain(), opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.build = f.build
	return s, f
}

// save stores a synthetic key for a provider, failing the test when the service refuses it.
func save(t *testing.T, s *Service, id, secret string) {
	t.Helper()
	if err := s.Save(id, secret); err != nil {
		t.Fatalf("Save %s: %v", id, err)
	}
}

// waitForInFlight waits until a client is holding want calls at once, so a test can act once a burst
// has filled the queue. It fails the test rather than waiting forever.
func waitForInFlight(t *testing.T, c *scriptClient, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, inFlight := c.stats(); inFlight >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	_, _, inFlight := c.stats()
	t.Fatalf("%d calls reached the provider, want %d: the queue is letting too few through", inFlight, want)
}

// replyText is what a reply says, for an assertion about which model answered.
func replyText(reply Reply) string {
	var b strings.Builder
	for _, part := range reply.Parts {
		if part.Kind == PartText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}
