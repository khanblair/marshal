package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

// Checkpoints (B5.3, build-plan 5.21): a real commit Marshal makes in a card's worktree, kept on a
// hidden ref, that the worktree and the branch can be reset back to. These tests are the Git half;
// the row that names the commit and the label a person reads are tested where the sessions live.

// writeNewFile writes a file under dir, making its folder.
func writeNewFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// head is the commit the branch is on now, and dirty says whether the worktree has anything
// uncommitted.
func head(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(git(t, dir, "rev-parse", "HEAD"))
}

func dirty(t *testing.T, dir string) bool {
	t.Helper()
	return strings.TrimSpace(git(t, dir, "status", "--porcelain")) != ""
}

// A checkpoint commits whatever the worktree holds, points the hidden ref at the commit, and leaves
// the worktree clean: restoring it is an ordinary reset, not a copy of files.
func TestACheckpointCommitsTheWorkAndNamesIt(t *testing.T) {
	repo := fixture(t, "small-repo")
	g := testGit()
	ctx := context.Background()

	writeNewFile(t, repo, "src/columns.tsx", "export const columns = [];\n")
	ref := gitx.CheckpointRef("01JD7Q4M2X8K9V0P5T3RB6NHAE")
	state, err := g.Checkpoint(ctx, repo, ref, "Marshal checkpoint: before turn 3")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if !state.Created {
		t.Error("a checkpoint of a dirty worktree says it made no commit")
	}
	if dirty(t, repo) {
		t.Error("the worktree is still dirty after a checkpoint")
	}
	if state.SHA != head(t, repo) {
		t.Errorf("the checkpoint points at %s, but the branch is on %s", state.SHA, head(t, repo))
	}
	// The ref points at the commit, so the checkpoint survives the branch moving on.
	if at := strings.TrimSpace(git(t, repo, "rev-parse", ref)); at != state.SHA {
		t.Errorf("the hidden ref points at %s, want %s", at, state.SHA)
	}
	if subject := strings.TrimSpace(git(t, repo, "log", "-1", "--format=%s", state.SHA)); subject != "Marshal checkpoint: before turn 3" {
		t.Errorf("the commit's message = %q, want the checkpoint's label", subject)
	}
}

// A clean worktree is not a failure: there is nothing to commit, and the checkpoint points at the
// branch's own head.
func TestACheckpointOfACleanWorktreeMakesNoCommit(t *testing.T) {
	repo := fixture(t, "small-repo")
	g := testGit()
	ctx := context.Background()
	before := head(t, repo)

	ref := gitx.CheckpointRef("01JD7Q4M2X8K9V0P5T3RB6NHB7")
	state, err := g.Checkpoint(ctx, repo, ref, "Marshal checkpoint")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if state.Created {
		t.Error("a checkpoint of a clean worktree says it made a commit")
	}
	if state.SHA != before {
		t.Errorf("the checkpoint points at %s, want the branch's head %s", state.SHA, before)
	}
}

// The hidden ref keeps the commit even after the branch moves on, which is what makes a checkpoint a
// restore point rather than a note about where the branch happened to be.
func TestACheckpointSurvivesTheBranchMovingOn(t *testing.T) {
	repo := fixture(t, "small-repo")
	g := testGit()
	ctx := context.Background()

	writeNewFile(t, repo, "src/one.ts", "one\n")
	ref := gitx.CheckpointRef("01JD7Q4M2X8K9V0P5T3RB6NHD1")
	state, err := g.Checkpoint(ctx, repo, ref, "Marshal checkpoint: before turn 1")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// The agent's next turn commits more work on the same branch.
	writeNewFile(t, repo, "src/two.ts", "two\n")
	commitFile(t, repo, "src/three.ts", "three\n")
	if head(t, repo) == state.SHA {
		t.Fatal("the branch did not move on, so the test proves nothing")
	}

	at, err := g.CheckpointCommit(ctx, repo, ref)
	if err != nil {
		t.Fatalf("CheckpointCommit: %v", err)
	}
	if at != state.SHA {
		t.Errorf("the hidden ref now points at %s, want the checkpoint's %s", at, state.SHA)
	}
}

