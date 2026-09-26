// Package dashboard owns Home: the cards that wait on a person, the cards with a running session,
// the numbers the tiles and charts show, and the activity stream the Recent activity list and its
// view-all page draw. Slice A of Phase 2 built the first version, which read the cards and sessions
// tables; slice D adds the stored daily numbers and the activity stream, so Home never scans every
// card (docs/backend-checklist.md B2.3).
//
// The package both reads and writes. Service answers the two Home routes from stored state, and the
// event subscriber in subscriber.go keeps `daily_stats` and `activity` current from the events the
// daemon already publishes. The subscriber is the only writer here, and it publishes activity.created
// on the home topic after a row is committed, so a client's feed and charts update live.
//
// It never touches another module's tables for writing, and it reads only its own two tables plus
// the cards, sessions, and projects a Home answer has always joined. A module with a goroutine
// closes before the bus (cmd/marshald's close order).
package dashboard

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// RangeWeek is the seven-day chart range, and the one a request that does not say gets.
	RangeWeek = 7
	// RangeMonth is the thirty-day chart range.
	RangeMonth = 30
	// RangeQuarter is the ninety-day chart range.
	RangeQuarter = 90
	// DefaultPageSize is how many activity entries one page holds when the caller does not say. It
	// matches the API's own default (api.DefaultPageLimit).
	DefaultPageSize = 50
	// MaxPageSize is the most entries one page returns, so one client cannot ask for the whole
	// stream at once. It matches the API's own cap.
	MaxPageSize = 200
	// activityRetentionDays is how long a row of the activity stream is kept. Older rows are trimmed
	// after every append (docs/architecture.md 16.3).
	activityRetentionDays = 90
)

// HomeRanges lists the chart ranges a Home request may ask for, in days, shortest first.
func HomeRanges() []int { return []int{RangeWeek, RangeMonth, RangeQuarter} }

// ValidRange reports whether days is one of the ranges the Home charts cover.
func ValidRange(days int) bool {
	for _, known := range HomeRanges() {
		if days == known {
			return true
		}
	}
	return false
}

// Deps are the parts the service is built from.
type Deps struct {
	// Store is the open database.
	Store *store.Store
}

// Service builds Home's answers. It is safe for use by many goroutines.
type Service struct {
	store *store.Store
	log   *slog.Logger
	now   func() time.Time
}

// New builds the service. A store is required: every answer comes from it.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("dashboard: a store is required")
	}
	s := &Service{store: deps.Store, log: slog.New(slog.DiscardHandler), now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock. The default is time.Now. The clock decides which day "merged today"
// counts and which day a stored number belongs to, so a test can fix it.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithLogger sets the logger.
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) {
		if log != nil {
			s.log = log
		}
	}
}

// Home returns everything the Home screen draws on its first load: the lists, the tiles, and the
// stored numbers the charts draw over `days` days. It runs its reads in one read transaction, and it
// never scans every card: the lists are the cards that wait and the cards that are awake, the tiles
// are one grouped count and one count for today, and the charts are one indexed range read of
// `daily_stats`.
//
// A range that is not one of the three the charts cover is answered with the seven-day one, so an
// answer is always drawable; the route refuses an unknown range before it gets here.
func (s *Service) Home(ctx context.Context, days int) (protocol.HomeSnapshot, error) {
	now := s.now()
	rangeDays := days
	if !ValidRange(rangeDays) {
		rangeDays = RangeWeek
	}
	today := startOfDay(now)
	from := today.AddDate(0, 0, -(rangeDays - 1))
	var (
		needs    []db.ListCardsNeedingYouRow
		awake    []db.ListAwakeCardsRow
		byState  []db.CountCardsByStateRow
		finished int64
		stats    []db.DailyStat
	)
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		if needs, err = q.ListCardsNeedingYou(ctx); err != nil {
			return fmt.Errorf("list the cards that need someone: %w", err)
		}
		if awake, err = q.ListAwakeCards(ctx); err != nil {
			return fmt.Errorf("list the cards with a running session: %w", err)
		}
		if byState, err = q.CountCardsByState(ctx); err != nil {
			return fmt.Errorf("count the cards by state: %w", err)
		}
		if finished, err = q.CountCardsFinishedSince(ctx, today.UnixMilli()); err != nil {
			return fmt.Errorf("count the cards finished today: %w", err)
		}
		if stats, err = q.ListDailyStats(ctx, db.ListDailyStatsParams{
			FromDay: from.UnixMilli(), ToDay: today.UnixMilli(),
		}); err != nil {
			return fmt.Errorf("read the stored numbers: %w", err)
		}
		return nil
	})
	if err != nil {
		return protocol.HomeSnapshot{}, err
	}
	return protocol.HomeSnapshot{
		Needs:      toNeedsCards(needs),
		Awake:      toAwakeCards(awake),
		Tiles:      tilesOf(byState, awake, finished),
		Stats:      toHomeStats(stats, rangeDays, from, today),
		ServerTime: protocol.NewTimestamp(now),
	}, nil
}

