package gitx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

// multiLine is a file long enough that two edits far apart merge without a conflict.
const multiLine = "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"

// folderRepo makes a repository whose main branch holds a.txt (multiLine) and b.txt, which is the
// owner's folder in these tests.
func folderRepo(t *testing.T) string {
	t.Helper()
	repo := newEmptyRepo(t)
	commitFile(t, repo, "a.txt", multiLine)
	commitFile(t, repo, "b.txt", "b base\n")
	return repo
}

// advanceMain makes a new commit on a branch off main without touching the checked out folder, and
// answers the commit. It is how a card's merged work is made ahead of the owner's folder.
func advanceMain(t *testing.T, repo, file, content string) string {
	t.Helper()
	ws := filepath.Join(t.TempDir(), "ws")
	git(t, repo, "worktree", "add", "--quiet", "--detach", ws, "main")
	commitFile(t, ws, file, content)
	tip := git(t, ws, "rev-parse", "HEAD")
	git(t, repo, "worktree", "remove", "--force", ws)
	git(t, repo, "branch", "-f", "ahead", tip)
	return tip
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestFastForwardFolderUpdatesACleanFolder(t *testing.T) {
	repo := folderRepo(t)
	tip := advanceMain(t, repo, "b.txt", "b by the card\n")
	if err := testGit().FastForwardFolder(context.Background(), repo, tip); err != nil {
		t.Fatalf("FastForwardFolder: %v", err)
	}
	if got := read(t, repo, "b.txt"); got != "b by the card\n" {
		t.Errorf("b.txt = %q, want the card's change", got)
	}
	if status := git(t, repo, "status", "--porcelain"); status != "" {
		t.Errorf("status = %q, want clean", status)
	}
}

func TestFastForwardFolderKeepsChangesToOtherFiles(t *testing.T) {
	repo := folderRepo(t)
	write(t, repo, "a.txt", "a edited by the owner\n")
	tip := advanceMain(t, repo, "b.txt", "b by the card\n")
	if err := testGit().FastForwardFolder(context.Background(), repo, tip); err != nil {
		t.Fatalf("FastForwardFolder: %v", err)
	}
	if got := read(t, repo, "a.txt"); got != "a edited by the owner\n" {
		t.Errorf("a.txt = %q, want the owner's edit kept", got)
	}
	if got := read(t, repo, "b.txt"); got != "b by the card\n" {
		t.Errorf("b.txt = %q, want the card's change", got)
	}
}

func TestFastForwardFolderRefusesChangesItWouldOverwrite(t *testing.T) {
	repo := folderRepo(t)
	write(t, repo, "b.txt", "b edited by the owner\n")
	tip := advanceMain(t, repo, "b.txt", "b by the card\n")
	err := testGit().FastForwardFolder(context.Background(), repo, tip)
	if !errors.Is(err, gitx.ErrLocalChanges) {
		t.Fatalf("FastForwardFolder = %v, want ErrLocalChanges", err)
	}
	if got := read(t, repo, "b.txt"); got != "b edited by the owner\n" {
		t.Errorf("b.txt = %q, want the owner's edit untouched", got)
	}
}

func TestFastForwardFolderRefusesAnUntrackedFileInTheWay(t *testing.T) {
	repo := folderRepo(t)
	write(t, repo, "new.txt", "the owner's own file\n")
	tip := advanceMain(t, repo, "new.txt", "the card's file\n")
	if err := testGit().FastForwardFolder(context.Background(), repo, tip); !errors.Is(err, gitx.ErrLocalChanges) {
		t.Fatalf("FastForwardFolder = %v, want ErrLocalChanges", err)
	}
	if got := read(t, repo, "new.txt"); got != "the owner's own file\n" {
		t.Errorf("new.txt = %q, want the owner's file untouched", got)
	}
}

func TestFastForwardFolderRefusesACommitThatIsNotAhead(t *testing.T) {
	repo := folderRepo(t)
	tip := advanceMain(t, repo, "b.txt", "b by the card\n")
	commitFile(t, repo, "c.txt", "c on main\n")
	if err := testGit().FastForwardFolder(context.Background(), repo, tip); !errors.Is(err, gitx.ErrNotFastForward) {
		t.Fatalf("FastForwardFolder = %v, want ErrNotFastForward", err)
	}
}

func TestSnapshotFolderKeepsEverythingAndTouchesNothing(t *testing.T) {
	repo := folderRepo(t)
	write(t, repo, "a.txt", "a edited\n")
	write(t, repo, "untracked.txt", "untracked\n")
	if err := os.Remove(filepath.Join(repo, "b.txt")); err != nil {
		t.Fatal(err)
	}
	statusBefore := git(t, repo, "status", "--porcelain")
	indexBefore, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	head := git(t, repo, "rev-parse", "HEAD")

	snap, err := testGit().SnapshotFolder(context.Background(), repo, "refs/marshal/wip/p/1", "wip")
	if err != nil {
		t.Fatalf("SnapshotFolder: %v", err)
	}
	if snap.Head != head || git(t, repo, "rev-parse", snap.Commit+"^") != head {
		t.Errorf("snapshot parent is not the folder's HEAD")
	}
	if git(t, repo, "rev-parse", "refs/marshal/wip/p/1") != snap.Commit {
		t.Errorf("the snapshot is not pinned at its ref")
	}
	if got := git(t, repo, "show", snap.Commit+":a.txt"); got != "a edited" {
		t.Errorf("snapshot a.txt = %q, want the edit", got)
	}
	if got := git(t, repo, "show", snap.Commit+":untracked.txt"); got != "untracked" {
		t.Errorf("snapshot untracked.txt = %q, want the untracked file", got)
	}
	if _, err := testGit().Run(context.Background(), repo, "cat-file", "-e", snap.Commit+":b.txt"); err == nil {
		t.Errorf("snapshot still has b.txt, which the owner deleted")
	}
	if git(t, repo, "status", "--porcelain") != statusBefore {
		t.Errorf("a snapshot changed the status of the folder")
	}
	indexAfter, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	if string(indexBefore) != string(indexAfter) {
		t.Errorf("a snapshot rewrote the folder's own index")
	}
}

func TestSnapshotFolderRefusesARefOutsideMarshal(t *testing.T) {
	repo := folderRepo(t)
	if _, err := testGit().SnapshotFolder(context.Background(), repo, "refs/heads/oops", "wip"); !errors.Is(err, gitx.ErrBadPath) {
		t.Errorf("SnapshotFolder(refs/heads/oops) = %v, want ErrBadPath", err)
	}
}

func TestFolderBusyFindsLocksAndHalfDoneOperations(t *testing.T) {
	repo := folderRepo(t)
	g := testGit()
	ctx := context.Background()
	if busy, err := g.FolderBusy(ctx, repo); err != nil || !busy.None() {
		t.Fatalf("FolderBusy of a quiet folder = %+v, %v; want none", busy, err)
	}
	lock := filepath.Join(repo, ".git", "index.lock")
	write(t, repo, ".git/index.lock", "")
	if busy, _ := g.FolderBusy(ctx, repo); !busy.Locked {
		t.Errorf("FolderBusy with index.lock = %+v, want Locked", busy)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	write(t, repo, ".git/MERGE_HEAD", git(t, repo, "rev-parse", "HEAD")+"\n")
	if busy, _ := g.FolderBusy(ctx, repo); busy.Operation != "merge" {
		t.Errorf("FolderBusy with MERGE_HEAD = %+v, want a merge", busy)
	}
}

// reconciled makes what the Integrator makes: the owner's snapshot merged onto the new tip, in a
// worktree of its own, and answers the tree.
func reconciled(t *testing.T, repo, tip string, snap gitx.Snapshot) string {
	t.Helper()
	ws := filepath.Join(t.TempDir(), "integrator")
	git(t, repo, "worktree", "add", "--quiet", "--detach", ws, tip)
	conflicts, err := testGit().StartMerge(context.Background(), ws, snap.Commit)
	if err != nil || len(conflicts) > 0 {
		t.Fatalf("StartMerge = %v, %v; want a clean merge", conflicts, err)
	}
	tree, err := testGit().WriteTree(context.Background(), ws)
	if err != nil {
		t.Fatalf("WriteTree: %v", err)
	}
	return tree
}

func TestMoveDeliversTheCardAndKeepsTheOwnersEdits(t *testing.T) {
	repo := folderRepo(t)
	g := testGit()
	ctx := context.Background()
	head := git(t, repo, "rev-parse", "HEAD")
	// The owner edits the top of a.txt and has a file of their own; the card edits the bottom.
	write(t, repo, "a.txt", strings.Replace(multiLine, "one\n", "ONE by the owner\n", 1))
	write(t, repo, "notes.txt", "owner notes\n")
	tip := advanceMain(t, repo, "a.txt", strings.Replace(multiLine, "ten\n", "TEN by the card\n", 1))
	snap, err := g.SnapshotFolder(ctx, repo, "refs/marshal/wip/p/1", "wip")
	if err != nil {
		t.Fatal(err)
	}
	tree := reconciled(t, repo, tip, snap)

	err = g.Move(ctx, gitx.MoveFolder{
		Dir: repo, Branch: "main", From: head, To: tip, Before: snap.Tree, After: tree,
	})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if got := git(t, repo, "rev-parse", "main"); got != tip {
		t.Errorf("main = %s, want the new tip %s", got, tip)
	}
	want := strings.Replace(strings.Replace(multiLine, "one\n", "ONE by the owner\n", 1), "ten\n", "TEN by the card\n", 1)
	if got := read(t, repo, "a.txt"); got != want {
		t.Errorf("a.txt = %q, want both changes", got)
	}
	// Only the owner's own work is uncommitted: the card's work is in the commit.
	diff := git(t, repo, "diff", "HEAD", "--", "a.txt")
	if !strings.Contains(diff, "+ONE by the owner") || strings.Contains(diff, "TEN") {
		t.Errorf("diff against HEAD = %q, want only the owner's edit", diff)
	}
	if got := git(t, repo, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("staged files = %q, want the index at the new tip", got)
	}
	if read(t, repo, "notes.txt") != "owner notes\n" {
		t.Errorf("the owner's untracked file changed")
	}
}

func TestMoveRefusesAFolderThatChangedSinceItWasRead(t *testing.T) {
	repo := folderRepo(t)
	g := testGit()
	ctx := context.Background()
	head := git(t, repo, "rev-parse", "HEAD")
	write(t, repo, "a.txt", strings.Replace(multiLine, "one\n", "ONE by the owner\n", 1))
	tip := advanceMain(t, repo, "b.txt", "b by the card\n")
	snap, err := g.SnapshotFolder(ctx, repo, "refs/marshal/wip/p/1", "wip")
	if err != nil {
		t.Fatal(err)
	}
	tree := reconciled(t, repo, tip, snap)
	// The owner's editor writes again after the snapshot.
	write(t, repo, "a.txt", "an editor wrote this\n")

	err = g.Move(ctx, gitx.MoveFolder{Dir: repo, Branch: "main", From: head, To: tip, Before: snap.Tree, After: tree})
	if !errors.Is(err, gitx.ErrFolderChanged) {
		t.Fatalf("Move = %v, want ErrFolderChanged", err)
	}
	if got := git(t, repo, "rev-parse", "main"); got != head {
		t.Errorf("main moved to %s on a refused move", got)
	}
	if got := read(t, repo, "a.txt"); got != "an editor wrote this\n" {
		t.Errorf("a.txt = %q, want the editor's text untouched", got)
	}
}

func TestMoveRefusesABusyFolder(t *testing.T) {
	repo := folderRepo(t)
	head := git(t, repo, "rev-parse", "HEAD")
	write(t, repo, ".git/index.lock", "")
	err := testGit().Move(context.Background(), gitx.MoveFolder{
		Dir: repo, Branch: "main", From: head, To: head, Before: git(t, repo, "rev-parse", "HEAD^{tree}"),
		After: git(t, repo, "rev-parse", "HEAD^{tree}"),
	})
	if !errors.Is(err, gitx.ErrFolderBusy) {
		t.Fatalf("Move = %v, want ErrFolderBusy", err)
	}
}

func TestMoveTakesADeliveryBackToTheOwnersOwnFiles(t *testing.T) {
	repo := folderRepo(t)
	g := testGit()
	ctx := context.Background()
	head := git(t, repo, "rev-parse", "HEAD")
	owner := strings.Replace(multiLine, "one\n", "ONE by the owner\n", 1)
	write(t, repo, "a.txt", owner)
	write(t, repo, "notes.txt", "owner notes\n")
	tip := advanceMain(t, repo, "a.txt", strings.Replace(multiLine, "ten\n", "TEN by the card\n", 1))
	snap, err := g.SnapshotFolder(ctx, repo, "refs/marshal/wip/p/1", "wip")
	if err != nil {
		t.Fatal(err)
	}
	tree := reconciled(t, repo, tip, snap)
	if err := g.Move(ctx, gitx.MoveFolder{Dir: repo, Branch: "main", From: head, To: tip, Before: snap.Tree, After: tree}); err != nil {
		t.Fatalf("Move: %v", err)
	}

	err = g.Move(ctx, gitx.MoveFolder{Dir: repo, Branch: "main", From: tip, To: head, Before: tree, After: snap.Tree})
	if err != nil {
		t.Fatalf("Move back: %v", err)
	}
	if got := git(t, repo, "rev-parse", "main"); got != head {
		t.Errorf("main = %s after the undo, want %s", got, head)
	}
	if got := read(t, repo, "a.txt"); got != owner {
		t.Errorf("a.txt = %q, want the owner's own file back", got)
	}
	if read(t, repo, "notes.txt") != "owner notes\n" {
		t.Errorf("the owner's untracked file changed")
	}
	if got := git(t, repo, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("staged files = %q, want the index at the old tip", got)
	}
}

func TestFolderMatchesComparesTrackedFilesOnlyWhenAsked(t *testing.T) {
	repo := folderRepo(t)
	g := testGit()
	ctx := context.Background()
	tree := git(t, repo, "rev-parse", "HEAD^{tree}")
	write(t, repo, "untracked.txt", "x\n")
	if ok, err := g.FolderMatches(ctx, repo, tree, true); err != nil || !ok {
		t.Errorf("FolderMatches(tracked) = %v, %v; want true with only an untracked file", ok, err)
	}
	if ok, err := g.FolderMatches(ctx, repo, tree, false); err != nil || ok {
		t.Errorf("FolderMatches(all) = %v, %v; want false with an untracked file", ok, err)
	}
	write(t, repo, "a.txt", "changed\n")
	if ok, _ := g.FolderMatches(ctx, repo, tree, true); ok {
		t.Errorf("FolderMatches(tracked) = true with a changed file")
	}
}

func TestSwapBranchOnlyMovesABranchThatIsWhereTheCallerSawIt(t *testing.T) {
	repo := folderRepo(t)
	g := testGit()
	ctx := context.Background()
	old := git(t, repo, "rev-parse", "main")
	tip := advanceMain(t, repo, "b.txt", "b2\n")
	if err := g.SwapBranch(ctx, repo, "ahead", tip, old); err != nil {
		t.Fatalf("SwapBranch backward: %v", err)
	}
	if git(t, repo, "rev-parse", "ahead") != old {
		t.Errorf("the branch did not move back")
	}
	if err := g.SwapBranch(ctx, repo, "ahead", tip, tip); !errors.Is(err, gitx.ErrBranchMoved) {
		t.Errorf("SwapBranch from a stale tip = %v, want ErrBranchMoved", err)
	}
}

func TestPruneRefsKeepsTheNewest(t *testing.T) {
	repo := folderRepo(t)
	g := testGit()
	ctx := context.Background()
	for _, name := range []string{"1000", "1001", "1002", "1003"} {
		git(t, repo, "update-ref", "refs/marshal/wip/p/"+name, "HEAD")
	}
	if err := g.PruneRefs(ctx, repo, "refs/marshal/wip/p/", 2); err != nil {
		t.Fatalf("PruneRefs: %v", err)
	}
	left := git(t, repo, "for-each-ref", "--format=%(refname)", "refs/marshal/wip/p/")
	if left != "refs/marshal/wip/p/1002\nrefs/marshal/wip/p/1003" {
		t.Errorf("refs left = %q, want the two newest", left)
	}
	if err := g.PruneRefs(ctx, repo, "refs/heads/", 0); !errors.Is(err, gitx.ErrBadPath) {
		t.Errorf("PruneRefs(refs/heads/) = %v, want ErrBadPath", err)
	}
}
