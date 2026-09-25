package events

import (
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no randomness") }

func TestNewMakesAnEpoch(t *testing.T) {
	first, err := New()
	if err != nil {
		t.Fatal(err)
	}
	second, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)
	if !protocol.ValidID(first.Epoch()) || first.Epoch() == second.Epoch() {
		t.Errorf("epochs %q and %q should both be new valid ids", first.Epoch(), second.Epoch())
	}
	if got := newBus(t).Epoch(); got != "epoch-1" {
		t.Errorf("WithEpoch was ignored: %q", got)
	}
	if _, err := New(withEntropy(failingReader{})); err == nil || !strings.Contains(err.Error(), "epoch") {
		t.Errorf("New without randomness = %v, want an epoch error", err)
	}
}

func TestSizeOptionsNeverGoBelowOne(t *testing.T) {
	bus := newBus(t, WithBufferSize(0), WithCriticalBufferSize(-3), WithReplaySize(0))
	if bus.bufferSize != 1 || bus.criticalSize != 1 || bus.replaySize != 1 {
		t.Errorf("sizes = %d, %d, %d, want 1, 1, 1", bus.bufferSize, bus.criticalSize, bus.replaySize)
	}
	defaults := newBus(t)
	if defaults.bufferSize != 256 || defaults.criticalSize != 1024 || defaults.replaySize != 2000 {
		t.Errorf("defaults = %d, %d, %d, want 256, 1024, 2000", defaults.bufferSize, defaults.criticalSize, defaults.replaySize)
	}
}

func TestPublishNumbersAndStampsEvents(t *testing.T) {
	moments := []time.Time{time.Unix(100, 0), time.Unix(200, 0)}
	next := 0
	bus := newBus(t, WithClock(func() time.Time { moment := moments[next]; next++; return moment }))
	sub := bus.Subscribe(AllTopics())
	if got := bus.Seq(); got != 0 {
		t.Errorf("Seq before any event = %d, want 0", got)
	}
	if seq := bus.Publish("home", "card.created", "a", false); seq != 1 {
		t.Errorf("first Publish returned %d, want 1", seq)
	}
	if seq := bus.Publish("project:api", "card.moved", "b", true); seq != 2 {
		t.Errorf("second Publish returned %d, want 2", seq)
	}
	got := receiveN(t, sub, 2)
	want := []Event{
		{Seq: 1, Topic: "home", Type: "card.created", At: moments[0], Data: "a"},
		{Seq: 2, Topic: "project:api", Type: "card.moved", At: moments[1], Critical: true, Data: "b"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if bus.Seq() != 2 {
		t.Errorf("Seq = %d, want 2", bus.Seq())
	}
}

type payload struct{ publisher, n int }

func TestSequenceNumbersUnderConcurrentPublishers(t *testing.T) {
	const publishers, each = 8, 500
	const total = publishers * each
	bus := newBus(t, WithBufferSize(total), WithReplaySize(total))
	sub := bus.Subscribe(AllTopics())

	returned := make(chan uint64, total)
	var wg sync.WaitGroup
	for p := range publishers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range each {
				returned <- bus.Publish("t", "tick", payload{p, n}, n%7 == 0)
			}
		}()
	}
	got := receiveN(t, sub, total)
	wg.Wait()
	close(returned)

	last := make([]int, publishers)
	for i := range last {
		last[i] = -1
	}
	for i, ev := range got {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d has seq %d, want %d", i, ev.Seq, i+1)
		}
		data := ev.Data.(payload)
		if data.n != last[data.publisher]+1 {
			t.Fatalf("publisher %d: event %d came after %d", data.publisher, data.n, last[data.publisher])
		}
		last[data.publisher] = data.n
	}
	seen := make(map[uint64]bool, total)
	for seq := range returned {
		if seen[seq] || seq < 1 || seq > total {
			t.Fatalf("Publish returned %d twice or out of range", seq)
		}
		seen[seq] = true
	}
	if sub.Lagged() {
		t.Error("a subscriber with room was told it lagged")
	}
	ring, replay := bus.Since("epoch-1", 0)
	if replay != ReplayOK || len(ring) != total || ring[total-1].Seq != total {
		t.Errorf("ring holds %d events (%s), want all %d", len(ring), replay, total)
	}
}

