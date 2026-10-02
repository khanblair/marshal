package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// devOnlyFile exists on the development branch of the repository these tests build, and not on main.
const devOnlyFile = "dev-only.txt"

// projectOnDevelopment makes a project whose repository is on a development branch that has one
// commit main does not, so the project's integration branch is development from the start.
func (e *env) projectOnDevelopment(t *testing.T) protocol.Project {
	t.Helper()
	ctx := context.Background()
	path := testutil.Fixture(t, "small-repo")
	if _, err := e.git.Run(ctx, path, "checkout", "-b", "development"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.git.CommitFile(ctx, path, devOnlyFile, "work from development\n", "A commit only development has"); err != nil {
		t.Fatal(err)
	}
	p, err := e.proj.Create(ctx, protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// startedWorktree starts a card and answers the folder of its worktree.
func (e *env) startedWorktree(t *testing.T, projectID string) string {
	t.Helper()
	card := e.card(t, projectID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	path, _, err := e.proj.Worktree(context.Background(), card.ID)
	if err != nil || path == "" {
		t.Fatalf("Worktree = %q, %v", path, err)
	}
	return path
}

func TestACardsWorktreeStartsFromTheIntegrationBranch(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		if err := e.mgr.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	project := e.projectOnDevelopment(t)
	if project.DefaultBranch != "main" || project.Target() != "development" {
		t.Fatalf("project = %+v, want default main and target development", project)
	}
	path := e.startedWorktree(t, project.ID)
	if _, err := os.Stat(filepath.Join(path, devOnlyFile)); err != nil {
		t.Errorf("the worktree does not hold the integration branch's work: %v", err)
	}
}

func TestACardsWorktreeStartsFromTheDefaultBranchWhenNoIntegrationBranchIsChosen(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		if err := e.mgr.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	project := e.projectOnDevelopment(t)
	empty := ""
	if _, err := e.proj.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{IntegrationBranch: &empty}); err != nil {
		t.Fatal(err)
	}
	path := e.startedWorktree(t, project.ID)
	if _, err := os.Stat(filepath.Join(path, devOnlyFile)); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v, want the worktree to start from main, which lacks the file", devOnlyFile, err)
	}
}
