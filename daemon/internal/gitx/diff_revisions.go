package gitx

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// DiffFilesBetween compares two exact revisions, including private snapshots. It does not use
// their merge base and never adds files from the live folder to a saved card's result.
func (g *Git) DiffFilesBetween(ctx context.Context, dir, base, result string, maxBytes int) ([]DiffFile, bool, error) {
	for _, rev := range []string{base, result} {
		if err := checkRevision(rev); err != nil {
			return nil, false, err
		}
	}
	files, cut, err := g.diffNameStatusRevisions(ctx, dir, maxBytes, base, result, "--")
	if err != nil {
		return nil, false, err
	}
	counts, countsCut, err := g.diffCountsRevisions(ctx, dir, maxBytes, base, result, "--")
	if err != nil {
		return nil, false, err
	}
	for i := range files {
		count := counts[files[i].Path]
		files[i].Additions, files[i].Deletions, files[i].Binary = count.Additions, count.Deletions, count.Binary
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, cut || countsCut, nil
}

// HunkLimits bounds the source and lines returned by a saved file diff.
type HunkLimits struct {
	MaxLines int
	MaxBytes int
}

// DiffHunksBetween reads one path from two saved revisions. Its result is independent of later
// edits in the project folder, and pathspecs are literal just as in DiffHunks.
func (g *Git) DiffHunksBetween(ctx context.Context, dir, base, result, path string, limits HunkLimits) (DiffFileHunks, bool, error) {
	for _, rev := range []string{base, result} {
		if err := checkRevision(rev); err != nil {
			return DiffFileHunks{}, false, err
		}
	}
	if err := checkDiffPath(path); err != nil {
		return DiffFileHunks{}, false, err
	}
	out, cut, err := g.runCapped(ctx, dir, limits.MaxBytes,
		"--no-optional-locks", "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--unified=3", "-M",
		base, result, "--", ":(literal)"+path)
	if err != nil {
		return DiffFileHunks{}, false, fmt.Errorf("read the saved diff of %s: %w", path, err)
	}
	if strings.TrimSpace(out) == "" {
		return DiffFileHunks{}, false, nil
	}
	file := parseUnifiedDiff(out)
	file.Hunks, file.Truncated = cutHunks(file.Hunks, limits.MaxLines)
	file.Truncated = file.Truncated || cut
	return file, true, nil
}
