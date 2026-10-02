package integrator_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// withOwnerTop is multiLine with its first line changed.
func withLine(base, from, to string) string { return strings.Replace(base, from+"\n", to+"\n", 1) }

func TestDirtyFilesTheCardDoesNotTouchStayWhileTheCardLands(t *testing.T) {
	e := newEnv(t)
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	e.write(e.repo, "a.txt", "a edited by the owner\n")

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged", result)
	}
	if got := e.read(e.repo, "a.txt"); got != "a edited by the owner\n" {
		t.Errorf("a.txt = %q, want the owner's edit kept", got)
	}
	if got := e.read(e.repo, "b.txt"); got != "b by the card\n" {
		t.Errorf("b.txt = %q, want the card's change", got)
	}
	if status := e.status(); status != "M a.txt" && status != " M a.txt" {
		t.Errorf("status = %q, want only the owner's own edit", status)
	}
	if history := e.integration().History; len(history) != 1 || history[0].Summary == "" {
		t.Errorf("history = %+v, want one delivery", history)
	}
}

func TestDirtyEditsToTheSameFileInOtherPlacesAreMergedIntoTheFolder(t *testing.T) {
	e := newEnv(t, withTester(passing()))
	card := e.card("Edit the end of a", map[string]string{"a.txt": withLine(multiLine, "ten", "TEN by the card")})
	mine := withLine(multiLine, "one", "ONE by the owner")
	e.write(e.repo, "a.txt", mine)
	e.write(e.repo, "notes.txt", "the owner's notes\n")

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged", result)
	}
	both := withLine(mine, "ten", "TEN by the card")
	if got := e.read(e.repo, "a.txt"); got != both {
		t.Errorf("a.txt = %q, want both changes", got)
	}
	// The branch holds the card's work and not the owner's.
	if got := e.run(e.repo, "show", "main:a.txt"); got != strings.TrimRight(withLine(multiLine, "ten", "TEN by the card"), "\n") {
		t.Errorf("main:a.txt = %q, want only the card's change", got)
	}
	// The folder shows only the owner's own edit as uncommitted.
	diff := e.run(e.repo, "diff", "HEAD", "--", "a.txt")
	if !strings.Contains(diff, "+ONE by the owner") || strings.Contains(diff, "TEN") {
		t.Errorf("diff against HEAD = %q, want only the owner's edit", diff)
	}
	if got := e.read(e.repo, "notes.txt"); got != "the owner's notes\n" {
		t.Errorf("notes.txt = %q, want the owner's untracked file kept", got)
	}
	if staged := e.run(e.repo, "diff", "--cached", "--name-only"); staged != "" {
		t.Errorf("staged = %q, want the index at the new tip", staged)
	}
	if !strings.Contains(result.Note, "uncommitted changes were merged") {
		t.Errorf("note = %q, want it to say the owner's changes were merged", result.Note)
	}
	refs := e.run(e.repo, "for-each-ref", "--format=%(refname)", "refs/marshal/wip/")
	if !strings.HasPrefix(refs, "refs/marshal/wip/"+e.project.ID+"/") {
		t.Errorf("the snapshot is not pinned: %q", refs)
	}
	// The integrator branch never holds the owner's work.
	if got := e.run(e.repo, "show", protocol.IntegrationBranchName+":a.txt"); strings.Contains(got, "by the owner") {
		t.Errorf("the owner's edit reached the integrator branch")
	}
}

func TestDirtyEditsToTheSameLinesWithNoResolverLeaveTheFolderByteIdentical(t *testing.T) {
	e := newEnv(t)
	card := e.card("Edit a", map[string]string{"a.txt": withLine(multiLine, "one", "ONE by the card")})
	e.write(e.repo, "a.txt", withLine(multiLine, "one", "ONE by the owner"))
	e.write(e.repo, "notes.txt", "the owner's notes\n")
	folder := e.folderSnapshot()
	before := e.tip("main")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged {
		t.Fatalf("a clash was merged: %+v", result)
	}
	if !strings.Contains(result.Reason, "Your uncommitted changes clash with the merge in a.txt") {
		t.Errorf("reason = %q, want the clash named", result.Reason)
	}
	if got := e.folderSnapshot(); got != folder {
		t.Errorf("the owner's folder changed:\nbefore %q\nafter  %q", folder, got)
	}
	if e.tip("main") != before {
		t.Errorf("main moved")
	}
	if refs := e.run(e.repo, "for-each-ref", "--format=%(refname)", "refs/marshal/wip/"); refs != "" {
		t.Errorf("an unused snapshot was kept: %q", refs)
	}
	// Only the delivery stopped: the tested merge waits on the integrator branch for a retry.
	state := e.integration()
	if state.AheadBy != 2 || state.State != protocol.IntegratorStateWaiting || !strings.Contains(state.Message, card.Key) {
		t.Errorf("state = %+v, want waiting on the card with the integrator branch 2 commits ahead (the card's and its merge)", state)
	}
}

