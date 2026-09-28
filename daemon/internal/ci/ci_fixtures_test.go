package ci_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/ci"
	gh "github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/harness"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The CI monitor is driven over a real database in a temporary directory, because the rows it writes
// are the rows the daemon writes and the fix loop's whole job is to leave them in the right state.
// Nothing else is real: the projects, the cards, the roles, the session, the bus, and the forge are
// all fakes, and no test opens a socket, starts a program, or needs a GitHub account.

var testNow = time.Date(2026, time.September, 27, 11, 0, 0, 0, time.UTC)

const (
	testProjectID = "web-dashboard"
	testCardID    = "01M3C107JB041061050R3GG28A"
	testBranch    = "marshal/ci-dashboard"
	testRole      = "engineer"
	testRunID     = int64(7001234)
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

// seedProject adds a project and its board. Nothing else about it matters here.
func seedProject(t *testing.T, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		now := testNow.UnixMilli()
		if err := q.CreateProject(ctx, db.CreateProjectParams{
			ID: id, Name: id, RepoPath: "/code/" + id, DefaultBranch: "main",
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

// seedCard adds a card on the given branch, with the given role. An empty branch is a card that has
// not started.
func seedCard(t *testing.T, st *store.Store, id, projectID, branch, role string) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		now := testNow.UnixMilli()
		if err := q.CreateCard(ctx, db.CreateCardParams{
			ID: id, ProjectID: projectID, Number: 1, BoardID: "board-" + projectID,
			Title: "Card", State: string(protocol.CardStateWorking), AgentKind: "claude",
			PermissionMode: "auto-edits", Role: role, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		if branch == "" {
			return nil
		}
		_, err := q.UpdateCardWorktree(ctx, db.UpdateCardWorktreeParams{
			WorktreePath: "/code/" + projectID + "/.marshal/" + id, Branch: branch,
			UpdatedAt: now, ID: id,
		})
		return err
	})
	if err != nil {
		t.Fatalf("seed the card %s: %v", id, err)
	}
}

// seedTurn records one message delivered into a card's session, which is what the harness counts as
// a round. It is how a test puts a card near its loop limit without a session manager. The card's
// session row is made first, because a session event belongs to a session.
func seedTurn(t *testing.T, st *store.Store, cardID string, seq int64) {
	t.Helper()
	ctx := context.Background()
	err := st.Write(ctx, func(q *db.Queries) error {
		now := testNow.UnixMilli()
		if seq == 1 {
			if err := q.CreateCardSession(ctx, db.CreateCardSessionParams{
				ID: "sess-" + cardID, CardID: cardID, AgentKind: "claude", AgentSessionID: "",
				State: "awake", Model: "", Thinking: "", PermissionMode: "auto-edits",
				LastActiveAt: now, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		return q.InsertSessionEvent(ctx, db.InsertSessionEventParams{
			ID: fmt.Sprintf("evt-%s-%d", cardID, seq), CardID: cardID, SessionID: "sess-" + cardID,
			Seq: seq, Kind: "user", State: "", Summary: "a message", DetailJSON: "{}",
			LogRef: "", CreatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("seed a turn for card %s: %v", cardID, err)
	}
}

// seedRun inserts a run row directly, for the poll's and the snapshot's tests.
func seedRun(t *testing.T, st *store.Store, row db.CiRun) {
	t.Helper()
	err := st.Write(context.Background(), func(q *db.Queries) error {
		return q.UpsertCiRun(context.Background(), db.UpsertCiRunParams{
			ID: row.ID, ProjectID: row.ProjectID, CardID: row.CardID, Branch: row.Branch,
			Workflow: row.Workflow, Status: row.Status, Url: row.Url,
			StartedAt: row.StartedAt, UpdatedAt: row.UpdatedAt, RerunAt: row.RerunAt,
		})
	})
	if err != nil {
		t.Fatalf("seed the run %s: %v", row.ID, err)
	}
}

// readRun reads one run back, so a test can assert what the monitor stored.
func readRun(t *testing.T, st *store.Store, id string) db.CiRun {
	t.Helper()
	var row db.CiRun
	err := st.Read(context.Background(), func(q *db.Queries) error {
		got, err := q.GetCiRun(context.Background(), id)
		if err != nil {
			return err
		}
		row = got
		return nil
	})
	if err != nil {
		t.Fatalf("read the run %s: %v", id, err)
	}
	return row
}

// The fakes.

// fakeProjects answers the project list and one project.
type fakeProjects struct {
	projects []protocol.Project
	worktree string
	err      error
}

func (f *fakeProjects) List(context.Context) (protocol.ProjectListSnapshot, error) {
	if f.err != nil {
		return protocol.ProjectListSnapshot{}, f.err
	}
	out := protocol.ProjectListSnapshot{Projects: f.projects, ServerTime: protocol.NewTimestamp(testNow)}
	return out, nil
}

func (f *fakeProjects) Get(_ context.Context, id string) (protocol.Project, error) {
	if f.err != nil {
		return protocol.Project{}, f.err
	}
	for _, project := range f.projects {
		if project.ID == id {
			return project, nil
		}
	}
	return protocol.Project{}, errors.New("no such project")
}

// Worktree answers the folder a card's work is in, which only the real mode of a simulated failure
// reads. The default is a path that exists only in this test, so nothing here can write a file.
func (f *fakeProjects) Worktree(context.Context, string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return f.worktree, testBranch, nil
}

// fakeGit stands in for Git in the real mode of a simulated failure: it reports the remotes an
// Inspect would, and remembers the marked commit and the push instead of writing a file or opening
// a connection. No test here touches a branch or a network.
type fakeGit struct {
	remotes []gitx.Remote
	main    string

	committed []gitxFile
	pushed    []gitxPush
	commitErr error
	pushErr   error
}

// gitxFile is one file fakeGit was asked to commit.
type gitxFile struct {
	dir, path, content, message string
}

// gitxPush is one push fakeGit was asked to run.
type gitxPush struct {
	dir, remote, branch, main string
}

func (f *fakeGit) Inspect(context.Context, string) (gitx.RepoInfo, error) {
	return gitx.RepoInfo{DefaultBranch: f.main, Remotes: f.remotes}, nil
}

func (f *fakeGit) CommitFile(_ context.Context, dir, path, content, message string) (string, error) {
	if f.commitErr != nil {
		return "", f.commitErr
	}
	f.committed = append(f.committed, gitxFile{dir: dir, path: path, content: content, message: message})
	return "the-marked-commit", nil
}

func (f *fakeGit) Push(_ context.Context, dir, remote, branch, main string) error {
	if f.pushErr != nil {
		return f.pushErr
	}
	f.pushed = append(f.pushed, gitxPush{dir: dir, remote: remote, branch: branch, main: main})
	return nil
}

// fakeCards answers the project's cards, remembers the movements to Needs you, and remembers every
// CI state written onto a card.
type fakeCards struct {
	cards   []protocol.Card
	needs   []protocol.NeedsReason
	ciState []*protocol.CIState
	err     error
}

func (f *fakeCards) Card(_ context.Context, id string) (protocol.Card, error) {
	if f.err != nil {
		return protocol.Card{}, f.err
	}
	for _, card := range f.cards {
		if card.ID == id {
			return card, nil
		}
	}
	return protocol.Card{}, protocol.NotFound("card")
}

func (f *fakeCards) Cards(_ context.Context, projectID string) ([]protocol.Card, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []protocol.Card
	for _, card := range f.cards {
		if card.ProjectID == projectID {
			out = append(out, card)
		}
	}
	return out, nil
}

func (f *fakeCards) SetNeeds(_ context.Context, id string, reason protocol.NeedsReason) (protocol.Card, error) {
	f.needs = append(f.needs, reason)
	for _, card := range f.cards {
		if card.ID == id {
			return card, nil
		}
	}
	return protocol.Card{ID: id}, nil
}

func (f *fakeCards) SetCI(_ context.Context, id string, state *protocol.CIState) (protocol.Card, error) {
	f.ciState = append(f.ciState, state)
	for _, card := range f.cards {
		if card.ID == id {
			return card, nil
		}
	}
	return protocol.Card{ID: id}, nil
}

// fakeRoles answers one project's ceilings for one role.
type fakeRoles struct {
	limits harness.Limits
	found  bool
	err    error
}

func (f *fakeRoles) LimitsFor(context.Context, string, string) (harness.Limits, bool, error) {
	return f.limits, f.found, f.err
}

// fakeForge is the forge, with every call recorded so a test can assert what was asked and what was
// never asked.
type fakeForge struct {
	runs       []gh.WorkflowRun
	log        string
	rerunErr   error
	logErr     error
	listErr    error
	reruns     []int64
	logReads   []int64
	listBrancs []string
}

func (f *fakeForge) ListWorkflowRuns(_ context.Context, _ gh.Repository, branch string) ([]gh.WorkflowRun, error) {
	f.listBrancs = append(f.listBrancs, branch)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.runs, nil
}

func (f *fakeForge) RerunFailedJobs(_ context.Context, _ gh.Repository, runID int64) error {
	f.reruns = append(f.reruns, runID)
	return f.rerunErr
}

func (f *fakeForge) FailedLog(_ context.Context, _ gh.Repository, runID int64, _ int) (string, error) {
	f.logReads = append(f.logReads, runID)
	if f.logErr != nil {
		return "", f.logErr
	}
	return f.log, nil
}

// fakeWorker records the messages handed to a card's session.
type fakeWorker struct {
	sent []string
	err  error
}

func (f *fakeWorker) Send(_ context.Context, _, text string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, text)
	return nil
}

// fakeBus records what was published.
type fakeBus struct {
	topics []string
	types  []string
	data   []any
}

func (f *fakeBus) Publish(topic, eventType string, data any, _ bool) uint64 {
	f.topics = append(f.topics, topic)
	f.types = append(f.types, eventType)
	f.data = append(f.data, data)
	return uint64(len(f.topics))
}

// fixtures builds a monitor over a seeded store with every fake wired in.
type fixtures struct {
	svc      *ci.Service
	store    *store.Store
	cards    *fakeCards
	projects *fakeProjects
	roles    *fakeRoles
	forge    *fakeForge
	worker   *fakeWorker
	bus      *fakeBus
	git      *fakeGit
	repo     gh.Repository
}

// newFixture wires the monitor. The repository a project is comes from the Repo seam, so no test
// reads a real remote or needs a Git repository on disk. The Git seam is wired too, but only the
// real mode of a simulated failure ever calls it, and fakeGit writes nothing anywhere.
func newFixture(t *testing.T, opts ...func(*ci.Options)) *fixtures {
	t.Helper()
	st := openTestStore(t)
	seedProject(t, st, testProjectID)
	seedCard(t, st, testCardID, testProjectID, testBranch, testRole)

	repo := gh.Repository{Owner: "khanblair", Name: "marshal"}
	f := &fixtures{
		store: st,
		cards: &fakeCards{cards: []protocol.Card{{
			ID: testCardID, ProjectID: testProjectID, Branch: testBranch, Role: testRole,
		}}},
		projects: &fakeProjects{projects: []protocol.Project{{
			ID: testProjectID, Name: testProjectID, Path: "/code/" + testProjectID,
			DefaultBranch: "main",
		}}, worktree: "/code/" + testProjectID + "/.marshal/" + testCardID},
		roles:  &fakeRoles{},
		forge:  &fakeForge{},
		worker: &fakeWorker{},
		bus:    &fakeBus{},
		git: &fakeGit{
			main:    "main",
			remotes: []gitx.Remote{{Name: "origin", URL: "https://github.com/khanblair/marshal.git"}},
		},
		repo: repo,
	}
	options := ci.Options{
		Now:   func() time.Time { return testNow },
		Forge: f.forge,
		Repo:  func(context.Context, protocol.Project) (gh.Repository, bool) { return repo, true },
	}
	for _, opt := range opts {
		opt(&options)
	}
	recorder, err := audit.New(st, func() time.Time { return testNow }, nil, nil)
	if err != nil {
		t.Fatalf("build the audit recorder: %v", err)
	}
	svc, err := ci.New(ci.Deps{
		Store: st, Cards: f.cards, Projects: f.projects, Roles: f.roles, Git: f.git,
		Worker: f.worker, Bus: f.bus, Audit: recorder, Options: options,
	})
	if err != nil {
		t.Fatalf("build the CI monitor: %v", err)
	}
	f.svc = svc
	return f
}

// deliveryBody is a `workflow_run` delivery as GitHub sends it, with only the fields Marshal reads.
func deliveryBody(repo gh.Repository, id int64, branch, status, conclusion string) []byte {
	body := map[string]any{
		"action": "completed",
		"workflow_run": map[string]any{
			"id": id, "name": "ci", "head_branch": branch, "status": status,
			"conclusion": conclusion,
			"html_url":   fmt.Sprintf("https://github.com/%s/actions/runs/%d", repo.String(), id),
		},
		"repository": map[string]any{
			"full_name": repo.String(), "name": repo.Name,
			"owner": map[string]any{"login": repo.Owner},
		},
	}
	// A queued run has not started, so GitHub sends no start time and the wire says null.
	if status != "queued" {
		body["workflow_run"].(map[string]any)["run_started_at"] =
			testNow.Add(-3 * time.Minute).Format(time.RFC3339)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return encoded
}

// deliver hands one delivery to the monitor's sink, the way the webhook route does.
func (f *fixtures) deliver(t *testing.T, id int64, status, conclusion string) {
	t.Helper()
	err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "workflow_run", Delivery: fmt.Sprintf("delivery-%d", id),
		Body: deliveryBody(f.repo, id, testBranch, status, conclusion),
	})
	if err != nil {
		t.Fatalf("deliver a workflow run: %v", err)
	}
}
