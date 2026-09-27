package protocol

import "time"

// The wire shape of CI (docs/architecture.md sections 9 and 10, build-plan 6.2 to 6.4, N10, N28).
// One shape serves the CI health page, a project's board, and the badge on a card: the newest state
// of one workflow on one branch. The daemon keeps one row per branch and workflow rather than a
// history, because what a screen shows is a workflow's state and not its runs.

// CiRun is one workflow's newest state on one branch. It is what the `ci_runs` row holds, plus the
// two times a screen needs to say "4 min ago" without a second call.
type CiRun struct {
	// ID is the run's opaque id. It is the forge's own run id when there is one.
	ID string `json:"id"`
	// ProjectID is the project whose repository the run is in.
	ProjectID string `json:"projectId"`
	// CardID is the card whose branch the run is on. Empty when the branch is not a card's: the
	// default branch, or another branch a person pushed for their own reasons.
	CardID string `json:"cardId"`
	// Branch is the Git branch the run is on.
	Branch string `json:"branch"`
	// Workflow is the workflow's name as the forge names it, such as "ci". For a monorepo it is
	// the workflow with the package it covers, such as "ci packages/web".
	Workflow string `json:"workflow"`
	// Status is where the run is.
	Status CIState `json:"status"`
	// URL is the address of the run on the forge, so a person can open it. Empty when unknown.
	URL string `json:"url"`
	// StartedAt is when the run started. Null while it is still queued, so a screen shows the
	// queued state rather than a time that has not happened.
	StartedAt *Timestamp `json:"startedAt" tstype:"Timestamp | null"`
	// UpdatedAt is when the run last changed. It is always set: it is what "4 min ago" is counted
	// from, and a run always has the moment Marshal first heard of it.
	UpdatedAt Timestamp `json:"updatedAt"`
}

// ProjectCI is one project's CI health: the state of its default branch, and every run Marshal
// knows about for it, newest first. A project Marshal has no run for has no entry at all, so a
// screen shows an honest "GitHub is not connected" instead of an empty list.
type ProjectCI struct {
	// ProjectID is the project the runs belong to.
	ProjectID string `json:"projectId"`
	// Status is the state of the project's default branch: the newest run on it, or queued when
	// Marshal knows nothing about that branch.
	Status CIState `json:"status"`
	// Runs are the project's runs, newest first. Never null.
	Runs []CiRun `json:"runs"`
}

// CiSnapshot is the answer to GET /v1/ci: every project Marshal has CI data for, in project order.
type CISnapshot struct {
	// Projects has one entry per project that has at least one run. A project with none is left
	// out on purpose: the CI health page says GitHub is not connected when none has any.
	Projects []ProjectCI `json:"projects"`
	// ServerTime is the daemon's time when the answer was made, so a client counts ages from it.
	ServerTime Timestamp `json:"serverTime"`
}

// NewCISnapshot makes an answer stamped with the daemon's time. Nil lists become empty ones, so
// the JSON has [] and never null.
func NewCISnapshot(projects []ProjectCI, now time.Time) CISnapshot {
	out := make([]ProjectCI, len(projects))
	for i, project := range projects {
		runs := make([]CiRun, len(project.Runs))
		copy(runs, project.Runs)
		project.Runs = runs
		out[i] = project
	}
	return CISnapshot{Projects: out, ServerTime: NewTimestamp(now)}
}

// CIEventData is what a `ci.updated` event carries (section 11.2). One run changes at a time, but a
// screen never shows one run alone: a board draws a project's CI health, a card draws its own badge,
// and the CI health page draws every project. So the event carries the project's CI health when it
// is published on a project's topic, and the whole snapshot when it is published on the home topic.
type CIEventData struct {
	// Project is the project whose CI changed. Set on a project's topic, and null on the home one.
	Project *ProjectCI `json:"project,omitempty"`
	// Snapshot is every project's CI as it now is. Set on the home topic, and null on a project's.
	Snapshot *CISnapshot `json:"snapshot,omitempty"`
}

// SimulateMode is which way the "Simulate CI failure" action runs (N28, B6.4, decision D5).
type SimulateMode string

const (
	// SimulateModeSynthetic injects a failed run through the CI monitor's own path - the same
	// rerun, the same trimmed log, the same loop limits, the same notice - and never touches the
	// forge. It is the mode every test uses.
	SimulateModeSynthetic SimulateMode = "synthetic"
	// SimulateModeReal pushes a deliberately failing change to the card's own branch, so the
	// forge's Actions really run and the failure comes back as a delivery. It uses Actions
	// minutes, it is shown only in dev mode or with Developer options on, and Marshal never runs
	// it without a person asking.
	SimulateModeReal SimulateMode = "real"
)

// SimulateModeValues lists every mode, the harmless one first.
func SimulateModeValues() []SimulateMode {
	return []SimulateMode{SimulateModeSynthetic, SimulateModeReal}
}

// Valid reports whether m is a mode.
func (m SimulateMode) Valid() bool {
	for _, v := range SimulateModeValues() {
		if v == m {
			return true
		}
	}
	return false
}

// SimulateCIFailureRequest is the body of POST /v1/cards/{id}/ci-failure. The card comes from the
// path, so the body carries only which mode to run.
type SimulateCIFailureRequest struct {
	// Mode is the mode to run.
	Mode SimulateMode `json:"mode"`
}

// SimulateCIFailureResult is what a simulated failure produced. Both modes answer with the same
// shape, so the screen that shows which one ran does not branch.
type SimulateCIFailureResult struct {
	// CardID is the card the failure was injected for.
	CardID string `json:"cardId"`
	// Mode is the mode that ran, so the audit line and the screen agree.
	Mode SimulateMode `json:"mode"`
	// Run is the failed run as the CI monitor now holds it.
	Run CiRun `json:"run"`
	// FixStarted is true when the failure was handed to the fix loop. It is false for a card that
	// had no session to send the log to, or one the loop limits already stopped.
	FixStarted bool `json:"fixStarted"`
	// Commit is the marked commit the real mode pushed. Empty in synthetic mode, which changes
	// nothing on the branch.
	Commit string `json:"commit"`
}
