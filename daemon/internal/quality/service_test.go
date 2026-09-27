package quality_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/quality"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The quality module is driven over a real database in a temporary directory - the rows that are
// written are the rows the daemon would write - but never a real linter, a real agent, or the
// network: Cards, Projects, Git, the worker, the bus, and the linter are fakes. The only program the
// tests start is Git, through gitx over a throwaway folder that holds the "worktree" files.

var testNow = time.Date(2026, time.September, 27, 11, 0, 0, 0, time.UTC)

const (
	testProjectID = "web-dashboard"
	testCardID    = "01M3C107JB041061050R3GG28A"
)

// openTestStore opens a real database in a temporary directory.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(),
		filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedProject adds a project and a board, so a card can be added and findings have somewhere to
// point. Nothing else about the project matters to the quality module.
func seedProject(t *testing.T, st *store.Store, id, defaultBranch string) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		now := testNow.UnixMilli()
		if err := q.CreateProject(ctx, db.CreateProjectParams{
			ID: id, Name: id, RepoPath: "/code/" + id, DefaultBranch: defaultBranch,
			PackagesJSON: "[]", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return q.CreateBoard(ctx, db.CreateBoardParams{
			ID: "board-" + id, ProjectID: id, ColumnsJSON: `["backlog","working","needs","done"]`,
		})
	})
	if err != nil {
		t.Fatalf("seed the project %s: %v", id, err)
	}
}

// seedCard adds a card, so a finding has a card to belong to.
func seedCard(t *testing.T, st *store.Store, id, projectID string) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		now := testNow.UnixMilli()
		return q.CreateCard(ctx, db.CreateCardParams{
			ID: id, ProjectID: projectID, Number: 1, BoardID: "board-" + projectID,
			Title: "Card", State: string(protocol.CardStateWorking), AgentKind: "claude",
			PermissionMode: "auto-edits", CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("seed the card %s: %v", id, err)
	}
}

// fakeCards answers one card.
type fakeCards struct {
	card protocol.Card
	err  error
}

func (f *fakeCards) Card(_ context.Context, id string) (protocol.Card, error) {
	if f.err != nil {
		return protocol.Card{}, f.err
	}
	out := f.card
	if out.ID == "" {
		out.ID = id
	}
	return out, nil
}

// fakeProjects answers one project and one worktree path.
type fakeProjects struct {
	project protocol.Project
	path    string
	branch  string
	err     error
}

func (f *fakeProjects) Get(_ context.Context, id string) (protocol.Project, error) {
	if f.err != nil {
		return protocol.Project{}, f.err
	}
	out := f.project
	if out.ID == "" {
		out.ID = id
	}
	return out, nil
}

func (f *fakeProjects) Worktree(_ context.Context, _ string) (string, string, error) {
	return f.path, f.branch, nil
}

// fakeGit answers a card's diff, its files' hunks, and the two plain commands the checks read.
type fakeGit struct {
	files    []gitx.DiffFile
	hunks    map[string]gitx.DiffFileHunks
	baseCopy map[string]string
	head     string
	diffRuns int
	err      error
}

func (g *fakeGit) DiffFiles(_ context.Context, _, _ string, _ int) ([]gitx.DiffFile, bool, error) {
	g.diffRuns++
	if g.err != nil {
		return nil, false, g.err
	}
	return g.files, false, nil
}

func (g *fakeGit) DiffHunks(_ context.Context, _, _, path string, _, _ int) (gitx.DiffFileHunks, bool, error) {
	hunks, found := g.hunks[path]
	return hunks, found, nil
}

func (g *fakeGit) FileAtCommit(_ context.Context, _, _, path string) (string, error) {
	return g.baseCopy[path], nil
}

func (g *fakeGit) Run(_ context.Context, _ string, args ...string) (string, error) {
	if len(args) > 0 {
		switch args[0] {
		case "rev-parse":
			return g.head + "\n", nil
		case "merge-base":
			return "basesha\n", nil
		}
	}
	return "", nil
}

