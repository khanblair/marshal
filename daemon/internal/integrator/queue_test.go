package integrator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// wireHook makes a card that becomes ready go to the queue, the way the daemon wires it.
func (e *env) wireHook() { e.proj.SetOnReadyToMerge(e.svc.Enqueue) }

func TestAnIdleProjectHasNothingToShow(t *testing.T) {
	e := newEnv(t)
	state := e.integration()

	if state.State != protocol.IntegratorStateIdle || state.Target != "main" ||
		state.IntegratorBranch != protocol.IntegrationBranchName || state.AheadBy != 0 {
		t.Errorf("state = %+v, want idle on main with the integrator branch level", state)
	}
	if state.Queue == nil || state.History == nil || len(state.Queue) != 0 || len(state.History) != 0 {
		t.Errorf("queue = %v, history = %v; want both empty and never null", state.Queue, state.History)
	}
	if state.ServerTime.Time().IsZero() {
		t.Errorf("the state has no server time")
	}
}

func TestTheQueueListsReadyCardsInTheOrderTheyBecameReady(t *testing.T) {
	e := newEnv(t)
	first := e.card("First", map[string]string{"b.txt": "b\n"})
	second := e.card("Second", map[string]string{"c.txt": "c\n"})

	state := e.integration()

	if len(state.Queue) != 2 {
		t.Fatalf("queue = %+v, want 2 cards", state.Queue)
	}
	for i, want := range []protocol.Card{first, second} {
		item := state.Queue[i]
		if item.CardID != want.ID || item.Position != i+1 || item.Phase != protocol.MergePhaseQueued ||
			item.Key != want.Key || item.Title != want.Title {
			t.Errorf("queue[%d] = %+v, want %s at position %d, queued", i, item, want.Key, i+1)
		}
	}
	if state.CurrentCardID != "" {
		t.Errorf("current = %q, want none", state.CurrentCardID)
	}
}

func TestAReadyCardIsMergedOnItsOwn(t *testing.T) {
	e := newEnv(t)
	e.wireHook()
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.svc.Wait()

	if got := e.state(card.ID); got.State != protocol.CardStateDone {
		t.Fatalf("card state = %s, want done", got.State)
	}
	if e.read(e.repo, "b.txt") != "b by the card\n" || e.status() != "" {
		t.Errorf("the owner's folder does not hold the card's work")
	}
}

func TestReadyCardsAreMergedInTheOrderTheyBecameReady(t *testing.T) {
	e := newEnv(t)
	e.wireHook()
	first := e.card("First", map[string]string{"b.txt": "b\n"})
	second := e.card("Second", map[string]string{"c.txt": "c\n"})
	third := e.card("Third", map[string]string{"a.txt": "a\n"})
	e.svc.Wait()

	history := e.integration().History
	if len(history) != 3 {
		t.Fatalf("history = %+v, want 3 deliveries", history)
	}
	// Newest first.
	for i, want := range []protocol.Card{third, second, first} {
		if history[i].CardID != want.ID {
			t.Errorf("history[%d] is %s, want %s", i, history[i].Key, want.Key)
		}
	}
}

func TestPausingStopsAutoMergeAndResumingMergesTheWaitingCards(t *testing.T) {
	e := newEnv(t)
	e.wireHook()
	state, err := e.svc.Pause(e.ctx, e.project.ID)
	if err != nil || state.State != protocol.IntegratorStatePaused {
		t.Fatalf("Pause = %+v, %v; want paused", state, err)
	}
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.svc.Wait()

	if got := e.state(card.ID); got.State != protocol.CardStateReady {
		t.Fatalf("card state = %s while paused, want it to wait in ready", got.State)
	}
	if got := e.integration(); got.State != protocol.IntegratorStatePaused || len(got.Queue) != 1 {
		t.Errorf("state = %+v, want paused with the card queued", got)
	}

	state, err = e.svc.Resume(e.ctx, e.project.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	e.svc.Wait()

	if state.State == protocol.IntegratorStatePaused {
		t.Errorf("state after Resume = %s", state.State)
	}
	if got := e.state(card.ID); got.State != protocol.CardStateDone {
		t.Errorf("card state = %s after Resume, want done", got.State)
	}
}

func TestAManualMergeStillWorksWhileMergingIsPaused(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Pause(e.ctx, e.project.ID); err != nil {
		t.Fatal(err)
	}
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})

	if result := e.merge(card); !result.Merged {
		t.Errorf("result = %+v, want a merge the owner asked for to run", result)
	}
}

