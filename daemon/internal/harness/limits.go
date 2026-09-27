package harness

import (
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// This file is the limits half of the harness. harness.go makes the permission decision; this file,
// with stuck.go, says when a card has gone too far or gone round in circles (docs/architecture.md
// section 3, "Permissions, limits, retries, fallback, stuck detection, activity feed";
// docs/backend-checklist.md B5.3, build-plan 5.3). Both halves are pure functions of what the
// daemon already knows, so a limit or a loop is decided without running an agent and can be tested
// on its own.
//
// A card's ceilings come from its role (protocol.RoleLimits) and are read when the card runs, not
// when the role is saved. They are not the install's own ceilings (protocol.Limit, the cost and
// awake limits of internal/providers/limits.go): a limit is what Marshal may spend in a day, these
// are what one role's cards aim for.

// Limits are the ceilings one role sets on a card's work. Zero means the role sets no ceiling of
// that kind, and a Limits with every field zero limits nothing at all.
type Limits struct {
	// TurnMinutes is the longest one turn may run, in whole minutes.
	TurnMinutes int
	// CostDollars is the most one card may cost, in whole dollars.
	CostDollars int
	// Rounds is the most turns one card may take.
	Rounds int
}

// Usage is what a card has used so far: the length of the turn that just ended, the card's spend,
// and the number of turns it has taken.
type Usage struct {
	// TurnMinutes is how long the turn that just ended ran, in whole minutes.
	TurnMinutes int
	// CostDollars is the card's spend so far, in whole dollars.
	CostDollars int
	// Rounds is the number of turns the card has taken, counting the one that just ended.
	Rounds int
}

// Reason is why a card is stopped: the kind a client acts on and the sentence a person reads. A zero
// Reason is not a reason, which is how "the card is fine" travels.
type Reason struct {
	// Kind is the wire reason the card is moved to Needs you with.
	Kind protocol.NeedsReasonKind
	// Text is the plain sentence shown under the card.
	Text string
}

// Check reports the first ceiling a card has gone past, and whether it went past any. A ceiling is
// gone past when the card is over it: a role that allows 12 turns allows exactly 12, and the 13th
// is what stops the card. The order is time, then cost, then rounds, so the sentence a person reads
// names the most immediate thing first, and the same usage always answers the same way.
func (l Limits) Check(u Usage) (Reason, bool) {
	switch {
	case l.TurnMinutes > 0 && u.TurnMinutes > l.TurnMinutes:
		return Reason{Kind: protocol.NeedsReasonKindLimit, Text: fmt.Sprintf(
			"This card's turn ran %d minutes, past the role's %d minute limit.",
			u.TurnMinutes, l.TurnMinutes)}, true
	case l.CostDollars > 0 && u.CostDollars > l.CostDollars:
		return Reason{Kind: protocol.NeedsReasonKindLimit, Text: fmt.Sprintf(
			"This card has cost $%d, past the role's $%d limit.",
			u.CostDollars, l.CostDollars)}, true
	case l.Rounds > 0 && u.Rounds > l.Rounds:
		return Reason{Kind: protocol.NeedsReasonKindLimit, Text: fmt.Sprintf(
			"This card has taken %d turns, past the role's limit of %d.",
			u.Rounds, l.Rounds)}, true
	}
	return Reason{}, false
}

// IsZero reports whether the limits hold nothing at all, so a caller can skip reading the card's
// usage for a role that sets no ceilings.
func (l Limits) IsZero() bool {
	return l.TurnMinutes == 0 && l.CostDollars == 0 && l.Rounds == 0
}
