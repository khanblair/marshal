package providers

import (
	"context"
	"errors"
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

// TestABurstIsHeldToTheProvidersLimit is scope 7.6's promise: ten parallel calls on one key finish
// instead of failing on a rate limit, because the queue lets only so many run at once and the rest
// wait their turn. The script client refuses anything above the same limit, so a queue that let the
// whole burst through would show up as refusals.
func TestABurstIsHeldToTheProvidersLimit(t *testing.T) {
	const parallel, limit = 10, 4
	s, f := managedService(t, Options{MaxConcurrent: limit})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	stub := f.get(t, AnthropicID)
	stub.limit = limit
	stub.answer = Reply{StopReason: StopEndTurn}

	ready := make(chan struct{})
	stub.ready = ready

	errs := make([]error, parallel)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, callErr := resolved.Client.Complete(context.Background(), Request{Model: resolved.Model})
			errs[i] = callErr
		}()
	}
	close(start)
	// Let the queue fill, then release the calls that are inside the provider, so the ones waiting
	// get their turn.
	waitForInFlight(t, stub, limit)
	close(ready)
	wg.Wait()

	for i, callErr := range errs {
		if callErr != nil {
			t.Errorf("parallel call %d failed: %v", i, callErr)
		}
	}
	peak, refused, _ := stub.stats()
	if peak > limit {
		t.Errorf("%d calls were inside the provider at once, want at most %d", peak, limit)
	}
	if refused != 0 {
		t.Errorf("%d calls were refused as a burst, want none: the queue did not hold the line", refused)
	}
	if got := len(stub.requestsOf()); got != parallel {
		t.Errorf("the provider was called %d times for %d calls, want one each", got, parallel)
	}
}

// TestARateLimitedCallIsRetried is the other half of scope 7.6: a request the provider did refuse is
// tried again after a wait, rather than reported as a failure.
func TestARateLimitedCallIsRetried(t *testing.T) {
	var sleeps []time.Duration
	s, f := managedService(t, Options{
		RetryAttempts: 3,
		RetryBackoff:  20 * time.Millisecond,
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		},
	})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	stub := f.get(t, AnthropicID)
	stub.errs = []error{ErrRateLimited}
	stub.answer = Reply{Parts: []Part{{Kind: PartText, Text: "second time lucky"}}, StopReason: StopEndTurn}

	reply, err := resolved.Client.Complete(context.Background(), Request{Model: resolved.Model})
	if err != nil {
		t.Fatalf("a rate-limited call was not retried: %v", err)
	}
	if got := replyText(reply); got != "second time lucky" {
		t.Errorf("the retried call answered %q, want the second attempt's answer", got)
	}
	if got := len(stub.requestsOf()); got != 2 {
		t.Errorf("the provider was called %d times, want one refusal and one retry", got)
	}
	if len(sleeps) != 1 || sleeps[0] != 20*time.Millisecond {
		t.Errorf("the wait between tries was %v, want the configured 20ms once", sleeps)
	}
}

// TestARefusedKeyIsNotRetried is the other side of the policy: a key the provider rejected will be
// rejected again, so the call is answered at once rather than after four pointless tries.
func TestARefusedKeyIsNotRetried(t *testing.T) {
	s, f := managedService(t, Options{RetryAttempts: 4})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	stub := f.get(t, AnthropicID)
	stub.always = ErrAuth

	if _, err := resolved.Client.Complete(context.Background(), Request{Model: resolved.Model}); !errors.Is(err, ErrAuth) {
		t.Errorf("Complete answered %v, want the key refusal it was given", err)
	}
	if got := len(stub.requestsOf()); got != 1 {
		t.Errorf("the provider was called %d times, want one: a refused key is not retried", got)
	}
}

