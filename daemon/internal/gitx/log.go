package gitx

import (
	"context"
	"fmt"
	"strings"
)

// Reading the commits an agent made (docs/backend-checklist.md B3.5). A card's worktree is on its
// own branch, cut from the project's default branch, so the commits on that branch and not on the
// default one are exactly the commits the agent made.

// unitSeparator is the character the log format uses between the fields of one commit. It cannot
// appear in a SHA or a subject line, so the two never run together.
const unitSeparator = "\x1f"

// Commit is one commit read from a worktree's branch.
type Commit struct {
	// SHA is the commit's full hash.
	SHA string
	// Subject is the commit's first line, for a person to read.
	Subject string
}

// CommitsOnBranch returns the commits that are on the branch checked out at dir and not reachable
// from base, oldest first. It is what "the commits the agent made" means: the worktree was cut from
// base, so everything after it is the agent's.
//
// An empty base is refused rather than read as "every commit": without a base there is no way to
// tell the agent's work from the project's history, and scanning all of it every turn is neither
// cheap nor what the caller asked for. A branch with no new commits is an empty list, not an error.
func (g *Git) CommitsOnBranch(ctx context.Context, dir, base string) ([]Commit, error) {
	if strings.TrimSpace(base) == "" {
		return nil, nil
	}
	out, err := g.Run(ctx, dir, "log", "--reverse", "--no-color",
		"--format=%H"+unitSeparator+"%s", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	return parseCommits(out), nil
}

// parseCommits reads the lines CommitsOnBranch asked for.
func parseCommits(out string) []Commit {
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		sha, subject, found := strings.Cut(line, unitSeparator)
		if !found || sha == "" {
			continue
		}
		commits = append(commits, Commit{SHA: sha, Subject: subject})
	}
	return commits
}

// ChangedPaths returns the files a commit changed that still exist in it. A file the commit deleted
// is left out, because there is nothing at the commit to scan; a file it renamed is the new name.
//
// The paths are relative to the repository root, which is how Git names them everywhere.
func (g *Git) ChangedPaths(ctx context.Context, dir, sha string) ([]string, error) {
	out, err := g.Run(ctx, dir, "diff-tree", "--no-commit-id", "--name-only", "-r",
		"--diff-filter=d", "--root", sha)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// FileAtCommit returns the contents of one path as of one commit. A path the commit does not hold
// is an error, because the caller passes the commit's own changed paths, which do exist in it.
func (g *Git) FileAtCommit(ctx context.Context, dir, sha, path string) (string, error) {
	if !safePath(path) {
		return "", fmt.Errorf("refuse to read the path %q at a commit", path)
	}
	return g.Run(ctx, dir, "show", sha+":"+path)
}

// safePath refuses a path that could be read as something other than a path. The argument Git is
// handed is "<sha>:<path>", which begins with the SHA and so can never be read as an option; this
// only keeps a NUL, a newline, or an empty name from reaching the command line.
func safePath(path string) bool {
	return path != "" && !strings.HasPrefix(path, "-") && !strings.ContainsAny(path, "\x00\n\r")
}
