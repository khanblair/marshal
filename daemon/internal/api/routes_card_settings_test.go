package api_test

import (
	"net/http"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A card's settings can be changed while it runs. The edit is written into the card's own chat as a
// system note (N8), so the person can see that the next turn runs with the new setting. These
// routes read the same history the session manager writes, so a note is a real row of a real card.

func TestChangingASettingIsSaidInTheCardsChat(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Switch the model")
	// A card a session has run for has a session row and a chat; every stored event points at one.
	addHistory(t, st, card.ID, history.Record{Kind: history.KindUser, Summary: "Go."})

	model := "opus"
	st.do(http.MethodPatch, "/v1/cards/"+card.ID, protocol.UpdateCardRequest{Model: &model}).
		want(t, http.StatusOK)

	notes := systemNotes(t, st, card.ID)
	if len(notes) != 1 || notes[0].Text != "Model set to opus. It takes effect on the next turn." {
		t.Errorf("the card's system notes = %+v, want one note about the model", notes)
	}
}

func TestAnEditThatChangesNoSettingSaysNothing(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Rename me")
	addHistory(t, st, card.ID, history.Record{Kind: history.KindUser, Summary: "Go."})

	title := "Rename me, too"
	st.do(http.MethodPatch, "/v1/cards/"+card.ID, protocol.UpdateCardRequest{Title: &title}).
		want(t, http.StatusOK)

	if notes := systemNotes(t, st, card.ID); len(notes) != 0 {
		t.Errorf("a title change wrote system notes: %+v", notes)
	}
}

// systemNotes reads a card's chat and returns its system notes, newest first.
func systemNotes(t *testing.T, st *stack, cardID string) []protocol.ChatMessage {
	t.Helper()
	page := decode[protocol.Page[protocol.ChatMessage]](t,
		st.do(http.MethodGet, "/v1/cards/"+cardID+"/messages", nil).want(t, http.StatusOK))
	var notes []protocol.ChatMessage
	for _, m := range page.Items {
		if m.Kind == protocol.ChatMessageKindSystem {
			notes = append(notes, m)
		}
	}
	return notes
}
