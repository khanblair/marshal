package googlecal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fake runs a Calendar API that answers by path, and returns a client pointed at it.
func fake(t *testing.T, routes map[string]http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for prefix, handler := range routes {
			if strings.Contains(r.URL.Path, prefix) {
				handler(w, r)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := New(context.Background(), server.Client(), server.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func json(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

var (
	rangeStart = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd   = time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
)

func TestConfigAsksForExactlyTheScopesGiven(t *testing.T) {
	cfg := Config("id", "secret", "http://127.0.0.1:1/cb", Endpoint, Scope)
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != Scope {
		t.Errorf("scopes = %v, want only the calendar scope", cfg.Scopes)
	}
	if cfg.Endpoint != Endpoint {
		t.Error("a zero endpoint must mean Google's own")
	}
}

func TestCalendarsListsEveryCalendarWithItsKind(t *testing.T) {
	client := fake(t, map[string]http.HandlerFunc{"calendarList": json(`{"items":[
		{"id":"me@x.com","summary":"me@x.com","primary":true,"accessRole":"owner","selected":true,"backgroundColor":"#9fe1e7"},
		{"id":"work","summary":"Work","summaryOverride":"My work","accessRole":"owner","selected":true},
		{"id":"team","summary":"Team","accessRole":"reader","selected":false},
		{"id":"holidays","summary":"Holidays","accessRole":"reader","selected":true},
		{"id":"gone","summary":"Gone","deleted":true}
	]}`)})
	got, err := client.Calendars(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("calendars = %+v, want 4 (a deleted one is left out)", got)
	}
	if !got[0].Primary || !got[0].Owned || got[0].Color != "#9fe1e7" {
		t.Errorf("primary = %+v", got[0])
	}
	if got[1].Name != "My work" || !got[1].Owned {
		t.Errorf("work = %+v, want the person's own name for it", got[1])
	}
	if got[2].Owned || got[2].SelectedInGoogle {
		t.Errorf("team = %+v, want shared and not ticked in Google", got[2])
	}
}

func TestEventsReadTimesAllDayLinksAndLeaveOutCancelledAndDeclined(t *testing.T) {
	client := fake(t, map[string]http.HandlerFunc{"calendars/main/events": json(`{"items":[
		{"id":"a","summary":"Standup","status":"confirmed","location":"Room 2","htmlLink":"https://g/a",
		 "hangoutLink":"https://meet/x","start":{"dateTime":"2026-10-02T09:30:00+03:00"},"end":{"dateTime":"2026-10-02T10:00:00+03:00"}},
		{"id":"b","summary":"Holiday","start":{"date":"2026-10-03"},"end":{"date":"2026-10-04"}},
		{"id":"c","summary":"Gone","status":"cancelled","start":{"dateTime":"2026-10-02T11:00:00Z"}},
		{"id":"d","summary":"Turned down","start":{"dateTime":"2026-10-02T12:00:00Z"},
		 "attendees":[{"self":true,"responseStatus":"declined"}]},
		{"id":"e","summary":"Call","start":{"dateTime":"2026-10-02T08:00:00Z"},"end":{"dateTime":"2026-10-02T08:30:00Z"},
		 "conferenceData":{"entryPoints":[{"entryPointType":"phone","uri":"tel:1"},{"entryPointType":"video","uri":"https://zoom/1"}]}}
	]}`)})
	events, err := client.Events(context.Background(), []Ref{{ID: "main", Name: "Main"}}, rangeStart, rangeEnd)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, e := range events {
		titles = append(titles, e.Title)
	}
	if strings.Join(titles, ",") != "Standup,Call,Holiday" {
		t.Fatalf("events = %v, want Standup, Call, Holiday in time order with the cancelled and declined left out", titles)
	}
	standup := events[0]
	if standup.Location != "Room 2" || standup.URL != "https://g/a" || standup.JoinURL != "https://meet/x" ||
		standup.CalendarName != "Main" || standup.EndAt.Sub(standup.StartAt) != 30*time.Minute || standup.AllDay {
		t.Errorf("standup = %+v", standup)
	}
	if !events[2].AllDay || events[2].EndAt.Sub(events[2].StartAt) != 24*time.Hour {
		t.Errorf("holiday = %+v, want all-day lasting one day", events[2])
	}
	if events[1].JoinURL != "https://zoom/1" {
		t.Errorf("call join link = %q, want the video entry point", events[1].JoinURL)
	}
}

func TestOneBadCalendarDoesNotHideTheOthers(t *testing.T) {
	client := fake(t, map[string]http.HandlerFunc{
		"calendars/good/events": json(`{"items":[{"id":"a","summary":"Fine","start":{"dateTime":"2026-10-02T09:00:00Z"}}]}`),
		"calendars/bad/events": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error":{"code":404}}`, http.StatusNotFound)
		},
	})
	refs := []Ref{{ID: "bad", Name: "Bad"}, {ID: "good", Name: "Good"}}
	events, err := client.Events(context.Background(), refs, rangeStart, rangeEnd)
	if err != nil || len(events) != 1 || events[0].Title != "Fine" {
		t.Fatalf("events = %+v, err = %v, want the good calendar's event and no error", events, err)
	}
	if _, err := client.Events(context.Background(), refs[:1], rangeStart, rangeEnd); err == nil {
		t.Error("when every calendar fails the read must fail")
	}
}

func TestABusyGoogleIsTriedAgainButARefusalIsNot(t *testing.T) {
	retryDelay = time.Millisecond
	t.Cleanup(func() { retryDelay = 300 * time.Millisecond })
	var busy, refused atomic.Int32
	client := fake(t, map[string]http.HandlerFunc{
		"calendars/busy/events": func(w http.ResponseWriter, r *http.Request) {
			if busy.Add(1) < 3 {
				http.Error(w, `{"error":{"code":503}}`, http.StatusServiceUnavailable)
				return
			}
			json(`{"items":[{"id":"a","summary":"Late","start":{"dateTime":"2026-10-02T09:00:00Z"}}]}`)(w, r)
		},
		"calendars/denied/events": func(w http.ResponseWriter, _ *http.Request) {
			refused.Add(1)
			http.Error(w, `{"error":{"code":401}}`, http.StatusUnauthorized)
		},
	})
	events, err := client.Events(context.Background(), []Ref{{ID: "busy", Name: "Busy"}}, rangeStart, rangeEnd)
	if err != nil || len(events) != 1 || busy.Load() != 3 {
		t.Fatalf("events = %+v, err = %v, tries = %d, want success on the third try", events, err, busy.Load())
	}
	if _, err := client.Events(context.Background(), []Ref{{ID: "denied", Name: "D"}}, rangeStart, rangeEnd); err == nil || refused.Load() != 1 {
		t.Errorf("err = %v, tries = %d, want one try and an error for a refused token", err, refused.Load())
	}
}
