package projects_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeStates stands in for the session module, which owns the sessions table: the projects service
// only asks it what state and view a card's session is in (docs/architecture.md section 3).
type fakeStates struct {
	mu    sync.Mutex
	infos map[string]projects.SessionInfo
	err   error
}

// set records a card's session state, in the chat view.
func (f *fakeStates) set(cardID string, state protocol.SessionState) {
	f.setView(cardID, state, protocol.CardViewModeChat)
}

// setView records a card's session state and the view its agent runs in.
func (f *fakeStates) setView(cardID string, state protocol.SessionState, view protocol.CardViewMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.infos == nil {
		f.infos = map[string]projects.SessionInfo{}
	}
	f.infos[cardID] = projects.SessionInfo{State: state, View: view}
}

func (f *fakeStates) CardSession(_ context.Context, cardID string) (*projects.SessionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	info, ok := f.infos[cardID]
	if !ok {
		return nil, nil
	}
	return &info, nil
}

func (f *fakeStates) ProjectSessions(context.Context, string) (map[string]projects.SessionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]projects.SessionInfo, len(f.infos))
	for id, info := range f.infos {
		out[id] = info
	}
	return out, nil
}

// wantSession fails the test unless the card's session is this state, or null when want is empty.
func wantSession(t *testing.T, what string, card protocol.Card, want protocol.SessionState) {
	t.Helper()
	switch {
	case want == "" && card.Session != nil:
		t.Errorf("%s: session = %q, want null", what, *card.Session)
	case want != "" && card.Session == nil:
		t.Errorf("%s: session is null, want %q", what, want)
	case want != "" && *card.Session != want:
		t.Errorf("%s: session = %q, want %q", what, *card.Session, want)
	}
}

// The wire card carries its session state wherever a card is read or sent, and null for a card that
// has never had a session, so a client never sees the field undefined.
func TestEveryCardCarriesItsSessionState(t *testing.T) {
	states := &fakeStates{}
	e := newEnv(t, projects.WithSessionStates(states))
	ctx := context.Background()
	project := e.folder(t, "small-repo")
	asleep := e.card(t, project.ID, "Asleep")
	never := e.card(t, project.ID, "Never started")
	wantSession(t, "a new card", asleep, "")
	states.set(asleep.ID, protocol.SessionStateAsleep)

	board, err := e.svc.Board(ctx, project.ID)
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	for _, card := range board.Cards {
		switch card.ID {
		case asleep.ID:
			wantSession(t, "the board's sleeping card", card, protocol.SessionStateAsleep)
		case never.ID:
			wantSession(t, "the board's card with no session", card, "")
		}
	}
	list, err := e.svc.Cards(ctx, project.ID)
	if err != nil {
		t.Fatalf("Cards: %v", err)
	}
	for _, card := range list {
		if card.ID == asleep.ID {
			wantSession(t, "the card list", card, protocol.SessionStateAsleep)
		}
	}
	one, err := e.svc.Card(ctx, asleep.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	wantSession(t, "Card", one, protocol.SessionStateAsleep)
	byKey, err := e.svc.CardByKey(ctx, protocol.CardKey{ProjectID: project.ID, Number: asleep.Number})
	if err != nil {
		t.Fatalf("CardByKey: %v", err)
	}
	wantSession(t, "CardByKey", byKey, protocol.SessionStateAsleep)
}

// Every card the service answers with or publishes after a change carries the session too. An
// event carries the card as it now is, not a difference, so one sent with a null session would
// wipe an awake card's state off a client.
func TestEveryChangeAnswersAndPublishesTheSessionState(t *testing.T) {
	states := &fakeStates{}
	e := newEnv(t, projects.WithSessionStates(states))
	ctx := context.Background()
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Awake")
	states.set(card.ID, protocol.SessionStateAwake)
	topic := protocol.ProjectTopic(project.ID)
	yes := true
	title := "Renamed"

	e.drainEvents()
	held, err := e.svc.SetHold(ctx, card.ID, &yes, nil)
	if err != nil {
		t.Fatalf("SetHold: %v", err)
	}
	wantSession(t, "SetHold's answer", held, protocol.SessionStateAwake)
	eventCard(t, e.nextType(t, protocol.EventTypeCardUpdated, topic), "the hold's card.updated", protocol.SessionStateAwake)

	updated, err := e.svc.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Title: &title})
	if err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	wantSession(t, "UpdateCard's answer", updated, protocol.SessionStateAwake)
	eventCard(t, e.nextType(t, protocol.EventTypeCardUpdated, topic), "the edit's card.updated", protocol.SessionStateAwake)

	states.set(card.ID, protocol.SessionStateWorking)
	moved, err := e.svc.MoveCard(ctx, card.ID, protocol.MoveCardRequest{State: protocol.CardStateWorking})
	if err != nil {
		t.Fatalf("MoveCard: %v", err)
	}
	wantSession(t, "MoveCard's answer", moved, protocol.SessionStateWorking)
	movedEvent := e.nextType(t, protocol.EventTypeCardMoved, topic)
	if data, ok := movedEvent.Data.(protocol.CardMovedEventData); !ok {
		t.Errorf("card.moved carries %T", movedEvent.Data)
	} else {
		wantSession(t, "card.moved", data.Card, protocol.SessionStateWorking)
	}

	stateful, err := e.svc.SetState(ctx, card.ID, protocol.CardStateNeeds)
	if err != nil {
		t.Fatalf("SetState: %v", err)
	}
	wantSession(t, "SetState's answer", stateful, protocol.SessionStateWorking)
}

