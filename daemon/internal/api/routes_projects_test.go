package api_test

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestProjectsThroughHTTP(t *testing.T) {
	st := newStack(t)

	empty := st.do(http.MethodGet, "/v1/projects", nil).want(t, http.StatusOK)
	sameShape(t, "project-list", empty.Body)
	if list := decode[protocol.ProjectListSnapshot](t, empty); len(list.Projects) != 0 || list.Projects == nil {
		t.Errorf("a new daemon lists %v, want an empty list that is [] and not null", list.Projects)
	}

	path := testutil.Fixture(t, "small-repo")
	created := st.do(http.MethodPost, "/v1/projects",
		protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path, Name: "Small repo"}).
		want(t, http.StatusCreated)
	sameShape(t, "project", created.Body)
	project := decode[protocol.Project](t, created)
	if got, want := created.Header.Get("Location"), "/v1/projects/"+project.ID; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if project.ID != "small-repo" || project.Name != "Small repo" || project.DefaultBranch != "main" {
		t.Errorf("project = %+v, want id small-repo, name Small repo, branch main", project)
	}

	got := st.do(http.MethodGet, created.Header.Get("Location"), nil).want(t, http.StatusOK)
	if !reflect.DeepEqual(decode[protocol.Project](t, got), project) {
		t.Errorf("GET returned %s, want the project that was created", got.Body)
	}
	listed := decode[protocol.ProjectListSnapshot](t, st.do(http.MethodGet, "/v1/projects", nil).want(t, http.StatusOK))
	if len(listed.Projects) != 1 || listed.Projects[0].ID != project.ID {
		t.Errorf("the list is %+v, want the one project", listed.Projects)
	}

	board := st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).want(t, http.StatusOK)
	sameShape(t, "board", board.Body)
	snapshot := decode[protocol.BoardSnapshot](t, board)
	if snapshot.ProjectID != project.ID || len(snapshot.Columns) != 7 || snapshot.Cards == nil || len(snapshot.Cards) != 0 {
		t.Errorf("board = %+v, want the seven columns and an empty list of cards", snapshot)
	}
}

func TestProjectUpdateThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	path := "/v1/projects/" + project.ID

	renamed := decode[protocol.Project](t, st.do(http.MethodPatch, path, `{"name":"Renamed","devCommand":"make run"}`).want(t, http.StatusOK))
	if renamed.Name != "Renamed" || renamed.DevCommand != "make run" || renamed.ID != project.ID {
		t.Errorf("after a rename the project is %+v", renamed)
	}
	unchanged := decode[protocol.Project](t, st.do(http.MethodPatch, path, `{}`).want(t, http.StatusOK))
	if !reflect.DeepEqual(unchanged, renamed) {
		t.Errorf("an empty patch changed the project: %+v", unchanged)
	}
	cleared := decode[protocol.Project](t, st.do(http.MethodPatch, path, `{"devCommand":""}`).want(t, http.StatusOK))
	if cleared.DevCommand != "" {
		t.Errorf("an empty dev command should clear it, got %q", cleared.DevCommand)
	}
	locked := decode[protocol.Project](t, st.do(http.MethodPatch, path, `{"bypassLocked":true}`).want(t, http.StatusOK))
	if !locked.BypassLocked {
		t.Error("bypassLocked was not turned on")
	}
}

