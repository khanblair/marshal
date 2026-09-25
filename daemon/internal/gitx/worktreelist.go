package gitx

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Worktree is one working folder of a repository. The first one listed is the main folder.
type Worktree struct {
	Path string
	// Head is the commit that is checked out.
	Head string
	// Branch is the checked out branch, and is empty when HEAD is detached or the repository is bare.
	Branch   string
	Bare     bool
	Detached bool
	Locked   bool
	// Prunable is true when the folder is gone and Git would forget the worktree on a prune.
	Prunable bool
}

// ListWorktrees lists the working folders of a repository, the main one first.
func (g *Git) ListWorktrees(ctx context.Context, repo string) ([]Worktree, error) {
	out, err := g.Run(ctx, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	return parseWorktrees(out), nil
}

// parseWorktrees reads `git worktree list --porcelain -z`: every line of a worktree ends with a
// NUL character, and a NUL on its own ends the worktree.
func parseWorktrees(out string) []Worktree {
	var list []Worktree
	for _, field := range strings.Split(out, "\x00") {
		key, value, _ := strings.Cut(field, " ")
		if key == "worktree" {
			list = append(list, Worktree{Path: filepath.Clean(filepath.FromSlash(value))})
			continue
		}
		if key == "" || len(list) == 0 {
			continue
		}
		wt := &list[len(list)-1]
		switch key {
		case "HEAD":
			wt.Head = value
		case "branch":
			wt.Branch = strings.TrimPrefix(value, headsPrefix)
		case "bare":
			wt.Bare = true
		case "detached":
			wt.Detached = true
		case "locked":
			wt.Locked = true
		case "prunable":
			wt.Prunable = true
		}
	}
	return list
}

// isWorktree reports whether Git knows a worktree at a folder. The file system decides what "the
// same folder" means, so a link or a different spelling of the path still finds it.
func (g *Git) isWorktree(ctx context.Context, repo, dir string) (bool, error) {
	list, err := g.ListWorktrees(ctx, repo)
	if err != nil {
		return false, err
	}
	for _, wt := range list {
		if sameFolderAs(wt.Path, dir) {
			return true, nil
		}
	}
	return false, nil
}

// PruneWorktrees makes Git forget worktrees whose folders are gone.
func (g *Git) PruneWorktrees(ctx context.Context, repo string) error {
	if _, err := g.Run(ctx, repo, "worktree", "prune"); err != nil {
		return fmt.Errorf("prune worktrees: %w", err)
	}
	return nil
}
