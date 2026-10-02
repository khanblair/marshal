package integrator_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// multiLine is a file long enough that two edits far apart merge without a conflict.
const multiLine = "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n"

// env is a merge queue over a real store, a real event bus, the real projects module, and a real
// repository in a temporary folder: the owner's folder, on main, with a.txt, b.txt, and c.txt.
type env struct {
	t        *testing.T
	ctx      context.Context
	store    *store.Store
	bus      *events.Bus
	proj     *projects.Service
	git      *gitx.Git
	dataDir  string
	repo     string
	project  protocol.Project
	svc      *integrator.Service
	sub      *events.Subscription
	tests    *stubTests
	resolver integrator.ConflictResolver
	tweak    func(*integrator.Deps)
}

// option changes how an env is built.
type option func(*env)

// withResolver gives the queue a resolver.
func withResolver(r integrator.ConflictResolver) option { return func(e *env) { e.resolver = r } }

// withTester makes the queue run tests that answer as the stub says.
func withTester(t *stubTests) option { return func(e *env) { e.tests = t } }

// withDeps changes the queue's dependencies before it is built.
func withDeps(fn func(*integrator.Deps)) option { return func(e *env) { e.tweak = fn } }

func newEnv(t *testing.T, opts ...option) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	bus, err := events.New(events.WithEpoch("test-epoch"))
	if err != nil {
		t.Fatalf("make the bus: %v", err)
	}
	t.Cleanup(bus.Close)
	g := testutil.Git()
	dataDir := filepath.Join(dir, "data")
	proj, err := projects.New(projects.Deps{Store: st, Bus: bus, Git: g, DataDir: dataDir})
	if err != nil {
		t.Fatalf("make the projects module: %v", err)
	}
	e := &env{t: t, ctx: ctx, store: st, bus: bus, proj: proj, git: g, dataDir: dataDir}
	e.repo = filepath.Join(dir, "owner")
	e.makeRepo()
	e.project, err = proj.Create(ctx, protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: e.repo})
	if err != nil {
		t.Fatalf("add the project: %v", err)
	}
	for _, opt := range opts {
		opt(e)
	}
	e.sub = bus.Subscribe(events.AllTopics())
	t.Cleanup(e.sub.Close)
	e.build()
	return e
}

// build makes the merge queue from the env's parts.
func (e *env) build() {
	e.t.Helper()
	deps := integrator.Deps{
		Cards: e.proj, Projects: e.proj, Git: e.git, DataDir: e.dataDir,
		Ledger: integrator.NewLedger(e.store), Events: e.bus, Resolver: e.resolver,
		Folder: integrator.FolderRetry{Attempts: 2, Wait: time.Millisecond},
	}
	if e.tests != nil {
		deps.Tests = e.tests
	}
	if e.tweak != nil {
		e.tweak(&deps)
	}
	svc, err := integrator.New(deps)
	if err != nil {
		e.t.Fatalf("make the merge queue: %v", err)
	}
	e.svc = svc
	e.t.Cleanup(svc.Close)
}

// makeRepo makes the owner's folder: a repository on main with three files.
func (e *env) makeRepo() {
	e.t.Helper()
	if err := os.MkdirAll(e.repo, 0o755); err != nil {
		e.t.Fatal(err)
	}
	e.run(e.repo, "init", "--quiet", "--initial-branch=main")
	e.write(e.repo, "a.txt", multiLine)
	e.write(e.repo, "b.txt", "b base\n")
	e.write(e.repo, "c.txt", "c base\n")
	e.run(e.repo, "add", "--all")
	e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "Base")
}

