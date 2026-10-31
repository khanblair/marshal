package integrations_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Apply is the only place Marshal writes to Trello. It writes the changes the person names, as the
// linked person, after a live check that Trello did not change the same thing meanwhile. The reasons
// below are phrases from the sentences a person reads when a change is skipped.

const (
	skipConflict = "changed this since Marshal last read it"
	skipGone     = "gone from Trello"
	skipStale    = "no longer pending"
	skipArchived = "archived"
	skipToken    = "refused the token"
	skipSlow     = "slow down"
)

// skipReasons is each skipped change's reason, by change id.
func skipReasons(result protocol.ApplyTrelloResult) map[string]string {
	out := map[string]string{}
	for _, s := range result.Skipped {
		out[s.ChangeID] = s.Reason
	}
	return out
}

func wantSkipped(t *testing.T, result protocol.ApplyTrelloResult, id, phrase string) {
	t.Helper()
	reason, ok := skipReasons(result)[id]
	if !ok || !strings.Contains(reason, phrase) {
		t.Errorf("change %s: skipped = %+v, want a reason with %q", id, result.Skipped, phrase)
	}
}

func wantApplied(t *testing.T, result protocol.ApplyTrelloResult, ids ...string) {
	t.Helper()
	if !slices.Equal(result.Applied, ids) {
		t.Errorf("applied = %v, want %v (skipped: %+v)", result.Applied, ids, result.Skipped)
	}
}

// threeRenamed imports three plain Trello cards, tc-1 to tc-3, and renames each in Marshal. It answers
// the three rename changes in the order they are listed.
func threeRenamed(e *engine) []protocol.TrelloChange {
	e.t.Helper()
	for i, name := range []string{"One", "Two", "Three"} {
		e.trello.addCard(fmt.Sprintf("tc-%d", i+1), "list-todo", name, "")
	}
	e.refresh()
	for i := 1; i <= 3; i++ {
		e.board.edit(e.marshalCard(fmt.Sprintf("tc-%d", i)), ptr(fmt.Sprintf("Renamed %d", i)), nil)
	}
	changes := e.pending().Changes
	if len(changes) != 3 {
		e.t.Fatalf("pending = %+v, want the three renames", changes)
	}
	return changes
}

func TestApplyWritesEachKindOfChangeToTrello(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	newCardID := editEverything(e, cardID)
	ids := changeIDs(e.pending().Changes)
	e.trello.clearLog()

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: ids})

	wantApplied(t, result, ids...)
	if len(result.Skipped) != 0 {
		t.Errorf("skipped = %+v, want nothing skipped", result.Skipped)
	}
	fix := e.trello.card("tc-1")
	if fix.Name != "Fix login in Safari" || fix.Desc != "Users cannot log in" || fix.IDList != "list-done" {
		t.Errorf("card = %q / %q on %q, want the new title, the same description, on the Done list", fix.Name, fix.Desc, fix.IDList)
	}
	if len(fix.Checklists) != 2 {
		t.Fatalf("checklists = %+v, want Steps and the new Release only, not the empty one", fix.Checklists)
	}
	wantItems(t, fix.Checklists[0], "reproduce:complete", "write test:complete", "deploy:incomplete")
	wantItems(t, fix.Checklists[1], "tag it:incomplete", "announce:incomplete")
	// The text is exactly what Marshal held: no agent name and no prefix, whoever wrote it.
	wantComments := []string{"Looks like a cookie issue", "Same here", "Patched the cookie", "Tests are green"}
	if got := e.trello.commentTexts(); !slices.Equal(got, wantComments) {
		t.Errorf("Trello's comments = %q, want %q", got, wantComments)
	}
	made := e.trello.cardNamed("Brand new")
	if made.Desc != "Only in Marshal" || made.IDList != "list-todo" {
		t.Errorf("new card = %+v, want its description, on the first list that is not done", made)
	}
	if got := e.marshalCard(made.ID); got != newCardID {
		t.Errorf("the new Trello card is linked to %q, want the Marshal card it was made from, %q", got, newCardID)
	}
	wantWrites := map[string]int{
		"PUT /cards/tc-1":                                          2, // the title, then the move to Done
		"PUT /cards/tc-1/checkItem/tci-1":                          1,
		"POST /checklists/tcl-1/checkItems":                        1,
		"POST /checklists":                                         1,
		"POST /checklists/" + fix.Checklists[1].ID + "/checkItems": 2,
		"POST /cards/tc-1/actions/comments":                        2,
		"POST /cards":                                              1,
	}
	if got := e.trello.writeCounts(); !maps.Equal(got, wantWrites) {
		t.Errorf("writes = %v, want %v", got, wantWrites)
	}
}

