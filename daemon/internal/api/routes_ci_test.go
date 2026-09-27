package api_test

// The CI route (docs/architecture.md sections 9 and 11.1, docs/backend-checklist.md B6.2 and B6.6,
// build-plan 6.2): one read of every project Marshal holds a workflow run for. internal/ci is the
// only writer of a run, and its own tests drive the loop over a webhook delivery, a polling backup,
// and the loop limits; this file tests the wire - the order of the projects and of the runs inside
// one, what a queued run's start time is, and that a project with no run is left out rather than
// answered with an empty list.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// seedRun writes one run straight into the store, the way internal/ci writes one when a delivery
// arrives: the row is a workflow's state on one branch and not a history.
func seedRun(t *testing.T, st *stack, params db.UpsertCiRunParams) {
	t.Helper()
	err := st.store.Write(context.Background(), func(q *db.Queries) error {
		return q.UpsertCiRun(context.Background(), params)
	})
	if err != nil {
		t.Fatalf("seed run %s: %v", params.ID, err)
	}
}

// A project Marshal holds a run for answers it, newest first, with the project's status taken from
// the newest run, a queued run's start time null, and the daemon's time stamped on the answer so a
// client counts ages from it.
func TestTheCIRouteAnswersEveryProjectThatHasARun(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Watch my CI")
	other := st.addCard(project.ID, "Also has a run")
	branch := "marshal/" + card.ID
	otherBranch := "marshal/" + other.ID
	older, newer := int64(1_700_000_000_000), int64(1_700_000_060_000)

	seedRun(t, st, db.UpsertCiRunParams{
		ID: "1531", ProjectID: project.ID, CardID: &card.ID, Branch: branch,
		Workflow: "ci", Status: string(protocol.CIStatePassed),
		Url: "https://github.com/o/r/actions/runs/1531", StartedAt: older, UpdatedAt: older,
	})
	seedRun(t, st, db.UpsertCiRunParams{
		ID: "1532", ProjectID: project.ID, CardID: &other.ID, Branch: otherBranch,
		Workflow: "ci", Status: string(protocol.CIStateRunning),
		Url: "https://github.com/o/r/actions/runs/1532", StartedAt: 0, UpdatedAt: newer,
	})

	got := decode[protocol.CISnapshot](t, st.do(http.MethodGet, "/v1/ci", nil).want(t, http.StatusOK))
	if len(got.Projects) != 1 {
		t.Fatalf("the snapshot = %+v, want one project", got.Projects)
	}
	ci := got.Projects[0]
	if ci.ProjectID != project.ID {
		t.Errorf("project = %q, want %q", ci.ProjectID, project.ID)
	}
	if ci.Status != protocol.CIStateRunning {
		t.Errorf("the project's status = %q, want running (the newest run)", ci.Status)
	}
	if len(ci.Runs) != 2 || ci.Runs[0].ID != "1532" || ci.Runs[1].ID != "1531" {
		t.Fatalf("the runs = %+v, want 1532 then 1531", ci.Runs)
	}
	if ci.Runs[0].CardID != other.ID || ci.Runs[0].Branch != otherBranch || ci.Runs[0].Workflow != "ci" {
		t.Errorf("the newest run = %+v, want it named for the card, the branch, and the workflow", ci.Runs[0])
	}
	if ci.Runs[0].StartedAt != nil {
		t.Errorf("the running run's startedAt = %v, want null while it has not started", ci.Runs[0].StartedAt)
	}
	if ci.Runs[1].StartedAt == nil || ci.Runs[1].StartedAt.Time().UnixMilli() != older {
		t.Errorf("the passed run's startedAt = %v, want %d", ci.Runs[1].StartedAt, older)
	}
	if got.ServerTime.Time().IsZero() {
		t.Error("the answer carries no server time")
	}
}

// A daemon nobody has connected GitHub to, and a project with no run, answer no projects at all, and
// the bytes are the empty array, so a screen draws "GitHub is not connected" without a special case
// for a missing list.
func TestTheCIRouteLeavesOutProjectsWithNoRuns(t *testing.T) {
	st := newStack(t)
	st.addProject("small-repo")

	r := st.do(http.MethodGet, "/v1/ci", nil).want(t, http.StatusOK)
	got := decode[protocol.CISnapshot](t, r)
	if len(got.Projects) != 0 {
		t.Errorf("the snapshot = %+v, want no projects", got.Projects)
	}
	if body := string(r.Body); !strings.Contains(body, `"projects":[]`) {
		t.Errorf("the answer = %s, want an empty projects array", body)
	}
}

// startedCard makes a project that resolves to a repository and a card that has been started, which
// is the only shape a simulated failure has anything to work with: the monitor matches a run by the
// project's origin remote, exactly as the pull-request and review services do, and a card with no
// branch has no run to fail on.
func (st *stack) startedCard(t *testing.T) (protocol.Project, protocol.Card) {
	t.Helper()
	project, repo := st.addProject("small-repo")
	st.gitOutput(repo, "remote", "add", "origin", "https://github.com/marshal-fixtures/small-repo.git")
	card := st.addCard(project.ID, "CI fails on this branch")
	st.startWorktree(t, project, repo, card)
	// The card is read back because starting it is what gives it its branch.
	return project, decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
}

