// Package schedules owns the scheduled jobs and briefs and the cron that runs them
// (docs/backend-checklist.md B8.1, build-plan 8.1, docs/architecture.md section 10). A schedule is
// one row: a brief or a job, when it runs, and what it does. The rows are the store's; this package
// is what turns a row's `when` into a real cron spec (ParseCron), keeps the running entries in step
// with the rows, and records every run so a missed one is visible rather than silent.
package schedules

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/khanblair/marshal/daemon/internal/briefs"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/runctx"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Previewer writes the message a brief would send now, without sending it or recording a run.
type Previewer func(ctx context.Context, s protocol.Schedule, since time.Time) (string, error)

// SetPreviewer says how a brief is previewed. The daemon gives it the brief composer.
func (s *Service) SetPreviewer(p Previewer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preview = p
}

// Preview answers what a chat would get from the schedule now, reading from where its last scheduled
// run stopped, as its next run will. Only a brief can be previewed.
func (s *Service) Preview(ctx context.Context, id string) (protocol.SchedulePreview, error) {
	sched, err := s.Get(ctx, id)
	if err != nil {
		return protocol.SchedulePreview{}, err
	}
	s.mu.Lock()
	preview := s.preview
	s.mu.Unlock()
	if sched.Kind != "brief" || preview == nil {
		return protocol.SchedulePreview{}, protocol.Refused("Only a brief can be previewed. This kind of schedule cannot run yet.")
	}
	text, err := preview(ctx, sched, s.sinceOf(ctx, id))
	if err != nil {
		return protocol.SchedulePreview{}, err
	}
	return protocol.SchedulePreview{Text: text, ServerTime: protocol.NewTimestamp(s.now())}, nil
}

// StatusUnsupported is the status of a run that had no handler: it did nothing, and says so.
const StatusUnsupported = "unsupported"

// ErrNotFound is what reading or updating a schedule Marshal has no row for answers. It is the
// ordinary state of an id that was deleted, and the route turns it into the same not-found answer a
// card gets, so a screen is never told to look for something that is not there.
var ErrNotFound = errors.New("schedule not found")

// triggerOneTime and triggerEvent are two of the four Trigger values the Schedules screen offers
// (routes_schedules.go's scheduleTriggers), the two this package treats specially.
const (
	triggerOneTime = "One-time"
	triggerEvent   = "Event"
)

// missedRunOnWake is the missed_policy value that means a schedule the daemon's own downtime cost a
// firing should run once, on the next start, rather than wait for its next natural tick. It is the
// mock's own wire string (apps/web/src/mock/seed/calendar.ts), not an enum: the field is a person's
// choice on the Schedules screen, and any other value - including "Skip" and an unrecognised one -
// means the same thing here, do nothing and let the schedule resume on its own next tick.
const missedRunOnWake = "Run once on wake"

// ActionHandler is a callback invoked when a schedule fires. It is looked up by the schedule's own
// Action first (a job's own action, once actions are structured - see the package doc), and by its
// Kind when nothing more specific is registered (every brief, since a brief's Action is a person's
// free-text sentence describing what happens, not a dispatchable key). since is the boundary a
// handler gathers "what changed" from: the schedule's last scheduled run, or the zero time if it
// has never run (the handler then reads back as far as it chooses) - read fresh at every fire, not from a closure captured when the cron entry was made, so a
// schedule's second firing sees its first firing's own time. The returned string becomes the
// run's own recorded details, on success as much as on failure, so what a handler did - a brief's
// own text, say - is readable back through the run history even before any delivery channel exists.
type ActionHandler func(ctx context.Context, s protocol.Schedule, since time.Time) (string, error)

// Service manages schedules and runs them using cron.
type Service struct {
	mu       sync.Mutex
	store    *store.Store
	logger   *slog.Logger
	cron     *cron.Cron
	now      func() time.Time
	entropy  io.Reader
	handlers map[string]ActionHandler
	entries  map[string]cron.EntryID
	// calendar is where an Event schedule waits for its event. Nil means none does.
	calendar CalendarSource
	// preview writes what a brief would say now, without sending it. Nil means nothing can.
	preview Previewer
	// stopWatch ends the calendar watcher. Nil when it is not running.
	stopWatch context.CancelFunc
}

// Option tunes a Service. Every field may be left out.
type Option func(*Service)

