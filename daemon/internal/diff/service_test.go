package diff_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/diff"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The card diff service (docs/backend-checklist.md B2.9, N15). Every test but the two that check a
// refusal works on a real repository and a real worktree, because the diff is Git's own answer and
// a mock would only test the mock.

// testTime is the clock every answer is stamped with.
var testTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// notFoundCard is the answer the projects module gives for a card that is not there.
func notFoundCard() *protocol.Error {
	return protocol.NotFound("card").With("id", "01M3C107JB041061050R3GG28A")
}

// projects is a stand-in for the projects module: it answers for one card, its project, and its
// worktree, and each answer can be made to fail.
type projects struct {
	card        protocol.Card
	project     protocol.Project
	path        string
	branch      string
	cardErr     error
	projectErr  error
	worktreeErr error
}

func (p *projects) Card(context.Context, string) (protocol.Card, error) {
	if p.cardErr != nil {
		return protocol.Card{}, p.cardErr
	}
	return p.card, nil
}

func (p *projects) Get(context.Context, string) (protocol.Project, error) {
	if p.projectErr != nil {
		return protocol.Project{}, p.projectErr
	}
	return p.project, nil
}

func (p *projects) Worktree(context.Context, string) (string, string, error) {
	if p.worktreeErr != nil {
		return "", "", p.worktreeErr
	}
	return p.path, p.branch, nil
}

// env is the service over a real repository and a worktree of it.
type env struct {
	service  *diff.Service
	projects *projects
	worktree string
}

// newEnv copies a fixture repository, makes a worktree for a card branch, and builds the service
// over both, with the stand-in projects module wired to the same paths.
func newEnv(t *testing.T, opts ...diff.Option) *env {
	t.Helper()
	git := testutil.Git()
	repo := testutil.Fixture(t, "small-repo")
	worktree := filepath.Join(t.TempDir(), "worktree")
	branch := gitx.CardBranchPrefix + "small-repo-41-change-the-thing"
	spec := gitx.WorktreeSpec{Path: worktree, Branch: branch, Base: "main"}
	if err := git.AddWorktree(context.Background(), repo, spec); err != nil {
		t.Fatalf("make a worktree: %v", err)
	}
	stand := &projects{
		card:    protocol.Card{ID: "01M3C107JB041061050R3GG28A", ProjectID: "small-repo", Number: 41},
		project: protocol.Project{ID: "small-repo", Name: "small-repo", DefaultBranch: "main"},
		path:    worktree,
		branch:  branch,
	}
	options := append([]diff.Option{diff.WithClock(func() time.Time { return testTime })}, opts...)
	service, err := diff.New(diff.Deps{Projects: stand, Git: git}, options...)
	if err != nil {
		t.Fatalf("make the service: %v", err)
	}
	return &env{service: service, projects: stand, worktree: worktree}
}

// write puts a file in the worktree, making its folders, the way an agent edits a card's worktree.
func (e *env) write(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join(e.worktree, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// remove deletes a file from the worktree.
func (e *env) remove(t *testing.T, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(e.worktree, filepath.FromSlash(name))); err != nil {
		t.Fatal(err)
	}
}

// find returns one file of the answer, and fails the test when the diff does not hold it.
func find(t *testing.T, answer protocol.CardDiff, path string) protocol.ChangedFile {
	t.Helper()
	for _, file := range answer.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("the diff does not hold %s: %+v", path, answer.Files)
	return protocol.ChangedFile{}
}

// wantNotFoundCard fails the test unless err is the not found answer for a card.
func wantNotFoundCard(t *testing.T, err error) {
	t.Helper()
	var apiErr *protocol.Error
	if !errors.As(err, &apiErr) || apiErr.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("error = %v, want not found", err)
	}
	if apiErr.Details["id"] == "" {
		t.Errorf("the answer does not name the card: %+v", apiErr.Details)
	}
}

// wantNotFound fails the test unless err is the not found answer for this path.
func wantNotFound(t *testing.T, err error, path string) {
	t.Helper()
	var apiErr *protocol.Error
	if !errors.As(err, &apiErr) || apiErr.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("error = %v, want not found", err)
	}
	if apiErr.Details["path"] != path {
		t.Errorf("the answer names %q, want %q", apiErr.Details["path"], path)
	}
}