// run runs a Git command that must succeed.
func (e *env) run(dir string, args ...string) string {
	e.t.Helper()
	out, err := e.git.Run(e.ctx, dir, args...)
	if err != nil {
		e.t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func (e *env) write(dir, name, content string) {
	e.t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) read(dir, name string) string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		e.t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// tip is the commit a branch is at.
func (e *env) tip(branch string) string {
	e.t.Helper()
	return e.run(e.repo, "rev-parse", branch)
}

// commitOn makes a commit on a branch without touching the owner's folder, by way of a temporary
// worktree, and answers the commit. The branch is made from base when it does not exist.
func (e *env) commitOn(branch, base string, files map[string]string) string {
	e.t.Helper()
	if _, err := e.git.Run(e.ctx, e.repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		e.run(e.repo, "branch", "--no-track", branch, base)
	}
	wt := filepath.Join(e.t.TempDir(), "wt")
	e.run(e.repo, "worktree", "add", "--quiet", wt, branch)
	for name, content := range files {
		e.write(wt, name, content)
	}
	e.run(wt, "add", "--all")
	e.run(wt, "-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "Work on "+branch)
	sha := e.run(wt, "rev-parse", "HEAD")
	e.run(e.repo, "worktree", "remove", "--force", wt)
	return sha
}

// card makes a card with its own branch, off main, holding the given files, and moves it to Ready to
// merge. Nothing merges it until the test says so.
func (e *env) card(title string, files map[string]string) protocol.Card {
	e.t.Helper()
	card, err := e.proj.CreateCard(e.ctx, e.project.ID, protocol.CreateCardRequest{Title: title})
	if err != nil {
		e.t.Fatalf("make a card: %v", err)
	}
	branch := "marshal/" + strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	e.commitOn(branch, e.project.Target(), files)
	if _, err := e.proj.SetWorktree(e.ctx, card.ID, "", branch); err != nil {
		e.t.Fatalf("record the card's branch: %v", err)
	}
	return e.ready(card.ID)
}

// ready moves a card to Ready to merge.
func (e *env) ready(id string) protocol.Card {
	e.t.Helper()
	card, err := e.proj.SetState(e.ctx, id, protocol.CardStateReady)
	if err != nil {
		e.t.Fatalf("move a card to ready: %v", err)
	}
	return card
}

// merge runs a card's merge and answers the result.
func (e *env) merge(card protocol.Card) integrator.Result {
	e.t.Helper()
	res, err := e.svc.Merge(e.ctx, card.ID)
	if err != nil {
		e.t.Fatalf("Merge: %v", err)
	}
	return res
}

// state reads a card's state.
func (e *env) state(id string) protocol.Card {
	e.t.Helper()
	card, err := e.proj.Card(e.ctx, id)
	if err != nil {
		e.t.Fatalf("read a card: %v", err)
	}
	return card
}

// mergeColumns reads the merge phase and note stored on a card.
func (e *env) mergeColumns(id string) (phase, note string) {
	e.t.Helper()
	row, err := e.store.Queries().GetCard(e.ctx, id)
	if err != nil {
		e.t.Fatalf("read a card row: %v", err)
	}
	return row.MergePhase, row.MergeNote
}

// useBranch makes a branch of its own, at main, the project's integration branch.
func (e *env) useBranch(name string) {
	e.t.Helper()
	e.run(e.repo, "branch", "--no-track", name, "main")
	e.chooseBranch(name)
}

// chooseBranch makes an existing branch the project's integration branch.
func (e *env) chooseBranch(name string) {
	e.t.Helper()
	project, err := e.proj.Update(e.ctx, e.project.ID, projects.UpdateInput{IntegrationBranch: &name})
	if err != nil {
		e.t.Fatalf("choose the integration branch: %v", err)
	}
	e.project = project
}

// ownerCommits makes a commit in the owner's folder, on the branch it is on.
func (e *env) ownerCommits(file, content string) {
	e.t.Helper()
	e.write(e.repo, file, content)
	e.run(e.repo, "add", "--all")
	e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "Owner edits "+file)
}

// folderSnapshot records everything about the owner's folder a delivery must not change when it is
// refused: every file, the status, and the diff.
func (e *env) folderSnapshot() string {
	e.t.Helper()
	var parts []string
	err := filepath.WalkDir(e.repo, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, _ := filepath.Rel(e.repo, path)
		parts = append(parts, rel+"="+string(data))
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
	parts = append(parts, e.status(), e.run(e.repo, "diff", "HEAD"), e.run(e.repo, "rev-parse", "HEAD"))
	return strings.Join(parts, "\x00")
}

// assertStopped checks everything a merge that stopped must leave: the card in Needs you, the phase
// "stopped" and the reason as the merge note, on the stored card and on the card the module sends, a
// reason that says it is the merge, and no place in the queue.
func (e *env) assertStopped(t *testing.T, card protocol.Card, result integrator.Result) {
	t.Helper()
	if result.Merged || result.Reason == "" {
		t.Fatalf("result = %+v, want a merge that stopped with a reason", result)
	}
	got := e.state(card.ID)
	if got.State != protocol.CardStateNeeds {
		t.Errorf("card state = %s, want needs", got.State)
	}
	phase, note := e.mergeColumns(card.ID)
	if phase != string(protocol.MergePhaseStopped) || note != result.Reason {
		t.Errorf("merge columns = %q, %q; want %q and the reason %q", phase, note, protocol.MergePhaseStopped, result.Reason)
	}
	if got.MergePhase != protocol.MergePhaseStopped || got.MergeNote != result.Reason {
		t.Errorf("card mergePhase = %q, mergeNote = %q; want the same on the wire card", got.MergePhase, got.MergeNote)
	}
	if !strings.Contains(strings.ToLower(result.Reason), "merge") {
		t.Errorf("reason = %q, want it to say it is the merge that stopped", result.Reason)
	}
	for _, item := range e.integration().Queue {
		if item.CardID == card.ID {
			t.Errorf("a stopped card is in the queue: %+v", item)
		}
	}
}

// status is the folder's `git status --porcelain`.
func (e *env) status() string { return e.run(e.repo, "status", "--porcelain") }

// integration reads the queue's state for the project.
func (e *env) integration() protocol.IntegrationState {
	e.t.Helper()
	state, err := e.svc.State(e.ctx, e.project.ID)
	if err != nil {
		e.t.Fatalf("State: %v", err)
	}
	return state
}

// progress reads the merge.progress events published so far, by topic, in order, and waits a moment
// for ones still on their way.
func (e *env) progress() map[string][]protocol.MergePhase {
	e.t.Helper()
	out := map[string][]protocol.MergePhase{}
	for {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				return out
			}
			if ev.Type != string(protocol.EventTypeMergeProgress) {
				continue
			}
			data, ok := ev.Data.(protocol.MergeProgressEvent)
			if !ok {
				e.t.Fatalf("a merge.progress event carries %T", ev.Data)
			}
			out[ev.Topic] = append(out[ev.Topic], data.Phase)
		case <-time.After(100 * time.Millisecond):
			return out
		}
	}
}

// stubTests is a Tester that answers as the test says and records how it was called.
type stubTests struct {
	mu      sync.Mutex
	outcome integrator.Outcome
	err     error
	// onRun runs before each answer, with the number of the call from 1.
	onRun func(call int)
	calls int
	files [][]string
}

func passing() *stubTests { return &stubTests{outcome: integrator.Outcome{Passed: true}} }

func failing(summary string) *stubTests {
	return &stubTests{outcome: integrator.Outcome{Passed: false, Summary: summary}}
}

func (s *stubTests) Run(_ context.Context, _ string, changed []string) (integrator.Outcome, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.files = append(s.files, changed)
	hook, outcome, err := s.onRun, s.outcome, s.err
	s.mu.Unlock()
	if hook != nil {
		hook(call)
	}
	return outcome, err
}

func (s *stubTests) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// resolverFunc is a ConflictResolver made of a function. It records every task it is given.
type resolverFunc struct {
	mu    sync.Mutex
	fn    func(task integrator.MergeTask) (integrator.Verdict, error)
	tasks []integrator.MergeTask
}

func (r *resolverFunc) Resolve(_ context.Context, task integrator.MergeTask) (integrator.Verdict, error) {
	r.mu.Lock()
	r.tasks = append(r.tasks, task)
	r.mu.Unlock()
	return r.fn(task)
}

func (r *resolverFunc) taskList() []integrator.MergeTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]integrator.MergeTask{}, r.tasks...)
}

