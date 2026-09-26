package projects

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// DeleteCard removes a card and everything Marshal made for it. It follows the order the removal
// of a project follows (docs/architecture.md 16.1), one card at a time:
//
//  1. Stop the card's session, so nothing runs in a worktree that is about to go.
//  2. Remove the card's worktree under <data>/worktrees/<project>, and delete its branch, under
//     the repository's lock.
//  3. Delete the card's session log folder.
//  4. Delete the card's row. Its session row goes with it by the database.
//  5. Publish card.deleted.
//
// It never deletes or changes a file in the repository folder. A step that fails leaves the card
// in place, and calling it again finishes the job: the worktree is already gone, and removing a
// worktree that is not there is fine.
func (s *Service) DeleteCard(ctx context.Context, id string) error {
	row, err := s.store.Queries().GetCard(ctx, id)
	if err != nil {
		return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
	}
	if err := s.sessions.StopCardSession(ctx, id); err != nil {
		return protocol.Unavailable("Marshal could not stop this card's agent. Try again.").WithCause(err)
	}
	if err := s.removeCardWork(ctx, row); err != nil {
		return err
	}
	if err := s.sessions.RemoveCardLogs(ctx, id); err != nil {
		return protocol.Unavailable("Marshal could not delete this card's logs. Close anything that uses them, then try again.").
			WithCause(err)
	}
	if err := s.deleteCardRow(ctx, id); err != nil {
		return err
	}
	s.log.Info("deleted a card", "project_id", row.ProjectID, "card_id", id, "number", row.Number)
	key := protocol.CardKey{ProjectID: row.ProjectID, Number: int(row.Number)}.String()
	s.publish(protocol.ProjectTopic(row.ProjectID), protocol.EventTypeCardDeleted,
		protocol.CardDeletedEventData{CardID: id, Key: key, ProjectID: row.ProjectID}, true)
	return nil
}

// removeCardWork removes the card's worktree and its branch, holding the repository's lock so no
// other Git write runs in the repository meanwhile. A card that never started has neither, and
// that is not an error.
func (s *Service) removeCardWork(ctx context.Context, row db.Card) error {
	if row.WorktreePath == "" && row.Branch == "" {
		return nil
	}
	project, err := s.store.Queries().GetProject(ctx, row.ProjectID)
	if err != nil {
		return notFound(fmt.Errorf("read project %s: %w", row.ProjectID, err), notFoundProject(row.ProjectID))
	}
	release, err := s.locks.acquire(ctx, project.RepoPath)
	if err != nil {
		return fmt.Errorf("wait for the repository of project %s: %w", project.ID, err)
	}
	defer release()
	if !folderExists(project.RepoPath) {
		// The person moved or deleted the repository. There is no Git to ask, and the worktree
		// folder is Marshal's own, so it is simply removed.
		s.log.Warn("the repository folder is gone, so the card's worktree is removed without Git",
			"project_id", project.ID, "card_id", row.ID)
		return removeFolder(WorktreesDir(s.dataDir, project.ID))
	}
	if err := s.removeCardWorktree(ctx, project, row); err != nil {
		return protocol.Unavailable("Marshal could not remove this card's worktree. Close anything that uses it, then try again.").
			WithCause(err)
	}
	s.deleteCardBranch(ctx, project, row)
	return nil
}

// removeCardWorktree removes the folder the card's agent worked in. A worktree with changes that
// are not committed is removed too, and the log says so: the person confirmed the deletion, and
// the session that could write to it is stopped.
func (s *Service) removeCardWorktree(ctx context.Context, project db.Project, row db.Card) error {
	if row.WorktreePath == "" {
		return nil
	}
	root := WorktreesDir(s.dataDir, project.ID)
	err := s.git.RemoveWorktree(ctx, project.RepoPath, row.WorktreePath, root, false)
	if errors.Is(err, gitx.ErrDirty) {
		s.log.Warn("removing a worktree that has changes that are not committed",
			"project_id", project.ID, "card_id", row.ID)
		err = s.git.RemoveWorktree(ctx, project.RepoPath, row.WorktreePath, root, true)
	}
	if err != nil {
		return fmt.Errorf("remove the worktree of card %s: %w", row.ID, err)
	}
	return s.git.PruneWorktrees(ctx, project.RepoPath)
}

// deleteCardBranch deletes the branch Marshal made for the card. A branch that cannot be deleted
// is kept and logged, and never stops the deletion: keeping a branch is the safe side of a
// failure, and the usual cause is that the person has it checked out in their own folder.
func (s *Service) deleteCardBranch(ctx context.Context, project db.Project, row db.Card) {
	// Only a branch that carries Marshal's own prefix is ours to delete.
	if row.Branch == "" || !strings.HasPrefix(row.Branch, gitx.CardBranchPrefix) {
		return
	}
	found, err := s.git.BranchExists(ctx, project.RepoPath, row.Branch)
	if err != nil || !found {
		return
	}
	if err := s.git.DeleteBranch(ctx, project.RepoPath, row.Branch, true); err != nil {
		s.log.Warn("kept a branch that could not be deleted",
			"project_id", project.ID, "card_id", row.ID, "branch", row.Branch, "error", err)
	}
}

// deleteCardRow deletes the card. Its sessions and its labels go with it by the database.
func (s *Service) deleteCardRow(ctx context.Context, id string) error {
	return s.store.Write(ctx, func(q *db.Queries) error {
		removed, err := q.DeleteCard(ctx, id)
		if err != nil {
			return fmt.Errorf("delete card %s: %w", id, err)
		}
		if removed == 0 {
			return notFoundCard(id)
		}
		return nil
	})
}
