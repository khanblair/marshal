package integrator_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// stubReviewer answers the verdict it was given and records what it was asked to read.
type stubReviewer struct {
	verdict integrator.ChangeVerdict
	err     error
	mu      sync.Mutex
	asked   []integrator.ChangeReview
}

func (r *stubReviewer) ReviewChanges(_ context.Context, req integrator.ChangeReview) (integrator.ChangeVerdict, error) {
	r.mu.Lock()
	r.asked = append(r.asked, req)
	r.mu.Unlock()
	return r.verdict, r.err
}

func (r *stubReviewer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.asked)
}

// stubChecker answers the outcome it was given.
type stubChecker struct {
	outcome integrator.CheckOutcome
	err     error
	ran     int
	mu      sync.Mutex
}

func (c *stubChecker) CheckChanges(context.Context, string) (integrator.CheckOutcome, error) {
	c.mu.Lock()
	c.ran++
	c.mu.Unlock()
	return c.outcome, c.err
}

// recordingNotes and recordingWorker keep what they are told.
type recordingNotes struct {
	mu    sync.Mutex
	notes []string
}

func (n *recordingNotes) Note(_ context.Context, _, body string) error {
	n.mu.Lock()
	n.notes = append(n.notes, body)
	n.mu.Unlock()
	return nil
}

type recordingWorker struct {
	mu   sync.Mutex
	sent []string
	err  error
}

func (w *recordingWorker) Send(_ context.Context, _, text string) error {
	w.mu.Lock()
	w.sent = append(w.sent, text)
	w.mu.Unlock()
	return w.err
}

// shipping is an env whose queue has a reviewer, a checker, notes, and a worker, and whose ready hook
// is wired, so a card that is sent runs the whole way.
type shipping struct {
	*env
	reviewer *stubReviewer
	checker  *stubChecker
	notes    *recordingNotes
	worker   *recordingWorker
}

func newShipping(t *testing.T) *shipping {
	t.Helper()
	s := &shipping{
		reviewer: &stubReviewer{verdict: integrator.ChangeVerdict{Approved: true, Summary: "Looks right."}},
		checker:  &stubChecker{outcome: integrator.CheckOutcome{Ran: true, Passed: true}},
		notes:    &recordingNotes{},
		worker:   &recordingWorker{},
	}
	s.env = newEnv(t, withDeps(func(d *integrator.Deps) {
		d.Reviewer, d.Checker, d.Notes, d.Worker = s.reviewer, s.checker, s.notes, s.worker
	}))
	s.wireHook()
	return s
}

func (s *shipping) send(files map[string]string) protocol.Card {
	s.t.Helper()
	card := s.work("Add retry", protocol.CardStateWorking, files)
	if _, err := s.svc.SendToMerge(s.ctx, card.ID); err != nil {
		s.t.Fatalf("SendToMerge: %v", err)
	}
	s.svc.Wait()
	return card
}

func TestACardThatPassesReviewAndChecksIsMergedAndKeepsTheirNotes(t *testing.T) {
	s := newShipping(t)
	card := s.send(map[string]string{"b.txt": "b by the card\n"})

	if got := s.state(card.ID); got.State != protocol.CardStateDone {
		t.Fatalf("the card is %s, want done", got.State)
	}
	if s.reviewer.count() != 1 || s.checker.ran != 1 {
		t.Errorf("review ran %d times and checks %d, want once each", s.reviewer.count(), s.checker.ran)
	}
	if got := s.reviewer.asked[0]; got.Base != "main" || got.Worktree != s.state(card.ID).Worktree {
		t.Errorf("the reviewer was asked to read %+v, want the card's changes against main", got)
	}
	if len(s.notes.notes) != 1 || !strings.Contains(s.notes.notes[0], "approved") {
		t.Errorf("notes = %v, want one that says the review approved", s.notes.notes)
	}
	if ci := s.state(card.ID).CI; ci == nil || *ci != protocol.CIStatePassed {
		t.Errorf("the card's CI = %v, want passed after the checks ran", ci)
	}
	if got := s.read(s.repo, "b.txt"); got != "b by the card\n" {
		t.Errorf("b.txt = %q, want the card's change in the folder", got)
	}
}

