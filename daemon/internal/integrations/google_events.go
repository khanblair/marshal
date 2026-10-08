package integrations

// This file is what Marshal reads from Google Calendar: which calendars a person has and which of
// them Marshal reads, and the events in a date range. Every screen and the briefs ask through here,
// so one short-lived cache stands between them and Google, and a Google that is down shows the last
// events read instead of nothing.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/khanblair/marshal/daemon/internal/integrations/googlecal"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// eventsFreshFor is how long a read of events is reused. Short enough that a new event shows
	// within a minute, long enough that Home, the calendar, and a brief do not each ask Google.
	eventsFreshFor = time.Minute
	// calendarsFreshFor is how long the list of calendars is reused.
	calendarsFreshFor = 10 * time.Minute
	// eventsStaleFor is how old a read may be and still be shown when Google cannot be reached.
	eventsStaleFor = 24 * time.Hour
)

// Words a person reads when Google's events are missing or old.
const (
	googleReconnectSentence = "Google no longer accepts Marshal's access. Reconnect Google Calendar in Settings."
	unreadableSentence      = "Google Calendar could not be read just now. Try again in a moment."
	staleSentence           = "Google Calendar could not be reached just now. These are the last events Marshal read."
)

// readCache keeps what was last read from Google, per connection grant, so another person's or an
// older grant's events are never shown after a reconnect.
type readCache struct {
	mu        sync.Mutex
	grant     string
	calendars cachedCalendars
	events    map[string]cachedEvents
}

type cachedCalendars struct {
	at   time.Time
	list []googlecal.CalendarInfo
}

type cachedEvents struct {
	at     time.Time
	events []googlecal.Event
}

// forgetGoogleReads drops everything read from Google, after a connection changes.
func (s *Service) forgetGoogleReads() {
	s.forgetFolders()
	s.google.read.mu.Lock()
	defer s.google.read.mu.Unlock()
	s.google.read.grant = ""
	s.google.read.calendars = cachedCalendars{}
	s.google.read.events = nil
}

// grantOf names the access a read was made with, so the cache is never shared across grants.
func grantOf(refresh string) string {
	sum := sha256.Sum256([]byte(refresh))
	return hex.EncodeToString(sum[:8])
}

// calendarList reads the person's calendars, from the cache when it is fresh.
func (s *Service) calendarList(ctx context.Context, client *googlecal.Client, grant string) ([]googlecal.CalendarInfo, error) {
	cache := &s.google.read
	cache.mu.Lock()
	if cache.grant == grant && s.now().Sub(cache.calendars.at) < calendarsFreshFor && cache.calendars.list != nil {
		list := cache.calendars.list
		cache.mu.Unlock()
		return list, nil
	}
	cache.mu.Unlock()
	list, err := client.Calendars(ctx)
	if err != nil {
		return nil, err
	}
	cache.mu.Lock()
	if cache.grant != grant {
		cache.grant, cache.events = grant, nil
	}
	cache.calendars = cachedCalendars{at: s.now(), list: list}
	cache.mu.Unlock()
	return list, nil
}

// selectedCalendars is the calendars Marshal reads: the ones the person chose, or, until they
// choose, the ones ticked in Google Calendar's own side list.
func selectedCalendars(config gcalConfig, all []googlecal.CalendarInfo) []googlecal.CalendarInfo {
	var out []googlecal.CalendarInfo
	for _, info := range all {
		selected := info.SelectedInGoogle
		if config.CalendarsChosen {
			selected = slices.Contains(config.Calendars, info.ID)
		}
		if selected {
			out = append(out, info)
		}
	}
	return out
}

