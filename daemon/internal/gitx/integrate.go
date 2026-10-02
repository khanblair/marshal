package gitx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Primitives for the Integrator's own workspace: one worktree on one branch that Marshal owns,
// where card branches are merged before the result is delivered. The folder is Marshal's, so it may
// be reset and cleaned. The owner's folder never is: see folder.go.

// ErrBranchInUse means a branch is checked out in a worktree other than the one asked for.
var ErrBranchInUse = errors.New("that branch is already checked out in another worktree")

// ErrBranchMoved means a branch is not where the caller last saw it, so the caller's move was
// refused rather than overwritten.
var ErrBranchMoved = errors.New("the branch moved while Marshal was working")

// identityArgs is the author Marshal commits with, with signing off, so a machine that has no Git
// identity or a signing key still commits.
func identityArgs() []string {
	return []string{"-c", "user.name=Marshal", "-c", "user.email=marshal@localhost", "-c", "commit.gpgsign=false"}
}

// Rev resolves a branch or commit name to a full commit id.
func (g *Git) Rev(ctx context.Context, repo, rev string) (string, error) {
	if err := checkRevision(rev); err != nil {
		return "", err
	}
	return g.revParse(ctx, repo, rev)
}

// CountAhead says how many commits tip has that base does not: `git rev-list --count base..tip`.
func (g *Git) CountAhead(ctx context.Context, repo, base, tip string) (int, error) {
	if err := checkRevision(base); err != nil {
		return 0, err
	}
	if err := checkRevision(tip); err != nil {
		return 0, err
	}
	out, err := g.Run(ctx, repo, "rev-list", "--count", base+".."+tip)
	if err != nil {
		return 0, fmt.Errorf("count the commits %s has beyond %s: %w", tip, base, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("read the commit count %q: %w", out, err)
	}
	return n, nil
}

// TreeOf resolves a commit or ref to the id of its tree.
func (g *Git) TreeOf(ctx context.Context, repo, rev string) (string, error) {
	if err := checkRevision(rev); err != nil {
		return "", err
	}
	tree, err := g.Run(ctx, repo, "rev-parse", "--verify", rev+"^{tree}")
	if err != nil {
		return "", fmt.Errorf("resolve the tree of %s: %w", rev, err)
	}
	return tree, nil
}

// ChangedSince lists the files a branch changes since it split from base, in name order.
func (g *Git) ChangedSince(ctx context.Context, repo, base, branch string) ([]string, error) {
	if err := checkRevision(base); err != nil {
		return nil, err
	}
	if err := checkRevision(branch); err != nil {
		return nil, err
	}
	return g.changedFiles(ctx, repo, base, branch)
}

// SameFolder reports whether two paths lead to the same folder, however they are spelled. A
// temporary folder on macOS is reached through /var and listed by Git as /private/var, so paths
// are never compared as strings.
func SameFolder(a, b string) bool { return sameFolderAs(a, b) }

// CheckedOutAt finds the worktree that has branch checked out. The second answer is false when no
// worktree does.
func (g *Git) CheckedOutAt(ctx context.Context, repo, branch string) (Worktree, bool, error) {
	list, err := g.ListWorktrees(ctx, repo)
	if err != nil {
		return Worktree{}, false, err
	}
	for _, wt := range list {
		if wt.Branch == branch && !wt.Prunable {
			return wt, true, nil
		}
	}
	return Worktree{}, false, nil
}

// EnsureBranchWorktree makes path a worktree of repo with branch checked out, creating the branch
// at base when it does not exist. A worktree whose folder was deleted is forgotten and made again;
// a folder that is not a worktree, or is on another branch, is replaced. The path must be inside
// root, which is what makes replacing it safe.
func (g *Git) EnsureBranchWorktree(ctx context.Context, repo, path, root, branch, base string) error {
	dir, err := checkInsideRoot(path, root)
	if err != nil {
		return err
	}
	if err := g.ValidBranchName(ctx, branch); err != nil {
		return err
	}
	_, statErr := os.Lstat(dir)
	present := statErr == nil
	if !present {
		if err := g.PruneWorktrees(ctx, repo); err != nil {
			return err
		}
	}
	list, err := g.ListWorktrees(ctx, repo)
	if err != nil {
		return err
	}
	here := false
	for _, wt := range list {
		switch {
		case present && sameFolderAs(wt.Path, dir):
			if wt.Branch == branch {
				return nil
			}
			here = true
		case wt.Branch == branch:
			return newOpError(ErrBranchInUse, wt.Path, nil)
		}
	}
	if here || present {
		if err := g.RemoveWorktree(ctx, repo, dir, root, true); err != nil {
			return err
		}
	}
	return g.addBranchWorktree(ctx, repo, dir, branch, base)
}

// addBranchWorktree checks an existing branch out at dir, or makes the branch at base first.
func (g *Git) addBranchWorktree(ctx context.Context, repo, dir, branch, base string) error {
	exists, err := g.BranchExists(ctx, repo, branch)
	if err != nil {
		return err
	}
	if !exists {
		return g.AddWorktree(ctx, repo, WorktreeSpec{Path: dir, Branch: branch, Base: base})
	}
	if _, err := g.Run(ctx, repo, subcommandWorktree, "add", "--", dir, branch); err != nil {
		return fmt.Errorf("check %s out at %s: %w", branch, dir, err)
	}
	return nil
}

// ResetWorktree puts a worktree of Marshal's own back to rev, dropping a merge that is half done,
// every change, and every untracked file that is not ignored. The folder must be inside root: this
// is never done to the owner's folder.
func (g *Git) ResetWorktree(ctx context.Context, dir, root, rev string) error {
	clean, err := checkInsideRoot(dir, root)
	if err != nil {
		return err
	}
	if rev == "" {
		rev = "HEAD"
	}
	if err := checkRevision(rev); err != nil {
		return err
	}
	// Nothing to abort is not a problem, and a failed abort is followed by the reset below.
	_ = g.AbortMerge(ctx, clean)
	if _, err := g.Run(ctx, clean, "reset", "--hard", "--quiet", rev); err != nil {
		return fmt.Errorf("reset %s to %s: %w", clean, rev, err)
	}
	if _, err := g.Run(ctx, clean, "clean", "-fdq"); err != nil {
		return fmt.Errorf("clean %s: %w", clean, err)
	}
	return nil
}

// FastForwardWorktree moves the branch checked out in a worktree of Marshal's own forward to rev,
// and refuses (ErrNotFastForward) when that would not be a fast-forward.
func (g *Git) FastForwardWorktree(ctx context.Context, dir, rev string) error {
	if err := checkRevision(rev); err != nil {
		return err
	}
	out, code, err := g.runOutput(ctx, dir, subcommandMerge, "--ff-only", "--quiet", "--", rev)
	if err != nil {
		return fmt.Errorf("fast-forward to %s: %w", rev, err)
	}
	if code == 0 {
		return nil
	}
	if strings.Contains(out, "fast-forward") {
		return newOpError(ErrNotFastForward, rev, nil)
	}
	return fmt.Errorf("fast-forward to %s: git merge exited %d: %s", rev, code, strings.TrimSpace(out))
}

// StartMerge merges rev into the worktree's branch without committing, and answers the files that
// conflict. A clean merge answers none and waits, staged, for CommitMerge or WriteTree. Marshal's
// own identity is used, as in MergeInto.
func (g *Git) StartMerge(ctx context.Context, dir, rev string) ([]string, error) {
	if err := checkRevision(rev); err != nil {
		return nil, err
	}
	args := append(identityArgs(), subcommandMerge, "--no-commit", "--no-ff", "--", rev)
	out, code, err := g.runOutput(ctx, dir, args...)
	if err != nil {
		return nil, fmt.Errorf("merge %s: %w", rev, err)
	}
	if code == 0 {
		return nil, nil
	}
	if !strings.Contains(out, "CONFLICT") && !strings.Contains(out, "conflict") {
		return nil, fmt.Errorf("merge %s: git merge exited %d: %s", rev, code, strings.TrimSpace(out))
	}
	return g.UnmergedPaths(ctx, dir)
}

// UnmergedPaths lists the files of a merge that are still in conflict, in name order.
func (g *Git) UnmergedPaths(ctx context.Context, dir string) ([]string, error) {
	out, err := g.Run(ctx, dir, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, fmt.Errorf("list the files in conflict: %w", err)
	}
	return splitNul(out), nil
}

// StageResolved stages the given files as they are now, so a conflict the Integrator resolved by
// editing the file counts as resolved. A path Git does not know is skipped: an agent may name a
// file it did not touch. Only the named files are staged, never the whole worktree.
func (g *Git) StageResolved(ctx context.Context, dir string, paths []string) error {
	for _, path := range paths {
		if !filepath.IsLocal(filepath.FromSlash(path)) {
			continue
		}
		if _, err := g.Run(ctx, dir, "add", "-A", "--", path); err != nil {
			if strings.Contains(err.Error(), "did not match any files") {
				continue
			}
			return fmt.Errorf("stage %s: %w", path, err)
		}
	}
	return nil
}

// CommitMerge commits a merge that is waiting in the worktree, with the given message, and answers
// the commit. Hooks and signing are off.
func (g *Git) CommitMerge(ctx context.Context, dir, message string) (string, error) {
	args := append(identityArgs(), "commit", "--no-verify", "--quiet", "-m", message)
	if _, err := g.Run(ctx, dir, args...); err != nil {
		return "", fmt.Errorf("commit the merge: %w", err)
	}
	return g.revParse(ctx, dir, "HEAD")
}

// WriteTree writes the staged files of a worktree as a tree and answers its id. It fails while any
// file is still in conflict.
func (g *Git) WriteTree(ctx context.Context, dir string) (string, error) {
	tree, err := g.Run(ctx, dir, "write-tree")
	if err != nil {
		return "", fmt.Errorf("write the staged files as a tree: %w", err)
	}
	return tree, nil
}

// ConflictMarkers answers which of the given files still hold the markers of an unresolved
// conflict. A file counts when it has a line that starts with seven "<" or seven ">" characters,
// which no resolved file keeps. A file that is gone, or looks binary, is not read.
func ConflictMarkers(dir string, paths []string) ([]string, error) {
	var left []string
	for _, path := range paths {
		rel := filepath.FromSlash(path)
		if !filepath.IsLocal(rel) {
			continue
		}
		found, err := fileHasMarkers(filepath.Join(dir, rel))
		if err != nil {
			return nil, err
		}
		if found {
			left = append(left, path)
		}
	}
	return left, nil
}

// markerScanBytes is the longest line the marker scan reads.
const markerScanBytes = 1 << 20

// fileHasMarkers reads a file line by line for a leftover conflict marker.
func fileHasMarkers(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), markerScanBytes)
	for first := true; scanner.Scan(); first = false {
		line := scanner.Text()
		if first && strings.ContainsRune(line, 0) {
			return false, nil
		}
		if isMarkerLine(line, '<') || isMarkerLine(line, '>') {
			return true, nil
		}
	}
	// A line longer than the buffer is not a marker line, so the scan simply stops there.
	return false, nil
}

