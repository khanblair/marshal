package session_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Session hold (checklist item B2.15): pause, sleep, wake, and pin, under the rules of
// docs/architecture.md section 5.1, and the Start that continues a stopped, asleep, or paused
// card's own session.

// untilState reads events until a card's session reports this state, and returns that payload and
// whether the event was critical. It exists beside untilType because several events of the same
// type arrive in a row (waking, then awake), and a test must be able to name the one it means.
func (e *env) untilState(t *testing.T, cardID string, state protocol.SessionState) (protocol.SessionStateChangedEventData, bool) {
	t.Helper()
	timeout := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				t.Fatalf("the subscription closed while waiting for %s", state)
			}
			data, ok := ev.Data.(protocol.SessionStateChangedEventData)
			if ev.Type != string(protocol.EventTypeSessionStateChanged) || !ok ||
				data.CardID != cardID || data.State != state {
				continue
			}
			return data, ev.Critical
		case <-timeout:
			t.Fatalf("no %s event arrived for card %s", state, cardID)
			return protocol.SessionStateChangedEventData{}, false
		}
	}
}

// untilCard reads events until a card.updated event for this card matches, and returns the card it
// carried. A start publishes card.updated of its own (the worktree), so a test that means "the
// pause reached every client" has to say which change it is waiting for.
func (e *env) untilCard(t *testing.T, cardID string, match func(protocol.Card) bool) protocol.Card {
	t.Helper()
	timeout := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				t.Fatalf("the subscription closed while waiting for a card.updated of %s", cardID)
			}
			if ev.Type != string(protocol.EventTypeCardUpdated) {
				continue
			}
			data, ok := ev.Data.(protocol.CardEventData)
			if !ok || data.Card.ID != cardID || !match(data.Card) {
				continue
			}
			return data.Card
		case <-timeout:
			t.Fatalf("no matching card.updated arrived for card %s", cardID)
			return protocol.Card{}
		}
	}
}

// startAndPause builds an env with a working card that a pause holds, which is where most of the
// sleep and wake rules begin. It consumes the events the start and the pause published, so the
// events a test waits for afterwards are its own.
func startAndPause(t *testing.T) (*env, protocol.Project, protocol.Card) {
	t.Helper()
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Hold me")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	held, err := e.mgr.Pause(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if !held.Paused {
		t.Fatalf("the card is not paused after Pause: %+v", held)
	}
	if got := e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.Paused }); got.Paused != held.Paused {
		t.Fatalf("the pause event carries %+v", got)
	}
	return e, project, card
}

// fakeStarts says how many brand new sessions the fake agent was asked to start, which is how a
// test tells "this session resumed" apart from "a second process was started".
func fakeStarts(a *fakeAgent) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.nextID
}

// fakeTurns says how many turns the fake agent has answered on a session.
func fakeTurns(s *fakeSession) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turns
}

// Only a working card can be paused, and a refused pause leaves the card exactly as it was.
func TestPauseRefusesACardThatIsNotWorking(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Not started")

	_, err := e.mgr.Pause(context.Background(), card.ID)
	perr := wantCode(t, err, protocol.ErrorCodeRefused)
	if perr.Message != "Only working cards can be paused." {
		t.Errorf("message = %q", perr.Message)
	}
	if got := perr.Details["reason"]; got != string(protocol.HoldRefusalReasonPauseNotWorking) {
		t.Errorf("reason = %q, want %q", got, protocol.HoldRefusalReasonPauseNotWorking)
	}
	after, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if after.Paused || after.State != protocol.CardStateBacklog || after.Pinned {
		t.Errorf("a refused pause changed the card: %+v", after)
	}
}

// A working card can be paused, every client hears about it through card.updated, and pausing it
// again changes nothing.
func TestPauseHoldsAWorkingCardAndTellsEveryClient(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Hold me")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	held, err := e.mgr.Pause(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if !held.Paused || held.State != protocol.CardStateWorking {
		t.Fatalf("the paused card = %+v, want a working card with paused set", held)
	}
	if got := e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.Paused }); !got.Paused {
		t.Errorf("card.updated carries %+v, want the pause", got)
	}
	again, err := e.mgr.Pause(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Pause again: %v", err)
	}
	if !again.Paused {
		t.Errorf("pausing an already paused card gave %+v", again)
	}
}

