package harness_test

import (
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A role's ceilings (B5.3, build-plan 5.3): the pure decision half. The wiring that reads a role and
// feeds it a card's usage is tested where the sessions live; these are the arithmetic the decision
// is made of, so they are tested without a store, a card, or an agent.

// The ceiling is gone past only when the card is over it: the number a role allows is allowed.
func TestACeilingIsGonePastOnlyWhenTheCardIsOverIt(t *testing.T) {
	limits := harness.Limits{TurnMinutes: 5, CostDollars: 10, Rounds: 12}
	at := harness.Usage{TurnMinutes: 5, CostDollars: 10, Rounds: 12}
	if reason, hit := limits.Check(at); hit {
		t.Errorf("a card exactly at its ceilings was stopped: %+v", reason)
	}
	over := harness.Usage{TurnMinutes: 6, CostDollars: 11, Rounds: 13}
	if _, hit := limits.Check(over); !hit {
		t.Error("a card over every ceiling was not stopped")
	}
}

// A role that sets no ceiling of a kind never stops a card for it, however far the card goes.
func TestAZeroCeilingIsNoCeiling(t *testing.T) {
	limits := harness.Limits{}
	if !limits.IsZero() {
		t.Error("a limits with no numbers is not zero")
	}
	far := harness.Usage{TurnMinutes: 10_000, CostDollars: 10_000, Rounds: 10_000}
	if reason, hit := limits.Check(far); hit {
		t.Errorf("a role with no ceilings stopped a card: %+v", reason)
	}
}

// Each ceiling on its own stops the card with the limit reason, and the sentence names the number
// that was passed and the ceiling it passed, so a person reading the card knows which one it was.
func TestEachCeilingStopsTheCardWithItsOwnSentence(t *testing.T) {
	tests := []struct {
		name   string
		limits harness.Limits
		usage  harness.Usage
		text   string
	}{
		{
			"time", harness.Limits{TurnMinutes: 5}, harness.Usage{TurnMinutes: 9},
			"This card's turn ran 9 minutes, past the role's 5 minute limit.",
		},
		{
			"cost", harness.Limits{CostDollars: 3}, harness.Usage{CostDollars: 4},
			"This card has cost $4, past the role's $3 limit.",
		},
		{
			"rounds", harness.Limits{Rounds: 12}, harness.Usage{Rounds: 13},
			"This card has taken 13 turns, past the role's limit of 12.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, hit := tt.limits.Check(tt.usage)
			if !hit {
				t.Fatalf("the %s ceiling did not stop the card", tt.name)
			}
			if reason.Kind != protocol.NeedsReasonKindLimit {
				t.Errorf("kind = %q, want %q", reason.Kind, protocol.NeedsReasonKindLimit)
			}
			if reason.Text != tt.text {
				t.Errorf("text = %q, want %q", reason.Text, tt.text)
			}
		})
	}
}

// When more than one ceiling is past, the one a person most needs to hear is named first: time,
// then cost, then rounds. The same usage always answers the same way.
func TestTheMostImmediateCeilingIsNamedFirst(t *testing.T) {
	limits := harness.Limits{TurnMinutes: 1, CostDollars: 1, Rounds: 1}
	over := harness.Usage{TurnMinutes: 2, CostDollars: 2, Rounds: 2}
	reason, hit := limits.Check(over)
	if !hit {
		t.Fatal("a card over every ceiling was not stopped")
	}
	if !strings.Contains(reason.Text, "turn ran") {
		t.Errorf("text = %q, want the time ceiling named first", reason.Text)
	}

	// With the time ceiling satisfied, the cost ceiling is the next one named.
	reason, _ = limits.Check(harness.Usage{TurnMinutes: 1, CostDollars: 2, Rounds: 2})
	if !strings.Contains(reason.Text, "has cost") {
		t.Errorf("text = %q, want the cost ceiling named next", reason.Text)
	}

	// And with time and cost satisfied, rounds is what stops the card.
	reason, _ = limits.Check(harness.Usage{TurnMinutes: 1, CostDollars: 1, Rounds: 2})
	if !strings.Contains(reason.Text, "has taken") {
		t.Errorf("text = %q, want the round ceiling named last", reason.Text)
	}
}
