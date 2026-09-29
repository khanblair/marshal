package integrations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The Trello sync's Trello-to-Marshal half (architecture.md section 19.4, B8.2): a card added to the
// linked board's import list becomes a Marshal card once, and the pair is remembered so a replayed
// delivery cannot make a second one.

// fakeCards is the board module a test hands the sync: it records what was made and answers a card
// with an id of its own, so nothing here needs a project on disk.
type fakeCards struct {
	projectID string
	created   []protocol.CreateCardRequest
	moved     []protocol.MoveCardRequest
	nextID    string
	fail      error
}

func (f *fakeCards) CreateCard(_ context.Context, projectID string, in protocol.CreateCardRequest) (protocol.Card, error) {
	if f.fail != nil {
		return protocol.Card{}, f.fail
	}
	f.projectID = projectID
	f.created = append(f.created, in)
	id := f.nextID
	if id == "" {
		id = "01H1234567890ABCDEFGHJKMNP"
	}
	return protocol.Card{ID: id, ProjectID: projectID, Title: in.Title, State: protocol.CardStateBacklog}, nil
}

func (f *fakeCards) MoveCard(_ context.Context, id string, in protocol.MoveCardRequest) (protocol.Card, error) {
	f.moved = append(f.moved, in)
	return protocol.Card{ID: id, State: in.State}, nil
}

func (f *fakeCards) Card(_ context.Context, id string) (protocol.Card, error) {
	return protocol.Card{ID: id}, nil
}

// linkedFixture is a connections service with a Trello board linked to a project and an import list,
// the way Settings saves one. It marks the test as needing no real Trello at all: the delivery is
// handed in directly, so there is no client and no server. The card the fake board answers with is a
// real row in the fixture's store, because a link points at a card and the table says so.
// linkedCardID is the real card row linkedFixture seeds, for a test that links it to a Trello card.
const linkedCardID = "01H1234567890ABCDEFGHJKMNP"