// A message sent to a paused card waits: it is not history yet, no turn starts, and the person's
// next press (here Unpause) delivers it to the same session.
func TestAMessageWaitsWhileACardIsPaused(t *testing.T) {
	e, _, card := startAndPause(t)
	if err := e.mgr.Send(context.Background(), card.ID, "wait for the pause"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if stored := e.historyOf(t, card.ID); len(stored) != 0 {
		t.Fatalf("a message sent during a pause was stored already: %+v", stored)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateAwake) {
		t.Errorf("session state while paused = %s, want the awake session the pause found", row.State)
	}

	if _, err := e.mgr.Unpause(context.Background(), card.ID); err != nil {
		t.Fatalf("Unpause: %v", err)
	}
	waitForHistory(t, e, card.ID, 1)
	if sent := oneKind(t, e.historyOf(t, card.ID), history.KindUser); sent.Summary != "wait for the pause" {
		t.Errorf("the delivered message = %+v", sent)
	}
	e.untilType(t, protocol.EventTypeSessionOutput)
	answered := oneKind(t, e.historyOf(t, card.ID), history.KindAgent)
	if !strings.Contains(answered.Summary, "wait for the pause") {
		t.Errorf("the agent answered %q, want the message the pause was holding", answered.Summary)
	}
}

// A turn that is running when the card is paused finishes, and the message queued behind it waits
// for the person rather than starting a second turn on its own.
func TestAPauseHoldsTheMessageQueuedBehindARunningTurn(t *testing.T) {
	e := newEnv(t)
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold); _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Hold the queue")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send(first): %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWorking)
	if _, err := e.mgr.Pause(context.Background(), card.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "second"); err != nil {
		t.Fatalf("Send(second): %v", err)
	}

	e.agent.hold <- struct{}{} // let the first turn end
	waitForHistory(t, e, card.ID, 2)
	// The pause must keep "second" in the queue: give the pump the moment it needs to reach that
	// decision before looking at the fake agent's turn count.
	time.Sleep(20 * time.Millisecond)
	if _, fs := e.liveFakeSession(t, card.ID); fakeTurns(fs) != 1 {
		t.Fatalf("the pause let a second turn start: the agent answered %d turns, want 1", fakeTurns(fs))
	}

	if _, err := e.mgr.Unpause(context.Background(), card.ID); err != nil {
		t.Fatalf("Unpause: %v", err)
	}
	e.agent.hold <- struct{}{} // let the second turn end
	waitForHistory(t, e, card.ID, 4)
	if _, fs := e.liveFakeSession(t, card.ID); fakeTurns(fs) != 2 {
		t.Errorf("after the resume the agent answered %d turns, want 2", fakeTurns(fs))
	}
}

// Start on a paused card releases the hold instead of starting anything again: the process that
// was already running is the one that delivers the message the pause was holding.
func TestStartOnAPausedCardReleasesTheHoldWithoutStartingAgain(t *testing.T) {
	e, _, card := startAndPause(t)
	before := fakeStarts(e.agent)

	updated, err := e.mgr.Start(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if updated.Paused {
		t.Errorf("Start left the pause on: %+v", updated)
	}
	if got := fakeStarts(e.agent); got != before {
		t.Errorf("Start started %d sessions, want no new one (it was %d before)", got, before)
	}
}

// The three sleep rules of architecture.md 5.1, each with its own sentence and reason code.
func TestSleepRefusals(t *testing.T) {
	t.Run("a working card that is not paused", func(t *testing.T) {
		e := newEnv(t)
		t.Cleanup(func() { _ = e.mgr.Close() })
		project := e.project(t, "small-repo")
		card := e.card(t, project.ID, "Busy")
		if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
			t.Fatalf("Start: %v", err)
		}
		perr := wantCode(t, e.mgr.Sleep(context.Background(), card.ID), protocol.ErrorCodeRefused)
		if perr.Message != "Working cards don't sleep. Pause the card first." {
			t.Errorf("message = %q", perr.Message)
		}
		if got := perr.Details["reason"]; got != string(protocol.HoldRefusalReasonSleepWorking) {
			t.Errorf("reason = %q", got)
		}
		if e.mgr.RecentOutput(card.ID) == nil {
			t.Error("a refused sleep stopped the session")
		}
	})

	t.Run("a card that is waiting on a person", func(t *testing.T) {
		e := newEnv(t)
		t.Cleanup(func() { _ = e.mgr.Close() })
		project := e.project(t, "small-repo")
		card := e.card(t, project.ID, "Needs you")
		if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if _, err := e.proj.SetState(context.Background(), card.ID, protocol.CardStateNeeds); err != nil {
			t.Fatalf("SetState(needs): %v", err)
		}
		perr := wantCode(t, e.mgr.Sleep(context.Background(), card.ID), protocol.ErrorCodeRefused)
		if perr.Message != "This card is waiting on you, so it stays awake." {
			t.Errorf("message = %q", perr.Message)
		}
		if got := perr.Details["reason"]; got != string(protocol.HoldRefusalReasonSleepNeedsYou) {
			t.Errorf("reason = %q", got)
		}
	})

	t.Run("a card that was never started", func(t *testing.T) {
		e := newEnv(t)
		t.Cleanup(func() { _ = e.mgr.Close() })
		project := e.project(t, "small-repo")
		card := e.card(t, project.ID, "Nothing running")
		perr := wantCode(t, e.mgr.Sleep(context.Background(), card.ID), protocol.ErrorCodeRefused)
		if perr.Message != "This card has no awake session." {
			t.Errorf("message = %q", perr.Message)
		}
		if got := perr.Details["reason"]; got != string(protocol.HoldRefusalReasonSleepNoSession) {
			t.Errorf("reason = %q", got)
		}
	})

	t.Run("a card that is already asleep", func(t *testing.T) {
		e, _, card := startAndPause(t)
		if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
			t.Fatalf("Sleep: %v", err)
		}
		perr := wantCode(t, e.mgr.Sleep(context.Background(), card.ID), protocol.ErrorCodeRefused)
		if got := perr.Details["reason"]; got != string(protocol.HoldRefusalReasonSleepNoSession) {
			t.Errorf("reason = %q", got)
		}
	})

	t.Run("a pause that is still holding a message", func(t *testing.T) {
		e, _, card := startAndPause(t)
		if err := e.mgr.Send(context.Background(), card.ID, "still waiting"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		perr := wantCode(t, e.mgr.Sleep(context.Background(), card.ID), protocol.ErrorCodeRefused)
		if perr.Message != "This card has a message waiting for you to resume it. Resume the card first." {
			t.Errorf("message = %q", perr.Message)
		}
		if got := perr.Details["reason"]; got != string(protocol.HoldRefusalReasonSleepHoldingMessages) {
			t.Errorf("reason = %q", got)
		}
	})
}

