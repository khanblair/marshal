package session_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// allowPushPolicy answers the same for every project.
type allowPushPolicy bool

func (p allowPushPolicy) AllowPush(context.Context, string) bool { return bool(p) }

// A card's agent may stage and commit its own work, and pushing is refused: nothing it does leaves
// the machine unless the project says so.
func TestACardsAgentMayCommitButNotPushByDefault(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	spec := e.agent.startSpecs()[0]
	for _, want := range []string{"Bash(git add:*)", "Bash(git commit:*)"} {
		if !slices.Contains(spec.AllowedTools, want) {
			t.Errorf("allowed tools = %v, want %s", spec.AllowedTools, want)
		}
	}
	if !slices.Contains(spec.DisallowedTools, "Bash(git push:*)") {
		t.Errorf("disallowed tools = %v, want git push refused", spec.DisallowedTools)
	}
}

func TestAProjectThatAllowsPushingDoesNotRefuseIt(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.mgr.SetPushPolicy(allowPushPolicy(true))
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if spec := e.agent.startSpecs()[0]; len(spec.DisallowedTools) != 0 {
		t.Errorf("disallowed tools = %v, want none when the project allows pushing", spec.DisallowedTools)
	}
}

// Whatever an agent leaves uncommitted when its turn ends is committed for it, so the card's branch
// always holds the work and a review or a merge reads the same thing.
func TestTheWorkLeftAtTheEndOfATurnIsCommittedOnTheCardsBranch(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a note")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	tree := worktreeOf(t, e, card.ID)
	// The turn is held open so the file is written while the agent is working, after the restore
	// point that is made as the turn begins.
	hold := make(chan struct{})
	e.agent.mu.Lock()
	e.agent.hold = hold
	e.agent.mu.Unlock()
	if err := e.mgr.Send(ctx, card.ID, "write the note"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWorking)
	before, err := e.git.Run(ctx, tree, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("read the card's head: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "note.md"), []byte("# Note\n"), 0o600); err != nil {
		t.Fatalf("write the agent's file: %v", err)
	}
	hold <- struct{}{}
	waitForIdle(t, e, card.ID)

	waitFor(t, "the card's work to be committed", func() bool {
		after, err := e.git.Run(ctx, tree, "rev-parse", "HEAD")
		return err == nil && after != before
	})
	status, err := e.git.Run(ctx, tree, "status", "--porcelain")
	if err != nil || strings.TrimSpace(status) != "" {
		t.Errorf("status = %q, %v, want a clean worktree after the turn", status, err)
	}
	subject, _ := e.git.Run(ctx, tree, "log", "-1", "--format=%s")
	if !strings.HasPrefix(strings.TrimSpace(subject), "Marshal: Add a note (after turn ") {
		t.Errorf("the commit's subject = %q, want it to name the card", subject)
	}
}

// Ask puts a question to a fresh agent and answers what it said, then lets the agent go.
func TestAskAnswersWhatAFreshAgentSaid(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })

	text, err := e.mgr.Ask(context.Background(), "claude", agentsSpec(t), "Is this change safe?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(text, "Is this change safe?") {
		t.Errorf("the answer was %q, want the fake agent's echo of the question", text)
	}
}

// agentsSpec is a spec for a question: a folder to stand in, and nothing else.
func agentsSpec(t *testing.T) agents.StartSpec {
	t.Helper()
	return agents.StartSpec{Cwd: t.TempDir(), Label: "ask"}
}

// finishRecorder keeps the cards its agents finished.
type finishRecorder struct {
	mu   sync.Mutex
	done []string
}

func (r *finishRecorder) CardFinished(_ context.Context, cardID, summary string) {
	r.mu.Lock()
	r.done = append(r.done, cardID+": "+summary)
	r.mu.Unlock()
}

func (r *finishRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.done...)
}

// An agent that says it is finished has its card handed on when its turn ends, and not before: its
// last commit is made first, and a turn that never said it was finished hands nothing on.
func TestACardItsAgentSaidWasFinishedIsHandedOnWhenTheTurnEnds(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = e.mgr.Close() })
	rec := &finishRecorder{}
	e.mgr.SetFinishHandler(rec)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a note")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	hold := make(chan struct{})
	e.agent.mu.Lock()
	e.agent.hold = hold
	e.agent.mu.Unlock()
	if err := e.mgr.Send(ctx, card.ID, "do it"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWorking)
	if err := e.mgr.Finish(ctx, card.ID, "Added the note."); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := rec.seen(); len(got) != 0 {
		t.Fatalf("the card was handed on mid-turn: %v", got)
	}
	hold <- struct{}{}
	waitForIdle(t, e, card.ID)
	waitFor(t, "the finished card to be handed on", func() bool { return len(rec.seen()) == 1 })
	if got := rec.seen()[0]; got != card.ID+": Added the note." {
		t.Errorf("handed on %q, want the card and the agent's summary", got)
	}

	// The next turn did not say it was finished, so nothing more is handed on.
	e.agent.mu.Lock()
	e.agent.hold = nil
	e.agent.mu.Unlock()
	if err := e.mgr.Send(ctx, card.ID, "one more thing"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForIdle(t, e, card.ID)
	if got := rec.seen(); len(got) != 1 {
		t.Errorf("handed on %v, want only the turn that said it was finished", got)
	}
}

// A turn that ended on a command Claude refused for lack of a permission leaves the card in Needs you
// saying so, instead of idle in Working with nothing on the board to say the agent is stuck.
func TestATurnThatEndedOnARefusedCommandAlertsThePerson(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Run the tests")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.agent.setTurnScript(
		agents.ToolCall{ID: "t1", Title: "Bash: npm test", Kind: "execute", Status: agents.StatusPending},
		agents.ToolCallUpdate{ID: "t1", Status: agents.StatusFailed, Content: "Permission for this tool use was denied. It requires approval."},
	)
	if err := e.mgr.Send(ctx, card.ID, "run the tests"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitFor(t, "the card to say it is blocked", func() bool {
		got, err := e.proj.Card(ctx, card.ID)
		return err == nil && got.State == protocol.CardStateNeeds && got.NeedsReason != nil &&
			strings.Contains(got.NeedsReason.Text, "Bash: npm test was refused")
	})
}

func TestARefusalTheAgentWorkedAroundDoesNotAlertAnyone(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Run the tests")
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	e.agent.setTurnScript(
		agents.ToolCall{ID: "t1", Title: "Bash: npm test", Status: agents.StatusPending},
		agents.ToolCallUpdate{ID: "t1", Status: agents.StatusFailed, Content: "Permission for this tool use was denied."},
		agents.ToolCall{ID: "t2", Title: "Read package.json", Status: agents.StatusPending},
		agents.ToolCallUpdate{ID: "t2", Status: agents.StatusCompleted, Content: "{}"},
	)
	if err := e.mgr.Send(ctx, card.ID, "run the tests"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitForIdle(t, e, card.ID)
	got, err := e.proj.Card(ctx, card.ID)
	if err != nil || got.State == protocol.CardStateNeeds {
		t.Errorf("card = %s, %v, want it left in Working: the agent went on after the refusal", got.State, err)
	}
}
