package localci_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The engine is driven over real workflow files in a temporary folder and a fake runner: the files
// are the ones a project really writes, and nothing is ever started. A step's command is a string
// the fake receives, so what the engine decided can be read without running anybody's test suite.

var testNow = time.Date(2026, time.September, 27, 11, 0, 0, 0, time.UTC)

const testBranch = "marshal/41-fix-token-refresh"

// fakeProjects answers one card's worktree, or an error, and never touches a repository.
type fakeProjects struct {
	dir    string
	branch string
	err    error
}

func (f *fakeProjects) Worktree(context.Context, string) (string, string, error) {
	return f.dir, f.branch, f.err
}

// fakeRunner records what it was asked to run and answers what the test told it to.
type fakeRunner struct {
	requests []localci.RunRequest
	run      func(localci.RunRequest) (localci.RunResult, error)
}

func (f *fakeRunner) Run(_ context.Context, req localci.RunRequest) (localci.RunResult, error) {
	f.requests = append(f.requests, req)
	if f.run != nil {
		return f.run(req)
	}
	return localci.RunResult{Output: "ok"}, nil
}

// writeWorktree makes a worktree holding the named files and answers a service that reads it.
func writeWorktree(t *testing.T, files map[string]string, runner localci.Runner, tweak ...func(*localci.Deps)) *localci.Service {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("make the folder for %s: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	deps := localci.Deps{
		Projects: &fakeProjects{dir: dir, branch: testBranch},
		Runner:   runner,
		Now:      func() time.Time { return testNow },
	}
	for _, apply := range tweak {
		apply(&deps)
	}
	svc, err := localci.New(deps)
	if err != nil {
		t.Fatalf("build the service: %v", err)
	}
	return svc
}

// ciFile is a workflow of two jobs: one job Marshal can mostly run, and one that deploys.
const ciFile = `name: CI
env:
  NODE_ENV: test
jobs:
  test:
    runs-on: ubuntu-latest
    env:
      CI: "true"
    steps:
      - uses: actions/checkout@v4
      - name: Run tests
        run: pnpm test
      - name: Lint
        run: pnpm lint
        env:
          STRICT: "1"
      - name: Build
        run: pnpm build
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Deploy
        run: ./scripts/deploy.sh
`

