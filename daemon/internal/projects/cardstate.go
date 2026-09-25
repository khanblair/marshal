package projects

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// SetState moves a card to a state and publishes card.moved after the commit. The state must be
// one of the fixed list. Which moves are allowed between states is decided by the caller for now:
// the allowed-move rules come with the card life cycle. Moving a card to the state it is in
// changes nothing and publishes nothing.
//
// A card that moves into or out of "needs" changes its project's badge, so project.updated
// follows on the home topic.
func (s *Service) SetState(ctx context.Context, id string, state protocol.CardState) (protocol.Card, error) {
	if !state.Valid() {
		return protocol.Card{}, protocol.InvalidArgument("That is not a card state Marshal knows.").With("state", string(state))
	}
	var before, after db.Card
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCard(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
		}
		before, after = row, row
		if row.State == string(state) {
			return nil
		}
		after.State, after.UpdatedAt = string(state), s.now().UnixMilli()
		if _, err := q.UpdateCardState(ctx, db.UpdateCardStateParams{State: after.State, UpdatedAt: after.UpdatedAt, ID: id}); err != nil {
			return fmt.Errorf("update the state of card %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card := toCard(after)
	if before.State == after.State {
		return card, nil
	}
	from := protocol.CardState(before.State)
	s.log.Info("moved a card", "project_id", card.ProjectID, "card_id", id, "from", from, "to", state)
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardMoved,
		protocol.CardMovedEventData{Card: card, From: from}, true)
	s.announceBadges(ctx, card.ProjectID, from, state)
	return card, nil
}

// announceBadges publishes the project again when a card moved into or out of "needs", because
// that changes the project's badge. It runs after the move is committed and announced, so a
// failure is logged and not returned: the client gets the right badge with its next list.
func (s *Service) announceBadges(ctx context.Context, projectID string, from, to protocol.CardState) {
	if from != protocol.CardStateNeeds && to != protocol.CardStateNeeds {
		return
	}
	// The caller may give up after the commit. The announcement is still owed.
	project, err := s.Get(context.WithoutCancel(ctx), projectID)
	if err != nil {
		s.log.Warn("could not announce the new badges of a project", "project_id", projectID, "error", err)
		return
	}
	s.publish(protocol.HomeTopic, protocol.EventTypeProjectUpdated, protocol.ProjectEventData{Project: project}, false)
}

// SetWorktree records the worktree folder and the branch that were made for a card, and publishes
// card.updated after the commit. The folder must be inside the project's own worktrees folder
// (see WorktreesDir), because removing a project cleans that folder up. An empty path with an
// empty branch clears both. An empty path with a branch records that the worktree is gone and
// the branch is kept.
func (s *Service) SetWorktree(ctx context.Context, id, path, branch string) (protocol.Card, error) {
	row, err := s.store.Queries().GetCard(ctx, id)
	if err != nil {
		return protocol.Card{}, notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
	}
	path, err = s.checkWorktree(ctx, row.ProjectID, path, branch)
	if err != nil {
		return protocol.Card{}, fmt.Errorf("record the worktree of card %s: %w", id, err)
	}
	var after db.Card
	changed := false
	err = s.store.Write(ctx, func(q *db.Queries) error {
		current, err := q.GetCard(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
		}
		after = current
		if current.WorktreePath == path && current.Branch == branch {
			return nil
		}
		after.WorktreePath, after.Branch, after.UpdatedAt = path, branch, s.now().UnixMilli()
		if _, err := q.UpdateCardWorktree(ctx, db.UpdateCardWorktreeParams{
			WorktreePath: path, Branch: branch, UpdatedAt: after.UpdatedAt, ID: id,
		}); err != nil {
			return fmt.Errorf("update the worktree of card %s: %w", id, err)
		}
		changed = true
		return nil
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card := toCard(after)
	if changed {
		s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated, protocol.CardEventData{Card: card}, false)
	}
	return card, nil
}

// checkWorktree checks a worktree folder and a branch, and returns the cleaned folder.
func (s *Service) checkWorktree(ctx context.Context, projectID, path, branch string) (string, error) {
	if path == "" && branch == "" {
		return "", nil
	}
	if branch == "" {
		return "", errors.New("a worktree needs a branch")
	}
	if err := s.git.ValidBranchName(ctx, branch); err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	dir := WorktreesDir(s.dataDir, projectID)
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) || clean == dir || !within(dir, clean) {
		return "", fmt.Errorf("the folder is not inside the worktrees folder of project %s", projectID)
	}
	return clean, nil
}
