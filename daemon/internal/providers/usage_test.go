package providers

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
	// The zones below must load on a machine with no zone database.
	_ "time/tzdata"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The ids a receipt test files against. They only have to be plain strings; nothing here is a card.
const (
	testCardID    = "01J8Z00000000000000000000CRD"
	testProjectID = "01J8Z00000000000000000000PRJ"
	testRoleID    = "reviewer"
)

// countingEntropy gives every id it is asked for different bytes, so two rows written in one test
// have different ids without the test reaching for randomness.
type countingEntropy struct{ n byte }

func (c *countingEntropy) Read(p []byte) (int, error) {
	for i := range p {
		c.n++
		p[i] = c.n
	}
	return len(p), nil
}

// openTestStore opens a real database in a temporary directory. It is a file rather than :memory:
// because the store's writer and its readers are separate connections and only a file is shared
// between them - the same reason the store's own tests use a temp file.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(),
		filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// usageRows reads a project's usage rows back, newest first.
func usageRows(t *testing.T, st *store.Store, projectID string) []db.Usage {
	t.Helper()
	rows, err := st.Queries().ListUsageForProject(context.Background(), db.ListUsageForProjectParams{
		ProjectID: projectID, Limit: 50,
	})
	if err != nil {
		t.Fatalf("read the usage rows: %v", err)
	}
	return rows
}

// dayRows reads one day's stored numbers back.
func dayRows(t *testing.T, st *store.Store, at time.Time) []db.DailyStat {
	t.Helper()
	day := dayStart(at).UnixMilli()
	rows, err := st.Queries().ListDailyStats(context.Background(), db.ListDailyStatsParams{
		FromDay: day, ToDay: day,
	})
	if err != nil {
		t.Fatalf("read the day's numbers: %v", err)
	}
	return rows
}

