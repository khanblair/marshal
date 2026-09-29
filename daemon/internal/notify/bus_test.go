package notify_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/notify"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Turning the daemon's events into notices (B9.4, build-plan 9.7). The mapping is decided here
// exactly rather than inferred through a live bus; one end-to-end test then proves a published
// event actually reaches a chat and does so inside five seconds.

func TestAnApprovalIsOfferedAsTheAnswersTheAgentGave(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	notice, wanted := svc.EventOf(events.Event{
		Type: string(protocol.EventTypeApprovalRequested),
		Data: protocol.ApprovalRequestedEventData{
			CardID: "crd_1",
			Approval: protocol.Approval{
				ID:    "app_1",
				Title: "Run the test suite",
				Path:  "go test ./...",
				Options: []protocol.ApprovalOption{
					{ID: "allow_once", Name: "Allow once", Kind: "allow_once"},
					{ID: "reject_once", Name: "Not now", Kind: "reject_once"},
				},
			},
		},
	})
	if !wanted {
		t.Fatal("an approval request is not routed to a phone")
	}
	if notice.Type != notify.EventApproval {
		t.Errorf("the event type is %q, want %q", notice.Type, notify.EventApproval)
	}
	if notice.Title != "Run the test suite" {
		t.Errorf("the title is %q, want the agent's own words", notice.Title)
	}
	if len(notice.Actions) != 2 {
		t.Fatalf("the notice offers %d actions, want the agent's two", len(notice.Actions))
	}
	if notice.Actions[0].Label != "Allow once" || notice.Actions[0].Data != "approval:app_1:allow_once" {
		t.Errorf("the first action is %+v", notice.Actions[0])
	}
}

func TestAnApprovalWithNoTitleIsStillSayable(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	notice, _ := svc.EventOf(events.Event{
		Data: protocol.ApprovalRequestedEventData{
			Approval: protocol.Approval{
				ID:      "app_1",
				Command: "rm -rf build",
				Options: []protocol.ApprovalOption{{ID: "allow_once"}},
			},
		},
	})
	if notice.Title == "" {
		t.Error("an approval with no title would send a notice with no title")
	}
	if notice.Body != "rm -rf build" {
		t.Errorf("the body is %q, want the command", notice.Body)
	}
	if notice.Actions[0].Label != "allow_once" {
		t.Errorf("an option with no name has label %q, want its own id", notice.Actions[0].Label)
	}
}

func TestAnEventNoOneIsBuzzedAboutIsNotRouted(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	for _, data := range []any{
		nil,
		protocol.CIEventData{},
		protocol.ApprovalResolvedEventData{CardID: "crd_1", ApprovalID: "app_1"},
		protocol.ChatEventData{},
	} {
		if _, wanted := svc.EventOf(events.Event{Data: data}); wanted {
			t.Errorf("%T was routed to a phone, want silence", data)
		}
	}
}

func TestOnlyTheNoticesThatAreNewAreSent(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	list := func(id string) protocol.NoticeListEventData {
		notices := []protocol.Notice{{ID: "n1", Text: "Card 3 will sleep"}}
		if id != "" {
			notices = append(notices, protocol.Notice{ID: id, Text: "Card 5 failed CI"})
		}
		return protocol.NoticeListEventData{Notices: notices}
	}
	if _, wanted := svc.EventOf(events.Event{Data: list("")}); !wanted {
		t.Fatal("the first standing notice was not routed")
	}
	if _, wanted := svc.EventOf(events.Event{Data: list("")}); wanted {
		t.Error("a standing notice that was already sent was sent again")
	}
	notice, wanted := svc.EventOf(events.Event{Data: list("n2")})
	if !wanted {
		t.Fatal("a genuinely new notice was not routed")
	}
	if !strings.Contains(notice.Body, "Card 5 failed CI") {
		t.Errorf("the grouped notice does not name the new one:\n%s", notice.Body)
	}
}

// An approval published on a real bus reaches a chat, and does so well inside five seconds. The
// subscription is made by Follow, so the publish is repeated until it lands: the very first one can
// beat the subscription by a few microseconds, and a test that depends on that race would be flaky.
func TestAPublishedApprovalReachesAChatUnderFiveSeconds(t *testing.T) {
	bus, err := events.New()
	if err != nil {
		t.Fatalf("events.New: %v", err)
	}
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Follow(ctx, bus)

	payload := protocol.ApprovalRequestedEventData{
		CardID:   "crd_1",
		Approval: protocol.Approval{ID: "app_1", Title: "Card 3 needs you", Options: []protocol.ApprovalOption{{ID: "allow_once", Name: "Allow"}}},
	}
	start := time.Now()
	for attempt := 0; attempt < 50; attempt++ {
		bus.Publish("card:crd_1", string(protocol.EventTypeApprovalRequested), payload, true)
		select {
		case <-rec.done:
			if elapsed := time.Since(start); elapsed >= 5*time.Second {
				t.Fatalf("the approval took %s to reach the chat, want under 5s", elapsed)
			}
			if got := rec.forChannel(notify.ChannelTelegram); len(got) == 0 {
				t.Fatal("the approval never reached Telegram")
			} else if got[0].notice.Title != "Card 3 needs you" {
				t.Errorf("the notice says %q", got[0].notice.Title)
			}
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal("no published approval ever reached a chat")
}
