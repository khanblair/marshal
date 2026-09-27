package integrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

type fakeCards struct {
	card    protocol.Card
	states  []protocol.CardState
	needs   []protocol.NeedsReason
	stateFn func(protocol.CardState) error
}

func (f *fakeCards) Card(context.Context, string) (protocol.Card, error) { return f.card, nil }
func (f *fakeCards) SetState(_ context.Context, _ string, s protocol.CardState) (protocol.Card, error) {
	if f.stateFn != nil {
		if err := f.stateFn(s); err != nil {
			return protocol.Card{}, err
		}
	}
	f.states = append(f.states, s)
	f.card.State = s
	return f.card, nil
}
func (f *fakeCards) SetNeeds(_ context.Context, _ string, r protocol.NeedsReason) (protocol.Card, error) {
	f.needs = append(f.needs, r)
	f.card.State = protocol.CardStateNeeds
	return f.card, nil
}

type fakeProjects struct{ project protocol.Project }

func (f fakeProjects) Get(context.Context, string) (protocol.Project, error) { return f.project, nil }

type fakeGit struct {
	preview    gitx.MergePreview
	previewErr error
	mergeErr   error
	mergeSHA   string
	ffErr      error

	dryRunCalled  bool
	ffTarget      string
	ffCommit      string
	backupCalled  bool
	worktreeAdded bool
	abortCalled   bool
	removedWT     bool
	mergeWorktree string
}

func (f *fakeGit) DryRunMerge(context.Context, string, string, string) (gitx.MergePreview, error) {
	f.dryRunCalled = true
	return f.preview, f.previewErr
}
func (f *fakeGit) BackupBranch(context.Context, string, string, string) error {
	f.backupCalled = true
	return nil
}
func (f *fakeGit) AddMergeWorktree(_ context.Context, _, path, _ string) error {
	f.worktreeAdded = true
	f.mergeWorktree = path
	return nil
}
func (f *fakeGit) MergeInto(context.Context, string, string, string) (string, error) {
	return f.mergeSHA, f.mergeErr
}
func (f *fakeGit) FastForwardRef(_ context.Context, _, target, commit string) error {
	f.ffTarget, f.ffCommit = target, commit
	return f.ffErr
}
func (f *fakeGit) AbortMerge(context.Context, string) error { f.abortCalled = true; return nil }
func (f *fakeGit) RemoveWorktree(context.Context, string, string, string, bool) error {
	f.removedWT = true
	return nil
}

type fakeTester struct {
	outcome Outcome
	err     error
	called  bool
	changed []string
}

func (f *fakeTester) Run(_ context.Context, _ string, changed []string) (Outcome, error) {
	f.called = true
	f.changed = changed
	return f.outcome, f.err
}

func readyCard() protocol.Card {
	return protocol.Card{ID: "card-0001", ProjectID: "web", Key: "web#1", Title: "Add retry",
		State: protocol.CardStateReady, Branch: "marshal/card-1"}
}

