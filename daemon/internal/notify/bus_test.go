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
	// One new notice reads as itself: its text is the title, and only its own sub-line is the body.
	if !strings.Contains(notice.Title, "Card 5 failed CI") || strings.Contains(notice.Title+notice.Body, "Card 3") {
		t.Errorf("the notice does not name only the new one:\n%s\n%s", notice.Title, notice.Body)
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

func card(state protocol.CardState, reason *protocol.NeedsReason) protocol.Card {
	return protocol.Card{ID: "crd_9", Title: "Fix login", State: state, NeedsReason: reason}
}

func TestACardThatFinishedIsToldOnce(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	moved := protocol.CardMovedEventData{Card: card(protocol.CardStateDone, nil), From: protocol.CardStateReview}
	notice, wanted := svc.EventOf(events.Event{Data: moved})
	if !wanted || notice.Type != notify.EventCardDone || notice.Title != "Finished: Fix login" || notice.CardID != "crd_9" {
		t.Fatalf("a finished card reads %+v (wanted %v)", notice, wanted)
	}
	moved.From = protocol.CardStateDone
	if _, wanted := svc.EventOf(events.Event{Data: moved}); wanted {
		t.Error("a card that was already done was told again")
	}
}

func TestACardThatNeedsThePersonSaysWhyExceptForAPermission(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	needs := func(kind protocol.NeedsReasonKind, text string) protocol.CardMovedEventData {
		return protocol.CardMovedEventData{
			Card: card(protocol.CardStateNeeds, &protocol.NeedsReason{Kind: kind, Text: text}),
			From: protocol.CardStateWorking,
		}
	}
	stuck, wanted := svc.EventOf(events.Event{Data: needs(protocol.NeedsReasonKindStuck, "It cannot find the config.")})
	if !wanted || stuck.Type != notify.EventAgentStuck || stuck.Title != "Fix login needs you" || stuck.Body != "It cannot find the config." {
		t.Errorf("a stuck card reads %+v (wanted %v)", stuck, wanted)
	}
	keyed := needs(protocol.NeedsReasonKindStuck, "It cannot find the config.")
	keyed.Card.Key = "api#4"
	if got, _ := svc.EventOf(events.Event{Data: keyed}); got.Title != "api#4 needs you: Fix login" {
		t.Errorf("a stuck card with a key reads %q, want the project and number first", got.Title)
	}
	red, _ := svc.EventOf(events.Event{Data: needs(protocol.NeedsReasonKindCIFailed, "3 tests failed.")})
	if red.Type != notify.EventCIFailed || red.Title != "CI failed: Fix login" {
		t.Errorf("a card with red CI reads %+v", red)
	}
	if _, wanted := svc.EventOf(events.Event{Data: needs(protocol.NeedsReasonKindApprovalNeeded, "")}); wanted {
		t.Error("a card waiting on a permission was told twice: the approval is its own notice")
	}
	already := needs(protocol.NeedsReasonKindStuck, "")
	already.From = protocol.CardStateNeeds
	if _, wanted := svc.EventOf(events.Event{Data: already}); wanted {
		t.Error("a card that was already waiting was told again")
	}
}

func TestAProjectsBranchTurningRedIsToldOnceUntilItRecovers(t *testing.T) {
	svc := newService(t, newRecorder(), notify.Options{})
	ci := func(status protocol.CIState) events.Event {
		return events.Event{Data: protocol.CIEventData{Project: &protocol.ProjectCI{ProjectID: "api", Status: status}}}
	}
	notice, wanted := svc.EventOf(ci(protocol.CIStateFailed))
	if !wanted || notice.Type != notify.EventCIFailed || notice.Title != "CI failed on api" {
		t.Fatalf("a red branch reads %+v (wanted %v)", notice, wanted)
	}
	if _, wanted := svc.EventOf(ci(protocol.CIStateFailed)); wanted {
		t.Error("a branch that stayed red was told again")
	}
	if _, wanted := svc.EventOf(ci(protocol.CIStatePassed)); wanted {
		t.Error("a branch turning green was told as if it were news")
	}
	if _, wanted := svc.EventOf(ci(protocol.CIStateFailed)); !wanted {
		t.Error("a branch that went red again after recovering was not told")
	}
}

func TestANoticeLinksToTheCardWhereEachChannelCanOpenIt(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{
		LinkBase: func() string { return "http://laptop.tail1.ts.net:47800/" },
		Routes: []notify.Route{{Event: notify.EventAgentStuck, Channels: []notify.Channel{
			notify.ChannelTelegram, notify.ChannelNtfy,
		}}},
	})
	svc.Notify(context.Background(), notify.Event{Type: notify.EventAgentStuck, Title: "Fix login needs you", CardID: "crd_9"})
	svc.Flush(context.Background())
	chat := rec.forChannel(notify.ChannelTelegram)
	app := rec.forChannel(notify.ChannelNtfy)
	if len(chat) != 1 || chat[0].notice.URL != "http://laptop.tail1.ts.net:47800/?open=card%2Fcrd_9" {
		t.Errorf("the chat link is %+v, want the daemon's page opened at the card", chat)
	}
	if len(app) != 1 || app[0].notice.URL != "marshal://card/crd_9" {
		t.Errorf("the ntfy link is %+v, want the phone app's own link", app)
	}
}

func TestANoticeHasNoLinkWhenTheDaemonKnowsNoAddress(t *testing.T) {
	rec := newRecorder()
	svc := newService(t, rec, notify.Options{})
	svc.Notify(context.Background(), notify.Event{Type: notify.EventCardDone, Title: "Finished: x", CardID: "crd_9"})
	svc.Flush(context.Background())
	if got := rec.forChannel(notify.ChannelTelegram); len(got) != 1 || got[0].notice.URL != "" {
		t.Errorf("the notice is %+v, want no link", got)
	}
}
