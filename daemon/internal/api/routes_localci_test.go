package api_test

// The local-CI route (docs/architecture.md sections 9 and 11.1, docs/backend-checklist.md B6.5,
// build-plan 6.5, docs/marshal-product-scope.md 15.3). internal/localci's own tests drive the reader,
// the classifier, and the engine over fixture files and a fake runner; this file tests the wire - the
// request names one workflow or none, a card's worktree is what is read, a step Marshal cannot run is
// reported rather than failing the run, and a card with no worktree or a name that matches nothing is
// refused in a plain sentence. The command runner is a fake, so no step ever starts a process.

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeStepRunner stands in for the command runner behind the local-CI route, so the wire is proven
// without starting a process. It answers a canned result per command and remembers what it was
// asked, so the test can prove the worktree and the workflow's environment reached it.
type fakeStepRunner struct {
	mu      sync.Mutex
	results map[string]localci.RunResult
	seen    []localci.RunRequest
}

func (f *fakeStepRunner) Run(_ context.Context, req localci.RunRequest) (localci.RunResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, req)
	if result, ok := f.results[req.Command]; ok {
		return result, nil
	}
	return localci.RunResult{}, nil
}

func (f *fakeStepRunner) ran() []localci.RunRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]localci.RunRequest(nil), f.seen...)
}

// The fixture workflow has every shape the wire carries: an action step Marshal will not run, a test
// and a lint step it will, and a job on a runner it refuses whole, so its step is reported with the
// job's reason rather than a step's.
const fixtureWorkflow = `name: CI
on:
  push:
    branches: [main]
env:
  NODE_ENV: test
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install and test
        run: pnpm test
      - name: Typecheck
        run: pnpm exec tsc --noEmit
  release:
    runs-on: windows-latest
    steps:
      - name: Build
        run: npm run build
`

