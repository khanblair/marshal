package dashboard

import (
	"sort"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// What a row of the activity stream is about, as stored in `subject_kind`. The kind says how a
// client opens the row: a card is opened by its key, a scheduled job by its id, and anything else
// (a brief, a system line) is only read. A kind the app does not know leaves the subject out, so
// the row still draws.
const (
	subjectCard     = "card"
	subjectSchedule = "schedule"
)

// toNeedsCards builds the "needs you" rows. The list is never nil, so a screen that draws it
// iterates an empty list rather than checking for null.
func toNeedsCards(rows []db.ListCardsNeedingYouRow) []protocol.NeedsCard {
	cards := make([]protocol.NeedsCard, 0, len(rows))
	for _, row := range rows {
		card := protocol.NeedsCard{
			CardID:       row.ID,
			Key:          protocol.CardKey{ProjectID: row.ProjectID, Number: int(row.Number)}.String(),
			Number:       int(row.Number),
			ProjectID:    row.ProjectID,
			ProjectName:  row.ProjectName,
			Title:        row.Title,
			Role:         row.Role,
			WaitingSince: waitingSince(row.NeedsSince),
		}
		if row.NeedsReasonKind != "" {
			card.Reason = protocol.NeedsReason{
				Kind: protocol.NeedsReasonKind(row.NeedsReasonKind),
				Text: row.NeedsReasonText,
			}
		}
		cards = append(cards, card)
	}
	return cards
}

// toAwakeCards builds the "agents awake" rows.
func toAwakeCards(rows []db.ListAwakeCardsRow) []protocol.AwakeCard {
	cards := make([]protocol.AwakeCard, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, protocol.AwakeCard{
			CardID:      row.CardID,
			Key:         protocol.CardKey{ProjectID: row.ProjectID, Number: int(row.Number)}.String(),
			Number:      int(row.Number),
			ProjectID:   row.ProjectID,
			ProjectName: row.ProjectName,
			Title:       row.Title,
			State:       protocol.CardState(row.State),
			Session:     protocol.SessionState(row.SessionState),
			DoingNow:    row.DoingNow,
			Pinned:      row.Pinned != 0,
			Paused:      row.Paused != 0,
			ContextUsed: int(row.ContextUsed),
			AwakeSince:  awakeSince(row.SessionStartedAt),
		})
	}
	return cards
}

// toHomeStats builds the chart data from the stored rows of a day range. Every day of the range is
// on the answer, in order and with a zeroed one where nothing was stored, so a chart always has the
// number of points its range says it has; and the per-project series are padded the same way, so a
// line per project has the same days as the totals.
//
// The rows arrive ordered by day and project, and the projects are ordered by id here rather than by
// their first day, so the same stored numbers always answer in the same order.
func toHomeStats(rows []db.DailyStat, days int, from, to time.Time) *protocol.HomeStats {
	totals := make(map[int64]protocol.HomeStatDay, days)
	byProject := make(map[string]map[int64]protocol.HomeStatDay)
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		at := protocol.NewTimestamp(time.UnixMilli(row.Day))
		total := totals[row.Day]
		total.Day = at
		total.CardsFinished += int(row.CardsFinished)
		total.Merges += int(row.Merges)
		total.CIFailures += int(row.CiFailures)
		total.CostMicros += row.CostMicros
		totals[row.Day] = total

		series, seen := byProject[row.ProjectID]
		if !seen {
			series = make(map[int64]protocol.HomeStatDay, days)
			byProject[row.ProjectID] = series
			ids = append(ids, row.ProjectID)
		}
		own := series[row.Day]
		own.Day = at
		own.CardsFinished += int(row.CardsFinished)
		own.Merges += int(row.Merges)
		own.CIFailures += int(row.CiFailures)
		own.CostMicros += row.CostMicros
		series[row.Day] = own
	}
	sort.Strings(ids)

	stats := &protocol.HomeStats{
		Range:    days,
		From:     protocol.NewTimestamp(from),
		To:       protocol.NewTimestamp(to),
		Days:     fillDays(totals, days, from),
		Projects: make([]protocol.HomeProjectStats, 0, len(ids)),
	}
	for _, id := range ids {
		stats.Projects = append(stats.Projects, protocol.HomeProjectStats{
			ProjectID: id,
			Days:      fillDays(byProject[id], days, from),
		})
	}
	return stats
}

// fillDays lays a map of stored days out over the whole range, oldest first, filling the days with
// nothing stored with a zeroed row that still carries its date.
func fillDays(stored map[int64]protocol.HomeStatDay, days int, from time.Time) []protocol.HomeStatDay {
	out := make([]protocol.HomeStatDay, 0, days)
	for offset := range days {
		at := from.AddDate(0, 0, offset)
		if day, ok := stored[at.UnixMilli()]; ok {
			out = append(out, day)
			continue
		}
		out = append(out, protocol.HomeStatDay{Day: protocol.NewTimestamp(at)})
	}
	return out
}

// toFeedEntry maps one stored row of the activity stream to the wire entry a client draws. The
// project is left out for an entry that belongs to none, so a client can tell "no project" from a
// project whose id it does not know.
func toFeedEntry(row db.Activity) protocol.FeedEntry {
	entry := protocol.FeedEntry{
		ID:   row.ID,
		Kind: protocol.FeedKind(row.Kind),
		Text: row.Summary,
		At:   protocol.NewTimestamp(time.UnixMilli(row.CreatedAt)),
	}
	if row.ProjectID != "" {
		projectID := row.ProjectID
		entry.ProjectID = &projectID
	}
	switch row.SubjectKind {
	case subjectCard:
		entry.CardID = row.SubjectID
		entry.CardKey = row.SubjectKey
	case subjectSchedule:
		entry.JobID = row.SubjectID
	}
	return entry
}

// waitingSince is when a card started waiting on a person, or nil when the daemon does not know.
func waitingSince(ms *int64) *protocol.Timestamp {
	if ms == nil {
		return nil
	}
	ts := store.Timestamp(*ms)
	return &ts
}

// awakeSince is when the card's session was made. It is how long the agent has been running.
func awakeSince(ms int64) *protocol.Timestamp {
	ts := store.Timestamp(ms)
	return &ts
}
