package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// calendarRange is GET /v1/calendar?start=&end=: the one call N21 asks for, so the calendar view
// and Home's coming-up list read the schedules, the due cards, and Google Calendar's events
// together instead of three separate addresses (docs/backend-checklist.md B8.4, build-plan task
// 8.9). start and end are epoch milliseconds and both are required, so the daemon never guesses a
// range on a caller's behalf.
func (s *Server) calendarRange(w http.ResponseWriter, r *http.Request) {
	start, end, err := calendarRangeOf(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	list, err := s.schedules.List(r.Context(), "")
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	due, err := s.dueCards(r.Context(), start, end)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	events, google := s.calendarEvents(r.Context(), start, end)
	s.writeJSON(w, http.StatusOK, protocol.NewCalendarList(list, due, events, google, s.now()))
}

// calendarEvents reads Google Calendar's events for the range. Nothing connected answers an empty
// list and connected=false - the honest "Not connected" state, never sample data - and a Google that
// cannot be read answers the last events read, or none, with the reason, so the schedules and due
// cards beside them are never lost to it.
func (s *Server) calendarEvents(ctx context.Context, start, end int64) ([]protocol.CalendarEvent, protocol.GoogleReading) {
	if s.integrations == nil {
		return nil, protocol.GoogleReading{}
	}
	found, reading := s.integrations.GoogleEvents(ctx, time.UnixMilli(start), time.UnixMilli(end))
	events := make([]protocol.CalendarEvent, len(found))
	for i, event := range found {
		wire := protocol.CalendarEvent{
			ID: event.ID, Title: event.Title, Start: protocol.NewTimestamp(event.StartAt),
			AllDay: event.AllDay, StartDate: event.StartDate, EndDate: event.EndDate, Location: event.Location, URL: event.URL, JoinURL: event.JoinURL,
			Calendar: event.CalendarName,
		}
		if !event.EndAt.IsZero() {
			end := protocol.NewTimestamp(event.EndAt)
			wire.End = &end
		}
		events[i] = wire
	}
	return events, reading
}

// calendarRangeOf reads the required start and end query parameters, both epoch milliseconds.
func calendarRangeOf(r *http.Request) (start, end int64, err error) {
	start, startErr := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
	if startErr != nil {
		return 0, 0, protocol.InvalidArgument("start must be an epoch-millisecond timestamp.")
	}
	end, endErr := strconv.ParseInt(r.URL.Query().Get("end"), 10, 64)
	if endErr != nil {
		return 0, 0, protocol.InvalidArgument("end must be an epoch-millisecond timestamp.")
	}
	if end < start {
		return 0, 0, protocol.InvalidArgument("end must not be before start.")
	}
	return start, end, nil
}

// dueCards finds every card, across every project, due inside [start, end] (both epoch
// milliseconds, inclusive). It reads each project's own cards through the same projects service
// the card routes use, rather than a query of its own, so a due card here is read exactly the way
// the board itself reads it.
func (s *Server) dueCards(ctx context.Context, start, end int64) ([]protocol.Card, error) {
	projectList, err := s.projects.List(ctx)
	if err != nil {
		return nil, err
	}
	due := make([]protocol.Card, 0, len(projectList.Projects))
	for _, project := range projectList.Projects {
		cards, err := s.projects.Cards(ctx, project.ID)
		if err != nil {
			return nil, err
		}
		for _, card := range cards {
			if card.Due == nil {
				continue
			}
			at := card.Due.Time().UnixMilli()
			if at >= start && at <= end {
				due = append(due, card)
			}
		}
	}
	return due, nil
}
