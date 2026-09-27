package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

// Reading the commits an agent made (docs/backend-checklist.md B3.5): the commits on a worktree's
// branch and not on the project's default branch are exactly the ones the agent wrote, and the
// files of one of those commits can be read back as they were.

// shaOf is the commit a folder is on.
func shaOf(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(git(t, dir, "rev-parse", "HEAD"))
}

// commitsOnBranch reads the commits a folder's branch added on top of base.
func commitsOnBranch(t *testing.T, dir, base string) []gitx.Commit {
	t.Helper()
	commits, err := testGit().CommitsOnBranch(context.Background(), dir, base)
	if err != nil {
		t.Fatalf("CommitsOnBranch(base=%q): %v", base, err)
	}
	return commits
}

func TestCommitsOnBranchAreTheOnesTheAgentMade(t *testing.T) {
	dir := fixture(t, "small-repo")
	base := shaOf(t, dir)
	git(t, dir, "checkout", "--quiet", "-b", "marshal/card-1")
	commitFile(t, dir, "one.txt", "one\n")
	first := shaOf(t, dir)
	commitFile(t, dir, "two.txt", "two\n")
	second := shaOf(t, dir)

	commits := commitsOnBranch(t, dir, "main")
	if len(commits) != 2 {
		t.Fatalf("CommitsOnBranch = %+v, want the two commits the branch added", commits)
	}
	if commits[0].SHA != first || commits[1].SHA != second {
		t.Errorf("the commits are %s,%s, want %s,%s (oldest first)",
			commits[0].SHA, commits[1].SHA, first, second)
	}
	if commits[0].Subject != "Add one.txt" || commits[1].Subject != "Add two.txt" {
		t.Errorf("the subjects are %q,%q, want the commit messages",
			commits[0].Subject, commits[1].Subject)
	}
	// The base's own commit is the project's history, not the agent's work.
	for _, c := range commits {
		if c.SHA == base {
			t.Errorf("the base commit %s was read as the agent's own", base)
		}
	}
}

func TestCommitsOnBranchWithNothingNewIsEmpty(t *testing.T) {
	dir := fixture(t, "small-repo")
	if commits := commitsOnBranch(t, dir, "main"); len(commits) != 0 {
		t.Errorf("CommitsOnBranch = %+v, want nothing on a branch that added no commits", commits)
	}
}

// TestCommitsOnBranchRefusesAnEmptyBase covers the deliberate refusal: without a base there is no
// way to tell the agent's work from the project's history, so it is not read as "every commit".
func TestCommitsOnBranchRefusesAnEmptyBase(t *testing.T) {
	dir := fixture(t, "small-repo")
	commitFile(t, dir, "one.txt", "one\n")
	for _, base := range []string{"", "   "} {
		if commits := commitsOnBranch(t, dir, base); len(commits) != 0 {
			t.Errorf("CommitsOnBranch(base=%q) = %+v, want nothing", base, commits)
		}
	}
}

func TestChangedPathsNamesTheFilesACommitChanged(t *testing.T) {
	dir := fixture(t, "small-repo")
	commitFile(t, dir, "kept.txt", "kept\n")
	commitFile(t, dir, "gone.txt", "gone\n")
	// One commit that changes kept.txt and deletes gone.txt, so the deleted file can be told apart
	// from the changed one.
	if err := os.WriteFile(filepath.Join(dir, "kept.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "--all")
	git(t, dir, "-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "Change one, delete one")

	paths, err := testGit().ChangedPaths(context.Background(), dir, shaOf(t, dir))
	if err != nil {
		t.Fatalf("ChangedPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != "kept.txt" {
		t.Errorf("ChangedPaths = %v, want only the file that still exists in the commit", paths)
	}
}

// TestChangedPathsOfTheFirstCommit covers --root: a repository whose first commit added files has
// no parent to diff against, and every file in it is a change.
func TestChangedPathsOfTheFirstCommit(t *testing.T) {
	dir := newEmptyRepo(t)
	commitFile(t, dir, "first.txt", "hello\n")

	paths, err := testGit().ChangedPaths(context.Background(), dir, shaOf(t, dir))
	if err != nil {
		t.Fatalf("ChangedPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != "first.txt" {
		t.Errorf("ChangedPaths of a root commit = %v, want the file it added", paths)
	}
}

func TestFileAtCommitReadsTheFileAsItWas(t *testing.T) {
	dir := fixture(t, "small-repo")
	commitFile(t, dir, "config.txt", "before\n")
	first := shaOf(t, dir)
	commitFile(t, dir, "config.txt", "after\n")

	got, err := testGit().FileAtCommit(context.Background(), dir, first, "config.txt")
	if err != nil {
		t.Fatalf("FileAtCommit: %v", err)
	}
	// Git.Run drops the trailing newline, so the content is compared without one; that is all a
	// caller scanning it for a credential needs.
	if got != "before" {
		t.Errorf("FileAtCommit = %q, want the file as it was at that commit", got)
	}

	// A path the commit does not hold is an error, because the caller passes the commit's own
	// changed paths, which do exist in it.
	if _, err := testGit().FileAtCommit(context.Background(), dir, first, "missing.txt"); err == nil {
		t.Error("FileAtCommit read a path the commit does not hold")
	}
}

func TestFileAtCommitRefusesAPathThatIsNotOne(t *testing.T) {
	dir := fixture(t, "small-repo")
	for _, path := range []string{"", "-p", "a\nb"} {
		if _, err := testGit().FileAtCommit(context.Background(), dir, shaOf(t, dir), path); err == nil {
			t.Errorf("FileAtCommit(%q) was allowed, want a refusal", path)
		}
	}
}