// TestAnAnsweredCallFilesItsReceipt is B4.4: the call records which provider and model answered, what
// it read and wrote, what that cost, and the card, project, and role it ran for.
func TestAnAnsweredCallFilesItsReceipt(t *testing.T) {
	rec := &fakeRecorder{}
	s, f := managedService(t, Options{Recorder: rec})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.get(t, AnthropicID).answer = Reply{
		Parts:      []Part{{Kind: PartText, Text: "done"}},
		StopReason: StopEndTurn,
		Usage:      Usage{InputTokens: 1_000_000, OutputTokens: 2_000},
	}
	ctx := WithCall(context.Background(), Call{
		SessionID: "01J8Z000000000000000000SES",
		CardID:    "01J8Z00000000000000000000CRD",
		ProjectID: "01J8Z00000000000000000000PRJ",
		RoleID:    "reviewer",
	})
	if _, err := resolved.Client.Complete(ctx, Request{Model: resolved.Model}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	all := rec.all()
	if len(all) != 1 {
		t.Fatalf("filed %d receipts, want one per call", len(all))
	}
	got := all[0]
	want := UsageRecord{
		CardID: "01J8Z00000000000000000000CRD", ProjectID: "01J8Z00000000000000000000PRJ",
		RoleID: "reviewer", Provider: AnthropicID, Model: "claude-sonnet-4-5",
		InputTokens: 1_000_000, OutputTokens: 2_000,
		// 1000000*3 + 2000*15 micro-dollars per million, which is exactly 3_030_000.
		CostMicros: 3_030_000,
		At:         testNow,
	}
	if got != want {
		t.Errorf("the receipt was\n  %+v\nwant\n  %+v", got, want)
	}
}

// TestACallWithNoCardFilesAnEmptyOne is the chat and connection-test case: the fields a call does
// not have are left empty rather than guessed at.
func TestACallWithNoCardFilesAnEmptyOne(t *testing.T) {
	rec := &fakeRecorder{}
	s, f := managedService(t, Options{Recorder: rec})
	save(t, s, OpenAIID, openAITestKey)
	resolved, err := s.Resolve("gpt-5-mini")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.get(t, OpenAIID).answer = Reply{StopReason: StopEndTurn, Usage: Usage{InputTokens: 10, OutputTokens: 10}}
	if _, err := resolved.Client.Complete(context.Background(), Request{Model: resolved.Model}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := rec.all()
	if len(got) != 1 {
		t.Fatalf("filed %d receipts, want one", len(got))
	}
	if got[0].CardID != "" || got[0].ProjectID != "" || got[0].RoleID != "" {
		t.Errorf("a call with no card filed %+v, want the card fields empty", got[0])
	}
}

// TestAnUnpricedModelIsRecordedAtZeroCostAndSaidSo is the honesty rule: a model Marshal cannot price
// still gets its row - the tokens are the record - but at zero cost, with a line saying so, rather
// than a number nobody computed.
func TestAnUnpricedModelIsRecordedAtZeroCostAndSaidSo(t *testing.T) {
	rec := &fakeRecorder{}
	var logs strings.Builder
	s, f := managedService(t, Options{
		Recorder: rec,
		Logger:   slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})
	save(t, s, LMStudioID, testLocalAddress)
	resolved, err := s.Resolve("some-model-nobody-priced-7b")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.get(t, LMStudioID).answer = Reply{StopReason: StopEndTurn, Usage: Usage{InputTokens: 4_000, OutputTokens: 500}}
	if _, err := resolved.Client.Complete(context.Background(), Request{Model: resolved.Model}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got := rec.all()
	if len(got) != 1 {
		t.Fatalf("filed %d receipts, want one", len(got))
	}
	if got[0].CostMicros != 0 {
		t.Errorf("an unpriced model was billed %d micro-dollars, want zero", got[0].CostMicros)
	}
	if got[0].InputTokens != 4_000 || got[0].OutputTokens != 500 {
		t.Errorf("the tokens were dropped: %+v", got[0])
	}
	if !strings.Contains(logs.String(), "no price") {
		t.Errorf("nothing said the model was unpriced; the log was:\n%s", logs.String())
	}
}

// TestAReceiptThatCannotBeStoredDoesNotFailTheCall is the rule that bookkeeping is never the work: a
// call that answered did its job, so a store failure is a line in the log and the answer still
// reaches the caller.
func TestAReceiptThatCannotBeStoredDoesNotFailTheCall(t *testing.T) {
	rec := &fakeRecorder{err: errors.New("the database is busy")}
	s, f := managedService(t, Options{Recorder: rec})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.get(t, AnthropicID).answer = Reply{Parts: []Part{{Kind: PartText, Text: "still here"}}, StopReason: StopEndTurn}

	reply, err := resolved.Client.Complete(context.Background(), Request{Model: resolved.Model})
	if err != nil {
		t.Fatalf("a storage failure was reported as a call failure: %v", err)
	}
	if got := replyText(reply); got != "still here" {
		t.Errorf("the answer was lost: %q", got)
	}
}

// TestAProviderThatCannotBeReachedFallsBackToTheBackup is scope 7.5: an outage on the model that was
// asked for switches the call to the backup model, tells the person, and files the receipt against
// the model that really answered.
func TestAProviderThatCannotBeReachedFallsBackToTheBackup(t *testing.T) {
	rec := &fakeRecorder{}
	var notices []string
	s, f := managedService(t, Options{
		Recorder: rec,
		Notices:  func(_ context.Context, text string) { notices = append(notices, text) },
	})
	save(t, s, AnthropicID, anthropicTestKey)
	save(t, s, OpenAIID, openAITestKey)
	backup := f.seed(OpenAIID)
	backup.answer = Reply{Parts: []Part{{Kind: PartText, Text: "from the backup"}}, StopReason: StopEndTurn,
		Usage: Usage{InputTokens: 100, OutputTokens: 10}}

	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	primary := f.get(t, AnthropicID)
	primary.always = ErrUnavailable

	ctx := WithCall(context.Background(), Call{Backup: "gpt-5-mini"})
	reply, err := resolved.Client.Complete(ctx, Request{Model: resolved.Model})
	if err != nil {
		t.Fatalf("the backup was not tried: %v", err)
	}
	if got := replyText(reply); got != "from the backup" {
		t.Errorf("the call answered %q, want the backup's answer", got)
	}
	if got := len(primary.requestsOf()); got != defaultAttempts {
		t.Errorf("the failed provider was called %d times, want %d tries before the backup",
			got, defaultAttempts)
	}
	if len(notices) != 1 {
		t.Fatalf("told the person %d times, want one notice", len(notices))
	}
	if !strings.Contains(notices[0], "gpt-5-mini") || !strings.Contains(notices[0], "claude-sonnet-4-5") {
		t.Errorf("the notice did not name both models: %q", notices[0])
	}
	all := rec.all()
	if len(all) != 1 {
		t.Fatalf("filed %d receipts, want only the call that answered", len(all))
	}
	if all[0].Provider != OpenAIID || all[0].Model != "gpt-5-mini" {
		t.Errorf("the receipt blamed %s/%s, want the model that really answered", all[0].Provider, all[0].Model)
	}
}

// TestAFallbackIsNotTriedTwice means one backup is a backup, not a loop: when the backup is broken
// too, the caller hears about the model it actually asked for.
func TestAFallbackIsNotTriedTwice(t *testing.T) {
	s, f := managedService(t, Options{RetryAttempts: 1})
	save(t, s, AnthropicID, anthropicTestKey)
	save(t, s, OpenAIID, openAITestKey)
	backup := f.seed(OpenAIID)
	backup.always = ErrUnavailable

	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.get(t, AnthropicID).always = ErrUnavailable

	ctx := WithCall(context.Background(), Call{Backup: "gpt-5-mini"})
	if _, err := resolved.Client.Complete(ctx, Request{Model: resolved.Model}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("both models being down answered %v, want the outage", err)
	}
	if got := len(backup.requestsOf()); got != 1 {
		t.Errorf("the backup was called %d times, want once: a backup has no backup", got)
	}
}

// TestNoBackupReportsTheOutageAsItIs is the plain case: a caller that named no backup gets the
// provider's own failure rather than a quiet switch to a model it did not ask for.
func TestNoBackupReportsTheOutageAsItIs(t *testing.T) {
	s, f := managedService(t, Options{RetryAttempts: 1})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.get(t, AnthropicID).always = ErrUnavailable

	if _, err := resolved.Client.Complete(context.Background(), Request{Model: resolved.Model}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("a call with no backup answered %v, want the outage", err)
	}
}

// TestABackupThatIsTheSameModelIsNotTried means a card whose backup equals its model does not switch
// to the model that just failed.
func TestABackupThatIsTheSameModelIsNotTried(t *testing.T) {
	s, f := managedService(t, Options{RetryAttempts: 1})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	stub := f.get(t, AnthropicID)
	stub.always = ErrUnavailable

	ctx := WithCall(context.Background(), Call{Backup: "claude-sonnet-4-5"})
	if _, err := resolved.Client.Complete(ctx, Request{Model: resolved.Model}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("answering %v, want the outage", err)
	}
	if got := len(stub.requestsOf()); got != 1 {
		t.Errorf("the provider was called %d times, want one: the backup was the model that failed", got)
	}
}

// TestAStreamThatFailsToStartFallsBackToTheBackup is scope 7.5 for the path the built-in agent
// actually uses: a stream that cannot be opened at all starts again on the backup model, and the
// receipt names the model that delivered the answer.
func TestAStreamThatFailsToStartFallsBackToTheBackup(t *testing.T) {
	rec := &fakeRecorder{}
	var notices []string
	s, f := managedService(t, Options{
		RetryAttempts: 1,
		Recorder:      rec,
		Notices:       func(_ context.Context, text string) { notices = append(notices, text) },
	})
	save(t, s, AnthropicID, anthropicTestKey)
	save(t, s, OpenAIID, openAITestKey)
	backup := f.seed(OpenAIID)
	backup.streamEvents = []Event{
		{Kind: EventText, Text: "from the backup"},
		{Kind: EventDone, StopReason: StopEndTurn, Usage: Usage{InputTokens: 50, OutputTokens: 5}},
	}

	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.get(t, AnthropicID).streamErr = ErrUnavailable

	ctx := WithCall(context.Background(), Call{Backup: "gpt-5-mini"})
	stream, err := resolved.Client.Stream(ctx, Request{Model: resolved.Model})
	if err != nil {
		t.Fatalf("the stream did not fall back: %v", err)
	}
	defer func() { _ = stream.Close() }()

	var said strings.Builder
	for {
		ev, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			t.Fatalf("Recv: %v", recvErr)
		}
		if ev.Kind == EventText {
			said.WriteString(ev.Text)
		}
	}
	if got := said.String(); got != "from the backup" {
		t.Errorf("the stream said %q, want the backup's answer", got)
	}
	if len(notices) != 1 {
		t.Errorf("told the person %d times, want one notice", len(notices))
	}
	all := rec.all()
	if len(all) != 1 || all[0].Provider != OpenAIID || all[0].Model != "gpt-5-mini" {
		t.Errorf("the stream receipt is %+v, want the model that really answered", all)
	}
}

// TestAStreamThatFailedAfterDeliveringDoesNotSwitch is the limit of a fallback: an answer that has
// already been shown to the caller cannot be restarted somewhere else without repeating itself, so
// the failure is reported as it is.
func TestAStreamThatFailedAfterDeliveringDoesNotSwitch(t *testing.T) {
	s, f := managedService(t, Options{RetryAttempts: 1})
	save(t, s, AnthropicID, anthropicTestKey)
	save(t, s, OpenAIID, openAITestKey)
	backup := f.seed(OpenAIID)
	backup.streamEvents = []Event{{Kind: EventDone, StopReason: StopEndTurn}}

	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	primary := f.get(t, AnthropicID)
	primary.streamEvents = []Event{{Kind: EventText, Text: "half an answer"}}
	primary.streamFailAfter = ErrUnavailable

	ctx := WithCall(context.Background(), Call{Backup: "gpt-5-mini"})
	stream, err := resolved.Client.Stream(ctx, Request{Model: resolved.Model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = stream.Close() }()

	if ev, err := stream.Recv(); err != nil || ev.Text != "half an answer" {
		t.Fatalf("the first event was %+v (%v), want the text the provider delivered", ev, err)
	}
	if _, err := stream.Recv(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("the failure after delivery answered %v, want the outage", err)
	}
	if got := len(backup.requestsOf()); got != 0 {
		t.Errorf("the backup was called %d times after an answer had been delivered, want none", got)
	}
}

// TestAStreamWithNoBackupReportsTheStartFailure is the same plain case for streams.
func TestAStreamWithNoBackupReportsTheStartFailure(t *testing.T) {
	s, f := managedService(t, Options{RetryAttempts: 1})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f.get(t, AnthropicID).streamErr = ErrUnavailable

	if _, err := resolved.Client.Stream(context.Background(), Request{Model: resolved.Model}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("a stream with no backup answered %v, want the outage", err)
	}
}

// TestACallWaitingForATurnStopsWhenItsContextIsDone is the cancellation rule for the queue: a call
// that is only waiting its turn gives up the moment its context is cancelled, rather than holding
// the goroutine until the provider frees up.
func TestACallWaitingForATurnStopsWhenItsContextIsDone(t *testing.T) {
	s, f := managedService(t, Options{MaxConcurrent: 1})
	save(t, s, AnthropicID, anthropicTestKey)
	resolved, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	stub := f.get(t, AnthropicID)
	stub.answer = Reply{StopReason: StopEndTurn}
	ready := make(chan struct{})
	stub.ready = ready

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// The first call takes the queue's only slot and waits inside the provider.
		_, _ = resolved.Client.Complete(context.Background(), Request{Model: resolved.Model})
	}()
	waitForInFlight(t, stub, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = resolved.Client.Complete(ctx, Request{Model: resolved.Model})
	close(ready)
	wg.Wait()

	if !errors.Is(err, context.Canceled) {
		t.Errorf("a call cancelled while waiting answered %v, want context.Canceled", err)
	}
}

// TestABackoffGrowsBetweenTries pins the wait: the base doubles each try, so a provider having a bad
// minute is not hammered, and it never grows past the cap.
func TestABackoffGrowsBetweenTries(t *testing.T) {
	cases := []struct {
		try  int
		want time.Duration
	}{
		{1, 250 * time.Millisecond},
		{2, 500 * time.Millisecond},
		{3, time.Second},
		{4, 2 * time.Second},
	}
	for _, tc := range cases {
		if got := backoffFor(250*time.Millisecond, tc.try); got != tc.want {
			t.Errorf("the wait before try %d is %v, want %v", tc.try, got, tc.want)
		}
	}
	if got := backoffFor(time.Second, 20); got != maxBackoff {
		t.Errorf("the wait before a late try is %v, want it capped at %v", got, maxBackoff)
	}
	if got := backoffFor(0, 3); got != 0 {
		t.Errorf("a zero base waited %v, want no wait", got)
	}
}

// TestTheRealSleepHonoursTheContext is the default when a caller sets no Sleep: it waits the time it
// was given, gives up at once when the call is cancelled, and does not wait at all for a zero wait.
func TestTheRealSleepHonoursTheContext(t *testing.T) {
	ctx := context.Background()
	if err := sleepFor(ctx, 0); err != nil {
		t.Errorf("a zero wait answered %v, want no wait and no error", err)
	}
	if err := sleepFor(ctx, -time.Second); err != nil {
		t.Errorf("a negative wait answered %v, want no wait and no error", err)
	}

	start := time.Now()
	if err := sleepFor(ctx, 20*time.Millisecond); err != nil {
		t.Errorf("a short wait answered %v, want it to elapse quietly", err)
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Errorf("a 20ms wait returned after %v, so it did not really wait", elapsed)
	}

	done, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepFor(done, time.Minute); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled wait answered %v, want context.Canceled", err)
	}
}

// TestTheRealSleepStopsWhenTheCallIsCancelled is the same rule while the wait is running, which is
// what a card stopped mid-retry does.
func TestTheRealSleepStopsWhenTheCallIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if err := sleepFor(ctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Errorf("a wait interrupted by cancellation answered %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("the wait ran for %v after cancellation, so it did not stop", elapsed)
	}
}

// TestAQueueNeverDeadlocksOnANonsenseSize means a caller cannot build a queue that refuses every
// request it is given.
func TestAQueueNeverDeadlocksOnANonsenseSize(t *testing.T) {
	q := newQueue(0)
	release, err := q.acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()
	release() // a second release must not take a slot that was never held
	release, err = q.acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	release()
}
