package integrator_test

import (
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// canUndo reads whether the newest delivery of the project can be undone.
func (e *env) canUndo() bool {
	e.t.Helper()
	history := e.integration().History
	if len(history) == 0 {
		e.t.Fatalf("the project has no deliveries")
	}
	return history[0].CanUndo
}

func TestUndoPutsTheBranchAndTheFolderBack(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	before := e.tip("main")
	e.merge(card)
	if !e.canUndo() {
		t.Fatalf("a delivery into a clean folder cannot be undone")
	}

	got, err := e.svc.Undo(e.ctx, card.ID)
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if e.tip("main") != before {
		t.Errorf("main = %s, want it back at %s", e.tip("main"), before)
	}
	if e.read(e.repo, "b.txt") != "b base\n" || e.status() != "" {
		t.Errorf("the folder is not back: b.txt = %q, status = %q", e.read(e.repo, "b.txt"), e.status())
	}
	if got.State != protocol.CardStateNeeds || got.NeedsReason == nil || !strings.Contains(got.NeedsReason.Text, "undid this merge") {
		t.Errorf("card = %s %+v, want needs with a sentence that the merge was undone", got.State, got.NeedsReason)
	}
	if phase, note := e.mergeColumns(card.ID); phase != string(protocol.MergePhaseStopped) || note != got.NeedsReason.Text {
		t.Errorf("merge columns = %q, %q; want the stopped phase with the sentence as the note", phase, note)
	}
	if e.canUndo() {
		t.Errorf("an undone delivery can be undone again")
	}
	if _, err := e.svc.Undo(e.ctx, card.ID); !refusedWith(err, "undo_unavailable") {
		t.Errorf("a second Undo = %v, want an undo_unavailable refusal", err)
	}
}

func TestAnUndoneCardCanBeMergedAgainWithRetry(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.merge(card)
	if _, err := e.svc.Undo(e.ctx, card.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.Retry(e.ctx, card.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	e.svc.Wait()

	if got := e.state(card.ID); got.State != protocol.CardStateDone || e.read(e.repo, "b.txt") != "b by the card\n" {
		t.Errorf("card = %s, b.txt = %q; want it delivered again", got.State, e.read(e.repo, "b.txt"))
	}
}

func TestUndoIsRefusedWhileTheFolderHasChangedFilesSinceTheMerge(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.merge(card)
	tip := e.tip("main")
	e.write(e.repo, "a.txt", "edited after the merge\n")

	if e.canUndo() {
		t.Errorf("CanUndo = true with a changed file in the folder")
	}
	if _, err := e.svc.Undo(e.ctx, card.ID); !refusedWith(err, "undo_folder_changed") {
		t.Fatalf("Undo = %v, want an undo_folder_changed refusal", err)
	}
	if e.tip("main") != tip || e.read(e.repo, "a.txt") != "edited after the merge\n" {
		t.Errorf("a refused undo changed something")
	}
}

func TestUndoIgnoresAnUntrackedFileInTheFolder(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.merge(card)
	e.write(e.repo, "notes.txt", "my notes\n")

	if !e.canUndo() {
		t.Fatalf("CanUndo = false with only an untracked file in the folder")
	}
	if _, err := e.svc.Undo(e.ctx, card.ID); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if e.read(e.repo, "notes.txt") != "my notes\n" || e.read(e.repo, "b.txt") != "b base\n" {
		t.Errorf("the untracked file or the undo is wrong")
	}
}

func TestUndoIsRefusedOnceTheBranchHasMovedOn(t *testing.T) {
	e := newEnv(t)
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	e.merge(card)
	e.ownerCommits("c.txt", "c by the owner\n")
	tip := e.tip("main")

	if e.canUndo() {
		t.Errorf("CanUndo = true after the branch moved on")
	}
	if _, err := e.svc.Undo(e.ctx, card.ID); !refusedWith(err, "undo_moved") {
		t.Fatalf("Undo = %v, want an undo_moved refusal", err)
	}
	if e.tip("main") != tip {
		t.Errorf("a refused undo moved the branch")
	}
}

func TestUndoAfterTheOwnersChangesWereMergedGivesThemTheirFolderBack(t *testing.T) {
	e := newEnv(t)
	card := e.card("Edit the end of a", map[string]string{"a.txt": withLine(multiLine, "ten", "TEN by the card")})
	mine := withLine(multiLine, "one", "ONE by the owner")
	e.write(e.repo, "a.txt", mine)
	e.write(e.repo, "notes.txt", "the owner's notes\n")
	before := e.tip("main")
	statusBefore := e.status()
	if r := e.merge(card); !r.Merged {
		t.Fatalf("merge: %+v", r)
	}
	if !e.canUndo() {
		t.Fatalf("CanUndo = false for a delivery that merged the owner's changes into an untouched folder")
	}

	if _, err := e.svc.Undo(e.ctx, card.ID); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if e.tip("main") != before {
		t.Errorf("main = %s, want %s", e.tip("main"), before)
	}
	if got := e.read(e.repo, "a.txt"); got != mine {
		t.Errorf("a.txt = %q, want exactly the owner's own file", got)
	}
	if e.read(e.repo, "notes.txt") != "the owner's notes\n" {
		t.Errorf("the owner's untracked file changed")
	}
	if e.status() != statusBefore {
		t.Errorf("status = %q, want what it was before the merge, %q", e.status(), statusBefore)
	}
}

func TestUndoIsRefusedWhenTheOwnerKeptEditingAfterTheirChangesWereMerged(t *testing.T) {
	e := newEnv(t)
	card := e.card("Edit the end of a", map[string]string{"a.txt": withLine(multiLine, "ten", "TEN by the card")})
	e.write(e.repo, "a.txt", withLine(multiLine, "one", "ONE by the owner"))
	e.merge(card)
	e.write(e.repo, "a.txt", e.read(e.repo, "a.txt")+"and more\n")

	if e.canUndo() {
		t.Errorf("CanUndo = true although the owner kept editing")
	}
	if _, err := e.svc.Undo(e.ctx, card.ID); !refusedWith(err, "undo_folder_changed") {
		t.Errorf("Undo = %v, want an undo_folder_changed refusal", err)
	}
}

func TestUndoMovesABranchNoWorktreeHasCheckedOut(t *testing.T) {
	e := newEnv(t)
	e.useBranch("development")
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})
	before := e.tip("development")
	e.write(e.repo, "a.txt", "the owner's edit\n")
	e.merge(card)
	if !e.canUndo() {
		t.Fatalf("CanUndo = false for a branch the folder does not have checked out")
	}

	if _, err := e.svc.Undo(e.ctx, card.ID); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if e.tip("development") != before {
		t.Errorf("development = %s, want %s", e.tip("development"), before)
	}
	if e.read(e.repo, "a.txt") != "the owner's edit\n" {
		t.Errorf("the owner's folder changed")
	}
}
