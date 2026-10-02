package integrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// run is one card's merge, from the moment the project's lock is held to the card's last state.
type run struct {
	s       *Service
	ctx     context.Context
	card    protocol.Card
	project protocol.Project
	target  string

	// ws is the Integrator workspace.
	ws string
	// preTip is the integrator branch before this card was merged: where a stopped merge puts it back.
	preTip string
	// tip is the merged result that waits for delivery.
	tip string
	// already is true when the card's work is in the target branch already.
	already bool
	// tested is true when tip is the commit that passed its tests before a delivery stopped.
	tested bool
	// siblings are other cards whose work waits on the integrator branch with this one.
	siblings []CardRow

	resolved  int
	summaries []string
}

// stop is the reason a merge does not go on.
type stop struct {
	kind protocol.NeedsReasonKind
	text string
	// moved means the target branch moved under the merge, so it starts over.
	moved bool
	// keep means the tested merge stays on the integrator branch, because only its delivery stopped.
	keep bool
}

func conflictStop(text string) *stop {
	return &stop{kind: protocol.NeedsReasonKindConflict, text: aboutTheMerge(text)}
}

func keepStop(text string) *stop {
	return &stop{kind: protocol.NeedsReasonKindConflict, text: aboutTheMerge(text), keep: true}
}

// aboutTheMerge makes sure the sentence a person reads says it is the merge that stopped, so a
// reason about a folder or a branch is not mistaken for something else on the card.
func aboutTheMerge(text string) string {
	if strings.Contains(strings.ToLower(text), "merge") {
		return text
	}
	return text + " The merge did not finish."
}

func movedStop() *stop { return &stop{moved: true} }

// delivered says what a delivery did, for the history.
type delivered struct {
	commit     string
	prevTip    string
	backup     string
	wipRef     string
	folderTree string
	note       string
	already    bool
}

// run starts a card's merge. The caller holds the project's lock.
func (s *Service) run(ctx context.Context, card protocol.Card, project protocol.Project) (Result, error) {
	r := &run{s: s, ctx: ctx, card: card, project: project, target: project.Target()}
	return r.execute()
}

// execute runs the merge, starting over when the target moves under it, and leaves the card Done or
// in Needs you.
func (r *run) execute() (Result, error) {
	if strings.TrimSpace(r.card.Branch) == "" {
		return r.fail(conflictStop("This card has no branch to merge."))
	}
	if strings.TrimSpace(r.target) == "" {
		return r.fail(conflictStop("This project has no integration branch to merge into."))
	}
	// The board shows the card as merging from here until it is done or sent back.
	if _, err := r.s.cards.SetState(r.ctx, r.card.ID, protocol.CardStateMerging); err != nil {
		return Result{}, err
	}
	var st *stop
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var d delivered
		d, st = r.attempt()
		if st == nil {
			return r.finish(d)
		}
		if err := r.ctx.Err(); err != nil {
			r.reset(r.preTip)
			return Result{}, err
		}
		if !st.moved {
			break
		}
		if attempt == maxAttempts {
			st = conflictStop(fmt.Sprintf("%s kept moving while this merge ran. Retry it.", r.target))
			break
		}
		r.s.log.Info("the target moved during a merge, starting it again", "card_id", r.card.ID, "target", r.target)
		r.reset(r.preTip)
	}
	return r.fail(st)
}

// attempt makes one pass: bring the integrator branch up to date, merge the card, test, deliver.
func (r *run) attempt() (delivered, *stop) {
	if st := r.prepare(); st != nil {
		return delivered{}, st
	}
	changed, st := r.integrateCard()
	if st != nil {
		return delivered{}, st
	}
	if r.already {
		return delivered{already: true}, nil
	}
	if st := r.runTests(changed); st != nil {
		return delivered{}, st
	}
	return r.deliver()
}

