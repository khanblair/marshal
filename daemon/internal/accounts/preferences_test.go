package accounts_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

var statusNeeds = []protocol.Filter{{Key: protocol.FilterKeyStatus, Value: "needs"}}

// A person who changed nothing has the system theme, no column choices, no sort, and no project
// preferences, and none of them are lists that are null.
func TestPreferencesOfANewPerson(t *testing.T) {
	e := newEnv(t)
	got, err := e.svc.Preferences(context.Background(), e.userID)
	if err != nil {
		t.Fatalf("Preferences: %v", err)
	}
	want := protocol.Preferences{
		Theme: protocol.ThemeSystem, ListColumns: map[string]bool{}, Projects: map[string]protocol.ProjectPreferences{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("preferences = %+v, want %+v", got, want)
	}
}

// The theme, the List columns, and the two tables' sort are saved and published. Columns merge by
// key and sorts merge by table, so one device changing one thing never resets another.
func TestUserPreferencesMerge(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	got, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{
		Theme: ptr(protocol.ThemeDark), ListColumns: map[string]bool{"pkg": true, "cost": false},
		Sort: &protocol.UpdateSortPreferences{List: &protocol.SortOrder{Key: "id", Direction: protocol.SortDirectionDesc}},
	})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if got.Theme != protocol.ThemeDark || !reflect.DeepEqual(got.ListColumns, map[string]bool{"pkg": true, "cost": false}) ||
		got.Sort.List == nil || got.Sort.List.Key != "id" || got.Sort.Agents != nil {
		t.Fatalf("preferences = %+v", got)
	}
	if me := e.nextMe(t); !reflect.DeepEqual(me.Preferences, got) {
		t.Errorf("the event's preferences = %+v, want %+v", me.Preferences, got)
	}

	next, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{
		ListColumns: map[string]bool{"think": true, "cost": true},
		Sort:        &protocol.UpdateSortPreferences{Agents: &protocol.SortOrder{Key: "state", Direction: protocol.SortDirectionAsc}},
	})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if next.Theme != protocol.ThemeDark {
		t.Errorf("the theme = %s, want dark kept", next.Theme)
	}
	if want := map[string]bool{"pkg": true, "cost": true, "think": true}; !reflect.DeepEqual(next.ListColumns, want) {
		t.Errorf("columns = %v, want %v", next.ListColumns, want)
	}
	if next.Sort.List == nil || next.Sort.List.Direction != protocol.SortDirectionDesc || next.Sort.Agents == nil {
		t.Errorf("sort = %+v, want the list's kept and the agents' set", next.Sort)
	}
	e.nextMe(t)

	e.restart(t)
	if again, err := e.svc.Preferences(ctx, e.userID); err != nil || !reflect.DeepEqual(again, next) {
		t.Errorf("after a restart the preferences = %+v, %v; want %+v", again, err, next)
	}
}

// A project's preferences merge by field: the view it opens in, its filters, search, swimlane, folded
// lanes, whether Done shows every card, and the saved view in use. A project that was not named keeps
// what it had.
func TestProjectPreferencesMerge(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	api, web := e.project(t, "small-repo"), e.project(t, "monorepo")
	view := e.savedView(t, api.ID, "Needs me")
	e.drainEvents()

	first, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{Projects: map[string]protocol.UpdateProjectPreferences{
		api.ID: {LastView: ptr(protocol.ProjectViewList), Filters: &statusNeeds, Swimlane: ptr(protocol.SwimlaneRole)},
		web.ID: {LastView: ptr(protocol.ProjectViewTimeline)},
	}})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if got := first.Projects[api.ID]; got.LastView != protocol.ProjectViewList || !reflect.DeepEqual(got.Filters, statusNeeds) ||
		got.Swimlane != protocol.SwimlaneRole || got.Query != "" || got.ShowAllDone || got.SavedViewID != nil || got.CollapsedLanes == nil {
		t.Errorf("the first project = %+v", got)
	}
	e.nextMe(t)

	lanes := []string{"role:Tester"}
	second, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{Projects: map[string]protocol.UpdateProjectPreferences{
		api.ID: {Query: ptr("cache"), CollapsedLanes: &lanes, ShowAllDone: ptr(true), SavedViewID: &view.ID},
	}})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	got := second.Projects[api.ID]
	if got.LastView != protocol.ProjectViewList || !reflect.DeepEqual(got.Filters, statusNeeds) || got.Swimlane != protocol.SwimlaneRole {
		t.Errorf("the first change was lost: %+v", got)
	}
	if got.Query != "cache" || !reflect.DeepEqual(got.CollapsedLanes, lanes) || !got.ShowAllDone || got.SavedViewID == nil || *got.SavedViewID != view.ID {
		t.Errorf("the second change is not there: %+v", got)
	}
	if second.Projects[web.ID].LastView != protocol.ProjectViewTimeline {
		t.Errorf("a project that was not named lost its view: %+v", second.Projects[web.ID])
	}
	e.nextMe(t)

	// Filters and folded lanes can be emptied, and the saved view in use is cleared with "".
	none := []protocol.Filter{}
	noLanes := []string{}
	third, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{Projects: map[string]protocol.UpdateProjectPreferences{
		api.ID: {Filters: &none, CollapsedLanes: &noLanes, SavedViewID: ptr("")},
	}})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if got := third.Projects[api.ID]; len(got.Filters) != 0 || got.Filters == nil || len(got.CollapsedLanes) != 0 || got.CollapsedLanes == nil || got.SavedViewID != nil {
		t.Errorf("the emptied project = %+v", got)
	}
	e.nextMe(t)

	e.restart(t)
	if again, err := e.svc.Preferences(ctx, e.userID); err != nil || !reflect.DeepEqual(again, third) {
		t.Errorf("after a restart the preferences = %+v, %v; want %+v", again, err, third)
	}
}