// A run answers every step of the card's workflow files, in file and step order, with what Marshal
// ran, what it would not, and why, and the workflow's environment reaches the command it runs.
func TestRunningACardsWorkflowsLocallyThroughHTTP(t *testing.T) {
	runner := &fakeStepRunner{results: map[string]localci.RunResult{
		"pnpm test":              {Output: "all good", Took: 1500 * time.Millisecond},
		"pnpm exec tsc --noEmit": {Output: "no types wrong", Took: 800 * time.Millisecond},
	}}
	st := newStack(t, withLocalCIRunner(runner))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Fix the report")
	dir := st.startWorktree(t, project, repo, card)
	edit(t, dir, ".github/workflows/ci.yml", fixtureWorkflow)

	r := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/local-ci", protocol.LocalCIRequest{}).want(t, http.StatusOK)
	sameShape(t, "local-ci-result", r.Body)
	got := decode[protocol.LocalCIResult](t, r)

	if got.CardID != card.ID || got.Branch == "" {
		t.Errorf("the answer = %+v, want the card and the branch its worktree is on", got)
	}
	if got.ServerTime.Time().IsZero() {
		t.Error("the answer carries no server time")
	}
	if len(got.Workflows) != 1 {
		t.Fatalf("the answer = %+v, want one workflow", got.Workflows)
	}
	w := got.Workflows[0]
	if w.File != ".github/workflows/ci.yml" || w.Name != "CI" {
		t.Errorf("the workflow = %+v, want the file's path and its own name", w)
	}
	if w.Status != protocol.LocalCIStatusPassed {
		t.Errorf("the workflow's status = %q, want passed (nothing Marshal ran failed)", w.Status)
	}
	if len(w.Steps) != 4 {
		t.Fatalf("the steps = %+v, want every step of the file", w.Steps)
	}
	for i, want := range []string{"test", "test", "test", "release"} {
		if w.Steps[i].Job != want {
			t.Errorf("step %d is in job %q, want %q", i, w.Steps[i].Job, want)
		}
	}

	// A step that runs an action is reported with its sentence and never run.
	action := w.Steps[0]
	if action.Name != "actions/checkout@v4" || action.Kind != protocol.LocalCIKindOther {
		t.Errorf("the action step = %+v, want the action's name and the other kind", action)
	}
	if action.Status != protocol.LocalCIStatusUnsupported || action.Reason == "" {
		t.Errorf("the action step = %+v, want it unsupported with a reason", action)
	}
	if action.Command != "" || action.Output != "" || action.TookMs != 0 {
		t.Errorf("the action step = %+v, want nothing run for it", action)
	}

	// A test step runs, and the workflow's own environment goes with it, as the job's and the step's
	// would.
	test := w.Steps[1]
	if test.Name != "Install and test" || test.Kind != protocol.LocalCIKindTest {
		t.Errorf("the test step = %+v, want it named and a test", test)
	}
	if test.Status != protocol.LocalCIStatusPassed || test.Output != "all good" || test.TookMs != 1500 {
		t.Errorf("the test step = %+v, want it passed with what it printed", test)
	}
	if test.Reason != "" {
		t.Errorf("a step that ran answers no reason, got %q", test.Reason)
	}
	// A lint step is run too, which is the second of the two kinds the product scope promises.
	if lint := w.Steps[2]; lint.Kind != protocol.LocalCIKindLint || lint.Status != protocol.LocalCIStatusPassed {
		t.Errorf("the lint step = %+v, want a lint step that ran", lint)
	}

	// A job on a runner Marshal will not stand in for reports every step of it with the job's reason.
	release := w.Steps[3]
	if release.Status != protocol.LocalCIStatusUnsupported || !strings.Contains(release.Reason, "Windows") {
		t.Errorf("the release step = %+v, want it reported for the job's runner", release)
	}
	if release.Output != "" {
		t.Errorf("the release step printed %q; nothing was run for it", release.Output)
	}

	// The two runnable steps reached the runner, in the worktree, with the workflow's environment.
	ran := runner.ran()
	if len(ran) != 2 {
		t.Fatalf("the runner was asked %d times, want 2; it saw %+v", len(ran), ran)
	}
	if ran[0].Command != "pnpm test" || ran[1].Command != "pnpm exec tsc --noEmit" {
		t.Errorf("the runner was asked for %q then %q", ran[0].Command, ran[1].Command)
	}
	if ran[0].Dir != dir {
		t.Errorf("the runner ran in %q, want the card's worktree %q", ran[0].Dir, dir)
	}
	if !hasEnv(ran[0].Env, "NODE_ENV=test") {
		t.Errorf("the runner's environment = %v, want the workflow's own NODE_ENV=test", ran[0].Env)
	}
}

// A failing step fails the workflow, and the rest of that job's steps are reported as not reached,
// while the workflow's other jobs keep going - exactly as GitHub runs them.
func TestAFailingStepStopsItsJobButNotTheOtherJobs(t *testing.T) {
	runner := &fakeStepRunner{results: map[string]localci.RunResult{
		"pnpm test": {Output: "1 test failed", Failed: true, Took: 900 * time.Millisecond},
	}}
	st := newStack(t, withLocalCIRunner(runner))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "A failing check")
	dir := st.startWorktree(t, project, repo, card)
	edit(t, dir, ".github/workflows/ci.yml", `name: CI
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Install and test
        run: pnpm test
      - name: Typecheck
        run: pnpm exec tsc --noEmit
  lint:
    runs-on: ubuntu-latest
    steps:
      - name: Lint the sources
        run: pnpm lint
`)

	got := decode[protocol.LocalCIResult](t, st.do(http.MethodPost,
		"/v1/cards/"+card.ID+"/local-ci", protocol.LocalCIRequest{}).want(t, http.StatusOK))
	if len(got.Workflows) != 1 || len(got.Workflows[0].Steps) != 3 {
		t.Fatalf("the answer = %+v, want three steps", got.Workflows)
	}
	if got.Workflows[0].Status != protocol.LocalCIStatusFailed {
		t.Errorf("the workflow's status = %q, want failed", got.Workflows[0].Status)
	}
	failed := got.Workflows[0].Steps[0]
	if failed.Status != protocol.LocalCIStatusFailed || failed.Output != "1 test failed" {
		t.Errorf("the test step = %+v, want it failed with what it printed", failed)
	}
	skipped := got.Workflows[0].Steps[1]
	if skipped.Status != protocol.LocalCIStatusSkipped || !strings.Contains(skipped.Reason, "earlier step") {
		t.Errorf("the step after the failure = %+v, want it skipped for the earlier failure", skipped)
	}
	// The other job's step runs: its job does not depend on the one that failed.
	other := got.Workflows[0].Steps[2]
	if other.Job != "lint" || other.Status != protocol.LocalCIStatusPassed {
		t.Errorf("the other job's step = %+v, want it run", other)
	}
}

