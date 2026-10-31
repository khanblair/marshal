package integrations_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// What Marshal would change in Trello: worked out each time from the difference between a Marshal
// card and the snapshot of its Trello card, so it cannot drift from what the cards say.

// editEverything makes one of each kind of change on the imported card, and makes a card Trello has
// never heard of. The changes sit in the order the daemon lists them.
func editEverything(e *engine, cardID string) (newCardID string) {
	e.board.edit(cardID, ptr("Fix login in Safari"), nil)
	e.board.setState(cardID, protocol.CardStateDone)
	steps := e.checklists(cardID)[0]
	e.tick(cardID, steps.ID, steps.Items[0].ID, true)
	e.addLine(cardID, steps.ID, "deploy")
	e.addChecklist(cardID, "Empty")
	e.addChecklist(cardID, "Release", "tag it", "announce")
	e.say(cardID, "Patched the cookie")
	e.agentSays(cardID, "Tests are green")
	return e.board.add("Brand new", "Only in Marshal")
}

func TestPendingListsExactlyWhatMarshalChanged(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	newCardID := editEverything(e, cardID)

	pending := e.pending()

	want := []protocol.TrelloChangeKind{
		protocol.TrelloChangeKindUpdateCard, protocol.TrelloChangeKindMoveDone, protocol.TrelloChangeKindSetItem,
		protocol.TrelloChangeKindAddItem, protocol.TrelloChangeKindAddChecklist,
		protocol.TrelloChangeKindAddComment, protocol.TrelloChangeKindAddComment, protocol.TrelloChangeKindCreateCard,
	}
	if got := changeKinds(pending.Changes); !slices.Equal(got, want) {
		t.Fatalf("pending kinds = %v, want %v", got, want)
	}
	byKind := []string{"person", "person", "person", "", "", "person", "agent", ""}
	for i, c := range pending.Changes {
		if c.ByKind != byKind[i] {
			t.Errorf("change %d (%s) is by %q, want %q", i, c.Kind, c.ByKind, byKind[i])
		}
		if c.Conflict || c.ConflictNote != "" {
			t.Errorf("change %d (%s) is a conflict with nothing changed in Trello: %+v", i, c.Kind, c)
		}
	}
	seen := map[string]bool{}
	for _, c := range pending.Changes {
		if c.ID == "" || seen[c.ID] {
			t.Errorf("change ids are not unique and filled in: %+v", changeIDs(pending.Changes))
		}
		seen[c.ID] = true
	}
	if !pending.Linked || pending.Changes[7].CardID != newCardID || pending.Changes[0].CardTitle != "Fix login in Safari" {
		t.Errorf("pending = %+v, want the cards named by their Marshal title", pending.Changes)
	}
}

func TestPendingChangesSayWhatWouldBeWritten(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	editEverything(e, cardID)

	pending := e.pending()

	rename := e.changeOf(pending, protocol.TrelloChangeKindUpdateCard, cardID)
	if rename.Detail != "Fix login in Safari" || !strings.Contains(rename.Summary, "Rename") {
		t.Errorf("rename = %+v, want the new title as its detail", rename)
	}
	if move := e.changeOf(pending, protocol.TrelloChangeKindMoveDone, cardID); !strings.Contains(move.Summary, "Done") {
		t.Errorf("move = %+v, want it to say Done", move)
	}
	if tick := e.changeOf(pending, protocol.TrelloChangeKindSetItem, cardID); !strings.Contains(tick.Summary, "Tick") || !strings.Contains(tick.Summary, "reproduce") {
		t.Errorf("tick = %+v, want it to name the line it ticks", tick)
	}
	if add := e.changeOf(pending, protocol.TrelloChangeKindAddItem, cardID); !strings.Contains(add.Summary, "deploy") {
		t.Errorf("new line = %+v, want it to name the line", add)
	}
	list := e.changeOf(pending, protocol.TrelloChangeKindAddChecklist, cardID)
	if !strings.Contains(list.Summary, "Release") || !strings.Contains(list.Summary, "2") || list.Detail != "- tag it\n- announce" {
		t.Errorf("new list = %+v, want its name, its two lines, and the lines as detail", list)
	}
	var comments []string
	for _, c := range pending.Changes {
		if c.Kind == protocol.TrelloChangeKindAddComment {
			comments = append(comments, c.Detail)
		}
	}
	if !slices.Equal(comments, []string{"Patched the cookie", "Tests are green"}) {
		t.Errorf("comments = %q, want the two Marshal wrote, with the text that would be posted", comments)
	}
}

func TestPendingNeverOffersACommentThatCameFromTrello(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.trello.comment("tc-1", "Linus", "Another one")
	e.refresh()
	if got := len(e.comments(cardID)); got != 3 {
		t.Fatalf("the card has %d comments, want the 3 imported", got)
	}

	if got := len(e.pending().Changes); got != 0 {
		t.Errorf("%d changes are pending, want none: every comment came from Trello", got)
	}

	e.say(cardID, "My own comment")
	pending := e.pending()
	if len(pending.Changes) != 1 || pending.Changes[0].Detail != "My own comment" {
		t.Errorf("pending = %+v, want only the comment Marshal wrote", pending.Changes)
	}
}

func TestPendingAddsAChecklistOnlyWhenItHasLines(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	listID, _ := e.addChecklist(cardID, "Later")

	if got := len(e.pending().Changes); got != 0 {
		t.Errorf("an empty checklist is pending: %+v", e.pending().Changes)
	}

	e.addLine(cardID, listID, "first line")
	pending := e.pending()
	if got := changeKinds(pending.Changes); !slices.Equal(got, []protocol.TrelloChangeKind{protocol.TrelloChangeKindAddChecklist}) {
		t.Errorf("pending kinds = %v, want one new checklist once it has a line", got)
	}
}