// A change that changes nothing publishes nothing, and a project whose preferences are still the
// defaults is not stored.
func TestPreferencesThatChangeNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	e.drainEvents()
	for name, in := range map[string]protocol.UpdatePreferencesRequest{
		"an empty body":         {},
		"the theme it has":      {Theme: ptr(protocol.ThemeSystem)},
		"an empty column set":   {ListColumns: map[string]bool{}},
		"an empty sort":         {Sort: &protocol.UpdateSortPreferences{}},
		"an empty project list": {Projects: map[string]protocol.UpdateProjectPreferences{}},
		"a project at defaults": {Projects: map[string]protocol.UpdateProjectPreferences{
			project.ID: {LastView: ptr(protocol.ProjectViewBoard), Swimlane: ptr(protocol.SwimlaneNone), ShowAllDone: ptr(false)},
		}},
	} {
		got, err := e.svc.UpdatePreferences(ctx, e.userID, in)
		if err != nil || len(got.Projects) != 0 || got.Theme != protocol.ThemeSystem {
			t.Errorf("%s: %+v, %v; want the defaults, with no project stored", name, got, err)
		}
	}
	e.noEvent(t)

	// The same change twice is one event.
	in := protocol.UpdatePreferencesRequest{Theme: ptr(protocol.ThemeLight)}
	if _, err := e.svc.UpdatePreferences(ctx, e.userID, in); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	e.nextMe(t)
	if _, err := e.svc.UpdatePreferences(ctx, e.userID, in); err != nil {
		t.Fatalf("UpdatePreferences again: %v", err)
	}
	e.noEvent(t)
}

// A person's preferences are theirs: another person keeps the defaults.
func TestPreferencesArePerPerson(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	other := e.secondUser(t, "Sam Reyes")
	if _, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{
		Theme:    ptr(protocol.ThemeDark),
		Projects: map[string]protocol.UpdateProjectPreferences{project.ID: {LastView: ptr(protocol.ProjectViewList)}},
	}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	got, err := e.svc.Preferences(ctx, other)
	if err != nil || got.Theme != protocol.ThemeSystem || len(got.Projects) != 0 {
		t.Errorf("the other person's preferences = %+v, %v; want the defaults", got, err)
	}
}