func wantItems(t *testing.T, list trello.Checklist, want ...string) {
	t.Helper()
	got := []string{}
	for _, it := range list.Items {
		got = append(got, it.Name+":"+it.State)
	}
	if !slices.Equal(got, want) {
		t.Errorf("checklist %q = %v, want %v", list.Name, got, want)
	}
}

func TestAnAppliedChangeStopsBeingPendingAndTheNextRefreshAgrees(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	newCardID := editEverything(e, cardID)

	result := e.applyAll()

	if got := len(result.Pending.Changes); got != 0 {
		t.Errorf("%d changes are still pending after applying all: %+v", got, result.Pending.Changes)
	}
	var fromMarshal []string
	for _, c := range result.Pending.Cards {
		if !c.FromTrello {
			fromMarshal = append(fromMarshal, c.CardID)
		}
	}
	if !slices.Equal(fromMarshal, []string{newCardID}) {
		t.Errorf("cards pushed to Trello = %v, want the one made in Marshal, %s", fromMarshal, newCardID)
	}

	// Reading Trello back finds what Marshal just wrote and takes nothing from it, and the comments
	// Marshal posted are not copied back in as new ones.
	e.board.calls = nil
	counts := e.refresh()
	if counts.Imported != 0 || counts.Updated != 0 {
		t.Errorf("counts = %+v, want a refresh after an apply to find nothing new", counts)
	}
	if calls := e.board.callList(); len(calls) != 0 {
		t.Errorf("the board was asked to %q, want nothing", calls)
	}
	if got := len(e.comments(cardID)); got != 4 {
		t.Errorf("the card has %d comments, want the 2 imported and the 2 written, with no echo", got)
	}
	if got := len(e.pending().Changes); got != 0 {
		t.Errorf("%d changes are pending after the refresh", got)
	}
}

func TestApplyLeavesAChangeThatWasNotNamedUntouched(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Renamed in Marshal"), nil)
	e.say(cardID, "Only this one goes")
	pending := e.pending()
	comment := e.changeOf(pending, protocol.TrelloChangeKindAddComment, cardID)
	rename := e.changeOf(pending, protocol.TrelloChangeKindUpdateCard, cardID)

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{comment.ID}})

	wantApplied(t, result, comment.ID)
	if name := e.trello.card("tc-1").Name; name != "Fix login" {
		t.Errorf("Trello's title = %q, want it untouched", name)
	}
	if got := changeIDs(result.Pending.Changes); !slices.Equal(got, []string{rename.ID}) {
		t.Errorf("still pending = %v, want only the rename", got)
	}
}

func TestApplyWithNoChangeNamedWritesNothing(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Renamed in Marshal"), nil)
	rename := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	e.trello.clearLog()

	// An override on its own names no change to write.
	result := e.apply(protocol.ApplyTrelloRequest{OverrideIDs: []string{rename.ID}})

	if len(result.Applied) != 0 || len(result.Skipped) != 0 {
		t.Errorf("result = %+v, want nothing applied and nothing skipped", result)
	}
	if writes := e.trello.writes(); len(writes) != 0 {
		t.Errorf("writes = %+v, want none", writes)
	}
}