func newService(t *testing.T, card protocol.Card, git Git, tests Tester) (*Service, *fakeCards) {
	t.Helper()
	cards := &fakeCards{card: card}
	svc, err := New(Deps{
		Cards:    cards,
		Projects: fakeProjects{project: protocol.Project{ID: "web", Path: "/code/web", DefaultBranch: "main"}},
		Git:      git,
		Tests:    tests,
		DataDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc, cards
}

func TestACleanMergeFastForwardsTheTargetAndMarksTheCardDone(t *testing.T) {
	git := &fakeGit{preview: gitx.MergePreview{Clean: true, Changed: []string{"b.txt"}}, mergeSHA: "deadbeef"}
	svc, cards := newService(t, readyCard(), git, nil)

	result, err := svc.Merge(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !result.Merged || result.Commit != "deadbeef" {
		t.Errorf("result = %+v, want merged deadbeef", result)
	}
	if git.ffTarget != "main" || git.ffCommit != "deadbeef" {
		t.Errorf("fast-forward = %s -> %s, want main -> deadbeef", git.ffTarget, git.ffCommit)
	}
	if !git.backupCalled {
		t.Errorf("a backup branch was not made")
	}
	if !git.removedWT {
		t.Errorf("the temporary worktree was not removed")
	}
	if len(cards.states) != 2 || cards.states[0] != protocol.CardStateMerging || cards.states[1] != protocol.CardStateDone {
		t.Errorf("states = %v, want [merging done]", cards.states)
	}
	if len(cards.needs) != 0 {
		t.Errorf("a merged card was sent to needs you: %v", cards.needs)
	}
}

func TestAConflictingMergeLeavesTheTargetAndSendsTheCardToNeeds(t *testing.T) {
	git := &fakeGit{preview: gitx.MergePreview{Clean: false, Conflicts: []string{"a.txt"}}}
	svc, cards := newService(t, readyCard(), git, nil)

	result, err := svc.Merge(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if result.Merged {
		t.Errorf("result = %+v, want not merged", result)
	}
	if git.ffTarget != "" {
		t.Errorf("the target was moved forward on a conflict: %s", git.ffTarget)
	}
	if len(cards.needs) != 1 || cards.needs[0].Kind != protocol.NeedsReasonKindConflict {
		t.Fatalf("needs = %v, want one conflict reason", cards.needs)
	}
	if !strings.Contains(cards.needs[0].Text, "a.txt") || !strings.Contains(cards.needs[0].Text, "not changed") {
		t.Errorf("reason = %q, want it to name the file and say the target was not changed", cards.needs[0].Text)
	}
	if git.worktreeAdded {
		t.Errorf("a worktree was made for a merge known to conflict")
	}
}

func TestAFailingMergeIntoAbortsAndKeepsTheTarget(t *testing.T) {
	git := &fakeGit{
		preview:  gitx.MergePreview{Clean: true},
		mergeErr: gitx.ErrMergeConflict,
	}
	svc, cards := newService(t, readyCard(), git, nil)

	result, err := svc.Merge(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if result.Merged || git.ffTarget != "" {
		t.Errorf("the target moved after a conflicting merge: %+v", result)
	}
	if !git.abortCalled {
		t.Errorf("the merge was not aborted")
	}
	if len(cards.needs) != 1 || cards.needs[0].Kind != protocol.NeedsReasonKindConflict {
		t.Errorf("needs = %v, want one conflict reason", cards.needs)
	}
}

func TestFailingTestsKeepTheTargetAndSendTheCardToNeeds(t *testing.T) {
	git := &fakeGit{preview: gitx.MergePreview{Clean: true, Changed: []string{"b.txt"}}, mergeSHA: "cafe"}
	tester := &fakeTester{outcome: Outcome{Passed: false, Summary: "TestAddRetry failed"}}
	svc, cards := newService(t, readyCard(), git, tester)

	result, err := svc.Merge(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if result.Merged || git.ffTarget != "" {
		t.Errorf("the target moved with failing tests: %+v", result)
	}
	if !tester.called || len(tester.changed) != 1 || tester.changed[0] != "b.txt" {
		t.Errorf("the tester was not given the changed files: %+v", tester.changed)
	}
	if len(cards.needs) != 1 || cards.needs[0].Kind != protocol.NeedsReasonKindCIFailed {
		t.Fatalf("needs = %v, want one ci-failed reason", cards.needs)
	}
	if !strings.Contains(cards.needs[0].Text, "TestAddRetry failed") {
		t.Errorf("reason = %q, want the test summary", cards.needs[0].Text)
	}
}

func TestPassingTestsMoveTheTargetForward(t *testing.T) {
	git := &fakeGit{preview: gitx.MergePreview{Clean: true}, mergeSHA: "cafe"}
	tester := &fakeTester{outcome: Outcome{Passed: true}}
	svc, cards := newService(t, readyCard(), git, tester)

	result, err := svc.Merge(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !result.Merged || git.ffCommit != "cafe" {
		t.Errorf("result = %+v, ffCommit = %s, want merged cafe", result, git.ffCommit)
	}
	if cards.card.State != protocol.CardStateDone {
		t.Errorf("card state = %s, want done", cards.card.State)
	}
}

func TestACardThatIsNotReadyIsRefused(t *testing.T) {
	card := readyCard()
	card.State = protocol.CardStateWorking
	svc, _ := newService(t, card, &fakeGit{}, nil)
	_, err := svc.Merge(context.Background(), "card-0001")
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeRefused {
		t.Fatalf("err = %v, want a refusal", err)
	}
}
