package integrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// CompletionWorkspace holds the project folder while an idle card is reviewed and checked.
// It is reentrant with that card's main session, but excludes another card or project chat.
type CompletionWorkspace interface {
	AcquireCompletion(ctx context.Context, cardID string) (release func(), err error)
	StopCardSession(ctx context.Context, cardID string) error
}

const directFinished = "Changes are in the project folder. Configured completion checks are finished."

// directCard validates a task without treating the shared branch as that task's private work.
func (s *Service) directCard(ctx context.Context, id string) (protocol.Card, protocol.Project, error) {
	card, err := s.cards.Card(ctx, id)
	if err != nil {
		return card, protocol.Project{}, err
	}
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return card, project, err
	}
	if card.Worktree == "" || !gitx.SameFolder(card.Worktree, project.Path) {
		return card, project, refusedSend(id, "finish_legacy_workspace",
			"This card has no direct project workspace. Work in an older worktree has been preserved; recover it into the project folder and start a new card.")
	}
	branch, err := s.git.Run(ctx, project.Path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != card.Branch {
		return card, project, refusedSend(id, "finish_branch_changed",
			"The project branch changed since this card started. Return to its branch before finishing, or start a new card for the current branch.")
	}
	switch {
	case card.State == protocol.CardStateDone || card.State == protocol.CardStateMerging:
		return card, project, refusedSend(id, "finish_not_open", "This card is already finished or being processed.")
	case card.State == protocol.CardStateBacklog || card.State == protocol.CardStatePlanning:
		return card, project, refusedSend(id, "finish_not_started", "Start this card before finishing it.")
	case card.Session != nil && *card.Session == protocol.SessionStateWorking:
		return card, project, refusedSend(id, "finish_agent_working", "The main agent is still working. Wait for its turn to finish before checking the card.")
	}
	if s.lists != nil {
		open, err := s.lists.OpenRequiredItems(ctx, id)
		if err != nil {
			return card, project, err
		}
		if open > 0 {
			return card, project, refusedSend(id, "finish_checklist_open", "A required checklist still has open items. Finish them before completing this card.")
		}
	}
	if _, err := s.git.Rev(ctx, project.Path, gitx.DirectBaseRef(id)); err != nil {
		return card, project, refusedSend(id, "finish_baseline_missing", "This card's starting snapshot is missing. Its files were left unchanged; start a new card to track the work.")
	}
	return card, project, nil
}

func (s *Service) acquireCompletion(ctx context.Context, id string) (func(), error) {
	if workspace, ok := s.worker.(CompletionWorkspace); ok {
		return workspace.AcquireCompletion(ctx, id)
	}
	// A service without a session manager is useful in isolated tests. Production always supplies it.
	if s.worker != nil {
		return nil, protocol.Unavailable("Marshal cannot reserve the project folder to finish this card.")
	}
	return func() {}, nil
}

// sendDirect reserves the project before starting its asynchronous review. Unlike the retired
// merge route, it stages nothing and never changes the checked-out branch or project files.
func (s *Service) sendDirect(ctx context.Context, card protocol.Card) (protocol.Card, error) {
	if s.closing.Load() {
		return card, protocol.Unavailable("Marshal is shutting down.")
	}
	if !s.startShip(card.ID) {
		return card, nil
	}
	release, err := s.acquireCompletion(ctx, card.ID)
	if err != nil {
		s.endShip(card.ID)
		return card, err
	}
	fresh, project, err := s.directCard(ctx, card.ID)
	if err != nil {
		release()
		s.endShip(card.ID)
		return card, err
	}
	moved, err := s.cards.SetState(ctx, card.ID, protocol.CardStateReview)
	if err != nil {
		release()
		s.endShip(card.ID)
		return card, err
	}
	s.spawn(func(ctx context.Context) {
		defer s.endShip(card.ID)
		defer release()
		if _, err := s.finishDirect(ctx, fresh, project, release); err != nil {
			s.stopShip(ctx, fresh, "The card could not be completed: "+err.Error())
		}
	})
	return moved, nil
}

// completeDirect covers explicit Merge and old queue calls without entering the worktree merger.
func (s *Service) completeDirect(ctx context.Context, card protocol.Card) (Result, error) {
	if s.closing.Load() {
		return Result{}, protocol.Unavailable("Marshal is shutting down.")
	}
	if !s.startShip(card.ID) {
		return Result{}, refusedSend(card.ID, "finish_in_progress", "This card's review and checks are already running.")
	}
	defer s.endShip(card.ID)
	release, err := s.acquireCompletion(ctx, card.ID)
	if err != nil {
		return Result{}, err
	}
	defer release()
	fresh, project, err := s.directCard(ctx, card.ID)
	if err != nil {
		return Result{}, err
	}
	result, err := s.finishDirect(ctx, fresh, project, release)
	if err != nil {
		s.stopShip(ctx, fresh, "The card could not be completed: "+err.Error())
	}
	return result, err
}

