package providers

// This file is the layer the Client interface promises but no adapter implements. An adapter knows
// one provider's protocol; what every call also needs is the per-provider queue (scope 7.6), a
// retry that waits out a rate limit instead of failing (library-docs.md section 2.4), a switch to a
// backup model when a provider is down (scope 7.5), and the usage row that files what the call cost
// (architecture.md section 10). All of that is here, once, so that the built-in agent's own code
// only ever calls Resolved.Client and gets it.
//
// The wrapper is a Client, so Resolve hands it out exactly where a raw adapter would go. Nothing
// else in Marshal may call an adapter directly, or a call would skip the queue, the retries, the
// fallback, and the receipt.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/khanblair/marshal/daemon/internal/netwatch"
)

// Defaults for a Service's call behaviour. They are what a daemon runs with; a test sets Options to
// shrink or still them.
const (
	// defaultAttempts is the most times one request is tried, first try included, when the network is
	// what keeps failing. The waits grow (networkBackoff, up to maxBackoff) and networkBudget bounds the
	// whole retry, so a Wi-Fi switch or a short outage is waited out.
	defaultAttempts = 12
	// defaultOtherAttempts is how many times a request is tried when the provider answers badly but the
	// network is fine: a rate limit or a server error. Four short tries is enough for a burst to clear,
	// and then a backup model, if the call has one, is tried without a long wait.
	defaultOtherAttempts = 4
	// defaultBackoff is the wait before the second try of a rate limit or server error. It doubles.
	defaultBackoff = 250 * time.Millisecond
	// networkBackoff is the wait before the second try when the network is down. It doubles from there.
	networkBackoff = time.Second
	// maxBackoff caps the wait between two tries however long the outage lasts.
	maxBackoff = 30 * time.Second
	// networkBudget is the most time one request spends waiting out a network failure, the wait for the
	// internet to come back included. After it the call fails and says why.
	networkBudget = 5 * time.Minute
	// defaultMaxConcurrent is how many requests one provider runs at once. More than a handful on
	// one key is what draws a rate limit in the first place, so this is deliberately small and the
	// rest wait.
	defaultMaxConcurrent = 4
)

// provider is one provider's adapter together with everything a call to it needs: the queue that
// rations it, and the identity the receipt carries. It implements Client, so the resolver hands it
// out as it stands, and the Service's own call path uses the fields.
type provider struct {
	svc   *Service
	info  Info
	inner Client
	gate  *queue
}

// ID is the provider's own id, such as "anthropic". It is what a caller sees, not the adapter's.
func (p *provider) ID() string { return p.info.ID }

// Complete makes one non-streamed call, with the queue, retries, fallback, and receipt.
func (p *provider) Complete(ctx context.Context, req Request) (Reply, error) {
	return p.svc.complete(ctx, p, req)
}

// Stream makes one streamed call, with the queue, retries, fallback, and receipt.
func (p *provider) Stream(ctx context.Context, req Request) (Stream, error) {
	return p.svc.stream(ctx, p, req)
}

// retryPolicy is how many times a request is tried and how long it waits between tries.
type retryPolicy struct {
	attempts int
	backoff  time.Duration
	// budget bounds how long a request waits out network failures. Zero means no bound beyond
	// attempts, which is what a test that must not wait uses.
	budget time.Duration
	// net is told of a network failure and asked to wait for the internet. Nil never waits.
	net Connectivity
}

// Connectivity is what a retry asks about the internet. The daemon's is the monitor in
// internal/netwatch; a nil one is an internet nobody watches, and then a network-looking failure is
// taken to be the network.
type Connectivity interface {
	// Check reports a network-looking failure and says whether the internet is reachable now. A
	// failure aimed at one server leaves it reachable, and that failure is the provider's, not the
	// network's.
	Check(ctx context.Context) bool
	// WaitOnline blocks until the internet is reachable, or max passes, or ctx ends.
	WaitOnline(ctx context.Context, max time.Duration) bool
	// MarkOnline says a call to the internet just worked.
	MarkOnline()
}

