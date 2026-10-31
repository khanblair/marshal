package integrations_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The refresh of a linked Trello board (docs/marshal-product-scope.md section 19.2): Trello into
// Marshal, never the other way. The setup is in trello_harness_test.go.

func TestRefreshImportsUnseenTrelloCardsAsIdleCards(t *testing.T) {
	e := newEngine(t)
	seedFixLogin(e.trello)
	e.trello.addCard("tc-2", "list-done", "Ship v1", "")
	e.trello.addCard("tc-3", "list-doing", "Write docs", "Needs examples")

	counts := e.refresh()

	if counts.Imported != 3 || counts.Updated != 0 {
		t.Fatalf("counts = %+v, want 3 imported and 0 updated", counts)
	}
	fix, shipped, docs := e.board.get(e.marshalCard("tc-1")), e.board.get(e.marshalCard("tc-2")), e.board.get(e.marshalCard("tc-3"))
	if fix.Title != "Fix login" || fix.Body != "Users cannot log in" || docs.Body != "Needs examples" {
		t.Errorf("imported cards = %+v and %+v, want Trello's names and descriptions", fix, docs)
	}
	// Idle: waiting in the backlog for the person, never started. Only a card in a list that reads
	// as done lands in done.
	if fix.State != protocol.CardStateBacklog || docs.State != protocol.CardStateBacklog {
		t.Errorf("states = %q and %q, want both in the backlog", fix.State, docs.State)
	}
	if shipped.State != protocol.CardStateDone {
		t.Errorf("a card on the Done list is %q, want done", shipped.State)
	}
	if calls := e.board.callList(); len(calls) != 1 || !strings.HasPrefix(calls[0], "move "+shipped.ID) {
		t.Errorf("the board was asked to %q, want only the one move to done", calls)
	}
	if writes := e.trello.writes(); len(writes) != 0 {
		t.Errorf("reading the board wrote to Trello: %+v", writes)
	}
	if got := <-e.notified; got != engineProject {
		t.Errorf("notified project %q, want %q", got, engineProject)
	}
}

func TestRefreshCopiesChecklistsAndTicksAndCommentsWithoutWakingAnAgent(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()

	lists := e.checklists(cardID)
	if len(lists) != 1 || lists[0].Name != "Steps" || len(lists[0].Items) != 2 {
		t.Fatalf("checklists = %+v, want Steps with two lines", lists)
	}
	reproduce, test := lists[0].Items[0], lists[0].Items[1]
	if reproduce.Text != "reproduce" || reproduce.Done || test.Text != "write test" || !test.Done {
		t.Errorf("lines = %+v and %+v, want reproduce open and write test ticked", reproduce, test)
	}
	if test.DoneByKind != "person" || test.DoneByID != engineOwner {
		t.Errorf("the tick reads %q %q, want the owner as a person", test.DoneByKind, test.DoneByID)
	}

	comments := e.comments(cardID)
	if len(comments) != 2 {
		t.Fatalf("comments = %+v, want two", comments)
	}
	wantBodies := []string{"Ada Lovelace on Trello: Looks like a cookie issue", "Grace Hopper on Trello: Same here"}
	for i, c := range comments {
		if c.Body != wantBodies[i] {
			t.Errorf("comment %d reads %q, want %q", i, c.Body, wantBodies[i])
		}
		if c.AuthorKind != protocol.AuthorKindPerson || c.AuthorID != engineOwner {
			t.Errorf("comment %d is by %q %q, want the owner as a person", i, c.AuthorKind, c.AuthorID)
		}
		if c.AgentReadAt != nil {
			t.Errorf("comment %d was read by the agent", i)
		}
	}
	if want := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC); !comments[0].CreatedAt.Time().Equal(want) {
		t.Errorf("the first comment is dated %v, want when it was written in Trello, %v", comments[0].CreatedAt.Time(), want)
	}
	if sent := e.agent.messages(); len(sent) != 0 {
		t.Errorf("imported comments reached the agent: %q", sent)
	}
}

func TestAnImportLeavesNothingPending(t *testing.T) {
	e := newEngine(t)
	seedFixLogin(e.trello)
	e.trello.addCard("tc-2", "list-done", "Ship v1", "")
	e.refresh()

	pending := e.pending()

	if !pending.Linked || pending.BoardName != "Sprint Board" || pending.FetchedAt == nil {
		t.Errorf("pending = %+v, want a linked board with its name and when it was read", pending)
	}
	if len(pending.Changes) != 0 {
		t.Errorf("a fresh import left pending changes: %+v", pending.Changes)
	}
	// A card imported into the done list is done and needs no move, so it is not pending either.
	if len(pending.Cards) != 2 || !pending.Cards[0].FromTrello || pending.Cards[0].ListName != "To do" {
		t.Errorf("linked cards = %+v, want the two imported cards on their lists", pending.Cards)
	}
	if pending.Cards[0].URL != "https://trello.com/c/tc-1" {
		t.Errorf("card url = %q, want the Trello card's address", pending.Cards[0].URL)
	}
}

