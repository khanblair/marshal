package integrator_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func directEnv(t *testing.T, adjust func(*integrator.Deps)) *env {
	t.Helper()
	return newEnv(t, withDeps(func(d *integrator.Deps) {
		d.DirectWorkspace = true
		if adjust != nil {
			adjust(d)
		}
	}))
}

func directCard(t *testing.T, e *env) protocol.Card {
	t.Helper()
	card, err := e.proj.CreateCard(e.ctx, e.project.ID, protocol.CreateCardRequest{Title: "Edit in the project"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.proj.SetWorktree(e.ctx, card.ID, e.repo, "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.git.SnapshotFolder(e.ctx, e.repo, gitx.DirectBaseRef(card.ID), "Before card"); err != nil {
		t.Fatal(err)
	}
	card, err = e.proj.SetState(e.ctx, card.ID, protocol.CardStateWorking)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func finishDirect(t *testing.T, e *env, id string) {
	t.Helper()
	if _, err := e.svc.SendToMerge(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
}

func TestDirectCompletionPreservesSourceIndexAndBranchWithoutWorktrees(t *testing.T) {
	reviewer := &stubReviewer{verdict: integrator.ChangeVerdict{Approved: true}}
	e := directEnv(t, func(d *integrator.Deps) { d.Reviewer = reviewer })
	e.write(e.repo, "a.txt", "personal staged edit\n")
	e.run(e.repo, "add", "a.txt")
	e.write(e.repo, "a.txt", "personal unstaged edit\n")
	card := directCard(t, e)
	e.write(e.repo, "b.txt", "agent edit\n")
	e.write(e.repo, "new.txt", "agent untracked file\n")
	beforeHead := e.tip("HEAD")
	beforeIndex := e.run(e.repo, "diff", "--cached")
	beforeStatus := e.status()
	finishDirect(t, e, card.ID)

	if got := e.state(card.ID); got.State != protocol.CardStateDone {
		t.Fatalf("card = %s with %+v, want done", got.State, got.NeedsReason)
	}
	if e.tip("HEAD") != beforeHead || e.run(e.repo, "diff", "--cached") != beforeIndex || e.status() != beforeStatus {
		t.Fatal("completion changed the active branch, staging area, or source status")
	}
	if got := e.run(e.repo, "worktree", "list", "--porcelain"); strings.Count(got, "worktree ") != 1 {
		t.Fatalf("created extra worktrees: %s", got)
	}
	if reviewer.count() != 1 || reviewer.asked[0].Head == "" || reviewer.asked[0].Base == "main" {
		t.Fatalf("review did not receive exact card snapshots: %+v", reviewer.asked)
	}
	if _, err := e.git.Rev(e.ctx, e.repo, gitx.DirectResultRef(card.ID)); err != nil {
		t.Fatal("completion did not retain the final snapshot")
	}
	if _, err := e.svc.Undo(e.ctx, card.ID); err == nil {
		t.Fatal("direct completion allowed destructive shared-folder undo")
	}
}

func TestDirectCompletionFailureLeavesSourceFilesAndCardUnfinished(t *testing.T) {
	checker := &stubChecker{outcome: integrator.CheckOutcome{Ran: true, Passed: false, Summary: "test failed"}}
	e := directEnv(t, func(d *integrator.Deps) { d.Checker = checker })
	card := directCard(t, e)
	e.write(e.repo, "b.txt", "still being fixed\n")
	finishDirect(t, e, card.ID)
	if e.state(card.ID).State == protocol.CardStateDone {
		t.Fatal("failed checks completed the card")
	}
	if got := e.read(e.repo, "b.txt"); got != "still being fixed\n" {
		t.Fatal("failed checks discarded direct edits")
	}
}

func TestDirectCompletionRefusesLegacyWorktreeWithoutRunningMerge(t *testing.T) {
	e := directEnv(t, nil)
	card := e.work("Old task", protocol.CardStateWorking, map[string]string{"b.txt": "old work\n"})
	before := e.tip("HEAD")
	if _, err := e.svc.SendToMerge(e.ctx, card.ID); err == nil || !strings.Contains(err.Error(), "preserved") {
		t.Fatalf("legacy card = %v, want recovery refusal", err)
	}
	if e.tip("HEAD") != before {
		t.Fatal("legacy card moved the active branch")
	}
}

type completionWorker struct {
	stopErr error
	stopped bool
	held    bool
	sent    bool
}

func (w *completionWorker) Send(context.Context, string, string) error {
	if w.held {
		return errors.New("completion is holding the folder")
	}
	w.sent = true
	return nil
}

func (w *completionWorker) AcquireCompletion(context.Context, string) (func(), error) {
	w.held = true
	return func() { w.held = false }, nil
}

func (w *completionWorker) StopCardSession(context.Context, string) error {
	w.stopped = w.held
	return w.stopErr
}

func TestDirectCompletionMustStopItsMainEditorBeforeMarkingDone(t *testing.T) {
	w := &completionWorker{stopErr: errors.New("process did not stop")}
	e := directEnv(t, func(d *integrator.Deps) { d.Worker = w })
	card := directCard(t, e)
	e.write(e.repo, "b.txt", "direct edit\n")
	finishDirect(t, e, card.ID)
	if !w.stopped || w.held || e.state(card.ID).State == protocol.CardStateDone {
		t.Fatal("completion did not retain ownership through stop, release it, and refuse Done on stop error")
	}
}

func TestDirectCompletionRefusesChangesMadeAfterTheCardsLastTurn(t *testing.T) {
	e := directEnv(t, nil)
	card := directCard(t, e)
	e.write(e.repo, "b.txt", "card edit\n")
	last, err := e.git.SnapshotFolder(e.ctx, e.repo, gitx.DirectResultRef(card.ID), "End of turn")
	if err != nil {
		t.Fatal(err)
	}
	e.write(e.repo, "c.txt", "someone else's later edit\n")
	finishDirect(t, e, card.ID)
	if e.state(card.ID).State == protocol.CardStateDone {
		t.Fatal("later project edits were attributed to the old card")
	}
	if got := e.tip(gitx.DirectResultRef(card.ID)); got != last.Commit {
		t.Fatal("rejected completion overwrote the card's retained result")
	}
}

func TestDirectFailedChecksReleaseCompletionBeforeSendingCorrections(t *testing.T) {
	w := &completionWorker{}
	checker := &stubChecker{outcome: integrator.CheckOutcome{Ran: true, Passed: false, Summary: "test failed"}}
	e := directEnv(t, func(d *integrator.Deps) { d.Worker, d.Checker = w, checker })
	card := directCard(t, e)
	e.write(e.repo, "b.txt", "fix this\n")
	finishDirect(t, e, card.ID)
	if !w.sent || w.held || e.state(card.ID).State != protocol.CardStateWorking {
		t.Fatal("failed checks could not return feedback to the main agent after releasing completion")
	}
}

// The explicit Merge route must obey the same shared-folder safety rules as SendToMerge.
func TestExplicitDirectMergeCompletesWithoutChangingTheProjectBranch(t *testing.T) {
	w := &completionWorker{}
	e := directEnv(t, func(d *integrator.Deps) { d.Worker = w })
	card := directCard(t, e)
	e.write(e.repo, "b.txt", "completed direct edit\n")
	head, status := e.tip("HEAD"), e.status()
	result, err := e.svc.Merge(e.ctx, card.ID)
	if err != nil || !result.Merged || result.Commit == "" || e.state(card.ID).State != protocol.CardStateDone {
		t.Fatalf("direct Merge did not complete: %+v, %v", result, err)
	}
	if !w.stopped || w.held || e.tip("HEAD") != head || e.status() != status {
		t.Fatal("completion failed to stop its editor, release ownership, or preserve the project branch and files")
	}
	if _, err := e.svc.Merge(e.ctx, card.ID); err == nil {
		t.Fatal("the explicit route accepted an already completed card")
	}
}

func TestExplicitDirectMergeRefusesAnEditorThatCannotStop(t *testing.T) {
	w := &completionWorker{stopErr: errors.New("editor still running")}
	e := directEnv(t, func(d *integrator.Deps) { d.Worker = w })
	card := directCard(t, e)
	e.write(e.repo, "b.txt", "preserve unfinished work\n")
	result, err := e.svc.Merge(e.ctx, card.ID)
	if err == nil || result.Merged || e.state(card.ID).State == protocol.CardStateDone || w.held {
		t.Fatalf("unsafe completion after stop failure: %+v, %v", result, err)
	}
	if e.read(e.repo, "b.txt") != "preserve unfinished work\n" {
		t.Fatal("failed completion changed the project files")
	}
}

func TestExplicitDirectMergeRefusesLegacyWorkAndShutdown(t *testing.T) {
	e := directEnv(t, nil)
	legacy := e.work("Preserved old task", protocol.CardStateWorking, map[string]string{"b.txt": "old work\n"})
	head := e.tip("HEAD")
	if _, err := e.svc.Merge(e.ctx, legacy.ID); err == nil || !strings.Contains(err.Error(), "preserved") {
		t.Fatalf("legacy work was not refused: %v", err)
	}
	if e.tip("HEAD") != head {
		t.Fatal("legacy completion moved the project branch")
	}
	e.svc.Close()
	if _, err := e.svc.Merge(e.ctx, legacy.ID); err == nil || !strings.Contains(err.Error(), "shutting down") {
		t.Fatalf("shutdown accepted completion: %v", err)
	}
}