// fakeWorker records what was sent to the agent.
type fakeWorker struct {
	sent []string
	err  error
}

func (w *fakeWorker) Send(_ context.Context, _, text string) error {
	if w.err != nil {
		return w.err
	}
	w.sent = append(w.sent, text)
	return nil
}

// fakeBus records the event types published.
type fakeBus struct{ events []string }

func (b *fakeBus) Publish(_, eventType string, _ any, _ bool) uint64 {
	b.events = append(b.events, eventType)
	return uint64(len(b.events))
}

// fakeLinter answers whatever a test told it to, and records the requests it was given.
type fakeLinter struct {
	findings []quality.LinterFinding
	err      error
	requests []quality.LintRequest
}

func (l *fakeLinter) Lint(_ context.Context, req quality.LintRequest) ([]quality.LinterFinding, error) {
	l.requests = append(l.requests, req)
	if l.err != nil {
		return nil, l.err
	}
	return l.findings, nil
}

// newService builds a service over the store, with a fixed clock and fixed entropy so the answers
// and the findings' ids are the same on every run.
func newService(t *testing.T, st *store.Store, deps quality.Deps, opts ...quality.Option) *quality.Service {
	t.Helper()
	deps.Store = st
	opts = append([]quality.Option{
		quality.WithClock(func() time.Time { return testNow }),
		quality.WithEntropy(rand.New(rand.NewSource(1))),
	}, opts...)
	svc, err := quality.New(deps, opts...)
	if err != nil {
		t.Fatalf("build the quality service: %v", err)
	}
	return svc
}

// projectDeps is the usual set of fakes: one project, one card, and the diff a test wants.
func projectDeps(git *fakeGit, path string) quality.Deps {
	return quality.Deps{
		Cards: &fakeCards{card: protocol.Card{ID: testCardID, ProjectID: testProjectID}},
		Projects: &fakeProjects{
			project: protocol.Project{ID: testProjectID, DefaultBranch: "main"},
			path:    path,
		},
		Git: git,
	}
}

// writeFile writes one file into a folder, making the folders on the way.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("make the folder for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// padding is a file of n distinct comment lines, which no built-in check flags: it is used to push a
// file over the length limit without tripping the line, number, nesting, or duplication checks.
func padding(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "// padding line %d\n", i)
	}
	return b.String()
}

// seedCheckable makes the store and the fakes for one card whose worktree is a real folder. It
// answers the service and the fake Git, so a test can assert on the number of diff runs.
func seedCheckable(t *testing.T, path string, git *fakeGit, opts ...quality.Option) *quality.Service {
	t.Helper()
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	seedCard(t, st, testCardID, testProjectID)
	return newService(t, st, projectDeps(git, path), opts...)
}

// protocolError reads the API error out of an error, or fails the test.
func protocolError(t *testing.T, err error) *protocol.Error {
	t.Helper()
	if err == nil {
		t.Fatal("wanted an error, got none")
	}
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("wanted a protocol error, got %T: %v", err, err)
	}
	return perr
}

func TestACardWithNoWorktreeIsNotChecked(t *testing.T) {
	git := &fakeGit{}
	svc := seedCheckable(t, "", git)

	list, err := svc.Check(context.Background(), testCardID)
	if err != nil {
		t.Fatalf("check a card that never started: %v", err)
	}
	if len(list.Findings) != 0 {
		t.Fatalf("a card with no worktree has no findings, got %d", len(list.Findings))
	}
	if list.Blocking != 0 || list.Commit != "" || list.Checked != nil {
		t.Fatalf("an unchecked card answers an empty, unchecked list, got %+v", list)
	}
	if git.diffRuns != 0 {
		t.Fatalf("a card with no worktree runs no diff, ran %d", git.diffRuns)
	}
}

