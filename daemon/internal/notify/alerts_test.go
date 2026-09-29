package notify_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/notify"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The alert settings (B9.4): where each kind of alert goes as a screen reads and changes it, kept
// across restarts, and applied to the running router at once.

// memoryStore keeps the routes in a map, the way the settings table keeps them in a row.
type memoryStore struct{ routes map[string][]string }

func (m *memoryStore) AlertRoutes(context.Context) (map[string][]string, error) {
	out := map[string][]string{}
	for event, channels := range m.routes {
		out[event] = append([]string(nil), channels...)
	}
	return out, nil
}

func (m *memoryStore) SetAlertRoutes(_ context.Context, routes map[string][]string) error {
	m.routes = routes
	return nil
}

func newAlerts(t *testing.T, store *memoryStore) (*notify.Alerts, *notify.Service, *recorder) {
	t.Helper()
	rec := newRecorder()
	router := newService(t, rec, notify.Options{})
	alerts, err := notify.NewAlerts(router, store, func(context.Context) map[notify.Channel]bool {
		return map[notify.Channel]bool{notify.ChannelTelegram: true}
	}, nil)
	if err != nil {
		t.Fatalf("NewAlerts: %v", err)
	}
	return alerts, router, rec
}

func routeOf(settings protocol.AlertSettings, event string) []string {
	for _, route := range settings.Routes {
		if route.Event == event {
			return route.Channels
		}
	}
	return nil
}

func TestTheSettingsListEveryAlertAndWhichChannelsAreConnected(t *testing.T) {
	alerts, _, _ := newAlerts(t, &memoryStore{})
	got, err := alerts.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Routes) != 5 || got.Routes[1].Event != notify.EventAgentStuck || got.Routes[1].Label == "" {
		t.Fatalf("the routes are %+v, want the five alerts in the screen's order, each labelled", got.Routes)
	}
	if !reflect.DeepEqual(routeOf(got, notify.EventCIFailed), []string{"telegram"}) {
		t.Errorf("CI failed goes to %v, want the default, Telegram", routeOf(got, notify.EventCIFailed))
	}
	connected := map[string]bool{}
	for _, channel := range got.Channels {
		connected[channel.ID] = channel.Connected
	}
	if len(got.Channels) != 3 || !connected["telegram"] || connected["ntfy"] || connected["discord"] {
		t.Errorf("the channels are %+v, want three with only Telegram connected", got.Channels)
	}
}

func TestASavedChoiceIsKeptAppliedAtOnceAndOthersAreLeftAlone(t *testing.T) {
	store := &memoryStore{}
	alerts, router, rec := newAlerts(t, store)
	got, err := alerts.Save(context.Background(), protocol.SaveAlertSettingsRequest{Routes: []protocol.AlertRouteChoice{
		{Event: notify.EventCardDone, Channels: []string{"ntfy", "ntfy", "telegram"}},
		{Event: notify.EventCIFailed, Channels: []string{}},
	}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !reflect.DeepEqual(routeOf(got, notify.EventCardDone), []string{"ntfy", "telegram"}) {
		t.Errorf("a card finishing goes to %v, want ntfy and Telegram once each", routeOf(got, notify.EventCardDone))
	}
	if len(routeOf(got, notify.EventCIFailed)) != 0 {
		t.Errorf("CI failed goes to %v, want nowhere", routeOf(got, notify.EventCIFailed))
	}
	if !reflect.DeepEqual(routeOf(got, notify.EventAgentStuck), []string{"telegram", "discord"}) {
		t.Errorf("an alert left out changed to %v", routeOf(got, notify.EventAgentStuck))
	}
	if len(store.routes) != 2 {
		t.Errorf("the store holds %v, want only the two changed alerts", store.routes)
	}
	// The running router follows the choice with no restart.
	router.Notify(context.Background(), notify.Event{Type: notify.EventCardDone, Title: "Finished: x"})
	router.Notify(context.Background(), notify.Event{Type: notify.EventCIFailed, Title: "CI failed: x"})
	router.Flush(context.Background())
	if len(rec.forChannel(notify.ChannelNtfy)) != 1 || len(rec.forChannel(notify.ChannelTelegram)) != 1 {
		t.Errorf("the router delivered %+v, want the finished card on ntfy and Telegram and no CI alert", rec.all())
	}
}

func TestAChoiceTheScreenDoesNotOfferRefusesTheWholeSave(t *testing.T) {
	store := &memoryStore{}
	alerts, _, _ := newAlerts(t, store)
	for name, choice := range map[string]protocol.AlertRouteChoice{
		"an unknown alert":   {Event: "card.exploded", Channels: []string{"telegram"}},
		"an unknown channel": {Event: notify.EventCardDone, Channels: []string{"fax"}},
	} {
		_, err := alerts.Save(context.Background(), protocol.SaveAlertSettingsRequest{Routes: []protocol.AlertRouteChoice{
			{Event: notify.EventCIFailed, Channels: []string{"discord"}}, choice,
		}})
		if err == nil {
			t.Errorf("%s was saved", name)
		}
		if len(store.routes) != 0 {
			t.Errorf("%s left %v in the store, want nothing changed", name, store.routes)
		}
	}
}

func TestSavedChoicesAreAppliedWhenTheDaemonStarts(t *testing.T) {
	store := &memoryStore{routes: map[string][]string{notify.EventCardDone: {"ntfy"}}}
	alerts, router, rec := newAlerts(t, store)
	if err := alerts.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	router.Notify(context.Background(), notify.Event{Type: notify.EventCardDone, Title: "Finished: x"})
	router.Flush(context.Background())
	if len(rec.forChannel(notify.ChannelNtfy)) != 1 || len(rec.forChannel(notify.ChannelTelegram)) != 0 {
		t.Errorf("after a restart the router delivered %+v, want the saved choice", rec.all())
	}
}
