package ci_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

// Tests.

func TestAWorkflowRunDeliveryRecordsTheRunAndTheCardsBadge(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, testRunID, "completed", "success")

	row := readRun(t, f.store, fmt.Sprint(testRunID))
	if row.Status != string(protocol.CIStatePassed) {
		t.Fatalf("the run's status is %q, want %q", row.Status, protocol.CIStatePassed)
	}
	if row.CardID == nil || *row.CardID != testCardID {
		t.Fatalf("the run's card is %v, want %s", row.CardID, testCardID)
	}
	if row.Branch != testBranch || row.Workflow != "ci" || row.Url == "" {
		t.Fatalf("the run was stored as %+v", row)
	}
	if len(f.cards.ciState) != 1 || f.cards.ciState[0] == nil || *f.cards.ciState[0] != protocol.CIStatePassed {
		t.Fatalf("the card's badge was written as %v, want passed", f.cards.ciState)
	}
}

func TestARunTellsTheProjectAndHomeThatItsStateChanged(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, testRunID, "completed", "success")

	wantTopics := []string{string(protocol.ProjectTopic(testProjectID)), string(protocol.HomeTopic)}
	if strings.Join(f.bus.topics, ",") != strings.Join(wantTopics, ",") {
		t.Fatalf("published on %v, want %v", f.bus.topics, wantTopics)
	}
	for i, typ := range f.bus.types {
		if typ != string(protocol.EventTypeCIUpdated) {
			t.Fatalf("event %d is %q, want ci.updated", i, typ)
		}
	}
	project, ok := f.bus.data[0].(protocol.CIEventData)
	if !ok || project.Project == nil || project.Snapshot != nil {
		t.Fatalf("the project event carries %#v", f.bus.data[0])
	}
	home, ok := f.bus.data[1].(protocol.CIEventData)
	if !ok || home.Snapshot == nil || home.Project != nil {
		t.Fatalf("the home event carries %#v", f.bus.data[1])
	}
	if len(home.Snapshot.Projects) != 1 || home.Snapshot.Projects[0].ProjectID != testProjectID {
		t.Fatalf("the home snapshot is %+v", home.Snapshot)
	}
}

func TestOnlyTheWorkflowRunDeliveryIsRead(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "check_run", Body: []byte(`{"action":"completed"}`),
	}); err != nil {
		t.Fatalf("deliver a check_run: %v", err)
	}
	if err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "workflow_run", Body: []byte(`not json at all`),
	}); err != nil {
		t.Fatalf("an unreadable body must be accepted: %v", err)
	}
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if len(snapshot.Projects) != 0 {
		t.Fatalf("a delivery Marshal does not handle wrote %+v", snapshot.Projects)
	}
}

func TestADeliveryFromSomebodyElsesRepositoryIsIgnored(t *testing.T) {
	f := newFixture(t)
	elsewhere := gh.Repository{Owner: "someone", Name: "elsewhere"}
	body := deliveryBody(elsewhere, testRunID, testBranch, "completed", "success")
	if err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "workflow_run", Body: body,
	}); err != nil {
		t.Fatalf("deliver from another repository: %v", err)
	}
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if len(snapshot.Projects) != 0 {
		t.Fatalf("a run from another repository was recorded as %+v", snapshot.Projects)
	}
}

func TestAFailedRunRerunsTheFailedJobsOnce(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.forge.reruns) != 1 || f.forge.reruns[0] != testRunID {
		t.Fatalf("rerun asked for %v, want [%d]", f.forge.reruns, testRunID)
	}
	if len(f.forge.logReads) != 0 {
		t.Fatalf("the log was read on the first failure: %v", f.forge.logReads)
	}
	if len(f.worker.sent) != 0 {
		t.Fatalf("a message was sent on the first failure: %v", f.worker.sent)
	}
	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.RerunAt == 0 {
		t.Fatal("the rerun was not remembered, so a restart would rerun again")
	}
}

