package gitx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Primitives for the owner's own folder, the one project.Path names. The rules here are the ones
// that keep a person's work safe:
//
//   - A read never touches the folder's index. Marshal works on a temporary copy of it, pointed at
//     with GIT_INDEX_FILE, and the folder's own index changes only at the very end of a move.
//   - Nothing resets, checks out over, or cleans the folder. Files change only through
//     `git read-tree -m -u`, which refuses to write over a file that is not what Git last saw.
//   - Before a branch moves, the folder's state is read again and compared with what the caller
//     planned from. Any difference is ErrFolderChanged and nothing is written.

// The cases a caller handles when it works in the owner's folder.
var (
	// ErrLocalChanges means Git refused a fast-forward because files in the folder would be
	// overwritten: uncommitted changes, or untracked files in the way.
	ErrLocalChanges = errors.New("changes in the folder would be overwritten")
	// ErrFolderBusy means another Git operation is under way in the folder: a merge, a rebase, a
	// cherry-pick, a revert, unresolved conflicts, or a program holding the index lock.
	ErrFolderBusy = errors.New("another Git operation is under way in the folder")
	// ErrFolderChanged means the folder is not as it was when the caller read it. Nothing was
	// written, and reading it again is the way on.
	ErrFolderChanged = errors.New("the folder changed while Marshal was reading it")
)

// indexRetries and indexRetryWait bound the wait for the folder's index to be free again when the
// last step of a move writes it.
const (
	indexRetries   = 3
	indexRetryWait = 100 * time.Millisecond
)

// FolderHead is what a working folder has checked out.
type FolderHead struct {
	// Commit is the commit HEAD points at.
	Commit string
	// Branch is the checked out branch. It is empty when HEAD is detached.
	Branch string
}

// ReadFolderHead reads what a working folder has checked out.
func (g *Git) ReadFolderHead(ctx context.Context, dir string) (FolderHead, error) {
	commit, err := g.revParse(ctx, dir, "HEAD")
	if err != nil {
		return FolderHead{}, err
	}
	out, code, err := g.runOutput(ctx, dir, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return FolderHead{}, fmt.Errorf("read the checked out branch: %w", err)
	}
	if code != 0 {
		return FolderHead{Commit: commit}, nil
	}
	return FolderHead{Commit: commit, Branch: strings.TrimPrefix(strings.TrimSpace(out), headsPrefix)}, nil
}

// Busy says what Git is in the middle of in a folder. The zero value means nothing is.
type Busy struct {
	// Operation names a long operation the person has to finish or abort: "merge", "rebase",
	// "cherry-pick", "revert", or "conflict" for files still unresolved. Empty when none.
	Operation string
	// Locked is true when the folder's index is locked, which is usually a program that will let go
	// in a moment.
	Locked bool
}

// None reports whether the folder is free.
func (b Busy) None() bool { return b.Operation == "" && !b.Locked }

// FolderBusy looks for an operation in the middle of a folder, using only reads. The places are
// asked of Git (`rev-parse --git-path`), so a linked worktree or a moved Git folder is found too.
func (g *Git) FolderBusy(ctx context.Context, dir string) (Busy, error) {
	names := []struct{ file, operation string }{
		{"MERGE_HEAD", subcommandMerge}, {"rebase-merge", "rebase"}, {"rebase-apply", "rebase"},
		{"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"}, {"index.lock", ""},
	}
	args := []string{"rev-parse"}
	for _, n := range names {
		args = append(args, "--git-path", n.file)
	}
	out, err := g.Run(ctx, dir, args...)
	if err != nil {
		return Busy{}, fmt.Errorf("look for a Git operation in %s: %w", dir, err)
	}
	paths := strings.Split(out, "\n")
	if len(paths) != len(names) {
		return Busy{}, fmt.Errorf("look for a Git operation in %s: unexpected answer %q", dir, out)
	}
	for i, n := range names {
		path := paths[i]
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if _, err := os.Lstat(path); err != nil {
			continue
		}
		if n.file == "index.lock" {
			return Busy{Locked: true}, nil
		}
		return Busy{Operation: n.operation}, nil
	}
	unmerged, err := g.Run(ctx, dir, "--no-optional-locks", "ls-files", "--unmerged", "-z")
	if err != nil {
		return Busy{}, fmt.Errorf("look for files in conflict in %s: %w", dir, err)
	}
	if unmerged != "" {
		return Busy{Operation: "conflict"}, nil
	}
	return Busy{}, nil
}