// A saved view that is deleted leaves no view in use, and a project that is removed leaves no
// preferences: the database clears both, so no id points at nothing. A client learns of the first
// from saved_view.updated.
func TestPreferencesFollowTheirSavedViewsAndProjects(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project, gone := e.project(t, "small-repo"), e.project(t, "monorepo")
	view := e.savedView(t, project.ID, "Needs me")
	if _, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{Projects: map[string]protocol.UpdateProjectPreferences{
		project.ID: {SavedViewID: &view.ID, Swimlane: ptr(protocol.SwimlaneAgent)},
		gone.ID:    {LastView: ptr(protocol.ProjectViewList)},
	}}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if err := e.projects.DeleteSavedView(ctx, view.ID); err != nil {
		t.Fatalf("DeleteSavedView: %v", err)
	}
	if err := e.projects.Remove(ctx, gone.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, err := e.svc.Preferences(ctx, e.userID)
	if err != nil {
		t.Fatalf("Preferences: %v", err)
	}
	if len(got.Projects) != 1 || got.Projects[project.ID].SavedViewID != nil || got.Projects[project.ID].Swimlane != protocol.SwimlaneAgent {
		t.Errorf("preferences = %+v, want the one project, with its swimlane and no saved view", got.Projects)
	}
}

// The words the screens send are checked: each refusal is a plain sentence, and nothing is saved or
// published by it.
func TestUpdatePreferencesRefusals(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project, other := e.project(t, "small-repo"), e.project(t, "monorepo")
	view := e.savedView(t, other.ID, "Needs me")
	e.drainEvents()
	const (
		badSwimlane = "That is not a swimlane Marshal knows. Group by role, agent, package, or label, or use none."
		badFilter   = "That is not a filter Marshal knows. Filter by status, role, agent, model, label, or package."
		badColumn   = "A column key is a short word of lower case letters and digits, at most 24 characters."
		noProject   = "Marshal cannot find that project. It may have been removed."
		noView      = "Marshal cannot find that saved view. It may have been removed."
	)
	manyColumns := map[string]bool{}
	for i := range 51 {
		manyColumns[fmt.Sprintf("col%d", i)] = true
	}
	manyProjects := map[string]protocol.UpdateProjectPreferences{}
	for i := range 101 {
		manyProjects[fmt.Sprintf("project-%d", i)] = protocol.UpdateProjectPreferences{}
	}
	lanes := func(names ...string) *[]string { return &names }
	on := func(u protocol.UpdateProjectPreferences) protocol.UpdatePreferencesRequest {
		return protocol.UpdatePreferencesRequest{Projects: map[string]protocol.UpdateProjectPreferences{project.ID: u}}
	}
	tests := []struct {
		name string
		in   protocol.UpdatePreferencesRequest
		code protocol.ErrorCode
		text string
	}{
		{"a theme that is not one", protocol.UpdatePreferencesRequest{Theme: ptr(protocol.Theme("sepia"))},
			protocol.ErrorCodeInvalidArgument, "That is not a theme Marshal knows. Choose light, dark, or system."},
		{"a column key in capitals", protocol.UpdatePreferencesRequest{ListColumns: map[string]bool{"Cost": true}}, protocol.ErrorCodeInvalidArgument, badColumn},
		{"an empty column key", protocol.UpdatePreferencesRequest{ListColumns: map[string]bool{"": true}}, protocol.ErrorCodeInvalidArgument, badColumn},
		{"a long column key", protocol.UpdatePreferencesRequest{ListColumns: map[string]bool{strings.Repeat("a", 25): true}}, protocol.ErrorCodeInvalidArgument, badColumn},
		{"too many columns", protocol.UpdatePreferencesRequest{ListColumns: manyColumns}, protocol.ErrorCodeInvalidArgument,
			"Marshal keeps the choice of at most 50 List columns."},
		{"a sort key that is not a word", protocol.UpdatePreferencesRequest{Sort: &protocol.UpdateSortPreferences{
			List: &protocol.SortOrder{Key: "a b", Direction: protocol.SortDirectionAsc}}}, protocol.ErrorCodeInvalidArgument, badColumn},
		{"a sort direction that is not one", protocol.UpdatePreferencesRequest{Sort: &protocol.UpdateSortPreferences{
			Agents: &protocol.SortOrder{Key: "state", Direction: "up"}}}, protocol.ErrorCodeInvalidArgument, "A table is sorted asc or desc."},
		{"too many projects at once", protocol.UpdatePreferencesRequest{Projects: manyProjects}, protocol.ErrorCodeInvalidArgument,
			"One change can name at most 100 projects."},
		{"a view that is not one", on(protocol.UpdateProjectPreferences{LastView: ptr(protocol.ProjectView("kanban"))}), protocol.ErrorCodeInvalidArgument,
			"That is not a view Marshal knows. Use chat, agents, board, list, timeline, or calendar."},
		{"a filter that is not one", on(protocol.UpdateProjectPreferences{Filters: &[]protocol.Filter{{Key: "cost", Value: "1"}}}),
			protocol.ErrorCodeInvalidArgument, badFilter},
		{"a search that is too long", on(protocol.UpdateProjectPreferences{Query: ptr(strings.Repeat("x", 201))}), protocol.ErrorCodeInvalidArgument,
			"A search can have at most 200 characters."},
		{"a swimlane that is not one", on(protocol.UpdateProjectPreferences{Swimlane: ptr(protocol.Swimlane("owner"))}), protocol.ErrorCodeInvalidArgument, badSwimlane},
		{"too many folded lanes", on(protocol.UpdateProjectPreferences{CollapsedLanes: lanes(make([]string, 101)...)}), protocol.ErrorCodeInvalidArgument,
			"Marshal keeps at most 100 folded lanes for a board."},
		{"an empty folded lane", on(protocol.UpdateProjectPreferences{CollapsedLanes: lanes("")}), protocol.ErrorCodeInvalidArgument,
			"A folded lane needs a name of at most 200 characters."},
		{"a project that is not there", protocol.UpdatePreferencesRequest{Projects: map[string]protocol.UpdateProjectPreferences{
			"no-such-project": {LastView: ptr(protocol.ProjectViewList)}}}, protocol.ErrorCodeNotFound, noProject},
		{"a project id that is not the shape of one", protocol.UpdatePreferencesRequest{Projects: map[string]protocol.UpdateProjectPreferences{
			"Not A Project!": {}}}, protocol.ErrorCodeNotFound, noProject},
		{"a saved view that is not there", on(protocol.UpdateProjectPreferences{SavedViewID: ptr("01M3C107JB041061050R3GG28A")}), protocol.ErrorCodeNotFound, noView},
		{"a saved view id that is not the shape of one", on(protocol.UpdateProjectPreferences{SavedViewID: ptr("nope")}), protocol.ErrorCodeNotFound, noView},
		{"a saved view of another project", on(protocol.UpdateProjectPreferences{SavedViewID: &view.ID}), protocol.ErrorCodeInvalidArgument,
			"That saved view belongs to another project."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.UpdatePreferences(ctx, e.userID, tc.in)
			wantCode(t, err, tc.code, tc.text)
		})
	}
	got, _ := e.svc.Preferences(ctx, e.userID)
	if got.Theme != protocol.ThemeSystem || len(got.ListColumns) != 0 || len(got.Projects) != 0 {
		t.Errorf("a refusal changed the preferences: %+v", got)
	}
	e.noEvent(t)
}