func TestASecondFailureSendsTheFailedStepsLogToTheCard(t *testing.T) {
	f := newFixture(t, func(o *ci.Options) { o.LogBytes = 4 << 10; o.LogLines = 3 })
	f.forge.log = "line one\nline two\nline three\nline four\nline five\n"
	f.deliver(t, testRunID, "completed", "failure")
	// The rerun comes back as the same run failing again, which is the second failure.
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.forge.logReads) != 1 || f.forge.logReads[0] != testRunID {
		t.Fatalf("the log was read %v, want [%d]", f.forge.logReads, testRunID)
	}
	if len(f.worker.sent) != 1 {
		t.Fatalf("messages sent %d, want 1", len(f.worker.sent))
	}
	message := f.worker.sent[0]
	for _, want := range []string{testBranch, "https://github.com/khanblair/marshal/actions/runs/", "line five"} {
		if !strings.Contains(message, want) {
			t.Fatalf("the message does not mention %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "line one") || strings.Contains(message, "line two") {
		t.Fatalf("the message kept lines past the limit:\n%s", message)
	}
	if !strings.Contains(message, "not shown") {
		t.Fatalf("the message does not say lines were dropped:\n%s", message)
	}
	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.FixSentAt == 0 {
		t.Fatal("sending the log was not remembered, so it would be sent again")
	}
}

func TestTheSameFailureIsSentOnlyOnce(t *testing.T) {
	f := newFixture(t)
	f.forge.log = "boom\n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.worker.sent) != 1 {
		t.Fatalf("messages sent %d, want 1", len(f.worker.sent))
	}
	if len(f.forge.logReads) != 1 {
		t.Fatalf("the log was read %d times, want 1", len(f.forge.logReads))
	}
}

func TestANewRunOnTheSameBranchStartsTheLoopOver(t *testing.T) {
	f := newFixture(t)
	f.forge.log = "boom\n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	// A second run of the same workflow on the same branch is a new run: it takes the row over and
	// the loop starts again from the rerun.
	const secondRun = int64(7009999)
	f.deliver(t, secondRun, "completed", "failure")
	if len(f.forge.reruns) != 2 || f.forge.reruns[1] != secondRun {
		t.Fatalf("rerun asked for %v, want the second run rerun", f.forge.reruns)
	}
	if row := readRun(t, f.store, fmt.Sprint(secondRun)); row.RerunAt == 0 || row.FixSentAt != 0 {
		t.Fatalf("the new run's memory is rerun_at=%d fix_sent_at=%d, want rerun and no fix", row.RerunAt, row.FixSentAt)
	}
}

func TestTheLoopStopsWhenTheCardIsAtItsRoleLimit(t *testing.T) {
	f := newFixture(t)
	// The card's role allows one round, and the card has already taken it.
	f.roles.limits, f.roles.found = harness.Limits{Rounds: 1}, true
	seedTurn(t, f.store, testCardID, 1)
	f.forge.log = "boom\n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.worker.sent) != 0 {
		t.Fatalf("a message was sent past the limit: %v", f.worker.sent)
	}
	if len(f.forge.logReads) != 0 {
		t.Fatalf("the log was read past the limit: %v", f.forge.logReads)
	}
	if len(f.cards.needs) != 1 {
		t.Fatalf("the card was moved to Needs you %d times, want 1", len(f.cards.needs))
	}
	if kind := f.cards.needs[0].Kind; kind != protocol.NeedsReasonKindCIFailed {
		t.Fatalf("the reason is %q, want %q", kind, protocol.NeedsReasonKindCIFailed)
	}
	if text := f.cards.needs[0].Text; !strings.Contains(text, "2") || !strings.Contains(text, "1") {
		t.Fatalf("the reason does not name the rounds and the limit: %q", text)
	}
}

func TestTheLoopDoesNotStopWhenTheRoleSetsNoCeiling(t *testing.T) {
	f := newFixture(t)
	f.roles.limits, f.roles.found = harness.Limits{}, true
	seedTurn(t, f.store, testCardID, 1)
	seedTurn(t, f.store, testCardID, 2)
	seedTurn(t, f.store, testCardID, 3)
	f.forge.log = "boom\n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.cards.needs) != 0 {
		t.Fatalf("a card with no ceiling was stopped: %v", f.cards.needs)
	}
	if len(f.worker.sent) != 1 {
		t.Fatalf("messages sent %d, want 1", len(f.worker.sent))
	}
}

func TestAFailureWithNoLogSendsNothing(t *testing.T) {
	f := newFixture(t)
	f.forge.log = "   \n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.worker.sent) != 0 {
		t.Fatalf("an empty log was sent: %v", f.worker.sent)
	}
	// Nothing was handed over, so nothing is remembered as handed over: a forge uploads a run's log
	// a little after the run finishes, and a later delivery of the same run must be free to find it.
	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.FixSentAt != 0 {
		t.Fatal("a run with no log was marked as sent, so its log would never be looked for again")
	}
}

