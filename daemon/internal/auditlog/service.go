// Package auditlog reads the audit log back (docs/backend-checklist.md B3.5,
// docs/marshal-product-scope.md section 14.7). internal/audit writes the rows; this package is how
// they are read: one page of them newest first, a search across the columns a person can search by,
// and an export of everything a query matched.
//
// It reads only. Nothing here writes a row, publishes an event, or can change what happened.
package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// DefaultPageSize is how many rows one page holds when the caller does not say. It matches the
	// API's own default (api.DefaultPageLimit) and the history service's.
	DefaultPageSize = 50
	// MaxPageSize is the most rows one page returns, so one client cannot ask for the whole log at
	// once. It matches the API's own cap.
	MaxPageSize = 200
	// MaxExport is the most rows one export carries. An export is not paged, so this is the ceiling
	// that keeps one request from reading an unbounded table.
	MaxExport = 10000
)

// Deps are the parts the service is built from. The store is required.
type Deps struct {
	// Store is the open database the audit rows are read from.
	Store *store.Store
}

// Service serves the audit log, read only. It is safe for use by many goroutines.
type Service struct {
	store *store.Store
	log   *slog.Logger
	now   func() time.Time
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock the answer's serverTime comes from. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithLogger sets the logger.
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) {
		if log != nil {
			s.log = log
		}
	}
}

// New builds the service.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("auditlog: a store is required")
	}
	s := &Service{store: deps.Store, log: slog.New(slog.DiscardHandler), now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Cursor is where a page of the log starts. It is the (createdAt, id) pair of the last row the page
// before ended at, which is the pair the table indexes and the only pair this reads in order by. The
// zero Cursor means the newest page.
type Cursor struct {
	// CreatedAt is the write time of the last row on the page before, in milliseconds.
	CreatedAt int64 `json:"createdAt,omitempty"`
	// ID is the id of the last row on the page before.
	ID string `json:"id,omitempty"`
}

// Page is one page of audit entries, with where the page after it starts.
type Page struct {
	// Items are the entries of this page, newest first. Never nil, so a page with nothing on it
	// sends an empty list rather than null.
	Items []protocol.AuditEntry
	// Next is where the next page starts. It is only meaningful when More is true.
	Next Cursor
	// More reports whether another page follows this one.
	More bool
}

// Entries returns one page of the audit log, newest first. An empty query means every row; a query
// keeps the rows whose actor, action, target, or detail contains it. The cursor is the position the
// page before ended at, and the zero cursor asks for the newest page.
func (s *Service) Entries(ctx context.Context, query string, cursor Cursor, limit int) (Page, error) {
	limit = pageSize(limit)
	before := start(cursor)
	// Ask for one more than the page holds: getting it back is how the page knows another follows,
	// without a second query.
	rows, err := s.rows(ctx, query, before, int64(limit+1))
	if err != nil {
		return Page{}, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	items := make([]protocol.AuditEntry, 0, len(rows))
	for _, row := range rows {
		items = append(items, entryOf(row))
	}
	var next Cursor
	if more && len(rows) > 0 {
		next = cursorOf(rows[len(rows)-1])
	}
	return Page{Items: items, Next: next, More: more}, nil
}

// Export returns every row a query matched, newest first, for a download. An empty query means
// every row.
func (s *Service) Export(ctx context.Context, query string) (protocol.AuditExport, error) {
	var (
		rows []db.AuditLog
		err  error
	)
	if query == "" {
		rows, err = s.store.Queries().ExportAuditLog(ctx, MaxExport)
	} else {
		rows, err = s.store.Queries().SearchAuditLogExport(ctx, db.SearchAuditLogExportParams{
			Pattern: like(query), ExportLimit: MaxExport,
		})
	}
	if err != nil {
		return protocol.AuditExport{}, err
	}
	entries := make([]protocol.AuditEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, entryOf(row))
	}
	return protocol.AuditExport{
		Query: query, Entries: entries, Total: len(entries),
		ServerTime: protocol.NewTimestamp(s.now()),
	}, nil
}

// rows reads one page of rows, plain or searched.
func (s *Service) rows(ctx context.Context, query string, before Cursor, limit int64) ([]db.AuditLog, error) {
	if query == "" {
		return s.store.Queries().ListAuditLogPage(ctx, db.ListAuditLogPageParams{
			BeforeTime: before.CreatedAt, BeforeID: before.ID, PageLimit: limit,
		})
	}
	return s.store.Queries().SearchAuditLogPage(ctx, db.SearchAuditLogPageParams{
		BeforeTime: before.CreatedAt, BeforeID: before.ID, Pattern: like(query), PageLimit: limit,
	})
}

// start turns a cursor into the bound a page reads before. The zero cursor becomes the end of time
// with an empty id, which is every row.
func start(cursor Cursor) Cursor {
	if cursor.CreatedAt == 0 && cursor.ID == "" {
		return Cursor{CreatedAt: math.MaxInt64}
	}
	return cursor
}

// cursorOf makes the cursor that follows a row.
func cursorOf(row db.AuditLog) Cursor {
	return Cursor{CreatedAt: row.CreatedAt, ID: row.ID}
}

// like wraps a person's words as a SQL LIKE pattern.
func like(query string) string { return "%" + query + "%" }

// pageSize keeps a requested page size inside the bounds the service allows.
func pageSize(limit int) int {
	switch {
	case limit <= 0:
		return DefaultPageSize
	case limit > MaxPageSize:
		return MaxPageSize
	default:
		return limit
	}
}

// entryOf maps one stored row to the wire entry. A detail that is not JSON, or is empty, is left
// out rather than failing the read: a row that cannot be understood is still a row that happened.
func entryOf(row db.AuditLog) protocol.AuditEntry {
	entry := protocol.AuditEntry{
		ID: row.ID, SessionID: row.SessionID, Actor: row.Actor, Action: row.Action,
		Target: row.Target, At: protocol.NewTimestamp(time.UnixMilli(row.CreatedAt).UTC()),
	}
	if row.DetailJSON != "" && row.DetailJSON != "{}" {
		var detail map[string]any
		if err := json.Unmarshal([]byte(row.DetailJSON), &detail); err == nil && len(detail) > 0 {
			entry.Detail = detail
		}
	}
	return entry
}
