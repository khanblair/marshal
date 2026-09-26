package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/sample"
)

// The sample project (docs/backend-checklist.md B2.12, N24) through the real routes: it is added
// with only its source, it is an ordinary project with the sample's own history, and a card on it
// starts with the stub agent in a worktree of that history like a card on any other project. The
// rules of the sample itself are tested in internal/sample and internal/projects.

func TestTheSampleProjectIsAnOrdinaryProjectThroughHTTP(t *testing.T) {
	st := newStack(t)
	stream := st.dial(protocol.HomeTopic)

	created := st.do(http.MethodPost, "/v1/projects", protocol.CreateProjectRequest{Source: protocol.ProjectSourceSample}).
		want(t, http.StatusCreated)
	sameShape(t, "project", created.Body)
	project := decode[protocol.Project](t, created)
	if project.ID != sample.FolderName || project.Name != sample.FolderName ||
		project.DefaultBranch != sample.Branch || project.Language != "TypeScript" {
		t.Errorf("project = %+v, want the sample's name, its main branch, and TypeScript", project)
	}
	if want := "/v1/projects/" + project.ID; created.Header.Get("Location") != want {
		t.Errorf("Location = %q, want %q", created.Header.Get("Location"), want)
	}
	stream.until(ofType(protocol.EventTypeProjectCreated))

	// The folder is under the data folder, is a real repository on main, and has the history.
	if !strings.HasPrefix(evalLinks(t, project.Path), evalLinks(t, st.dataDir)) {
		t.Errorf("the sample is at %s, want a folder under the data folder %s", project.Path, st.dataDir)
	}
	if branch := st.gitOutput(project.Path, "branch", "--show-current"); branch != sample.Branch {
		t.Errorf("branch = %q, want %q", branch, sample.Branch)
	}
	if count := st.gitOutput(project.Path, "rev-list", "--count", sample.Branch); count != "2" {
		t.Errorf("the sample has %s commits, want 2", count)
	}

	// A second sample is refused with the sentence and the id of the one there is.
	refusal := st.do(http.MethodPost, "/v1/projects", protocol.CreateProjectRequest{Source: protocol.ProjectSourceSample}).
		apiError(t, http.StatusConflict, protocol.ErrorCodeConflict)
	if refusal.Message != "The sample project is already in Marshal." || refusal.Details["projectId"] != project.ID {
		t.Errorf("second sample: %q %v", refusal.Message, refusal.Details)
	}

	// A card on the sample starts with the stub agent, in a worktree that holds the sample's files.
	card := st.addCard(project.ID, "Add a health check endpoint")
	started := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK))
	if started.State != protocol.CardStateWorking || !strings.HasPrefix(started.Branch, "marshal/") {
		t.Fatalf("the started card is %+v, want it working on a marshal/ branch", started)
	}
	worktree, branch, err := st.proj.Worktree(context.Background(), card.ID)
	if err != nil || branch != started.Branch {
		t.Fatalf("the card's worktree = %q on %q, %v; want one on %q", worktree, branch, err, started.Branch)
	}
	if _, err := os.Stat(filepath.Join(worktree, "src", "todos.ts")); err != nil {
		t.Errorf("the card's worktree has no copy of the sample's files: %v", err)
	}
	if head := st.gitOutput(worktree, "log", "-1", "--format=%s"); head != "Add due dates to todos" {
		t.Errorf("the worktree starts from %q, want the sample's latest commit", head)
	}
}

// evalLinks resolves links in a path, so two spellings of one folder compare equal.
func evalLinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	return resolved
}
