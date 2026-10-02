package integrator

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// resolve asks the resolver to settle the conflicts waiting in the workspace, then checks the work
// itself: nothing may be left unmerged, and no file may keep a conflict marker. A resolver that is
// not sure, or is wrong, does not get its work delivered.
//
// The stop it answers keeps the integrator branch only for the owner's changes (MergeTaskWIP),
// where the tested merge is already on it; a card's own merge is thrown away with the branch.
func (r *run) resolve(kind MergeTaskKind, conflicts []string) *stop {
	r.progress(protocol.MergePhaseResolving, resolvingNote(kind, len(conflicts)))
	id, err := r.s.newID()
	if err != nil {
		return r.unresolved(kind, conflicts, "", "a task could not be made: "+err.Error())
	}
	task := MergeTask{
		ID: id, ProjectID: r.project.ID, Kind: kind, Worktree: r.ws, Target: r.target,
		Conflicts: conflicts, Cards: r.cardContexts(),
	}
	ctx, cancel := context.WithTimeout(r.ctx, r.s.timeout)
	defer cancel()
	verdict, err := r.s.resolver.Resolve(ctx, task)
	if err != nil {
		return r.unresolved(kind, conflicts, "", "the Integrator could not finish: "+err.Error())
	}
	if st := r.checkVerdict(kind, conflicts, verdict); st != nil {
		return st
	}
	r.resolved += len(conflicts)
	if summary := strings.TrimSpace(verdict.Summary); summary != "" {
		r.summaries = append(r.summaries, summary)
	}
	return nil
}

// resolvingNote is the sentence shown while the resolver works.
func resolvingNote(kind MergeTaskKind, n int) string {
	what := "conflicts"
	if n == 1 {
		what = "conflict"
	}
	if kind == MergeTaskWIP {
		return fmt.Sprintf("Merging your uncommitted changes: resolving %d %s", n, what)
	}
	return fmt.Sprintf("Resolving %d %s", n, what)
}

// checkVerdict decides whether the resolver's answer can be used.
func (r *run) checkVerdict(kind MergeTaskKind, conflicts []string, v Verdict) *stop {
	switch {
	case !v.Resolved:
		return r.unresolved(kind, conflicts, detail(v), "the Integrator could not resolve it")
	case !v.Confident:
		return r.unresolved(kind, conflicts, detail(v), "the Integrator resolved it but is not sure of the result")
	}
	g := r.s.git
	paths := slices.Clone(conflicts)
	for _, file := range v.Files {
		if !slices.Contains(paths, file) {
			paths = append(paths, file)
		}
	}
	if err := g.StageResolved(r.ctx, r.ws, paths); err != nil {
		return r.unresolved(kind, conflicts, "", "the resolved files could not be staged: "+err.Error())
	}
	left, err := gitx.ConflictMarkers(r.ws, paths)
	if err != nil {
		return r.unresolved(kind, conflicts, "", "the resolved files could not be read: "+err.Error())
	}
	if len(left) > 0 {
		return r.unresolved(kind, left, detail(v), "the Integrator said it resolved it, but conflict markers are still in "+strings.Join(left, ", "))
	}
	unmerged, err := g.UnmergedPaths(r.ctx, r.ws)
	if err != nil {
		return r.unresolved(kind, conflicts, "", "the merge could not be read: "+err.Error())
	}
	if len(unmerged) > 0 {
		return r.unresolved(kind, unmerged, detail(v), "the Integrator said it resolved it, but "+strings.Join(unmerged, ", ")+" is still in conflict")
	}
	return nil
}

// detail is what the resolver said, in one string, for the sentence a person reads.
func detail(v Verdict) string {
	var parts []string
	if s := strings.TrimSpace(v.Summary); s != "" {
		parts = append(parts, s)
	}
	for _, q := range v.Questions {
		if q = strings.TrimSpace(q); q != "" {
			parts = append(parts, "Question: "+q)
		}
	}
	return strings.Join(parts, " ")
}

// unresolved is the stop for a conflict that was not settled. The sentence says why, in the words of
// whose work it was: a card's, or the owner's own uncommitted changes.
func (r *run) unresolved(kind MergeTaskKind, files []string, said, why string) *stop {
	list := "a file"
	if len(files) > 0 {
		list = strings.Join(files, ", ")
	}
	var text string
	if kind == MergeTaskWIP {
		text = fmt.Sprintf("Your uncommitted changes clash with the merge in %s (%s). Your folder was not changed. "+
			"Commit or stash your changes, then retry.", list, why)
	} else {
		text = fmt.Sprintf("The merge into %s conflicted in %s, and %s. The target branch was not changed. "+
			"Resolve it on the card's branch, then send the card to ready again.", r.target, list, why)
	}
	if said != "" {
		text += " " + said
	}
	return &stop{kind: protocol.NeedsReasonKindConflict, text: text, keep: kind == MergeTaskWIP}
}

// clashReason is the sentence for the owner's changes clashing when nothing resolves them.
func clashReason(files []string) string {
	return fmt.Sprintf("Your uncommitted changes clash with the merge in %s. Your folder was not changed. "+
		"Commit or stash your changes, then retry.", strings.Join(files, ", "))
}

// cardContexts is what the resolver is told about the cards involved: the card being merged first,
// then every card whose work already waits on the integrator branch.
func (r *run) cardContexts() []CardContext {
	out := []CardContext{r.contextOf(r.card)}
	for _, sibling := range r.siblings {
		card, err := r.s.cards.Card(r.ctx, sibling.ID)
		if err != nil {
			r.s.log.Warn("could not read a card for the resolver", "card_id", sibling.ID, "error", err)
			continue
		}
		out = append(out, r.contextOf(card))
	}
	return out
}

// contextOf describes one card for the resolver.
func (r *run) contextOf(card protocol.Card) CardContext {
	c := CardContext{CardID: card.ID, Key: card.Key, Title: card.Title, Body: card.Body, Branch: card.Branch}
	if r.s.briefs != nil {
		if brief, err := r.s.briefs.Brief(r.ctx, card.ID); err == nil {
			c.Plan, c.Handoff = brief.Plan, brief.Handoff
		} else {
			r.s.log.Warn("could not read a card's plan for the resolver", "card_id", card.ID, "error", err)
		}
	}
	if changed, err := r.s.git.ChangedSince(r.ctx, r.ws, r.target, card.Branch); err == nil {
		c.Changed = changed
	}
	return c
}
