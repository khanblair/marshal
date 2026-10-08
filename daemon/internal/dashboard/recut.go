package dashboard

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// recutKey is one project's day, the primary key of `daily_stats`.
type recutKey struct {
	day       int64
	projectID string
}

// recutTotals is the four numbers of a day, or the sum of a range of days.
type recutTotals struct {
	finished, merges, ciFailures, cost int64
}

func (t *recutTotals) add(o recutTotals) {
	t.finished += o.finished
	t.merges += o.merges
	t.ciFailures += o.ciFailures
	t.cost += o.cost
}

// Recut rebuilds the last ninety days of `daily_stats` in loc, in one transaction, from the activity
// stream and the usage rows. Older days are left as they are. Call it when the person's zone changes.
func (s *Service) Recut(ctx context.Context, loc *time.Location) error {
	if loc == nil {
		return fmt.Errorf("dashboard: a re-cut needs the time zone to cut the days by")
	}
	cutoff := recutCutoff(s.now(), loc)
	var before, after recutTotals
	err := s.store.Write(ctx, func(q *db.Queries) error {
		stored, err := q.ListDailyStats(ctx, db.ListDailyStatsParams{FromDay: cutoff.UnixMilli(), ToDay: math.MaxInt64})
		if err != nil {
			return fmt.Errorf("read the stored numbers: %w", err)
		}
		days, err := rebuildDays(ctx, q, cutoff, loc)
		if err != nil {
			return err
		}
		if err := q.DeleteDailyStatsSince(ctx, cutoff.UnixMilli()); err != nil {
			return fmt.Errorf("drop the days to cut again: %w", err)
		}
		for key, day := range days {
			if err := q.UpsertDailyStat(ctx, db.UpsertDailyStatParams{
				Day: key.day, ProjectID: key.projectID, CardsFinished: day.finished,
				Merges: day.merges, CiFailures: day.ciFailures, CostMicros: day.cost,
			}); err != nil {
				return fmt.Errorf("store a re-cut day: %w", err)
			}
			after.add(*day)
		}
		for _, row := range stored {
			before.add(recutTotals{row.CardsFinished, row.Merges, row.CiFailures, row.CostMicros})
		}
		return nil
	})
	if err != nil {
		return err
	}
	// The day straddling the cutoff can only add to the new rows, so fewer than before means a loss.
	if after.finished < before.finished || after.merges < before.merges ||
		after.ciFailures < before.ciFailures || after.cost < before.cost {
		s.log.Warn("a re-cut found fewer finished cards, merges, or cost than were stored",
			"zone", loc.String(), "before", before, "after", after)
	}
	s.log.Info("re-cut the daily numbers", "zone", loc.String(), "from", cutoff, "totals", after)
	return nil
}

// recutCutoff is midnight of the oldest day kept, ninety days ending with today, in loc.
func recutCutoff(now time.Time, loc *time.Location) time.Time {
	return startOfDay(now.In(loc)).AddDate(0, 0, -(activityRetentionDays - 1))
}

// dayOf is the key of the day in loc that an instant, in Unix milliseconds, falls on.
func dayOf(ms int64, loc *time.Location) int64 {
	return startOfDay(time.UnixMilli(ms).In(loc)).UnixMilli()
}

// rebuildDays works out every project's numbers for every day from the cutoff on. A finished card
// is a merge row of the stream and a CI failure a ci row (the subscriber writes each with its count),
// and cost is the sum of the usage rows (the recorder writes those together too).
func rebuildDays(ctx context.Context, q *db.Queries, cutoff time.Time, loc *time.Location) (map[recutKey]*recutTotals, error) {
	days := make(map[recutKey]*recutTotals)
	at := func(day int64, projectID string) *recutTotals {
		key := recutKey{day: day, projectID: projectID}
		if days[key] == nil {
			days[key] = &recutTotals{}
		}
		return days[key]
	}
	since := cutoff.UnixMilli()
	merged, err := q.ListActivityCountsByMinute(ctx, db.ListActivityCountsByMinuteParams{
		Kind: string(protocol.FeedKindMerge), Since: since,
	})
	if err != nil {
		return nil, fmt.Errorf("count the finished cards: %w", err)
	}
	for _, row := range merged {
		day := at(dayOf(row.MinuteMs, loc), row.ProjectID)
		day.finished += row.Entries
		day.merges += row.Entries
	}
	costs, err := q.ListUsageCostByMinute(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("add up the cost: %w", err)
	}
	for _, row := range costs {
		at(dayOf(row.MinuteMs, loc), row.ProjectID).cost += row.CostMicros
	}
	failed, err := q.ListActivityCountsByMinute(ctx, db.ListActivityCountsByMinuteParams{
		Kind: string(protocol.FeedKindCI), Since: since,
	})
	if err != nil {
		return nil, fmt.Errorf("count the CI failures: %w", err)
	}
	for _, row := range failed {
		at(dayOf(row.MinuteMs, loc), row.ProjectID).ciFailures += row.Entries
	}
	return days, nil
}
