package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A project's saved views (docs/backend-checklist.md B2.5) over the real server, the real store, and
// the real event bus. The rules are tested in internal/projects; these tests are about what the
// routes read, what they answer, and what they announce.

func savedViewsPath(projectID string) string { return "/v1/projects/" + projectID + "/saved-views" }

func TestTheSavedViewRoutesSaveListChangeAndDelete(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	topic := protocol.ProjectTopic(project.ID)
	stream := st.connect(topic)

	// A new project has no views, and the list is empty and not null.
	r := st.do(http.MethodGet, savedViewsPath(project.ID), nil).want(t, http.StatusOK)
	sameShape(t, "saved-view-list", r.Body)
	if list := decode[protocol.SavedViewListSnapshot](t, r); list.Views == nil || len(list.Views) != 0 || list.ProjectID != project.ID {
		t.Fatalf("the list of a new project = %+v", list)
	}

	// Saving a view is 201 with its address, and is announced with the project's whole list.
	filters := []protocol.Filter{{Key: protocol.FilterKeyStatus, Value: "needs"}}
	created := st.do(http.MethodPost, savedViewsPath(project.ID), protocol.CreateSavedViewRequest{
		Name: "Needs me", Filters: filters, Swimlane: protocol.SwimlaneRole,
	}).want(t, http.StatusCreated)
	sameShape(t, "saved-view", created.Body)
	view := decode[protocol.SavedView](t, created)
	if want := "/v1/saved-views/" + view.ID; created.Header.Get("Location") != want {
		t.Errorf("Location = %q, want %q", created.Header.Get("Location"), want)
	}
	if view.Name != "Needs me" || view.Swimlane != protocol.SwimlaneRole || len(view.Filters) != 1 || view.ProjectID != project.ID {
		t.Errorf("view = %+v", view)
	}
	heard := dataOf[protocol.SavedViewUpdatedEventData](t, lastOf(stream.until(ofType(protocol.EventTypeSavedViewUpdated))))
	if heard.ProjectID != project.ID || len(heard.Views) != 1 || heard.Views[0].ID != view.ID {
		t.Errorf("the event = %+v, want the project's one view", heard)
	}

	// Saving under the same name replaces the view and keeps its id, so it is 200 and no Location.
	replaced := st.do(http.MethodPost, savedViewsPath(project.ID), protocol.CreateSavedViewRequest{Name: "needs me"}).
		want(t, http.StatusOK)
	if got := decode[protocol.SavedView](t, replaced); got.ID != view.ID || got.Swimlane != protocol.SwimlaneNone || len(got.Filters) != 0 {
		t.Errorf("the replaced view = %+v, want the same id with no filters and no swimlane", got)
	}
	if replaced.Header.Get("Location") != "" {
		t.Errorf("a replaced view has a Location: %q", replaced.Header.Get("Location"))
	}
	stream.until(ofType(protocol.EventTypeSavedViewUpdated))

	// A change to one view.
	name := "By role"
	patched := st.do(http.MethodPatch, "/v1/saved-views/"+view.ID, protocol.UpdateSavedViewRequest{
		Name: &name, Swimlane: ptr(protocol.SwimlaneAgent),
	}).want(t, http.StatusOK)
	sameShape(t, "saved-view", patched.Body)
	if got := decode[protocol.SavedView](t, patched); got.Name != name || got.Swimlane != protocol.SwimlaneAgent {
		t.Errorf("the changed view = %+v", got)
	}
	stream.until(ofType(protocol.EventTypeSavedViewUpdated))

	// The daemon starts again on the same data, and the views are there.
	st.restart()
	list := decode[protocol.SavedViewListSnapshot](t, st.do(http.MethodGet, savedViewsPath(project.ID), nil).want(t, http.StatusOK))
	if len(list.Views) != 1 || list.Views[0].Name != name {
		t.Errorf("the list after a restart = %+v", list.Views)
	}

	// A restart closes the streams, so the device connects again before the next change.
	stream = st.connect(topic)
	st.do(http.MethodDelete, "/v1/saved-views/"+view.ID, nil).want(t, http.StatusNoContent)
	heard = dataOf[protocol.SavedViewUpdatedEventData](t, lastOf(stream.until(ofType(protocol.EventTypeSavedViewUpdated))))
	if len(heard.Views) != 0 {
		t.Errorf("the delete event still lists %+v", heard.Views)
	}
	st.do(http.MethodDelete, "/v1/saved-views/"+view.ID, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

func lastOf(events []protocol.Event) protocol.Event { return events[len(events)-1] }

func TestTheSavedViewRoutesRefuseWhatIsNotAllowed(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	view := decode[protocol.SavedView](t, st.do(http.MethodPost, savedViewsPath(project.ID),
		protocol.CreateSavedViewRequest{Name: "Needs me"}).want(t, http.StatusCreated))
	second := decode[protocol.SavedView](t, st.do(http.MethodPost, savedViewsPath(project.ID),
		protocol.CreateSavedViewRequest{Name: "By agent"}).want(t, http.StatusCreated))

	// A name with nothing in it, a swimlane that is not one, and a filter that is not one.
	got := st.do(http.MethodPost, savedViewsPath(project.ID), protocol.CreateSavedViewRequest{Name: "  "}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Message != "Give the view a name." {
		t.Errorf("message = %q", got.Message)
	}
	st.do(http.MethodPost, savedViewsPath(project.ID), protocol.CreateSavedViewRequest{Name: "x", Swimlane: "owner"}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, savedViewsPath(project.ID), protocol.CreateSavedViewRequest{
		Name: "x", Filters: []protocol.Filter{{Key: "cost", Value: "1"}},
	}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPost, savedViewsPath(project.ID), `{"name":"x","colour":"red"}`).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	// A rename onto a name another view has is a conflict.
	taken := "needs me"
	conflict := st.do(http.MethodPatch, "/v1/saved-views/"+second.ID, protocol.UpdateSavedViewRequest{Name: &taken}).
		apiError(t, http.StatusConflict, protocol.ErrorCodeConflict)
	if conflict.Message != "This project already has a saved view with that name." {
		t.Errorf("message = %q", conflict.Message)
	}

	// A project or a view that is not there, or an id that cannot be one, is not found.
	st.do(http.MethodGet, savedViewsPath("no-such-project"), nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, savedViewsPath("no-such-project"), protocol.CreateSavedViewRequest{Name: "x"}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	for _, id := range []string{"01M3C107JB041061050R3GG28Z", "nope"} {
		st.do(http.MethodPatch, "/v1/saved-views/"+id, protocol.UpdateSavedViewRequest{}).
			apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
		st.do(http.MethodDelete, "/v1/saved-views/"+id, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	}
	st.do(http.MethodGet, "/v1/saved-views/"+view.ID, nil).apiError(t, http.StatusMethodNotAllowed, protocol.ErrorCodeMethodNotAllowed)

	// A project holds at most 50 views.
	for i := range 48 {
		st.do(http.MethodPost, savedViewsPath(project.ID), protocol.CreateSavedViewRequest{Name: fmt.Sprintf("view %d", i)}).
			want(t, http.StatusCreated)
	}
	full := st.do(http.MethodPost, savedViewsPath(project.ID), protocol.CreateSavedViewRequest{Name: "one too many"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if full.Message != "A project can have at most 50 saved views. Delete one and try again." {
		t.Errorf("message = %q", full.Message)
	}
}

// A project that is removed takes its saved views with it, and its address stops answering.
func TestRemovingAProjectRemovesItsSavedViewsThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	view := decode[protocol.SavedView](t, st.do(http.MethodPost, savedViewsPath(project.ID),
		protocol.CreateSavedViewRequest{Name: "Needs me"}).want(t, http.StatusCreated))
	st.do(http.MethodDelete, "/v1/projects/"+project.ID, nil).want(t, http.StatusNoContent)
	st.do(http.MethodGet, savedViewsPath(project.ID), nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodDelete, "/v1/saved-views/"+view.ID, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}
