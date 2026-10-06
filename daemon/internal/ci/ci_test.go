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
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Tests.

func TestAWorkflowRunDeliveryRecordsTheRunAndTheCardsBadge(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, testRunID, "completed", "success")

	row := readRun(t, f.store, fmt.Sprint(testRunID))
	if row.Status != string(protocol.CIStatePassed) {
		t.Fatalf("the run's status is %q, want %q", row.Status, protocol.CIStatePassed)
	}
	if row.CardID == nil || *row.CardID != testCardID {
		t.Fatalf("the run's card is %v, want %s", row.CardID, testCardID)
	}
	if row.Branch != testBranch || row.Workflow != "ci" || row.Url == "" {
		t.Fatalf("the run was stored as %+v", row)
	}
	if len(f.cards.ciState) != 1 || f.cards.ciState[0] == nil || *f.cards.ciState[0] != protocol.CIStatePassed {
		t.Fatalf("the card's badge was written as %v, want passed", f.cards.ciState)
	}
}

func TestARunTellsTheProjectAndHomeThatItsStateChanged(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, testRunID, "completed", "success")

	wantTopics := []string{string(protocol.ProjectTopic(testProjectID)), string(protocol.HomeTopic)}
	if strings.Join(f.bus.topics, ",") != strings.Join(wantTopics, ",") {
		t.Fatalf("published on %v, want %v", f.bus.topics, wantTopics)
	}
	for i, typ := range f.bus.types {
		if typ != string(protocol.EventTypeCIUpdated) {
			t.Fatalf("event %d is %q, want ci.updated", i, typ)
		}
	}
	project, ok := f.bus.data[0].(protocol.CIEventData)
	if !ok || project.Project == nil || project.Snapshot != nil {
		t.Fatalf("the project event carries %#v", f.bus.data[0])
	}
	home, ok := f.bus.data[1].(protocol.CIEventData)
	if !ok || home.Snapshot == nil || home.Project != nil {
		t.Fatalf("the home event carries %#v", f.bus.data[1])
	}
	if len(home.Snapshot.Projects) != 1 || home.Snapshot.Projects[0].ProjectID != testProjectID {
		t.Fatalf("the home snapshot is %+v", home.Snapshot)
	}
}

func TestOnlyTheWorkflowRunDeliveryIsRead(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "check_run", Body: []byte(`{"action":"completed"}`),
	}); err != nil {
		t.Fatalf("deliver a check_run: %v", err)
	}
	if err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "workflow_run", Body: []byte(`not json at all`),
	}); err != nil {
		t.Fatalf("an unreadable body must be accepted: %v", err)
	}
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if len(snapshot.Projects) != 0 {
		t.Fatalf("a delivery Marshal does not handle wrote %+v", snapshot.Projects)
	}
}

func TestADeliveryFromSomebodyElsesRepositoryIsIgnored(t *testing.T) {
	f := newFixture(t)
	elsewhere := gh.Repository{Owner: "someone", Name: "elsewhere"}
	body := deliveryBody(elsewhere, testRunID, testBranch, "completed", "success")
	if err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "workflow_run", Body: body,
	}); err != nil {
		t.Fatalf("deliver from another repository: %v", err)
	}
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if len(snapshot.Projects) != 0 {
		t.Fatalf("a run from another repository was recorded as %+v", snapshot.Projects)
	}
}

func TestAFailedRunRerunsTheFailedJobsOnce(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.forge.reruns) != 1 || f.forge.reruns[0] != testRunID {
		t.Fatalf("rerun asked for %v, want [%d]", f.forge.reruns, testRunID)
	}
	if len(f.forge.logReads) != 0 {
		t.Fatalf("the log was read on the first failure: %v", f.forge.logReads)
	}
	if len(f.worker.sent) != 0 {
		t.Fatalf("a message was sent on the first failure: %v", f.worker.sent)
	}
	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.RerunAt == 0 {
		t.Fatal("the rerun was not remembered, so a restart would rerun again")
	}
}

