package projects_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// SetCI records the state of the runs on a card's branch (docs/architecture.md sections 9 and 10,
// docs/backend-checklist.md B6.2). It is the CI monitor's write, not a person's, and it is the only
// thing that makes the badge on a card and the CI column of the lists follow the forge.
func TestSetCIWritesTheCardsBadgeAndPublishesTheCard(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Watch my CI")
	e.drainEvents()

	failed := protocol.CIStateFailed
	after, err := e.svc.SetCI(context.Background(), card.ID, &failed)
	if err != nil {
		t.Fatalf("SetCI: %v", err)
	}
	if after.CI == nil || *after.CI != protocol.CIStateFailed {
		t.Fatalf("SetCI gave %+v, want the card's CI state failed", after.CI)
	}
	ev := e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID))
	data, ok := ev.Data.(protocol.CardEventData)
	if !ok || data.Card.CI == nil || *data.Card.CI != protocol.CIStateFailed {
		t.Fatalf("card.updated carries %+v, want the new CI state", ev.Data)
	}
	if ev.Critical {
		t.Error("card.updated is not a critical event")
	}
}

// The same state twice is a no-op, so a delivery that repeats a run's outcome does not make the
// board redraw.
func TestSetCIIsQuietWhenTheStateIsUnchanged(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Watch my CI")
	e.drainEvents()

	passed := protocol.CIStatePassed
	if _, err := e.svc.SetCI(context.Background(), card.ID, &passed); err != nil {
		t.Fatalf("SetCI: %v", err)
	}
	e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID))

	again := protocol.CIStatePassed
	if _, err := e.svc.SetCI(context.Background(), card.ID, &again); err != nil {
		t.Fatalf("SetCI again: %v", err)
	}
	e.noEvent(t)
}

// A nil state clears the column, which is the honest answer for a branch Marshal no longer has a run
// for, and the wire says null rather than a made-up state.
func TestSetCIClearsTheState(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Watch my CI")
	e.drainEvents()

	running := protocol.CIStateRunning
	state, err := e.svc.SetCI(context.Background(), card.ID, &running)
	if err != nil {
		t.Fatalf("SetCI: %v", err)
	}
	e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID))

	cleared, err := e.svc.SetCI(context.Background(), card.ID, nil)
	if err != nil {
		t.Fatalf("SetCI(nil): %v", err)
	}
	if cleared.CI != nil {
		t.Fatalf("SetCI(nil) left the state as %v, want null", cleared.CI)
	}
	if state.CI == nil {
		t.Fatal("the first call should have written a state")
	}
	e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID))

	// Clearing a state that is already clear changes nothing.
	if _, err := e.svc.SetCI(context.Background(), card.ID, nil); err != nil {
		t.Fatalf("SetCI(nil) again: %v", err)
	}
	e.noEvent(t)
}

// A card that is not there is not found, the same as on every other card write.
func TestSetCIOnAnUnknownCardIsNotFound(t *testing.T) {
	e := newEnv(t)
	passed := protocol.CIStatePassed
	_, err := e.svc.SetCI(context.Background(), "01M3C107JB041061050R3GG28A", &passed)
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
}