func TestAReviewThatAsksForChangesSendsTheCardBackToItsAgentWithoutMerging(t *testing.T) {
	s := newShipping(t)
	s.reviewer.verdict = integrator.ChangeVerdict{
		Summary:  "The retry has no limit.",
		Comments: []integrator.ChangeComment{{Path: "b.txt", Line: 1, Body: "Stop after three tries."}},
	}
	card := s.send(map[string]string{"b.txt": "b by the card\n"})

	if got := s.state(card.ID).State; got != protocol.CardStateWorking {
		t.Errorf("the card is %s, want working so its agent fixes it", got)
	}
	if s.checker.ran != 0 {
		t.Error("the checks ran on changes the Reviewer rejected")
	}
	if len(s.worker.sent) != 1 || !strings.Contains(s.worker.sent[0], "Stop after three tries.") ||
		!strings.Contains(s.worker.sent[0], "finish the card again") {
		t.Errorf("the agent was told %v, want the comment and how to go on", s.worker.sent)
	}
	if got := s.read(s.repo, "b.txt"); got == "b by the card\n" {
		t.Error("changes that were not approved reached the folder")
	}
}

func TestChecksThatFailSendTheCardBackWithWhatFailed(t *testing.T) {
	s := newShipping(t)
	s.checker.outcome = integrator.CheckOutcome{Ran: true, Passed: false, Summary: "go test: 2 failed"}
	card := s.send(map[string]string{"b.txt": "b by the card\n"})

	if got := s.state(card.ID).State; got != protocol.CardStateWorking {
		t.Errorf("the card is %s, want working", got)
	}
	if len(s.worker.sent) != 1 || !strings.Contains(s.worker.sent[0], "go test: 2 failed") {
		t.Errorf("the agent was told %v, want what failed", s.worker.sent)
	}
	if ci := s.state(card.ID).CI; ci != nil && *ci == protocol.CIStatePassed {
		t.Error("checks that failed were recorded as passed")
	}
}

func TestAReviewThatCannotRunLeavesTheCardInNeedsYouWithTheReason(t *testing.T) {
	s := newShipping(t)
	s.reviewer.err = errors.New("Claude is signed out")
	card := s.send(map[string]string{"b.txt": "b by the card\n"})

	got := s.state(card.ID)
	if got.State != protocol.CardStateNeeds || got.NeedsReason == nil ||
		!strings.Contains(got.NeedsReason.Text, "The review could not run: Claude is signed out") {
		t.Errorf("the card is %s with %+v, want Needs you with the reason", got.State, got.NeedsReason)
	}
}