func TestASlowSubscriberDoesNotSlowThePublisher(t *testing.T) {
	const events = 50_000
	bus := newBus(t, WithBufferSize(4), WithCriticalBufferSize(4))
	slow := bus.Subscribe(AllTopics()) // Never reads.
	fast := bus.Subscribe(AllTopics())
	lastSeen := make(chan uint64, 1)
	go func() {
		var last uint64
		for ev := range fast.C() {
			last = ev.Seq
			if last == events {
				break
			}
		}
		lastSeen <- last
	}()

	published := make(chan struct{})
	go func() {
		for range events {
			bus.Publish("t", "tick", nil, false)
		}
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(testTimeout):
		t.Fatal("Publish was held up by a subscriber that does not read")
	}
	if !slow.Lagged() {
		t.Error("the subscriber that never reads was not told it lagged")
	}
	select {
	case last := <-lastSeen:
		if last != events {
			t.Errorf("the fast subscriber ended at %d, want the newest event %d", last, events)
		}
	case <-time.After(testTimeout):
		t.Fatal("the fast subscriber never saw the newest event")
	}
}

func TestOrdinaryEventsAreDroppedOldestFirst(t *testing.T) {
	bus := newBus(t, WithBufferSize(3))
	sub := bus.Subscribe(AllTopics())
	bus.Publish("t", "tick", nil, false)
	waitDrained(t, sub) // The goroutine now holds event 1 and waits for the reader.
	for range 9 {
		bus.Publish("t", "tick", nil, false)
	}
	if !sub.Lagged() {
		t.Error("Lagged is false after events were dropped")
	}
	checkSeqs(t, receiveN(t, sub, 4), 1, 8, 9, 10)
	expectNothing(t, sub)
	if !sub.TakeLagged() || sub.TakeLagged() || sub.Lagged() {
		t.Error("TakeLagged should report the drop once and then clear it")
	}
	bus.Publish("t", "tick", nil, false)
	if sub.Lagged() {
		t.Error("Lagged came back with no new drop")
	}
	checkSeqs(t, receiveN(t, sub, 1), 11)
}

func TestCriticalEventsSurviveAFullBuffer(t *testing.T) {
	bus := newBus(t, WithBufferSize(2), WithCriticalBufferSize(10))
	sub := bus.Subscribe(AllTopics())
	bus.Publish("t", "tick", nil, false)
	waitDrained(t, sub)
	kinds := []bool{true, false, false, false, false, true, false} // Events 2 to 8.
	for _, critical := range kinds {
		bus.Publish("t", "tick", nil, critical)
	}
	// Only the two newest ordinary events (6 and 8) are kept, and the critical ones (2 and 7) are
	// all kept. They arrive in sequence order.
	got := receiveN(t, sub, 5)
	checkSeqs(t, got, 1, 2, 6, 7, 8)
	if !got[1].Critical || !got[3].Critical || got[2].Critical {
		t.Errorf("critical flags are wrong: %+v", got)
	}
	expectNothing(t, sub)
}

func TestCriticalOverflowClosesTheSubscriptionForResync(t *testing.T) {
	bus := newBus(t, WithCriticalBufferSize(3))
	stuck := bus.Subscribe(AllTopics())
	healthy := bus.Subscribe(AllTopics())
	bus.Publish("t", "approval", nil, true)
	waitDrained(t, stuck)
	got := receiveN(t, healthy, 1)
	for range 4 {
		bus.Publish("t", "approval", nil, true)
		got = append(got, receive(t, healthy)) // The healthy subscriber keeps up.
	}

	if got := stuck.Reason(); got != ClosedForResync {
		t.Errorf("Reason = %v, want ClosedForResync", got)
	}
	if !stuck.Lagged() || !stuck.TakeLagged() || !stuck.Lagged() {
		t.Error("a subscription closed for resync must keep reporting Lagged")
	}
	for _, ev := range drain(t, stuck) {
		if ev.Seq != 1 {
			t.Errorf("event %d arrived after the overflow, want at most the one already in hand", ev.Seq)
		}
	}
	bus.mu.Lock()
	remaining := len(bus.subs)
	bus.mu.Unlock()
	if remaining != 1 {
		t.Errorf("the bus follows %d subscriptions, want only the healthy one", remaining)
	}
	checkSeqs(t, got, 1, 2, 3, 4, 5)
	bus.Publish("t", "approval", nil, true) // The bus keeps working.
	checkSeqs(t, receiveN(t, healthy, 1), 6)
}

