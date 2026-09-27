package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The Integrator's merge-queue primitives (docs/architecture.md section 8, docs/backend-checklist.md
// B5.5, build-plan 5.8 and 5.9). None of them pushes anywhere and none of them forces a branch: a
// target branch only ever moves forward, and only after a merge has been made in a throwaway
// worktree and its tests have passed. The order the Integrator uses them in is:
//
//  1. DryRunMerge - test the merge without touching any files, and learn the conflicts.
//  2. BackupBranch - keep the target's tip on its own branch, so a mistaken move is recoverable.
//  3. AddMergeWorktree - a detached worktree at the target's tip.
//  4. MergeInto - make the merge commit there.
//  5. (the caller runs the tests)
//  6. FastForwardRef - move the target branch to the merge commit, forward only.
//  7. or AbortMerge + RemoveWorktree - leave the target exactly where it was.
//
// Each method follows the package's style: `(g *Git) Method(ctx, repo, ...) (..., error)`, with
// wrapped errors, and a sentinel a caller can match with errors.Is where there is a case to handle.

// ErrMergeConflict means a merge left conflicts that a person or the Integrator has to resolve.
var ErrMergeConflict = errors.New("the merge has conflicts")

// ErrNotFastForward means a target branch cannot move to a commit because doing so would drop
// commits it already has. Marshal never moves a branch backwards, so this is always a failure.
var ErrNotFastForward = errors.New("that move would not be a fast-forward")

// MergePreview is what a dry-run merge found: whether the merge is clean, the files the branch
// changes, and, when it is not clean, the files that conflict.
type MergePreview struct {
	// Clean is true when the merge would apply without conflicts.
	Clean bool
	// Changed is the files the branch changes relative to the merge base, in name order. It is what
	// the caller uses to run only the affected tests.
	Changed []string
	// Conflicts is the files that conflict, in the order Git named them. Empty when Clean is true.
	Conflicts []string
}

// DryRunMerge tests merging branch into into without touching a single file, and reports whether it
// is clean and what it changes. It is `git merge-tree --write-tree`, which computes the merge in
// memory.
//
// The changed files come from a plain `git diff --name-only <into>...<branch>` (three dots: the
// branch's changes since the merge base), because that is exactly the set of files a test runner
// cares about. The conflicting files come from merge-tree's own output.
//
// Nothing is written: no worktree, no index, no ref. A repository the caller does not want changed
// is safe to test.
func (g *Git) DryRunMerge(ctx context.Context, repo, into, branch string) (MergePreview, error) {
	if err := checkRevision(into); err != nil {
		return MergePreview{}, err
	}
	if err := checkRevision(branch); err != nil {
		return MergePreview{}, err
	}
	changed, err := g.changedFiles(ctx, repo, into, branch)
	if err != nil {
		return MergePreview{}, err
	}
	out, code, err := g.runOutput(ctx, repo, "merge-tree", "--write-tree", into, branch)
	if err != nil {
		return MergePreview{}, fmt.Errorf("dry-run merge of %s into %s: %w", branch, into, err)
	}
	switch code {
	case 0:
		return MergePreview{Clean: true, Changed: changed}, nil
	case 1:
		return MergePreview{Clean: false, Changed: changed, Conflicts: conflictPaths(out)}, nil
	default:
		return MergePreview{}, fmt.Errorf("dry-run merge of %s into %s: git merge-tree exited %d: %s",
			branch, into, code, strings.TrimSpace(out))
	}
}

// changedFiles lists the files a branch changes since the merge base with into, in name order. An
// empty answer is a branch with no changes of its own, which is not an error.
func (g *Git) changedFiles(ctx context.Context, repo, into, branch string) ([]string, error) {
	out, err := g.Run(ctx, repo, "diff", "--name-only", into+"..."+branch)
	if err != nil {
		return nil, fmt.Errorf("list the files %s changes: %w", branch, err)
	}
	return splitLines(out), nil
}

// conflictPaths reads the conflicting paths out of git merge-tree's output. Each conflict is a line
// naming the path, either as "CONFLICT (...): ... in <path>" or as a bare path in --name-only
// output. The path is the last token that looks like one, so both forms are read.
func conflictPaths(out string) []string {
	var paths []string
	for _, line := range splitLines(out) {
		line = strings.TrimSpace(line)
		if line == "" || isHexObjectName(line) {
			continue
		}
		if strings.Contains(line, "CONFLICT") || strings.Contains(line, "conflict") {
			if _, after, ok := strings.Cut(line, " in "); ok {
				paths = append(paths, strings.TrimSpace(after))
				continue
			}
			fields := strings.Fields(line)
			if len(fields) > 0 {
				paths = append(paths, fields[len(fields)-1])
			}
			continue
		}
		// A bare path line (the --name-only form).
		paths = append(paths, line)
	}
	return paths
}