// eventCard checks the session of the card a card.updated event carries.
func eventCard(t *testing.T, ev events.Event, what string, want protocol.SessionState) {
	t.Helper()
	data, ok := ev.Data.(protocol.CardEventData)
	if !ok {
		t.Fatalf("%s carries %T, want a card", what, ev.Data)
	}
	wantSession(t, what, data.Card, want)
}

// SessionChanged is how the session module tells the board and Home that a card's session moved:
// it publishes card.updated with the card as it now is, as a critical event, on the project topic.
func TestSessionChangedPublishesTheCardOnTheProjectTopic(t *testing.T) {
	states := &fakeStates{}
	e := newEnv(t, projects.WithSessionStates(states))
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Goes to sleep")
	states.set(card.ID, protocol.SessionStateAsleep)
	e.drainEvents()

	e.svc.SessionChanged(context.Background(), card.ID)
	ev := e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID))
	data, ok := ev.Data.(protocol.CardEventData)
	if !ok || data.Card.ID != card.ID {
		t.Fatalf("card.updated carries %+v, want the card", ev.Data)
	}
	wantSession(t, "the session change's card.updated", data.Card, protocol.SessionStateAsleep)
	if !ev.Critical {
		t.Error("a session change must be critical: a client that missed it would keep drawing the card awake")
	}
	e.noEvent(t)

	// A card that is not there has nothing to announce, and that is logged, not raised.
	e.svc.SessionChanged(context.Background(), "01M3C107JB041061050R3GG28A")
	e.noEvent(t)
}

// A session state that cannot be read is an error. Sending null instead would tell a client that
// the card's agent is gone.
func TestASessionStateThatCannotBeReadIsAnError(t *testing.T) {
	states := &fakeStates{}
	e := newEnv(t, projects.WithSessionStates(states))
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Unreadable")
	states.err = errors.New("the database is locked")

	if _, err := e.svc.Card(context.Background(), card.ID); err == nil {
		t.Error("Card answered although the session state could not be read")
	}
	if _, err := e.svc.Board(context.Background(), project.ID); err == nil {
		t.Error("Board answered although the session states could not be read")
	}
}

// The view a card's agent runs in rides with its session on every card the service sends: chat for a
// card that never had a session, and whatever the session module stored for one that did.
func TestEveryCardCarriesTheViewItsAgentRunsIn(t *testing.T) {
	states := &fakeStates{}
	e := newEnv(t, projects.WithSessionStates(states))
	ctx := context.Background()
	project := e.folder(t, "small-repo")
	terminal := e.card(t, project.ID, "In the terminal")
	chat := e.card(t, project.ID, "In the chat")
	never := e.card(t, project.ID, "Never started")
	for _, card := range []protocol.Card{terminal, chat, never} {
		if card.ViewMode != protocol.CardViewModeChat {
			t.Errorf("a new card is in the %q view, want chat", card.ViewMode)
		}
	}
	states.setView(terminal.ID, protocol.SessionStateAwake, protocol.CardViewModeTerminal)
	states.setView(chat.ID, protocol.SessionStateAwake, protocol.CardViewModeChat)

	want := map[string]protocol.CardViewMode{
		terminal.ID: protocol.CardViewModeTerminal, chat.ID: protocol.CardViewModeChat, never.ID: protocol.CardViewModeChat,
	}
	board, err := e.svc.Board(ctx, project.ID)
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	for _, card := range board.Cards {
		if card.ViewMode != want[card.ID] {
			t.Errorf("the board has %s in the %q view, want %q", card.Title, card.ViewMode, want[card.ID])
		}
	}
	one, err := e.svc.Card(ctx, terminal.ID)
	if err != nil || one.ViewMode != protocol.CardViewModeTerminal || one.Session == nil || *one.Session != protocol.SessionStateAwake {
		t.Errorf("Card = view %q, session %v, %v, want an awake session in the terminal view", one.ViewMode, one.Session, err)
	}
	// SessionChanged is what tells the board a card changed views, so its card.updated carries the view.
	e.drainEvents()
	e.svc.SessionChanged(ctx, terminal.ID)
	data, ok := e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID)).Data.(protocol.CardEventData)
	if !ok || data.Card.ViewMode != protocol.CardViewModeTerminal {
		t.Errorf("card.updated carries %+v, want the card in the terminal view", data)
	}
}
