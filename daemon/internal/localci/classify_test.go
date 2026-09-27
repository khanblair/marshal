package localci

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// TestClassifyDecidesWhatAStepIs covers the decision that turns a step into a run: which kind of
// step it is, and the sentence for every shape Marshal will not run.
func TestClassifyDecidesWhatAStepIs(t *testing.T) {
	cases := []struct {
		name   string
		step   Step
		kind   protocol.LocalCIKind
		reason string
	}{
		{
			name: "a test script",
			step: Step{Run: "pnpm test"},
			kind: protocol.LocalCIKindTest,
		},
		{
			name: "a test runner under another name",
			step: Step{Run: "pnpm exec vitest run --no-coverage"},
			kind: protocol.LocalCIKindTest,
		},
		{
			name: "a test script with a colon in it",
			step: Step{Run: "npm run test:unit"},
			kind: protocol.LocalCIKindTest,
		},
		{
			name: "a build",
			step: Step{Run: "go build ./..."},
			kind: protocol.LocalCIKindBuild,
		},
		{
			name: "a type check",
			step: Step{Run: "pnpm exec tsc --noEmit"},
			kind: protocol.LocalCIKindLint,
		},
		{
			name: "a linter's own name",
			step: Step{Run: "npx biome check ."},
			kind: protocol.LocalCIKindLint,
		},
		{
			name: "a step whose name says what it is",
			step: Step{Name: "Lint", Run: "./bin/check-the-things.sh"},
			kind: protocol.LocalCIKindLint,
		},
		{
			name: "a tool that takes a subcommand",
			step: Step{Run: "make test"},
			kind: protocol.LocalCIKindTest,
		},
		{
			name: "a tool whose subcommand names nothing Marshal runs",
			step: Step{Run: "make install"},
			kind: protocol.LocalCIKindOther,

			reason: reasonOther,
		},
		{
			name:   "a deploy",
			step:   Step{Run: "./scripts/deploy.sh"},
			kind:   protocol.LocalCIKindOther,
			reason: reasonSideEffect,
		},
		{
			name:   "a build that also publishes",
			step:   Step{Run: "npm run build && aws s3 sync dist s3://bucket"},
			kind:   protocol.LocalCIKindOther,
			reason: reasonSideEffect,
		},
		{
			name: "a test that is about pushing something",
			step: Step{Name: "Test push notifications", Run: "pnpm test"},
			kind: protocol.LocalCIKindTest,
		},
		{
			name:   "an action",
			step:   Step{Uses: "actions/checkout@v4"},
			kind:   protocol.LocalCIKindOther,
			reason: reasonAction,
		},
		{
			name:   "a step that runs only on a condition",
			step:   Step{Run: "pnpm test", If: "github.event_name == 'push'"},
			kind:   protocol.LocalCIKindTest,
			reason: reasonCondition,
		},
		{
			name:   "a step whose environment Marshal cannot use",
			step:   Step{Run: "pnpm test", EnvReason: reasonEnvironment},
			kind:   protocol.LocalCIKindTest,
			reason: reasonEnvironment,
		},
		{
			name:   "a command that names a value only GitHub has",
			step:   Step{Run: "echo ${{ github.ref }}"},
			kind:   protocol.LocalCIKindOther,
			reason: reasonInterpolation,
		},
		{
			name:   "a shell Marshal will not stand in for",
			step:   Step{Run: "pnpm test", Shell: "pwsh"},
			kind:   protocol.LocalCIKindTest,
			reason: reasonShell,
		},
		{
			name:   "a step with nothing for Marshal to run",
			step:   Step{Name: "Nothing here"},
			kind:   protocol.LocalCIKindOther,
			reason: reasonNoCommand,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.step)
			if got.Kind != tc.kind {
				t.Errorf("kind = %q, want %q", got.Kind, tc.kind)
			}
			if got.Reason != tc.reason {
				t.Errorf("reason = %q, want %q", got.Reason, tc.reason)
			}
		})
	}
}

// TestRunnerReasonPlacesARunner covers which machines Marshal will run a job on.
func TestRunnerReasonPlacesARunner(t *testing.T) {
	cases := []struct {
		runsOn string
		want   string
	}{
		{runsOn: "", want: ""},
		{runsOn: "ubuntu-latest", want: ""},
		{runsOn: "ubuntu-22.04", want: ""},
		{runsOn: "macos-14", want: ""},
		{runsOn: "depot-ubuntu-22.04", want: ""},
		{runsOn: "windows-latest", want: reasonWindows},
		{runsOn: "windows-2022", want: reasonWindows},
		{runsOn: "self-hosted", want: reasonSelfHosted},
		{runsOn: "[self-hosted, linux]", want: reasonSelfHosted},
		{runsOn: "${{ matrix.os }}", want: reasonRunner},
		{runsOn: "big-iron", want: reasonRunner},
	}
	for _, tc := range cases {
		if got := runnerReason(tc.runsOn); got != tc.want {
			t.Errorf("runnerReason(%q) = %q, want %q", tc.runsOn, got, tc.want)
		}
	}
}

