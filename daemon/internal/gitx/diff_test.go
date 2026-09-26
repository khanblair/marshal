package gitx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

// The diff a card's screen draws (docs/backend-checklist.md B2.9, N15). Every test here works on a
// real repository and a real worktree, because the parsing, the merge base, and the statuses are
// Git's own answers and a mock would only test the mock.

// diffRepo is a repository with a worktree checked out on its own branch, which is what a started
// card has.
type diffRepo struct {
	repo     string
	worktree string
	branch   string
}

// newDiffRepo copies a fixture into a temp folder and makes a worktree for a card branch in it, the
// way internal/session does when a card starts.
func newDiffRepo(t *testing.T) diffRepo {
	t.Helper()
	repo := fixture(t, "small-repo")
	worktree := filepath.Join(t.TempDir(), "worktree")
	branch := gitx.CardBranchPrefix + "small-repo-41-change-the-thing"
	spec := gitx.WorktreeSpec{Path: worktree, Branch: branch, Base: "main"}
	if err := testGit().AddWorktree(context.Background(), repo, spec); err != nil {
		t.Fatalf("make a worktree: %v", err)
	}
	return diffRepo{repo: repo, worktree: worktree, branch: branch}
}

// write puts a file in the worktree, making its folders, and returns nothing: it is how a test acts
// the part of an agent that edits files.
func (r diffRepo) write(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join(r.worktree, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// remove deletes a file from the worktree.
func (r diffRepo) remove(t *testing.T, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(r.worktree, filepath.FromSlash(name))); err != nil {
		t.Fatal(err)
	}
}

// change acts out what an agent does to a card's worktree: an edit, a deletion, a rename, a new
// file, and a new binary file. The ignored file is there to prove it is left out.
func (r diffRepo) change(t *testing.T) {
	t.Helper()
	r.write(t, "src/util.js", "/** Adds two numbers. */\nexport function add(a, b) {\n  return a + b + 1;\n}\n")
	r.remove(t, "test/util.test.js")
	git(t, r.worktree, "mv", "src/index.js", "src/main.js")
	r.write(t, "src/new.js", "export const one = 1;\nexport const two = 2;\n")
	r.write(t, "logo.bin", "\x00\x01\x02binary")
	r.write(t, "node_modules/left-out.js", "// ignored\n")
}

// find returns the file with this path, and fails the test when the diff does not hold it.
func find(t *testing.T, files []gitx.DiffFile, path string) gitx.DiffFile {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("the diff does not hold %s: %+v", path, files)
	return gitx.DiffFile{}
}

func TestDiffFilesListsWhatAWorktreeChanged(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	repo.change(t)

	files, truncated, err := testGit().DiffFiles(ctx, repo.worktree, "main", 0)
	if err != nil {
		t.Fatalf("read the diff: %v", err)
	}
	if truncated {
		t.Error("a small diff says it was cut short")
	}
	want := []string{"logo.bin", "src/main.js", "src/new.js", "src/util.js", "test/util.test.js"}
	got := make([]string, 0, len(files))
	for _, file := range files {
		got = append(got, file.Path)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the files = %v, want %v (in path order)", got, want)
	}

	edited := find(t, files, "src/util.js")
	if edited.Status != gitx.DiffStatusModified || edited.Additions != 1 || edited.Deletions != 1 {
		t.Errorf("the edited file = %+v, want a modification of one line each way", edited)
	}
	removed := find(t, files, "test/util.test.js")
	if removed.Status != gitx.DiffStatusDeleted || removed.Additions != 0 || removed.Deletions == 0 {
		t.Errorf("the removed file = %+v, want a deletion with lines lost", removed)
	}
	moved := find(t, files, "src/main.js")
	if moved.Status != gitx.DiffStatusRenamed || moved.OldPath != "src/index.js" {
		t.Errorf("the moved file = %+v, want a rename from src/index.js", moved)
	}
	added := find(t, files, "src/new.js")
	if added.Status != gitx.DiffStatusAdded || added.Additions != 2 || added.Deletions != 0 {
		t.Errorf("the new file = %+v, want an addition of two lines", added)
	}
	binary := find(t, files, "logo.bin")
	if binary.Status != gitx.DiffStatusAdded || !binary.Binary || binary.Additions != 0 {
		t.Errorf("the binary file = %+v, want an addition that is not text", binary)
	}
	for _, file := range files {
		if strings.Contains(file.Path, "node_modules") {
			t.Errorf("%s is ignored by the repository but is in the diff", file.Path)
		}
	}
}

// The comparison starts at the merge base, so work that landed on the base branch after the card
// started is not the card's change.
func TestDiffFilesIgnoreWhatLandedOnTheBaseBranchAfterwards(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	repo.write(t, "src/util.js", "// the card's own change\n")
	// The base branch moves on, the way a person's main does while a card is open.
	commitFile(t, repo.repo, "src/other.js", "// landed on main\n")

	files, _, err := testGit().DiffFiles(ctx, repo.worktree, "main", 0)
	if err != nil {
		t.Fatalf("read the diff: %v", err)
	}
	if len(files) != 1 || files[0].Path != "src/util.js" {
		t.Errorf("the diff = %+v, want only the card's own file", files)
	}
}

func TestDiffHunksReadOneFile(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	repo.write(t, "src/util.js", "/** Adds two numbers. */\nexport function add(a, b) {\n  return a + b + 1;\n}\n")

	hunks, found, err := testGit().DiffHunks(ctx, repo.worktree, "main", "src/util.js", 0, 0)
	if err != nil || !found {
		t.Fatalf("read the hunks: found=%v err=%v", found, err)
	}
	if hunks.Status != gitx.DiffStatusModified {
		t.Errorf("status = %q, want modified", hunks.Status)
	}
	if len(hunks.Hunks) != 1 {
		t.Fatalf("the file has %d hunks, want 1: %+v", len(hunks.Hunks), hunks.Hunks)
	}
	hunk := hunks.Hunks[0]
	if !strings.HasPrefix(hunk.Header, "@@ -1,4 +1,4 @@") {
		t.Errorf("header = %q, want the changed file's own range", hunk.Header)
	}
	// The third line of the four-line file changed, so the hunk holds two context lines above it,
	// the removed line, the added line, and the closing brace. Each line carries its own side's
	// number, and the side it is not on is zero.
	wantKinds := []gitx.DiffLineKind{
		gitx.DiffLineContext, gitx.DiffLineContext, gitx.DiffLineRemoved, gitx.DiffLineAdded, gitx.DiffLineContext,
	}
	gotKinds := make([]gitx.DiffLineKind, 0, len(hunk.Lines))
	for _, line := range hunk.Lines {
		gotKinds = append(gotKinds, line.Kind)
	}
	if strings.Join(toStrings(gotKinds), ",") != strings.Join(toStrings(wantKinds), ",") {
		t.Fatalf("the lines = %v, want %v", gotKinds, wantKinds)
	}
	if hunk.Lines[2].OldLine != 3 || hunk.Lines[2].NewLine != 0 {
		t.Errorf("the removed line = %+v, want old line 3 and no new line", hunk.Lines[2])
	}
	if hunk.Lines[3].OldLine != 0 || hunk.Lines[3].NewLine != 3 {
		t.Errorf("the added line = %+v, want new line 3 and no old line", hunk.Lines[3])
	}
	if hunk.Lines[3].Text != "  return a + b + 1;" {
		t.Errorf("the added line's text = %q", hunk.Lines[3].Text)
	}
	if hunk.Lines[4].OldLine != 4 || hunk.Lines[4].NewLine != 4 {
		t.Errorf("the last context line = %+v, want line 4 on both sides", hunk.Lines[4])
	}
}

// A new file has no side in the base revision, so Git says nothing about it and the hunks come
// from the file itself, as an addition.
func TestDiffHunksOfANewFile(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	repo.write(t, "src/new.js", "export const one = 1;\nexport const two = 2;\n")

	hunks, found, err := testGit().DiffHunks(ctx, repo.worktree, "main", "src/new.js", 0, 0)
	if err != nil || !found {
		t.Fatalf("read the hunks: found=%v err=%v", found, err)
	}
	if hunks.Status != gitx.DiffStatusAdded || len(hunks.Hunks) != 1 {
		t.Fatalf("the new file's hunks = %+v, want one hunk of an addition", hunks)
	}
	if got := hunks.Hunks[0].Lines; len(got) != 2 || got[0].Text != "export const one = 1;" || got[1].NewLine != 2 {
		t.Errorf("the new file's lines = %+v", got)
	}
	if hunks.Hunks[0].Header != "@@ -0,0 +1,2 @@" {
		t.Errorf("header = %q", hunks.Hunks[0].Header)
	}
}

// A path that changed does not include a file that did not, and the caller is told so: that is what
// makes the hunks route answer not found for a path that names nothing.
func TestDiffHunksOfAFileThatDidNotChange(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)

	hunks, found, err := testGit().DiffHunks(ctx, repo.worktree, "main", "src/util.js", 0, 0)
	if err != nil {
		t.Fatalf("read the hunks: %v", err)
	}
	if found {
		t.Errorf("an unchanged file reads as found: %+v", hunks)
	}
}

