package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The two scopes the tests use: the whole install, and one project. The project id is a slug, which
// is what a project's id really is (the fixture's are "api", "web-dashboard", "mobile"), not an
// opaque id - the reason checkLimitKey does not look at a scope's shape.
const limitsProject = "web-dashboard"

// limitsFor builds a limits service over a fresh store.
func limitsFor(t *testing.T) (*Limits, *store.Store) {
	t.Helper()
	st := openTestStore(t)
	return NewLimits(st), st
}

// wantLimits compares a whole answer with the pairs a test expects, in order.
func wantLimits(t *testing.T, got protocol.LimitList, want ...protocol.Limit) {
	t.Helper()
	if len(got.Limits) != len(want) {
		t.Fatalf("limits = %v, want %v", got.Limits, want)
	}
	for i := range want {
		if got.Limits[i] != want[i] {
			t.Errorf("limits[%d] = %v, want %v", i, got.Limits[i], want[i])
		}
	}
}

// TestLimitsAreEmptyOnAFreshInstall is the shipped state: nothing is set up, so nothing is limited,
// and the answer is an empty list rather than an error or a nil.
func TestLimitsAreEmptyOnAFreshInstall(t *testing.T) {
	limits, _ := limitsFor(t)
	got, err := limits.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Limits == nil {
		t.Error("a fresh install's limits are nil, want an empty list so the JSON is []")
	}
	wantLimits(t, got)
}

// TestSetStoresAndListReadsItBack is the round trip a saving screen makes.
func TestSetStoresAndListReadsItBack(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	// Set answers with the whole list, so a screen redraws its form from the one answer.
	got, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostDay, 25_000_000)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	wantLimits(t, got,
		protocol.Limit{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindCostDay, Value: 25_000_000})

	got, err = limits.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantLimits(t, got,
		protocol.Limit{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindCostDay, Value: 25_000_000})
}

// TestSetReplacesTheSameScopeAndKind is "saving twice leaves one ceiling", which is what editing a
// number does: the table's (scope, kind) is the primary key, so the second save must not add a row.
func TestSetReplacesTheSameScopeAndKind(t *testing.T) {
	limits, st := limitsFor(t)
	ctx := context.Background()

	if _, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindAwake, 8); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindAwake, 18)
	if err != nil {
		t.Fatalf("Set again: %v", err)
	}
	wantLimits(t, got,
		protocol.Limit{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindAwake, Value: 18})

	rows, err := st.Queries().ListLimits(ctx)
	if err != nil {
		t.Fatalf("read the rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("the table holds %d rows, want 1: saving twice must replace, not add", len(rows))
	}
}

// TestTheThreeKindsAndASecondScopeKeepTheirOwnCeilings is the whole shape of a populated form: one
// scope holds its three ceilings side by side, and a second scope's are its own.
func TestTheThreeKindsAndASecondScopeKeepTheirOwnCeilings(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	for _, set := range []struct {
		scope string
		kind  protocol.LimitKind
		value int64
	}{
		{protocol.LimitScopeGlobal, protocol.LimitKindCostDay, 25_000_000},
		{protocol.LimitScopeGlobal, protocol.LimitKindCostMonth, 400_000_000},
		{protocol.LimitScopeGlobal, protocol.LimitKindAwake, 18},
		{limitsProject, protocol.LimitKindCostDay, 8_000_000},
		{limitsProject, protocol.LimitKindAwake, 6},
	} {
		if _, err := limits.Set(ctx, set.scope, set.kind, set.value); err != nil {
			t.Fatalf("Set(%s, %s): %v", set.scope, set.kind, err)
		}
	}

	got, err := limits.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Global first, its kinds in the form's order, then the project's, with the ceiling it has not
	// set left out rather than shown as zero.
	wantLimits(t, got,
		protocol.Limit{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindCostDay, Value: 25_000_000},
		protocol.Limit{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindCostMonth, Value: 400_000_000},
		protocol.Limit{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindAwake, Value: 18},
		protocol.Limit{Scope: limitsProject, Kind: protocol.LimitKindCostDay, Value: 8_000_000},
		protocol.Limit{Scope: limitsProject, Kind: protocol.LimitKindAwake, Value: 6},
	)
}

// TestTheGlobalScopeComesBeforeTheProjects is the ordering rule on its own, because it is not the
// order the query gives: a project id starts with a digit, so it sorts before "global".
func TestTheGlobalScopeComesBeforeTheProjects(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	// A project id that sorts before "global" (it starts with a digit) and one that sorts after it,
	// so a reader that kept the query's order would put the first one ahead of the global ceiling.
	for _, scope := range []string{"01JD7Q4M2X8K9V0P5T3RB6NHAE", "web-dashboard"} {
		if _, err := limits.Set(ctx, scope, protocol.LimitKindCostDay, 5_000_000); err != nil {
			t.Fatalf("Set(%s): %v", scope, err)
		}
	}
	got, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostDay, 25_000_000)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	var scopes []string
	for _, limit := range got.Limits {
		scopes = append(scopes, limit.Scope)
	}
	want := []string{protocol.LimitScopeGlobal, "01JD7Q4M2X8K9V0P5T3RB6NHAE", "web-dashboard"}
	for i := range want {
		if scopes[i] != want[i] {
			t.Fatalf("scopes = %v, want %v (global first, then the projects by id)", scopes, want)
		}
	}
}