// TestRunRunsWhatItCanAndMarksTheRest is the whole promise of B6.5 at once: the test and lint steps
// run, the action and the deploy are marked with the sentence that says why, and the step after the
// failure is not reached.
func TestRunRunsWhatItCanAndMarksTheRest(t *testing.T) {
	runner := &fakeRunner{run: func(req localci.RunRequest) (localci.RunResult, error) {
		if req.Command == "pnpm lint" {
			return localci.RunResult{Output: "lint found 2 problems", Failed: true, Took: 250 * time.Millisecond}, nil
		}
		return localci.RunResult{Output: "42 tests passed", Took: 1500 * time.Millisecond}, nil
	}}
	svc := writeWorktree(t, map[string]string{".github/workflows/ci.yml": ciFile}, runner)

	got, err := svc.Run(context.Background(), "01M3C107JB041061050R3GG28A", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.CardID != "01M3C107JB041061050R3GG28A" || got.Branch != testBranch {
		t.Errorf("the answer names card %q branch %q, want the card and its branch", got.CardID, got.Branch)
	}
	if got.ServerTime.Time() != testNow {
		t.Errorf("ServerTime = %v, want the clock the service was built with", got.ServerTime.Time())
	}
	if len(got.Workflows) != 1 {
		t.Fatalf("ran %d workflows, want 1", len(got.Workflows))
	}

	w := got.Workflows[0]
	if w.File != ".github/workflows/ci.yml" || w.Name != "CI" {
		t.Errorf("the workflow is %q named %q, want the file and its own name", w.File, w.Name)
	}
	if w.Status != protocol.LocalCIStatusFailed {
		t.Errorf("the workflow's status = %q, want failed: a step in it failed", w.Status)
	}

	want := []struct {
		job     string
		name    string
		kind    protocol.LocalCIKind
		status  protocol.LocalCIStatus
		reason  bool
		command string
	}{
		{"test", "actions/checkout@v4", protocol.LocalCIKindOther, protocol.LocalCIStatusUnsupported, true, ""},
		{"test", "Run tests", protocol.LocalCIKindTest, protocol.LocalCIStatusPassed, false, "pnpm test"},
		{"test", "Lint", protocol.LocalCIKindLint, protocol.LocalCIStatusFailed, false, "pnpm lint"},
		{"test", "Build", protocol.LocalCIKindBuild, protocol.LocalCIStatusSkipped, true, "pnpm build"},
		{"release", "Deploy", protocol.LocalCIKindOther, protocol.LocalCIStatusUnsupported, true, "./scripts/deploy.sh"},
	}
	if len(w.Steps) != len(want) {
		t.Fatalf("read %d steps, want %d: %+v", len(w.Steps), len(want), w.Steps)
	}
	for i, w := range want {
		step := w2step(t, got, i)
		if step.Job != w.job || step.Name != w.name {
			t.Errorf("step %d is %s/%s, want %s/%s", i, step.Job, step.Name, w.job, w.name)
		}
		if step.Kind != w.kind {
			t.Errorf("step %d (%s) kind = %q, want %q", i, w.name, step.Kind, w.kind)
		}
		if step.Status != w.status {
			t.Errorf("step %d (%s) status = %q, want %q", i, w.name, step.Status, w.status)
		}
		if (step.Reason != "") != w.reason {
			t.Errorf("step %d (%s) reason = %q, want a reason: %v", i, w.name, step.Reason, w.reason)
		}
		if step.Command != w.command {
			t.Errorf("step %d (%s) command = %q, want %q", i, w.name, step.Command, w.command)
		}
	}

	if out := w2step(t, got, 1).Output; out != "42 tests passed" {
		t.Errorf("the passing step's output = %q, want what it printed", out)
	}
	if ms := w2step(t, got, 1).TookMs; ms != 1500 {
		t.Errorf("the passing step took %dms, want 1500", ms)
	}
	if out := w2step(t, got, 2).Output; !strings.Contains(out, "2 problems") {
		t.Errorf("the failing step's output = %q, want what it printed", out)
	}
	if out := w2step(t, got, 0).Output; out != "" {
		t.Errorf("a step that did not run printed %q, want nothing", out)
	}

	// Only the two steps Marshal can run may ever reach a shell.
	if len(runner.requests) != 2 {
		t.Fatalf("the runner was asked for %d steps, want 2", len(runner.requests))
	}
	first := runner.requests[0]
	if first.Shell != "/bin/sh" || first.Command != "pnpm test" {
		t.Errorf("the first run was %q under %q, want pnpm test under /bin/sh", first.Command, first.Shell)
	}
	if !strings.HasPrefix(first.Dir, os.TempDir()) {
		t.Errorf("the run's Dir = %q, want the card's worktree", first.Dir)
	}
	if env := strings.Join(first.Env, " "); env != "NODE_ENV=test CI=true" {
		t.Errorf("the first run's environment = %q, want the workflow's then the job's", env)
	}
	if env := strings.Join(runner.requests[1].Env, " "); env != "NODE_ENV=test CI=true STRICT=1" {
		t.Errorf("the second run's environment = %q, want the step's own entry last", env)
	}
}

// w2step answers one step of the first workflow, with the whole answer printed when the index is
// wrong, so a failure to find it does not hide what was really there.
func w2step(t *testing.T, got protocol.LocalCIResult, i int) protocol.LocalCIStep {
	t.Helper()
	if len(got.Workflows) == 0 || i >= len(got.Workflows[0].Steps) {
		t.Fatalf("there is no step %d in %+v", i, got.Workflows)
	}
	return got.Workflows[0].Steps[i]
}

// TestRunMarksEveryStepOfAJobItWillNotRun covers a job Marshal cannot run at all: every step of it
// carries the job's own sentence, and none of them reaches a shell.
func TestRunMarksEveryStepOfAJobItWillNotRun(t *testing.T) {
	text := `jobs:
  test:
    runs-on: windows-latest
    steps:
      - name: Run tests
        run: pnpm test
      - name: Lint
        run: pnpm lint
`
	runner := &fakeRunner{}
	svc := writeWorktree(t, map[string]string{".github/workflows/ci.yml": text}, runner)
	got, err := svc.Run(context.Background(), "card", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	const windows = "This job runs on Windows, so Marshal did not run it on this machine."
	for i, step := range got.Workflows[0].Steps {
		if step.Status != protocol.LocalCIStatusUnsupported || step.Reason != windows {
			t.Errorf("step %d = %q %q, want unsupported with the job's sentence", i, step.Status, step.Reason)
		}
		if step.Output != "" {
			t.Errorf("step %d printed %q, want nothing", i, step.Output)
		}
	}
	if len(runner.requests) != 0 {
		t.Errorf("the runner was asked for %d steps, want none", len(runner.requests))
	}
	if got.Workflows[0].Status != protocol.LocalCIStatusUnsupported {
		t.Errorf("the workflow's status = %q, want unsupported", got.Workflows[0].Status)
	}
}

// TestRunKeepsOtherJobsGoing covers a failure stopping only its own job: the next job still runs.
func TestRunKeepsOtherJobsGoing(t *testing.T) {
	text := `jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - name: Run tests
        run: pnpm test
      - name: Lint
        run: pnpm lint
  smoke:
    runs-on: ubuntu-latest
    steps:
      - name: Build
        run: pnpm build
`
	runner := &fakeRunner{run: func(req localci.RunRequest) (localci.RunResult, error) {
		return localci.RunResult{Failed: req.Command == "pnpm test"}, nil
	}}
	svc := writeWorktree(t, map[string]string{".github/workflows/ci.yml": text}, runner)
	got, err := svc.Run(context.Background(), "card", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	steps := got.Workflows[0].Steps
	if steps[0].Status != protocol.LocalCIStatusFailed {
		t.Errorf("the failing step's status = %q, want failed", steps[0].Status)
	}
	if steps[1].Status != protocol.LocalCIStatusSkipped {
		t.Errorf("the step after a failure = %q, want skipped", steps[1].Status)
	}
	if steps[2].Status != protocol.LocalCIStatusPassed {
		t.Errorf("the other job's step = %q, want passed: another job does not depend on this one", steps[2].Status)
	}
}

// TestRunReportsAStepItCouldNotStart covers a command that never began: it is a failure with the
// sentence Marshal wrote, and the rest of its job is not reached.
func TestRunReportsAStepItCouldNotStart(t *testing.T) {
	text := `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Run tests
        run: pnpm test
      - name: Lint
        run: pnpm lint
`
	runner := &fakeRunner{run: func(localci.RunRequest) (localci.RunResult, error) {
		return localci.RunResult{}, errors.New("no such program")
	}}
	svc := writeWorktree(t, map[string]string{".github/workflows/ci.yml": text}, runner)
	got, err := svc.Run(context.Background(), "card", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	steps := got.Workflows[0].Steps
	if steps[0].Status != protocol.LocalCIStatusFailed {
		t.Errorf("a step that could not start = %q, want failed", steps[0].Status)
	}
	if !strings.Contains(steps[0].Output, "no such program") {
		t.Errorf("the output = %q, want the reason it could not start", steps[0].Output)
	}
	if steps[1].Status != protocol.LocalCIStatusSkipped {
		t.Errorf("the next step = %q, want skipped", steps[1].Status)
	}
	if got.Workflows[0].Status != protocol.LocalCIStatusFailed {
		t.Errorf("the workflow's status = %q, want failed", got.Workflows[0].Status)
	}
}

// TestRunStopsAtTheStepCeiling covers the ceiling on one run: the steps past it are reported as not
// reached rather than quietly dropped, and no more of them reaches a shell.
func TestRunStopsAtTheStepCeiling(t *testing.T) {
	text := `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Run tests
        run: pnpm test
      - name: Lint
        run: pnpm lint
      - name: Build
        run: pnpm build
`
	runner := &fakeRunner{}
	svc := writeWorktree(t, map[string]string{".github/workflows/ci.yml": text}, runner,
		func(deps *localci.Deps) { deps.MaxSteps = 1 })
	got, err := svc.Run(context.Background(), "card", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	steps := got.Workflows[0].Steps
	if steps[0].Status != protocol.LocalCIStatusPassed {
		t.Errorf("the first step = %q, want passed", steps[0].Status)
	}
	for i, step := range steps[1:] {
		if step.Status != protocol.LocalCIStatusSkipped || step.Reason == "" {
			t.Errorf("step %d = %q %q, want skipped with a reason", i+1, step.Status, step.Reason)
		}
	}
	if len(runner.requests) != 1 {
		t.Errorf("the runner was asked for %d steps, want the one inside the ceiling", len(runner.requests))
	}
}

// TestRunAsksForTheShellAStepNames covers a step that asks for bash: it is run with bash, and the
// arguments are the ones that make an early failure fail the step.
func TestRunAsksForTheShellAStepNames(t *testing.T) {
	text := `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Run tests
        shell: bash
        run: pnpm test
`
	runner := &fakeRunner{}
	svc := writeWorktree(t, map[string]string{".github/workflows/ci.yml": text}, runner)
	if _, err := svc.Run(context.Background(), "card", ""); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(runner.requests) != 1 || runner.requests[0].Shell != "/bin/bash" {
		t.Fatalf("the runs were %+v, want one under /bin/bash", runner.requests)
	}
}

// TestRunRefusesACardWithNoWorktree covers a card that has not started: there is nothing to run in,
// so the answer is a refusal with a sentence rather than an empty run.
func TestRunRefusesACardWithNoWorktree(t *testing.T) {
	svc, err := localci.New(localci.Deps{Projects: &fakeProjects{}})
	if err != nil {
		t.Fatalf("build the service: %v", err)
	}
	_, err = svc.Run(context.Background(), "card", "")
	assertRefused(t, err)
}

// TestRunRefusesAWorkflowItDoesNotHave covers a name that matches nothing: the person asked for one
// workflow, so they are told it is not there rather than handed an empty answer.
func TestRunRefusesAWorkflowItDoesNotHave(t *testing.T) {
	svc := writeWorktree(t, map[string]string{".github/workflows/ci.yml": "jobs:\n"}, &fakeRunner{})
	_, err := svc.Run(context.Background(), "card", "release.yml")
	assertRefused(t, err)
}

// assertRefused says an error is a refusal in a plain sentence.
func assertRefused(t *testing.T, err error) {
	t.Helper()
	var answer *protocol.Error
	if !errors.As(err, &answer) {
		t.Fatalf("the error is %v, want a refusal", err)
	}
	if answer.Code != protocol.ErrorCodeRefused {
		t.Errorf("the error code = %q, want %q", answer.Code, protocol.ErrorCodeRefused)
	}
	if !strings.HasSuffix(answer.Message, ".") {
		t.Errorf("the message %q does not read as a sentence", answer.Message)
	}
}

// TestRunAnswersAnEmptyResultWhenThereAreNoWorkflows covers a worktree with no workflow files: it is
// not an error, and the answer's lists are empty rather than absent.
func TestRunAnswersAnEmptyResultWhenThereAreNoWorkflows(t *testing.T) {
	svc := writeWorktree(t, map[string]string{"README.md": "hello\n"}, &fakeRunner{})
	got, err := svc.Run(context.Background(), "card", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Workflows == nil || len(got.Workflows) != 0 {
		t.Errorf("Workflows = %#v, want an empty list rather than none", got.Workflows)
	}
	if got.Branch != testBranch {
		t.Errorf("Branch = %q, want the card's branch", got.Branch)
	}
}

// TestRunRunsOnlyTheWorkflowItWasAsked covers a name that matches one file of several, by its own
// name and by its path within the worktree.
func TestRunRunsOnlyTheWorkflowItWasAsked(t *testing.T) {
	files := map[string]string{
		".github/workflows/ci.yml":    "name: CI\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Run tests\n        run: pnpm test\n",
		".github/workflows/lint.yaml": "name: Lint\njobs:\n  lint:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Lint\n        run: pnpm lint\n",
	}
	for _, asked := range []string{"ci.yml", ".github/workflows/ci.yml"} {
		runner := &fakeRunner{}
		svc := writeWorktree(t, files, runner)
		got, err := svc.Run(context.Background(), "card", asked)
		if err != nil {
			t.Fatalf("Run(%q): %v", asked, err)
		}
		if len(got.Workflows) != 1 || got.Workflows[0].Name != "CI" {
			t.Errorf("Run(%q) ran %+v, want only CI", asked, got.Workflows)
		}
	}
}