func linkedFixture(t *testing.T) (*fixture, *fakeCards) {
	t.Helper()
	f := newFixture(t)
	seedCardRow(t, f, linkedCardID)
	save := trelloSave()
	save.ProjectID = "small-repo"
	save.NewCardListID = "list-new"
	if err := f.svc.SaveTrello(context.Background(), save); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	cards := &fakeCards{nextID: linkedCardID}
	f.svc.SetTrelloCards(cards)
	return f, cards
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

// trelloCreateEvent is the delivery Trello sends when a card is added to a list.
func trelloCreateEvent(boardID, listID, cardID, name, desc string) trello.WebhookEvent {
	var event trello.WebhookEvent
	event.Action.Type = trello.ActionCreateCard
	event.Action.ID = "action-1"
	event.Action.Data.Board.ID = boardID
	event.Action.Data.Card = trello.Card{ID: cardID, Name: name, Desc: desc, IDList: listID}
	event.Model.ID = boardID
	return event
}

// trelloMoveEvent is the delivery Trello sends when a card moves to another list.
func trelloMoveEvent(boardID, cardID, listID, listName string) trello.WebhookEvent {
	var event trello.WebhookEvent
	event.Action.Type = "updateCard"
	event.Action.ID = "action-2"
	event.Action.Data.Board.ID = boardID
	event.Action.Data.Card = trello.Card{ID: cardID, IDList: listID}
	event.Action.Data.ListAfter = &trello.List{ID: listID, Name: listName}
	event.Model.ID = boardID
	return event
}

// TestTrelloDeliveryMovesALinkedCardToDone covers handleTrelloMove: a linked card moved to a list
// whose name reads as "done" moves Marshal's own card there too.
func TestTrelloDeliveryMovesALinkedCardToDone(t *testing.T) {
	f, cards := linkedFixture(t)
	ctx := context.Background()
	if err := f.svc.LinkExternal(ctx, linkedCardID, trello.ID, "trello-card-9"); err != nil {
		t.Fatalf("LinkExternal: %v", err)
	}

	outcome, err := f.svc.HandleTrelloDelivery(ctx, trelloMoveEvent("board-1", "trello-card-9", "list-done", "Done"))
	if err != nil {
		t.Fatalf("HandleTrelloDelivery: %v", err)
	}
	if outcome != integrations.TrelloOutcomeMoved {
		t.Fatalf("outcome = %q, want moved", outcome)
	}
	if len(cards.moved) != 1 || cards.moved[0].State != protocol.CardStateDone {
		t.Fatalf("moved = %+v, want one move to done", cards.moved)
	}
}

// TestTrelloDeliveryIgnoresAMoveToAnOrdinaryList covers looksDone's other side: a list whose name
// does not read as "done" is not acted on.
func TestTrelloDeliveryIgnoresAMoveToAnOrdinaryList(t *testing.T) {
	f, cards := linkedFixture(t)
	ctx := context.Background()
	if err := f.svc.LinkExternal(ctx, linkedCardID, trello.ID, "trello-card-9"); err != nil {
		t.Fatalf("LinkExternal: %v", err)
	}

	outcome, err := f.svc.HandleTrelloDelivery(ctx, trelloMoveEvent("board-1", "trello-card-9", "list-doing", "Doing"))
	if err != nil {
		t.Fatalf("HandleTrelloDelivery: %v", err)
	}
	if outcome != integrations.TrelloOutcomeIgnored {
		t.Errorf("outcome = %q, want ignored", outcome)
	}
	if len(cards.moved) != 0 {
		t.Errorf("moved = %+v, want none", cards.moved)
	}
}

// TestTrelloDeliveryIgnoresAMoveOfAnUnlinkedCard covers a move for a Trello card Marshal never
// imported: nothing to move, so nothing is moved.
func TestTrelloDeliveryIgnoresAMoveOfAnUnlinkedCard(t *testing.T) {
	f, cards := linkedFixture(t)
	ctx := context.Background()

	outcome, err := f.svc.HandleTrelloDelivery(ctx, trelloMoveEvent("board-1", "trello-card-unknown", "list-done", "Done"))
	if err != nil {
		t.Fatalf("HandleTrelloDelivery: %v", err)
	}
	if outcome != integrations.TrelloOutcomeIgnored {
		t.Errorf("outcome = %q, want ignored", outcome)
	}
	if len(cards.moved) != 0 {
		t.Errorf("moved = %+v, want none", cards.moved)
	}
}

func TestTrelloDeliveryImportsACardOnce(t *testing.T) {
	f, cards := linkedFixture(t)
	ctx := context.Background()
	event := trelloCreateEvent("board-1", "list-new", "trello-card-9", "Fix the flaky test", "It fails one run in ten.")

	outcome, err := f.svc.HandleTrelloDelivery(ctx, event)
	if err != nil {
		t.Fatalf("HandleTrelloDelivery: %v", err)
	}
	if outcome != integrations.TrelloOutcomeImported {
		t.Fatalf("outcome = %q, want imported", outcome)
	}
	if len(cards.created) != 1 {
		t.Fatalf("the board got %d cards, want 1", len(cards.created))
	}
	if cards.projectID != "small-repo" {
		t.Errorf("the card was made in project %q, want the linked one", cards.projectID)
	}
	if got := cards.created[0]; got.Title != "Fix the flaky test" || got.Body != "It fails one run in ten." {
		t.Errorf("the imported card is %+v, want the Trello card's name and description", got)
	}

	// The pair is remembered, which is what makes the next delivery a no-op.
	cardID, ok, err := f.svc.ExternalCard(ctx, trello.ID, "trello-card-9")
	if err != nil {
		t.Fatalf("ExternalCard: %v", err)
	}
	if !ok || cardID != "01H1234567890ABCDEFGHJKMNP" {
		t.Errorf("ExternalCard = %q, %v, want the new card", cardID, ok)
	}

	// Trello redelivers, so the same event must not make a second card.
	outcome, err = f.svc.HandleTrelloDelivery(ctx, event)
	if err != nil {
		t.Fatalf("HandleTrelloDelivery (replay): %v", err)
	}
	if outcome != integrations.TrelloOutcomeKnown {
		t.Errorf("a replayed delivery = %q, want known", outcome)
	}
	if len(cards.created) != 1 {
		t.Errorf("a replay made a second card: %+v", cards.created)
	}
}

// TestTrelloDeliveryIgnoresWhatIsNotIts proves the deliveries the sync is not for are outcomes rather
// than cards: another board, another list, an action type Marshal does not act on, and a card with no
// id.
func TestTrelloDeliveryIgnoresWhatIsNotIts(t *testing.T) {
	f, cards := linkedFixture(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		event trello.WebhookEvent
	}{
		{"another board", trelloCreateEvent("board-other", "list-new", "c1", "x", "")},
		{"another list", trelloCreateEvent("board-1", "list-other", "c2", "x", "")},
		{"no board named", trelloCreateEvent("", "list-new", "c3", "x", "")},
		{"no card id", trelloCreateEvent("board-1", "list-new", "", "x", "")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, err := f.svc.HandleTrelloDelivery(ctx, tt.event)
			if err != nil {
				t.Fatalf("HandleTrelloDelivery: %v", err)
			}
			if outcome != integrations.TrelloOutcomeIgnored {
				t.Errorf("outcome = %q, want ignored", outcome)
			}
		})
	}

	var update trello.WebhookEvent
	update.Action.Type = trello.ActionUpdateCard
	update.Model.ID = "board-1"
	if outcome, err := f.svc.HandleTrelloDelivery(ctx, update); err != nil || outcome != integrations.TrelloOutcomeIgnored {
		t.Errorf("an update delivery = %q, %v, want ignored", outcome, err)
	}

	if len(cards.created) != 0 {
		t.Errorf("a delivery that was not for Marshal made cards: %+v", cards.created)
	}
}

// TestTrelloDeliveryWithNoBoardModuleImportsNothing proves a believed delivery on a daemon with no
// board module attached is named as ignored rather than failing the delivery: the route must answer
// Trello, or Trello sends it again forever.
func TestTrelloDeliveryWithNoBoardModuleImportsNothing(t *testing.T) {
	f := newFixture(t)
	save := trelloSave()
	save.ProjectID = "small-repo"
	save.NewCardListID = "list-new"
	if err := f.svc.SaveTrello(context.Background(), save); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	outcome, err := f.svc.HandleTrelloDelivery(context.Background(),
		trelloCreateEvent("board-1", "list-new", "c1", "x", ""))
	if err != nil {
		t.Fatalf("HandleTrelloDelivery: %v", err)
	}
	if outcome != integrations.TrelloOutcomeIgnored {
		t.Errorf("outcome = %q, want ignored", outcome)
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

// TestABoardModuleThatFailsIsAnError proves a real failure while writing a card is the daemon's own
// failure, which Trello redelivering will retry, rather than a silently ignored delivery.
func TestABoardModuleThatFailsIsAnError(t *testing.T) {
	f, cards := linkedFixture(t)
	cards.fail = errors.New("the store is not there")
	_, err := f.svc.HandleTrelloDelivery(context.Background(),
		trelloCreateEvent("board-1", "list-new", "c1", "x", ""))
	if err == nil {
		t.Fatal("a failing board module was not an error")
	}
}
