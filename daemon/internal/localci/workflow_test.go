package localci_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/localci"
)

// fixtureWorkflow is a workflow file as projects really write one: a comment, a trigger block with
// nested keys, environment at all three levels, two jobs, an action with its own inputs, a command
// written as a block, a condition, and a runner Marshal can use.
const fixtureWorkflow = `# The project's own checks, run on every push.
name: CI

on:
  push:
    branches: [main]
  pull_request:

env:
  NODE_ENV: test       # what the tests expect

jobs:
  test:
    runs-on: ubuntu-latest
    env:
      CI: "true"
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - name: Install
        run: pnpm install --frozen-lockfile
      - name: Run tests
        run: |
          pnpm test
          pnpm test:e2e
        env:
          COVERAGE: "1"
      - name: Lint
        run: pnpm lint
        if: github.event_name == 'pull_request'
  build:
    runs-on: macos-14
    steps:
      - name: Build
        run: pnpm build
`

// jobNamed answers one job of a workflow by its name.
func jobNamed(t *testing.T, w localci.Workflow, name string) localci.Job {
	t.Helper()
	for _, job := range w.Jobs {
		if job.Name == name {
			return job
		}
	}
	t.Fatalf("the workflow has no job named %q; it has %d jobs", name, len(w.Jobs))
	return localci.Job{}
}

// TestParseWorkflowReadsTheShapesMarshalRuns covers the whole file at once: the name, the workflow's
// own environment, the trigger block that must not look like jobs, both jobs, and each step.
func TestParseWorkflowReadsTheShapesMarshalRuns(t *testing.T) {
	w := localci.ParseWorkflow(".github/workflows/ci.yml", fixtureWorkflow)

	if w.File != ".github/workflows/ci.yml" {
		t.Errorf("File = %q, want the path it was given", w.File)
	}
	if w.Name != "CI" {
		t.Errorf("Name = %q, want CI", w.Name)
	}
	if len(w.Env) != 1 || w.Env[0] != "NODE_ENV=test" {
		t.Errorf("Env = %q, want [NODE_ENV=test]; a trailing comment is not part of the value", w.Env)
	}
	if w.EnvReason != "" {
		t.Errorf("EnvReason = %q, want empty", w.EnvReason)
	}
	if len(w.Jobs) != 2 {
		t.Fatalf("read %d jobs, want 2: the `on:` block's children are not jobs", len(w.Jobs))
	}

	test := jobNamed(t, w, "test")
	if test.Skip != "" {
		t.Errorf("the test job's Skip = %q, want empty: ubuntu-latest is a machine Marshal can use", test.Skip)
	}
	if len(test.Env) != 1 || test.Env[0] != "CI=true" {
		t.Errorf("the test job's Env = %q, want [CI=true] with the quotes taken off", test.Env)
	}
	if len(test.Steps) != 4 {
		t.Fatalf("the test job has %d steps, want 4", len(test.Steps))
	}

	checkout := test.Steps[0]
	if checkout.Uses != "actions/checkout@v4" || checkout.Run != "" {
		t.Errorf("step 0 = %+v, want an action and no command", checkout)
	}
	if checkout.Name != "actions/checkout@v4" {
		t.Errorf("a step with no name is called %q, want its action", checkout.Name)
	}
	if len(test.Steps) > 1 && test.Steps[1].Uses == "fetch-depth" {
		t.Error("the action's `with:` inputs were read as steps of their own")
	}

	install := test.Steps[1]
	if install.Name != "Install" || install.Run != "pnpm install --frozen-lockfile" {
		t.Errorf("step 1 = %+v, want the install command", install)
	}

	tests := test.Steps[2]
	if tests.Run != "pnpm test\npnpm test:e2e" {
		t.Errorf("step 2's Run = %q, want both lines of the block command", tests.Run)
	}
	if len(tests.Env) != 1 || tests.Env[0] != "COVERAGE=1" {
		t.Errorf("step 2's Env = %q, want [COVERAGE=1]", tests.Env)
	}

	lint := test.Steps[3]
	if lint.If != "github.event_name == 'pull_request'" {
		t.Errorf("step 3's If = %q, want the condition as the file has it", lint.If)
	}

	build := jobNamed(t, w, "build")
	if build.Skip != "" {
		t.Errorf("the build job's Skip = %q, want empty: a macOS runner is this kind of machine", build.Skip)
	}
	if len(build.Steps) != 1 || build.Steps[0].Run != "pnpm build" {
		t.Errorf("the build job's steps = %+v, want the one build step", build.Steps)
	}
}

