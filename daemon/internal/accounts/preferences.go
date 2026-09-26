package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The screen preferences that follow a person between devices (decision D2, inventory N30): the
// theme, the List's columns, both tables' sort, and for each project the view it opens in, its
// filters, search, swimlane, folded lanes, and the saved view in use. Layout stays on the device.
// A missing row reads as the defaults, and nothing is stored until something is changed.

const (
	// maxListColumns is the most List columns whose choice is kept.
	maxListColumns = 50
	// maxColumnKeyChars is the longest column or sort key. Keys are the app's own short words, such
	// as "state" or "pkg", so this only stops nonsense.
	maxColumnKeyChars = 24
	// maxProjectsPerChange is the most projects one change can name.
	maxProjectsPerChange = 100
	// maxQueryChars is the longest search text kept for a board.
	maxQueryChars = 200
	// maxCollapsedLanes is the most folded lanes kept for one board.
	maxCollapsedLanes = 100
	// maxLaneChars is the longest folded lane name, written "<swimlane>:<lane>".
	maxLaneChars = 200
)

// Preferences returns the person's preferences. A person who changed nothing has the defaults: the
// system theme, no column choices, no sort, and no project preferences.
func (s *Service) Preferences(ctx context.Context, userID string) (protocol.Preferences, error) {
	if _, err := readUser(ctx, s.store.Queries(), userID); err != nil {
		return protocol.Preferences{}, err
	}
	return readPreferences(ctx, s.store.Queries(), userID)
}

