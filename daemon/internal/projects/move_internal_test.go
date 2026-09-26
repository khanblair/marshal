package projects

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The rules of docs/architecture.md section 6.1, checked in order, with the exact reason code and
// sentence each refusal carries. This is the specification as a table, so a change to a rule or a
// word fails here.
func TestCheckMoveRules(t *testing.T) {
	pullRequest := &protocol.PullRequest{Number: 287, URL: "https://example.test/pull/287"}
	passed := protocol.CIStatePassed
	failed := protocol.CIStateFailed

	tests := []struct {
		name    string
		card    protocol.Card
		target  protocol.CardState
		reason  protocol.MoveRefusalReason // empty means the move is allowed
		message string
	}{
		{
			name: "rule 1: a done card cannot move", card: protocol.Card{State: protocol.CardStateDone},
			target: protocol.CardStateWorking, reason: protocol.MoveRefusalReasonFromDone,
			message: "Done cards are merged. Fork the card to keep working on it.",
		},
		{
			name: "rule 2: nothing moves to done by hand", card: protocol.Card{State: protocol.CardStateWorking},
			target: protocol.CardStateDone, reason: protocol.MoveRefusalReasonToDone,
			message: "Cards move to Done by themselves after they merge.",
		},
		{
			name: "rule 3: nothing moves to needs by hand", card: protocol.Card{State: protocol.CardStateWorking},
			target: protocol.CardStateNeeds, reason: protocol.MoveRefusalReasonToNeeds,
			message: "Cards move to Needs you by themselves when an agent is waiting on you.",
		},
		{
			name: "rule 4: review needs a pull request", card: protocol.Card{State: protocol.CardStateWorking},
			target: protocol.CardStateReview, reason: protocol.MoveRefusalReasonNeedsPullRequest,
			message: "In review needs an open pull request. The agent opens one when the work is ready.",
		},
		{
			name:   "rule 4: review from planning is refused even with a pull request",
			card:   protocol.Card{State: protocol.CardStatePlanning, PullRequest: pullRequest},
			target: protocol.CardStateReview, reason: protocol.MoveRefusalReasonNeedsPullRequest,
			message: "In review needs an open pull request. The agent opens one when the work is ready.",
		},
		{
			name:   "rule 4: review from backlog is refused",
			card:   protocol.Card{State: protocol.CardStateBacklog, PullRequest: pullRequest},
			target: protocol.CardStateReview, reason: protocol.MoveRefusalReasonNeedsPullRequest,
			message: "In review needs an open pull request. The agent opens one when the work is ready.",
		},
		{
			name: "rule 5: ready needs review first", card: protocol.Card{State: protocol.CardStateWorking},
			target: protocol.CardStateReady, reason: protocol.MoveRefusalReasonNeedsReview,
			message: "Ready to merge needs an approved review and passing checks.",
		},
		{
			name:   "rule 6: ready needs passing checks",
			card:   protocol.Card{State: protocol.CardStateReview, PullRequest: pullRequest, CI: &failed},
			target: protocol.CardStateReady, reason: protocol.MoveRefusalReasonChecksNotPassed,
			message: "Checks haven't passed on this card yet, so it can't be ready to merge.",
		},
		{
			name:   "rule 6: ready needs a CI state at all",
			card:   protocol.Card{State: protocol.CardStateReview, PullRequest: pullRequest},
			target: protocol.CardStateReady, reason: protocol.MoveRefusalReasonChecksNotPassed,
			message: "Checks haven't passed on this card yet, so it can't be ready to merge.",
		},
		{
			name: "rule 7: a merging card stays put", card: protocol.Card{State: protocol.CardStateMerging},
			target: protocol.CardStateWorking, reason: protocol.MoveRefusalReasonCardMerging,
			message: "The Integrator is merging this card. Wait for the merge to finish.",
		},
		{
			// A drop back on the column the card is already in is not a rule at all: it is
			// checked first, so it does nothing and succeeds. Rule 1 is about moving a done card
			// somewhere else, and its sentence does not fit this case.
			name: "the order: a done card dropped on done is a no-op, not rule 1",
			card: protocol.Card{State: protocol.CardStateDone}, target: protocol.CardStateDone,
		},
		{
			name: "allowed: backlog to working", card: protocol.Card{State: protocol.CardStateBacklog},
			target: protocol.CardStateWorking,
		},
		{
			name: "allowed: backlog to planning", card: protocol.Card{State: protocol.CardStateBacklog},
			target: protocol.CardStatePlanning,
		},
		{
			name: "allowed: anything to backlog", card: protocol.Card{State: protocol.CardStateReview, PullRequest: pullRequest},
			target: protocol.CardStateBacklog,
		},
		{
			name:   "allowed: review to working",
			card:   protocol.Card{State: protocol.CardStateReview, PullRequest: pullRequest},
			target: protocol.CardStateWorking,
		},
		{
			name:   "allowed: working to review with a pull request",
			card:   protocol.Card{State: protocol.CardStateWorking, PullRequest: pullRequest},
			target: protocol.CardStateReview,
		},
		{
			name:   "allowed: review to ready with passing checks",
			card:   protocol.Card{State: protocol.CardStateReview, PullRequest: pullRequest, CI: &passed},
			target: protocol.CardStateReady,
		},
		{
			name: "allowed: planning to working", card: protocol.Card{State: protocol.CardStatePlanning},
			target: protocol.CardStateWorking,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkMove(tt.card, tt.target)
			if tt.reason == "" {
				if got != nil {
					t.Fatalf("checkMove = %v, want the move allowed", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("checkMove allowed the move, want %s", tt.reason)
			}
			if got.Code != protocol.ErrorCodeRefused {
				t.Errorf("code = %s, want refused", got.Code)
			}
			if got.Message != tt.message {
				t.Errorf("message = %q, want %q", got.Message, tt.message)
			}
			if got.Details["reason"] != string(tt.reason) {
				t.Errorf("reason = %q, want %q", got.Details["reason"], tt.reason)
			}
		})
	}
}

// A move to the column the card is already in does nothing and succeeds, so a drag that lands back
// on its own column never shows a refusal.
func TestCheckMoveToTheSameColumnIsAllowed(t *testing.T) {
	for _, state := range protocol.CardStateValues() {
		if got := checkMove(protocol.Card{State: state}, state); got != nil {
			t.Errorf("moving a %s card to %s = %v, want it allowed", state, state, got)
		}
	}
}

// Every "doing now" line a move leaves behind.
func TestDoingNowAfterMove(t *testing.T) {
	tests := []struct {
		name   string
		card   protocol.Card
		target protocol.CardState
		want   string
	}{
		{"review to working", protocol.Card{State: protocol.CardStateReview, DoingNow: "Reviewing"}, protocol.CardStateWorking, addressingReviewComments},
		{"planning to working", protocol.Card{State: protocol.CardStatePlanning, DoingNow: "Planning"}, protocol.CardStateWorking, startingWithoutAPlan},
		{"any other move keeps the line", protocol.Card{State: protocol.CardStateWorking, DoingNow: "Writing the handler"}, protocol.CardStateBacklog, "Writing the handler"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := doingNowAfterMove(tt.card, tt.target); got != tt.want {
				t.Errorf("doingNowAfterMove = %q, want %q", got, tt.want)
			}
		})
	}
}

// The sentence of every reason is in the table, so a new reason cannot be added without words.
func TestEveryMoveRefusalHasASentence(t *testing.T) {
	for _, reason := range protocol.MoveRefusalReasonValues() {
		if refusalMessage(reason) == "" {
			t.Errorf("reason %s has no sentence", reason)
		}
	}
}
