package notify

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// RouteStore keeps the routes a person chose, by event name, across restarts. The daemon's is the
// settings table (internal/settings).
type RouteStore interface {
	// AlertRoutes reads the saved routes. None saved is an empty map, not an error.
	AlertRoutes(ctx context.Context) (map[string][]string, error)
	// SetAlertRoutes stores the routes, replacing what was saved.
	SetAlertRoutes(ctx context.Context, routes map[string][]string) error
}

// QuietStore keeps whether alerts are held during calendar events, across restarts. A RouteStore may
// also be one; when it is not, the choice lasts until the daemon restarts.
type QuietStore interface {
	AlertQuiet(ctx context.Context) (bool, error)
	SetAlertQuiet(ctx context.Context, quiet bool) error
}

// alertEvent is one kind of alert a person can route, and the words the screen calls it.
type alertEvent struct {
	event string
	label string
}

// alertEvents are the alerts the Settings screen offers, in the order it shows them.
func alertEvents() []alertEvent {
	return []alertEvent{
		{EventApproval, "Needs your approval"},
		{EventAgentStuck, "An agent is stuck or needs you"},
		{EventCIFailed, "CI failed"},
		{EventCardDone, "A card finished"},
		{EventNeedsYou, "Notices from Marshal"},
	}
}

// alertChannels are the places an alert can go, in the order the screen shows them.
func alertChannels() []alertChannel {
	return []alertChannel{
		{ChannelTelegram, "Telegram"},
		{ChannelDiscord, "Discord"},
		{ChannelNtfy, "ntfy"},
	}
}

type alertChannel struct {
	id   Channel
	name string
}

// Alerts is the alert settings: the router's routing table as the Settings screen reads and writes
// it, kept across restarts. It changes the running router at once, so a choice takes effect without
// a restart.
type Alerts struct {
	router    *Service
	store     RouteStore
	connected func(ctx context.Context) map[Channel]bool
	now       func() time.Time

	mu sync.Mutex
	// quiet is whether alerts are held during calendar events, and inEvent says whether one is on.
	quiet   bool
	inEvent func(ctx context.Context) bool
	// busyAt and busy keep the last answer of inEvent for a short while, so the router asking every
	// flush does not ask the calendar every flush.
	busyAt time.Time
	busy   bool
}

// busyFor is how long the answer to "is an event on" is kept.
const busyFor = 30 * time.Second

// SetInEvent says how to tell that a calendar event is on. Without it nothing is ever held.
func (a *Alerts) SetInEvent(inEvent func(ctx context.Context) bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inEvent = inEvent
}

// Quiet says whether notices should be held right now: the person asked for it, and an event is on.
// Google that cannot be read never holds a notice back.
func (a *Alerts) Quiet(ctx context.Context) bool {
	a.mu.Lock()
	quiet, inEvent := a.quiet, a.inEvent
	if !quiet || inEvent == nil {
		a.mu.Unlock()
		return false
	}
	if a.now().Sub(a.busyAt) < busyFor {
		busy := a.busy
		a.mu.Unlock()
		return busy
	}
	a.mu.Unlock()
	busy := inEvent(ctx)
	a.mu.Lock()
	a.busy, a.busyAt = busy, a.now()
	a.mu.Unlock()
	return busy
}

// NewAlerts builds the alert settings over a router and a store. connected answers which channels
// have a working connection; nil means none is known to.
func NewAlerts(router *Service, store RouteStore, connected func(ctx context.Context) map[Channel]bool, now func() time.Time) (*Alerts, error) {
	if router == nil || store == nil {
		return nil, fmt.Errorf("alert settings need a router and somewhere to keep the choices")
	}
	if now == nil {
		now = time.Now
	}
	return &Alerts{router: router, store: store, connected: connected, now: now}, nil
}

// Load applies the saved routes to the router. It is called once at start, so a choice made before
// the last restart is still the one the daemon follows.
func (a *Alerts) Load(ctx context.Context) error {
	saved, err := a.store.AlertRoutes(ctx)
	if err != nil {
		return err
	}
	for event, channels := range saved {
		a.router.SetRoute(event, toChannels(channels)...)
	}
	if store, ok := a.store.(QuietStore); ok {
		quiet, err := store.AlertQuiet(ctx)
		if err != nil {
			return err
		}
		a.setQuiet(quiet)
	}
	return nil
}

