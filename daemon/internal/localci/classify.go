package localci

import (
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The classifier decides what a workflow step is and whether Marshal can run it here. It is the
// half of this package that turns the shapes the reader understood into the sentence a person
// reads, and it is kept apart from the reader so the decisions can be read on their own.
//
// Two rules guide it. A step Marshal does not understand is refused with a sentence rather than
// guessed at, because "run the same checks as GitHub Actions" is only worth anything if a local
// run really is the run GitHub would do. And a step Marshal *misreads* is run rather than skipped:
// running a harmless step costs seconds, while skipping a real test step costs a person the failure
// they were trying to catch before pushing.

// The sentences for the shapes that stop a step or a job from running locally. They are the words a
// person reads in the checks list, so each one says what Marshal would not do and why.
const (
	// reasonAction is a step that runs a GitHub action rather than a command. An action is code
	// GitHub fetches and runs, so running it here would not be the same run.
	reasonAction = "This step runs a GitHub action rather than a command, so it did not run here."
	// reasonCondition is a step whose `if:` decides from a GitHub value whether it runs.
	reasonCondition = "This step runs only when a condition Marshal cannot decide here holds."
	// reasonJobCondition is the same for a job: none of its steps can be known to run.
	reasonJobCondition = "This job runs only when a condition Marshal cannot decide here holds."
	// reasonEnvironment is a step or a job whose own environment needs a value only GitHub has.
	reasonEnvironment = "This step needs a value only GitHub can give it, so it did not run here."
	// reasonInterpolation is a step whose command itself uses a value only GitHub has.
	reasonInterpolation = "This step's command uses a value only GitHub can give it, so it did not run here."
	// reasonShell is a step that asks for a shell Marshal will not stand in for.
	reasonShell = "This step asks for a different shell, so it did not run here."
	// reasonNoCommand is a step with nothing for Marshal to run.
	reasonNoCommand = "This step has no command for Marshal to run."
	// reasonSideEffect is a step that deploys, publishes, or pushes something. A local run changes
	// nothing outside the worktree, so a step that would change something outside it is reported.
	reasonSideEffect = "This step deploys or publishes something, so Marshal did not run it here."
	// reasonOther is a step that is not one of the three kinds Marshal runs locally.
	reasonOther = "This step is not a test, lint, or build step, so Marshal does not run it here."
	// reasonWindows is a job that runs on Windows, whose commands are usually Windows' own.
	reasonWindows = "This job runs on Windows, so Marshal did not run it on this machine."
	// reasonSelfHosted is a job that asks for a self-hosted machine, which is a machine of its own
	// with labels Marshal cannot stand in for, whatever operating system it also names.
	reasonSelfHosted = "This job asks for a self-hosted machine of its own, so Marshal did not run it here."
	// reasonRunner is a job that asks for a runner Marshal does not recognise.
	reasonRunner = "This job asks for a runner Marshal does not recognise, so Marshal did not guess at it."
	// reasonMatrix is a job that runs once for each value of a matrix.
	reasonMatrix = "This job runs once for each value of a matrix, so Marshal did not run it here."
	// reasonService is a job that needs a service container, such as a database.
	reasonService = "This job needs a service container, which Marshal does not start."
	// reasonReusable is a job that calls another workflow rather than running steps of its own.
	reasonReusable = "This job calls another workflow rather than running steps of its own."
	// reasonEarlierFailed is a step that was not reached because an earlier step in its job failed,
	// which is how GitHub runs a job too.
	reasonEarlierFailed = "An earlier step in this job failed, so this step did not run."
	// reasonTooManySteps is a step Marshal did not reach because one run has a ceiling on steps.
	reasonTooManySteps = "This run reached the most steps Marshal runs at once, so this step did not run."
)

// decision is what Marshal decided about one step.
type decision struct {
	// Kind is what the step does.
	Kind protocol.LocalCIKind
	// Reason is why the step cannot run locally, in a sentence. Empty when it can, which is the
	// answer for every step whose kind Marshal runs.
	Reason string
}

// classify decides what a step is and whether Marshal can run it here. A step that can run answers
// an empty reason; every other step answers the sentence that says why not.
func classify(step Step) decision {
	kind := stepKind(step)
	switch {
	case step.Uses != "":
		return decision{Kind: protocol.LocalCIKindOther, Reason: reasonAction}
	case step.EnvReason != "":
		return decision{Kind: kind, Reason: reasonEnvironment}
	case step.If != "":
		return decision{Kind: kind, Reason: reasonCondition}
	case strings.TrimSpace(step.Run) == "":
		return decision{Kind: protocol.LocalCIKindOther, Reason: reasonNoCommand}
	case !knownShell(step.Shell):
		return decision{Kind: kind, Reason: reasonShell}
	case sideEffect(step.Run):
		return decision{Kind: protocol.LocalCIKindOther, Reason: reasonSideEffect}
	case strings.Contains(step.Run, interpolation):
		return decision{Kind: kind, Reason: reasonInterpolation}
	case kind == protocol.LocalCIKindOther:
		return decision{Kind: kind, Reason: reasonOther}
	}
	return decision{Kind: kind}
}

// stepKind decides what a step does from what it is called and what it runs. A step that runs an
// action is not any of the three kinds: an action is not a command at all.
func stepKind(step Step) protocol.LocalCIKind {
	if step.Uses != "" {
		return protocol.LocalCIKindOther
	}
	// The step's own name is what a person wrote to say what the step is for, so it is read first;
	// the command decides when the name says nothing.
	if kind := kindOf(step.Name); kind != protocol.LocalCIKindOther {
		return kind
	}
	return kindOf(step.Run)
}

// kindOf reads the first word of a text that names a kind, matching whole words so "test" is found
// in `pnpm test` and `npm run test:unit` and not in "latest".
func kindOf(text string) protocol.LocalCIKind {
	kindWords := kindWords()
	for _, word := range words(text) {
		if kind, ok := kindWords[word]; ok {
			return kind
		}
	}
	return protocol.LocalCIKindOther
}

// words splits a text into lowercase words on everything that is not a letter or a digit, so a
// command's flags, paths, and punctuation do not hide the word that says what it does.
func words(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

// kindWords is the vocabulary Marshal reads a step with: the words that name each kind, as people
// write them in workflows. It is short on purpose, and it holds only words that name an activity
// rather than a tool that takes a subcommand: `cargo test` is a test because of "test", not because
// of "cargo", and a program that runs many things ("make", "docker", "npm", "go") says nothing on
// its own. A word that is nobody's is not found at all, and the step is reported instead.
func kindWords() map[string]protocol.LocalCIKind {
	return map[string]protocol.LocalCIKind{
		// Words that name a test step, including the test runner's own name.
		"test":       protocol.LocalCIKindTest,
		"tests":      protocol.LocalCIKindTest,
		"spec":       protocol.LocalCIKindTest,
		"specs":      protocol.LocalCIKindTest,
		"jest":       protocol.LocalCIKindTest,
		"vitest":     protocol.LocalCIKindTest,
		"pytest":     protocol.LocalCIKindTest,
		"gotestsum":  protocol.LocalCIKindTest,
		"rspec":      protocol.LocalCIKindTest,
		"phpunit":    protocol.LocalCIKindTest,
		"playwright": protocol.LocalCIKindTest,
		"cypress":    protocol.LocalCIKindTest,

		// Words that name a lint, format, or type-check step, including the linter's own name.
		"lint":        protocol.LocalCIKindLint,
		"linters":     protocol.LocalCIKindLint,
		"eslint":      protocol.LocalCIKindLint,
		"stylelint":   protocol.LocalCIKindLint,
		"biome":       protocol.LocalCIKindLint,
		"prettier":    protocol.LocalCIKindLint,
		"format":      protocol.LocalCIKindLint,
		"fmt":         protocol.LocalCIKindLint,
		"gofmt":       protocol.LocalCIKindLint,
		"vet":         protocol.LocalCIKindLint,
		"golangci":    protocol.LocalCIKindLint,
		"clippy":      protocol.LocalCIKindLint,
		"ruff":        protocol.LocalCIKindLint,
		"flake8":      protocol.LocalCIKindLint,
		"rubocop":     protocol.LocalCIKindLint,
		"tsc":         protocol.LocalCIKindLint,
		"typecheck":   protocol.LocalCIKindLint,
		"checkstyle":  protocol.LocalCIKindLint,
		"staticcheck": protocol.LocalCIKindLint,
		"shellcheck":  protocol.LocalCIKindLint,

		// Words that name a build step, including the bundler's own name.
		"build":   protocol.LocalCIKindBuild,
		"builds":  protocol.LocalCIKindBuild,
		"compile": protocol.LocalCIKindBuild,
		"webpack": protocol.LocalCIKindBuild,
		"rollup":  protocol.LocalCIKindBuild,
		"esbuild": protocol.LocalCIKindBuild,
		"tsup":    protocol.LocalCIKindBuild,
	}
}

// sideEffectWords are the words that say a step changes something outside the worktree: it deploys,
// publishes, or pushes. Marshal never runs one of these from a local run, whatever else the step
// says it does, so a step that builds and then deploys is reported rather than half-run. The words
// are read from the command alone and never from the step's name, so a test *about* pushing
// something is still a test.
func sideEffectWords() map[string]bool {
	return map[string]bool{
		"deploy": true, "deploys": true, "deployment": true,
		"publish": true, "publishes": true, "release": true,
		"upload": true, "uploads": true,
		"push": true, "pushes": true,
		"sync": true, "rsync": true, "scp": true, "ssh": true,
		"helm": true, "kubectl": true, "terraform": true, "ansible": true,
		"heroku": true, "netlify": true, "vercel": true, "surge": true,
	}
}

// sideEffect says whether a command deploys, publishes, or pushes something.
func sideEffect(command string) bool {
	sideEffectWords := sideEffectWords()
	for _, word := range words(command) {
		if sideEffectWords[word] {
			return true
		}
	}
	return false
}

// runnerReason says why a job's `runs-on:` stops it from running on this machine. An empty answer
// means the job runs where Marshal can run it.
//
// A GitHub-hosted Linux, Ubuntu, or macOS runner is named by the word alone and is accepted: those
// are the same kind of machine Marshal's own shell is on, and refusing them would make the whole
// feature useless on the machines people use it from. Everything else is refused rather than
// guessed at: a Windows runner, because the commands a Windows job runs are usually Windows' own; a
// self-hosted runner, because it is a machine of its own with labels Marshal cannot stand in for;
// and a name Marshal does not place at all.
func runnerReason(runsOn string) string {
	text := strings.ToLower(strings.TrimSpace(runsOn))
	switch {
	case text == "":
		return ""
	case strings.Contains(text, "windows"):
		return reasonWindows
	case strings.Contains(text, "self-hosted"), strings.Contains(text, "self hosted"):
		return reasonSelfHosted
	case strings.Contains(text, "ubuntu"), strings.Contains(text, "linux"),
		strings.Contains(text, "macos"):
		return ""
	}
	return reasonRunner
}

// knownShell says whether Marshal will run a step that names a shell. Marshal runs the two shells
// it has: /bin/sh, which is GitHub's own default, and /bin/bash. A step that names any other shell
// is reported rather than run under one that is not the one it asked for.
func knownShell(shell string) bool {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "", "sh", "bash":
		return true
	}
	return false
}

// shellPath answers the program to run a step's command with. A shell Marshal will not stand in for
// is refused by classify, before a run is ever started, so this only ever answers one of the two.
func shellPath(shell string) string {
	if strings.EqualFold(strings.TrimSpace(shell), "bash") {
		return "/bin/bash"
	}
	return "/bin/sh"
}

// stepName is the name a step with no `name:` of its own is called: its action, or its command.
func stepName(step Step) string {
	if step.Uses != "" {
		return step.Uses
	}
	text := strings.TrimSpace(step.Run)
	if text == "" {
		return "step"
	}
	return firstLine(text)
}

// firstLine answers the first line of a text, which is what a step with no name is labelled with
// even when its command is a block of several lines.
func firstLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.TrimSpace(text[:i])
	}
	return text
}

// orReason keeps the first reason Marshal found, so the sentence a person reads names the shape the
// file met first rather than the last one the reader happened to notice.
func orReason(have, add string) string {
	if have != "" {
		return have
	}
	return add
}