// tempIndex is a private copy of a folder's index, in a folder of its own outside the working
// folder, that Git can be pointed at with GIT_INDEX_FILE.
type tempIndex struct {
	root string
	path string
}

// newTempIndex copies the folder's index, so Git keeps the stat information it needs to avoid
// reading every file again. A folder with no index yet starts from HEAD.
func (g *Git) newTempIndex(ctx context.Context, dir string) (*tempIndex, error) {
	root, err := os.MkdirTemp("", "marshal-index-")
	if err != nil {
		return nil, fmt.Errorf("make a temporary index: %w", err)
	}
	idx := &tempIndex{root: root, path: filepath.Join(root, "index")}
	real, err := g.Run(ctx, dir, "rev-parse", "--git-path", "index")
	if err == nil {
		if !filepath.IsAbs(real) {
			real = filepath.Join(dir, real)
		}
		err = copyFile(real, idx.path)
	}
	if err != nil {
		if _, headErr := g.runEnv(ctx, dir, idx.env(), "read-tree", "HEAD"); headErr != nil {
			idx.close()
			return nil, fmt.Errorf("copy the index of %s: %w", dir, errors.Join(err, headErr))
		}
	}
	return idx, nil
}

func (t *tempIndex) env() []string { return []string{"GIT_INDEX_FILE=" + t.path} }

func (t *tempIndex) close() { _ = os.RemoveAll(t.root) }

// copyFile copies one file, writing it with the same permission as an index.
func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// stage brings the temporary index up to what the folder holds now: every file, new ones too, or
// only the files Git already tracks. It answers the tree of the result.
func (g *Git) stage(ctx context.Context, dir string, idx *tempIndex, tracked bool) (string, error) {
	flag := "-A"
	if tracked {
		flag = "-u"
	}
	if _, err := g.runEnv(ctx, dir, idx.env(), "add", flag); err != nil {
		return "", fmt.Errorf("read the files of %s: %w", dir, err)
	}
	tree, err := g.runEnv(ctx, dir, idx.env(), "write-tree")
	if err != nil {
		return "", fmt.Errorf("write the files of %s as a tree: %w", dir, err)
	}
	return tree, nil
}

// Snapshot is a commit of everything in a folder, made without touching the folder.
type Snapshot struct {
	// Head is the commit the folder had checked out, and the snapshot's parent.
	Head string
	// Tree is every file in the folder: tracked, changed, and untracked, but not ignored.
	Tree string
	// Commit is the snapshot commit, pinned at the ref it was made for.
	Commit string
}

// SnapshotFolder commits what the folder holds, changes and untracked files included, onto a ref of
// its own, so the owner's work is kept in Git before anything else is done. The folder and its
// index are not touched: the commit is built in a temporary index.
//
// The ref must be under refs/marshal/. Nothing else in the repository moves.
func (g *Git) SnapshotFolder(ctx context.Context, dir, ref, message string) (Snapshot, error) {
	if !strings.HasPrefix(ref, "refs/marshal/") {
		return Snapshot{}, newOpError(ErrBadPath, "a snapshot is kept under refs/marshal/", nil)
	}
	head, err := g.revParse(ctx, dir, "HEAD")
	if err != nil {
		return Snapshot{}, err
	}
	idx, err := g.newTempIndex(ctx, dir)
	if err != nil {
		return Snapshot{}, err
	}
	defer idx.close()
	tree, err := g.stage(ctx, dir, idx, false)
	if err != nil {
		return Snapshot{}, err
	}
	args := append(identityArgs(), "commit-tree", tree, "-p", head, "-m", message)
	commit, err := g.Run(ctx, dir, args...)
	if err != nil {
		return Snapshot{}, fmt.Errorf("commit a snapshot of %s: %w", dir, err)
	}
	if _, err := g.Run(ctx, dir, "update-ref", ref, commit); err != nil {
		return Snapshot{}, fmt.Errorf("keep a snapshot of %s at %s: %w", dir, ref, err)
	}
	return Snapshot{Head: head, Tree: tree, Commit: commit}, nil
}

