package schedules_test

import (
	"context"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/schedules"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

func TestParseCalendarTriggerReadsTheSentencesAPersonWrites(t *testing.T) {
	for _, tc := range []struct {
		when string
		want schedules.CalendarTrigger
		ok   bool
	}{
		{"When 30 minutes before my first calendar event", schedules.CalendarTrigger{Before: 30 * time.Minute, FirstOfDay: true}, true},
		{"when 1 hour before my first event of the day", schedules.CalendarTrigger{Before: time.Hour, FirstOfDay: true}, true},
		{"When a calendar event starts", schedules.CalendarTrigger{}, true},
		{`When a calendar event named "Morning brief" starts`, schedules.CalendarTrigger{Title: "Morning brief"}, true},
		{`When 10 minutes before an event called "Design review"`, schedules.CalendarTrigger{Before: 10 * time.Minute, Title: "Design review"}, true},
		{"When 2 days before my first event", schedules.CalendarTrigger{}, false},
		{"Every weekday at 9:00", schedules.CalendarTrigger{}, false},
		{"When the build fails", schedules.CalendarTrigger{}, false},
	} {
		got, ok := schedules.ParseCalendarTrigger(tc.when)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%q = %+v, %v; want %+v, %v", tc.when, got, ok, tc.want, tc.ok)
		}
	}
}

// fakeCalendar answers the events it holds, or says Google cannot be read.
type fakeCalendar struct {
	events []schedules.CalendarEvent
	down   bool
}

func (f *fakeCalendar) Events(context.Context, time.Time, time.Time) ([]schedules.CalendarEvent, bool) {
	return f.events, !f.down
}

var zone = time.FixedZone("EAT", 3*60*60)

func at(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, zone) }

// eventSchedule saves an enabled Event schedule and returns its id and the runs it has had.
func eventSchedule(t *testing.T, svc *schedules.Service, name, when string) string {
	t.Helper()
	created, err := svc.Save(context.Background(), protocol.SaveScheduleRequest{
		Project: "p1", Name: name, Kind: "brief", Icon: "sunrise", Trigger: "Event", When: when,
		Action: "send", Enabled: true, Missed: "Skip",
	})
	if err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func runsOf(t *testing.T, svc *schedules.Service, st interface {
	Queries() *db.Queries
}, id string) []db.ScheduleRun {
	t.Helper()
	runs, err := st.Queries().ListScheduleRuns(context.Background(), db.ListScheduleRunsParams{ScheduleID: id, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	return runs
}

func TestASmartBriefRunsBeforeTheFirstEventOfTheDayAndOnlyOnce(t *testing.T) {
	st := newTestStore(t)
	clock := at(2, 8, 0)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	calendar := &fakeCalendar{events: []schedules.CalendarEvent{
		{ID: "all", Title: "Holiday", AllDay: true, Start: at(2, 0, 0)},
		{ID: "b", Title: "Design review", Start: at(2, 11, 0)},
		{ID: "a", Title: "Standup", Start: at(2, 9, 30)},
		{ID: "c", Title: "Standup", Start: at(3, 9, 30)},
	}}
	svc.SetCalendar(calendar)
	var heard []string
	svc.RegisterHandler("brief", func(_ context.Context, s protocol.Schedule, _ time.Time) (string, error) {
		heard = append(heard, s.Name)
		return "brief", nil
	})
	id := eventSchedule(t, svc, "Smart brief", "When 30 minutes before my first calendar event")

	svc.CheckCalendar(context.Background()) // 08:00: the 09:00 moment has not come
	if len(runsOf(t, svc, st, id)) != 0 {
		t.Fatal("ran before it was due")
	}
	clock = at(2, 9, 5) // 09:00 was the moment: first event is Standup at 09:30
	svc.CheckCalendar(context.Background())
	runs := runsOf(t, svc, st, id)
	if len(runs) != 1 || runs[0].Status != "success" || len(heard) != 1 {
		t.Fatalf("runs = %+v, heard = %v, want one run", runs, heard)
	}
	if want := `Started by the event "Standup" at 09:30.` + "\nbrief"; runs[0].Details != want {
		t.Errorf("details = %q, want %q", runs[0].Details, want)
	}
	clock = at(2, 9, 40)
	svc.CheckCalendar(context.Background()) // already ran for today's first event
	if len(runsOf(t, svc, st, id)) != 1 {
		t.Error("ran twice for the same event")
	}
}

func TestABriefTakesItsTimeFromAnEventWithItsName(t *testing.T) {
	st := newTestStore(t)
	clock := at(2, 7, 59)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	svc.SetCalendar(&fakeCalendar{events: []schedules.CalendarEvent{
		{ID: "x", Title: "Lunch", Start: at(2, 8, 0)},
		{ID: "m", Title: "morning BRIEF", Start: at(2, 8, 0)},
	}})
	svc.RegisterHandler("brief", func(context.Context, protocol.Schedule, time.Time) (string, error) { return "ok", nil })
	id := eventSchedule(t, svc, "Morning brief", `When a calendar event named "Morning brief" starts`)
	svc.CheckCalendar(context.Background())
	if len(runsOf(t, svc, st, id)) != 0 {
		t.Fatal("ran a minute early")
	}
	clock = at(2, 8, 1)
	svc.CheckCalendar(context.Background())
	runs := runsOf(t, svc, st, id)
	if len(runs) != 1 || runs[0].Details[:len(`Started by the event "morning BRIEF"`)] != `Started by the event "morning BRIEF"` {
		t.Fatalf("runs = %+v, want one run started by the matching event, not Lunch", runs)
	}
}

func TestACalendarScheduleDoesNotRunLateOrWhenGoogleCannotBeRead(t *testing.T) {
	st := newTestStore(t)
	clock := at(2, 10, 0)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	calendar := &fakeCalendar{events: []schedules.CalendarEvent{{ID: "a", Title: "Standup", Start: at(2, 9, 0)}}}
	svc.SetCalendar(calendar)
	id := eventSchedule(t, svc, "Heads up", "When a calendar event starts")
	svc.CheckCalendar(context.Background()) // the event began an hour ago: too late to be a heads up
	if len(runsOf(t, svc, st, id)) != 0 {
		t.Error("ran an hour after the event began")
	}
	clock = at(2, 9, 2)
	calendar.down = true
	svc.CheckCalendar(context.Background())
	if len(runsOf(t, svc, st, id)) != 0 {
		t.Error("ran on a guess while Google could not be read")
	}
	calendar.down = false
	svc.CheckCalendar(context.Background())
	if len(runsOf(t, svc, st, id)) != 1 {
		t.Error("did not run once Google could be read again")
	}
}

func TestAnEventScheduleWithWordsItCannotReadWaitsForNothing(t *testing.T) {
	st := newTestStore(t)
	clock := at(2, 9, 0)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	svc.SetCalendar(&fakeCalendar{events: []schedules.CalendarEvent{{ID: "a", Title: "Standup", Start: at(2, 9, 0)}}})
	id := eventSchedule(t, svc, "Odd", "When the moon is full")
	svc.CheckCalendar(context.Background())
	if len(runsOf(t, svc, st, id)) != 0 {
		t.Error("an unreadable sentence must never fire")
	}
}
