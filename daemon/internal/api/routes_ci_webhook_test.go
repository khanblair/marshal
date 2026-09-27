package api_test

// The replayed-webhook half of the CI monitor (docs/architecture.md sections 9 and 11.2,
// docs/backend-checklist.md B6.2, build-plan 6.2, definition of done in the phase brief). The read
// route is proven in routes_ci_test.go, the monitor's own loop over a real store in internal/ci, and
// the route's signature check in routes_webhook_test.go. This file joins them: a recorded
// workflow_run delivery, replayed at the route with the secret the daemon holds, reaches the CI
// monitor through the connections service, records the failed run against the card whose branch it
// is on, writes the card's badge, and answers the CI health Home's section reads.
//
// Nothing here dials GitHub: the body and its headers are the recorded fixture's own, and the
// signature is computed with the daemon's secret the way tools/hooks-replay computes it.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// hookRecording is one saved delivery: the headers it came with, and its body exactly as recorded.
// It is the shape tools/hooks-replay reads, so the fixture one side replays is the fixture the other
// side is tested against.
type hookRecording struct {
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

// loadHook reads a recorded delivery from daemon/testdata/hooks/github.
func loadHook(t *testing.T, name string) hookRecording {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "hooks", "github", name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the %s recording: %v", name, err)
	}
	var rec hookRecording
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("read the %s recording: %v", name, err)
	}
	if len(rec.Body) == 0 {
		t.Fatalf("the %s recording has no body", name)
	}
	return rec
}

// nameBranch gives a card a worktree on the exact branch a recording names, so a replayed delivery
// resolves to the card. It is startWorktree with the branch taken from the fixture rather than
// derived from the card, because the recording's branch was fixed when it was recorded.
func (st *stack) nameBranch(t *testing.T, project protocol.Project, repo string, card protocol.Card, branch string) {
	t.Helper()
	ctx := context.Background()
	dir := filepath.Join(projects.WorktreesDir(st.dataDir, project.ID), card.ID)
	spec := gitx.WorktreeSpec{Path: dir, Branch: branch, Base: project.DefaultBranch}
	if err := st.git.AddWorktree(ctx, repo, spec); err != nil {
		t.Fatalf("make a worktree for card %s: %v", card.ID, err)
	}
	if _, err := st.proj.SetWorktree(ctx, card.ID, dir, branch); err != nil {
		t.Fatalf("record the worktree of card %s: %v", card.ID, err)
	}
}

func TestAReplayedWorkflowRunUpdatesTheCardsBadgeAndTheCIHealth(t *testing.T) {
	const secret = "s3cr3t"
	st := newStack(t, withWebhooks(secret))
	project, repo := st.addProject("small-repo")
	// The recording is a delivery from marshal-fixtures/small-repo, so the project has to be that
	// repository for the delivery to resolve to it: the monitor matches a run by the project's
	// origin remote, exactly as the pull-request and review services do.
	st.gitOutput(repo, "remote", "add", "origin", "https://github.com/marshal-fixtures/small-repo.git")
	card := st.addCard(project.ID, "CI fails on this branch")

	hook := loadHook(t, "workflow-run")
	var payload struct {
		WorkflowRun struct {
			Branch string `json:"head_branch"`
		} `json:"workflow_run"`
	}
	if err := json.Unmarshal(hook.Body, &payload); err != nil {
		t.Fatalf("read the recording's run: %v", err)
	}
	st.nameBranch(t, project, repo, card, payload.WorkflowRun.Branch)

	st.deliver(t, map[string]string{
		githubapp.SignatureHeader: webhookSignature(secret, hook.Body),
		githubapp.EventHeader:     hook.Headers[githubapp.EventHeader],
		githubapp.DeliveryHeader:  hook.Headers[githubapp.DeliveryHeader],
	}, hook.Body).want(t, http.StatusOK)

	// The badge follows the forge without a person asking: the card's own CI state is the run's.
	after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
	if after.CI == nil || *after.CI != protocol.CIStateFailed {
		t.Fatalf("the card's CI = %v, want failed after the replayed failure", after.CI)
	}

	// Home's CI health holds the run, named for the project, the card, the branch, and the workflow.
	snapshot := decode[protocol.CISnapshot](t, st.do(http.MethodGet, "/v1/ci", nil).want(t, http.StatusOK))
	if len(snapshot.Projects) != 1 {
		t.Fatalf("the CI health = %+v, want the one project", snapshot.Projects)
	}
	projectCI := snapshot.Projects[0]
	if projectCI.ProjectID != project.ID || projectCI.Status != protocol.CIStateFailed {
		t.Errorf("the project's CI = %+v, want %s failed", projectCI, project.ID)
	}
	if len(projectCI.Runs) != 1 {
		t.Fatalf("the project's runs = %+v, want the one replayed run", projectCI.Runs)
	}
	run := projectCI.Runs[0]
	if run.ID != "1531" || run.CardID != card.ID || run.Branch != payload.WorkflowRun.Branch || run.Workflow != "ci" {
		t.Errorf("the run = %+v, want the recorded run against the card", run)
	}
	if run.URL != "https://github.com/marshal-fixtures/small-repo/actions/runs/1531" {
		t.Errorf("the run's url = %q, want the recording's own", run.URL)
	}
}