func TestASecondFailureSendsTheFailedStepsLogToTheCard(t *testing.T) {
	f := newFixture(t, func(o *ci.Options) { o.LogBytes = 4 << 10; o.LogLines = 3 })
	f.forge.log = "line one\nline two\nline three\nline four\nline five\n"
	f.deliver(t, testRunID, "completed", "failure")
	// The rerun comes back as the same run failing again, which is the second failure.
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.forge.logReads) != 1 || f.forge.logReads[0] != testRunID {
		t.Fatalf("the log was read %v, want [%d]", f.forge.logReads, testRunID)
	}
	if len(f.worker.sent) != 1 {
		t.Fatalf("messages sent %d, want 1", len(f.worker.sent))
	}
	message := f.worker.sent[0]
	for _, want := range []string{testBranch, "https://github.com/khanblair/marshal/actions/runs/", "line five"} {
		if !strings.Contains(message, want) {
			t.Fatalf("the message does not mention %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "line one") || strings.Contains(message, "line two") {
		t.Fatalf("the message kept lines past the limit:\n%s", message)
	}
	if !strings.Contains(message, "not shown") {
		t.Fatalf("the message does not say lines were dropped:\n%s", message)
	}
	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.FixSentAt == 0 {
		t.Fatal("sending the log was not remembered, so it would be sent again")
	}
}

func TestTheSameFailureIsSentOnlyOnce(t *testing.T) {
	f := newFixture(t)
	f.forge.log = "boom\n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.worker.sent) != 1 {
		t.Fatalf("messages sent %d, want 1", len(f.worker.sent))
	}
	if len(f.forge.logReads) != 1 {
		t.Fatalf("the log was read %d times, want 1", len(f.forge.logReads))
	}
}

func TestANewRunOnTheSameBranchStartsTheLoopOver(t *testing.T) {
	f := newFixture(t)
	f.forge.log = "boom\n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	// A second run of the same workflow on the same branch is a new run: it takes the row over and
	// the loop starts again from the rerun.
	const secondRun = int64(7009999)
	f.deliver(t, secondRun, "completed", "failure")
	if len(f.forge.reruns) != 2 || f.forge.reruns[1] != secondRun {
		t.Fatalf("rerun asked for %v, want the second run rerun", f.forge.reruns)
	}
	if row := readRun(t, f.store, fmt.Sprint(secondRun)); row.RerunAt == 0 || row.FixSentAt != 0 {
		t.Fatalf("the new run's memory is rerun_at=%d fix_sent_at=%d, want rerun and no fix", row.RerunAt, row.FixSentAt)
	}
}

func TestTheLoopStopsWhenTheCardIsAtItsRoleLimit(t *testing.T) {
	f := newFixture(t)
	// The card's role allows one round, and the card has already taken it.
	f.roles.limits, f.roles.found = harness.Limits{Rounds: 1}, true
	seedTurn(t, f.store, testCardID, 1)
	f.forge.log = "boom\n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.worker.sent) != 0 {
		t.Fatalf("a message was sent past the limit: %v", f.worker.sent)
	}
	if len(f.forge.logReads) != 0 {
		t.Fatalf("the log was read past the limit: %v", f.forge.logReads)
	}
	if len(f.cards.needs) != 1 {
		t.Fatalf("the card was moved to Needs you %d times, want 1", len(f.cards.needs))
	}
	if kind := f.cards.needs[0].Kind; kind != protocol.NeedsReasonKindCIFailed {
		t.Fatalf("the reason is %q, want %q", kind, protocol.NeedsReasonKindCIFailed)
	}
	if text := f.cards.needs[0].Text; !strings.Contains(text, "2") || !strings.Contains(text, "1") {
		t.Fatalf("the reason does not name the rounds and the limit: %q", text)
	}
}

func TestTheLoopDoesNotStopWhenTheRoleSetsNoCeiling(t *testing.T) {
	f := newFixture(t)
	f.roles.limits, f.roles.found = harness.Limits{}, true
	seedTurn(t, f.store, testCardID, 1)
	seedTurn(t, f.store, testCardID, 2)
	seedTurn(t, f.store, testCardID, 3)
	f.forge.log = "boom\n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.cards.needs) != 0 {
		t.Fatalf("a card with no ceiling was stopped: %v", f.cards.needs)
	}
	if len(f.worker.sent) != 1 {
		t.Fatalf("messages sent %d, want 1", len(f.worker.sent))
	}
}