// Columns already chosen count toward the limit, so a change cannot grow the set past it one key at
// a time.
func TestTheColumnLimitCountsWhatIsAlreadyChosen(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	columns := map[string]bool{}
	for i := range 50 {
		columns[fmt.Sprintf("col%d", i)] = true
	}
	if _, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{ListColumns: columns}); err != nil {
		t.Fatalf("UpdatePreferences of 50 columns: %v", err)
	}
	e.nextMe(t)
	_, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{ListColumns: map[string]bool{"one-more": true}})
	wantCode(t, err, protocol.ErrorCodeInvalidArgument, "Marshal keeps the choice of at most 50 List columns.")
	// Changing a column that is already chosen is fine.
	if _, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{ListColumns: map[string]bool{"col1": false}}); err != nil {
		t.Errorf("UpdatePreferences of a chosen column: %v", err)
	}
}

// Changes made at the same time are each kept, and the last event a device hears has the state they
// all made, so a device that applies events in order ends where the daemon is.
func TestConcurrentChangesAreAllKeptAndTheLastEventHasThemAll(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const changes = 12
	var wg sync.WaitGroup
	for i := range changes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.UpdatePreferences(ctx, e.userID, protocol.UpdatePreferencesRequest{
				ListColumns: map[string]bool{fmt.Sprintf("col%d", i): true},
			})
			if err != nil {
				t.Errorf("UpdatePreferences %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	final, err := e.svc.Preferences(ctx, e.userID)
	if err != nil || len(final.ListColumns) != changes {
		t.Fatalf("preferences = %+v, %v; want %d columns", final, err, changes)
	}
	var last protocol.MeUpdatedEventData
	for range changes {
		last = e.nextMe(t)
	}
	if !reflect.DeepEqual(last.Preferences, final) {
		t.Errorf("the last event's preferences = %+v, want %+v", last.Preferences, final)
	}
	e.noEvent(t)
}