// PinRef points a ref under refs/marshal/ at a commit, so the commit is kept.
func (g *Git) PinRef(ctx context.Context, repo, ref, commit string) error {
	if !strings.HasPrefix(ref, "refs/marshal/") {
		return newOpError(ErrBadPath, "only refs under refs/marshal/ are pinned", nil)
	}
	if err := checkRevision(commit); err != nil {
		return err
	}
	if _, err := g.Run(ctx, repo, "update-ref", ref, commit); err != nil {
		return fmt.Errorf("pin %s at %s: %w", ref, commit, err)
	}
	return nil
}

// ForgetRef deletes a ref under refs/marshal/. A ref that is not there is not an error.
func (g *Git) ForgetRef(ctx context.Context, repo, ref string) error {
	if !strings.HasPrefix(ref, "refs/marshal/") {
		return newOpError(ErrBadPath, "only refs under refs/marshal/ are forgotten", nil)
	}
	if _, err := g.Run(ctx, repo, "update-ref", "-d", ref); err != nil {
		return fmt.Errorf("forget %s: %w", ref, err)
	}
	return nil
}

// PruneRefs keeps the newest `keep` refs under prefix and deletes the rest. Names sort oldest
// first, which holds for refs named by a fixed width timestamp.
func (g *Git) PruneRefs(ctx context.Context, repo, prefix string, keep int) error {
	if !strings.HasPrefix(prefix, "refs/marshal/") {
		return newOpError(ErrBadPath, "only refs under refs/marshal/ are pruned", nil)
	}
	out, err := g.Run(ctx, repo, "for-each-ref", "--format=%(refname)", prefix)
	if err != nil {
		return fmt.Errorf("list %s: %w", prefix, err)
	}
	refs := splitLines(out)
	for i := 0; i < len(refs)-keep; i++ {
		if err := g.ForgetRef(ctx, repo, refs[i]); err != nil {
			return err
		}
	}
	return nil
}

// FolderMatches reports whether the folder holds exactly the given tree. With tracked set, only the
// files Git already tracks are compared, so an untracked file does not count.
func (g *Git) FolderMatches(ctx context.Context, dir, tree string, tracked bool) (bool, error) {
	idx, err := g.newTempIndex(ctx, dir)
	if err != nil {
		return false, err
	}
	defer idx.close()
	now, err := g.stage(ctx, dir, idx, tracked)
	if err != nil {
		return false, err
	}
	want, err := g.Run(ctx, dir, "rev-parse", "--verify", tree+"^{tree}")
	if err != nil {
		return false, fmt.Errorf("resolve tree %s: %w", tree, err)
	}
	return now == want, nil
}

// DeletedFiles lists the tracked files that are gone from the folder but not yet removed from the
// index: the owner deleted them and has not committed or staged the deletion. Git's own fast-forward
// does not count these as changes, and brings a file back when the incoming commit changes it, so
// a caller that must not do that looks here first.
func (g *Git) DeletedFiles(ctx context.Context, dir string) ([]string, error) {
	out, err := g.Run(ctx, dir, "--no-optional-locks", "ls-files", "--deleted", "-z")
	if err != nil {
		return nil, fmt.Errorf("list the files deleted in %s: %w", dir, err)
	}
	return splitNul(out), nil
}