func (s *Service) finishDirect(ctx context.Context, card protocol.Card, project protocol.Project, release func()) (Result, error) {
	settings, err := s.ledger.Settings(ctx, project.ID)
	if err != nil {
		return Result{}, err
	}
	base, err := s.git.Rev(ctx, project.Path, gitx.DirectBaseRef(card.ID))
	if err != nil {
		return Result{}, err
	}
	checkingRef := gitx.DirectResultRef(card.ID) + "-checking"
	snapshot, err := s.git.SnapshotFolder(ctx, project.Path, checkingRef, "Marshal: review "+card.Title)
	if err != nil {
		return Result{}, err
	}
	if previous, err := s.git.TreeOf(ctx, project.Path, gitx.DirectResultRef(card.ID)); err == nil && previous != snapshot.Tree {
		return Result{}, errors.New("the project files changed since this card's last turn. Ask its main agent to review the current folder and finish again")
	}
	if settings.Review && s.reviewer != nil {
		approved, err := s.reviewDirectSnapshot(ctx, card, project, base, snapshot, release)
		if err != nil || !approved {
			return Result{Reason: "The review needs attention."}, err
		}
	}
	feedback := func(ctx context.Context, card protocol.Card, instructions, reason string) {
		// Validation has ended. The main agent must be able to accept the correction message.
		release()
		s.sendBack(ctx, card, instructions, reason)
	}
	if settings.Checks && s.checker != nil && !s.checkWithFeedback(ctx, card, feedback) {
		return Result{Reason: "The checks need attention."}, nil
	}
	// Check commands and the person's editor can change files. Never approve a different tree
	// from the one the reviewer saw, even though the snapshot remains recoverable in Git.
	checked, err := s.git.SnapshotFolder(ctx, project.Path, checkingRef, "Marshal: checked "+card.Title)
	if err != nil {
		return Result{}, err
	}
	if checked.Tree != snapshot.Tree {
		return Result{}, errors.New("the project files changed during review or checks. Review those changes and finish the card again")
	}
	if workspace, ok := s.worker.(CompletionWorkspace); ok {
		if err := workspace.StopCardSession(ctx, card.ID); err != nil {
			return Result{}, fmt.Errorf("stop the main agent before completing the card: %w", err)
		}
	}
	stopped, err := s.git.SnapshotFolder(ctx, project.Path, checkingRef, "Marshal: finished "+card.Title)
	if err != nil {
		return Result{}, err
	}
	if stopped.Tree != snapshot.Tree {
		return Result{}, errors.New("the project files changed while the main agent was stopping. Review the latest changes before finishing the card")
	}
	if _, _, err := s.directCard(ctx, card.ID); err != nil {
		return Result{}, err
	}
	if err := s.git.PinRef(ctx, project.Path, gitx.DirectResultRef(card.ID), snapshot.Commit); err != nil {
		return Result{}, err
	}
	s.clearProgress(ctx, card, directFinished)
	if _, err := s.cards.SetState(ctx, card.ID, protocol.CardStateDone); err != nil {
		return Result{}, err
	}
	if id, err := s.newID(); err == nil {
		err = s.ledger.AddDelivery(ctx, Delivery{
			ID: id, ProjectID: project.ID, CardID: card.ID, Target: card.Branch,
			MergedAt: s.now(), Commit: snapshot.Commit, Summary: directFinished,
		})
		if err != nil {
			s.log.Warn("could not record a completed direct card", "card_id", card.ID, "error", err)
		}
	}
	return Result{Merged: true, Commit: snapshot.Commit, Note: directFinished}, nil
}

func (s *Service) reviewDirectSnapshot(ctx context.Context, card protocol.Card, project protocol.Project, base string, snapshot gitx.Snapshot, release func()) (bool, error) {
	baseTree, err := s.git.TreeOf(ctx, project.Path, base)
	if err != nil {
		return false, err
	}
	return baseTree == snapshot.Tree || s.reviewDirect(ctx, card, project, base, snapshot.Commit, release), nil
}

func (s *Service) reviewDirect(ctx context.Context, card protocol.Card, project protocol.Project, base, head string, release func()) bool {
	s.setProgress(ctx, card, Progress{Phase: protocol.MergePhaseReviewing, Note: "Reading the changes", DoingNow: "Being reviewed"})
	verdict, err := s.reviewer.ReviewChanges(ctx, ChangeReview{Card: card, Project: project, Worktree: project.Path, Base: base, Head: head})
	if err != nil {
		s.stopShip(ctx, card, "The review could not run: "+err.Error())
		return false
	}
	s.note(ctx, card.ID, reviewNote(verdict))
	if !verdict.Approved {
		toWorker := strings.Replace(workerReviewText(verdict), "Fix these, commit, and finish", "Fix these and finish", 1)
		release()
		s.sendBack(ctx, card, toWorker, "The Reviewer asked for changes: "+oneLine(verdict.Summary))
	}
	return verdict.Approved
}
