package dashboard_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fixedTime is the moment every test in this file works from. It is a Wednesday at midday, in a
// fixed zone, so "today" is the same wherever the test runs.
func fixedTime() time.Time {
	return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
}

func newService(t *testing.T, now time.Time) (*dashboard.Service, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc, err := dashboard.New(dashboard.Deps{Store: st}, dashboard.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("make the service: %v", err)
	}
	return svc, st
}

// seedProject adds a project and a board, so cards can be added to it.
func seedProject(t *testing.T, st *store.Store, id, name string) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		now := fixedTime().UnixMilli()
		if err := q.CreateProject(ctx, db.CreateProjectParams{
			ID: id, Name: name, RepoPath: "/code/" + id, DefaultBranch: "main",
			PackagesJSON: "[]", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return q.CreateBoard(ctx, db.CreateBoardParams{
			ID: "board-" + id, ProjectID: id, ColumnsJSON: `["backlog","working","needs","done"]`,
		})
	})
	if err != nil {
		t.Fatalf("seed the project %s: %v", id, err)
	}
}

// cardSeed is one card a test wants in the store, so seedCard stays a short argument list.
type cardSeed struct {
	projectID string
	id        string
	number    int
	state     protocol.CardState
	// mutate changes fields the plain insert does not carry, such as the needs reason or the
	// update time. It may be nil.
	mutate func(*db.Card)
}

// seedCard adds a card, with the fields the tests care about.
func seedCard(t *testing.T, st *store.Store, in cardSeed) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		now := fixedTime().UnixMilli()
		row := db.Card{
			ID: in.id, ProjectID: in.projectID, Number: int64(in.number), BoardID: "board-" + in.projectID,
			Title: "Card " + in.id, State: string(in.state), AgentKind: "claude",
			PermissionMode: "auto-edits", CreatedAt: now, UpdatedAt: now,
		}
		if in.mutate != nil {
			in.mutate(&row)
		}
		if err := q.CreateCard(ctx, db.CreateCardParams{
			ID: row.ID, ProjectID: row.ProjectID, Number: row.Number, BoardID: row.BoardID,
			Title: row.Title, Body: row.Body, State: row.State, AgentKind: row.AgentKind,
			Model: row.Model, Thinking: row.Thinking, PermissionMode: row.PermissionMode,
			CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}); err != nil {
			return err
		}
		return updateFields(ctx, q, row)
	})
	if err != nil {
		t.Fatalf("seed the card %s: %v", in.id, err)
	}
}

// updateFields writes the columns the plain insert does not carry.
func updateFields(ctx context.Context, q *db.Queries, row db.Card) error {
	_, err := q.UpdateCardFields(ctx, db.UpdateCardFieldsParams{
		Number: row.Number, State: row.State, Title: row.Title, Body: row.Body, AgentKind: row.AgentKind,
		Model: row.Model, Thinking: row.Thinking, PermissionMode: row.PermissionMode,
		Role: row.Role, Package: row.Package, PlannedStart: row.PlannedStart, PlannedEnd: row.PlannedEnd,
		Due: row.Due, ActualStart: row.ActualStart, ActualEnd: row.ActualEnd,
		PullRequestNumber: row.PullRequestNumber, PullRequestUrl: row.PullRequestUrl,
		CiState: row.CiState, ContextUsed: row.ContextUsed,
		NeedsReasonKind: row.NeedsReasonKind, NeedsReasonText: row.NeedsReasonText,
		NeedsSince: row.NeedsSince, DoingNow: row.DoingNow, Paused: row.Paused,
		Pinned: row.Pinned, UpdatedAt: row.UpdatedAt, ID: row.ID,
	})
	return err
}