// WithClock sets the clock a new schedule's id and timestamps are made from. Nil is ignored.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithEntropy sets where a new schedule's id gets its random part. The default is crypto/rand; a
// test hands in its own reader so an id is the same every run.
func WithEntropy(r io.Reader) Option {
	return func(s *Service) {
		if r != nil {
			s.entropy = r
		}
	}
}

// NewService creates a new schedules service.
func NewService(st *store.Store, logger *slog.Logger, opts ...Option) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{
		store:    st,
		logger:   logger,
		now:      time.Now,
		entropy:  rand.Reader,
		handlers: make(map[string]ActionHandler),
		entries:  make(map[string]cron.EntryID),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	// The cron reads its clock times in the zone of the service's own clock, so "8:00" is the
	// person's 8:00.
	s.cron = cron.New(cron.WithLocation(s.now().Location()))
	return s
}

// RegisterHandler registers a handler for a schedule action (e.g. "morning_brief", "evening_brief").
func (s *Service) RegisterHandler(action string, handler ActionHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[action] = handler
}

// Start starts the cron scheduler and loads active schedules.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	s.cron.Start()
	err := s.reloadLocked(ctx)
	if s.calendar != nil && s.stopWatch == nil {
		// The watcher outlives the call that started it, so it takes its own context.
		watchCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
		s.stopWatch = stop
		go s.watchCalendar(watchCtx)
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	// Unlocked: checkMissedRuns reads the store and may call executeSchedule, which takes s.mu
	// itself for the handler lookup. Holding the lock across this call would deadlock against that.
	s.checkMissedRuns(ctx)
	return nil
}

// Rezone moves every cron schedule to the zone of the service's clock, which the daemon calls when the
// person chooses another zone: a schedule set for 8:00 stays set for 8:00, now in the new zone.
func (s *Service) Rezone(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Not waited for: a job that is running takes the lock to find its handler.
	s.cron.Stop()
	s.cron = cron.New(cron.WithLocation(s.now().Location()))
	s.cron.Start()
	return s.reloadLocked(ctx)
}

// Stop stops the cron scheduler.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cron.Stop()
	s.entries = make(map[string]cron.EntryID)
	if s.stopWatch != nil {
		s.stopWatch()
		s.stopWatch = nil
	}
}

func (s *Service) reloadLocked(ctx context.Context) error {
	rows, err := s.store.Queries().ListSchedules(ctx)
	if err != nil {
		return err
	}

	for _, entryID := range s.entries {
		s.cron.Remove(entryID)
	}
	s.entries = make(map[string]cron.EntryID)

	for _, row := range rows {
		if row.Enabled == 1 && row.CronExpr != "" && scheduledByCron(row.TriggerType) {
			sched := toProtocolSchedule(row)
			entryID, err := s.cron.AddFunc(row.CronExpr, func() {
				s.executeSchedule(context.Background(), sched)
			})
			if err == nil {
				s.entries[row.ID] = entryID
			}
		}
	}
	return nil
}

// cronExprFor is the 5-field spec Save stores for req, by its own Trigger: ParseCron's plain
// translation for Cron and Interval, ParseOneTime's one exact date for One-time, and none at all
// for Event - it waits on the event sources B8.2 and B8.3 add, not a clock, so it has no cron
// spec to be given one.
func cronExprFor(req protocol.SaveScheduleRequest, now time.Time) string {
	if req.Trigger == triggerOneTime {
		return ParseOneTime(req.When, req.Time, now)
	}
	if req.Trigger == triggerEvent {
		return ""
	}
	spec, _ := ParseCron(req.When, req.Time, req.Days)
	return spec
}

// scheduledByCron reports whether trigger is one cron itself runs: Cron, Interval, and One-time
// all have a real 5-field spec (cronExprFor). Only Event does not - it has no clock to wait on.
func scheduledByCron(trigger string) bool {
	return trigger == "Cron" || trigger == "Interval" || trigger == triggerOneTime
}

func (s *Service) executeSchedule(ctx context.Context, sched protocol.Schedule) {
	s.runBecause(ctx, sched, "", false)
}