func TestANewFileOverTheLengthLimitBlocks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	svc := seedCheckable(t, dir, git)

	list, err := svc.Check(context.Background(), testCardID)
	if err != nil {
		t.Fatalf("check the card: %v", err)
	}
	if len(list.Findings) != 1 {
		t.Fatalf("a new file of 900 lines is one finding, got %d: %+v", len(list.Findings), list.Findings)
	}
	found := list.Findings[0]
	if found.Smell != string(protocol.SmellCheckLargeFile) {
		t.Fatalf("the finding is the large-file check, got %q", found.Smell)
	}
	if found.Family != protocol.SmellFamilyBloaters {
		t.Fatalf("a large file is a bloater, got %q", found.Family)
	}
	if found.Severity != protocol.SmellSeverityBlocking {
		t.Fatalf("the default profile blocks a large file, got %q", found.Severity)
	}
	if found.Status != protocol.SmellStatusOpen {
		t.Fatalf("a new finding is open, got %q", found.Status)
	}
	if found.Line != 0 {
		t.Fatalf("a finding about a whole file names no line, got %d", found.Line)
	}
	if list.Blocking != 1 {
		t.Fatalf("one blocking finding blocks the card, got %d", list.Blocking)
	}
	if list.Commit != "abc123" {
		t.Fatalf("the findings are stamped with the head commit, got %q", list.Commit)
	}
	if list.Checked == nil || !list.Checked.Time().Equal(testNow) {
		t.Fatalf("a checked card says when it was checked, got %v", list.Checked)
	}
	if !protocol.ValidID(found.ID) {
		t.Fatalf("a finding's id is an opaque id, got %q", found.ID)
	}
}

func TestACommitIsCheckedOnce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	svc := seedCheckable(t, dir, git)
	ctx := context.Background()

	first, err := svc.Check(ctx, testCardID)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	second, err := svc.Check(ctx, testCardID)
	if err != nil {
		t.Fatalf("second check: %v", err)
	}
	if git.diffRuns != 1 {
		t.Fatalf("the same commit is checked once, the diff ran %d times", git.diffRuns)
	}
	if len(second.Findings) != len(first.Findings) {
		t.Fatalf("the second check answers what the first saved: %d vs %d", len(second.Findings), len(first.Findings))
	}
	if second.Findings[0].ID != first.Findings[0].ID {
		t.Fatalf("the cached finding keeps its id: %q vs %q", second.Findings[0].ID, first.Findings[0].ID)
	}
}

func TestFindingsReadsWithoutRunningAnything(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	svc := seedCheckable(t, dir, git)
	ctx := context.Background()

	if _, err := svc.Check(ctx, testCardID); err != nil {
		t.Fatalf("check the card: %v", err)
	}
	before := git.diffRuns
	list, err := svc.Findings(ctx, testCardID)
	if err != nil {
		t.Fatalf("read the findings: %v", err)
	}
	if git.diffRuns != before {
		t.Fatalf("reading the findings runs no diff, ran %d more", git.diffRuns-before)
	}
	if len(list.Findings) != 1 || list.Blocking != 1 {
		t.Fatalf("reading answers what the check saved, got %+v", list)
	}
}

func TestABlockingFindingGoesBackToTheAgentBeforeReview(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	worker := &fakeWorker{}
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	seedCard(t, st, testCardID, testProjectID)
	svc := newService(t, st, quality.Deps{
		Cards:    &fakeCards{card: protocol.Card{ID: testCardID, ProjectID: testProjectID}},
		Projects: &fakeProjects{project: protocol.Project{ID: testProjectID, DefaultBranch: "main"}, path: dir},
		Git:      git,
		Worker:   worker,
	})

	list, err := svc.BeforeReview(context.Background(), testCardID)
	if err != nil {
		t.Fatalf("check before review: %v", err)
	}
	if list.Blocking != 1 {
		t.Fatalf("the new file blocks the move to review, got %d", list.Blocking)
	}
	if len(worker.sent) != 1 {
		t.Fatalf("a blocking finding is sent to the agent once, sent %d", len(worker.sent))
	}
	if !strings.Contains(worker.sent[0], "code-quality checks") {
		t.Fatalf("the message to the agent says what it is, got %q", worker.sent[0])
	}
}

