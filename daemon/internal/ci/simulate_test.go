package ci_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/ci"
	gh "github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Simulate CI failure (B6.4, build-plan 6.10, N28). The synthetic mode is the one every test here
// uses: it goes through the monitor's own entry point, so what it proves is the loop the daemon
// really runs. The real mode is tested up to, and not including, the push: fakeGit records the
// marked commit and the push it was asked for, and nothing here writes a file or opens a socket.

// simulate runs one mode and fails the test when it is refused.
func (f *fixtures) simulate(t *testing.T, mode protocol.SimulateMode) protocol.SimulateCIFailureResult {
	t.Helper()
	result, err := f.svc.Simulate(context.Background(), testCardID, mode)
	if err != nil {
		t.Fatalf("simulate a CI failure (%s): %v", mode, err)
	}
	return result
}

// simulatedRun reads the run of the workflow the synthetic mode makes up.
func (f *fixtures) simulatedRun(t *testing.T) db.CiRun {
	t.Helper()
	var row db.CiRun
	err := f.store.Read(context.Background(), func(q *db.Queries) error {
		got, err := q.GetCiRunFor(context.Background(), db.GetCiRunForParams{
			ProjectID: testProjectID, Branch: testBranch, Workflow: "simulated-ci",
		})
		if err != nil {
			return err
		}
		row = got
		return nil
	})
	if err != nil {
		t.Fatalf("read the simulated run: %v", err)
	}
	return row
}

// auditRows reads every audit row back, oldest first, which is the order the simulations ran in.
func auditRows(t *testing.T, f *fixtures) []db.AuditLog {
	t.Helper()
	rows, err := f.store.Queries().ListAuditLog(context.Background(), 100)
	if err != nil {
		t.Fatalf("read the audit log: %v", err)
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows
}

// codeOf reads the error code off a refusal, so a test can assert how a mode was turned down without
// caring which sentence came with it.
func codeOf(t *testing.T, err error) protocol.ErrorCode {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("the answer %v is not a Marshal error", err)
	}
	return perr.Code
}

// wantBadge fails unless a card's badge was written at least once and every write said the same
// thing. The fix loop injects a failure once per delivery - two deliveries for one story - so the
// badge is written twice, and both writes must say the run failed.
func wantBadge(t *testing.T, states []*protocol.CIState, want protocol.CIState) {
	t.Helper()
	if len(states) == 0 {
		t.Fatalf("the card's badge was never written, want %s", want)
	}
	for i, state := range states {
		if state == nil || *state != want {
			t.Fatalf("badge write %d was %v, want %s", i, state, want)
		}
	}
}

func TestTheSyntheticModeRunsTheWholeFixLoop(t *testing.T) {
	f := newFixture(t)
	result := f.simulate(t, protocol.SimulateModeSynthetic)

	row := f.simulatedRun(t)
	if row.Status != string(protocol.CIStateFailed) {
		t.Fatalf("the run is %q, want failed", row.Status)
	}
	if row.CardID == nil || *row.CardID != testCardID {
		t.Fatalf("the run's card is %v, want %s", row.CardID, testCardID)
	}
	// Step 1 happened: Marshal asked for the rerun, and remembered that it did.
	if row.RerunAt == 0 {
		t.Fatal("the failed jobs were never rerun, so the loop's first step did not run")
	}
	// Step 2 happened: the failed step's log was fetched and handed to the card's session, and
	// remembered, so every later delivery of this run sends nothing again.
	if row.FixSentAt == 0 {
		t.Fatal("the failure was never handed over, so the loop's second step did not run")
	}
	if len(f.worker.sent) != 1 {
		t.Fatalf("messages sent %d, want 1", len(f.worker.sent))
	}
	if !strings.Contains(f.worker.sent[0], testBranch) || !strings.Contains(f.worker.sent[0], "TestSimulatedFailure") {
		t.Fatalf("the message does not name the branch and the failure:\n%s", f.worker.sent[0])
	}
	if !result.FixStarted {
		t.Fatal("the answer says the failure never reached the fix loop")
	}
	if result.Mode != protocol.SimulateModeSynthetic || result.CardID != testCardID {
		t.Fatalf("the answer is %+v", result)
	}
	if result.Commit != "" {
		t.Fatalf("the synthetic mode answered a commit %q; it changes nothing on the branch", result.Commit)
	}
	if result.Run.Workflow != "simulated-ci" || result.Run.Status != protocol.CIStateFailed {
		t.Fatalf("the answered run is %+v", result.Run)
	}
	// The card's badge followed, and the project and Home were told, exactly as they are for a run
	// GitHub reported.
	wantBadge(t, f.cards.ciState, protocol.CIStateFailed)
	wantTopics := []string{
		string(protocol.ProjectTopic(testProjectID)), string(protocol.HomeTopic),
		string(protocol.ProjectTopic(testProjectID)), string(protocol.HomeTopic),
	}
	if strings.Join(f.bus.topics, ",") != strings.Join(wantTopics, ",") {
		t.Fatalf("published on %v, want %v", f.bus.topics, wantTopics)
	}
}