// runBecause runs one schedule. cause, when it is not empty, is what started it, and heads the run's
// details so the history says why a calendar-driven run happened.
func (s *Service) runBecause(ctx context.Context, sched protocol.Schedule, cause string, manual bool) {
	s.logger.Info("executing schedule", "id", sched.ID, "name", sched.Name, "action", sched.Action)
	since := s.sinceOf(ctx, sched.ID)

	s.mu.Lock()
	handler := s.handlers[sched.Action]
	if handler == nil {
		handler = s.handlers[sched.Kind]
	}
	s.mu.Unlock()

	status := "success"
	details := ""
	if handler == nil {
		// Nothing here knows how to do this action. Saying "success" for a run that did nothing would
		// be a lie a person reads weeks later in the history.
		status = StatusUnsupported
		details = "This kind of schedule cannot run yet, so nothing was done. Briefs can."
	} else {
		result, err := handler(ctx, sched, since)
		details = result
		if err != nil {
			status = "failed"
			if details != "" {
				details += "\n\n"
			}
			details += err.Error()
			s.logger.Error("schedule execution failed", "id", sched.ID, "error", err)
		}
	}

	if cause != "" {
		details = cause + "\n" + details
	}
	s.recordRun(ctx, sched, status, details, manual)
}

// recordRun stores one run in the history. A run by hand does not move the last-run time, so the next
// scheduled brief still reads from where the last scheduled one stopped and a missed run is still
// caught up. A failure to store is logged, not swallowed: a run nobody can read back is a lie.
func (s *Service) recordRun(ctx context.Context, sched protocol.Schedule, status, details string, manual bool) {
	now := s.now().UnixMilli()
	// Two runs can land in one millisecond, so the id is its own, not made from the time.
	runID, err := protocol.NewID(s.now(), s.entropy)
	if err != nil {
		runID = fmt.Sprintf("run_%d_%s", now, sched.ID)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		if !manual {
			if err := q.UpdateScheduleLastRun(ctx, db.UpdateScheduleLastRunParams{
				ID:        sched.ID,
				LastRunAt: now,
				UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		_, err := q.CreateScheduleRun(ctx, db.CreateScheduleRunParams{
			ID:         runID,
			ScheduleID: sched.ID,
			RunAt:      now,
			Status:     status,
			Details:    details,
		})
		return err
	})
	if err != nil {
		s.logger.Error("could not record a schedule run", "id", sched.ID, "error", err)
	}
	if sched.Trigger == triggerOneTime && !manual {
		// A one-time job's spec carries no year, so left enabled it would fire again next year on
		// the same date - the opposite of "once". Disabling it here, right after its one real run,
		// is what makes cron.AddFunc's ordinary machinery serve a job that never repeats.
		_ = s.store.Write(ctx, func(q *db.Queries) error {
			return q.DisableSchedule(ctx, db.DisableScheduleParams{ID: sched.ID, UpdatedAt: now})
		})
	}
}

// RunNow runs a schedule once, now, and answers the run it recorded. It is how a person tries a brief,
// and the chats it goes to, without waiting for the clock. It reads from where the last scheduled run
// stopped, as the next one will, and it does not move that point. A schedule that is switched off can
// be run: switching it on is about when it runs by itself.
func (s *Service) RunNow(ctx context.Context, id string) (protocol.ScheduleRun, error) {
	sched, err := s.Get(ctx, id)
	if err != nil {
		return protocol.ScheduleRun{}, err
	}
	s.runBecause(ctx, sched, "Run by hand.", true)
	runs, err := s.Runs(ctx, id)
	if err != nil {
		return protocol.ScheduleRun{}, err
	}
	if len(runs) == 0 {
		return protocol.ScheduleRun{}, fmt.Errorf("the run of schedule %s was not recorded", id)
	}
	return runs[0], nil
}

// sinceOf is a schedule's last run, or the zero time when it has never run, read fresh from the
// store at fire time. It is what a handler gathers "what changed" since - the cron closure that
// calls executeSchedule was made once, at reload, and would answer the same stale moment on every
// tick if the value were captured there instead.
func (s *Service) sinceOf(ctx context.Context, id string) time.Time {
	row, err := s.store.Queries().GetSchedule(ctx, id)
	if err != nil {
		return s.now().Add(-24 * time.Hour)
	}
	if row.LastRunAt > 0 {
		return time.UnixMilli(row.LastRunAt)
	}
	// A schedule that has never run has no "since": a handler reads back as far as it chooses, and a
	// brief reads back as far as its template allows, not from the moment the schedule was made.
	return time.Time{}
}

// checkMissedRuns fires each schedule the daemon's own downtime cost a run (docs/marshal-product-
// scope.md 17.5): one whose own cron spec's next fire time, counted from its last run or - if it has
// never run - its creation, already passed. reloadLocked runs first so this only touches schedules
// the cron is already tracking, and it fires each one at most once: catching up means the schedule
// is not left silent, not replaying every tick the daemon slept through.
func (s *Service) checkMissedRuns(ctx context.Context) {
	rows, err := s.store.Queries().ListSchedules(ctx)
	if err != nil {
		s.logger.Error("read schedules for the missed-run check", "error", err)
		return
	}
	now := s.now()
	for _, row := range rows {
		if row.Enabled != 1 || row.CronExpr == "" || row.MissedPolicy != missedRunOnWake || !scheduledByCron(row.TriggerType) {
			continue
		}
		spec, err := cron.ParseStandard(row.CronExpr)
		if err != nil {
			continue
		}
		since := time.UnixMilli(row.CreatedAt)
		if row.LastRunAt > 0 {
			since = time.UnixMilli(row.LastRunAt)
		}
		// The spec is read in the clock's zone, as the cron reads it.
		due := spec.Next(since.In(now.Location()))
		if due.After(now) {
			continue
		}
		s.logger.Info("catching up a missed schedule", "id", row.ID, "name", row.Name)
		s.executeSchedule(runctx.WithLate(ctx, due), toProtocolSchedule(row))
	}
}

// List returns all schedules for a project (or all if projectID is empty).
func (s *Service) List(ctx context.Context, projectID string) ([]protocol.Schedule, error) {
	var rows []db.Schedule
	var err error
	if projectID == "" {
		rows, err = s.store.Queries().ListSchedules(ctx)
	} else {
		rows, err = s.store.Queries().ListSchedulesByProject(ctx, projectID)
	}
	if err != nil {
		return nil, err
	}

	out := make([]protocol.Schedule, len(rows))
	for i, r := range rows {
		out[i] = toProtocolSchedule(r)
	}
	return out, nil
}

// Get returns a single schedule by ID.
func (s *Service) Get(ctx context.Context, id string) (protocol.Schedule, error) {
	row, err := s.store.Queries().GetSchedule(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return protocol.Schedule{}, ErrNotFound
		}
		return protocol.Schedule{}, err
	}
	return toProtocolSchedule(row), nil
}

// Save creates or updates a schedule.
func (s *Service) Save(ctx context.Context, req protocol.SaveScheduleRequest) (protocol.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nowTime := s.now()
	now := nowTime.UnixMilli()
	req.Time, req.Days = Reconcile(req)
	cronExpr := cronExprFor(req, nowTime)

	daysJSONBytes, _ := json.Marshal(req.Days)
	if req.Days == nil {
		daysJSONBytes = []byte("[]")
	}

	enabledInt := int64(0)
	if req.Enabled {
		enabledInt = 1
	}

	id := req.ID
	var savedRow db.Schedule

	// The id is made before the write, not as part of it: protocol.NewID can fail on a clock the
	// id cannot hold, and a failure there must leave the database untouched rather than half-written.
	if id == "" {
		made, err := protocol.NewID(s.now(), s.entropy)
		if err != nil {
			return protocol.Schedule{}, fmt.Errorf("make a schedule id: %w", err)
		}
		id = made
	}

	err := s.store.Write(ctx, func(q *db.Queries) error {
		var wErr error
		if req.ID == "" {
			savedRow, wErr = q.CreateSchedule(ctx, db.CreateScheduleParams{
				ID:             id,
				ProjectID:      req.Project,
				Name:           req.Name,
				Kind:           req.Kind,
				Icon:           req.Icon,
				TriggerType:    req.Trigger,
				WhenText:       req.When,
				CronExpr:       cronExpr,
				TimeStr:        req.Time,
				DaysJSON:       string(daysJSONBytes),
				Action:         req.Action,
				Enabled:        enabledInt,
				MissedPolicy:   req.Missed,
				CreatedAt:      now,
				UpdatedAt:      now,
				LastRunAt:      0,
				Template:       req.Template,
				SectionsJSON:   listJSON(req.Sections),
				DeliverJSON:    listJSON(req.Deliver),
				QuietWhenEmpty: boolInt(req.QuietWhenEmpty),
			})
			return wErr
		}

		savedRow, wErr = q.UpdateSchedule(ctx, db.UpdateScheduleParams{
			ID:             id,
			Name:           req.Name,
			TriggerType:    req.Trigger,
			WhenText:       req.When,
			CronExpr:       cronExpr,
			TimeStr:        req.Time,
			DaysJSON:       string(daysJSONBytes),
			Action:         req.Action,
			Enabled:        enabledInt,
			MissedPolicy:   req.Missed,
			SectionsJSON:   listJSON(req.Sections),
			DeliverJSON:    listJSON(req.Deliver),
			QuietWhenEmpty: boolInt(req.QuietWhenEmpty),
			UpdatedAt:      now,
		})
		if errors.Is(wErr, sql.ErrNoRows) {
			// Updating a schedule that is not there is not an internal failure: it is an id the
			// screen still holds that somebody else deleted, and it answers not found like any other.
			return ErrNotFound
		}
		return wErr
	})

	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return protocol.Schedule{}, ErrNotFound
		}
		return protocol.Schedule{}, err
	}

	_ = s.reloadLocked(ctx)
	return toProtocolSchedule(savedRow), nil
}