// TestParseWorkflowReportsTheJobShapesItWillNotRun covers every shape that stops a whole job: none
// of its steps may be run, and each one carries the sentence that says why.
func TestParseWorkflowReportsTheJobShapesItWillNotRun(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		reason string
	}{
		{
			name:   "a Windows runner",
			body:   "    runs-on: windows-latest\n",
			reason: "This job runs on Windows, so Marshal did not run it on this machine.",
		},
		{
			name:   "a runner Marshal cannot place",
			body:   "    runs-on: big-iron\n",
			reason: "This job asks for a runner Marshal does not recognise, so Marshal did not guess at it.",
		},
		{
			name:   "a self-hosted machine",
			body:   "    runs-on: [self-hosted, linux]\n",
			reason: "This job asks for a self-hosted machine of its own, so Marshal did not run it here.",
		},
		{
			name:   "a runner only GitHub knows",
			body:   "    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n",
			reason: "This job asks for a runner Marshal does not recognise, so Marshal did not guess at it.",
		},
		{
			name:   "a condition on the job",
			body:   "    runs-on: ubuntu-latest\n    if: github.ref == 'refs/heads/main'\n",
			reason: "This job runs only when a condition Marshal cannot decide here holds.",
		},
		{
			name:   "a matrix",
			body:   "    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        node: [20, 22]\n",
			reason: "This job runs once for each value of a matrix, so Marshal did not run it here.",
		},
		{
			name:   "a service container",
			body:   "    runs-on: ubuntu-latest\n    services:\n      db:\n        image: postgres\n",
			reason: "This job needs a service container, which Marshal does not start.",
		},
		{
			name:   "a job that calls another workflow",
			body:   "    uses: ./.github/workflows/reusable.yml\n",
			reason: "This job calls another workflow rather than running steps of its own.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := "name: CI\njobs:\n  test:\n" + tc.body + "    steps:\n      - name: Test\n        run: pnpm test\n"
			job := jobNamed(t, localci.ParseWorkflow("ci.yml", text), "test")
			if job.Skip != tc.reason {
				t.Errorf("Skip = %q, want %q", job.Skip, tc.reason)
			}
			if len(job.Steps) != 1 || job.Steps[0].Run != "pnpm test" {
				t.Errorf("the job's steps = %+v, want the steps still read", job.Steps)
			}
		})
	}
}

// TestParseWorkflowKeepsReadingPastABlockRunner covers the shape that has to be skipped whole: a
// `runs-on:` written as a list. Its items are deeper than the job's own keys, and the steps below it
// must still be read rather than swallowed with it.
func TestParseWorkflowKeepsReadingPastABlockRunner(t *testing.T) {
	text := `jobs:
  test:
    runs-on:
      - self-hosted
      - linux
    steps:
      - name: Test
        run: pnpm test
  other:
    runs-on: ubuntu-latest
    steps:
      - name: Build
        run: pnpm build
`
	w := localci.ParseWorkflow("ci.yml", text)
	if len(w.Jobs) != 2 {
		t.Fatalf("read %d jobs, want 2", len(w.Jobs))
	}
	test := jobNamed(t, w, "test")
	if test.Skip == "" {
		t.Error("a self-hosted runner must not be run")
	}
	if len(test.Steps) != 1 || test.Steps[0].Run != "pnpm test" {
		t.Errorf("the steps under a block runner were not read: %+v", test.Steps)
	}
	if other := jobNamed(t, w, "other"); len(other.Steps) != 1 {
		t.Errorf("the job after a block runner has %d steps, want 1", len(other.Steps))
	}
}

// TestParseWorkflowReportsAnEnvironmentItCannotUse covers the three ways an environment entry holds
// a value Marshal cannot know: an interpolation, a nested map, and a list.
func TestParseWorkflowReportsAnEnvironmentItCannotUse(t *testing.T) {
	cases := []struct {
		name  string
		block string
		want  []string
	}{
		{
			name:  "a GitHub value",
			block: "env:\n  TOKEN: ${{ secrets.TOKEN }}\n  PLAIN: yes\n",
			want:  []string{"PLAIN=yes"},
		},
		{
			name:  "a nested map",
			block: "env:\n  OUTER:\n    INNER: value\n",
			want:  nil,
		},
		{
			name:  "a list",
			block: "env:\n  - FOO=bar\n",
			want:  nil,
		},
		{
			name:  "an inline map",
			block: "env: {FOO: bar}\n",
			want:  nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := localci.ParseWorkflow("ci.yml", "name: CI\n"+tc.block+"jobs:\n")
			if w.EnvReason == "" {
				t.Error("the environment was reported as usable, want a reason")
			}
			if len(w.Env) != len(tc.want) {
				t.Fatalf("Env = %q, want %q", w.Env, tc.want)
			}
			for i := range tc.want {
				if w.Env[i] != tc.want[i] {
					t.Errorf("Env[%d] = %q, want %q", i, w.Env[i], tc.want[i])
				}
			}
		})
	}
}

