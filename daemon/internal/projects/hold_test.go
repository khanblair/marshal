package projects_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// SetHold writes a card's pause and keep-awake flags and publishes card.updated when one of them
// really changed. The rules that decide when a card may be paused, slept, or pinned live in
// internal/session (docs/architecture.md section 5.1); this is the write those rules use.
func TestSetHoldWritesTheCardFlagsAndPublishesTheCard(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Hold me")
	e.drainEvents()

	yes := true
	held, err := e.svc.SetHold(context.Background(), card.ID, &yes, &yes)
	if err != nil {
		t.Fatalf("SetHold: %v", err)
	}
	if !held.Paused || !held.Pinned || held.ID != card.ID {
		t.Fatalf("SetHold gave %+v, want both flags set on the card", held)
	}
	ev := e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID))
	if data, ok := ev.Data.(protocol.CardEventData); !ok || !data.Card.Paused || !data.Card.Pinned {
		t.Errorf("card.updated carries %+v, want the new flags", ev.Data)
	}
	if ev.Critical {
		t.Error("card.updated is not a critical event")
	}

	// Setting a flag that is already set changes nothing, so nothing is published.
	if again, err := e.svc.SetHold(context.Background(), card.ID, &yes, nil); err != nil {
		t.Fatalf("SetHold again: %v", err)
	} else if !again.Paused || !again.Pinned {
		t.Errorf("SetHold again gave %+v, want the flags left alone", again)
	}
	e.noEvent(t)

	// Clearing one flag leaves the other as it was: a nil flag is not touched at all.
	no := false
	cleared, err := e.svc.SetHold(context.Background(), card.ID, &no, nil)
	if err != nil {
		t.Fatalf("SetHold(clear the pause): %v", err)
	}
	if cleared.Paused || !cleared.Pinned {
		t.Errorf("SetHold(clear the pause) gave %+v, want the pin kept", cleared)
	}
	e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID))

	// A card that is not there is not found, the same as on every other card write.
	_, err = e.svc.SetHold(context.Background(), "01M3C107JB041061050R3GG28A", &yes, nil)
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
}