// TestKnownShell covers which shells Marshal will run a step under.
func TestKnownShell(t *testing.T) {
	for _, shell := range []string{"", "sh", "bash", "BASH", " bash "} {
		if !knownShell(shell) {
			t.Errorf("knownShell(%q) = false, want true", shell)
		}
	}
	for _, shell := range []string{"pwsh", "powershell", "python", "zsh"} {
		if knownShell(shell) {
			t.Errorf("knownShell(%q) = true, want false", shell)
		}
	}
}

// TestShellPathPicksTheShell covers the program a step's command is handed to, and the arguments
// that make an early failing command fail the step, as GitHub's own default does.
func TestShellPathPicksTheShell(t *testing.T) {
	if got := shellPath("bash"); got != "/bin/bash" {
		t.Errorf("shellPath(bash) = %q, want /bin/bash", got)
	}
	if got := shellPath(""); got != "/bin/sh" {
		t.Errorf("shellPath(empty) = %q, want /bin/sh", got)
	}

	bash := shellArgs("/bin/bash", "pnpm test")
	if len(bash) != 5 || bash[0] != "-e" || bash[1] != "-o" || bash[2] != "pipefail" || bash[3] != "-c" {
		t.Errorf("shellArgs for bash = %q, want -e -o pipefail -c and the command", bash)
	}
	if last := bash[len(bash)-1]; last != "pnpm test" {
		t.Errorf("the command came last as %q, want pnpm test", last)
	}

	sh := shellArgs("/bin/sh", "pnpm test")
	if len(sh) != 3 || sh[0] != "-e" || sh[1] != "-c" {
		t.Errorf("shellArgs for sh = %q, want -e -c and the command", sh)
	}
}

// TestSideEffect covers the commands Marshal never runs from a local run.
func TestSideEffect(t *testing.T) {
	for _, command := range []string{
		"git push origin HEAD",
		"npm run deploy",
		"aws s3 sync dist s3://bucket",
		"kubectl apply -f k8s.yaml",
		"npm publish",
	} {
		if !sideEffect(command) {
			t.Errorf("sideEffect(%q) = false, want true", command)
		}
	}
	for _, command := range []string{"pnpm test", "go build ./...", "echo hello", "pnpm lint"} {
		if sideEffect(command) {
			t.Errorf("sideEffect(%q) = true, want false", command)
		}
	}
}

// TestStepName covers what a step with no `name:` is called.
func TestStepName(t *testing.T) {
	cases := []struct {
		step Step
		want string
	}{
		{step: Step{Uses: "actions/checkout@v4"}, want: "actions/checkout@v4"},
		{step: Step{Run: "pnpm test"}, want: "pnpm test"},
		{step: Step{Run: "pnpm test\npnpm lint"}, want: "pnpm test"},
		{step: Step{}, want: "step"},
	}
	for _, tc := range cases {
		if got := stepName(tc.step); got != tc.want {
			t.Errorf("stepName(%+v) = %q, want %q", tc.step, got, tc.want)
		}
	}
}

// TestTailWriterKeepsTheEnd covers the reader of a step's output: a step that prints without end must
// not fill memory, and the end is what is kept.
func TestTailWriterKeepsTheEnd(t *testing.T) {
	w := &tailWriter{limit: 8}
	if _, err := w.Write([]byte("0123456789abcdef")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := w.String(); got != "89abcdef" {
		t.Errorf("kept %q, want the last 8 bytes", got)
	}

	none := &tailWriter{}
	if _, err := none.Write([]byte("anything")); err != nil {
		t.Fatalf("Write with no limit: %v", err)
	}
	if got := none.String(); got != "" {
		t.Errorf("kept %q, want nothing with a limit of zero", got)
	}
}

// TestTailLinesKeepsTheEndOfALog covers how much of a step's output is kept.
func TestTailLinesKeepsTheEndOfALog(t *testing.T) {
	text := "one\ntwo\nthree\nfour\n"
	if got := tailLines(text, 2); got != "three\nfour" {
		t.Errorf("tailLines(_, 2) = %q, want the last two lines", got)
	}
	if got := tailLines(text, 99); got != "one\ntwo\nthree\nfour" {
		t.Errorf("tailLines(_, 99) = %q, want every line", got)
	}
	if got := tailLines("  spaced  \n\n", 5); got != "  spaced" {
		t.Errorf("tailLines = %q, want the trailing whitespace gone and the text kept", got)
	}
}
