package integrator_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestACleanMergeIntoACleanFolderDeliversToTheBranchAndTheFolder(t *testing.T) {
	tests := passing()
	e := newEnv(t, withTester(tests))
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	before := e.tip("main")

	result := e.merge(card)

	if !result.Merged || result.Commit == "" {
		t.Fatalf("result = %+v, want merged", result)
	}
	if got := e.tip("main"); got != result.Commit || got == before {
		t.Errorf("main = %s, want the merge commit %s (was %s)", got, result.Commit, before)
	}
	if got := e.read(e.repo, "b.txt"); got != "b by the card\n" {
		t.Errorf("the owner's b.txt = %q, want the card's change", got)
	}
	if status := e.status(); status != "" {
		t.Errorf("status = %q, want a clean folder: the merge must not show as a reverse change", status)
	}
	if got := e.state(card.ID); got.State != protocol.CardStateDone {
		t.Errorf("card state = %s, want done", got.State)
	}
	if got := e.tip(protocol.IntegrationBranchName); got != result.Commit {
		t.Errorf("integrator = %s, want it at the target tip %s", got, result.Commit)
	}
	if phase, note := e.mergeColumns(card.ID); phase != "" || note != "" {
		t.Errorf("merge columns = %q, %q; want them cleared", phase, note)
	}
	if got := tests.files; len(got) != 1 || !slices.Equal(got[0], []string{"b.txt"}) {
		t.Errorf("the tests ran over %v, want [b.txt]", got)
	}
	if branches := e.run(e.repo, "branch", "--list", "marshal/backup/*"); !strings.Contains(branches, "marshal/backup/main-") {
		t.Errorf("no backup branch was made: %q", branches)
	}
}

func TestMergeAnnouncesEveryPhaseOnTheCardAndTheProject(t *testing.T) {
	e := newEnv(t, withTester(passing()))
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.merge(card)

	got := e.progress()
	want := []protocol.MergePhase{
		protocol.MergePhaseQueued, protocol.MergePhaseResolving, protocol.MergePhaseResolving,
		protocol.MergePhaseTesting, protocol.MergePhaseLanding, "",
	}
	if cardPhases := got[string(protocol.CardTopic(card.ID))]; !slices.Equal(cardPhases, want) {
		t.Errorf("card phases = %v, want %v", cardPhases, want)
	}
	if projectPhases := got[string(protocol.ProjectTopic(e.project.ID))]; !slices.Equal(projectPhases, want) {
		t.Errorf("project phases = %v, want %v", projectPhases, want)
	}
}

func TestTheIntegratorWorksInItsOwnFolderAndNeverInTheOwners(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.merge(card)

	ws := integrator.WorkspaceDir(e.dataDir, e.project.ID)
	if got := e.run(ws, "rev-parse", "--abbrev-ref", "HEAD"); got != protocol.IntegrationBranchName {
		t.Errorf("the workspace is on %q, want %q", got, protocol.IntegrationBranchName)
	}
	if got := e.run(e.repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("the owner's folder is on %q, want main", got)
	}
}

func TestAConflictWithNoResolverLeavesEverythingAsItWas(t *testing.T) {
	e := newEnv(t)
	card := e.card("Edit a", map[string]string{"a.txt": "a by the card\n"})
	e.write(e.repo, "a.txt", "a on main\n")
	e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--all", "--message", "Main edits a")
	before := e.tip("main")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged {
		t.Fatalf("a conflict merged: %+v", result)
	}
	if !strings.Contains(result.Reason, "a.txt") || !strings.Contains(result.Reason, "not changed") {
		t.Errorf("reason = %q, want the file named and the target said to be unchanged", result.Reason)
	}
	got := e.state(card.ID)
	if got.State != protocol.CardStateNeeds || got.NeedsReason == nil || got.NeedsReason.Kind != protocol.NeedsReasonKindConflict {
		t.Errorf("card = %s %+v, want needs with a conflict reason", got.State, got.NeedsReason)
	}
	if e.tip("main") != before {
		t.Errorf("main moved on a conflict")
	}
	if e.tip(protocol.IntegrationBranchName) != before {
		t.Errorf("the integrator branch holds more than main after a conflict")
	}
}

