package dashboard_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// The subscriber that keeps the stored numbers and the activity stream current from the events the
// daemon publishes (docs/backend-checklist.md B2.3). The events are published by the test the way
// the projects module publishes them, and the rows are read back from the database.

// newSubscriber builds a service and its subscriber on a real bus and a real store, and closes both
// when the test ends. The subscriber closes before the bus, so nothing is left running.
func newSubscriber(t *testing.T, now time.Time) (*dashboard.Service, *dashboard.Subscriber, *store.Store, *events.Bus) {
	t.Helper()
	svc, st := newService(t, now)
	bus, err := events.New()
	if err != nil {
		t.Fatalf("make the event bus: %v", err)
	}
	t.Cleanup(bus.Close)
	sub, err := dashboard.NewSubscriber(svc, bus)
	if err != nil {
		t.Fatalf("make the subscriber: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })
	return svc, sub, st, bus
}

// eventually waits for a check to pass, so a test follows the subscriber's own goroutine instead of
// guessing how long it takes.
func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// finishedCard is a card in the done state, as a card event carries it.
func finishedCard() protocol.Card {
	return protocol.Card{
		ID: "01M3C107JB041061050R3GG28A", ProjectID: "api", Number: 41, Key: "api#41",
		Title: "Fix token refresh on login", State: protocol.CardStateDone,
	}
}

// A card that reaches done writes one row of the stream and adds one card finished and one merge to
// its project's day, and the home topic is told so a client updates without asking again.
func TestSubscriberCountsAFinishedCard(t *testing.T) {
	_, sub, st, bus := newSubscriber(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	watcher := bus.Subscribe(events.Topics(string(protocol.HomeTopic)))
	t.Cleanup(watcher.Close)
	if err := sub.Start(context.Background()); err != nil {
		t.Fatalf("start the subscriber: %v", err)
	}

	card := finishedCard()
	bus.Publish(string(protocol.ProjectTopic("api")), string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{Card: card, From: protocol.CardStateWorking}, true)

	eventually(t, "the finished card to be stored", func() bool { return len(allActivity(t, st)) == 1 })
	row := allActivity(t, st)[0]
	if row.Kind != string(protocol.FeedKindMerge) || row.ProjectID != "api" ||
		row.SubjectKind != "card" || row.SubjectID != card.ID || row.SubjectKey != "api#41" {
		t.Errorf("stream row = %+v", row)
	}
	if !strings.Contains(row.Summary, "#41") || !strings.Contains(row.Summary, card.Title) {
		t.Errorf("stream row says %q, want the card's number and title", row.Summary)
	}
	if row.CreatedAt != fixedTime().UnixMilli() {
		t.Errorf("stream row time = %d, want the clock's time", row.CreatedAt)
	}

	stats := allStats(t, st)
	if len(stats) != 1 {
		t.Fatalf("stored days = %+v, want one", stats)
	}
	if stats[0].Day != midnight(0).UnixMilli() || stats[0].ProjectID != "api" ||
		stats[0].CardsFinished != 1 || stats[0].Merges != 1 {
		t.Errorf("stored day = %+v, want one card finished and one merge for api today", stats[0])
	}

	select {
	case ev := <-watcher.C():
		if protocol.EventType(ev.Type) != protocol.EventTypeActivityCreated || ev.Topic != string(protocol.HomeTopic) {
			t.Fatalf("event = %+v, want activity.created on the home topic", ev)
		}
		data, ok := ev.Data.(protocol.ActivityCreatedEventData)
		if !ok {
			t.Fatalf("event data = %T, want ActivityCreatedEventData", ev.Data)
		}
		if data.Entry.CardKey != "api#41" || data.Entry.Kind != protocol.FeedKindMerge {
			t.Errorf("entry = %+v, want the merge row", data.Entry)
		}
		if data.Day == nil || data.Day.CardsFinished != 1 || data.Day.Merges != 1 {
			t.Errorf("day = %+v, want the whole day after the change", data.Day)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the subscriber published no activity.created event")
	}
}

// A card that was already done and is told to be done again is not counted twice: the event carries
// the state it came from, so one finish is one finish.
func TestSubscriberDoesNotCountAMoveFromDone(t *testing.T) {
	_, sub, st, bus := newSubscriber(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	if err := sub.Start(context.Background()); err != nil {
		t.Fatalf("start the subscriber: %v", err)
	}
	topic := string(protocol.ProjectTopic("api"))
	card := finishedCard()
	bus.Publish(topic, string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{Card: card, From: protocol.CardStateDone}, true)
	// A move that really finishes the card follows, so the wait is on a real change rather than on a
	// sleep: events on one subscription are applied in order.
	bus.Publish(topic, string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{Card: card, From: protocol.CardStateWorking}, true)

	eventually(t, "the one real finish to be stored", func() bool { return len(allActivity(t, st)) == 1 })
	stats := allStats(t, st)
	if len(stats) != 1 || stats[0].CardsFinished != 1 {
		t.Errorf("stored days = %+v, want one card finished, not two", stats)
	}
}

// A card created directly in the done state is finished the moment it exists, which is how a
// fixture's done cards are counted.
func TestSubscriberCountsACardCreatedDone(t *testing.T) {
	_, sub, st, bus := newSubscriber(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	if err := sub.Start(context.Background()); err != nil {
		t.Fatalf("start the subscriber: %v", err)
	}
	card := finishedCard()
	bus.Publish(string(protocol.ProjectTopic("api")), string(protocol.EventTypeCardCreated),
		protocol.CardEventData{Card: card}, true)

	eventually(t, "the created done card to be stored", func() bool { return len(allActivity(t, st)) == 1 })
	if stats := allStats(t, st); len(stats) != 1 || stats[0].Merges != 1 {
		t.Errorf("stored days = %+v, want one merge", stats)
	}
}

// A card that is created in any other state is not counted: only done means finished.
func TestSubscriberIgnoresACardThatIsNotDone(t *testing.T) {
	_, sub, st, bus := newSubscriber(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	if err := sub.Start(context.Background()); err != nil {
		t.Fatalf("start the subscriber: %v", err)
	}
	topic := string(protocol.ProjectTopic("api"))
	working := finishedCard()
	working.State = protocol.CardStateWorking
	bus.Publish(topic, string(protocol.EventTypeCardCreated), protocol.CardEventData{Card: working}, true)
	bus.Publish(topic, string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{Card: working, From: protocol.CardStateBacklog}, true)
	// A real finish follows, so the wait is on the subscriber having applied the two events above.
	other := finishedCard()
	other.ID = "01M3C107JB041061050R3GG28B"
	other.Number = 42
	other.Key = "api#42"
	bus.Publish(topic, string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{Card: other, From: protocol.CardStateWorking}, true)

	eventually(t, "the finished card to be stored", func() bool { return len(allActivity(t, st)) == 1 })
	row := allActivity(t, st)[0]
	if row.SubjectKey != "api#42" {
		t.Errorf("stored row = %+v, want only the card that reached done", row)
	}
}

// The stream is trimmed after ninety days, so one row older than that goes when the next row arrives.
func TestSubscriberTrimsTheStreamAfterNinetyDays(t *testing.T) {
	_, sub, st, bus := newSubscriber(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	old := fixedTime().AddDate(0, 0, -100)
	seedActivity(t, st, activityRow{
		id: "old", projectID: "api", kind: protocol.FeedKindTool, summary: "long ago", at: old,
	})
	if err := sub.Start(context.Background()); err != nil {
		t.Fatalf("start the subscriber: %v", err)
	}
	bus.Publish(string(protocol.ProjectTopic("api")), string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{Card: finishedCard(), From: protocol.CardStateWorking}, true)

	eventually(t, "the old row to be trimmed", func() bool {
		rows := allActivity(t, st)
		return len(rows) == 1 && rows[0].ID != "old"
	})
}

// A project that is removed loses its rows of the stream, and its topic is dropped. A row with no
// project stays.
func TestSubscriberDropsARemovedProjectsActivity(t *testing.T) {
	_, sub, st, bus := newSubscriber(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	seedActivity(t, st,
		activityRow{id: "keep", projectID: "", kind: protocol.FeedKindBrief, summary: "brief", at: midnight(1)},
		activityRow{id: "drop", projectID: "api", kind: protocol.FeedKindMerge, summary: "merged", at: midnight(1)},
	)
	if err := sub.Start(context.Background()); err != nil {
		t.Fatalf("start the subscriber: %v", err)
	}
	bus.Publish(string(protocol.HomeTopic), string(protocol.EventTypeProjectRemoved),
		protocol.ProjectRemovedEventData{ProjectID: "api"}, false)

	eventually(t, "the removed project's rows to go", func() bool {
		rows := allActivity(t, st)
		return len(rows) == 1 && rows[0].ID == "keep"
	})
}

// A project created while the subscriber is running is followed too, so its cards count from then on.
func TestSubscriberFollowsANewProject(t *testing.T) {
	_, sub, st, bus := newSubscriber(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	if err := sub.Start(context.Background()); err != nil {
		t.Fatalf("start the subscriber: %v", err)
	}
	bus.Publish(string(protocol.HomeTopic), string(protocol.EventTypeProjectCreated),
		protocol.ProjectEventData{Project: protocol.Project{ID: "billing", Name: "billing"}}, false)
	// The subscription adds a topic between events, so the next publish has to wait for it. There is
	// nothing to observe the add with, so this gives the subscriber's own goroutine a turn; the wait
	// below is on the row, not on this.
	time.Sleep(50 * time.Millisecond)
	card := finishedCard()
	card.ProjectID = "billing"
	card.Key = "billing#1"
	card.Number = 1
	bus.Publish(string(protocol.ProjectTopic("billing")), string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{Card: card, From: protocol.CardStateWorking}, true)

	eventually(t, "the new project's finished card to be stored", func() bool { return len(allActivity(t, st)) == 1 })
	if stats := allStats(t, st); len(stats) != 1 || stats[0].ProjectID != "billing" {
		t.Errorf("stored days = %+v, want the new project's", stats)
	}
}

// The subscriber needs both halves: a service with a store, and the bus.
func TestNewSubscriberNeedsAServiceAndABus(t *testing.T) {
	svc, _ := newService(t, fixedTime())
	bus, err := events.New()
	if err != nil {
		t.Fatalf("make the event bus: %v", err)
	}
	t.Cleanup(bus.Close)
	if _, err := dashboard.NewSubscriber(nil, nil); err == nil {
		t.Error("NewSubscriber with nothing succeeded")
	}
	if _, err := dashboard.NewSubscriber(svc, nil); err == nil {
		t.Error("NewSubscriber without a bus succeeded")
	}
	if _, err := dashboard.NewSubscriber(nil, bus); err == nil {
		t.Error("NewSubscriber without a service succeeded")
	}
}
