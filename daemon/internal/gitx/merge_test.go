package gitx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

// baseRepo makes a repository with two files on main, so one branch can change one of them and
// another can change the other (a clean merge) or the same one (a conflict).
func baseRepo(t *testing.T) string {
	t.Helper()
	repo := newEmptyRepo(t)
	commitFile(t, repo, "a.txt", "a base\n")
	commitFile(t, repo, "b.txt", "b base\n")
	return repo
}

// branchWithCommit makes a branch from from, writes one file on it, commits, and returns to from.
func branchWithCommit(t *testing.T, repo, name, from, file, content string) {
	t.Helper()
	git(t, repo, "checkout", "--quiet", "-b", name, from)
	commitFile(t, repo, file, content)
	git(t, repo, "checkout", "--quiet", from)
}

func TestDryRunMergeFindsACleanMergeAndTheChangedFiles(t *testing.T) {
	repo := baseRepo(t)
	branchWithCommit(t, repo, "card", "main", "b.txt", "b changed by the card\n")

	preview, err := testGit().DryRunMerge(context.Background(), repo, "main", "card")
	if err != nil {
		t.Fatalf("DryRunMerge: %v", err)
	}
	if !preview.Clean {
		t.Fatalf("clean = false, want a clean merge; conflicts = %v", preview.Conflicts)
	}
	if len(preview.Changed) != 1 || preview.Changed[0] != "b.txt" {
		t.Errorf("changed = %v, want [b.txt]", preview.Changed)
	}
}

func TestDryRunMergeFindsConflictsAndTouchesNothing(t *testing.T) {
	repo := baseRepo(t)
	branchWithCommit(t, repo, "card", "main", "a.txt", "a changed by the card\n")
	commitFile(t, repo, "a.txt", "a changed on main\n")

	before, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := testGit().DryRunMerge(context.Background(), repo, "main", "card")
	if err != nil {
		t.Fatalf("DryRunMerge: %v", err)
	}
	if preview.Clean {
		t.Fatalf("clean = true, want a conflict; changed = %v", preview.Changed)
	}
	if len(preview.Conflicts) == 0 {
		t.Fatalf("conflicts = %v, want at least one file", preview.Conflicts)
	}
	after, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("a dry-run merge changed a.txt on disk: %q -> %q", before, after)
	}
	if status := git(t, repo, "status", "--porcelain"); status != "" {
		t.Errorf("a dry-run merge left the worktree dirty: %q", status)
	}
}

func TestDryRunMergeRefusesAnOptionAsARevision(t *testing.T) {
	repo := baseRepo(t)
	if _, err := testGit().DryRunMerge(context.Background(), repo, "--all", "main"); !errors.Is(err, gitx.ErrBadBranchName) {
		t.Errorf("DryRunMerge(--all) = %v, want ErrBadBranchName", err)
	}
}

func TestBackupBranchKeepsTheOldTipAndRefusesToOverwrite(t *testing.T) {
	repo := baseRepo(t)
	oldTip := git(t, repo, "rev-parse", "main")
	g := testGit()
	if err := g.BackupBranch(context.Background(), repo, "marshal/backup/main-1", "main"); err != nil {
		t.Fatalf("BackupBranch: %v", err)
	}
	if tip := git(t, repo, "rev-parse", "marshal/backup/main-1"); tip != oldTip {
		t.Errorf("backup tip = %s, want %s", tip, oldTip)
	}
	if err := g.BackupBranch(context.Background(), repo, "marshal/backup/main-1", "main"); !errors.Is(err, gitx.ErrBranchExists) {
		t.Errorf("BackupBranch twice = %v, want ErrBranchExists", err)
	}
}

