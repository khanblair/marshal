package api_test

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

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
		// A card with no labels carries [], never null.
		Labels: []protocol.Label{},
		// Every card carries its view mode; a new one starts in chat (docs/architecture.md 4.3).
		ViewMode: protocol.CardViewModeChat,
	}
	if !reflect.DeepEqual(card, want) {
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
	if !reflect.DeepEqual(got, want) {
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
		if !reflect.DeepEqual(got, want) {
			t.Errorf("card %s reads back as %+v, want %+v", want.Key, got, want)
		}
	}
	if a.Key == b.Key {
		t.Errorf("both cards have the key %q", a.Key)
	}
}

// A manual move that the rules in architecture.md section 6.1 do not allow answers 422 with the
// stable reason and the sentence the app shows, and the card does not move.
func TestMovingACardByHand(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Move me by hand")

	moved := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/move",
		protocol.MoveCardRequest{State: protocol.CardStateWorking}).want(t, http.StatusOK))
	if moved.State != protocol.CardStateWorking {
		t.Errorf("state = %s, want working", moved.State)
	}

	// To Done is refused by rule 2, with its own sentence.
	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/move",
		protocol.MoveCardRequest{State: protocol.CardStateDone}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != string(protocol.MoveRefusalReasonToDone) {
		t.Errorf("reason = %q, want %q", got.Details["reason"], protocol.MoveRefusalReasonToDone)
	}
	if got.Message != "Cards move to Done by themselves after they merge." {
		t.Errorf("message = %q", got.Message)
	}
	if again := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK)); again.State != protocol.CardStateWorking {
		t.Errorf("the refused move changed the card: %s", again.State)
	}

	// In review needs a pull request, which no card has yet.
	got = st.do(http.MethodPost, "/v1/cards/"+card.ID+"/move",
		protocol.MoveCardRequest{State: protocol.CardStateReview}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != string(protocol.MoveRefusalReasonNeedsPullRequest) {
		t.Errorf("reason = %q, want %q", got.Details["reason"], protocol.MoveRefusalReasonNeedsPullRequest)
	}

	// A column Marshal does not have is a bad request, not a rule refusal.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/move",
		protocol.MoveCardRequest{State: "sideways"}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}