func TestDirtyEditsToTheSameLinesWithAResolverKeepBothChanges(t *testing.T) {
	resolver := keepBoth("Kept the card's line and the owner's line.")
	e := newEnv(t, withResolver(resolver))
	card := e.card("Edit a", map[string]string{"a.txt": withLine(multiLine, "one", "ONE by the card")})
	e.write(e.repo, "a.txt", withLine(multiLine, "one", "ONE by the owner"))

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged", result)
	}
	got := e.read(e.repo, "a.txt")
	if !strings.Contains(got, "ONE by the card") || !strings.Contains(got, "ONE by the owner") || strings.Contains(got, "<<<<") {
		t.Errorf("a.txt = %q, want both changes and no markers", got)
	}
	if committed := e.run(e.repo, "show", "main:a.txt"); strings.Contains(committed, "by the owner") {
		t.Errorf("the owner's line was committed: %q", committed)
	}
	tasks := resolver.taskList()
	if len(tasks) != 1 || tasks[0].Kind != integrator.MergeTaskWIP || !slices.Equal(tasks[0].Conflicts, []string{"a.txt"}) {
		t.Fatalf("tasks = %+v, want one WIP task for a.txt", tasks)
	}
	if status := e.status(); status != "M a.txt" && status != " M a.txt" {
		t.Errorf("status = %q, want the owner's work as uncommitted changes", status)
	}
}

func TestAnUntrackedFileCollidingWithAnAddedFileLeavesTheFolderAlone(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add new", map[string]string{"new.txt": "the card's file\n"})
	e.write(e.repo, "new.txt", "the owner's own file\n")
	folder := e.folderSnapshot()

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "new.txt") {
		t.Fatalf("result = %+v, want a stop that names new.txt", result)
	}
	if got := e.folderSnapshot(); got != folder {
		t.Errorf("the owner's folder changed")
	}
}

func TestAnUntrackedFileTheCardAlsoAddsIsMergedWhenTheResolverKeepsBoth(t *testing.T) {
	e := newEnv(t, withResolver(keepBoth("Kept both versions of new.txt.")))
	card := e.card("Add new", map[string]string{"new.txt": "the card's file\n"})
	e.write(e.repo, "new.txt", "the owner's own file\n")

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged", result)
	}
	got := e.read(e.repo, "new.txt")
	if !strings.Contains(got, "the card's file") || !strings.Contains(got, "the owner's own file") {
		t.Errorf("new.txt = %q, want both versions", got)
	}
}

func TestAFileTheOwnerDeletedAndTheCardModifiedIsAClash(t *testing.T) {
	e := newEnv(t)
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	if err := os.Remove(filepath.Join(e.repo, "b.txt")); err != nil {
		t.Fatal(err)
	}
	folder := e.folderSnapshot()

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "b.txt") {
		t.Fatalf("result = %+v, want a stop that names b.txt", result)
	}
	if got := e.folderSnapshot(); got != folder {
		t.Errorf("the folder changed: the file the owner deleted must stay deleted")
	}
	if fileExists(filepath.Join(e.repo, "b.txt")) {
		t.Errorf("b.txt came back")
	}
}