func TestApplyOfAnUnknownOrFinishedChangeIsSkippedAsNotPending(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Renamed in Marshal"), nil)
	rename := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	e.trello.clearLog()

	unknown := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{"not-a-change"}})
	wantSkipped(t, unknown, "not-a-change", skipStale)
	if len(unknown.Applied) != 0 || len(e.trello.writes()) != 0 {
		t.Errorf("an unknown id applied %v and wrote %+v", unknown.Applied, e.trello.writes())
	}

	first := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{rename.ID}})
	wantApplied(t, first, rename.ID)
	again := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{rename.ID}})
	wantSkipped(t, again, rename.ID, skipStale)
	if got := e.trello.writeCounts()["PUT /cards/tc-1"]; got != 1 {
		t.Errorf("the rename was written %d times, want once", got)
	}
}

func TestApplyOfADismissedChangeIsSkippedAsNotPending(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Renamed in Marshal"), nil)
	rename := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	if err := e.f.svc.DismissTrello(t.Context(), engineProject, []string{rename.ID}); err != nil {
		t.Fatalf("DismissTrello: %v", err)
	}

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{rename.ID}})

	wantSkipped(t, result, rename.ID, skipStale)
	if writes := e.trello.writes(); len(writes) != 0 {
		t.Errorf("a dismissed change was written: %+v", writes)
	}
}

func TestApplySkipsAConflictTheLastReadFoundUnlessItIsOverridden(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Fix login in Safari"), nil)
	e.trello.edit("tc-1", func(c *trello.Card) { c.Name = "Fix login (urgent)" })
	e.refresh()
	rename := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	if !rename.Conflict {
		t.Fatalf("the setup made no conflict: %+v", rename)
	}

	skipped := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{rename.ID}})

	wantSkipped(t, skipped, rename.ID, skipConflict)
	if name := e.trello.card("tc-1").Name; name != "Fix login (urgent)" {
		t.Errorf("Trello's title = %q, want Trello's own kept", name)
	}
	if got := changeIDs(skipped.Pending.Changes); !slices.Equal(got, []string{rename.ID}) {
		t.Errorf("still pending = %v, want the skipped rename", got)
	}

	forced := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{rename.ID}, OverrideIDs: []string{rename.ID}})

	wantApplied(t, forced, rename.ID)
	if name := e.trello.card("tc-1").Name; name != "Fix login in Safari" {
		t.Errorf("Trello's title = %q, want Marshal's after the override", name)
	}
	if got := len(forced.Pending.Changes); got != 0 {
		t.Errorf("%d changes are pending after the override", got)
	}
}

// liveConflicts are edits made in Trello after Marshal last read the board, so the list does not know
// them yet. Apply asks Trello again before it writes.
var liveConflicts = []struct {
	name    string
	kind    protocol.TrelloChangeKind
	marshal func(e *engine, cardID string)
	trello  func(e *engine)
	// kept says Trello still holds its own edit, and written says the apply went through.
	kept, written func(c trello.Card) bool
}{
	{
		name: "a rename", kind: protocol.TrelloChangeKindUpdateCard,
		marshal: func(e *engine, cardID string) { e.board.edit(cardID, ptr("Marshal title"), nil) },
		trello:  func(e *engine) { e.trello.edit("tc-1", func(c *trello.Card) { c.Name = "Trello title" }) },
		kept:    func(c trello.Card) bool { return c.Name == "Trello title" },
		written: func(c trello.Card) bool { return c.Name == "Marshal title" },
	},
	{
		name: "a description", kind: protocol.TrelloChangeKindUpdateCard,
		marshal: func(e *engine, cardID string) { e.board.edit(cardID, nil, ptr("Marshal words")) },
		trello:  func(e *engine) { e.trello.edit("tc-1", func(c *trello.Card) { c.Desc = "Trello words" }) },
		kept:    func(c trello.Card) bool { return c.Desc == "Trello words" },
		written: func(c trello.Card) bool { return c.Desc == "Marshal words" },
	},
	{
		name: "a move to done", kind: protocol.TrelloChangeKindMoveDone,
		marshal: func(e *engine, cardID string) { e.board.setState(cardID, protocol.CardStateDone) },
		trello:  func(e *engine) { e.trello.edit("tc-1", func(c *trello.Card) { c.IDList = "list-doing" }) },
		kept:    func(c trello.Card) bool { return c.IDList == "list-doing" },
		written: func(c trello.Card) bool { return c.IDList == "list-done" },
	},
	{
		name: "a tick", kind: protocol.TrelloChangeKindSetItem,
		marshal: func(e *engine, cardID string) {
			steps := e.checklists(cardID)[0]
			e.tick(cardID, steps.ID, steps.Items[0].ID, true)
		},
		trello:  func(e *engine) { e.trello.tick("tc-1", "tci-1", true) },
		kept:    func(c trello.Card) bool { return c.Checklists[0].Items[0].State == "complete" },
		written: func(c trello.Card) bool { return c.Checklists[0].Items[0].State == "complete" },
	},
}