// Editing a card: a field that is not sent is not touched, a date can be set and cleared, and a
// field that is not allowed is refused.
func TestEditingACard(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Before")

	title, body := "After", "A longer description."
	role := "Implementer"
	pkg := "packages/api"
	dueAt := protocol.NewTimestamp(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	due := protocol.DateChange{At: &dueAt}
	updated := decode[protocol.Card](t, st.do(http.MethodPatch, "/v1/cards/"+card.ID, protocol.UpdateCardRequest{
		Title: &title, Body: &body, Role: &role, Package: &pkg, Due: &due,
	}).want(t, http.StatusOK))
	if updated.Title != "After" || updated.Body != body || updated.Role != role || updated.Package != pkg {
		t.Errorf("the edit did not apply: %+v", updated)
	}
	if updated.Due == nil || updated.Due.Time().UTC() != dueAt.Time().UTC() {
		t.Errorf("due = %v, want %v", updated.Due, dueAt.Time())
	}
	// What was not sent is untouched.
	if updated.ID != card.ID || updated.Number != card.Number || updated.Key != card.Key ||
		updated.State != card.State || updated.Agent != card.Agent || updated.PermissionMode != card.PermissionMode {
		t.Errorf("the edit changed a field it was not given: %+v", updated)
	}
	// The history of the change is published, and applying it twice ends in the same place.
	again := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
	if !reflect.DeepEqual(again, updated) {
		t.Errorf("the card reads back as %+v, want %+v", again, updated)
	}
	// Clearing the date is its own request, because an absent field means "leave it".
	cleared := decode[protocol.Card](t, st.do(http.MethodPatch, "/v1/cards/"+card.ID,
		protocol.UpdateCardRequest{Due: &protocol.DateChange{Clear: true}}).want(t, http.StatusOK))
	if cleared.Due != nil {
		t.Errorf("due = %v, want it cleared", cleared.Due)
	}
	if cleared.Title != "After" {
		t.Errorf("clearing the date changed the title: %q", cleared.Title)
	}

	empty, unknownAgent := "", protocol.AgentKind("gpt")
	st.do(http.MethodPatch, "/v1/cards/"+card.ID, protocol.UpdateCardRequest{Title: &empty}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPatch, "/v1/cards/"+card.ID, protocol.UpdateCardRequest{Agent: &unknownAgent}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodPatch, "/v1/cards/01M3C107JB041061050R3GG28A", protocol.UpdateCardRequest{Title: &title}).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}

// Deleting a card by hand removes it, and reading it afterwards is not found.
func TestDeletingACard(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Delete me")

	st.do(http.MethodDelete, "/v1/cards/"+card.ID, nil).want(t, http.StatusNoContent)
	st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	// Deleting it again is not found, not a second success.
	st.do(http.MethodDelete, "/v1/cards/"+card.ID, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	// The board no longer lists it.
	board := decode[protocol.BoardSnapshot](t, st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).want(t, http.StatusOK))
	if len(board.Cards) != 0 {
		t.Errorf("the board still lists %d cards", len(board.Cards))
	}
}

// A card asked for in Working or Planning starts its session as it is added, and is never shown as
// working before its agent exists.
func TestCreatingACardInAStartedColumn(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	working := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards",
		protocol.CreateCardRequest{Title: "Start me", StartState: protocol.CardStateWorking}).want(t, http.StatusCreated))
	if working.State != protocol.CardStateWorking {
		t.Errorf("state = %s, want working", working.State)
	}
	if row, err := st.store.Queries().GetSessionByCard(context.Background(), working.ID); err != nil || row.State != string(protocol.SessionStateAwake) {
		t.Errorf("session = %+v, %v; want an awake session", row, err)
	}

	planning := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards",
		protocol.CreateCardRequest{Title: "Plan me", StartState: protocol.CardStatePlanning}).want(t, http.StatusCreated))
	if planning.State != protocol.CardStatePlanning {
		t.Errorf("state = %s, want planning", planning.State)
	}
	if row, err := st.store.Queries().GetSessionByCard(context.Background(), planning.ID); err != nil || row.State != string(protocol.SessionStateAwake) {
		t.Errorf("session = %+v, %v; want a running session behind the plan", row, err)
	}

	// A column a card cannot be added to is a bad request.
	st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards",
		protocol.CreateCardRequest{Title: "No", StartState: protocol.CardStateDone}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	// The default is still the backlog, with no session started.
	plain := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards",
		protocol.CreateCardRequest{Title: "Plain"}).want(t, http.StatusCreated))
	if plain.State != protocol.CardStateBacklog {
		t.Errorf("state = %s, want backlog", plain.State)
	}
	if _, err := st.store.Queries().GetSessionByCard(context.Background(), plain.ID); err == nil {
		t.Error("a card added to the backlog started a session")
	}
}

// With no session manager there is nothing to start an agent with, so a start state is refused
// before anything is written, and the route stays registered.
func TestCreatingACardInAStartedColumnWithoutSessions(t *testing.T) {
	st := newStack(t, withoutSessions())
	project, _ := st.addProject("small-repo")
	st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards",
		protocol.CreateCardRequest{Title: "Start me", StartState: protocol.CardStateWorking}).
		apiError(t, http.StatusServiceUnavailable, protocol.ErrorCodeUnavailable)
	// Nothing was written.
	board := decode[protocol.BoardSnapshot](t, st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).want(t, http.StatusOK))
	if len(board.Cards) != 0 {
		t.Errorf("a card was written anyway: %+v", board.Cards)
	}
}

// Forking a card by hand: a new card in the backlog that starts from the card it came from. A card
// that never started has nothing to fork from.
func TestForkingACard(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Fork me")

	// Nothing to fork from yet.
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/fork", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)

	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(t, http.StatusOK)
	fork := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/fork", nil).want(t, http.StatusCreated))
	if fork.Title != "Fork me (fork)" || fork.State != protocol.CardStateBacklog {
		t.Errorf("fork = %+v", fork)
	}
	if fork.ID == card.ID || fork.Number == card.Number {
		t.Errorf("the fork is the source card: %+v", fork)
	}
	// Both cards are on the board.
	board := decode[protocol.BoardSnapshot](t, st.do(http.MethodGet, "/v1/projects/"+project.ID+"/board", nil).want(t, http.StatusOK))
	if len(board.Cards) != 2 {
		t.Errorf("the board lists %d cards, want 2", len(board.Cards))
	}
	// The fork can be started, and it gets its own branch.
	started := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/cards/"+fork.ID+"/start", nil).want(t, http.StatusOK))
	if started.Branch == "" || started.Branch == card.Branch {
		t.Errorf("the fork's branch = %q, want its own", started.Branch)
	}
	// Forking a card that is not there is not found.
	st.do(http.MethodPost, "/v1/cards/01M3C107JB041061050R3GG28A/fork", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
}
