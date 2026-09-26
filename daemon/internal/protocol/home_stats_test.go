package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The stored Home numbers and the activity stream (docs/backend-checklist.md B2.3, sections S19a
// and S20, inventory N17). A Go test writes the golden files and the TypeScript tests read them, so
// the two sides cannot drift apart.

// homeDay is midnight at the start of the day `daysAgo` days before the sample day, which is what a
// stored day is and what a chart's axis is built from.
func homeDay(daysAgo int) protocol.Timestamp {
	midnight := time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)
	return protocol.NewTimestamp(midnight.AddDate(0, 0, -daysAgo))
}

// homeStatDays is one week of days with the projects added together, oldest first. The sums of the
// two per-project series below are exactly these, so a client that adds the projects up gets the
// same chart as one that draws the totals.
func homeStatDays() []protocol.HomeStatDay {
	return []protocol.HomeStatDay{
		{Day: homeDay(6), CardsFinished: 2, Merges: 2},
		{Day: homeDay(5), CardsFinished: 3, Merges: 3},
		{Day: homeDay(4), CardsFinished: 1, Merges: 1, CIFailures: 1},
		{Day: homeDay(3), CardsFinished: 4, Merges: 3},
		{Day: homeDay(2), CardsFinished: 2, Merges: 2},
		{Day: homeDay(1), CardsFinished: 5, Merges: 4, CIFailures: 2},
		{Day: homeDay(0), CardsFinished: 3, Merges: 3},
	}
}

// homeStatProjects is the same week, one series per project, for the chart that draws a line per
// project.
func homeStatProjects() []protocol.HomeProjectStats {
	return []protocol.HomeProjectStats{
		{
			ProjectID: "api",
			Days: []protocol.HomeStatDay{
				{Day: homeDay(6), CardsFinished: 1, Merges: 1},
				{Day: homeDay(5), CardsFinished: 2, Merges: 2},
				{Day: homeDay(4), CIFailures: 1},
				{Day: homeDay(3), CardsFinished: 2, Merges: 2},
				{Day: homeDay(2), CardsFinished: 1, Merges: 1},
				{Day: homeDay(1), CardsFinished: 3, Merges: 2, CIFailures: 2},
				{Day: homeDay(0), CardsFinished: 1, Merges: 1},
			},
		},
		{
			ProjectID: "web",
			Days: []protocol.HomeStatDay{
				{Day: homeDay(6), CardsFinished: 1, Merges: 1},
				{Day: homeDay(5), CardsFinished: 1, Merges: 1},
				{Day: homeDay(4), CardsFinished: 1, Merges: 1},
				{Day: homeDay(3), CardsFinished: 2, Merges: 1},
				{Day: homeDay(2), CardsFinished: 1, Merges: 1},
				{Day: homeDay(1), CardsFinished: 2, Merges: 2},
				{Day: homeDay(0), CardsFinished: 2, Merges: 2},
			},
		},
	}
}

// sampleStats is the chart data of a seven-day Home answer.
func sampleStats() protocol.HomeStats {
	return protocol.HomeStats{
		Range:    7,
		From:     homeDay(6),
		To:       homeDay(0),
		Days:     homeStatDays(),
		Projects: homeStatProjects(),
	}
}

// The stored numbers behind the Home charts, over the range the route asked for.
func TestHomeStatsGolden(t *testing.T) {
	testutil.Golden(t, "home-stats", sampleStats())
}

// A range with nothing stored is a list of zeroed days, never null: a chart with no history draws a
// flat line rather than nothing at all.
func TestHomeStatsWithNothingStored(t *testing.T) {
	empty := protocol.HomeStats{
		Range:    30,
		From:     homeDay(29),
		To:       homeDay(0),
		Days:     []protocol.HomeStatDay{},
		Projects: []protocol.HomeProjectStats{},
	}
	testutil.Golden(t, "home-stats-empty", empty)
}

// One row of the activity stream and the page the view-all list reads. Every kind of subject is in
// the page: an entry about a card, one that belongs to no project, and one about a scheduled job.
func TestHomeActivityPageGolden(t *testing.T) {
	web := "web"
	api := "api"
	cardEntry := protocol.FeedEntry{
		ID:        "01M3C107JB041061050R3GG28A",
		Kind:      protocol.FeedKindMerge,
		Text:      "#110 Fix flaky login e2e test merged into main",
		ProjectID: &web,
		At:        sampleDate(),
		CardID:    "01M3C107JB041061050R3GG28B",
		CardKey:   "web#110",
	}
	briefEntry := protocol.FeedEntry{
		ID:    "01M3C107JB041061050R3GG28C",
		Kind:  protocol.FeedKindBrief,
		Text:  "Morning brief is ready",
		At:    protocol.NewTimestamp(time.Date(2026, time.September, 30, 9, 0, 0, 0, time.UTC)),
		JobID: "s1",
	}
	ciEntry := protocol.FeedEntry{
		ID:        "01M3C107JB041061050R3GG28D",
		Kind:      protocol.FeedKindCI,
		Text:      "CI passed on marshal/39-slog",
		ProjectID: &api,
		At:        protocol.NewTimestamp(time.Date(2026, time.September, 30, 8, 30, 0, 0, time.UTC)),
		CardID:    "01M3C107JB041061050R3GG28E",
		CardKey:   "api#39",
	}
	page := protocol.Page[protocol.FeedEntry]{
		Items:      []protocol.FeedEntry{cardEntry, briefEntry, ciEntry},
		NextCursor: "eyJzZXEiOjF9",
		ServerTime: sampleDate(),
	}
	testutil.Golden(t, "home-activity", page)
}

// The event that tells a client a row was appended, with the project's own day the row left behind
// it: the whole day, so applying the event twice changes nothing.
func TestActivityCreatedEventDataGolden(t *testing.T) {
	web := "web"
	entry := protocol.FeedEntry{
		ID:        "01M3C107JB041061050R3GG28A",
		Kind:      protocol.FeedKindMerge,
		Text:      "#110 Fix flaky login e2e test merged into main",
		ProjectID: &web,
		At:        sampleDate(),
		CardID:    "01M3C107JB041061050R3GG28B",
		CardKey:   "web#110",
	}
	today := protocol.HomeStatDay{Day: homeDay(0), CardsFinished: 3, Merges: 3}
	testutil.Golden(t, "activity-created", protocol.ActivityCreatedEventData{Entry: entry, Day: &today})
}

// An entry that does not change the numbers carries no day at all, so a client knows there is
// nothing to write into its chart.
func TestActivityCreatedEventDataWithoutADay(t *testing.T) {
	entry := protocol.FeedEntry{
		ID:    "01M3C107JB041061050R3GG28C",
		Kind:  protocol.FeedKindBrief,
		Text:  "Morning brief is ready",
		At:    sampleDate(),
		JobID: "s1",
	}
	testutil.Golden(t, "activity-created-no-day", protocol.ActivityCreatedEventData{Entry: entry, Day: nil})
}