func TestANonBlockingFindingIsNotSentToTheAgent(t *testing.T) {
	dir := t.TempDir()
	long := "const value = \"" + strings.Repeat("x", 200) + "\";"
	writeFile(t, dir, "lines.js", long+"\n")
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "lines.js", Status: gitx.DiffStatusAdded}},
		head:  "def456",
	}
	worker := &fakeWorker{}
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	seedCard(t, st, testCardID, testProjectID)
	svc := newService(t, st, quality.Deps{
		Cards:    &fakeCards{card: protocol.Card{ID: testCardID, ProjectID: testProjectID}},
		Projects: &fakeProjects{project: protocol.Project{ID: testProjectID, DefaultBranch: "main"}, path: dir},
		Git:      git,
		Worker:   worker,
	})

	n, err := svc.BlockingFindings(context.Background(), testCardID)
	if err != nil {
		t.Fatalf("ask whether anything blocks: %v", err)
	}
	if n != 0 {
		t.Fatalf("a long line warns rather than blocks, got %d blocking", n)
	}
	if len(worker.sent) != 0 {
		t.Fatalf("a warning is not sent to the agent, sent %d messages", len(worker.sent))
	}
}

func TestAPeerLintersFindingIsKeptOnlyOnTheCardsOwnLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/util.js", "const a = 1;\nconst b = 2;\nconst c = 3;\nconst d = 4;\n")
	linter := &fakeLinter{findings: []quality.LinterFinding{
		{File: "src/util.js", Line: 3, Message: "this line reaches into another module"},
		{File: "src/util.js", Line: 1, Message: "this line was already here"},
	}}
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "src/util.js", Status: gitx.DiffStatusModified}},
		hunks: map[string]gitx.DiffFileHunks{
			"src/util.js": {Hunks: []gitx.DiffHunk{{Lines: []gitx.DiffLine{
				{Kind: gitx.DiffLineContext, NewLine: 1, Text: "const a = 1;"},
				{Kind: gitx.DiffLineAdded, NewLine: 3, Text: "const c = 3;"},
			}}}},
		},
		baseCopy: map[string]string{"src/util.js": "const a = 1;\nconst b = 2;\n"},
		head:     "abc123",
	}
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	seedCard(t, st, testCardID, testProjectID)
	svc := newService(t, st, quality.Deps{
		Cards:    &fakeCards{card: protocol.Card{ID: testCardID, ProjectID: testProjectID}},
		Projects: &fakeProjects{project: protocol.Project{ID: testProjectID, DefaultBranch: "main"}, path: dir},
		Git:      git,
		NewLinter: func(protocol.SmellLinter) (quality.Linter, error) {
			return linter, nil
		},
	})
	profile := protocol.DefaultSmellProfile()
	profile.Linters = []protocol.SmellLinter{
		{Name: "eslint", Command: []string{"eslint"}, Family: protocol.SmellFamilyCouplers},
	}
	if _, err := svc.SetProfile(context.Background(), testProjectID, profile); err != nil {
		t.Fatalf("save the profile: %v", err)
	}

	list, err := svc.Check(context.Background(), testCardID)
	if err != nil {
		t.Fatalf("check the card: %v", err)
	}
	if len(linter.requests) != 1 {
		t.Fatalf("the project's linter runs once, ran %d", len(linter.requests))
	}
	if got := linter.requests[0].Files; len(got) != 1 || got[0] != "src/util.js" {
		t.Fatalf("the linter is given the card's changed files, got %v", got)
	}
	if len(list.Findings) != 1 {
		t.Fatalf("only the finding on an added line is kept, got %d: %+v", len(list.Findings), list.Findings)
	}
	found := list.Findings[0]
	if found.Smell != "eslint" || found.Family != protocol.SmellFamilyCouplers {
		t.Fatalf("a linter's finding carries its name and family, got %+v", found)
	}
	if found.Severity != protocol.SmellSeverityWarning {
		t.Fatalf("a project linter's finding warns rather than blocks, got %q", found.Severity)
	}
}

