package integrations

// This file is the Trello sync (docs/architecture.md section 19.4, docs/marshal-product-scope.md
// section 19.2, B8.2). Today it is the Trello-to-Marshal half of the card sync: a card added to the
// linked board's import list becomes a Marshal card, once, and the pair is remembered in
// `external_links` so a replayed delivery cannot make a second one.
//
// What is deliberately not here yet, and is named so it is not mistaken for done: moving a card
// between lists (Marshal to Trello and the other way), members, the agent's Trello label, and
// checklists, comments, and attachments. The last three are blocked on the board module not having
// those things at all - they arrive in Phase 10 - so a "syncs both ways" claim about them would be
// false today. See the phase report.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// TrelloCards is the board work the Trello sync needs: making a card, moving one, and reading one
// back. The projects service implements all three, and cmd/marshald adapts it, so this package never
// depends on the board module and a test can hand in a recorder instead.
type TrelloCards interface {
	// CreateCard adds a card to a project in the backlog.
	CreateCard(ctx context.Context, projectID string, in protocol.CreateCardRequest) (protocol.Card, error)
	// MoveCard moves a card to another column.
	MoveCard(ctx context.Context, id string, in protocol.MoveCardRequest) (protocol.Card, error)
	// Card reads one card back.
	Card(ctx context.Context, id string) (protocol.Card, error)
}

// TrelloOutcome says what one verified Trello delivery did. It is what the route logs and what a test
// reads instead of guessing from the board, so "a delivery arrived and nothing happened" is a thing
// the daemon can say out loud.
type TrelloOutcome string

const (
	// TrelloOutcomeIgnored is a delivery that was believed and means nothing to the sync: an action
	// type Marshal does not act on, a card on another board, a card on a list that is not the import
	// list, or a daemon with no board module attached.
	TrelloOutcomeIgnored TrelloOutcome = "ignored"
	// TrelloOutcomeImported is a delivery that made a new Marshal card, and linked it.
	TrelloOutcomeImported TrelloOutcome = "imported"
	// TrelloOutcomeKnown is a delivery for a card Marshal already has, which a replay or a repeat
	// arrives as. Nothing is written.
	TrelloOutcomeKnown TrelloOutcome = "known"
	// TrelloOutcomeMoved is a delivery that moved a linked card to Marshal's done column.
	TrelloOutcomeMoved TrelloOutcome = "moved"
)

// doneListWords are the words a Trello list's name is checked against to recognize it as the
// board's "done" column (the owner's ruling: assume standard names - no per-board setting screen
// this pass).
func doneListWords() []string { return []string{"done", "complete", "completed", "finished"} }

