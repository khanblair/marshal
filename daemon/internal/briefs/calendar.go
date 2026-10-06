package briefs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/googlecal"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// eveningFromHour is the hour a brief with no telling name is read as an evening one from.
const eveningFromHour = 16

// IsEvening says a brief looks ahead to tomorrow rather than at today. Its name says so ("Evening
// brief"), or, for a brief named something else, its time does.
func IsEvening(sched protocol.Schedule) bool {
	if strings.Contains(strings.ToLower(sched.Name), "evening") {
		return true
	}
	if strings.Contains(strings.ToLower(sched.Name), "morning") {
		return false
	}
	var hour int
	if _, err := fmt.Sscanf(sched.Time, "%d:", &hour); err == nil {
		return hour >= eveningFromHour
	}
	return false
}

// calendarSection is the calendar text of a brief made before sections existed.
func (s *Service) calendarSection(ctx context.Context, sched protocol.Schedule) string {
	return s.calendarPart(ctx, sched, s.now()).text
}

// calendarWindow is the days a brief's calendar covers: today for a morning brief, tomorrow for an
// evening one (docs/marshal-product-scope.md 18.1), and the coming seven days for a weekly review.
func calendarWindow(sched protocol.Schedule, now time.Time) (start, end time.Time, title string) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case sched.Template == TemplateWeekly:
		start = day.AddDate(0, 0, 1)
		return start, start.AddDate(0, 0, weekAhead), "The coming week's calendar"
	case sched.Template == TemplateWindDown || IsEvening(sched):
		start = day.AddDate(0, 0, 1)
		return start, start.AddDate(0, 0, 1), "Tomorrow's calendar"
	}
	return day, day.AddDate(0, 0, 1), "Today's calendar"
}

// calendarPart is the brief's calendar part. It is empty when no calendar is wired in or Google
// Calendar is not connected, so a brief never mentions a calendar the person does not have, and says
// so in one line when Google could not be read.
func (s *Service) calendarPart(ctx context.Context, sched protocol.Schedule, now time.Time) part {
	if s.events == nil {
		return part{}
	}
	start, end, title := calendarWindow(sched, now)
	events, reading := s.events.GoogleEvents(ctx, start, end)
	if !reading.Connected && reading.Error == "" {
		return part{}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n", title)
	if reading.Error != "" && len(events) == 0 {
		fmt.Fprintf(&b, "%s\n", reading.Error)
		return part{text: strings.TrimRight(b.String(), "\n")}
	}
	if len(events) == 0 {
		b.WriteString("Nothing on the calendar.\n")
		return part{text: strings.TrimRight(b.String(), "\n")}
	}
	manyDays := end.Sub(start) > 36*time.Hour
	for _, event := range events {
		line := eventLine(event, now.Location())
		if manyDays {
			line = event.StartAt.In(now.Location()).Format("Mon") + " " + line
		}
		fmt.Fprintf(&b, "- %s\n", line)
	}
	if reading.Stale {
		fmt.Fprintf(&b, "%s\n", reading.Error)
	}
	return part{text: strings.TrimRight(b.String(), "\n"), items: len(events)}
}

// eventLine is one event as a brief writes it: when, what, and where.
func eventLine(event googlecal.Event, loc *time.Location) string {
	when := "All day"
	if !event.AllDay {
		when = event.StartAt.In(loc).Format("15:04")
		if !event.EndAt.IsZero() {
			when += "-" + event.EndAt.In(loc).Format("15:04")
		}
	}
	line := fmt.Sprintf("%s %s", when, event.Title)
	if event.Location != "" {
		line += " (" + event.Location + ")"
	}
	return line
}
