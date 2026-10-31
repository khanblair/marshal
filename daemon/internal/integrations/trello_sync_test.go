package integrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The Trello webhook side of the sync (docs/marshal-product-scope.md section 19.2). A believed delivery
// changes nothing by itself: it asks the poller for a read, or it is ignored. Reading the board, and
// everything that follows from it, is covered in trello_engine_test.go.

// linkedFixture is a connections service with a Trello board linked to the project small-repo, the way
// Settings saves one.
func linkedFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	if err := f.svc.SaveTrello(context.Background(), trelloSave()); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	return f
}

// deliveryFor is a delivery Trello sends for a card change, naming the board in the action.
func deliveryFor(boardID string) trello.WebhookEvent {
	var event trello.WebhookEvent
	event.Action.Type = trello.ActionUpdateCard
	event.Action.ID = "action-1"
	event.Action.Data.Board.ID = boardID
	event.Action.Data.Card = trello.Card{ID: "tc-1", Name: "A card"}
	return event
}

func TestADeliveryForTheLinkedBoardAsksForARefresh(t *testing.T) {
	f := linkedFixture(t)
	if _, err := integrations.NewTrelloPoller(f.svc); err != nil {
		t.Fatalf("NewTrelloPoller: %v", err)
	}
	byModel := trello.WebhookEvent{}
	byModel.Model.ID = fakeBoardID

	tests := []struct {
		name  string
		event trello.WebhookEvent
	}{
		{"a board named in the action", deliveryFor(fakeBoardID)},
		{"a board named as the model", byModel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, err := f.svc.HandleTrelloDelivery(context.Background(), tt.event)
			if err != nil {
				t.Fatalf("HandleTrelloDelivery: %v", err)
			}
			if outcome != integrations.TrelloOutcomeRefresh {
				t.Errorf("outcome = %q, want refresh", outcome)
			}
		})
	}
}

func TestADeliveryThatIsNotForTheLinkedBoardIsIgnored(t *testing.T) {
	f := linkedFixture(t)
	if _, err := integrations.NewTrelloPoller(f.svc); err != nil {
		t.Fatalf("NewTrelloPoller: %v", err)
	}

	tests := []struct {
		name  string
		event trello.WebhookEvent
	}{
		{"another board", deliveryFor("board-other")},
		{"no board named", deliveryFor("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, err := f.svc.HandleTrelloDelivery(context.Background(), tt.event)
			if err != nil {
				t.Fatalf("HandleTrelloDelivery: %v", err)
			}
			if outcome != integrations.TrelloOutcomeIgnored {
				t.Errorf("outcome = %q, want ignored", outcome)
			}
		})
	}
}

func TestADeliveryWithNoLinkIsIgnored(t *testing.T) {
	f := newFixture(t)
	if _, err := integrations.NewTrelloPoller(f.svc); err != nil {
		t.Fatalf("NewTrelloPoller: %v", err)
	}

	outcome, err := f.svc.HandleTrelloDelivery(context.Background(), deliveryFor(fakeBoardID))

	if err != nil || outcome != integrations.TrelloOutcomeIgnored {
		t.Errorf("HandleTrelloDelivery = %q, %v, want ignored with no error", outcome, err)
	}
}

// With no poller registered a delivery is believed and logged, and nothing else: the honest state of a
// daemon whose poller is not built, and the reason a delivery never fails the request.
func TestADeliveryWithNoPollerIsIgnored(t *testing.T) {
	f := linkedFixture(t)

	outcome, err := f.svc.HandleTrelloDelivery(context.Background(), deliveryFor(fakeBoardID))

	if err != nil || outcome != integrations.TrelloOutcomeIgnored {
		t.Errorf("HandleTrelloDelivery = %q, %v, want ignored with no error", outcome, err)
	}
}

func TestADeliveryNeitherReadsNorWritesAnythingByItself(t *testing.T) {
	e := newEngine(t)
	seedFixLogin(e.trello)
	if _, err := integrations.NewTrelloPoller(e.f.svc); err != nil {
		t.Fatalf("NewTrelloPoller: %v", err)
	}

	outcome, err := e.f.svc.HandleTrelloDelivery(context.Background(), deliveryFor(fakeBoardID))

	if err != nil || outcome != integrations.TrelloOutcomeRefresh {
		t.Fatalf("HandleTrelloDelivery = %q, %v, want refresh", outcome, err)
	}
	if got := len(e.board.order); got != 0 {
		t.Errorf("the delivery made %d cards, want none: the poller has not run", got)
	}
	if got := e.trello.count("GET", "/boards/board-1"); got != 0 {
		t.Errorf("the delivery read the board %d times by itself", got)
	}
	if writes := e.trello.writes(); len(writes) != 0 {
		t.Errorf("the delivery wrote to Trello: %+v", writes)
	}
}

