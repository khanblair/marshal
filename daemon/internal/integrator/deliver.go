package integrator

import (
	"errors"
	"fmt"
	"slices"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// deliver moves the integration branch to the tested merge, and the owner's folder with it when the
// folder has that branch checked out. Where the branch is checked out decides how:
//
//   - nowhere: the branch moves by compare-and-swap, and the folder does not change;
//   - in another worktree: nothing moves, and the card goes to Needs you;
//   - in the owner's folder: Git's own fast-forward, or, when that would overwrite the owner's
//     uncommitted changes, a merge of those changes into the result (wip.go).
func (r *run) deliver() (delivered, *stop) {
	r.progress(protocol.MergePhaseLanding, "Delivering to "+r.target)
	g := r.s.git
	prev, err := g.Rev(r.ctx, r.project.Path, r.target)
	if err != nil {
		return delivered{}, keepStop("Branch " + r.target + " could not be read: " + err.Error())
	}
	descends, err := g.IsMerged(r.ctx, r.project.Path, prev, r.tip)
	if err != nil {
		return delivered{}, keepStop("The merge could not be checked against " + r.target + ": " + err.Error())
	}
	if !descends {
		return delivered{}, movedStop()
	}
	d := delivered{commit: r.tip, prevTip: prev, backup: r.makeBackup()}
	where, found, err := g.CheckedOutAt(r.ctx, r.project.Path, r.target)
	if err != nil {
		return d, keepStop("Where " + r.target + " is checked out could not be read: " + err.Error())
	}
	switch {
	case !found:
		return d, r.deliverRef(&d)
	case !gitx.SameFolder(where.Path, r.project.Path):
		return d, keepStop(fmt.Sprintf("Branch %s is open in another worktree (%s). Close it or choose another integration branch.",
			r.target, where.Path))
	}
	return d, r.deliverFolder(&d)
}

// makeBackup keeps the target's tip on a branch of its own before it moves. The backup is a safety
// net and not a requirement: one that cannot be made, for example because a retry already made it,
// does not stop the delivery. It answers the branch's name, or "" when none was made.
func (r *run) makeBackup() string {
	name := "marshal/backup/" + r.target + "-" + shortID(r.card.ID)
	err := r.s.git.BackupBranch(r.ctx, r.project.Path, name, r.target)
	switch {
	case err == nil:
		return name
	case errors.Is(err, gitx.ErrBranchExists):
		return ""
	}
	r.s.log.Warn("could not make a merge backup branch", "card_id", r.card.ID, "branch", name, "error", err)
	return ""
}

// deliverRef moves a branch that no worktree has checked out. The owner's folder is on some other
// branch, so its files do not change, and the card says so.
func (r *run) deliverRef(d *delivered) *stop {
	err := r.s.git.FastForwardRef(r.ctx, r.project.Path, r.target, r.tip)
	switch {
	case err == nil:
		d.note = r.folderNote()
		return nil
	case errors.Is(err, gitx.ErrNotFastForward):
		return movedStop()
	}
	return keepStop("The target branch could not be moved forward: " + err.Error())
}

// folderNote is the sentence for a delivery that left the owner's folder alone because it is on
// another branch. A folder that cannot be read has nothing to say.
func (r *run) folderNote() string {
	head, err := r.s.git.ReadFolderHead(r.ctx, r.project.Path)
	if err != nil {
		return ""
	}
	if head.Branch == "" {
		return "Your folder is on a detached commit, so its files did not change."
	}
	return fmt.Sprintf("Your folder is on %s, so its files did not change.", head.Branch)
}

// deliverFolder updates the owner's folder with Git's own fast-forward, which changes files only
// when it loses nothing. When Git refuses because changes would be overwritten, the snapshot path
// merges them in instead.
func (r *run) deliverFolder(d *delivered) *stop {
	for try := 1; ; try++ {
		if st := r.waitForFolder(); st != nil {
			return st
		}
		if r.deletedFileClash(d.prevTip) {
			return r.deliverWithSnapshot(d)
		}
		err := r.s.git.FastForwardFolder(r.ctx, r.project.Path, r.tip)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, gitx.ErrNotFastForward):
			return movedStop()
		case errors.Is(err, gitx.ErrLocalChanges):
			return r.deliverWithSnapshot(d)
		}
		// Another program may have taken the index lock since the folder was looked at.
		busy, busyErr := r.s.git.FolderBusy(r.ctx, r.project.Path)
		if busyErr != nil || !busy.Locked || try >= r.s.folder.Attempts || !r.sleep() {
			return keepStop("Your folder could not be updated: " + err.Error())
		}
	}
}

// deletedFileClash says whether the owner deleted a file, without committing the deletion, that the
// delivery changes. Git's fast-forward would bring the file back without a word, so a clash like
// that goes the snapshot way, where it is a conflict like any other.
func (r *run) deletedFileClash(prevTip string) bool {
	deleted, err := r.s.git.DeletedFiles(r.ctx, r.project.Path)
	if err != nil || len(deleted) == 0 {
		return false
	}
	changed, err := r.s.git.ChangedSince(r.ctx, r.project.Path, prevTip, r.tip)
	if err != nil {
		return false
	}
	for _, file := range deleted {
		if slices.Contains(changed, file) {
			return true
		}
	}
	return false
}

// waitForFolder waits, for a bounded time, until nothing holds the folder's index. A half-done merge,
// rebase, cherry-pick, or revert is not waited for: it needs the owner.
func (r *run) waitForFolder() *stop {
	for try := 1; ; try++ {
		busy, err := r.s.git.FolderBusy(r.ctx, r.project.Path)
		switch {
		case err != nil:
			return keepStop("Your folder could not be read: " + err.Error())
		case busy.Operation == "conflict":
			return keepStop("Your folder has files in conflict. Resolve them, then retry.")
		case busy.Operation != "":
			return keepStop(fmt.Sprintf("Your folder is in the middle of a %s. Finish or abort it, then retry.", busy.Operation))
		case !busy.Locked:
			return nil
		case try >= r.s.folder.Attempts:
			return keepStop("Git is busy in your folder: another program holds its index lock. Retry in a moment.")
		}
		if !r.sleep() {
			return keepStop("The delivery was cancelled while it waited for your folder.")
		}
	}
}
