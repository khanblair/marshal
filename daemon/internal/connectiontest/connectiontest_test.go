package connectiontest

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// This file checks the machinery every connection test shares: the time limit, the cooldown, and the
// saved result. Nothing here reaches a service - every Tester answers from memory - and the store is a
// real database in a temporary directory, so the row that is saved is the row that would be saved.

var testNow = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

// openTestStore opens a real database in a temporary directory. It is a file rather than :memory: so
// that the migrations run exactly as they do in the daemon.
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

// clock is a clock a test moves by hand, so a cooldown can be waited out without waiting.
type clock struct{ at time.Time }

func (c *clock) now() time.Time          { return c.at }
func (c *clock) advance(d time.Duration) { c.at = c.at.Add(d) }

// passing is a Tester that succeeds without reaching anything. It counts how many times it was asked,
// which is how a test proves the cooldown stopped a second call.
type passing struct {
	calls int
	check string
}

func (p *passing) Test(context.Context) (protocol.TestResult, error) {
	p.calls++
	return protocol.NewTestResult("", []protocol.TestCheck{
		{Name: p.check, State: protocol.CheckStatePassed, Message: "it answered"},
	}, testNow), nil
}

// newRunner returns a runner over a fresh store with a clock a test controls.
func newRunner(t *testing.T) (*Runner, *store.Store, *clock) {
	t.Helper()
	st := openTestStore(t)
	at := &clock{at: testNow}
	return New(st, Options{Now: at.now, Cooldown: time.Minute, TimeLimit: 5 * time.Second}), st, at
}

// TestATestIsRunAndSaved is the whole of a first test: the connection is asked, the result is stamped
// with the connection's id, and it is in the table afterwards.
func TestATestIsRunAndSaved(t *testing.T) {
	r, st, _ := newRunner(t)
	tester := &passing{check: "API key"}
	got, err := r.Run(t.Context(), KindProvider, "anthropic", tester)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if tester.calls != 1 {
		t.Errorf("the connection was tested %d times, want once", tester.calls)
	}
	if got.ConnectionID != "anthropic" {
		t.Errorf("connection = %q, want the id it was run for", got.ConnectionID)
	}
	if !got.OK {
		t.Errorf("the result is not OK: %+v", got.Checks)
	}
	row, err := st.Queries().GetIntegration(t.Context(), "anthropic")
	if err != nil {
		t.Fatalf("read the row back: %v", err)
	}
	if row.Kind != KindProvider {
		t.Errorf("the row's kind = %q, want %q", row.Kind, KindProvider)
	}
	if row.LastTestAt != testNow.UnixMilli() {
		t.Errorf("the row is stamped %d, want %d", row.LastTestAt, testNow.UnixMilli())
	}
	var saved protocol.TestResult
	if err := json.Unmarshal([]byte(row.LastTestResultJSON), &saved); err != nil {
		t.Fatalf("the saved result is not JSON: %v", err)
	}
	if saved.ConnectionID != "anthropic" || !saved.OK {
		t.Errorf("the saved result = %+v, want the one that was answered", saved)
	}
}