// GoogleCalendars lists every calendar the person has, with whether Marshal reads it, for the tick
// list in Settings. ErrNotConnected means Calendar's consent has not completed.
func (s *Service) GoogleCalendars(ctx context.Context) (protocol.GoogleCalendarChoices, error) {
	client, token, err := s.calendarClient(ctx)
	if err != nil {
		return protocol.GoogleCalendarChoices{}, err
	}
	all, err := s.calendarList(ctx, client, grantOf(token))
	if err != nil {
		return protocol.GoogleCalendarChoices{}, err
	}
	config, _, err := s.readGCal(ctx)
	if err != nil {
		return protocol.GoogleCalendarChoices{}, err
	}
	picked := selectedCalendars(config, all)
	out := protocol.GoogleCalendarChoices{Calendars: make([]protocol.GoogleCalendarChoice, 0, len(all)), Chosen: config.CalendarsChosen}
	for _, info := range all {
		out.Calendars = append(out.Calendars, protocol.GoogleCalendarChoice{
			ID: info.ID, Name: info.Name, Color: info.Color, Primary: info.Primary, Owned: info.Owned,
			Selected: slices.ContainsFunc(picked, func(p googlecal.CalendarInfo) bool { return p.ID == info.ID }),
		})
	}
	return out, nil
}

// SetGoogleCalendars saves which calendars Marshal reads. An id Google does not list is refused, so
// a typo never silently reads nothing. An empty list is valid: none.
func (s *Service) SetGoogleCalendars(ctx context.Context, ids []string) error {
	client, token, err := s.calendarClient(ctx)
	if err != nil {
		return err
	}
	all, err := s.calendarList(ctx, client, grantOf(token))
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !slices.ContainsFunc(all, func(info googlecal.CalendarInfo) bool { return info.ID == id }) {
			return protocol.InvalidArgument("One of those calendars is not one of yours in Google Calendar.").With("id", id)
		}
	}
	config, _, err := s.readGCal(ctx)
	if err != nil {
		return err
	}
	config.Calendars = slices.Compact(slices.Sorted(slices.Values(ids)))
	config.CalendarsChosen = true
	if err := s.writeGCal(ctx, config); err != nil {
		return err
	}
	s.google.read.mu.Lock()
	s.google.read.events = nil
	s.google.read.mu.Unlock()
	return nil
}

// calendarClient builds Calendar's client and names the grant behind it.
func (s *Service) calendarClient(ctx context.Context) (*googlecal.Client, string, error) {
	fresh, err := s.freshGoogleToken(ctx, GCalID)
	if err != nil {
		return nil, "", err
	}
	client, err := googlecal.New(ctx, oauth2.NewClient(ctx, oauth2.StaticTokenSource(fresh)), s.gcalBase)
	if err != nil {
		return nil, "", err
	}
	return client, fresh.RefreshToken, nil
}

// GoogleEvents reads the events of every calendar Marshal reads, for [start, end). It never fails:
// how the read went comes back as a protocol.GoogleReading, so the calendar view can still show its
// schedules and due cards when Google cannot be read, and say why Google's events are missing.
func (s *Service) GoogleEvents(ctx context.Context, start, end time.Time) ([]googlecal.Event, protocol.GoogleReading) {
	events, reading := s.googleEvents(ctx, start, end)
	return anchorAllDay(events, s.now().Location()), reading
}

// anchorAllDay puts each all-day event's start and end at midnight of its own dates in the zone
// the person chose, so an event "on the 9th" starts when their 9th does. It works on a copy: the
// cache keeps what Google said, and a zone chosen later reads the same cached events correctly.
func anchorAllDay(events []googlecal.Event, loc *time.Location) []googlecal.Event {
	out := make([]googlecal.Event, len(events))
	copy(out, events)
	for i := range out {
		if !out[i].AllDay {
			continue
		}
		if at, err := time.ParseInLocation("2006-01-02", out[i].StartDate, loc); err == nil {
			out[i].StartAt = at
		}
		if at, err := time.ParseInLocation("2006-01-02", out[i].EndDate, loc); err == nil {
			out[i].EndAt = at
		}
	}
	return out
}

