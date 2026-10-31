package integrations_test

import (
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Each test here describes what the Trello sync should do and does not do yet. They are skipped so the
// suite stays green. Fix the code, remove the Skip line, and the test is the regression test.

func wantNoCredential(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{fakeKey, fakeToken} {
		if strings.Contains(text, secret) {
			t.Errorf("%q holds %q", text, secret)
		}
	}
}

func TestApplyOfANewCardOnAnUnreachableTrelloShowsNoCredential(t *testing.T) {
	e := newEngine(t)
	e.importedFixLogin()
	newCardID := e.board.add("Brand new", "")
	change := e.changeOf(e.pending(), protocol.TrelloChangeKindCreateCard, newCardID)
	e.trello.Close()

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{change.ID}})

	wantNoCredential(t, skipReasons(result)[change.ID])
}

func TestARefreshOfAnUnreachableTrelloReturnsNoCredential(t *testing.T) {
	e := newEngine(t)
	e.trello.Close()

	_, err := e.f.svc.RefreshTrello(t.Context(), engineProject, nil)

	if err == nil {
		t.Fatal("a refresh of an unreachable Trello succeeded")
	}
	wantNoCredential(t, err.Error())
}

func TestApplyStopsTheRestWhenAMoveOrACreateIsToldToStop(t *testing.T) {
	tests := []struct {
		name         string
		method, path string
		status       int
		phrase       string
		setup        func(e *engine) []protocol.TrelloChange
	}{
		{"a move told to slow down", "PUT", "/cards/tc-1", 429, skipSlow, func(e *engine) []protocol.TrelloChange {
			e.trello.addCard("tc-1", "list-todo", "One", "")
			e.trello.addCard("tc-2", "list-todo", "Two", "")
			e.refresh()
			e.board.setState(e.marshalCard("tc-1"), protocol.CardStateDone)
			e.board.setState(e.marshalCard("tc-2"), protocol.CardStateDone)
			return e.pending().Changes
		}},
		{"a create refused the token", "POST", "/cards", 401, skipToken, func(e *engine) []protocol.TrelloChange {
			e.board.add("One", "")
			e.board.add("Two", "")
			return e.pending().Changes
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEngine(t)
			changes := tt.setup(e)
			e.trello.failWith(tt.method, tt.path, tt.status)

			result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: changeIDs(changes)})

			if len(result.Applied) != 0 {
				t.Errorf("applied = %v, want the second change held back once Trello said stop", result.Applied)
			}
			for _, c := range changes {
				wantSkipped(t, result, c.ID, tt.phrase)
			}
		})
	}
}

func TestRefreshAgreesWhenMarshalAndTrelloBothMovedACardToDone(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.setState(cardID, protocol.CardStateDone)
	e.trello.edit("tc-1", func(c *trello.Card) { c.IDList = "list-done" })

	e.refresh()

	if got := e.pending().Changes; len(got) != 0 {
		t.Errorf("pending = %+v, want nothing: the card is done on both sides", got)
	}
}

func TestPendingFlagsAWordsChangeWhenTrelloChangedTheDescription(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("New title"), ptr("New description"))
	e.trello.edit("tc-1", func(c *trello.Card) { c.Desc = "Trello's description" })
	e.refresh()

	change := e.changeOf(e.pending(), protocol.TrelloChangeKindUpdateCard, cardID)

	if !change.Conflict {
		t.Errorf("change = %+v, want a conflict: Trello changed the description Marshal also changed", change)
	}
}

func TestApplyingOneChangeKeepsTheConflictNoteOfAnother(t *testing.T) {
	e := newEngine(t)
	cardID := e.importedFixLogin()
	e.board.edit(cardID, ptr("Marshal title"), nil)
	e.trello.edit("tc-1", func(c *trello.Card) { c.Name = "Trello title" })
	e.refresh()
	e.say(cardID, "A note")
	comment := e.changeOf(e.pending(), protocol.TrelloChangeKindAddComment, cardID)

	result := e.apply(protocol.ApplyTrelloRequest{ChangeIDs: []string{comment.ID}})

	if rename := e.changeOf(result.Pending, protocol.TrelloChangeKindUpdateCard, cardID); !rename.Conflict {
		t.Errorf("rename = %+v, want its conflict kept: Trello still holds a different title", rename)
	}
}