// Restoring puts the tracked files back to the checkpoint and removes the files the checkpoint does
// not hold, so the worktree is exactly what it was. Ignored files are left alone: a build folder is
// not what a restore point is about.
func TestRestoringPutsTheWorktreeBackAndLeavesIgnoredFiles(t *testing.T) {
	repo := fixture(t, "small-repo")
	g := testGit()
	ctx := context.Background()

	writeNewFile(t, repo, "src/keep.ts", "keep\n")
	ref := gitx.CheckpointRef("01JD7Q4M2X8K9V0P5T3RB6NHE5")
	state, err := g.Checkpoint(ctx, repo, ref, "Marshal checkpoint: before turn 2")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// The next turn edits one file, deletes another, and adds a third.
	writeNewFile(t, repo, "src/keep.ts", "changed\n")
	if err := os.Remove(filepath.Join(repo, "src/index.js")); err != nil {
		t.Fatal(err)
	}
	writeNewFile(t, repo, "src/added.ts", "added\n")
	// A build folder is ignored, so a restore must not touch it.
	writeNewFile(t, repo, "node_modules/left-alone.txt", "build output\n")

	if err := g.RestoreCheckpoint(ctx, repo, state.SHA); err != nil {
		t.Fatalf("RestoreCheckpoint: %v", err)
	}
	if dirty(t, repo) {
		t.Error("the worktree is dirty after a restore")
	}
	if got := readFile(t, repo, "src/keep.ts"); got != "keep\n" {
		t.Errorf("src/keep.ts = %q, want it back to the checkpoint's content", got)
	}
	if !exists(filepath.Join(repo, "src/index.js")) {
		t.Error("a file the checkpoint holds was not brought back")
	}
	if exists(filepath.Join(repo, "src/added.ts")) {
		t.Error("a file the checkpoint does not hold was left in the worktree")
	}
	if !exists(filepath.Join(repo, "node_modules/left-alone.txt")) {
		t.Error("a restore removed an ignored file")
	}
}

// Deleting a checkpoint's ref removes the name and leaves the commit, which is what trims a card's
// list without rewriting the branch it was made from.
func TestDeletingACheckpointRefKeepsTheCommitAndTheBranch(t *testing.T) {
	repo := fixture(t, "small-repo")
	g := testGit()
	ctx := context.Background()

	writeNewFile(t, repo, "src/one.ts", "one\n")
	ref := gitx.CheckpointRef("01JD7Q4M2X8K9V0P5T3RB6NHE9")
	state, err := g.Checkpoint(ctx, repo, ref, "Marshal checkpoint")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if err := g.DeleteCheckpointRef(ctx, repo, ref); err != nil {
		t.Fatalf("DeleteCheckpointRef: %v", err)
	}
	if _, err := g.CheckpointCommit(ctx, repo, ref); err == nil {
		t.Error("the hidden ref is still there after it was deleted")
	}
	// The commit itself is still reachable from the branch, so nothing was rewritten.
	if head := head(t, repo); head != state.SHA {
		t.Errorf("the branch moved: it is on %s, want the checkpoint's %s", head, state.SHA)
	}
}

// A checkpoint needs a place and a name, and a restore needs a commit: each is refused rather than
// run against an empty argument.
func TestTheCheckpointCallsRefuseEmptyArguments(t *testing.T) {
	repo := fixture(t, "small-repo")
	g := testGit()
	ctx := context.Background()

	if _, err := g.Checkpoint(ctx, "", gitx.CheckpointRef("x"), "m"); err == nil {
		t.Error("a checkpoint with no worktree was not refused")
	}
	if _, err := g.Checkpoint(ctx, repo, "", "m"); err == nil {
		t.Error("a checkpoint with no ref was not refused")
	}
	if err := g.RestoreCheckpoint(ctx, repo, ""); err == nil {
		t.Error("a restore with no commit was not refused")
	}
	if err := g.RestoreCheckpoint(ctx, "", "abc"); err == nil {
		t.Error("a restore with no worktree was not refused")
	}
}

// A restore to a commit that is not there fails rather than quietly doing nothing, so a card whose
// checkpoint was garbage-collected is told, not left half-restored.
func TestRestoringToACommitThatIsNotThereFails(t *testing.T) {
	repo := fixture(t, "small-repo")
	g := testGit()
	before := head(t, repo)

	if err := g.RestoreCheckpoint(context.Background(), repo, "0123456789012345678901234567890123456789"); err == nil {
		t.Fatal("a restore to a commit that does not exist was not refused")
	}
	if head(t, repo) != before {
		t.Error("a failed restore moved the branch")
	}
}

// readFile reads a file under dir as text.
func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