func TestTheSyntheticModeNeverAsksTheRealForge(t *testing.T) {
	f := newFixture(t)
	f.simulate(t, protocol.SimulateModeSynthetic)

	if len(f.forge.reruns) != 0 || len(f.forge.logReads) != 0 || len(f.forge.listBrancs) != 0 {
		t.Fatalf("a synthetic failure asked the forge: reruns %v, logs %v, branches %v",
			f.forge.reruns, f.forge.logReads, f.forge.listBrancs)
	}
	if len(f.git.committed) != 0 || len(f.git.pushed) != 0 {
		t.Fatal("a synthetic failure wrote to a worktree or pushed")
	}
}

func TestTheSyntheticModeStopsAtTheRoleCeiling(t *testing.T) {
	f := newFixture(t)
	f.roles.limits, f.roles.found = harness.Limits{Rounds: 2}, true
	seedTurn(t, f.store, testCardID, 1)
	seedTurn(t, f.store, testCardID, 2)

	result := f.simulate(t, protocol.SimulateModeSynthetic)

	if len(f.cards.needs) != 1 {
		t.Fatalf("the card was moved to Needs you %d times, want 1", len(f.cards.needs))
	}
	if kind := f.cards.needs[0].Kind; kind != protocol.NeedsReasonKindCIFailed {
		t.Fatalf("the reason is %q, want %q", kind, protocol.NeedsReasonKindCIFailed)
	}
	if len(f.worker.sent) != 0 {
		t.Fatalf("a card at its ceiling was sent %v", f.worker.sent)
	}
	if result.FixStarted {
		t.Fatal("the answer says the loop took the failure, but its ceiling stopped it")
	}
}

func TestTheSyntheticModeWithNoSessionStillRecordsTheFailure(t *testing.T) {
	f := newFixture(t)
	f.worker = nil
	svc, err := ci.New(ci.Deps{
		Store: f.store, Cards: f.cards, Projects: f.projects, Roles: f.roles, Git: f.git,
		Bus: f.bus, Options: ci.Options{
			Now: func() time.Time { return testNow }, Forge: f.forge,
			Repo: func(context.Context, protocol.Project) (gh.Repository, bool) { return f.repo, true },
		},
	})
	if err != nil {
		t.Fatalf("build a monitor with no session: %v", err)
	}
	result, err := svc.Simulate(context.Background(), testCardID, protocol.SimulateModeSynthetic)
	if err != nil {
		t.Fatalf("simulate with no session: %v", err)
	}
	row := f.simulatedRun(t)
	if row.Status != string(protocol.CIStateFailed) {
		t.Fatalf("the run is %q, want failed", row.Status)
	}
	if result.FixStarted {
		t.Fatal("the answer says the failure was handed over, but there is no session to hand it to")
	}
}

func TestEverySimulationIsItsOwnRun(t *testing.T) {
	f := newFixture(t)
	first := f.simulate(t, protocol.SimulateModeSynthetic)
	second := f.simulate(t, protocol.SimulateModeSynthetic)

	if first.Run.ID == "" || first.Run.ID == second.Run.ID {
		t.Fatalf("two simulations answered the run ids %q and %q, want two different runs",
			first.Run.ID, second.Run.ID)
	}
	// The second simulation is a new run, so its row starts the loop over rather than being a run
	// Marshal already sent.
	row := f.simulatedRun(t)
	if row.ID != second.Run.ID {
		t.Fatalf("the stored run is %s, want the newest simulated one %s", row.ID, second.Run.ID)
	}
	if row.RerunAt == 0 || row.FixSentAt == 0 {
		t.Fatalf("the newest run did not go through the loop: rerun %d, fix sent %d", row.RerunAt, row.FixSentAt)
	}
	if len(f.worker.sent) != 2 {
		t.Fatalf("messages sent %d, want 2, one per simulation", len(f.worker.sent))
	}
}

