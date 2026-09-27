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
)

// Defaults for a Service's call behaviour. They are what a daemon runs with; a test sets Options to
// shrink or still them.
const (
	// defaultAttempts is how many times one request is tried, first try included. Four tries of a
	// rate limit, waiting longer each time, is enough for a short burst to clear.
	defaultAttempts = 4
	// defaultBackoff is the wait before the second try. It doubles from there.
	defaultBackoff = 250 * time.Millisecond
	// maxBackoff caps the wait however many tries there have been, so a long outage does not turn
	// into a minutes-long stall inside one call.
	maxBackoff = 8 * time.Second
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
func withRetry[T any](ctx context.Context, pol retryPolicy, sleep sleepFunc, attempt func() (T, error)) (T, error) {
	var zero T
	var last error
	tries := max(pol.attempts, 1)
	for try := range tries {
		if try > 0 {
			if err := sleep(ctx, backoffFor(pol.backoff, try)); err != nil {
				return zero, err
			}
		}
		value, err := attempt()
		if err == nil {
			return value, nil
		}
		last = err
		if !retryable(err) || ctx.Err() != nil {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return zero, last
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