// FastForwardFolder moves the branch checked out in the owner's folder forward to commit with
// Git's own `merge --ff-only`. Git changes files only when it can do so without losing anything,
// and refuses otherwise: that refusal is ErrLocalChanges. A commit that is not ahead of the branch
// is ErrNotFastForward. Hooks are off.
func (g *Git) FastForwardFolder(ctx context.Context, dir, commit string) error {
	if err := checkRevision(commit); err != nil {
		return err
	}
	args := []string{"-c", "core.hooksPath=" + os.DevNull, subcommandMerge, "--ff-only", "--no-edit", "--quiet", "--", commit}
	out, code, err := g.runOutput(ctx, dir, args...)
	if err != nil {
		return fmt.Errorf("fast-forward %s: %w", dir, err)
	}
	if code == 0 {
		return nil
	}
	switch {
	case strings.Contains(out, "would be overwritten"), strings.Contains(out, "not uptodate"),
		strings.Contains(out, "commit your changes or stash"):
		return newOpError(ErrLocalChanges, firstLine(out), nil)
	case strings.Contains(out, "fast-forward"):
		return newOpError(ErrNotFastForward, commit, nil)
	}
	return fmt.Errorf("fast-forward %s: git merge exited %d: %s", dir, code, strings.TrimSpace(out))
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

// MoveFolder is the one move of the owner's folder from one state to another: the branch goes from
// From to To, the files go from the tree Before to the tree After, and the index ends at To.
type MoveFolder struct {
	// Dir is the owner's folder, with Branch checked out at From.
	Dir    string
	Branch string
	From   string
	To     string
	// Before is the tree the folder must hold now, changes and untracked files included. Another
	// state is ErrFolderChanged and nothing is written.
	Before string
	// After is the tree the folder's files end up as.
	After string
	// Tracked compares only tracked files against Before, for a folder whose untracked files are
	// not part of what was planned from.
	Tracked bool
}

// Move carries out a MoveFolder in the order that can always be taken back:
//
//  1. read the folder again, in a temporary index, and refuse if it is not Before;
//  2. move the branch with a compare-and-swap, so a branch that moved is refused;
//  3. update the files with `read-tree -m -u Before After` in the temporary index, which writes
//     nothing at all if any file is not what the index last saw; if it refuses, the branch is put
//     back;
//  4. set the folder's own index to To, so everything that differs is plain uncommitted work.
//
// Nothing is reset, checked out, or cleaned.
func (g *Git) Move(ctx context.Context, in MoveFolder) error {
	if err := g.checkMoveReady(ctx, in); err != nil {
		return err
	}
	idx, err := g.newTempIndex(ctx, in.Dir)
	if err != nil {
		return err
	}
	defer idx.close()
	now, err := g.stage(ctx, in.Dir, idx, in.Tracked)
	if err != nil {
		return err
	}
	before, err := g.Run(ctx, in.Dir, "rev-parse", "--verify", in.Before+"^{tree}")
	if err != nil {
		return fmt.Errorf("resolve tree %s: %w", in.Before, err)
	}
	if now != before {
		return newOpError(ErrFolderChanged, "its files are not what Marshal planned from", nil)
	}
	if err := g.SwapBranch(ctx, in.Dir, in.Branch, in.From, in.To); err != nil {
		return err
	}
	if _, err := g.runEnv(ctx, in.Dir, idx.env(), "read-tree", "-m", "-u", in.Before, in.After); err != nil {
		g.putBranchBack(ctx, in)
		return newOpError(ErrFolderChanged, "a file changed under Marshal", err)
	}
	return g.setIndex(ctx, in.Dir, in.To)
}

// checkMoveReady refuses a move when the folder is busy or is not on the branch at From.
func (g *Git) checkMoveReady(ctx context.Context, in MoveFolder) error {
	busy, err := g.FolderBusy(ctx, in.Dir)
	if err != nil {
		return err
	}
	if !busy.None() {
		return newOpError(ErrFolderBusy, busyText(busy), nil)
	}
	head, err := g.ReadFolderHead(ctx, in.Dir)
	if err != nil {
		return err
	}
	if head.Branch != in.Branch || head.Commit != in.From {
		return newOpError(ErrFolderChanged, "it is not on "+in.Branch+" where it was", nil)
	}
	return nil
}

// putBranchBack undoes the branch move of a move that could not finish. It uses its own context,
// so a cancelled caller still gets the branch back.
func (g *Git) putBranchBack(ctx context.Context, in MoveFolder) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	_ = g.SwapBranch(ctx, in.Dir, in.Branch, in.To, in.From)
}

// setIndex points the folder's own index at a commit, with a few tries because a program may hold
// the index lock for a moment. Git reads the files again the next time it asks for the status.
func (g *Git) setIndex(ctx context.Context, dir, commit string) error {
	var err error
	for try := range indexRetries {
		if try > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(indexRetryWait):
			}
		}
		// No -m: a merging read-tree refuses an entry whose file differs from the old index, which
		// is exactly what the card's work and the owner's edits made.
		if _, err = g.Run(ctx, dir, "read-tree", commit); err == nil {
			return nil
		}
	}
	return fmt.Errorf("set the index of %s to %s: %w", dir, commit, err)
}

// busyText is a plain phrase for what a folder is in the middle of.
func busyText(b Busy) string {
	if b.Locked {
		return "another program holds the index lock"
	}
	return "a " + b.Operation + " is not finished"
}
