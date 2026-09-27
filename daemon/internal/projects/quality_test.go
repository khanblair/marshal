package projects_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeReviewGate is the quality gate driven by hand: it answers the count and the error it was
// given, and remembers every card it was asked about. It is the whole of what this module expects
// of the quality module (docs/architecture.md section 17.1).
type fakeReviewGate struct {
	blocking int
	err      error
	asked    []string
}

func (g *fakeReviewGate) BlockingFindings(_ context.Context, cardID string) (int, error) {
	g.asked = append(g.asked, cardID)
	return g.blocking, g.err
}

// qualityBlockingMessage is the sentence of architecture.md section 6.1 for a blocking smell. It is
// repeated here rather than read from the daemon so a reword fails this test.
const qualityBlockingMessage = "This card's changes have code smells to fix first. The agent has been told."

// inWorkingWithAPullRequest makes a card that is ready to move to In review: it has an open pull
// request and is in Working, so the only rule left before it may move is the quality gate.
func inWorkingWithAPullRequest(t *testing.T, e *env, projectID string) protocol.Card {
	t.Helper()
	card := e.card(t, projectID, "Ready to review")
	if _, err := e.svc.SetPullRequest(context.Background(), card.ID,
		protocol.PullRequest{Number: 287, URL: "https://example.test/pull/287"}); err != nil {
		t.Fatalf("SetPullRequest: %v", err)
	}
	working, err := e.svc.MoveCard(context.Background(), card.ID,
		protocol.MoveCardRequest{State: protocol.CardStateWorking})
	if err != nil {
		t.Fatalf("MoveCard to working: %v", err)
	}
	e.drainEvents()
	return working
}

// A person's drag to In review is refused when the quality gate reports a blocking finding: the card
// stays in Working, the refusal carries the reason and the sentence, and nothing is published.
func TestMoveToReviewIsRefusedWhenTheQualityGateBlocks(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	gate := &fakeReviewGate{blocking: 2}
	e.svc.SetReviewGate(gate)
	working := inWorkingWithAPullRequest(t, e, project.ID)

	_, err := e.svc.MoveCard(context.Background(), working.ID,
		protocol.MoveCardRequest{State: protocol.CardStateReview})
	perr := wantCode(t, err, protocol.ErrorCodeRefused)
	if got := perr.Details["reason"]; got != string(protocol.MoveRefusalReasonQualityBlocking) {
		t.Errorf("reason = %q, want %q", got, protocol.MoveRefusalReasonQualityBlocking)
	}
	if perr.Message != qualityBlockingMessage {
		t.Errorf("message = %q, want %q", perr.Message, qualityBlockingMessage)
	}
	if len(gate.asked) != 1 || gate.asked[0] != working.ID {
		t.Errorf("the gate was asked about %v, want [%s]", gate.asked, working.ID)
	}
	e.noEvent(t)
	after, err := e.svc.Card(context.Background(), working.ID)
	if err != nil {
		t.Fatalf("Card after the refusal: %v", err)
	}
	if !reflect.DeepEqual(after, working) {
		t.Errorf("the refused move changed the card: %+v, want %+v", after, working)
	}
}

// The daemon's own move to In review - the one an agent's pull request being opened drives
// (internal/pullrequest) - is refused by the same gate, so a blocking smell keeps a card out of
// review however it got there.
func TestSetStateToReviewIsRefusedWhenTheQualityGateBlocks(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	gate := &fakeReviewGate{blocking: 1}
	e.svc.SetReviewGate(gate)
	working := inWorkingWithAPullRequest(t, e, project.ID)

	_, err := e.svc.SetState(context.Background(), working.ID, protocol.CardStateReview)
	perr := wantCode(t, err, protocol.ErrorCodeRefused)
	if got := perr.Details["reason"]; got != string(protocol.MoveRefusalReasonQualityBlocking) {
		t.Errorf("reason = %q, want %q", got, protocol.MoveRefusalReasonQualityBlocking)
	}
	if len(gate.asked) != 1 || gate.asked[0] != working.ID {
		t.Errorf("the gate was asked about %v, want [%s]", gate.asked, working.ID)
	}
	e.noEvent(t)
	after, err := e.svc.Card(context.Background(), working.ID)
	if err != nil || after.State != protocol.CardStateWorking {
		t.Errorf("state after the refusal = %v (%v), want working", after.State, err)
	}
}

// The gate is consulted only for a move to In review: a move to any other column never asks it.
func TestTheQualityGateIsNotAskedAboutOtherColumns(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	gate := &fakeReviewGate{blocking: 5}
	e.svc.SetReviewGate(gate)
	inWorkingWithAPullRequest(t, e, project.ID)

	moved, err := e.svc.MoveCard(context.Background(), e.card(t, project.ID, "Elsewhere").ID,
		protocol.MoveCardRequest{State: protocol.CardStateWorking})
	if err != nil {
		t.Fatalf("MoveCard to working: %v", err)
	}
	if moved.State != protocol.CardStateWorking {
		t.Errorf("state = %s, want working", moved.State)
	}
	if len(gate.asked) != 0 {
		t.Errorf("the gate was asked about %v, want nothing", gate.asked)
	}
}

// A move to In review goes ahead when nothing blocks it: no gate wired at all, a gate that finds
// nothing, and - importantly - a gate whose checks could not run, because a smell checker that could
// not start must never keep a person's work out of review.
func TestMoveToReviewProceedsWhenNothingBlocks(t *testing.T) {
	tests := []struct {
		name string
		gate projects.ReviewGate
	}{
		{name: "no gate at all"},
		{name: "the gate finds nothing", gate: &fakeReviewGate{}},
		{name: "the gate could not run", gate: &fakeReviewGate{err: errors.New("no linter was available")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			project := e.folder(t, "small-repo")
			if tt.gate != nil {
				e.svc.SetReviewGate(tt.gate)
			}
			working := inWorkingWithAPullRequest(t, e, project.ID)

			moved, err := e.svc.MoveCard(context.Background(), working.ID,
				protocol.MoveCardRequest{State: protocol.CardStateReview})
			if err != nil {
				t.Fatalf("MoveCard to review: %v", err)
			}
			if moved.State != protocol.CardStateReview {
				t.Errorf("state = %s, want review", moved.State)
			}
			e.nextType(t, protocol.EventTypeCardMoved, protocol.ProjectTopic(project.ID))
		})
	}
}

// Setting the gate twice is a wiring mistake, but the second call wins rather than panicking: the
// last one set is the one asked, which is what a start-up that rebuilds its modules relies on.
func TestSetReviewGateReplacesTheGate(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	first := &fakeReviewGate{blocking: 1}
	second := &fakeReviewGate{}
	e.svc.SetReviewGate(first)
	e.svc.SetReviewGate(second)
	working := inWorkingWithAPullRequest(t, e, project.ID)

	if _, err := e.svc.MoveCard(context.Background(), working.ID,
		protocol.MoveCardRequest{State: protocol.CardStateReview}); err != nil {
		t.Fatalf("MoveCard to review: %v", err)
	}
	if len(first.asked) != 0 {
		t.Errorf("the replaced gate was still asked about %v", first.asked)
	}
	if len(second.asked) != 1 || second.asked[0] != working.ID {
		t.Errorf("the live gate was asked about %v, want [%s]", second.asked, working.ID)
	}
}
