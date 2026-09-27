package providers

// This file is the per-provider request queue (docs/marshal-product-scope.md section 7.6). A
// provider counts its rate limit per API key, and Marshal runs several cards at once on one key, so
// a burst has to wait its turn instead of being refused: the queue lets a few requests run and makes
// the rest wait for a slot. It is deliberately one queue per provider rather than one per card,
// because two cards on one key share the limit that is actually on the key.
//
// The queue is one half of that promise; retrying a request the provider did refuse is the other
// (see withRetry in managed.go). Together they are what makes ten parallel agents on one key finish
// rather than fail.

import (
	"context"
	"sync"
)

// queue rations one provider's requests to max in flight at a time. The zero queue is not usable;
// make one with newQueue.
type queue struct {
	slots chan struct{}
}

// newQueue makes a queue that lets max requests run at once. A max below one is treated as one, so a
// caller can never make a queue that deadlocks every request it is given.
func newQueue(max int) *queue {
	if max < 1 {
		max = 1
	}
	return &queue{slots: make(chan struct{}, max)}
}

// acquire takes a slot, waiting until one is free or ctx is done. The returned release gives the
// slot back; it is safe to call more than once, so a caller can both release early and defer it
// without counting twice.
func (q *queue) acquire(ctx context.Context) (release func(), err error) {
	select {
	case q.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var once sync.Once
	return func() { once.Do(func() { <-q.slots }) }, nil
}
