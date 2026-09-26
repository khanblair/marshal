package projects_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

var needsFilter = []protocol.Filter{{Key: protocol.FilterKeyStatus, Value: "needs"}}

// A saved view is saved, listed, changed, and deleted, and every change is published with the
// project's whole list, so a client's chips are never stale.
func TestSavedViewsLifecycle(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	e.drainEvents()
	ctx := context.Background()
	topic := protocol.ProjectTopic(project.ID)

	empty, err := e.svc.SavedViews(ctx, project.ID)
	if err != nil || empty.Views == nil || len(empty.Views) != 0 || empty.ProjectID != project.ID {
		t.Fatalf("SavedViews of a new project = %+v, %v; want an empty list that is not null", empty, err)
	}

	created, isNew, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{
		Name: "  Needs me  ", Filters: needsFilter, Swimlane: protocol.SwimlaneRole,
	})
	if err != nil || !isNew {
		t.Fatalf("CreateSavedView = %+v, new %v, %v", created, isNew, err)
	}
	if created.Name != "Needs me" || created.ProjectID != project.ID || created.ID == "" ||
		created.Swimlane != protocol.SwimlaneRole || !reflect.DeepEqual(created.Filters, needsFilter) {
		t.Errorf("view = %+v", created)
	}
	ev := e.nextType(t, protocol.EventTypeSavedViewUpdated, topic)
	data, ok := ev.Data.(protocol.SavedViewUpdatedEventData)
	if !ok || !ev.Critical || data.ProjectID != project.ID || len(data.Views) != 1 || data.Views[0].ID != created.ID {
		t.Errorf("event = %+v (critical %v), want the project's one view and a critical event", ev.Data, ev.Critical)
	}

	// No filters and no swimlane is a view of the whole board, with an empty list of filters.
	all, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "All cards"})
	if err != nil {
		t.Fatalf("CreateSavedView with nothing set: %v", err)
	}
	if all.Swimlane != protocol.SwimlaneNone || all.Filters == nil || len(all.Filters) != 0 {
		t.Errorf("view = %+v, want no swimlane and an empty list of filters", all)
	}
	e.nextType(t, protocol.EventTypeSavedViewUpdated, topic)

	got, err := e.svc.SavedView(ctx, created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Errorf("SavedView = %+v, %v; want %+v", got, err, created)
	}

	name, swim := "Needs me now", protocol.SwimlaneLabel
	filters := []protocol.Filter{}
	updated, err := e.svc.UpdateSavedView(ctx, created.ID, protocol.UpdateSavedViewRequest{
		Name: &name, Filters: &filters, Swimlane: &swim,
	})
	if err != nil {
		t.Fatalf("UpdateSavedView: %v", err)
	}
	if updated.Name != name || len(updated.Filters) != 0 || updated.Swimlane != swim || updated.CreatedAt != created.CreatedAt {
		t.Errorf("view = %+v", updated)
	}
	e.nextType(t, protocol.EventTypeSavedViewUpdated, topic)

	// Changing one field leaves the others as they were.
	swim = protocol.SwimlaneAgent
	again, err := e.svc.UpdateSavedView(ctx, created.ID, protocol.UpdateSavedViewRequest{Swimlane: &swim})
	if err != nil || again.Name != name || again.Swimlane != swim {
		t.Errorf("UpdateSavedView of the swimlane only = %+v, %v", again, err)
	}
	e.nextType(t, protocol.EventTypeSavedViewUpdated, topic)

	// An empty body answers with the view as it is, and publishes nothing.
	same, err := e.svc.UpdateSavedView(ctx, created.ID, protocol.UpdateSavedViewRequest{})
	if err != nil || !reflect.DeepEqual(same, again) {
		t.Errorf("UpdateSavedView with nothing set = %+v, %v; want %+v", same, err, again)
	}
	e.noEvent(t)

	if err := e.svc.DeleteSavedView(ctx, created.ID); err != nil {
		t.Fatalf("DeleteSavedView: %v", err)
	}
	after, err := e.svc.SavedViews(ctx, project.ID)
	if err != nil || len(after.Views) != 1 || after.Views[0].ID != all.ID {
		t.Errorf("SavedViews after the delete = %+v, %v; want only the other view", after, err)
	}
	ev = e.nextType(t, protocol.EventTypeSavedViewUpdated, topic)
	if data, ok := ev.Data.(protocol.SavedViewUpdatedEventData); !ok || len(data.Views) != 1 {
		t.Errorf("the delete event = %+v, want the list without the deleted view", ev.Data)
	}
	if _, err := e.svc.SavedView(ctx, created.ID); err == nil {
		t.Error("a deleted view could still be read")
	}
	if err := e.svc.DeleteSavedView(ctx, created.ID); err == nil {
		t.Error("a deleted view could be deleted again")
	}
}

