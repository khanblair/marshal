package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/dashboard"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The two Home routes (docs/architecture.md 11.1, docs/backend-checklist.md B2.3). The first is the
// dashboard answer, whose `range` says how many days the charts cover; the second is the paged,
// filterable view-all list behind Recent activity. Both read stored state and never scan every card:
// the charts are `daily_stats` and the list is `activity`.

// home is GET /v1/home/dashboard: the cards that wait on a person, the cards with a running
// session, the tile counts, and the stored numbers the charts draw over `range` days. The range is
// optional and defaults to seven days.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	days, err := homeRange(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	snapshot, err := s.dashboard.Home(r.Context(), days)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, snapshot)
}

// homeActivity is GET /v1/home/activity: the activity stream, newest first, paged with `limit` and
// `cursor` and narrowed by `kind` and `project` when they are given. A kind that is not one of the
// fixed list, or a project id that cannot exist, is an invalid_argument error, so a client that
// misspells one is told instead of shown nothing.
func (s *Server) homeActivity(w http.ResponseWriter, r *http.Request) {
	filter, err := homeActivityFilter(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	// The paging is the same as the card history's: a page size and a sequence the last page ended
	// at, with the cursor opaque to the client. The helpers are shared so the two lists cannot
	// page differently.
	limit, position, err := readPage(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	page, err := s.dashboard.Activity(r.Context(), filter, position, limit)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	answer, err := historyPage(page.Items, page.Cursor, page.More, s.now())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// homeRange reads the optional `range` parameter of the dashboard route. Empty means the seven-day
// chart, and anything else must be one of the three ranges the charts cover.
func homeRange(r *http.Request) (int, error) {
	text := r.URL.Query().Get("range")
	if text == "" {
		return dashboard.RangeWeek, nil
	}
	days, err := strconv.Atoi(text)
	if err != nil || !dashboard.ValidRange(days) {
		return 0, protocol.InvalidArgument("The chart range must be " + homeRangeNames() +
			" days. Choose one of those and try again.")
	}
	return days, nil
}

// homeRangeNames lists the ranges the charts cover, in the order of the fixed list, for the sentence
// a refused range gets.
func homeRangeNames() string {
	ranges := dashboard.HomeRanges()
	names := make([]string, len(ranges))
	for i, days := range ranges {
		names[i] = strconv.Itoa(days)
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
}

// homeActivityFilter reads the optional `kind` and `project` parameters of the activity route.
// Empty means every value.
func homeActivityFilter(r *http.Request) (dashboard.ActivityFilter, error) {
	var filter dashboard.ActivityFilter
	query := r.URL.Query()
	if text := query.Get("kind"); text != "" {
		kind := protocol.FeedKind(text)
		if !kind.Valid() {
			return filter, protocol.InvalidArgument("That is not a kind of activity. Choose one of: " +
				strings.Join(feedKindNames(), ", ") + ".")
		}
		filter.Kind = kind
	}
	if text := query.Get("project"); text != "" {
		if !protocol.ValidProjectID(text) {
			return filter, protocol.InvalidArgument("That is not a project id. Choose one of the projects on Home and try again.")
		}
		filter.ProjectID = text
	}
	return filter, nil
}

// feedKindNames lists the kinds an activity filter takes, in the order of the fixed list, for the
// sentence a refused filter gets.
func feedKindNames() []string {
	kinds := protocol.FeedKindValues()
	names := make([]string, len(kinds))
	for i, kind := range kinds {
		names[i] = string(kind)
	}
	return names
}
