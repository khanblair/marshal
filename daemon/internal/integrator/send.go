package integrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// SendToMerge is how a card of a project with no GitHub origin reaches Ready to merge. A project on
// GitHub gets there through a pull request, a review and passing checks; one without them has none
// of those to wait for, so a person decides the card's work is finished. The card is moved the way
// the daemon moves it, so Enqueue sees it like any other card that became ready, and the merge runs
// when the project's settings allow it.
//
// A card that is already in Ready to merge is not moved again, but the queue is asked to look at it,
// which is what a person who pressed Merge on a waiting card wants. Every refusal is a plain
// sentence with a stable reason, and nothing is changed by one.
func (s *Service) SendToMerge(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if card.State == protocol.CardStateReady {
		s.kick(ctx, card.ProjectID)
		return card, nil
	}
	if err := s.canSend(ctx, card); err != nil {
		return protocol.Card{}, err
	}
	return s.cards.SetState(ctx, cardID, protocol.CardStateReady)
}

// canSend says why a card cannot be sent to the queue, or nil when it can.
func (s *Service) canSend(ctx context.Context, card protocol.Card) error {
	if err := sendStateRefusal(card); err != nil {
		return err
	}
	if s.stalled(ctx, card) {
		return refusedSend(card.ID, "send_merge_stopped", "A merge stopped this card. Retry it from the Integration view.")
	}
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return err
	}
	if s.hasGitHubOrigin(ctx, project.Path) {
		return refusedSend(card.ID, "send_has_github",
			"This project is on GitHub, so its cards reach Ready to merge through a pull request and a review.")
	}
	ahead, err := s.git.CountAhead(ctx, project.Path, project.Target(), card.Branch)
	if err != nil {
		return fmt.Errorf("count the commits of card %s: %w", card.ID, err)
	}
	if ahead == 0 {
		return refusedSend(card.ID, "send_no_commits",
			"This card has no commits to merge yet. Commit its work, or ask the agent to, and send it again.")
	}
	if s.lists != nil {
		open, err := s.lists.OpenRequiredItems(ctx, card.ID)
		if err != nil {
			return err
		}
		if open > 0 {
			return refusedSend(card.ID, "send_checklist_open",
				"A required checklist on this card still has open items. Finish it before the merge.")
		}
	}
	return nil
}

// sendStateRefusal checks the card itself: it has a branch, it is not mid-merge or done, and its
// agent is not still writing to the branch the merge is about to read.
func sendStateRefusal(card protocol.Card) error {
	switch {
	case card.State == protocol.CardStateDone || card.State == protocol.CardStateMerging:
		return refusedSend(card.ID, "send_not_open", "This card is already merged or being merged.")
	case card.State == protocol.CardStateBacklog || card.State == protocol.CardStatePlanning || card.Branch == "":
		return refusedSend(card.ID, "send_not_started", "This card has no work yet. Start it first.")
	case card.Session != nil && *card.Session == protocol.SessionStateWorking:
		return refusedSend(card.ID, "send_agent_working",
			"The agent is still working on this card. Pause it or wait for it to finish, so the merge takes everything it did.")
	}
	return nil
}

// hasGitHubOrigin says whether the project's origin remote is on GitHub. A project with no origin, or
// one whose origin is somewhere else, has no pull request to wait for.
func (s *Service) hasGitHubOrigin(ctx context.Context, repo string) bool {
	url, err := s.git.Run(ctx, repo, "remote", "get-url", "origin")
	return err == nil && strings.Contains(strings.ToLower(url), "github.com")
}

// refusedSend is the refusal for a card that cannot be sent to the merge queue.
func refusedSend(cardID, reason, message string) *protocol.Error {
	return protocol.Refused(message).With("cardId", cardID).With("reason", reason)
}
