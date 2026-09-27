package protocol

import "time"

// The wire shape of local CI (docs/architecture.md sections 9 and 11.1, docs/marshal-product-scope.md
// 15.3, docs/backend-checklist.md B6.5, build-plan 6.5): the project's own workflow files, read
// one step at a time, with what Marshal ran, what it refused to run, and why. "Run the same checks
// as GitHub Actions on your machine, before pushing" is the whole point, so a step Marshal cannot
// run locally is reported as `unsupported` with a sentence rather than being passed over quietly.

// LocalCIKind is what a workflow step does. Marshal runs the first three locally; the fourth is
// reported and never run, because a local run of it would not be the same run GitHub does.
type LocalCIKind string

const (
	// LocalCIKindTest is a step that runs tests.
	LocalCIKindTest LocalCIKind = "test"
	// LocalCIKindLint is a step that lints, formats, or type-checks.
	LocalCIKindLint LocalCIKind = "lint"
	// LocalCIKindBuild is a step that builds or compiles.
	LocalCIKindBuild LocalCIKind = "build"
	// LocalCIKindOther is a step that is none of the three, such as a deploy or a packaging step.
	LocalCIKindOther LocalCIKind = "other"
)

// LocalCIKindValues lists every kind, in the order a run reports them.
func LocalCIKindValues() []LocalCIKind {
	return []LocalCIKind{LocalCIKindTest, LocalCIKindLint, LocalCIKindBuild, LocalCIKindOther}
}

// Valid reports whether k is a kind.
func (k LocalCIKind) Valid() bool {
	for _, v := range LocalCIKindValues() {
		if v == k {
			return true
		}
	}
	return false
}

// LocalCIStatus is where a step ended, or where a whole workflow ended. `unsupported` and `skipped`
// are different on purpose: the first is a step Marshal cannot run locally, the second is one that
// was not reached because an earlier step in the same job failed.
type LocalCIStatus string

const (
	// LocalCIStatusPassed is a step that ran and succeeded.
	LocalCIStatusPassed LocalCIStatus = "passed"
	// LocalCIStatusFailed is a step that ran and failed, which is what a person fixes before
	// pushing.
	LocalCIStatusFailed LocalCIStatus = "failed"
	// LocalCIStatusUnsupported is a step Marshal will not run locally, with `Reason` saying why.
	LocalCIStatusUnsupported LocalCIStatus = "unsupported"
	// LocalCIStatusSkipped is a step that was not reached, with `Reason` saying what stopped it.
	LocalCIStatusSkipped LocalCIStatus = "skipped"
)

// LocalCIStatusValues lists every status, worst last.
func LocalCIStatusValues() []LocalCIStatus {
	return []LocalCIStatus{
		LocalCIStatusPassed, LocalCIStatusFailed, LocalCIStatusUnsupported, LocalCIStatusSkipped,
	}
}

// Valid reports whether s is a status.
func (s LocalCIStatus) Valid() bool {
	for _, v := range LocalCIStatusValues() {
		if v == s {
			return true
		}
	}
	return false
}

// LocalCIRequest is the body of POST /v1/cards/{id}/local-ci. The card comes from the path.
type LocalCIRequest struct {
	// Workflow names one workflow file, as it is named in `.github/workflows`, such as "ci.yml".
	// Empty means every workflow the card's worktree has.
	Workflow string `json:"workflow"`
}

// LocalCIStep is one step of one job, as Marshal found it and as far as it got.
type LocalCIStep struct {
	// Job is the job's name in the workflow file.
	Job string `json:"job"`
	// Name is the step's own name: its `name:`, or the command itself when it has none.
	Name string `json:"name"`
	// Kind is what the step does.
	Kind LocalCIKind `json:"kind"`
	// Status is where it ended.
	Status LocalCIStatus `json:"status"`
	// Reason says why it was not run, in a sentence a person reads. Empty for a step that ran.
	Reason string `json:"reason"`
	// Command is the step's `run:` line, as the workflow file has it. Empty for a step that was
	// not a command.
	Command string `json:"command"`
	// Output is the end of what the step printed, trimmed. Empty for a step that was not run.
	Output string `json:"output"`
	// TookMs is how long the step ran, in milliseconds. Zero for a step that was not run.
	TookMs int64 `json:"tookMs"`
}

// LocalCIWorkflow is one workflow file of the card's worktree, with every step it declares.
type LocalCIWorkflow struct {
	// File is the file's path, relative to the worktree, with forward slashes.
	File string `json:"file"`
	// Name is the workflow's own name: its top-level `name:`, or the file's name without its
	// extension when it has none.
	Name string `json:"name"`
	// Status is what the workflow ended as: failed when any step failed, unsupported when every
	// step was, passed when nothing failed, and skipped when nothing ran.
	Status LocalCIStatus `json:"status"`
	// Steps are the workflow's steps, job by job, in the order the file declares them. Never null.
	Steps []LocalCIStep `json:"steps"`
}

// LocalCIResult is what a local run produced: every workflow the request covered.
type LocalCIResult struct {
	// CardID is the card whose branch was run.
	CardID string `json:"cardId"`
	// Branch is the branch the card's worktree is on, so a person knows what was run.
	Branch string `json:"branch"`
	// Workflows are the workflow files that were run, in file order. Never null.
	Workflows []LocalCIWorkflow `json:"workflows"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewLocalCIResult makes an answer stamped with the daemon's time. Nil lists become empty ones, so
// the JSON has [] and never null.
func NewLocalCIResult(cardID, branch string, workflows []LocalCIWorkflow, now time.Time) LocalCIResult {
	out := make([]LocalCIWorkflow, len(workflows))
	for i, workflow := range workflows {
		steps := make([]LocalCIStep, len(workflow.Steps))
		copy(steps, workflow.Steps)
		workflow.Steps = steps
		out[i] = workflow
	}
	return LocalCIResult{
		CardID: cardID, Branch: branch, Workflows: out, ServerTime: NewTimestamp(now),
	}
}
