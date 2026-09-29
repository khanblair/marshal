package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations"
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
	events, connected, err := s.calendarEvents(r.Context(), start, end)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, protocol.NewCalendarList(list, due, events, connected, s.now()))
}

// calendarEvents reads Google Calendar's events for the range, or answers connected=false with no
// events when nothing is connected yet - the honest "Not connected" state, never sample data.
func (s *Server) calendarEvents(ctx context.Context, start, end int64) ([]protocol.CalendarEvent, bool, error) {
	if s.integrations == nil {
		return nil, false, nil
	}
	client, err := s.integrations.GoogleCalendarClient(ctx)
	if errors.Is(err, integrations.ErrNotConnected) || errors.Is(err, integrations.ErrNoGoogleClient) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	found, err := client.Events(ctx, time.UnixMilli(start), time.UnixMilli(end))
	if err != nil {
		return nil, false, err
	}
	events := make([]protocol.CalendarEvent, len(found))
	for i, event := range found {
		events[i] = protocol.CalendarEvent{
			ID: event.ID, Title: event.Title, Start: protocol.NewTimestamp(event.StartAt),
		}
	}
	return events, true, nil
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
