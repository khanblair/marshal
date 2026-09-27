package localci

import (
	"context"
	"errors"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The service is the layer between the route and the work: it finds the card's worktree, reads the
// workflow files there, decides step by step what Marshal can run, runs those, and answers what it
// found and what it refused. It publishes nothing and writes nothing: a local run is a read of the
// worktree plus commands that touch only the worktree, so there is no state to keep and no event
// to send. A person asking twice gets the same answer twice.

const (
	// DefaultMaxSteps is the most steps one request runs. A workflow with more has the rest of its
	// steps reported as not reached, because a person waiting on a hundred steps is not getting the
	// fast feedback this feature is for.
	DefaultMaxSteps = 50
)

// The answers for a run that cannot be made, in the words of docs/ui-rules.md.
const (
	// messageNoWorktree is answered for a card that has not started, which has nothing to run in.
	messageNoWorktree = "This card has no worktree yet, so there are no checks to run. Start the card first."
	// messageNoWorkflow is answered when the card's worktree has no workflow file by the asked name.
	messageNoWorkflow = "This card's worktree has no workflow file by that name. Check the name and try again."
)

// Projects is the part of the projects module this module needs: where a card's agent worked.
type Projects interface {
	// Worktree returns the folder and branch recorded for a card's worktree. Both are empty until
	// the card starts.
	Worktree(ctx context.Context, cardID string) (path, branch string, err error)
}

// Deps are the parts the service is built from. Projects is required.
type Deps struct {
	// Projects reads the card's worktree, which is where the workflow files are and where the
	// commands run.
	Projects Projects
	// Runner runs a step's command. Nil uses CommandRunner, which is what the daemon uses; a test
	// gives a fake that never starts a process.
	Runner Runner
	// Log is where a step that could not be started is written. Nil discards.
	Log *slog.Logger
	// Now is the clock. Nil uses the real one.
	Now func() time.Time
	// Timeout bounds one step's run. Zero takes DefaultStepTimeout.
	Timeout time.Duration
	// MaxBytes is how much of one step's output is kept. Zero takes DefaultOutputBytes.
	MaxBytes int
	// MaxLines is how many lines of one step's output are kept. Zero takes DefaultOutputLines.
	MaxLines int
	// MaxSteps is the most steps one request runs. Zero takes DefaultMaxSteps.
	MaxSteps int
}

// Service runs a card's workflow steps locally. It is safe for use by many goroutines: it keeps no
// state of its own beyond what it was built with.
type Service struct {
	projects Projects
	runner   Runner
	log      *slog.Logger
	now      func() time.Time
	maxSteps int
}

// New builds the service. Without the projects module there is no worktree to read or run in.
func New(deps Deps) (*Service, error) {
	if deps.Projects == nil {
		return nil, errors.New("localci: the projects module is required")
	}
	runner := deps.Runner
	if runner == nil {
		runner = NewCommandRunner(deps.Timeout, deps.MaxBytes, deps.MaxLines)
	}
	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	maxSteps := deps.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	return &Service{projects: deps.Projects, runner: runner, log: log, now: now, maxSteps: maxSteps}, nil
}

// Run reads the workflow files of a card's worktree and runs the steps Marshal can run there.
// `workflow` names one file, as it is named in `.github/workflows`; an empty name covers every
// workflow the worktree has. A worktree with no workflow files at all answers an empty result,
// which is the honest answer and not an error; a name that matches nothing is refused, because the
// person asked for something specific and must be told it is not there.
func (s *Service) Run(ctx context.Context, cardID, workflow string) (protocol.LocalCIResult, error) {
	dir, branch, err := s.projects.Worktree(ctx, cardID)
	if err != nil {
		return protocol.LocalCIResult{}, err
	}
	if strings.TrimSpace(dir) == "" {
		return protocol.LocalCIResult{}, protocol.Refused(messageNoWorktree)
	}
	list, err := ReadWorkflows(dir)
	if err != nil {
		return protocol.LocalCIResult{}, err
	}
	if want := strings.TrimSpace(workflow); want != "" {
		list = pickWorkflows(list, want)
		if len(list) == 0 {
			return protocol.LocalCIResult{}, protocol.Refused(messageNoWorkflow).With("workflow", want)
		}
	}
	budget := s.maxSteps
	out := make([]protocol.LocalCIWorkflow, 0, len(list))
	for _, w := range list {
		out = append(out, s.runWorkflow(ctx, dir, w, &budget))
	}
	return protocol.NewLocalCIResult(cardID, branch, out, s.now()), nil
}

// pickWorkflows keeps the workflows a name matches: the file as it is named in the folder ("ci.yml")
// or its whole path within the worktree (".github/workflows/ci.yml").
func pickWorkflows(list []Workflow, want string) []Workflow {
	var out []Workflow
	for _, w := range list {
		if w.File == want || path.Base(w.File) == want {
			out = append(out, w)
		}
	}
	return out
}

// runWorkflow runs every step of one workflow, job by job, and reports where the workflow ended.
func (s *Service) runWorkflow(ctx context.Context, dir string, w Workflow, budget *int) protocol.LocalCIWorkflow {
	out := protocol.LocalCIWorkflow{File: w.File, Name: w.Name}
	for _, job := range w.Jobs {
		// A job's steps stop at its first failure, exactly as GitHub runs a job. The other jobs of
		// the workflow keep going: they do not depend on this one.
		failed := false
		for _, step := range job.Steps {
			out.Steps = append(out.Steps, s.runStep(ctx, dir, w, job, step, &failed, budget))
		}
	}
	out.Status = workflowStatus(out.Steps)
	return out
}

// runStep decides one step and runs it when Marshal can. A step Marshal will not run answers the
// sentence that says why, and a step it runs answers what it printed.
func (s *Service) runStep(ctx context.Context, dir string, w Workflow, job Job, step Step, failed *bool, budget *int) protocol.LocalCIStep {
	item := protocol.LocalCIStep{Job: step.Job, Name: step.Name, Command: step.Run}
	decided := classify(step)
	item.Kind = decided.Kind
	switch {
	case job.Skip != "":
		// The whole job cannot run here, so nothing in it can. The job's own sentence says why,
		// which is more use than repeating a step's reason.
		item.Status, item.Reason = protocol.LocalCIStatusUnsupported, job.Skip
	case decided.Reason != "":
		item.Status, item.Reason = protocol.LocalCIStatusUnsupported, decided.Reason
	case *failed:
		item.Status, item.Reason = protocol.LocalCIStatusSkipped, reasonEarlierFailed
	case *budget <= 0:
		item.Status, item.Reason = protocol.LocalCIStatusSkipped, reasonTooManySteps
	default:
		*budget--
		s.run(ctx, dir, w, job, step, &item, failed)
	}
	return item
}

// run runs one step and fills in where it ended. A step that could not be started at all is a
// failure rather than a refusal: Marshal tried, and something on this machine stopped it.
func (s *Service) run(ctx context.Context, dir string, w Workflow, job Job, step Step, item *protocol.LocalCIStep, failed *bool) {
	result, err := s.runner.Run(ctx, RunRequest{
		Dir: dir, Shell: shellPath(step.Shell), Command: step.Run, Env: mergeEnv(w, job, step),
	})
	item.TookMs = result.Took.Milliseconds()
	if err != nil {
		s.log.WarnContext(ctx, "a workflow step could not be started", "step", step.Name, "error", err)
		item.Status = protocol.LocalCIStatusFailed
		item.Output = "Marshal could not start this step: " + err.Error()
		*failed = true
		return
	}
	item.Output = result.Output
	if result.Failed {
		item.Status = protocol.LocalCIStatusFailed
		*failed = true
		return
	}
	item.Status = protocol.LocalCIStatusPassed
}

// mergeEnv is the environment a step runs with: the workflow's, then the job's, then the step's own,
// in that order. A later entry wins, which is how GitHub resolves the same three, so a step that
// sets a value overrides the job's.
func mergeEnv(w Workflow, job Job, step Step) []string {
	env := make([]string, 0, len(w.Env)+len(job.Env)+len(step.Env))
	env = append(env, w.Env...)
	env = append(env, job.Env...)
	env = append(env, step.Env...)
	return env
}

// workflowStatus is where one workflow ended: failed when any step failed, unsupported when every
// step was one Marshal will not run, skipped when nothing ran at all, and otherwise passed.
func workflowStatus(steps []protocol.LocalCIStep) protocol.LocalCIStatus {
	if len(steps) == 0 {
		return protocol.LocalCIStatusSkipped
	}
	unsupported, ran := 0, 0
	for _, step := range steps {
		switch step.Status {
		case protocol.LocalCIStatusFailed:
			return protocol.LocalCIStatusFailed
		case protocol.LocalCIStatusUnsupported:
			unsupported++
		case protocol.LocalCIStatusPassed:
			ran++
		}
	}
	switch {
	case unsupported == len(steps):
		return protocol.LocalCIStatusUnsupported
	case ran == 0:
		return protocol.LocalCIStatusSkipped
	}
	return protocol.LocalCIStatusPassed
}