func TestASimulatedFailureIsRefusedForACardWithNoBranch(t *testing.T) {
	f := newFixture(t)
	f.cards.cards[0].Branch = ""

	_, err := f.svc.Simulate(context.Background(), testCardID, protocol.SimulateModeSynthetic)
	if code := codeOf(t, err); code != protocol.ErrorCodeRefused {
		t.Fatalf("a card with no branch answered %v (%v), want refused", code, err)
	}
}

func TestASimulatedFailureRefusesAModeMarshalDoesNotKnow(t *testing.T) {
	f := newFixture(t)
	for _, mode := range []protocol.SimulateMode{"", "fast"} {
		_, err := f.svc.Simulate(context.Background(), testCardID, mode)
		if code := codeOf(t, err); code != protocol.ErrorCodeInvalidArgument {
			t.Fatalf("mode %q answered %v (%v), want invalid_argument", mode, code, err)
		}
	}
}

func TestASimulatedFailureForAnUnknownCardIsNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Simulate(context.Background(), "01NOTACARD0000000000000000", protocol.SimulateModeSynthetic)
	if code := codeOf(t, err); code != protocol.ErrorCodeNotFound {
		t.Fatalf("an unknown card answered %v (%v), want not_found", code, err)
	}
}

func TestTheRealModeMarksTheBranchAndPushesIt(t *testing.T) {
	f := newFixture(t)
	result := f.simulate(t, protocol.SimulateModeReal)

	if len(f.git.committed) != 1 {
		t.Fatalf("marked commits made %d, want 1", len(f.git.committed))
	}
	commit := f.git.committed[0]
	if commit.path != ".github/workflows/marshal-ci-simulate.yml" {
		t.Fatalf("the marked commit touched %q", commit.path)
	}
	if !strings.Contains(commit.content, "exit 1") {
		t.Fatalf("the marked file does not fail on purpose:\n%s", commit.content)
	}
	if !strings.Contains(commit.message, testCardID) {
		t.Fatalf("the marked commit's message does not name the card: %q", commit.message)
	}
	if commit.dir != f.projects.worktree {
		t.Fatalf("the marked commit was made in %q, want the card's worktree %q", commit.dir, f.projects.worktree)
	}
	if len(f.git.pushed) != 1 {
		t.Fatalf("pushes made %d, want 1", len(f.git.pushed))
	}
	push := f.git.pushed[0]
	if push.remote != "origin" || push.branch != testBranch || push.main != "main" {
		t.Fatalf("the push was %+v, want origin/%s with main as the branch to stay off", push, testBranch)
	}
	if result.Commit != "the-marked-commit" {
		t.Fatalf("the answer names the commit %q", result.Commit)
	}
	// Nothing about the run is known yet: the failure arrives later as an ordinary delivery.
	if result.Run.Status != protocol.CIStateQueued || result.Run.ID != "" || result.Run.Branch != testBranch {
		t.Fatalf("the answered run is %+v, want a queued run on %s with no id", result.Run, testBranch)
	}
	if result.FixStarted {
		t.Fatal("the real mode answered that a loop ran, but the failure has not come back yet")
	}
	// The real mode never runs the loop itself: it injects nothing.
	if len(f.bus.topics) != 0 || len(f.cards.ciState) != 0 {
		t.Fatalf("the real mode published %v and wrote %v", f.bus.topics, f.cards.ciState)
	}
}

func TestTheRealModeNeedsTheAppConnected(t *testing.T) {
	f := newFixture(t, func(o *ci.Options) { o.Forge = nil })
	_, err := f.svc.Simulate(context.Background(), testCardID, protocol.SimulateModeReal)
	if code := codeOf(t, err); code != protocol.ErrorCodeRefused {
		t.Fatalf("the real mode with nothing connected answered %v (%v), want refused", code, err)
	}
	if len(f.git.committed) != 0 || len(f.git.pushed) != 0 {
		t.Fatal("the real mode wrote to the worktree even though nothing is connected")
	}
}

func TestTheRealModeIsRefusedWithoutAWorktree(t *testing.T) {
	f := newFixture(t)
	f.projects.worktree = ""
	_, err := f.svc.Simulate(context.Background(), testCardID, protocol.SimulateModeReal)
	if code := codeOf(t, err); code != protocol.ErrorCodeRefused {
		t.Fatalf("a card with no worktree answered %v (%v), want refused", code, err)
	}
}