func TestAForgeThatCannotBeAskedLeavesTheStateAlone(t *testing.T) {
	f := newFixture(t)
	f.forge.rerunErr = errors.New("github is unreachable")
	f.deliver(t, testRunID, "completed", "failure")

	row := readRun(t, f.store, fmt.Sprint(testRunID))
	if row.Status != string(protocol.CIStateFailed) {
		t.Fatalf("the failure was lost: the run is %q", row.Status)
	}
	if row.RerunAt != 0 {
		t.Fatal("a rerun that was never asked for was remembered, so it would never be retried")
	}
}

func TestADaemonWithNoForgeStillRecordsAFailure(t *testing.T) {
	f := newFixture(t, func(o *ci.Options) { o.Forge = nil })
	f.deliver(t, testRunID, "completed", "failure")

	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.Status != string(protocol.CIStateFailed) {
		t.Fatalf("the run's status is %q, want failed", row.Status)
	}
}

func TestRunStateMapsTheForgesOwnWords(t *testing.T) {
	cases := []struct {
		status, conclusion string
		want               protocol.CIState
	}{
		{"queued", "", protocol.CIStateQueued},
		{"requested", "", protocol.CIStateQueued},
		{"waiting", "", protocol.CIStateQueued},
		{"pending", "", protocol.CIStateQueued},
		{"in_progress", "", protocol.CIStateRunning},
		{"completed", "success", protocol.CIStatePassed},
		{"completed", "failure", protocol.CIStateFailed},
		{"completed", "timed_out", protocol.CIStateFailed},
		{"completed", "action_required", protocol.CIStateFailed},
		{"completed", "startup_failure", protocol.CIStateFailed},
		// A check that did not run must never be read as green.
		{"completed", "skipped", protocol.CIStateCancelled},
		{"completed", "neutral", protocol.CIStateCancelled},
		{"completed", "cancelled", protocol.CIStateCancelled},
		{"completed", "", protocol.CIStateCancelled},
		{"", "", protocol.CIStateQueued},
	}
	for _, tc := range cases {
		t.Run(tc.status+"/"+tc.conclusion, func(t *testing.T) {
			f := newFixture(t)
			f.deliver(t, testRunID, tc.status, tc.conclusion)
			row := readRun(t, f.store, fmt.Sprint(testRunID))
			if row.Status != string(tc.want) {
				t.Fatalf("status %q conclusion %q became %q, want %q",
					tc.status, tc.conclusion, row.Status, tc.want)
			}
		})
	}
}

func TestThePollAsksAboutTheRunsMarshalIsWaitingOn(t *testing.T) {
	f := newFixture(t)
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStateRunning),
		Url: "https://example.test/run", StartedAt: testNow.Add(-20 * time.Minute).UnixMilli(),
		UpdatedAt: testNow.Add(-20 * time.Minute).UnixMilli(),
	})
	f.forge.runs = []gh.WorkflowRun{{
		ID: testRunID, Name: "ci", Branch: testBranch, Status: "completed", Conclusion: "success",
		URL: "https://example.test/run", StartedAt: testNow.Add(-19 * time.Minute).Format(time.RFC3339),
	}}
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(f.forge.listBrancs) != 1 || f.forge.listBrancs[0] != testBranch {
		t.Fatalf("the backup asked about %v, want [%s]", f.forge.listBrancs, testBranch)
	}
	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.Status != string(protocol.CIStatePassed) {
		t.Fatalf("the polled run is %q, want passed", row.Status)
	}
}

func TestThePollLeavesARunItIsNotWaitingOnAlone(t *testing.T) {
	f := newFixture(t)
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStatePassed),
		UpdatedAt: testNow.Add(-90 * time.Minute).UnixMilli(),
	})
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(f.forge.listBrancs) != 0 {
		t.Fatalf("the backup asked about a run that had already finished: %v", f.forge.listBrancs)
	}
}

func TestThePollLeavesAFreshRunAlone(t *testing.T) {
	f := newFixture(t)
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStateRunning),
		UpdatedAt: testNow.Add(-time.Minute).UnixMilli(),
	})
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(f.forge.listBrancs) != 0 {
		t.Fatalf("the backup asked about a run that is merely slow: %v", f.forge.listBrancs)
	}
}

