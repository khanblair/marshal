package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The Schedules screen's own routes (B8.1, build-plan 8.1). A schedule is a row the daemon makes and
// owns, so the tests drive the list-get-save-delete shape end to end over HTTP the way the screen
// does: make one, read it back, edit it, and remove it.

func morningBrief() protocol.SaveScheduleRequest {
	return protocol.SaveScheduleRequest{
		Project: "All projects",
		Name:    "Morning brief",
		Kind:    "brief",
		Icon:    "sunrise",
		Trigger: "Cron",
		When:    "Every weekday at 08:00",
		Time:    "08:00",
		Days:    []int{1, 2, 3, 4, 5},
		Action:  "Send the brief to the app and Obsidian",
		Enabled: true,
		Missed:  "Run once on wake",
	}
}

func TestScheduleRoutesThroughHTTP(t *testing.T) {
	st := newStack(t)
	const route = "/v1/schedules"

	// A daemon nobody has scheduled anything on answers an empty list, stamped with its time.
	empty := decode[protocol.ScheduleList](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK))
	if len(empty.Schedules) != 0 {
		t.Fatalf("an empty daemon answered %+v, want no schedules", empty.Schedules)
	}
	if empty.ServerTime.Time().IsZero() {
		t.Error("the list came back without the daemon's time")
	}

	// Making one hands back the row the daemon stored, with an id of its own.
	created := decode[protocol.Schedule](t, st.do(http.MethodPost, route, morningBrief()).want(t, http.StatusOK))
	if !protocol.ValidID(created.ID) {
		t.Fatalf("the daemon made schedule id %q, which is not an opaque id", created.ID)
	}
	if created.Name != "Morning brief" || !created.Enabled || created.When != "Every weekday at 08:00" {
		t.Errorf("the created schedule = %+v", created)
	}
	if len(created.Days) != 5 || created.Missed != "Run once on wake" {
		t.Errorf("the created schedule lost a field: %+v", created)
	}

	// It is in the list now.
	list := decode[protocol.ScheduleList](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK))
	if len(list.Schedules) != 1 || list.Schedules[0].ID != created.ID {
		t.Fatalf("the list = %+v, want the one schedule just made", list.Schedules)
	}

	// Editing writes the id in the address, whatever the body says about an id.
	edited := morningBrief()
	edited.ID = "01H1234567890ABCDEFGHJKMNP"
	edited.Name = "Morning brief (edited)"
	edited.Enabled = false
	saved := decode[protocol.Schedule](t, st.do(http.MethodPut, route+"/"+created.ID, edited).want(t, http.StatusOK))
	if saved.ID != created.ID || saved.Name != "Morning brief (edited)" || saved.Enabled {
		t.Errorf("the edited schedule = %+v, want the id from the address and the new name", saved)
	}
	if list := decode[protocol.ScheduleList](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK)); len(list.Schedules) != 1 {
		t.Fatalf("editing left %d schedules, want 1", len(list.Schedules))
	}

	// Deleting removes it, and the list is empty again.
	st.do(http.MethodDelete, route+"/"+created.ID, nil).want(t, http.StatusNoContent)
	if list := decode[protocol.ScheduleList](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK)); len(list.Schedules) != 0 {
		t.Errorf("deleting left %+v, want none", list.Schedules)
	}
}

// TestScheduleRoutesFilterByProject proves ?project= narrows the list: a brief that spans every
// project is not filed under one.
func TestScheduleRoutesFilterByProject(t *testing.T) {
	st := newStack(t)
	const route = "/v1/schedules"

	everywhere := decode[protocol.Schedule](t, st.do(http.MethodPost, route, morningBrief()).want(t, http.StatusOK))
	oneProject := morningBrief()
	oneProject.Project = "small-repo"
	oneProject.Name = "Check open issues"
	inProject := decode[protocol.Schedule](t, st.do(http.MethodPost, route, oneProject).want(t, http.StatusOK))

	filtered := decode[protocol.ScheduleList](t,
		st.do(http.MethodGet, route+"?project=small-repo", nil).want(t, http.StatusOK))
	if len(filtered.Schedules) != 1 || filtered.Schedules[0].ID != inProject.ID {
		t.Errorf("the project's list = %+v, want only its own schedule", filtered.Schedules)
	}
	if everywhere.ID == inProject.ID {
		t.Fatal("the two schedules came back with the same id")
	}
}

