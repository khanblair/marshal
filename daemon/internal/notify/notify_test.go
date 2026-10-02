package notify_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/notify"
)

// Routing and grouping notices (B9.4, build-plan 9.7). Every test uses a sender that answers from
// memory, so nothing here touches a chat service and no token is ever needed.

// delivered is one notice a fake sender received, and which channel it was for.
type delivered struct {
	channel notify.Channel
	notice  chatbot.Notice
	at      time.Time
}

// recorder is a Sender that keeps what it was handed.
type recorder struct {
	mu   sync.Mutex
	got  []delivered
	done chan struct{}
	once sync.Once
}

func newRecorder() *recorder { return &recorder{done: make(chan struct{})} }

func (r *recorder) Send(_ context.Context, channel notify.Channel, notice chatbot.Notice) error {
	r.mu.Lock()
	r.got = append(r.got, delivered{channel: channel, notice: notice, at: time.Now()})
	r.mu.Unlock()
	r.once.Do(func() { close(r.done) })
	return nil
}

// all answers everything received so far.
func (r *recorder) all() []delivered {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]delivered(nil), r.got...)
}

// forChannel answers what one channel received.
func (r *recorder) forChannel(channel notify.Channel) []delivered {
	var out []delivered
	for _, d := range r.all() {
		if d.channel == channel {
			out = append(out, d)
		}
	}
	return out
}

func newService(t *testing.T, r notify.Sender, opts notify.Options) *notify.Service {
	t.Helper()
	svc, err := notify.New(r, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func TestAnApprovalIsSentAtOnceAndWithinFiveSeconds(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	start := time.Now()
	svc.Notify(context.Background(), notify.Event{
		Type:    notify.EventApproval,
		Title:   "Card 42 needs you",
		Actions: []chatbot.Action{{Label: "Approve", Data: "approve:abc123"}},
	})
	select {
	case <-rec.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the approval did not reach a phone within five seconds")
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Errorf("the approval took %s, want under 5s", elapsed)
	}
	if len(rec.forChannel(notify.ChannelTelegram)) != 1 {
		t.Errorf("Telegram received %d approvals, want 1", len(rec.forChannel(notify.ChannelTelegram)))
	}
	if len(rec.forChannel(notify.ChannelDiscord)) != 1 {
		t.Errorf("Discord received %d approvals, want 1", len(rec.forChannel(notify.ChannelDiscord)))
	}
}

func TestAGroupableNoticeWaitsAndIsNotSentAtOnce(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	svc.Notify(context.Background(), notify.Event{Type: notify.EventCIFailed, Title: "CI failed on card 7"})
	if got := rec.all(); len(got) != 0 {
		t.Fatalf("a groupable notice was sent at once: %+v", got)
	}
	svc.Flush(context.Background())
	if len(rec.forChannel(notify.ChannelTelegram)) != 1 {
		t.Errorf("Telegram received %d grouped notices, want 1", len(rec.forChannel(notify.ChannelTelegram)))
	}
}

func TestABurstOfEventsBecomesOneMessagePerChannel(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	for _, title := range []string{"CI failed on card 7", "CI failed on card 8", "Card 9 merged"} {
		svc.Notify(context.Background(), notify.Event{Type: notify.EventCIFailed, Title: title})
	}
	svc.Flush(context.Background())
	got := rec.forChannel(notify.ChannelTelegram)
	if len(got) != 1 {
		t.Fatalf("Telegram received %d messages, want 1 grouped message", len(got))
	}
	for _, title := range []string{"CI failed on card 7", "CI failed on card 8", "Card 9 merged"} {
		if !strings.Contains(got[0].notice.Body, title) {
			t.Errorf("the grouped notice does not mention %q:\n%s", title, got[0].notice.Body)
		}
	}
}

func TestAnEventTypeWithNoRouteGoesToTheDefaultChannel(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	svc.Notify(context.Background(), notify.Event{Type: "something.new", Title: "A new kind of event"})
	svc.Flush(context.Background())
	if len(rec.forChannel(notify.ChannelTelegram)) != 1 {
		t.Errorf("an unrouted event did not go to the default channel: %+v", rec.all())
	}
}

func TestARouteCanBeTurnedOff(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	svc.SetRoute(notify.EventCIFailed)
	svc.Notify(context.Background(), notify.Event{Type: notify.EventCIFailed, Title: "CI failed on card 7"})
	svc.Flush(context.Background())
	if len(rec.all()) != 0 {
		t.Errorf("an event whose route was turned off was still sent: %+v", rec.all())
	}
}

func TestRunFlushesWithinTheWindowAndUnderFiveSeconds(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{Window: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	start := time.Now()
	svc.Notify(ctx, notify.Event{Type: notify.EventCIFailed, Title: "CI failed on card 7"})
	select {
	case <-rec.done:
	case <-time.After(5 * time.Second):
		t.Fatal("a grouped notice did not reach a phone within five seconds")
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Errorf("the grouped notice took %s, want under 5s", elapsed)
	}
}

func TestRoutesAreReadableAndDefaulted(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	routes := svc.Routes()
	if len(routes) != len(notify.DefaultRoutes()) {
		t.Fatalf("Routes answered %d rows, want %d", len(routes), len(notify.DefaultRoutes()))
	}
}

func TestNoticesAreHeldWhileQuietAndSentTogetherAfterwards(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	quiet := true
	svc.SetQuiet(func(context.Context) bool { return quiet })
	for _, title := range []string{"CI failed on card 7", "CI failed on card 8"} {
		svc.Notify(context.Background(), notify.Event{Type: notify.EventCIFailed, Title: title})
	}
	svc.Flush(context.Background())
	if got := rec.all(); len(got) != 0 {
		t.Fatalf("notices were sent during a calendar event: %+v", got)
	}
	// An approval is asking for an answer, so it is never held.
	svc.Notify(context.Background(), notify.Event{
		Type: notify.EventApproval, Title: "Card 42 needs you",
		Actions: []chatbot.Action{{Label: "Approve", Data: "approve:abc"}},
	})
	if len(rec.forChannel(notify.ChannelTelegram)) != 1 {
		t.Fatal("an approval was held back during a calendar event")
	}
	quiet = false
	svc.Flush(context.Background())
	got := rec.forChannel(notify.ChannelTelegram)
	if len(got) != 2 || !strings.Contains(got[1].notice.Body, "CI failed on card 8") {
		t.Fatalf("after the event: %+v, want the held notices as one message", got)
	}
}

func TestQuietIsOnlyAskedWhenSomethingIsWaiting(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	asked := 0
	svc.SetQuiet(func(context.Context) bool { asked++; return false })
	svc.Flush(context.Background())
	if asked != 0 {
		t.Errorf("quiet was asked %d times with nothing waiting, want 0 (it can cost a calendar read)", asked)
	}
}

func TestALongQuietKeepsOnlyTheNewestNotices(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	quiet := true
	svc.SetQuiet(func(context.Context) bool { return quiet })
	for i := range 130 {
		svc.Notify(context.Background(), notify.Event{Type: notify.EventCIFailed, Title: fmt.Sprintf("notice %03d", i)})
	}
	svc.Flush(context.Background())
	quiet = false
	svc.Flush(context.Background())
	got := rec.forChannel(notify.ChannelTelegram)
	if len(got) != 1 || strings.Contains(got[0].notice.Body, "notice 000") || !strings.Contains(got[0].notice.Body, "notice 129") {
		t.Fatalf("held notices = %+v, want the newest 100 and not the oldest", got)
	}
}