func TestThePollStopsAskingAboutABranchWithNoRuns(t *testing.T) {
	f := newFixture(t)
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStateRunning),
		UpdatedAt: testNow.Add(-20 * time.Minute).UnixMilli(),
	})
	for i := 0; i < 3; i++ {
		if err := f.svc.Poll(context.Background()); err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
	}
	if len(f.forge.listBrancs) != 1 {
		t.Fatalf("the backup asked %d times about a branch with no runs, want 1", len(f.forge.listBrancs))
	}
}

func TestThePollAsksAgainOnceTheAnswerIsOld(t *testing.T) {
	now := testNow
	f := newFixture(t, func(o *ci.Options) { o.Now = func() time.Time { return now } })
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStateRunning),
		UpdatedAt: testNow.Add(-20 * time.Minute).UnixMilli(),
	})
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	now = now.Add(11 * time.Minute)
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if len(f.forge.listBrancs) != 2 {
		t.Fatalf("the backup asked %d times, want 2 once the answer went stale", len(f.forge.listBrancs))
	}
}

func TestPollWithoutAForgeSaysSo(t *testing.T) {
	f := newFixture(t, func(o *ci.Options) { o.Forge = nil })
	if err := f.svc.Poll(context.Background()); !errors.Is(err, ci.ErrNoForge) {
		t.Fatalf("poll answered %v, want ErrNoForge", err)
	}
}

func TestTheSnapshotGroupsRunsByProjectInProjectOrder(t *testing.T) {
	f := newFixture(t)
	seedProject(t, f.store, "mobile-app")
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: "1", ProjectID: "mobile-app", Branch: "main", Workflow: "ci",
		Status: string(protocol.CIStateFailed), UpdatedAt: testNow.UnixMilli(),
	})
	seedRun(t, f.store, db.CiRun{
		ID: "2", ProjectID: testProjectID, CardID: &cardID, Branch: testBranch,
		Workflow: "ci", Status: string(protocol.CIStateRunning), UpdatedAt: testNow.UnixMilli(),
	})
	f.projects.projects = []protocol.Project{
		{ID: testProjectID, Name: testProjectID, DefaultBranch: "main"},
		{ID: "mobile-app", Name: "mobile-app", DefaultBranch: "main"},
	}
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if len(snapshot.Projects) != 2 {
		t.Fatalf("the snapshot has %d projects, want 2", len(snapshot.Projects))
	}
	if snapshot.Projects[0].ProjectID != testProjectID || snapshot.Projects[1].ProjectID != "mobile-app" {
		t.Fatalf("the projects are %s then %s, want the project order",
			snapshot.Projects[0].ProjectID, snapshot.Projects[1].ProjectID)
	}
	if snapshot.Projects[0].Status != protocol.CIStateRunning {
		t.Fatalf("the first project's status is %q, want running", snapshot.Projects[0].Status)
	}
	if len(snapshot.Projects[0].Runs) != 1 || snapshot.Projects[0].Runs[0].StartedAt != nil {
		t.Fatalf("a queued run's start time should be null: %+v", snapshot.Projects[0].Runs)
	}
}

func TestAProjectWithNoRunsIsLeftOutOfTheSnapshot(t *testing.T) {
	f := newFixture(t)
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if len(snapshot.Projects) != 0 {
		t.Fatalf("a project with no runs appeared as %+v", snapshot.Projects)
	}
}

func TestStartedAtIsNullWhileARunIsQueued(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, testRunID, "queued", "")
	project, err := f.svc.RunsForProject(context.Background(), testProjectID)
	if err != nil {
		t.Fatalf("read a project's CI health: %v", err)
	}
	if len(project.Runs) != 1 {
		t.Fatalf("the project has %d runs, want 1", len(project.Runs))
	}
	if project.Runs[0].StartedAt != nil {
		t.Fatalf("a queued run's startedAt is %v, want null", project.Runs[0].StartedAt)
	}
	if project.Status != protocol.CIStateQueued {
		t.Fatalf("the project's status is %q, want queued", project.Status)
	}
}

func TestARunWithoutABranchIsRefusedNotRecorded(t *testing.T) {
	f := newFixture(t)
	body := []byte(`{"action":"completed","workflow_run":{"id":42,"status":"queued"},
		"repository":{"full_name":"khanblair/marshal","name":"marshal","owner":{"login":"khanblair"}}}`)
	if err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "workflow_run", Body: body,
	}); err != nil {
		t.Fatalf("a delivery with no branch must be accepted and ignored: %v", err)
	}
	if len(f.bus.topics) != 0 {
		t.Fatalf("a run with no branch was published: %v", f.bus.topics)
	}
}