// Saving under a name the project uses replaces that view: it keeps its id and its first-saved
// time, takes the new filters, and moves to the end of the list, as the board's Save view does.
// The name is matched without regard to case.
func TestSavingUnderAUsedNameReplacesTheView(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	first, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "Needs me", Filters: needsFilter})
	if err != nil {
		t.Fatalf("CreateSavedView: %v", err)
	}
	other, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "By agent", Swimlane: protocol.SwimlaneAgent})
	if err != nil {
		t.Fatalf("CreateSavedView: %v", err)
	}
	e.drainEvents()

	replaced, isNew, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{
		Name: "needs ME", Swimlane: protocol.SwimlanePackage,
	})
	if err != nil || isNew {
		t.Fatalf("CreateSavedView under a used name = %+v, new %v, %v", replaced, isNew, err)
	}
	if replaced.ID != first.ID || replaced.CreatedAt != first.CreatedAt || replaced.Name != "needs ME" ||
		replaced.Swimlane != protocol.SwimlanePackage || len(replaced.Filters) != 0 {
		t.Errorf("view = %+v, want the first view's id and time with the new name, swimlane, and no filters", replaced)
	}
	list, err := e.svc.SavedViews(ctx, project.ID)
	if err != nil || len(list.Views) != 2 || list.Views[0].ID != other.ID || list.Views[1].ID != first.ID {
		t.Errorf("list = %+v, %v; want the other view first and the replaced one last", list.Views, err)
	}
	e.nextType(t, protocol.EventTypeSavedViewUpdated, protocol.ProjectTopic(project.ID))
}

// Two views of a project cannot have one name, whether a view is renamed onto it or saved under it.
// Another project may use the same name.
func TestSavedViewNamesAreUniqueInAProject(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	if _, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "Needs me"}); err != nil {
		t.Fatalf("CreateSavedView: %v", err)
	}
	second, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "By agent"})
	if err != nil {
		t.Fatalf("CreateSavedView: %v", err)
	}
	for _, taken := range []string{"Needs me", "needs me", "  NEEDS ME  "} {
		_, err := e.svc.UpdateSavedView(ctx, second.ID, protocol.UpdateSavedViewRequest{Name: &taken})
		perr := wantCode(t, err, protocol.ErrorCodeConflict)
		if perr.Message != "This project already has a saved view with that name." {
			t.Errorf("message = %q", perr.Message)
		}
	}
	// A view may keep its own name, in another spelling.
	own := "BY AGENT"
	if _, err := e.svc.UpdateSavedView(ctx, second.ID, protocol.UpdateSavedViewRequest{Name: &own}); err != nil {
		t.Errorf("a view could not be renamed to its own name in capitals: %v", err)
	}
	other := e.folder(t, "monorepo")
	if _, isNew, err := e.svc.CreateSavedView(ctx, other.ID, protocol.CreateSavedViewRequest{Name: "Needs me"}); err != nil || !isNew {
		t.Errorf("CreateSavedView in another project = new %v, %v; want a new view", isNew, err)
	}
}

// The list keeps the order the views were last saved in, and a view is not visible in another
// project's list.
func TestSavedViewsAreListedInTheOrderTheyWereSaved(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	other := e.folder(t, "monorepo")
	ctx := context.Background()
	var ids []string
	for _, name := range []string{"one", "two", "three"} {
		view, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: name})
		if err != nil {
			t.Fatalf("CreateSavedView %s: %v", name, err)
		}
		ids = append(ids, view.ID)
	}
	if _, _, err := e.svc.CreateSavedView(ctx, other.ID, protocol.CreateSavedViewRequest{Name: "elsewhere"}); err != nil {
		t.Fatalf("CreateSavedView: %v", err)
	}
	// Changing the first moves it to the end.
	swim := protocol.SwimlaneRole
	if _, err := e.svc.UpdateSavedView(ctx, ids[0], protocol.UpdateSavedViewRequest{Swimlane: &swim}); err != nil {
		t.Fatalf("UpdateSavedView: %v", err)
	}
	list, err := e.svc.SavedViews(ctx, project.ID)
	if err != nil {
		t.Fatalf("SavedViews: %v", err)
	}
	var got []string
	for _, view := range list.Views {
		got = append(got, view.ID)
	}
	if want := []string{ids[1], ids[2], ids[0]}; !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// A project holds at most 50 saved views, and a view that replaces another does not count as one
// more.
func TestAProjectHoldsAtMostFiftySavedViews(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	for i := range 50 {
		if _, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: fmt.Sprintf("view %d", i)}); err != nil {
			t.Fatalf("CreateSavedView %d: %v", i, err)
		}
	}
	_, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "one too many"})
	perr := wantCode(t, err, protocol.ErrorCodeRefused)
	if perr.Message != "A project can have at most 50 saved views. Delete one and try again." {
		t.Errorf("message = %q", perr.Message)
	}
	if _, isNew, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "VIEW 7"}); err != nil || isNew {
		t.Errorf("saving over a view when the project is full = new %v, %v; want a replace", isNew, err)
	}
}