// TestTheKindsComeInTheFormsOrder pins the second half of the ordering rule: LimitKindValues is the
// order the settings form shows, and the answer follows it rather than the alphabet.
func TestTheKindsComeInTheFormsOrder(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	// Set them in the reverse of the form's order, so a passing test cannot be the insert order.
	for _, kind := range []protocol.LimitKind{
		protocol.LimitKindAwake, protocol.LimitKindCostMonth, protocol.LimitKindCostDay,
	} {
		if _, err := limits.Set(ctx, protocol.LimitScopeGlobal, kind, 1); err != nil {
			t.Fatalf("Set(%s): %v", kind, err)
		}
	}

	got, err := limits.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var kinds []protocol.LimitKind
	for _, limit := range got.Limits {
		kinds = append(kinds, limit.Kind)
	}
	want := protocol.LimitKindValues()
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
}

// TestDeleteRemovesOneCeilingAndLeavesTheRest is clearing one field: the other two and the other
// scope are untouched.
func TestDeleteRemovesOneCeilingAndLeavesTheRest(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	if _, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostDay, 25_000_000); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindAwake, 18); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := limits.Set(ctx, limitsProject, protocol.LimitKindCostDay, 8_000_000); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := limits.Delete(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostDay)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	wantLimits(t, got,
		protocol.Limit{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindAwake, Value: 18},
		protocol.Limit{Scope: limitsProject, Kind: protocol.LimitKindCostDay, Value: 8_000_000},
	)
}

// TestDeleteOfACeilingThatIsNotSetIsNotAnError is the "clear a field twice" rule: the answer is the
// same list, and nothing is refused.
func TestDeleteOfACeilingThatIsNotSetIsNotAnError(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	got, err := limits.Delete(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostMonth)
	if err != nil {
		t.Fatalf("Delete of a ceiling that is not set: %v", err)
	}
	wantLimits(t, got)

	// And it is still not an error the second time, after the first delete of a real one.
	if _, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostMonth, 400_000_000); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := limits.Delete(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostMonth); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = limits.Delete(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostMonth)
	if err != nil {
		t.Fatalf("Delete again: %v", err)
	}
	wantLimits(t, got)
}

// TestALimitOfZeroOrLessIsRefused is the one value rule: the form's own sentence, and nothing
// written.
func TestALimitOfZeroOrLessIsRefused(t *testing.T) {
	limits, st := limitsFor(t)
	ctx := context.Background()

	for _, value := range []int64{0, -1, -25_000_000} {
		_, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostDay, value)
		if err == nil {
			t.Fatalf("Set(%d) was allowed, want a refusal", value)
		}
		var problem *protocol.Error
		if !errors.As(err, &problem) {
			t.Fatalf("Set(%d): %v, want a protocol error", value, err)
		}
		if problem.Code != protocol.ErrorCodeInvalidArgument {
			t.Errorf("Set(%d) code = %s, want %s", value, problem.Code, protocol.ErrorCodeInvalidArgument)
		}
		if problem.Message != "Enter a limit above zero." {
			t.Errorf("Set(%d) message = %q, want the form's own sentence", value, problem.Message)
		}
	}

	rows, err := st.Queries().ListLimits(ctx)
	if err != nil {
		t.Fatalf("read the rows: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("a refused limit left %d rows, want none", len(rows))
	}
}

// TestAnUnknownKindIsRefused keeps a word the constants do not name out of the table: the kind
// decides what a value's unit is, so a kind nothing reads would be a number with no meaning.
func TestAnUnknownKindIsRefused(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	for _, kind := range []protocol.LimitKind{"", "spend", "cost", "awake-cards"} {
		if _, err := limits.Set(ctx, protocol.LimitScopeGlobal, kind, 1); err == nil {
			t.Errorf("Set(%q) was allowed, want a refusal", kind)
		}
		if _, err := limits.Delete(ctx, protocol.LimitScopeGlobal, kind); err == nil {
			t.Errorf("Delete(%q) was allowed, want a refusal", kind)
		}
	}
}

// TestAScopeWithoutAProjectIsRefused is the only scope rule: a ceiling belongs to something.
func TestAScopeWithoutAProjectIsRefused(t *testing.T) {
	limits, _ := limitsFor(t)

	_, err := limits.Set(context.Background(), "", protocol.LimitKindCostDay, 1)
	if err == nil {
		t.Fatal("Set with no scope was allowed, want a refusal")
	}
	var problem *protocol.Error
	if !errors.As(err, &problem) || problem.Code != protocol.ErrorCodeInvalidArgument {
		t.Fatalf("Set with no scope: %v, want invalid_argument", err)
	}
}

