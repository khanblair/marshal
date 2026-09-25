package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RepoInfo is what Marshal needs to know about a repository before it manages it.
type RepoInfo struct {
	// Root is the top folder of the working tree, with links followed.
	Root string
	// CurrentBranch is the checked out branch. It is empty when HEAD is detached.
	CurrentBranch string
	// DefaultBranch is the branch new work starts from: the target of origin/HEAD, or main, or
	// master, or the current branch, in that order.
	DefaultBranch string
	// HasCommits is false for a new repository. A card cannot start on one.
	HasCommits bool
	// Clean is true when there are no changes to tracked files and no untracked files.
	Clean   bool
	Remotes []Remote
}

// Remote is a place a repository fetches from.
type Remote struct {
	Name string
	// URL has any user name and password removed, so it is safe to show and to store.
	URL string
}

const (
	originHead   = "refs/remotes/origin/HEAD"
	originPrefix = "origin/"
)

// Inspect reads a repository. The path must be the top folder of a working tree. A subfolder, a
// bare repository, and a folder that is not a repository are all ErrNotARepo.
func (g *Git) Inspect(ctx context.Context, path string) (RepoInfo, error) {
	root, err := g.workTreeRoot(ctx, path)
	if err != nil {
		return RepoInfo{}, err
	}
	info := RepoInfo{Root: root}
	if info.CurrentBranch, err = g.Run(ctx, root, "branch", "--show-current"); err != nil {
		return RepoInfo{}, fmt.Errorf("read the current branch: %w", err)
	}
	if info.HasCommits, err = g.ask(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err != nil {
		return RepoInfo{}, fmt.Errorf("check for commits: %w", err)
	}
	// Optional locks are off, so looking at a repository never blocks the user's own Git.
	status, err := g.Run(ctx, root, "--no-optional-locks", "status", "--porcelain")
	if err != nil {
		return RepoInfo{}, fmt.Errorf("read the status: %w", err)
	}
	info.Clean = status == ""
	if info.Remotes, err = g.remotes(ctx, root); err != nil {
		return RepoInfo{}, err
	}
	if info.DefaultBranch, err = g.defaultBranch(ctx, root, info.CurrentBranch); err != nil {
		return RepoInfo{}, err
	}
	return info, nil
}

// workTreeRoot returns the top folder of the working tree at path, and ErrNotARepo unless path is
// that folder. The file system compares the two folders, so a link, a trailing separator, or
// different case on a case-insensitive volume cannot make a real repository look wrong.
func (g *Git) workTreeRoot(ctx context.Context, path string) (string, error) {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return "", newOpError(ErrNotARepo, "the folder does not exist", err)
	}
	out, err := g.Run(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		if exitCode(err) != exitFatal {
			return "", fmt.Errorf("find the top folder of %s: %w", path, err)
		}
		var gitErr *Error
		if errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, "dubious ownership") {
			return "", newOpError(ErrNotARepo, "Git does not trust the owner of this folder", err)
		}
		return "", newOpError(ErrNotARepo, "", err)
	}
	root := filepath.Clean(filepath.FromSlash(out))
	if !sameFolderAs(root, path) {
		return "", newOpError(ErrNotARepo, "it is inside a repository, so choose the top folder, "+root, nil)
	}
	return root, nil
}

// defaultBranch picks the branch new work starts from.
func (g *Git) defaultBranch(ctx context.Context, root, current string) (string, error) {
	target, err := g.Run(ctx, root, "symbolic-ref", "--quiet", "--short", originHead)
	switch {
	case err == nil && strings.HasPrefix(target, originPrefix):
		return strings.TrimPrefix(target, originPrefix), nil
	case err != nil && exitCode(err) != exitFalse:
		return "", fmt.Errorf("read the default branch: %w", err)
	}
	for _, name := range []string{"main", "master"} {
		found, err := g.BranchExists(ctx, root, name)
		if err != nil {
			return "", err
		}
		if found {
			return name, nil
		}
	}
	return current, nil
}

const (
	remoteKeyPrefix = "remote."
	remoteURLSuffix = ".url"
)

// remotes lists the fetch addresses in the repository's configuration, with credentials removed.
func (g *Git) remotes(ctx context.Context, root string) ([]Remote, error) {
	out, err := g.Run(ctx, root, "config", "--null", "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		if exitCode(err) == exitFalse {
			return nil, nil // no remotes are configured
		}
		return nil, fmt.Errorf("read the remotes: %w", err)
	}
	var remotes []Remote
	seen := map[string]bool{}
	for _, entry := range strings.Split(out, "\x00") {
		key, value, ok := strings.Cut(entry, "\n")
		if !ok || !strings.HasPrefix(key, remoteKeyPrefix) || !strings.HasSuffix(key, remoteURLSuffix) {
			continue
		}
		// Names can hold dots, so the name is what is left after the fixed ends are cut off.
		name := strings.TrimSuffix(strings.TrimPrefix(key, remoteKeyPrefix), remoteURLSuffix)
		if seen[name] {
			continue
		}
		seen[name] = true
		remotes = append(remotes, Remote{Name: name, URL: StripCredentials(value)})
	}
	sort.Slice(remotes, func(i, j int) bool { return remotes[i].Name < remotes[j].Name })
	return remotes, nil
}
