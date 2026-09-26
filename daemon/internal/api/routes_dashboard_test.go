package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The two Home routes (docs/backend-checklist.md B2.3, sections S19a and S20). The first is the
// dashboard answer, whose `range` decides how many days the charts cover; the second is the paged,
// filterable view-all list behind Recent activity.

// addDailyStat stores one day's numbers for a project, the way the subscriber's upsert does.
func addDailyStat(t *testing.T, st *stack, day time.Time, projectID string, finished, merges int64) {
	t.Helper()
	ctx := context.Background()
	err := st.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertDailyStat(ctx, db.UpsertDailyStatParams{
			Day: day.UnixMilli(), ProjectID: projectID, CardsFinished: finished, Merges: merges,
		})
	})
	if err != nil {
		t.Fatalf("store a day's numbers: %v", err)
	}
}

// feedRow is one row addFeedRow appends to the activity stream.
type feedRow struct {
	id, projectID, summary string
	kind                   protocol.FeedKind
	at                     time.Time
}

// addFeedRow appends one row of the activity stream, the way the subscriber does. A row that
// belongs to a project is about a card of it; a row with no project is about nothing in particular.
func addFeedRow(t *testing.T, st *stack, row feedRow) {
	t.Helper()
	params := db.InsertActivityParams{
		ID: row.id, ProjectID: row.projectID, Kind: string(row.kind), Summary: row.summary,
		CreatedAt: row.at.UnixMilli(),
	}
	if row.projectID != "" {
		params.SubjectKind = "card"
		params.SubjectID = "01M3C107JB041061050R3GG28A"
		params.SubjectKey = row.projectID + "#41"
	}
	ctx := context.Background()
	err := st.store.Write(ctx, func(q *db.Queries) error {
		seq, err := q.NextActivitySeq(ctx)
		if err != nil {
			return err
		}
		params.Seq = seq
		return q.InsertActivity(ctx, params)
	})
	if err != nil {
		t.Fatalf("store a row of the stream: %v", err)
	}
}

// localMidnight is midnight at the start of the daemon's own day, which is the day a stored number
// belongs to.
func localMidnight(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// Home reads from stored state: the cards that wait on a person, the cards with a running session,
// and the tile counts. It is there before anything exists, with empty lists.
func TestHomeThroughHTTP(t *testing.T) {
	st := newStack(t)
	empty := decode[protocol.HomeSnapshot](t, st.do(http.MethodGet, "/v1/home/dashboard", nil).want(t, http.StatusOK))
	if len(empty.Needs) != 0 || len(empty.Awake) != 0 || empty.Tiles != (protocol.HomeTiles{}) {
		t.Errorf("Home with nothing = %+v", empty)
	}
	if empty.Stats == nil || empty.Stats.Range != 7 || len(empty.Stats.Days) != 7 {
		t.Errorf("Home with nothing has no charts: %+v", empty.Stats)
	}
	if empty.ServerTime == (protocol.Timestamp{}) {
		t.Error("Home does not carry the daemon's time")
	}

	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Waiting on me")
	// Put the card in Needs you the way the daemon does when an agent waits, then ask Home again.
	if _, err := st.proj.SetState(context.Background(), card.ID, protocol.CardStateNeeds); err != nil {
		t.Fatalf("move the card to needs: %v", err)
	}

	got := decode[protocol.HomeSnapshot](t, st.do(http.MethodGet, "/v1/home/dashboard", nil).want(t, http.StatusOK))
	if len(got.Needs) != 1 || got.Needs[0].Key != card.Key || got.Needs[0].ProjectName != project.Name {
		t.Errorf("needs = %+v, want the card with its project's name", got.Needs)
	}
	if got.Tiles.Needs != 1 {
		t.Errorf("needs tile = %d, want 1", got.Tiles.Needs)
	}
}

// The charts are the stored numbers over the range the request asked for, and every day of the range
// is on the answer.
func TestHomeChartsThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	today := localMidnight(st.now())
	addDailyStat(t, st, today, project.ID, 3, 3)

	week := decode[protocol.HomeSnapshot](t, st.do(http.MethodGet, "/v1/home/dashboard?range=7", nil).want(t, http.StatusOK))
	if week.Stats == nil || week.Stats.Range != 7 || len(week.Stats.Days) != 7 {
		t.Fatalf("seven days = %+v", week.Stats)
	}
	if last := week.Stats.Days[6]; last.CardsFinished != 3 || last.Merges != 3 {
		t.Errorf("today = %+v, want the stored row", last)
	}
	if week.Stats.Days[0].CardsFinished != 0 {
		t.Errorf("a day with nothing stored = %+v, want zero", week.Stats.Days[0])
	}

	month := decode[protocol.HomeSnapshot](t, st.do(http.MethodGet, "/v1/home/dashboard?range=30", nil).want(t, http.StatusOK))
	if month.Stats == nil || month.Stats.Range != 30 || len(month.Stats.Days) != 30 {
		t.Errorf("thirty days = %+v", month.Stats)
	}
	quarter := decode[protocol.HomeSnapshot](t, st.do(http.MethodGet, "/v1/home/dashboard?range=90", nil).want(t, http.StatusOK))
	if quarter.Stats == nil || quarter.Stats.Range != 90 || len(quarter.Stats.Days) != 90 {
		t.Errorf("ninety days = %+v", quarter.Stats)
	}
}

