// Package googlecal is Marshal's Google Calendar client: the OAuth2 config, and the one call that
// reads a person's primary calendar for a date range (B8.3, docs/marshal-product-scope.md 18-19).
// Read-only: the scope Marshal asks for cannot write even if the code tried to (hard rule 2).
package googlecal

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	oauthgoogle "golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// Scope is what Marshal asks Google Calendar for.
const Scope = calendar.CalendarReadonlyScope

// Config builds the OAuth2 config for the Calendar connection. redirectURL is the daemon's own
// callback address. extraScopes are asked for in the same consent, alongside Calendar's own - the
// one client Gmail's own connection shares (B8.3, phase-reports/phase-08-automation.md §6).
func Config(clientID, clientSecret, redirectURL string, extraScopes ...string) oauth2.Config {
	return oauth2.Config{
		ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
		Scopes: append([]string{Scope}, extraScopes...), Endpoint: oauthgoogle.Endpoint,
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

// Event is one event Marshal reads back.
type Event struct {
	ID      string
	Title   string
	StartAt time.Time
	AllDay  bool
}

// Events reads the primary calendar for [start, end), earliest first, expanding a recurring
// event into its own occurrences.
func (c *Client) Events(ctx context.Context, start, end time.Time) ([]Event, error) {
	answer, err := c.svc.Events.List("primary").
		TimeMin(start.Format(time.RFC3339)).TimeMax(end.Format(time.RFC3339)).
		SingleEvents(true).OrderBy("startTime").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list calendar events: %w", err)
	}
	events := make([]Event, 0, len(answer.Items))
	for _, item := range answer.Items {
		events = append(events, eventOf(item))
	}
	return events, nil
}

// eventOf reads one Google event's id, title, and start - a date for an all-day event, a full
// date-time otherwise. An unparseable start is left zero rather than failing the whole read.
func eventOf(item *calendar.Event) Event {
	if item.Start == nil {
		return Event{ID: item.Id, Title: item.Summary}
	}
	if item.Start.Date != "" {
		at, _ := time.Parse("2006-01-02", item.Start.Date)
		return Event{ID: item.Id, Title: item.Summary, StartAt: at, AllDay: true}
	}
	at, _ := time.Parse(time.RFC3339, item.Start.DateTime)
	return Event{ID: item.Id, Title: item.Summary, StartAt: at}
}

// About answers who the token belongs to and how many calendars it can read, for the connection
// test. It reads the calendar list, the cheapest call that proves the token actually works.
func (c *Client) About(ctx context.Context) (calendars int, err error) {
	list, err := c.svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("list the calendars this token can read: %w", err)
	}
	return len(list.Items), nil
}
