package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The wire card carries its session state (docs/architecture.md section 11.5, backlog B2.15), and
// every change to it is announced on the project topic as well as the card's own, because the board
// and Home follow only the project topic.

// collectUntil reads events, in order, until one matches, and returns every event it read, the
// matching one last.
func (e *env) collectUntil(t *testing.T, done func(events.Event) bool) []events.Event {
	t.Helper()
	timeout := time.After(eventTimeout)
	var seen []events.Event
	for {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				t.Fatal("the subscription closed while collecting events")
			}
			seen = append(seen, ev)
			if done(ev) {
				return seen
			}
		case <-timeout:
			t.Fatalf("the event this test waits for never arrived, after %d events", len(seen))
			return nil
		}
	}
}

// cardEventWith matches a card.updated event of this card whose session is in this state.
func cardEventWith(cardID string, state protocol.SessionState) func(events.Event) bool {
	return func(ev events.Event) bool {
		data, ok := ev.Data.(protocol.CardEventData)
		return ev.Type == string(protocol.EventTypeCardUpdated) && ok && data.Card.ID == cardID &&
			data.Card.Session != nil && *data.Card.Session == state
	}
}

// count says how many of the events match.
func count(evs []events.Event, match func(events.Event) bool) int {
	n := 0
	for _, ev := range evs {
		if match(ev) {
			n++
		}
	}
	return n
}

// A sleep and a wake each reach the project topic, once, with the card as it now is. The Home awake
// list hears only that topic, so before this a card that went to sleep stayed on the list.
func TestASleepAndAWakeAreAnnouncedOnTheProjectTopic(t *testing.T) {
	e, project, card := startAndPause(t)

	if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	seen := e.collectUntil(t, cardEventWith(card.ID, protocol.SessionStateAsleep))
	slept := seen[len(seen)-1]
	if slept.Topic != string(protocol.ProjectTopic(project.ID)) || !slept.Critical {
		t.Errorf("the sleep's card.updated = %+v, want a critical event on the project topic", slept)
	}
	if data := slept.Data.(protocol.CardEventData); !data.Card.Paused || data.Card.State != protocol.CardStateWorking {
		t.Errorf("the card in the sleep's card.updated = %+v, want the paused working card", data.Card)
	}

	if err := e.mgr.Wake(context.Background(), card.ID); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	seen = e.collectUntil(t, cardEventWith(card.ID, protocol.SessionStateAwake))
	// Each change is sent once. The sleep was read above, so a second card.updated for it would
	// show up here, before the wake's own.
	want := map[protocol.SessionState]int{
		protocol.SessionStateAsleep: 0, protocol.SessionStateWaking: 1, protocol.SessionStateAwake: 1,
	}
	for state, n := range want {
		if got := count(seen, cardEventWith(card.ID, state)); got != n {
			t.Errorf("%d card.updated events carry the session state %s during the wake, want %d", got, state, n)
		}
	}
	// The state changes reach the card's own topic too.
	if count(seen, func(ev events.Event) bool {
		data, ok := ev.Data.(protocol.SessionStateChangedEventData)
		return ok && ev.Topic == string(protocol.CardTopic(card.ID)) && data.State == protocol.SessionStateWaking
	}) != 1 {
		t.Error("the wake did not reach the card's own topic as session.state_changed")
	}
}

// StoredStates answers from the sessions table, so a card's state is there without any manager
// running: that is what lets it survive a restart.
func TestStoredStatesReadTheSessionRows(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	started := e.card(t, project.ID, "Started")
	idle := e.card(t, project.ID, "Never started")
	states, err := session.NewStoredStates(e.store)
	if err != nil {
		t.Fatalf("NewStoredStates: %v", err)
	}
	if _, err := session.NewStoredStates(nil); err == nil {
		t.Error("NewStoredStates(nil) succeeded, want an error")
	}

	if got, err := states.CardSession(ctx, started.ID); err != nil || got != nil {
		t.Errorf("the state of a card with no session = %v, %v, want nil", got, err)
	}
	if _, err := e.mgr.Start(ctx, started.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilState(t, started.ID, protocol.SessionStateAwake)

	got, err := states.CardSession(ctx, started.ID)
	if err != nil || got == nil || got.State != protocol.SessionStateAwake || got.View != protocol.CardViewModeChat {
		t.Errorf("the state of the started card = %v, %v, want awake in the chat view", got, err)
	}
	// A chat's session belongs to no card, and its empty card id must not be read as one.
	seedChatSession(t, e, project.ID)
	if got, err := states.CardSession(ctx, ""); err != nil || got != nil {
		t.Errorf("the state of the empty card id = %v, %v, want nil: a chat's session is not a card's", got, err)
	}
	all, err := states.ProjectSessions(ctx, project.ID)
	if err != nil {
		t.Fatalf("ProjectSessions: %v", err)
	}
	if len(all) != 1 || all[started.ID].State != protocol.SessionStateAwake || all[started.ID].View != protocol.CardViewModeChat {
		t.Errorf("the project's session states = %v, want only the started card's, awake", all)
	}
	if _, ok := all[idle.ID]; ok {
		t.Error("a card that never started is in the project's session states")
	}
}

// seedChatSession writes a chat and its own session row straight into the store, the way the
// chats service does when a chat is made, so a test can check that a chat's session stays out of
// what belongs to cards.
func seedChatSession(t *testing.T, e *env, projectID string) (chatID, sessionID string) {
	t.Helper()
	chatID, sessionID = "01M3C107JB041061050R3GG28A", "01M3C107JB041061050R3GG28B"
	now := time.Now().UnixMilli()
	err := e.store.Write(context.Background(), func(q *db.Queries) error {
		if err := q.CreateChat(context.Background(), db.CreateChatParams{
			ID: chatID, ProjectID: projectID, Title: "New chat", TargetKind: "orchestrator",
			AgentKind: "claude", PermissionMode: "auto-edits", LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return q.CreateChatSession(context.Background(), db.CreateChatSessionParams{
			ID: sessionID, ChatID: &chatID, AgentKind: "claude", State: string(protocol.SessionStateAwake),
			PermissionMode: "auto-edits", LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("seed a chat and its session: %v", err)
	}
	return chatID, sessionID
}
