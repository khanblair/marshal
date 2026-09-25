package gitx

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// patternCharacters are the characters Git will not take in a folder name for cone mode, because
// they would be read as a pattern. Folders with these in their names cannot be checked out alone.
const patternCharacters = `*?[]\`

// cleanSparseFolders checks the folders a monorepo worktree is limited to, and returns them in
// the form Git wants: relative to the repository, with forward slashes, no duplicates.
func cleanSparseFolders(folders []string) ([]string, error) {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(folders))
	for _, folder := range folders {
		one, err := cleanSparseFolder(folder)
		if err != nil {
			return nil, err
		}
		if !seen[one] {
			seen[one] = true
			cleaned = append(cleaned, one)
		}
	}
	return cleaned, nil
}

func cleanSparseFolder(folder string) (string, error) {
	bad := func(why string) (string, error) {
		return "", newOpError(ErrBadPath, fmt.Sprintf("the folder %q %s", folder, why), nil)
	}
	slashed := filepath.ToSlash(folder)
	switch {
	case strings.ContainsFunc(slashed, unicode.IsControl):
		return bad("has a control character in it")
	case strings.ContainsAny(slashed, patternCharacters):
		return bad("has a character in its name that Git cannot check out alone")
	case strings.HasPrefix(slashed, "/") || filepath.IsAbs(folder) || strings.Contains(slashed, ":"):
		return bad("must be relative to the repository")
	case strings.HasPrefix(slashed, "-") || strings.HasPrefix(slashed, "!") || strings.HasPrefix(slashed, "#"):
		return bad("starts with a character that Git reads as something else")
	}
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return bad("must stay inside the repository")
		}
	}
	cleaned := path.Clean(slashed)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return bad("is not a folder inside the repository")
	}
	return cleaned, nil
}

// limitToFolders turns on cone-mode sparse checkout in a worktree that has nothing checked out,
// and then checks out only the listed folders and the files at the top of the repository. Nothing
// is checked out until the limit is in place, so the whole tree never lands on disk. If the limit
// cannot be set, nothing is checked out and an error comes back: the caller must not check out
// anything itself, because Git would then check out the whole tree.
func (g *Git) limitToFolders(ctx context.Context, worktree string, folders []string) error {
	// No "--" is needed: cleanSparseFolder refuses anything that starts with a dash.
	args := append([]string{"sparse-checkout", "set", "--cone"}, folders...)
	if _, err := g.Run(ctx, worktree, args...); err != nil {
		return fmt.Errorf("limit the worktree to %d folders: %w", len(folders), err)
	}
	on, err := g.Run(ctx, worktree, "config", "--get", "core.sparseCheckout")
	if err != nil {
		return fmt.Errorf("check that the worktree is limited to its folders: %w", err)
	}
	if on != "true" {
		return errors.New("the worktree is not limited to its folders")
	}
	// This is the step that fills the worktree, following the limit set above.
	if _, err := g.Run(ctx, worktree, "read-tree", "-m", "-u", "HEAD"); err != nil {
		return fmt.Errorf("check out the folders: %w", err)
	}
	return nil
}
