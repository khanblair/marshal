package providers

// This file carries who a provider call is for. A call is made deep inside an agent turn, and what
// it cost has to be filed against the card, project, and role it ran for - but the neutral Request
// is only about what to ask the provider. So the facts about the work travel in the context, the
// way a request id does, and the managed client reads them when it writes a usage row
// (docs/architecture.md section 10's `usage` columns).
//
// The context also carries the backup model, because that is a per-call choice: the layer that
// knows a card's or a role's backup is the layer that knows the card, and the providers package
// never reads the cards table.

import "context"

// Call is what Marshal knows about the work a provider call belongs to. Every field is optional:
// a connection test or a chat title has no card, and a call from a test has nothing at all.
type Call struct {
	// SessionID is the session the call ran in.
	SessionID string
	// CardID is the card the call ran for. Empty for a chat or a test.
	CardID string
	// ProjectID is the project the card belongs to. It is stored on the usage row rather than
	// looked up from the card, so spend survives a card being removed.
	ProjectID string
	// RoleID is the role the session was running as.
	RoleID string
	// Backup names the model to run instead when the model that was asked for cannot be reached:
	// the provider is out, it is rate limiting, or it does not offer that model. Empty means the
	// call has no backup, and a failure is reported as it is.
	Backup string
}

// callKey is the type the Call is stored under. It is unexported and empty, so no other package can
// put one there and two Call values can never be confused with anything else in a context.
type callKey struct{}

// WithCall returns a context that carries who the call is for. The session manager sets this once
// per turn; everything the turn does then files its cost against the right card.
func WithCall(ctx context.Context, call Call) context.Context {
	return context.WithValue(ctx, callKey{}, call)
}

// CallFrom returns what a context says about the call, or the zero Call when nothing set it.
func CallFrom(ctx context.Context) Call {
	call, _ := ctx.Value(callKey{}).(Call)
	return call
}