// Each refusal is a plain sentence, and nothing is changed by it.
func TestSavedViewRefusals(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	view, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "Needs me", Filters: needsFilter})
	if err != nil {
		t.Fatalf("CreateSavedView: %v", err)
	}
	e.drainEvents()
	blank, long := "   ", strings.Repeat("x", 61)
	badSwim := protocol.Swimlane("owner")
	badFilters := []protocol.Filter{{Key: "cost", Value: "1"}}
	const knownSwimlanes = "That is not a swimlane Marshal knows. Group by role, agent, package, or label, or use none."
	const unknownFilter = "That is not a filter Marshal knows. Filter by status, role, agent, model, label, or package."

	tests := []struct {
		name string
		call func() error
		code protocol.ErrorCode
		text string
	}{
		{"a create with no name", func() error {
			_, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: blank})
			return err
		}, protocol.ErrorCodeInvalidArgument, "Give the view a name."},
		{"a create with a long name", func() error {
			_, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: long})
			return err
		}, protocol.ErrorCodeInvalidArgument, "Saved view names can have at most 60 characters."},
		{"a create with a swimlane that is not one", func() error {
			_, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "ok", Swimlane: badSwim})
			return err
		}, protocol.ErrorCodeInvalidArgument, knownSwimlanes},
		{"a create with a filter that is not one", func() error {
			_, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "ok", Filters: badFilters})
			return err
		}, protocol.ErrorCodeInvalidArgument, unknownFilter},
		{"a create in a project that is not there", func() error {
			_, _, err := e.svc.CreateSavedView(ctx, "no-such-project", protocol.CreateSavedViewRequest{Name: "ok"})
			return err
		}, protocol.ErrorCodeNotFound, "Marshal cannot find that project. It may have been removed."},
		{"the list of a project that is not there", func() error {
			_, err := e.svc.SavedViews(ctx, "no-such-project")
			return err
		}, protocol.ErrorCodeNotFound, "Marshal cannot find that project. It may have been removed."},
		{"an update with a blank name", func() error {
			_, err := e.svc.UpdateSavedView(ctx, view.ID, protocol.UpdateSavedViewRequest{Name: &blank})
			return err
		}, protocol.ErrorCodeInvalidArgument, "Give the view a name."},
		{"an update with a swimlane that is not one", func() error {
			_, err := e.svc.UpdateSavedView(ctx, view.ID, protocol.UpdateSavedViewRequest{Swimlane: &badSwim})
			return err
		}, protocol.ErrorCodeInvalidArgument, knownSwimlanes},
		{"an update with a filter that is not one", func() error {
			_, err := e.svc.UpdateSavedView(ctx, view.ID, protocol.UpdateSavedViewRequest{Filters: &badFilters})
			return err
		}, protocol.ErrorCodeInvalidArgument, unknownFilter},
		{"an update of a view that is not there", func() error {
			name := "renamed"
			_, err := e.svc.UpdateSavedView(ctx, "01M3C107JB041061050R3GG28A", protocol.UpdateSavedViewRequest{Name: &name})
			return err
		}, protocol.ErrorCodeNotFound, "Marshal cannot find that saved view. It may have been removed."},
		{"a read of a view that is not there", func() error {
			_, err := e.svc.SavedView(ctx, "01M3C107JB041061050R3GG28A")
			return err
		}, protocol.ErrorCodeNotFound, "Marshal cannot find that saved view. It may have been removed."},
		{"an empty update of a view that is not there", func() error {
			_, err := e.svc.UpdateSavedView(ctx, "01M3C107JB041061050R3GG28A", protocol.UpdateSavedViewRequest{})
			return err
		}, protocol.ErrorCodeNotFound, "Marshal cannot find that saved view. It may have been removed."},
		{"a delete of a view that is not there", func() error {
			return e.svc.DeleteSavedView(ctx, "01M3C107JB041061050R3GG28A")
		}, protocol.ErrorCodeNotFound, "Marshal cannot find that saved view. It may have been removed."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			perr := wantCode(t, tc.call(), tc.code)
			if perr.Message != tc.text {
				t.Errorf("message = %q, want %q", perr.Message, tc.text)
			}
		})
	}
	e.noEvent(t)
	if got, err := e.svc.SavedView(ctx, view.ID); err != nil || !reflect.DeepEqual(got, view) {
		t.Errorf("the view after the refusals = %+v, %v; want it unchanged", got, err)
	}
}

// A project that is removed takes its saved views with it.
func TestRemovingAProjectRemovesItsSavedViews(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	view, _, err := e.svc.CreateSavedView(ctx, project.ID, protocol.CreateSavedViewRequest{Name: "Needs me"})
	if err != nil {
		t.Fatalf("CreateSavedView: %v", err)
	}
	if err := e.svc.Remove(ctx, project.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := e.svc.SavedView(ctx, view.ID); err == nil {
		t.Error("the saved view outlived its project")
	}
}
