package connectiontest

// This package is the machinery every connection test shares (docs/architecture.md section 18,
// inventory B4.6/B4.8, build-plan task 4.9): a thing to test, a time limit on it, a cooldown per
// connection, and the saving of the result in the `integrations` table. A model provider's own test
// is the first of them (internal/providers/test.go); GitHub, Trello, Calendar, Gmail, Telegram,
// Discord, and MCP servers are later phases' and plug in here rather than writing their own
// cooldown. Section 18 gives them all the same mechanics on purpose.
//
// It is deliberately thin, because a connection test is not a health check: one call is made, its
// answer is turned into checks by the Tester, and the checks are saved whole. What lives here is the
// part that must not be got wrong twice - a test that respects the service's rate limits (a
// cooldown), a test that cannot hang (a time limit), and a result that survives a restart (a row).
//
// Nothing in this package reaches a service. The Tester is handed in, and every test in this package
// hands in one that answers from memory.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// KindProvider is the kind of a model provider's connection. It is the id a row in the
	// `integrations` table carries, and the id a provider's own test is filed under
	// (internal/providers/test.go). Later phases add their own.
	KindProvider = "provider"
	// KindGitHub is the kind of the GitHub App connection (B6.1, B6.7). Its row's id is also
	// "github", so the connection's id and its kind are the same word, which is true of the
	// integrations whose id a person never sees doubled.
	KindGitHub = "github"
	// KindObsidian is the kind of the Obsidian vault connection (B7.4, build-plan 7.7). It is the
	// one connection Marshal owns rather than a person setting it up: the vault is a folder in
	// Marshal's own data directory, so its test asks the file system rather than a service, and its
	// row reads what Marshal can see for itself.
	KindObsidian = "obsidian"
)

// Cooldown and time limit defaults (docs/architecture.md section 18: "Each test has a time limit and
// a cooldown per connection", and ui-rules.md's "the button waits a few seconds before it can run
// again, to respect the service's rate limits"). Both are overridable so a test can drive them.
const (
	// DefaultCooldown is how long a connection waits before it can be tested again. "A few seconds":
	// long enough that a person cannot hammer a provider's endpoint by pressing Test, short enough
	// that a first test that failed for a passing reason can be tried again without waiting.
	DefaultCooldown = 5 * time.Second
	// DefaultTimeLimit is how long one test may take before it is given up on. A connection test is
	// a tiny request; a provider that has not answered in twenty seconds is not about to, and the
	// person should be told rather than watching a spinner.
	DefaultTimeLimit = 20 * time.Second
)

// Tester is one connection's test. Its error is the daemon's own failure - Marshal could not run the
// test at all - and its TestResult is the answer, which says passed, partly working, or failed. A
// failed test is a result and not an error: "the key is wrong" is what the person asked to find out.
type Tester interface {
	Test(ctx context.Context) (protocol.TestResult, error)
}

// TesterFunc adapts a function to a Tester, so a caller can hand one in without a type of its own.
type TesterFunc func(ctx context.Context) (protocol.TestResult, error)

// Test runs the function.
func (f TesterFunc) Test(ctx context.Context) (protocol.TestResult, error) { return f(ctx) }

// Runner runs connection tests over the daemon's store. One is built when the daemon starts and it is
// safe for concurrent use: two connections can be tested at once.
type Runner struct {
	store    *store.Store
	now      func() time.Time
	cooldown time.Duration
	limit    time.Duration
	log      *slog.Logger
}

// Options tunes a Runner. Every field may be left out.
type Options struct {
	// Now is the clock. Nil means the real one.
	Now func() time.Time
	// Cooldown is how long a connection waits between tests. Below one means DefaultCooldown.
	Cooldown time.Duration
	// TimeLimit is how long one test may take. Below one means DefaultTimeLimit.
	TimeLimit time.Duration
	// Logger records what a test did. Nil means nothing is logged.
	Logger *slog.Logger
}

// New returns a Runner over st. The store is required: a test whose result cannot be saved is not a
// connection test, because the whole point of saving it is that the next screen, and the next test's
// cooldown, read it back.
func New(st *store.Store, opts Options) *Runner {
	r := &Runner{
		store:    st,
		now:      opts.Now,
		cooldown: opts.Cooldown,
		limit:    opts.TimeLimit,
		log:      opts.Logger,
	}
	if r.now == nil {
		r.now = time.Now
	}
	if r.cooldown <= 0 {
		r.cooldown = DefaultCooldown
	}
	if r.limit <= 0 {
		r.limit = DefaultTimeLimit
	}
	return r
}

// Run tests one connection and saves the result, and answers what to show the person. Within the
// cooldown it refuses, with how long to wait, rather than calling the service again: the cooldown is
// there to respect the service's rate limits, and a refusal is the only honest way to keep it once a
// person has found the Test button.
//
// The test runs under a time limit of its own, so a service that never answers cannot hold a request
// open; the timeout is reported by the Tester as the failed check it is (providers.testFailure), not
// as an error here.
func (r *Runner) Run(ctx context.Context, kind, id string, tester Tester) (protocol.TestResult, error) {
	return r.run(ctx, kind, id, tester, true)
}