// The synthetic mode is the whole point of the route on a dev machine: one call makes a failed run
// on a card's branch, the fix loop answers it as it answers one GitHub reported, the card's badge
// and Home's CI health both move, and the one audit row says who asked for it and how.
func TestSimulatingACIFailureInjectsARunAndRecordsIt(t *testing.T) {
	st := newStack(t)
	project, card := st.startedCard(t)

	got := decode[protocol.SimulateCIFailureResult](t, st.do(http.MethodPost,
		"/v1/cards/"+card.ID+"/ci-failure",
		protocol.SimulateCIFailureRequest{Mode: protocol.SimulateModeSynthetic}).want(t, http.StatusOK))

	if got.CardID != card.ID || got.Mode != protocol.SimulateModeSynthetic {
		t.Fatalf("the answer = %+v, want the synthetic mode on card %s", got, card.ID)
	}
	if got.Commit != "" {
		t.Errorf("the synthetic mode answered the commit %q; it changes nothing on the branch", got.Commit)
	}
	if got.Run.ID == "" || got.Run.CardID != card.ID || got.Run.Branch != card.Branch {
		t.Errorf("the answered run = %+v, want it named for the card and its branch", got.Run)
	}
	// The run is Marshal's own and not a project workflow's, so it can never be mistaken for one the
	// project really ran.
	if got.Run.Workflow != "simulated-ci" || got.Run.Status != protocol.CIStateFailed {
		t.Fatalf("the answered run = %+v, want a failed simulated-ci run", got.Run)
	}

	after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
	if after.CI == nil || *after.CI != protocol.CIStateFailed {
		t.Errorf("the card's CI = %v, want failed after the simulated failure", after.CI)
	}

	snapshot := decode[protocol.CISnapshot](t, st.do(http.MethodGet, "/v1/ci", nil).want(t, http.StatusOK))
	if len(snapshot.Projects) != 1 || snapshot.Projects[0].ProjectID != project.ID {
		t.Fatalf("the CI health = %+v, want the one project", snapshot.Projects)
	}
	if runs := snapshot.Projects[0].Runs; len(runs) != 1 || runs[0].ID != got.Run.ID {
		t.Fatalf("the project's runs = %+v, want the injected run %s", runs, got.Run.ID)
	}

	rows, err := st.store.Queries().ListAuditLog(context.Background(), 10)
	if err != nil {
		t.Fatalf("read the audit log: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(rows))
	}
	if rows[0].Action != audit.ActionCISimulated || rows[0].Actor != audit.ActorPerson {
		t.Errorf("the audit row = %+v, want a person's %s", rows[0], audit.ActionCISimulated)
	}
	if rows[0].Target != card.ID || !strings.Contains(rows[0].DetailJSON, `"mode":"synthetic"`) {
		t.Errorf("the audit row = %+v, want it naming the card and the mode", rows[0])
	}
}

// A normal daemon has no such address at all: the real mode spends Actions minutes on the person's
// own GitHub account, so the route exists only on a dev daemon, exactly as the dev reset does.
func TestTheSimulatedFailureRouteIsOnlyOnADevDaemon(t *testing.T) {
	st := newStack(t, normalDaemon())
	project, repo := st.addProject("small-repo")
	st.gitOutput(repo, "remote", "add", "origin", "https://github.com/marshal-fixtures/small-repo.git")
	card := st.addCard(project.ID, "Nothing to simulate here")

	r := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/ci-failure",
		protocol.SimulateCIFailureRequest{Mode: protocol.SimulateModeSynthetic})
	if r.Status != http.StatusNotFound {
		t.Fatalf("a normal daemon answered %d, want 404; body: %s", r.Status, r.Body)
	}
}

// The two modes are the monitor's own, so a mode it does not know is refused by the monitor, with
// the sentence that names both, rather than by a second copy of the list here.
func TestASimulatedFailureRefusesAModeMarshalDoesNotKnow(t *testing.T) {
	st := newStack(t)
	_, card := st.startedCard(t)

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/ci-failure",
		map[string]string{"mode": "fast"}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

// A card that has not been started has no branch, so there is nothing for a run to fail on and the
// real mode would have nothing to commit to.
func TestASimulatedFailureForACardThatHasNotStartedIsRefused(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Not started yet")

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/ci-failure",
		protocol.SimulateCIFailureRequest{Mode: protocol.SimulateModeSynthetic}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
}

// The real mode is refused before it commits anything when the card has no worktree, so asking for a
// real failure on a card whose folder has gone away changes nothing anywhere and records nothing.
func TestTheRealModeIsRefusedForACardWithNoWorktree(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	st.gitOutput(repo, "remote", "add", "origin", "https://github.com/marshal-fixtures/small-repo.git")
	card := st.addCard(project.ID, "Worktree is gone")
	// The card is given a branch and no worktree, which is the state a card is in when its folder
	// has been removed. The row is written straight in because starting a card makes both.
	nameBranchOnly(t, st, card.ID, "marshal/gone-1-worktree-is-gone")

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/ci-failure",
		protocol.SimulateCIFailureRequest{Mode: protocol.SimulateModeReal}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)

	if entries, err := st.store.Queries().ListAuditLog(context.Background(), 10); err != nil || len(entries) != 0 {
		t.Fatalf("a refused real failure recorded %v (err %v), want nothing", entries, err)
	}
}

// nameBranchOnly gives a card a branch without a worktree, which is what a card whose folder has gone
// away looks like. It writes the row straight in, the way seedRun writes a run.
func nameBranchOnly(t *testing.T, st *stack, cardID, branch string) {
	t.Helper()
	err := st.store.Write(context.Background(), func(q *db.Queries) error {
		_, err := q.UpdateCardWorktree(context.Background(), db.UpdateCardWorktreeParams{
			WorktreePath: "", Branch: branch, UpdatedAt: time.Now().UnixMilli(), ID: cardID,
		})
		return err
	})
	if err != nil {
		t.Fatalf("give card %s a branch: %v", cardID, err)
	}
}
