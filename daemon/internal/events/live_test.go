package events

import (
	"testing"
	"time"
)

// A live-only event reaches the subscribers of its topic in order with the events around it,
// carries the next sequence number, and is not critical.
func TestPublishLiveIsAnOrdinaryEventThatSkipsTheRing(t *testing.T) {
	bus := newBus(t)
	sub := bus.Subscribe(Topics("card:a"))
	if seq := bus.Publish("card:a", "card.updated", "one", false); seq != 1 {
		t.Fatalf("Publish returned %d, want 1", seq)
	}
	if seq := bus.PublishLive("card:a", "session.terminal_output", "bytes"); seq != 2 {
		t.Fatalf("PublishLive returned %d, want 2: it takes the next number like any event", seq)
	}
	if seq := bus.Publish("card:a", "card.updated", "three", true); seq != 3 {
		t.Fatalf("Publish returned %d, want 3", seq)
	}
	got := receiveN(t, sub, 3)
	checkSeqs(t, got, 1, 2, 3)
	if got[1].Critical || got[1].Type != "session.terminal_output" || got[1].Data != "bytes" {
		t.Errorf("the live event arrived as %+v", got[1])
	}
	// The ring holds the two replayable events and not the live one, so a client that has applied
	// none of them is replayed exactly those.
	missed, replay := bus.Since("epoch-1", 0)
	if replay != ReplayOK {
		t.Fatalf("Replay = %s, want ok", replay)
	}
	checkSeqs(t, missed, 1, 3)
}

// The sequence numbers of the ring are no longer one after the other, so the replay must not
// count them: a run of live events between two replayable ones must not change what is replayed.
func TestReplaySkipsLiveEventsWithoutMiscountingTheRing(t *testing.T) {
	bus := newBus(t, WithReplaySize(3))
	bus.Publish("a", "tick", nil, false) // 1
	for range 5 {
		bus.PublishLive("a", "bytes", nil) // 2 to 6
	}
	bus.Publish("a", "tick", nil, false) // 7
	bus.Publish("a", "tick", nil, false) // 8
	tests := []struct {
		name       string
		seq        uint64
		wantReplay Replay
		wantSeqs   []uint64
	}{
		{"nothing missed", 8, ReplayOK, nil},
		{"one behind", 7, ReplayOK, []uint64{8}},
		{"after a run of live events", 1, ReplayOK, []uint64{7, 8}},
		{"inside the run of live events", 4, ReplayOK, []uint64{7, 8}},
		{"from the start while the ring still holds everything replayable", 0, ReplayOK, []uint64{1, 7, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, replay := bus.Since("epoch-1", tt.seq)
			if replay != tt.wantReplay {
				t.Errorf("Replay = %s, want %s", replay, tt.wantReplay)
			}
			checkSeqs(t, got, tt.wantSeqs...)
		})
	}
}

// Live events do not push replayable ones out of the ring, however many there are.
func TestLiveEventsDoNotEvictReplayableOnes(t *testing.T) {
	bus := newBus(t, WithReplaySize(2))
	bus.Publish("a", "tick", nil, false) // 1
	for range 1000 {
		bus.PublishLive("a", "bytes", nil)
	}
	bus.Publish("a", "tick", nil, false) // 1002
	got, replay := bus.Since("epoch-1", 0)
	if replay != ReplayOK {
		t.Fatalf("Replay = %s, want ok: a thousand live events cannot have pushed anything out", replay)
	}
	checkSeqs(t, got, 1, 1002)
	// A third replayable event does push the first one out, and a client behind it must reload.
	bus.Publish("a", "tick", nil, false) // 1003
	if _, replay := bus.Since("epoch-1", 0); replay != ReplayTooOld {
		t.Errorf("Replay = %s, want too-old once a replayable event left the ring", replay)
	}
	got, replay = bus.Since("epoch-1", 1)
	if replay != ReplayOK {
		t.Fatalf("Replay = %s, want ok for a client that has applied the event that left", replay)
	}
	checkSeqs(t, got, 1002, 1003)
}

// A client that applied a live event and reconnects sends that number, which is not in the ring,
// and is replayed what came after it.
func TestASinceThatIsALiveEventIsReplayed(t *testing.T) {
	bus := newBus(t)
	bus.Publish("a", "tick", nil, false) // 1
	bus.PublishLive("a", "bytes", nil)   // 2
	bus.Publish("a", "tick", nil, false) // 3
	resume := bus.SubscribeSince(AllTopics(), "epoch-1", 2)
	defer resume.Subscription.Close()
	if resume.Replay != ReplayOK {
		t.Fatalf("Replay = %s, want ok", resume.Replay)
	}
	checkSeqs(t, resume.Events, 3)
}

// A live event is never replayed to a subscriber that joins later, and it reaches the ones that
// were there.
func TestLiveEventsAreOnlyForThoseSubscribedNow(t *testing.T) {
	bus := newBus(t)
	before := bus.Subscribe(Topics("card:a"))
	other := bus.Subscribe(Topics("card:b"))
	bus.PublishLive("card:a", "session.terminal_output", "x")
	after := bus.Subscribe(Topics("card:a"))
	checkSeqs(t, receiveN(t, before, 1), 1)
	expectNothing(t, other)
	expectNothing(t, after)
}

// A subscriber that never reads cannot slow the publisher down or grow without bound: it loses
// its oldest ordinary events, which includes live ones, and is told to reload. Critical events
// beside them are still kept.
func TestALiveFirehoseDoesNotStallTheBusOrDropCriticalEvents(t *testing.T) {
	const live = 20_000
	bus := newBus(t, WithBufferSize(8), WithCriticalBufferSize(8))
	slow := bus.Subscribe(AllTopics()) // Never reads.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range live {
			bus.PublishLive("card:a", "session.terminal_output", nil)
		}
		bus.Publish("card:a", "session.state_changed", "asleep", true)
	}()
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("the publisher was held up by a subscriber that does not read")
	}
	if !slow.Lagged() {
		t.Error("a subscriber that lost live events was not told it lagged")
	}
	// The critical event is still waiting for the reader, after at most a few ordinary ones.
	deadline := time.After(testTimeout)
	for {
		select {
		case ev, ok := <-slow.C():
			if !ok {
				t.Fatal("the subscription closed before the critical event arrived")
			}
			if ev.Critical {
				if ev.Data != "asleep" {
					t.Errorf("critical event = %+v", ev)
				}
				return
			}
		case <-deadline:
			t.Fatal("the critical event was dropped")
		}
	}
}