// RetryInfo says a request is waiting to be tried again. The zero value says the waiting is over.
type RetryInfo struct {
	// Attempt is the number of the try that comes next, counting the first as one.
	Attempt int
	// Max is the most tries the request gets.
	Max int
	// Wait is how long it waits before that try. Zero when it waits for the internet to return.
	Wait time.Duration
	// Network says the network is why: the failure was the network, not the provider.
	Network bool
	// Offline says the internet is not reachable, so the request waits for it to come back.
	Offline bool
}

type retryObserverKey struct{}

// WithRetryObserver returns a context whose requests tell fn when they wait to be tried again, and
// tell it the zero RetryInfo when they stop waiting. It is how a card shows that its agent is waiting
// for the network. fn runs on the goroutine making the request, so it must not block.
func WithRetryObserver(ctx context.Context, fn func(RetryInfo)) context.Context {
	return context.WithValue(ctx, retryObserverKey{}, fn)
}

func observerFrom(ctx context.Context) func(RetryInfo) {
	if fn, ok := ctx.Value(retryObserverKey{}).(func(RetryInfo)); ok {
		return fn
	}
	return func(RetryInfo) {}
}

// sleepFunc waits, or gives up when the context is done. It is a field on the Service so that a
// test can run without waiting, and so a call cancelled while it waits its turn stops at once.
type sleepFunc func(ctx context.Context, d time.Duration) error

// sleepFor is the real sleep: a timer that the context can cut short.
func sleepFor(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// retryable reports whether err is worth trying the same request again. A rate limit or a provider
// that could not be reached will often clear; a refused key or a model the provider does not offer
// will not, so those are answered at once.
func retryable(err error) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUnavailable)
}

// fallbackable reports whether err is one a backup model might get past: the provider is out or
// rate limiting (scope 7.5), or it does not offer the model that was asked for. A refused key is
// not, because a backup on the same key would be refused too.
func fallbackable(err error) bool {
	return retryable(err) || errors.Is(err, ErrNoSuchModel)
}

