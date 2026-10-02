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
	// bases are the branches DiffFiles was asked to compare against.
	bases []string
	err   error
}

func (g *fakeGit) DiffFiles(_ context.Context, _, base string, _ int) ([]gitx.DiffFile, bool, error) {
	g.diffRuns++
	g.bases = append(g.bases, base)
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