// Sleep stops the process, keeps the session row and its agent session id, records the session as
// asleep, and leaves the worktree and branch in place.
func TestSleepStopsTheProcessAndKeepsTheSessionID(t *testing.T) {
	e, project, card := startAndPause(t)
	before, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	pathBefore, branchBefore, err := e.proj.Worktree(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	data, critical := e.untilState(t, card.ID, protocol.SessionStateAsleep)
	if data.SessionID != before.ID || data.CardID != card.ID {
		t.Errorf("the asleep event = %+v, want the card and its own session", data)
	}
	if !critical {
		t.Error("session.state_changed must be critical")
	}
	after, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if after.ID != before.ID || after.AgentSessionID != before.AgentSessionID {
		t.Errorf("the session row became %+v, want the same row and the same agent session id", after)
	}
	if after.State != string(protocol.SessionStateAsleep) {
		t.Errorf("session state after Sleep = %s, want asleep", after.State)
	}
	if e.mgr.RecentOutput(card.ID) != nil {
		t.Error("the agent process is still live after a sleep")
	}
	if awake, err := e.mgr.AwakeCards(context.Background(), project.ID); err != nil || awake != 0 {
		t.Errorf("AwakeCards = %d (error %v), want 0", awake, err)
	}
	pathAfter, branchAfter, err := e.proj.Worktree(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if pathAfter != pathBefore || branchAfter != branchBefore {
		t.Errorf("the worktree became %q/%q, want %q/%q kept", pathAfter, branchAfter, pathBefore, branchBefore)
	}
}

// A sleep whose agent refuses to stop does not record the session as asleep, the way a Stop that
// fails does not record it as stopped.
func TestSleepThatCannotStopTheAgentChangesNothing(t *testing.T) {
	e, _, card := startAndPause(t)
	e.agent.stopErr = errors.New("the process will not die")
	t.Cleanup(func() { e.agent.stopErr = nil })

	err := e.mgr.Sleep(context.Background(), card.ID)
	if err == nil || !strings.Contains(err.Error(), "the process will not die") {
		t.Fatalf("Sleep error = %v, want it to wrap the agent's own error", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State == string(protocol.SessionStateAsleep) {
		t.Error("a session that did not stop must not be recorded as asleep")
	}
	if e.mgr.RecentOutput(card.ID) == nil {
		t.Error("the session must still be live when the agent refused to stop")
	}
}

// Wake resumes the sleeping session through its saved id, records waking and then awake, and the
// conversation carries on where it left off.
func TestWakeBringsTheSameSessionBackWithItsContext(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Sleep and wake")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	if err := e.mgr.Send(context.Background(), card.ID, "first question"); err != nil {
		t.Fatalf("Send(first): %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	if _, err := e.mgr.Pause(context.Background(), card.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	before, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAsleep)

	if err := e.mgr.Wake(context.Background(), card.ID); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	waking, _ := e.untilState(t, card.ID, protocol.SessionStateWaking)
	if waking.SessionID != before.ID {
		t.Errorf("the waking event = %+v, want the card's own session", waking)
	}
	awake, _ := e.untilState(t, card.ID, protocol.SessionStateAwake)
	if awake.SessionID != before.ID {
		t.Errorf("the awake event = %+v, want the card's own session", awake)
	}
	after, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if after.State != string(protocol.SessionStateAwake) || after.AgentSessionID != before.AgentSessionID {
		t.Errorf("the row after Wake = %+v, want awake on the same agent session id %q", after, before.AgentSessionID)
	}

	// Wake leaves the pause where it was (the card panel offers Resume next), so the person
	// releases it and the next message continues the same conversation.
	if _, err := e.mgr.Unpause(context.Background(), card.ID); err != nil {
		t.Fatalf("Unpause: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "second question"); err != nil {
		t.Fatalf("Send(second): %v", err)
	}
	waitForHistory(t, e, card.ID, 4)
	answered := kindEvents(t, e.historyOf(t, card.ID), history.KindAgent)
	if len(answered) == 0 || !strings.Contains(answered[0].Summary, "turn 2 remembers 1 earlier turns") {
		t.Errorf("the answer after the wake = %+v, want the session to remember the first turn", answered)
	}
}

// Wake leaves a pause alone, and Start is what releases it.
func TestWakeLeavesAPauseAndStartReleasesIt(t *testing.T) {
	e, _, card := startAndPause(t)
	if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if err := e.mgr.Wake(context.Background(), card.ID); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	afterWake, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if !afterWake.Paused {
		t.Error("Wake must leave the pause where it was")
	}
	released, err := e.mgr.Start(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if released.Paused {
		t.Errorf("Start left the card paused: %+v", released)
	}
}

// A message sent to a sleeping card wakes it, releases the pause that let it sleep, and reaches
// the agent: the card panel's own words are "Message 41. This wakes the session."
func TestAMessageWakesASleepingSession(t *testing.T) {
	e, _, card := startAndPause(t)
	if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAsleep)

	if err := e.mgr.Send(context.Background(), card.ID, "good morning"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWaking)
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	woken, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if woken.Paused {
		t.Error("waking a card with a message left the pause on, so the message could never be delivered")
	}
	waitForHistory(t, e, card.ID, 1)
	if sent := oneKind(t, e.historyOf(t, card.ID), history.KindUser); sent.Summary != "good morning" {
		t.Errorf("the stored message = %+v", sent)
	}
}

// Wake does nothing to a session that is already awake, is not found for a card with no session
// at all, and is refused for one that has stopped.
func TestWakeRefusals(t *testing.T) {
	e, _, card := startAndPause(t)
	if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if err := e.mgr.Wake(context.Background(), card.ID); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if err := e.mgr.Wake(context.Background(), card.ID); err != nil {
		t.Errorf("waking an awake session = %v, want nothing to do", err)
	}

	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	perr := wantCode(t, e.mgr.Wake(context.Background(), card.ID), protocol.ErrorCodeRefused)
	if perr.Message != "This card's session has stopped and cannot be resumed." {
		t.Errorf("message = %q", perr.Message)
	}

	err := e.mgr.Wake(context.Background(), "no-such-card")
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
}

// Pin and unpin record the person's choice on the card and publish it, the way any other card
// field does.
func TestPinAndUnpinRecordTheChoiceOnTheCard(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Keep me awake")

	pinned, err := e.mgr.Pin(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if !pinned.Pinned {
		t.Fatalf("Pin gave %+v", pinned)
	}
	if got := e.untilCard(t, card.ID, func(c protocol.Card) bool { return c.Pinned }); !got.Pinned {
		t.Errorf("the pin event carries %+v", got)
	}
	again, err := e.mgr.Pin(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Pin again: %v", err)
	}
	if !again.Pinned {
		t.Errorf("pinning a pinned card gave %+v", again)
	}

	unpinned, err := e.mgr.Unpin(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Unpin: %v", err)
	}
	if unpinned.Pinned {
		t.Errorf("Unpin gave %+v", unpinned)
	}
	if got := e.untilCard(t, card.ID, func(c protocol.Card) bool { return !c.Pinned }); got.Pinned {
		t.Errorf("the unpin event carries %+v", got)
	}
}

// Start on a card whose session has stopped continues that session instead of refusing, and the
// conversation keeps its context. This is the owner's decision of 2026-09-26.
func TestStartOnAStoppedSessionContinuesTheSameConversation(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Stopped but not lost")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	if err := e.mgr.Send(context.Background(), card.ID, "first question"); err != nil {
		t.Fatalf("Send(first): %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateStopped)
	before, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if before.State != string(protocol.SessionStateStopped) {
		t.Fatalf("session state after Stop = %s, want stopped", before.State)
	}

	updated, err := e.mgr.Start(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Start on a stopped session: %v", err)
	}
	if updated.State != protocol.CardStateWorking {
		t.Errorf("card state after the start = %s, want working", updated.State)
	}
	after, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if after.ID != before.ID || after.AgentSessionID != before.AgentSessionID || after.State != string(protocol.SessionStateAwake) {
		t.Errorf("the row after the start = %+v, want the same session awake", after)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "second question"); err != nil {
		t.Fatalf("Send(second): %v", err)
	}
	waitForHistory(t, e, card.ID, 4)
	answered := kindEvents(t, e.historyOf(t, card.ID), history.KindAgent)
	if len(answered) == 0 || !strings.Contains(answered[0].Summary, "turn 2 remembers 1 earlier turns") {
		t.Errorf("the answer after the start = %+v, want the session to remember the first turn", answered)
	}
}

// A sleeping session survives a restart as asleep, nothing resumes it on its own, and the person
// wakes the same session with its context afterwards. This is the checklist's own "Done when".
func TestASleepingSessionSurvivesARestart(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Sleep through a restart")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	if err := e.mgr.Send(context.Background(), card.ID, "first question"); err != nil {
		t.Fatalf("Send(first): %v", err)
	}
	waitForHistory(t, e, card.ID, 2)
	if _, err := e.mgr.Pause(context.Background(), card.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	before, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if err := e.mgr.Sleep(context.Background(), card.ID); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fresh := e.reopen(t)
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	after, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if after.State != string(protocol.SessionStateAsleep) {
		t.Fatalf("session state after the restart = %s, want asleep: sleep is a person's decision", after.State)
	}
	if fresh.RecentOutput(card.ID) != nil {
		t.Error("a sleeping session was resumed by the restart")
	}
	held, err := e.proj.Card(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if !held.Paused {
		t.Error("the pause did not survive the restart")
	}

	if err := fresh.Wake(context.Background(), card.ID); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWaking)
	awake, _ := e.untilState(t, card.ID, protocol.SessionStateAwake)
	if awake.SessionID != before.ID {
		t.Errorf("the woken session = %q, want %q", awake.SessionID, before.ID)
	}
	woken, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if woken.State != string(protocol.SessionStateAwake) || woken.AgentSessionID != before.AgentSessionID {
		t.Errorf("the row after waking = %+v, want awake on the agent session id %q", woken, before.AgentSessionID)
	}
	if _, err := fresh.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := fresh.Send(context.Background(), card.ID, "second question"); err != nil {
		t.Fatalf("Send(second): %v", err)
	}
	waitForHistory(t, e, card.ID, 4)
	answered := kindEvents(t, e.historyOf(t, card.ID), history.KindAgent)
	if len(answered) == 0 || !strings.Contains(answered[0].Summary, "turn 2 remembers 1 earlier turns") {
		t.Errorf("the answer after the restart = %+v, want the session to remember the first turn", answered)
	}
}

// A daemon that stopped in the middle of a wake left a row reading waking and nothing running.
// The restart picks that row up, so the card is not stranded as waking forever.
func TestARestartPicksUpASessionLeftWaking(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Half woken")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateAwake)
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err = e.store.Write(context.Background(), func(q *db.Queries) error {
		_, err := q.UpdateSessionRuntime(context.Background(), db.UpdateSessionRuntimeParams{
			State: string(protocol.SessionStateWaking), AgentSessionID: row.AgentSessionID,
			LastActiveAt: row.LastActiveAt, UpdatedAt: row.UpdatedAt, ID: row.ID,
		})
		return err
	})
	if err != nil {
		t.Fatalf("write a waking row: %v", err)
	}

	fresh := e.reopen(t)
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	after, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if after.State != string(protocol.SessionStateAwake) {
		t.Errorf("session state after the restart = %s, want awake", after.State)
	}
	if after.AgentSessionID != row.AgentSessionID {
		t.Errorf("the resumed row = %+v, want the agent session id %q", after, row.AgentSessionID)
	}
}
