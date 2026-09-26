package projects

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// SetHold writes a card's pause flag, its keep-awake flag, or both, and publishes card.updated
// when one of them really changed. The rules for when a card may be paused, put to sleep, or
// pinned live in internal/session (docs/architecture.md section 5.1); this is only the write,
// because a card's own fields are this module's to change.
//
// A nil flag is left as it is, so the caller sends only what it means to change. Setting a flag
// that is already set changes nothing and publishes nothing, the way moving a card to the column
// it is already in does.
func (s *Service) SetHold(ctx context.Context, id string, paused, pinned *bool) (protocol.Card, error) {
	var after db.Card
	changed := false
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCard(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
		}
		after = row
		if paused != nil && boolFromInt(row.Paused) != *paused {
			after.Paused, changed = intFromBool(*paused), true
		}
		if pinned != nil && boolFromInt(row.Pinned) != *pinned {
			after.Pinned, changed = intFromBool(*pinned), true
		}
		if !changed {
			return nil
		}
		after.UpdatedAt = s.now().UnixMilli()
		return updateCardRow(ctx, q, after)
	})
	if err != nil {
		return protocol.Card{}, err
	}
	card, err := s.cardWithLabels(ctx, after)
	if err != nil {
		return protocol.Card{}, err
	}
	if !changed {
		return card, nil
	}
	s.log.Info("changed a card's hold", "project_id", card.ProjectID, "card_id", id,
		"paused", card.Paused, "pinned", card.Pinned)
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated,
		protocol.CardEventData{Card: card}, false)
	return card, nil
}

// SessionChanged publishes card.updated with the card as it is now, for a change to the card's
// session. The session module owns the session and announces the change as session.state_changed
// on the card's own topic; the wire card carries its session state too, so the board and Home,
// which follow the project topic, hear the same change here. It is critical, like the state change
// it follows, so no client keeps drawing a card as awake after it went to sleep. A card that cannot
// be read is logged and not returned: the change is already stored and announced on the card's
// own topic, and the next board load shows it.
func (s *Service) SessionChanged(ctx context.Context, cardID string) {
	card, err := s.Card(ctx, cardID)
	if err != nil {
		s.log.Warn("could not read a card to announce its session state", "card_id", cardID, "error", err)
		return
	}
	s.publish(protocol.ProjectTopic(card.ProjectID), protocol.EventTypeCardUpdated,
		protocol.CardEventData{Card: card}, true)
}