func TestApplyChecksTrelloAgainBeforeItWrites(t *testing.T) {
	for _, tt := range liveConflicts {
		t.Run(tt.name, func(t *testing.T) {
			e := newEngine(t)
			cardID := e.importedFixLogin()
			tt.marshal(e, cardID)
			change := e.changeOf(e.pending(), tt.kind, cardID)
			tt.trello(e)
			e.trello.clearLog()

			skipped := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{change.ID}})

			wantSkipped(t, skipped, change.ID, skipConflict)
			if writes := e.trello.writes(); len(writes) != 0 {
				t.Errorf("a change in conflict was written: %+v", writes)
			}
			if !tt.kept(e.trello.card("tc-1")) {
				t.Error("Trello's own edit was lost")
			}

			forced := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{change.ID}, OverrideIDs: []string{change.ID}})

			wantApplied(t, forced, change.ID)
			if !tt.written(e.trello.card("tc-1")) {
				t.Error("the override did not write the change")
			}
		})
	}
}

func TestApplyDoesNotCallAnEditToSomethingElseAConflict(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Renamed in Marshal"), nil)
	rename := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	// Trello changed the description, a comment, and a line, none of which the rename touches.
	e.trello.edit("tc-1", func(c *trello.Card) { c.Desc = "Trello's new words" })
	e.trello.comment("tc-1", "Linus", "A comment")
	e.trello.tick("tc-1", "tci-1", true)

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{rename.ID}})

	wantApplied(t, result, rename.ID)
	if got := e.trello.card("tc-1"); got.Name != "Renamed in Marshal" || got.Desc != "Trello's new words" {
		t.Errorf("card = %q / %q, want the new title written and Trello's description kept", got.Name, got.Desc)
	}
}

func TestApplySkipsAnArchivedTrelloCard(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Renamed in Marshal"), nil)
	rename := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	e.trello.edit("tc-1", func(c *trello.Card) { c.Closed = true })
	e.trello.clearLog()

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{rename.ID}, OverrideIDs: []string{rename.ID}})

	wantSkipped(t, result, rename.ID, skipArchived)
	if writes := e.trello.writes(); len(writes) != 0 {
		t.Errorf("a change was written to an archived card: %+v", writes)
	}
}

func TestApplySkipsAChangeWhoseTrelloCardIsGoneAndGoesOn(t *testing.T) {
	tests := []struct {
		name string
		gone func(e *engine)
	}{
		{"the card was deleted", func(e *engine) { e.trello.remove("tc-1") }},
		{"the write found nothing", func(e *engine) { e.trello.failWith("PUT", "/cards/tc-1", 404) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEngine(t)
			changes := threeRenamed(e)
			tt.gone(e)

			result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: changeIDs(changes)})

			wantSkipped(t, result, changes[0].ID, skipGone)
			wantApplied(t, result, changes[1].ID, changes[2].ID)
			if got := e.trello.card("tc-3").Name; got != "Renamed 3" {
				t.Errorf("the card after the gone one is %q, want it renamed too", got)
			}
		})
	}
}

