package schedules_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/schedules"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "marshal.db")
	st, err := store.Open(context.Background(), dbPath, store.WithLogger(nil))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestScheduleServiceCRUD(t *testing.T) {
	st := newTestStore(t)
	svc := schedules.NewService(st, nil)
	ctx := context.Background()

	// List initially empty
	list, err := svc.List(ctx, "")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("got %d schedules, want 0", len(list))
	}

	// Save new schedule
	req := protocol.SaveScheduleRequest{
		Project: "small-repo",
		Name:    "Morning Brief",
		Kind:    "brief",
		Icon:    "sun",
		Trigger: "Cron",
		When:    "Every weekday at 09:00",
		Time:    "09:00",
		Days:    []int{1, 2, 3, 4, 5},
		Action:  "morning_brief",
		Enabled: true,
		Missed:  "run_now",
	}

	created, err := svc.Save(ctx, req)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if created.ID == "" || created.Name != "Morning Brief" {
		t.Errorf("created schedule unexpected: %+v", created)
	}

	// Get by ID
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Name != "Morning Brief" || !got.Enabled {
		t.Errorf("got schedule unexpected: %+v", got)
	}

	// Update schedule
	reqUpdate := protocol.SaveScheduleRequest{
		ID:      created.ID,
		Project: "small-repo",
		Name:    "Morning Brief Updated",
		Kind:    "brief",
		Icon:    "sun",
		Trigger: "Cron",
		When:    "Every day at 09:30",
		Time:    "09:30",
		Days:    []int{0, 1, 2, 3, 4, 5, 6},
		Action:  "morning_brief",
		Enabled: false,
		Missed:  "skip",
	}

	updated, err := svc.Save(ctx, reqUpdate)
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if updated.Name != "Morning Brief Updated" || updated.Enabled {
		t.Errorf("updated schedule unexpected: %+v", updated)
	}

	// Delete
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	_, err = svc.Get(ctx, created.ID)
	if err != schedules.ErrNotFound {
		t.Errorf("Get after delete error = %v, want ErrNotFound", err)
	}
}

// TestStartCatchesUpAScheduleMissedOnWake covers 17.5: a schedule whose "missed" is "Run once on
// wake" and whose next tick, counted from its creation, already passed - one the daemon's own
// downtime cost a firing - runs once as soon as Start loads it.
func TestStartCatchesUpAScheduleMissedOnWake(t *testing.T) {
	st := newTestStore(t)
	// The clock moves between the save (the schedule's creation time) and Start (when the daemon
	// asks "did we miss anything"), the same way a real gap between a save and the next start does.
	clock := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	ctx := context.Background()

	created, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Project: "p1", Name: "Catch-up job", Kind: "job", Icon: "bolt", Trigger: "Cron",
		When: "Every 15 minutes", Action: "noop", Enabled: true, Missed: "Run once on wake",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	clock = clock.Add(time.Hour)

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	runs, err := st.Queries().ListScheduleRuns(ctx, db.ListScheduleRunsParams{ScheduleID: created.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListScheduleRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("recorded %d runs, want 1 catch-up run", len(runs))
	}
	if runs[0].Status != "success" {
		t.Errorf("the catch-up run's status = %q, want success (no handler registered)", runs[0].Status)
	}

	row, err := st.Queries().GetSchedule(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if row.LastRunAt == 0 {
		t.Error("the schedule's last run time is still zero after the catch-up")
	}
}

// TestStartSkipsAScheduleWhoseMissedPolicyIsSkip covers 17.5's other setting: a schedule that
// missed a tick while the daemon was down but is set to "Skip" runs no catch-up.
func TestStartSkipsAScheduleWhoseMissedPolicyIsSkip(t *testing.T) {
	st := newTestStore(t)
	longAgo := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return longAgo }))
	ctx := context.Background()

	created, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Project: "p1", Name: "Skip job", Kind: "job", Icon: "bolt", Trigger: "Cron",
		When: "Every 15 minutes", Action: "noop", Enabled: true, Missed: "Skip",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	runs, err := st.Queries().ListScheduleRuns(ctx, db.ListScheduleRunsParams{ScheduleID: created.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListScheduleRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("recorded %d runs, want 0: a Skip policy must not catch up", len(runs))
	}
}

