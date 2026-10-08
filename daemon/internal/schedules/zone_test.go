package schedules_test

import (
	"context"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/runctx"
	"github.com/khanblair/marshal/daemon/internal/schedules"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A weekday schedule made on Wed Oct 7 at 15:00 UTC, and a daemon that starts on Thu Oct 8 at 11:35 UTC.
var (
	madeAt    = time.Date(2026, time.October, 7, 15, 0, 0, 0, time.UTC)
	startedAt = time.Date(2026, time.October, 8, 11, 35, 0, 0, time.UTC)
)

// catchUp saves a "Run once on wake" 08:00 weekday schedule while the clock reads madeAt in the zone,
// starts the service at startedAt, and answers when the handler was told the run was due ("" for a run
// that was not a catch-up) and how many runs were recorded.
func catchUp(t *testing.T, zoneName string) (due time.Time, runs int) {
	t.Helper()
	loc, err := time.LoadLocation(zoneName)
	if err != nil {
		t.Fatal(err)
	}
	st := newTestStore(t)
	instant := madeAt
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return instant.In(loc) }))
	svc.RegisterHandler("brief", func(ctx context.Context, _ protocol.Schedule, _ time.Time) (string, error) {
		if got, late := runctx.Late(ctx); late {
			due = got
		}
		return "", nil
	})
	ctx := context.Background()
	created, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Name: "Morning brief", Kind: "brief", Icon: "sunrise", Trigger: "Cron",
		When: "Every weekday at 8:00", Time: "08:00", Days: []int{1, 2, 3, 4, 5},
		Action: "x", Enabled: true, Missed: "Run once on wake",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	instant = startedAt
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()
	rows, err := st.Queries().ListScheduleRuns(ctx, db.ListScheduleRunsParams{ScheduleID: created.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	return due, len(rows)
}

func TestACatchUpRunKnowsWhenItWasDue(t *testing.T) {
	due, runs := catchUp(t, "Africa/Kampala")
	// 08:00 on Thursday in Kampala is 05:00 UTC.
	want := time.Date(2026, time.October, 8, 5, 0, 0, 0, time.UTC)
	if runs != 1 || !due.Equal(want) {
		t.Fatalf("runs = %d, due = %v; want one run due at %v", runs, due, want)
	}
}

func TestTheSameInstantIsDueInOneZoneAndNotYetInAnother(t *testing.T) {
	// 11:35 UTC is 14:35 in Kampala, past that day's 08:00, but 04:35 in Los Angeles, before it.
	if _, runs := catchUp(t, "Africa/Kampala"); runs != 1 {
		t.Errorf("Kampala caught up %d runs, want 1", runs)
	}
	if _, runs := catchUp(t, "America/Los_Angeles"); runs != 0 {
		t.Errorf("Los Angeles caught up %d runs, want 0: its 08:00 has not come", runs)
	}
}

func TestRezoneKeepsEveryScheduleRunning(t *testing.T) {
	st := newTestStore(t)
	loc := time.UTC
	svc := schedules.NewService(st, nil, schedules.WithClock(func() time.Time { return madeAt.In(loc) }))
	ctx := context.Background()
	if _, err := svc.Save(ctx, protocol.SaveScheduleRequest{
		Name: "Morning brief", Kind: "brief", Icon: "sunrise", Trigger: "Cron",
		When: "Every weekday at 8:00", Time: "08:00", Days: []int{1, 2, 3, 4, 5},
		Action: "x", Enabled: true, Missed: "Skip",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer svc.Stop()
	loc = time.FixedZone("EAT", 3*60*60)
	if err := svc.Rezone(ctx); err != nil {
		t.Fatalf("Rezone: %v", err)
	}
	if err := svc.Rezone(ctx); err != nil {
		t.Fatalf("a second Rezone: %v", err)
	}
}