func TestProjectRemoveThroughHTTP(t *testing.T) {
	tests := []struct {
		name string
		body any
	}{
		{"with no body", nil},
		{"with the options", protocol.RemoveProjectRequest{KeepBranches: true, KeepMemory: true}},
		{"with an empty object", `{}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t)
			project, repo := st.addProject("small-repo")
			before := digestTree(t, repo)

			r := st.do(http.MethodDelete, "/v1/projects/"+project.ID, tc.body).want(t, http.StatusNoContent)
			if len(r.Body) != 0 {
				t.Errorf("a 204 has no body, got %q", r.Body)
			}
			st.do(http.MethodGet, "/v1/projects/"+project.ID, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
			st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)

			after := digestTree(t, repo)
			if len(before) != len(after) {
				t.Fatalf("the repository has %d files, it had %d", len(after), len(before))
			}
			for file, sum := range before {
				if after[file] != sum {
					t.Errorf("%s changed when the project was removed", file)
				}
			}
			// The same project can be added again: nothing is left behind.
			if again := st.addProjectAt(repo); again.ID != project.ID {
				t.Errorf("the project came back as %q, want %q", again.ID, project.ID)
			}
		})
	}
}

func TestProjectRefusals(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	notARepo := t.TempDir()
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	folder := func(path string) protocol.CreateProjectRequest {
		return protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path}
	}
	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		code    protocol.ErrorCode
		message string // an exact sentence, or "" when the test only needs the code
	}{
		{"a folder that is not a Git repository", http.MethodPost, "/v1/projects", folder(notARepo),
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"That folder is not a Git repository. Choose the top folder of a repository."},
		{"a folder inside a repository", http.MethodPost, "/v1/projects", folder(filepath.Join(repo, "src")),
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"That folder is not a Git repository. Choose the top folder of a repository."},
		{"a path that is a file", http.MethodPost, "/v1/projects", folder(file),
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"That is not a folder. Choose the top folder of a repository."},
		{"a folder that does not exist", http.MethodPost, "/v1/projects", folder(filepath.Join(notARepo, "nope")),
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"That folder does not exist. Check the path and try again."},
		{"a relative path", http.MethodPost, "/v1/projects", folder("code/api"),
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"Enter the full path of the folder, starting from the top of the disk or with ~."},
		{"a path that is already a project", http.MethodPost, "/v1/projects", folder(repo),
			http.StatusConflict, protocol.ErrorCodeConflict,
			"That repository is already a project in Marshal."},
		{"no source", http.MethodPost, "/v1/projects", `{"path":"/x"}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"Choose whether to add a folder or clone a repository."},
		{"a folder with no path", http.MethodPost, "/v1/projects", `{"source":"folder"}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "Choose the folder of the repository."},
		{"a clone with no address", http.MethodPost, "/v1/projects", `{"source":"clone","dest":"/x"}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"Enter the address to clone from and the folder to clone into."},
		{"a name that is too long", http.MethodPost, "/v1/projects",
			protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: notARepo, Name: strings.Repeat("n", 101)},
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "Project names can have at most 100 characters."},
		{"an empty body", http.MethodPost, "/v1/projects", "",
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "The request body is empty. Send a JSON object."},
		{"text that is not JSON", http.MethodPost, "/v1/projects", "{nope",
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "The request body is not valid JSON. Check it and try again."},
		{"a field Marshal does not know", http.MethodPost, "/v1/projects", `{"source":"folder","color":"red"}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			`The field "color" is not one Marshal knows. Remove it and try again.`},
		{"a field of the wrong kind", http.MethodPost, "/v1/projects", `{"source":7}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, `The field "source" must be text.`},
		{"two values in the body", http.MethodPost, "/v1/projects", `{"source":"folder"}{"source":"folder"}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, ""},
		{"an empty name on a rename", http.MethodPatch, "/v1/projects/" + project.ID, `{"name":"  "}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "Project names can't be empty. The old name is kept."},
		{"a dev command over many lines", http.MethodPatch, "/v1/projects/" + project.ID, `{"devCommand":"a\nb"}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "The dev command must be one line."},
		{"a default branch that does not exist", http.MethodPatch, "/v1/projects/" + project.ID, `{"defaultBranch":"nope"}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "That branch does not exist in the repository."},
		{"a patch with no body", http.MethodPatch, "/v1/projects/" + project.ID, nil,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "The request body is empty. Send a JSON object."},
		{"a remove with a body of the wrong kind", http.MethodDelete, "/v1/projects/" + project.ID, `{"keepBranches":"yes"}`,
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, `The field "keepBranches" must be true or false.`},
		{"an unknown project", http.MethodGet, "/v1/projects/nobody", nil,
			http.StatusNotFound, protocol.ErrorCodeNotFound, "Marshal cannot find that project. It may have been removed."},
		{"an unknown project's board", http.MethodGet, "/v1/projects/nobody/board", nil,
			http.StatusNotFound, protocol.ErrorCodeNotFound, "Marshal cannot find that project. It may have been removed."},
		{"a rename of an unknown project", http.MethodPatch, "/v1/projects/nobody", `{}`,
			http.StatusNotFound, protocol.ErrorCodeNotFound, "Marshal cannot find that project. It may have been removed."},
		{"a remove of an unknown project", http.MethodDelete, "/v1/projects/nobody", nil,
			http.StatusNotFound, protocol.ErrorCodeNotFound, "Marshal cannot find that project. It may have been removed."},
		{"a method the address does not take", http.MethodPut, "/v1/projects/" + project.ID, `{}`,
			http.StatusMethodNotAllowed, protocol.ErrorCodeMethodNotAllowed, ""},
		{"an address that does not exist", http.MethodGet, "/v1/nothing", nil,
			http.StatusNotFound, protocol.ErrorCodeNotFound, "Marshal has nothing at that address. Check the address and try again."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := st.do(tc.method, tc.path, tc.body).apiError(t, tc.status, tc.code)
			if tc.message != "" && got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
	// None of the refusals changed anything.
	if listed := decode[protocol.ProjectListSnapshot](t, st.do(http.MethodGet, "/v1/projects", nil).want(t, http.StatusOK)); len(listed.Projects) != 1 {
		t.Errorf("the refusals changed the list: %+v", listed.Projects)
	}
}

// A body sent with the wrong content type is refused before it is read, and a body over the size
// limit is refused with the limit in the sentence.
func TestBodyRulesOnADomainRoute(t *testing.T) {
	st := newStack(t, withLimits(api.Limits{MaxBodyBytes: 4096}))
	req := st.newRequest(http.MethodPost, "/v1/projects", `{"source":"folder"}`)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer "+st.token)
	got := st.send(req).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if !strings.Contains(got.Message, "Content-Type: application/json") {
		t.Errorf("message = %q, want it to name the header", got.Message)
	}
	big := `{"source":"folder","path":"` + strings.Repeat("a", 8192) + `"}`
	got = st.do(http.MethodPost, "/v1/projects", big).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if want := "The request is larger than Marshal accepts. Send less than 4 KiB."; got.Message != want {
		t.Errorf("message = %q, want %q", got.Message, want)
	}
}

// An id that can never exist, an unknown one that has the right shape, and a missing one are all
// "not found", in the same words, so an id of the wrong shape says nothing more than an unknown one.
func TestABadIDIsNotFoundLikeAnUnknownOne(t *testing.T) {
	st := newStack(t)
	unknownCard := "01M3C107JB041061050R3GG28A"
	tests := []struct {
		name          string
		method        string
		badPath       string
		unknownPath   string
		body          any
		wantKindWords string
	}{
		{"a project", http.MethodGet, "/v1/projects/Bad_ID", "/v1/projects/nobody", nil, "project"},
		{"a board", http.MethodGet, "/v1/projects/Bad_ID/board", "/v1/projects/nobody/board", nil, "project"},
		{"a card", http.MethodGet, "/v1/cards/not-an-id", "/v1/cards/" + unknownCard, nil, "card"},
		{"a card to start", http.MethodPost, "/v1/cards/not-an-id/start", "/v1/cards/" + unknownCard + "/start", nil, "card"},
		{"a card to stop", http.MethodPost, "/v1/cards/not-an-id/stop", "/v1/cards/" + unknownCard + "/stop", nil, "card"},
		{"a card to resume", http.MethodPost, "/v1/cards/not-an-id/resume", "/v1/cards/" + unknownCard + "/resume", nil, "card"},
		{"a card to send to", http.MethodPost, "/v1/cards/not-an-id/messages", "/v1/cards/" + unknownCard + "/messages",
			protocol.SendMessageRequest{Text: "hello"}, "card"},
		{"a card to add to a project", http.MethodPost, "/v1/projects/Bad_ID/cards", "/v1/projects/nobody/cards",
			protocol.CreateCardRequest{Title: "x"}, "project"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bad := st.do(tc.method, tc.badPath, tc.body).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
			unknown := st.do(tc.method, tc.unknownPath, tc.body).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
			if bad.Message != unknown.Message {
				t.Errorf("a bad id says %q but an unknown one says %q", bad.Message, unknown.Message)
			}
			if !strings.Contains(bad.Message, tc.wantKindWords) {
				t.Errorf("message %q does not name the %s", bad.Message, tc.wantKindWords)
			}
			if len(bad.Details) != 1 || len(unknown.Details) != 1 || bad.Details["id"] == "" || unknown.Details["id"] == "" {
				t.Errorf("details differ in shape: %v and %v", bad.Details, unknown.Details)
			}
		})
	}
}

// A very long address is not repeated in full in the answer.
func TestALongIDIsCutInTheAnswer(t *testing.T) {
	st := newStack(t)
	long := strings.Repeat("a", 4000)
	got := st.do(http.MethodGet, "/v1/projects/"+long, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	if n := len(got.Details["id"]); n == 0 || n > 64 {
		t.Errorf("the answer repeats %d bytes of the id, want at most 64", n)
	}
}