// seedSession gives a card a session in a state, so Home's awake list can be tested.
func seedSession(t *testing.T, st *store.Store, cardID string, state protocol.SessionState, startedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		return q.CreateCardSession(ctx, db.CreateCardSessionParams{
			ID: "sess-" + cardID, CardID: cardID, AgentKind: "claude", State: string(state),
			LastActiveAt: startedAt.UnixMilli(), CreatedAt: startedAt.UnixMilli(), UpdatedAt: startedAt.UnixMilli(),
		})
	})
	if err != nil {
		t.Fatalf("seed the session of %s: %v", cardID, err)
	}
}

// An empty Home is empty lists and zero tiles, never null lists.
func TestHomeWithNothing(t *testing.T) {
	svc, _ := newService(t, fixedTime())
	got, err := svc.Home(context.Background(), dashboard.RangeWeek)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if got.Needs == nil || got.Awake == nil || len(got.Needs) != 0 || len(got.Awake) != 0 {
		t.Errorf("Home = %+v, want empty lists rather than null", got)
	}
	if got.Tiles != (protocol.HomeTiles{}) {
		t.Errorf("tiles = %+v, want zero", got.Tiles)
	}
	if got.Stats == nil || len(got.Stats.Days) != dashboard.RangeWeek || len(got.Stats.Projects) != 0 {
		t.Errorf("stats = %+v, want one zeroed row per day of the range", got.Stats)
	}
	if got.ServerTime.Time().UTC() != fixedTime() {
		t.Errorf("serverTime = %v, want the clock's time", got.ServerTime.Time())
	}
}

// The needs list carries the project's name, the reason, and how long the card has waited, and the
// tiles count it.
func TestHomeListsTheCardsThatNeedSomeone(t *testing.T) {
	waiting := fixedTime().Add(-2 * time.Hour)
	svc, st := newService(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	seedProject(t, st, "web", "web-dashboard")
	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28A", number: 41, state: protocol.CardStateNeeds, mutate: func(row *db.Card) {
		row.Title = "Fix token refresh on login"
		row.Role = "Implementer"
		row.NeedsReasonKind = string(protocol.NeedsReasonKindPlanReady)
		row.NeedsReasonText = "The plan is ready for review."
		row.NeedsSince = &[]int64{waiting.UnixMilli()}[0]
	}})
	seedCard(t, st, cardSeed{projectID: "web", id: "01M3C107JB041061050R3GG28B", number: 12, state: protocol.CardStateWorking})

	got, err := svc.Home(context.Background(), dashboard.RangeWeek)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if len(got.Needs) != 1 {
		t.Fatalf("needs = %+v, want one card", got.Needs)
	}
	card := got.Needs[0]
	if card.Key != "api#41" || card.Number != 41 || card.ProjectName != "api-gateway" ||
		card.Title != "Fix token refresh on login" || card.Role != "Implementer" {
		t.Errorf("needs card = %+v", card)
	}
	if card.Reason.Kind != protocol.NeedsReasonKindPlanReady || card.Reason.Text != "The plan is ready for review." {
		t.Errorf("reason = %+v", card.Reason)
	}
	if card.WaitingSince == nil || card.WaitingSince.Time().UTC() != waiting {
		t.Errorf("waitingSince = %v, want %v", card.WaitingSince, waiting)
	}
	if got.Tiles.Needs != 1 || got.Tiles.Working != 0 {
		t.Errorf("tiles = %+v, want one needing and none working (the card has no session)", got.Tiles)
	}
}

