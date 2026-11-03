package projects_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestDirectWorkspaceMustBeTheOriginalProjectFolder(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "Direct task")
	got, err := e.svc.SetWorktree(ctx, card.ID, project.Path, "main")
	if err != nil || got.Worktree != project.Path {
		t.Fatalf("bind original folder = %+v, %v", got, err)
	}
	other := t.TempDir()
	if _, err := e.svc.SetWorktree(ctx, card.ID, other, "main"); err == nil {
		t.Error("accepted a folder outside the project or legacy worktree root")
	}
}

func TestRemovingDirectWorkspaceMetadataKeepsTheFolderAndBorrowedBranch(t *testing.T) {
	for _, remove := range []string{"card", "project"} {
		t.Run(remove, func(t *testing.T) {
			ctx := context.Background()
			e := newEnv(t)
			project := e.folder(t, "small-repo")
			card := e.card(t, project.ID, "Direct task")
			branch := "marshal/owners-existing-branch"
			if _, err := e.git.Run(ctx, project.Path, "switch", "-c", branch); err != nil {
				t.Fatal(err)
			}
			if _, err := e.svc.SetWorktree(ctx, card.ID, project.Path, branch); err != nil {
				t.Fatal(err)
			}
			// Even when no longer checked out, this is the owner's branch, not one to remove.
			if _, err := e.git.Run(ctx, project.Path, "switch", "main"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(project.Path, "owners-untracked.txt")
			if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
				t.Fatal(err)
			}
			var err error
			if remove == "card" {
				err = e.svc.DeleteCard(ctx, card.ID)
			} else {
				err = e.svc.Remove(ctx, project.ID, protocol.RemoveProjectRequest{})
			}
			if err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "keep me" {
				t.Fatalf("source folder changed: %q, %v", data, err)
			}
			if !e.branchExists(t, project, branch) {
				t.Error("removal deleted the owner's borrowed branch")
			}
		})
	}
}
