package providers

// This file is the recorded half of a provider call: what one call cost, and how that reaches the
// store (docs/architecture.md section 10's `usage` table and `daily_stats`, inventory B4.4). It is
// the daemon's usage writer that store/queries/usage.sql names: it inserts one row per call and, in
// the same transaction, adds the same cost to that day's daily_stats.cost_micros, so the Home chart
// and the usage detail can never disagree.
//
// It is an interface as well as an implementation, because the providers package must be testable
// without a database: a test passes a fake recorder and reads what a call would have filed, and the
// running daemon passes the store-backed one built here.

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// UsageRecord is one call's cost, ready to store: the columns of the `usage` table, with the time as
// a Time rather than the milliseconds it is stored as.
type UsageRecord struct {
	// CardID is the card the call ran for. Empty for a chat, a title, or a connection test.
	CardID string
	// ProjectID is the card's project. It is stored on the row so the spend it caused survives the
	// card being removed, which is why it is written rather than joined from cards at read time.
	ProjectID string
	// RoleID is the role the call ran as. Empty when the session had no role.
	RoleID string
	// Provider is the provider that answered, such as "anthropic".
	Provider string
	// Model is the provider's own model name.
	Model string
	// InputTokens and OutputTokens are what the provider reported for the call.
	InputTokens  int64
	OutputTokens int64
	// CostMicros is what the call cost in micro-dollars (see Cost). Zero when the model has no
	// known price, which is why the token counts are stored beside it: the row still says what was
	// spent, only not what it was worth.
	CostMicros int64
	// At is when the call finished.
	At time.Time
}

// UsageRecorder files what a call cost. It is one method so that a Service can be given a fake, and
// so the providers package can be exercised without a database.
type UsageRecorder interface {
	Record(ctx context.Context, r UsageRecord) error
}

// StoreRecorder writes usage rows into the daemon's store. It is the recorder a running daemon uses
// (see marshald's wiring), and the store is required: a recorder with nowhere to write would
// quietly lose every receipt.
type StoreRecorder struct {
	store   *store.Store
	entropy io.Reader
}

// NewStoreRecorder returns a recorder that writes into st. entropy makes each row's opaque id; use
// crypto/rand.Reader in the daemon and a fixed reader in a test.
func NewStoreRecorder(st *store.Store, entropy io.Reader) *StoreRecorder {
	return &StoreRecorder{store: st, entropy: entropy}
}

// Record writes one usage row and adds its cost to that day's total, in one transaction. The two are
// one write because they are one fact counted two ways: the row is the detail a person can trace,
// and the day's number is the pre-computed total the Home chart reads. A crash between two separate
// writes would leave a chart that disagrees with the rows under it, which is exactly what the usage
// writer is supposed to make impossible.
//
// A call with no cost still gets its row - the tokens are the record of what was spent - but adds
// nothing to the day, rather than taking the writer's lock to add zero.
func (r *StoreRecorder) Record(ctx context.Context, rec UsageRecord) error {
	if r == nil || r.store == nil {
		return nil
	}
	id, err := protocol.NewID(rec.At, r.entropy)
	if err != nil {
		return fmt.Errorf("make a usage id: %w", err)
	}
	return r.store.Write(ctx, func(q *db.Queries) error {
		if err := q.InsertUsage(ctx, db.InsertUsageParams{
			ID:           id,
			CardID:       rec.CardID,
			ProjectID:    rec.ProjectID,
			RoleID:       rec.RoleID,
			Provider:     rec.Provider,
			Model:        rec.Model,
			InputTokens:  rec.InputTokens,
			OutputTokens: rec.OutputTokens,
			CostMicros:   rec.CostMicros,
			CreatedAt:    rec.At.UnixMilli(),
		}); err != nil {
			return fmt.Errorf("record a model call: %w", err)
		}
		if rec.CostMicros == 0 {
			return nil
		}
		if err := q.UpsertDailyStat(ctx, db.UpsertDailyStatParams{
			Day:        dayStart(rec.At).UnixMilli(),
			ProjectID:  rec.ProjectID,
			CostMicros: rec.CostMicros,
		}); err != nil {
			return fmt.Errorf("add the cost to the day's total: %w", err)
		}
		return nil
	})
}

// dayStart is midnight of t's own day, in t's own location. It has to bucket a call's cost into the
// same day the dashboard's cardFinished writes its numbers into (internal/dashboard.startOfDay), or
// the Home chart and the usage rows would disagree about which day a call belongs to.
func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