// seedCardRow puts a project, a board, and one card in the fixture's store, so a link has a card to
// point at. It is the smallest row set that satisfies the schema's foreign keys; nothing here reads
// the card back.
func seedCardRow(t *testing.T, f *fixture, cardID string) {
	t.Helper()
	ctx := context.Background()
	projectID := "p-" + cardID
	now := time.Now().UnixMilli()
	err := f.store.Write(ctx, func(q *db.Queries) error {
		if err := q.CreateProject(ctx, db.CreateProjectParams{
			ID: projectID, Name: projectID, RepoPath: "/code/" + projectID, DefaultBranch: "main",
			PackagesJSON: "[]", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		if err := q.CreateBoard(ctx, db.CreateBoardParams{
			ID: "board-" + projectID, ProjectID: projectID,
			ColumnsJSON: `["backlog","working","needs","done"]`,
		}); err != nil {
			return err
		}
		return q.CreateCard(ctx, db.CreateCardParams{
			ID: cardID, ProjectID: projectID, Number: 1, BoardID: "board-" + projectID,
			Title: "Card", State: string(protocol.CardStateBacklog), AgentKind: "claude",
			PermissionMode: "auto-edits", CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("seed the card %s: %v", cardID, err)
	}
}

// TestTrelloLinkNeedsAProject proves the connection refuses to exist without the project it writes
// into: one project links to one board, and a board with nowhere to put an imported card is a
// connection the sync could only ever ignore.
func TestTrelloLinkNeedsAProject(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	noProject := trelloSave()
	noProject.ProjectID = ""
	if err := f.svc.SaveTrello(ctx, noProject); err == nil {
		t.Error("a connection with no project was accepted")
	}
	badProject := trelloSave()
	badProject.ProjectID = "Not A Project Id"
	if err := f.svc.SaveTrello(ctx, badProject); err == nil {
		t.Error("a connection with a malformed project id was accepted")
	}

	// Before anything is saved there is no link at all, which is not an error.
	if _, ok, err := f.svc.TrelloLink(ctx); err != nil || ok {
		t.Errorf("TrelloLink on an unlinked daemon = %v, %v, want no link and no error", ok, err)
	}

	save := trelloSave()
	save.ProjectID = "small-repo"
	save.NewCardListID = "list-new"
	if err := f.svc.SaveTrello(ctx, save); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	link, ok, err := f.svc.TrelloLink(ctx)
	if err != nil || !ok {
		t.Fatalf("TrelloLink = %v, %v, want a link", ok, err)
	}
	if link.ProjectID != "small-repo" || link.BoardID != "board-1" || link.NewCardListID != "list-new" {
		t.Errorf("TrelloLink = %+v, want what was saved", link)
	}
}

// TestAnOutsideItemBelongsToOneCard proves the reverse index does its job: a second Marshal card
// cannot claim a Trello card that is already linked, which is what stops a duplicate delivery racing
// the first one from making two cards.
func TestAnOutsideItemBelongsToOneCard(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	first := "01H1234567890ABCDEFGHJKMNP"
	second := "01H1234567890ABCDEFGHJKMNPQ"
	seedCardRow(t, f, first)
	seedCardRow(t, f, second)
	if err := f.svc.LinkExternal(ctx, first, trello.ID, "trello-card-1"); err != nil {
		t.Fatalf("LinkExternal: %v", err)
	}
	if err := f.svc.LinkExternal(ctx, second, trello.ID, "trello-card-1"); err == nil {
		t.Error("a second card claimed a Trello card that was already linked")
	}
	// Linking the same card to a different item replaces its link rather than adding one: one card
	// has at most one item per kind.
	if err := f.svc.LinkExternal(ctx, first, trello.ID, "trello-card-2"); err != nil {
		t.Fatalf("relinking the same card: %v", err)
	}
	cardID, ok, err := f.svc.ExternalCard(ctx, trello.ID, "trello-card-2")
	if err != nil || !ok || cardID != first {
		t.Errorf("ExternalCard after a relink = %q, %v, %v, want the same card", cardID, ok, err)
	}
}