// looksDone reports whether a Trello list's name reads as a "done" column.
func looksDone(listName string) bool {
	lower := strings.ToLower(listName)
	for _, word := range doneListWords() {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// doneListOf finds the first list on a board whose name reads as done, for the outbound sync
// (trello_outbound.go) to move a card into.
func doneListOf(lists []trello.List) (trello.List, bool) {
	for _, list := range lists {
		if looksDone(list.Name) {
			return list, true
		}
	}
	return trello.List{}, false
}

// SetTrelloCards attaches the board operations a verified Trello delivery is applied through. It is
// called while the daemon starts, before the route is served. Until it is called, a delivery that is
// believed is logged and nothing else - the honest state of a daemon whose board module is not built
// yet, and the reason a delivery never fails a request.
func (s *Service) SetTrelloCards(cards TrelloCards) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trelloCards = cards
}

// HandleTrelloDelivery applies one verified Trello delivery and answers what it did. Nothing here is
// an error a delivery should be refused for: a body Marshal cannot act on is an outcome, not a
// failure, because the delivery is already believed by the time this runs and refusing it would only
// make Trello redeliver it.
//
// The one error it can answer is its own store or board module failing, which the route turns into
// this daemon's failure and which Trello redelivering will retry.
func (s *Service) HandleTrelloDelivery(ctx context.Context, event trello.WebhookEvent) (TrelloOutcome, error) {
	if list, moved := event.Moved(); moved {
		return s.handleTrelloMove(ctx, event, list)
	}
	if event.Action.Type != trello.ActionCreateCard {
		return TrelloOutcomeIgnored, nil
	}
	card := event.Action.Data.Card
	if card.ID == "" {
		return TrelloOutcomeIgnored, nil
	}
	link, ok, err := s.TrelloLink(ctx)
	if err != nil {
		return "", err
	}
	if !ok || link.NewCardListID == "" {
		// A connection that is not linked to a project, or has no import list, reads Trello and
		// never writes to it. That is a supported state, not a misconfiguration.
		return TrelloOutcomeIgnored, nil
	}
	if !deliveryMatchesBoard(event, link.BoardID) || card.IDList != link.NewCardListID {
		return TrelloOutcomeIgnored, nil
	}
	// The pair is remembered in external_links, so a replay - which Trello does send - is recognized
	// instead of making a second Marshal card.
	if _, known, err := s.ExternalCard(ctx, trello.ID, card.ID); err != nil {
		return "", err
	} else if known {
		return TrelloOutcomeKnown, nil
	}
	cards := s.attachedTrelloCards()
	if cards == nil {
		s.log.Info("a Trello delivery was believed but no board module is attached, so nothing was imported",
			"card", card.ID)
		return TrelloOutcomeIgnored, nil
	}
	created, err := cards.CreateCard(ctx, link.ProjectID, protocol.CreateCardRequest{
		Title: card.Name,
		Body:  card.Desc,
	})
	if err != nil {
		return "", fmt.Errorf("import the Trello card %s: %w", card.ID, err)
	}
	if err := s.LinkExternal(ctx, created.ID, trello.ID, card.ID); err != nil {
		return "", err
	}
	s.log.Info("imported a Trello card",
		"trello_card", card.ID, "card", created.ID, "project", link.ProjectID)
	return TrelloOutcomeImported, nil
}

// handleTrelloMove applies a card moved into list, when the card is already linked and list looks
// like the board's done column (looksDone). Any other list is ignored: without a per-board setting
// (the same ruling as doneListWords), only the done move is one this pass acts on.
func (s *Service) handleTrelloMove(ctx context.Context, event trello.WebhookEvent, list trello.List) (TrelloOutcome, error) {
	if !looksDone(list.Name) {
		return TrelloOutcomeIgnored, nil
	}
	card := event.Action.Data.Card
	if card.ID == "" {
		return TrelloOutcomeIgnored, nil
	}
	link, ok, err := s.TrelloLink(ctx)
	if err != nil {
		return "", err
	}
	if !ok || !deliveryMatchesBoard(event, link.BoardID) {
		return TrelloOutcomeIgnored, nil
	}
	cardID, known, err := s.ExternalCard(ctx, trello.ID, card.ID)
	if err != nil {
		return "", err
	}
	if !known {
		return TrelloOutcomeIgnored, nil
	}
	cards := s.attachedTrelloCards()
	if cards == nil {
		s.log.Info("a Trello move was believed but no board module is attached, so nothing moved",
			"card", card.ID)
		return TrelloOutcomeIgnored, nil
	}
	if _, err := cards.MoveCard(ctx, cardID, protocol.MoveCardRequest{State: protocol.CardStateDone}); err != nil {
		return "", fmt.Errorf("move the card %s to done: %w", cardID, err)
	}
	s.log.Info("moved a card to done from Trello", "trello_card", card.ID, "card", cardID)
	return TrelloOutcomeMoved, nil
}

// deliveryMatchesBoard reports whether a delivery is about a board Marshal watches. Trello says which
// board twice - in `action.data.board` and in `model.id` - and different webhook scopes carry one or
// the other, so either naming the linked board is enough. A delivery that names no board at all is
// not matched, because acting on one would mean acting on a board Marshal cannot check.
func deliveryMatchesBoard(event trello.WebhookEvent, boardID string) bool {
	return event.Action.Data.Board.ID == boardID || event.Model.ID == boardID
}

// attachedTrelloCards reads the attached board operations under the lock. A delivery arrives on its
// own goroutine while the daemon starts, so the field is never read without it.
func (s *Service) attachedTrelloCards() TrelloCards {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trelloCards
}

// ExternalCard answers the Marshal card a provider's item is linked to: `kind` is the connection's
// kind ("trello") and `externalID` is that provider's own id for the item. It is how a delivery
// recognizes a card it has already imported.
func (s *Service) ExternalCard(ctx context.Context, kind, externalID string) (string, bool, error) {
	var cardID string
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		id, err := q.ExternalCard(ctx, db.ExternalCardParams{Kind: kind, ExternalID: externalID})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read the %s link for %s: %w", kind, externalID, err)
		}
		cardID, found = id, true
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return cardID, found, nil
}

// LinkExternal records that a Marshal card is a provider's item. A card has at most one item per
// kind (the table's primary key), so linking again replaces the item rather than adding a second, and
// the reverse index refuses an item that is already another card's - which is what a duplicate
// delivery racing the first one hits.
func (s *Service) LinkExternal(ctx context.Context, cardID, kind, externalID string) error {
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.LinkExternal(ctx, db.LinkExternalParams{
			CardID: cardID, Kind: kind, ExternalID: externalID,
		})
	})
	if err != nil {
		return fmt.Errorf("link the %s item %s to the card %s: %w", kind, externalID, cardID, err)
	}
	return nil
}
