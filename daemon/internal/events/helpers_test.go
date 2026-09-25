package events

import (
	"io"
	"slices"
	"testing"
	"time"
)

// testTimeout is how long a test waits for something that should happen at once.
const testTimeout = 5 * time.Second

// quietPeriod is how long a test waits before it decides that nothing more will arrive.
const quietPeriod = 30 * time.Millisecond

// newBus makes a bus with a fixed epoch and closes it when the test ends, which also stops the
// goroutine of every subscription the test forgot to close.
func newBus(t *testing.T, opts ...Option) *Bus {
	t.Helper()
	bus, err := New(append([]Option{WithEpoch("epoch-1")}, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(bus.Close)
	return bus
}

// withEntropy replaces the random source that makes the default epoch.
func withEntropy(r io.Reader) Option {
	return func(c *config) { c.entropy = r }
}

func receive(t *testing.T, sub *Subscription) Event {
	t.Helper()
	select {
	case ev, ok := <-sub.C():
		if !ok {
			t.Fatal("the subscription closed, want an event")
		}
		return ev
	case <-time.After(testTimeout):
		t.Fatal("no event arrived")
		return Event{}
	}
}

func receiveN(t *testing.T, sub *Subscription, n int) []Event {
	t.Helper()
	out := make([]Event, 0, n)
	for range n {
		out = append(out, receive(t, sub))
	}
	return out
}

// expectNothing fails if an event arrives, or the channel closes, within the quiet period.
func expectNothing(t *testing.T, sub *Subscription) {
	t.Helper()
	select {
	case ev, ok := <-sub.C():
		t.Fatalf("unexpected receive: %+v, open %v", ev, ok)
	case <-time.After(quietPeriod):
	}
}

// drain reads until the channel closes and returns what it read.
func drain(t *testing.T, sub *Subscription) []Event {
	t.Helper()
	var out []Event
	timeout := time.After(testTimeout)
	for {
		select {
		case ev, ok := <-sub.C():
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			t.Fatal("the channel did not close")
			return out
		}
	}
}

// waitDrained waits until the subscription's goroutine has taken every queued event, so it holds
// at most one event and waits for the reader. That makes the drop tests exact.
func waitDrained(t *testing.T, sub *Subscription) {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) {
		sub.mu.Lock()
		queued := sub.normal.len() + sub.critical.len()
		sub.mu.Unlock()
		if queued == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the subscription never took its queued events")
}

func seqs(events []Event) []uint64 {
	out := make([]uint64, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Seq)
	}
	return out
}

func checkSeqs(t *testing.T, got []Event, want ...uint64) {
	t.Helper()
	if !slices.Equal(seqs(got), want) {
		t.Errorf("seqs = %v, want %v", seqs(got), want)
	}
}