// The awake list is the cards with a session that is awake, working, or waking, and it is ordered
// by how long the session has run.
func TestHomeListsTheAgentsThatAreAwake(t *testing.T) {
	older := fixedTime().Add(-3 * time.Hour)
	newer := fixedTime().Add(-1 * time.Hour)
	svc, st := newService(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28A", number: 41, state: protocol.CardStateWorking, mutate: func(row *db.Card) {
		row.DoingNow = "Writing the handler"
		row.ContextUsed = 42
		row.Pinned = 1
	}})
	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28B", number: 42, state: protocol.CardStateWorking})
	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28C", number: 43, state: protocol.CardStateWorking})
	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28D", number: 44, state: protocol.CardStateBacklog})
	seedSession(t, st, "01M3C107JB041061050R3GG28A", protocol.SessionStateWorking, older)
	seedSession(t, st, "01M3C107JB041061050R3GG28B", protocol.SessionStateAwake, newer)
	// A card whose session stopped is not awake, and neither is one that never had a session.
	seedSession(t, st, "01M3C107JB041061050R3GG28C", protocol.SessionStateStopped, newer)

	got, err := svc.Home(context.Background(), dashboard.RangeWeek)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if len(got.Awake) != 2 {
		t.Fatalf("awake = %+v, want two cards", got.Awake)
	}
	first := got.Awake[0]
	if first.Key != "api#41" || first.Session != protocol.SessionStateWorking || first.State != protocol.CardStateWorking {
		t.Errorf("first awake card = %+v", first)
	}
	if first.DoingNow != "Writing the handler" || first.ContextUsed != 42 || !first.Pinned {
		t.Errorf("the awake card lost its fields: %+v", first)
	}
	if first.AwakeSince == nil || first.AwakeSince.Time().UTC() != older {
		t.Errorf("awakeSince = %v, want %v", first.AwakeSince, older)
	}
	if got.Awake[1].Key != "api#42" {
		t.Errorf("the list is not oldest first: %+v", got.Awake)
	}
	if got.Tiles.Working != 2 {
		t.Errorf("working tile = %d, want 2", got.Tiles.Working)
	}
}

// "Merged today" counts the cards that reached Done since the clock's own midnight, whatever the
// date in UTC is.
func TestHomeCountsTheCardsFinishedToday(t *testing.T) {
	zone := time.FixedZone("test", -5*60*60)
	now := time.Date(2026, 9, 30, 1, 0, 0, 0, zone) // still 2026-09-29 in UTC
	svc, st := newService(t, now)
	seedProject(t, st, "api", "api-gateway")

	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28A", number: 41, state: protocol.CardStateDone, mutate: func(row *db.Card) {
		// Two hours after this day's midnight, so it counts.
		row.UpdatedAt = time.Date(2026, 9, 30, 2, 0, 0, 0, zone).UnixMilli()
	}})
	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28B", number: 42, state: protocol.CardStateDone, mutate: func(row *db.Card) {
		// Just before this day's midnight, so it does not.
		row.UpdatedAt = time.Date(2026, 9, 29, 23, 0, 0, 0, zone).UnixMilli()
	}})
	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28C", number: 43, state: protocol.CardStateWorking, mutate: func(row *db.Card) {
		row.UpdatedAt = time.Date(2026, 9, 30, 3, 0, 0, 0, zone).UnixMilli()
	}})

	got, err := svc.Home(context.Background(), dashboard.RangeWeek)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if got.Tiles.MergedToday != 1 {
		t.Errorf("merged today = %d, want 1 (only the card that reached done after this day's midnight)", got.Tiles.MergedToday)
	}
}

// A card that waits on someone with no reason recorded still appears: the list is about the state,
// not the reason.
func TestHomeShowsACardWaitingWithNoReason(t *testing.T) {
	svc, st := newService(t, fixedTime())
	seedProject(t, st, "api", "api-gateway")
	seedCard(t, st, cardSeed{projectID: "api", id: "01M3C107JB041061050R3GG28A", number: 41, state: protocol.CardStateNeeds})

	got, err := svc.Home(context.Background(), dashboard.RangeWeek)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if len(got.Needs) != 1 {
		t.Fatalf("needs = %+v", got.Needs)
	}
	if got.Needs[0].Reason.Kind != "" || got.Needs[0].WaitingSince != nil {
		t.Errorf("a card with no reason should carry none: %+v", got.Needs[0])
	}
}

// A service with no store cannot be built, because every answer comes from one.
func TestNewNeedsAStore(t *testing.T) {
	if _, err := dashboard.New(dashboard.Deps{}); err == nil {
		t.Error("New without a store succeeded")
	}
}