func TestAConflictTheResolverSettlesIsMergedAndRecorded(t *testing.T) {
	resolver := keepBoth("Kept both sides of a.txt.")
	e := newEnv(t, withResolver(resolver), withTester(passing()))
	card := e.card("Edit a", map[string]string{"a.txt": "card line\n"})
	e.write(e.repo, "a.txt", "main line\n")
	e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--all", "--message", "Main edits a")

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged", result)
	}
	got := e.run(e.repo, "show", "main:a.txt")
	if !strings.Contains(got, "card line") || !strings.Contains(got, "main line") || strings.Contains(got, "<<<<") {
		t.Errorf("main:a.txt = %q, want both sides and no markers", got)
	}
	tasks := resolver.taskList()
	if len(tasks) != 1 || tasks[0].Kind != integrator.MergeTaskCard || !slices.Equal(tasks[0].Conflicts, []string{"a.txt"}) {
		t.Fatalf("tasks = %+v, want one card task for a.txt", tasks)
	}
	if tasks[0].Target != "main" || tasks[0].ProjectID != e.project.ID || tasks[0].Worktree != integrator.WorkspaceDir(e.dataDir, e.project.ID) {
		t.Errorf("task = %+v, want the project, the target, and the integrator workspace", tasks[0])
	}
	if len(tasks[0].Cards) != 1 || tasks[0].Cards[0].CardID != card.ID || tasks[0].Cards[0].Title != "Edit a" ||
		!slices.Equal(tasks[0].Cards[0].Changed, []string{"a.txt"}) {
		t.Errorf("task cards = %+v, want the card with its changed files", tasks[0].Cards)
	}
	history := e.integration().History
	if len(history) != 1 || history[0].Resolved != 1 || !strings.Contains(history[0].Summary, "Kept both sides") {
		t.Errorf("history = %+v, want one delivery with 1 resolved conflict and the resolver's summary", history)
	}
}

func TestAResolverThatIsNotConfidentSendsTheCardToNeedsWithItsQuestions(t *testing.T) {
	resolver := answering(integrator.Verdict{
		Resolved: true, Confident: false, Summary: "Two edits of one line.", Questions: []string{"Which retry count wins?"},
	}, nil)
	e := newEnv(t, withResolver(resolver))
	card := e.card("Edit a", map[string]string{"a.txt": "card line\n"})
	e.write(e.repo, "a.txt", "main line\n")
	e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--all", "--message", "Main edits a")
	before := e.tip("main")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged {
		t.Fatalf("an unsure resolver's work was merged: %+v", result)
	}
	for _, want := range []string{"a.txt", "not sure", "Two edits of one line.", "Which retry count wins?"} {
		if !strings.Contains(result.Reason, want) {
			t.Errorf("reason = %q, want it to contain %q", result.Reason, want)
		}
	}
	if e.tip("main") != before || e.tip(protocol.IntegrationBranchName) != before {
		t.Errorf("a branch moved after an unsure resolver")
	}
}

func TestAResolverThatSaysResolvedWhileMarkersRemainIsNotBelieved(t *testing.T) {
	resolver := answering(integrator.Verdict{Resolved: true, Confident: true, Summary: "All fine."}, nil)
	e := newEnv(t, withResolver(resolver))
	card := e.card("Edit a", map[string]string{"a.txt": "card line\n"})
	e.write(e.repo, "a.txt", "main line\n")
	e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--all", "--message", "Main edits a")
	before := e.tip("main")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "conflict markers are still in a.txt") {
		t.Fatalf("result = %+v, want a stop that names the markers left in a.txt", result)
	}
	if e.tip("main") != before || e.tip(protocol.IntegrationBranchName) != before {
		t.Errorf("a branch moved although the resolver left markers")
	}
	ws := integrator.WorkspaceDir(e.dataDir, e.project.ID)
	if status := e.run(ws, "status", "--porcelain"); status != "" {
		t.Errorf("the workspace was left dirty: %q", status)
	}
}

func TestAResolverThatFailsSendsTheCardToNeeds(t *testing.T) {
	resolver := answering(integrator.Verdict{}, os.ErrDeadlineExceeded)
	e := newEnv(t, withResolver(resolver))
	card := e.card("Edit a", map[string]string{"a.txt": "card line\n"})
	e.write(e.repo, "a.txt", "main line\n")
	e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--all", "--message", "Main edits a")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "could not finish") {
		t.Fatalf("result = %+v, want a stop that says the Integrator could not finish", result)
	}
}

func TestFailingTestsKeepTheTargetAndResetTheIntegratorBranch(t *testing.T) {
	e := newEnv(t, withTester(failing("TestAddRetry failed")))
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	before := e.tip("main")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged {
		t.Fatalf("the target moved with failing tests: %+v", result)
	}
	got := e.state(card.ID)
	if got.State != protocol.CardStateNeeds || got.NeedsReason == nil || got.NeedsReason.Kind != protocol.NeedsReasonKindCIFailed ||
		!strings.Contains(got.NeedsReason.Text, "TestAddRetry failed") {
		t.Errorf("card = %s %+v, want needs with the failing test named", got.State, got.NeedsReason)
	}
	if e.tip("main") != before {
		t.Errorf("main moved with failing tests")
	}
	if e.tip(protocol.IntegrationBranchName) != before {
		t.Errorf("the integrator branch was not reset to its pre-merge tip")
	}
	if e.read(e.repo, "b.txt") != "b base\n" || e.status() != "" {
		t.Errorf("the owner's folder changed")
	}
	if _, note := e.mergeColumns(card.ID); !strings.Contains(note, "TestAddRetry failed") {
		t.Errorf("merge note = %q, want the reason kept", note)
	}
}