// TestStartRunsAScheduleOnceForAWakeItAlreadyCaughtUpOn covers the "at most once" half of 17.5: a
// schedule that already has a recent last run is not fired again just because Start loads it.
func TestStartDoesNotRefireAScheduleThatAlreadyRanRecently(t *testing.T) {
	st := newTestStore(t)
	now := time.Now().UTC()
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return now }))
	ctx := context.Background()

	created, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Project: "p1", Name: "Fresh job", Kind: "job", Icon: "bolt", Trigger: "Cron",
		When: "Every 15 minutes", Action: "noop", Enabled: true, Missed: "Run once on wake",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := st.Write(ctx, func(q *db.Queries) error {
		return q.UpdateScheduleLastRun(ctx, db.UpdateScheduleLastRunParams{
			ID: created.ID, LastRunAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
		})
	}); err != nil {
		t.Fatalf("seed a recent last run: %v", err)
	}

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	runs, err := st.Queries().ListScheduleRuns(ctx, db.ListScheduleRunsParams{ScheduleID: created.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListScheduleRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("recorded %d runs, want 0: the next tick has not arrived yet", len(runs))
	}
}

// TestStartDoesNotCatchUpAOneTimeOrEventSchedule covers repeats(): ParseCron always answers a
// spec, so without this gate a One-time or Event row would be treated as a missed repeating tick.
func TestStartDoesNotCatchUpAOneTimeOrEventSchedule(t *testing.T) {
	st := newTestStore(t)
	clock := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	ctx := context.Background()

	for _, trigger := range []string{"One-time", "Event"} {
		created, err := svc.Save(ctx, protocol.SaveScheduleRequest{
			Project: "p1", Name: trigger + " job", Kind: "job", Icon: "bolt", Trigger: trigger,
			When: "Every 15 minutes", Action: "noop", Enabled: true, Missed: "Run once on wake",
		})
		if err != nil {
			t.Fatalf("Save(%s): %v", trigger, err)
		}
		clock = clock.Add(time.Hour)
		if err := svc.Start(ctx); err != nil {
			t.Fatalf("Start: %v", err)
		}
		runs, err := st.Queries().ListScheduleRuns(ctx, db.ListScheduleRunsParams{ScheduleID: created.ID, Limit: 10})
		if err != nil {
			t.Fatalf("ListScheduleRuns: %v", err)
		}
		if len(runs) != 0 {
			t.Errorf("%s: recorded %d runs, want 0: not a repeating trigger", trigger, len(runs))
		}
		svc.Stop()
	}
}

// TestExecuteScheduleFallsBackFromActionToKind covers the dispatch fallback: a brief's Action is a
// person's free-text sentence, so it is looked up by Kind instead once Action matches nothing.
func TestExecuteScheduleFallsBackFromActionToKind(t *testing.T) {
	st := newTestStore(t)
	clock := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	ctx := context.Background()

	var gotAction, gotKind string
	svc.RegisterHandler("brief", func(_ context.Context, s protocol.Schedule, _ time.Time) (string, error) {
		gotAction, gotKind = s.Action, s.Kind
		return "composed", nil
	})

	created, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Project: "p1", Name: "Morning brief", Kind: "brief", Icon: "sun", Trigger: "Cron",
		When: "Every 15 minutes", Action: "Send the brief to the app", Enabled: true, Missed: "Run once on wake",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	clock = clock.Add(time.Hour)

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	if gotKind != "brief" {
		t.Errorf("the handler registered under Kind was not the one called: action=%q kind=%q", gotAction, gotKind)
	}

	runs, err := st.Queries().ListScheduleRuns(ctx, db.ListScheduleRunsParams{ScheduleID: created.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListScheduleRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Details != "composed" {
		t.Errorf("runs = %+v, want one run whose details is the handler's own returned string", runs)
	}
}

// TestOneTimeScheduleFiresOnceThenDisables covers a One-time schedule end to end: its date already
// passed (missed-run catch-up fires it, since ParseOneTime's spec carries no year), and it disables
// itself right after, so it never fires again next year on the same date.
func TestOneTimeScheduleFiresOnceThenDisables(t *testing.T) {
	st := newTestStore(t)
	clock := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return clock }))
	ctx := context.Background()

	created, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Project: "p1", Name: "One-time job", Kind: "job", Icon: "bolt", Trigger: "One-time",
		When: "On 15 March at 09:00", Action: "noop", Enabled: true, Missed: "Run once on wake",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// March 15, 2020 has long since passed by the time Start runs.
	clock = clock.AddDate(1, 0, 0)

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	runs, err := st.Queries().ListScheduleRuns(ctx, db.ListScheduleRunsParams{ScheduleID: created.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListScheduleRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("recorded %d runs, want exactly 1", len(runs))
	}

	row, err := st.Queries().GetSchedule(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if row.Enabled != 0 {
		t.Error("a one-time schedule stayed enabled after firing, want it disabled")
	}
}