// ActivityFilter narrows the Home activity stream. An empty field means every value.
type ActivityFilter struct {
	// Kind is one feed kind, or empty for all of them.
	Kind protocol.FeedKind
	// ProjectID is one project's short id, or empty for all of them.
	ProjectID string
}

// Page is one page of a list, with the position the page after it starts from.
type Page[T any] struct {
	// Items are the items of this page, newest first. Never nil, so a page with nothing on it sends
	// an empty list rather than null.
	Items []T
	// Cursor is the sequence of the oldest row this page read, which is where the next page starts.
	// It is only meaningful when More is true, and it is the cursor the caller gave when the page is
	// empty.
	Cursor int64
	// More reports whether another page follows this one.
	More bool
}

// Activity returns one page of the Home activity stream, newest first, filtered by kind and project
// (docs/backend-checklist.md B2.3, section S20). The cursor is the sequence the previous page ended
// at, or zero for the newest page. A filter that matches nothing is an empty page, never an error.
//
// It is one indexed read of `activity` for the limit plus one row, so it never walks the stream and
// never touches a card.
func (s *Service) Activity(ctx context.Context, filter ActivityFilter, cursor int64, limit int) (Page[protocol.FeedEntry], error) {
	if filter.Kind != "" && !filter.Kind.Valid() {
		return Page[protocol.FeedEntry]{}, fmt.Errorf("dashboard: %q is not a feed kind", filter.Kind)
	}
	size := pageSize(limit)
	rows, err := s.readActivity(ctx, filter, cursor, size+1)
	if err != nil {
		return Page[protocol.FeedEntry]{}, err
	}
	page := Page[protocol.FeedEntry]{Items: []protocol.FeedEntry{}, Cursor: cursor}
	if len(rows) > size {
		// There is one row more than the page holds, so another page follows. That extra row is only
		// how that is known; it belongs to the next page.
		page.More = true
		rows = rows[:size]
	}
	for _, row := range rows {
		page.Items = append(page.Items, toFeedEntry(row))
		page.Cursor = row.Seq
	}
	return page, nil
}

// readActivity reads one more row than the page holds, choosing the query for the filter so each
// kind of page uses its own index.
func (s *Service) readActivity(ctx context.Context, filter ActivityFilter, cursor int64, limit int) ([]db.Activity, error) {
	seq := before(cursor)
	var rows []db.Activity
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		switch {
		case filter.Kind != "" && filter.ProjectID != "":
			rows, err = q.ListActivityByKindAndProject(ctx, db.ListActivityByKindAndProjectParams{
				Kind: string(filter.Kind), ProjectID: filter.ProjectID, Seq: seq, Limit: int64(limit),
			})
		case filter.Kind != "":
			rows, err = q.ListActivityByKind(ctx, db.ListActivityByKindParams{
				Kind: string(filter.Kind), Seq: seq, Limit: int64(limit),
			})
		case filter.ProjectID != "":
			rows, err = q.ListActivityByProject(ctx, db.ListActivityByProjectParams{
				ProjectID: filter.ProjectID, Seq: seq, Limit: int64(limit),
			})
		default:
			rows, err = q.ListActivity(ctx, db.ListActivityParams{Seq: seq, Limit: int64(limit)})
		}
		if err != nil {
			return fmt.Errorf("read the activity stream: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// before turns a page cursor into the "sequence less than" bound the activity queries take. No
// cursor (zero, the newest page) becomes the largest possible bound, since no stored seq is ever
// this large: the stream's own seq starts at 1 (see NextActivitySeq).
func before(cursor int64) int64 {
	if cursor <= 0 {
		return math.MaxInt64
	}
	return cursor
}

// pageSize is the number of rows a page reads, whatever the caller asked for.
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

// startOfDay is midnight at the start of the clock's own day. It is what "merged today" counts
// from, and it is the day a stored number belongs to, so the number changes at the person's
// midnight, not at UTC's.
func startOfDay(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// tilesOf works out the three counts Home shows at the top. The needs count and the merged count
// come from the database; the working count is the cards that have a running session, which is the
// same list the awake part of the screen draws.
func tilesOf(byState []db.CountCardsByStateRow, awake []db.ListAwakeCardsRow, finished int64) protocol.HomeTiles {
	tiles := protocol.HomeTiles{MergedToday: int(finished), Working: len(awake)}
	for _, row := range byState {
		if protocol.CardState(row.State) == protocol.CardStateNeeds {
			tiles.Needs += int(row.Cards)
		}
	}
	return tiles
}
