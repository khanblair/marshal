// Package googlecal is Marshal's Google Calendar client: the OAuth2 config, the list of calendars a
// person has, and the call that reads their events for a date range (B8.3,
// docs/marshal-product-scope.md 18-19). Read-only: the scope Marshal asks for cannot write even if
// the code tried to (hard rule 2).
package googlecal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
	oauthgoogle "golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// Scope is what Marshal asks Google Calendar for.
const Scope = calendar.CalendarReadonlyScope

// Endpoint is Google's own OAuth2 endpoint. A test passes another, so the consent flow can be run
// against a fake server.
var Endpoint = oauthgoogle.Endpoint

// Config builds the OAuth2 config for a Google connection. redirectURL is the daemon's own callback
// address. scopes are exactly what is asked for, and nothing more: Calendar asks for its own scope
// and Gmail for its own, each in its own consent. A zero endpoint means Google's own.
func Config(clientID, clientSecret, redirectURL string, endpoint oauth2.Endpoint, scopes ...string) oauth2.Config {
	if endpoint == (oauth2.Endpoint{}) {
		endpoint = Endpoint
	}
	return oauth2.Config{
		ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
		Scopes: scopes, Endpoint: endpoint,
	}
}

// Client wraps the generated Calendar service.
type Client struct {
	svc *calendar.Service
}

// New builds a Client. httpClient is normally an OAuth2 config's own client for a live token; a
// test passes one that never leaves the process. baseURL overrides where the API is reached, for
// a test; empty means the real Google API.
func New(ctx context.Context, httpClient *http.Client, baseURL string) (*Client, error) {
	svc, err := calendar.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("build the Calendar client: %w", err)
	}
	if baseURL != "" {
		svc.BasePath = baseURL
	}
	return &Client{svc: svc}, nil
}

// CalendarInfo is one calendar a person has: their own, one made for a team, one shared with them,
// or one they subscribed to, such as public holidays.
type CalendarInfo struct {
	ID   string
	Name string
	// Primary is the calendar named after the person's own address.
	Primary bool
	// Owned says the person owns it, as opposed to being shared it or subscribed to it.
	Owned bool
	// SelectedInGoogle is whether the calendar is ticked in Google Calendar's own side list.
	SelectedInGoogle bool
	// Color is the calendar's own background color, a #rrggbb string, or empty.
	Color string
}

