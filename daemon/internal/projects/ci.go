package projects

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// SetCI records the state of the CI runs on a card's branch (docs/architecture.md sections 9 and 10,
// docs/backend-checklist.md B6.2, build-plan 6.2). It is the daemon's own write and not a person's:
// the CI monitor calls it when a workflow run on the card's branch changes, so the badge on the card
// and the CI column of every list follow GitHub without anyone asking.
//
// It is a whole-state write rather than a field of the person-facing PATCH for the same reason
// SetPullRequest is: a person corrects a link, but the daemon is what watches the forge. The row is
// read and written inside one transaction, so a concurrent edit of the card is never clobbered.
//
// A nil state clears the column, which is the honest answer for a branch Marshal no longer has a run
// for. The same state twice is a no-op, so an unchanged run does not publish a second event.
func (s *Service) SetCI(ctx context.Context, id string, state *protocol.CIState) (protocol.Card, error) {
	var after db.Card
	want := ""
	if state != nil {
		want = string(*state)
	}
	changed := false
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCard(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
		}
		after = row
		if row.CiState == want {
			return nil
		}
		after.CiState = want
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
		s.log.Info("recorded a card's CI state", "project_id", card.ProjectID, "card_id", id,
			"ci_state", want)
		s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated,
			protocol.CardEventData{Card: card}, false)
	}
	return card, nil
}