func TestAFileTheOwnerDeletedStaysDeletedWhenTheCardTouchesOtherFiles(t *testing.T) {
	e := newEnv(t)
	card := e.card("Edit c", map[string]string{"c.txt": "c by the card\n"})
	if err := os.Remove(filepath.Join(e.repo, "b.txt")); err != nil {
		t.Fatal(err)
	}

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged", result)
	}
	if fileExists(filepath.Join(e.repo, "b.txt")) {
		t.Errorf("the file the owner deleted came back")
	}
	if got := e.read(e.repo, "c.txt"); got != "c by the card\n" {
		t.Errorf("c.txt = %q, want the card's change", got)
	}
	if status := e.status(); status != " D b.txt" && status != "D  b.txt" {
		t.Errorf("status = %q, want the owner's deletion still uncommitted", status)
	}
}

func TestATargetNoWorktreeHasCheckedOutMovesWithoutTouchingTheFolder(t *testing.T) {
	e := newEnv(t)
	e.useBranch("development")
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	e.write(e.repo, "a.txt", "a edited by the owner\n")
	mainTip := e.tip("main")

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged", result)
	}
	if e.tip("development") != result.Commit || e.tip("main") != mainTip {
		t.Errorf("development = %s, main = %s; want only development moved to %s", e.tip("development"), e.tip("main"), result.Commit)
	}
	if got := e.read(e.repo, "b.txt"); got != "b base\n" {
		t.Errorf("b.txt = %q, want the owner's folder untouched", got)
	}
	if got := e.read(e.repo, "a.txt"); got != "a edited by the owner\n" {
		t.Errorf("a.txt = %q, want the owner's edit kept", got)
	}
	if result.Note != "Your folder is on main, so its files did not change." {
		t.Errorf("note = %q, want the folder's branch named", result.Note)
	}
	if _, note := e.mergeColumns(card.ID); note != result.Note {
		t.Errorf("the card's merge note = %q, want the delivery's note", note)
	}
}

func TestAFolderOnADetachedHeadIsToldItsFilesDidNotChange(t *testing.T) {
	e := newEnv(t)
	e.useBranch("development")
	e.run(e.repo, "checkout", "--quiet", "--detach")
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})

	result := e.merge(card)

	if !result.Merged || result.Note != "Your folder is on a detached commit, so its files did not change." {
		t.Errorf("result = %+v, want merged with the detached note", result)
	}
}

func TestATargetOpenInAnotherWorktreeStopsTheDelivery(t *testing.T) {
	e := newEnv(t)
	e.useBranch("development")
	other := filepath.Join(t.TempDir(), "other")
	e.run(e.repo, "worktree", "add", "--quiet", other, "development")
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	before := e.tip("development")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged {
		t.Fatalf("result = %+v, want the delivery stopped", result)
	}
	if !strings.Contains(result.Reason, "Branch development is open in another worktree") ||
		!strings.Contains(result.Reason, "Close it or choose another integration branch.") {
		t.Errorf("reason = %q, want the other worktree named", result.Reason)
	}
	if e.tip("development") != before {
		t.Errorf("development moved")
	}
}

func TestAnIndexLockStopsTheDeliveryUntilARetryFindsTheFolderFree(t *testing.T) {
	tests := passing()
	e := newEnv(t, withTester(tests))
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	lock := filepath.Join(e.repo, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	before := e.tip("main")

	result := e.merge(card)
	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "index lock") {
		t.Fatalf("result = %+v, want a stop about the index lock", result)
	}
	if e.tip("main") != before || e.read(e.repo, "b.txt") != "b base\n" {
		t.Errorf("the target or the folder changed while the index was locked")
	}
	if !fileExists(lock) {
		t.Errorf("the lock someone else holds was removed")
	}

	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Retry(e.ctx, card.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	e.svc.Wait()

	if got := e.state(card.ID); got.State != protocol.CardStateDone {
		t.Fatalf("card state after the retry = %s, want done", got.State)
	}
	if e.read(e.repo, "b.txt") != "b by the card\n" || e.status() != "" {
		t.Errorf("the folder was not delivered to after the retry")
	}
	if tests.count() != 1 {
		t.Errorf("the tests ran %d times, want 1: a retry only delivers", tests.count())
	}
}

func TestAFolderInTheMiddleOfAMergeIsNotTouched(t *testing.T) {
	e := newEnv(t)
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	e.write(e.repo, ".git/MERGE_HEAD", e.tip("main")+"\n")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "in the middle of a merge") {
		t.Fatalf("result = %+v, want a stop that says the folder is in the middle of a merge", result)
	}
	if e.read(e.repo, "b.txt") != "b base\n" {
		t.Errorf("the folder changed")
	}
}

