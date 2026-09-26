package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The wire card carries its session state (docs/architecture.md section 5.1, backlog B2.15), so a
// client that loads the board after a reload or a daemon restart knows which cards are asleep, and
// one that follows only the project topic hears every change to it.

// cardSession matches the card.updated event that carries this card with its session in a state.
func cardSession(cardID string, state protocol.SessionState) func(protocol.Event) bool {
	return func(ev protocol.Event) bool {
		if ev.Type != protocol.EventTypeCardUpdated {
			return false
		}
		var data protocol.CardEventData
		return json.Unmarshal(ev.Data, &data) == nil && data.Card.ID == cardID &&
			data.Card.Session != nil && *data.Card.Session == state
	}
}

// onBoard reads the project's board over HTTP and returns its cards by id.
func (st *stack) onBoard(projectID string) map[string]protocol.Card {
	st.t.Helper()
	board := decode[protocol.BoardSnapshot](st.t,
		st.do(http.MethodGet, "/v1/projects/"+projectID+"/board", nil).want(st.t, http.StatusOK))
	cards := make(map[string]protocol.Card, len(board.Cards))
	for _, card := range board.Cards {
		cards[card.ID] = card
	}
	return cards
}

// wantHeld checks a card's session, pause, and pin as the wire shows them.
func wantHeld(t *testing.T, what string, card protocol.Card, session protocol.SessionState, paused, pinned bool) {
	t.Helper()
	switch {
	case session == "" && card.Session != nil:
		t.Errorf("%s: session = %q, want null", what, *card.Session)
	case session != "" && (card.Session == nil || *card.Session != session):
		t.Errorf("%s: session = %v, want %q", what, card.Session, session)
	}
	if card.Paused != paused || card.Pinned != pinned {
		t.Errorf("%s: paused = %v and pinned = %v, want %v and %v", what, card.Paused, card.Pinned, paused, pinned)
	}
}

// A client that follows only the project topic, as Home and the board do, hears a sleep and a wake
// as card.updated with the card's own session state, once each.
func TestASleepAndAWakeReachAClientOnTheProjectTopic(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Sleep on it")
	base := "/v1/cards/" + card.ID
	stream := st.dial(protocol.ProjectTopic(project.ID))

	st.do(http.MethodPost, base+"/start", nil).want(t, http.StatusOK)
	stream.until(cardSession(card.ID, protocol.SessionStateAwake))
	st.do(http.MethodPost, base+"/pause", nil).want(t, http.StatusOK)
	st.do(http.MethodPost, base+"/sleep", nil).want(t, http.StatusNoContent)
	slept := stream.until(cardSession(card.ID, protocol.SessionStateAsleep))
	if got := dataOf[protocol.CardEventData](t, slept[len(slept)-1]).Card; !got.Paused {
		t.Errorf("the sleep's card = %+v, want the paused card", got)
	}

	st.do(http.MethodPost, base+"/wake", nil).want(t, http.StatusNoContent)
	stream.until(cardSession(card.ID, protocol.SessionStateWaking))
	stream.until(cardSession(card.ID, protocol.SessionStateAwake))
	// Sleeping and waking have not changed the board's own answer: it says the same as the events.
	wantHeld(t, "the board after the wake", st.onBoard(project.ID)[card.ID], protocol.SessionStateAwake, true, false)
}

// The session states survive a daemon restart, which is the "Done when" of B2.15. A card put to
// sleep, one that is paused, and one that is pinned are read back from the board by a new session
// manager and a new server over the same store, before and after the restore that brings the
// awake sessions back. The board is read before anything restores a session, so what it shows can
// only have come from the store.
func TestSessionStatesSurviveARestart(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	sleeper := st.addCard(project.ID, "Sleeper")
	held := st.addCard(project.ID, "Held")
	pinned := st.addCard(project.ID, "Pinned")
	idle := st.addCard(project.ID, "Never started")
	stream := st.dial(protocol.ProjectTopic(project.ID))
	for _, card := range []protocol.Card{sleeper, held, pinned} {
		st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
		stream.until(cardSession(card.ID, protocol.SessionStateAwake))
	}
	st.do(http.MethodPost, "/v1/cards/"+sleeper.ID+"/pause", nil).want(t, http.StatusOK)
	st.do(http.MethodPost, "/v1/cards/"+sleeper.ID+"/sleep", nil).want(t, http.StatusNoContent)
	stream.until(cardSession(sleeper.ID, protocol.SessionStateAsleep))
	st.do(http.MethodPost, "/v1/cards/"+held.ID+"/pause", nil).want(t, http.StatusOK)
	st.do(http.MethodPost, "/v1/cards/"+pinned.ID+"/pin", nil).want(t, http.StatusOK)

	check := func(when string) {
		t.Helper()
		board := st.onBoard(project.ID)
		wantHeld(t, when+", the sleeping card", board[sleeper.ID], protocol.SessionStateAsleep, true, false)
		wantHeld(t, when+", the paused card", board[held.ID], protocol.SessionStateAwake, true, false)
		wantHeld(t, when+", the pinned card", board[pinned.ID], protocol.SessionStateAwake, false, true)
		wantHeld(t, when+", the card with no session", board[idle.ID], "", false, false)
		one := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+sleeper.ID, nil).want(t, http.StatusOK))
		wantHeld(t, when+", the sleeping card read by itself", one, protocol.SessionStateAsleep, true, false)
	}
	check("before the restart")

	oldManager := st.mgr
	st.restart()
	if st.mgr == oldManager {
		t.Fatal("the restart kept the old session manager")
	}
	// Nothing is running yet: this is what a client sees the moment the daemon is back.
	check("after the restart, before the restore")

	if err := st.mgr.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	// A sleeping session stays asleep, and the awake ones are resumed with their holds kept.
	check("after the restore")
}
