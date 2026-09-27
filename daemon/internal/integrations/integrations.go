// Package integrations owns the connections Marshal is set up with, apart from model providers:
// today GitHub, and in later phases Trello, a calendar, Gmail, Telegram, Discord, and an Obsidian
// vault (docs/architecture.md section 18, `docs/backend-checklist.md` B6.1 and B6.7).
//
// It is the second half of the shape Phase 4 built: internal/connectiontest owns the mechanics every
// connection test shares - the cooldown, the time limit, the saved result - and this package owns
// what is true of one kind of connection: where its settings and its secret live, what "connected"
// means for it, and what its own test asks. The GitHub App's own two halves - turning an App's key
// into a github.Client, and believing a delivery - live in internal/integrations/github; this
// package is what wires them to the store, the keychain, and a route.
//
// It holds the one thing that must not be spread around: how a connection's secret is read. Nothing
// outside this package and internal/providers reads the keychain for a connection.
package integrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/connectiontest"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// GitHubID is the GitHub App connection's own id, and the id its `integrations` row, its keychain
// entry, and its test result are filed under.
const GitHubID = "github"

// ObsidianID is the Obsidian vault connection's own id (B7.4, build-plan 7.7). It is the connection
// Marshal owns rather than one a person connects: the vault is a folder in Marshal's own data
// directory, so there is no setting and no secret, and its row says where the vault is.
const ObsidianID = "obsidian"

// KindGitHub is the kind the GitHub App connection's row and its test are filed under.
const KindGitHub = connectiontest.KindGitHub

// KindObsidian is the kind the Obsidian vault's row and its test are filed under.
const KindObsidian = connectiontest.KindObsidian

// Info is one connection Marshal can be set up with. The id is the row's id, and the kind is what
// its test is filed under.
type Info struct {
	// ID is the connection's own id, such as "github".
	ID string
	// Kind is the sort of connection it is, such as "github" or "calendar".
	Kind string
	// Wired reports whether this daemon can actually set the connection up and test it today. The
	// connections of later phases are listed so the screen shows every row, and read as "not
	// connected" until their own phase builds them. Nothing in the daemon pretends to test one.
	Wired bool
}

// known is every connection Marshal knows, in the order the settings screen shows them. The ids
// match the ones the mock store and the screen already use
// (apps/web/src/mock/seed/settings.ts), so a cutover does not renumber anything.
func known() []Info {
	return []Info{
		{ID: GitHubID, Kind: connectiontest.KindGitHub, Wired: true},
		{ID: "trello", Kind: "trello"},
		{ID: "gcal", Kind: "calendar"},
		{ID: "gmail", Kind: "gmail"},
		{ID: "telegram", Kind: "telegram"},
		{ID: "discord", Kind: "discord"},
		{ID: ObsidianID, Kind: KindObsidian, Wired: true},
	}
}

// Lookup finds a known connection by id.
func Lookup(id string) (Info, bool) {
	for _, info := range known() {
		if info.ID == id {
			return info, true
		}
	}
	return Info{}, false
}

// Options tunes a Service. Every field may be left out.
type Options struct {
	// Logger records a connection that could not be read, and a delivery the monitor could not
	// handle. Nil uses slog.Default().
	Logger *slog.Logger
	// Now is the clock. Nil uses time.Now.
	Now func() time.Time
	// VaultRoot is where the memory module keeps a person's vault
	// (`<data>/vault`, docs/architecture.md section 12). The Obsidian connection is a folder and not
	// a service, so its row and its test ask the file system here. An empty root is "Marshal does
	// not know where its vault is yet", which its row and its test say rather than guess.
	VaultRoot string
	// Tester overrides how one connection's test is run, replacing the real ones (test.go,
	// obsidian.go). Nil uses the real ones. It exists so a route test never dials GitHub.
	Tester func(ctx context.Context, info Info) (protocol.TestResult, error)
	// App overrides how the GitHub App client is built, so the connection test's own checks can be
	// driven against a fake server without a live App. Nil builds it from the stored connection,
	// which is what the daemon does. It is the same seam a provider service has for its client
	// factory: nothing but a test sets it.
	App func(ctx context.Context) (*githubapp.App, error)
}