// Calendars lists every calendar the token can read. It also proves the token works, so the
// connection test uses it.
func (c *Client) Calendars(ctx context.Context) ([]CalendarInfo, error) {
	var out []CalendarInfo
	token := ""
	for {
		var list *calendar.CalendarList
		err := withRetry(ctx, func() (err error) {
			call := c.svc.CalendarList.List().Context(ctx)
			if token != "" {
				call = call.PageToken(token)
			}
			list, err = call.Do()
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("list the calendars this token can read: %w", err)
		}
		for _, item := range list.Items {
			if item.Deleted || item.Hidden && !item.Selected {
				continue
			}
			name := item.SummaryOverride
			if name == "" {
				name = item.Summary
			}
			out = append(out, CalendarInfo{
				ID: item.Id, Name: name, Primary: item.Primary, Owned: item.AccessRole == "owner",
				SelectedInGoogle: item.Selected, Color: item.BackgroundColor,
			})
		}
		if list.NextPageToken == "" {
			return out, nil
		}
		token = list.NextPageToken
	}
}

// Event is one event Marshal reads back.
type Event struct {
	ID         string
	CalendarID string
	// CalendarName is the name of the calendar the event is on, so a person can tell them apart.
	CalendarName string
	Title        string
	StartAt      time.Time
	// EndAt is when the event ends. For an all-day event it is the first day it no longer covers,
	// which is how Google states it. Zero when Google gave none.
	EndAt  time.Time
	AllDay bool
	// Location is the place, as typed. Empty when there is none.
	Location string
	// URL opens the event in Google Calendar.
	URL string
	// JoinURL is the video call link, when the event has one.
	JoinURL string
}

// Ref names a calendar to read, by id, with the name to show on its events.
type Ref struct {
	ID   string
	Name string
}

// Events reads each calendar for [start, end), earliest first, expanding a recurring event into its
// own occurrences. A cancelled event and one the person declined are left out: they are not on the
// person's day. A calendar that cannot be read (one shared with them that was since unshared, say)
// is skipped, and named in the returned error only when none could be read, so one bad calendar
// never hides the rest.
func (c *Client) Events(ctx context.Context, calendars []Ref, start, end time.Time) ([]Event, error) {
	var events []Event
	var failures []error
	for _, ref := range calendars {
		found, err := c.calendarEvents(ctx, ref, start, end)
		if err != nil {
			failures = append(failures, fmt.Errorf("calendar %q: %w", ref.Name, err))
			continue
		}
		events = append(events, found...)
	}
	if len(calendars) > 0 && len(failures) == len(calendars) {
		return nil, fmt.Errorf("list calendar events: %w", errors.Join(failures...))
	}
	slices.SortStableFunc(events, func(a, b Event) int { return a.StartAt.Compare(b.StartAt) })
	return events, nil
}

func (c *Client) calendarEvents(ctx context.Context, ref Ref, start, end time.Time) ([]Event, error) {
	var out []Event
	token := ""
	for {
		var answer *calendar.Events
		err := withRetry(ctx, func() (err error) {
			call := c.svc.Events.List(ref.ID).
				TimeMin(start.Format(time.RFC3339)).TimeMax(end.Format(time.RFC3339)).
				SingleEvents(true).OrderBy("startTime").MaxResults(250).Context(ctx)
			if token != "" {
				call = call.PageToken(token)
			}
			answer, err = call.Do()
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, item := range answer.Items {
			if item.Status == "cancelled" || declined(item) {
				continue
			}
			out = append(out, eventOf(item, ref))
		}
		if answer.NextPageToken == "" {
			return out, nil
		}
		token = answer.NextPageToken
	}
}

// declined says the person turned the event down, which takes it off their day.
func declined(item *calendar.Event) bool {
	for _, attendee := range item.Attendees {
		if attendee.Self && attendee.ResponseStatus == "declined" {
			return true
		}
	}
	return false
}

// eventOf reads one Google event: its title, when it starts and ends - a date for an all-day event,
// a full date-time otherwise - and where to find it. An unparseable time is left zero rather than
// failing the whole read.
func eventOf(item *calendar.Event, ref Ref) Event {
	event := Event{
		ID: item.Id, CalendarID: ref.ID, CalendarName: ref.Name, Title: item.Summary,
		Location: item.Location, URL: item.HtmlLink, JoinURL: joinURL(item),
	}
	if item.Start == nil {
		return event
	}
	if item.Start.Date != "" {
		event.AllDay = true
		event.StartAt, _ = time.ParseInLocation("2006-01-02", item.Start.Date, time.Local)
		if item.End != nil && item.End.Date != "" {
			event.EndAt, _ = time.ParseInLocation("2006-01-02", item.End.Date, time.Local)
		}
		return event
	}
	event.StartAt, _ = time.Parse(time.RFC3339, item.Start.DateTime)
	if item.End != nil {
		event.EndAt, _ = time.Parse(time.RFC3339, item.End.DateTime)
	}
	return event
}

// joinURL is the event's video call link: the one in its conference data, or the older Meet link.
func joinURL(item *calendar.Event) string {
	if item.ConferenceData != nil {
		for _, entry := range item.ConferenceData.EntryPoints {
			if entry.EntryPointType == "video" && entry.Uri != "" {
				return entry.Uri
			}
		}
	}
	return item.HangoutLink
}

// retryDelay is how long the first retry waits; each one waits twice as long as the one before. A
// test shortens it.
var retryDelay = 300 * time.Millisecond

const maxTries = 3

// withRetry runs one Google call, and tries it again when Google says it is busy or broken for a
// moment (429 and 5xx), up to three times. Any other answer, a refused token included, returns at
// once: waiting would not change it.
func withRetry(ctx context.Context, call func() error) error {
	delay := retryDelay
	var err error
	for try := 1; ; try++ {
		if err = call(); err == nil || try == maxTries || !temporary(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// temporary says a Google error is worth trying again.
func temporary(err error) bool {
	var api *googleapi.Error
	if !errors.As(err, &api) {
		return false
	}
	if api.Code == http.StatusTooManyRequests || api.Code >= http.StatusInternalServerError {
		return true
	}
	// Google reports a quota hit as a 403 with one of these reasons.
	for _, item := range api.Errors {
		if strings.Contains(item.Reason, "rateLimitExceeded") || strings.Contains(item.Reason, "userRateLimitExceeded") {
			return true
		}
	}
	return false
}