func TestMergeInATemporaryWorktreeThenFastForwardTheTarget(t *testing.T) {
	repo := baseRepo(t)
	branchWithCommit(t, repo, "card", "main", "b.txt", "b changed by the card\n")
	oldTip := git(t, repo, "rev-parse", "main")
	g := testGit()
	ctx := context.Background()
	worktree := filepath.Join(t.TempDir(), "merge-wt")

	if err := g.AddMergeWorktree(ctx, repo, worktree, "main"); err != nil {
		t.Fatalf("AddMergeWorktree: %v", err)
	}
	mergeCommit, err := g.MergeInto(ctx, worktree, "card", "Merge card")
	if err != nil {
		t.Fatalf("MergeInto: %v", err)
	}
	// Neither the target nor the repository moved while the merge was only in the worktree.
	if tip := git(t, repo, "rev-parse", "main"); tip != oldTip {
		t.Fatalf("main moved before the fast-forward: %s != %s", tip, oldTip)
	}
	if err := g.FastForwardRef(ctx, repo, "main", mergeCommit); err != nil {
		t.Fatalf("FastForwardRef: %v", err)
	}
	if tip := git(t, repo, "rev-parse", "main"); tip != mergeCommit {
		t.Errorf("main tip = %s, want the merge commit %s", tip, mergeCommit)
	}
	// A fast-forward moves the ref; it does not refresh a working tree that has main checked out.
	// The card's change is in the branch's tree, which is what a later checkout or clone reads.
	if content := git(t, repo, "show", "main:b.txt"); content != "b changed by the card" {
		t.Errorf("main:b.txt = %q, want the card's change", content)
	}
}

func TestFastForwardRefRefusesAMoveThatIsNotForward(t *testing.T) {
	repo := baseRepo(t)
	oldTip := git(t, repo, "rev-parse", "main")
	branchWithCommit(t, repo, "card", "main", "b.txt", "b changed\n")
	// A second branch off the same base, so neither branch contains the other.
	branchWithCommit(t, repo, "other", "main", "a.txt", "a changed\n")
	cardTip := git(t, repo, "rev-parse", "card")
	otherTip := git(t, repo, "rev-parse", "other")
	g := testGit()
	ctx := context.Background()
	if err := g.FastForwardRef(ctx, repo, "main", cardTip); err != nil {
		t.Fatalf("FastForwardRef onto a descendant: %v", err)
	}
	// main is now cardTip, which is not an ancestor of otherTip.
	if err := g.FastForwardRef(ctx, repo, "main", otherTip); !errors.Is(err, gitx.ErrNotFastForward) {
		t.Errorf("FastForwardRef sideways = %v, want ErrNotFastForward", err)
	}
	if tip := git(t, repo, "rev-parse", "main"); tip == oldTip {
		t.Errorf("main did not move to the card's commit")
	}
	// Moving to the target it is already on changes nothing and is allowed.
	if err := g.FastForwardRef(ctx, repo, "main", cardTip); err != nil {
		t.Errorf("FastForwardRef to the same commit: %v", err)
	}
}

func TestMergeIntoConflictThenAbortLeavesTheWorktreeAsItWas(t *testing.T) {
	repo := baseRepo(t)
	branchWithCommit(t, repo, "card", "main", "a.txt", "a changed by the card\n")
	commitFile(t, repo, "a.txt", "a changed on main\n")
	g := testGit()
	ctx := context.Background()
	worktree := filepath.Join(t.TempDir(), "merge-wt")

	if err := g.AddMergeWorktree(ctx, repo, worktree, "main"); err != nil {
		t.Fatalf("AddMergeWorktree: %v", err)
	}
	if _, err := g.MergeInto(ctx, worktree, "card", "Merge card"); !errors.Is(err, gitx.ErrMergeConflict) {
		t.Fatalf("MergeInto = %v, want ErrMergeConflict", err)
	}
	if err := g.AbortMerge(ctx, worktree); err != nil {
		t.Fatalf("AbortMerge: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(worktree, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "a changed on main\n" {
		t.Errorf("after abort a.txt = %q, want main's version", content)
	}
	// The target branch never moved.
	if tip := git(t, repo, "rev-parse", "main"); tip != git(t, repo, "rev-parse", "HEAD") {
		t.Errorf("main moved during a conflicting merge")
	}
}

func TestMergeIntoRefusesAnOptionAsABranch(t *testing.T) {
	repo := baseRepo(t)
	worktree := filepath.Join(t.TempDir(), "merge-wt")
	g := testGit()
	ctx := context.Background()
	if err := g.AddMergeWorktree(ctx, repo, worktree, "main"); err != nil {
		t.Fatalf("AddMergeWorktree: %v", err)
	}
	if _, err := g.MergeInto(ctx, worktree, "--abort", "x"); !errors.Is(err, gitx.ErrBadBranchName) {
		t.Errorf("MergeInto(--abort) = %v, want ErrBadBranchName", err)
	}
}