// TestParseWorkflowKeepsABlockCommandHonest covers what a block scalar is: the file's own text, where
// a "#" is part of the command and not a comment, and where a folded block joins its lines.
func TestParseWorkflowKeepsABlockCommandHonest(t *testing.T) {
	text := `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Test
        run: |
          echo "count: #3"
          pnpm test
      - name: Folded
        run: >
          pnpm lint
          --fix
`
	job := jobNamed(t, localci.ParseWorkflow("ci.yml", text), "test")
	if len(job.Steps) != 2 {
		t.Fatalf("read %d steps, want 2", len(job.Steps))
	}
	if want := "echo \"count: #3\"\npnpm test"; job.Steps[0].Run != want {
		t.Errorf("the literal block's Run = %q, want %q: a # inside one is content", job.Steps[0].Run, want)
	}
	if want := "pnpm lint --fix"; job.Steps[1].Run != want {
		t.Errorf("the folded block's Run = %q, want %q", job.Steps[1].Run, want)
	}
}

// TestParseWorkflowNamesAStepWithNoName covers what a step with no `name:` is called, and the shape
// where a step's keys start on the line under its dash.
func TestParseWorkflowNamesAStepWithNoName(t *testing.T) {
	text := `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      -
        run: pnpm test
      - name: "Quoted name"
        shell: bash
        run: pnpm lint
`
	job := jobNamed(t, localci.ParseWorkflow("ci.yml", text), "test")
	if len(job.Steps) != 2 {
		t.Fatalf("read %d steps, want 2", len(job.Steps))
	}
	if job.Steps[0].Name != "pnpm test" {
		t.Errorf("a step with no name is called %q, want its command", job.Steps[0].Name)
	}
	if job.Steps[1].Name != "Quoted name" {
		t.Errorf("a quoted name is %q, want the text without its quotes", job.Steps[1].Name)
	}
	if job.Steps[1].Shell != "bash" {
		t.Errorf("Shell = %q, want bash", job.Steps[1].Shell)
	}
}

// TestParseWorkflowFallsBackToTheFileName covers a workflow with no name of its own.
func TestParseWorkflowFallsBackToTheFileName(t *testing.T) {
	for _, tc := range []struct{ file, want string }{
		{".github/workflows/ci.yml", "ci"},
		{".github/workflows/release.yaml", "release"},
	} {
		if got := localci.ParseWorkflow(tc.file, "jobs:\n").Name; got != tc.want {
			t.Errorf("ParseWorkflow(%q).Name = %q, want %q", tc.file, got, tc.want)
		}
	}
}

// TestReadWorkflowsReadsTheFolder covers the folder itself: only workflow files, in file order, and
// a worktree with no folder at all, which has none rather than an error.
func TestReadWorkflowsReadsTheFolder(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, localci.WorkflowsDir)
	if err := os.MkdirAll(filepath.Join(folder, "nested"), 0o755); err != nil {
		t.Fatalf("make the workflow folder: %v", err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(folder, name), []byte(text), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("z-last.yml", "name: Last\njobs:\n")
	write("a-first.yaml", "name: First\njobs:\n")
	write("notes.md", "this is not a workflow\n")

	list, err := localci.ReadWorkflows(dir)
	if err != nil {
		t.Fatalf("ReadWorkflows: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("read %d workflows, want 2: a .md file and a folder are not workflows", len(list))
	}
	if list[0].File != ".github/workflows/a-first.yaml" || list[1].File != ".github/workflows/z-last.yml" {
		t.Errorf("the files came back as %q and %q, want them in file order", list[0].File, list[1].File)
	}

	readNothing(t, t.TempDir(), "a worktree with no workflow folder")
}

// TestReadWorkflowsOnAWorktreeWithNoWorkflows covers the folder being absent and being empty.
func TestReadWorkflowsOnAWorktreeWithNoWorkflows(t *testing.T) {
	readNothing(t, t.TempDir(), "a worktree with no workflow folder")

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, localci.WorkflowsDir), 0o755); err != nil {
		t.Fatalf("make the workflow folder: %v", err)
	}
	readNothing(t, dir, "a worktree with an empty workflow folder")
}

// readNothing says a worktree answers no workflows and no error.
func readNothing(t *testing.T, dir, what string) {
	t.Helper()
	list, err := localci.ReadWorkflows(dir)
	if err != nil {
		t.Fatalf("ReadWorkflows for %s: %v", what, err)
	}
	if len(list) != 0 {
		t.Errorf("%s answered %d workflows, want none", what, len(list))
	}
}