func TestALinterThatCouldNotRunDoesNotFailTheCheck(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/util.js", "const a = 1;\n")
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "src/util.js", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	seedCard(t, st, testCardID, testProjectID)
	svc := newService(t, st, quality.Deps{
		Cards:    &fakeCards{card: protocol.Card{ID: testCardID, ProjectID: testProjectID}},
		Projects: &fakeProjects{project: protocol.Project{ID: testProjectID, DefaultBranch: "main"}, path: dir},
		Git:      git,
		NewLinter: func(protocol.SmellLinter) (quality.Linter, error) {
			return nil, errors.New("the linter is not installed")
		},
	})
	profile := protocol.DefaultSmellProfile()
	profile.Linters = []protocol.SmellLinter{{Name: "golangci-lint", Command: []string{"golangci-lint"}}}
	if _, err := svc.SetProfile(context.Background(), testProjectID, profile); err != nil {
		t.Fatalf("save the profile: %v", err)
	}

	if _, err := svc.Check(context.Background(), testCardID); err != nil {
		t.Fatalf("a linter that could not be built must not fail the check: %v", err)
	}
}

func TestADiffThatCouldNotBeReadIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", "package main\n")
	git := &fakeGit{err: errors.New("git is unhappy"), head: "abc123"}
	svc := seedCheckable(t, dir, git)

	if _, err := svc.Check(context.Background(), testCardID); err == nil {
		t.Fatal("a check that could not read the diff answers an error")
	}
}

func TestFixAsksTheAgentAndMarksTheFindingFixed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	worker := &fakeWorker{}
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	seedCard(t, st, testCardID, testProjectID)
	svc := newService(t, st, quality.Deps{
		Cards:    &fakeCards{card: protocol.Card{ID: testCardID, ProjectID: testProjectID}},
		Projects: &fakeProjects{project: protocol.Project{ID: testProjectID, DefaultBranch: "main"}, path: dir},
		Git:      git,
		Worker:   worker,
	})
	ctx := context.Background()

	list, err := svc.Check(ctx, testCardID)
	if err != nil {
		t.Fatalf("check the card: %v", err)
	}
	fixed, err := svc.Fix(ctx, testCardID, list.Findings[0].ID)
	if err != nil {
		t.Fatalf("ask the agent to fix the finding: %v", err)
	}
	if fixed.Status != protocol.SmellStatusFixed {
		t.Fatalf("a handed-over finding reads fixed, got %q", fixed.Status)
	}
	if len(worker.sent) != 1 || !strings.Contains(worker.sent[0], "Please fix") {
		t.Fatalf("the agent is asked to fix it, sent %v", worker.sent)
	}
	after, err := svc.Findings(ctx, testCardID)
	if err != nil {
		t.Fatalf("read the findings again: %v", err)
	}
	if after.Findings[0].Status != protocol.SmellStatusFixed {
		t.Fatalf("the fixed status is kept, got %q", after.Findings[0].Status)
	}
	if after.Blocking != 0 {
		t.Fatalf("a fixed blocking finding no longer blocks, got %d", after.Blocking)
	}
}

func TestFixWithoutAnAgentIsRefused(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	svc := seedCheckable(t, dir, git)
	ctx := context.Background()

	list, err := svc.Check(ctx, testCardID)
	if err != nil {
		t.Fatalf("check the card: %v", err)
	}
	_, err = svc.Fix(ctx, testCardID, list.Findings[0].ID)
	perr := protocolError(t, err)
	if perr.Code != protocol.ErrorCodeRefused {
		t.Fatalf("a card with no agent cannot be fixed: got %q", perr.Code)
	}
	if perr.Details["reason"] != "quality_no_agent" {
		t.Fatalf("the refusal says why, got %q", perr.Details["reason"])
	}
}