// TestAProjectScopeIsAnySlug is the reason the scope's shape is not checked: a project's id is its
// own slug, not an opaque id, so a ceiling for "web-dashboard" must be stored like any other.
func TestAProjectScopeIsAnySlug(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	got, err := limits.Set(ctx, limitsProject, protocol.LimitKindAwake, 6)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(got.Limits) != 1 || got.Limits[0].Scope != limitsProject {
		t.Fatalf("limits = %v, want one for %s", got.Limits, limitsProject)
	}
}

// TestALimitSurvivesAReopenedStore is the settings-persist rule: a limit is in the database, not in
// memory, so it is still there after a restart.
func TestALimitSurvivesAReopenedStore(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/marshal.db"

	st, err := store.Open(ctx, path, store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	if _, err := NewLimits(st).Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindAwake, 18); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}

	again, err := store.Open(ctx, path, store.WithLogger(nil))
	if err != nil {
		t.Fatalf("reopen the store: %v", err)
	}
	t.Cleanup(func() { _ = again.Close() })
	got, err := NewLimits(again).List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantLimits(t, got,
		protocol.Limit{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindAwake, Value: 18})
}

// TestLimitsWithNoStoreFailsLoudly is the wiring rule: a daemon built without a store must not answer
// "nothing is limited", which is what a person reads as "Marshal may spend anything".
func TestLimitsWithNoStoreFailsLoudly(t *testing.T) {
	var limits *Limits
	if _, err := limits.List(context.Background()); err == nil {
		t.Error("List with no store was allowed, want a failure")
	}
	if _, err := NewLimits(nil).Set(
		context.Background(), protocol.LimitScopeGlobal, protocol.LimitKindCostDay, 1,
	); err == nil {
		t.Error("Set with no store was allowed, want a failure")
	}
}

// TestARowWithAnUnknownKindIsKeptLastNotDropped covers a row a different build wrote: the answer
// still carries it, at the end, rather than losing a ceiling a person set.
func TestARowWithAnUnknownKindIsKeptLastNotDropped(t *testing.T) {
	limits, st := limitsFor(t)
	ctx := context.Background()

	if _, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindCostDay, 25_000_000); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Written straight into the table, as a build with another kind vocabulary would.
	if err := st.Write(ctx, func(q *db.Queries) error {
		return q.SetLimit(ctx, db.SetLimitParams{
			Scope: protocol.LimitScopeGlobal, Kind: "cost", Value: 1,
		})
	}); err != nil {
		t.Fatalf("write the foreign row: %v", err)
	}

	got, err := limits.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Limits) != 2 {
		t.Fatalf("limits = %v, want the foreign row kept", got.Limits)
	}
	if got.Limits[0].Kind != protocol.LimitKindCostDay || got.Limits[1].Kind != protocol.LimitKind("cost") {
		t.Errorf("limits = %v, want the named kind first and the unknown one last", got.Limits)
	}
}

// TestAwakeLimitPrefersTheProjectsOwnCeiling is the rule every limit follows, read through the one
// question the session manager asks when a card must wake (B5.6): a project's own ceiling wins over
// the whole install's, and a project that sets none follows the global one.
func TestAwakeLimitPrefersTheProjectsOwnCeiling(t *testing.T) {
	limits, _ := limitsFor(t)
	ctx := context.Background()

	// Nothing is set on a fresh install, which is the shipped state: no ceiling to enforce.
	if value, set, err := limits.AwakeLimit(ctx, limitsProject); err != nil || set {
		t.Fatalf("AwakeLimit with nothing set = (%d, %v, %v), want (0, false, nil)", value, set, err)
	}

	if _, err := limits.Set(ctx, protocol.LimitScopeGlobal, protocol.LimitKindAwake, 4); err != nil {
		t.Fatalf("Set the global ceiling: %v", err)
	}
	if value, set, err := limits.AwakeLimit(ctx, limitsProject); err != nil || !set || value != 4 {
		t.Fatalf("AwakeLimit under the global ceiling = (%d, %v, %v), want (4, true, nil)", value, set, err)
	}

	if _, err := limits.Set(ctx, limitsProject, protocol.LimitKindAwake, 2); err != nil {
		t.Fatalf("Set the project ceiling: %v", err)
	}
	if value, set, err := limits.AwakeLimit(ctx, limitsProject); err != nil || !set || value != 2 {
		t.Fatalf("AwakeLimit with a project ceiling = (%d, %v, %v), want (2, true, nil)", value, set, err)
	}

	// Another project has set none, so it still follows the global one.
	if value, set, err := limits.AwakeLimit(ctx, "api"); err != nil || !set || value != 4 {
		t.Fatalf("AwakeLimit for another project = (%d, %v, %v), want the global 4", value, set, err)
	}
}

// TestAwakeLimitWithNoStoreFailsLoudly proves a daemon wired without a store does not answer "no
// ceiling", which would be the one answer that must never be given by mistake.
func TestAwakeLimitWithNoStoreFailsLoudly(t *testing.T) {
	var limits *Limits
	if _, _, err := limits.AwakeLimit(context.Background(), limitsProject); err == nil {
		t.Error("AwakeLimit with no store was allowed, want a failure")
	}
	if _, _, err := NewLimits(nil).AwakeLimit(context.Background(), limitsProject); err == nil {
		t.Error("AwakeLimit on a service with no store was allowed, want a failure")
	}
}