// TestScheduleRoutesRefuseNonsense proves a row no screen could draw is refused rather than stored,
// and that an unknown id is not found, never a half-written row.
func TestScheduleRoutesRefuseNonsense(t *testing.T) {
	st := newStack(t)
	const route = "/v1/schedules"

	noName := morningBrief()
	noName.Name = "   "
	st.do(http.MethodPost, route, noName).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	badKind := morningBrief()
	badKind.Kind = "chore"
	st.do(http.MethodPost, route, badKind).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	badTrigger := morningBrief()
	badTrigger.Trigger = "Whenever"
	st.do(http.MethodPost, route, badTrigger).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	// Nothing was stored by any of the refused saves.
	if list := decode[protocol.ScheduleList](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK)); len(list.Schedules) != 0 {
		t.Errorf("a refused save stored something: %+v", list.Schedules)
	}

	// An id Marshal has no row for is not found, and an id that is not the right shape is not found
	// too - the two are deliberately the same answer.
	unknown := "01H1234567890ABCDEFGHJKMNP"
	st.do(http.MethodPut, route+"/"+unknown, morningBrief()).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPut, route+"/s1", morningBrief()).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

func TestTheScheduleCatalogListsStartersPartsAndChats(t *testing.T) {
	st := newStack(t)
	catalog := decode[protocol.ScheduleCatalog](t, st.do(http.MethodGet, "/v1/schedules/catalog", nil).want(t, http.StatusOK))
	if len(catalog.Templates) != 7 || len(catalog.Sections) == 0 || len(catalog.Channels) != 3 {
		t.Fatalf("the catalog = %d templates, %d sections, %d channels", len(catalog.Templates), len(catalog.Sections), len(catalog.Channels))
	}
	if catalog.Templates[0].Key != "morning" || len(catalog.Templates[0].Sections) == 0 {
		t.Errorf("the first starter is %+v", catalog.Templates[0])
	}
	if catalog.ServerTime.Time().IsZero() {
		t.Error("the catalog came back without the daemon's time")
	}
}

func TestAScheduleKeepsItsStarterPartsAndChatsOverHTTP(t *testing.T) {
	st := newStack(t)
	req := morningBrief()
	req.Template, req.Sections, req.Deliver, req.QuietWhenEmpty = "morning", []string{"calendar", "needs-you"}, []string{"telegram"}, true
	created := decode[protocol.Schedule](t, st.do(http.MethodPost, "/v1/schedules", req).want(t, http.StatusOK))
	if created.Template != "morning" || len(created.Sections) != 2 || len(created.Deliver) != 1 || !created.QuietWhenEmpty {
		t.Fatalf("the created schedule lost a brief field: %+v", created)
	}
}

func TestAScheduleNamingAnUnknownStarterPartOrChatIsRefused(t *testing.T) {
	st := newStack(t)
	for name, mutate := range map[string]func(*protocol.SaveScheduleRequest){
		"template": func(r *protocol.SaveScheduleRequest) { r.Template = "nope" },
		"section":  func(r *protocol.SaveScheduleRequest) { r.Sections = []string{"calendar", "nope"} },
		"channel":  func(r *protocol.SaveScheduleRequest) { r.Deliver = []string{"fax"} },
	} {
		req := morningBrief()
		mutate(&req)
		st.do(http.MethodPost, "/v1/schedules", req).want(t, http.StatusBadRequest)
		t.Log("refused an unknown", name)
	}
}

func TestRunningAScheduleNowRecordsAndAnswersTheRun(t *testing.T) {
	st := newStack(t)
	made := decode[protocol.Schedule](t, st.do(http.MethodPost, "/v1/schedules", morningBrief()).want(t, http.StatusOK))
	run := decode[protocol.ScheduleRun](t, st.do(http.MethodPost, "/v1/schedules/"+made.ID+"/run", nil).want(t, http.StatusOK))
	if run.ID == "" || run.Status == "" || !strings.Contains(run.Details, "Run by hand.") {
		t.Fatalf("the run answered = %+v", run)
	}
	runs := decode[protocol.ScheduleRunList](t, st.do(http.MethodGet, "/v1/schedules/"+made.ID+"/runs", nil).want(t, http.StatusOK))
	if len(runs.Runs) != 1 || runs.Runs[0].ID != run.ID {
		t.Fatalf("the history = %+v, want the one run just made", runs.Runs)
	}
	st.do(http.MethodPost, "/v1/schedules/01H1234567890ABCDEFGHJKMNP/run", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

func TestPreviewingAScheduleNothingCanPreviewIsRefused(t *testing.T) {
	st := newStack(t)
	made := decode[protocol.Schedule](t, st.do(http.MethodPost, "/v1/schedules", morningBrief()).want(t, http.StatusOK))
	st.do(http.MethodGet, "/v1/schedules/"+made.ID+"/preview", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	st.do(http.MethodGet, "/v1/schedules/01H1234567890ABCDEFGHJKMNP/preview", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}