func TestATargetThatKeepsMovingStopsTheMergeWithARetryableSentence(t *testing.T) {
	tests := passing()
	e := newEnv(t, withTester(tests))
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	tests.onRun = func(call int) {
		e.ownerCommits("c.txt", strings.Repeat("c\n", call))
	}

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "kept moving") {
		t.Fatalf("result = %+v, want a stop that says main kept moving", result)
	}
	if tests.count() != 3 {
		t.Errorf("the tests ran %d times, want 3 attempts", tests.count())
	}
	if got := e.state(card.ID); got.State != protocol.CardStateNeeds {
		t.Errorf("card state = %s, want needs", got.State)
	}
	if ahead := e.run(e.repo, "rev-list", "--count", "main.."+protocol.IntegrationBranchName); ahead != "0" {
		t.Errorf("the integrator branch holds %s commits main does not have, want 0", ahead)
	}
}

func TestATargetNoWorktreeHasCheckedOutThatMovesDuringTheTestsIsMergedAgain(t *testing.T) {
	tests := passing()
	e := newEnv(t, withTester(tests))
	e.useBranch("development")
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	tests.onRun = func(call int) {
		if call == 1 {
			e.commitOn("development", "main", map[string]string{"c.txt": "c by someone else\n"})
		}
	}

	result := e.merge(card)

	if !result.Merged || tests.count() != 2 {
		t.Fatalf("result = %+v after %d test runs, want merged on the second attempt", result, tests.count())
	}
	if got := e.run(e.repo, "show", "development:c.txt"); got != "c by someone else" {
		t.Errorf("development:c.txt = %q, want the other commit kept", got)
	}
	if got := e.run(e.repo, "show", "development:b.txt"); got != "b by the card" {
		t.Errorf("development:b.txt = %q, want the card's change", got)
	}
}

