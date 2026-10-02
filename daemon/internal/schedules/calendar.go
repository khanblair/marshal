package schedules

// This file is how a schedule waits on Google Calendar instead of a clock (docs/marshal-product-
// scope.md 17 and 18.2): an Event schedule whose "when" names a calendar event. Three things it
// covers with one sentence each:
//
//	"When 30 minutes before my first calendar event"        smart brief time
//	"When a calendar event named \"Morning brief\" starts"    a brief takes its time from an event
//	"When 10 minutes before an event called \"Design review\""   any job before a matching event
//
// A watcher reads the calendar once a minute, through a cache that makes that one Google call a
// minute at most, and only when some schedule asks for it.

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// watchEvery is how often the watcher looks at the calendar.
	watchEvery = time.Minute
	// fireWindow is how late a firing may still run. A daemon that was asleep through the moment does
	// not run a "before my meeting" job after the meeting has begun.
	fireWindow = 15 * time.Minute
	// lookAhead is how far ahead of now the watcher reads events: today and tomorrow, so the first
	// event of a day is found for the day it belongs to.
	lookAhead = 48 * time.Hour
	// maxBefore is the longest "before" a schedule may ask for. Longer than a day is a typo.
	maxBefore = 24 * time.Hour
)

// CalendarEvent is what the scheduler needs to know of one event.
type CalendarEvent struct {
	ID     string
	Title  string
	Start  time.Time
	AllDay bool
}

// CalendarSource reads the calendar. ok is false when Google could not be read just now, which
// leaves every schedule waiting rather than firing on a guess.
type CalendarSource interface {
	Events(ctx context.Context, start, end time.Time) (events []CalendarEvent, ok bool)
}

// CalendarTrigger is what an Event schedule's "when" asks for.
type CalendarTrigger struct {
	// Before is how long before the event starts the schedule runs. Zero is when it starts.
	Before time.Duration
	// FirstOfDay limits it to the first timed event of each day, which is "my first event".
	FirstOfDay bool
	// Title limits it to events whose title contains these words, ignoring case. Empty is any.
	Title string
}

var calendarWhen = regexp.MustCompile(
	`(?i)^when\s+(?:(\d+)\s*(minutes?|mins?|hours?|hrs?)\s+before\s+)?` +
		`(?:my\s+|an?\s+|any\s+|every\s+)?(first\s+)?(?:calendar\s+)?event` +
		`(?:\s+(?:called|named|titled)\s+["“”']?(.+?)["“”']?)?` +
		`(?:\s+(?:of\s+the\s+day|starts?|begins?))?\s*$`)

// ParseCalendarTrigger reads an Event schedule's "when". It reports false for any other sentence,
// and then the schedule waits for no calendar event: its words are a person's own and are kept.
func ParseCalendarTrigger(when string) (CalendarTrigger, bool) {
	m := calendarWhen.FindStringSubmatch(strings.TrimSpace(when))
	if m == nil {
		return CalendarTrigger{}, false
	}
	trigger := CalendarTrigger{FirstOfDay: m[3] != "", Title: strings.TrimSpace(m[4])}
	if m[1] != "" {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return CalendarTrigger{}, false
		}
		unit := time.Minute
		if strings.HasPrefix(strings.ToLower(m[2]), "h") {
			unit = time.Hour
		}
		trigger.Before = time.Duration(n) * unit
	}
	if trigger.Before > maxBefore {
		return CalendarTrigger{}, false
	}
	// "my first event named X" asks for two different things at once, so the title wins and "first"
	// reads as the first of the matching ones.
	return trigger, true
}

// SetCalendar gives the scheduler Google Calendar, so Event schedules can wait on it. The daemon
// calls it once, while it starts, before Start.
func (s *Service) SetCalendar(source CalendarSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calendar = source
}

// watchCalendar checks the calendar every minute until ctx ends.
func (s *Service) watchCalendar(ctx context.Context) {
	ticker := time.NewTicker(watchEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.CheckCalendar(ctx)
		}
	}
}

// due is one firing an Event schedule is owed.
type due struct {
	row   db.Schedule
	event CalendarEvent
	at    time.Time
}

// CheckCalendar runs every Event schedule that a calendar event has just made due. It is what the
// watcher calls each minute, exported so a test can drive the clock without waiting.
func (s *Service) CheckCalendar(ctx context.Context) {
	s.mu.Lock()
	source := s.calendar
	s.mu.Unlock()
	if source == nil {
		return
	}
	rows, err := s.store.Queries().ListSchedules(ctx)
	if err != nil {
		s.logger.Error("read schedules for the calendar check", "error", err)
		return
	}
	wanted := map[string]CalendarTrigger{}
	for _, row := range rows {
		if row.Enabled != 1 || row.TriggerType != triggerEvent {
			continue
		}
		if trigger, ok := ParseCalendarTrigger(row.WhenText); ok {
			wanted[row.ID] = trigger
		}
	}
	if len(wanted) == 0 {
		return
	}
	now := s.now()
	events, ok := source.Events(ctx, startOfDay(now), startOfDay(now).Add(lookAhead))
	if !ok {
		return
	}
	for _, firing := range s.owed(rows, wanted, events, now) {
		s.logger.Info("a calendar event made a schedule due", "id", firing.row.ID, "event", firing.event.Title)
		cause := fmt.Sprintf("Started by the event %q at %s.", firing.event.Title, firing.event.Start.In(now.Location()).Format("15:04"))
		s.runBecause(ctx, toProtocolSchedule(firing.row), cause)
	}
}

// owed lists the firings that are due now, at most one per schedule. A schedule that already ran at
// or after the moment a firing was due has had it, which is what keeps a restart from running it
// twice.
func (s *Service) owed(rows []db.Schedule, wanted map[string]CalendarTrigger, events []CalendarEvent, now time.Time) []due {
	timed := slices.DeleteFunc(slices.Clone(events), func(e CalendarEvent) bool { return e.AllDay })
	slices.SortStableFunc(timed, func(a, b CalendarEvent) int { return a.Start.Compare(b.Start) })
	var out []due
	for _, row := range rows {
		trigger, ok := wanted[row.ID]
		if !ok {
			continue
		}
		lastRun := time.UnixMilli(row.LastRunAt)
		for _, event := range candidates(trigger, timed, now.Location()) {
			at := event.Start.Add(-trigger.Before)
			if at.After(now) || now.Sub(at) >= fireWindow || !lastRun.Before(at) {
				continue
			}
			out = append(out, due{row: row, event: event, at: at})
			break
		}
	}
	return out
}

// candidates are the events a trigger could fire for, soonest first.
func candidates(trigger CalendarTrigger, timed []CalendarEvent, loc *time.Location) []CalendarEvent {
	var matching []CalendarEvent
	for _, event := range timed {
		if trigger.Title == "" || strings.Contains(strings.ToLower(event.Title), strings.ToLower(trigger.Title)) {
			matching = append(matching, event)
		}
	}
	if !trigger.FirstOfDay {
		return matching
	}
	var firsts []CalendarEvent
	seen := map[string]bool{}
	for _, event := range matching {
		day := event.Start.In(loc).Format("2006-01-02")
		if !seen[day] {
			seen[day] = true
			firsts = append(firsts, event)
		}
	}
	return firsts
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
