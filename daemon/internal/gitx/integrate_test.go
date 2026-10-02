package gitx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

// integratorFolders gives the folders of an integrator workspace test: the root Marshal keeps its
// workspaces under, and the workspace inside it.
func integratorFolders(t *testing.T) (root, ws string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "integrator")
	return root, filepath.Join(root, "p")
}

func TestEnsureBranchWorktreeMakesTheBranchAndTheWorktree(t *testing.T) {
	repo := baseRepo(t)
	root, ws := integratorFolders(t)
	g := testGit()
	ctx := context.Background()

	if err := g.EnsureBranchWorktree(ctx, repo, ws, root, "integrator", "main"); err != nil {
		t.Fatalf("EnsureBranchWorktree: %v", err)
	}
	if got := git(t, ws, "rev-parse", "--abbrev-ref", "HEAD"); got != "integrator" {
		t.Errorf("workspace branch = %q, want integrator", got)
	}
	if git(t, repo, "rev-parse", "integrator") != git(t, repo, "rev-parse", "main") {
		t.Errorf("integrator did not start at main")
	}
	// A second call changes nothing.
	if err := g.EnsureBranchWorktree(ctx, repo, ws, root, "integrator", "main"); err != nil {
		t.Errorf("EnsureBranchWorktree again: %v", err)
	}
}

func TestEnsureBranchWorktreeRepairsADeletedFolderAndKeepsTheBranch(t *testing.T) {
	repo := baseRepo(t)
	root, ws := integratorFolders(t)
	g := testGit()
	ctx := context.Background()
	if err := g.EnsureBranchWorktree(ctx, repo, ws, root, "integrator", "main"); err != nil {
		t.Fatal(err)
	}
	commitFile(t, ws, "kept.txt", "work on the integrator\n")
	kept := git(t, ws, "rev-parse", "HEAD")
	if err := os.RemoveAll(ws); err != nil {
		t.Fatal(err)
	}

	if err := g.EnsureBranchWorktree(ctx, repo, ws, root, "integrator", "main"); err != nil {
		t.Fatalf("EnsureBranchWorktree after the folder was deleted: %v", err)
	}
	if got := git(t, ws, "rev-parse", "HEAD"); got != kept {
		t.Errorf("the integrator branch is at %s, want the commit it had, %s", got, kept)
	}
}

func TestEnsureBranchWorktreeRefusesABranchCheckedOutElsewhere(t *testing.T) {
	repo := baseRepo(t)
	root, ws := integratorFolders(t)
	git(t, repo, "checkout", "--quiet", "-b", "integrator")
	err := testGit().EnsureBranchWorktree(context.Background(), repo, ws, root, "integrator", "main")
	if !errors.Is(err, gitx.ErrBranchInUse) {
		t.Fatalf("EnsureBranchWorktree = %v, want ErrBranchInUse", err)
	}
}

func TestEnsureBranchWorktreeRefusesAPathOutsideItsRoot(t *testing.T) {
	repo := baseRepo(t)
	root, _ := integratorFolders(t)
	outside := filepath.Join(t.TempDir(), "elsewhere")
	err := testGit().EnsureBranchWorktree(context.Background(), repo, outside, root, "integrator", "main")
	if !errors.Is(err, gitx.ErrOutsideRoot) {
		t.Fatalf("EnsureBranchWorktree = %v, want ErrOutsideRoot", err)
	}
}

func TestCheckedOutAtFindsTheFolderThatHasABranch(t *testing.T) {
	repo := baseRepo(t)
	g := testGit()
	ctx := context.Background()
	wt, found, err := g.CheckedOutAt(ctx, repo, "main")
	if err != nil || !found {
		t.Fatalf("CheckedOutAt(main) = %+v, %v, %v; want found", wt, found, err)
	}
	if !gitx.SameFolder(wt.Path, repo) {
		t.Errorf("main is checked out at %s, want %s", wt.Path, repo)
	}
	if _, found, _ := g.CheckedOutAt(ctx, repo, "nowhere"); found {
		t.Errorf("CheckedOutAt(nowhere) found a worktree")
	}
}