func TestDismissKeepsTheReason(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	svc := seedCheckable(t, dir, git)
	ctx := context.Background()

	list, err := svc.Check(ctx, testCardID)
	if err != nil {
		t.Fatalf("check the card: %v", err)
	}
	id := list.Findings[0].ID

	if _, err := svc.Dismiss(ctx, testCardID, id, "   "); err == nil {
		t.Fatal("a dismissal with no reason is refused")
	} else if perr := protocolError(t, err); perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Fatalf("a dismissal with no reason is invalid_argument, got %q", perr.Code)
	}
	long := strings.Repeat("x", 501)
	if _, err := svc.Dismiss(ctx, testCardID, id, long); err == nil {
		t.Fatal("an over-long reason is refused")
	} else if perr := protocolError(t, err); perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Fatalf("an over-long reason is invalid_argument, got %q", perr.Code)
	}

	dismissed, err := svc.Dismiss(ctx, testCardID, id, "not our house style")
	if err != nil {
		t.Fatalf("dismiss the finding: %v", err)
	}
	if dismissed.Status != protocol.SmellStatusDismissed || dismissed.DismissReason != "not our house style" {
		t.Fatalf("a dismissed finding keeps why, got %+v", dismissed)
	}
	after, err := svc.Findings(ctx, testCardID)
	if err != nil {
		t.Fatalf("read the findings again: %v", err)
	}
	if after.Blocking != 0 {
		t.Fatalf("a dismissed blocking finding no longer blocks, got %d", after.Blocking)
	}
}

func TestAUnknownFindingIsNotFound(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}}, head: "abc123"}
	svc := seedCheckable(t, dir, git)
	ctx := context.Background()

	for _, id := range []string{"not-an-id", "01M3C107JB041061050R3GG28B"} {
		if _, err := svc.Dismiss(ctx, testCardID, id, "why not"); err == nil {
			t.Fatalf("dismissing %q must fail", id)
		} else if perr := protocolError(t, err); perr.Code != protocol.ErrorCodeNotFound {
			t.Fatalf("an unknown finding is not_found, got %q for %q", perr.Code, id)
		}
	}
}

func TestTheDefaultProfileFillsInEveryThresholdAndCheck(t *testing.T) {
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	svc := newService(t, st, quality.Deps{
		Cards:    &fakeCards{},
		Projects: &fakeProjects{project: protocol.Project{ID: testProjectID, DefaultBranch: "main"}},
		Git:      &fakeGit{},
	})

	profile, err := svc.Profile(context.Background(), testProjectID)
	if err != nil {
		t.Fatalf("read the profile: %v", err)
	}
	if profile.ProjectID != testProjectID {
		t.Fatalf("the profile names its project, got %q", profile.ProjectID)
	}
	if len(profile.Checks) != len(protocol.SmellCheckValues()) {
		t.Fatalf("a resolved profile names every check, got %d", len(profile.Checks))
	}
	want := protocol.DefaultSmellProfile()
	if profile.MaxFunctionLines != want.MaxFunctionLines || profile.MaxFileLines != want.MaxFileLines {
		t.Fatalf("a project with no profile uses the defaults, got %+v", profile)
	}
	blocking := map[protocol.SmellCheck]bool{}
	for _, setting := range profile.Checks {
		blocking[setting.Check] = setting.Severity == protocol.SmellSeverityBlocking
	}
	for _, check := range []protocol.SmellCheck{
		protocol.SmellCheckLongFunction, protocol.SmellCheckLargeFile, protocol.SmellCheckDuplicateBlock,
	} {
		if !blocking[check] {
			t.Fatalf("the default profile blocks %q", check)
		}
	}
}

