package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// workflowRunner is local CI: it runs a card's own workflow steps on this machine.
type workflowRunner interface {
	Run(ctx context.Context, cardID, workflow string) (protocol.LocalCIResult, error)
}

// localChecks is how the merge queue runs a project's checks on a card's changes: local CI, over every
// workflow the card's worktree has. A project with no workflow files, or with only steps Marshal will
// not run here, has nothing to run, which is not a failure.
type localChecks struct{ ci workflowRunner }

// maxCheckOutput is how much of a failed step's output a card carries.
const maxCheckOutput = 1500

// CheckChanges runs the checks and says whether they passed, and what failed when they did not.
func (c localChecks) CheckChanges(ctx context.Context, cardID string) (integrator.CheckOutcome, error) {
	result, err := c.ci.Run(ctx, cardID, "")
	if err != nil {
		return integrator.CheckOutcome{}, err
	}
	var out integrator.CheckOutcome
	var failures []string
	for _, workflow := range result.Workflows {
		for _, step := range workflow.Steps {
			switch step.Status {
			case protocol.LocalCIStatusPassed:
				out.Ran = true
			case protocol.LocalCIStatusFailed:
				out.Ran = true
				failures = append(failures, failureText(workflow.Name, step))
			}
		}
	}
	out.Passed = out.Ran && len(failures) == 0
	out.Summary = strings.Join(failures, "\n\n")
	return out, nil
}

// failureText is one failed step: where it was, and the end of what it printed.
func failureText(workflow string, step protocol.LocalCIStep) string {
	name := strings.TrimSpace(step.Name)
	if name == "" {
		name = step.Command
	}
	text := fmt.Sprintf("%s: %s / %s failed", workflow, step.Job, name)
	if output := strings.TrimSpace(step.Output); output != "" {
		if r := []rune(output); len(r) > maxCheckOutput {
			output = "..." + string(r[len(r)-maxCheckOutput:])
		}
		text += ":\n" + output
	}
	return text
}

// commentPoster posts a comment the card's agent wrote.
type commentPoster interface {
	PostAsAgent(ctx context.Context, cardID, body string) (protocol.Comment, error)
}

// reviewNotes is how what the Reviewer found reaches the card's Comments tab.
type reviewNotes struct{ panel commentPoster }

// Note posts the note as a comment on the card.
func (n reviewNotes) Note(ctx context.Context, cardID, body string) error {
	_, err := n.panel.PostAsAgent(ctx, cardID, body)
	return err
}
