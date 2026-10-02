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

// calendarSection is the brief's calendar part: today's events in a morning brief, tomorrow's in an
// evening one (docs/marshal-product-scope.md 18.1). It is empty when no calendar is wired in or
// Google Calendar is not connected, so a brief never mentions a calendar the person does not have,
// and says so in one line when Google could not be read.
func (s *Service) calendarSection(ctx context.Context, sched protocol.Schedule) string {
	if s.events == nil {
		return ""
	}
	now := s.now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	title := "Today's calendar"
	if IsEvening(sched) {
		day = day.AddDate(0, 0, 1)
		title = "Tomorrow's calendar"
	}
	events, reading := s.events.GoogleEvents(ctx, day, day.AddDate(0, 0, 1))
	if !reading.Connected && reading.Error == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n", title)
	if reading.Error != "" && len(events) == 0 {
		fmt.Fprintf(&b, "%s\n", reading.Error)
		return strings.TrimRight(b.String(), "\n")
	}
	if len(events) == 0 {
		b.WriteString("Nothing on the calendar.\n")
		return strings.TrimRight(b.String(), "\n")
	}
	for _, event := range events {
		fmt.Fprintf(&b, "- %s\n", eventLine(event, now.Location()))
	}
	if reading.Stale {
		fmt.Fprintf(&b, "%s\n", reading.Error)
	}
	return strings.TrimRight(b.String(), "\n")
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