func TestSetProfileRoundTripsAndRefusesWhatItCannotUse(t *testing.T) {
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	svc := newService(t, st, quality.Deps{
		Cards:    &fakeCards{},
		Projects: &fakeProjects{project: protocol.Project{ID: testProjectID, DefaultBranch: "main"}},
		Git:      &fakeGit{},
	})
	ctx := context.Background()

	want := protocol.DefaultSmellProfile()
	want.MaxFunctionLines = 120
	want.Checks = []protocol.SmellCheckSetting{
		{Check: protocol.SmellCheckLongFunction, Enabled: true, Severity: protocol.SmellSeverityInfo},
	}
	want.Linters = []protocol.SmellLinter{{Name: "eslint", Command: []string{"eslint", "--format", "unix"}}}
	saved, err := svc.SetProfile(ctx, testProjectID, want)
	if err != nil {
		t.Fatalf("save the profile: %v", err)
	}
	if saved.MaxFunctionLines != 120 {
		t.Fatalf("the saved threshold is answered back, got %d", saved.MaxFunctionLines)
	}
	if len(saved.Linters) != 1 || saved.Linters[0].Name != "eslint" {
		t.Fatalf("the project's linters are kept, got %+v", saved.Linters)
	}
	read, err := svc.Profile(ctx, testProjectID)
	if err != nil {
		t.Fatalf("read the profile back: %v", err)
	}
	if read.MaxFunctionLines != 120 {
		t.Fatalf("the threshold is kept, got %d", read.MaxFunctionLines)
	}

	cases := map[string]protocol.SmellProfile{
		"a threshold under its bound": {MaxFunctionLines: 1},
		"an unknown check": {Checks: []protocol.SmellCheckSetting{
			{Check: "no-such-check", Enabled: true, Severity: protocol.SmellSeverityWarning}}},
		"an unknown severity": {Checks: []protocol.SmellCheckSetting{
			{Check: protocol.SmellCheckLongLine, Enabled: true, Severity: "loud"}}},
		"a check listed twice": {Checks: []protocol.SmellCheckSetting{
			{Check: protocol.SmellCheckLongLine, Enabled: true, Severity: protocol.SmellSeverityWarning},
			{Check: protocol.SmellCheckLongLine, Enabled: false, Severity: protocol.SmellSeverityInfo}}},
		"a linter with no name": {Linters: []protocol.SmellLinter{{Command: []string{"eslint"}}}},
		"a linter with no program": {
			Linters: []protocol.SmellLinter{{Name: "eslint", Command: []string{}}}},
	}
	for name, profile := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.SetProfile(ctx, testProjectID, profile)
			if err == nil {
				t.Fatal("wanted a refusal")
			}
			if perr := protocolError(t, err); perr.Code != protocol.ErrorCodeInvalidArgument {
				t.Fatalf("wanted invalid_argument, got %q", perr.Code)
			}
		})
	}
}

func TestTheChecksPublishOneEventForTheCard(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "big.go", padding(900))
	git := &fakeGit{
		files: []gitx.DiffFile{{Path: "big.go", Status: gitx.DiffStatusAdded}},
		head:  "abc123",
	}
	bus := &fakeBus{}
	st := openTestStore(t)
	seedProject(t, st, testProjectID, "main")
	seedCard(t, st, testCardID, testProjectID)
	svc := newService(t, st, quality.Deps{
		Cards:    &fakeCards{card: protocol.Card{ID: testCardID, ProjectID: testProjectID}},
		Projects: &fakeProjects{project: protocol.Project{ID: testProjectID, DefaultBranch: "main"}, path: dir},
		Git:      git,
		Bus:      bus,
	})

	if _, err := svc.Check(context.Background(), testCardID); err != nil {
		t.Fatalf("check the card: %v", err)
	}
	if len(bus.events) != 1 || bus.events[0] != string(protocol.EventTypeQualityChecked) {
		t.Fatalf("a check publishes one quality.checked event, got %v", bus.events)
	}
}