// isMarkerLine reports whether a line is seven of one marker character, then a space or the end.
func isMarkerLine(line string, marker byte) bool {
	const run = 7
	if len(line) < run {
		return false
	}
	for i := range run {
		if line[i] != marker {
			return false
		}
	}
	return len(line) == run || line[run] == ' '
}

// SwapBranch moves a branch from one commit to another, but only while the branch is still at
// from: Git's own compare-and-swap. A branch that moved is ErrBranchMoved and is left alone. It
// moves a branch in either direction, so it is for callers that know both ends, such as an undo.
func (g *Git) SwapBranch(ctx context.Context, repo, branch, from, to string) error {
	for _, name := range []string{branch, from, to} {
		if err := checkRevision(name); err != nil {
			return err
		}
	}
	if _, err := g.Run(ctx, repo, "update-ref", "--create-reflog", headsPrefix+branch, to, from); err != nil {
		if tip, tipErr := g.revParse(ctx, repo, headsPrefix+branch); tipErr == nil && tip != from {
			return newOpError(ErrBranchMoved, branch, err)
		}
		return fmt.Errorf("move %s from %s to %s: %w", branch, from, to, err)
	}
	return nil
}

// splitNul splits NUL separated output into its non-empty parts.
func splitNul(out string) []string {
	var parts []string
	for _, part := range strings.Split(out, "\x00") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}