// backoffFor is the wait before a given try: the base, doubling each try, never past maxBackoff.
func backoffFor(base time.Duration, try int) time.Duration {
	if base <= 0 {
		return 0
	}
	d := base
	for i := 1; i < try; i++ {
		d *= 2
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	return min(d, maxBackoff)
}

// withRetry runs attempt until it answers without a retryable failure, waiting longer between tries,
// and returns the last answer. It stops at once for a failure that will not change, and when the
// context is done - in which case the context's own error is the answer, because "you cancelled"
// is more useful than whatever the provider said as it was cut off.
//
// Three kinds of failure are told apart. A provider that answered badly (a rate limit, a server
// error), or a network-looking failure while the internet is reachable (a local model that is not
// running), gets a few short tries. A failure while the internet is down waits for it to return, and
// does not use up tries while it waits. A failure that looks like the network when nothing watches the
// internet (a nil monitor) is taken to be the network and gets many tries with growing waits. The
// whole wait for the network is bounded by the policy's budget.
func withRetry[T any](ctx context.Context, pol retryPolicy, sleep sleepFunc, attempt func() (T, error)) (T, error) {
	var zero T
	var last error
	run := newRetryRun(ctx, pol, sleep)
	defer run.observe(RetryInfo{})
	for try := 0; try < run.tries; try++ {
		if try > 0 {
			stop, err := run.pause(ctx, try)
			if err != nil {
				return zero, err
			}
			if stop {
				break
			}
		}
		value, err := attempt()
		if err == nil {
			if run.kind != failureOther && last != nil && pol.net != nil {
				pol.net.MarkOnline()
			}
			return value, nil
		}
		last = err
		if run.settle(ctx, err) {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return zero, last
}

// retryRun is the state of one withRetry call: what kind of failure the last try was, and how many
// tries each kind has used.
type retryRun struct {
	pol          retryPolicy
	sleep        sleepFunc
	observe      func(RetryInfo)
	started      time.Time
	tries        int
	otherLimit   int
	kind         failureKind
	networkTries int
	otherFails   int
}

func newRetryRun(ctx context.Context, pol retryPolicy, sleep sleepFunc) *retryRun {
	tries := max(pol.attempts, 1)
	return &retryRun{
		pol: pol, sleep: sleep, observe: observerFrom(ctx), started: time.Now(),
		tries: tries, otherLimit: min(tries, defaultOtherAttempts),
	}
}

// settle records a failed try and says whether to stop trying: the failure will not change, the
// context is done, or the short tries for a provider's own failures are used up.
func (r *retryRun) settle(ctx context.Context, err error) bool {
	if !retryable(err) || ctx.Err() != nil {
		return true
	}
	r.kind = r.pol.classify(ctx, err)
	if r.kind != failureOther {
		return false
	}
	r.otherFails++
	return r.otherFails >= r.otherLimit
}

// pause waits before try number try, which is at least 1. It answers stop when the budget is spent, so
// there should be no more tries, and the context's error when it ended during the wait.
func (r *retryRun) pause(ctx context.Context, try int) (stop bool, err error) {
	spent := r.pol.budget > 0 && time.Since(r.started) >= r.pol.budget
	if r.kind == failureOffline {
		return r.waitOffline(ctx, try, spent)
	}
	return r.waitBackoff(ctx, try, spent)
}

// waitOffline waits for the internet to return, without using up tries.
func (r *retryRun) waitOffline(ctx context.Context, try int, spent bool) (bool, error) {
	r.networkTries++
	if spent {
		return true, nil
	}
	r.observe(RetryInfo{Attempt: try + 1, Max: r.tries, Network: true, Offline: true})
	// A minute at most, then the request tries anyway: a monitor whose own check cannot get
	// out (a proxy-only network) would otherwise keep a working connection waiting.
	r.pol.net.WaitOnline(ctx, offlinePatience(r.pol.budget, r.started))
	return false, ctx.Err()
}

// waitBackoff waits a growing time before the next try of a failure the internet did not cause.
func (r *retryRun) waitBackoff(ctx context.Context, try int, spent bool) (bool, error) {
	network := r.kind == failureNetwork
	wait := backoffFor(r.pol.backoff, max(try-r.networkTries, 1))
	limit := r.otherLimit
	if network {
		r.networkTries++
		wait = backoffFor(networkBackoff, r.networkTries)
		limit = r.tries
	}
	if r.pol.budget > 0 && (spent || time.Since(r.started)+wait > r.pol.budget) {
		return true, nil
	}
	r.observe(RetryInfo{Attempt: try + 1, Max: limit, Wait: wait, Network: network})
	return false, r.sleep(ctx, wait)
}

// failureKind is why a try failed, as far as the retry can tell.
type failureKind int

const (
	// failureOther is a provider answering badly, or a failure aimed at one server.
	failureOther failureKind = iota
	// failureNetwork looks like the network and nothing can confirm it, because nothing watches.
	failureNetwork
	// failureOffline is the network, confirmed: the internet is not reachable.
	failureOffline
)

// classify decides what a retryable failure was. A network-looking error is the network only when
// the monitor confirms the internet is down; otherwise it is the provider's.
func (p retryPolicy) classify(ctx context.Context, err error) failureKind {
	if !netwatch.IsNetworkError(err) {
		return failureOther
	}
	if p.net == nil {
		return failureNetwork
	}
	if p.net.Check(ctx) {
		return failureOther
	}
	return failureOffline
}

// offlinePatienceMax is how long a request waits for the internet before it tries anyway.
const offlinePatienceMax = time.Minute

// offlinePatience is how long one wait for the internet may last: a minute, or what is left of the
// budget when that is less.
func offlinePatience(budget time.Duration, started time.Time) time.Duration {
	if budget <= 0 {
		return offlinePatienceMax
	}
	return min(offlinePatienceMax, remaining(budget, started))
}

// remaining is how much of a budget is left, or zero (no limit) when there is none.
func remaining(budget time.Duration, started time.Time) time.Duration {
	if budget <= 0 {
		return 0
	}
	return max(budget-time.Since(started), time.Millisecond)
}

// complete makes one non-streamed call on p, switching to the call's backup model if the provider
// fails in a way a backup might get past.
func (s *Service) complete(ctx context.Context, p *provider, req Request) (Reply, error) {
	reply, err := s.completeOn(ctx, p, req)
	if err == nil {
		return reply, nil
	}
	backup, ok := s.backupFor(ctx, req.Model, err)
	if !ok {
		return Reply{}, err
	}
	b, model, rerr := s.resolve(backup)
	if rerr != nil {
		// The backup itself cannot be set up, so the failure to report is the original one: the
		// caller's own model is what they asked for.
		return Reply{}, err
	}
	backed := req
	backed.Model = model
	breply, berr := s.completeOn(ctx, b, backed)
	if berr != nil {
		return Reply{}, err
	}
	s.noticeFallback(ctx, p.info.ID, req.Model, b.info.ID, model)
	return breply, nil
}

// completeOn makes one call on one provider: it takes the provider's queue slot, retries a rate-limit
// or unreachable answer, and files the receipt for the model that really answered.
func (s *Service) completeOn(ctx context.Context, p *provider, req Request) (Reply, error) {
	release, err := p.gate.acquire(ctx)
	if err != nil {
		return Reply{}, err
	}
	defer release()

	reply, err := withRetry(ctx, s.retry, s.sleep, func() (Reply, error) {
		return p.inner.Complete(ctx, req)
	})
	if err != nil {
		return Reply{}, err
	}
	s.record(ctx, p.info.ID, req.Model, reply.Usage)
	return reply, nil
}

// stream makes one streamed call on p. The retry covers starting the stream, and a provider that
// fails before it has delivered anything can be replaced by the call's backup model.
func (s *Service) stream(ctx context.Context, p *provider, req Request) (Stream, error) {
	release, err := p.gate.acquire(ctx)
	if err != nil {
		return nil, err
	}
	inner, err := withRetry(ctx, s.retry, s.sleep, func() (Stream, error) {
		return p.inner.Stream(ctx, req)
	})
	if err == nil {
		return &managedStream{svc: s, ctx: ctx, provider: p, req: req, gate: release, stream: inner}, nil
	}
	release()

	backup, ok := s.backupFor(ctx, req.Model, err)
	if !ok {
		return nil, err
	}
	b, model, rerr := s.resolve(backup)
	if rerr != nil {
		return nil, err
	}
	backed := req
	backed.Model = model
	gate, aerr := b.gate.acquire(ctx)
	if aerr != nil {
		return nil, err
	}
	binner, berr := withRetry(ctx, s.retry, s.sleep, func() (Stream, error) {
		return b.inner.Stream(ctx, backed)
	})
	if berr != nil {
		gate()
		return nil, err
	}
	s.noticeFallback(ctx, p.info.ID, req.Model, b.info.ID, model)
	return &managedStream{svc: s, ctx: ctx, provider: b, req: backed, gate: gate, stream: binner, fellBack: true}, nil
}

// backupFor returns the model to try instead when err is one a backup might get past, and whether
// there is one to try at all. A call with no backup, or one whose backup is the model that just
// failed, reports the failure as it is.
func (s *Service) backupFor(ctx context.Context, current string, err error) (string, bool) {
	backup := CallFrom(ctx).Backup
	if backup == "" || backup == current || !fallbackable(err) {
		return "", false
	}
	return backup, true
}

// noticeFallback tells the person Marshal changed models on its own (scope 7.5, "notifies you").
// The notice list's own store is a later phase's work, so the seam is a sentence: the daemon passes
// its notice writer here, and the line is logged either way so the change is never silent.
func (s *Service) noticeFallback(ctx context.Context, fromProvider, fromModel, toProvider, toModel string) {
	s.log.Info("a provider call fell back to the backup model",
		"from", fromProvider+"/"+fromModel, "to", toProvider+"/"+toModel)
	if s.notices == nil {
		return
	}
	s.notices(ctx, fmt.Sprintf(
		"Marshal switched from %s %s to the backup model %s %s, because %s could not be reached.",
		fromProvider, fromModel, toProvider, toModel, fromProvider))
}

// record files what a call cost. It never fails the call: a call that answered did its work whether
// or not its receipt could be stored, so a storage failure is a log line and nothing more. A model
// Marshal has no price for is recorded too, at zero cost, with a line saying so - an unpriced call
// is visible rather than silently free.
func (s *Service) record(ctx context.Context, providerID, model string, usage Usage) {
	if s.recorder == nil {
		return
	}
	cost, priced := Cost(model, usage)
	if !priced {
		s.log.Warn("a model answered but Marshal has no price for it, so the call is recorded at zero cost",
			"provider", providerID, "model", model)
	}
	call := CallFrom(ctx)
	err := s.recorder.Record(ctx, UsageRecord{
		CardID:       call.CardID,
		ProjectID:    call.ProjectID,
		RoleID:       call.RoleID,
		Provider:     providerID,
		Model:        model,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CostMicros:   cost,
		At:           s.now(),
	})
	if err != nil {
		s.log.Error("could not record what a call cost",
			"provider", providerID, "model", model, "error", err)
	}
}

// managedStream is a stream plus the bookkeeping one call needs: it holds the provider's queue slot
// until the answer is over, files the receipt when the answer ends, and - if the provider fails
// before it has delivered anything - starts again on the call's backup model, because an answer that
// has not begun can be restarted somewhere else, and one that has begun cannot.
type managedStream struct {
	svc      *Service
	ctx      context.Context
	provider *provider
	req      Request
	gate     func()
	stream   Stream

	delivered bool // an event has been handed to the caller
	fellBack  bool // a backup has been tried already, so there is no second one
}

// Recv returns the next event. It passes events through untouched, records the receipt on the last
// one, and gives the queue slot back when the answer is over.
func (m *managedStream) Recv() (Event, error) {
	for {
		ev, err := m.stream.Recv()
		switch {
		case err == nil:
			m.delivered = true
			if ev.Kind == EventDone {
				m.svc.record(m.ctx, m.provider.info.ID, m.req.Model, ev.Usage)
				m.release()
			}
			return ev, nil
		case errors.Is(err, io.EOF):
			m.release()
			return ev, err
		}
		// A failure. Before the provider has delivered anything, and with a backup left to try,
		// the answer can begin again somewhere else: the outage case of scope 7.5.
		if !m.delivered && !m.fellBack {
			if next, ok := m.restart(err); ok {
				m.stream = next
				continue
			}
		}
		m.release()
		return ev, err
	}
}

// Close abandons the stream and gives the slot back. It is safe to call more than once, as the
// Stream interface promises, and it is what the built-in agent's own defer calls when a turn ends.
func (m *managedStream) Close() error {
	err := m.stream.Close()
	m.release()
	return err
}

// release gives the queue slot back exactly once.
func (m *managedStream) release() { m.gate() }

// restart closes a failed stream and opens the backup model's in its place, answering false to mean
// "report the failure as it is". It moves the queue slot from the failed provider to the backup's,
// so the failed provider is not left rationed by a stream that is over.
func (m *managedStream) restart(cause error) (Stream, bool) {
	backup, ok := m.svc.backupFor(m.ctx, m.req.Model, cause)
	if !ok {
		return nil, false
	}
	b, model, err := m.svc.resolve(backup)
	if err != nil {
		return nil, false
	}
	// Stop reading the failed stream before taking the backup's slot, so the two are never held at
	// once. Closing a stream twice is safe, so nothing here has to remember whether it was closed.
	_ = m.stream.Close()
	gate, err := b.gate.acquire(m.ctx)
	if err != nil {
		return nil, false
	}
	backed := m.req
	backed.Model = model
	next, err := withRetry(m.ctx, m.svc.retry, m.svc.sleep, func() (Stream, error) {
		return b.inner.Stream(m.ctx, backed)
	})
	if err != nil {
		gate()
		return nil, false
	}
	fromProvider, fromModel := m.provider.info.ID, m.req.Model
	m.release()
	m.provider, m.req, m.gate, m.fellBack = b, backed, gate, true
	m.svc.noticeFallback(m.ctx, fromProvider, fromModel, b.info.ID, model)
	return next, true
}