func TestAFailureWithNoLogSendsNothing(t *testing.T) {
	f := newFixture(t)
	f.forge.log = "   \n"
	f.deliver(t, testRunID, "completed", "failure")
	f.deliver(t, testRunID, "completed", "failure")

	if len(f.worker.sent) != 0 {
		t.Fatalf("an empty log was sent: %v", f.worker.sent)
	}
	// Nothing was handed over, so nothing is remembered as handed over: a forge uploads a run's log
	// a little after the run finishes, and a later delivery of the same run must be free to find it.
	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.FixSentAt != 0 {
		t.Fatal("a run with no log was marked as sent, so its log would never be looked for again")
	}
}

func TestAForgeThatCannotBeAskedLeavesTheStateAlone(t *testing.T) {
	f := newFixture(t)
	f.forge.rerunErr = errors.New("github is unreachable")
	f.deliver(t, testRunID, "completed", "failure")

	row := readRun(t, f.store, fmt.Sprint(testRunID))
	if row.Status != string(protocol.CIStateFailed) {
		t.Fatalf("the failure was lost: the run is %q", row.Status)
	}
	if row.RerunAt != 0 {
		t.Fatal("a rerun that was never asked for was remembered, so it would never be retried")
	}
}

func TestADaemonWithNoForgeStillRecordsAFailure(t *testing.T) {
	f := newFixture(t, func(o *ci.Options) { o.Forge = nil })
	f.deliver(t, testRunID, "completed", "failure")

	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.Status != string(protocol.CIStateFailed) {
		t.Fatalf("the run's status is %q, want failed", row.Status)
	}
}

func TestRunStateMapsTheForgesOwnWords(t *testing.T) {
	cases := []struct {
		status, conclusion string
		want               protocol.CIState
	}{
		{"queued", "", protocol.CIStateQueued},
		{"requested", "", protocol.CIStateQueued},
		{"waiting", "", protocol.CIStateQueued},
		{"pending", "", protocol.CIStateQueued},
		{"in_progress", "", protocol.CIStateRunning},
		{"completed", "success", protocol.CIStatePassed},
		{"completed", "failure", protocol.CIStateFailed},
		{"completed", "timed_out", protocol.CIStateFailed},
		{"completed", "action_required", protocol.CIStateFailed},
		{"completed", "startup_failure", protocol.CIStateFailed},
		// A check that did not run must never be read as green.
		{"completed", "skipped", protocol.CIStateCancelled},
		{"completed", "neutral", protocol.CIStateCancelled},
		{"completed", "cancelled", protocol.CIStateCancelled},
		{"completed", "", protocol.CIStateCancelled},
		{"", "", protocol.CIStateQueued},
	}
	for _, tc := range cases {
		t.Run(tc.status+"/"+tc.conclusion, func(t *testing.T) {
			f := newFixture(t)
			f.deliver(t, testRunID, tc.status, tc.conclusion)
			row := readRun(t, f.store, fmt.Sprint(testRunID))
			if row.Status != string(tc.want) {
				t.Fatalf("status %q conclusion %q became %q, want %q",
					tc.status, tc.conclusion, row.Status, tc.want)
			}
		})
	}
}

func TestThePollAsksAboutTheRunsMarshalIsWaitingOn(t *testing.T) {
	f := newFixture(t)
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStateRunning),
		Url: "https://example.test/run", StartedAt: testNow.Add(-20 * time.Minute).UnixMilli(),
		UpdatedAt: testNow.Add(-20 * time.Minute).UnixMilli(),
	})
	f.forge.runs = []gh.WorkflowRun{{
		ID: testRunID, Name: "ci", Branch: testBranch, Status: "completed", Conclusion: "success",
		URL: "https://example.test/run", StartedAt: testNow.Add(-19 * time.Minute).Format(time.RFC3339),
	}}
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(f.forge.listBrancs) != 1 || f.forge.listBrancs[0] != testBranch {
		t.Fatalf("the backup asked about %v, want [%s]", f.forge.listBrancs, testBranch)
	}
	if row := readRun(t, f.store, fmt.Sprint(testRunID)); row.Status != string(protocol.CIStatePassed) {
		t.Fatalf("the polled run is %q, want passed", row.Status)
	}
}

func TestThePollLeavesARunItIsNotWaitingOnAlone(t *testing.T) {
	f := newFixture(t)
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStatePassed),
		UpdatedAt: testNow.Add(-90 * time.Minute).UnixMilli(),
	})
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(f.forge.listBrancs) != 0 {
		t.Fatalf("the backup asked about a run that had already finished: %v", f.forge.listBrancs)
	}
}

