package dashboard_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The stored numbers and the activity stream (docs/backend-checklist.md B2.3, sections S19a and
// S20). The rows are written here the way the subscriber writes them, so what the routes answer is
// what the tables hold.

// statRow is one day and project's numbers, as a test gives them.
type statRow struct {
	day        time.Time
	projectID  string
	finished   int64
	merges     int64
	ciFailures int64
	costMicros int64
}

// seedStats writes daily number rows the way the subscriber's upsert does.
func seedStats(t *testing.T, st *store.Store, rows ...statRow) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		for _, row := range rows {
			if err := q.UpsertDailyStat(ctx, db.UpsertDailyStatParams{
				Day:           row.day.UnixMilli(),
				ProjectID:     row.projectID,
				CardsFinished: row.finished,
				Merges:        row.merges,
				CiFailures:    row.ciFailures,
				CostMicros:    row.costMicros,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed the stored numbers: %v", err)
	}
}

// activityRow is one row of the stream, as a test gives it.
type activityRow struct {
	id          string
	projectID   string
	kind        protocol.FeedKind
	subjectKind string
	subjectID   string
	subjectKey  string
	summary     string
	at          time.Time
}

// seedActivity appends rows to the stream in the order given, numbering them from 1.
func seedActivity(t *testing.T, st *store.Store, rows ...activityRow) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		for _, row := range rows {
			seq, err := q.NextActivitySeq(ctx)
			if err != nil {
				return err
			}
			if err := q.InsertActivity(ctx, db.InsertActivityParams{
				ID:          row.id,
				Seq:         seq,
				ProjectID:   row.projectID,
				Kind:        string(row.kind),
				SubjectKind: row.subjectKind,
				SubjectID:   row.subjectID,
				SubjectKey:  row.subjectKey,
				Summary:     row.summary,
				CreatedAt:   row.at.UnixMilli(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed the activity stream: %v", err)
	}
}

// allActivity reads every row of the stream, newest first, for an assertion.
func allActivity(t *testing.T, st *store.Store) []db.Activity {
	t.Helper()
	var rows []db.Activity
	err := st.Read(context.Background(), func(q *db.Queries) error {
		var err error
		rows, err = q.ListActivity(context.Background(), db.ListActivityParams{Seq: math.MaxInt64, Limit: 100})
		return err
	})
	if err != nil {
		t.Fatalf("read the activity stream: %v", err)
	}
	return rows
}

// allStats reads every stored day.
func allStats(t *testing.T, st *store.Store) []db.DailyStat {
	t.Helper()
	var rows []db.DailyStat
	err := st.Read(context.Background(), func(q *db.Queries) error {
		var err error
		rows, err = q.ListDailyStats(context.Background(), db.ListDailyStatsParams{FromDay: 0, ToDay: math.MaxInt64})
		return err
	})
	if err != nil {
		t.Fatalf("read the stored numbers: %v", err)
	}
	return rows
}

func midnight(daysAgo int) time.Time {
	return time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -daysAgo)
}

// A chart range is drawn from the stored days, every day of the range is on the answer, and the
// projects are the series a line-per-project chart needs.
func TestHomeStatsReadsTheStoredRange(t *testing.T) {
	svc, st := newService(t, fixedTime())
	seedStats(t, st,
		statRow{day: midnight(2), projectID: "web", finished: 1, merges: 1},
		statRow{day: midnight(1), projectID: "api", finished: 2, merges: 2},
		statRow{day: midnight(1), projectID: "web", finished: 1, merges: 1, ciFailures: 3},
		statRow{day: midnight(0), projectID: "api", finished: 1, merges: 1, costMicros: 4_000},
	)

	got, err := svc.Home(context.Background(), dashboard.RangeWeek)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	stats := got.Stats
	if stats == nil {
		t.Fatal("Home carries no stored numbers")
	}
	if stats.Range != dashboard.RangeWeek || len(stats.Days) != dashboard.RangeWeek {
		t.Fatalf("stats = %+v, want %d days", stats, dashboard.RangeWeek)
	}
	if stats.From.Time().UTC() != midnight(6) || stats.To.Time().UTC() != midnight(0) {
		t.Errorf("range = %v..%v, want %v..%v", stats.From.Time(), stats.To.Time(), midnight(6), midnight(0))
	}
	// The days with nothing stored still carry their date, so a chart has one point per day.
	for i, day := range stats.Days {
		want := midnight(dashboard.RangeWeek - 1 - i)
		if day.Day.Time().UTC() != want {
			t.Errorf("day %d = %v, want %v", i, day.Day.Time(), want)
		}
	}
	yesterday := stats.Days[dashboard.RangeWeek-2]
	if yesterday.CardsFinished != 3 || yesterday.Merges != 3 || yesterday.CIFailures != 3 {
		t.Errorf("yesterday = %+v, want the two projects added together", yesterday)
	}
	today := stats.Days[dashboard.RangeWeek-1]
	if today.CardsFinished != 1 || today.CostMicros != 4_000 {
		t.Errorf("today = %+v, want the api project's row", today)
	}
	if len(stats.Projects) != 2 || stats.Projects[0].ProjectID != "api" || stats.Projects[1].ProjectID != "web" {
		t.Fatalf("projects = %+v, want api then web", stats.Projects)
	}
	for _, series := range stats.Projects {
		if len(series.Days) != dashboard.RangeWeek {
			t.Errorf("project %s has %d days, want %d", series.ProjectID, len(series.Days), dashboard.RangeWeek)
		}
	}
	if series := stats.Projects[1]; series.Days[dashboard.RangeWeek-2].CIFailures != 3 {
		t.Errorf("the web series lost its CI failures: %+v", series.Days)
	}
}

// The three ranges the charts cover, and nothing else: a range the charts do not cover is answered
// with the seven-day one rather than with an error, because the answer must always be drawable.
func TestHomeStatsCoversTheThreeRanges(t *testing.T) {
	svc, _ := newService(t, fixedTime())
	for _, days := range dashboard.HomeRanges() {
		got, err := svc.Home(context.Background(), days)
		if err != nil {
			t.Fatalf("Home(%d): %v", days, err)
		}
		if got.Stats == nil || got.Stats.Range != days || len(got.Stats.Days) != days {
			t.Errorf("Home(%d) = %+v, want %d days", days, got.Stats, days)
		}
	}
	got, err := svc.Home(context.Background(), 5)
	if err != nil {
		t.Fatalf("Home(5): %v", err)
	}
	if got.Stats == nil || got.Stats.Range != dashboard.RangeWeek {
		t.Errorf("Home(5) = %+v, want the seven-day range", got.Stats)
	}
}

// The view-all list pages newest first by cursor, and the page after the last one is empty with the
// cursor the caller gave.
func TestActivityPagesNewestFirst(t *testing.T) {
	svc, st := newService(t, fixedTime())
	seedActivity(t, st,
		activityRow{id: "a1", projectID: "api", kind: protocol.FeedKindMerge, summary: "first", at: midnight(2)},
		activityRow{id: "a2", projectID: "api", kind: protocol.FeedKindCI, summary: "second", at: midnight(1)},
		activityRow{id: "a3", projectID: "api", kind: protocol.FeedKindTool, summary: "third", at: midnight(0)},
	)

	first, err := svc.Activity(context.Background(), dashboard.ActivityFilter{}, 0, 2)
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "a3" || first.Items[1].ID != "a2" {
		t.Fatalf("first page = %+v, want the two newest", first.Items)
	}
	if !first.More {
		t.Error("the first page says there is no more, but a third row follows")
	}
	second, err := svc.Activity(context.Background(), dashboard.ActivityFilter{}, first.Cursor, 2)
	if err != nil {
		t.Fatalf("Activity page two: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "a1" || second.More {
		t.Fatalf("second page = %+v (more=%v), want only the first row and no more", second.Items, second.More)
	}
	empty, err := svc.Activity(context.Background(), dashboard.ActivityFilter{}, second.Cursor, 2)
	if err != nil {
		t.Fatalf("Activity page three: %v", err)
	}
	if len(empty.Items) != 0 || empty.More || empty.Cursor != second.Cursor {
		t.Errorf("past the end = %+v, want an empty page that keeps the cursor", empty)
	}
}

// The filters narrow the stream by kind and by project, and the entry carries how a client opens it.
func TestActivityFiltersByKindAndProject(t *testing.T) {
	svc, st := newService(t, fixedTime())
	seedActivity(t, st,
		activityRow{
			id:          "a1",
			projectID:   "api",
			kind:        protocol.FeedKindMerge,
			subjectKind: "card",
			subjectID:   "01M3C107JB041061050R3GG28A",
			subjectKey:  "api#41",
			summary:     "merged",
			at:          midnight(2),
		},
		activityRow{id: "a2", projectID: "web", kind: protocol.FeedKindMerge, summary: "merged too", at: midnight(1)},
		activityRow{id: "a3", projectID: "", kind: protocol.FeedKindBrief, summary: "brief", at: midnight(0)},
	)

	kind, err := svc.Activity(context.Background(), dashboard.ActivityFilter{Kind: protocol.FeedKindMerge}, 0, 10)
	if err != nil {
		t.Fatalf("Activity by kind: %v", err)
	}
	if len(kind.Items) != 2 {
		t.Fatalf("by kind = %+v, want the two merges", kind.Items)
	}
	project, err := svc.Activity(context.Background(), dashboard.ActivityFilter{ProjectID: "api"}, 0, 10)
	if err != nil {
		t.Fatalf("Activity by project: %v", err)
	}
	if len(project.Items) != 1 || project.Items[0].ID != "a1" {
		t.Fatalf("by project = %+v, want the api row", project.Items)
	}
	entry := project.Items[0]
	if entry.CardID != "01M3C107JB041061050R3GG28A" || entry.CardKey != "api#41" {
		t.Errorf("entry = %+v, want the card it is about", entry)
	}
	if entry.ProjectID == nil || *entry.ProjectID != "api" {
		t.Errorf("projectId = %v, want api", entry.ProjectID)
	}
	both, err := svc.Activity(context.Background(), dashboard.ActivityFilter{
		Kind: protocol.FeedKindMerge, ProjectID: "web",
	}, 0, 10)
	if err != nil {
		t.Fatalf("Activity by kind and project: %v", err)
	}
	if len(both.Items) != 1 || both.Items[0].ID != "a2" {
		t.Fatalf("by kind and project = %+v, want the web merge", both.Items)
	}
	// An entry that belongs to no project carries no project, which is how the app tells "no
	// project" from "a project I do not know".
	none, err := svc.Activity(context.Background(), dashboard.ActivityFilter{Kind: protocol.FeedKindBrief}, 0, 10)
	if err != nil {
		t.Fatalf("Activity briefs: %v", err)
	}
	if len(none.Items) != 1 || none.Items[0].ProjectID != nil {
		t.Fatalf("brief = %+v, want no project", none.Items)
	}
}

// A kind that is not one of the fixed list is refused by the service, so a caller that skipped the
// route's own check cannot store or read garbage.
func TestActivityRejectsAnUnknownKind(t *testing.T) {
	svc, _ := newService(t, fixedTime())
	if _, err := svc.Activity(context.Background(), dashboard.ActivityFilter{Kind: "nonsense"}, 0, 10); err == nil {
		t.Error("Activity with an unknown kind succeeded")
	}
}

// The page size follows the API's own limits, whatever the caller asks for.
func TestActivityPageSize(t *testing.T) {
	svc, st := newService(t, fixedTime())
	for i := range 5 {
		seedActivity(t, st, activityRow{
			id: string(rune('a' + i)), projectID: "api", kind: protocol.FeedKindTool,
			summary: "row", at: midnight(0),
		})
	}
	got, err := svc.Activity(context.Background(), dashboard.ActivityFilter{}, 0, 0)
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if len(got.Items) != 5 {
		t.Errorf("page = %d rows, want all five when nothing stored is bigger than the default", len(got.Items))
	}
}