func TestARealModeThatCannotPushSaysSoAndKeepsItsMark(t *testing.T) {
	f := newFixture(t)
	f.git.pushErr = errors.New("the remote refused the push")
	_, err := f.svc.Simulate(context.Background(), testCardID, protocol.SimulateModeReal)
	if code := codeOf(t, err); code != protocol.ErrorCodeUnavailable {
		t.Fatalf("a push that failed answered %v (%v), want unavailable", code, err)
	}
	if len(f.git.committed) != 1 {
		t.Fatal("the marked commit was not made, so there was nothing to push")
	}
	if len(f.git.pushed) != 0 {
		t.Fatal("a push that failed was recorded as one that went through")
	}
}

func TestBothModesWriteOneAuditRow(t *testing.T) {
	f := newFixture(t)
	f.simulate(t, protocol.SimulateModeSynthetic)
	f.simulate(t, protocol.SimulateModeReal)

	rows := auditRows(t, f)
	if len(rows) != 2 {
		t.Fatalf("audit rows %d, want 2", len(rows))
	}
	for i, row := range rows {
		if row.Action != "ci.simulated" {
			t.Fatalf("row %d is the action %q, want ci.simulated", i, row.Action)
		}
		if row.Actor != "person" {
			t.Fatalf("row %d was written by %q, want person", i, row.Actor)
		}
		if row.Target != testCardID {
			t.Fatalf("row %d targets %q, want the card %s", i, row.Target, testCardID)
		}
	}
	if !strings.Contains(rows[0].DetailJSON, `"mode":"synthetic"`) {
		t.Fatalf("the first row's detail is %s", rows[0].DetailJSON)
	}
	if !strings.Contains(rows[1].DetailJSON, `"mode":"real"`) ||
		!strings.Contains(rows[1].DetailJSON, `"commit":"the-marked-commit"`) {
		t.Fatalf("the second row's detail is %s", rows[1].DetailJSON)
	}
}

// A simulated failure must answer exactly what a real one leaves behind. This runs the two paths
// against the same fixtures - the recorded delivery the replay tool sends, and the synthetic
// injection - and compares what each one did.
func TestASimulatedFailureLeavesWhatARealOneLeaves(t *testing.T) {
	real := newFixture(t)
	// The failed step's log is what the loop sends the card. Without one there is nothing to send,
	// which is its own rule and its own test; the comparison needs both paths to send something.
	real.forge.log = "line one\nline two\nFAIL\n"
	real.deliver(t, testRunID, "completed", "failure")
	real.deliver(t, testRunID, "completed", "failure")

	fake := newFixture(t)
	fake.simulate(t, protocol.SimulateModeSynthetic)

	realRow := readRun(t, real.store, fmt.Sprint(testRunID))
	fakeRow := fake.simulatedRun(t)
	if realRow.Status != fakeRow.Status {
		t.Fatalf("the real run is %q and the simulated one is %q", realRow.Status, fakeRow.Status)
	}
	for _, row := range []db.CiRun{realRow, fakeRow} {
		if row.RerunAt == 0 || row.FixSentAt == 0 {
			t.Fatalf("a run did not go through the loop: %+v", row)
		}
	}
	if len(real.worker.sent) != 1 || len(fake.worker.sent) != 1 {
		t.Fatalf("messages sent: real %d, simulated %d, want one each", len(real.worker.sent), len(fake.worker.sent))
	}
	// The same badge, written the same number of times: one write per injection of the story.
	wantBadge(t, real.cards.ciState, protocol.CIStateFailed)
	wantBadge(t, fake.cards.ciState, protocol.CIStateFailed)
	if len(real.cards.ciState) != len(fake.cards.ciState) {
		t.Fatalf("card badges written: real %d, simulated %d, want the same",
			len(real.cards.ciState), len(fake.cards.ciState))
	}
	if *real.cards.ciState[0] != *fake.cards.ciState[0] {
		t.Fatalf("the badge is %v for the real run and %v for the simulated one",
			*real.cards.ciState[0], *fake.cards.ciState[0])
	}
	// The same two topics, in the same order, for each delivery of the same story.
	if strings.Join(real.bus.topics, ",") != strings.Join(fake.bus.topics, ",") {
		t.Fatalf("published on %v for the real run and %v for the simulated one",
			real.bus.topics, fake.bus.topics)
	}
}
