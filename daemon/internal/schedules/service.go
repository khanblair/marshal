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

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

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
// handler gathers "what changed" from: the schedule's last run, or its creation if it has never
// run - read fresh at every fire, not from a closure captured when the cron entry was made, so a
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
		cron:     cron.New(),
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
	s.mu.Unlock()
	if err != nil {
		return err
	}
	// Unlocked: checkMissedRuns reads the store and may call executeSchedule, which takes s.mu
	// itself for the handler lookup. Holding the lock across this call would deadlock against that.
	s.checkMissedRuns(ctx)
	return nil
}

// Stop stops the cron scheduler.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cron.Stop()
	s.entries = make(map[string]cron.EntryID)
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
	if handler != nil {
		result, err := handler(ctx, sched, since)
		details = result
		if err != nil {
			status = "failed"
			details = err.Error()
			s.logger.Error("schedule execution failed", "id", sched.ID, "error", err)
		}
	}

	now := s.now().UnixMilli()
	_ = s.store.Write(ctx, func(q *db.Queries) error {
		if err := q.UpdateScheduleLastRun(ctx, db.UpdateScheduleLastRunParams{
			ID:        sched.ID,
			LastRunAt: now,
			UpdatedAt: now,
		}); err != nil {
			return err
		}

		runID := fmt.Sprintf("run_%d_%s", now, sched.ID)
		_, err := q.CreateScheduleRun(ctx, db.CreateScheduleRunParams{
			ID:         runID,
			ScheduleID: sched.ID,
			RunAt:      now,
			Status:     status,
			Details:    details,
		})
		return err
	})
	if sched.Trigger == triggerOneTime {
		// A one-time job's spec carries no year, so left enabled it would fire again next year on
		// the same date - the opposite of "once". Disabling it here, right after its one real run,
		// is what makes cron.AddFunc's ordinary machinery serve a job that never repeats.
		_ = s.store.Write(ctx, func(q *db.Queries) error {
			return q.DisableSchedule(ctx, db.DisableScheduleParams{ID: sched.ID, UpdatedAt: now})
		})
	}
}

// sinceOf is a schedule's last run, or its creation when it has never run, read fresh from the
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
	return time.UnixMilli(row.CreatedAt)
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
		if spec.Next(since).After(now) {
			continue
		}
		s.logger.Info("catching up a missed schedule", "id", row.ID, "name", row.Name)
		s.executeSchedule(ctx, toProtocolSchedule(row))
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
				ID:           id,
				ProjectID:    req.Project,
				Name:         req.Name,
				Kind:         req.Kind,
				Icon:         req.Icon,
				TriggerType:  req.Trigger,
				WhenText:     req.When,
				CronExpr:     cronExpr,
				TimeStr:      req.Time,
				DaysJSON:     string(daysJSONBytes),
				Action:       req.Action,
				Enabled:      enabledInt,
				MissedPolicy: req.Missed,
				CreatedAt:    now,
				UpdatedAt:    now,
				LastRunAt:    0,
			})
			return wErr
		}

		savedRow, wErr = q.UpdateSchedule(ctx, db.UpdateScheduleParams{
			ID:           id,
			Name:         req.Name,
			WhenText:     req.When,
			CronExpr:     cronExpr,
			TimeStr:      req.Time,
			DaysJSON:     string(daysJSONBytes),
			Action:       req.Action,
			Enabled:      enabledInt,
			MissedPolicy: req.Missed,
			UpdatedAt:    now,
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
// details, and this is how a person reads one back until a real delivery channel exists (B8.5,
// phase-reports/phase-08-automation.md).
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
		ID:      r.ID,
		Name:    r.Name,
		Kind:    r.Kind,
		Icon:    r.Icon,
		Trigger: r.TriggerType,
		When:    r.WhenText,
		Time:    r.TimeStr,
		Days:    days,
		Action:  r.Action,
		Project: r.ProjectID,
		Enabled: r.Enabled == 1,
		Missed:  r.MissedPolicy,
	}
}