func TestAConflictedMergeIsResolvedStagedAndCommittedInTheWorkspace(t *testing.T) {
	repo := baseRepo(t)
	root, ws := integratorFolders(t)
	g := testGit()
	ctx := context.Background()
	branchWithCommit(t, repo, "card", "main", "a.txt", "a by the card\n")
	commitFile(t, repo, "a.txt", "a on main\n")
	if err := g.EnsureBranchWorktree(ctx, repo, ws, root, "integrator", "main"); err != nil {
		t.Fatal(err)
	}

	conflicts, err := g.StartMerge(ctx, ws, "card")
	if err != nil {
		t.Fatalf("StartMerge: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0] != "a.txt" {
		t.Fatalf("conflicts = %v, want [a.txt]", conflicts)
	}
	if left, err := gitx.ConflictMarkers(ws, conflicts); err != nil || len(left) != 1 {
		t.Errorf("ConflictMarkers before resolving = %v, %v; want a.txt", left, err)
	}
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("a by both\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if left, err := gitx.ConflictMarkers(ws, conflicts); err != nil || len(left) != 0 {
		t.Errorf("ConflictMarkers after resolving = %v, %v; want none", left, err)
	}
	if err := g.StageResolved(ctx, ws, conflicts); err != nil {
		t.Fatalf("StageResolved: %v", err)
	}
	if left, _ := g.UnmergedPaths(ctx, ws); len(left) != 0 {
		t.Errorf("unmerged after staging = %v, want none", left)
	}
	commit, err := g.CommitMerge(ctx, ws, "Marshal: merge the card")
	if err != nil {
		t.Fatalf("CommitMerge: %v", err)
	}
	if got := git(t, ws, "rev-parse", "HEAD"); got != commit {
		t.Errorf("HEAD = %s, want the merge commit %s", got, commit)
	}
	if got := git(t, ws, "show", "HEAD:a.txt"); got != "a by both" {
		t.Errorf("a.txt in the merge = %q, want the resolution", got)
	}
	if parents := git(t, ws, "rev-list", "--parents", "-n", "1", "HEAD"); len(parents) < 80 {
		t.Errorf("the merge commit has one parent: %q", parents)
	}
}

func TestConflictMarkersIgnoresAHeadingLineAndFilesThatAreGone(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "README.md", "Title\n=======\ntext\n")
	write(t, dir, "bin.dat", "\x00<<<<<<< HEAD\n")
	write(t, dir, "bad.txt", "keep\n<<<<<<< HEAD\nmine\n=======\ntheirs\n>>>>>>> card\n")
	left, err := gitx.ConflictMarkers(dir, []string{"README.md", "gone.txt", "bin.dat", "bad.txt", "../escape"})
	if err != nil {
		t.Fatalf("ConflictMarkers: %v", err)
	}
	if len(left) != 1 || left[0] != "bad.txt" {
		t.Errorf("ConflictMarkers = %v, want only bad.txt", left)
	}
}

func TestResetWorktreeDropsAHalfDoneMergeAndStrayFiles(t *testing.T) {
	repo := baseRepo(t)
	root, ws := integratorFolders(t)
	g := testGit()
	ctx := context.Background()
	branchWithCommit(t, repo, "card", "main", "a.txt", "a by the card\n")
	commitFile(t, repo, "a.txt", "a on main\n")
	if err := g.EnsureBranchWorktree(ctx, repo, ws, root, "integrator", "main"); err != nil {
		t.Fatal(err)
	}
	tip := git(t, ws, "rev-parse", "HEAD")
	if _, err := g.StartMerge(ctx, ws, "card"); err != nil {
		t.Fatal(err)
	}
	write(t, ws, "stray.txt", "left by a test run\n")

	if err := g.ResetWorktree(ctx, ws, root, tip); err != nil {
		t.Fatalf("ResetWorktree: %v", err)
	}
	if status := git(t, ws, "status", "--porcelain"); status != "" {
		t.Errorf("status after the reset = %q, want clean", status)
	}
	if _, err := os.Stat(filepath.Join(ws, "stray.txt")); err == nil {
		t.Errorf("the stray file is still there")
	}
	if err := g.ResetWorktree(ctx, repo, root, tip); !errors.Is(err, gitx.ErrOutsideRoot) {
		t.Errorf("ResetWorktree of the owner's folder = %v, want ErrOutsideRoot", err)
	}
}

func TestCountAheadAndFastForwardWorktree(t *testing.T) {
	repo := baseRepo(t)
	root, ws := integratorFolders(t)
	g := testGit()
	ctx := context.Background()
	if err := g.EnsureBranchWorktree(ctx, repo, ws, root, "integrator", "main"); err != nil {
		t.Fatal(err)
	}
	commitFile(t, repo, "c.txt", "c on main\n")
	if n, err := g.CountAhead(ctx, repo, "integrator", "main"); err != nil || n != 1 {
		t.Fatalf("CountAhead(integrator..main) = %d, %v; want 1", n, err)
	}
	if err := g.FastForwardWorktree(ctx, ws, "main"); err != nil {
		t.Fatalf("FastForwardWorktree: %v", err)
	}
	if git(t, repo, "rev-parse", "integrator") != git(t, repo, "rev-parse", "main") {
		t.Errorf("integrator did not catch up with main")
	}
	commitFile(t, ws, "d.txt", "d on the integrator\n")
	commitFile(t, repo, "e.txt", "e on main\n")
	if err := g.FastForwardWorktree(ctx, ws, "main"); !errors.Is(err, gitx.ErrNotFastForward) {
		t.Errorf("FastForwardWorktree to a diverged tip = %v, want ErrNotFastForward", err)
	}
}