func TestFilesReadARealWorktree(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.write(t, "src/util.js", "/** Adds two numbers. */\nexport function add(a, b) {\n  return a + b + 1;\n}\n")
	e.remove(t, "test/util.test.js")
	e.write(t, "src/new.js", "export const one = 1;\nexport const two = 2;\n")

	answer, err := e.service.Files(ctx, e.projects.card.ID)
	if err != nil {
		t.Fatalf("read the diff: %v", err)
	}
	if answer.CardID != e.projects.card.ID || answer.Base != "main" || answer.Branch != e.projects.branch {
		t.Errorf("the answer = %+v, want the card, its base branch, and its branch", answer)
	}
	if answer.ServerTime.Time() != testTime {
		t.Errorf("serverTime = %v, want the service's clock", answer.ServerTime)
	}
	if answer.Truncated || answer.FileCount != 3 {
		t.Errorf("the answer = %+v, want three whole files", answer)
	}
	if answer.Additions != 3 || answer.Deletions != 8 {
		t.Errorf("the totals = +%d -%d, want +3 -8", answer.Additions, answer.Deletions)
	}
	edited := find(t, answer, "src/util.js")
	if edited.Status != protocol.DiffFileStatusModified || edited.Additions != 1 || edited.Deletions != 1 {
		t.Errorf("the edited file = %+v", edited)
	}
	removed := find(t, answer, "test/util.test.js")
	if removed.Status != protocol.DiffFileStatusDeleted || removed.Deletions == 0 {
		t.Errorf("the removed file = %+v", removed)
	}
	added := find(t, answer, "src/new.js")
	if added.Status != protocol.DiffFileStatusAdded || added.Additions != 2 || added.OldPath != "" {
		t.Errorf("the new file = %+v", added)
	}
	for _, file := range answer.Files {
		if file.Large {
			t.Errorf("%s is called large, but the whole diff is three lines: %+v", file.Path, file)
		}
	}
}

// A diff that is cut says so, and its totals still describe the whole diff.
func TestFilesCapTheListAndSaySo(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, diff.WithLimits(diff.Limits{MaxFiles: 2}))
	e.write(t, "src/util.js", "// changed\n")
	e.write(t, "src/new.js", "export const one = 1;\n")
	e.write(t, "src/another.js", "export const another = 1;\n")

	answer, err := e.service.Files(ctx, e.projects.card.ID)
	if err != nil {
		t.Fatalf("read the diff: %v", err)
	}
	if len(answer.Files) != 2 || !answer.Truncated {
		t.Fatalf("the answer = %+v, want two files and Truncated", answer.Files)
	}
	if answer.FileCount != 3 {
		t.Errorf("fileCount = %d, want the whole diff's 3 files", answer.FileCount)
	}
	if answer.Additions != 3 || answer.Deletions != 4 {
		t.Errorf("the totals = +%d -%d, want the whole diff's +3 -4", answer.Additions, answer.Deletions)
	}
}

// A file with more changed lines than the limit is marked large, which is what keeps the screen
// from drawing thousands of lines the moment the tab opens.
func TestFilesMarkALargeFile(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, diff.WithLimits(diff.Limits{LargeLines: 5}))
	e.write(t, "src/util.js", "// one\n// two\n// three\n// four\n// five\n// six\n")
	e.write(t, "src/small.js", "// one\n")

	answer, err := e.service.Files(ctx, e.projects.card.ID)
	if err != nil {
		t.Fatalf("read the diff: %v", err)
	}
	if !find(t, answer, "src/util.js").Large {
		t.Errorf("a file with six changed lines is not called large: %+v", find(t, answer, "src/util.js"))
	}
	if find(t, answer, "src/small.js").Large {
		t.Error("a file with one changed line is called large")
	}
}

// A card that never started has no worktree. That is an empty diff, never an error: the Diff tab
// draws its empty state for it.
func TestFilesOfACardThatNeverStarted(t *testing.T) {
	e := newEnv(t)
	e.projects.path, e.projects.branch = "", ""

	answer, err := e.service.Files(context.Background(), e.projects.card.ID)
	if err != nil {
		t.Fatalf("a card that never started is an error: %v", err)
	}
	if answer.Files == nil || len(answer.Files) != 0 || answer.FileCount != 0 || answer.Truncated {
		t.Errorf("the answer = %+v, want an empty list and not null", answer)
	}
	if answer.Base != "main" || answer.Branch != "" {
		t.Errorf("the answer = %+v, want the base branch and no branch", answer)
	}
}