func TestASecondRefreshChangesNothing(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.calls = nil

	counts := e.refresh()

	if counts.Imported != 0 || counts.Updated != 0 {
		t.Errorf("counts = %+v, want nothing imported or updated", counts)
	}
	if calls := e.board.callList(); len(calls) != 0 {
		t.Errorf("the board was asked to %q, want nothing", calls)
	}
	if got := len(e.board.order); got != 1 {
		t.Errorf("the board holds %d cards, want 1", got)
	}
	if lists := e.checklists(cardID); len(lists) != 1 || len(lists[0].Items) != 2 {
		t.Errorf("checklists = %+v, want the one list with its two lines, not copied again", lists)
	}
	if got := len(e.comments(cardID)); got != 2 {
		t.Errorf("the card has %d comments, want the 2 it was imported with", got)
	}
	if got := len(e.pending().Changes); got != 0 {
		t.Errorf("%d changes are pending after a refresh that changed nothing", got)
	}
}

func TestRefreshOfAProjectThatIsNotLinkedIsRefused(t *testing.T) {
	e := newEngine(t)

	_, err := e.f.svc.RefreshTrello(context.Background(), "other-repo", nil)

	if !errors.Is(err, integrations.ErrTrelloNotLinked) {
		t.Errorf("error = %v, want ErrTrelloNotLinked", err)
	}
	if got := e.trello.count("GET", "/boards/board-1"); got != 0 {
		t.Errorf("Trello was asked %d times about a project that is not linked", got)
	}
}

func TestRefreshWithNoConnectionIsRefused(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.RefreshTrello(context.Background(), engineProject, nil)

	if !errors.Is(err, integrations.ErrTrelloNotLinked) {
		t.Errorf("error = %v, want ErrTrelloNotLinked", err)
	}
}

func TestRefreshTakesOverATrelloRenameWhenMarshalDidNotChangeTheTitle(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.trello.edit("tc-1", func(c *trello.Card) { c.Name = "Fix login (urgent)"; c.Desc = "Safari only" })

	counts := e.refresh()

	if counts.Updated != 1 {
		t.Errorf("counts = %+v, want the card updated", counts)
	}
	card := e.board.get(cardID)
	if card.Title != "Fix login (urgent)" || card.Body != "Safari only" {
		t.Errorf("card = %q / %q, want Trello's new words", card.Title, card.Body)
	}
	if got := e.pending(); len(got.Changes) != 0 {
		t.Errorf("taking Trello's words left changes pending: %+v", got.Changes)
	}
}

func TestRefreshKeepsMarshalsTitleAndRecordsAConflictWhenBothChangedIt(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Fix login in Safari"), nil)
	e.trello.edit("tc-1", func(c *trello.Card) { c.Name = "Fix login (urgent)" })

	counts := e.refresh()

	if counts.Updated != 0 {
		t.Errorf("counts = %+v, want no card updated, because Marshal keeps its own", counts)
	}
	if title := e.board.get(cardID).Title; title != "Fix login in Safari" {
		t.Errorf("title = %q, want Marshal's own", title)
	}
	change := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	if !change.Conflict || !strings.Contains(change.ConflictNote, "Fix login (urgent)") {
		t.Errorf("change = %+v, want a conflict that says what Trello holds", change)
	}
}

func TestKeepTrelloCardIDsSetsTheCardBackToTrellosVersion(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Fix login in Safari"), nil)
	e.trello.edit("tc-1", func(c *trello.Card) { c.Name = "Fix login (urgent)" })
	e.refresh()
	if !e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID).Conflict {
		t.Fatal("the setup did not make a conflict")
	}

	counts := e.refresh(cardID)

	if counts.Updated != 1 {
		t.Errorf("counts = %+v, want the card updated", counts)
	}
	if title := e.board.get(cardID).Title; title != "Fix login (urgent)" {
		t.Errorf("title = %q, want Trello's", title)
	}
	if got := e.pending(); len(got.Changes) != 0 {
		t.Errorf("a card set back to Trello's version still has changes: %+v", got.Changes)
	}
}