// Delete removes a schedule by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.DeleteSchedule(ctx, id)
	})
	if err != nil {
		return err
	}
	return s.reloadLocked(ctx)
}

// maxRuns is the most runs one read of a schedule's history answers, newest first.
const maxRuns = 50

// Runs answers a schedule's own run history, newest first - a brief's own text is a run's
// details, and this is how a person reads one back until a real delivery channel exists (B8.5).
func (s *Service) Runs(ctx context.Context, id string) ([]protocol.ScheduleRun, error) {
	rows, err := s.store.Queries().ListScheduleRuns(ctx, db.ListScheduleRunsParams{ScheduleID: id, Limit: maxRuns})
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ScheduleRun, len(rows))
	for i, row := range rows {
		out[i] = protocol.ScheduleRun{
			ID: row.ID, RunAt: protocol.NewTimestamp(time.UnixMilli(row.RunAt)),
			Status: row.Status, Details: row.Details,
		}
	}
	return out, nil
}

func toProtocolSchedule(r db.Schedule) protocol.Schedule {
	var days []int
	if r.DaysJSON != "" {
		_ = json.Unmarshal([]byte(r.DaysJSON), &days)
	}
	if days == nil {
		days = []int{}
	}
	return protocol.Schedule{
		Template:       r.Template,
		Sections:       decodeList(r.SectionsJSON),
		Deliver:        decodeList(r.DeliverJSON),
		QuietWhenEmpty: r.QuietWhenEmpty == 1,
		ID:             r.ID,
		Name:           r.Name,
		Kind:           r.Kind,
		Icon:           r.Icon,
		Trigger:        r.TriggerType,
		When:           r.WhenText,
		Time:           r.TimeStr,
		Days:           days,
		Action:         r.Action,
		Project:        r.ProjectID,
		Enabled:        r.Enabled == 1,
		Missed:         r.MissedPolicy,
	}
}