// isHexObjectName reports whether a line is a bare object name, which merge-tree prints first and
// which is not a path.
func isHexObjectName(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// BackupBranch points a new branch at the tip of from, so a target branch that is about to move has
// a copy of where it was. A branch that already exists is ErrBranchExists, which keeps a backup of
// one moment from being overwritten by another.
func (g *Git) BackupBranch(ctx context.Context, repo, name, from string) error {
	if _, err := g.revParse(ctx, repo, from); err != nil {
		return err
	}
	return g.CreateBranch(ctx, repo, name, from)
}

// AddMergeWorktree adds a detached worktree at base, for the Integrator to merge in. It creates no
// branch: the temporary worktree is left detached on purpose, so the only branch that can move is
// the target, and only when the caller says so.
func (g *Git) AddMergeWorktree(ctx context.Context, repo, path, base string) error {
	if err := checkRevision(base); err != nil {
		return err
	}
	dir, err := checkAbsolute(path)
	if err != nil {
		return err
	}
	if relationOf(repo, dir) != unrelated {
		return newOpError(ErrBadPath, "a merge worktree cannot be inside the repository folder", nil)
	}
	if _, err := emptyOrMissing(dir); err != nil {
		if errors.Is(err, errFolderNotEmpty) {
			err = newOpError(ErrWorktreeExists, dir, nil)
		}
		return err
	}
	if _, err := g.Run(ctx, repo, "worktree", "add", "--detach", "--", dir, base); err != nil {
		return fmt.Errorf("add a merge worktree at %s: %w", dir, err)
	}
	return nil
}

// MergeInto merges branch into the worktree's current commit and returns the merge commit. The merge
// is always a real merge commit (--no-ff), so a card's work is visible as one merged branch.
//
// A conflict leaves the worktree mid-merge and answers ErrMergeConflict; the caller resolves it or
// aborts with AbortMerge. Marshal's own identity is used, and signing and hooks are off, so a
// machine with no Git identity still merges.
func (g *Git) MergeInto(ctx context.Context, worktree, branch, message string) (string, error) {
	if err := checkRevision(branch); err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		message = "Marshal: merge " + branch
	}
	args := []string{
		"-c", "user.name=Marshal", "-c", "user.email=marshal@localhost",
		"-c", "commit.gpgsign=false", "merge", "--no-ff", "--no-edit", "-m", message, "--", branch,
	}
	out, code, err := g.runOutput(ctx, worktree, args...)
	if err != nil {
		return "", fmt.Errorf("merge %s: %w", branch, err)
	}
	if code != 0 {
		if strings.Contains(out, "CONFLICT") || strings.Contains(out, "conflict") {
			return "", newOpError(ErrMergeConflict, branch, nil)
		}
		return "", fmt.Errorf("merge %s: git merge exited %d: %s", branch, code, strings.TrimSpace(out))
	}
	sha, err := g.revParse(ctx, worktree, "HEAD")
	if err != nil {
		return "", err
	}
	return sha, nil
}

// FastForwardRef moves a target branch to commit, forward only. It refuses when the target's tip is
// not an ancestor of commit (ErrNotFastForward), and it passes the old tip to Git's own
// compare-and-swap, so a target that moved since the caller read it is refused rather than
// overwritten.
//
// It is the only method here that moves a branch, and it can only move one forward.
func (g *Git) FastForwardRef(ctx context.Context, repo, target, commit string) error {
	if err := checkRevision(target); err != nil {
		return err
	}
	if err := checkRevision(commit); err != nil {
		return err
	}
	old, err := g.revParse(ctx, repo, target)
	if err != nil {
		return err
	}
	newTip, err := g.revParse(ctx, repo, commit)
	if err != nil {
		return err
	}
	ok, err := g.ask(ctx, repo, "merge-base", "--is-ancestor", old, newTip)
	if err != nil {
		return fmt.Errorf("check whether %s can move to %s: %w", target, commit, err)
	}
	if !ok {
		return newOpError(ErrNotFastForward, target+" is not behind "+commit, nil)
	}
	if _, err := g.Run(ctx, repo, "update-ref", "--create-reflog", headsPrefix+target, newTip, old); err != nil {
		return fmt.Errorf("move %s to %s: %w", target, newTip, err)
	}
	return nil
}

// AbortMerge gives up a merge that is in progress in a worktree, putting it back to the commit it
// started from. It is safe to call when no merge is in progress: it then does nothing.
func (g *Git) AbortMerge(ctx context.Context, worktree string) error {
	if _, err := g.Run(ctx, worktree, "merge", "--abort"); err != nil {
		// A worktree with no merge in progress has nothing to abort.
		if strings.Contains(err.Error(), "no merge to abort") || strings.Contains(err.Error(), "MERGE_HEAD") {
			return nil
		}
		return fmt.Errorf("abort the merge: %w", err)
	}
	return nil
}

// revParse resolves a revision to a full commit id.
func (g *Git) revParse(ctx context.Context, repo, rev string) (string, error) {
	sha, err := g.Run(ctx, repo, "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", rev, err)
	}
	return sha, nil
}

// runOutput runs a Git command and returns its combined output and its exit code, with a non-zero
// exit code reported as a code and not as an error. It exists for the merge queue, where exit codes
// 0 and 1 are both answers (`merge-tree` says "clean" or "conflicts" that way) and only any other
// code is a real failure.
func (g *Git) runOutput(ctx context.Context, dir string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, g.bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), g.env...)
	cmd.WaitDelay = killWaitDelay
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err == nil {
		return strings.TrimRight(buf.String(), "\r\n"), 0, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "", -1, errors.New("Git is not installed, or is not on the PATH")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return strings.TrimRight(buf.String(), "\r\n"), exitErr.ExitCode(), nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", -1, fmt.Errorf("%w: %w", ctxErr, err)
	}
	return "", -1, err
}

// splitLines splits command output into non-empty trimmed lines.
func splitLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}