// A range that is not one of the three the charts cover is refused with a sentence, not guessed at.
func TestHomeRangeIsChecked(t *testing.T) {
	st := newStack(t)
	for _, bad := range []string{"5", "0", "-7", "days", "7.0"} {
		answer := st.do(http.MethodGet, "/v1/home/dashboard?range="+bad, nil).want(t, http.StatusBadRequest)
		got := answer.apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
		if got.Message == "" {
			t.Errorf("range=%s was refused without a sentence", bad)
		}
	}
}

// The activity stream is a real page of the stored rows: newest first, one page at a time, and
// narrowed by kind and by project.
func TestHomeActivityThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	now := st.now()
	addFeedRow(t, st, feedRow{
		id: "01M3C107JB041061050R3GG28A", projectID: project.ID, kind: protocol.FeedKindMerge,
		summary: "merged", at: now.Add(-2 * time.Minute),
	})
	addFeedRow(t, st, feedRow{
		id: "01M3C107JB041061050R3GG28B", projectID: project.ID, kind: protocol.FeedKindCI,
		summary: "CI failed", at: now.Add(-time.Minute),
	})
	addFeedRow(t, st, feedRow{
		id: "01M3C107JB041061050R3GG28C", projectID: "", kind: protocol.FeedKindBrief,
		summary: "Morning brief", at: now,
	})

	page := decode[protocol.Page[protocol.FeedEntry]](t, st.do(http.MethodGet, "/v1/home/activity", nil).want(t, http.StatusOK))
	if len(page.Items) != 3 || page.Items[0].ID != "01M3C107JB041061050R3GG28C" {
		t.Fatalf("the page = %+v, want the three rows newest first", page.Items)
	}
	if page.NextCursor != "" {
		t.Errorf("nextCursor = %q, want empty at the end of a short list", page.NextCursor)
	}
	if page.Items[2].ProjectID == nil || *page.Items[2].ProjectID != project.ID ||
		page.Items[2].CardKey != project.ID+"#41" {
		t.Errorf("the merge row lost how to open it: %+v", page.Items[2])
	}
	if page.Items[0].ProjectID != nil {
		t.Errorf("the brief belongs to no project, so it must carry none: %+v", page.Items[0])
	}

	limited := decode[protocol.Page[protocol.FeedEntry]](t, st.do(http.MethodGet, "/v1/home/activity?limit=1", nil).want(t, http.StatusOK))
	if len(limited.Items) != 1 || limited.NextCursor == "" {
		t.Fatalf("a page of one = %+v", limited)
	}
	next := decode[protocol.Page[protocol.FeedEntry]](t, st.do(http.MethodGet, "/v1/home/activity?limit=1&cursor="+limited.NextCursor, nil).want(t, http.StatusOK))
	if len(next.Items) != 1 || next.Items[0].ID == limited.Items[0].ID {
		t.Errorf("the page after the cursor = %+v, want the next row", next.Items)
	}

	merges := decode[protocol.Page[protocol.FeedEntry]](t, st.do(http.MethodGet, "/v1/home/activity?kind=merge", nil).want(t, http.StatusOK))
	if len(merges.Items) != 1 || merges.Items[0].Kind != protocol.FeedKindMerge {
		t.Errorf("kind=merge = %+v", merges.Items)
	}
	byProject := decode[protocol.Page[protocol.FeedEntry]](t, st.do(http.MethodGet, "/v1/home/activity?project="+project.ID, nil).want(t, http.StatusOK))
	if len(byProject.Items) != 2 {
		t.Errorf("project=%s = %+v, want the two rows of the project", project.ID, byProject.Items)
	}
}

// A filter that cannot be meant is refused with a sentence, so a client that misspells a kind is
// told instead of shown nothing.
func TestHomeActivityFiltersAreChecked(t *testing.T) {
	st := newStack(t)
	bad := st.do(http.MethodGet, "/v1/home/activity?kind=nonsense", nil).want(t, http.StatusBadRequest)
	if got := bad.apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument); got.Message == "" {
		t.Error("an unknown kind was refused without a sentence")
	}
	badProject := st.do(http.MethodGet, "/v1/home/activity?project=Not_A_Project", nil).want(t, http.StatusBadRequest)
	if got := badProject.apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument); got.Message == "" {
		t.Error("an impossible project id was refused without a sentence")
	}
	badCursor := st.do(http.MethodGet, "/v1/home/activity?cursor=not-a-cursor", nil).want(t, http.StatusBadRequest)
	if got := badCursor.apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument); got.Message == "" {
		t.Error("a bad cursor was refused without a sentence")
	}
}