// listJSON stores a list of ids. A missing list is stored as an empty one.
func listJSON(list []string) string {
	if list == nil {
		return "[]"
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// decodeList reads a stored list of ids. It is never nil, so the wire never carries null.
func decodeList(raw string) []string {
	var list []string
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &list)
	}
	if list == nil {
		return []string{}
	}
	return list
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// startersKey is the setting that says the starter schedules were made. It is set once, so a starter
// a person deletes is not made again.
const startersKey = "schedules.starters.v1"

// EnsureStarters makes one schedule from each starter template the first time a daemon runs, all
// switched off, delivering to every chat (a chat that is not connected is skipped when a brief goes
// out). The person turns the ones they want on and changes the rest. It is called once while the daemon
// starts, and does nothing after that, even if every starter was deleted.
func (s *Service) EnsureStarters(ctx context.Context) error {
	if _, err := s.store.Queries().GetSetting(ctx, startersKey); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read whether the starter schedules were made: %w", err)
	}
	existing, err := s.store.Queries().ListSchedules(ctx)
	if err != nil {
		return fmt.Errorf("list the schedules before making the starters: %w", err)
	}
	have := map[string]bool{}
	for _, row := range existing {
		have[row.Template] = true
	}
	var channels []string
	for _, channel := range briefs.Channels() {
		channels = append(channels, channel.ID)
	}
	for _, t := range briefs.Templates() {
		if have[t.Key] {
			continue
		}
		if _, err := s.Save(ctx, protocol.SaveScheduleRequest{
			Name: t.Name, Kind: "brief", Icon: t.Icon, Trigger: t.Trigger, When: t.When, Time: t.Time,
			Days: t.Days, Action: t.Summary, Enabled: false, Missed: t.Missed, Template: t.Key,
			Sections: t.Sections, Deliver: channels, QuietWhenEmpty: t.QuietWhenEmpty,
		}); err != nil {
			return fmt.Errorf("make the %s starter schedule: %w", t.Name, err)
		}
	}
	return s.store.Write(ctx, func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: startersKey, ValueJSON: "true"})
	})
}