func TestRefreshFollowsTrelloMovesIntoAndOutOfDone(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()

	e.trello.edit("tc-1", func(c *trello.Card) { c.IDList = "list-done" })
	e.refresh()
	if state := e.board.get(cardID).State; state != protocol.CardStateDone {
		t.Fatalf("state = %q after Trello moved the card to Done, want done", state)
	}
	if got := e.pending(); len(got.Changes) != 0 || got.Cards[0].ListName != "Done" {
		t.Errorf("pending = %+v, want nothing to send and the list noted as Done", got)
	}

	e.trello.edit("tc-1", func(c *trello.Card) { c.IDList = "list-doing" })
	e.refresh()
	if state := e.board.get(cardID).State; state != protocol.CardStateBacklog {
		t.Errorf("state = %q after Trello moved the card out of Done, want the backlog", state)
	}
}

func TestRefreshKeepsADoneMarshalCardAndNotesWhereTrelloPutIt(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.setState(cardID, protocol.CardStateDone)
	e.trello.edit("tc-1", func(c *trello.Card) { c.IDList = "list-doing" })

	e.refresh()

	if state := e.board.get(cardID).State; state != protocol.CardStateDone {
		t.Errorf("state = %q, want Marshal's own done kept", state)
	}
	change := e.changeOf(e.pending(), protocol.TrelloChangeKindMoveDone, cardID)
	if !change.Conflict || !strings.Contains(change.ConflictNote, "Doing") {
		t.Errorf("change = %+v, want a conflict that names the list Trello put the card on", change)
	}
}

func TestKeepTrelloCardIDsPutsADoneMarshalCardWhereTrelloPutIt(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.setState(cardID, protocol.CardStateDone)
	e.trello.edit("tc-1", func(c *trello.Card) { c.IDList = "list-doing" })
	e.refresh()

	counts := e.refresh(cardID)

	if counts.Updated != 1 {
		t.Errorf("counts = %+v, want the card updated", counts)
	}
	if state := e.board.get(cardID).State; state != protocol.CardStateBacklog {
		t.Errorf("state = %q, want the card out of done, as Trello has it", state)
	}
	if got := e.pending(); len(got.Changes) != 0 || got.Cards[0].ListName != "Doing" {
		t.Errorf("pending = %+v, want nothing to send and the list noted as Doing", got)
	}
}

func TestRefreshCopiesWhatTrelloAddedAfterTheImport(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.trello.comment("tc-1", "Linus", "A late comment")
	e.trello.edit("tc-1", func(c *trello.Card) {
		c.Checklists[0].Items = append(c.Checklists[0].Items, checkItem("tci-3", "deploy", false))
	})
	e.trello.tick("tc-1", "tci-1", true)
	e.trello.addChecklist("tc-1", "tcl-2", "Release", checkItem("tci-4", "tag it", true))

	counts := e.refresh()

	if counts.Updated != 1 {
		t.Errorf("counts = %+v, want the card updated", counts)
	}
	lists := e.checklists(cardID)
	if len(lists) != 2 || len(lists[0].Items) != 3 || lists[0].Items[2].Text != "deploy" {
		t.Fatalf("checklists = %+v, want the new line and the new list copied", lists)
	}
	if !lists[0].Items[0].Done {
		t.Error("the line Trello ticked is still open in Marshal")
	}
	if lists[1].Name != "Release" || len(lists[1].Items) != 1 || !lists[1].Items[0].Done {
		t.Errorf("second list = %+v, want Release with its ticked line", lists[1])
	}
	comments := e.comments(cardID)
	if len(comments) != 3 || comments[2].Body != "Linus on Trello: A late comment" {
		t.Errorf("comments = %+v, want the late comment added once", comments)
	}
	if got := e.pending(); len(got.Changes) != 0 {
		t.Errorf("what came from Trello is pending to go back: %+v", got.Changes)
	}
	if sent := e.agent.messages(); len(sent) != 0 {
		t.Errorf("the agent was sent %q", sent)
	}
}

func TestRefreshDoesNotTakeTrellosTickOverAMarshalTick(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	list := e.checklists(cardID)[0]
	// Marshal reopens the ticked line, which Trello does not know. Trello reopens it as well.
	e.tick(cardID, list.ID, list.Items[1].ID, false)
	e.trello.tick("tc-1", "tci-2", false)

	e.refresh()

	if e.checklists(cardID)[0].Items[1].Done {
		t.Error("the line is ticked in Marshal")
	}
	if got := e.pending(); len(got.Changes) != 0 {
		t.Errorf("both sides agree now, but %+v is pending", got.Changes)
	}
}
