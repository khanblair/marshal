package integrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// finish ends a merge that delivered: the history gets its row, the integrator branch follows the
// target, the card moves to Done, and any card whose work was delivered with it is closed too.
func (r *run) finish(d delivered) (Result, error) {
	ctx := context.WithoutCancel(r.ctx)
	r.followTarget(ctx)
	r.record(ctx, d)
	held := false
	if settings, err := r.s.ledger.Settings(ctx, r.project.ID); err == nil {
		held = settings.PendingTip != ""
	}
	if err := r.s.ledger.SetPendingTip(ctx, r.project.ID, ""); err != nil {
		r.s.log.Warn("could not clear the merge that waited for delivery", "project_id", r.project.ID, "error", err)
	}
	r.closeSiblings(ctx, d)
	note := d.note
	commit := d.commit
	if d.already {
		note = "Already in " + r.target + "."
		if tip, err := r.s.git.Rev(ctx, r.project.Path, r.target); err == nil {
			commit = tip
		}
	}
	// The progress is cleared before the card moves, so the card announced with the move is current.
	r.s.clearProgress(ctx, r.card, note)
	result := Result{Merged: true, Commit: commit, Note: note}
	if _, err := r.s.cards.SetState(ctx, r.card.ID, protocol.CardStateDone); err != nil {
		return result, err
	}
	r.s.log.Info("merged a card", "project_id", r.project.ID, "card_id", r.card.ID, "branch", r.card.Branch, "commit", commit)
	if held {
		// Cards were held back while a delivery waited for the owner; it is delivered now.
		r.s.kick(ctx, r.project.ID)
	}
	return result, nil
}

// followTarget moves the integrator branch up to the target's tip, which is where a delivery has just
// put it. It is forward only, and a failure is only logged: the next merge catches the branch up.
func (r *run) followTarget(ctx context.Context) {
	g := r.s.git
	tip, err := g.Rev(ctx, r.project.Path, r.target)
	if err != nil || r.ws == "" {
		return
	}
	if err := g.FastForwardWorktree(ctx, r.ws, tip); err != nil {
		r.s.log.Debug("the Integrator branch did not follow the target", "target", r.target, "error", err)
	}
}

// record writes the delivery to the history. A failure is logged and does not undo the delivery.
func (r *run) record(ctx context.Context, d delivered) {
	if d.already {
		return
	}
	id, err := r.s.newID()
	if err != nil {
		r.s.log.Error("could not make an id for the merge history", "card_id", r.card.ID, "error", err)
		return
	}
	err = r.s.ledger.AddDelivery(ctx, Delivery{
		ID: id, ProjectID: r.project.ID, CardID: r.card.ID, Target: r.target, MergedAt: r.s.now(),
		Commit: d.commit, PrevTip: d.prevTip, Resolved: r.resolved, Summary: r.summary(d),
		BackupBranch: d.backup, WIPRef: d.wipRef, FolderTree: d.folderTree,
	})
	if err != nil {
		r.s.log.Error("could not write the merge history", "card_id", r.card.ID, "error", err)
	}
}

// summary is the paragraph the history shows: what the resolver said, then what the delivery did.
func (r *run) summary(d delivered) string {
	parts := append([]string{}, r.summaries...)
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("Merged %s into %s.", r.card.Key, r.target))
	}
	if d.note != "" {
		parts = append(parts, d.note)
	}
	return strings.Join(parts, " ")
}

// closeSiblings closes the cards that a merge stopped and that were delivered together with this
// one, because their work sat on the integrator branch and is now in the target. Their history row
// has no tip to go back to, so only the delivery that carried them can be undone.
func (r *run) closeSiblings(ctx context.Context, d delivered) {
	for _, sibling := range r.siblings {
		if sibling.State != protocol.CardStateNeeds || sibling.Phase != protocol.MergePhaseStopped {
			continue
		}
		inTarget, err := r.s.git.IsMerged(ctx, r.project.Path, sibling.Branch, r.target)
		if err != nil || !inTarget {
			continue
		}
		if id, err := r.s.newID(); err == nil {
			err = r.s.ledger.AddDelivery(ctx, Delivery{
				ID: id, ProjectID: r.project.ID, CardID: sibling.ID, Target: r.target, MergedAt: r.s.now(),
				Commit: d.commit, Summary: "Delivered together with " + r.card.Key + ".",
			})
			if err != nil {
				r.s.log.Error("could not write the merge history", "card_id", sibling.ID, "error", err)
			}
		}
		card := protocol.Card{ID: sibling.ID, ProjectID: r.project.ID}
		r.s.clearProgress(ctx, card, "")
		if _, err := r.s.cards.SetState(ctx, sibling.ID, protocol.CardStateDone); err != nil {
			r.s.log.Error("could not close a card that was delivered with another", "card_id", sibling.ID, "error", err)
		}
	}
}