func TestTurningAutoMergeOffKeepsCardsInReadyUntilItIsOnAgain(t *testing.T) {
	e := newEnv(t)
	e.wireHook()
	if _, err := e.svc.SetAutoMerge(e.ctx, e.project.ID, false); err != nil {
		t.Fatal(err)
	}
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.svc.Wait()

	if got := e.state(card.ID); got.State != protocol.CardStateReady {
		t.Fatalf("card state = %s with auto-merge off, want ready", got.State)
	}
	if got := e.integration(); got.State != protocol.IntegratorStateWaiting || !strings.Contains(got.Message, "Auto-merge is off") {
		t.Errorf("state = %s %q, want waiting with a message that auto-merge is off", got.State, got.Message)
	}

	if _, err := e.svc.SetAutoMerge(e.ctx, e.project.ID, true); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()

	if got := e.state(card.ID); got.State != protocol.CardStateDone {
		t.Errorf("card state = %s after turning auto-merge on, want done", got.State)
	}
}

func TestACardThatIsHeldBackWhileADeliveryWaitsIsMergedOnceItIsDelivered(t *testing.T) {
	e := newEnv(t)
	e.wireHook()
	lock := filepath.Join(e.repo, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	first := e.card("First", map[string]string{"b.txt": "b by the first\n"})
	e.svc.Wait()
	if got := e.state(first.ID); got.State != protocol.CardStateNeeds {
		t.Fatalf("first card state = %s, want needs while the index is locked", got.State)
	}
	second := e.card("Second", map[string]string{"c.txt": "c by the second\n"})
	e.svc.Wait()
	if got := e.state(second.ID); got.State != protocol.CardStateReady {
		t.Fatalf("second card state = %s, want it held back in ready", got.State)
	}

	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Retry(e.ctx, first.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	e.svc.Wait()

	for _, card := range []protocol.Card{first, second} {
		if got := e.state(card.ID); got.State != protocol.CardStateDone {
			t.Errorf("%s state = %s, want done", card.Key, got.State)
		}
	}
	if e.read(e.repo, "b.txt") != "b by the first\n" || e.read(e.repo, "c.txt") != "c by the second\n" {
		t.Errorf("the folder does not hold both cards' work")
	}
}

func TestRetryRunsAStoppedMergeAgain(t *testing.T) {
	tests := failing("TestAddRetry failed")
	e := newEnv(t, withTester(tests))
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	if result := e.merge(card); result.Merged {
		t.Fatalf("the first run should have stopped: %+v", result)
	}

	tests.mu.Lock()
	tests.outcome = integrator.Outcome{Passed: true}
	tests.mu.Unlock()
	returned, err := e.svc.Retry(e.ctx, card.ID)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if returned.MergePhase == protocol.MergePhaseStopped {
		t.Errorf("the card Retry answers is still stopped")
	}
	e.svc.Wait()

	if got := e.state(card.ID); got.State != protocol.CardStateDone {
		t.Errorf("card state = %s after the retry, want done", got.State)
	}
	if phase, note := e.mergeColumns(card.ID); phase != "" || note != "" {
		t.Errorf("merge columns = %q, %q after a successful retry; want them cleared", phase, note)
	}
	if tests.count() != 2 {
		t.Errorf("the tests ran %d times, want 2: a stop before the delivery runs them again", tests.count())
	}
}

func TestRetryRefusesACardNoMergeStopped(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	if _, err := e.svc.Retry(e.ctx, card.ID); !refusedWith(err, "merge_not_stalled") {
		t.Errorf("Retry of a ready card = %v, want a merge_not_stalled refusal", err)
	}
}

func TestACardThatIsNotWaitingBecauseOfAMergeCannotBeRetried(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	// The card waits on a person for another reason: an agent question.
	if _, err := e.proj.SetNeeds(e.ctx, card.ID, protocol.NeedsReason{Kind: protocol.NeedsReasonKindQuestion, Text: "Which one?"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Retry(e.ctx, card.ID); !refusedWith(err, "merge_not_stalled") {
		t.Errorf("Retry of a card with a question = %v, want a merge_not_stalled refusal", err)
	}
}

func TestRecoverPutsACardThatACrashLeftMergingBackToReady(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	if _, err := e.proj.SetState(e.ctx, card.ID, protocol.CardStateMerging); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.Recover(e.ctx); err != nil {
		t.Fatalf("Recover: %v", err)
	}

	if got := e.state(card.ID); got.State != protocol.CardStateReady {
		t.Errorf("card state = %s, want ready", got.State)
	}
}

func TestTheWorkspaceIsMadeFromTheTargetAndMadeAgainWhenItsFolderIsDeleted(t *testing.T) {
	e := newEnv(t)
	ws, err := integrator.NewWorkspace(integrator.WorkspaceDeps{Projects: e.proj, Git: e.git, DataDir: e.dataDir})
	if err != nil {
		t.Fatal(err)
	}

	dir, err := ws.Ensure(e.ctx, e.project.ID)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if dir != integrator.WorkspaceDir(e.dataDir, e.project.ID) {
		t.Errorf("workspace = %s, want %s", dir, integrator.WorkspaceDir(e.dataDir, e.project.ID))
	}
	if got := e.run(dir, "rev-parse", "HEAD"); got != e.tip("main") {
		t.Errorf("the workspace starts at %s, want the target's tip", got)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	again, err := ws.Ensure(e.ctx, e.project.ID)
	if err != nil || again != dir {
		t.Fatalf("Ensure after the folder was deleted = %s, %v", again, err)
	}
	if got := e.run(dir, "rev-parse", "--abbrev-ref", "HEAD"); got != protocol.IntegrationBranchName {
		t.Errorf("the repaired workspace is on %q", got)
	}
}

func TestNewNeedsTheCardsTheProjectsAndGit(t *testing.T) {
	if _, err := integrator.New(integrator.Deps{DataDir: t.TempDir()}); err == nil {
		t.Errorf("New with nothing = nil error")
	}
	e := newEnv(t)
	if _, err := integrator.New(integrator.Deps{Cards: e.proj, Projects: e.proj, Git: e.git, DataDir: "relative"}); err == nil {
		t.Errorf("New with a relative data folder = nil error")
	}
}

func TestAStoppedPhaseDoesNotOutliveTheCardBecomingReadyAgain(t *testing.T) {
	e := newEnv(t, withTester(failing("TestAddRetry failed")))
	e.wireHook()
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.svc.Wait()
	if phase, _ := e.mergeColumns(card.ID); phase != string(protocol.MergePhaseStopped) {
		t.Fatalf("merge phase = %q after a stop, want stopped", phase)
	}
	if _, err := e.svc.SetAutoMerge(e.ctx, e.project.ID, false); err != nil {
		t.Fatal(err)
	}

	e.ready(card.ID)
	e.svc.Wait()

	if phase, note := e.mergeColumns(card.ID); phase != "" || note != "" {
		t.Errorf("merge columns = %q, %q for a card that is ready again; want them cleared", phase, note)
	}
	item := e.integration().Queue
	if len(item) != 1 || item[0].Phase != protocol.MergePhaseQueued {
		t.Errorf("queue = %+v, want the card queued", item)
	}
}

func TestACardThatWasStoppedAndMovedAwayCannotBeRetriedForAnotherReason(t *testing.T) {
	e := newEnv(t, withTester(failing("TestAddRetry failed")))
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.assertStopped(t, card, e.merge(card))
	// The owner takes the card back to work on it. Nothing clears the stopped phase on the card.
	if _, err := e.proj.MoveCard(e.ctx, card.ID, protocol.MoveCardRequest{State: protocol.CardStateWorking}); err != nil {
		t.Fatal(err)
	}
	// Later it waits on the owner for an unrelated reason.
	if _, err := e.proj.SetNeeds(e.ctx, card.ID, protocol.NeedsReason{Kind: protocol.NeedsReasonKindQuestion, Text: "Which one?"}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.Retry(e.ctx, card.ID); !refusedWith(err, "merge_not_stalled") {
		t.Errorf("Retry = %v, want a merge_not_stalled refusal", err)
	}
	if state := e.integration(); state.State == protocol.IntegratorStateWaiting {
		t.Errorf("state = %+v, want the card's question not to be read as a stopped merge", state)
	}
}
