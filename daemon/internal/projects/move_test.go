package projects_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// An allowed move changes the state, publishes card.moved with the state it came from, and leaves
// the rest of the card alone.
func TestMoveCardAppliesAnAllowedMove(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Move me")
	e.drainEvents()

	moved, err := e.svc.MoveCard(context.Background(), card.ID,
		protocol.MoveCardRequest{State: protocol.CardStateWorking})
	if err != nil {
		t.Fatalf("MoveCard: %v", err)
	}
	if moved.State != protocol.CardStateWorking {
		t.Errorf("state = %s, want working", moved.State)
	}
	if moved.Title != card.Title || moved.Number != card.Number || moved.ID != card.ID {
		t.Errorf("the move changed more than the state: %+v", moved)
	}
	if moved.UpdatedAt == card.UpdatedAt {
		t.Error("the move did not touch updatedAt")
	}
	ev := e.nextType(t, protocol.EventTypeCardMoved, protocol.ProjectTopic(project.ID))
	data, ok := ev.Data.(protocol.CardMovedEventData)
	if !ok || !reflect.DeepEqual(data.Card, moved) || data.From != protocol.CardStateBacklog {
		t.Errorf("event = %+v, want the moved card and from=backlog", ev.Data)
	}
	if !ev.Critical {
		t.Error("card.moved must be critical: a client that missed it would draw the wrong board")
	}
	again, err := e.svc.Card(context.Background(), card.ID)
	if err != nil || !reflect.DeepEqual(again, moved) {
		t.Errorf("Card after the move = %+v, %v; want %+v", again, err, moved)
	}
}

// A refused move leaves the card exactly as it was, publishes nothing, and answers with the reason
// and the sentence the app shows.
func TestMoveCardRefusesAndChangesNothing(t *testing.T) {
	tests := []struct {
		name    string
		target  protocol.CardState
		reason  protocol.MoveRefusalReason
		message string
	}{
		{
			name: "to done", target: protocol.CardStateDone, reason: protocol.MoveRefusalReasonToDone,
			message: "Cards move to Done by themselves after they merge.",
		},
		{
			name: "to needs", target: protocol.CardStateNeeds, reason: protocol.MoveRefusalReasonToNeeds,
			message: "Cards move to Needs you by themselves when an agent is waiting on you.",
		},
		{
			name: "to review without a pull request", target: protocol.CardStateReview,
			reason:  protocol.MoveRefusalReasonNeedsPullRequest,
			message: "In review needs an open pull request. The agent opens one when the work is ready.",
		},
		{
			name: "to ready from backlog", target: protocol.CardStateReady,
			reason:  protocol.MoveRefusalReasonNeedsReview,
			message: "Ready to merge needs an approved review and passing checks.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			project := e.folder(t, "small-repo")
			card := e.card(t, project.ID, "Leave me alone")
			e.drainEvents()

			_, err := e.svc.MoveCard(context.Background(), card.ID, protocol.MoveCardRequest{State: tt.target})
			perr, ok := err.(*protocol.Error)
			if !ok {
				t.Fatalf("MoveCard = %v, want a *protocol.Error", err)
			}
			if perr.Code != protocol.ErrorCodeRefused {
				t.Errorf("code = %s, want refused", perr.Code)
			}
			if perr.Details["reason"] != string(tt.reason) {
				t.Errorf("reason = %q, want %q", perr.Details["reason"], tt.reason)
			}
			if perr.Message != tt.message {
				t.Errorf("message = %q, want %q", perr.Message, tt.message)
			}
			e.noEvent(t)
			after, err := e.svc.Card(context.Background(), card.ID)
			if err != nil || !reflect.DeepEqual(after, card) {
				t.Errorf("the card changed: %+v, want %+v", after, card)
			}
		})
	}
}

// A move to the column the card is already in does nothing, publishes nothing, and succeeds.
func TestMoveCardToItsOwnColumnDoesNothing(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Already here")
	e.drainEvents()

	got, err := e.svc.MoveCard(context.Background(), card.ID,
		protocol.MoveCardRequest{State: protocol.CardStateBacklog})
	if err != nil {
		t.Fatalf("MoveCard: %v", err)
	}
	if !reflect.DeepEqual(got, card) {
		t.Errorf("card = %+v, want it unchanged", got)
	}
	e.noEvent(t)
}

// A column Marshal does not have, and the merging state, which belongs to the Integrator and has no
// column of its own, are both refused as bad requests rather than as rule refusals.
func TestMoveCardRefusesAColumnThatIsNotOne(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Nowhere to go")

	for _, target := range []protocol.CardState{"sideways", protocol.CardStateMerging} {
		_, err := e.svc.MoveCard(context.Background(), card.ID, protocol.MoveCardRequest{State: target})
		perr, ok := err.(*protocol.Error)
		if !ok || perr.Code != protocol.ErrorCodeInvalidArgument {
			t.Errorf("MoveCard to %q = %v, want invalid_argument", target, err)
		}
	}
}

// A card that does not exist is not found, and no rule is consulted.
func TestMoveCardOfAnUnknownCard(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.MoveCard(context.Background(), "01M3C107JB041061050R3GG28A",
		protocol.MoveCardRequest{State: protocol.CardStateWorking})
	perr, ok := err.(*protocol.Error)
	if !ok || perr.Code != protocol.ErrorCodeNotFound {
		t.Errorf("MoveCard = %v, want not_found", err)
	}
}

// The "doing now" line a person sees after a move back from review.
func TestMoveCardFromReviewToWorkingSetsDoingNow(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Back to work")
	// SetState is the plain state change the session manager uses; a test uses it to reach a
	// state that no manual move can reach without a pull request.
	if _, err := e.svc.SetState(context.Background(), card.ID, protocol.CardStateReview); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	moved, err := e.svc.MoveCard(context.Background(), card.ID,
		protocol.MoveCardRequest{State: protocol.CardStateWorking})
	if err != nil {
		t.Fatalf("MoveCard: %v", err)
	}
	if moved.DoingNow != "Addressing review comments" {
		t.Errorf("doing now = %q, want the review line", moved.DoingNow)
	}
}
