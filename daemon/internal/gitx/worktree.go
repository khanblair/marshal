package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// undoTimeout bounds the clean up after a failed AddWorktree, which must still run when the
// caller's context is the reason it failed.
const undoTimeout = 30 * time.Second

// WorktreeSpec says where a worktree goes and what it starts from.
type WorktreeSpec struct {
	// Path is the new folder. It must be a full path, outside the repository folder, and either
	// missing or empty.
	Path string
	// Branch is the new branch to create and check out in the worktree.
	Branch string
	// Base is the commit or branch the new branch starts from.
	Base string
	// Sparse limits the worktree to these folders, given relative to the repository, plus the
	// files at its top. It is for monorepos. Leave it empty to check out everything.
	Sparse []string
}

// AddWorktree creates a branch from spec.Base and a worktree for it at spec.Path, in one step. If
// anything fails, the branch and the worktree are removed again, and the repository is as it was.
func (g *Git) AddWorktree(ctx context.Context, repo string, spec WorktreeSpec) error {
	dir, folders, err := g.checkWorktreeSpec(ctx, repo, spec)
	if err != nil {
		return err
	}
	spec.Path = dir
	// A card branch does not follow the branch it started from, even a remote one.
	args := []string{"worktree", "add", "--no-track", "-b", spec.Branch}
	if len(folders) > 0 {
		// Nothing is checked out yet, so the whole tree is never written and then trimmed.
		args = append(args, "--no-checkout")
	}
	args = append(args, "--", dir, spec.Base)
	if _, err := g.Run(ctx, repo, args...); err != nil {
		return g.undoAdd(ctx, repo, spec, fmt.Errorf("add a worktree for %s: %w", spec.Branch, err))
	}
	if len(folders) > 0 {
		if err := g.limitToFolders(ctx, dir, folders); err != nil {
			return g.undoAdd(ctx, repo, spec, err)
		}
	}
	return nil
}

// checkWorktreeSpec refuses a spec that would fail or do damage, before Git is asked to do
// anything. It returns the cleaned path and the cleaned sparse folders.
func (g *Git) checkWorktreeSpec(ctx context.Context, repo string, spec WorktreeSpec) (string, []string, error) {
	dir, err := checkAbsolute(spec.Path)
	if err != nil {
		return "", nil, err
	}
	root, err := g.workTreeRoot(ctx, repo)
	if err != nil {
		return "", nil, err
	}
	if relationOf(root, dir) != unrelated {
		return "", nil, newOpError(ErrBadPath, "a worktree cannot be inside the repository folder", nil)
	}
	if _, err := emptyOrMissing(dir); err != nil {
		if errors.Is(err, errFolderNotEmpty) {
			err = newOpError(ErrWorktreeExists, dir, nil)
		}
		return "", nil, err
	}
	// A worktree whose folder was deleted by hand is still registered, and Git refuses to add
	// another at the same place. Removing the old one is the caller's decision, not ours.
	if registered, err := g.isWorktree(ctx, repo, dir); err != nil {
		return "", nil, err
	} else if registered {
		return "", nil, newOpError(ErrWorktreeExists, dir+" is still registered, though its folder is gone", nil)
	}
	if err := g.ValidBranchName(ctx, spec.Branch); err != nil {
		return "", nil, err
	}
	if err := checkRevision(spec.Base); err != nil {
		return "", nil, err
	}
	taken, err := g.BranchExists(ctx, repo, spec.Branch)
	if err != nil {
		return "", nil, err
	}
	if taken {
		return "", nil, newOpError(ErrBranchExists, spec.Branch, nil)
	}
	folders, err := cleanSparseFolders(spec.Sparse)
	return dir, folders, err
}

// undoAdd puts a repository back as it was before a failed AddWorktree, and returns the cause
// with anything the clean up could not do. Git can leave the new branch behind, and a worktree
// that was added but not filled has to be taken out again.
func (g *Git) undoAdd(ctx context.Context, repo string, spec WorktreeSpec, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	errs := []error{cause}
	if found, err := g.isWorktree(ctx, repo, spec.Path); err != nil {
		errs = append(errs, err)
	} else if found {
		if _, err := g.Run(ctx, repo, "worktree", "remove", "--force", "--", spec.Path); err != nil {
			errs = append(errs, fmt.Errorf("remove the new worktree: %w", err))
		}
	}
	if err := g.PruneWorktrees(ctx, repo); err != nil {
		errs = append(errs, err)
	}
	// The branch did not exist when AddWorktree started, so any branch with this name is ours.
	if found, err := g.BranchExists(ctx, repo, spec.Branch); err != nil {
		errs = append(errs, err)
	} else if found {
		if err := g.DeleteBranch(ctx, repo, spec.Branch, true); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RemoveWorktree removes a worktree folder and makes Git forget it. It never removes the branch,
// because the branch can hold work that is not merged yet.
//
// The path must be inside root, the folder where Marshal keeps its worktrees, and not root
// itself, or the answer is ErrOutsideRoot. A worktree with changes that are not committed is
// ErrDirty unless force is true.
func (g *Git) RemoveWorktree(ctx context.Context, repo, path, root string, force bool) error {
	dir, err := checkInsideRoot(path, root)
	if err != nil {
		return err
	}
	registered, err := g.isWorktree(ctx, repo, dir)
	if err != nil {
		return err
	}
	_, statErr := os.Lstat(dir)
	present := statErr == nil
	switch {
	case registered && present:
		if err := g.removeRegistered(ctx, repo, dir, force); err != nil {
			return err
		}
	case present:
		if err := checkOrphan(dir, force); err != nil {
			return err
		}
	}
	if err := g.PruneWorktrees(ctx, repo); err != nil {
		return err
	}
	// Git can leave a folder behind, for example on Windows when a program still has a file open.
	// The folder is inside root, so it is Marshal's to remove.
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove the leftover folder %s: %w", dir, err)
	}
	return nil
}

// checkInsideRoot returns the cleaned path if it is strictly inside root.
func checkInsideRoot(path, root string) (string, error) {
	cleanRoot, err := checkAbsolute(root)
	if err != nil {
		return "", newOpError(ErrOutsideRoot, "the worktrees folder is not a full path", err)
	}
	dir, err := checkAbsolute(path)
	if err != nil {
		return "", newOpError(ErrOutsideRoot, path, err)
	}
	if relationOf(cleanRoot, dir) != insideFolder {
		return "", newOpError(ErrOutsideRoot, dir, nil)
	}
	return dir, nil
}

// removeRegistered asks Git to remove a worktree it knows, after checking for changes that
// would be lost.
func (g *Git) removeRegistered(ctx context.Context, repo, dir string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	} else {
		status, err := g.Run(ctx, dir, "--no-optional-locks", "status", "--porcelain")
		if err != nil {
			return fmt.Errorf("look for changes in %s: %w", dir, err)
		}
		if status != "" {
			return newOpError(ErrDirty, dir, nil)
		}
	}
	if _, err := g.Run(ctx, repo, append(args, "--", dir)...); err != nil {
		return fmt.Errorf("remove the worktree %s: %w", dir, err)
	}
	return nil
}

// checkOrphan looks at a folder that Git does not know as a worktree. It may hold work that was
// never committed, so a folder with anything in it needs force.
func checkOrphan(dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("look inside %s: %w", dir, err)
	}
	if len(entries) > 0 && !force {
		return newOpError(ErrDirty, dir+" is not a worktree Git knows, and it has files in it", nil)
	}
	return nil
}
