package integrator_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrator"
)

// The prompt is checked through the real path: the text the chat is sent for a task. The checks are
// for the clauses the agent must be told, not for the exact words around them.

// agentPromptFor resolves a task against an agent that reports at once, and answers what the chat was sent.
func agentPromptFor(t *testing.T, task integrator.MergeTask) string {
	t.Helper()
	h := newAgentHarness(t, nil)
	h.reportOnSend(task.ID, agentResolved())
	if _, err := h.r.Resolve(context.Background(), task); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return h.chat.lastText()
}

func agentWantAll(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(text, part) {
			t.Errorf("the prompt lacks %q:\n%s", part, text)
		}
	}
}

func agentWantNone(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if strings.Contains(text, part) {
			t.Errorf("the prompt has %q:\n%s", part, text)
		}
	}
}

func TestPromptForACardTask(t *testing.T) {
	text := agentPromptFor(t, agentTask("task-9", "proj-1"))
	agentWantAll(t, text,
		"You are the Integrator for project proj-1",
		"merge task task-9",
		"integrator workspace, /data/integrator/proj-1",
		"Do not read or change the owner's project folder",
		"Merging card PROJ#12", "branch marshal/proj-12", "into development",
		"- src/a.go", "- src/b.go",
		"PROJ#13", "Add logout",
		`merge_context with {"task_id": "task-9"}`,
		"Read BOTH sides", "task and plan, and its diff",
		"by intent", "no conflict markers", "git add",
		"Do NOT commit, switch branches, push", "moves a branch",
		"merge --abort or --continue",
		"merge_report with task_id, resolved, confident, summary, files, and questions",
		"set confident to false and put your questions in questions",
		"ask_owner with task_id and question",
		"Report only for task task-9",
	)
	agentWantNone(t, text, "UNCOMMITTED WORK", "prefer the owner's version", "told the Integrator chat")
}

func TestPromptForTheOwnersUncommittedWork(t *testing.T) {
	task := agentTask("task-9", "proj-1")
	task.Kind = integrator.MergeTaskWIP
	text := agentPromptFor(t, task)
	agentWantAll(t, text,
		"You are the Integrator for project proj-1",
		"THE OWNER'S UNCOMMITTED WORK, not a card",
		"development just moved forward", "waiting in your workspace",
		"preserve both sides",
		"prefer the owner's version", "say so in your summary",
		"Never stash, reset, restore, clean or check out",
		"Do NOT commit, switch branches, push",
		"merge_report",
	)
	agentWantNone(t, text, "stopped on conflicts")
}

func TestPromptCarriesTheOwnersInstructionWhenThereIsOne(t *testing.T) {
	task := agentTask("t1", "p1")
	task.Instruction = "  Keep the new logout page.  "
	agentWantAll(t, agentPromptFor(t, task), "The owner told the Integrator chat this about the merge:", "Keep the new logout page.")
}

func TestPromptWithNoCardsOrConflictsListedStillTellsTheAgentWhatToDo(t *testing.T) {
	task := agentTask("t1", "p1")
	task.Cards, task.Conflicts, task.Target = nil, nil, ""
	text := agentPromptFor(t, task)
	agentWantAll(t, text, "Merging a card's branch into the integration branch", "run git status in the workspace")
	agentWantNone(t, text, "Cards involved")
}
