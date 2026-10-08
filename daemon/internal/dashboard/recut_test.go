package dashboard_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	// The zones below must load on a machine with no zone database.
	_ "time/tzdata"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Re-cutting Home's daily numbers when the person's time zone changes. The raw rows are the merge
// lines of the activity stream and the usage rows; the stored days are rebuilt from them.

const (
	recutAlpha = "alpha"
	recutBravo = "bravo"
)

func zoneNamed(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load zone %s: %v", name, err)
	}
	return loc
}

// recutNow is the clock of these tests: midday UTC on October 9, which is October 9 in all three zones.
func recutNow() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }

// usageSeed is one model call a test wants in the store.
type usageSeed struct {
	projectID string
	at        time.Time
	cost      int64
}

func seedUsage(t *testing.T, st *store.Store, rows ...usageSeed) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		for i, row := range rows {
			if err := q.InsertUsage(ctx, db.InsertUsageParams{
				ID: fmt.Sprintf("usage-%d-%d", row.at.UnixMilli(), i), ProjectID: row.projectID,
				Provider: "anthropic", Model: "claude-sonnet-4-5", InputTokens: 10, OutputTokens: 10,
				CostMicros: row.cost, CreatedAt: row.at.UnixMilli(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed the usage rows: %v", err)
	}
}

// mergeLine is the line the subscriber writes when a card reaches done.
func mergeLine(id, projectID string, kind protocol.FeedKind, at time.Time) activityRow {
	return activityRow{
		id: id, projectID: projectID, kind: kind, subjectKind: "card", subjectID: "card-" + id,
		subjectKey: projectID + "#1", summary: "#1 merged into main", at: at,
	}
}

// sumStats adds up every counter of a set of stored days.
func sumStats(rows []db.DailyStat) statRow {
	var sum statRow
	for _, row := range rows {
		sum.finished += row.CardsFinished
		sum.merges += row.Merges
		sum.ciFailures += row.CiFailures
		sum.costMicros += row.CostMicros
	}
	return sum
}

// expectStats checks the stored days are exactly the wanted rows, in day then project order.
func expectStats(t *testing.T, got []db.DailyStat, want ...statRow) {
	t.Helper()
	wantRows := make([]db.DailyStat, 0, len(want))
	for _, row := range want {
		wantRows = append(wantRows, db.DailyStat{
			Day: row.day.UnixMilli(), ProjectID: row.projectID, CardsFinished: row.finished,
			Merges: row.merges, CiFailures: row.ciFailures, CostMicros: row.costMicros,
		})
	}
	if !slices.Equal(got, wantRows) {
		t.Errorf("stored days:\n got  %+v\n want %+v", got, wantRows)
	}
}

// seedRecutWorld stores the events of October 7 and 8, and the days the daemon wrote for them in UTC.
func seedRecutWorld(t *testing.T, st *store.Store) {
	t.Helper()
	utc := func(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC) }
	seedProject(t, st, recutAlpha, "Alpha")
	seedProject(t, st, recutBravo, "Bravo")
	seedActivity(t, st,
		mergeLine("m1", recutAlpha, protocol.FeedKindMerge, utc(7, 23, 0)),
		mergeLine("m2", recutAlpha, protocol.FeedKindMerge, utc(8, 5, 0)),
		mergeLine("m3", recutAlpha, protocol.FeedKindMerge, utc(8, 23, 0)),
		mergeLine("m4", recutBravo, protocol.FeedKindMerge, utc(8, 23, 30)),
		// A line that is not a finished card adds nothing to the day.
		mergeLine("t1", recutAlpha, protocol.FeedKindTool, utc(8, 23, 10)),
		// Each CI failure is a line of its own.
		mergeLine("c1", recutAlpha, protocol.FeedKindCI, utc(8, 5, 5)),
		mergeLine("c2", recutAlpha, protocol.FeedKindCI, utc(8, 23, 5)),
	)
	seedUsage(t, st,
		usageSeed{recutAlpha, utc(8, 5, 0), 250_000},
		usageSeed{recutAlpha, utc(8, 23, 0), 1_000_000},
		// A call with no project is a day of its own, and a call that cost nothing is no day at all.
		usageSeed{"", utc(8, 23, 15), 40_000},
		usageSeed{recutAlpha, utc(8, 23, 20), 0},
		// A removed project's spend stays in `usage` but was dropped from the days with the project.
		usageSeed{"gone", utc(8, 23, 25), 777_000},
	)
	// What the daemon stored as these events came in, with the clock in UTC.
	seedStats(t, st,
		statRow{day: utc(7, 0, 0), projectID: recutAlpha, finished: 1, merges: 1},
		statRow{day: utc(8, 0, 0), projectID: "", costMicros: 40_000},
		statRow{day: utc(8, 0, 0), projectID: recutAlpha, finished: 2, merges: 2, ciFailures: 2, costMicros: 1_250_000},
		statRow{day: utc(8, 0, 0), projectID: recutBravo, finished: 1, merges: 1},
	)
}

// The same events land on different days in different zones, no number is lost or invented on the
// way, and cutting again for the same zone changes nothing.
func TestRecutMovesEventsToTheDaysOfTheNewZone(t *testing.T) {
	svc, st := newService(t, recutNow())
	seedRecutWorld(t, st)
	utc, kampala, la := time.UTC, zoneNamed(t, "Africa/Kampala"), zoneNamed(t, "America/Los_Angeles")
	day := func(loc *time.Location, d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, loc) }

	steps := []struct {
		name string
		loc  *time.Location
		want []statRow
	}{
		{"UTC, the zone they were stored in, changes nothing", utc, []statRow{
			{day: day(utc, 7), projectID: recutAlpha, finished: 1, merges: 1},
			{day: day(utc, 8), projectID: "", costMicros: 40_000},
			{day: day(utc, 8), projectID: recutAlpha, finished: 2, merges: 2, ciFailures: 2, costMicros: 1_250_000},
			{day: day(utc, 8), projectID: recutBravo, finished: 1, merges: 1},
		}},
		{"Kampala is three hours ahead, so 23:00 UTC is already the next day", kampala, []statRow{
			{day: day(kampala, 8), projectID: recutAlpha, finished: 2, merges: 2, ciFailures: 1, costMicros: 250_000},
			{day: day(kampala, 9), projectID: "", costMicros: 40_000},
			{day: day(kampala, 9), projectID: recutAlpha, finished: 1, merges: 1, ciFailures: 1, costMicros: 1_000_000},
			{day: day(kampala, 9), projectID: recutBravo, finished: 1, merges: 1},
		}},
		{"Los Angeles is seven hours behind, so the early events fall on the day before", la, []statRow{
			{day: day(la, 7), projectID: recutAlpha, finished: 2, merges: 2, ciFailures: 1, costMicros: 250_000},
			{day: day(la, 8), projectID: "", costMicros: 40_000},
			{day: day(la, 8), projectID: recutAlpha, finished: 1, merges: 1, ciFailures: 1, costMicros: 1_000_000},
			{day: day(la, 8), projectID: recutBravo, finished: 1, merges: 1},
		}},
		{"and back to Kampala", kampala, []statRow{
			{day: day(kampala, 8), projectID: recutAlpha, finished: 2, merges: 2, ciFailures: 1, costMicros: 250_000},
			{day: day(kampala, 9), projectID: "", costMicros: 40_000},
			{day: day(kampala, 9), projectID: recutAlpha, finished: 1, merges: 1, ciFailures: 1, costMicros: 1_000_000},
			{day: day(kampala, 9), projectID: recutBravo, finished: 1, merges: 1},
		}},
	}
	before := sumStats(allStats(t, st))
	for _, step := range steps {
		for pass := 1; pass <= 2; pass++ {
			if err := svc.Recut(context.Background(), step.loc); err != nil {
				t.Fatalf("%s: Recut pass %d: %v", step.name, pass, err)
			}
			got := allStats(t, st)
			expectStats(t, got, step.want...)
			if sum := sumStats(got); sum != before {
				t.Errorf("%s (pass %d): totals are %+v, want %+v conserved", step.name, pass, sum, before)
			}
		}
	}
	if want := (statRow{finished: 4, merges: 4, ciFailures: 2, costMicros: 1_290_000}); before != want {
		t.Errorf("seeded totals = %+v, want %+v", before, want)
	}
}

