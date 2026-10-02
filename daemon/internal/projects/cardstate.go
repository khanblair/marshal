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
// One rule does run here: a card moving to In review passes the quality gate of
// docs/architecture.md section 17.1, which is what keeps a card whose changes have a blocking code
// smell in Working while the finding goes back to its agent. It is here and not only in MoveCard
// because the daemon's own move to review - an agent's pull request being opened (internal/
// pullrequest) - is this call, and a blocking smell must keep the card out of review either way.
//
// A card that moves into or out of "needs" changes its project's badge, so project.updated
// follows on the home topic.
func (s *Service) SetState(ctx context.Context, id string, state protocol.CardState) (protocol.Card, error) {
	if !state.Valid() {
		return protocol.Card{}, protocol.InvalidArgument("That is not a card state Marshal knows.").With("state", string(state))
	}
	if state == protocol.CardStateReview {
		card, err := s.Card(ctx, id)
		if err != nil {
			return protocol.Card{}, err
		}
		if card.State != protocol.CardStateReview {
			if refusal := s.checkQuality(ctx, id); refusal != nil {
				s.log.Info("kept a card out of review", "project_id", card.ProjectID, "card_id", id,
					"from", card.State, "reason", refusal.Details["reason"])
				return protocol.Card{}, refusal
			}
		}
	}
	if state == protocol.CardStateReady {
		card, err := s.Card(ctx, id)
		if err != nil {
			return protocol.Card{}, err
		}
		if card.State != protocol.CardStateReady {
			if refusal := s.checkChecklists(ctx, id); refusal != nil {
				s.log.Info("kept a card out of ready", "project_id", card.ProjectID, "card_id", id,
					"from", card.State, "reason", refusal.Details["reason"])
				return protocol.Card{}, refusal
			}
		}
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
		if state != protocol.CardStateNeeds {
			// A card that leaves needs has waited as long as it is going to (applyMove does the
			// same for a manual move); UpdateCardState alone only ever touched state and
			// updated_at, so a caller that moved a card off needs this way - clearApprovalNeeds,
			// internal/session/approval.go - left the stale reason for needsReasonOf to keep
			// reading back, including the answered approval's own id.
			after.NeedsReasonKind, after.NeedsReasonText = "", ""
			after.NeedsSince = nil
			if err := updateCardRow(ctx, q, after); err != nil {
				return fmt.Errorf("update the state of card %s: %w", id, err)
			}
			return nil
		}
		if _, err := q.UpdateCardState(ctx, db.UpdateCardStateParams{State: after.State, UpdatedAt: after.UpdatedAt, ID: id}); err != nil {
			return fmt.Errorf("update the state of card %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card, err := s.cardWithLabels(ctx, after)
	if err != nil {
		return protocol.Card{}, err
	}
	if before.State == after.State {
		return card, nil
	}
	from := protocol.CardState(before.State)
	s.log.Info("moved a card", "project_id", card.ProjectID, "card_id", id, "from", from, "to", state)
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardMoved,
		protocol.CardMovedEventData{Card: card, From: from}, true)
	s.announceBadges(ctx, card.ProjectID, from, state)
	s.notifyMoved(ctx, id, state)
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

// SetNeeds moves a card to Needs you and gives it the reason a person will read, in one write. It
// is how the daemon's own needs moves carry a reason: SetState only changes the state, and the
// person-facing UpdateCard only writes the reason, so neither moves a card to needs with one.
//
// The reason kind must be one of the fixed list. A card already in needs with the same reason is
// left alone; a card already in needs with a different reason has its reason replaced, because the
// last thing that stopped the card is the thing a person should read.
func (s *Service) SetNeeds(ctx context.Context, id string, reason protocol.NeedsReason) (protocol.Card, error) {
	if !reason.Kind.Valid() {
		return protocol.Card{}, protocol.InvalidArgument("Marshal does not know that needs-you reason.").With("kind", string(reason.Kind))
	}
	var before, after db.Card
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCard(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
		}
		before, after = row, row
		now := s.now().UnixMilli()
		after.State = string(protocol.CardStateNeeds)
		after.NeedsReasonKind, after.NeedsReasonText = string(reason.Kind), reason.Text
		after.NeedsSince, after.UpdatedAt = &now, now
		return updateCardRow(ctx, q, after)
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card, err := s.cardWithLabels(ctx, after)
	if err != nil {
		return protocol.Card{}, err
	}
	from := protocol.CardState(before.State)
	switch {
	case from == protocol.CardStateNeeds && before.NeedsReasonKind == after.NeedsReasonKind &&
		before.NeedsReasonText == after.NeedsReasonText:
		// Nothing changed: the card is already waiting for this reason.
		return card, nil
	case from == protocol.CardStateNeeds:
		// The card was already waiting; only its reason changed. The card is announced so the new
		// reason reaches the screens, and the project badge is not touched because entering or
		// leaving needs is what changes it.
		s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated,
			protocol.CardEventData{Card: card}, false)
		return card, nil
	}
	s.log.Info("moved a card to needs you", "project_id", card.ProjectID, "card_id", id,
		"from", from, "reason", reason.Kind)
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardMoved,
		protocol.CardMovedEventData{Card: card, From: from}, true)
	s.announceBadges(ctx, card.ProjectID, from, protocol.CardStateNeeds)
	return card, nil
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
	card, err := s.cardWithLabels(ctx, after)
	if err != nil {
		return protocol.Card{}, err
	}
	if changed {
		s.trustFolder(path)
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

// Worktree returns the worktree folder and branch stored for a card. Both are empty until the
// card starts. This is a Go-level accessor for another daemon module (the session manager), not a
// wire type: protocol.Card deliberately leaves the worktree's local filesystem path off the wire,
// the way a project's own repository folder is on protocol.Project.Path but a card's worktree,
// which nobody typed in, is not.
func (s *Service) Worktree(ctx context.Context, cardID string) (path, branch string, err error) {
	row, err := s.store.Queries().GetCard(ctx, cardID)
	if err != nil {
		return "", "", notFound(fmt.Errorf("read card %s: %w", cardID, err), notFoundCard(cardID))
	}
	return row.WorktreePath, row.Branch, nil
}