func TestDiffHunksOfADeletedFile(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	repo.remove(t, "test/util.test.js")

	hunks, found, err := testGit().DiffHunks(ctx, repo.worktree, "main", "test/util.test.js", 0, 0)
	if err != nil || !found {
		t.Fatalf("read the hunks: found=%v err=%v", found, err)
	}
	if hunks.Status != gitx.DiffStatusDeleted {
		t.Errorf("status = %q, want deleted", hunks.Status)
	}
	for _, hunk := range hunks.Hunks {
		for _, line := range hunk.Lines {
			if line.Kind == gitx.DiffLineAdded {
				t.Errorf("a deleted file gained a line: %+v", line)
			}
		}
	}
}

// The caps are what keeps a huge file from stalling the screen: the answer is cut and says so.
func TestDiffHunksAreCappedInLinesAndBytes(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	var added strings.Builder
	for i := 0; i < 50; i++ {
		added.WriteString("// a line the card added\n")
	}
	repo.write(t, "src/util.js", added.String())

	hunks, found, err := testGit().DiffHunks(ctx, repo.worktree, "main", "src/util.js", 5, 0)
	if err != nil || !found {
		t.Fatalf("read the hunks: found=%v err=%v", found, err)
	}
	if !hunks.Truncated {
		t.Error("the hunks were cut at five lines and do not say so")
	}
	if lines := countHunkLines(hunks.Hunks); lines != 5 {
		t.Errorf("the hunks carry %d lines, want 5", lines)
	}

	small, found, err := testGit().DiffHunks(ctx, repo.worktree, "main", "src/util.js", 0, 64)
	if err != nil || !found {
		t.Fatalf("read the hunks with a byte cap: found=%v err=%v", found, err)
	}
	if !small.Truncated {
		t.Error("the hunks were cut at 64 bytes and do not say so")
	}
}

