package projects

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Forking a card (inventory N3, task 2.10). A fork is a new card in the backlog that carries a
// copy of the card it came from, and whose branch starts from that card's latest commit instead of
// the project's default branch. Checkpoints, which the prototype's chat message mentions, arrive
// in Phase 5.
//
// The branch itself is made when the fork starts, not here: a card's worktree and branch are made
// together in one step, and a branch made now would stop the worktree being added later. What a
// fork records is where its branch should start from.

const (
	// forkTitleSuffix is what a fork's title gets, the way the prototype showed it.
	forkTitleSuffix = " (fork)"
	// forkDoingNow is the line a fork shows before it starts.
	forkDoingNow = "Starting from the latest checkpoint"
)

// ForkCard adds a card that starts from the given card's latest commit. The new card goes in the
// backlog with a copy of the card's settings and labels, and card.created is published after the
// commit.
func (s *Service) ForkCard(ctx context.Context, id string) (protocol.Card, error) {
	source, err := s.Card(ctx, id)
	if err != nil {
		return protocol.Card{}, err
	}
	if source.Branch == "" {
		// There is no commit to start from: a card that never started has no branch.
		return protocol.Card{}, protocol.Refused(
			"This card has not started, so there is nothing to fork yet. Start it first.").
			With("cardId", id)
	}
	fork, err := s.CreateCard(ctx, source.ProjectID, forkInputOf(source),
		WithForkedFrom(source.ID), WithDoingNow(forkDoingNow))
	if err != nil {
		return protocol.Card{}, err
	}
	// A fork carries the labels of the card it came from.
	if ids := labelIDsOf(source); len(ids) > 0 {
		if fork, err = s.replaceLabels(ctx, fork, ids); err != nil {
			return protocol.Card{}, err
		}
	}
	s.log.Info("forked a card", "project_id", source.ProjectID, "card_id", fork.ID, "from", source.ID)
	return fork, nil
}

// ForkBase is the branch a card's branch should start from, or "" for the project's default
// branch. It is the branch of the card this one was forked from. A source branch that is gone
// falls back to the default and says so in the log, so a fork always starts: the person asked for
// a card, not for an error about a branch.
func (s *Service) ForkBase(ctx context.Context, project protocol.Project, cardID string) string {
	forkedFrom, err := s.store.Queries().GetCardForkedFrom(ctx, cardID)
	if err != nil || forkedFrom == "" {
		return ""
	}
	source, err := s.Card(ctx, forkedFrom)
	if err != nil || source.Branch == "" {
		s.log.Warn("the card a fork came from is gone, so its branch starts from the default branch",
			"card_id", cardID, "forked_from", forkedFrom)
		return ""
	}
	found, err := s.git.BranchExists(ctx, project.Path, source.Branch)
	if err != nil || !found {
		s.log.Warn("the branch a fork came from is gone, so its branch starts from the default branch",
			"card_id", cardID, "forked_from", forkedFrom, "branch", source.Branch, "error", err)
		return ""
	}
	return source.Branch
}

// forkInputOf copies what a fork keeps from the card it came from. The state is not copied: a fork
// always starts in the backlog, and it reaches the other columns the way any card does.
func forkInputOf(source protocol.Card) CardInput {
	thinking := protocol.ThinkingMode("")
	if source.Thinking != nil {
		thinking = *source.Thinking
	}
	return CardInput{
		Title:          source.Title + forkTitleSuffix,
		Body:           source.Body,
		Agent:          source.Agent,
		Model:          source.Model,
		Thinking:       thinking,
		PermissionMode: source.PermissionMode,
		Role:           source.Role,
		Package:        source.Package,
	}
}

// labelIDsOf lists a card's label ids.
func labelIDsOf(card protocol.Card) []string {
	ids := make([]string, 0, len(card.Labels))
	for _, label := range card.Labels {
		ids = append(ids, label.ID)
	}
	return ids
}

// replaceLabels sets a card's labels and answers with the card as it now is. The ids came from a
// card of the same project, so they are not checked again the way a request's are.
func (s *Service) replaceLabels(ctx context.Context, card protocol.Card, ids []string) (protocol.Card, error) {
	var labels []protocol.Label
	err := s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		if labels, err = replaceCardLabels(ctx, q, card.ProjectID, card.ID, ids); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card.Labels = labels
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated, protocol.CardEventData{Card: card}, false)
	return card, nil
}
