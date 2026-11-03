// Package changereview is the Reviewer that reads a card's changes on this machine: the diff of its
// branch against the project's integration branch, read by a model that may look at the files but
// change none of them. It answers with a verdict the merge queue acts on (internal/integrator's
// ChangeReviewer), so a card is reviewed without a pull request, a forge, or a push.
//
// The model is asked for one JSON object and nothing else. An answer that is not that object is an
// error, never an approval: a review that could not be read must not let a card through.
package changereview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// RoleName is the starter role that reads changes.
const RoleName = "Reviewer"

const (
	// maxDiffBytes caps the diff put in the prompt. A bigger one is cut, and the prompt says so, so the
	// model reads the rest from the files.
	maxDiffBytes = 120_000
	// reviewTimeout bounds one review.
	reviewTimeout = 10 * time.Minute
)

// Asker puts one question to a fresh agent and answers what it said. The session manager is one.
type Asker interface {
	Ask(ctx context.Context, kind protocol.AgentKind, spec agents.StartSpec, prompt string) (string, error)
}

// Roles reads a role by name, the project's own version when it keeps one.
type Roles interface {
	Role(ctx context.Context, name, projectID string) (protocol.Role, error)
}

// Git runs a Git command in a folder and answers what it printed.
type Git interface {
	Run(ctx context.Context, dir string, args ...string) (string, error)
}

// Reviewer reads a card's changes with a model.
type Reviewer struct {
	asker Asker
	roles Roles
	git   Git
}

var _ integrator.ChangeReviewer = (*Reviewer)(nil)

// New builds a Reviewer. All three parts are required.
func New(asker Asker, roles Roles, git Git) (*Reviewer, error) {
	if asker == nil || roles == nil || git == nil {
		return nil, errors.New("the change reviewer needs an agent to ask, the roles, and Git")
	}
	return &Reviewer{asker: asker, roles: roles, git: git}, nil
}

// ReviewChanges reads the card's committed changes against the base branch and answers with a verdict.
func (r *Reviewer) ReviewChanges(ctx context.Context, req integrator.ChangeReview) (integrator.ChangeVerdict, error) {
	ctx, cancel := context.WithTimeout(ctx, reviewTimeout)
	defer cancel()
	role, err := r.roles.Role(ctx, RoleName, req.Project.ID)
	if err != nil {
		return integrator.ChangeVerdict{}, fmt.Errorf("this project has no %s role: %w", RoleName, err)
	}
	args := []string{"diff", req.Base + "...HEAD"}
	if req.Head != "" {
		args = []string{"diff", req.Base, req.Head, "--"}
	}
	diff, err := r.git.Run(ctx, req.Worktree, args...)
	if err != nil {
		return integrator.ChangeVerdict{}, fmt.Errorf("read the card's changes: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return integrator.ChangeVerdict{}, errors.New("the card has no changes against " + req.Base + " to review")
	}
	answer, err := r.asker.Ask(ctx, req.Card.Agent, specFor(req, role), promptFor(req, diff))
	if err != nil {
		return integrator.ChangeVerdict{}, err
	}
	return parseVerdict(answer)
}

// specFor is how the reviewing agent is started: in the card's folder, as the Reviewer role, able to
// read the files and run Git's read-only commands, and unable to change anything.
func specFor(req integrator.ChangeReview, role protocol.Role) agents.StartSpec {
	model := req.Card.Model
	return agents.StartSpec{
		Cwd: req.Worktree, Model: model, PermissionMode: string(protocol.PermissionModeAsk),
		Instructions: role.Spec.Instr, Label: "review " + req.Card.ID,
		AllowedTools:    []string{"Read", "Grep", "Glob", "Bash(git status:*)", "Bash(git diff:*)", "Bash(git log:*)", "Bash(git show:*)"},
		DisallowedTools: []string{"Edit", "Write", "NotebookEdit"},
	}
}

// promptFor is the one message the reviewing agent is sent.
func promptFor(req integrator.ChangeReview, diff string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review the changes of card %s, \"%s\".\n\n", req.Card.Key, req.Card.Title)
	if body := strings.TrimSpace(req.Card.Body); body != "" {
		b.WriteString("The card's task:\n" + body + "\n\n")
	}
	fmt.Fprintf(&b, "Below is the diff of the card's branch against %s. You are in the card's folder, so you may read\n"+
		"any file to see the code around a change. Do not change anything.\n\n", req.Base)
	if len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes] + "\n[the diff was cut here; read the rest with git diff]"
	}
	b.WriteString("```diff\n" + diff + "\n```\n\n")
	b.WriteString("Judge whether the changes do what the task asks and whether anything in them must change before they\n" +
		"are merged: a bug, a missing case, a broken test, something unsafe, or code outside the task.\n\n" +
		"Reply with ONLY one JSON object, with no text before or after it:\n" +
		"{\"approved\": true or false, \"summary\": \"one or two plain sentences\", " +
		"\"comments\": [{\"path\": \"file\", \"line\": 12, \"body\": \"what to change\"}]}\n" +
		"Set approved to true only when you found nothing that must change. When it is false, give specific comments.")
	return b.String()
}

// verdictJSON is the object the model is asked for.
type verdictJSON struct {
	Approved *bool  `json:"approved"`
	Summary  string `json:"summary"`
	Comments []struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Body string `json:"body"`
	} `json:"comments"`
}

// parseVerdict reads the object out of what the model said. It takes the text from the first "{" to
// the last "}", so a model that wraps its answer in a code fence is still read; anything that is not a
// whole object with an approved flag is an error.
func parseVerdict(answer string) (integrator.ChangeVerdict, error) {
	start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	if start < 0 || end <= start {
		return integrator.ChangeVerdict{}, fmt.Errorf("the Reviewer's answer could not be read: %s", cut(answer))
	}
	var parsed verdictJSON
	if err := json.Unmarshal([]byte(answer[start:end+1]), &parsed); err != nil || parsed.Approved == nil {
		return integrator.ChangeVerdict{}, fmt.Errorf("the Reviewer's answer could not be read: %s", cut(answer))
	}
	verdict := integrator.ChangeVerdict{Approved: *parsed.Approved, Summary: strings.TrimSpace(parsed.Summary)}
	for _, c := range parsed.Comments {
		if strings.TrimSpace(c.Body) == "" {
			continue
		}
		verdict.Comments = append(verdict.Comments, integrator.ChangeComment{Path: c.Path, Line: c.Line, Body: strings.TrimSpace(c.Body)})
	}
	return verdict, nil
}

// cut is a text shortened to what an error message carries.
func cut(text string) string {
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > 200 {
		return string(r[:200]) + "..."
	}
	return text
}