// A worktree folder that was deleted by hand leaves a card with a path that names nothing. That is
// an empty diff too, so the tab still draws instead of answering an error.
func TestFilesOfACardWhoseWorktreeIsGone(t *testing.T) {
	e := newEnv(t)
	e.write(t, "src/util.js", "// changed\n")
	if err := os.RemoveAll(e.worktree); err != nil {
		t.Fatal(err)
	}

	answer, err := e.service.Files(context.Background(), e.projects.card.ID)
	if err != nil {
		t.Fatalf("a worktree that is gone is an error: %v", err)
	}
	if len(answer.Files) != 0 {
		t.Errorf("the answer = %+v, want an empty diff", answer.Files)
	}
}

// A card whose project has no default branch has nothing to compare against, which is an empty
// diff rather than a Git failure.
func TestFilesOfACardWithNoBaseBranch(t *testing.T) {
	e := newEnv(t)
	e.projects.project.DefaultBranch = ""
	e.write(t, "src/util.js", "// changed\n")

	answer, err := e.service.Files(context.Background(), e.projects.card.ID)
	if err != nil || len(answer.Files) != 0 {
		t.Fatalf("answer = %+v, err = %v; want an empty diff", answer, err)
	}
}

func TestFilesPassOnWhatTheModulesSay(t *testing.T) {
	ctx := context.Background()
	want := errors.New("the database is gone")
	tests := []struct {
		name string
		set  func(*projects)
	}{
		{"an unknown card", func(p *projects) { p.cardErr = notFoundCard() }},
		{"a project that cannot be read", func(p *projects) { p.projectErr = want }},
		{"a worktree that cannot be read", func(p *projects) { p.worktreeErr = want }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			tc.set(e.projects)
			_, err := e.service.Files(ctx, e.projects.card.ID)
			if err == nil {
				t.Fatal("the answer was made even though the modules failed")
			}
			if tc.name == "an unknown card" {
				wantNotFoundCard(t, err)
				return
			}
			if !errors.Is(err, want) {
				t.Errorf("error = %v, want it to wrap %v", err, want)
			}
		})
	}
}

// A base branch that does not exist is a real failure, not an empty diff: the daemon cannot answer
// honestly, so it says so instead of drawing a card with no changes.
func TestFilesReportAGitFailure(t *testing.T) {
	e := newEnv(t)
	e.projects.project.DefaultBranch = "no-such-branch"
	e.write(t, "src/util.js", "// changed\n")

	if _, err := e.service.Files(context.Background(), e.projects.card.ID); err == nil {
		t.Fatal("a diff against a branch that does not exist answered without an error")
	}
}

func TestHunksOfAChangedFile(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.write(t, "src/util.js", "/** Adds two numbers. */\nexport function add(a, b) {\n  return a + b + 1;\n}\n")

	answer, err := e.service.Hunks(ctx, e.projects.card.ID, "src/util.js")
	if err != nil {
		t.Fatalf("read the hunks: %v", err)
	}
	if answer.Path != "src/util.js" || answer.Status != protocol.DiffFileStatusModified {
		t.Errorf("the answer = %+v", answer)
	}
	if answer.Truncated || len(answer.Hunks) != 1 {
		t.Fatalf("the answer = %+v, want one whole hunk", answer)
	}
	if answer.ServerTime.Time() != testTime {
		t.Errorf("serverTime = %v, want the service's clock", answer.ServerTime)
	}
	lines := answer.Hunks[0].Lines
	if len(lines) != 5 || lines[2].Kind != protocol.DiffLineKindRemoved || lines[3].Kind != protocol.DiffLineKindAdded {
		t.Fatalf("the lines = %+v", lines)
	}
	if lines[2].NewLine != 0 || lines[3].OldLine != 0 {
		t.Errorf("a line carries a number on a side it is not on: %+v %+v", lines[2], lines[3])
	}
}

// A file the agent created is not in the base revision, so its hunks are every line it holds.
func TestHunksOfANewFile(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.write(t, "src/new.js", "export const one = 1;\nexport const two = 2;\n")

	answer, err := e.service.Hunks(ctx, e.projects.card.ID, "src/new.js")
	if err != nil {
		t.Fatalf("read the hunks: %v", err)
	}
	if answer.Status != protocol.DiffFileStatusAdded || len(answer.Hunks) != 1 {
		t.Fatalf("the answer = %+v, want one hunk of an addition", answer)
	}
	if lines := answer.Hunks[0].Lines; len(lines) != 2 || lines[1].Text != "export const two = 2;" {
		t.Errorf("the lines = %+v", lines)
	}
}

