package gitx

import (
	"context"
	"fmt"
	"strings"
)

// headsPrefix is where Git keeps the tips of local branches.
const headsPrefix = "refs/heads/"

// ValidBranchName returns ErrBadBranchName unless Git accepts name as the name of a new branch.
// Names that only look valid are refused too: one that starts with a dash, or is @{-1} or another
// shorthand that Git would expand into the name of a different branch.
func (g *Git) ValidBranchName(ctx context.Context, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "refs/") || name == "@" ||
		strings.Contains(name, "@{") {
		return newOpError(ErrBadBranchName, name, nil)
	}
	out, err := g.Run(ctx, "", "check-ref-format", "--branch", name)
	if err != nil {
		if exitCode(err) > 0 {
			return newOpError(ErrBadBranchName, name, err)
		}
		return fmt.Errorf("check the branch name: %w", err)
	}
	if out != name {
		return newOpError(ErrBadBranchName, name, nil)
	}
	return nil
}

// checkRevision refuses a branch or commit name that Git could read as an option.
func checkRevision(name string) error {
	if name == "" || strings.HasPrefix(name, "-") {
		return newOpError(ErrBadBranchName, name, nil)
	}
	return nil
}

// BranchExists reports whether a local branch with this name exists.
func (g *Git) BranchExists(ctx context.Context, repo, name string) (bool, error) {
	found, err := g.ask(ctx, repo, "show-ref", "--verify", "--quiet", "--", headsPrefix+name)
	if err != nil {
		return false, fmt.Errorf("look for branch %s: %w", name, err)
	}
	return found, nil
}

// CreateBranch makes a branch that points at base, without switching to it. A branch that is
// already there is ErrBranchExists.
func (g *Git) CreateBranch(ctx context.Context, repo, name, base string) error {
	if err := g.ValidBranchName(ctx, name); err != nil {
		return err
	}
	if err := checkRevision(base); err != nil {
		return err
	}
	if _, err := g.Run(ctx, repo, "branch", "--no-track", "--", name, base); err != nil {
		if found, askErr := g.BranchExists(ctx, repo, name); askErr == nil && found {
			return newOpError(ErrBranchExists, name, err)
		}
		return fmt.Errorf("create branch %s: %w", name, err)
	}
	return nil
}

// DeleteBranch deletes a local branch. Without force, Git refuses a branch that has commits not
// merged anywhere else, so work is never lost by accident.
func (g *Git) DeleteBranch(ctx context.Context, repo, name string, force bool) error {
	if err := checkRevision(name); err != nil {
		return err
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	if _, err := g.Run(ctx, repo, "branch", flag, "--", name); err != nil {
		return fmt.Errorf("delete branch %s: %w", name, err)
	}
	return nil
}

// Branches lists the local branches whose names start with prefix, in name order. An empty prefix
// lists them all.
func (g *Git) Branches(ctx context.Context, repo, prefix string) ([]string, error) {
	// The prefix is matched here, not by Git, because Git's own pattern matching stops at slashes.
	out, err := g.Run(ctx, repo, "for-each-ref", "--format=%(refname)", headsPrefix)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimPrefix(line, headsPrefix)
		if line != "" && strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	return names, nil
}

// IsMerged reports whether every commit on branch is already in into. Squash merges are not
// noticed. Marshal fast-forwards, so its own branches are found this way.
func (g *Git) IsMerged(ctx context.Context, repo, branch, into string) (bool, error) {
	if err := checkRevision(branch); err != nil {
		return false, err
	}
	if err := checkRevision(into); err != nil {
		return false, err
	}
	merged, err := g.ask(ctx, repo, "merge-base", "--is-ancestor", branch, into)
	if err != nil {
		return false, fmt.Errorf("check whether %s is merged into %s: %w", branch, into, err)
	}
	return merged, nil
}
