package api_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A project's lessons over the API (docs/architecture.md sections 10 and 12, docs/backend-checklist.
// md B7.4, B7.6, build-plan task 7.13). A lesson is a markdown file in the vault the same way a card's
// note is, so these tests read the file back off disk as well as the answer, the same rule
// routes_notes_test.go follows for notes.

func TestLessonRoutesThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	route := "/v1/projects/" + project.ID + "/lessons"
	wantPath := project.ID + "/lessons/ci-is-flaky.md"

	// A project with no lessons yet answers an empty list, never a missing one.
	list := decode[[]protocol.Lesson](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK))
	if len(list) != 0 {
		t.Fatalf("a project with no lessons answered %+v, want none", list)
	}

	// Saving with no slug in the address makes the lesson the first time, and the answer already
	// carries the slug the title was made into.
	body := "# CI is flaky\n\nRetry the flaky step once before failing the run.\n"
	saved := decode[protocol.Lesson](t, st.do(http.MethodPost, route,
		protocol.SaveLessonRequest{Title: "CI is flaky", Body: body}).want(t, http.StatusOK))
	if saved.Slug != "ci-is-flaky" || saved.Path != wantPath || saved.ProjectID != project.ID {
		t.Errorf("the saved lesson = %+v", saved)
	}
	if saved.Author != protocol.NoteAuthorPerson {
		t.Errorf("a lesson saved through this route is authored %q, want person", saved.Author)
	}
	if saved.UpdatedAt.Time().IsZero() {
		t.Error("a saved lesson has no save time")
	}
	onDisk, err := os.ReadFile(filepath.Join(st.vault, filepath.FromSlash(wantPath)))
	if err != nil {
		t.Fatalf("read the lesson file: %v", err)
	}
	if string(onDisk) != body {
		t.Errorf("the vault holds %q, want the body that was saved", onDisk)
	}

	// The lesson is now in the list, and readable by its own slug.
	list = decode[[]protocol.Lesson](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK))
	if len(list) != 1 || list[0].Slug != "ci-is-flaky" {
		t.Fatalf("the list = %+v, want the one lesson just saved", list)
	}
	got := decode[protocol.Lesson](t, st.do(http.MethodGet, route+"/ci-is-flaky", nil).want(t, http.StatusOK))
	if got.Body != body || got.Title != "CI is flaky" {
		t.Errorf("reading by slug answered %+v", got)
	}

	// Saving to the existing slug edits it in place: the same title, so the same slug, one row.
	edited := decode[protocol.Lesson](t, st.do(http.MethodPut, route+"/ci-is-flaky",
		protocol.SaveLessonRequest{Title: "CI is flaky", Body: "edited"}).want(t, http.StatusOK))
	if edited.Slug != "ci-is-flaky" || edited.Body != "edited" {
		t.Errorf("the edited lesson = %+v", edited)
	}
	if list := decode[[]protocol.Lesson](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK)); len(list) != 1 {
		t.Fatalf("editing in place left %d lessons, want 1", len(list))
	}

	// A slug nobody has saved a lesson under is not found, and an empty title is refused.
	st.do(http.MethodGet, route+"/no-such-lesson", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, route, protocol.SaveLessonRequest{Title: "  ", Body: "x"}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	// Deleting removes it outright: gone from the list, gone from the vault.
	st.do(http.MethodDelete, route+"/ci-is-flaky", nil).want(t, http.StatusNoContent)
	if list := decode[[]protocol.Lesson](t, st.do(http.MethodGet, route, nil).want(t, http.StatusOK)); len(list) != 0 {
		t.Errorf("deleting left %+v, want none", list)
	}
	if _, err := os.Stat(filepath.Join(st.vault, filepath.FromSlash(wantPath))); !os.IsNotExist(err) {
		t.Errorf("the lesson file is still there after delete (stat: %v)", err)
	}
}

// A project that is not there answers not found for every one of the four lesson routes, the same
// way a card that is not there does for the note routes.
func TestLessonRoutesOfAProjectThatIsNotThere(t *testing.T) {
	st := newStack(t)
	route := "/v1/projects/no-such-project/lessons"
	st.do(http.MethodGet, route, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPost, route, protocol.SaveLessonRequest{Title: "x", Body: "x"}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodGet, route+"/x", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodPut, route+"/x", protocol.SaveLessonRequest{Title: "x", Body: "x"}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodDelete, route+"/x", nil).want(t, http.StatusNoContent)
}

// A project's lessons stay under its own folder, and one project's lessons are never another's.
func TestLessonsBelongToTheirOwnProject(t *testing.T) {
	st := newStack(t)
	small, _ := st.addProject("small-repo")
	other, _ := st.addProject("monorepo")
	st.do(http.MethodPost, "/v1/projects/"+small.ID+"/lessons",
		protocol.SaveLessonRequest{Title: "Small repo's own lesson", Body: "x"}).want(t, http.StatusOK)

	otherList := decode[[]protocol.Lesson](t,
		st.do(http.MethodGet, "/v1/projects/"+other.ID+"/lessons", nil).want(t, http.StatusOK))
	if len(otherList) != 0 {
		t.Errorf("another project's lessons answered %+v, want none", otherList)
	}
}
