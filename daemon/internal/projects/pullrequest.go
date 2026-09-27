package projects

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// SetPullRequest records the pull request a card's work was opened as (docs/architecture.md
// sections 6 and 8, docs/backend-checklist.md B5.4, build-plan 5.6). It is the daemon's own write,
// separate from the person-facing PATCH: a person can correct the link, but the daemon is what
// opens the pull request and records it here. It publishes card.updated after the commit.
//
// A card keeps its pull request once it has one: the same number and address is a no-op, so opening
// twice does not publish twice. A number of zero with no address clears the link, which is what a
// fork's own card reads.
func (s *Service) SetPullRequest(ctx context.Context, id string, pr protocol.PullRequest) (protocol.Card, error) {
	if pr.Number <= 0 && strings.TrimSpace(pr.URL) == "" {
		return protocol.Card{}, protocol.InvalidArgument("A pull request needs a number or an address.").With("cardId", id)
	}
	var after db.Card
	changed := false
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCard(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
		}
		after = row
		if row.PullRequestNumber != nil && int(*row.PullRequestNumber) == pr.Number && row.PullRequestUrl == pr.URL {
			return nil
		}
		number := int64(pr.Number)
		if pr.Number > 0 {
			after.PullRequestNumber = &number
		} else {
			after.PullRequestNumber = nil
		}
		after.PullRequestUrl = pr.URL
		after.UpdatedAt = s.now().UnixMilli()
		changed = true
		return updateCardRow(ctx, q, after)
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card, err := s.cardWithLabels(ctx, after)
	if err != nil {
		return protocol.Card{}, err
	}
	if changed {
		s.log.Info("recorded a card's pull request", "project_id", card.ProjectID, "card_id", id, "number", pr.Number)
		s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated, protocol.CardEventData{Card: card}, false)
	}
	return card, nil
}
