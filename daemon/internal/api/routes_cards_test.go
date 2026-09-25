package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestCardsThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	created := st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards",
		protocol.CreateCardRequest{Title: "  Add a health check  ", Body: "Serve GET /health."}).
		want(t, http.StatusCreated)
	sameShape(t, "card", created.Body)
	card := decode[protocol.Card](t, created)
	if got, want := created.Header.Get("Location"), "/v1/cards/"+card.ID; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	want := protocol.Card{
		ID: card.ID, ProjectID: project.ID, Number: 1, Key: project.ID + "#1", Title: "Add a health check",
		Body: "Serve GET /health.", State: protocol.CardStateBacklog, Agent: protocol.AgentKindClaude,
		PermissionMode: protocol.PermissionModeAutoEdits, CreatedAt: card.CreatedAt, UpdatedAt: card.UpdatedAt,
	}
	if card != want {
		t.Errorf("card = %+v\nwant  %+v", card, want)
	}

	// The card is marked as made by the person whose token was used, never by anything in the body.
	row, err := st.store.Queries().GetCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("read the card row: %v", err)
	}
	if row.CreatedBy != st.dev.Caller.UserID || row.CreatedBy == "" {
		t.Errorf("created by %q, want the caller's user %q", row.CreatedBy, st.dev.Caller.UserID)
	}

	full := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards", protocol.CreateCardRequest{
		Title: "Fix it", Agent: protocol.AgentKindGemini, Model: "gemini-2.5-pro",
		Thinking: protocol.ThinkingModeHigh, PermissionMode: protocol.PermissionModePlan,
	}).want(t, http.StatusCreated))
	if full.Number != 2 || full.Agent != protocol.AgentKindGemini || full.Model != "gemini-2.5-pro" ||
		full.Thinking == nil || *full.Thinking != protocol.ThinkingModeHigh || full.PermissionMode != protocol.PermissionModePlan {
		t.Errorf("the second card is %+v", full)
	}

	got := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
	if got != want {
		t.Errorf("GET returned %+v, want %+v", got, want)
	}

	board := st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).want(t, http.StatusOK)
	sameShape(t, "board", board.Body)
	cards := decode[protocol.BoardSnapshot](t, board).Cards
	if len(cards) != 2 || cards[0].ID != card.ID || cards[1].ID != full.ID {
		t.Errorf("the board has %+v, want the two cards in number order", cards)
	}
}

func TestCardRefusals(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	path := "/v1/projects/" + project.ID + "/cards"
	tests := []struct {
		name    string
		path    string
		body    any
		status  int
		code    protocol.ErrorCode
		message string
	}{
		{"a card with no title", path, `{"title":"   "}`, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"Give the card a title."},
		{"a title that is too long", path, protocol.CreateCardRequest{Title: strings.Repeat("t", 201)},
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument, "Card titles can have at most 200 characters."},
		{"a description that is too long", path, protocol.CreateCardRequest{Title: "x", Body: strings.Repeat("b", 100*1024+1)},
			http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"That description is too long. Shorten it, or attach a file instead."},
		{"an agent Marshal does not know", path, `{"title":"x","agent":"cursor"}`, http.StatusBadRequest,
			protocol.ErrorCodeInvalidArgument, "Marshal does not know that agent."},
		{"a thinking setting Marshal does not know", path, `{"title":"x","thinking":"deep"}`, http.StatusBadRequest,
			protocol.ErrorCodeInvalidArgument, "That is not a thinking setting Marshal knows."},
		{"a permission mode Marshal does not know", path, `{"title":"x","permissionMode":"yolo"}`, http.StatusBadRequest,
			protocol.ErrorCodeInvalidArgument, "That is not a permission mode Marshal knows."},
		{"a body with no title field", path, `{}`, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"Give the card a title."},
		{"a field Marshal does not know", path, `{"title":"x","state":"done"}`, http.StatusBadRequest,
			protocol.ErrorCodeInvalidArgument, `The field "state" is not one Marshal knows. Remove it and try again.`},
		{"no body", path, nil, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument,
			"The request body is empty. Send a JSON object."},
		{"a project that does not exist", "/v1/projects/nobody/cards", protocol.CreateCardRequest{Title: "x"},
			http.StatusNotFound, protocol.ErrorCodeNotFound, "Marshal cannot find that project. It may have been removed."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := st.do(http.MethodPost, tc.path, tc.body).apiError(t, tc.status, tc.code)
			if got.Message != tc.message {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.message)
			}
		})
	}
	st.do(http.MethodGet, "/v1/cards/01M3C107JB041061050R3GG28A", nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	board := decode[protocol.BoardSnapshot](t, st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).want(t, http.StatusOK))
	if len(board.Cards) != 0 {
		t.Errorf("the refusals made %d cards", len(board.Cards))
	}
}

// Two projects each have a card 1, and each card is read by its own id.
func TestCardsWithTheSameNumberInTwoProjects(t *testing.T) {
	st := newStack(t)
	first, _ := st.addProject("small-repo")
	second, _ := st.addProject("monorepo")
	a := st.addCard(first.ID, "First project card")
	b := st.addCard(second.ID, "Second project card")
	if a.Number != 1 || b.Number != 1 || a.ID == b.ID {
		t.Fatalf("cards %+v and %+v should both be number 1 with different ids", a, b)
	}
	for _, want := range []protocol.Card{a, b} {
		got := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+want.ID, nil).want(t, http.StatusOK))
		if got != want {
			t.Errorf("card %s reads back as %+v, want %+v", want.Key, got, want)
		}
	}
	if a.Key == b.Key {
		t.Errorf("both cards have the key %q", a.Key)
	}
}
