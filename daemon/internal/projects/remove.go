package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Remove stops Marshal from managing a project. It follows the order of docs/architecture.md
// section 16.1:
//
//  1. Stop the project's sessions, so nothing runs in a worktree that is about to go.
//  2. Clean up the project's worktrees under <data>/worktrees/<project>, and delete the branches
//     Marshal made for its cards unless KeepBranches is set.
//  3. Delete the project's rows: its board and cards go with it.
//  4. Delete the project's memory folder unless KeepMemory is set.
//  5. Publish project.removed.
//
// It never deletes or changes a file in the repository folder. Every step can be run again, so a
// Remove that stopped half way (a worktree in use on Windows, say) is finished by calling it again.
func (s *Service) Remove(ctx context.Context, id string, opts RemoveOptions) error {
	row, err := s.store.Queries().GetProject(ctx, id)
	if err != nil {
		return notFound(fmt.Errorf("read project %s: %w", id, err), notFoundProject(id))
	}
	if err := s.sessions.StopProjectSessions(ctx, id); err != nil {
		return protocol.Unavailable("Marshal could not stop the running agents of this project. Try again.").WithCause(err)
	}
	if err := s.cleanUp(ctx, row, opts); err != nil {
		return err
	}
	if err := s.deleteRows(ctx, id); err != nil {
		return err
	}
	s.log.Info("removed a project", "project_id", id, "keep_branches", opts.KeepBranches, "keep_memory", opts.KeepMemory)
	var memoryErr error
	if !opts.KeepMemory {
		memoryErr = s.memory.RemoveProjectMemory(ctx, id)
	}
	// The project is gone whether or not its memory could be deleted, so clients hear about it.
	s.publish(protocol.HomeTopic, protocol.EventTypeProjectRemoved, protocol.ProjectRemovedEventData{ProjectID: id}, true)
	if memoryErr != nil {
		return protocol.NewError(protocol.ErrorCodeInternal,
			"The project was removed, but Marshal could not delete its memory folder. You can delete the folder yourself.").
			WithCause(memoryErr)
	}
	return nil
}

// deleteRows deletes the project row. The board and the cards are deleted with it by the
// database.
func (s *Service) deleteRows(ctx context.Context, id string) error {
	return s.store.Write(ctx, func(q *db.Queries) error {
		removed, err := q.DeleteProject(ctx, id)
		if err != nil {
			return fmt.Errorf("delete project %s: %w", id, err)
		}
		if removed == 0 {
			return notFoundProject(id)
		}
		return nil
	})
}

// cleanUp removes the project's worktrees and, unless they are kept, its card branches. It holds
// the repository's lock, so no other Git write runs in the repository meanwhile.
func (s *Service) cleanUp(ctx context.Context, project db.Project, opts RemoveOptions) error {
	branches, err := s.store.Queries().ListCardBranches(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("list the branches of project %s: %w", project.ID, err)
	}
	release, err := s.locks.acquire(ctx, project.RepoPath)
	if err != nil {
		return fmt.Errorf("wait for the repository of project %s: %w", project.ID, err)
	}
	defer release()
	dir := WorktreesDir(s.dataDir, project.ID)
	if !folderExists(project.RepoPath) {
		// The person moved or deleted the repository. There is no Git to ask, and no record to
		// keep: the worktree folders are Marshal's own, so they are simply removed.
		s.log.Warn("the repository folder is gone, so its worktrees are removed without Git",
			"project_id", project.ID)
		return removeFolder(dir)
	}
	if err := s.removeWorktrees(ctx, project, dir); err != nil {
		return protocol.Unavailable("Marshal could not clean up a worktree of this project. Close anything that uses it, then try again.").
			WithCause(err)
	}
	if !opts.KeepBranches {
		s.deleteBranches(ctx, project, branches)
	}
	return nil
}

// removeWorktrees removes every folder under the project's worktrees folder. That is the card
// worktrees, and also any folder left there by a card that never finished starting. A worktree
// with changes that are not committed is removed too, and the log says so: the person confirmed
// the removal, and the sessions that could write to it are stopped.
func (s *Service) removeWorktrees(ctx context.Context, project db.Project, dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return s.git.PruneWorktrees(ctx, project.RepoPath)
	}
	if err != nil {
		return fmt.Errorf("look inside %s: %w", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			s.log.Warn("left a file that is not a worktree in the worktrees folder", "project_id", project.ID)
			continue
		}
		path := filepath.Join(dir, entry.Name())
		err := s.git.RemoveWorktree(ctx, project.RepoPath, path, dir, false)
		if errors.Is(err, gitx.ErrDirty) {
			s.log.Warn("removing a worktree that has changes that are not committed",
				"project_id", project.ID, "worktree", entry.Name())
			err = s.git.RemoveWorktree(ctx, project.RepoPath, path, dir, true)
		}
		if err != nil {
			return fmt.Errorf("remove the worktree %s: %w", entry.Name(), err)
		}
	}
	if err := s.git.PruneWorktrees(ctx, project.RepoPath); err != nil {
		return err
	}
	return removeIfEmpty(dir)
}

// deleteBranches deletes the branches Marshal made for the project's cards. A branch that cannot
// be deleted is kept and logged, and never stops the removal: keeping a branch is the safe side
// of a failure, and the usual cause is that the person has it checked out in their own folder.
func (s *Service) deleteBranches(ctx context.Context, project db.Project, branches []string) {
	slices.Sort(branches)
	for _, branch := range slices.Compact(branches) {
		if ctx.Err() != nil {
			return
		}
		// Only branches that carry Marshal's own prefix are ours to delete.
		if !strings.HasPrefix(branch, gitx.CardBranchPrefix) {
			continue
		}
		found, err := s.git.BranchExists(ctx, project.RepoPath, branch)
		if err == nil && found {
			err = s.git.DeleteBranch(ctx, project.RepoPath, branch, true)
		}
		if err != nil {
			s.log.Warn("kept a branch that could not be deleted", "project_id", project.ID, "branch", branch, "error", err)
		}
	}
}

// removeFolder deletes a folder that belongs to Marshal, if it is there.
func removeFolder(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}

// removeIfEmpty deletes a folder only when nothing is in it. Anything else that someone put in the
// project's worktrees folder is left alone.
func removeIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look inside %s: %w", dir, err)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}