func (a *Alerts) setQuiet(quiet bool) {
	a.mu.Lock()
	a.quiet, a.busyAt = quiet, time.Time{}
	a.mu.Unlock()
}

// Get answers where every kind of alert goes now, and which channels can be used.
func (a *Alerts) Get(ctx context.Context) (protocol.AlertSettings, error) {
	live := map[string][]Channel{}
	for _, route := range a.router.Routes() {
		live[route.Event] = route.Channels
	}
	routes := make([]protocol.AlertRoute, 0, len(alertEvents()))
	for _, one := range alertEvents() {
		routes = append(routes, protocol.AlertRoute{
			Event: one.event, Label: one.label, Channels: fromChannels(live[one.event]),
		})
	}
	var up map[Channel]bool
	if a.connected != nil {
		up = a.connected(ctx)
	}
	channels := make([]protocol.AlertChannel, 0, len(alertChannels()))
	for _, one := range alertChannels() {
		channels = append(channels, protocol.AlertChannel{ID: string(one.id), Name: one.name, Connected: up[one.id]})
	}
	a.mu.Lock()
	quiet := a.quiet
	a.mu.Unlock()
	return protocol.AlertSettings{
		Routes: routes, Channels: channels, QuietDuringEvents: quiet, ServerTime: protocol.NewTimestamp(a.now()),
	}, nil
}

// Save checks the choices, keeps them, applies them to the router, and answers the settings as they
// now are. Nothing is changed when any choice is refused, so one bad row cannot half-apply a save.
func (a *Alerts) Save(ctx context.Context, req protocol.SaveAlertSettingsRequest) (protocol.AlertSettings, error) {
	for _, choice := range req.Routes {
		if err := checkChoice(choice); err != nil {
			return protocol.AlertSettings{}, err
		}
	}
	saved, err := a.store.AlertRoutes(ctx)
	if err != nil {
		return protocol.AlertSettings{}, err
	}
	for _, choice := range req.Routes {
		saved[choice.Event] = dedupe(choice.Channels)
	}
	if err := a.store.SetAlertRoutes(ctx, saved); err != nil {
		return protocol.AlertSettings{}, err
	}
	for _, choice := range req.Routes {
		a.router.SetRoute(choice.Event, toChannels(dedupe(choice.Channels))...)
	}
	if req.QuietDuringEvents != nil {
		if store, ok := a.store.(QuietStore); ok {
			if err := store.SetAlertQuiet(ctx, *req.QuietDuringEvents); err != nil {
				return protocol.AlertSettings{}, err
			}
		}
		a.setQuiet(*req.QuietDuringEvents)
	}
	return a.Get(ctx)
}

// checkChoice refuses an alert or a channel the screen does not offer, in the sentence a person can
// act on, so a mistyped body is not stored as a route nothing will ever follow.
func checkChoice(choice protocol.AlertRouteChoice) error {
	if !slices.ContainsFunc(alertEvents(), func(one alertEvent) bool { return one.event == choice.Event }) {
		return protocol.InvalidArgument("Marshal has no alert of that kind.").With("event", choice.Event)
	}
	for _, channel := range choice.Channels {
		if !slices.ContainsFunc(alertChannels(), func(one alertChannel) bool { return string(one.id) == channel }) {
			return protocol.InvalidArgument("Choose Telegram, Discord, or ntfy.").With("channel", channel)
		}
	}
	return nil
}

func dedupe(channels []string) []string {
	out := make([]string, 0, len(channels))
	for _, channel := range channels {
		if !slices.Contains(out, channel) {
			out = append(out, channel)
		}
	}
	return out
}

// toChannels turns channel ids into channels, always non-nil, because SetRoute reads a nil list as
// "leave the default" while an empty one is "off".
func toChannels(ids []string) []Channel {
	out := make([]Channel, 0, len(ids))
	for _, id := range ids {
		out = append(out, Channel(id))
	}
	return out
}

func fromChannels(channels []Channel) []string {
	out := make([]string, 0, len(channels))
	for _, channel := range channels {
		out = append(out, string(channel))
	}
	return out
}
