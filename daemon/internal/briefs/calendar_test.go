package briefs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/googlecal"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeEvents is a calendar that answers what it was given, and records the range it was asked for.
type fakeEvents struct {
	events  []googlecal.Event
	reading protocol.GoogleReading
	start   time.Time
	end     time.Time
}

func (f *fakeEvents) GoogleEvents(_ context.Context, start, end time.Time) ([]googlecal.Event, protocol.GoogleReading) {
	f.start, f.end = start, end
	return f.events, f.reading
}

var (
	zone  = time.FixedZone("EAT", 3*60*60)
	noon  = time.Date(2026, 10, 2, 12, 0, 0, 0, zone)
	today = time.Date(2026, 10, 2, 0, 0, 0, 0, zone)
)

func service(events EventSource) *Service {
	return &Service{events: events, now: func() time.Time { return noon }}
}

func at(hour, minute int) time.Time { return time.Date(2026, 10, 2, hour, minute, 0, 0, zone) }

func TestAMorningBriefListsTodaysEventsWithTimeAndPlace(t *testing.T) {
	fake := &fakeEvents{
		events: []googlecal.Event{
			{Title: "Holiday", AllDay: true, StartAt: today},
			{Title: "Standup", StartAt: at(9, 30), EndAt: at(9, 45), Location: "Room 2"},
		},
		reading: protocol.GoogleReading{Connected: true},
	}
	got := service(fake).calendarSection(context.Background(), protocol.Schedule{Name: "Morning brief"})
	want := "## Today's calendar\n- All day Holiday\n- 09:30-09:45 Standup (Room 2)"
	if got != want {
		t.Errorf("section = %q, want %q", got, want)
	}
	if !fake.start.Equal(today) || !fake.end.Equal(today.AddDate(0, 0, 1)) {
		t.Errorf("asked for %v to %v, want today's own day", fake.start, fake.end)
	}
}

func TestAnEveningBriefLooksAtTomorrow(t *testing.T) {
	fake := &fakeEvents{reading: protocol.GoogleReading{Connected: true}}
	got := service(fake).calendarSection(context.Background(), protocol.Schedule{Name: "Evening brief"})
	if got != "## Tomorrow's calendar\nNothing on the calendar." {
		t.Errorf("section = %q", got)
	}
	if !fake.start.Equal(today.AddDate(0, 0, 1)) {
		t.Errorf("asked from %v, want tomorrow", fake.start)
	}
}

func TestABriefHasNoCalendarPartWhenGoogleIsNotConnectedOrNoneIsWiredIn(t *testing.T) {
	if got := service(nil).calendarSection(context.Background(), protocol.Schedule{Name: "Morning brief"}); got != "" {
		t.Errorf("with no calendar wired in: %q, want nothing", got)
	}
	notConnected := &fakeEvents{}
	if got := service(notConnected).calendarSection(context.Background(), protocol.Schedule{Name: "Morning brief"}); got != "" {
		t.Errorf("not connected: %q, want nothing, so a brief never mentions a calendar the person lacks", got)
	}
}

func TestABriefSaysWhenGoogleCouldNotBeRead(t *testing.T) {
	down := &fakeEvents{reading: protocol.GoogleReading{Error: "Google Calendar could not be read just now."}}
	got := service(down).calendarSection(context.Background(), protocol.Schedule{Name: "Morning brief"})
	if !strings.Contains(got, "could not be read just now") {
		t.Errorf("section = %q, want the reason", got)
	}
	stale := &fakeEvents{
		events:  []googlecal.Event{{Title: "Standup", StartAt: at(9, 30)}},
		reading: protocol.GoogleReading{Connected: true, Stale: true, Error: "These are the last events Marshal read."},
	}
	got = service(stale).calendarSection(context.Background(), protocol.Schedule{Name: "Morning brief"})
	if !strings.Contains(got, "Standup") || !strings.Contains(got, "last events Marshal read") {
		t.Errorf("stale section = %q, want the events and the warning", got)
	}
}

func TestWhichBriefsAreEveningOnes(t *testing.T) {
	for _, tc := range []struct {
		name, time string
		want       bool
	}{
		{"Evening brief", "08:00", true},
		{"Morning brief", "18:00", false},
		{"Daily brief", "18:00", true},
		{"Daily brief", "07:30", false},
		{"Daily brief", "", false},
	} {
		if got := IsEvening(protocol.Schedule{Name: tc.name, Time: tc.time}); got != tc.want {
			t.Errorf("%q at %q: evening = %v, want %v", tc.name, tc.time, got, tc.want)
		}
	}
}