func TestTopicFiltersAndLiveAddRemove(t *testing.T) {
	bus := newBus(t)
	sub := bus.Subscribe(Topics("a"))
	everything := bus.Subscribe(AllTopics())
	bus.Publish("a", "tick", 1, false)
	bus.Publish("b", "tick", 2, false)
	checkSeqs(t, receiveN(t, sub, 1), 1)

	sub.Add("b", "c")
	bus.Publish("b", "tick", 3, false)
	bus.Publish("c", "tick", 4, false)
	sub.Remove("a")
	bus.Publish("a", "tick", 5, false)
	bus.Publish("c", "tick", 6, false)
	checkSeqs(t, receiveN(t, sub, 3), 3, 4, 6)
	expectNothing(t, sub)

	everything.Add("z")
	everything.Remove("a")
	bus.Publish("a", "tick", 7, false)
	if got := receiveN(t, everything, 7); got[6].Seq != 7 {
		t.Errorf("a subscriber to every topic missed an event: %v", seqs(got))
	}
}

func TestAFilterIsCopiedOnSubscribe(t *testing.T) {
	bus := newBus(t)
	filter := Topics("a")
	sub := bus.Subscribe(filter)
	sub.Add("b")
	if filter.Matches("b") {
		t.Error("the subscription changed the caller's filter")
	}
}

func TestCloseIsIdempotentAndReleasesTheGoroutine(t *testing.T) {
	bus := newBus(t)
	before := runtime.NumGoroutine()
	sub := bus.Subscribe(AllTopics())
	bus.Publish("t", "tick", nil, false)

	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); sub.Close() }()
	}
	wg.Wait()
	sub.Close()
	drain(t, sub)
	if got := sub.Reason(); got != ClosedByCaller {
		t.Errorf("Reason = %v, want ClosedByCaller", got)
	}
	bus.mu.Lock()
	remaining := len(bus.subs)
	bus.mu.Unlock()
	if remaining != 0 {
		t.Errorf("the bus still follows %d closed subscriptions", remaining)
	}
	sub.Add("x")
	sub.Remove("x")
	// The three closing goroutines may need a moment to finish, so wait for the count to settle.
	deadline := time.Now().Add(testTimeout)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines after Close = %d, before Subscribe = %d", after, before)
	}
}

func TestBusCloseEndsEverySubscription(t *testing.T) {
	bus := newBus(t)
	subs := []*Subscription{bus.Subscribe(AllTopics()), bus.Subscribe(Topics("a"))}
	var readers sync.WaitGroup
	for _, sub := range subs {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range sub.C() { // Ends only when the channel closes.
			}
		}()
	}
	bus.Close()
	bus.Close()
	readers.Wait()
	for _, sub := range subs {
		if got := sub.Reason(); got != ClosedByBus {
			t.Errorf("Reason = %v, want ClosedByBus", got)
		}
	}
	if seq := bus.Publish("a", "tick", nil, false); seq != 0 {
		t.Errorf("Publish on a closed bus returned %d, want 0", seq)
	}
	if _, replay := bus.Since("epoch-1", 0); replay != ReplayOK {
		t.Errorf("Since on a closed bus with no events = %s, want ok", replay)
	}
	late := bus.Subscribe(AllTopics())
	drain(t, late)
	if late.Reason() != ClosedByBus {
		t.Errorf("a subscription made after Close has reason %v, want ClosedByBus", late.Reason())
	}
	late.Close()
}

func TestNoGoroutinePerEvent(t *testing.T) {
	bus := newBus(t)
	sub := bus.Subscribe(AllTopics()) // Never reads, so events pile up and are dropped.
	before := runtime.NumGoroutine()
	for range 20_000 {
		bus.Publish("t", "tick", nil, false)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines grew from %d to %d while publishing", before, after)
	}
	sub.Close()
}
