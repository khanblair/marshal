// Package runctx carries facts about why a schedule is running, from the scheduler to the handler
// that writes the message, which are packages that do not know each other.
package runctx

import (
	"context"
	"time"
)

type lateKey struct{}

// WithLate marks a run as a catch-up: it was due at due, and is only running now because the daemon
// was not up then.
func WithLate(ctx context.Context, due time.Time) context.Context {
	return context.WithValue(ctx, lateKey{}, due)
}

// Late says when the run was due, when it is a catch-up.
func Late(ctx context.Context) (due time.Time, late bool) {
	due, late = ctx.Value(lateKey{}).(time.Time)
	return due, late
}