// keepBoth is a resolver that keeps both sides of every conflict it is given, ours first. A file with
// no markers, such as one a modify/delete conflict leaves, is kept as it is.
func keepBoth(summary string) *resolverFunc {
	return &resolverFunc{fn: func(task integrator.MergeTask) (integrator.Verdict, error) {
		for _, file := range task.Conflicts {
			path := filepath.Join(task.Worktree, file)
			data, err := os.ReadFile(path)
			if err != nil {
				return integrator.Verdict{}, err
			}
			if err := os.WriteFile(path, []byte(unionOf(string(data))), 0o600); err != nil {
				return integrator.Verdict{}, err
			}
		}
		return integrator.Verdict{Resolved: true, Confident: true, Summary: summary, Files: task.Conflicts}, nil
	}}
}

// unionOf drops the conflict markers of a file and keeps the lines of both sides.
func unionOf(text string) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "<<<<<<<"), strings.HasPrefix(line, "======="), strings.HasPrefix(line, ">>>>>>>"):
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// answering is a resolver that returns the verdict without touching a file.
func answering(v integrator.Verdict, err error) *resolverFunc {
	return &resolverFunc{fn: func(integrator.MergeTask) (integrator.Verdict, error) { return v, err }}
}

// refusedWith reports whether err is a refusal with the given reason.
func refusedWith(err error, reason string) bool {
	var refusal *protocol.Error
	return errors.As(err, &refusal) && refusal.Code == protocol.ErrorCodeRefused && refusal.Details["reason"] == reason
}