// TestStoreRecorderWritesTheRowAndTheDayTogether is the one-transaction rule: the call's own row and
// the day's total are written by the same Record, and they agree.
func TestStoreRecorderWritesTheRowAndTheDayTogether(t *testing.T) {
	st := openTestStore(t)
	at := time.Date(2026, time.September, 27, 9, 15, 0, 0, time.UTC)
	rec := NewStoreRecorder(st, &countingEntropy{})
	err := rec.Record(context.Background(), UsageRecord{
		CardID: testCardID, ProjectID: testProjectID, RoleID: testRoleID,
		Provider: AnthropicID, Model: "claude-sonnet-4-5",
		InputTokens: 1_000_000, OutputTokens: 2_000, CostMicros: 3_030_000, At: at,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	rows := usageRows(t, st, testProjectID)
	if len(rows) != 1 {
		t.Fatalf("stored %d usage rows, want one per call", len(rows))
	}
	got := rows[0]
	if got.CardID != testCardID || got.RoleID != testRoleID {
		t.Errorf("the row lost the card or the role: %+v", got)
	}
	if got.Provider != AnthropicID || got.Model != "claude-sonnet-4-5" {
		t.Errorf("the row names %s/%s, want the provider and model that answered", got.Provider, got.Model)
	}
	if got.InputTokens != 1_000_000 || got.OutputTokens != 2_000 || got.CostMicros != 3_030_000 {
		t.Errorf("the row's numbers are %+v, want the tokens and cost it was given", got)
	}
	if got.CreatedAt != at.UnixMilli() {
		t.Errorf("the row is stamped %d, want the call's own time %d", got.CreatedAt, at.UnixMilli())
	}
	if !protocol.ValidID(got.ID) {
		t.Errorf("the row's id %q is not an opaque id", got.ID)
	}

	days := dayRows(t, st, at)
	if len(days) != 1 {
		t.Fatalf("the day has %d rows, want one for the call's project", len(days))
	}
	if days[0].ProjectID != testProjectID {
		t.Errorf("the day's row is for project %q, want the card's project", days[0].ProjectID)
	}
	if days[0].CostMicros != 3_030_000 {
		t.Errorf("the day's cost is %d, want the same %d the row carries", days[0].CostMicros, 3_030_000)
	}
}

// TestStoreRecorderAddsTwoCallsIntoOneDay is what makes the Home chart's number the sum of the rows
// under it: two calls on one day and project leave two rows and one day total.
func TestStoreRecorderAddsTwoCallsIntoOneDay(t *testing.T) {
	st := openTestStore(t)
	at := time.Date(2026, time.September, 27, 11, 0, 0, 0, time.UTC)
	rec := NewStoreRecorder(st, &countingEntropy{})
	for _, cost := range []int64{1_500_000, 2_500_000} {
		if err := rec.Record(context.Background(), UsageRecord{
			ProjectID: testProjectID, Provider: OpenAIID, Model: "gpt-5-mini",
			InputTokens: 100, OutputTokens: 100, CostMicros: cost, At: at.Add(time.Second),
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if got := len(usageRows(t, st, testProjectID)); got != 2 {
		t.Errorf("stored %d usage rows, want two", got)
	}
	days := dayRows(t, st, at)
	if len(days) != 1 {
		t.Fatalf("the day has %d rows, want one", len(days))
	}
	if days[0].CostMicros != 4_000_000 {
		t.Errorf("the day's cost is %d, want both calls added up (4000000)", days[0].CostMicros)
	}
}

// TestStoreRecorderAddsNothingToADayForAZeroCostCall is the unpriced and the genuinely free case
// together: the row is the record of what was spent, and there is no cost to add to the day.
func TestStoreRecorderAddsNothingToADayForAZeroCostCall(t *testing.T) {
	st := openTestStore(t)
	at := time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC)
	rec := NewStoreRecorder(st, &countingEntropy{})
	if err := rec.Record(context.Background(), UsageRecord{
		ProjectID: testProjectID, Provider: LMStudioID, Model: "some-model-nobody-priced-7b",
		InputTokens: 4_000, OutputTokens: 500, CostMicros: 0, At: at,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	rows := usageRows(t, st, testProjectID)
	if len(rows) != 1 {
		t.Fatalf("stored %d usage rows, want the call recorded even at no cost", len(rows))
	}
	if rows[0].InputTokens != 4_000 {
		t.Errorf("the row's tokens are %d, want the 4000 read", rows[0].InputTokens)
	}
	if got := dayRows(t, st, at); len(got) != 0 {
		t.Errorf("a call that cost nothing left %d day rows, want none", len(got))
	}
}

// TestStoreRecorderWithNoStoreRecordsNothing is the guard for a daemon wired without a store: a
// recorder with nowhere to write is a no-op rather than a panic in the middle of a call.
func TestStoreRecorderWithNoStoreRecordsNothing(t *testing.T) {
	var rec *StoreRecorder
	if err := rec.Record(context.Background(), UsageRecord{At: testNow}); err != nil {
		t.Errorf("a recorder with no store answered %v, want nothing", err)
	}
}

// allDays reads every stored day, oldest first.
func allDays(t *testing.T, st *store.Store) []db.DailyStat {
	t.Helper()
	rows, err := st.Queries().ListDailyStats(context.Background(), db.ListDailyStatsParams{
		FromDay: 0, ToDay: math.MaxInt64,
	})
	if err != nil {
		t.Fatalf("read the days: %v", err)
	}
	return rows
}

func zoneNamed(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load zone %s: %v", name, err)
	}
	return loc
}

// TestStoreRecorderBucketsCostByTheChosenZone is the person's zone applied to the Home chart: one
// call at 23:00 UTC on October 8 is October 9 in Kampala and still October 8 in Los Angeles.
func TestStoreRecorderBucketsCostByTheChosenZone(t *testing.T) {
	kampala, la := zoneNamed(t, "Africa/Kampala"), zoneNamed(t, "America/Los_Angeles")
	at := time.Date(2026, time.October, 8, 23, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		at   time.Time
		loc  func() *time.Location
		want time.Time
	}{
		{"no zone chosen keeps the call's own", at.In(kampala), nil, time.Date(2026, 10, 9, 0, 0, 0, 0, kampala)},
		{"a zone that answers nil keeps the call's own", at, func() *time.Location { return nil }, time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)},
		{"Kampala", at, func() *time.Location { return kampala }, time.Date(2026, 10, 9, 0, 0, 0, 0, kampala)},
		{"Los Angeles", at, func() *time.Location { return la }, time.Date(2026, 10, 8, 0, 0, 0, 0, la)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openTestStore(t)
			rec := NewStoreRecorder(st, &countingEntropy{}, WithLocation(tc.loc))
			if err := rec.Record(context.Background(), UsageRecord{
				ProjectID: testProjectID, Provider: AnthropicID, Model: "claude-sonnet-4-5",
				CostMicros: 2_000_000, At: tc.at,
			}); err != nil {
				t.Fatalf("Record: %v", err)
			}
			days := allDays(t, st)
			if len(days) != 1 || days[0].Day != tc.want.UnixMilli() || days[0].CostMicros != 2_000_000 {
				t.Errorf("days = %+v, want one day at %v holding 2000000", days, tc.want)
			}
		})
	}
}

// TestStoreRecorderFollowsAZoneChangedBetweenCalls: the zone is read on each call, so a call after
// the person changes it lands in the new zone's day while the earlier one keeps its day.
func TestStoreRecorderFollowsAZoneChangedBetweenCalls(t *testing.T) {
	kampala, la := zoneNamed(t, "Africa/Kampala"), zoneNamed(t, "America/Los_Angeles")
	zone := kampala
	st := openTestStore(t)
	rec := NewStoreRecorder(st, &countingEntropy{}, WithLocation(func() *time.Location { return zone }))
	at := time.Date(2026, time.October, 8, 23, 0, 0, 0, time.UTC)
	for _, loc := range []*time.Location{kampala, la} {
		zone = loc
		if err := rec.Record(context.Background(), UsageRecord{
			ProjectID: testProjectID, Provider: OpenAIID, Model: "gpt-5-mini", CostMicros: 1_000, At: at,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	days := allDays(t, st)
	if len(days) != 2 {
		t.Fatalf("stored %d days, want one per zone: %+v", len(days), days)
	}
	// Los Angeles' October 8 begins at 07:00 UTC and Kampala's October 9 at 21:00 UTC the day before.
	if want := time.Date(2026, 10, 8, 0, 0, 0, 0, la).UnixMilli(); days[0].Day != want {
		t.Errorf("first day = %d, want Los Angeles' October 8 (%d)", days[0].Day, want)
	}
	if want := time.Date(2026, 10, 9, 0, 0, 0, 0, kampala).UnixMilli(); days[1].Day != want {
		t.Errorf("second day = %d, want Kampala's October 9 (%d)", days[1].Day, want)
	}
}