func TestPendingSaysReopenForAnItemMarshalOpenedAgain(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	steps := e.checklists(cardID)[0]
	e.tick(cardID, steps.ID, steps.Items[1].ID, false)

	change := e.changeOf(e.pending(), protocol.TrelloChangeKindSetItem, cardID)

	if !strings.Contains(change.Summary, "Reopen") || !strings.Contains(change.Summary, "write test") {
		t.Errorf("change = %+v, want it to reopen the line", change)
	}
}

func TestPendingForAChangedTitleOrDescription(t *testing.T) {
	tests := []struct {
		name    string
		title   *string
		body    *string
		summary string
		detail  string
	}{
		{"a new title", ptr("New title"), nil, "Rename", "New title"},
		{"a new description", nil, ptr("New words"), "description", "New words"},
		{"both", ptr("New title"), ptr("New words"), "title and description", "New title\n\nNew words"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEngine(t)
			cardID := e.importedFixLogin()
			e.board.edit(cardID, tt.title, tt.body)

			change := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)

			if !strings.Contains(change.Summary, tt.summary) || change.Detail != tt.detail {
				t.Errorf("change = %+v, want a summary with %q and detail %q", change, tt.summary, tt.detail)
			}
		})
	}
}

func TestPendingForACardMadeInMarshalIsToCreateItInTrello(t *testing.T) {
	e := newEngine(t)
	e.importedFixLogin()
	newCardID := e.board.add("Brand new", "Only in Marshal")

	change := e.changeOf(e.pending(), protocol.TrelloChangeKindCreateCard, newCardID)

	if change.Detail != "Only in Marshal" || change.CardTitle != "Brand new" {
		t.Errorf("change = %+v, want the card's title and description", change)
	}
}

func TestPendingIdsStayTheSameUntilAChangeChanges(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("First title"), nil)
	first := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID).ID

	again := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID).ID
	e.board.edit(cardID, ptr("Second title"), nil)
	edited := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID).ID

	if first != again {
		t.Errorf("the same change has ids %q and %q", first, again)
	}
	if edited == first {
		t.Error("an edited change kept its id")
	}
}

func TestDismissHidesAChangeAndAnEditedChangeShowsAgain(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Draft title"), nil)
	e.say(cardID, "A note for later")
	pending := e.pending()
	rename := e.changeOf(pending, protocol.TrelloChangeKindUpdateCard, cardID)
	for len(e.notified) > 0 {
		<-e.notified
	}

	if err := e.f.svc.DismissTrello(context.Background(), engineProject, []string{rename.ID, rename.ID}); err != nil {
		t.Fatalf("DismissTrello: %v", err)
	}

	after := e.pending()
	if got := changeKinds(after.Changes); !slices.Equal(got, []protocol.TrelloChangeKind{protocol.TrelloChangeKindAddComment}) {
		t.Errorf("pending kinds = %v, want only the comment, after the rename was dismissed", got)
	}
	if again := e.pending(); len(again.Changes) != 1 {
		t.Errorf("a dismissed change came back on its own: %+v", again.Changes)
	}
	if title := e.board.get(cardID).Title; title != "Draft title" {
		t.Errorf("dismissing changed the Marshal card's title to %q", title)
	}
	if writes := e.trello.writes(); len(writes) != 0 {
		t.Errorf("dismissing wrote to Trello: %+v", writes)
	}
	if len(e.notified) != 1 {
		t.Errorf("dismissing notified %d times, want once, so open screens read the list again", len(e.notified))
	}

	e.board.edit(cardID, ptr("Final title"), nil)
	edited := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	if edited.ID == rename.ID || edited.Detail != "Final title" {
		t.Errorf("an edited change = %+v, want a new id and the new title", edited)
	}
}

func TestPendingForAProjectThatIsNotLinkedIsEmptyNotAnError(t *testing.T) {
	e := newEngine(t)
	e.importedFixLogin()
	bare := newFixture(t)

	for name, call := range map[string]func() (protocol.TrelloPending, error){
		"another project": func() (protocol.TrelloPending, error) {
			return e.f.svc.PendingTrello(context.Background(), "other-repo")
		},
		"no connection at all": func() (protocol.TrelloPending, error) {
			return bare.svc.PendingTrello(context.Background(), engineProject)
		},
	} {
		t.Run(name, func(t *testing.T) {
			pending, err := call()
			if err != nil {
				t.Fatalf("PendingTrello: %v", err)
			}
			if pending.Linked || pending.BoardName != "" || pending.FetchedAt != nil {
				t.Errorf("pending = %+v, want linked false and nothing else", pending)
			}
			if pending.Changes == nil || pending.Cards == nil || len(pending.Changes) != 0 || len(pending.Cards) != 0 {
				t.Errorf("changes = %#v and cards = %#v, want empty lists that are not nil", pending.Changes, pending.Cards)
			}
			if pending.ServerTime.Time().IsZero() {
				t.Error("the answer has no server time")
			}
		})
	}
}

func TestTheLinkedCardsListFollowsTheBoard(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.add("Local only", "")

	pending := e.pending()

	if len(pending.Cards) != 1 || pending.Cards[0].CardID != cardID {
		t.Errorf("linked cards = %+v, want only the card that has a Trello card", pending.Cards)
	}
}