func TestThePollLeavesAFreshRunAlone(t *testing.T) {
	f := newFixture(t)
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStateRunning),
		UpdatedAt: testNow.Add(-time.Minute).UnixMilli(),
	})
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(f.forge.listBrancs) != 0 {
		t.Fatalf("the backup asked about a run that is merely slow: %v", f.forge.listBrancs)
	}
}

func TestThePollStopsAskingAboutABranchWithNoRuns(t *testing.T) {
	f := newFixture(t)
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStateRunning),
		UpdatedAt: testNow.Add(-20 * time.Minute).UnixMilli(),
	})
	for i := 0; i < 3; i++ {
		if err := f.svc.Poll(context.Background()); err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
	}
	if len(f.forge.listBrancs) != 1 {
		t.Fatalf("the backup asked %d times about a branch with no runs, want 1", len(f.forge.listBrancs))
	}
}

func TestThePollAsksAgainOnceTheAnswerIsOld(t *testing.T) {
	now := testNow
	f := newFixture(t, func(o *ci.Options) { o.Now = func() time.Time { return now } })
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: fmt.Sprint(testRunID), ProjectID: testProjectID, CardID: &cardID,
		Branch: testBranch, Workflow: "ci", Status: string(protocol.CIStateRunning),
		UpdatedAt: testNow.Add(-20 * time.Minute).UnixMilli(),
	})
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	now = now.Add(11 * time.Minute)
	if err := f.svc.Poll(context.Background()); err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if len(f.forge.listBrancs) != 2 {
		t.Fatalf("the backup asked %d times, want 2 once the answer went stale", len(f.forge.listBrancs))
	}
}

func TestPollWithoutAForgeSaysSo(t *testing.T) {
	f := newFixture(t, func(o *ci.Options) { o.Forge = nil })
	if err := f.svc.Poll(context.Background()); !errors.Is(err, ci.ErrNoForge) {
		t.Fatalf("poll answered %v, want ErrNoForge", err)
	}
}

func TestTheSnapshotGroupsRunsByProjectInProjectOrder(t *testing.T) {
	f := newFixture(t)
	seedProject(t, f.store, "mobile-app")
	cardID := testCardID
	seedRun(t, f.store, db.CiRun{
		ID: "1", ProjectID: "mobile-app", Branch: "main", Workflow: "ci",
		Status: string(protocol.CIStateFailed), UpdatedAt: testNow.UnixMilli(),
	})
	seedRun(t, f.store, db.CiRun{
		ID: "2", ProjectID: testProjectID, CardID: &cardID, Branch: testBranch,
		Workflow: "ci", Status: string(protocol.CIStateRunning), UpdatedAt: testNow.UnixMilli(),
	})
	f.projects.projects = []protocol.Project{
		{ID: testProjectID, Name: testProjectID, DefaultBranch: "main"},
		{ID: "mobile-app", Name: "mobile-app", DefaultBranch: "main"},
	}
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if len(snapshot.Projects) != 2 {
		t.Fatalf("the snapshot has %d projects, want 2", len(snapshot.Projects))
	}
	if snapshot.Projects[0].ProjectID != testProjectID || snapshot.Projects[1].ProjectID != "mobile-app" {
		t.Fatalf("the projects are %s then %s, want the project order",
			snapshot.Projects[0].ProjectID, snapshot.Projects[1].ProjectID)
	}
	if snapshot.Projects[0].Status != protocol.CIStateRunning {
		t.Fatalf("the first project's status is %q, want running", snapshot.Projects[0].Status)
	}
	if len(snapshot.Projects[0].Runs) != 1 || snapshot.Projects[0].Runs[0].StartedAt != nil {
		t.Fatalf("a queued run's start time should be null: %+v", snapshot.Projects[0].Runs)
	}
}

func TestAProjectWithNoRunsIsLeftOutOfTheSnapshot(t *testing.T) {
	f := newFixture(t)
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if len(snapshot.Projects) != 0 {
		t.Fatalf("a project with no runs appeared as %+v", snapshot.Projects)
	}
}