func TestATesterThatCannotRunStopsTheCardAsCIFailed(t *testing.T) {
	tests := passing()
	tests.err = os.ErrNotExist
	e := newEnv(t, withTester(tests))
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "could not run") {
		t.Errorf("result = %+v, want a stop that says the tests could not run", result)
	}
}

func TestACardThatIsNotReadyIsRefused(t *testing.T) {
	e := newEnv(t)
	card, err := e.proj.CreateCard(e.ctx, e.project.ID, protocol.CreateCardRequest{Title: "Not yet"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Merge(e.ctx, card.ID); !refusedWith(err, "merge_not_ready") {
		t.Fatalf("Merge of a backlog card = %v, want a merge_not_ready refusal", err)
	}
}

func TestACardWhoseBranchIsAlreadyInTheTargetIsDoneWithoutADelivery(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.run(e.repo, "merge", "--quiet", "--ff-only", e.run(e.repo, "rev-parse", "marshal/add-retry"))
	tip := e.tip("main")

	result := e.merge(card)

	if !result.Merged || result.Commit != tip || !strings.Contains(result.Note, "Already in main") {
		t.Errorf("result = %+v, want merged at the tip with a note that it was already in", result)
	}
	if got := e.state(card.ID); got.State != protocol.CardStateDone {
		t.Errorf("card state = %s, want done", got.State)
	}
	if len(e.integration().History) != 0 {
		t.Errorf("a card that was already in the target got a delivery row")
	}
}

func TestTheTargetMovingDuringTheTestsMakesTheMergeStartOver(t *testing.T) {
	tests := passing()
	e := newEnv(t, withTester(tests))
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	tests.onRun = func(call int) {
		if call == 1 {
			// The owner commits on main in their own folder while the tests run.
			e.write(e.repo, "c.txt", "c by the owner\n")
			e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--all", "--message", "Owner commits")
		}
	}

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged after starting over", result)
	}
	if tests.count() != 2 {
		t.Errorf("the tests ran %d times, want 2: once for each attempt", tests.count())
	}
	if got := e.run(e.repo, "show", "main:c.txt"); got != "c by the owner" {
		t.Errorf("main:c.txt = %q, want the owner's commit kept", got)
	}
	if got := e.run(e.repo, "show", "main:b.txt"); got != "b by the card" {
		t.Errorf("main:b.txt = %q, want the card's change", got)
	}
	if e.status() != "" {
		t.Errorf("status = %q, want a clean folder", e.status())
	}
}

func TestABackupBranchThatCannotBeMadeDoesNotStopTheMerge(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	// A branch with the backup's own name is already there.
	e.run(e.repo, "branch", "marshal/backup/main-"+card.ID[:8])

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want the merge to go on without its backup", result)
	}
}

func TestTwoCardsMergeOneAfterTheOther(t *testing.T) {
	e := newEnv(t)
	first := e.card("First", map[string]string{"b.txt": "b by the first\n"})
	second := e.card("Second", map[string]string{"c.txt": "c by the second\n"})

	if r := e.merge(first); !r.Merged {
		t.Fatalf("first: %+v", r)
	}
	if r := e.merge(second); !r.Merged {
		t.Fatalf("second: %+v", r)
	}
	if e.read(e.repo, "b.txt") != "b by the first\n" || e.read(e.repo, "c.txt") != "c by the second\n" || e.status() != "" {
		t.Errorf("the folder does not hold both cards' work cleanly: %q", e.status())
	}
	if n := len(e.integration().History); n != 2 {
		t.Errorf("history has %d rows, want 2", n)
	}
	if !fileExists(filepath.Join(integrator.WorkspaceDir(e.dataDir, e.project.ID), "c.txt")) {
		t.Errorf("the integrator workspace does not hold the second card's work")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// slowResolver waits until it is cancelled.
type slowResolver struct{}

func (slowResolver) Resolve(ctx context.Context, _ integrator.MergeTask) (integrator.Verdict, error) {
	<-ctx.Done()
	return integrator.Verdict{}, ctx.Err()
}

func TestAResolverThatTakesLongerThanItsTimeoutIsStopped(t *testing.T) {
	e := newEnv(t, withResolver(slowResolver{}), withDeps(func(d *integrator.Deps) { d.ResolveTimeout = 50 * time.Millisecond }))
	card := e.card("Edit a", map[string]string{"a.txt": "card line\n"})
	e.write(e.repo, "a.txt", "main line\n")
	e.run(e.repo, "-c", "commit.gpgsign=false", "commit", "--quiet", "--all", "--message", "Main edits a")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "could not finish") || !strings.Contains(result.Reason, "deadline") {
		t.Errorf("result = %+v, want a stop that says the Integrator ran out of time", result)
	}
}