// Days before the ninety-day window are not touched, and neither are events older than it.
func TestRecutLeavesDaysBeforeTheWindowAlone(t *testing.T) {
	svc, st := newService(t, recutNow())
	kampala := zoneNamed(t, "Africa/Kampala")
	seedProject(t, st, recutAlpha, "Alpha")
	// Today is October 9, so the window starts at midnight Kampala on July 12: 21:00 UTC on July 11.
	cutoff := time.Date(2026, 7, 12, 0, 0, 0, 0, kampala)
	old := statRow{day: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), projectID: recutAlpha, finished: 5, merges: 5, ciFailures: 1, costMicros: 9}
	seedStats(t, st, old,
		// A day inside the window that nothing in the stream or in `usage` backs is not carried over.
		statRow{day: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), projectID: recutAlpha, finished: 9, merges: 9},
	)
	seedActivity(t, st,
		mergeLine("before", recutAlpha, protocol.FeedKindMerge, cutoff.Add(-time.Minute)),
		mergeLine("after", recutAlpha, protocol.FeedKindMerge, cutoff.Add(time.Minute)),
		mergeLine("long-ago", recutAlpha, protocol.FeedKindMerge, old.day.Add(time.Hour)),
	)
	seedUsage(t, st,
		usageSeed{recutAlpha, cutoff.Add(-time.Minute), 3_000},
		usageSeed{recutAlpha, cutoff.Add(time.Minute), 4_000},
	)

	if err := svc.Recut(context.Background(), kampala); err != nil {
		t.Fatalf("Recut: %v", err)
	}
	expectStats(t, allStats(t, st), old,
		statRow{day: cutoff, projectID: recutAlpha, finished: 1, merges: 1, costMicros: 4_000})
}

// A zone is required: there is no day to cut by without one.
func TestRecutNeedsAZone(t *testing.T) {
	svc, st := newService(t, recutNow())
	seedStats(t, st, statRow{day: midnight(1), projectID: recutAlpha, finished: 1, merges: 1})
	if err := svc.Recut(context.Background(), nil); err == nil {
		t.Fatal("Recut(nil) succeeded, want an error")
	}
	if got := len(allStats(t, st)); got != 1 {
		t.Errorf("a refused re-cut left %d rows, want the 1 it found", got)
	}
}