func TestStartedAtIsNullWhileARunIsQueued(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, testRunID, "queued", "")
	project, err := f.svc.RunsForProject(context.Background(), testProjectID)
	if err != nil {
		t.Fatalf("read a project's CI health: %v", err)
	}
	if len(project.Runs) != 1 {
		t.Fatalf("the project has %d runs, want 1", len(project.Runs))
	}
	if project.Runs[0].StartedAt != nil {
		t.Fatalf("a queued run's startedAt is %v, want null", project.Runs[0].StartedAt)
	}
	if project.Status != protocol.CIStateQueued {
		t.Fatalf("the project's status is %q, want queued", project.Status)
	}
}

func TestARunWithoutABranchIsRefusedNotRecorded(t *testing.T) {
	f := newFixture(t)
	body := []byte(`{"action":"completed","workflow_run":{"id":42,"status":"queued"},
		"repository":{"full_name":"khanblair/marshal","name":"marshal","owner":{"login":"khanblair"}}}`)
	if err := f.svc.Delivery(context.Background(), githubapp.Event{
		Kind: "workflow_run", Body: body,
	}); err != nil {
		t.Fatalf("a delivery with no branch must be accepted and ignored: %v", err)
	}
	if len(f.bus.topics) != 0 {
		t.Fatalf("a run with no branch was published: %v", f.bus.topics)
	}
}

func TestThePollOfDefaultBranchesFindsRunsNobodyAnnounced(t *testing.T) {
	f := newFixture(t)
	f.forge.runs = []gh.WorkflowRun{
		{ID: 77, Name: "ci", Branch: "main", Status: "completed", Conclusion: "failure", URL: "https://example.test/77"},
		{ID: 76, Name: "ci", Branch: "main", Status: "completed", Conclusion: "success", URL: "https://example.test/76"},
		{ID: 78, Name: "lint", Branch: "main", Status: "completed", Conclusion: "success", URL: "https://example.test/78"},
	}
	if err := f.svc.PollDefaultBranches(context.Background()); err != nil {
		t.Fatalf("poll the default branches: %v", err)
	}
	if len(f.forge.listBrancs) != 1 || f.forge.listBrancs[0] != "main" {
		t.Fatalf("the check asked about %v, want [main]", f.forge.listBrancs)
	}
	newest := readRun(t, f.store, "77")
	if newest.Status != string(protocol.CIStateFailed) || newest.CardID != nil || newest.Branch != "main" {
		t.Fatalf("the newest ci run is %+v, want failed, on main, with no card", newest)
	}
	if other := readRun(t, f.store, "78"); other.Workflow != "lint" || other.Status != string(protocol.CIStatePassed) {
		t.Fatalf("the other workflow's run is %+v", other)
	}
	if len(f.forge.reruns) != 0 || len(f.worker.sent) != 0 {
		t.Fatalf("a failure on main with no card started a fix: reruns %v, messages %v", f.forge.reruns, f.worker.sent)
	}
	snapshot, err := f.svc.Snapshot(context.Background())
	if err != nil || len(snapshot.Projects) != 1 || snapshot.Projects[0].Status != protocol.CIStateFailed {
		t.Fatalf("CI health after the check is %+v (err %v), want main failing", snapshot, err)
	}
}

func TestTheDefaultBranchIsAskedAboutOnlyOnceInAWhile(t *testing.T) {
	f := newFixture(t)
	f.forge.runs = []gh.WorkflowRun{{ID: 77, Name: "ci", Branch: "main", Status: "completed", Conclusion: "success"}}
	for i := 0; i < 3; i++ {
		if err := f.svc.PollDefaultBranches(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.forge.listBrancs) != 1 {
		t.Fatalf("the forge was asked %d times in a row, want once", len(f.forge.listBrancs))
	}
}

func TestOneProjectTheForgeCannotAnswerDoesNotStopTheCheck(t *testing.T) {
	f := newFixture(t)
	f.forge.listErr = errors.New("rate limited")
	if err := f.svc.PollDefaultBranches(context.Background()); err != nil {
		t.Fatalf("a forge error stopped the whole check: %v", err)
	}
}

func TestCheckingDefaultBranchesWithoutAForgeSaysSo(t *testing.T) {
	f := newFixture(t, func(o *ci.Options) { o.Forge = nil })
	if err := f.svc.PollDefaultBranches(context.Background()); !errors.Is(err, ci.ErrNoForge) {
		t.Fatalf("got %v, want ErrNoForge", err)
	}
}