// RunAfterConnect tests a connection that has just been added or changed, and is the call a route
// makes once a person saves a key (docs/architecture.md section 18: "a test runs automatically right
// after a connection is added"). It is Run without the cooldown, because the connection it is
// looking at has just changed: the old result would be about a key that is no longer stored, and the
// new one is exactly the thing worth knowing.
func (r *Runner) RunAfterConnect(ctx context.Context, kind, id string, tester Tester) (protocol.TestResult, error) {
	return r.run(ctx, kind, id, tester, false)
}

// Last answers the result of the last test of one connection, and whether there is one. A connection
// that has never been tested is not an error: it is the ordinary state of a connection nobody has
// saved or tested yet, and it is what a screen shows as "not tested yet" (the result is stored, not
// rebuilt, so this is also what survives a restart).
func (r *Runner) Last(ctx context.Context, id string) (protocol.TestResult, bool, error) {
	if err := r.ready(); err != nil {
		return protocol.TestResult{}, false, err
	}
	row, err := r.store.Queries().GetIntegration(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.TestResult{}, false, nil
	}
	if err != nil {
		return protocol.TestResult{}, false, fmt.Errorf("read the last test of %q: %w", id, err)
	}
	if row.LastTestResultJSON == "" {
		return protocol.TestResult{}, false, nil
	}
	var out protocol.TestResult
	if err := json.Unmarshal([]byte(row.LastTestResultJSON), &out); err != nil {
		// A row Marshal cannot read is not a result: it is a row written by a version that stored
		// something else, and the honest answer to "what did the last test say" is then nothing.
		if r.log != nil {
			r.log.Warn("a saved connection test could not be read",
				"connection", id, "err", err)
		}
		return protocol.TestResult{}, false, nil
	}
	if out.ConnectionID == "" {
		out.ConnectionID = row.ID
	}
	return out, true, nil
}

// run is Run and RunAfterConnect's own body.
func (r *Runner) run(ctx context.Context, kind, id string, tester Tester, cooldown bool) (protocol.TestResult, error) {
	if err := r.ready(); err != nil {
		return protocol.TestResult{}, err
	}
	if tester == nil {
		return protocol.TestResult{}, errors.New("a connection test needs something to run")
	}
	if kind == "" {
		return protocol.TestResult{}, protocol.InvalidArgument("a connection test needs a kind")
	}
	if id == "" {
		return protocol.TestResult{}, protocol.InvalidArgument("a connection test needs a connection id")
	}
	if cooldown {
		wait, err := r.wait(ctx, id)
		if err != nil {
			return protocol.TestResult{}, err
		}
		if wait > 0 {
			seconds := int64(math.Ceil(wait.Seconds()))
			return protocol.TestResult{}, protocol.
				Conflict(fmt.Sprintf("This connection was tested a moment ago. Try again in %d seconds.", seconds)).
				With("retryAfterMs", strconv.FormatInt(wait.Milliseconds(), 10))
		}
	}
	limited, cancel := context.WithTimeout(ctx, r.limit)
	defer cancel()
	result, err := tester.Test(limited)
	if err != nil {
		return protocol.TestResult{}, err
	}
	if result.ConnectionID == "" {
		result.ConnectionID = id
	}
	if result.Checks == nil {
		result.Checks = []protocol.TestCheck{}
	}
	// The result is saved on the request's own context and not the limited one: the test has
	// finished, and what is being saved is an answer Marshal already has in hand.
	if err := r.save(ctx, kind, id, result); err != nil {
		return result, err
	}
	if r.log != nil {
		r.log.Info("tested a connection", "connection", id, "kind", kind, "ok", result.OK)
	}
	return result, nil
}

// wait answers how long the connection must wait before it may be tested again, and zero when it may
// be tested now. A connection with no row, or a row with no test time, has never been tested.
func (r *Runner) wait(ctx context.Context, id string) (time.Duration, error) {
	row, err := r.store.Queries().GetIntegration(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the last test of %q: %w", id, err)
	}
	if row.LastTestAt == 0 {
		return 0, nil
	}
	elapsed := r.now().Sub(time.UnixMilli(row.LastTestAt))
	if elapsed >= r.cooldown {
		return 0, nil
	}
	return r.cooldown - elapsed, nil
}

// save writes one result, making the row the first time a connection is tested. Only the two test
// columns are touched on a row that already exists (queries/integrations.sql), so a test never
// clobbers a connection's settings or its keychain reference.
func (r *Runner) save(ctx context.Context, kind, id string, result protocol.TestResult) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("save the test of %q: %w", id, err)
	}
	at := result.RanAt.Time()
	if at.IsZero() {
		at = r.now()
	}
	err = r.store.Write(ctx, func(q *db.Queries) error {
		return q.SetIntegrationTest(ctx, db.SetIntegrationTestParams{
			ID:                 id,
			Kind:               kind,
			LastTestAt:         at.UnixMilli(),
			LastTestResultJSON: string(encoded),
		})
	})
	if err != nil {
		return fmt.Errorf("save the test of %q: %w", id, err)
	}
	return nil
}

// ready fails loudly when the Runner has no store, rather than answering "never tested" for every
// connection: a daemon wired without one would tell a person their key had not been tested when in
// fact nothing was saved, and the cooldown would not exist either.
func (r *Runner) ready() error {
	if r.store == nil {
		return errors.New("connection tests need the daemon's store, and this one has none")
	}
	return nil
}
