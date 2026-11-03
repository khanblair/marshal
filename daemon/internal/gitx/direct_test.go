package gitx_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

func TestDirectSnapshotsExcludeExistingWorkAndPreserveTheIndex(t *testing.T) {
	ctx := context.Background()
	repo := folderRepo(t)
	write(t, repo, "a.txt", "owner staged\n")
	git(t, repo, "add", "a.txt")
	write(t, repo, "b.txt", "owner unstaged\n")
	write(t, repo, "owner-new.txt", "owner untracked\n")
	head := git(t, repo, "rev-parse", "HEAD")
	index := git(t, repo, "write-tree")
	base, err := testGit().SnapshotFolder(ctx, repo, gitx.DirectBaseRef("card"), "start")
	if err != nil {
		t.Fatal(err)
	}
	write(t, repo, "task.txt", "card change\n")
	result, err := testGit().SnapshotFolder(ctx, repo, gitx.DirectResultRef("card"), "result")
	if err != nil {
		t.Fatal(err)
	}
	files, cut, err := testGit().DiffFilesBetween(ctx, repo, base.Commit, result.Commit, 0)
	if err != nil || cut || len(files) != 1 || files[0].Path != "task.txt" || files[0].Additions != 1 {
		t.Fatalf("saved diff = %+v, cut=%v, err=%v", files, cut, err)
	}
	if got := git(t, repo, "rev-parse", "HEAD"); got != head {
		t.Errorf("snapshot moved HEAD: %s", got)
	}
	if got := git(t, repo, "write-tree"); got != index {
		t.Errorf("snapshot changed the user's index: %s", got)
	}
	write(t, repo, "task.txt", "later card\n")
	file, found, err := testGit().DiffHunksBetween(ctx, repo, base.Commit, result.Commit, "task.txt", gitx.HunkLimits{MaxLines: 100})
	if err != nil || !found || len(file.Hunks) != 1 || file.Hunks[0].Lines[0].Text != "card change" {
		t.Fatalf("saved hunks changed with the live folder: %+v, found=%v, err=%v", file, found, err)
	}
	if _, found, err := testGit().DiffHunksBetween(ctx, repo, base.Commit, result.Commit, "owner-new.txt", gitx.HunkLimits{MaxLines: 100}); err != nil || found {
		t.Fatalf("baseline untracked file appeared as this card's work: found=%v, err=%v", found, err)
	}
	if _, _, err := testGit().DiffHunksBetween(ctx, repo, base.Commit, result.Commit, "../outside", gitx.HunkLimits{MaxLines: 100}); err == nil {
		t.Error("saved diff accepted a path outside the project")
	}
}

func TestDirectRefsAreSafeForOpaqueCardIDs(t *testing.T) {
	repo := folderRepo(t)
	for _, id := range []string{"card", "../HEAD~1:bad", "spaces and \n controls"} {
		for _, ref := range []string{gitx.DirectBaseRef(id), gitx.DirectResultRef(id)} {
			if !strings.HasPrefix(ref, "refs/marshal/direct/") {
				t.Fatalf("ref escaped its namespace: %q", ref)
			}
			git(t, repo, "check-ref-format", ref)
		}
	}
}