func TestWorkOfACardThatWasPulledBackIsNotDeliveredWithTheNextCard(t *testing.T) {
	e := newEnv(t)
	first := e.card("First", map[string]string{"b.txt": "b by the first\n"})
	lock := filepath.Join(e.repo, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := e.merge(first); r.Merged {
		t.Fatalf("the first delivery should have stopped: %+v", r)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	// The owner pulls the first card back to work on it.
	if _, err := e.proj.MoveCard(e.ctx, first.ID, protocol.MoveCardRequest{State: protocol.CardStateWorking}); err != nil {
		t.Fatalf("pull the card back: %v", err)
	}
	second := e.card("Second", map[string]string{"c.txt": "c by the second\n"})

	if r := e.merge(second); !r.Merged {
		t.Fatalf("second: %+v", r)
	}

	if got := e.read(e.repo, "b.txt"); got != "b base\n" {
		t.Errorf("b.txt = %q: the pulled back card's work was delivered", got)
	}
	if got := e.read(e.repo, "c.txt"); got != "c by the second\n" {
		t.Errorf("c.txt = %q, want the second card's work", got)
	}
	if refs := e.run(e.repo, "for-each-ref", "--format=%(refname)", "refs/marshal/integrator/"); refs == "" {
		t.Errorf("the discarded integrator work was not pinned")
	}
}

func TestACardWhoseDeliveryStoppedIsDeliveredTogetherWithTheNextOne(t *testing.T) {
	e := newEnv(t)
	first := e.card("First", map[string]string{"b.txt": "b by the first\n"})
	lock := filepath.Join(e.repo, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := e.merge(first); r.Merged {
		t.Fatalf("the first delivery should have stopped: %+v", r)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	second := e.card("Second", map[string]string{"c.txt": "c by the second\n"})

	if r := e.merge(second); !r.Merged {
		t.Fatalf("second: %+v", r)
	}

	if e.read(e.repo, "b.txt") != "b by the first\n" || e.read(e.repo, "c.txt") != "c by the second\n" {
		t.Errorf("the folder does not hold both cards' work")
	}
	if got := e.state(first.ID); got.State != protocol.CardStateDone {
		t.Errorf("first card state = %s, want done: it was delivered with the second", got.State)
	}
	if e.integration().State != protocol.IntegratorStateIdle {
		t.Errorf("state = %s, want idle once everything is delivered", e.integration().State)
	}
}

// editing is a resolver that keeps both sides, and, before each answer, lets the owner's editor write
// a file in their folder, as one does while a person types.
func editing(edit func(call int)) *resolverFunc {
	inner := keepBoth("Kept both sides.")
	calls := 0
	return &resolverFunc{fn: func(task integrator.MergeTask) (integrator.Verdict, error) {
		calls++
		edit(calls)
		return inner.fn(task)
	}}
}

func TestAnEditorWritingWhileTheOwnersChangesAreMergedMakesTheDeliveryReadTheFolderAgain(t *testing.T) {
	var e *env
	resolver := editing(func(call int) {
		if call == 1 {
			e.write(e.repo, "c.txt", "typed while the merge ran\n")
		}
	})
	e = newEnv(t, withResolver(resolver))
	card := e.card("Edit a", map[string]string{"a.txt": withLine(multiLine, "one", "ONE by the card")})
	e.write(e.repo, "a.txt", withLine(multiLine, "one", "ONE by the owner"))

	result := e.merge(card)

	if !result.Merged {
		t.Fatalf("result = %+v, want merged on the second read", result)
	}
	if got := e.read(e.repo, "c.txt"); got != "typed while the merge ran\n" {
		t.Errorf("c.txt = %q, want what the editor wrote kept", got)
	}
	if got := e.read(e.repo, "a.txt"); !strings.Contains(got, "ONE by the card") || !strings.Contains(got, "ONE by the owner") {
		t.Errorf("a.txt = %q, want both changes", got)
	}
	if n := len(resolver.taskList()); n != 2 {
		t.Errorf("the resolver was asked %d times, want 2: once for each read of the folder", n)
	}
	refs := strings.Fields(e.run(e.repo, "for-each-ref", "--format=%(refname)", "refs/marshal/wip/"))
	if len(refs) != 1 {
		t.Errorf("snapshots kept = %v, want only the one that was used", refs)
	}
}

func TestAFolderThatKeepsChangingIsLeftAloneWithARetryableSentence(t *testing.T) {
	var e *env
	resolver := editing(func(call int) {
		e.write(e.repo, "c.txt", strings.Repeat("typed\n", call))
	})
	e = newEnv(t, withResolver(resolver))
	card := e.card("Edit a", map[string]string{"a.txt": withLine(multiLine, "one", "ONE by the card")})
	mine := withLine(multiLine, "one", "ONE by the owner")
	e.write(e.repo, "a.txt", mine)
	before := e.tip("main")

	result := e.merge(card)

	e.assertStopped(t, card, result)

	if result.Merged || !strings.Contains(result.Reason, "kept changing") || !strings.Contains(result.Reason, "Retry") {
		t.Fatalf("result = %+v, want a stop that says the folder kept changing and to retry", result)
	}
	if e.tip("main") != before {
		t.Errorf("main moved")
	}
	if got := e.read(e.repo, "a.txt"); got != mine {
		t.Errorf("a.txt = %q, want the owner's file untouched", got)
	}
	if got := e.read(e.repo, "c.txt"); got != strings.Repeat("typed\n", 2) {
		t.Errorf("c.txt = %q, want the editor's last write untouched", got)
	}
	if refs := e.run(e.repo, "for-each-ref", "--format=%(refname)", "refs/marshal/wip/"); refs != "" {
		t.Errorf("snapshots kept = %q, want none for a folder that was not written", refs)
	}
}

func TestSwitchingTheIntegrationBranchDoesNotDeliverEarlierWorkToTheNewOne(t *testing.T) {
	e := newEnv(t)
	e.run(e.repo, "branch", "--no-track", "development", "main")
	first := e.card("First", map[string]string{"c.txt": "c by the first\n"})
	if r := e.merge(first); !r.Merged {
		t.Fatalf("first: %+v", r)
	}
	// The owner now chooses development, which has none of the first card's work.
	e.chooseBranch("development")
	second := e.card("Second", map[string]string{"b.txt": "b by the second\n"})

	if r := e.merge(second); !r.Merged {
		t.Fatalf("second: %+v", r)
	}

	if got := e.run(e.repo, "show", "development:b.txt"); got != "b by the second" {
		t.Errorf("development:b.txt = %q, want the second card's change", got)
	}
	if got := e.run(e.repo, "show", "development:c.txt"); got != "c base" {
		t.Errorf("development:c.txt = %q: the first card's work reached a branch it was not merged for", got)
	}
	if refs := e.run(e.repo, "for-each-ref", "--format=%(refname)", "refs/marshal/integrator/"); refs == "" {
		t.Errorf("the integrator work that was set aside was not pinned")
	}
}

func TestAWorkInProgressResolverThatIsNotConfidentLeavesTheFolderAlone(t *testing.T) {
	resolver := answering(integrator.Verdict{
		Resolved: true, Confident: false, Summary: "Two edits of one line.", Questions: []string{"Whose line wins?"},
	}, nil)
	e := newEnv(t, withResolver(resolver))
	card := e.card("Edit a", map[string]string{"a.txt": withLine(multiLine, "one", "ONE by the card")})
	e.write(e.repo, "a.txt", withLine(multiLine, "one", "ONE by the owner"))
	folder := e.folderSnapshot()
	before := e.tip("main")

	result := e.merge(card)
	e.assertStopped(t, card, result)

	for _, want := range []string{"Your uncommitted changes clash with the merge in a.txt", "not sure", "Whose line wins?"} {
		if !strings.Contains(result.Reason, want) {
			t.Errorf("reason = %q, want it to contain %q", result.Reason, want)
		}
	}
	if e.folderSnapshot() != folder || e.tip("main") != before {
		t.Errorf("the folder or the branch changed")
	}
	if state := e.integration(); state.AheadBy != 2 || state.State != protocol.IntegratorStateWaiting {
		t.Errorf("state = %+v, want the tested merge kept on the integrator branch and waiting", state)
	}
}

func TestAWorkInProgressResolverThatLeavesMarkersIsNotBelieved(t *testing.T) {
	resolver := answering(integrator.Verdict{Resolved: true, Confident: true, Summary: "All fine."}, nil)
	e := newEnv(t, withResolver(resolver))
	card := e.card("Edit a", map[string]string{"a.txt": withLine(multiLine, "one", "ONE by the card")})
	e.write(e.repo, "a.txt", withLine(multiLine, "one", "ONE by the owner"))
	folder := e.folderSnapshot()

	result := e.merge(card)
	e.assertStopped(t, card, result)

	if !strings.Contains(result.Reason, "conflict markers are still in a.txt") {
		t.Errorf("reason = %q, want the markers named", result.Reason)
	}
	if e.folderSnapshot() != folder {
		t.Errorf("the owner's folder changed")
	}
}

func TestAFolderWithFilesInConflictIsNotTouched(t *testing.T) {
	e := newEnv(t)
	// A stash that conflicts when it is applied leaves files in conflict and no merge in progress.
	e.write(e.repo, "a.txt", withLine(multiLine, "one", "ONE stashed"))
	e.run(e.repo, "stash", "push", "--quiet")
	e.ownerCommits("a.txt", withLine(multiLine, "one", "ONE committed"))
	if _, err := e.git.Run(e.ctx, e.repo, "stash", "pop", "--quiet"); err == nil {
		t.Fatalf("the stash was expected to conflict")
	}
	card := e.card("Edit b", map[string]string{"b.txt": "b by the card\n"})
	folder := e.folderSnapshot()

	result := e.merge(card)
	e.assertStopped(t, card, result)

	if !strings.Contains(result.Reason, "files in conflict") {
		t.Errorf("reason = %q, want the files in conflict named", result.Reason)
	}
	if e.folderSnapshot() != folder {
		t.Errorf("the owner's folder changed")
	}
}

func TestACardWithNoBranchIsStoppedWithAReason(t *testing.T) {
	e := newEnv(t)
	card, err := e.proj.CreateCard(e.ctx, e.project.ID, protocol.CreateCardRequest{Title: "No branch"})
	if err != nil {
		t.Fatal(err)
	}
	card = e.ready(card.ID)

	result := e.merge(card)

	e.assertStopped(t, card, result)
	if !strings.Contains(result.Reason, "no branch to merge") {
		t.Errorf("reason = %q, want it to say there is no branch to merge", result.Reason)
	}
}