// UpdatePreferences saves what the request sets and leaves the rest alone. Column choices merge by
// key and projects merge by id, so a device that changes one thing never resets another. A request
// that changes nothing answers with the preferences as they are and publishes nothing.
func (s *Service) UpdatePreferences(ctx context.Context, userID string, in protocol.UpdatePreferencesRequest) (protocol.Preferences, error) {
	if err := checkPreferencesUpdate(in); err != nil {
		return protocol.Preferences{}, err
	}
	// The projects and saved views are checked before the write starts: a transaction holds the
	// only writer, and asking the projects module inside it would stall every other write.
	if err := s.checkProjectPreferences(ctx, in.Projects); err != nil {
		return protocol.Preferences{}, err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	var (
		next    protocol.Preferences
		changed bool
	)
	err := s.store.Write(ctx, func(q *db.Queries) error {
		if _, err := readUser(ctx, q, userID); err != nil {
			return err
		}
		current, err := readPreferences(ctx, q, userID)
		if err != nil {
			return err
		}
		if next, err = applyPreferences(current, in); err != nil {
			return err
		}
		if changed = !reflect.DeepEqual(current, next); !changed {
			return nil
		}
		return writePreferences(ctx, q, preferenceChange{userID: userID, before: current, after: next, at: store.Millis(s.now())})
	})
	if err != nil {
		return protocol.Preferences{}, err
	}
	if changed {
		s.log.Info("saved the preferences", "user_id", userID)
		s.publishMe(ctx, userID)
	}
	return next, nil
}

// applyPreferences merges a request into the preferences a person has, as new values: neither the
// preferences nor the request is changed.
func applyPreferences(current protocol.Preferences, in protocol.UpdatePreferencesRequest) (protocol.Preferences, error) {
	next := protocol.Preferences{
		Theme: current.Theme, ListColumns: maps.Clone(current.ListColumns), Sort: current.Sort,
		Projects: maps.Clone(current.Projects),
	}
	if in.Theme != nil {
		next.Theme = *in.Theme
	}
	maps.Copy(next.ListColumns, in.ListColumns)
	if len(next.ListColumns) > maxListColumns {
		return protocol.Preferences{}, protocol.InvalidArgument(fmt.Sprintf(
			"Marshal keeps the choice of at most %d List columns.", maxListColumns))
	}
	if in.Sort != nil {
		if in.Sort.Agents != nil {
			order := *in.Sort.Agents
			next.Sort.Agents = &order
		}
		if in.Sort.List != nil {
			order := *in.Sort.List
			next.Sort.List = &order
		}
	}
	for id, change := range in.Projects {
		base, existed := next.Projects[id]
		if !existed {
			base = defaultProjectPreferences()
		}
		merged := applyProjectPreferences(base, change)
		// A project that had nothing saved and is changed to what the defaults already are has
		// nothing to keep.
		if !existed && reflect.DeepEqual(base, merged) {
			continue
		}
		next.Projects[id] = merged
	}
	return next, nil
}

// applyProjectPreferences merges one project's changes into its preferences, as a new value.
func applyProjectPreferences(base protocol.ProjectPreferences, in protocol.UpdateProjectPreferences) protocol.ProjectPreferences {
	next := base
	if in.LastView != nil {
		next.LastView = *in.LastView
	}
	if in.Filters != nil {
		next.Filters = slices.Clone(*in.Filters)
		if next.Filters == nil {
			next.Filters = []protocol.Filter{}
		}
	}
	if in.Query != nil {
		next.Query = *in.Query
	}
	if in.Swimlane != nil {
		next.Swimlane = *in.Swimlane
	}
	if in.CollapsedLanes != nil {
		next.CollapsedLanes = slices.Clone(*in.CollapsedLanes)
		if next.CollapsedLanes == nil {
			next.CollapsedLanes = []string{}
		}
	}
	if in.ShowAllDone != nil {
		next.ShowAllDone = *in.ShowAllDone
	}
	if in.SavedViewID != nil {
		next.SavedViewID = nil
		if *in.SavedViewID != "" {
			id := *in.SavedViewID
			next.SavedViewID = &id
		}
	}
	return next
}

// preferenceChange is one write of the preferences: whose, what they were, what they are to be,
// and when. It keeps writePreferences inside the parameter limit.
type preferenceChange struct {
	userID        string
	before, after protocol.Preferences
	at            int64
}

// writePreferences saves what differs between two states of the preferences: the person's own row
// when the theme, the columns, or the sort changed, and each project's row that changed.
func writePreferences(ctx context.Context, q *db.Queries, change preferenceChange) error {
	before, after := change.before, change.after
	if before.Theme != after.Theme || !reflect.DeepEqual(before.ListColumns, after.ListColumns) ||
		!reflect.DeepEqual(before.Sort, after.Sort) {
		columns, err := encodeJSON(after.ListColumns)
		if err != nil {
			return err
		}
		sort, err := encodeJSON(after.Sort)
		if err != nil {
			return err
		}
		err = q.UpsertUserPreferences(ctx, db.UpsertUserPreferencesParams{
			UserID: change.userID, Theme: string(after.Theme), ListColumnsJSON: columns, SortJSON: sort, UpdatedAt: change.at,
		})
		if err != nil {
			return fmt.Errorf("save the preferences of user %s: %w", change.userID, err)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(after.Projects)) {
		was, existed := before.Projects[id]
		if existed && reflect.DeepEqual(was, after.Projects[id]) {
			continue
		}
		if err := writeProjectPreferences(ctx, q, change, id); err != nil {
			return err
		}
	}
	return nil
}

// writeProjectPreferences saves one project's row.
func writeProjectPreferences(ctx context.Context, q *db.Queries, change preferenceChange, projectID string) error {
	prefs := change.after.Projects[projectID]
	filters, err := encodeJSON(prefs.Filters)
	if err != nil {
		return err
	}
	lanes, err := encodeJSON(prefs.CollapsedLanes)
	if err != nil {
		return err
	}
	var showAllDone int64
	if prefs.ShowAllDone {
		showAllDone = 1
	}
	err = q.UpsertProjectPreferences(ctx, db.UpsertProjectPreferencesParams{
		UserID: change.userID, ProjectID: projectID, LastView: string(prefs.LastView), FiltersJSON: filters,
		Query: prefs.Query, Swimlane: string(prefs.Swimlane), CollapsedLanesJSON: lanes,
		ShowAllDone: showAllDone, SavedViewID: prefs.SavedViewID, UpdatedAt: change.at,
	})
	if err != nil {
		return fmt.Errorf("save the preferences of project %s for user %s: %w", projectID, change.userID, err)
	}
	return nil
}

// checkProjectPreferences checks that every project a change names is there, and that every saved
// view it puts in use is one of that project's. A project or a view that is not there is not
// found, as it is anywhere else in the API.
func (s *Service) checkProjectPreferences(ctx context.Context, changes map[string]protocol.UpdateProjectPreferences) error {
	for _, id := range slices.Sorted(maps.Keys(changes)) {
		if !protocol.ValidProjectID(id) {
			return protocol.NotFound("project").With("id", truncate(id))
		}
		if _, err := s.projects.Get(ctx, id); err != nil {
			return err
		}
		view := changes[id].SavedViewID
		if view == nil || *view == "" {
			continue
		}
		if !protocol.ValidID(*view) {
			return protocol.NotFound("saved view").With("id", truncate(*view))
		}
		found, err := s.projects.SavedView(ctx, *view)
		if err != nil {
			return err
		}
		if found.ProjectID != id {
			return protocol.InvalidArgument("That saved view belongs to another project.").With("id", *view)
		}
	}
	return nil
}

// truncate cuts an id a client sent before it is repeated in an answer.
func truncate(id string) string {
	const maxEchoedBytes = 64
	if len(id) > maxEchoedBytes {
		return id[:maxEchoedBytes]
	}
	return id
}

// checkPreferencesUpdate refuses a change that is not allowed and needs no other module to judge:
// a word that is not one the screens know, or a value that is too long or too many.
func checkPreferencesUpdate(in protocol.UpdatePreferencesRequest) error {
	if in.Theme != nil && !in.Theme.Valid() {
		return protocol.InvalidArgument("That is not a theme Marshal knows. Choose light, dark, or system.").
			With("theme", string(*in.Theme))
	}
	if len(in.ListColumns) > maxListColumns {
		return protocol.InvalidArgument(fmt.Sprintf("Marshal keeps the choice of at most %d List columns.", maxListColumns))
	}
	for key := range in.ListColumns {
		if err := checkColumnKey(key); err != nil {
			return err
		}
	}
	if in.Sort != nil {
		for _, order := range []*protocol.SortOrder{in.Sort.Agents, in.Sort.List} {
			if err := checkSortOrder(order); err != nil {
				return err
			}
		}
	}
	return checkProjectUpdates(in.Projects)
}

// checkProjectUpdates refuses a set of project changes that is not allowed, in the order of the
// project ids, so the same request always gets the same sentence.
func checkProjectUpdates(changes map[string]protocol.UpdateProjectPreferences) error {
	if len(changes) > maxProjectsPerChange {
		return protocol.InvalidArgument(fmt.Sprintf("One change can name at most %d projects.", maxProjectsPerChange))
	}
	for _, id := range slices.Sorted(maps.Keys(changes)) {
		if err := checkProjectUpdate(changes[id]); err != nil {
			return err
		}
	}
	return nil
}

// checkProjectUpdate refuses one project's change that is not allowed.
func checkProjectUpdate(in protocol.UpdateProjectPreferences) error {
	if in.LastView != nil && !in.LastView.Valid() {
		return protocol.InvalidArgument("That is not a view Marshal knows. Use chat, agents, board, list, timeline, or calendar.").
			With("lastView", string(*in.LastView))
	}
	if in.Filters != nil {
		if err := protocol.CheckFilters(*in.Filters); err != nil {
			return err
		}
	}
	if in.Query != nil && utf8.RuneCountInString(*in.Query) > maxQueryChars {
		return protocol.InvalidArgument(fmt.Sprintf("A search can have at most %d characters.", maxQueryChars))
	}
	if in.Swimlane != nil && !in.Swimlane.Valid() {
		return protocol.InvalidArgument("That is not a swimlane Marshal knows. Group by role, agent, package, or label, or use none.").
			With("swimlane", string(*in.Swimlane))
	}
	if in.CollapsedLanes != nil {
		return checkLanes(*in.CollapsedLanes)
	}
	return nil
}

func checkLanes(lanes []string) error {
	if len(lanes) > maxCollapsedLanes {
		return protocol.InvalidArgument(fmt.Sprintf("Marshal keeps at most %d folded lanes for a board.", maxCollapsedLanes))
	}
	for _, lane := range lanes {
		if lane == "" || utf8.RuneCountInString(lane) > maxLaneChars {
			return protocol.InvalidArgument(fmt.Sprintf("A folded lane needs a name of at most %d characters.", maxLaneChars))
		}
	}
	return nil
}

// checkSortOrder refuses a sort order that is not allowed. A nil order is not set, so it passes.
func checkSortOrder(order *protocol.SortOrder) error {
	if order == nil {
		return nil
	}
	if err := checkColumnKey(order.Key); err != nil {
		return err
	}
	if !order.Direction.Valid() {
		return protocol.InvalidArgument("A table is sorted asc or desc.").With("direction", string(order.Direction))
	}
	return nil
}

// checkColumnKey refuses a column or sort key that is not a short lower case word. The screen owns
// the columns, so the daemon holds no list of them.
func checkColumnKey(key string) error {
	valid := key != "" && utf8.RuneCountInString(key) <= maxColumnKeyChars
	for _, r := range key {
		valid = valid && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}
	if !valid {
		return protocol.InvalidArgument(fmt.Sprintf(
			"A column key is a short word of lower case letters and digits, at most %d characters.", maxColumnKeyChars)).
			With("key", truncate(key))
	}
	return nil
}

// defaultPreferences are the preferences of a person who changed nothing.
func defaultPreferences() protocol.Preferences {
	return protocol.Preferences{
		Theme: protocol.ThemeSystem, ListColumns: map[string]bool{},
		Projects: map[string]protocol.ProjectPreferences{},
	}
}

// defaultProjectPreferences are a project's preferences before anything was changed.
func defaultProjectPreferences() protocol.ProjectPreferences {
	return protocol.ProjectPreferences{
		LastView: protocol.ProjectViewBoard, Filters: []protocol.Filter{}, Swimlane: protocol.SwimlaneNone,
		CollapsedLanes: []string{},
	}
}

// readPreferences reads a person's preferences and every project's, in one answer.
func readPreferences(ctx context.Context, q *db.Queries, userID string) (protocol.Preferences, error) {
	prefs := defaultPreferences()
	row, err := q.GetUserPreferences(ctx, userID)
	switch {
	case err == nil:
		if prefs, err = toPreferences(row); err != nil {
			return protocol.Preferences{}, err
		}
	case !store.IsNotFound(err):
		return protocol.Preferences{}, fmt.Errorf("read the preferences of user %s: %w", userID, err)
	}
	rows, err := q.ListProjectPreferences(ctx, userID)
	if err != nil {
		return protocol.Preferences{}, fmt.Errorf("list the project preferences of user %s: %w", userID, err)
	}
	for _, projectRow := range rows {
		project, err := toProjectPreferences(projectRow)
		if err != nil {
			return protocol.Preferences{}, err
		}
		prefs.Projects[projectRow.ProjectID] = project
	}
	return prefs, nil
}

// toPreferences builds the wire preferences of a person's own row, without any project's.
func toPreferences(row db.UserPreference) (protocol.Preferences, error) {
	prefs := defaultPreferences()
	prefs.Theme = protocol.Theme(row.Theme)
	if err := json.Unmarshal([]byte(row.ListColumnsJSON), &prefs.ListColumns); err != nil {
		return protocol.Preferences{}, fmt.Errorf("read the list columns of user %s: %w", row.UserID, err)
	}
	if prefs.ListColumns == nil {
		prefs.ListColumns = map[string]bool{}
	}
	if err := json.Unmarshal([]byte(row.SortJSON), &prefs.Sort); err != nil {
		return protocol.Preferences{}, fmt.Errorf("read the sort of user %s: %w", row.UserID, err)
	}
	return prefs, nil
}

// toProjectPreferences builds the wire preferences of one project from its row.
func toProjectPreferences(row db.ProjectPreference) (protocol.ProjectPreferences, error) {
	prefs := protocol.ProjectPreferences{
		LastView: protocol.ProjectView(row.LastView), Query: row.Query, Swimlane: protocol.Swimlane(row.Swimlane),
		ShowAllDone: row.ShowAllDone == 1, SavedViewID: row.SavedViewID,
	}
	if err := json.Unmarshal([]byte(row.FiltersJSON), &prefs.Filters); err != nil {
		return protocol.ProjectPreferences{}, fmt.Errorf("read the filters of project %s: %w", row.ProjectID, err)
	}
	if err := json.Unmarshal([]byte(row.CollapsedLanesJSON), &prefs.CollapsedLanes); err != nil {
		return protocol.ProjectPreferences{}, fmt.Errorf("read the folded lanes of project %s: %w", row.ProjectID, err)
	}
	if prefs.Filters == nil {
		prefs.Filters = []protocol.Filter{}
	}
	if prefs.CollapsedLanes == nil {
		prefs.CollapsedLanes = []string{}
	}
	return prefs, nil
}

// encodeJSON turns a value into the JSON the tables store.
func encodeJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode a preference: %w", err)
	}
	return string(data), nil
}