func (s *Service) googleEvents(ctx context.Context, start, end time.Time) ([]googlecal.Event, protocol.GoogleReading) {
	client, refresh, err := s.calendarClient(ctx)
	switch {
	case errors.Is(err, ErrNotConnected) || errors.Is(err, ErrNoGoogleClient):
		return nil, protocol.GoogleReading{}
	case errors.Is(err, ErrNeedsReconnect):
		return nil, protocol.GoogleReading{Error: googleReconnectSentence}
	case err != nil:
		s.log.Warn("Google Calendar could not be set up for a read", "err", err)
		return nil, protocol.GoogleReading{Error: unreadableSentence}
	}
	grant := grantOf(refresh)
	key := fmt.Sprintf("%d-%d", start.Unix(), end.Unix())
	cache := &s.google.read
	cache.mu.Lock()
	hit, hasHit := cache.events[key]
	sameGrant := cache.grant == grant
	cache.mu.Unlock()
	if hasHit && sameGrant && s.now().Sub(hit.at) < eventsFreshFor {
		return hit.events, protocol.GoogleReading{Connected: true}
	}
	events, err := s.readEvents(ctx, client, grant, start, end)
	if err == nil {
		cache.mu.Lock()
		if cache.events == nil {
			cache.events = map[string]cachedEvents{}
		}
		cache.events[key] = cachedEvents{at: s.now(), events: events}
		cache.mu.Unlock()
		return events, protocol.GoogleReading{Connected: true}
	}
	if refused(err) {
		return nil, protocol.GoogleReading{Error: googleReconnectSentence}
	}
	s.log.Warn("Google Calendar events could not be read", "err", err)
	if hasHit && sameGrant && s.now().Sub(hit.at) < eventsStaleFor {
		return hit.events, protocol.GoogleReading{Connected: true, Stale: true, Error: staleSentence}
	}
	return nil, protocol.GoogleReading{Connected: true, Error: unreadableSentence}
}

// readEvents asks Google for the events of the calendars Marshal reads.
func (s *Service) readEvents(ctx context.Context, client *googlecal.Client, grant string, start, end time.Time) ([]googlecal.Event, error) {
	all, err := s.calendarList(ctx, client, grant)
	if err != nil {
		return nil, err
	}
	config, _, err := s.readGCal(ctx)
	if err != nil {
		return nil, err
	}
	var refs []googlecal.Ref
	for _, info := range selectedCalendars(config, all) {
		refs = append(refs, googlecal.Ref{ID: info.ID, Name: info.Name})
	}
	if len(refs) == 0 {
		return []googlecal.Event{}, nil
	}
	return client.Events(ctx, refs, start, end)
}

// EventNow answers the timed event that is on at t and makes the person busy, if there is one. An
// event they marked as available does not count. Google that cannot be read, or
// is not connected, is no event: nothing is ever held back because of it. It reads the same two days
// the scheduler's watcher does, so the two share one cached call.
func (s *Service) EventNow(ctx context.Context, t time.Time) (googlecal.Event, bool) {
	t = t.In(s.now().Location())
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	events, reading := s.GoogleEvents(ctx, start, start.Add(2*24*time.Hour))
	if !reading.Connected || reading.Error != "" {
		return googlecal.Event{}, false
	}
	for _, event := range events {
		if !event.AllDay && !event.Free && !event.StartAt.After(t) && event.EndAt.After(t) {
			return event, true
		}
	}
	return googlecal.Event{}, false
}

// EventNamed finds the first event in [start, end) whose title starts with name, ignoring case.
// Briefs use it to take their time from an event called "Morning brief" or "Evening brief".
func EventNamed(events []googlecal.Event, name string) (googlecal.Event, bool) {
	for _, event := range events {
		if !event.AllDay && strings.HasPrefix(strings.ToLower(strings.TrimSpace(event.Title)), strings.ToLower(name)) {
			return event, true
		}
	}
	return googlecal.Event{}, false
}
