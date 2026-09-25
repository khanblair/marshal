package agents

import (
	"testing"
	"time"
)

// sinkTimeout bounds a test that waits for a blocked sender to finish. It is a liveness bound, not
// a wait for a fixed time: the sender is expected to return at once, and this only stops the test
// hanging forever if it never does.
const sinkTimeout = 2 * time.Second

// drain reads what is already on the channel and returns it, without waiting for more.
func drain(ch <-chan AgentEvent) []AgentEvent {
	var got []AgentEvent
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, ev)
		default:
			return got
		}
	}
}

// waitForSink fails the test when the sender does not finish within the bound.
func waitForSink(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(sinkTimeout):
		t.Fatalf("%s did not finish", what)
	}
}

func TestEventSinkDeliversInOrderAndClosesOnce(t *testing.T) {
	s := NewEventSink(4)
	s.Emit(MessageChunk{Text: "one"})
	s.Emit(MessageChunk{Text: "two"})
	s.Close()
	s.Close() // closing a sink twice must not close its channel twice

	got := drain(s.C())
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if first, ok := got[0].(MessageChunk); !ok || first.Text != "one" {
		t.Errorf("first event = %#v, want the message one", got[0])
	}
	if second, ok := got[1].(MessageChunk); !ok || second.Text != "two" {
		t.Errorf("second event = %#v, want the message two", got[1])
	}
	if _, ok := <-s.C(); ok {
		t.Error("the channel is still open after Close")
	}
}

func TestEventSinkDropsWhatComesAfterClose(t *testing.T) {
	s := NewEventSink(1)
	s.Close()
	// A late event must be dropped, not sent on a closed channel, which would panic.
	s.Emit(MessageChunk{Text: "late"})
	s.EmitFinal(MessageChunk{Text: "later"})
	if got := drain(s.C()); len(got) != 0 {
		t.Errorf("got %d events after Close, want none", len(got))
	}
}

func TestEventSinkEmitWaitsForRoomAndThenDelivers(t *testing.T) {
	s := NewEventSink(1)
	s.Emit(MessageChunk{Text: "fills the buffer"})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Emit(MessageChunk{Text: "waited for room"})
	}()
	// Reading the first event makes room, which lets the waiting sender through.
	if first := <-s.C(); first.(MessageChunk).Text != "fills the buffer" {
		t.Fatalf("first event = %#v", first)
	}
	waitForSink(t, done, "Emit")
	if second, ok := (<-s.C()).(MessageChunk); !ok || second.Text != "waited for room" {
		t.Errorf("second event = %#v, want the waiting message", second)
	}
}

func TestEventSinkEmitGivesUpWhenReleased(t *testing.T) {
	s := NewEventSink(1)
	s.Emit(MessageChunk{Text: "fills the buffer"})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Emit(MessageChunk{Text: "no room, and released"})
	}()
	s.Release()
	s.Release() // releasing twice must not close the abort channel twice
	waitForSink(t, done, "Emit")
	// The released sender dropped its event: only the one that fit was delivered.
	got := drain(s.C())
	if len(got) != 1 || got[0].(MessageChunk).Text != "fills the buffer" {
		t.Errorf("got %#v, want only the event that fit", got)
	}
}

func TestEventSinkEmitFinalStillDeliversAfterRelease(t *testing.T) {
	s := NewEventSink(2)
	s.Release()
	s.EmitFinal(MessageChunk{Text: "the last word"})
	got := drain(s.C())
	if len(got) != 1 || got[0].(MessageChunk).Text != "the last word" {
		t.Errorf("EmitFinal after Release = %#v, want the event", got)
	}
}

func TestEventSinkEmitFinalGivesUpWhenFullAndReleased(t *testing.T) {
	s := NewEventSink(1)
	s.Emit(MessageChunk{Text: "fills the buffer"})
	s.Release()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.EmitFinal(MessageChunk{Text: "no room, and released"})
	}()
	waitForSink(t, done, "EmitFinal")
	if got := drain(s.C()); len(got) != 1 {
		t.Errorf("got %d events, want only the event that fit", len(got))
	}
}

func TestEventSinkCloseWaitsForASender(t *testing.T) {
	s := NewEventSink(1)
	s.Emit(MessageChunk{Text: "fills the buffer"})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Emit(MessageChunk{Text: "waits for room"})
	}()
	// Release lets the blocked sender give up, and Close takes its write lock, so it cannot close
	// the channel while a sender is still inside Emit.
	s.Release()
	s.Close()
	waitForSink(t, done, "Emit")
	got := drain(s.C())
	if len(got) != 1 {
		t.Errorf("got %d events, want only the event that fit", len(got))
	}
}