// prepare makes the workspace ready: clean, without work that belongs to a card that was pulled
// back, and up to date with the target.
func (r *run) prepare() *stop {
	r.progress(protocol.MergePhaseResolving, "Getting the Integrator branch ready")
	ws, err := r.s.ws.Ensure(r.ctx, r.project.ID)
	if err != nil {
		return conflictStop("The Integrator workspace could not be made: " + err.Error())
	}
	r.ws = ws
	if err := r.s.git.ResetWorktree(r.ctx, ws, WorkspaceRoot(r.s.dataDir), "HEAD"); err != nil {
		return conflictStop("The Integrator workspace could not be cleaned: " + err.Error())
	}
	if st := r.dropOrphanedWork(); st != nil {
		return st
	}
	if st := r.syncTarget(); st != nil {
		return st
	}
	tip, err := r.s.git.Rev(r.ctx, ws, "HEAD")
	if err != nil {
		return conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	r.preTip = tip
	return nil
}

// dropOrphanedWork looks at what the integrator branch holds beyond the target. It may be ahead only
// while a tested merge waits for delivery, and it is then exactly that merge: anything else, such as
// work from before the integration branch was changed, or commits a crash or a person left, is not
// delivered. The branch is pinned and put back to the target. Work that waits for delivery stays
// only if it belongs to cards that still wait for a merge, and is delivered with this card.
func (r *run) dropOrphanedWork() *stop {
	r.siblings = nil
	ahead, err := r.s.git.CountAhead(r.ctx, r.project.Path, r.target, protocol.IntegrationBranchName)
	if err != nil {
		return conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	if ahead == 0 {
		return nil
	}
	tip, err := r.s.git.Rev(r.ctx, r.ws, "HEAD")
	if err != nil {
		return conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	settings, err := r.s.ledger.Settings(r.ctx, r.project.ID)
	if err != nil {
		return conflictStop("The merge settings could not be read: " + err.Error())
	}
	if settings.PendingTip == "" || settings.PendingTip != tip {
		return r.discardIntegratorWork()
	}
	owners, err := r.workOnIntegrator()
	if err != nil {
		return conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	for _, owner := range owners {
		if !r.waitingOnMerge(owner) {
			return r.discardIntegratorWork()
		}
	}
	r.siblings = owners
	return nil
}

// waitingOnMerge says whether a card is one whose work belongs on the integrator branch: it waits
// for a merge, is in one, a merge stopped it, or its retry has started.
func (r *run) waitingOnMerge(c CardRow) bool {
	switch c.State {
	case protocol.CardStateReady, protocol.CardStateMerging:
		return true
	case protocol.CardStateNeeds:
		return c.Phase == protocol.MergePhaseStopped || r.s.isRetrying(c.ID)
	}
	return false
}

// workOnIntegrator lists the other cards whose branches are on the integrator branch and not yet in
// the target.
func (r *run) workOnIntegrator() ([]CardRow, error) {
	rows, err := r.s.ledger.Branches(r.ctx, r.project.ID)
	if err != nil {
		return nil, err
	}
	var owners []CardRow
	for _, row := range rows {
		if row.ID == r.card.ID {
			continue
		}
		onIntegrator, err := r.s.git.IsMerged(r.ctx, r.project.Path, row.Branch, protocol.IntegrationBranchName)
		if err != nil || !onIntegrator {
			continue
		}
		inTarget, err := r.s.git.IsMerged(r.ctx, r.project.Path, row.Branch, r.target)
		if err != nil || inTarget {
			continue
		}
		owners = append(owners, row)
	}
	return owners, nil
}

// discardIntegratorWork pins the integrator branch where it is and puts it back to the target. What
// waited for delivery waits no more.
func (r *run) discardIntegratorWork() *stop {
	g := r.s.git
	tip, err := g.Rev(r.ctx, r.ws, "HEAD")
	if err != nil {
		return conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	ref := fmt.Sprintf("refs/marshal/integrator/%s/%013d", r.project.ID, r.s.now().UnixMilli())
	if err := g.PinRef(r.ctx, r.project.Path, ref, tip); err != nil {
		return conflictStop("The Integrator branch could not be kept before it was reset: " + err.Error())
	}
	r.s.log.Warn("the integrator branch held work of a card that is no longer waiting; it was reset", "ref", ref)
	targetTip, err := g.Rev(r.ctx, r.project.Path, r.target)
	if err != nil {
		return conflictStop("Branch " + r.target + " could not be read: " + err.Error())
	}
	if err := g.ResetWorktree(r.ctx, r.ws, WorkspaceRoot(r.s.dataDir), targetTip); err != nil {
		return conflictStop("The Integrator branch could not be reset: " + err.Error())
	}
	if err := r.s.ledger.SetPendingTip(r.ctx, r.project.ID, ""); err != nil {
		r.s.log.Warn("could not clear the merge that waited for delivery", "project_id", r.project.ID, "error", err)
	}
	return nil
}

// syncTarget brings the integrator branch up to the target's tip: a fast-forward when it is behind,
// nothing when it is ahead, and a merge, which can conflict, when the two have drifted apart.
func (r *run) syncTarget() *stop {
	g := r.s.git
	targetTip, err := g.Rev(r.ctx, r.project.Path, r.target)
	if err != nil {
		return conflictStop("Branch " + r.target + " could not be read: " + err.Error())
	}
	tip, err := g.Rev(r.ctx, r.ws, "HEAD")
	if err != nil {
		return conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	if tip == targetTip {
		return nil
	}
	ahead, err := g.IsMerged(r.ctx, r.ws, targetTip, tip)
	if err != nil {
		return conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	if ahead {
		return nil
	}
	behind, err := g.IsMerged(r.ctx, r.ws, tip, targetTip)
	if err != nil {
		return conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	if behind {
		if err := g.FastForwardWorktree(r.ctx, r.ws, targetTip); err != nil {
			return conflictStop("The Integrator branch could not catch up with " + r.target + ": " + err.Error())
		}
		return nil
	}
	r.progress(protocol.MergePhaseResolving, "Merging "+r.target+" into the Integrator branch")
	_, st := r.integrate(targetTip, "Marshal: bring "+protocol.IntegrationBranchName+" up to date with "+r.target)
	return st
}

// integrateCard merges the card's branch into the integrator branch, or finds that it is there
// already or in the target. It answers the files the card changes, for the tests.
func (r *run) integrateCard() ([]string, *stop) {
	g := r.s.git
	inTarget, err := g.IsMerged(r.ctx, r.project.Path, r.card.Branch, r.target)
	if err != nil {
		return nil, conflictStop("The card's branch could not be read: " + err.Error())
	}
	if inTarget {
		r.already = true
		return nil, nil
	}
	r.progress(protocol.MergePhaseResolving, "Merging the card into the Integrator branch")
	onIntegrator, err := g.IsMerged(r.ctx, r.ws, r.card.Branch, protocol.IntegrationBranchName)
	if err != nil {
		return nil, conflictStop("The card's branch could not be read: " + err.Error())
	}
	var changed []string
	if onIntegrator {
		changed, err = g.ChangedSince(r.ctx, r.ws, r.target, r.card.Branch)
		if err != nil {
			return nil, conflictStop("The merge could not be read: " + err.Error())
		}
	} else {
		var st *stop
		if changed, st = r.integrate(r.card.Branch, mergeMessage(r.card)); st != nil {
			return nil, st
		}
	}
	if r.tip, err = g.Rev(r.ctx, r.ws, "HEAD"); err != nil {
		return nil, conflictStop("The Integrator branch could not be read: " + err.Error())
	}
	if settings, err := r.s.ledger.Settings(r.ctx, r.project.ID); err == nil {
		r.tested = onIntegrator && settings.PendingTip == r.tip
	}
	return changed, nil
}

// integrate merges rev into the integrator branch in the workspace and answers the files it changes.
// A clean merge is made at once. A conflict goes to the resolver, and without one, or when it is not
// confident, the merge stops.
func (r *run) integrate(rev, message string) ([]string, *stop) {
	g := r.s.git
	preview, err := g.DryRunMerge(r.ctx, r.ws, protocol.IntegrationBranchName, rev)
	if err != nil {
		return nil, conflictStop("The merge could not be tested: " + err.Error())
	}
	conflicts := preview.Conflicts
	switch {
	case preview.Clean:
		_, err := g.MergeInto(r.ctx, r.ws, rev, message)
		if err == nil {
			return preview.Changed, nil
		}
		if !errors.Is(err, gitx.ErrMergeConflict) {
			return nil, conflictStop("The merge failed: " + err.Error())
		}
		// A conflict the dry run did not see: go on with the files Git names now.
		conflicts, _ = g.UnmergedPaths(r.ctx, r.ws)
	case r.s.resolver != nil:
		found, err := g.StartMerge(r.ctx, r.ws, rev)
		if err != nil {
			return nil, conflictStop("The merge failed: " + err.Error())
		}
		if len(found) > 0 {
			conflicts = found
		}
	}
	if r.s.resolver == nil {
		return nil, conflictStop(conflictReason(r.target, conflicts))
	}
	if st := r.resolve(MergeTaskCard, conflicts); st != nil {
		return nil, st
	}
	if _, err := g.CommitMerge(r.ctx, r.ws, message); err != nil {
		return nil, conflictStop("The resolved merge could not be committed: " + err.Error())
	}
	return preview.Changed, nil
}

// runTests runs the tests a merge must pass over the merged result. They are skipped when the commit
// already passed them before its delivery stopped.
func (r *run) runTests(changed []string) *stop {
	if r.s.tests == nil || r.tested {
		return nil
	}
	r.progress(protocol.MergePhaseTesting, "Running the tests")
	outcome, err := r.s.tests.Run(r.ctx, r.ws, changed)
	if err != nil {
		return &stop{kind: protocol.NeedsReasonKindCIFailed,
			text: "The merge queue's tests could not run: " + err.Error() + " The target branch was not changed."}
	}
	if !outcome.Passed {
		summary := outcome.Summary
		if strings.TrimSpace(summary) == "" {
			summary = "the tests failed"
		}
		return &stop{kind: protocol.NeedsReasonKindCIFailed,
			text: "The merge queue's tests did not pass: " + summary + " The target branch was not changed."}
	}
	return nil
}

// fail sends the card to Needs you with the reason, and puts the integrator branch back to where it
// was, unless only the delivery stopped: then the tested merge stays, and a retry delivers it.
func (r *run) fail(st *stop) (Result, error) {
	ctx := context.WithoutCancel(r.ctx)
	if st.keep && r.tip != "" {
		if err := r.s.ledger.SetPendingTip(ctx, r.project.ID, r.tip); err != nil {
			r.s.log.Warn("could not record the merge that waits for delivery", "card_id", r.card.ID, "error", err)
		}
	} else {
		r.reset(r.preTip)
	}
	// The stopped phase is written just before the card moves, so the card announced with the move
	// already carries it, and the reason stays on the card as its merge note.
	r.s.stopProgress(ctx, r.card, st.text)
	if _, err := r.s.cards.SetNeeds(ctx, r.card.ID, protocol.NeedsReason{Kind: st.kind, Text: st.text}); err != nil {
		return Result{}, err
	}
	r.s.log.Warn("the merge queue stopped a card", "card_id", r.card.ID, "reason", st.text)
	return Result{Merged: false, Reason: st.text}, nil
}

// reset puts the integrator branch back to a commit, in the workspace. It never fails the merge: a
// workspace left as it is gets cleaned at the start of the next merge.
func (r *run) reset(tip string) {
	if r.ws == "" || tip == "" {
		return
	}
	ctx := context.WithoutCancel(r.ctx)
	if err := r.s.git.ResetWorktree(ctx, r.ws, WorkspaceRoot(r.s.dataDir), tip); err != nil {
		r.s.log.Warn("could not reset the Integrator branch", "card_id", r.card.ID, "tip", tip, "error", err)
	}
}

// progress tells the card and the clients which phase the merge is in.
func (r *run) progress(phase protocol.MergePhase, note string) {
	r.s.setProgress(r.ctx, r.card, Progress{Phase: phase, Note: note, DoingNow: note})
}

// sleep waits for the folder retry time and answers false when the merge was cancelled meanwhile.
func (r *run) sleep() bool {
	select {
	case <-r.ctx.Done():
		return false
	case <-time.After(r.s.folder.Wait):
		return true
	}
}

// setProgress writes a card's merge progress and publishes merge.progress on the project's topic
// and the card's. A failed write is logged: progress is a display, and never stops a merge.
func (s *Service) setProgress(ctx context.Context, card protocol.Card, p Progress) {
	if err := s.ledger.SetProgress(ctx, card.ID, p); err != nil {
		s.log.Warn("could not write the merge progress of a card", "card_id", card.ID, "error", err)
	}
	if s.events == nil {
		return
	}
	data := protocol.MergeProgressEvent{ProjectID: card.ProjectID, CardID: card.ID, Phase: p.Phase, Note: p.Note}
	critical := p.Phase == "" || p.Phase == protocol.MergePhaseStopped
	s.events.Publish(string(protocol.ProjectTopic(card.ProjectID)), string(protocol.EventTypeMergeProgress), data, critical)
	s.events.Publish(string(protocol.CardTopic(card.ID)), string(protocol.EventTypeMergeProgress), data, critical)
}

// markQueued shows a card as waiting for its turn.
func (s *Service) markQueued(ctx context.Context, card protocol.Card) {
	s.setProgress(ctx, card, Progress{Phase: protocol.MergePhaseQueued, Note: "Waiting for its turn", DoingNow: "Waiting to be merged"})
}

// stopProgress marks a card's merge as stopped, with the reason as its note and no "doing now" line.
func (s *Service) stopProgress(ctx context.Context, card protocol.Card, reason string) {
	s.setProgress(ctx, card, Progress{Phase: protocol.MergePhaseStopped, Note: reason})
}

// clearProgress ends a card's merge progress. The note stays on the card, so a stopped merge keeps
// its reason and a delivery keeps what it had to say.
func (s *Service) clearProgress(ctx context.Context, card protocol.Card, note string) {
	s.setProgress(ctx, card, Progress{Note: note})
}