func TestAProjectThatTurnsReviewAndChecksOffGoesStraightToTheMerge(t *testing.T) {
	s := newShipping(t)
	off := false
	if _, err := s.svc.UpdateSettings(s.ctx, s.project.ID, protocol.IntegrationSettingsPatch{Review: &off, Checks: &off}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	card := s.send(map[string]string{"b.txt": "b by the card\n"})

	if s.reviewer.count() != 0 || s.checker.ran != 0 {
		t.Errorf("review ran %d times and checks %d, want neither", s.reviewer.count(), s.checker.ran)
	}
	if got := s.state(card.ID).State; got != protocol.CardStateDone {
		t.Errorf("the card is %s, want done", got)
	}
}

func TestChecksWithNothingToRunDoNotHoldACardBack(t *testing.T) {
	s := newShipping(t)
	s.checker.outcome = integrator.CheckOutcome{Ran: false}
	card := s.send(map[string]string{"b.txt": "b by the card\n"})
	if got := s.state(card.ID).State; got != protocol.CardStateDone {
		t.Errorf("the card is %s, want done: a project with no workflow files has no checks to fail", got)
	}
}

func TestACardItsAgentFinishedGoesTheWholeWayWhenAutoShipIsOn(t *testing.T) {
	s := newShipping(t)
	card := s.work("Add retry", protocol.CardStateWorking, map[string]string{"b.txt": "b by the card\n"})

	s.svc.CardFinished(s.ctx, card.ID, "Added the retry.")
	s.svc.Wait()

	if got := s.state(card.ID).State; got != protocol.CardStateDone {
		t.Errorf("the card is %s, want done", got)
	}
	if len(s.notes.notes) == 0 || !strings.Contains(s.notes.notes[0], "Added the retry.") {
		t.Errorf("notes = %v, want the agent's summary first", s.notes.notes)
	}
}

func TestACardItsAgentFinishedWaitsInReviewWhenAutoShipIsOff(t *testing.T) {
	s := newShipping(t)
	off := false
	if _, err := s.svc.UpdateSettings(s.ctx, s.project.ID, protocol.IntegrationSettingsPatch{AutoShip: &off}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	card := s.work("Add retry", protocol.CardStateWorking, map[string]string{"b.txt": "b by the card\n"})

	s.svc.CardFinished(s.ctx, card.ID, "Added the retry.")
	s.svc.Wait()

	if got := s.state(card.ID).State; got != protocol.CardStateReview {
		t.Errorf("the card is %s, want in review for a person to send", got)
	}
	if s.reviewer.count() != 0 {
		t.Error("the review ran although auto-ship is off")
	}
}

func TestACardItsAgentCalledFinishedWithNothingCommittedIsNotSilentlyIgnored(t *testing.T) {
	s := newShipping(t)
	card := s.work("No commits", protocol.CardStateWorking, nil)

	s.svc.CardFinished(s.ctx, card.ID, "Done.")
	s.svc.Wait()

	got := s.state(card.ID)
	if got.State != protocol.CardStateNeeds || got.NeedsReason == nil || !strings.Contains(got.NeedsReason.Text, "no commits") {
		t.Errorf("the card is %s with %+v, want Needs you saying there is nothing committed", got.State, got.NeedsReason)
	}
}

// A backup branch only serves the undo of its own merge, so once another card has landed on the target
// the older backups are gone and only the newest stays.
func TestABackupBranchOutlivesItsMergeOnlyUntilTheTargetMovesOn(t *testing.T) {
	e := newEnv(t, withTester(passing()))
	first := e.card("Add retry", map[string]string{"b.txt": "b by the first\n"})
	e.merge(first)
	if branches := e.run(e.repo, "branch", "--list", "marshal/backup/*"); strings.Count(branches, "marshal/backup/") != 1 {
		t.Fatalf("after one merge the backups are %q, want one", branches)
	}
	second := e.card("Add cache", map[string]string{"c.txt": "c by the second\n"})
	e.merge(second)

	branches := e.run(e.repo, "branch", "--list", "marshal/backup/*")
	if strings.Count(branches, "marshal/backup/") != 1 {
		t.Errorf("after two merges the backups are %q, want only the newest", branches)
	}
}

// Two cards that changed the same file are told apart from a card that changed its own: the forecast
// names the file and both cards while they are still working, long before a merge would find it.
func TestTwoCardsThatChangedTheSameFileAreForecastAsAnOverlap(t *testing.T) {
	e := newEnv(t)
	a := e.work("First card", protocol.CardStateWorking, map[string]string{"a.txt": "by the first\n", "c.txt": "only the first\n"})
	b := e.work("Second card", protocol.CardStateWorking, map[string]string{"a.txt": "by the second\n"})

	overlaps := e.svc.Overlaps(e.ctx, e.project.ID)
	if len(overlaps) != 1 || overlaps[0].Path != "a.txt" || len(overlaps[0].Cards) != 2 {
		t.Fatalf("overlaps = %+v, want one on a.txt held by both cards", overlaps)
	}
	if got := overlaps[0].CardIDs; len(got) != 2 || (got[0] != a.ID && got[1] != a.ID) || (got[0] != b.ID && got[1] != b.ID) {
		t.Errorf("the overlap names cards %v, want %s and %s", got, a.ID, b.ID)
	}
	state, err := e.svc.State(e.ctx, e.project.ID)
	if err != nil || len(state.Overlaps) != 1 {
		t.Errorf("the integration state carries %d overlaps (%v), want the same one", len(state.Overlaps), err)
	}
}

func TestACardWithNoSharedFilesHasNoOverlapAndTheListIsNeverNil(t *testing.T) {
	e := newEnv(t)
	e.work("Only card", protocol.CardStateWorking, map[string]string{"a.txt": "mine\n"})
	if got := e.svc.Overlaps(e.ctx, e.project.ID); got == nil || len(got) != 0 {
		t.Errorf("overlaps = %#v, want an empty list that is not nil", got)
	}
}