// A file with more lines than the limit comes back cut, which is the whole of what keeps a huge
// file from stalling the panel that opened it.
func TestHunksAreCutAtTheLineLimit(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, diff.WithLimits(diff.Limits{MaxLines: 3}))
	e.write(t, "src/util.js", "// one\n// two\n// three\n// four\n// five\n")

	answer, err := e.service.Hunks(ctx, e.projects.card.ID, "src/util.js")
	if err != nil {
		t.Fatalf("read the hunks: %v", err)
	}
	if !answer.Truncated || len(answer.Hunks) != 1 || len(answer.Hunks[0].Lines) != 3 {
		t.Errorf("the answer = %+v, want three lines and Truncated", answer)
	}
}

func TestHunksAnswerNotFound(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		path string
		gone bool
	}{
		{"a file that did not change", "README.md", false},
		{"a path that walks upwards", "../secret", false},
		{"a path that is empty", "", false},
		{"a card whose worktree is gone", "src/util.js", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write(t, "src/util.js", "// changed\n")
			if tc.gone {
				e.projects.path, e.projects.branch = "", ""
			}
			_, err := e.service.Hunks(ctx, e.projects.card.ID, tc.path)
			wantNotFound(t, err, tc.path)
		})
	}
}

// A path whose name looks like a Git pattern is read as the name it is, and one that is not part of
// the card's diff is not found rather than read from somewhere else in the worktree.
func TestHunksReadAFilePathLiterally(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.write(t, "src/*.js", "export const star = true;\n")

	answer, err := e.service.Hunks(ctx, e.projects.card.ID, "src/*.js")
	if err != nil {
		t.Fatalf("read the hunks of a file named like a pattern: %v", err)
	}
	if answer.Status != protocol.DiffFileStatusAdded {
		t.Errorf("the answer = %+v", answer)
	}
}

func TestHunksReportAGitFailure(t *testing.T) {
	e := newEnv(t)
	e.projects.project.DefaultBranch = "no-such-branch"
	e.write(t, "src/util.js", "// changed\n")

	if _, err := e.service.Hunks(context.Background(), e.projects.card.ID, "src/util.js"); err == nil {
		t.Fatal("a diff against a branch that does not exist answered without an error")
	}
}

func TestHunksPassOnWhatTheModulesSay(t *testing.T) {
	e := newEnv(t)
	e.projects.cardErr = notFoundCard()
	_, err := e.service.Hunks(context.Background(), e.projects.card.ID, "src/util.go")
	wantNotFoundCard(t, err)
}

// The service needs both of its parts, and it refuses to be built without them.
func TestNewNeedsItsParts(t *testing.T) {
	git := testutil.Git()
	tests := []struct {
		name string
		deps diff.Deps
	}{
		{"no projects module", diff.Deps{Git: git}},
		{"no Git", diff.Deps{Projects: &projects{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := diff.New(tc.deps); err == nil {
				t.Error("the service was built without one of its parts")
			}
		})
	}
}

// A limit that is left at zero takes its default, so a caller can change one bound and leave the
// rest alone.
func TestLimitsFallBackToTheirDefaults(t *testing.T) {
	tests := []struct {
		name   string
		limits diff.Limits
		large  int
	}{
		{"nothing set", diff.Limits{}, diff.DefaultLargeLines},
		{"one set", diff.Limits{MaxFiles: 3}, diff.DefaultLargeLines},
		{"all set", diff.Limits{MaxFiles: 3, MaxLines: 4, MaxBytes: 1024, LargeLines: 7}, 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, diff.WithLimits(tc.limits))
			var added strings.Builder
			for i := 0; i <= tc.large; i++ {
				added.WriteString("// a line the card added\n")
			}
			e.write(t, "src/util.js", added.String())
			answer, err := e.service.Files(context.Background(), e.projects.card.ID)
			if err != nil {
				t.Fatalf("read the diff: %v", err)
			}
			if !find(t, answer, "src/util.js").Large {
				t.Errorf("a file with %d changed lines is not large", tc.large+1)
			}
		})
	}
}
