package integrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Undo puts the integration branch back to where it was before the card's merge, and the owner's
// folder with it. It is allowed only while the branch tip is still the commit the merge made, and
// the folder holds what the delivery left: nothing changed in the tracked files after a plain
// fast-forward, or exactly the merged tree after the owner's own changes were merged in.
//
// The branch moves back by compare-and-swap and the folder by gitx.Move, which refuses to write over
// a file that changed. A delivery that merged the owner's changes puts the folder back to exactly
// what the owner had before, from the snapshot it kept.
//
// The card is left in Needs you, with a sentence that says it was undone. It is not put back in
// Ready to merge, because that would merge it again at once, and not left in Done, because its work
// is no longer in the branch.
func (s *Service) Undo(ctx context.Context, cardID string) (protocol.Card, error) {
	row, found, err := s.ledger.LatestDelivery(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if !found || !row.UndoneAt.IsZero() || row.PrevTip == "" {
		return protocol.Card{}, undoRefused(cardID, "This merge cannot be undone.", "undo_unavailable")
	}
	project, err := s.projects.Get(ctx, row.ProjectID)
	if err != nil {
		return protocol.Card{}, err
	}
	unlock := s.locks.Lock(project.ID)
	defer unlock()
	if err := s.undoDelivery(ctx, project, row); err != nil {
		return protocol.Card{}, err
	}
	if changed, err := s.ledger.MarkUndone(ctx, row.ID, s.now()); err != nil || !changed {
		s.log.Warn("could not mark a delivery as undone", "delivery_id", row.ID, "error", err)
	}
	text := fmt.Sprintf("You undid this merge, so %s is back where it was. Send the card to ready again when you want it delivered.", row.Target)
	card := protocol.Card{ID: cardID, ProjectID: project.ID}
	s.stopProgress(ctx, card, text)
	return s.cards.SetNeeds(ctx, cardID, protocol.NeedsReason{Kind: protocol.NeedsReasonKindConflict, Text: text})
}

func undoRefused(cardID, text, reason string) *protocol.Error {
	return protocol.Refused(text).With("cardId", cardID).With("reason", reason)
}

// undoDelivery moves the branch and the folder back.
func (s *Service) undoDelivery(ctx context.Context, project protocol.Project, row Delivery) error {
	g := s.git
	tip, err := g.Rev(ctx, project.Path, row.Target)
	if err != nil {
		return err
	}
	if tip != row.Commit {
		return undoRefused(row.CardID, fmt.Sprintf("%s has moved on since this merge, so it cannot be undone.", row.Target), "undo_moved")
	}
	where, found, err := g.CheckedOutAt(ctx, project.Path, row.Target)
	if err != nil {
		return err
	}
	switch {
	case !found:
		return s.swapBack(ctx, project, row)
	case !gitx.SameFolder(where.Path, project.Path):
		return undoRefused(row.CardID, fmt.Sprintf("Branch %s is open in another worktree (%s). Close it, then undo.", row.Target, where.Path), "undo_open_elsewhere")
	}
	move, err := s.undoMove(ctx, project, row)
	if err != nil {
		return err
	}
	err = g.Move(ctx, move)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gitx.ErrFolderChanged), errors.Is(err, gitx.ErrFolderBusy), errors.Is(err, gitx.ErrBranchMoved):
		return undoRefused(row.CardID, "Your folder changed since this merge, so it was left alone. Nothing was undone.", "undo_folder_changed")
	}
	return err
}

// swapBack moves a branch that no worktree has checked out.
func (s *Service) swapBack(ctx context.Context, project protocol.Project, row Delivery) error {
	err := s.git.SwapBranch(ctx, project.Path, row.Target, row.Commit, row.PrevTip)
	if errors.Is(err, gitx.ErrBranchMoved) {
		return undoRefused(row.CardID, fmt.Sprintf("%s has moved on since this merge, so it cannot be undone.", row.Target), "undo_moved")
	}
	return err
}

// undoMove says how the folder goes back. A delivery that merged the owner's changes was left with
// the merged tree and goes back to the snapshot; any other delivery goes back to the old tip's tree,
// and only the tracked files must be as the delivery left them.
func (s *Service) undoMove(ctx context.Context, project protocol.Project, row Delivery) (gitx.MoveFolder, error) {
	g := s.git
	move := gitx.MoveFolder{Dir: project.Path, Branch: row.Target, From: row.Commit, To: row.PrevTip}
	if row.FolderTree != "" {
		wip, err := g.TreeOf(ctx, project.Path, row.WIPRef)
		if err != nil {
			return move, undoRefused(row.CardID, "The snapshot of your folder from this merge is gone, so it cannot be undone.", "undo_snapshot_gone")
		}
		move.Before, move.After = row.FolderTree, wip
		return move, nil
	}
	var err error
	if move.Before, err = g.TreeOf(ctx, project.Path, row.Commit); err != nil {
		return move, err
	}
	if move.After, err = g.TreeOf(ctx, project.Path, row.PrevTip); err != nil {
		return move, err
	}
	move.Tracked = true
	return move, nil
}

// canUndo says whether a delivery can be undone now, without changing anything.
func (s *Service) canUndo(ctx context.Context, project protocol.Project, row Delivery) bool {
	if !row.UndoneAt.IsZero() || row.PrevTip == "" {
		return false
	}
	g := s.git
	tip, err := g.Rev(ctx, project.Path, row.Target)
	if err != nil || tip != row.Commit {
		return false
	}
	where, found, err := g.CheckedOutAt(ctx, project.Path, row.Target)
	if err != nil {
		return false
	}
	if !found {
		return true
	}
	if !gitx.SameFolder(where.Path, project.Path) {
		return false
	}
	busy, err := g.FolderBusy(ctx, project.Path)
	if err != nil || !busy.None() {
		return false
	}
	move, err := s.undoMove(ctx, project, row)
	if err != nil {
		return false
	}
	ok, err := g.FolderMatches(ctx, project.Path, move.Before, move.Tracked)
	return err == nil && ok
}
