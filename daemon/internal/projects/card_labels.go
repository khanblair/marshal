package projects

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// cardWithLabels builds a wire card with the labels that are on it now. Callers that already hold
// a read closure use toCard directly with what ListLabelsForCard returned, so a board does not run
// one query per card.
func (s *Service) cardWithLabels(ctx context.Context, row db.Card) (protocol.Card, error) {
	var labels []protocol.Label
	err := s.store.Read(ctx, func(q *db.Queries) error {
		rows, err := q.ListLabelsForCard(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("read the labels of card %s: %w", row.ID, err)
		}
		labels = toLabels(rows)
		return nil
	})
	if err != nil {
		return protocol.Card{}, err
	}
	return s.withSession(ctx, toCard(row, labels))
}

// withSession sets a card's session state and view from the session module (see SessionStates). A
// read that fails is an error rather than a null: a card that said it had no session while it had
// one would tell a client its agent is gone.
func (s *Service) withSession(ctx context.Context, card protocol.Card) (protocol.Card, error) {
	info, err := s.states.CardSession(ctx, card.ID)
	if err != nil {
		return protocol.Card{}, fmt.Errorf("read the session state of card %s: %w", card.ID, err)
	}
	if info != nil {
		card = withSessionInfo(card, *info)
	}
	return card, nil
}

// withSessionInfo sets the session and view of a card from what the session module stored.
func withSessionInfo(card protocol.Card, info SessionInfo) protocol.Card {
	state := info.State
	card.Session = &state
	card.ViewMode = info.View
	return card
}

// withSessions sets the session state of a project's cards from one read, so a board asks once for
// every card rather than once per card.
func (s *Service) withSessions(ctx context.Context, projectID string, cards []protocol.Card) ([]protocol.Card, error) {
	infos, err := s.states.ProjectSessions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the session states of project %s: %w", projectID, err)
	}
	for i := range cards {
		if info, ok := infos[cards[i].ID]; ok {
			cards[i] = withSessionInfo(cards[i], info)
		}
	}
	return cards, nil
}

// cardLabelsByCard reads every label on every card of a project, grouped by card. It runs inside a
// read closure, so a board is one query for the cards and one for their labels.
func cardLabelsByCard(ctx context.Context, q *db.Queries, projectID string) (map[string][]protocol.Label, error) {
	rows, err := q.ListCardLabelsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the labels of the cards of project %s: %w", projectID, err)
	}
	return groupLabelsByCard(rows), nil
}
