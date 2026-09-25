package events

import (
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestSince(t *testing.T) {
	// The ring holds five events, so after 12 events it holds 8 to 12.
	bus := newBus(t, WithReplaySize(5))
	empty, replay := bus.Since("epoch-1", 0)
	if replay != ReplayOK || len(empty) != 0 {
		t.Errorf("Since on an empty bus = %v, %s, want ok and nothing", empty, replay)
	}
	for range 12 {
		bus.Publish("t", "tick", nil, false)
	}
	tests := []struct {
		name       string
		epoch      string
		seq        uint64
		wantReplay Replay
		wantSeqs   []uint64
	}{
		{name: "the client is up to date", epoch: "epoch-1", seq: 12, wantReplay: ReplayOK},
		{name: "one event behind", epoch: "epoch-1", seq: 11, wantReplay: ReplayOK, wantSeqs: []uint64{12}},
		{name: "the oldest event in the ring is the next one", epoch: "epoch-1", seq: 7, wantReplay: ReplayOK, wantSeqs: []uint64{8, 9, 10, 11, 12}},
		{name: "one event past what the ring holds", epoch: "epoch-1", seq: 6, wantReplay: ReplayTooOld},
		{name: "a first connection", epoch: "epoch-1", seq: 0, wantReplay: ReplayTooOld},
		{name: "a client ahead of the bus", epoch: "epoch-1", seq: 13, wantReplay: ReplayAhead},
		{name: "another epoch", epoch: "epoch-0", seq: 10, wantReplay: ReplayOtherEpoch},
		{name: "no epoch yet", epoch: "", seq: 0, wantReplay: ReplayOtherEpoch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, replay := bus.Since(tt.epoch, tt.seq)
			if replay != tt.wantReplay {
				t.Errorf("Replay = %s, want %s", replay, tt.wantReplay)
			}
			checkSeqs(t, got, tt.wantSeqs...)
		})
	}
}

func TestSinceBeforeTheRingWraps(t *testing.T) {
	bus := newBus(t)
	for range 3 {
		bus.Publish("t", "tick", nil, false)
	}
	got, replay := bus.Since("epoch-1", 0)
	if replay != ReplayOK {
		t.Fatalf("Replay = %s, want ok: the ring has held everything so far", replay)
	}
	checkSeqs(t, got, 1, 2, 3)
}

func TestReplayIsFilteredByTopic(t *testing.T) {
	bus := newBus(t)
	for _, topic := range []string{"a", "b", "a", "c", "b"} {
		bus.Publish(topic, "tick", nil, false)
	}
	missed, replay := bus.Since("epoch-1", 0)
	if replay != ReplayOK {
		t.Fatal(replay)
	}
	// The client sees gaps in the numbers, which is normal.
	checkSeqs(t, Topics("a", "c").Apply(missed), 1, 3, 4)
	checkSeqs(t, Topics().Apply(missed))
	checkSeqs(t, AllTopics().Apply(missed), 1, 2, 3, 4, 5)
}

func TestReplayMapsToResyncReasons(t *testing.T) {
	tests := []struct {
		replay Replay
		want   protocol.ResyncReason
		needed bool
	}{
		{ReplayOK, "", false},
		{ReplayTooOld, protocol.ResyncReasonTooFarBehind, true},
		{ReplayOtherEpoch, protocol.ResyncReasonEpochChanged, true},
		{ReplayAhead, protocol.ResyncReasonUnknownPosition, true},
		{Replay("something else"), "", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.replay), func(t *testing.T) {
			got, needed := tt.replay.ResyncReason()
			if got != tt.want || needed != tt.needed {
				t.Errorf("ResyncReason = %q, %v, want %q, %v", got, needed, tt.want, tt.needed)
			}
		})
	}
}

func TestSubscribeSinceReplaysThenGoesLive(t *testing.T) {
	bus := newBus(t)
	for _, topic := range []string{"a", "b", "a", "b", "a"} {
		bus.Publish(topic, "tick", nil, false)
	}
	resume := bus.SubscribeSince(Topics("a"), "epoch-1", 1)
	if resume.Replay != ReplayOK || resume.Newest != 5 {
		t.Fatalf("Replay = %s, Newest = %d, want ok and 5", resume.Replay, resume.Newest)
	}
	checkSeqs(t, resume.Events, 3, 5)
	bus.Publish("b", "tick", nil, false)
	bus.Publish("a", "tick", nil, false)
	checkSeqs(t, receiveN(t, resume.Subscription, 1), 7) // Live events start right after Newest.
}

func TestSubscribeSinceWhenReplayIsNotPossible(t *testing.T) {
	bus := newBus(t, WithReplaySize(2))
	for range 5 {
		bus.Publish("a", "tick", nil, false)
	}
	tests := []struct {
		name  string
		epoch string
		seq   uint64
		want  Replay
	}{
		{"too old", "epoch-1", 1, ReplayTooOld},
		{"other epoch", "epoch-0", 4, ReplayOtherEpoch},
		{"ahead", "epoch-1", 1_000, ReplayAhead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newest := bus.Seq()
			resume := bus.SubscribeSince(AllTopics(), tt.epoch, tt.seq)
			defer resume.Subscription.Close()
			if resume.Replay != tt.want || len(resume.Events) != 0 || resume.Newest != newest {
				t.Errorf("Resume = %s, %d events, newest %d, want newest %d", resume.Replay, len(resume.Events), resume.Newest, newest)
			}
			// The resync frame carries Newest, and live events follow on the same subscription.
			bus.Publish("a", "tick", nil, false)
			if got := receive(t, resume.Subscription); got.Seq != bus.Seq() {
				t.Errorf("first live event = %d, want %d", got.Seq, bus.Seq())
			}
		})
	}
}

// TestSubscribeSinceLeavesNoGapAndNoRepeat subscribes while other goroutines publish. The events
// replayed and the events delivered live must join into one unbroken run of sequence numbers.
func TestSubscribeSinceLeavesNoGapAndNoRepeat(t *testing.T) {
	const publishers, each = 4, 1000
	const total = publishers * each
	bus := newBus(t, WithBufferSize(total), WithReplaySize(total))
	var wg sync.WaitGroup
	for range publishers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				bus.Publish("t", "tick", nil, false)
			}
		}()
	}
	for bus.Seq() < total/4 { // Subscribe while the publishers are still going.
	}
	resume := bus.SubscribeSince(AllTopics(), "epoch-1", 0)
	wg.Wait()
	if resume.Replay != ReplayOK {
		t.Fatalf("Replay = %s", resume.Replay)
	}
	got := resume.Events
	for uint64(len(got)) < total {
		got = append(got, receive(t, resume.Subscription))
	}
	for i, ev := range got {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d has seq %d: a gap or a repeat at the join (replayed %d events)", i, ev.Seq, len(resume.Events))
		}
	}
	if resume.Newest < total/4 || resume.Newest > total {
		t.Errorf("Newest = %d, out of range", resume.Newest)
	}
}

func TestSinceAfterClose(t *testing.T) {
	bus := newBus(t)
	for range 3 {
		bus.Publish("a", "tick", nil, false)
	}
	bus.Close() // Close empties the ring.
	tests := []struct {
		seq  uint64
		want Replay
	}{{3, ReplayOK}, {1, ReplayTooOld}, {4, ReplayAhead}}
	for _, tt := range tests {
		if got, replay := bus.Since("epoch-1", tt.seq); replay != tt.want || len(got) != 0 {
			t.Errorf("Since(%d) = %v, %s, want no events and %s", tt.seq, got, replay, tt.want)
		}
	}
}
