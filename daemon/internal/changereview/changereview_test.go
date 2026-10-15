package changereview_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/changereview"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// asker answers a canned text and keeps what it was asked.
type asker struct {
	answer string
	err    error
	spec   agents.StartSpec
	prompt string
}

func (a *asker) Ask(_ context.Context, _ protocol.AgentKind, spec agents.StartSpec, prompt string) (string, error) {
	a.spec, a.prompt = spec, prompt
	return a.answer, a.err
}

type roles struct{ err error }

func (r roles) Role(context.Context, string, string) (protocol.Role, error) {
	return protocol.Role{Name: "Reviewer", Spec: protocol.RoleSpec{Instr: "Review the diff."}}, r.err
}

type git struct {
	diff string
	err  error
}

func (g git) Run(context.Context, string, ...string) (string, error) { return g.diff, g.err }

func review(t *testing.T, a *asker, g git) (integrator.ChangeVerdict, error) {
	t.Helper()
	r, err := changereview.New(a, roles{}, g)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r.ReviewChanges(context.Background(), integrator.ChangeReview{
		Card:     protocol.Card{ID: "c1", Key: "web#4", Title: "Add retry", Body: "Retry three times.", Agent: "claude"},
		Project:  protocol.Project{ID: "web"},
		Worktree: "/work/c1", Base: "main",
	})
}

func TestTheDiffAndTheTaskAreWhatTheModelIsAskedToReadAndItMayNotChangeAnything(t *testing.T) {
	a := &asker{answer: `{"approved": true, "summary": "Fine."}`}
	if _, err := review(t, a, git{diff: "+retry(3)\n"}); err != nil {
		t.Fatalf("ReviewChanges: %v", err)
	}
	for _, want := range []string{"web#4", "Retry three times.", "+retry(3)", "against main"} {
		if !strings.Contains(a.prompt, want) {
			t.Errorf("the prompt has no %q: %s", want, a.prompt)
		}
	}
	if a.spec.Cwd != "/work/c1" || a.spec.Instructions != "Review the diff." {
		t.Errorf("spec = %+v, want the card's folder and the role's instructions", a.spec)
	}
	for _, tool := range []string{"Edit", "Write"} {
		found := false
		for _, denied := range a.spec.DisallowedTools {
			found = found || denied == tool
		}
		if !found {
			t.Errorf("%s is not denied: %v", tool, a.spec.DisallowedTools)
		}
	}
}

func TestAnApprovalAndARequestForChangesAreBothRead(t *testing.T) {
	approved, err := review(t, &asker{answer: `{"approved": true, "summary": "Looks right."}`}, git{diff: "+x"})
	if err != nil || !approved.Approved || approved.Summary != "Looks right." {
		t.Errorf("approval = %+v, %v", approved, err)
	}
	// A model that wraps its answer in a code fence is still read.
	wrapped := "```json\n{\"approved\": false, \"summary\": \"No limit.\", \"comments\": [{\"path\": \"a.go\", \"line\": 7, \"body\": \"Stop after three.\"}, {\"body\": \"  \"}]}\n```"
	changes, err := review(t, &asker{answer: wrapped}, git{diff: "+x"})
	if err != nil || changes.Approved || len(changes.Comments) != 1 || changes.Comments[0].Path != "a.go" || changes.Comments[0].Line != 7 {
		t.Errorf("request for changes = %+v, %v, want one comment on a.go:7 and the empty one dropped", changes, err)
	}
}

// An answer that is not a whole verdict is an error and never an approval: a review that could not
// be read must not let a card through.
func TestAnAnswerThatIsNotAVerdictIsNeverAnApproval(t *testing.T) {
	for name, answer := range map[string]string{
		"prose":            "Looks good to me!",
		"no approved flag": `{"summary": "ok"}`,
		"broken json":      `{"approved": tru`,
	} {
		t.Run(name, func(t *testing.T) {
			verdict, err := review(t, &asker{answer: answer}, git{diff: "+x"})
			if err == nil || verdict.Approved {
				t.Errorf("verdict = %+v, %v, want an error and no approval", verdict, err)
			}
		})
	}
}

func TestAReviewThatCannotStartOrHasNothingToReadIsAnError(t *testing.T) {
	if _, err := review(t, &asker{err: errors.New("Claude is signed out")}, git{diff: "+x"}); err == nil || !strings.Contains(err.Error(), "signed out") {
		t.Errorf("err = %v, want the agent's own words", err)
	}
	if _, err := review(t, &asker{answer: `{"approved": true}`}, git{diff: "  \n"}); err == nil {
		t.Error("a card with no changes was reviewed")
	}
}
