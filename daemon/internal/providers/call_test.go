package providers

import (
	"context"
	"testing"
)

// TestCallTravelsInTheContext is the contract the built-in agent relies on: the session manager puts
// who a call is for into the context once, and the managed client reads it back when it writes the
// receipt - without the frozen four-argument Client interface growing a parameter for it.
func TestCallTravelsInTheContext(t *testing.T) {
	want := Call{
		SessionID: "01J8Z000000000000000000SES",
		CardID:    "01J8Z00000000000000000000CRD",
		ProjectID: "01J8Z00000000000000000000PRJ",
		RoleID:    "reviewer",
		Backup:    "gpt-5-mini",
	}
	got := CallFrom(WithCall(context.Background(), want))
	if got != want {
		t.Errorf("the context carried %+v, want %+v", got, want)
	}
}

// TestCallFromAnswersTheZeroCallWhenNothingSetIt is what a connection test or a chat title gets: a
// call that belongs to no card, so every field is empty and nothing is invented.
func TestCallFromAnswersTheZeroCallWhenNothingSetIt(t *testing.T) {
	if got := CallFrom(context.Background()); got != (Call{}) {
		t.Errorf("a context with no call carried %+v, want the zero Call", got)
	}
}

// TestACallsValueStaysInItsOwnContext proves the call is scoped to the context it was put in rather
// than being a package-level default: the context a caller already held carries nothing, and a new
// value replaces the old one rather than adding to it.
func TestACallsValueStaysInItsOwnContext(t *testing.T) {
	base := context.Background()
	first := WithCall(base, Call{CardID: "01J8Z00000000000000000000CRD"})
	if got := CallFrom(base); got != (Call{}) {
		t.Errorf("the context the value was derived from carried %+v, want nothing", got)
	}
	second := WithCall(first, Call{RoleID: "reviewer"})
	if got := CallFrom(second); got.CardID != "" || got.RoleID != "reviewer" {
		t.Errorf("the newer call carried %+v, want only its own fields", got)
	}
	if got := CallFrom(first); got.CardID == "" || got.RoleID != "" {
		t.Errorf("the earlier context carried %+v, want the card it was given", got)
	}
}
