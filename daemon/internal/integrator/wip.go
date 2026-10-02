package integrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// keepWIPRefs is how many snapshots of a project's folder are kept for undo and for safety.
const keepWIPRefs = 20

// deliverWithSnapshot delivers into a folder that has uncommitted changes Git's fast-forward would
// overwrite. Safety comes first, in this order:
//
//  1. a snapshot commit of everything in the folder is pinned under refs/marshal/wip/, built in a
//     temporary index so the folder and its index are not touched;
//  2. the snapshot is merged onto the tested result in the Integrator workspace. A conflict goes to
//     the resolver; without one the folder is left exactly as it was;
//  3. gitx.Move reads the folder again, and only if it is still the snapshot does it move the branch
//     and write the files, refusing to overwrite any file that changed. If the owner's editor wrote
//     meanwhile, the whole thing is tried again, a bounded number of times.
//
// The result: the branch holds the card's work committed, and the folder shows that work plus the
// owner's own edits, which are uncommitted again.
func (r *run) deliverWithSnapshot(d *delivered) *stop {
	for try := 1; try <= r.s.folder.Attempts; try++ {
		if try > 1 && !r.sleep() {
			return keepStop("The delivery was cancelled while it waited for your folder.")
		}
		st, again := r.snapshotOnce(d)
		if !again {
			return st
		}
	}
	return keepStop("Your folder kept changing while Marshal merged your uncommitted changes. Retry when it is quiet.")
}

// snapshotOnce makes one try. It answers true when the folder changed under it and another try may
// work.
func (r *run) snapshotOnce(d *delivered) (*stop, bool) {
	g := r.s.git
	ref := fmt.Sprintf("refs/marshal/wip/%s/%013d", r.project.ID, r.s.now().UnixMilli())
	snap, err := g.SnapshotFolder(r.ctx, r.project.Path, ref, "Marshal: your uncommitted changes before merging "+r.card.Key)
	if err != nil {
		return keepStop("Your folder could not be read: " + err.Error()), false
	}
	if snap.Head != d.prevTip {
		r.forgetRef(ref)
		return movedStop(), false
	}
	tree, st := r.reconcile(snap)
	if st != nil {
		r.forgetRef(ref)
		return st, false
	}
	err = g.Move(r.ctx, gitx.MoveFolder{
		Dir: r.project.Path, Branch: r.target, From: snap.Head, To: r.tip, Before: snap.Tree, After: tree,
	})
	switch {
	case err == nil:
		d.wipRef, d.folderTree = ref, tree
		d.note = "Your uncommitted changes were merged with this work."
		r.pruneWIPRefs()
		return nil, false
	case errors.Is(err, gitx.ErrFolderChanged), errors.Is(err, gitx.ErrFolderBusy):
		r.forgetRef(ref)
		return nil, true
	case errors.Is(err, gitx.ErrBranchMoved):
		r.forgetRef(ref)
		return movedStop(), false
	}
	r.forgetRef(ref)
	return keepStop("Your folder could not be updated: " + err.Error()), false
}

// reconcile merges the owner's snapshot onto the tested result in the workspace and answers the tree
// the folder should end up as. Whatever happens, the snapshot never stays on the integrator branch:
// the workspace is put back to the tested result before this returns.
func (r *run) reconcile(snap gitx.Snapshot) (string, *stop) {
	g := r.s.git
	root := WorkspaceRoot(r.s.dataDir)
	defer func() {
		if err := g.ResetWorktree(context.WithoutCancel(r.ctx), r.ws, root, r.tip); err != nil {
			r.s.log.Warn("could not put the Integrator workspace back after merging a snapshot", "error", err)
		}
	}()
	if err := g.ResetWorktree(r.ctx, r.ws, root, r.tip); err != nil {
		return "", keepStop("The Integrator workspace could not be cleaned: " + err.Error())
	}
	conflicts, err := g.StartMerge(r.ctx, r.ws, snap.Commit)
	if err != nil {
		return "", keepStop("Your uncommitted changes could not be merged with this work: " + err.Error())
	}
	if len(conflicts) > 0 {
		if r.s.resolver == nil {
			return "", keepStop(clashReason(conflicts))
		}
		if st := r.resolve(MergeTaskWIP, conflicts); st != nil {
			return "", st
		}
		r.progress(protocol.MergePhaseLanding, "Delivering to "+r.target)
	}
	tree, err := g.WriteTree(r.ctx, r.ws)
	if err != nil {
		return "", keepStop("Your uncommitted changes could not be merged with this work: " + err.Error())
	}
	return tree, nil
}

// forgetRef drops a snapshot that was not used. Nothing was written to the folder, so the snapshot
// has nothing to keep safe.
func (r *run) forgetRef(ref string) {
	if err := r.s.git.ForgetRef(context.WithoutCancel(r.ctx), r.project.Path, ref); err != nil {
		r.s.log.Warn("could not drop an unused snapshot", "ref", ref, "error", err)
	}
}

// pruneWIPRefs keeps the newest snapshots of the project and drops the rest.
func (r *run) pruneWIPRefs() {
	prefix := fmt.Sprintf("refs/marshal/wip/%s/", r.project.ID)
	if err := r.s.git.PruneRefs(context.WithoutCancel(r.ctx), r.project.Path, prefix, keepWIPRefs); err != nil {
		r.s.log.Warn("could not prune old snapshots", "error", err)
	}
}
