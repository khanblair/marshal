package review

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/github"
)

// ChecksReviewer is the Reviewer the daemon ships in Phase 5. The Reviewer role's own instruction
// is "Review the diff against the card's task and acceptance checks. Leave specific comments.
// Approve only when every check passes", and this reads that half of it: the checks the forge
// reports on the pull request's own branch.
//
// It answers three ways. Every check that exists passed, so it approves. A check finished and did
// not pass, so it asks for changes and names each one. Or the checks are still running, so it waits
// and the card is left exactly where it is. A pull request with no checks at all waits too: nothing
// ran, so nothing was approved.
//
// The other half of the role, reading the diff and writing prose about it, needs a model. It is a
// second implementation of Reviewer, and nothing in the service changes when it arrives.
type ChecksReviewer struct {
	client github.Client
}

// NewChecksReviewer builds a ChecksReviewer. The forge client is required.
func NewChecksReviewer(client github.Client) (*ChecksReviewer, error) {
	if client == nil {
		return nil, fmt.Errorf("the checks reviewer needs a forge client")
	}
	return &ChecksReviewer{client: client}, nil
}

// Review reads the checks on the pull request's branch and answers with a verdict.
func (r *ChecksReviewer) Review(ctx context.Context, req Request) (Verdict, error) {
	checks, err := r.client.ListChecks(ctx, req.Target.Repo, req.Target.Head)
	if err != nil {
		return Verdict{}, err
	}
	name := roleName(req.Role)
	if len(checks) == 0 {
		return Verdict{Waiting: true, Summary: fmt.Sprintf(
			"%s is waiting: pull request #%d has no checks yet.", name, req.Target.Number)}, nil
	}
	var failed []github.Check
	waiting := false
	for _, check := range checks {
		if check.Status != statusCompleted {
			waiting = true
			continue
		}
		if !check.Passed() {
			failed = append(failed, check)
		}
	}
	if len(failed) > 0 {
		return Verdict{
			Summary: fmt.Sprintf("%s asked for changes: %s did not pass on pull request #%d.",
				name, checkNames(failed), req.Target.Number),
			Comments: failureComments(failed),
		}, nil
	}
	if waiting {
		// Reached only with no check having failed, so what is left is a check still running.
		return Verdict{Waiting: true, Summary: fmt.Sprintf(
			"%s is waiting: the checks on pull request #%d are still running.", name, req.Target.Number)}, nil
	}
	return Verdict{Approved: true, Summary: fmt.Sprintf(
		"%s approved. All %s passed on pull request #%d.", name, count(checks, "check"), req.Target.Number)}, nil
}

// statusCompleted is the status a check has when it has finished.
const statusCompleted = "completed"

// failureComments is one comment per check that did not pass, naming the check and what it
// answered, so the failing check is readable on the pull request itself.
func failureComments(failed []github.Check) []Comment {
	out := make([]Comment, 0, len(failed))
	for _, check := range failed {
		body := fmt.Sprintf("`%s` did not pass on this pull request (%s).", check.Name, conclusion(check))
		if check.URL != "" {
			body += " " + check.URL
		}
		out = append(out, Comment{Body: body})
	}
	return out
}

// conclusion is a check's result, naming the one case where the forge says a check did not run.
func conclusion(check github.Check) string {
	if strings.TrimSpace(check.Conclusion) == "" {
		return "no result"
	}
	return check.Conclusion
}

// checkNames lists the checks that did not pass, cut to a length a person reads in one line.
func checkNames(failed []github.Check) string {
	names := make([]string, 0, len(failed))
	for _, check := range failed {
		name := strings.TrimSpace(check.Name)
		if name == "" {
			name = "a check"
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// count is "1 check" and "3 checks", so a sentence a person reads is never "1 checks".
func count(checks []github.Check, noun string) string {
	if len(checks) == 1 {
		return fmt.Sprintf("%d %s", len(checks), noun)
	}
	return fmt.Sprintf("%d %ss", len(checks), noun)
}