// TestLastReadsTheSavedResultBack is what the next screen and the cooldown both read.
func TestLastReadsTheSavedResultBack(t *testing.T) {
	r, _, _ := newRunner(t)
	if _, ok, err := r.Last(t.Context(), "anthropic"); err != nil || ok {
		t.Errorf("a connection that has never been tested answered ok=%v err=%v, want neither", ok, err)
	}
	if _, err := r.Run(t.Context(), KindProvider, "anthropic", &passing{check: "API key"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, ok, err := r.Last(t.Context(), "anthropic")
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	if !ok {
		t.Fatal("a saved result was not read back")
	}
	if got.ConnectionID != "anthropic" || len(got.Checks) != 1 || got.Checks[0].Name != "API key" {
		t.Errorf("the read-back result = %+v", got)
	}
}

// TestTheCooldownStopsASecondTest is the rate-limit rule: a connection tested a moment ago is not
// tested again, and the answer says how long to wait rather than pretending nothing happened.
func TestTheCooldownStopsASecondTest(t *testing.T) {
	r, _, at := newRunner(t)
	first := &passing{check: "API key"}
	if _, err := r.Run(t.Context(), KindProvider, "anthropic", first); err != nil {
		t.Fatalf("the first test: %v", err)
	}
	second := &passing{check: "API key"}
	_, err := r.Run(t.Context(), KindProvider, "anthropic", second)
	if err == nil {
		t.Fatal("a second test inside the cooldown was run")
	}
	if code, ok := errorCode(err); !ok || code != protocol.ErrorCodeConflict {
		t.Fatalf("the refusal = %v, want a conflict", err)
	}
	if second.calls != 0 {
		t.Error("the connection was called again inside the cooldown")
	}
	at.advance(time.Minute)
	if _, err := r.Run(t.Context(), KindProvider, "anthropic", second); err != nil {
		t.Fatalf("the test after the cooldown: %v", err)
	}
	if second.calls != 1 {
		t.Error("the connection was not tested once the cooldown had passed")
	}
}

// TestASavedConnectionIsTestedAgainAtOnce is the save path: a key that has just changed must be
// checked now, whatever the cooldown says about the key that is gone.
func TestASavedConnectionIsTestedAgainAtOnce(t *testing.T) {
	r, _, _ := newRunner(t)
	if _, err := r.Run(t.Context(), KindProvider, "anthropic", &passing{check: "API key"}); err != nil {
		t.Fatalf("the first test: %v", err)
	}
	after := &passing{check: "API key"}
	if _, err := r.RunAfterConnect(t.Context(), KindProvider, "anthropic", after); err != nil {
		t.Fatalf("RunAfterConnect: %v", err)
	}
	if after.calls != 1 {
		t.Error("a connection that had just been saved was not tested again")
	}
}

// TestATestThatCannotBeRunSavesNothing: an error from the Tester is the daemon's own failure, and a
// row saying "tested" would be a lie about a connection nothing was learned about.
func TestATestThatCannotBeRunSavesNothing(t *testing.T) {
	r, _, _ := newRunner(t)
	broken := errors.New("the test itself broke")
	_, err := r.Run(t.Context(), KindProvider, "anthropic", TesterFunc(
		func(context.Context) (protocol.TestResult, error) { return protocol.TestResult{}, broken }))
	if !errors.Is(err, broken) {
		t.Fatalf("Run answered %v, want the Tester's own failure", err)
	}
	if _, ok, err := r.Last(t.Context(), "anthropic"); err != nil || ok {
		t.Errorf("a failed test left a result behind: ok=%v err=%v", ok, err)
	}
}

// TestTheTimeLimitIsPutOnTheTest: a service that never answers must not hold a request open, and the
// context the Tester is handed is what carries the limit.
func TestTheTimeLimitIsPutOnTheTest(t *testing.T) {
	st := openTestStore(t)
	r := New(st, Options{Now: func() time.Time { return testNow }, TimeLimit: time.Minute})
	var seen bool
	_, err := r.Run(t.Context(), KindProvider, "anthropic", TesterFunc(
		func(ctx context.Context) (protocol.TestResult, error) {
			deadline, ok := ctx.Deadline()
			seen = ok && time.Until(deadline) <= time.Minute
			return protocol.NewTestResult("", nil, testNow), nil
		}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !seen {
		t.Error("the test was not handed a deadline")
	}
}

// TestWhatIsSavedIsTheWholeResult: every check, its state, its message, and its fix survive the round
// trip, because a screen shows them and a restart must not lose them.
func TestWhatIsSavedIsTheWholeResult(t *testing.T) {
	r, _, _ := newRunner(t)
	want := []protocol.TestCheck{
		{Name: "API key", State: protocol.CheckStateFailed, Message: "Anthropic refused this key.",
			Fix: "Check the key and save it again."},
		{Name: "Rate limits", State: protocol.CheckStateWarning, Message: "Anthropic did not say."},
	}
	if _, err := r.Run(t.Context(), KindProvider, "anthropic", TesterFunc(
		func(context.Context) (protocol.TestResult, error) {
			return protocol.NewTestResult("anthropic", want, testNow), nil
		})); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, ok, err := r.Last(t.Context(), "anthropic")
	if err != nil || !ok {
		t.Fatalf("Last: %v (ok=%v)", err, ok)
	}
	if got.OK {
		t.Error("a result with a failed check reported itself as OK")
	}
	if len(got.Checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(got.Checks))
	}
	if got.Checks[0].State != protocol.CheckStateFailed || got.Checks[0].Fix != want[0].Fix {
		t.Errorf("the first check came back as %+v", got.Checks[0])
	}
	if got.Checks[1].State != protocol.CheckStateWarning {
		t.Errorf("the second check came back as %+v", got.Checks[1])
	}
	if failed, ok := got.FirstFailed(); !ok || failed.Name != "API key" {
		t.Errorf("the first failed check = %+v (ok=%v)", failed, ok)
	}
}

// TestARowMarshalCannotReadIsNotAResult: a stored result written by a version that stored something
// else is not an answer, and "nothing is known" is the honest answer to what the last test said.
func TestARowMarshalCannotReadIsNotAResult(t *testing.T) {
	r, st, _ := newRunner(t)
	err := st.Write(t.Context(), func(q *db.Queries) error {
		return q.SetIntegrationTest(t.Context(), db.SetIntegrationTestParams{
			ID: "anthropic", Kind: KindProvider, LastTestAt: testNow.UnixMilli(),
			LastTestResultJSON: "not json at all",
		})
	})
	if err != nil {
		t.Fatalf("write the row: %v", err)
	}
	if _, ok, err := r.Last(t.Context(), "anthropic"); err != nil || ok {
		t.Errorf("an unreadable row answered ok=%v err=%v, want neither", ok, err)
	}
}

// TestRunnerWithoutAStoreFailsLoudly: a daemon wired without one would tell a person their key had not
// been tested when in fact nothing was saved, and the cooldown would not exist either.
func TestRunnerWithoutAStoreFailsLoudly(t *testing.T) {
	r := New(nil, Options{})
	if _, err := r.Run(t.Context(), KindProvider, "anthropic", &passing{check: "x"}); err == nil {
		t.Error("a runner with no store ran a test")
	}
	if _, _, err := r.Last(t.Context(), "anthropic"); err == nil {
		t.Error("a runner with no store read a result")
	}
}

// TestATestNeedsAConnectionAndSomethingToRun: the two things a test cannot be without. Both are
// programming mistakes rather than user input, so both are refused before anything is done.
func TestATestNeedsAConnectionAndSomethingToRun(t *testing.T) {
	r, _, _ := newRunner(t)
	for _, tc := range []struct {
		name   string
		kind   string
		id     string
		tester Tester
	}{
		{"no id", KindProvider, "", &passing{}},
		{"no kind", "", "anthropic", &passing{}},
		{"nothing to run", KindProvider, "anthropic", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.Run(t.Context(), tc.kind, tc.id, tc.tester); err == nil {
				t.Error("a test that cannot be run was run")
			}
		})
	}
}

// TestTwoConnectionsCoolDownApart: the cooldown is per connection, so testing one does not stop the
// person testing another.
func TestTwoConnectionsCoolDownApart(t *testing.T) {
	r, _, _ := newRunner(t)
	if _, err := r.Run(t.Context(), KindProvider, "anthropic", &passing{check: "key"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := r.Run(t.Context(), KindProvider, "openai", &passing{check: "key"}); err != nil {
		t.Fatalf("the other connection could not be tested: %v", err)
	}
	rows, err := r.store.Queries().ListIntegrations(t.Context(), KindProvider)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("there are %d rows, want one per connection", len(rows))
	}
}

// errorCode is a protocol error's own code, and whether err is one.
func errorCode(err error) (protocol.ErrorCode, bool) {
	var perr *protocol.Error
	if errors.As(err, &perr) {
		return perr.Code, true
	}
	return "", false
}
