package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The queue and the turn: a delivery that fails must release the turn, Send must write "working"
// before it asks the agent, and a turn that ends while Stop is running must not resurrect the
// session. The rest of the manager's rules are in manager_test.go, with the helpers both files use.

func TestAFailedQueuedDeliveryReleasesTheTurn(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold) }) // registered after Close, so it runs first
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "second"); err != nil {
		t.Fatalf("Send(second): %v", err)
	}
	e.agent.setSendErr(errors.New("cannot deliver right now"))
	e.agent.hold <- struct{}{} // let the first turn finish; onTurnEnded tries and fails to deliver "second"

	awake := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := awake.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateAwake {
		t.Fatalf("state after the first turn ended = %+v, want awake even though delivery failed", awake.Data)
	}
	// onTurnEnded publishes "awake" before it tries to deliver the queued message, so give that
	// attempt a moment to actually run (and fail) before resetting sendErr: otherwise the reset
	// can win a race against the pump's own delivery attempt and this would test nothing.
	time.Sleep(50 * time.Millisecond)
	e.agent.setSendErr(nil)
	// The failed delivery must still release the turn, so an ordinary send is accepted directly
	// (not queued behind a message that will never be delivered): a "working" event follows at
	// once, which only happens for a direct send, never for one that was merely queued.
	if err := e.mgr.Send(context.Background(), card.ID, "third"); err != nil {
		t.Errorf("Send after a failed queued delivery: %v", err)
	}
	working := e.untilType(t, protocol.EventTypeSessionStateChanged)
	if data, _ := working.Data.(protocol.SessionStateChangedEventData); data.State != protocol.SessionStateWorking {
		t.Errorf("state after the third send = %+v, want working (the turn must have been released)", working.Data)
	}
}

// TestSendMarksTheSessionWorkingBeforeAskingTheAgent proves Send writes "working" before it calls
// agent.Send, not after: a fast turn (a real agent, or the stub at full speed) can otherwise reach
// the pump's onTurnEnded, which writes "awake", before Send's own goroutine gets back to writing
// "working", leaving the row stuck on "working" for a session that is actually idle. The fake
// agent's beforeSend hook runs synchronously inside Send itself, before it decides anything, so
// this checks the ordering directly instead of trying to win a real race.
func TestSendMarksTheSessionWorkingBeforeAskingTheAgent(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var stateAtSend string
	e.agent.beforeSend = func() {
		row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
		if err != nil {
			t.Errorf("GetSessionByCard inside beforeSend: %v", err)
			return
		}
		stateAtSend = row.State
	}
	if err := e.mgr.Send(context.Background(), card.ID, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if stateAtSend != string(protocol.SessionStateWorking) {
		t.Errorf("session state at the moment agent.Send was called = %q, want %q", stateAtSend, protocol.SessionStateWorking)
	}
}

// TestStopDuringATurnDoesNotResurrectTheSession proves that a TurnEnded which arrives because of
// Stop's own graceful cancel (a real adapter, per docs/architecture.md 4.1's Interrupt rule, can
// end an in-flight turn with TurnEnded(cancelled) before the process actually exits) cannot undo
// what Stop already wrote: the row must stay "stopped", never bounce back to "awake", and no
// queued message may be delivered to a session that is on its way out.
func TestStopDuringATurnDoesNotResurrectTheSession(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.hold = make(chan struct{})
	t.Cleanup(func() { close(e.agent.hold) })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "queued behind the held turn"); err != nil {
		t.Fatalf("Send(queued): %v", err)
	}

	// The turn is held open (blocked inside the fake's runTurn) when Stop is called: the fake
	// agent's Stop (see fakeAgent.Stop) sees the session busy and queues TurnEnded(cancelled)
	// before Exited, exactly mirroring a real adapter's graceful stop.
	if err := e.mgr.Stop(context.Background(), card.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	if row.State != string(protocol.SessionStateStopped) {
		t.Fatalf("session row state right after Stop = %s, want stopped", row.State)
	}

	// Exactly three events were published before the held turn was ever released: awake (Start),
	// working (the first Send), and stopped (Stop itself, synchronously, before it returned).
	// Draining exactly these lets the check after prove nothing else follows.
	wantStates := []protocol.SessionState{protocol.SessionStateAwake, protocol.SessionStateWorking, protocol.SessionStateStopped}
	for _, want := range wantStates {
		ev := e.untilType(t, protocol.EventTypeSessionStateChanged)
		if data, _ := ev.Data.(protocol.SessionStateChangedEventData); data.State != want {
			t.Fatalf("state = %+v, want %s", ev.Data, want)
		}
	}

	// Every state change is announced twice: session.state_changed on the card's own topic, and
	// card.updated with the card as it now is on the project's, which is what the board and Home
	// hear. So "stopped" is followed, at once and from the same goroutine, by exactly one
	// card.updated that carries it. It is read here rather than skipped, so the check below still
	// fails on any other event.
	select {
	case ev := <-e.sub.C():
		data, _ := ev.Data.(protocol.CardEventData)
		if ev.Type != string(protocol.EventTypeCardUpdated) || data.Card.ID != card.ID ||
			data.Card.Session == nil || *data.Card.Session != protocol.SessionStateStopped {
			t.Fatalf("the event after stopped = %+v, want the card.updated that carries the stopped session", ev)
		}
	case <-time.After(eventTimeout):
		t.Fatal("no card.updated followed the stopped state")
	}

	// The pump is draining TurnEnded(cancelled) and Exited concurrently with this goroutine now;
	// poll for a while to prove the row never gets rewritten back to awake, and that no further
	// event (in particular, no "working" for the queued message being wrongly delivered) follows.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
		if err != nil {
			t.Fatalf("GetSessionByCard: %v", err)
		}
		if row.State != string(protocol.SessionStateStopped) {
			t.Fatalf("session row state = %s while the pump drained a stopped session, want it to stay stopped", row.State)
		}
		select {
		case ev := <-e.sub.C():
			t.Fatalf("an event arrived after stopped, want none: %+v", ev)
		default:
		}
	}
}
