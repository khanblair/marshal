package protocol

import "slices"

// BypassRequest is the body of POST /v1/cards/{id}/bypass. Turning bypass on is the one card
// setting that is not a plain field change, because it carries the acknowledgement a person gives
// before it is granted (docs/architecture.md section 10, docs/backend-checklist.md B3.2).
//
// The sentence a person reads belongs to the app, so what travels here is the acknowledgement
// itself: the app sends true only when the person accepted it. Sending false is refused, which is
// what stops a client, a script, or a stale tab from turning bypass on as a side effect of setting
// a permission mode.
type BypassRequest struct {
	// Acknowledged says the person accepted what bypass means: the agent runs every command and
	// edit in the card's worktree without asking.
	Acknowledged bool `json:"acknowledged"`
}

// BypassRefusalReason is why the daemon refused to turn bypass on. It travels in the details of a
// refused error, the way a view refusal does, so the app can show its own words for it.
type BypassRefusalReason string

const (
	// BypassRefusalReasonUnacknowledged is a request that did not acknowledge what bypass means.
	BypassRefusalReasonUnacknowledged BypassRefusalReason = "unacknowledged"
	// BypassRefusalReasonLocked is a card of a project whose settings forbid bypass. The lock is
	// the same one the app shows beside the switch (N16).
	BypassRefusalReasonLocked BypassRefusalReason = "locked"
)

// BypassRefusalReasonValues lists every reason, in the order of the constants above.
func BypassRefusalReasonValues() []BypassRefusalReason {
	return []BypassRefusalReason{BypassRefusalReasonUnacknowledged, BypassRefusalReasonLocked}
}

// Valid reports whether the reason is one of the known ones.
func (r BypassRefusalReason) Valid() bool { return slices.Contains(BypassRefusalReasonValues(), r) }