func TestApplyStopsTheRestWhenTrelloRefusesTheTokenOrAsksToSlowDown(t *testing.T) {
	tests := []struct {
		status int
		phrase string
	}{
		{401, skipToken},
		{403, skipToken},
		{429, skipSlow},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status, " on a write"), func(t *testing.T) {
			e := newEngine(t)
			changes := threeRenamed(e)
			e.trello.failWith("PUT", "/cards/tc-2", tt.status)
			e.trello.clearLog()

			result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: changeIDs(changes)})

			wantApplied(t, result, changes[0].ID)
			wantSkipped(t, result, changes[1].ID, tt.phrase)
			wantSkipped(t, result, changes[2].ID, tt.phrase)
			if got := e.trello.card("tc-3").Name; got != "Three" {
				t.Errorf("the third card is %q, want it left alone once Trello said stop", got)
			}
			if asked := e.trello.count("GET", "/cards/tc-3"); asked != 0 {
				t.Errorf("Trello was asked about the third card %d times after it said stop", asked)
			}
			if got, want := changeIDs(result.Pending.Changes), []string{changes[1].ID, changes[2].ID}; !slices.Equal(got, want) {
				t.Errorf("still pending = %v, want the two that were not written, %v", got, want)
			}
		})
		t.Run(fmt.Sprint(tt.status, " on the first read"), func(t *testing.T) {
			e := newEngine(t)
			changes := threeRenamed(e)
			e.trello.failWith("GET", "/cards/tc-1", tt.status)
			e.trello.clearLog()

			result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: changeIDs(changes)})

			if len(result.Applied) != 0 {
				t.Errorf("applied = %v, want nothing", result.Applied)
			}
			for _, c := range changes {
				wantSkipped(t, result, c.ID, tt.phrase)
			}
			if writes := e.trello.writes(); len(writes) != 0 {
				t.Errorf("writes = %+v, want none", writes)
			}
		})
	}
}

func TestApplyOfANewCardUsesTheListNewCardsGoTo(t *testing.T) {
	e := newEngine(t, func(save *protocol.SaveTrelloRequest) { save.NewCardListID = "list-doing" })
	e.importedFixLogin()
	newCardID := e.board.add("Brand new", "")
	change := e.changeOf(e.pending(), protocol.TrelloChangeKindCreateCard, newCardID)

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{change.ID}})

	wantApplied(t, result, change.ID)
	if made := e.trello.cardNamed("Brand new"); made.IDList != "list-doing" {
		t.Errorf("new card is on %q, want the list new cards go to", made.IDList)
	}
}

func TestApplyOfAMoveToDoneNeedsAListThatReadsAsDone(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.trello.setLists(trello.List{ID: "list-todo", Name: "To do"}, trello.List{ID: "list-doing", Name: "Doing"})
	e.board.setState(cardID, protocol.CardStateDone)
	move := e.changeOf(e.pending(), protocol.TrelloChangeKindMoveDone, cardID)

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{move.ID}})

	wantSkipped(t, result, move.ID, "no list that reads as done")
	if list := e.trello.card("tc-1").IDList; list != "list-todo" {
		t.Errorf("the card moved to %q on a board with no done list", list)
	}
}

func TestApplyWithNoListToPutANewCardOnSkipsIt(t *testing.T) {
	e := newEngine(t)
	e.importedFixLogin()
	e.trello.setLists()
	newCardID := e.board.add("Brand new", "")
	change := e.changeOf(e.pending(), protocol.TrelloChangeKindCreateCard, newCardID)

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{change.ID}})

	wantSkipped(t, result, change.ID, "no list")
	if got := e.trello.cardCount(); got != 1 {
		t.Errorf("Trello holds %d cards, want the one it had", got)
	}
}

func TestApplyOfAProjectThatIsNotLinkedIsRefused(t *testing.T) {
	e := newEngine(t)

	_, err := e.f.svc.ApplyTrello(t.Context(), "other-repo", protocol.ApplyTrelloRequest{ChangeIDs: []string{"x"}})

	if err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Errorf("error = %v, want a refusal that the project is not linked", err)
	}
}

func TestAnUnreachableTrelloErrorShownForAWriteHoldsNoCredential(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Renamed in Marshal"), nil)
	rename := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)
	e.trello.Close()

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{rename.ID}})

	reason := skipReasons(result)[rename.ID]
	if reason == "" {
		t.Fatalf("result = %+v, want the change skipped", result)
	}
	for _, secret := range []string{fakeKey, fakeToken} {
		if strings.Contains(reason, secret) {
			t.Errorf("the reason %q holds %q", reason, secret)
		}
	}
}