// Service is the one place that knows which connections are set up, and the only reader of a
// connection's secret. It is built once, when the daemon starts, and is safe for concurrent use.
type Service struct {
	store *store.Store
	keys  security.Keychain
	log   *slog.Logger
	now   func() time.Time
	vault string
	test  func(ctx context.Context, info Info) (protocol.TestResult, error)
	appFn func(ctx context.Context) (*githubapp.App, error)

	// webhooks verifies and accepts GitHub's deliveries. It is built with this service, so the
	// secret it checks against is read from the keychain on every delivery and a secret saved
	// through Settings takes effect without a restart.
	webhooks *githubapp.Receiver

	mu      sync.Mutex
	monitor githubapp.Sink
	app     *githubapp.App
	built   bool

	// secretsVal is the connection's keychain entry, read once and kept until the connection
	// changes, so a delivery does not pay for a keychain read. secretsLoaded says whether it has
	// been read; the zero value is "not read yet".
	secretsVal    githubSecrets
	secretsLoaded bool
}

// New builds the GitHub connection service. The store and the keychain are required: a connection
// whose settings cannot be saved, or whose secret cannot be kept, is not a connection.
func New(st *store.Store, keys security.Keychain, opts Options) (*Service, error) {
	if st == nil {
		return nil, errors.New("integrations: a store is required")
	}
	if keys == nil {
		return nil, errors.New("integrations: a keychain is required")
	}
	s := &Service{
		store: st, keys: keys, log: opts.Logger, now: opts.Now, vault: opts.VaultRoot,
		test: opts.Tester, appFn: opts.App,
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.test == nil {
		s.test = s.testFor
	}
	if s.appFn == nil {
		s.appFn = s.App
	}
	verifier, err := githubapp.NewVerifierFrom(s.webhookSecret)
	if err != nil {
		return nil, err
	}
	receiver, err := githubapp.NewReceiver(verifier, s, s.log)
	if err != nil {
		return nil, err
	}
	s.webhooks = receiver
	return s, nil
}

// Receiver is the GitHub delivery receiver the webhook route hands deliveries to.
func (s *Service) Receiver() *githubapp.Receiver { return s.webhooks }

// SetMonitor attaches what verified deliveries are handed to - the CI monitor (slice 2). It is
// called while the daemon starts, before the route is served. Until it is called, a verified
// delivery is accepted and remembered by the receiver's own count and nothing else.
func (s *Service) SetMonitor(monitor githubapp.Sink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.monitor = monitor
	s.webhooks.SetSink(s)
}

// Delivery is the receiver's sink. It forwards a verified delivery to the CI monitor when one is
// attached; with no monitor, a verified delivery is simply accepted, which is the honest answer for
// an event no part of Marshal handles yet.
func (s *Service) Delivery(ctx context.Context, event githubapp.Event) error {
	s.mu.Lock()
	monitor := s.monitor
	s.mu.Unlock()
	if monitor == nil {
		return nil
	}
	return monitor.Delivery(ctx, event)
}

// List answers every connection Marshal knows, in the screen's order, with what is stored for each
// and the last test's own answer. A connection of a later phase has no row and reads "not
// connected"; GitHub's row is filled from what is stored and what its last test found.
func (s *Service) List(ctx context.Context) ([]protocol.Integration, error) {
	out := make([]protocol.Integration, 0, len(known()))
	for _, info := range known() {
		row, ok, err := s.row(ctx, info.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, s.rowToWire(ctx, info, row, ok))
	}
	return out, nil
}

// rowToWire folds one connection's stored row and last test into the shape a screen reads.
//
// It has two shapes, because Marshal has two kinds of connection. A connection a person sets up
// (GitHub, and the integrations of later phases) is "not connected" until a setting and a secret are
// both saved, and its status is its last test's own answer. A connection Marshal owns (the Obsidian
// vault) has nothing for a person to save, so its status is what Marshal can see for itself, and a
// test that failed is what turns it to "needs attention".
func (s *Service) rowToWire(ctx context.Context, info Info, row integrationRow, ok bool) protocol.Integration {
	wire := protocol.Integration{ID: info.ID, Kind: info.Kind, Status: protocol.IntegrationStatusNone}
	last, tested, err := s.lastTest(ctx, info.ID)
	if err != nil {
		// A last test Marshal cannot read is not a failure of the connection: the reading is logged
		// and left out, and the status is decided by what the connection itself is.
		s.log.Warn("a connection test could not be read", "connection", info.ID, "err", err)
	}
	if tested {
		wire.LastTest = &last
	}
	if s.selfOwned(info) {
		wire.Status, wire.Detail = s.vaultStatus()
		if tested && !last.OK {
			// A test that asked about the vault and got a bad answer is exactly what the row is for:
			// the checks say what is wrong and the detail says how to fix it.
			wire.Status, wire.Detail = protocol.IntegrationStatusError, detailFor(info, last, true)
		}
		return wire
	}
	if !ok || !row.saved() {
		wire.Status, wire.LastTest = protocol.IntegrationStatusNone, nil
		return wire
	}
	wire.Status = statusFor(last, tested)
	wire.Detail = detailFor(info, last, tested)
	return wire
}

// selfOwned reports whether Marshal owns a connection itself rather than a person setting it up. The
// Obsidian vault is the one: it is a folder in Marshal's own data directory, so there is no setting
// to fill in, no secret to keep, and nothing for a "not connected" row to mean.
func (s *Service) selfOwned(info Info) bool { return info.Kind == KindObsidian }

// vaultStatus is the Obsidian row's own answer: where the vault is, and whether Marshal can see it.
// It is "connected" even before the folder exists, because the vault is Marshal's own folder and a
// person has nothing to connect - the sentence says where it will be, and the test says whether it
// is there and whether Marshal can write in it.
func (s *Service) vaultStatus() (protocol.IntegrationStatus, string) {
	if s.vault == "" {
		return protocol.IntegrationStatusNone, ""
	}
	if info, err := os.Stat(s.vault); err != nil || !info.IsDir() {
		return protocol.IntegrationStatusConnected,
			fmt.Sprintf("Vault at %s. Marshal makes the folder when the first note is saved.", s.vault)
	}
	return protocol.IntegrationStatusConnected, fmt.Sprintf("Vault at %s.", s.vault)
}

// statusFor decides a saved connection's status from its last test. A test that failed any check
// makes the connection need attention; a test that passed, warned, or has not run leaves it
// connected, because a warning is not a failure and what has not been tested is not known to be
// broken.
func statusFor(last protocol.TestResult, tested bool) protocol.IntegrationStatus {
	if tested && !last.OK {
		return protocol.IntegrationStatusError
	}
	return protocol.IntegrationStatusConnected
}

// detailFor is the one sentence under a row: the failed check's fix when the last test failed, the
// summary a passing test wrote when it passed, and a plain "run the test" when it has not run yet.
func detailFor(info Info, last protocol.TestResult, tested bool) string {
	if !tested {
		return "Connected. Run the test to check it."
	}
	if failed, ok := last.FirstFailed(); ok {
		if failed.Fix != "" {
			return failed.Fix
		}
		return failed.Message
	}
	if check, ok := summaryOf(last); ok {
		return check.Message
	}
	return "Connected."
}

// summaryOf finds the check whose message is the row's own sentence. Every connection test names one
// check "Summary" for exactly this (test.go), so a row reads back what the test found without this
// file knowing what that test asks.
func summaryOf(result protocol.TestResult) (protocol.TestCheck, bool) {
	for _, check := range result.Checks {
		if check.Name == CheckSummary {
			return check, true
		}
	}
	return protocol.TestCheck{}, false
}

// integrationRow is one `integrations` row's settings, read as this package's own shape.
type integrationRow struct {
	config   string
	keychain string
}

// saved reports whether a connection has both a setting and a secret, which is what "connected"
// means: a config with no secret cannot be used, and a secret with no config is inert.
func (r integrationRow) saved() bool {
	return strings.TrimSpace(r.config) != "" && strings.TrimSpace(r.keychain) != ""
}

// row reads one connection's row. A connection with no row is not an error: it is the ordinary
// state of a connection nobody has set up yet.
func (s *Service) row(ctx context.Context, id string) (integrationRow, bool, error) {
	var out integrationRow
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read the %q connection: %w", id, err)
		}
		found = true
		out = integrationRow{config: row.ConfigJSON, keychain: row.KeychainRef}
		return nil
	})
	return out, found, err
}

// lastTest answers the last saved test of one connection, and whether there is one. It reads the
// same two columns the connection-test runner writes, so a test run from the screens and a test run
// here are read the same way.
func (s *Service) lastTest(ctx context.Context, id string) (protocol.TestResult, bool, error) {
	var out protocol.TestResult
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || row.LastTestResultJSON == "" {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read the last test of %q: %w", id, err)
		}
		if err := json.Unmarshal([]byte(row.LastTestResultJSON), &out); err != nil {
			return fmt.Errorf("read the last test of %q: %w", id, err)
		}
		found = true
		return nil
	})
	if !found || err != nil {
		return protocol.TestResult{}, false, err
	}
	if out.ConnectionID == "" {
		out.ConnectionID = id
	}
	return out, true, nil
}
