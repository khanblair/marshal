package protocol

import "time"

// Schedule is one scheduled item as the screens see it (apps/web/src/mock/settings-types.ts).
type Schedule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"` // "brief" | "job"
	Icon    string `json:"icon"`
	Trigger string `json:"trigger"` // "Cron" | "Interval" | "One-time" | "Event"
	When    string `json:"when"`
	Time    string `json:"time"`
	Days    []int  `json:"days"`
	Action  string `json:"action"`
	Project string `json:"project"`
	Enabled bool   `json:"enabled"`
	Missed  string `json:"missed"`
}

// ScheduleList is the response to GET /v1/schedules.
type ScheduleList struct {
	Schedules  []Schedule `json:"schedules"`
	ServerTime Timestamp  `json:"serverTime"`
}

// NewScheduleList makes an answer stamped with the daemon's time.
func NewScheduleList(schedules []Schedule, now time.Time) ScheduleList {
	out := make([]Schedule, len(schedules))
	copy(out, schedules)
	return ScheduleList{Schedules: out, ServerTime: NewTimestamp(now)}
}

// ScheduleRun is one firing of a schedule: when, whether the handler succeeded, and what it left
// behind - a brief's own composed text, for a brief (B8.5).
type ScheduleRun struct {
	ID      string    `json:"id"`
	RunAt   Timestamp `json:"runAt"`
	Status  string    `json:"status"`
	Details string    `json:"details,omitempty"`
}

// ScheduleRunList is the answer to GET /v1/schedules/{id}/runs, newest first.
type ScheduleRunList struct {
	Runs       []ScheduleRun `json:"runs"`
	ServerTime Timestamp     `json:"serverTime"`
}

// NewScheduleRunList makes an answer stamped with the daemon's time.
func NewScheduleRunList(runs []ScheduleRun, now time.Time) ScheduleRunList {
	out := make([]ScheduleRun, len(runs))
	copy(out, runs)
	return ScheduleRunList{Runs: out, ServerTime: NewTimestamp(now)}
}

// SaveScheduleRequest is the body for POST/PUT /v1/schedules.
type SaveScheduleRequest struct {
	ID      string `json:"id,omitempty"`
	Project string `json:"project"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Icon    string `json:"icon"`
	Trigger string `json:"trigger"`
	When    string `json:"when"`
	Time    string `json:"time"`
	Days    []int  `json:"days"`
	Action  string `json:"action"`
	Enabled bool   `json:"enabled"`
	Missed  string `json:"missed"`
}

// CalendarEvent is one Google Calendar item (coming up / calendar view). It is only ever populated
// once Google Calendar is connected (B8.3); GoogleConnected on the list that carries it says which.
// Start is the one field the daemon fills in; a cutover's mapper derives Time and DayOffset from
// it (the mock's own relative shape), since only the viewer's own clock knows what day is "today".
type CalendarEvent struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Start     Timestamp `json:"start"`
	Time      string    `json:"time,omitempty"`
	Days      []int     `json:"days,omitempty"`
	DayOffset int       `json:"dayOffset,omitempty"`
}

// CalendarList is the answer to GET /v1/calendar: the one call N21 asks for so the calendar view
// and Home's coming-up list read everything on the calendar together, rather than three separate
// addresses. Schedules is every schedule, enabled or not - the same list GET /v1/schedules answers
// - not only the ones inside the asked-for range, because a schedule's own days and time, not a
// range, say which days it draws on; the screen already knows to skip a disabled one. DueCards is
// scoped to the range. GoogleConnected is false with an always-empty Events until B8.3 connects
// Google Calendar for real: never sample data.
type CalendarList struct {
	Schedules       []Schedule      `json:"schedules"`
	DueCards        []Card          `json:"dueCards"`
	Events          []CalendarEvent `json:"events"`
	GoogleConnected bool            `json:"googleConnected"`
	ServerTime      Timestamp       `json:"serverTime"`
}

// NewCalendarList makes an answer stamped with the daemon's time.
func NewCalendarList(schedules []Schedule, dueCards []Card, events []CalendarEvent, googleConnected bool, now time.Time) CalendarList {
	return CalendarList{
		Schedules: schedules, DueCards: dueCards, Events: events,
		GoogleConnected: googleConnected, ServerTime: NewTimestamp(now),
	}
}