// A byte cap cuts the file list too, and the caller is told rather than handed half a record.
func TestDiffFilesAreCappedInBytes(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	repo.change(t)

	files, truncated, err := testGit().DiffFiles(ctx, repo.worktree, "main", 40)
	if err != nil {
		t.Fatalf("read the diff: %v", err)
	}
	if !truncated {
		t.Error("a 40 byte cap read the whole diff and does not say it was cut")
	}
	if len(files) == 0 {
		t.Error("the answer holds nothing at all")
	}
	for _, file := range files {
		if file.Path == "" {
			t.Errorf("a record cut in half reached the caller: %+v", file)
		}
	}
}

func TestDiffRefusesWhatItCannotRead(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	tests := []struct {
		name  string
		base  string
		path  string
		files bool
	}{
		{"a base that looks like an option", "-x", "", true},
		{"a path that walks upwards", "main", "../secret", false},
		{"a path that is absolute", "main", "/etc/passwd", false},
		{"a path that is empty", "main", "", false},
		{"a path with a dot segment", "main", "src/./util.js", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.files {
				if _, _, err := testGit().DiffFiles(ctx, repo.worktree, tc.base, 0); !errors.Is(err, gitx.ErrBadBranchName) {
					t.Errorf("DiffFiles error = %v, want ErrBadBranchName", err)
				}
				return
			}
			if _, _, err := testGit().DiffHunks(ctx, repo.worktree, tc.base, tc.path, 0, 0); !errors.Is(err, gitx.ErrBadPath) {
				t.Errorf("DiffHunks error = %v, want ErrBadPath", err)
			}
		})
	}
}

// A file whose name looks like a pathspec pattern is read as the name it is.
func TestDiffOfAFileWhoseNameLooksLikeAPattern(t *testing.T) {
	ctx := context.Background()
	repo := newDiffRepo(t)
	repo.write(t, "src/*.js", "export const star = true;\n")

	hunks, found, err := testGit().DiffHunks(ctx, repo.worktree, "main", "src/*.js", 0, 0)
	if err != nil || !found {
		t.Fatalf("read the hunks of a file named like a pattern: found=%v err=%v", found, err)
	}
	if hunks.Status != gitx.DiffStatusAdded || len(hunks.Hunks) != 1 {
		t.Errorf("the file named like a pattern = %+v", hunks)
	}
}

// countHunkLines counts every line of a file's hunks.
func countHunkLines(hunks []gitx.DiffHunk) int {
	lines := 0
	for _, hunk := range hunks {
		lines += len(hunk.Lines)
	}
	return lines
}

// toStrings turns a list of line kinds into plain strings, for a readable failure.
func toStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}
