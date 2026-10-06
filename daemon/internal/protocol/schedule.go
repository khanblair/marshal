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
	// Template names the starter this schedule began as, such as "morning", or is empty for one made
	// from nothing. It never changes after the schedule is made.
	Template string `json:"template"`
	// Sections are the parts a brief is built from, by the ids GET /v1/schedules/catalog lists. Never
	// null. A schedule with none is a brief made the way briefs were before sections existed.
	Sections []string `json:"sections"`
	// Deliver are the chat channels the brief is sent to, besides being kept in the run history. Never
	// null. Channels that are not connected when the brief is sent are skipped.
	Deliver []string `json:"deliver"`
	// QuietWhenEmpty keeps a brief with nothing to report from being sent. The run is still recorded.
	QuietWhenEmpty bool `json:"quietWhenEmpty"`
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
	// Template is the starter a new schedule began as. It is read when a schedule is created and
	// ignored when one is edited.
	Template string `json:"template"`
	// Sections are the parts of a brief, by id. Each must be one the catalog lists.
	Sections []string `json:"sections"`
	// Deliver are the chat channels to send the brief to. Each must be one the catalog lists.
	Deliver []string `json:"deliver"`
	// QuietWhenEmpty keeps a brief with nothing to report from being sent.
	QuietWhenEmpty bool `json:"quietWhenEmpty"`
}

// SchedulePreview is the answer to GET /v1/schedules/{id}/preview: the message a chat would get if the
// brief were sent now. Nothing is sent and no run is recorded.
type SchedulePreview struct {
	// Text is the title, a blank line, and the body as a chat shows it.
	Text       string    `json:"text"`
	ServerTime Timestamp `json:"serverTime"`
}

// ScheduleSection is one part a brief can be built from.
type ScheduleSection struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Hint is one line on what the section lists, for the editor.
	Hint string `json:"hint"`
}

// ScheduleChannel is one chat a brief can be sent to.
type ScheduleChannel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ScheduleTemplate is a starter schedule: every field a new schedule needs, filled in. The daemon
// seeds one schedule per template, switched off, the first time it runs, and New schedule offers them
// again so one that was deleted can be added back.
type ScheduleTemplate struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Icon    string `json:"icon"`
	Trigger string `json:"trigger"`
	When    string `json:"when"`
	Time    string `json:"time"`
	Days    []int  `json:"days"`
	Missed  string `json:"missed"`
	// Sections are the parts the template is built from.
	Sections []string `json:"sections"`
	// QuietWhenEmpty is true for a template that should say nothing when there is nothing to say.
	QuietWhenEmpty bool `json:"quietWhenEmpty"`
}

// ScheduleCatalog is the answer to GET /v1/schedules/catalog: what the editor offers.
type ScheduleCatalog struct {
	Templates  []ScheduleTemplate `json:"templates"`
	Sections   []ScheduleSection  `json:"sections"`
	Channels   []ScheduleChannel  `json:"channels"`
	ServerTime Timestamp          `json:"serverTime"`
}

// CalendarEvent is one Google Calendar item (coming up / calendar view). It is only ever populated
// once Google Calendar is connected (B8.3); GoogleConnected on the list that carries it says which.
// Start is the one field the daemon fills in; a cutover's mapper derives Time and DayOffset from
// it (the mock's own relative shape), since only the viewer's own clock knows what day is "today".
type CalendarEvent struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Start Timestamp `json:"start"`
	// End is when the event ends. For an all-day event it is the first day it no longer covers.
	// Null when Google gave none.
	End *Timestamp `json:"end" tstype:"Timestamp | null"`
	// AllDay says the event covers whole days, so its time of day means nothing.
	AllDay bool `json:"allDay"`
	// Location is the place, as typed. Empty when there is none.
	Location string `json:"location"`
	// URL opens the event in Google Calendar. Empty when there is none.
	URL string `json:"url"`
	// JoinURL is the event's video call link. Empty when there is none.
	JoinURL string `json:"joinUrl"`
	// Calendar is the name of the calendar the event is on.
	Calendar  string `json:"calendar"`
	Time      string `json:"time,omitempty"`
	Days      []int  `json:"days,omitempty"`
	DayOffset int    `json:"dayOffset,omitempty"`
}

// CalendarList is the answer to GET /v1/calendar: the one call N21 asks for so the calendar view
// and Home's coming-up list read everything on the calendar together, rather than three separate
// addresses. Schedules is every schedule, enabled or not - the same list GET /v1/schedules answers
// - not only the ones inside the asked-for range, because a schedule's own days and time, not a
// range, say which days it draws on; the screen already knows to skip a disabled one. DueCards is
// scoped to the range. GoogleConnected is false with an always-empty Events until Google Calendar is
// connected: never sample data. When Google could not be read, GoogleError says why in a sentence a
// person can act on, and the rest of the list is still there: a Google that is down never takes the
// schedules and the due cards with it.
type CalendarList struct {
	Schedules       []Schedule      `json:"schedules"`
	DueCards        []Card          `json:"dueCards"`
	Events          []CalendarEvent `json:"events"`
	GoogleConnected bool            `json:"googleConnected"`
	// GoogleError is why Google's events are missing or old, or empty when nothing is wrong.
	GoogleError string `json:"googleError"`
	// GoogleStale says Events are the last ones read, because Google could not be reached just now.
	GoogleStale bool      `json:"googleStale"`
	ServerTime  Timestamp `json:"serverTime"`
}

// GoogleReading is how reading Google Calendar went, for the answer that carries its events.
type GoogleReading struct {
	// Connected says Google Calendar is set up and its access works.
	Connected bool
	// Error is why Google's events are missing, in words a person can act on. Empty when none.
	Error string
	// Stale says the events are the last ones read, because Google could not be reached just now.
	Stale bool
}

// NewCalendarList makes an answer stamped with the daemon's time.
func NewCalendarList(schedules []Schedule, dueCards []Card, events []CalendarEvent, google GoogleReading, now time.Time) CalendarList {
	// The lists are never null on the wire: a daemon with no Google Calendar connected has no events.
	if schedules == nil {
		schedules = []Schedule{}
	}
	if dueCards == nil {
		dueCards = []Card{}
	}
	if events == nil {
		events = []CalendarEvent{}
	}
	return CalendarList{
		Schedules: schedules, DueCards: dueCards, Events: events,
		GoogleConnected: google.Connected, GoogleError: google.Error, GoogleStale: google.Stale,
		ServerTime: NewTimestamp(now),
	}
}