// A request that names one workflow runs only that one, which is how a person re-runs a single check
// without running the whole set.
func TestLocalCIRunsOnlyTheNamedWorkflow(t *testing.T) {
	runner := &fakeStepRunner{}
	st := newStack(t, withLocalCIRunner(runner))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Pick a workflow")
	dir := st.startWorktree(t, project, repo, card)
	edit(t, dir, ".github/workflows/ci.yml", "name: CI\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Test\n        run: pnpm test\n")
	edit(t, dir, ".github/workflows/nightly.yml", "name: Nightly\njobs:\n  check:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Nightly test\n        run: pnpm test-nightly\n")

	got := decode[protocol.LocalCIResult](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/local-ci",
		protocol.LocalCIRequest{Workflow: "nightly.yml"}).want(t, http.StatusOK))
	if len(got.Workflows) != 1 || got.Workflows[0].Name != "Nightly" {
		t.Fatalf("the answer = %+v, want only the named workflow", got.Workflows)
	}
	if ran := runner.ran(); len(ran) != 1 || ran[0].Command != "pnpm test-nightly" {
		t.Errorf("the runner saw %+v, want only the named workflow's step", ran)
	}
}

// A name that matches nothing is refused rather than answered with an empty run: the person asked
// for something specific and must be told it is not there.
func TestLocalCIRefusesAWorkflowNameThatIsNotThere(t *testing.T) {
	st := newStack(t, withLocalCIRunner(&fakeStepRunner{}))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Wrong name")
	dir := st.startWorktree(t, project, repo, card)
	edit(t, dir, ".github/workflows/ci.yml", "name: CI\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Test\n        run: pnpm test\n")

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/local-ci",
		protocol.LocalCIRequest{Workflow: "deploy.yml"}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
}

// A card that has not started has no worktree, so there is nothing to read and nothing to run in.
func TestLocalCIRefusesACardWithNoWorktree(t *testing.T) {
	st := newStack(t, withLocalCIRunner(&fakeStepRunner{}))
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Not started yet")

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/local-ci", protocol.LocalCIRequest{}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
}

// A worktree with no workflow files is not an error: it answers an empty list, and the bytes are []
// so a screen draws "no checks here" without a special case for a missing list.
func TestACardWithNoWorkflowFilesAnswersNothingToRun(t *testing.T) {
	st := newStack(t, withLocalCIRunner(&fakeStepRunner{}))
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "No workflows")
	st.startWorktree(t, project, repo, card)

	r := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/local-ci", protocol.LocalCIRequest{}).want(t, http.StatusOK)
	got := decode[protocol.LocalCIResult](t, r)
	if len(got.Workflows) != 0 {
		t.Errorf("the answer = %+v, want no workflows", got.Workflows)
	}
	if body := string(r.Body); !strings.Contains(body, `"workflows":[]`) {
		t.Errorf("the answer = %s, want an empty workflows array", body)
	}
}

// hasEnv reports whether a KEY=value pair is in a command's environment.
func hasEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
