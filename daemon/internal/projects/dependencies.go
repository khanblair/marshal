package projects

import (
	"context"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Card dependencies: the cards a card waits for (docs/architecture.md section 10's `card_links`
// row, migration 0020; docs/marshal-product-scope.md section 10.2's goal-to-plan flow;
// docs/backend-checklist.md B7.3; build-plan task 7.4).
//
// # How an edge is made
//
// Only one way: a card is created with the keys of the cards it waits for (WithDependsOn). That is
// most of the design. An edge may only name a card that already exists, so a card never waits on
// something created after it, and a chain of edges always walks backwards through creation order and
// therefore ends - no cycle is possible, and nothing has to check for one. It is also why there is no
// call that edits an existing card's dependencies: adding that would be adding the one operation
// that could make a cycle, and the Orchestrator's own flow - create a card, read the key it answers
// with, name it in the next card - never needs one.
//
// # What an edge does
//
// Nothing is enforced from these rows yet. A card whose dependency is unfinished is not held back by
// the board, and finishing a card does not release the cards behind it. What an edge does today is
// say what the plan is: the Orchestrator's board_status answer carries each card's dependencies, and
// the awareness summary a card's agent is given each turn lists them. Making the board act on them
// belongs to the slice that gates starting a card on its dependencies, and naming that here is
// deliberate - it is the difference between "this is the plan" and "this is the plan and following
// it is enforced".

// WithDependsOn records the cards a new card waits for, by key. Each key must name a card that
// already exists in the same project; a key that does not refuses the whole create rather than
// writing part of a plan.
func WithDependsOn(keys []protocol.CardKey) CardOption {
	return func(c *cardConfig) { c.dependsOn = append([]protocol.CardKey(nil), keys...) }
}

// insertCardLinks writes the edges a new card's dependencies name, inside the caller's transaction.
// Each key is resolved to the card's id within the same project, so a key naming a card in another
// project - or no card at all - refuses the create. Repeating a key is not an error: the edge is the
// same edge, and the insert leaves the one row it already wrote.
func insertCardLinks(ctx context.Context, q *db.Queries, projectID, cardID string, keys []protocol.CardKey) error {
	for _, key := range keys {
		if key.ProjectID != projectID {
			return protocol.InvalidArgument(fmt.Sprintf("Card %s is not in this project.", key)).
				With("dependsOn", key.String())
		}
		if key.Number < 1 {
			return protocol.InvalidArgument("A card that is depended on must have a number of 1 or more.").
				With("dependsOn", key.String())
		}
		dep, err := q.GetCardByKey(ctx, db.GetCardByKeyParams{ProjectID: projectID, Number: int64(key.Number)})
		if err != nil {
			return notFound(
				fmt.Errorf("read the card %s depends on: %w", key, err),
				notFoundCard(key.String()),
			)
		}
		if dep.ID == cardID {
			return protocol.InvalidArgument("A card cannot depend on itself.").
				With("dependsOn", key.String())
		}
		if err := q.InsertCardLink(ctx, db.InsertCardLinkParams{CardID: cardID, DependsOnID: dep.ID}); err != nil {
			return fmt.Errorf("record that card %s depends on %s: %w", cardID, key, err)
		}
	}
	return nil
}

// CardDependencies answers the keys of the cards one card waits for, in number order. A card that
// waits for nothing answers an empty list rather than null, so a client never has to tell the two
// apart. A card that is not there comes back as the projects service's own not-found error.
func (s *Service) CardDependencies(ctx context.Context, cardID string) ([]protocol.CardKey, error) {
	card, err := s.Card(ctx, cardID)
	if err != nil {
		return nil, err
	}
	numbers, err := s.store.Queries().ListCardDependencyNumbers(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("read what card %s waits for: %w", cardID, err)
	}
	return cardKeysOf(card.ProjectID, numbers), nil
}

// ProjectDependencies answers every edge in one project, grouped by the card that waits and named by
// the keys of the cards it waits for. It is one query for the whole board, because the board summary
// and the per-turn awareness summary both read every card's dependencies at once, and a query per
// card would make a busy board's summary cost grow with the board.
func (s *Service) ProjectDependencies(ctx context.Context, projectID string) (map[string][]protocol.CardKey, error) {
	rows, err := s.store.Queries().ListProjectDependencies(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the dependencies of project %s: %w", projectID, err)
	}
	out := make(map[string][]protocol.CardKey, len(rows))
	for _, row := range rows {
		out[row.CardID] = append(out[row.CardID],
			protocol.CardKey{ProjectID: projectID, Number: int(row.DependsOnNumber)})
	}
	return out, nil
}

// cardKeysOf builds the keys of a project's cards from their numbers, in the order the query
// answered with them.
func cardKeysOf(projectID string, numbers []int64) []protocol.CardKey {
	out := make([]protocol.CardKey, 0, len(numbers))
	for _, number := range numbers {
		out = append(out, protocol.CardKey{ProjectID: projectID, Number: int(number)})
	}
	return out
}
